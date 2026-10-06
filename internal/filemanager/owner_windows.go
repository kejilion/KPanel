//go:build windows

package filemanager

import "os"

func fileOwner(os.FileInfo) (string, string) {
	return "", ""
}

func preserveFileOwnership(_ *os.File, _ os.FileInfo) error {
	return nil
}
