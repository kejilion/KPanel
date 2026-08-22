//go:build !linux

package filemanager

import "os"

// KPanel's supported production host is Linux. Keep local tooling on other
// platforms functional while the ordinary resource version provides the
// conservative fallback identity there.
func shareFileIdentity(os.FileInfo) string {
	return ""
}
