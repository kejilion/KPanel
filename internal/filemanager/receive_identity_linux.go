//go:build linux

package filemanager

import (
	"errors"
	"fmt"
	"os"
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
