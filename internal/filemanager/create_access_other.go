//go:build !windows

package filemanager

import (
	"os"
	"path"
)

// User-created directories must be traversable by web servers even when the
// Agent runs with umask 0077. Keep the parent and chmod target pinned.
func mkdirUserDirectory(root *fileRoot, name string) error {
	parent, err := root.OpenRoot(path.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Close()
	base := path.Base(name)
	if err := parent.Mkdir(base, 0700); err != nil {
		return err
	}
	directory, err := parent.Open(base)
	if err != nil {
		return err
	}
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrConflict
	}
	if err := directory.Chmod(0755); err != nil {
		return err
	}
	actual, err := directory.Stat()
	if err != nil {
		return err
	}
	visible, err := root.Lstat(name)
	if err != nil || !os.SameFile(actual, visible) || actual.Mode().Perm() != 0755 {
		return ErrConflict
	}
	return nil
}

func openAccessTarget(root *fileRoot, name string) (*os.File, error) { return root.Open(name) }

// Source-preserving operations start private; final metadata is applied only
// after copying content. Ordinary copies inherit the destination policy.
func createFileWithSourceAccess(root *fileRoot, name string, flags int, mode os.FileMode, source *os.File) (*os.File, error) {
	if source != nil {
		mode = 0600
	}
	return root.OpenFile(name, flags, mode)
}

func mkdirWithSourceAccess(root *fileRoot, name string, mode os.FileMode, source *os.File) error {
	if source != nil {
		mode = 0700
	}
	return root.Mkdir(name, mode)
}
