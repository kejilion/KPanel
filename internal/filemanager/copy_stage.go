package filemanager

import (
	"errors"
	"os"
)

// fileCopyStage keeps copied objects inside a private directory on the target
// filesystem until publication. Writable destination directories must never
// name a service-created object before it is published.
type fileCopyStage struct {
	workspace     *fileRoot
	workspacePath string
	workspaceInfo os.FileInfo
	root          *fileRoot
	name          string
	info          os.FileInfo
}

func (m *Manager) openCopyStage(targetDirectory string, targetInfo os.FileInfo) (*fileCopyStage, error) {
	workspace, workspacePath, workspaceInfo, err := m.openArchiveWorkspace(targetDirectory, targetInfo)
	if err != nil {
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		// Ordinary copies do not remove their source. When the managed root has
		// no protected ancestor, keep the same-filesystem private stage under the
		// held destination directory; destructive cross-filesystem callers have
		// already required protected source and destination parents.
		workspace, workspaceInfo, err = m.openReceiveDirectory(targetDirectory, false)
		if err != nil || !os.SameFile(targetInfo, workspaceInfo) || !archiveSameFilesystem(targetInfo, workspaceInfo) {
			if workspace != nil {
				_ = workspace.Close()
			}
			return nil, ErrConflict
		}
		workspacePath = targetDirectory
	}
	name := ".kpanel-copy-" + randomID()
	if err := workspace.Mkdir(name, 0700); err != nil {
		_ = workspace.Close()
		return nil, err
	}
	stageInfo, err := workspace.Lstat(name)
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			_ = removeOwnedReceiveDirectory(workspace, name, stageInfo, stageInfo, "")
		}
		_ = workspace.Close()
		return nil, ErrConflict
	}
	stage, err := workspace.OpenRoot(name)
	if err != nil {
		_ = removeOwnedReceiveDirectory(workspace, name, stageInfo, stageInfo, "")
		_ = workspace.Close()
		return nil, err
	}
	opened, err := stage.Stat(".")
	if err != nil || !os.SameFile(stageInfo, opened) || !archiveStagePrivate(opened, stage) {
		_ = stage.Close()
		_ = removeOwnedReceiveDirectory(workspace, name, stageInfo, stageInfo, "")
		_ = workspace.Close()
		return nil, ErrConflict
	}
	return &fileCopyStage{
		workspace: workspace, workspacePath: workspacePath, workspaceInfo: workspaceInfo,
		root: stage, name: name, info: opened,
	}, nil
}

func (m *Manager) verifyDirectoryIdentity(virtual string, expected os.FileInfo) error {
	if expected == nil {
		return ErrConflict
	}
	opened, info, err := m.openReceiveDirectory(virtual, false)
	if err != nil {
		return ErrConflict
	}
	defer opened.Close()
	if !os.SameFile(expected, info) {
		return ErrConflict
	}
	return nil
}

func (s *fileCopyStage) validate(m *Manager) error {
	workspace, info, err := m.openReceiveDirectory(s.workspacePath, false)
	if err != nil {
		return ErrConflict
	}
	_ = workspace.Close()
	if !os.SameFile(s.workspaceInfo, info) {
		return ErrConflict
	}
	visible, err := s.workspace.Lstat(s.name)
	if err != nil || !os.SameFile(s.info, visible) {
		return ErrConflict
	}
	opened, err := s.root.Stat(".")
	if err != nil || !os.SameFile(s.info, opened) || !archiveStagePrivate(opened, s.root) {
		return ErrConflict
	}
	return nil
}

func (s *fileCopyStage) close() {
	if s.root != nil {
		_ = s.root.Close()
	}
	if s.workspace != nil {
		if visible, err := s.workspace.Lstat(s.name); err == nil && s.info != nil {
			_ = removeOwnedReceiveDirectory(s.workspace, s.name, visible, s.info, "")
		}
		_ = s.workspace.Close()
	}
}
