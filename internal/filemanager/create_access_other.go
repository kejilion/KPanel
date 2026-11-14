//go:build !windows

package filemanager

import (
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
	"path"
	"syscall"
)

// User-created directories must be traversable by web servers even when the
// Agent runs with umask 0077. Keep the parent and chmod target pinned.
func (m *Manager) mkdirUserDirectory(virtual string, expected ...string) (contract.FileEntry, error) {
	parentVirtual := path.Dir(virtual)
	parent, info, err := m.openReceiveDirectory(parentVirtual, false)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer parent.Close()
	if len(expected) > 0 {
		if err := checkDirectoryIdentity(parent, info, expected[0]); err != nil {
			return contract.FileEntry{}, err
		}
	}
	base := path.Base(virtual)
	// mkdir cannot return an inode handle. Never reopen a new name in a
	// directory where a local writer can replace it before chmod.
	creation, name := parent, base
	if !archiveWorkspaceProtected(info, parent) {
		workspace, workspacePath, workspaceInfo, err := m.openArchiveWorkspace(parentVirtual, info)
		if err != nil {
			return contract.FileEntry{}, err
		}
		stage := &fileCopyStage{workspace: workspace, workspacePath: workspacePath, workspaceInfo: workspaceInfo, name: ".kpanel-copy-" + randomID()}
		defer stage.close()
		if err := workspace.Mkdir(stage.name, 0700); err != nil {
			return contract.FileEntry{}, err
		}
		stage.info, err = workspace.Lstat(stage.name)
		if err != nil {
			return contract.FileEntry{}, err
		}
		stage.root, err = workspace.OpenRoot(stage.name)
		if err != nil {
			return contract.FileEntry{}, err
		}
		if err := stage.validate(m); err != nil {
			return contract.FileEntry{}, err
		}
		if err := inheritArchiveDestinationDefaults(stage.root, parent); err != nil {
			return contract.FileEntry{}, err
		}
		creation, name = stage.root, "directory"
	}
	if err := creation.Mkdir(name, 0700); err != nil {
		return contract.FileEntry{}, err
	}
	directory, err := creation.Open(name)
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer directory.Close()
	created, err := directory.Stat()
	if err != nil {
		return contract.FileEntry{}, err
	}
	if !created.IsDir() {
		return contract.FileEntry{}, ErrConflict
	}
	mode := os.FileMode(0755)
	if info.Mode()&os.ModeSetgid != 0 {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return contract.FileEntry{}, ErrConflict
		}
		if err := directory.Chown(-1, int(stat.Gid)); err != nil {
			return contract.FileEntry{}, err
		}
		mode |= os.ModeSetgid
	} else if creation != parent {
		if err := directory.Chown(-1, os.Getegid()); err != nil {
			return contract.FileEntry{}, err
		}
	}
	if err := directory.Chmod(mode); err != nil {
		return contract.FileEntry{}, err
	}
	actual, err := directory.Stat()
	if err != nil {
		return contract.FileEntry{}, err
	}
	if creation != parent {
		if err := m.verifyDirectoryIdentity(parentVirtual, info); err != nil {
			return contract.FileEntry{}, err
		}
		if err := publishReceiveObject(creation, name, parent, base, false); err != nil {
			return contract.FileEntry{}, err
		}
	}
	visible, err := m.rootFS.Lstat(rootName(virtual))
	if err != nil || !os.SameFile(actual, visible) || actual.Mode().Perm() != 0755 {
		return contract.FileEntry{}, ErrConflict
	}
	entry := m.entry(virtual, actual)
	entry.DirectoryIdentity = stableReceiveIdentity(actual, directory)
	if entry.DirectoryIdentity == "" {
		return contract.FileEntry{}, ErrDirectoryConflict
	}
	return entry, nil
}

func openAccessTarget(root *fileRoot, name string) (*os.File, error) { return root.Open(name) }

// Source-preserving operations start private; final metadata is applied only
// after copying content. Ordinary copies inherit the destination policy.
func createFileWithSourceAccess(root *fileRoot, name string, flags int, mode os.FileMode, source *os.File) (*os.File, error) {
	if source != nil {
		mode = 0600
	}
	return root.OpenFile(name, flags, mode)
}

func mkdirWithSourceAccess(root *fileRoot, name string, mode os.FileMode, source *os.File) error {
	if source != nil {
		mode = 0700
	}
	return root.Mkdir(name, mode)
}
