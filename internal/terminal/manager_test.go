package terminal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTransientTerminalCommandUsesFixedRootShellContract(t *testing.T) {
	environment := terminalEnvironment("/bin/bash")
	command := transientTerminalCommand(
		"/usr/bin/systemd-run",
		"/bin/bash",
		"/root",
		"kpanel-terminal-test",
		environment,
	)
	joined := strings.Join(command.Args, " ")
	for _, expected := range []string{
		"--wait",
		"--collect",
		"--pty",
		"--unit=kpanel-terminal-test",
		"--property=User=root",
		"--property=WorkingDirectory=/root",
		"--property=NoNewPrivileges=no",
		"--property=ProtectSystem=no",
		"--property=RuntimeMaxSec=8h",
		"--property=PartOf=kejilion-agent.service",
		"--setenv=TERM=xterm-256color",
		"-- /bin/bash -l",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("command %q does not contain %q", joined, expected)
		}
	}
	if command.Dir != "/root" {
		t.Fatalf("command directory = %q", command.Dir)
	}
}

type fakeProcess struct {
	mu      sync.Mutex
	reader  *io.PipeReader
	writer  *io.PipeWriter
	input   bytes.Buffer
	resized [2]uint16
	killed  bool
}

func newFakeProcess() *fakeProcess {
	reader, writer := io.Pipe()
	return &fakeProcess{reader: reader, writer: writer}
}

func TestBackupBusyRetainsTerminalUntilScopeClosed(t *testing.T) {
	p := newFakeProcess()
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
	defer m.CloseAll()
	if m.Busy() {
		t.Fatal("empty manager is busy")
	}
	s, err := m.Open("backup-test", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Busy() {
		t.Fatal("running shell escaped backup gate")
	}
	item, err := m.lookup("backup-test", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	item.setExit(nil, time.Now())
	if !m.Busy() {
		t.Fatal("exited shell scope escaped before close")
	}
	if err := m.Close("backup-test", s.ID); err != nil {
		t.Fatal(err)
	}
	if m.Busy() {
		t.Fatal("closed shell blocks backup")
	}
}

func (p *fakeProcess) Read(data []byte) (int, error) { return p.reader.Read(data) }
func (p *fakeProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.Write(data)
}
func (p *fakeProcess) Close() error { _ = p.writer.Close(); return p.reader.Close() }
func (p *fakeProcess) Wait() error  { return nil }
func (p *fakeProcess) Kill() error  { p.mu.Lock(); p.killed = true; p.mu.Unlock(); return nil }
func (p *fakeProcess) Resize(rows, columns uint16) error {
	p.mu.Lock()
	p.resized = [2]uint16{rows, columns}
	p.mu.Unlock()
	return nil
}

func TestManagerOwnsBuffersAndTerminalLifecycle(t *testing.T) {
	process := newFakeProcess()
	manager := New(Config{Starter: func(uint16, uint16) (Process, error) { return process, nil }, BufferBytes: 8})
	snapshot, err := manager.Open("user-a", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := process.writer.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := manager.Output(ctx, "user-a", snapshot.ID, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(output.Data) != "23456789" || !output.Truncated || output.NextOffset != 10 {
		t.Fatalf("unexpected output: %#v", output)
	}
	if err := manager.Input("user-a", snapshot.ID, []byte("echo ok\n")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Resize("user-a", snapshot.ID, 40, 120); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Output(ctx, "user-b", snapshot.ID, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read = %v", err)
	}
	if err := manager.Close("user-a", snapshot.ID); err != nil {
		t.Fatal(err)
	}
}

func TestManagerEnforcesSessionAndInputLimits(t *testing.T) {
	manager := New(Config{Starter: func(uint16, uint16) (Process, error) { return newFakeProcess(), nil }, MaxSessions: 1, MaxOwnerSessions: 1})
	if _, err := manager.Open("owner", 24, 80); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Open("owner", 24, 80); !errors.Is(err, ErrLimit) {
		t.Fatalf("second open = %v", err)
	}
}
