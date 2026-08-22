//go:build linux

package filemanager

import (
	"fmt"
	"os"
	"syscall"
)

// shareFileIdentity binds a share to the Linux filesystem object as well as
// its content. The inode fields invalidate same-content replacements; ctime is
// supplementary metadata and is not treated as a monotonic change counter.
func shareFileIdentity(info os.FileInfo) (string, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec), true
}
