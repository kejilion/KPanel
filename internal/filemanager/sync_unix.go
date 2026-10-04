//go:build !windows

package filemanager

import "os"

func syncRootDirectory(root interface {
	Open(string) (*os.File, error)
}, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
