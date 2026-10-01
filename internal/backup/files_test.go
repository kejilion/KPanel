package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fnOS guides link /home/docker to a storage volume; the Agent state root lives
// below that link on a fresh install.
func TestCanonicalRootResolvesOnlyTrustedAncestorLinks(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	volume := filepath.Join(base, "vol1", "1000", "docker")
	if err := os.MkdirAll(filepath.Join(volume, "kpanel", "data"), 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "docker")
	if err := os.Symlink(volume, linked); err != nil {
		t.Skip("symbolic links unavailable:", err)
	}
	configured := filepath.Join(linked, "kpanel", "data", "agent")
	if err := PrivateDir(filepath.Join(configured, "backup-center")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unresolved linked ancestor = %v", err)
	}
	root, err := CanonicalRoot(configured)
	if err != nil || root != filepath.Join(volume, "kpanel", "data", "agent") {
		t.Fatal(root, err)
	}
	if err := PrivateDir(filepath.Join(root, "backup-center")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := PrivateDir(filepath.Join(root, "escape", "backup-center")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("link below the resolved root = %v", err)
	}
	if _, err := CanonicalRoot(filepath.Join("relative", "state")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative root = %v", err)
	}
}
