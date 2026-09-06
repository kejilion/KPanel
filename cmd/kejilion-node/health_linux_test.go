//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestHealthFilePermissionsAndSpecialFiles(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated root for real ownership boundary")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o750)
	p := filepath.Join(dir, "update-status.json")
	if got := readUpdateHealth(p, time.Now()); got.State != "missing" {
		t.Fatal(got)
	}
	os.WriteFile(p, []byte(`{"state":"missing"}`), 0o640)
	if got := readUpdateHealth(p, time.Now()); got.State != "missing" {
		t.Fatal(got)
	}
	os.Chmod(p, 0o644)
	if got := readUpdateHealth(p, time.Now()); got.State != "invalid" {
		t.Fatal("world-readable accepted")
	}
	os.Remove(p)
	os.Symlink("/dev/zero", p)
	if got := readUpdateHealth(p, time.Now()); got.State != "invalid" {
		t.Fatal("symlink accepted")
	}
	os.Remove(p)
	syscall.Mkfifo(p, 0o640)
	if got := readUpdateHealth(p, time.Now()); got.State != "invalid" {
		t.Fatal("fifo accepted")
	}
	os.Remove(p)
	os.WriteFile(p, []byte(`{}`), 0o640)
	os.Chown(p, 65534, 0)
	if got := readUpdateHealth(p, time.Now()); got.State != "invalid" {
		t.Fatal("untrusted owner accepted")
	}
}
