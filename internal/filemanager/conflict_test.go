package filemanager

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestActionConflictPolicy(t *testing.T) {
	for _, action := range []string{"copy", "move"} {
		t.Run(action, func(t *testing.T) {
			m, root := newTestManager(t)
			mustMkdirAll(t, filepath.Join(root, "from"))
			mustMkdirAll(t, filepath.Join(root, "to"))
			mustWrite(t, filepath.Join(root, "from", "a.txt"), "incoming")
			mustWrite(t, filepath.Join(root, "to", "a.txt"), "existing")
			old, _ := m.Stat("/to/a.txt")
			run := func(name, version string, overwrite bool) contract.FileActionResult {
				t.Helper()
				result, err := m.Action(context.Background(), contract.FileActionRequest{Action: action, Sources: []string{"/from/a.txt"}, Target: "/to", DestinationName: name, Overwrite: overwrite, ExpectedTargetResourceVersion: version})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			if r := run("", "", false); len(r.Failed) != 1 || r.Failed[0].Code != "already_exists" {
				t.Fatalf("unresolved conflict: %+v", r)
			}
			if r := run("", "stale", true); len(r.Failed) != 1 || r.Failed[0].Code != "resource_conflict" {
				t.Fatalf("stale consent: %+v", r)
			}
			body, _ := os.ReadFile(filepath.Join(root, "to", "a.txt"))
			if string(body) != "existing" {
				t.Fatal("stale overwrite changed destination")
			}
			if r := run("a (1).txt", "", false); len(r.Succeeded) != 1 {
				t.Fatalf("keep both: %+v", r)
			}
			body, _ = os.ReadFile(filepath.Join(root, "to", "a.txt"))
			if string(body) != "existing" {
				t.Fatal("keep both changed original")
			}
			if action == "move" {
				mustWrite(t, filepath.Join(root, "from", "a.txt"), "replacement")
			}
			if r := run("", old.ResourceVersion, true); len(r.Succeeded) != 1 {
				t.Fatalf("versioned overwrite: %+v", r)
			}
			body, _ = os.ReadFile(filepath.Join(root, "to", "a.txt"))
			want := "incoming"
			if action == "move" {
				want = "replacement"
			}
			if string(body) != want {
				t.Fatalf("got %q", body)
			}
		})
	}
}

func TestActionConflictRejectsUnsafeChoices(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "a"))
	mustWrite(t, filepath.Join(root, "a", "x"), "x")
	for _, input := range []contract.FileActionRequest{
		{Action: "copy", Sources: []string{"/a/x"}, Target: "/a", DestinationName: "../escape"},
		{Action: "copy", Sources: []string{"/a/x"}, Target: "/a", Overwrite: true},
		{Action: "move", Sources: []string{"/a/x"}, Target: "/a", ExpectedTargetResourceVersion: "fake"},
		{Action: "trash", Sources: []string{"/a/x"}, Overwrite: true, ExpectedTargetResourceVersion: "fake"},
	} {
		if _, err := m.Action(context.Background(), input); !errors.Is(err, ErrAction) {
			t.Fatalf("accepted %+v: %v", input, err)
		}
	}
	entry, _ := m.Stat("/a/x")
	r, err := m.Action(context.Background(), contract.FileActionRequest{Action: "copy", Sources: []string{"/a/x"}, Target: "/a", Overwrite: true, ExpectedTargetResourceVersion: entry.ResourceVersion})
	if err != nil || len(r.Failed) != 1 {
		t.Fatalf("same-source overwrite: %+v %v", r, err)
	}
}

func TestUploadConflictVersionAndEmptyDirectoryCleanup(t *testing.T) {
	m, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "x"), "old")
	old, _ := m.Stat("/x")
	if _, err := m.UploadExpected(context.Background(), "/", "x", bytes.NewBufferString("new"), 3, true, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale upload: %v", err)
	}
	if _, err := m.UploadExpected(context.Background(), "/", "x", bytes.NewBufferString("new"), 3, true, old.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, filepath.Join(root, "dir"))
	d, _ := m.Stat("/dir")
	mustWrite(t, filepath.Join(root, "dir", "new-child"), "keep")
	if _, err := m.Action(context.Background(), contract.FileActionRequest{Action: "rmdir", Sources: []string{"/dir"}, ExpectedResourceVersion: d.ResourceVersion}); err == nil {
		t.Fatal("removed a changed/nonempty directory")
	}
	if _, err := os.Stat(filepath.Join(root, "dir", "new-child")); err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, filepath.Join(root, "empty"))
	empty, _ := m.Stat("/empty")
	if _, err := m.Action(context.Background(), contract.FileActionRequest{Action: "rmdir", Sources: []string{"/empty"}, ExpectedResourceVersion: empty.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
}
