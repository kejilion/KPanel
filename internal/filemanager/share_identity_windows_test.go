//go:build windows

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsShareVersionDetectsSameMetadataReplacement(t *testing.T) {
	m, root := newTestManager(t)
	target := filepath.Join(root, "shared.txt")
	mustWrite(t, target, "same-content")
	before, err := m.ShareEntry(context.Background(), "/shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.txt")
	mustWrite(t, replacement, "same-content")
	if err := os.Chtimes(replacement, before.ModifiedAt, before.ModifiedAt); err != nil {
		t.Fatal(err)
	}
	if err := m.rootFS.Rename("replacement.txt", "shared.txt"); err != nil {
		t.Fatal(err)
	}
	after, err := m.ShareEntry(context.Background(), "/shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	if before.ResourceVersion != after.ResourceVersion {
		t.Fatal("replacement fixture changed ordinary metadata")
	}
	if before.ShareVersion == after.ShareVersion {
		t.Fatal("same-content replacement retained public share identity")
	}
}
