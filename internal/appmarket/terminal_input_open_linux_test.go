package appmarket

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const inputJobID = "abcdef0123456789abcdef0123456789"

func inputJobService(t *testing.T, status string, inputOpen bool) (*Service, *appJobRegistry) {
	t.Helper()
	registry := &appJobRegistry{
		stateDir: filepath.Join(t.TempDir(), "jobs"),
		jobs:     make(map[string]appJobRecord),
	}
	if err := ensureAppJobDirectory(registry.stateDir); err != nil {
		t.Fatal(err)
	}
	putInputJob(t, registry, status, inputOpen)
	return &Service{jobs: registry}, registry
}

func putInputJob(t *testing.T, registry *appJobRegistry, status string, inputOpen bool) {
	t.Helper()
	now := time.Now().UTC()
	record := appJobRecord{
		AppJob: AppJob{
			ID: inputJobID, AppID: "builtin-4", AppName: "test", Action: "install",
			Interactive: true, InputOpen: inputOpen, Status: status, Stage: "interactive",
			CreatedAt: now, Logs: []string{},
		},
		Adapter:  "kejilion",
		Selector: "4",
	}
	if err := registry.put(record); err != nil {
		t.Fatal(err)
	}
}

// The probe and the write share one rule, and a write that provably put nothing
// into the FIFO says so, which is what lets the input stream retry it.
func TestAppJobInputOpenIsTheRuleOfWriteAppJobInput(t *testing.T) {
	service, registry := inputJobService(t, "running", true)
	if err := createTerminalInput(registry.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	reader, err := openTerminalInput(registry.inputPath(inputJobID))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := service.AppJobInputOpen(inputJobID); err != nil {
		t.Fatalf("running job: %v", err)
	}
	if err := service.WriteAppJobInput(inputJobID, "ls\r"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "ls\r" {
		t.Fatalf("FIFO delivered %q (%v)", got, err)
	}

	putInputJob(t, registry, "succeeded", false)
	if err := service.AppJobInputOpen(inputJobID); !errors.Is(err, ErrConflict) {
		t.Fatalf("finished job probe: %v", err)
	}
	err = service.WriteAppJobInput(inputJobID, "late\r")
	if !errors.Is(err, ErrConflict) || errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("a finished job is closed for good, not retryable: %v", err)
	}
	for _, id := range []string{"ffffffffffffffffffffffffffffffff", "not-a-job"} {
		if err := service.AppJobInputOpen(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("job %q: %v", id, err)
		}
	}
}

func TestWriteAppJobInputWithoutAReaderIsReplaySafe(t *testing.T) {
	service, registry := inputJobService(t, "running", true)
	if err := createTerminalInput(registry.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	err := service.WriteAppJobInput(inputJobID, "ls\r")
	if !errors.Is(err, ErrConflict) || !errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("no reader yet: %v", err)
	}
}
