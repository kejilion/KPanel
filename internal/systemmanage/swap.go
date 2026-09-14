package systemmanage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

const swapUnitPrefix = "kejilion-panel-swap-"

type SwapTransactionResult struct {
	Changed    bool   `json:"changed"`
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message,omitempty"`
	ErrorCode  string `json:"errorCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

type managedSwapFile struct {
	path       string
	backupPath string
	existed    bool
	active     bool
	size       int64
	moved      bool
}

// RunSwapTransaction is called only by the Agent's root-only swap-run
// subcommand. It accepts a validated size and operates on the two fixed paths
// used by kejilion.sh and older KPanel releases.
func (m *Manager) RunSwapTransaction(ctx context.Context, sizeMiB int) SwapTransactionResult {
	changed, backup, message, err := m.setSwap(ctx, sizeMiB)
	if err == nil {
		return SwapTransactionResult{
			Changed: changed, BackupPath: backup, Message: message,
		}
	}
	return SwapTransactionResult{
		BackupPath: backup,
		ErrorCode:  swapErrorCode(err),
		Error:      err.Error(),
	}
}

func (m *Manager) runSwapViaSystemd(
	ctx context.Context,
	sizeMiB int,
) (bool, string, string, error) {
	if err := validateSwapSize(sizeMiB); err != nil {
		return false, "", "", err
	}
	executable, err := m.backgroundExecutable()
	if err != nil {
		return false, "", "", err
	}

	spec := m.backgroundJobSpec(
		swapUnitPrefix+idForMaintenance(m.now()),
		executable,
		[]string{
			"swap-run", "--state-dir", m.stateDir, "--swap-path", m.swapPath,
			"--size-mib", strconv.Itoa(sizeMiB),
		},
		[]string{
			"Type=oneshot", "TimeoutStartSec=5min", "TimeoutStopSec=1min",
			"User=root", "UMask=0077", "PrivateTmp=yes", "ProtectHome=read-only",
			"ReadWritePaths=" + m.stateDir, "ProtectSystem=no", "NoNewPrivileges=no",
			"RestrictAddressFamilies=AF_UNIX",
			"CapabilityBoundingSet=CAP_SYS_ADMIN CAP_DAC_OVERRIDE CAP_FOWNER",
			"Nice=10", "CPUWeight=20", "IOWeight=20", "SyslogIdentifier=kpanel-swap",
		},
		"0077",
		10,
		time.Minute,
	)
	transactionContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute+15*time.Second)
	defer cancel()
	output, err := jobcontrol.RunForeground(transactionContext, m.runner, spec)
	if err != nil {
		return false, "", "", fmt.Errorf("%w: start swap transaction: %v", ErrUnsupported, err)
	}
	var result SwapTransactionResult
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		return false, "", "", fmt.Errorf("%w: decode swap transaction result: %v", ErrNeedsAttention, err)
	}
	if result.ErrorCode != "" {
		return false, result.BackupPath, "", swapResultError(result.ErrorCode, result.Error)
	}
	return result.Changed, result.BackupPath, result.Message, nil
}

func (m *Manager) applySwap(
	ctx context.Context,
	sizeMiB int,
) (bool, string, string, error) {
	if err := validateSwapSize(sizeMiB); err != nil {
		return false, "", "", err
	}
	primaryPath := m.swapPath
	legacyPath := filepath.Join(m.stateDir, "swapfile")
	if primaryPath == legacyPath {
		legacyPath = ""
	}

	files := []managedSwapFile{{path: primaryPath}}
	if legacyPath != "" {
		files = append(files, managedSwapFile{path: legacyPath})
	}
	for index := range files {
		existed, size, err := regularSwapFile(files[index].path)
		if err != nil {
			return false, "", "", err
		}
		files[index].existed = existed
		files[index].size = size
		files[index].active = m.swapActive(files[index].path)
	}

	fstabPath := filepath.Join(m.etcRoot, "fstab")
	oldFstab, fstabExisted, fstabMode, err := snapshotFile(fstabPath)
	if err != nil {
		return false, "", "", err
	}
	newFstab := updateFstabSwap(oldFstab, primaryPath, legacyPath, sizeMiB > 0)
	primaryReady := files[0].existed && files[0].active &&
		sizeWithinMiB(files[0].size, sizeMiB)
	legacyPresent := len(files) > 1 && (files[1].existed || files[1].active)
	if sizeMiB > 0 && primaryReady && !legacyPresent && bytes.Equal(oldFstab, newFstab) {
		return false, "", fmt.Sprintf("/swapfile 已经是 %d MiB，无需调整", sizeMiB), nil
	}
	if sizeMiB == 0 && !files[0].existed && !files[0].active &&
		!legacyPresent && bytes.Equal(oldFstab, newFstab) {
		return false, "", "/swapfile 已经停用", nil
	}
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		return false, "", "", fmt.Errorf("%w: create system state directory: %v", ErrUnsupported, err)
	}

	backup, err := m.createBackup("swap", fstabPath)
	if err != nil {
		return false, "", "", err
	}
	transactionID := idForMaintenance(m.now())
	tempPath := ""
	if sizeMiB > 0 {
		temp, err := os.CreateTemp(filepath.Dir(primaryPath), ".swapfile.kpanel-new-*")
		if err != nil {
			return false, backup, "", fmt.Errorf("%w: create temporary swapfile: %v", ErrRolledBack, err)
		}
		tempPath = temp.Name()
		if closeErr := temp.Close(); closeErr != nil {
			_ = os.Remove(tempPath)
			return false, backup, "", fmt.Errorf("%w: close temporary swapfile: %v", ErrRolledBack, closeErr)
		}
		if err := os.Chmod(tempPath, 0o600); err != nil {
			_ = os.Remove(tempPath)
			return false, backup, "", fmt.Errorf("%w: protect temporary swapfile: %v", ErrRolledBack, err)
		}
		if _, err := m.runner.Run(ctx, "fallocate", "-l", strconv.Itoa(sizeMiB)+"M", tempPath); err != nil {
			_ = os.Remove(tempPath)
			return false, backup, "", fmt.Errorf("%w: allocate swapfile: %v", ErrRolledBack, err)
		}
		if _, err := m.runner.Run(ctx, "mkswap", tempPath); err != nil {
			_ = os.Remove(tempPath)
			return false, backup, "", fmt.Errorf("%w: mkswap: %v", ErrRolledBack, err)
		}
	}

	newPublished := false
	rollback := func() {
		if newPublished {
			if m.swapActive(primaryPath) {
				_, _ = m.runner.Run(context.Background(), "swapoff", primaryPath)
			}
			_ = os.Remove(primaryPath)
		} else if tempPath != "" {
			_ = os.Remove(tempPath)
		}
		_ = restoreFile(fstabPath, oldFstab, fstabExisted, fstabMode)
		for index := len(files) - 1; index >= 0; index-- {
			if files[index].moved {
				_ = os.Rename(files[index].backupPath, files[index].path)
			}
		}
		for _, file := range files {
			if file.active && !m.swapActive(file.path) {
				_, _ = m.runner.Run(context.Background(), "swapon", file.path)
			}
		}
	}

	for _, file := range files {
		if !file.active {
			continue
		}
		if _, err := m.runner.Run(ctx, "swapoff", file.path); err != nil {
			rollback()
			return false, backup, "", fmt.Errorf("%w: swapoff %s: %v", ErrRolledBack, file.path, err)
		}
	}
	for index := range files {
		if !files[index].existed {
			continue
		}
		files[index].backupPath = files[index].path + ".kpanel-previous-" + transactionID
		if _, err := os.Lstat(files[index].backupPath); err == nil {
			rollback()
			return false, backup, "", fmt.Errorf("%w: temporary rollback path already exists", ErrConflict)
		} else if !errors.Is(err, os.ErrNotExist) {
			rollback()
			return false, backup, "", fmt.Errorf("%w: inspect rollback path: %v", ErrConflict, err)
		}
		if err := os.Rename(files[index].path, files[index].backupPath); err != nil {
			rollback()
			return false, backup, "", fmt.Errorf("%w: preserve %s: %v", ErrRolledBack, files[index].path, err)
		}
		files[index].moved = true
	}
	if err := writeAtomic(fstabPath, newFstab, fileModeOr(fstabMode, 0o644)); err != nil {
		rollback()
		return false, backup, "", fmt.Errorf("%w: update fstab: %v", ErrRolledBack, err)
	}
	if sizeMiB > 0 {
		if err := os.Rename(tempPath, primaryPath); err != nil {
			rollback()
			return false, backup, "", fmt.Errorf("%w: publish /swapfile: %v", ErrRolledBack, err)
		}
		tempPath = ""
		newPublished = true
		if _, err := m.runner.Run(ctx, "swapon", primaryPath); err != nil {
			rollback()
			return false, backup, "", fmt.Errorf("%w: swapon /swapfile: %v", ErrRolledBack, err)
		}
		if !m.swapActive(primaryPath) {
			rollback()
			return false, backup, "", fmt.Errorf("%w: /swapfile activation could not be verified", ErrRolledBack)
		}
	}

	for _, file := range files {
		if !file.moved {
			continue
		}
		if err := os.Remove(file.backupPath); err != nil {
			return true, backup, "", fmt.Errorf("%w: swap changed but old file cleanup failed: %v", ErrNeedsAttention, err)
		}
	}
	if sizeMiB == 0 {
		return true, backup, "已停用 /swapfile 并清理旧版 KPanel Swap；其他 Swap 未受影响", nil
	}
	if legacyPresent {
		return true, backup, fmt.Sprintf(
			"已将 /swapfile 调整为 %d MiB，并合并旧版 KPanel Swap；其他 Swap 未受影响",
			sizeMiB,
		), nil
	}
	return true, backup, fmt.Sprintf(
		"已将 /swapfile 调整为 %d MiB，与 kejilion.sh 使用同一产物；其他 Swap 未受影响",
		sizeMiB,
	), nil
}

func regularSwapFile(path string) (bool, int64, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("%w: inspect %s: %v", ErrConflict, path, err)
	}
	if !info.Mode().IsRegular() {
		return false, 0, fmt.Errorf("%w: %s must be a regular file without symlinks", ErrConflict, path)
	}
	return true, info.Size(), nil
}

func validateSwapSize(sizeMiB int) error {
	if sizeMiB < 0 {
		return fmt.Errorf("%w: swapSizeMiB must be zero or a positive integer", ErrInvalidInput)
	}
	return nil
}

func swapErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrUnsupported):
		return "unsupported"
	case errors.Is(err, ErrConflict):
		return "conflict"
	case errors.Is(err, ErrRolledBack):
		return "rolled_back"
	default:
		return "needs_attention"
	}
}

func swapResultError(code, detail string) error {
	base := ErrNeedsAttention
	switch code {
	case "invalid_input":
		base = ErrInvalidInput
	case "unsupported":
		base = ErrUnsupported
	case "conflict":
		base = ErrConflict
	case "rolled_back":
		base = ErrRolledBack
	}
	if detail == "" {
		detail = "swap transaction failed"
	}
	return fmt.Errorf("%w: %s", base, detail)
}
