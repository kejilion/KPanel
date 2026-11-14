package filemanager

import (
	"context"
	"errors"
	"io"
	"os"
	"path"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type transferConflict struct {
	name      string
	overwrite bool
	version   string
}

func checkTransferTarget(parent *fileRoot, name, virtual string, source os.FileInfo, choice transferConflict) (bool, error) {
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		if choice.version != "" {
			return false, ErrConflict
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !choice.overwrite {
		return false, ErrAlreadyExists
	}
	if choice.version == "" || !source.Mode().IsRegular() || !info.Mode().IsRegular() || os.SameFile(source, info) || resourceVersion(virtual, info) != choice.version {
		return false, ErrConflict
	}
	return true, nil
}

// Only remove an empty directory, through pinned parents. A concurrent new
// child makes Remove fail; recursive deletion is deliberately not used here.
func (m *Manager) removeEmptyDirectory(virtual, expected string) (contract.FileEntry, error) {
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if normalized == "/" {
		return contract.FileEntry{}, ErrRootOperation
	}
	if err := m.mutationError(normalized); err != nil {
		return contract.FileEntry{}, err
	}
	if expected == "" {
		return contract.FileEntry{}, ErrConflict
	}
	parent, parentInfo, err := m.openReceiveDirectory(path.Dir(normalized), false)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer parent.Close()
	info, err := parent.Lstat(path.Base(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || resourceVersion(normalized, info) != expected {
		return contract.FileEntry{}, ErrConflict
	}
	if err := m.verifyDirectoryIdentity(path.Dir(normalized), parentInfo); err != nil {
		return contract.FileEntry{}, err
	}
	if err := removeCheckedEmptyDirectory(parent, path.Base(normalized), info); err != nil {
		return contract.FileEntry{}, err
	}
	return m.entry(normalized, info), syncRootDirectory(parent, ".")
}

// Preserve the old upload entry point for legacy callers. New clients bind
// overwrite consent to the exact destination version shown in their dialog.
func (m *Manager) UploadExpected(ctx context.Context, directory, name string, content io.Reader, length int64, overwrite bool, expected string, directoryIdentity ...string) (contract.FileEntry, error) {
	if length > MaxUploadBytes {
		return contract.FileEntry{}, ErrTooLarge
	}
	identity := ""
	if len(directoryIdentity) > 0 {
		identity = directoryIdentity[0]
	}
	return m.ReceiveStream(ctx, contract.FileReceiveInput{ExpectedDirectoryIdentity: identity, Directory: directory, Name: name, Kind: "file", SizeBytes: length, Overwrite: overwrite, ExpectedTargetResourceVersion: expected}, content, MaxUploadBytes)
}
