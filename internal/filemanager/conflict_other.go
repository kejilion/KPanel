//go:build !linux && !windows

package filemanager

import "os"

func removeCheckedEmptyDirectory(_ *fileRoot, _ string, _ os.FileInfo) error { return ErrAction }
