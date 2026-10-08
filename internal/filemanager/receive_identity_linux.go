//go:build linux

package filemanager

import (
	"fmt"
	"os"
	"syscall"
)

func stableReceiveIdentity(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("inode:%d:%d", stat.Dev, stat.Ino)
	}
	return ""
}
