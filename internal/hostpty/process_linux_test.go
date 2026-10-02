//go:build linux

package hostpty

import (
	"errors"
	"io"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPTYChildUsesSessionAndParentDeathSignal(t *testing.T) {
	// Keep this assertion next to the platform implementation: direct OpenRC
	// terminals must not outlive the Agent if graceful CloseAll cannot run.
	attributes := ptyProcessAttributes()
	if !attributes.Setsid || !attributes.Setctty || attributes.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("unsafe PTY process attributes: %#v", attributes)
	}
}

func TestWriteInputReportsReplaySafeFailures(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if err := WriteInput(missing, []byte("x")); !errors.Is(err, ErrNotWritten) {
		t.Fatalf("missing FIFO must be replay-safe: %v", err)
	}

	path := filepath.Join(dir, "input")
	if err := CreateInput(path); err != nil {
		t.Fatal(err)
	}
	// A FIFO without a reader refuses the open, so nothing can have been written.
	if err := WriteInput(path, []byte("x")); !errors.Is(err, ErrNotWritten) {
		t.Fatalf("FIFO without reader must be replay-safe: %v", err)
	}

	reader, err := OpenInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := WriteInput(path, []byte("hello")); err != nil {
		t.Fatalf("write with reader: %v", err)
	}
	got := make([]byte, 5)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != "hello" {
		t.Fatalf("FIFO content %q err=%v", got, err)
	}
}

func TestWriteInputFullFIFOIsReplaySafeForFrameSizedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := CreateInput(path); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Fill the kernel pipe buffer. Writes no larger than PIPE_BUF are atomic:
	// once it is full a frame-sized write is refused whole, never split.
	chunk := make([]byte, 4096)
	for i := 0; ; i++ {
		if err := WriteInput(path, chunk); err != nil {
			if !errors.Is(err, ErrNotWritten) {
				t.Fatalf("full FIFO after %d writes: %v", i, err)
			}
			break
		}
		if i > 1024 {
			t.Fatal("FIFO never filled")
		}
	}
}

func TestWriteInputAfterAcceptedBytesIsNeverReplaySafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := CreateInput(path); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Larger than the kernel pipe buffer and never read: a prefix is accepted,
	// then the FIFO stays busy until the deadline.
	err = WriteInput(path, make([]byte, 256<<10))
	if !errors.Is(err, ErrPartialWrite) || errors.Is(err, ErrNotWritten) {
		t.Fatalf("partially accepted write: %v", err)
	}
}
