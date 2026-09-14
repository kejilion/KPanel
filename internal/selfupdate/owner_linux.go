//go:build linux

package selfupdate

import (
	"os"
	"syscall"
)

func trustedFileOwner(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

func trustedStateDirectory(info os.FileInfo) bool {
	return trustedFileOwner(info) && info.Mode().Perm() == 0700
}

func trustedStateFile(info os.FileInfo) bool {
	return trustedFileOwner(info) && info.Mode().Perm()&0077 == 0
}
