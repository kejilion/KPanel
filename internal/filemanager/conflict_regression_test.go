package filemanager

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIndependentMergePinsApprovedTarget(t *testing.T) {
	for _, action := range []string{"copy", "move"} {
		t.Run(action, func(t *testing.T) {
			m, root := newTestManager(t)
			mustMkdirAll(t, filepath.Join(root, "from", "folder"))
			mustMkdirAll(t, filepath.Join(root, "to", "folder"))
			mustWrite(t, filepath.Join(root, "from", "folder", "new.txt"), "incoming")
			mustWrite(t, filepath.Join(root, "to", "folder", "only.txt"), "old-target")
			src, _ := m.Stat("/from/folder")
			prompts := 0
			session := &ConflictSession{}
			session.Prompt = func(_ context.Context, c contract.FileConflict) (contract.FileConflictDecision, error) {
				prompts++
				if prompts == 1 {
					if err := os.Rename(filepath.Join(root, "to", "folder"), filepath.Join(root, "to", "previous")); err != nil {
						t.Fatal(err)
					}
					mustMkdirAll(t, filepath.Join(root, "to", "folder"))
					mustWrite(t, filepath.Join(root, "to", "folder", "replacement.txt"), "different-object")
					return contract.FileConflictDecision{Policy: "merge"}, nil
				}
				return contract.FileConflictDecision{Policy: "skip"}, nil
			}
			result, err := m.Action(WithFileConflicts(context.Background(), session), contract.FileActionRequest{Action: action, Sources: []string{src.Path}, Target: "/to", ExpectedResourceVersions: map[string]string{src.Path: src.ResourceVersion}, ConflictPolicies: &contract.FileConflictPolicies{}})
			body, readErr := os.ReadFile(filepath.Join(root, "to", "folder", "new.txt"))
			t.Logf("prompts=%d result=%+v err=%v replacement.new=%q readErr=%v", prompts, result, err, body, readErr)
			if readErr == nil {
				t.Fatalf("approved directory was replaced, but task wrote into replacement without fresh decision; body=%q", body)
			}
		})
	}
}

func TestIndependentSkipTerminalState(t *testing.T) {
	for _, action := range []string{"rename", "compress", "extract"} {
		t.Run(action, func(t *testing.T) {
			m, root := newTestManager(t)
			mustMkdirAll(t, filepath.Join(root, "source"))
			mustMkdirAll(t, filepath.Join(root, "target"))
			mustWrite(t, filepath.Join(root, "source", "a.txt"), "new")
			input := contract.FileActionRequest{Action: action, Target: "/target", ConflictPolicies: &contract.FileConflictPolicies{}}
			switch action {
			case "rename":
				input.Sources = []string{"/source/a.txt"}
				input.Name = "a.txt"
				mustWrite(t, filepath.Join(root, "target", "a.txt"), "old")
			case "compress":
				input.Sources = []string{"/source/a.txt"}
				input.Name = "output.zip"
				input.Format = "zip"
				mustWrite(t, filepath.Join(root, "target", "output.zip"), "old")
			case "extract":
				if _, err := m.Action(context.Background(), contract.FileActionRequest{Action: "compress", Sources: []string{"/source/a.txt"}, Target: "/", Name: "input.zip", Format: "zip"}); err != nil {
					t.Fatal(err)
				}
				input.Sources = []string{"/input.zip"}
				input.Name = "folder"
				input.Format = "zip"
				mustMkdirAll(t, filepath.Join(root, "target", "folder"))
			}
			input.ExpectedResourceVersions = map[string]string{}
			for _, source := range input.Sources {
				e, err := m.Stat(source)
				if err != nil {
					t.Fatal(err)
				}
				input.ExpectedResourceVersions[source] = e.ResourceVersion
			}
			job, err := m.StartArchiveJob(input)
			if err != nil {
				t.Fatal(err)
			}
			waiting := waitConflictJob(t, m, job.ID, "waiting_input")
			if err := m.DecideArchiveJob(job.ID, contract.FileConflictDecision{Revision: waiting.Revision, Policy: "skip"}); err != nil {
				t.Fatal(err)
			}
			var terminal contract.FileArchiveJob
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				jobs, err := m.ArchiveJobs()
				if err != nil {
					t.Fatal(err)
				}
				for _, j := range jobs {
					if j.ID == job.ID && !archiveJobActive(j.State) {
						terminal = j
					}
				}
				if terminal.ID != "" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Logf("state=%s skipped=%d failed=%+v", terminal.State, terminal.Result.SkippedCount, terminal.Result.Failed)
			if terminal.State != "skipped" || len(terminal.Result.Failed) > 0 {
				t.Fatalf("explicit skip recorded as failure: %+v", terminal)
			}
		})
	}
}

func TestIndependentCancelledDirectoryReceiveReleasesReservation(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "target", "folder"))
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data.Bytes())
	hash := hex.EncodeToString(digest[:])
	key := strings.Repeat("a", 64)
	ctx := context.Background()
	input := contract.FileReceiveInput{Directory: "/target", Name: "folder", Kind: "directory", SizeBytes: -1, SourceKey: key, ConflictPolicies: &contract.FileConflictPolicies{}}
	session, err := m.BeginReceive(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	session, err = m.WriteReceiveChunk(ctx, session.ID, key, 0, bytes.NewReader(data.Bytes()), hash)
	if err != nil {
		t.Fatal(err)
	}
	session, err = m.CommitReceive(ctx, session.ID, key, int64(data.Len()), hash)
	if err != nil {
		t.Fatal(err)
	}
	waitConflictJob(t, m, session.PublishJobID, "waiting_input")
	if err := m.AbortReceive(ctx, session.ID, key); err != nil {
		t.Fatal(err)
	}
	waitConflictJob(t, m, session.PublishJobID, "cancelled")
	if err := m.AbortReceive(ctx, session.ID, key); err != nil {
		t.Fatal(err)
	}
	record := m.receives.records[session.ID]
	if record != nil {
		_, statErr := os.Stat(filepath.Join(root, "target", ".kpanel-upload-"+session.ID))
		t.Logf("after terminal cancellation: receiveState=%s reservation=%d bodyStat=%v", record.Session.State, receiveReservation(record.Input), statErr)
	}
	input.Name = "another"
	input.SourceKey = strings.Repeat("b", 64)
	next, err := m.BeginReceive(ctx, input)
	t.Logf("next directory receive=%+v err=%v", next, err)
	if err != nil {
		t.Fatalf("terminal cancelled directory still blocks next receive: %v", err)
	}
}
