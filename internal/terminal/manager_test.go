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

func TestOpeningSnapshotPreservesAlreadyCapturedOutput(t *testing.T) {
	for _, test := range []struct {
		name      string
		limit     int
		want      string
		truncated bool
	}{
		{"prompt", 1024, "ready-prompt$ ", false},
		{"truncated-prompt", 4, "pt$ ", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			item := &session{id: "new-session", notify: make(chan struct{}), createdAt: now}
			// Capture can finish its first read before Open returns its snapshot.
			item.append([]byte("ready-prompt$ "), test.limit, now)
			snapshot := item.snapshot()
			output, err := item.currentOutput(snapshot.Offset, 1024, now)
			if err != nil || string(output.Data) != test.want || output.Truncated != test.truncated {
				t.Fatalf("first read at %d = %#v, %v", snapshot.Offset, output, err)
			}
			if output.NextOffset != int64(len("ready-prompt$ ")) {
				t.Fatalf("next offset = %d", output.NextOffset)
			}
		})
	}
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

func TestSelectedStarterSharesOwnershipAndCapacity(t *testing.T) {
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return newFakeProcess(), nil }, MaxSessions: 2, MaxOwnerSessions: 2})
	defer m.CloseAll()
	if _, err := m.Open("owner", 24, 80); err != nil {
		t.Fatal(err)
	}
	p := newFakeProcess()
	s, err := m.OpenWithStarter("owner", 30, 120, func(rows, cols uint16) (Process, error) {
		if rows != 30 || cols != 120 {
			t.Fatalf("dimensions = %d x %d", rows, cols)
		}
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Input("other", s.ID, []byte("id\r")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner input = %v", err)
	}
	if _, err := m.Open("other", 24, 80); !errors.Is(err, ErrLimit) {
		t.Fatalf("shared capacity = %v", err)
	}
}

type pendingCleanupProcess struct {
	*fakeProcess
	cleanupMu sync.Mutex
	fail      bool
	closes    int
}

func (p *pendingCleanupProcess) Kill() error {
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	if p.fail {
		return errors.New("injected stop failure")
	}
	return p.fakeProcess.Kill()
}
func (p *pendingCleanupProcess) Close() error {
	p.cleanupMu.Lock()
	p.closes++
	p.cleanupMu.Unlock()
	return p.fakeProcess.Close()
}
func (p *pendingCleanupProcess) Wait() error { return ErrCleanupPending }

func TestDisconnectedProcessRetainsOwnerAndQuotaUntilCleanupConfirmed(t *testing.T) {
	p := &pendingCleanupProcess{fakeProcess: newFakeProcess(), fail: true}
	m := New(Config{MaxSessions: 1, MaxOwnerSessions: 1})
	defer m.CloseAll()
	s, err := m.OpenWithStarter("owner", 24, 80, func(uint16, uint16) (Process, error) { return p, nil })
	if err != nil {
		t.Fatal(err)
	}
	_ = p.writer.CloseWithError(errors.New("transport dropped"))
	item, _ := m.lookup("owner", s.ID)
	deadline := time.Now().Add(time.Second)
	for {
		item.mu.Lock()
		pending := item.closeFailed
		exited := item.exitedAt
		item.mu.Unlock()
		if exited != nil {
			t.Fatal("unconfirmed cleanup published an exit")
		}
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("capture did not retain cleanup")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := m.Open("other", 24, 80); !errors.Is(err, ErrLimit) {
		t.Fatalf("pending capacity = %v", err)
	}
	if err := m.Resize("owner", s.ID, 30, 100); !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("pending resize = %v", err)
	}
	if err := m.Input("owner", s.ID, []byte("id\r")); !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("pending input = %v", err)
	}
	if err := m.InputSequenced("owner", s.ID, InputFrame{Stream: strings.Repeat("a", 32), Seq: 0}); !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("pending sequenced input = %v", err)
	}
	m.reap(time.Now()) // Retry even while the session has not reached idle timeout.
	p.cleanupMu.Lock()
	if p.closes != 0 {
		t.Fatal("released process handles before confirmed termination")
	}
	p.fail = false
	p.cleanupMu.Unlock()
	m.reap(time.Now())
	if !item.snapshot().Closed {
		t.Fatal("reaper did not retry pending cleanup")
	}
}

func TestFailedStarterRetainsSharedAdmissionUntilCleanup(t *testing.T) {
	p := &pendingCleanupProcess{fakeProcess: newFakeProcess(), fail: true}
	m := New(Config{MaxSessions: 1, MaxOwnerSessions: 1})
	defer m.CloseAll()
	_, err := m.OpenWithStarter("owner", 24, 80, func(uint16, uint16) (Process, error) { return p, errors.New("resize failed after start") })
	if err == nil {
		t.Fatal("failed start succeeded")
	}
	if _, err := m.Open("other", 24, 80); !errors.Is(err, ErrLimit) {
		t.Fatalf("failed start escaped capacity: %v", err)
	}
	p.cleanupMu.Lock()
	p.fail = false
	p.cleanupMu.Unlock()
	m.reap(time.Now())
	if _, err := m.OpenWithStarter("other", 24, 80, func(uint16, uint16) (Process, error) { return newFakeProcess(), nil }); err != nil {
		t.Fatalf("confirmed cleanup did not release capacity: %v", err)
	}
}

// Continuous forward polling must keep a silent session alive past the idle
// timeout: batch executions poll output every second even while the command
// produces nothing, and that polling itself is the liveness signal.
func TestOutputPollingKeepsSilentSessionPastIdleTimeout(t *testing.T) {
	now := time.Now()
	process := newFakeProcess()
	manager := New(Config{
		Starter: func(uint16, uint16) (Process, error) { return process, nil },
		Now:     func() time.Time { return now },
	})
	snapshot, err := manager.Open("user-a", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	// Advance past the configured idle timeout while polling output, simulating
	// a silent long task (no process output, no input) with the browser's
	// 1-second output long-poll landing on every tick.
	for tick := 0; tick < 35; tick++ {
		now = now.Add(time.Minute)
		if _, err := manager.Output(context.Background(), "user-a", snapshot.ID, snapshot.Offset, 0); err != nil {
			t.Fatalf("poll tick %d: %v", tick, err)
		}
	}
	manager.reap(now.UTC())
	item, err := manager.lookup("user-a", snapshot.ID)
	if err != nil {
		t.Fatalf("idle reap closed a polled session: %v", err)
	}
	if item.closed || item.exitedAt != nil {
		t.Fatal("idle reap terminated a live session that was still being polled")
	}
	_ = manager.Close("user-a", snapshot.ID)
	manager.CloseAll()
}
