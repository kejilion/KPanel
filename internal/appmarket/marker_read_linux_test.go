package appmarket

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMarkerFIFOIsRejectedWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "appno.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { _, warning := (&Service{appRoot: root}).readMarkers(); done <- warning }()
	select {
	case warning := <-done:
		if warning == "" {
			t.Fatal("FIFO was accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("marker open blocked on FIFO")
	}
}
