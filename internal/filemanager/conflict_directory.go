package filemanager

import (
	"context"
	"errors"
	"os"
)

// Unlike a resource version, this identity remains stable while this task
// creates children. A decision must never migrate to a replacement directory.
var ErrDirectoryConflict = errors.New("目标文件夹已被替换，请刷新后重新发起操作")

type conflictDirectoryKey struct{}
type conflictDirectory struct{ path, identity string }

func withConflictDirectory(ctx context.Context, directory, identity string) context.Context {
	return context.WithValue(ctx, conflictDirectoryKey{}, conflictDirectory{directory, identity})
}

func checkConflictDirectory(ctx context.Context, directory string, root *fileRoot, info os.FileInfo) error {
	if expected, ok := ctx.Value(conflictDirectoryKey{}).(conflictDirectory); ok && expected.path == directory {
		return checkDirectoryIdentity(root, info, expected.identity)
	}
	return nil
}

func checkDirectoryIdentity(root *fileRoot, info os.FileInfo, expected string) error {
	if len(expected) > 256 || expected != "" && stableRootReceiveIdentity(root, ".", info) != expected {
		return ErrDirectoryConflict
	}
	return nil
}

func (m *Manager) bindConflictDirectory(ctx context.Context, directory string) (context.Context, error) {
	root, info, err := m.openReceiveDirectory(directory, false)
	if err != nil {
		return ctx, err
	}
	defer root.Close()
	if err := checkConflictDirectory(ctx, directory, root, info); err != nil {
		return ctx, err
	}
	identity := stableRootReceiveIdentity(root, ".", info)
	if identity == "" {
		return ctx, ErrDirectoryConflict
	}
	return withConflictDirectory(ctx, directory, identity), nil
}
