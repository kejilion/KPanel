package filemanager

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

// publishConflictTree consumes objects from an already validated private source
// (archive workspace or trash). All destination writes still use the real
// Manager's protection checks and pinned publication primitives.
func (m *Manager) publishConflictTree(ctx context.Context, source *fileRoot, sourceName, directory, name, display string, validate func() error, depth int) (contract.FileEntry, error) {
	if depth > 32 {
		return contract.FileEntry{}, ErrTooLarge
	}
	info, err := source.Lstat(sourceName)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return contract.FileEntry{}, ErrNotRegular
	}
	incoming := m.entry(display, info)
	incoming.Name = name
	changed := false
	checkedBudget := false
	for attempt := 0; attempt < 8; attempt++ {
		choice, err := m.resolvePublication(ctx, incoming, directory, changed)
		if err != nil {
			return contract.FileEntry{}, err
		}
		if choice.Skipped {
			return contract.FileEntry{}, ErrFileSkipped
		}
		if depth == 0 && !checkedBudget {
			if err := m.checkPolicyTreeBudget(ctx, source, sourceName, info); err != nil {
				return contract.FileEntry{}, err
			}
			checkedBudget = true
		}
		if err := validate(); err != nil {
			return contract.FileEntry{}, err
		}
		current, err := source.Lstat(sourceName)
		if err != nil || !os.SameFile(info, current) || resourceVersion(display, current) != incoming.ResourceVersion {
			return contract.FileEntry{}, ErrConflict
		}
		target := joinVirtual(directory, choice.Name)
		if err := m.mutationError(target); err != nil {
			return contract.FileEntry{}, err
		}
		parent, parentInfo, err := m.openReceiveDirectory(directory, false)
		if err != nil {
			return contract.FileEntry{}, err
		}
		if choice.Merge {
			targetInfo, err := parent.Lstat(choice.Name)
			parent.Close()
			if err != nil || !targetInfo.IsDir() || resourceVersion(target, targetInfo) != choice.Version {
				changed = true
				continue
			}
			root, err := source.OpenRoot(sourceName)
			if err != nil {
				return contract.FileEntry{}, err
			}
			file, err := root.Open(".")
			if err != nil {
				root.Close()
				return contract.FileEntry{}, err
			}
			children, readErr := file.ReadDir(m.maxCopyEntries + 1)
			file.Close()
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				root.Close()
				return contract.FileEntry{}, readErr
			}
			if len(children) > m.maxCopyEntries {
				root.Close()
				return contract.FileEntry{}, ErrTooLarge
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
			complete := true
			for _, child := range children {
				if err := m.verifyDirectoryIdentity(target, targetInfo); err != nil {
					root.Close()
					return contract.FileEntry{}, err
				}
				entry, err := m.publishConflictTree(ctx, root, child.Name(), target, child.Name(), joinVirtual(display, child.Name()), validate, depth+1)
				if errors.Is(err, ErrFileSkipped) {
					complete = false
					continue
				}
				if err != nil {
					root.Close()
					return contract.FileEntry{}, err
				}
				_ = entry
				if session := fileConflicts(ctx); session != nil && session.Result != nil {
					session.Result.CompletedCount++
				}
			}
			root.Close()
			if !complete {
				return contract.FileEntry{}, ErrFileSkipped
			}
			if err := removeCheckedEmptyDirectory(source, sourceName, info); err != nil {
				return contract.FileEntry{}, err
			}
			return m.Stat(target)
		}
		replace, err := checkTransferTarget(parent, choice.Name, target, info, transferConflict{overwrite: choice.Overwrite, version: choice.Version})
		if err != nil {
			parent.Close()
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrAlreadyExists) {
				changed = choice.Overwrite
				continue
			}
			return contract.FileEntry{}, err
		}
		if replace {
			entry, err := m.publishPrivateOverwrite(ctx, source, sourceName, info, directory, parent, parentInfo, target, display, choice, validate)
			parent.Close()
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrAlreadyExists) {
				changed = true
				continue
			}
			return entry, err
		}
		if err := validate(); err != nil {
			parent.Close()
			return contract.FileEntry{}, err
		}
		if err := m.verifyDirectoryIdentity(directory, parentInfo); err != nil {
			parent.Close()
			return contract.FileEntry{}, err
		}
		if _, err := checkTransferTarget(parent, choice.Name, target, info, transferConflict{overwrite: choice.Overwrite, version: choice.Version}); err != nil {
			parent.Close()
			changed = choice.Overwrite
			continue
		}
		err = publishReceiveObject(source, sourceName, parent, choice.Name, replace)
		if isCrossDeviceError(err) {
			// The source is private and cannot be namespace-swapped by an
			// unprivileged user. Copy to the existing same-filesystem staging
			// mechanism; remove the source only after verified publication.
			stage, stageErr := m.openCopyStage(directory, parentInfo)
			if stageErr == nil {
				budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
				stageErr = m.copyTreeWithAccessRoots(ctx, source, sourceName, stage.root, "payload", budget, true)
				if stageErr == nil {
					stageErr = validate()
				}
				if stageErr == nil {
					_, stageErr = checkTransferTarget(parent, choice.Name, target, info, transferConflict{overwrite: choice.Overwrite, version: choice.Version})
				}
				if stageErr == nil {
					stageErr = publishReceiveObject(stage.root, "payload", parent, choice.Name, replace)
				}
				if stageErr == nil {
					current, statErr := source.Lstat(sourceName)
					if statErr != nil || !os.SameFile(info, current) || resourceVersion(display, current) != incoming.ResourceVersion {
						stageErr = ErrConflict
					} else {
						stageErr = source.RemoveAll(sourceName)
					}
				}
				stage.close()
			}
			err = stageErr
		}
		if err == nil {
			err = syncRootDirectory(parent, ".")
		}
		if err == nil {
			err = m.verifyDirectoryIdentity(directory, parentInfo)
		}
		if err != nil {
			parent.Close()
			if errors.Is(err, os.ErrExist) {
				changed = choice.Overwrite
				continue
			}
			return contract.FileEntry{}, err
		}
		published, err := parent.Lstat(choice.Name)
		parent.Close()
		if err != nil {
			return contract.FileEntry{}, err
		}
		return m.entry(target, published), nil
	}
	return contract.FileEntry{}, ErrConflict
}
