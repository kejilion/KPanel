package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type retryCloseProcess struct {
	*fakeProcess
	killError error
	kills     int
	closes    int
}

type slowCloseProcess struct {
	*fakeProcess
	started chan struct{}
	release chan struct{}
}

func (p *slowCloseProcess) Kill() error { close(p.started); <-p.release; return nil }

func TestTerminalCloseDoesNotBlockOutputOrOtherOpens(t *testing.T) {
	p := &slowCloseProcess{fakeProcess: newFakeProcess(), started: make(chan struct{}), release: make(chan struct{})}
	first := true
	m := New(Config{Starter: func(uint16, uint16) (Process, error) {
		if first {
			first = false
			return p, nil
		}
		return newFakeProcess(), nil
	}})
	s, _ := m.Open("owner", 24, 80)
	done := make(chan error, 1)
	go func() { done <- m.Close("owner", s.ID) }()
	<-p.started
	checked := make(chan error, 1)
	go func() {
		if _, err := m.Output(context.Background(), "owner", s.ID, 0, 0); err != nil {
			checked <- err
			return
		}
		_, err := m.Open("other", 24, 80)
		checked <- err
	}()
	select {
	case err := <-checked:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("pending close blocked output or another open")
	}
	close(p.release)
	if err := <-done; err != nil {
		t.Error(err)
	}
	m.CloseAll()
}

func (p *retryCloseProcess) Kill() error  { p.kills++; return p.killError }
func (p *retryCloseProcess) Close() error { p.closes++; return p.fakeProcess.Close() }

func TestCloseFailureRetainsSessionForRetry(t *testing.T) {
	p := &retryCloseProcess{fakeProcess: newFakeProcess(), killError: errors.New("kill failed")}
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
	t.Cleanup(func() { p.killError = nil; m.CloseAll() })
	s, err := m.Open("owner", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close("other", s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner isolation: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := m.Close("owner", s.ID); !errors.Is(err, p.killError) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		item, _ := m.lookup("owner", s.ID)
		if item.snapshot().Closed || !item.isActive() || p.closes != 0 {
			t.Fatal("failed kill falsely closed session or PTY")
		}
	}
	p.killError = nil
	if err := m.Close("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Close("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	if p.kills != 3 || p.closes != 1 {
		t.Fatalf("kills=%d closes=%d", p.kills, p.closes)
	}
}

func TestTerminalFailedCloseSurvivesExitAndReaping(t *testing.T) {
	p := &retryCloseProcess{fakeProcess: newFakeProcess(), killError: errors.New("kill failed")}
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
	t.Cleanup(func() { p.killError = nil; m.CloseAll() })
	s, _ := m.Open("owner", 24, 80)
	_ = m.Close("owner", s.ID)
	item, _ := m.lookup("owner", s.ID)
	item.setExit(nil, time.Now().UTC())
	m.reap(time.Now().UTC().Add(9 * time.Hour))
	if _, err := m.lookup("owner", s.ID); err != nil || !item.isActive() {
		t.Fatal("failed close lost retry identity or capacity")
	}
	p.killError = nil
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			if err := m.Close("owner", s.ID); err != nil {
				t.Error(err)
			}
		})
	}
	wait.Wait()
	if p.kills != 3 || p.closes != 1 {
		t.Fatalf("kills=%d closes=%d", p.kills, p.closes)
	}
}

func TestTerminalUnitStopFailureRetryAndConfirmedAbsence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixed local fake systemctl requires POSIX shell")
	}
	for _, state := range []string{"loaded", "not-found", "query-failed"} {
		t.Run(state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "systemctl")
			script := "#!/bin/sh\nif [ \"$1\" = stop ]; then exit 1; fi\n"
			if state == "query-failed" {
				script += "exit 1\n"
			} else {
				script += "printf '%s\\n' " + state + "\n"
			}
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			p := &retryCloseProcess{fakeProcess: newFakeProcess()}
			t.Cleanup(func() { _ = p.fakeProcess.Close() })
			wrapped := &transientTerminalProcess{Process: p, systemctl: path, unit: "kpanel-terminal-test"}
			err := wrapped.Kill()
			if state == "not-found" {
				if err != nil || p.kills != 1 {
					t.Fatalf("confirmed absence: %v", err)
				}
				return
			}
			if err == nil || p.kills != 0 || wrapped.stopped {
				t.Fatal("unit stop failure was swallowed")
			}
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := wrapped.Kill(); err != nil || p.kills != 1 || !wrapped.stopped {
				t.Fatalf("retry=%v kills=%d", err, p.kills)
			}
		})
	}
}
