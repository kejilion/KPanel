//go:build windows

package filemanager

import "os"

func syncRootDirectory(*os.Root, string) error {
	return nil
}
