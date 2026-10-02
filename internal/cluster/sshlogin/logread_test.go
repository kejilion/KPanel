package sshlogin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type logreadRunner struct {
	output []byte
	err    error
	calls  int
	name   string
	args   []string
}

func (runner *logreadRunner) LookPath(name string) (string, error) {
	if name == "logread" {
		return "/sbin/logread", nil
	}
	return "", errors.New("unavailable")
}

func (runner *logreadRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls++
	runner.name, runner.args = name, args
	return runner.output, runner.err
}

func TestReaderUsesBoundedLogdSnapshotWithoutJournalOrFiles(t *testing.T) {
	when := time.Date(2026, 10, 2, 10, 20, 30, 123000000, time.UTC)
	line := fmt.Sprintf("Fri Oct  2 10:20:30 2026 [%d.123] authpriv.notice dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:50222\n", when.Unix())
	runner := &logreadRunner{output: []byte(line)}
	reader := NewReader(Config{LogRoot: t.TempDir(), RunRoot: t.TempDir(), Runner: runner})
	event, err := reader.latestFromLogs(context.Background(), when.Add(time.Second))
	if err != nil || event == nil || event.Username != "root" || event.RemoteAddress != "192.0.2.9" || event.Method != "password" || !event.OccurredAt.Equal(when) {
		t.Fatalf("logd result = %#v, %v", event, err)
	}
	if runner.calls != 1 || runner.name != "/sbin/logread" || !reflect.DeepEqual(runner.args, []string{"-l", "50", "-z", "4", "-z", "10", "-t"}) {
		t.Fatalf("unexpected log command: %#v", runner)
	}
	runner.output = []byte(strings.Replace(line, "10:20:30", "18:20:30", 1))
	again, err := reader.latestFromLogs(context.Background(), when.Add(time.Minute))
	if err != nil || again == nil || again.ID != event.ID || !again.OccurredAt.Equal(when) {
		t.Fatalf("unchanged log was not stable: %#v, %v", again, err)
	}
}

func TestLogreadRejectsUnrelatedTagsInjectedLinesAndBounds(t *testing.T) {
	prefix := "Fri Oct  2 10:20:30 2026 [1790936430.123] "
	for _, line := range []string{
		prefix + "daemon.notice dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:2222",
		prefix + "authpriv.notice app[42]: dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:2222",
		prefix + "authpriv.notice dropbear-malicious[42]: Password auth succeeded for 'root' from 192.0.2.9:2222",
		prefix + "authpriv.notice dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:2222\rsecret",
	} {
		runner := &logreadRunner{output: []byte(line)}
		entries, err := NewReader(Config{Runner: runner}).readLogread(context.Background())
		if err == nil && latest(entries, time.Now()) != nil {
			t.Fatalf("accepted unrelated or injected log line %q", line)
		}
	}
	for _, output := range []string{strings.Repeat("a", int(journalMaxBytes)+1), strings.Repeat("ignored\n", journalLines+1)} {
		_, err := NewReader(Config{Runner: &logreadRunner{output: []byte(output)}}).readLogread(context.Background())
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("unbounded output accepted: %v", err)
		}
	}
}

func TestDropbearAcceptsOnlyCompletedAuthentication(t *testing.T) {
	when := time.Now().UTC()
	for _, sample := range []struct{ message, method, address string }{
		{"Password auth succeeded for 'root' from 192.0.2.9:2222", "password", "192.0.2.9"},
		{"Pubkey auth succeeded for 'admin' with ssh-ed25519 key SHA256:aBc/Def+ghi= from 2001:db8::7:2222", "publickey", "2001:db8::7"},
		{"Password auth succeeded for 'root' from [fe80::1%eth0]:2222", "password", "fe80::1%eth0"},
	} {
		event, ok := parseSSHLoginEntry(logEntry{identifier: "dropbear", message: sample.message}, when)
		if !ok || event.Method != sample.method || event.RemoteAddress != sample.address {
			t.Fatalf("Dropbear parse(%q) = %#v, %v", sample.message, event, ok)
		}
	}
	for _, message := range []string{
		"Bad password attempt for 'root' from 192.0.2.9:2222",
		"Password auth succeeded for 'root' from 192.0.2.9:2222, extra auth required",
		"Pubkey auth succeeded for 'root' with ssh-rsa key SHA256:abc from 192.0.2.9:2222, extra auth required",
		"Password auth succeeded for 'root' from attacker.example:2222",
		"Password auth succeeded for 'root' from 192.0.2.9:65536",
		"Password auth succeeded for 'root' from 192.0.2.9:0",
		"Password auth succeeded for 'root with spaces' from 192.0.2.9:2222",
		"Password auth succeeded for 'root' from 192.0.2.9:2222\nsecret",
		"sshd: Accepted publickey for root from 192.0.2.9",
	} {
		if event, ok := parseSSHLoginEntry(logEntry{identifier: "dropbear", message: message}, when); ok {
			t.Fatalf("accepted invalid/partial Dropbear auth %q: %#v", message, event)
		}
	}
	for _, identifier := range []string{"cron", "dropbear-fake"} {
		if _, ok := parseSSHLoginEntry(logEntry{identifier: identifier, message: "Password auth succeeded for 'root' from 192.0.2.9:2222"}, when); ok {
			t.Fatalf("accepted untrusted daemon %q", identifier)
		}
		if _, ok := parseSSHLoginEntry(logEntry{identifier: identifier, message: "dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:2222"}, when); ok {
			t.Fatalf("accepted injected daemon tag from %q", identifier)
		}
	}
	event, ok := parseSSHLoginEntry(logEntry{identifier: "messages", message: "Oct  2 10:20:30 router dropbear[42]: Password auth succeeded for 'root' from 192.0.2.9:2222"}, when)
	if !ok || event.Method != "password" {
		t.Fatalf("fixed syslog Dropbear entry = %#v, %v", event, ok)
	}
}

func TestLogCommandOutputAndLifetimeAreBounded(t *testing.T) {
	output, err := (commandRunner{}).Run(context.Background(), os.Args[0], "-test.run=^TestLogCommandHelper$", "--", "overflow")
	if err == nil || len(output) != 0 || strings.Contains(err.Error(), "credential-value") {
		t.Fatalf("overflow result = %d bytes, %v", len(output), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = (commandRunner{}).Run(ctx, os.Args[0], "-test.run=^TestLogCommandHelper$", "--", "wait")
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("log command did not honor cancellation: %v after %v", err, time.Since(started))
	}
}

func TestLogreadLookupIgnoresInheritedPath(t *testing.T) {
	directory := t.TempDir()
	candidate := filepath.Join(directory, "logread")
	if err := os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	resolved, err := (commandRunner{}).LookPath("logread")
	if err == nil && resolved == candidate {
		t.Fatal("privileged log collector trusted inherited PATH")
	}
}

func TestLogCommandHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "overflow":
		_, _ = os.Stdout.WriteString(strings.Repeat("credential-value", int(journalMaxBytes)/10))
	case "wait":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}
