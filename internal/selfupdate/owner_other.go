//go:build !linux

package selfupdate

import "os"

func trustedFileOwner(os.FileInfo) bool { return true }

func trustedStateDirectory(os.FileInfo) bool { return true }

func trustedStateFile(os.FileInfo) bool { return true }
