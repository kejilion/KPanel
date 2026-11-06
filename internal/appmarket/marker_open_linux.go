package appmarket

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Pin the directory entry without opening a substituted device or FIFO. The
// proc descriptor reopens that exact inode, not the attacker-controlled path.
func openMarkerFile(root *os.Root) (*os.File, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	fd, err := unix.Openat(int(dir.Fd()), "appno.txt", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	path := os.NewFile(uintptr(fd), "appno.txt")
	defer path.Close()
	before, err := path.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() > maxAppMarkersBytes {
		return nil, errors.New("unsafe application marker")
	}
	file, err := os.OpenFile(fmt.Sprintf("/proc/self/fd/%d", path.Fd()), os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		file.Close()
		return nil, errors.New("application marker changed")
	}
	return file, nil
}
