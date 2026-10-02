package sites

import (
	"errors"
	"io"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const inputJobID = "0123456789abcdef0123456789abcdef"

func inputJobManager(t *testing.T, status string, inputOpen bool) (*Manager, *recipeJobRegistry) {
	t.Helper()
	registry := newRecipeJobRegistry(t.TempDir())
	manager := &Manager{recipeJobs: registry}
	putInputJob(t, registry, status, inputOpen)
	return manager, registry
}

func putInputJob(t *testing.T, registry *recipeJobRegistry, status string, inputOpen bool) {
	t.Helper()
	job := RecipeJob{
		ID: inputJobID, Domain: "proxy.example.com", Recipe: "reverse-proxy",
		Status: status, Stage: "installing", Interactive: true, InputOpen: inputOpen,
	}
	if err := registry.put(job); err != nil {
		t.Fatal(err)
	}
}

// The probe and the write share one rule, and a write that provably put nothing
// into the FIFO says so, which is what lets the input stream retry it.
func TestInstallationInputOpenIsTheRuleOfWriteInstallationInput(t *testing.T) {
	manager, registry := inputJobManager(t, "running", true)
	if err := hostpty.CreateInput(registry.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	reader, err := hostpty.OpenInput(registry.inputPath(inputJobID))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := manager.InstallationInputOpen(inputJobID); err != nil {
		t.Fatalf("running job: %v", err)
	}
	if err := manager.WriteInstallationInput(inputJobID, "ls\r"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 3)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "ls\r" {
		t.Fatalf("FIFO delivered %q (%v)", got, err)
	}

	putInputJob(t, registry, "succeeded", false)
	if err := manager.InstallationInputOpen(inputJobID); !errors.Is(err, ErrConflict) {
		t.Fatalf("finished job probe: %v", err)
	}
	err = manager.WriteInstallationInput(inputJobID, "late\r")
	if !errors.Is(err, ErrConflict) || errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("a finished job is closed for good, not retryable: %v", err)
	}
	for _, id := range []string{"ffffffffffffffffffffffffffffffff", "not-a-job"} {
		if err := manager.InstallationInputOpen(id); !errors.Is(err, ErrConflict) {
			t.Fatalf("job %q: %v", id, err)
		}
	}
}

func TestWriteInstallationInputWithoutAReaderIsReplaySafe(t *testing.T) {
	manager, registry := inputJobManager(t, "running", true)
	if err := hostpty.CreateInput(registry.inputPath(inputJobID)); err != nil {
		t.Fatal(err)
	}
	err := manager.WriteInstallationInput(inputJobID, "ls\r")
	if !errors.Is(err, ErrConflict) || !errors.Is(err, hostpty.ErrNotWritten) {
		t.Fatalf("no reader yet: %v", err)
	}
}
