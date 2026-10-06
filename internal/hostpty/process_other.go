//go:build !linux

package hostpty

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

func startPlatform(*exec.Cmd, uint16, uint16) (Process, error) {
	return nil, errors.New("interactive host terminals require Linux")
}

func createPlatformInput(string) error {
	return nil
}

func openPlatformInput(string) (*os.File, error) {
	return nil, errors.New("interactive host terminals require Linux")
}

func writePlatformInput(string, []byte) error {
	return notWritten(errors.New("interactive host terminals require Linux"))
}

func removePlatformInput(string) error {
	return nil
}

func isPlatformEnd(err error) bool {
	return errors.Is(err, io.EOF)
}
