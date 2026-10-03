package hostpty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ErrNotWritten marks a job-terminal input failure that happened before any
// byte entered the input FIFO, so the same bytes may safely be offered again.
// ErrPartialWrite marks one after some bytes were accepted: the prefix may
// already have reached the job and the input must never be replayed.
var (
	ErrNotWritten   = errors.New("terminal input was not written")
	ErrPartialWrite = errors.New("terminal input may be partially written")
)

func notWritten(err error) error {
	return fmt.Errorf("%w: %w", ErrNotWritten, err)
}

func partialWrite(err error) error {
	return fmt.Errorf("%w: %w", ErrPartialWrite, err)
}

type Process interface {
	io.ReadWriteCloser
	Wait() error
	Kill() error
	Resize(rows, columns uint16) error
}

func Start(command *exec.Cmd, rows, columns uint16) (Process, error) {
	return startPlatform(command, rows, columns)
}

func CreateInput(path string) error {
	return createPlatformInput(path)
}

func OpenInput(path string) (*os.File, error) {
	return openPlatformInput(path)
}

func WriteInput(path string, data []byte) error {
	return writePlatformInput(path, data)
}

func RemoveInput(path string) error {
	return removePlatformInput(path)
}

func IsEnd(err error) bool {
	return isPlatformEnd(err)
}
