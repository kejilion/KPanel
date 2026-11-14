//go:build windows

package filemanager

import (
	"golang.org/x/sys/windows"
	"os"
)

func removeCheckedEmptyDirectory(root *fileRoot, name string, expected os.FileInfo) error {
	root.mu.RLock()
	defer root.mu.RUnlock()
	if root.closed {
		return os.ErrClosed
	}
	parent, leaf, volume, err := root.parent(name)
	if err != nil {
		return err
	}
	defer closeParent(parent, volume)
	handle, err := ntOpen(parent, leaf, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), leaf)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(info, expected) || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrConflict
	}
	flag := byte(1)
	return ntError(windows.NtSetInformationFile(handle, &windows.IO_STATUS_BLOCK{}, &flag, 1, windows.FileDispositionInformation))
}
