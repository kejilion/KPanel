//go:build !linux

package filemanager

import "os"

// KPanel's supported production host is Linux. Keep local tooling on other
// platforms functional with the content digest and ordinary resource version;
// Linux adds the persistent object identity needed for same-content replacement.
func shareFileIdentity(os.FileInfo) (string, bool) {
	return "", true
}
