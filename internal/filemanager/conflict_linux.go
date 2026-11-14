//go:build linux

package filemanager

import (
	"golang.org/x/sys/unix"
	"os"
)

func removeCheckedEmptyDirectory(parent *fileRoot, name string, expected os.FileInfo) error {
	file, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer file.Close()
	current, err := parent.Lstat(name)
	if err != nil || !os.SameFile(current, expected) {
		return ErrConflict
	}
	// AT_REMOVEDIR cannot unlink a file or symlink substituted after Lstat.
	return unix.Unlinkat(int(file.Fd()), name, unix.AT_REMOVEDIR)
}
