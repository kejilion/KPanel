//go:build linux || darwin || freebsd

package bittorrent

import "golang.org/x/sys/unix"

func checkSpace(path string, size int64) error {
	var stat unix.Statfs_t
	if unix.Statfs(path, &stat) != nil || stat.Bsize <= 0 || uint64(size)+(64<<20) > stat.Bavail*uint64(stat.Bsize) {
		return ErrStorage
	}
	return nil
}
