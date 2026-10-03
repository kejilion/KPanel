//go:build !linux && !windows

package filemanager

import "os"

// chmodNoFollow keeps the rooted path-based change on platforms that do not
// run the host Agent. Linux uses descriptor-based resolution instead.
func (m *Manager) chmodNoFollow(virtual string, mode os.FileMode, _ os.FileInfo) error {
	return m.rootFS.Chmod(rootName(virtual), mode)
}
