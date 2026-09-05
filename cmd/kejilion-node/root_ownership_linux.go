//go:build linux

package main

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

func repairLegacyNodeConfigAccess() error {
	account, err := user.Lookup("kejilion-node")
	if err != nil {
		return err
	}
	group, err := user.LookupGroupId(account.Gid)
	if err != nil || group.Name != "kejilion-node" {
		return errors.New("telemetry service group is unavailable")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid <= 0 {
		return errors.New("telemetry service group is unsafe")
	}
	return repairLegacyNodeConfigAccessAt(defaultConfigPath, gid)
}

func repairLegacyNodeConfigAccessAt(path string, gid int) error {
	directory, err := os.Lstat(filepath.Dir(path))
	if err != nil || !directory.IsDir() || directory.Mode().Perm() != 0o750 || !rootOwned(directory) {
		return errors.New("telemetry configuration directory is unsafe")
	}
	if stat, ok := directory.Sys().(*syscall.Stat_t); !ok || int(stat.Gid) != gid {
		return errors.New("telemetry configuration directory group differs from installer")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !secureMigrationFileInfo(info) || (info.Mode().Perm() != 0o600 && info.Mode().Perm() != 0o640) {
		return errors.New("telemetry configuration permissions are unsafe")
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Gid != 0 && int(stat.Gid) != gid {
		return errors.New("telemetry configuration group differs from installer")
	}
	if err := file.Chown(0, gid); err != nil {
		return err
	}
	return file.Chmod(0o640)
}

func preserveNodeConfigAccess(file *os.File, previous os.FileInfo) error {
	if previous == nil {
		return file.Chmod(0o600)
	}
	stat, ok := previous.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("configuration ownership is unavailable")
	}
	if err := file.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}
	return file.Chmod(previous.Mode().Perm())
}

func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// The staged release and migration targets must be root-owned. The group
	// may intentionally be kejilion-node for the low-privilege relay.
	return ok && stat.Uid == 0
}
