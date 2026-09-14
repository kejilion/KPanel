//go:build linux

package jobcontrol

import (
	"os"
	"syscall"
)

func trustedOwner(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
