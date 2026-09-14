package systemmanage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

const (
	diskInspectOutputLimit = 4 << 20
	diskMaxDevices         = 512
	diskMaxDepth           = 16
	diskInspectTimeout     = 30 * time.Second
	diskUnitPrefix         = "kejilion-panel-disk-"
)

var diskProtocolVersionPattern = regexp.MustCompile(`(?m)^KPANEL_DISK_MANAGEMENT_PROTOCOL_VERSION="1"\r?$`)

type diskInspectEnvelope struct {
	Snapshot contract.DiskPartitionSnapshot `json:"snapshot"`
	Targets  []diskTarget                   `json:"targets"`
}

// diskTarget contains the privileged identity material that never crosses the
// Agent HTTP boundary. A Web request can name only the opaque ID.
type diskTarget struct {
	ID               string   `json:"id"`
	Path             string   `json:"path"`
	MajorMinor       string   `json:"majorMinor"`
	KName            string   `json:"kname"`
	PersistentMounts []string `json:"persistentMounts"`
	Holders          []string `json:"holders"`
}

type lsblkDocument struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

type lsblkNode struct {
	Name        string          `json:"name"`
	KName       string          `json:"kname"`
	Path        string          `json:"path"`
	Type        string          `json:"type"`
	PKName      string          `json:"pkname"`
	Size        json.RawMessage `json:"size"`
	RO          json.RawMessage `json:"ro"`
	RM          json.RawMessage `json:"rm"`
	Model       string          `json:"model"`
	Serial      string          `json:"serial"`
	Transport   string          `json:"tran"`
	WWN         string          `json:"wwn"`
	FSType      string          `json:"fstype"`
	FSVersion   string          `json:"fsver"`
	Label       string          `json:"label"`
	UUID        string          `json:"uuid"`
	PartUUID    string          `json:"partuuid"`
	MajorMinor  string          `json:"maj:min"`
	Mountpoints json.RawMessage `json:"mountpoints"`
	Mountpoint  json.RawMessage `json:"mountpoint"`
	Children    []lsblkNode     `json:"children"`
}

// UnmarshalJSON accepts the null string values emitted by util-linux while
// retaining strict rejection of unknown columns and malformed node shapes.
func (node *lsblkNode) UnmarshalJSON(data []byte) error {
	type rawNode struct {
		Name        json.RawMessage `json:"name"`
		KName       json.RawMessage `json:"kname"`
		Path        json.RawMessage `json:"path"`
		Type        json.RawMessage `json:"type"`
		PKName      json.RawMessage `json:"pkname"`
		Size        json.RawMessage `json:"size"`
		RO          json.RawMessage `json:"ro"`
		RM          json.RawMessage `json:"rm"`
		Model       json.RawMessage `json:"model"`
		Serial      json.RawMessage `json:"serial"`
		Transport   json.RawMessage `json:"tran"`
		WWN         json.RawMessage `json:"wwn"`
		FSType      json.RawMessage `json:"fstype"`
		FSVersion   json.RawMessage `json:"fsver"`
		Label       json.RawMessage `json:"label"`
		UUID        json.RawMessage `json:"uuid"`
		PartUUID    json.RawMessage `json:"partuuid"`
		MajorMinor  json.RawMessage `json:"maj:min"`
		Mountpoints json.RawMessage `json:"mountpoints"`
		Mountpoint  json.RawMessage `json:"mountpoint"`
		Children    []lsblkNode     `json:"children"`
	}
	var raw rawNode
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	read := func(value json.RawMessage) (string, error) {
		if len(value) == 0 || string(value) == "null" {
			return "", nil
		}
		var result string
		if err := json.Unmarshal(value, &result); err != nil {
			return "", err
		}
		return result, nil
	}
	values := []*string{&node.Name, &node.KName, &node.Path, &node.Type, &node.PKName,
		&node.Model, &node.Serial, &node.Transport, &node.WWN, &node.FSType,
		&node.FSVersion, &node.Label, &node.UUID, &node.PartUUID, &node.MajorMinor}
	rawValues := []json.RawMessage{raw.Name, raw.KName, raw.Path, raw.Type, raw.PKName,
		raw.Model, raw.Serial, raw.Transport, raw.WWN, raw.FSType,
		raw.FSVersion, raw.Label, raw.UUID, raw.PartUUID, raw.MajorMinor}
	for index := range values {
		value, err := read(rawValues[index])
		if err != nil {
			return err
		}
		*values[index] = value
	}
	node.Size, node.RO, node.RM = raw.Size, raw.RO, raw.RM
	node.Mountpoints, node.Mountpoint, node.Children = raw.Mountpoints, raw.Mountpoint, raw.Children
	return nil
}

type diskInventoryNode struct {
	lsblkNode
	parentPath string
	depth      int
	id         string
	holders    []string
	mounts     []string
}

// DiskPartitions performs inspection in a fixed transient unit. The main Agent
// service intentionally retains PrivateDevices=yes.
func (m *Manager) DiskPartitions(ctx context.Context) (contract.DiskPartitionSnapshot, error) {
	envelope, err := m.inspectDiskPartitions(ctx)
	if err != nil {
		return contract.DiskPartitionSnapshot{}, err
	}
	envelope.Snapshot.Job = m.currentDiskJob()
	return envelope.Snapshot, nil
}

func (m *Manager) inspectDiskPartitions(ctx context.Context) (diskInspectEnvelope, error) {
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return diskInspectEnvelope{}, fmt.Errorf("%w: disk inspection requires a root Linux Agent", ErrUnsupported)
	}
	executable, err := m.backgroundExecutable()
	if err != nil {
		return diskInspectEnvelope{}, err
	}
	if err := m.backgroundJobsAvailable(); err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	unitID := randomDiskID()
	spec := m.backgroundJobSpec(
		diskUnitPrefix+"inspect-"+unitID,
		executable,
		[]string{"disk-inspect", "--state-dir", m.stateDir},
		[]string{
			"Type=oneshot", "TimeoutStartSec=30s", "TimeoutStopSec=10s",
			"User=root", "UMask=0077", "PrivateDevices=no", "PrivateMounts=no",
			"NoNewPrivileges=yes", "CapabilityBoundingSet=", "AmbientCapabilities=",
			"RestrictNamespaces=yes", "RestrictAddressFamilies=AF_UNIX AF_NETLINK",
			"Nice=10", "CPUWeight=20", "IOWeight=20",
			"SyslogIdentifier=kpanel-disk-inspect",
		},
		"0077",
		10,
		10*time.Second,
	)
	command, arguments, _, err := jobcontrol.ForegroundInvocation(m.runner, spec)
	if err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("%w: disk inspection backend: %v", ErrUnsupported, err)
	}
	inspectContext, cancel := context.WithTimeout(ctx, diskInspectTimeout)
	defer cancel()
	output, _, err := m.runResourceCommand(inspectContext, diskInspectOutputLimit, command, arguments...)
	if err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("%w: disk inspection failed: %v", ErrUnsupported, err)
	}
	var envelope diskInspectEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("%w: invalid disk inspection receipt: %v", ErrUnsupported, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return diskInspectEnvelope{}, fmt.Errorf("%w: disk inspection returned trailing data", ErrUnsupported)
	}
	if err := validateDiskEnvelope(envelope); err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return envelope, nil
}

// InspectDiskPartitionsDirect is called only by the root-only disk-inspect
// worker. It reads the real /dev namespace and emits a bounded internal receipt.
func (m *Manager) InspectDiskPartitionsDirect(ctx context.Context) (any, error) {
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return nil, fmt.Errorf("disk-inspect requires root on Linux")
	}
	return m.collectDiskInventory(ctx)
}

func (m *Manager) collectDiskInventory(ctx context.Context) (diskInspectEnvelope, error) {
	if _, err := m.runner.LookPath("lsblk"); err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("lsblk is unavailable")
	}
	columns := "NAME,KNAME,PATH,TYPE,PKNAME,SIZE,RO,RM,MODEL,SERIAL,TRAN,WWN,FSTYPE,FSVER,LABEL,UUID,PARTUUID,MAJ:MIN,MOUNTPOINTS"
	output, _, err := m.runResourceCommand(ctx, diskInspectOutputLimit, "lsblk", "--json", "--bytes", "--paths", "--output", columns)
	if err != nil {
		if errors.Is(err, errResourceOutputTooLarge) {
			return diskInspectEnvelope{}, err
		}
		columns = "NAME,KNAME,PATH,TYPE,PKNAME,SIZE,RO,RM,MODEL,SERIAL,TRAN,WWN,FSTYPE,LABEL,UUID,PARTUUID,MAJ:MIN,MOUNTPOINT"
		output, _, err = m.runResourceCommand(ctx, diskInspectOutputLimit, "lsblk", "--json", "--bytes", "--paths", "--output", columns)
	}
	if err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("lsblk inventory failed: %w", err)
	}
	var document lsblkDocument
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return diskInspectEnvelope{}, fmt.Errorf("parse lsblk JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return diskInspectEnvelope{}, errors.New("lsblk returned trailing JSON data")
	}
	nodes := make([]diskInventoryNode, 0, len(document.BlockDevices))
	var walk func([]lsblkNode, string, int) error
	walk = func(values []lsblkNode, parent string, depth int) error {
		if depth > diskMaxDepth {
			return errors.New("lsblk hierarchy exceeds depth limit")
		}
		for _, value := range values {
			if len(nodes) >= diskMaxDevices {
				return errors.New("lsblk inventory exceeds device limit")
			}
			value.Path = strings.TrimSpace(value.Path)
			value.KName = strings.TrimSpace(value.KName)
			if value.Path == "" || !strings.HasPrefix(value.Path, "/dev/") || value.KName == "" {
				return errors.New("lsblk returned an invalid device identity")
			}
			node := diskInventoryNode{lsblkNode: value, parentPath: parent, depth: depth}
			node.mounts, err = diskMountpoints(value)
			if err != nil {
				return err
			}
			node.id = opaqueDiskID(value)
			node.holders = m.diskHolders(value.KName)
			nodes = append(nodes, node)
			if err := walk(value.Children, value.Path, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(document.BlockDevices, "", 0); err != nil {
		return diskInspectEnvelope{}, err
	}
	if len(nodes) == 0 {
		return diskInspectEnvelope{}, errors.New("lsblk returned no block devices")
	}
	disambiguateDiskIDs(nodes)
	// Older util-linux versions may return a flat list with PKNAME but no
	// children array. Reconstruct the same parent topology without guessing.
	pathsByKName := make(map[string]string, len(nodes))
	for _, node := range nodes {
		pathsByKName[filepath.Base(node.KName)] = node.Path
		pathsByKName[filepath.Base(node.Path)] = node.Path
	}
	for index := range nodes {
		if nodes[index].parentPath == "" && strings.TrimSpace(nodes[index].PKName) != "" {
			nodes[index].parentPath = pathsByKName[filepath.Base(nodes[index].PKName)]
		}
	}
	ids := make(map[string]bool, len(nodes))
	pathToID := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if ids[node.id] {
			return diskInspectEnvelope{}, errors.New("disk identity collision")
		}
		ids[node.id] = true
		pathToID[node.Path] = node.id
	}
	persistent, persistentByDevice := m.diskPersistentMounts(nodes)
	activeSwap := m.diskActiveSwap()
	protected := make(map[string][]string)
	children := make(map[string]bool)
	for _, node := range nodes {
		if node.parentPath != "" {
			children[node.parentPath] = true
		}
		if diskBool(node.RO) {
			protected[node.Path] = append(protected[node.Path], "设备为只读")
		}
		if len(node.holders) != 0 {
			protected[node.Path] = append(protected[node.Path], "设备存在活动 holder")
		}
		if activeSwap[node.Path] {
			protected[node.Path] = append(protected[node.Path], "设备正在作为 swap 使用")
		}
		for _, mountpoint := range node.mounts {
			if diskProtectedMountPoint(mountpoint) {
				protected[node.Path] = append(protected[node.Path], "设备挂载于受保护系统路径 "+mountpoint)
			}
		}
		for _, mountpoint := range persistentByDevice[node.Path] {
			if diskProtectedMountPoint(mountpoint) {
				protected[node.Path] = append(protected[node.Path], "设备配置了受保护的持久挂载 "+mountpoint)
			}
		}
	}
	for parent := range children {
		protected[parent] = append(protected[parent], "设备包含子设备")
	}
	for _, critical := range []string{"/", "/boot", "/boot/efi", "/home", "/var/lib/kejilion-panel", "/home/docker"} {
		if devicePath := deviceForProtectedPath(nodes, critical); devicePath != "" {
			protected[devicePath] = append(protected[devicePath], "承载受保护系统路径 "+critical)
		}
	}
	for index := range nodes {
		if len(protected[nodes[index].Path]) == 0 {
			continue
		}
		for parent := nodes[index].parentPath; parent != ""; {
			protected[parent] = append(protected[parent], "受保护设备的上层设备")
			next := ""
			for _, candidate := range nodes {
				if candidate.Path == parent {
					next = candidate.parentPath
					break
				}
			}
			parent = next
		}
	}
	platform := m.diskPlatform()
	writeReason := ""
	if !platform.Writable {
		writeReason = platform.Reason
	} else if !m.enabled {
		writeReason = "宿主机系统写入开关未启用"
	} else if err := m.diskWriteAvailability(); err != nil {
		writeReason = capabilityReason(err)
	}
	devices := make([]contract.DiskDevice, 0, len(nodes))
	targets := make([]diskTarget, 0, len(nodes))
	for _, node := range nodes {
		size, err := diskUint(node.Size)
		if err != nil {
			return diskInspectEnvelope{}, fmt.Errorf("invalid size for %s", node.KName)
		}
		mounts := make([]contract.DiskMount, 0, len(node.mounts))
		for _, mountpoint := range node.mounts {
			mount := contract.DiskMount{Path: mountpoint, Persistent: persistent[node.Path+"\x00"+mountpoint]}
			mount.TotalBytes, mount.UsedBytes, mount.AvailableBytes, mount.UsagePercent = diskMountUsage(mountpoint)
			mounts = append(mounts, mount)
		}
		filesystem := (*contract.DiskFilesystem)(nil)
		if value := strings.TrimSpace(node.FSType); value != "" {
			filesystem = &contract.DiskFilesystem{Type: value, Version: strings.TrimSpace(node.FSVersion), Label: strings.TrimSpace(node.Label), UUID: strings.TrimSpace(node.UUID), PartUUID: strings.TrimSpace(node.PartUUID)}
		}
		reasons := slices.Clone(protected[node.Path])
		if reasons == nil {
			reasons = []string{}
		}
		slices.Sort(reasons)
		reasons = slices.Compact(reasons)
		device := contract.DiskDevice{
			ID: node.id, Path: node.Path, Name: strings.TrimSpace(node.Name), Type: strings.TrimSpace(node.Type),
			SizeBytes: size, ReadOnly: diskBool(node.RO), Removable: diskBool(node.RM),
			Virtual: diskVirtual(node), Model: strings.TrimSpace(node.Model), Serial: strings.TrimSpace(node.Serial),
			Transport: strings.TrimSpace(node.Transport), Filesystem: filesystem, Mounts: mounts,
			Protected: len(reasons) != 0, ProtectionReasons: reasons,
			Operations: make(map[string]contract.DiskOperationState, 5),
		}
		if node.parentPath != "" {
			device.ParentID = pathToID[node.parentPath]
		}
		device.Operations = m.diskOperations(device, writeReason, children[node.Path])
		devices = append(devices, device)
		targets = append(targets, diskTarget{
			ID: node.id, Path: trustedDiskDevicePath(node.Path), MajorMinor: strings.TrimSpace(node.MajorMinor),
			KName: node.KName, PersistentMounts: slices.Clone(persistentByDevice[node.Path]), Holders: slices.Clone(node.holders),
		})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	resourceVersion := diskResourceVersion(devices, targets)
	observedAt := m.now().UTC()
	envelope := diskInspectEnvelope{Snapshot: contract.DiskPartitionSnapshot{
		ResourceVersion: resourceVersion, Platform: platform, Devices: devices, ObservedAt: observedAt,
	}, Targets: targets}
	if err := validateDiskEnvelope(envelope); err != nil {
		return diskInspectEnvelope{}, err
	}
	return envelope, nil
}

func (m *Manager) diskOperations(device contract.DiskDevice, commonReason string, nonLeaf bool) map[string]contract.DiskOperationState {
	result := make(map[string]contract.DiskOperationState, 5)
	set := func(name string, enabled bool, reason string) {
		result[name] = contract.DiskOperationState{Enabled: enabled, Reason: reason}
	}
	if commonReason != "" {
		for _, name := range []string{"mount", "unmount", "format", "check", "repair"} {
			set(name, false, commonReason)
		}
		return result
	}
	if device.Protected {
		reason := "设备受系统保护"
		for _, name := range []string{"mount", "unmount", "format", "check", "repair"} {
			set(name, false, reason)
		}
		return result
	}
	if nonLeaf {
		for _, name := range []string{"mount", "unmount", "format", "check", "repair"} {
			set(name, false, "仅支持叶子块设备")
		}
		return result
	}
	hasFS := device.Filesystem != nil && device.Filesystem.Type != ""
	mounted := len(device.Mounts) != 0
	_, mountErr := m.runner.LookPath("mount")
	_, unmountErr := m.runner.LookPath("umount")
	mountReady := hasFS && !mounted && mountErr == nil
	mountReason := operationReason(hasFS, !mounted, "设备没有可挂载的文件系统", "设备已挂载")
	if mountReason == "" && mountErr != nil {
		mountReason = "mount 工具不可用"
	}
	set("mount", mountReady, mountReason)
	unmountReady := mounted && unmountErr == nil
	unmountReason := operationReason(true, mounted, "", "设备尚未挂载")
	if unmountReason == "" && unmountErr != nil {
		unmountReason = "umount 工具不可用"
	}
	set("unmount", unmountReady, unmountReason)
	formatReady := !mounted && m.anyDiskFormatToolAvailable()
	formatReason := operationReason(true, !mounted, "", "请先卸载设备")
	if formatReason == "" && !m.anyDiskFormatToolAvailable() {
		formatReason = "未安装受支持的格式化工具"
	}
	set("format", formatReady, formatReason)
	checkFS := hasFS && diskSupportedFilesystem(device.Filesystem.Type)
	checkTool := false
	if checkFS {
		checkTool = m.diskFilesystemToolAvailable(device.Filesystem.Type, false)
	}
	checkReady := checkFS && checkTool && !mounted
	checkReason := operationReason(checkFS, !mounted, "当前文件系统不支持检查", "请先卸载设备")
	if checkReason == "" && !checkTool {
		checkReason = "当前文件系统的检查工具不可用"
	}
	set("check", checkReady, checkReason)
	set("repair", checkReady, checkReason)
	return result
}

func (m *Manager) anyDiskFormatToolAvailable() bool {
	for _, filesystem := range []string{"ext4", "xfs", "ntfs", "vfat"} {
		if m.diskFilesystemToolAvailable(filesystem, true) {
			return true
		}
	}
	return false
}

func (m *Manager) diskFilesystemToolAvailable(filesystem string, format bool) bool {
	var alternatives []string
	if format {
		switch filesystem {
		case "ext4":
			alternatives = []string{"mkfs.ext4"}
		case "xfs":
			alternatives = []string{"mkfs.xfs"}
		case "ntfs":
			alternatives = []string{"mkfs.ntfs", "mkntfs"}
		case "vfat":
			alternatives = []string{"mkfs.vfat", "mkfs.fat"}
		}
	} else {
		switch strings.ToLower(filesystem) {
		case "ext4":
			alternatives = []string{"e2fsck"}
		case "xfs":
			alternatives = []string{"xfs_repair"}
		case "ntfs":
			alternatives = []string{"ntfsfix"}
		case "vfat":
			alternatives = []string{"fsck.vfat", "fsck.fat"}
		}
	}
	for _, tool := range alternatives {
		if _, err := m.runner.LookPath(tool); err == nil {
			return true
		}
	}
	return false
}

func operationReason(first, second bool, firstReason, secondReason string) string {
	if !first {
		return firstReason
	}
	if !second {
		return secondReason
	}
	return ""
}

func diskSupportedFilesystem(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ext4", "xfs", "ntfs", "vfat":
		return true
	}
	return false
}

func (m *Manager) diskWriteAvailability() error {
	for _, tool := range []string{"env", "bash"} {
		if _, err := m.runner.LookPath(tool); err != nil {
			return fmt.Errorf("%s is unavailable", tool)
		}
	}
	if err := m.backgroundJobsAvailable(); err != nil {
		return err
	}
	if _, err := m.backgroundExecutable(); err != nil {
		return err
	}
	path, err := m.diskScript()
	if err != nil {
		return fmt.Errorf("trusted kejilion.sh disk protocol is unavailable")
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("trusted kejilion.sh disk protocol path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() <= 0 || info.Size() > resourceScriptMaxBytes || !m.diskScriptOwnerTrusted(info) {
		return fmt.Errorf("trusted kejilion.sh disk protocol is unavailable")
	}
	content, err := readResourceFile(path, resourceScriptMaxBytes)
	if err != nil || len(content) > resourceScriptMaxBytes || !trustedDiskScript(content) {
		return fmt.Errorf("trusted kejilion.sh disk protocol is unavailable")
	}
	return nil
}

func trustedDiskScript(content []byte) bool {
	text := string(content)
	return diskProtocolVersionPattern.Match(content) &&
		strings.Contains(text, "KJ_DISK_MANAGEMENT_NONINTERACTIVE") &&
		strings.Contains(text, "kpanel_disk_management_dispatch")
}

func findKejilionDiskScript() (string, error) {
	candidates := []string{
		"/home/docker/kpanel/bin/kejilion.sh",
		"/usr/local/bin/k",
		"/usr/bin/k",
		"/root/kejilion.sh",
	}
	if candidate, err := exec.LookPath("k"); err == nil {
		candidates = append(candidates, candidate)
	}
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > resourceScriptMaxBytes || info.Mode().Perm()&0022 != 0 || !dnsScriptOwnerTrusted(info) {
			continue
		}
		content, err := readResourceFile(candidate, resourceScriptMaxBytes)
		if err == nil && trustedDiskScript(content) {
			return candidate, nil
		}
	}
	return "", errors.New("a trusted kejilion.sh disk-management command was not found")
}

func (m *Manager) diskPlatform() contract.DiskPlatform {
	result := contract.DiskPlatform{Kind: "linux", Label: "Linux", Writable: true}
	if runtime.GOOS != "linux" {
		return contract.DiskPlatform{Kind: "unknown", Label: runtime.GOOS, Writable: false, Reason: "仅支持 Linux"}
	}
	release, _ := os.ReadFile(filepath.Join(m.procRoot, "sys", "kernel", "osrelease"))
	lowerRelease := strings.ToLower(string(release))
	isWSL := strings.Contains(lowerRelease, "microsoft") || strings.Contains(lowerRelease, "wsl")
	containerDetected := false
	if !isWSL {
		if _, err := os.Stat("/.dockerenv"); err == nil {
			containerDetected = true
		}
	}
	if marker, err := os.ReadFile(filepath.Join(m.runRoot, "systemd", "container")); err == nil {
		kind := strings.ToLower(strings.TrimSpace(string(marker)))
		if kind != "" && kind != "wsl" {
			containerDetected = true
		}
	}
	if _, err := os.Stat(filepath.Join(m.procRoot, "1", "cgroup")); err == nil {
		content, _ := os.ReadFile(filepath.Join(m.procRoot, "1", "cgroup"))
		lower := strings.ToLower(string(content))
		if strings.Contains(lower, "docker") || strings.Contains(lower, "containerd") || strings.Contains(lower, "kubepods") {
			containerDetected = true
		}
	}
	if containerDetected {
		return contract.DiskPlatform{Kind: "container", Label: "Linux container", Writable: false, Reason: "容器内禁止磁盘写入"}
	}
	if isWSL {
		if strings.Contains(lowerRelease, "wsl2") || strings.Contains(lowerRelease, "microsoft-standard") {
			result.Kind, result.Label = "wsl2", "WSL 2"
		} else {
			result = contract.DiskPlatform{Kind: "wsl1", Label: "WSL 1", Writable: false, Reason: "WSL 1 不支持块设备写入"}
		}
	}
	return result
}

func diskMountpoints(node lsblkNode) ([]string, error) {
	raw := node.Mountpoints
	if len(raw) == 0 || string(raw) == "null" {
		raw = node.Mountpoint
	}
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}
	var many []any
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &many); err != nil {
			return nil, errors.New("invalid lsblk mountpoints")
		}
	} else {
		var one any
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, errors.New("invalid lsblk mountpoint")
		}
		many = []any{one}
	}
	result := make([]string, 0, len(many))
	for _, value := range many {
		if value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("invalid lsblk mountpoint type")
		}
		text = strings.TrimSpace(text)
		if text != "" && strings.HasPrefix(text, "/") {
			result = append(result, text)
		}
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func diskUint(raw json.RawMessage) (uint64, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, err
	}
	switch typed := value.(type) {
	case json.Number:
		return strconv.ParseUint(string(typed), 10, 64)
	case string:
		return strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
	default:
		return 0, errors.New("not an unsigned integer")
	}
}

func diskBool(raw json.RawMessage) bool {
	value := strings.Trim(strings.TrimSpace(string(raw)), "\"")
	return value == "1" || strings.EqualFold(value, "true")
}

func opaqueDiskID(node lsblkNode) string {
	source := "majmin\x00" + strings.TrimSpace(node.MajorMinor)
	for _, candidate := range []struct{ kind, value string }{
		{"partuuid", node.PartUUID}, {"uuid", node.UUID}, {"wwn", node.WWN},
		{"serial", strings.TrimSpace(node.Serial) + "\x00" + strings.TrimSpace(node.Model)},
	} {
		if strings.Trim(candidate.value, "\x00 ") != "" {
			source = candidate.kind + "\x00" + strings.TrimSpace(candidate.value)
			break
		}
	}
	source += "\x00type\x00" + strings.ToLower(strings.TrimSpace(node.Type))
	if strings.ToLower(strings.TrimSpace(node.Type)) != "disk" && strings.TrimSpace(node.PartUUID) == "" && strings.TrimSpace(node.UUID) == "" {
		source += "\x00partition\x00" + strings.TrimSpace(node.MajorMinor)
	}
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func disambiguateDiskIDs(nodes []diskInventoryNode) {
	counts := make(map[string]int, len(nodes))
	for _, node := range nodes {
		counts[node.id]++
	}
	for index := range nodes {
		if counts[nodes[index].id] < 2 {
			continue
		}
		source := "collision\x00" + nodes[index].id + "\x00" + strings.TrimSpace(nodes[index].Type) + "\x00" + strings.TrimSpace(nodes[index].MajorMinor)
		sum := sha256.Sum256([]byte(source))
		nodes[index].id = hex.EncodeToString(sum[:])
	}
}

func diskResourceVersion(devices []contract.DiskDevice, targets []diskTarget) string {
	type versionMount struct {
		Path       string `json:"path"`
		Persistent bool   `json:"persistent"`
	}
	type versionDevice struct {
		ID, ParentID, Type string
		Size               uint64
		ReadOnly           bool
		FS                 *contract.DiskFilesystem
		Mounts             []versionMount
		ProtectionReasons  []string
		PersistentMounts   []string
		Holders            []string
	}
	persistentByID := make(map[string][]string, len(targets))
	holdersByID := make(map[string][]string, len(targets))
	for _, target := range targets {
		persistentByID[target.ID] = slices.Clone(target.PersistentMounts)
		slices.Sort(persistentByID[target.ID])
		holdersByID[target.ID] = slices.Clone(target.Holders)
		slices.Sort(holdersByID[target.ID])
	}
	values := make([]versionDevice, 0, len(devices))
	for _, device := range devices {
		mounts := make([]versionMount, 0, len(device.Mounts))
		for _, mount := range device.Mounts {
			mounts = append(mounts, versionMount{Path: mount.Path, Persistent: mount.Persistent})
		}
		sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
		reasons := slices.Clone(device.ProtectionReasons)
		slices.Sort(reasons)
		values = append(values, versionDevice{
			device.ID, device.ParentID, device.Type, device.SizeBytes, device.ReadOnly,
			device.Filesystem, mounts, reasons, persistentByID[device.ID], holdersByID[device.ID],
		})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	data, _ := json.Marshal(values)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) diskHolders(kname string) []string {
	directory, err := os.Open(filepath.Join(m.sysRoot, "class", "block", filepath.Base(kname), "holders"))
	if err != nil {
		return nil
	}
	defer directory.Close()
	names, err := directory.Readdirnames(129)
	if err != nil && err != io.EOF {
		return []string{"inventory-unavailable"}
	}
	if len(names) > 128 {
		return []string{"holder-limit-exceeded"}
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			result = append(result, name)
		}
	}
	slices.Sort(result)
	return result
}

func (m *Manager) diskPersistentMounts(nodes []diskInventoryNode) (map[string]bool, map[string][]string) {
	result := make(map[string]bool)
	byDevice := make(map[string][]string)
	uuidPaths := make(map[string]string, len(nodes)*2)
	devicePaths := make(map[string]string, len(nodes)*2)
	for _, node := range nodes {
		devicePaths[node.Path] = node.Path
		devicePaths[trustedDiskDevicePath(node.Path)] = node.Path
		if uuid := strings.TrimSpace(node.UUID); uuid != "" {
			uuidPaths["UUID="+uuid] = node.Path
		}
		if uuid := strings.TrimSpace(node.PartUUID); uuid != "" {
			uuidPaths["PARTUUID="+uuid] = node.Path
		}
	}
	data, err := readResourceFile(filepath.Join(m.etcRoot, "fstab"), 1<<20)
	if err != nil {
		return result, byDevice
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		spec, mountpoint := unescapeFstab(fields[0]), unescapeFstab(fields[1])
		devicePath := ""
		if strings.HasPrefix(spec, "/dev/") {
			devicePath = devicePaths[spec]
			if devicePath == "" {
				devicePath = devicePaths[trustedDiskDevicePath(spec)]
			}
		} else {
			devicePath = uuidPaths[spec]
		}
		if devicePath != "" && strings.HasPrefix(mountpoint, "/") {
			result[devicePath+"\x00"+mountpoint] = true
			byDevice[devicePath] = append(byDevice[devicePath], mountpoint)
		}
	}
	for path := range byDevice {
		slices.Sort(byDevice[path])
		byDevice[path] = slices.Compact(byDevice[path])
	}
	return result, byDevice
}

func unescapeFstab(value string) string {
	replacer := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\134", "\\", "\\043", "#")
	return replacer.Replace(value)
}

func (m *Manager) diskActiveSwap() map[string]bool {
	result := make(map[string]bool)
	data, err := readResourceFile(filepath.Join(m.procRoot, "swaps"), 1<<20)
	if err != nil {
		return result
	}
	for index, line := range strings.Split(string(data), "\n") {
		if index == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 0 && strings.HasPrefix(fields[0], "/dev/") {
			result[fields[0]] = true
			result[trustedDiskDevicePath(fields[0])] = true
		}
	}
	return result
}

func deviceForProtectedPath(nodes []diskInventoryNode, path string) string {
	best, bestLength := "", -1
	for _, node := range nodes {
		for _, mountpoint := range node.mounts {
			matches := mountpoint == "/" || path == mountpoint || strings.HasPrefix(path, strings.TrimSuffix(mountpoint, "/")+"/")
			if matches && len(mountpoint) > bestLength {
				best, bestLength = node.Path, len(mountpoint)
			}
		}
	}
	return best
}

func diskProtectedMountPoint(mountPoint string) bool {
	if mountPoint == "/" {
		return true
	}
	for _, critical := range []string{"/dev", "/proc", "/sys", "/run", "/boot", "/home", "/var/lib/kejilion-panel"} {
		if mountPoint == critical || strings.HasPrefix(mountPoint, critical+"/") {
			return true
		}
	}
	return false
}

func diskVirtual(node diskInventoryNode) bool {
	value := strings.ToLower(strings.TrimSpace(node.Type))
	return value == "loop" || value == "lvm" || value == "crypt" || strings.HasPrefix(value, "raid") || strings.HasPrefix(filepath.Base(node.Path), "dm-")
}

func trustedDiskDevicePath(displayPath string) string {
	resolved, err := diskEvalSymlinks(displayPath)
	if err == nil && strings.HasPrefix(resolved, "/dev/") {
		return filepath.Clean(resolved)
	}
	return path.Clean(displayPath)
}

var diskEvalSymlinks = filepath.EvalSymlinks

func validateDiskEnvelope(envelope diskInspectEnvelope) error {
	if len(envelope.Snapshot.Devices) == 0 || len(envelope.Snapshot.Devices) > diskMaxDevices {
		return errors.New("invalid device count")
	}
	if len(envelope.Targets) != len(envelope.Snapshot.Devices) {
		return errors.New("target count does not match device count")
	}
	if !resourceVersionPattern.MatchString(envelope.Snapshot.ResourceVersion) {
		return errors.New("invalid resource version")
	}
	switch envelope.Snapshot.Platform.Kind {
	case "linux", "wsl1", "wsl2", "container", "unknown":
	default:
		return errors.New("invalid platform kind")
	}
	ids := make(map[string]bool, len(envelope.Targets))
	for _, target := range envelope.Targets {
		if !resourceVersionPattern.MatchString(target.ID) || !strings.HasPrefix(target.Path, "/dev/") || len(target.Path) > 4096 || !diskMajorMinorExpr.MatchString(target.MajorMinor) || ids[target.ID] || len(target.PersistentMounts) > diskMaxDevices || len(target.Holders) > 128 {
			return errors.New("invalid or duplicate disk target")
		}
		ids[target.ID] = true
	}
	for _, device := range envelope.Snapshot.Devices {
		if !ids[device.ID] || len(device.Path) > 4096 || !strings.HasPrefix(device.Path, "/dev/") || len(device.Name) > 256 || len(device.Mounts) > diskMaxDevices || len(device.ProtectionReasons) > 32 {
			return errors.New("invalid disk device receipt")
		}
		if device.ParentID != "" && !ids[device.ParentID] {
			return errors.New("invalid disk parent identity")
		}
		for _, mount := range device.Mounts {
			if !strings.HasPrefix(mount.Path, "/") || len(mount.Path) > contract.DiskMountPointMaxBytes {
				return errors.New("invalid disk mount receipt")
			}
		}
	}
	return nil
}

func randomDiskID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		sum := sha256.Sum256([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
		return hex.EncodeToString(sum[:16])
	}
	return hex.EncodeToString(raw[:])
}
