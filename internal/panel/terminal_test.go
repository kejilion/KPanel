package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type terminalAgentStub struct {
	mu           sync.Mutex
	opened       int
	inputs       [][]byte
	resized      int
	closed       int
	output       []byte
	next         int64
	outputStatus int
	outputBody   []byte
}

func (s *terminalAgentStub) Get(_ context.Context, path, _ string, _ string) (AgentResponse, error) {
	if !strings.HasSuffix(path, "/output") {
		return AgentResponse{StatusCode: http.StatusNotFound, ContentType: "application/json"}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outputStatus != 0 {
		return AgentResponse{StatusCode: s.outputStatus, ContentType: "application/json", Body: s.outputBody}, nil
	}
	body, _ := json.Marshal(map[string]any{
		"data":       s.output,
		"offset":     s.next,
		"nextOffset": s.next + int64(len(s.output)),
		"truncated":  false,
		"closed":     false,
	})
	s.next += int64(len(s.output))
	s.output = nil
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
}

func TestLocalTerminalSessionStopsReconnectingWhenAgentSessionIsGone(t *testing.T) {
	server, tokenPath := newTestServer(t)
	problem, _ := json.Marshal(map[string]any{
		"title": "Terminal session not found", "status": http.StatusNotFound,
		"code": "terminal_not_found", "requestId": "agent-request", "retryable": false,
	})
	stub := &terminalAgentStub{outputStatus: http.StatusNotFound, outputBody: problem}
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), sessionCookie, csrfCookie, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	var result terminalOpenResponse
	if err := json.Unmarshal(opened.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode open: %v", err)
	}
	output := authenticatedRequest(server, http.MethodGet, "/api/v1/terminal-sessions/"+result.SessionID+"/output?offset=0&wait=0", nil, sessionCookie, csrfCookie, nil)
	if output.Code != http.StatusNotFound || !strings.Contains(output.Body.String(), "terminal_not_found") {
		t.Fatalf("missing Agent terminal = %d %s", output.Code, output.Body.String())
	}
	server.terminalMu.Lock()
	_, exists := server.terminalSessions[result.SessionID]
	server.terminalMu.Unlock()
	if exists {
		t.Fatal("missing backend terminal left a stale Panel session")
	}
}

func (s *terminalAgentStub) Do(_ context.Context, method, path, _ string, _ string, body []byte) (AgentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case method == http.MethodPost && path == "/v1/terminals":
		s.opened++
		payload, _ := json.Marshal(map[string]any{
			"id": "backend-terminal", "offset": 0,
			"createdAt": time.Now().UTC(), "updatedAt": time.Now().UTC(), "closed": false,
		})
		return AgentResponse{StatusCode: http.StatusCreated, ContentType: "application/json", Body: payload}, nil
	case strings.HasSuffix(path, "/input"):
		var input struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(body, &input)
		data, _ := base64.RawStdEncoding.DecodeString(input.Data)
		s.inputs = append(s.inputs, data)
	case strings.HasSuffix(path, "/resize"):
		s.resized++
	case strings.HasSuffix(path, "/close"):
		s.closed++
		return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"closed":true}`)}, nil
	default:
		return AgentResponse{StatusCode: http.StatusNotFound, ContentType: "application/json"}, nil
	}
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"accepted":true}`)}, nil
}

func TestTerminalSessionsRequireAuthenticatedMutationGuards(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	body := []byte(`{"hostId":"local","rows":30,"columns":120}`)

	unauthenticated := performRequest(server, http.MethodPost, "/api/v1/terminal-sessions", body, map[string]string{"Content-Type": "application/json"})
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticated.Code, http.StatusUnauthorized)
	}
	missingOrigin := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	})
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing origin = %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
}

func TestLocalTerminalSessionLifecycle(t *testing.T) {
	server, tokenPath := newTestServer(t)
	stub := &terminalAgentStub{output: []byte("ready\r\n")}
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), sessionCookie, csrfCookie, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	var result terminalOpenResponse
	if err := json.Unmarshal(opened.Body.Bytes(), &result); err != nil || result.SessionID == "" {
		t.Fatalf("decode open: result=%#v err=%v", result, err)
	}

	output := authenticatedRequest(server, http.MethodGet, "/api/v1/terminal-sessions/"+result.SessionID+"/output?offset=0&wait=0", nil, sessionCookie, csrfCookie, nil)
	if output.Code != http.StatusOK || !strings.Contains(output.Body.String(), base64.StdEncoding.EncodeToString([]byte("ready\r\n"))) {
		t.Fatalf("output = %d %s", output.Code, output.Body.String())
	}
	invalidOutputQuery := authenticatedRequest(server, http.MethodGet, "/api/v1/terminal-sessions/"+result.SessionID+"/output?offset=0&wait=0&extra=1", nil, sessionCookie, csrfCookie, nil)
	if invalidOutputQuery.Code != http.StatusBadRequest {
		t.Fatalf("output with extra query = %d %s", invalidOutputQuery.Code, invalidOutputQuery.Body.String())
	}
	invalidInputQuery := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+result.SessionID+"/input?extra=1", []byte(`{"data":"bHMNCg"}`), sessionCookie, csrfCookie, headers)
	if invalidInputQuery.Code != http.StatusBadRequest {
		t.Fatalf("input with query = %d %s", invalidInputQuery.Code, invalidInputQuery.Body.String())
	}
	input := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+result.SessionID+"/input", []byte(`{"data":"bHMNCg"}`), sessionCookie, csrfCookie, headers)
	if input.Code != http.StatusOK {
		t.Fatalf("input = %d %s", input.Code, input.Body.String())
	}
	resized := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+result.SessionID+"/resize", []byte(`{"rows":40,"columns":140}`), sessionCookie, csrfCookie, headers)
	if resized.Code != http.StatusOK {
		t.Fatalf("resize = %d %s", resized.Code, resized.Body.String())
	}
	closed := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+result.SessionID+"/close", []byte(`{}`), sessionCookie, csrfCookie, headers)
	if closed.Code != http.StatusOK {
		t.Fatalf("close = %d %s", closed.Code, closed.Body.String())
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.opened != 1 || stub.resized != 1 || stub.closed != 1 || len(stub.inputs) != 1 || string(stub.inputs[0]) != "ls\r\n" {
		t.Fatalf("unexpected Agent calls: %#v", stub)
	}
}

func TestTerminalSessionIsBoundToAuthenticatedUser(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.terminalMu.Lock()
	server.terminalSessions["private"] = panelTerminalSession{
		ID: "private", UserID: "another-user", HostID: "local",
	}
	server.terminalMu.Unlock()
	response := authenticatedRequest(
		server, http.MethodGet,
		"/api/v1/terminal-sessions/private/output?offset=0&wait=0", nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "another-user") {
		t.Fatalf("cross-user lookup = %d %s", response.Code, response.Body.String())
	}
}

func TestPruneTerminalSessionsRemovesOnlyExpiredEntries(t *testing.T) {
	server, _ := newTestServer(t)
	now := time.Now().UTC()
	server.terminalMu.Lock()
	server.terminalSessions["stale"] = panelTerminalSession{ID: "stale", UserID: "user", UpdatedAt: now.Add(-time.Hour)}
	server.terminalSessions["active"] = panelTerminalSession{ID: "active", UserID: "user", UpdatedAt: now}
	server.terminalMu.Unlock()

	stale := server.pruneTerminalSessions(now.Add(-panelTerminalIdleTTL))
	if len(stale) != 1 || stale[0].ID != "stale" {
		t.Fatalf("pruned sessions = %#v", stale)
	}
	server.terminalMu.Lock()
	defer server.terminalMu.Unlock()
	if _, ok := server.terminalSessions["active"]; !ok {
		t.Fatal("active terminal session was pruned")
	}
}

func TestReserveTerminalOpenIncludesConcurrentRequests(t *testing.T) {
	server, _ := newTestServer(t)
	for index := 0; index < maxPanelTerminalsByUser; index++ {
		if !server.reserveTerminalOpen("user") {
			t.Fatalf("reservation %d was unexpectedly rejected", index)
		}
	}
	if server.reserveTerminalOpen("user") {
		t.Fatal("concurrent per-user limit was not enforced")
	}
	if !server.reserveTerminalOpen("another-user") {
		t.Fatal("another user should retain an independent slot")
	}
	server.releaseTerminalOpen("user")
	if !server.reserveTerminalOpen("user") {
		t.Fatal("released slot was not reusable")
	}
}
