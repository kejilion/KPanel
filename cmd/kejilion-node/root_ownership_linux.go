//go:build linux

package main

import (
	"errors"
	"os"
	"syscall"
)

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
