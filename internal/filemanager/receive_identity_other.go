//go:build !linux

package filemanager

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

func stableReceiveIdentity(os.FileInfo) string { return "" }

func receiveFileTimes(root *fileRoot, name string, _ *os.File, when time.Time) error {
	return root.Chtimes(name, when, when)
}

// Non-Linux builds support local tooling. The supported Linux Agent uses
// descriptor-based link/rename; this portable adapter copies into private roots.
func stageReceiveFile(source *os.File, stage *fileRoot, name string) (os.FileInfo, error) {
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	file, err := stage.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.Copy(file, source); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := stage.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
		return nil, err
	}
	return file.Stat()
}

func publishReceiveObject(source *fileRoot, sourceName string, target *fileRoot, targetName string, replace bool) error {
	if !replace {
		if _, err := target.Lstat(targetName); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(filepath.Join(source.Name(), sourceName), filepath.Join(target.Name(), targetName))
}
