//go:build linux

package filemanager

import (
	"fmt"
	"os"
	"syscall"
)

// shareFileIdentity adds the Linux object identity and inode-change clock to
// the existing metadata version. Unlike mtime, ctime cannot be restored with
// ordinary filesystem APIs after an in-place rewrite.
func shareFileIdentity(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec)
}
