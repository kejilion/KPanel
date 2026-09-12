package systemmanage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	diskJobFileLimit     = 64 << 10
	diskRequestFileLimit = 16 << 10
	diskReceiptLimit     = 16 << 10
	diskJobLaunchGrace   = 10 * time.Second
	diskJobLaunchTimeout = 2 * time.Minute
)

var (
	diskJobIDPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
	diskMajorMinorExpr = regexp.MustCompile(`^[0-9]{1,7}:[0-9]{1,7}$`)
	diskReceiptHexExpr = regexp.MustCompile(`^[a-f0-9]*$`)
)

type diskJobRecord struct {
	ID                      string                              `json:"id"`
	Request                 contract.DiskPartitionActionRequest `json:"request"`
	Target                  diskTarget                          `json:"target"`
	ExpectedResourceVersion string                              `json:"expectedResourceVersion"`
}

type diskJobReceipt struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Stage      string    `json:"stage"`
	Message    string    `json:"message"`
	BackupPath string    `json:"backupPath,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`
}

func (m *Manager) DiskPartitionCapabilities() []contract.Capability {
	readErr := error(nil)
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		readErr = errors.New("磁盘检查需要以 root 运行的 Linux Agent")
	} else {
		for _, tool := range []string{"lsblk", "systemd-run"} {
			if _, err := m.runner.LookPath(tool); err != nil {
				readErr = fmt.Errorf("%s 不可用", tool)
				break
			}
		}
		if readErr == nil {
			_, readErr = m.backgroundExecutable()
		}
	}
	read := contract.Capability{ID: "system.disk-partitions.read", Enabled: readErr == nil, Methods: []string{"GET"}}
	if readErr != nil {
		read.Methods = nil
		read.Reason = readErr.Error()
	}
	writeErr := readErr
	if writeErr == nil && !m.enabled {
		writeErr = errors.New("宿主机系统写入开关未启用")
	}
	if writeErr == nil {
		platform := m.diskPlatform()
		if !platform.Writable {
			writeErr = errors.New(platform.Reason)
		}
	}
	if writeErr == nil {
		writeErr = m.diskWriteAvailability()
	}
	write := contract.Capability{ID: "system.disk-partitions.write", Enabled: writeErr == nil, Methods: []string{"POST"}}
	if writeErr != nil {
		write.Methods = nil
		write.Reason = capabilityReason(writeErr)
	}
	return []contract.Capability{read, write}
}

// StartDiskPartitionAction validates against a fresh privileged snapshot,
// persists a single durable job and only then detaches the transient worker.
func (m *Manager) StartDiskPartitionAction(ctx context.Context, request contract.DiskPartitionActionRequest) (contract.DiskPartitionJob, error) {
	if !m.enabled {
		return contract.DiskPartitionJob{}, ErrDisabled
	}
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return contract.DiskPartitionJob{}, ErrUnsupported
	}
	if field, detail := contract.ValidateDiskPartitionAction(&request); field != "" {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	if err := m.diskWriteAvailability(); err != nil {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	unlock, err := m.acquireDiskProcessLock(ctx)
	if err != nil {
		return contract.DiskPartitionJob{}, err
	}
	defer unlock()
	envelope, err := m.inspectDiskPartitions(ctx)
	if err != nil {
		return contract.DiskPartitionJob{}, err
	}
	if !envelope.Snapshot.Platform.Writable {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: %s", ErrUnsupported, envelope.Snapshot.Platform.Reason)
	}
	if envelope.Snapshot.ResourceVersion != request.ExpectedResourceVersion {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: disk inventory changed", ErrConflict)
	}
	var device *contract.DiskDevice
	var target diskTarget
	for index := range envelope.Snapshot.Devices {
		if envelope.Snapshot.Devices[index].ID == request.DeviceID {
			device = &envelope.Snapshot.Devices[index]
			break
		}
	}
	for _, candidate := range envelope.Targets {
		if candidate.ID == request.DeviceID {
			target = candidate
			break
		}
	}
	if device == nil || target.ID == "" {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: disk device is unavailable", ErrConflict)
	}
	operation, ok := device.Operations[request.Action]
	if !ok || !operation.Enabled {
		reason := strings.TrimSpace(operation.Reason)
		if reason == "" {
			reason = "operation is disabled"
		}
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: %s", ErrConflict, reason)
	}
	if err := m.validateDiskActionTarget(request, *device, target, envelope); err != nil {
		return contract.DiskPartitionJob{}, err
	}
	if current := m.readDiskJob(); current != nil && (current.Status == "queued" || current.Status == "running") {
		m.reconcileDiskJob(current)
		if current.Status == "queued" || current.Status == "running" {
			return contract.DiskPartitionJob{}, fmt.Errorf("%w: another disk job is active", ErrConflict)
		}
	}
	if !diskMajorMinorExpr.MatchString(target.MajorMinor) {
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: device major:minor is invalid", ErrConflict)
	}
	if err := m.ensureDiskJobDir(); err != nil {
		return contract.DiskPartitionJob{}, fmt.Errorf("persist disk job: %w", err)
	}
	now := m.now().UTC()
	job := contract.DiskPartitionJob{
		ID: randomDiskID(), Action: request.Action, DeviceID: request.DeviceID, DevicePath: target.Path,
		Status: "queued", Stage: "launching", Progress: 2, Message: "磁盘任务正在安全启动", CreatedAt: now,
	}
	record := diskJobRecord{ID: job.ID, Request: request, Target: target, ExpectedResourceVersion: envelope.Snapshot.ResourceVersion}
	if err := m.writeDiskRecord(record); err != nil {
		return contract.DiskPartitionJob{}, fmt.Errorf("persist disk request: %w", err)
	}
	_ = os.Remove(m.diskReceiptPath())
	if err := m.writeDiskJob(job); err != nil {
		return contract.DiskPartitionJob{}, fmt.Errorf("persist disk job: %w", err)
	}
	if err := m.launchDiskJob(ctx, record); err != nil {
		finished := m.now().UTC()
		job.Status = "failed"
		job.Stage = "launch_failed"
		job.Progress = 100
		job.Message = "无法启动磁盘后台任务"
		job.FinishedAt = &finished
		_ = m.writeDiskJob(job)
		return contract.DiskPartitionJob{}, fmt.Errorf("%w: start disk job: %v", ErrUnsupported, err)
	}
	return job, nil
}

func (m *Manager) launchDiskJob(ctx context.Context, record diskJobRecord) error {
	executable, err := m.backgroundExecutable()
	if err != nil {
		return err
	}
	timeout := "2min"
	if record.Request.Action == "format" || record.Request.Action == "check" || record.Request.Action == "repair" {
		timeout = "2h"
	}
	arguments := []string{
		"--unit=" + diskUnitPrefix + record.ID, "--collect", "--no-block",
		"--property=Type=oneshot", "--property=TimeoutStartSec=" + timeout, "--property=TimeoutStopSec=30s",
		"--property=User=root", "--property=UMask=0077",
		"--property=PrivateDevices=no", "--property=PrivateMounts=no",
		"--property=DevicePolicy=closed", "--property=DeviceAllow=" + record.Target.Path + " rw",
		"--property=CapabilityBoundingSet=CAP_SYS_ADMIN CAP_DAC_OVERRIDE CAP_FOWNER",
		"--property=AmbientCapabilities=CAP_SYS_ADMIN CAP_DAC_OVERRIDE CAP_FOWNER",
		"--property=NoNewPrivileges=yes", "--property=SystemCallFilter=@system-service @mount",
		"--property=RestrictAddressFamilies=AF_UNIX AF_NETLINK", "--property=Nice=10",
		"--property=CPUWeight=20", "--property=IOWeight=20", "--property=SyslogIdentifier=kpanel-disk-run",
		"--", executable, "disk-run", "--state-dir", m.stateDir, "--id", record.ID,
	}
	_, err = m.runner.Run(ctx, "systemd-run", arguments...)
	return err
}

// RunDiskJob is called only by the fixed root-only CLI worker.
func (m *Manager) RunDiskJob(ctx context.Context, id string) error {
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return errors.New("disk-run requires root on Linux")
	}
	if !diskJobIDPattern.MatchString(id) {
		return errors.New("invalid disk job id")
	}
	unlock, err := m.acquireDiskProcessLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	record, err := m.readDiskRecord()
	if err != nil {
		return err
	}
	if record.ID != id {
		return errors.New("disk job id does not match persisted request")
	}
	job := m.readDiskJob()
	if job == nil || job.ID != id {
		return errors.New("disk job state is unavailable")
	}
	started := m.now().UTC()
	job.Status = "running"
	job.Stage = "verifying"
	job.Progress = 10
	job.Message = "正在重新验证设备身份"
	job.StartedAt = &started
	job.FinishedAt = nil
	if err := m.writeDiskJob(*job); err != nil {
		return err
	}
	fail := func(status, stage, message string, cause error, recoveryPath ...string) error {
		finished := m.now().UTC()
		backupPath := ""
		if len(recoveryPath) != 0 {
			backupPath = recoveryPath[0]
		}
		receipt := diskJobReceipt{ID: id, Status: status, Stage: stage, Message: message, BackupPath: backupPath, FinishedAt: finished}
		_ = m.writeDiskReceipt(receipt)
		job.Status = status
		job.Stage = stage
		job.Progress = 100
		job.Message = message
		job.RecoveryPath = backupPath
		job.FinishedAt = &finished
		_ = m.writeDiskJob(*job)
		return cause
	}
	envelope, err := m.collectDiskInventory(ctx)
	if err != nil {
		return fail("needs_attention", "verification_failed", "无法重新读取磁盘状态，未执行操作", err)
	}
	if envelope.Snapshot.ResourceVersion != record.ExpectedResourceVersion {
		return fail("needs_attention", "inventory_changed", "磁盘状态已变化，操作已安全中止", ErrConflict)
	}
	target, device, ok := findDiskTarget(envelope, record.Target.ID)
	if !ok || target.Path != record.Target.Path || target.MajorMinor != record.Target.MajorMinor || device.Protected {
		return fail("needs_attention", "identity_changed", "设备身份或保护状态已变化，操作已安全中止", ErrConflict)
	}
	if err := m.validateDiskActionTarget(record.Request, device, target, envelope); err != nil {
		return fail("needs_attention", "precondition_changed", "操作前置条件已变化，操作已安全中止", err)
	}
	job.Stage = "executing"
	job.Progress = 30
	job.Message = "正在执行磁盘操作"
	_ = m.writeDiskJob(*job)
	receipt, err := m.executeDiskScript(ctx, record)
	if err != nil {
		status := contract.DiskPartitionJobFailed
		var statusErr *diskScriptStatusError
		if errors.As(err, &statusErr) && (statusErr.status == "needs-attention" || statusErr.status == "rollback-failed" || statusErr.status == "completion-unverified") {
			status = contract.DiskPartitionJobNeedsAttention
		}
		message, recoveryPath := "磁盘工具未确认操作成功", ""
		if statusErr != nil {
			if statusErr.receipt.Message != "" {
				message = statusErr.receipt.Message
			}
			recoveryPath = statusErr.receipt.BackupPath
		}
		return fail(status, "script_failed", message, err, recoveryPath)
	}
	job.Stage = "verifying_result"
	job.Progress = 85
	job.Message = "正在核对真实磁盘状态"
	_ = m.writeDiskJob(*job)
	after, err := m.collectDiskInventory(ctx)
	if err != nil {
		return fail("needs_attention", "result_unverified", "操作已执行，但无法回读磁盘状态", err)
	}
	if err := verifyDiskResult(record, receipt, after); err != nil {
		return fail("needs_attention", "result_unverified", "操作回执与真实磁盘状态不一致，需要人工检查", err)
	}
	finished := m.now().UTC()
	message := receipt.Message
	if message == "" {
		message = "磁盘操作已完成并验证"
	}
	finalReceipt := diskJobReceipt{ID: id, Status: "succeeded", Stage: "completed", Message: message, BackupPath: receipt.BackupPath, FinishedAt: finished}
	if err := m.writeDiskReceipt(finalReceipt); err != nil {
		return fail("needs_attention", "receipt_failed", "操作已验证，但无法持久化完成凭据", err)
	}
	job.Status = "succeeded"
	job.Stage = "completed"
	job.Progress = 100
	job.Message = message
	job.RecoveryPath = receipt.BackupPath
	job.FinishedAt = &finished
	return m.writeDiskJob(*job)
}

type parsedDiskReceipt struct{ Status, Device, Message, BackupPath string }

type diskScriptStatusError struct {
	status  string
	message string
	receipt parsedDiskReceipt
}

func (err *diskScriptStatusError) Error() string {
	if err.message == "" {
		return "disk script reported " + err.status
	}
	return "disk script reported " + err.status + ": " + err.message
}

func (m *Manager) executeDiskScript(ctx context.Context, record diskJobRecord) (parsedDiskReceipt, error) {
	if err := m.diskWriteAvailability(); err != nil {
		return parsedDiskReceipt{}, err
	}
	script, err := m.diskScript()
	if err != nil {
		return parsedDiskReceipt{}, err
	}
	scriptAction := record.Request.Action
	if scriptAction == contract.DiskPartitionActionRepair {
		scriptAction = contract.DiskPartitionActionCheck
	}
	arguments := []string{"KJ_DISK_MANAGEMENT_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script, "kpanel", "disk-management", scriptAction, record.Target.MajorMinor}
	switch record.Request.Action {
	case "mount":
		arguments = append(arguments, hex.EncodeToString([]byte(record.Request.MountPoint)), boolProtocol(record.Request.Persist, "1", "0"))
	case "unmount":
		arguments = append(arguments, hex.EncodeToString([]byte(record.Request.MountPoint)), boolProtocol(record.Request.RemovePersistence, "1", "0"))
	case "format":
		arguments = append(arguments, record.Request.Filesystem)
	case "check":
		arguments = append(arguments, "readonly")
	case "repair":
		arguments = append(arguments, "repair")
	default:
		return parsedDiskReceipt{}, ErrInvalidInput
	}
	output, _, runErr := m.runResourceCommand(ctx, diskReceiptLimit, "env", arguments...)
	receipt, receiptErr := parseDiskReceipt(output, record.Target.MajorMinor)
	if receiptErr != nil {
		if runErr != nil {
			return parsedDiskReceipt{}, &diskScriptStatusError{status: "completion-unverified", message: "command failed without a valid receipt"}
		}
		return parsedDiskReceipt{}, receiptErr
	}
	if receipt.Status != "applied" && receipt.Status != "unchanged" {
		return parsedDiskReceipt{}, &diskScriptStatusError{status: receipt.Status, message: receipt.Message, receipt: receipt}
	}
	if runErr != nil {
		return parsedDiskReceipt{}, &diskScriptStatusError{status: "completion-unverified", message: "successful receipt accompanied command failure"}
	}
	return receipt, nil
}

func parseDiskReceipt(output []byte, expectedDevice string) (parsedDiskReceipt, error) {
	if len(output) == 0 || len(output) > diskReceiptLimit {
		return parsedDiskReceipt{}, errors.New("disk script receipt is empty or oversized")
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 5 || strings.TrimSuffix(lines[0], "\r") != "KPANEL_DISK_MANAGEMENT_PROTOCOL 1" {
		return parsedDiskReceipt{}, errors.New("disk script receipt marker or field count is invalid")
	}
	values := make(map[string]string, 4)
	seen := make(map[string]bool, 4)
	fieldNames := []struct{ protocol, local string }{
		{"KPANEL_DISK_MANAGEMENT_STATUS", "status"},
		{"KPANEL_DISK_MANAGEMENT_DEVICE", "device"},
		{"KPANEL_DISK_MANAGEMENT_MESSAGE_HEX", "message_hex"},
		{"KPANEL_DISK_MANAGEMENT_BACKUP_HEX", "backup_hex"},
	}
	for index, line := range lines[1:] {
		line = strings.TrimSuffix(line, "\r")
		key, value, ok := strings.Cut(line, "=")
		mapped := fieldNames[index]
		if !ok || key != mapped.protocol || seen[mapped.local] {
			return parsedDiskReceipt{}, errors.New("disk script receipt contains invalid or duplicate fields")
		}
		seen[mapped.local] = true
		values[mapped.local] = value
	}
	if len(values) != 4 || values["device"] != expectedDevice {
		return parsedDiskReceipt{}, errors.New("disk script receipt device does not match")
	}
	switch values["status"] {
	case "applied", "unchanged", "failed", "conflict", "needs-attention", "rollback-failed":
	default:
		return parsedDiskReceipt{}, fmt.Errorf("disk script status %q is invalid", values["status"])
	}
	decode := func(value string) (string, error) {
		if len(value) > 8192 || len(value)%2 != 0 || !diskReceiptHexExpr.MatchString(value) {
			return "", errors.New("invalid receipt hex")
		}
		data, err := hex.DecodeString(value)
		if err != nil || !utf8.Valid(data) {
			return "", errors.New("invalid receipt hex")
		}
		text := string(data)
		for _, character := range text {
			if unicode.IsControl(character) {
				return "", errors.New("invalid receipt text")
			}
		}
		return text, nil
	}
	message, err := decode(values["message_hex"])
	if err != nil {
		return parsedDiskReceipt{}, err
	}
	backup, err := decode(values["backup_hex"])
	if err != nil {
		return parsedDiskReceipt{}, err
	}
	return parsedDiskReceipt{Status: values["status"], Device: values["device"], Message: message, BackupPath: backup}, nil
}

func verifyDiskResult(record diskJobRecord, receipt parsedDiskReceipt, after diskInspectEnvelope) error {
	_ = receipt
	afterTarget, device, ok := findDiskTarget(after, record.Target.ID)
	if record.Request.Action == "format" {
		// Formatting legitimately changes UUID-derived opaque IDs, so use the
		// trusted major:minor identity for the postcondition.
		for _, target := range after.Targets {
			if target.MajorMinor == record.Target.MajorMinor {
				afterTarget = target
				for _, candidate := range after.Snapshot.Devices {
					if candidate.ID == target.ID {
						device, ok = candidate, true
						break
					}
				}
				break
			}
		}
	}
	if !ok {
		return errors.New("target device disappeared")
	}
	switch record.Request.Action {
	case "mount":
		for _, mount := range device.Mounts {
			if mount.Path == record.Request.MountPoint && (record.Request.Persist == nil || !*record.Request.Persist || mount.Persistent) {
				return nil
			}
		}
		return errors.New("requested mount is absent")
	case "unmount":
		for _, mount := range device.Mounts {
			if mount.Path == record.Request.MountPoint {
				return errors.New("mount remains active")
			}
		}
		if record.Request.RemovePersistence != nil && *record.Request.RemovePersistence && slices.Contains(afterTarget.PersistentMounts, record.Request.MountPoint) {
			return errors.New("persistent mount remains configured")
		}
		return nil
	case "format":
		if device.Filesystem == nil || !strings.EqualFold(device.Filesystem.Type, record.Request.Filesystem) {
			return errors.New("filesystem does not match request")
		}
		return nil
	case "check", "repair":
		if device.ID != record.Target.ID {
			return errors.New("device identity changed")
		}
		return nil
	default:
		return ErrInvalidInput
	}
}

func findDiskTarget(envelope diskInspectEnvelope, id string) (diskTarget, contract.DiskDevice, bool) {
	var target diskTarget
	var device contract.DiskDevice
	foundTarget, foundDevice := false, false
	for _, candidate := range envelope.Targets {
		if candidate.ID == id {
			target, foundTarget = candidate, true
			break
		}
	}
	for _, candidate := range envelope.Snapshot.Devices {
		if candidate.ID == id {
			device, foundDevice = candidate, true
			break
		}
	}
	return target, device, foundTarget && foundDevice
}

func boolProtocol(value *bool, whenTrue, whenFalse string) string {
	if value != nil && *value {
		return whenTrue
	}
	return whenFalse
}

func (m *Manager) validateDiskActionTarget(request contract.DiskPartitionActionRequest, device contract.DiskDevice, target diskTarget, envelope diskInspectEnvelope) error {
	switch request.Action {
	case contract.DiskPartitionActionMount:
		if _, err := m.runner.LookPath("mount"); err != nil {
			return fmt.Errorf("%w: mount tool is unavailable", ErrUnsupported)
		}
		if err := m.validateDiskMountDestination(request.MountPoint, target.ID, envelope); err != nil {
			return fmt.Errorf("%w: mountPoint: %v", ErrInvalidInput, err)
		}
	case contract.DiskPartitionActionUnmount:
		if _, err := m.runner.LookPath("umount"); err != nil {
			return fmt.Errorf("%w: umount tool is unavailable", ErrUnsupported)
		}
		found := false
		for _, mount := range device.Mounts {
			if mount.Path == request.MountPoint {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: mountPoint is not mounted by the selected device", ErrConflict)
		}
	case contract.DiskPartitionActionFormat:
		if !m.diskFilesystemToolAvailable(request.Filesystem, true) {
			return fmt.Errorf("%w: requested filesystem formatter is unavailable", ErrUnsupported)
		}
	case contract.DiskPartitionActionCheck, contract.DiskPartitionActionRepair:
		if device.Filesystem == nil || !m.diskFilesystemToolAvailable(device.Filesystem.Type, false) {
			return fmt.Errorf("%w: filesystem check tool is unavailable", ErrUnsupported)
		}
	}
	return nil
}

func (m *Manager) validateDiskMountDestination(mountPoint, deviceID string, envelope diskInspectEnvelope) error {
	if diskProtectedMountPoint(mountPoint) {
		return errors.New("critical system mount points cannot be replaced")
	}
	for _, candidate := range envelope.Snapshot.Devices {
		for _, mount := range candidate.Mounts {
			if mount.Path == mountPoint {
				return errors.New("mount target is already occupied")
			}
		}
	}
	for _, candidate := range envelope.Targets {
		if candidate.ID == deviceID {
			continue
		}
		if slices.Contains(candidate.PersistentMounts, mountPoint) {
			return errors.New("mount target is reserved by another persistent device")
		}
	}
	active, mountErr := m.diskSystemMountActive(mountPoint)
	if mountErr != nil {
		return errors.New("active mount table cannot be verified")
	}
	if active {
		return errors.New("mount target is an active system mount")
	}
	info, err := os.Lstat(mountPoint)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("mount target must be a real directory")
		}
		resolved, resolveErr := filepath.EvalSymlinks(mountPoint)
		if resolveErr != nil || path.Clean(resolved) != mountPoint {
			return errors.New("mount target must not traverse symbolic links")
		}
		directory, openErr := os.Open(mountPoint)
		if openErr != nil {
			return errors.New("mount target directory is not readable")
		}
		_, readErr := directory.Readdirnames(1)
		_ = directory.Close()
		if readErr != io.EOF {
			return errors.New("mount target directory must be empty")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return errors.New("mount target cannot be inspected")
	}
	for ancestor := filepath.Dir(mountPoint); ancestor != "/" && ancestor != "."; ancestor = filepath.Dir(ancestor) {
		if _, err := os.Lstat(ancestor); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return errors.New("mount target ancestor cannot be inspected")
		}
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err != nil || path.Clean(resolved) != ancestor {
			return errors.New("mount target must not traverse symbolic links")
		}
		break
	}
	return nil
}

func (m *Manager) diskSystemMountActive(mountPoint string) (bool, error) {
	data, err := readResourceFile(filepath.Join(m.procRoot, "self", "mountinfo"), 1<<20)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && unescapeFstab(fields[4]) == mountPoint {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) currentDiskJob() *contract.DiskPartitionJob {
	job := m.readDiskJob()
	if job == nil {
		return nil
	}
	m.reconcileDiskJob(job)
	return job
}

// DiskJobActive prevents backup data switches while an asynchronous partition
// worker may still mount, unmount or change the underlying filesystem.
func (m *Manager) DiskJobActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Lstat(m.diskJobPath()); errors.Is(err, os.ErrNotExist) {
		return false
	}
	job := m.currentDiskJob()
	return job == nil || job.Status == "queued" || job.Status == "running"
}

func (m *Manager) reconcileDiskJob(job *contract.DiskPartitionJob) {
	if job == nil || (job.Status != "queued" && job.Status != "running") {
		return
	}
	if receipt, err := m.readDiskReceipt(); err == nil && receipt.ID == job.ID {
		job.Status, job.Stage, job.Progress, job.Message, job.FinishedAt = receipt.Status, receipt.Stage, 100, receipt.Message, &receipt.FinishedAt
		job.RecoveryPath = receipt.BackupPath
		_ = m.writeDiskJob(*job)
		return
	}
	if m.now().Sub(job.CreatedAt) < diskJobLaunchGrace {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := m.runner.Run(ctx, "systemctl", "show", diskUnitPrefix+job.ID, "--property=LoadState", "--property=ActiveState", "--property=SubState", "--property=Result", "--property=ExecMainStatus", "--no-pager")
	unit := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			unit[key] = value
		}
	}
	if err == nil && (unit["ActiveState"] == "active" || unit["ActiveState"] == "activating") {
		return
	}
	elapsed := m.now().Sub(job.CreatedAt)
	if elapsed < diskJobLaunchTimeout && (err != nil || (unit["LoadState"] == "" && unit["ActiveState"] == "")) {
		return
	}
	latest := m.readDiskJob()
	if latest == nil || latest.ID != job.ID || latest.Status != job.Status || latest.Stage != job.Stage {
		if latest != nil {
			*job = *latest
		}
		return
	}
	finished := m.now().UTC()
	job.Status = "needs_attention"
	job.Stage = "completion_unverified"
	job.Progress = 100
	job.FinishedAt = &finished
	job.Message = "磁盘任务已退出但没有可信完成凭据，不能判定为成功"
	_ = m.writeDiskJob(*job)
}

func (m *Manager) ensureDiskJobDir() error {
	path := m.diskJobDir()
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("disk job state path is not a directory")
		}
		return os.Chmod(path, 0700)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0700)
}
func (m *Manager) diskJobDir() string      { return filepath.Join(m.stateDir, "disk-jobs") }
func (m *Manager) diskJobPath() string     { return filepath.Join(m.diskJobDir(), "job.json") }
func (m *Manager) diskRecordPath() string  { return filepath.Join(m.diskJobDir(), "request.json") }
func (m *Manager) diskReceiptPath() string { return filepath.Join(m.diskJobDir(), "receipt.json") }

func (m *Manager) writeDiskJob(value contract.DiskPartitionJob) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeAtomic(m.diskJobPath(), append(data, '\n'), 0600); err != nil {
		return err
	}
	return syncDiskStateDirectory(m.diskJobDir())
}
func (m *Manager) writeDiskRecord(value diskJobRecord) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeAtomic(m.diskRecordPath(), append(data, '\n'), 0600); err != nil {
		return err
	}
	return syncDiskStateDirectory(m.diskJobDir())
}
func (m *Manager) writeDiskReceipt(value diskJobReceipt) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := writeAtomic(m.diskReceiptPath(), append(data, '\n'), 0600); err != nil {
		return err
	}
	return syncDiskStateDirectory(m.diskJobDir())
}

func readBoundedDiskJSON(path string, limit int, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > int64(limit) {
		return errors.New("invalid disk state file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("disk state file contains trailing or invalid JSON")
	}
	return nil
}
func (m *Manager) readDiskJob() *contract.DiskPartitionJob {
	var value contract.DiskPartitionJob
	if readBoundedDiskJSON(m.diskJobPath(), diskJobFileLimit, &value) != nil || !diskJobIDPattern.MatchString(value.ID) || value.Progress < 0 || value.Progress > 100 {
		return nil
	}
	validStatus := value.Status == contract.DiskPartitionJobQueued || value.Status == contract.DiskPartitionJobRunning || value.Status == contract.DiskPartitionJobSucceeded || value.Status == contract.DiskPartitionJobFailed || value.Status == contract.DiskPartitionJobNeedsAttention
	if !validStatus || len(value.DeviceID) != 64 || !strings.HasPrefix(value.DevicePath, "/dev/") {
		return nil
	}
	return &value
}
func (m *Manager) readDiskRecord() (diskJobRecord, error) {
	var value diskJobRecord
	err := readBoundedDiskJSON(m.diskRecordPath(), diskRequestFileLimit, &value)
	field, _ := contract.ValidateDiskPartitionAction(&value.Request)
	if err == nil && (!diskJobIDPattern.MatchString(value.ID) || field != "" || value.ExpectedResourceVersion != value.Request.ExpectedResourceVersion || value.Target.ID != value.Request.DeviceID || !strings.HasPrefix(value.Target.Path, "/dev/") || !diskMajorMinorExpr.MatchString(value.Target.MajorMinor)) {
		err = errors.New("invalid disk request identity")
	}
	return value, err
}
func (m *Manager) readDiskReceipt() (diskJobReceipt, error) {
	var value diskJobReceipt
	err := readBoundedDiskJSON(m.diskReceiptPath(), diskReceiptLimit, &value)
	validStatus := value.Status == contract.DiskPartitionJobSucceeded || value.Status == contract.DiskPartitionJobFailed || value.Status == contract.DiskPartitionJobNeedsAttention
	if err == nil && (!diskJobIDPattern.MatchString(value.ID) || value.FinishedAt.IsZero() || !validStatus) {
		err = errors.New("invalid disk receipt")
	}
	return value, err
}
