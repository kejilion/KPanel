//go:build linux

package agent

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var errStableFileUnavailable = errors.New("stable file lease unavailable")

type stableReadLease struct {
	fd      uintptr
	signals chan os.Signal
	stop    chan struct{}
	done    chan struct{}
	broken  atomic.Bool
	once    sync.Once
}

// tryStableReadLease fails immediately if any process already holds a writable
// handle/mapping. A competing writer triggers a break; release the lease and
// reject this snapshot rather than holding the writer until a long timeout.
// Only local filesystems with Linux lease semantics are eligible.
func tryStableReadLease(file interface{ Fd() uintptr }, onBreak func()) (*stableReadLease, error) {
	fd := file.Fd()
	var info unix.Statfs_t
	if err := unix.Fstatfs(int(fd), &info); err != nil {
		return nil, errStableFileUnavailable
	}
	switch uint64(info.Type) {
	case 0xef53, 0x58465342, 0x9123683e, 0x01021994: // ext family, XFS, Btrfs, tmpfs
	default:
		return nil, errStableFileUnavailable
	}
	lease := &stableReadLease{fd: fd, signals: make(chan os.Signal, 1), stop: make(chan struct{}), done: make(chan struct{})}
	signal.Notify(lease.signals, syscall.SIGIO)
	if _, err := unix.FcntlInt(fd, unix.F_SETOWN, os.Getpid()); err != nil {
		signal.Stop(lease.signals)
		return nil, errStableFileUnavailable
	}
	if _, err := unix.FcntlInt(fd, unix.F_SETLEASE, unix.F_RDLCK); err != nil {
		signal.Stop(lease.signals)
		return nil, errStableFileUnavailable
	}
	go func() {
		defer close(lease.done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-lease.stop:
				return
			case <-lease.signals:
				lease.broken.Store(true)
				_, _ = unix.FcntlInt(fd, unix.F_SETLEASE, unix.F_UNLCK)
				if onBreak != nil {
					onBreak()
				}
				return
			case <-ticker.C:
				if state, err := unix.FcntlInt(fd, unix.F_GETLEASE, 0); err != nil || state != unix.F_RDLCK {
					lease.broken.Store(true)
					_, _ = unix.FcntlInt(fd, unix.F_SETLEASE, unix.F_UNLCK)
					if onBreak != nil {
						onBreak()
					}
					return
				}
			}
		}
	}()
	return lease, nil
}

func (l *stableReadLease) valid() bool {
	if l.broken.Load() {
		return false
	}
	state, err := unix.FcntlInt(l.fd, unix.F_GETLEASE, 0)
	return err == nil && state == unix.F_RDLCK && !l.broken.Load()
}

func (l *stableReadLease) close() {
	l.once.Do(func() {
		close(l.stop)
		<-l.done
		_, _ = unix.FcntlInt(l.fd, unix.F_SETLEASE, unix.F_UNLCK)
		signal.Stop(l.signals)
	})
}
