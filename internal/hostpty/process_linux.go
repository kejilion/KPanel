//go:build linux

package hostpty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type linuxProcess struct {
	*os.File
	command *exec.Cmd
}

func startPlatform(command *exec.Cmd, rows, columns uint16) (Process, error) {
	masterFD, err := unix.Open(
		"/dev/ptmx",
		unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, err
	}
	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	closeMaster := true
	defer func() {
		if closeMaster {
			_ = master.Close()
		}
	}()
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		return nil, err
	}
	number, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(
		fmt.Sprintf("/dev/pts/%d", number),
		os.O_RDWR|syscall.O_NOCTTY,
		0,
	)
	if err != nil {
		return nil, err
	}
	defer slave.Close()
	if err := unix.IoctlSetWinsize(
		masterFD,
		unix.TIOCSWINSZ,
		&unix.Winsize{Row: rows, Col: columns},
	); err != nil {
		return nil, err
	}

	command.Stdin = slave
	command.Stdout = slave
	command.Stderr = slave
	command.SysProcAttr = ptyProcessAttributes()
	if err := command.Start(); err != nil {
		return nil, err
	}
	closeMaster = false
	return &linuxProcess{File: master, command: command}, nil
}

func ptyProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
		// OpenRC does not create a transient scope for the host terminal.
		// Prevent a direct PTY shell from surviving an abrupt Agent/worker exit.
		Pdeathsig: syscall.SIGKILL,
	}
}

func (process *linuxProcess) Wait() error {
	return process.command.Wait()
}

func (process *linuxProcess) Kill() error {
	if process.command.Process == nil {
		return nil
	}
	err := syscall.Kill(-process.command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (process *linuxProcess) Resize(rows, columns uint16) error {
	if rows == 0 || columns == 0 {
		return errors.New("terminal dimensions must be positive")
	}
	return unix.IoctlSetWinsize(
		int(process.Fd()),
		unix.TIOCSWINSZ,
		&unix.Winsize{Row: rows, Col: columns},
	)
}

func createPlatformInput(path string) error {
	_ = unix.Unlink(path)
	return unix.Mkfifo(path, 0o600)
}

func openPlatformInput(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0)
}

func writePlatformInput(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(data) > 0 {
		written, writeErr := unix.Write(int(file.Fd()), data)
		if written > 0 {
			data = data[written:]
		}
		switch {
		case writeErr == nil:
			if written == 0 {
				return io.ErrShortWrite
			}
		case errors.Is(writeErr, unix.EINTR):
			continue
		case errors.Is(writeErr, unix.EAGAIN):
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return errors.New("terminal input FIFO remained busy")
			}
			wait := min(remaining, 50*time.Millisecond)
			fd := file.Fd()
			if fd > uintptr(1<<31-1) {
				return errors.New("terminal input FIFO descriptor is out of range")
			}
			ready, pollErr := unix.Poll(
				[]unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}},
				int(wait.Milliseconds()),
			)
			if pollErr != nil && !errors.Is(pollErr, unix.EINTR) {
				return pollErr
			}
			if ready == 0 {
				continue
			}
		default:
			return writeErr
		}
	}
	return nil
}

func removePlatformInput(path string) error {
	err := unix.Unlink(path)
	if err == unix.ENOENT {
		return nil
	}
	return err
}

func isPlatformEnd(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO)
}
