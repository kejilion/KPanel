//go:build !windows

package hostbackup

import (
	"errors"
	"os"
	"syscall"
)

func restoreOwnership(path string, uid, gid int) error        { return os.Chown(path, uid, gid) }
func restoreSymlinkOwnership(path string, uid, gid int) error { return os.Lchown(path, uid, gid) }
func copyOwnership(path string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("file ownership unavailable")
	}
	return os.Chown(path, int(st.Uid), int(st.Gid))
}
func copySymlinkOwnership(path string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("symbolic link ownership unavailable")
	}
	return os.Lchown(path, int(st.Uid), int(st.Gid))
}
