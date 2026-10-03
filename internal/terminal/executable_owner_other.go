//go:build !linux

package terminal

import "os"

func terminalExecutableOwnerTrusted(os.FileInfo) bool {
	return false
}
