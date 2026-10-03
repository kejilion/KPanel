//go:build windows

package filemanager

// Windows does not offer a supported directory fsync. Every modified regular
// file is flushed before handle-relative publication; do not claim a durability
// guarantee for a power loss between the rename and filesystem journal commit.
func syncRootDirectory(any, string) error {
	return nil
}
