// Package atomicfile replaces private state files without conflating a failed
// commit with diagnostics from work performed after the replacement.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

// Result describes the visible replacement. An error returned by WritePrivate
// means the new file was not committed. Once Committed is true, callers must
// adopt the new state even if directory sync or backup cleanup failed.
type Result struct {
	Committed     bool
	DurabilityErr error
	CleanupErr    error
}

// Writer's zero value uses the operating system. Sync dependencies are per
// writer so failure tests do not replace process-wide filesystem functions.
type Writer struct {
	SyncFile      func(*os.File) error
	SyncDirectory func(string) error
}

// WritePrivate writes a 0600 file in directory, syncs and closes it, then
// replaces target. pattern controls only the existing temporary-file naming
// convention. Callers own directory validation, locking and transaction scope.
func (w Writer) WritePrivate(directory, target, pattern string, data []byte) (Result, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return Result{}, err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return Result{}, err
	}
	n, err := file.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = file.Close()
		return Result{}, err
	}
	syncFile := w.SyncFile
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	if err := syncFile(file); err != nil {
		_ = file.Close()
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}
	result, err := replace(temporary, target, runtime.GOOS == "windows", os.Rename, os.Remove)
	if err != nil {
		return result, err
	}
	syncDirectory := w.SyncDirectory
	if syncDirectory == nil {
		syncDirectory = syncParentDirectory
	}
	result.DurabilityErr = syncDirectory(directory)
	return result, nil
}

// The fallback is only for Windows rename-over-target failures. Other platforms
// leave the target untouched when their atomic rename fails. Keep the old file
// until the replacement succeeds, and report cleanup separately after commit.
func replace(source, target string, windows bool, rename func(string, string) error, remove func(string) error) (Result, error) {
	initialErr := rename(source, target)
	if initialErr == nil {
		return Result{Committed: true}, nil
	}
	if !windows {
		return Result{}, initialErr
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return Result{}, initialErr
	}
	backup := target + ".previous"
	if err := remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("remove previous state backup: %w", err)
	}
	if err := rename(target, backup); err != nil {
		return Result{}, err
	}
	if err := rename(source, target); err != nil {
		if restoreErr := rename(backup, target); restoreErr != nil {
			return Result{}, errors.Join(err, fmt.Errorf("restore previous state from %s: %w", backup, restoreErr))
		}
		return Result{}, err
	}
	return Result{Committed: true, CleanupErr: remove(backup)}, nil
}

func syncParentDirectory(path string) error {
	if runtime.GOOS == "windows" {
		// Preserve the existing Windows development behavior; nil means the
		// platform-supported sync steps succeeded, not a power-loss guarantee.
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
