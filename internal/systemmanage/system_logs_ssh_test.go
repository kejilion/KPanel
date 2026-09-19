package systemmanage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestParseSSHLoginEntryKeepsOnlyAcceptedSSHFields(t *testing.T) {
	occurredAt := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	event, ok := parseSSHLoginEntry(contract.SystemLogEntry{
		Timestamp:  &occurredAt,
		Cursor:     "cursor-1",
		Identifier: "sshd",
		Message:    "sshd[42]: Accepted publickey for admin from 2001:db8::10 port 22 ssh2",
	}, occurredAt.Add(time.Minute))
	if !ok {
		t.Fatal("parseSSHLoginEntry() rejected a valid accepted login")
	}
	if event.ID != "cursor-1" || event.Username != "admin" || event.RemoteAddress != "2001:db8::10" || event.Method != "publickey" || !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("parsed SSH event = %#v", event)
	}
	for _, message := range []string{
		"sshd[42]: Failed password for admin from 203.0.113.10 port 22 ssh2",
		"sshd[42]: Accepted publickey for admin from evil.example;rm -rf /",
		"systemd[1]: Started unrelated service",
	} {
		if _, ok := parseSSHLoginEntry(contract.SystemLogEntry{Identifier: "sshd", Message: message}, occurredAt); ok {
			t.Fatalf("parseSSHLoginEntry() accepted unsafe or non-login message: %q", message)
		}
	}
}

func TestLatestSSHLoginFallsBackWhenJournalHasNoAcceptedEvent(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(`{"SYSLOG_IDENTIFIER":"sshd","MESSAGE":"sshd: Failed password for admin from 203.0.113.9"}` + "\n"), nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	manager.logRoot = t.TempDir()
	mustWrite(t, filepath.Join(manager.logRoot, "auth.log"),
		"sshd[42]: Accepted publickey for admin from 203.0.113.9 port 22 ssh2\n")
	event, err := manager.latestSSHLoginFromLogs(context.Background(), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil || event == nil {
		t.Fatalf("latestSSHLoginFromLogs() = %#v, %v", event, err)
	}
	if event.Username != "admin" || event.RemoteAddress != "203.0.113.9" || event.Method != "publickey" {
		t.Fatalf("unexpected fallback SSH event: %#v", event)
	}
}

func TestSSHLoginEventFileIsStrictAndCredentialFree(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ssh-login.json")
	event := contract.SSHLoginEvent{
		ID: "cursor-1", OccurredAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
		Username: "admin", RemoteAddress: "203.0.113.9", Method: "publickey",
	}
	if err := WriteSSHLoginEvent(path, event); err != nil {
		t.Fatalf("WriteSSHLoginEvent() error = %v", err)
	}
	got, err := readSSHLoginEventFile(path)
	if err != nil || got == nil || *got != event {
		t.Fatalf("readSSHLoginEventFile() = %#v, %v", got, err)
	}
	if missing, err := readSSHLoginEventFile(filepath.Join(directory, "missing.json")); err != nil || missing != nil {
		t.Fatalf("missing event file = %#v, %v", missing, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := strings.TrimSpace(string(content))
	invalid = strings.TrimSuffix(invalid, "}") + `,"unexpected":true}`
	if err := os.WriteFile(path, []byte(invalid), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHLoginEventFile(path); err == nil {
		t.Fatal("readSSHLoginEventFile() accepted an unknown field")
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o660); err != nil {
			t.Fatal(err)
		}
		if _, err := readSSHLoginEventFile(path); err == nil {
			t.Fatal("readSSHLoginEventFile() accepted a group-writable event file")
		}
	}
}
