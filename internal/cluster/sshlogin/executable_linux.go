//go:build linux

package sshlogin

import (
	"os"
	"path/filepath"
	"syscall"
)

// System applet symlinks are allowed, but neither their directories nor the
// resolved executable may be writable by an unprivileged account.
func trustedLogExecutable(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(resolved) {
		return false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || !rootOwnedLogPath(info) {
		return false
	}
	for _, start := range []string{filepath.Dir(path), filepath.Dir(resolved)} {
		for directory := start; ; directory = filepath.Dir(directory) {
			info, err := os.Stat(directory)
			if err != nil || !info.IsDir() || !rootOwnedLogPath(info) {
				return false
			}
			if directory == filepath.Dir(directory) {
				break
			}
		}
	}
	return true
}

func rootOwnedLogPath(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0
}
