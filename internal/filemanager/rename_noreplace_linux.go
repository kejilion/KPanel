//go:build linux

package filemanager

import (
	"path"

	"golang.org/x/sys/unix"
)

func renameNoReplaceRoot(root *fileRoot, oldVirtual, newVirtual string) error {
	oldParent, err := root.Open(rootName(path.Dir(oldVirtual)))
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent, err := root.Open(rootName(path.Dir(newVirtual)))
	if err != nil {
		return err
	}
	defer newParent.Close()
	return unix.Renameat2(
		int(oldParent.Fd()),
		path.Base(oldVirtual),
		int(newParent.Fd()),
		path.Base(newVirtual),
		unix.RENAME_NOREPLACE,
	)
}
