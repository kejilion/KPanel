package diagnostics

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const inputJobID = "0123456789abcdef0123456789abcdef"

func inputJobService(t *testing.T, status string, inputOpen bool) *Service {
	t.Helper()
	root := t.TempDir()
	runner := &fakeRunner{}
	service := &Service{
		runner:       runner,
		scriptFinder: func() (string, error) { return "/usr/local/bin/k", nil },
		now:          fixedNow,
		jobs:         make(map[string]record),
	}
	if err := service.configure(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), runner); err != nil {
		t.Fatal(err)
	}
	putInputJob(t, service, status, inputOpen)
	return service
}

func putInputJob(t *testing.T, service *Service, status string, inputOpen bool) {
	t.Helper()
	service.mu.Lock()
	defer service.mu.Unlock()
	item := record{Job: Job{
		ID: inputJobID, CheckID: "yabs", Status: status, Stage: "running",
		Interactive: true, InputOpen: inputOpen, CreatedAt: fixedNow(),
	}}
	if err := service.putLocked(item); err != nil {
		t.Fatal(err)
	}
}

// The probe and the write share one rule, and a write that provably put nothing
// into the FIFO says so, which is what lets the input stream retry it.
func TestInputOpenIsTheRuleOfWriteInput(t *testing.T) {
	service := inputJobService(t, "running", true)
	if err := hostpty.CreateInput(service.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	reader, err := hostpty.OpenInput(service.inputPath(inputJobID))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := service.InputOpen(inputJobID); err != nil {
		t.Fatalf("running job: %v", err)
	}
	if err := service.WriteInput(inputJobID, "ls\r"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "ls\r" {
		t.Fatalf("FIFO delivered %q (%v)", got, err)
	}

	putInputJob(t, service, "succeeded", false)
	if err := service.InputOpen(inputJobID); !errors.Is(err, ErrConflict) {
		t.Fatalf("finished job probe: %v", err)
	}
	err = service.WriteInput(inputJobID, "late\r")
	if !errors.Is(err, ErrConflict) || errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("a finished job is closed for good, not retryable: %v", err)
	}
	for _, id := range []string{"ffffffffffffffffffffffffffffffff", "not-a-job"} {
		if err := service.InputOpen(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("job %q: %v", id, err)
		}
	}
}

func TestWriteInputWithoutAReaderIsReplaySafe(t *testing.T) {
	service := inputJobService(t, "running", true)
	if err := hostpty.CreateInput(service.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	err := service.WriteInput(inputJobID, "ls\r")
	if !errors.Is(err, ErrConflict) || !errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("no reader yet: %v", err)
	}
}
