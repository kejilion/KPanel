//go:build !linux

package jobcontrol

import "os"

func trustedOwner(os.FileInfo) bool { return true }
