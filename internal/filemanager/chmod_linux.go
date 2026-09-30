//go:build linux

package filemanager

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// chmodNoFollow changes the mode of the object validated at virtual. Every
// path component is opened with O_NOFOLLOW, so a symbolic link swapped in after
// validation is rejected instead of redirecting a root chmod to another file.
// The final object must also be the inode that was validated. The mode change
// goes through /proc/self/fd, as glibc's fchmodat(AT_SYMLINK_NOFOLLOW) does,
// because Linux rejects fchmod on the O_PATH descriptor that safely opens any
// file type.
func (m *Manager) chmodNoFollow(virtual string, mode os.FileMode, validated os.FileInfo) error {
	parts := strings.Split(strings.TrimPrefix(virtual, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ErrRootOperation
	}
	directory, err := unix.Open(m.root, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(directory) }()
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(directory, part, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return noFollowError(err)
		}
		_ = unix.Close(directory)
		directory = next
	}
	target, err := unix.Openat(directory, parts[len(parts)-1], unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return noFollowError(err)
	}
	defer func() { _ = unix.Close(target) }()
	var opened unix.Stat_t
	if err := unix.Fstat(target, &opened); err != nil {
		return err
	}
	if opened.Mode&unix.S_IFMT == unix.S_IFLNK {
		return ErrSymlink
	}
	expected, ok := validated.Sys().(*syscall.Stat_t)
	if !ok || uint64(expected.Dev) != uint64(opened.Dev) || expected.Ino != opened.Ino {
		return ErrConflict
	}
	return unix.Chmod("/proc/self/fd/"+strconv.Itoa(target), uint32(mode.Perm()))
}

func noFollowError(err error) error {
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		return ErrSymlink
	}
	return err
}
