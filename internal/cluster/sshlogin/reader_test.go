package sshlogin

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

type testRunner struct {
	journal []byte
	calls   int
}

func (r *testRunner) Run(context.Context, string, ...string) ([]byte, error) {
	r.calls++
	return r.journal, nil
}

func (r *testRunner) LookPath(string) (string, error) {
	return "/usr/bin/journalctl", nil
}

func TestParseSSHLoginEntryKeepsOnlyAcceptedFields(t *testing.T) {
	when := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	event, ok := parseSSHLoginEntry(logEntry{
		cursor: "cursor-1", identifier: "sshd",
		message: "sshd: Accepted publickey for admin from 203.0.113.9 port 22 ssh2 secret=hidden",
	}, when)
	if !ok || event.ID != "cursor-1" || event.Username != "admin" ||
		event.RemoteAddress != "203.0.113.9" || event.Method != "publickey" {
		t.Fatalf("parseSSHLoginEntry() = %#v, %v", event, ok)
	}
	for _, sample := range []logEntry{
		{identifier: "sshd", message: "sshd: Failed password for admin from 203.0.113.9"},
		{identifier: "sshd", message: "sshd: Accepted publickey for admin from 203.0.113.9\npassword=secret"},
		{identifier: "cron", message: "cron: Accepted publickey for admin from 203.0.113.9"},
	} {
		if _, ok := parseSSHLoginEntry(sample, when); ok {
			t.Fatalf("parseSSHLoginEntry() accepted unsafe or non-login message: %q", sample.message)
		}
	}
	if event, ok := parseSSHLoginEntry(logEntry{
		identifier: "sshd", message: "sshd: Accepted password for root from ::1 port 22 ssh2",
	}, when); !ok || event.RemoteAddress != "::1" {
		t.Fatalf("parseSSHLoginEntry() rejected compressed IPv6 loopback: %#v, %v", event, ok)
	}
}

func TestReaderFallsBackWhenJournalHasNoAcceptedEvent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host log collection is Linux-only")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "auth.log"), []byte(
		"sshd[42]: Accepted publickey for admin from 203.0.113.9 port 22 ssh2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &testRunner{journal: []byte(
		`{"SYSLOG_IDENTIFIER":"sshd","MESSAGE":"sshd: Failed password for admin from 203.0.113.9"}` + "\n",
	)}
	reader := NewReader(Config{
		LogRoot: root, Runner: runner, EffectiveUID: func() int { return 0 },
	})
	event, err := reader.LatestSSHLogin(context.Background())
	if err != nil || event == nil || event.Username != "admin" || event.Method != "publickey" {
		t.Fatalf("LatestSSHLogin() = %#v, %v", event, err)
	}
}

func TestReaderFallsBackToAlpineMessages(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host log collection is Linux-only")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "messages"), []byte(
		"Sep 14 10:11:12 alpine crond[5]: routine job\n"+
			"Sep 14 10:12:13 alpine sshd[42]: Accepted publickey for admin from 203.0.113.10 port 22 ssh2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &testRunner{journal: []byte("invalid journal\n")}
	reader := NewReader(Config{
		LogRoot: root, Runner: runner, EffectiveUID: func() int { return 0 },
	})
	event, err := reader.LatestSSHLogin(context.Background())
	if err != nil || event == nil || event.Username != "admin" || event.RemoteAddress != "203.0.113.10" {
		t.Fatalf("LatestSSHLogin() = %#v, %v", event, err)
	}
}

func TestReaderUsesLiveOpenRCMessagesBeforeStrayJournalctl(t *testing.T) {
	root := t.TempDir()
	runRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runRoot, "openrc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messages"), []byte(
		"Sep 14 10:12:13 alpine sshd[42]: Accepted publickey for openrc-user from 203.0.113.10 port 22 ssh2\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &testRunner{journal: []byte(
		`{"SYSLOG_IDENTIFIER":"sshd","MESSAGE":"Accepted password for journal-user from 198.51.100.4"}` + "\n",
	)}
	reader := NewReader(Config{
		LogRoot: root, RunRoot: runRoot, Runner: runner, EffectiveUID: func() int { return 0 },
	})
	event, err := reader.latestFromLogs(
		context.Background(), time.Date(2026, 9, 14, 10, 15, 0, 0, time.UTC),
	)
	if err != nil || event == nil || event.Username != "openrc-user" || event.RemoteAddress != "203.0.113.10" {
		t.Fatalf("LatestSSHLogin() = %#v, %v", event, err)
	}
	if runner.calls != 0 {
		t.Fatalf("live OpenRC invoked stray journalctl %d times", runner.calls)
	}
}

func TestRelayReaderDoesNotNeedHostLogCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("relay reader is Linux-only")
	}
	path := filepath.Join(t.TempDir(), "ssh-login.json")
	want := contract.SSHLoginEvent{
		ID: "cursor-2", OccurredAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
		Username: "root", RemoteAddress: "2001:db8::10", Method: "password",
	}
	if err := WriteSSHLoginEvent(path, want); err != nil {
		t.Fatal(err)
	}
	runner := &testRunner{}
	reader := NewReader(Config{
		EventPath: path, Runner: runner, EffectiveUID: func() int { return 1000 },
	})
	got, err := reader.LatestSSHLogin(context.Background())
	if err != nil || got == nil || *got != want {
		t.Fatalf("relay LatestSSHLogin() = %#v, %v", got, err)
	}
	if runner.calls != 0 {
		t.Fatalf("relay reader invoked host command %d times", runner.calls)
	}
}

func TestEventFileIsStrictAndCredentialFree(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ssh-login.json")
	event := contract.SSHLoginEvent{
		ID: "cursor-1", OccurredAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
		Username: "admin", RemoteAddress: "203.0.113.9", Method: "publickey",
	}
	if err := WriteSSHLoginEvent(path, event); err != nil {
		t.Fatalf("WriteSSHLoginEvent() error = %v", err)
	}
	got, err := readEventFile(path)
	if err != nil || got == nil || *got != event {
		t.Fatalf("readEventFile() = %#v, %v", got, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "admin") || strings.Contains(string(content), "secret") {
		t.Fatalf("relay content is unexpected: %s", content)
	}
	invalid := strings.TrimSpace(string(content))
	invalid = strings.TrimSuffix(invalid, "}") + `,"unexpected":true}`
	if err := os.WriteFile(path, []byte(invalid), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := readEventFile(path); err == nil {
		t.Fatal("readEventFile() accepted an unknown field")
	}
}
