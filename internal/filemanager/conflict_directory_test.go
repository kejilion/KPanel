package filemanager

import (
	"context"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryImportPinsParent(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "target"))
	approved, err := m.Stat("/target")
	if err != nil || approved.DirectoryIdentity == "" {
		t.Fatalf("identity: %+v %v", approved, err)
	}
	if err := os.Rename(filepath.Join(root, "target"), filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, filepath.Join(root, "target"))
	_, err = m.BeginReceive(context.Background(), contract.FileReceiveInput{Directory: "/target", Name: "a.txt", Kind: "file", SizeBytes: 1, SourceKey: strings.Repeat("c", 64), ExpectedDirectoryIdentity: approved.DirectoryIdentity})
	if !errors.Is(err, ErrDirectoryConflict) {
		t.Fatalf("receive: %v", err)
	}
	_, err = m.Action(context.Background(), contract.FileActionRequest{Action: "mkdir", Target: "/target", Name: "nested", ExpectedDirectoryIdentity: approved.DirectoryIdentity})
	if !errors.Is(err, ErrDirectoryConflict) {
		t.Fatalf("mkdir: %v", err)
	}
	children, err := os.ReadDir(filepath.Join(root, "target"))
	if err != nil || len(children) != 0 {
		t.Fatalf("replacement modified: %v %v", children, err)
	}
}
