package filemanager

import (
	"context"
	"os"
)

// A separate preflight counter bounds the entire selected tree, including
// same-filesystem moves. Actual copying retains its existing streaming budget
// so a source that grows after preflight cannot bypass the byte limit.
func (m *Manager) checkPolicyTreeBudget(ctx context.Context, root *fileRoot, name string, info os.FileInfo) error {
	session := fileConflicts(ctx)
	if session == nil {
		return ErrAction
	}
	if session.budget == nil {
		session.budget = &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	}
	ctx = context.WithValue(ctx, archiveContextKey{}, (*archiveOperation)(nil))
	reader := m
	if root != m.rootFS {
		reader = &Manager{rootFS: root}
	}
	return reader.walkArchive(ctx, "/"+rootName(name), "", info, session.budget, func(context.Context, string, string, os.FileInfo) error { return nil })
}
