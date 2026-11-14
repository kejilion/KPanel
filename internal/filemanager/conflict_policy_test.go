package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func waitConflictJob(t *testing.T, m *Manager, id, state string) contract.FileArchiveJob {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		jobs, err := m.ArchiveJobs()
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range jobs {
			if job.ID == id {
				if job.State == state {
					return job
				}
				if !archiveJobActive(job.State) {
					t.Fatalf("expected %s: %+v", state, job)
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task did not reach " + state)
	return contract.FileArchiveJob{}
}

func TestConflictTaskMergeChoicesAndSourceRetention(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "from", "folder"))
	mustMkdirAll(t, filepath.Join(root, "to", "folder"))
	for name, body := range map[string]string{"from/folder/a.txt": "incoming", "from/folder/b.txt": "new", "to/folder/a.txt": "original", "to/folder/only.txt": "retain"} {
		mustWrite(t, filepath.Join(root, filepath.FromSlash(name)), body)
	}
	source, _ := m.Stat("/from/folder")
	job, err := m.StartArchiveJob(contract.FileActionRequest{Action: "move", Sources: []string{source.Path}, Target: "/to", ExpectedResourceVersions: map[string]string{source.Path: source.ResourceVersion}, ConflictPolicies: &contract.FileConflictPolicies{}})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitConflictJob(t, m, job.ID, "waiting_input")
	if waiting.Conflict.Source.Kind != "directory" {
		t.Fatal(waiting)
	}
	if err := m.DecideArchiveJob(job.ID, contract.FileConflictDecision{Revision: waiting.Revision, Policy: "merge", ApplyToRest: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.DecideArchiveJob(job.ID, contract.FileConflictDecision{Revision: waiting.Revision, Policy: "skip"}); err != ErrConflict {
		t.Fatalf("stale decision accepted: %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		waiting = waitConflictJob(t, m, job.ID, "waiting_input")
		if waiting.Conflict.Source.Kind == "file" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if waiting.Conflict.Source.Kind != "file" {
		t.Fatal("folder choice leaked to files")
	}
	if err := m.DecideArchiveJob(job.ID, contract.FileConflictDecision{Revision: waiting.Revision, Policy: "skip", ApplyToRest: true}); err != nil {
		t.Fatal(err)
	}
	finished := waitConflictJob(t, m, job.ID, "partial")
	if len(finished.Result.Skipped) != 1 {
		t.Fatal(finished)
	}
	for name, want := range map[string]string{"from/folder/a.txt": "incoming", "to/folder/a.txt": "original", "to/folder/b.txt": "new", "to/folder/only.txt": "retain"} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(body) != want {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "from", "folder", "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("moved source retained: %v", err)
	}
}

func TestConflictPolicyNamesAndNoSilentTypeReplacement(t *testing.T) {
	for input, want := range map[string]string{"backup.tar.gz": "backup (1).tar.gz", ".env": ".env (1)", "a.txt": "a (1).txt"} {
		actual, err := CopyName(input, "file", 1)
		if err != nil || actual != want {
			t.Fatalf("%s => %q %v", input, actual, err)
		}
	}
	if name, _ := CopyName("folder.v1", "directory", 2); name != "folder.v1 (2)" {
		t.Fatal(name)
	}
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "from"))
	mustMkdirAll(t, filepath.Join(root, "to", "a"))
	mustWrite(t, filepath.Join(root, "from", "a"), "new")
	result, err := m.Action(context.Background(), contract.FileActionRequest{Action: "copy", Sources: []string{"/from/a"}, Target: "/to", ConflictPolicies: &contract.FileConflictPolicies{File: "overwrite"}})
	if err != nil || len(result.Failed) != 1 {
		t.Fatalf("cross type overwrite: %+v %v", result, err)
	}
	if info, err := os.Stat(filepath.Join(root, "to", "a")); err != nil || !info.IsDir() {
		t.Fatal("target directory replaced")
	}
}

func TestConflictArchiveMergeAndRestoreKeepBoth(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "source"))
	mustMkdirAll(t, filepath.Join(root, "target", "restore"))
	mustWrite(t, filepath.Join(root, "source", "a.txt"), "new")
	mustWrite(t, filepath.Join(root, "target", "restore", "a.txt"), "old")
	mustWrite(t, filepath.Join(root, "target", "restore", "only.txt"), "retain")
	_, err := m.Action(context.Background(), contract.FileActionRequest{Action: "compress", Sources: []string{"/source"}, Target: "/", Name: "input.zip", Format: "zip"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Action(context.Background(), contract.FileActionRequest{Action: "extract", Sources: []string{"/input.zip"}, Target: "/target", Name: "restore", Format: "zip", ConflictPolicies: &contract.FileConflictPolicies{File: "overwrite", Directory: "merge"}})
	if err != nil || len(result.Failed) > 0 {
		t.Fatalf("merge extraction %+v %v", result, err)
	}
	for name, want := range map[string]string{"a.txt": "new", "only.txt": "retain"} {
		body, err := os.ReadFile(filepath.Join(root, "target", "restore", name))
		if err != nil || string(body) != want {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
	trash, err := m.Action(context.Background(), contract.FileActionRequest{Action: "trash", Sources: []string{"/source/a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(trash.Succeeded[0].Destination)
	mustWrite(t, filepath.Join(root, "source", "a.txt"), "current")
	result, err = m.Action(context.Background(), contract.FileActionRequest{Action: "trash_restore", TrashIDs: []string{id}, ConflictPolicies: &contract.FileConflictPolicies{File: "keep-both"}})
	if err != nil || len(result.Failed) > 0 {
		t.Fatalf("restore %+v %v", result, err)
	}
	body, err := os.ReadFile(filepath.Join(root, "source", "a (1).txt"))
	if err != nil || string(body) != "new" {
		t.Fatalf("restore copy=%q %v", body, err)
	}
}
