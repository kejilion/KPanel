package remotedownload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransferJobStoreKeepsPrivatePermissions(t *testing.T) {
	parent := t.TempDir()
	currentRoot := filepath.Join(parent, "file-transfers")
	current, err := OpenTransferJobStore(currentRoot, filepath.Join(parent, "remote-downloads"))
	if err != nil || !current.Available() {
		t.Fatalf("fresh common store: %v", err)
	}
	for filename, want := range map[string]os.FileMode{currentRoot: 0700, filepath.Join(currentRoot, "jobs-v2.json"): 0600} {
		info, err := os.Stat(filename)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("private permissions for %s: %v", filename, err)
		}
	}
}

func TestTransferJobStoreRefusesLinkedIndexes(t *testing.T) {
	for _, filename := range []string{"jobs.json", "jobs-v1.json", "jobs-v2.json"} {
		t.Run(filename, func(t *testing.T) {
			parent := t.TempDir()
			currentRoot, legacyRoot := filepath.Join(parent, "file-transfers"), filepath.Join(parent, "remote-downloads")
			if _, err := OpenJobStore(legacyRoot); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(currentRoot, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(legacyRoot, "jobs.json"), filepath.Join(currentRoot, filename)); err != nil {
				t.Fatal(err)
			}
			current, err := OpenTransferJobStore(currentRoot, legacyRoot)
			if err != nil || current.Available() {
				t.Fatalf("linked index accepted: %v", err)
			}
			info, err := os.Lstat(filepath.Join(currentRoot, filename))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("linked index replaced: %v", err)
			}
		})
	}
}
