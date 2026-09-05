//go:build !linux

package main

import "os"

func preserveNodeConfigAccess(file *os.File, previous os.FileInfo) error {
	if previous != nil {
		return file.Chmod(previous.Mode().Perm())
	}
	return file.Chmod(0o600)
}

func rootOwned(os.FileInfo) bool {
	return false
}
