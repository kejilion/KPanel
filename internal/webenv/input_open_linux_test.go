package webenv

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const inputJobID = "0123456789abcdef0123456789abcdef"

func inputJobService(t *testing.T, status string) *Service {
	t.Helper()
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	putInputJob(t, service, status)
	return service
}

func putInputJob(t *testing.T, service *Service, status string) {
	t.Helper()
	now := time.Now().UTC()
	job := Job{ID: inputJobID, Action: "update", Status: status, Stage: "running", CreatedAt: now, StartedAt: &now}
	if err := service.writeJob(job); err != nil {
		t.Fatal(err)
	}
}

// The probe and the write share one rule, and a write that provably put nothing
// into the FIFO says so, which is what lets the input stream retry it.
func TestInputOpenIsTheRuleOfWriteInput(t *testing.T) {
	service := inputJobService(t, "running")
	if err := hostpty.CreateInput(service.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	reader, err := hostpty.OpenInput(service.inputPath(inputJobID))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := service.InputOpen(inputJobID); err != nil {
		t.Fatalf("running task: %v", err)
	}
	if err := service.WriteInput(inputJobID, "ls\r"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "ls\r" {
		t.Fatalf("FIFO delivered %q (%v)", got, err)
	}

	putInputJob(t, service, "succeeded")
	if err := service.InputOpen(inputJobID); !errors.Is(err, ErrConflict) {
		t.Fatalf("finished task probe: %v", err)
	}
	err = service.WriteInput(inputJobID, "late\r")
	if !errors.Is(err, ErrConflict) || errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("a finished task is closed for good, not retryable: %v", err)
	}
	for _, id := range []string{"ffffffffffffffffffffffffffffffff", "not-a-job"} {
		if err := service.InputOpen(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("task %q: %v", id, err)
		}
	}
}

func TestWriteInputWithoutAReaderIsReplaySafe(t *testing.T) {
	service := inputJobService(t, "running")
	if err := hostpty.CreateInput(service.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	err := service.WriteInput(inputJobID, "ls\r")
	if !errors.Is(err, ErrConflict) || !errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("no reader yet: %v", err)
	}
}
