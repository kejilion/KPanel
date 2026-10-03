//go:build !windows

package filemanager

import "os"

// POSIX creation already applies mode before a file is visible to other users.
// Preserve the existing ownership/xattr handling and copy semantics there.
func createFileWithSourceAccess(root *fileRoot, name string, flags int, mode os.FileMode, _ *os.File) (*os.File, error) {
	return root.OpenFile(name, flags, mode)
}

func mkdirWithSourceAccess(root *fileRoot, name string, mode os.FileMode, _ *os.File) error {
	return root.Mkdir(name, mode)
}
