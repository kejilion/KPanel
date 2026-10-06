//go:build !linux

package filemanager

import (
	"errors"
	"os"
)

func renameNoReplaceRoot(root *fileRoot, oldVirtual, newVirtual string) error {
	if _, err := root.Lstat(rootName(newVirtual)); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return root.Rename(rootName(oldVirtual), rootName(newVirtual))
}
