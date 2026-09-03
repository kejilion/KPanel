package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/cluster/sshlogin"
)

const (
	lightUpdateStagingPrefix = "kejilion-node-update."
	lightUpdateChecksumName  = "SHA256SUMS"
	maxLightChecksumBytes    = int64(64 << 10)
	maxLightBinaryBytes      = int64(128 << 20)

	lightSSHLoginServiceUnit = "/etc/systemd/system/kejilion-node-ssh-login.service"
)

// This is the fixed compatibility bridge used by the root update service. The
// old updater already downloads and verifies the release binary before calling
// its "version" command, so a verified staged binary can install this helper
// without asking users to run a command on every existing node.
const lightSSHLoginService = `[Unit]
Description=KPanel SSH Login Event Collector
After=systemd-journald.service
Wants=systemd-journald.service

[Service]
Type=simple
User=root
Group=kejilion-node
ExecStart=/usr/local/lib/kejilion-node/kejilion-node ssh-login-broker --output ` + sshlogin.EventPath + `
RuntimeDirectory=kejilion-node-ssh
RuntimeDirectoryMode=0750
Restart=always
RestartSec=15s
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictRealtime=true
RestrictNamespaces=true
RestrictAddressFamilies=AF_UNIX
SystemCallArchitectures=native
CapabilityBoundingSet=CAP_DAC_READ_SEARCH
AmbientCapabilities=CAP_DAC_READ_SEARCH
ReadWritePaths=/run/kejilion-node-ssh
UMask=0027

[Install]
WantedBy=multi-user.target
`

// maybeMigrateLegacySSHLoginInstall is intentionally narrow. It only runs for
// the exact release asset passed through the existing root-owned updater and
// only for an already-enrolled lightweight node. A normal "version" command
// therefore remains read-only.
func maybeMigrateLegacySSHLoginInstall() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	stagedBinary, ok, err := verifiedStagedNodeBinary()
	if err != nil || !ok {
		return err
	}
	if !secureMigrationFile(defaultConfigPath) {
		return nil
	}
	if err := installLightNodeSSHLoginIntegration(); err != nil {
		return fmt.Errorf("install from %s: %w", stagedBinary, err)
	}
	return nil
}

func verifiedStagedNodeBinary() (string, bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", false, nil
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", false, nil
	}
	executable = filepath.Clean(executable)
	info, err := os.Lstat(executable)
	if err != nil || !secureMigrationFileInfo(info) {
		return "", false, nil
	}
	directory := filepath.Dir(executable)
	if filepath.Clean(filepath.Dir(directory)) != "/tmp" ||
		!strings.HasPrefix(filepath.Base(directory), lightUpdateStagingPrefix) {
		return "", false, nil
	}
	if !secureMigrationDirectory(directory) {
		return "", false, nil
	}
	expectedName := "kejilion-node-linux-" + runtime.GOARCH
	if filepath.Base(executable) != expectedName {
		return "", false, nil
	}
	checksums, err := readBoundedMigrationFile(filepath.Join(directory, lightUpdateChecksumName), maxLightChecksumBytes)
	if err != nil {
		return "", false, nil
	}
	expected, ok := stagedChecksum(checksums, expectedName)
	if !ok {
		return "", false, nil
	}
	file, err := os.Open(executable)
	if err != nil {
		return "", false, nil
	}
	hasher := sha256.New()
	count, copyErr := io.Copy(hasher, io.LimitReader(file, maxLightBinaryBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || count > maxLightBinaryBytes {
		return "", false, nil
	}
	actual := hasher.Sum(nil)
	if subtle.ConstantTimeCompare(expected, actual) != 1 {
		return "", false, nil
	}
	return executable, true, nil
}

func stagedChecksum(content []byte, name string) ([]byte, bool) {
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != name || len(fields[0]) != sha256.Size*2 {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		if err == nil && len(sum) == sha256.Size {
			return sum, true
		}
	}
	return nil, false
}

func installLightNodeSSHLoginIntegration() error {
	if err := writeMigrationFile(lightSSHLoginServiceUnit, []byte(lightSSHLoginService), 0o644); err != nil {
		return err
	}
	systemctl, err := migrationSystemctlPath()
	if err != nil {
		return errors.New("systemctl is unavailable")
	}
	if err := runMigrationSystemctl(systemctl, "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := runMigrationSystemctl(systemctl, "enable", "kejilion-node-ssh-login.service"); err != nil {
		return fmt.Errorf("enable SSH login collector: %w", err)
	}
	// The staged binary is called before the updater replaces the installed
	// binary. Restart=always keeps the service retrying through that short
	// window, so no second manual update is needed.
	if err := runMigrationSystemctl(systemctl, "start", "--no-block", "kejilion-node-ssh-login.service"); err != nil {
		return fmt.Errorf("start SSH login collector: %w", err)
	}
	return nil
}

func runMigrationSystemctl(path string, arguments ...string) error {
	command := exec.Command(path, arguments...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func migrationSystemctlPath() (string, error) {
	for _, candidate := range []string{"/usr/bin/systemctl", "/bin/systemctl"} {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Lstat(resolved)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 && rootOwned(info) {
			return resolved, nil
		}
	}
	return "", errors.New("systemctl is unavailable")
}

func writeMigrationFile(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := ensureMigrationDirectory(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || !rootOwned(info) {
			return fmt.Errorf("migration target is unsafe: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".kejilion-node-migration-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryHandle.Close()
	return directoryHandle.Sync()
}

func readBoundedMigrationFile(path string, maximum int64) ([]byte, error) {
	if !secureMigrationFile(path) {
		return nil, errors.New("file is not a secure regular non-symlink")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(content)) > maximum {
		return nil, errors.New("file is too large")
	}
	return content, nil
}

func ensureMigrationDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || !rootOwned(info) {
		return fmt.Errorf("migration directory is unsafe: %s", path)
	}
	return os.Chmod(path, 0o755)
}

func secureMigrationDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm()&0o022 == 0 && rootOwned(info)
}

func secureMigrationFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && secureMigrationFileInfo(info)
}

func secureMigrationFileInfo(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm()&0o022 == 0 && rootOwned(info)
}
