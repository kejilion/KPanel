//go:build !windows

package backup

import (
	"errors"
	"golang.org/x/sys/unix"
)

func RequireSpace(path string, bytes int64) error {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return err
	}
	if bytes < 0 || uint64(bytes)+(64<<20) > s.Bavail*uint64(s.Bsize) {
		return errors.New("insufficient backup staging space")
	}
	return nil
}
