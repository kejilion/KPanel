//go:build !linux && !windows

package filemanager

import "os"

func preserveFileExtendedAttributes(_, _ *os.File) error {
	return nil
}
