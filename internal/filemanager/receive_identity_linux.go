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

func stableReceiveIdentity(info os.FileInfo, _ ...*os.File) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("inode:%d:%d", stat.Dev, stat.Ino)
	}
	return ""
}

func archiveStagePrivate(info os.FileInfo, _ ...*fileRoot) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.IsDir() && info.Mode().Perm()&0077 == 0
}

func archiveWorkspaceProtected(info os.FileInfo, _ ...*fileRoot) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.IsDir() && info.Mode().Perm()&0022 == 0
}

// A destructive cross-filesystem operation may remove its source only when
// other local users cannot replace the published directory entry. Sticky
// directories are not sufficient because the copied object may retain another
// user's ownership and remain replaceable by that user.
func publicationParentProtected(info os.FileInfo, roots ...*fileRoot) bool {
	if !info.IsDir() || len(roots) != 1 {
		return false
	}
	opened, err := roots[0].Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return false
	}
	stat, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return false
	}
	return opened.Mode().Perm()&0022 == 0
}

func archiveSameFilesystem(a, b os.FileInfo) bool {
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Dev == y.Dev
}

// Moving the workspace to a trusted ancestor must not change the selected
// destination's default ACL inheritance. Only the default ACL is copied: the
// outer staging directory's access permissions must remain private.
func inheritArchiveDestinationDefaults(stage, target *fileRoot) error {
	source, err := target.Open(".")
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := stage.Open(".")
	if err != nil {
		return err
	}
	defer destination.Close()
	const name = "system.posix_acl_default"
	size, err := unix.Fgetxattr(int(source.Fd()), name, nil)
	if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
		err = unix.Fremovexattr(int(destination.Fd()), name)
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	if size > 64<<10 {
		return ErrTooLarge
	}
	value := make([]byte, size)
	count, err := unix.Fgetxattr(int(source.Fd()), name, value)
	if err != nil {
		return err
	}
	return unix.Fsetxattr(int(destination.Fd()), name, value[:count], 0)
}

func openOrdinaryCopyRoot(stage, target *fileRoot) (*fileRoot, error) {
	if err := inheritArchiveDestinationDefaults(stage, target); err != nil {
		return nil, err
	}
	return stage, nil
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
