package remotedownload

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestConflictDecisionCASPersistsBeforeApplyingPolicy(t *testing.T) {
	store, err := OpenJobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{ID: strings.Repeat("a", 32), State: "waiting_input", Source: "https://example.com", TargetDirectory: "/home", Name: "a.txt", CreatedAt: now, UpdatedAt: now, Revision: 1, ConflictPolicies: &contract.FileConflictPolicies{}, Conflict: &contract.FileConflict{Source: contract.FileEntry{Name: "a.txt", Kind: "file"}, Target: contract.FileEntry{Name: "a.txt", Kind: "file", Path: "/home/a.txt", ResourceVersion: "v1"}}}
	if err := store.Create(job); err != nil {
		t.Fatal(err)
	}
	decision := contract.FileConflictDecision{Revision: 1, Policy: "overwrite", ApplyToRest: true}
	writer := store.writeAtomic
	store.writeAtomic = func(string, string, []byte) error { return errors.New("disk full") }
	if err := store.Decide(job.ID, decision); err == nil {
		t.Fatal("persistence failure accepted consent")
	}
	before, _ := store.Get(job.ID)
	if before.Revision != 1 || before.Decision != nil || before.ConflictPolicies.File != "" {
		t.Fatal("unpersisted policy escaped")
	}
	store.writeAtomic = writer
	if err := store.Decide(job.ID, decision); err != nil {
		t.Fatal(err)
	}
	if err := store.Decide(job.ID, decision); !errors.Is(err, ErrDecisionConflict) {
		t.Fatalf("stale decision accepted %v", err)
	}
	after, _ := store.Get(job.ID)
	if after.Revision != 2 || after.ConflictPolicies.File != "overwrite" {
		t.Fatal(after)
	}
	after.ConflictPolicies.File = "skip"
	original, _ := store.Get(job.ID)
	if original.ConflictPolicies.File != "overwrite" {
		t.Fatal("caller mutated durable policy")
	}
}
