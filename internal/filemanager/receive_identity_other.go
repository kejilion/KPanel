//go:build !linux

package filemanager

import (
	"os"
	"time"
)

func stableReceiveIdentity(os.FileInfo) string { return "" }

func receiveFileTimes(root *fileRoot, name string, _ *os.File, when time.Time) error {
	return root.Chtimes(name, when, when)
}
