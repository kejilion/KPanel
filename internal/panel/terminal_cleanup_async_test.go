package panel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type blockingTerminalCleanupAgent struct {
	terminalAgentStub
	started chan struct{}
	release chan struct{}
}

type shutdownTerminalCleanupAgent struct {
	terminalAgentStub
	started chan struct{}
	calls   atomic.Int32
}

type recoveringTerminalCleanupAgent struct {
	terminalAgentStub
	calls atomic.Int32
}

func (a *recoveringTerminalCleanupAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if strings.HasSuffix(path, "/close") {
		if a.calls.Add(1) == 1 {
			return AgentResponse{}, errors.New("offline")
		}
		return AgentResponse{StatusCode: http.StatusOK, Body: []byte(`{"closed":true}`)}, nil
	}
	return a.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
}

func TestTerminalCleanupRetriesDetachedSessionAfterRecovery(t *testing.T) {
	s, _ := newTestServer(t)
	agent := &recoveringTerminalCleanupAgent{}
	s.agent = agent
	s.terminalSessions["pending"] = panelTerminalSession{ID: "pending", HostID: "local", BackendSessionID: "old", CloseRequested: true, Detached: true}
	s.scheduleTerminalCleanup()
	s.terminalMu.Lock()
	done := s.terminalCleanupDone
	s.terminalMu.Unlock()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("cleanup did not retry")
	}
	s.terminalMu.Lock()
	_, exists := s.terminalSessions["pending"]
	s.terminalMu.Unlock()
	if exists || agent.calls.Load() != 2 {
		t.Fatalf("pending=%v calls=%d", exists, agent.calls.Load())
	}
}

func (a *shutdownTerminalCleanupAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if strings.HasSuffix(path, "/close") {
		if a.calls.Add(1) == 1 {
			close(a.started)
			<-ctx.Done()
			return AgentResponse{}, ctx.Err()
		}
		return AgentResponse{StatusCode: http.StatusOK, Body: []byte(`{"closed":true}`)}, nil
	}
	return a.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
}

func TestTerminalCleanupStopsBeforeServerClosesSessions(t *testing.T) {
	s, _ := newTestServer(t)
	agent := &shutdownTerminalCleanupAgent{started: make(chan struct{})}
	s.agent = agent
	s.terminalSessions["pending"] = panelTerminalSession{ID: "pending", HostID: "local", BackendSessionID: "old", CloseRequested: true}
	s.scheduleTerminalCleanup()
	select {
	case <-agent.started:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start")
	}
	closed := make(chan struct{})
	go func() { s.closeTerminalSessions(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown waited for a blocked cleanup")
	}
	if got := agent.calls.Load(); got != 2 {
		t.Fatalf("close attempts = %d, want cleanup and shutdown", got)
	}
	s.scheduleTerminalCleanup()
	s.terminalMu.Lock()
	running := s.terminalCleanupRunning
	s.terminalMu.Unlock()
	if running {
		t.Fatal("cleanup restarted after shutdown")
	}
}

func (a *blockingTerminalCleanupAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if strings.HasSuffix(path, "/close") {
		select {
		case a.started <- struct{}{}:
		default:
		}
		select {
		case <-a.release:
			return AgentResponse{}, errors.New("offline")
		case <-ctx.Done():
			return AgentResponse{}, ctx.Err()
		}
	}
	return a.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
}

func TestTerminalCleanupDoesNotBlockOpeningOtherSessions(t *testing.T) {
	s, tokenPath := newTestServer(t)
	agent := &blockingTerminalCleanupAgent{started: make(chan struct{}, 2), release: make(chan struct{})}
	s.agent = agent
	s.terminalSessions["pending"] = panelTerminalSession{ID: "pending", HostID: "local", UserID: "another", BackendSessionID: "old", CloseRequested: true}
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	result := make(chan int, 1)
	go func() {
		response := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), cookie, csrf, headers)
		result <- response.Code
	}()
	select {
	case status := <-result:
		if status != http.StatusCreated {
			t.Fatalf("open status = %d", status)
		}
	case <-time.After(2 * time.Second):
		close(agent.release)
		t.Fatal("opening a terminal waited for unreachable cleanup")
	}
	select {
	case <-agent.started:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start")
	}
	s.scheduleTerminalCleanup()
	select {
	case <-agent.started:
		t.Fatal("started duplicate cleanup")
	default:
	}
	close(agent.release)
	deadline := time.After(2 * time.Second)
	for {
		s.terminalMu.Lock()
		running := s.terminalCleanupRunning
		_, retained := s.terminalSessions["pending"]
		s.terminalMu.Unlock()
		if !running {
			if !retained {
				t.Fatal("unconfirmed close lost its identity")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("cleanup did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}
