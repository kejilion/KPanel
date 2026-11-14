package filemanager

import (
	"context"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
)

func applyConflictTargetAccess(stage *fileRoot, name string, parent *fileRoot, target, virtual, version string) error {
	old, err := parent.Open(target)
	if err != nil {
		return err
	}
	defer old.Close()
	info, err := old.Stat()
	if err != nil || !info.Mode().IsRegular() || resourceVersion(virtual, info) != version {
		return ErrConflict
	}
	file, err := stage.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := applySourceAccess(file, old, false); err != nil {
		return err
	}
	return file.Sync()
}

// Overwrite never changes the source object's metadata before publication.
// In particular a failed restore must leave its trash source intact.
func (m *Manager) publishPrivateOverwrite(ctx context.Context, source *fileRoot, sourceName string, sourceInfo os.FileInfo, directory string, parent *fileRoot, parentInfo os.FileInfo, target, display string, choice contract.FileConflictResolution, validate func() error) (contract.FileEntry, error) {
	stage, err := m.openCopyStage(directory, parentInfo)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer stage.close()
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	if err := m.copyTreeWithAccessRoots(ctx, source, sourceName, stage.root, "payload", budget, true); err != nil {
		return contract.FileEntry{}, err
	}
	if err := applyConflictTargetAccess(stage.root, "payload", parent, choice.Name, target, choice.Version); err != nil {
		return contract.FileEntry{}, err
	}
	if err := validate(); err != nil {
		return contract.FileEntry{}, err
	}
	current, err := source.Lstat(sourceName)
	if err != nil || !os.SameFile(sourceInfo, current) || resourceVersion(display, current) != resourceVersion(display, sourceInfo) {
		return contract.FileEntry{}, ErrConflict
	}
	if err := stage.validate(m); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.verifyDirectoryIdentity(directory, parentInfo); err != nil {
		return contract.FileEntry{}, err
	}
	if _, err := checkTransferTarget(parent, choice.Name, target, sourceInfo, transferConflict{overwrite: true, version: choice.Version}); err != nil {
		return contract.FileEntry{}, err
	}
	staged, err := stage.root.Lstat("payload")
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := publishReceiveObject(stage.root, "payload", parent, choice.Name, true); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(parent, "."); err != nil {
		return contract.FileEntry{}, err
	}
	published, err := parent.Lstat(choice.Name)
	if err != nil || !os.SameFile(staged, published) {
		return contract.FileEntry{}, ErrConflict
	}
	if err := validate(); err != nil {
		return contract.FileEntry{}, err
	}
	current, err = source.Lstat(sourceName)
	if err != nil || !os.SameFile(sourceInfo, current) || resourceVersion(display, current) != resourceVersion(display, sourceInfo) {
		return contract.FileEntry{}, ErrConflict
	}
	if err := source.Remove(sourceName); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(source, "."); err != nil {
		return contract.FileEntry{}, err
	}
	return m.entry(target, published), nil
}
