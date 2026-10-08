//go:build linux

package filemanager

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func stableReceiveIdentity(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("inode:%d:%d", stat.Dev, stat.Ino)
	}
	return ""
}

func receiveFileTimes(_ *fileRoot, _ string, file *os.File, when time.Time) error {
	stamp := unix.Timespec{Sec: when.Unix(), Nsec: int64(when.Nanosecond())}
	times := []unix.Timespec{stamp, stamp}
	err := unix.UtimesNanoAt(int(file.Fd()), "", times, unix.AT_EMPTY_PATH)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.ENOENT) {
		// Older Linux kernels lack AT_EMPTY_PATH for utimensat. /proc resolves
		// the already-open descriptor, never the replaceable staging name.
		return unix.UtimesNanoAt(unix.AT_FDCWD, "/proc/self/fd/"+strconv.Itoa(int(file.Fd())), times, 0)
	}
	return err
}

// Link the held inode into a private publication root. Never resolve the
// replaceable original staging basename after validation.
func stageReceiveFile(source *os.File, stage *fileRoot, name string) (os.FileInfo, error) {
	directory, err := stage.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	err = unix.Linkat(int(source.Fd()), "", int(directory.Fd()), name, unix.AT_EMPTY_PATH)
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOENT) {
		err = unix.Linkat(unix.AT_FDCWD, "/proc/self/fd/"+strconv.Itoa(int(source.Fd())), int(directory.Fd()), name, unix.AT_SYMLINK_FOLLOW)
	}
	if err != nil {
		return nil, err
	}
	info, err := stage.Lstat(name)
	opened, sourceErr := source.Stat()
	if err != nil || sourceErr != nil || !os.SameFile(info, opened) {
		return nil, ErrConflict
	}
	return info, nil
}

func publishReceiveObject(source *fileRoot, sourceName string, target *fileRoot, targetName string, replace bool) error {
	if path.Base(sourceName) != sourceName || path.Base(targetName) != targetName {
		return ErrInvalidPath
	}
	from, err := source.Open(".")
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := target.Open(".")
	if err != nil {
		return err
	}
	defer to.Close()
	flags := uint(unix.RENAME_NOREPLACE)
	if replace {
		flags = 0
	}
	return unix.Renameat2(int(from.Fd()), sourceName, int(to.Fd()), targetName, flags)
}
