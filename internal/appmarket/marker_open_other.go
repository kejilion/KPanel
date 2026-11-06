//go:build !linux

package appmarket

import (
	"errors"
	"os"
)

// Non-Linux builds do not run the production Agent. Keep development builds
// source-compatible and reject changed or non-regular files after opening.
func openMarkerFile(root *os.Root) (*os.File, error) {
	before, err := root.Lstat("appno.txt")
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > maxAppMarkersBytes {
		return nil, errors.New("unsafe application marker")
	}
	file, err := root.Open("appno.txt")
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
