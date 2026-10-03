//go:build linux

package agent

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

// A real FIFO, as the job processes use: the stream must survive a task that
// is not reading yet and still deliver every byte exactly once.
func TestJobTerminalSequencedInputThroughARealFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := hostpty.CreateInput(path); err != nil {
		t.Fatal(err)
	}
	s := testServer(t)
	s.jobInputBackends = map[string]jobInput{"app": {
		probe: func(string) error { return nil },
		write: func(_ string, data string) error {
			if err := hostpty.WriteInput(path, []byte(data)); err != nil {
				return fmt.Errorf("%w: interactive input is unavailable: %w", appmarket.ErrConflict, err)
			}
			return nil
		},
		invalid:  appmarket.ErrForbidden,
		missing:  appmarket.ErrNotFound,
		conflict: appmarket.ErrConflict,
	}}
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, ""))

	// The job process has not opened its end yet: nothing can have been written.
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 1, "first\r"), 503, "terminal_input_unavailable")

	reader, err := hostpty.OpenInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "first\r"))
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "first\r"))
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 2, "second\r"))

	want := "first\rsecond\r"
	got := make([]byte, len(want))
	done := make(chan error, 1)
	go func() { _, err := io.ReadFull(reader, got); done <- err }()
	select {
	case err := <-done:
		if err != nil || string(got) != want {
			t.Fatalf("FIFO delivered %q (err %v), want %q", got, err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("FIFO delivered only %q", strings.TrimRight(string(got), "\x00"))
	}
}
