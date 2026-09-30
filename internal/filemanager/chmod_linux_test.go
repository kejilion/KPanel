//go:build linux

package filemanager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestChmodNoFollowRejectsSymlinksSwappedInAfterValidation(t *testing.T) {
	manager, root := newTestManager(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "web")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "upload.txt")
	if err := os.WriteFile(target, []byte("site"), 0o600); err != nil {
		t.Fatal(err)
	}
	validated, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.chmodNoFollow("/web/upload.txt", 0o644, validated); err != nil {
		t.Fatalf("chmodNoFollow() error = %v", err)
	}
	if info, _ := os.Stat(target); info.Mode().Perm() != 0o644 {
		t.Fatalf("validated file mode = %v, want 0644", info.Mode().Perm())
	}

	// The final component is replaced by a link after validation.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	if err := manager.chmodNoFollow("/web/upload.txt", 0o777, validated); !errors.Is(err, ErrSymlink) {
		t.Fatalf("swapped final component error = %v, want ErrSymlink", err)
	}

	// An intermediate directory is replaced by a link to another tree that
	// holds a file of the same name.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(t.TempDir(), "decoy")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(outside, filepath.Join(decoy, "upload.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(decoy, directory); err != nil {
		t.Fatal(err)
	}
	if err := manager.chmodNoFollow("/web/upload.txt", 0o777, validated); !errors.Is(err, ErrSymlink) {
		t.Fatalf("swapped directory error = %v, want ErrSymlink", err)
	}
	if info, _ := os.Stat(filepath.Join(decoy, "upload.txt")); info.Mode().Perm() != 0o600 {
		t.Fatalf("file outside the validated path changed mode to %v", info.Mode().Perm())
	}

	// A different regular file at the validated path is a conflict.
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.chmodNoFollow("/web/upload.txt", 0o777, validated); !errors.Is(err, ErrConflict) {
		t.Fatalf("replaced file error = %v, want ErrConflict", err)
	}
}
