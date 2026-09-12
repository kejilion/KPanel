package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupArchiveAuthenticatedRoundTrip(t *testing.T) {
	source := filepath.Join(t.TempDir(), "panel.json")
	payload := bytes.Repeat([]byte("private API configuration\n"), 10000)
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	want, err := Write(&out, "long-backup-password", "test", []Source{{"panel", source}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("private API")) {
		t.Fatal("plaintext leaked")
	}
	dir := t.TempDir()
	got, err := Read(bytes.NewReader(out.Bytes()), "long-backup-password", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Parts[0] != want.Parts[0] {
		t.Fatal("manifest changed")
	}
	data, err := os.ReadFile(filepath.Join(dir, "panel.payload"))
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatal("payload changed", err)
	}
	cases := map[string][]byte{"truncated": out.Bytes()[:out.Len()-1], "appended": append(append([]byte{}, out.Bytes()...), 1)}
	changed := append([]byte{}, out.Bytes()...)
	changed[len(changed)/2] ^= 1
	cases["tampered"] = changed
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(bytes.NewReader(data), "long-backup-password", t.TempDir()); err == nil {
				t.Fatal("accepted damaged archive")
			}
		})
	}
	if _, err := Read(bytes.NewReader(out.Bytes()), "wrong-password", t.TempDir()); err == nil {
		t.Fatal("accepted wrong password")
	}
}

func TestBackupArchiveSelectionAndFileBoundary(t *testing.T) {
	for _, selection := range [][]string{nil, {"panel", "panel"}, {"../../etc"}} {
		if _, err := Selection(selection); err == nil {
			t.Fatal(selection)
		}
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(outside, []byte("secret"), 0600)
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err == nil {
		if _, err := OpenRegular(filepath.Join(dir, "link"), 100); err == nil {
			t.Fatal("followed symbolic link")
		}
	}
}

func TestBackupJobRecoveryAndSingleWriter(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := m.Reserve("import", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reserve("export", []string{"panel"}); !errors.Is(err, ErrBusy) {
		t.Fatal("allowed parallel job")
	}
	if err := m.ExpireQueued(time.Now().Add(3 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if m.Busy() {
		t.Fatal("upload lease never released")
	}
	r, _ := m.Get(record.ID)
	if r.ErrorCode != "upload_expired" {
		t.Fatal(r)
	}
	active, err := m.Reserve("export", []string{"panel"})
	if err != nil {
		t.Fatal(err)
	}
	wait := make(chan struct{})
	if err := m.Run(active.ID, func(ctx context.Context, id string) error { <-wait; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(active.ID, func(context.Context, string) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatal("job ran twice")
	}
	close(wait)
	m.Close()
	reopened, err := OpenManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	finished, _ := reopened.Get(active.ID)
	if finished.Status != "completed" {
		t.Fatal(finished)
	}
}
