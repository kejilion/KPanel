//go:build !linux

package filemanager

import "os"

func stableReceiveIdentity(os.FileInfo) string { return "" }
