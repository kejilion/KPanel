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

type browseWSAgentStub struct {
	mu     sync.Mutex
	calls  []agentCall
	opened int
	inputs []browseWSAgentInputInput
	closed int

	sessionID string

	outputs     []browseWSAgentOutputOutput
	outputIndex int
}

func (s *browseWSAgentStub) Get(_ context.Context, path, rawQuery, _ string) (AgentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, agentCall{method: http.MethodGet, path: path, rawQuery: rawQuery})
	if !strings.HasSuffix(path, "/output") {
		return AgentResponse{StatusCode: http.StatusNotFound, ContentType: "application/json"}, nil
	}
	var out browseWSAgentOutputOutput
	if s.outputIndex < len(s.outputs) {
		out = s.outputs[s.outputIndex]
		s.outputIndex++
	}
	body, _ := json.Marshal(out)
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
}

func (s *browseWSAgentStub) Do(_ context.Context, method, path, _ string, _ string, body []byte) (AgentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, agentCall{method: method, path: path, body: body})
	switch {
	case method == http.MethodPost && path == "/v1/browse/ws":
		s.opened++
		id := s.sessionID
		if id == "" {
			id = "backend-ws-1"
		}
		payload, _ := json.Marshal(map[string]string{"sessionId": id})
		return AgentResponse{StatusCode: http.StatusCreated, ContentType: "application/json", Body: payload}, nil
	case strings.HasSuffix(path, "/input"):
		var input browseWSAgentInputInput
		_ = json.Unmarshal(body, &input)
		s.inputs = append(s.inputs, input)
		return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"accepted":true}`)}, nil
	case strings.HasSuffix(path, "/close"):
		s.closed++
		return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"closed":true}`)}, nil
	default:
		return AgentResponse{StatusCode: http.StatusNotFound, ContentType: "application/json"}, nil
	}
}

func TestHandleBrowseWSSessionRejectsUnauthenticated(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	body, _ := json.Marshal(browseWSOpenRequest{URL: "wss://example.com/"})
	response := browsePerformRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions", body, map[string]string{
		"Content-Type": "application/json", "Origin": browseTestOrigin,
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseWSSessionRejectsMissingCSRF(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseWSOpenRequest{URL: "wss://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin,
			// X-CSRF-Token intentionally omitted.
		})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseWSSessionUnknownHostRejected(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseWSOpenRequest{HostID: "does-not-exist", URL: "wss://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseWSSessionRejectsEmptyURL(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseWSOpenRequest{HostID: "local"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestLocalBrowseWSSessionLifecycle(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	stub := &browseWSAgentStub{
		outputs: []browseWSAgentOutputOutput{
			{
				Messages:   []browseWSAgentMessage{{Type: "text", Data: base64.StdEncoding.EncodeToString([]byte("hello"))}},
				NextOffset: 1, Closed: false,
			},
		},
	}
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
	}

	opened := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions",
		[]byte(`{"hostId":"local","url":"wss://example.com/socket"}`), sessionCookie, csrfCookie, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	var result browseWSOpenResponse
	if err := json.Unmarshal(opened.Body.Bytes(), &result); err != nil || result.SessionID == "" {
		t.Fatalf("decode open: result=%#v err=%v", result, err)
	}
	if result.HostID != "local" {
		t.Fatalf("hostId = %q, want local", result.HostID)
	}

	output := browseAuthenticatedRequest(server, http.MethodGet,
		"/api/v1/browse/ws-sessions/"+result.SessionID+"/output?offset=0&wait=0", nil, sessionCookie, csrfCookie, nil)
	if output.Code != http.StatusOK || !strings.Contains(output.Body.String(), base64.StdEncoding.EncodeToString([]byte("hello"))) {
		t.Fatalf("output = %d %s", output.Code, output.Body.String())
	}
	invalidOutputQuery := browseAuthenticatedRequest(server, http.MethodGet,
		"/api/v1/browse/ws-sessions/"+result.SessionID+"/output?offset=0&wait=0&extra=1", nil, sessionCookie, csrfCookie, nil)
	if invalidOutputQuery.Code != http.StatusBadRequest {
		t.Fatalf("output with extra query = %d %s", invalidOutputQuery.Code, invalidOutputQuery.Body.String())
	}

	inputPayload := base64.StdEncoding.EncodeToString([]byte("ping"))
	input := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions/"+result.SessionID+"/input",
		[]byte(`{"type":"text","data":"`+inputPayload+`"}`), sessionCookie, csrfCookie, headers)
	if input.Code != http.StatusOK {
		t.Fatalf("input = %d %s", input.Code, input.Body.String())
	}

	closed := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions/"+result.SessionID+"/close",
		[]byte(`{}`), sessionCookie, csrfCookie, headers)
	if closed.Code != http.StatusOK {
		t.Fatalf("close = %d %s", closed.Code, closed.Body.String())
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.opened != 1 || stub.closed != 1 || len(stub.inputs) != 1 {
		t.Fatalf("unexpected Agent calls: opened=%d closed=%d inputs=%d", stub.opened, stub.closed, len(stub.inputs))
	}
	if stub.inputs[0].Type != "text" {
		t.Fatalf("input type = %q", stub.inputs[0].Type)
	}
	decoded, err := base64.StdEncoding.DecodeString(stub.inputs[0].Data)
	if err != nil || string(decoded) != "ping" {
		t.Fatalf("input data mismatch: %q err=%v", decoded, err)
	}
	if stub.inputs[0].Owner == "" {
		t.Fatal("owner was not forwarded to the Agent")
	}

	server.browseWSMu.Lock()
	_, exists := server.browseWSSessions[result.SessionID]
	server.browseWSMu.Unlock()
	if exists {
		t.Fatal("closed browse ws session left a stale Panel session")
	}
}

// TestBrowseWSSessionOutputClosedRemovesPanelSession confirms the Panel-side
// bookkeeping entry is cleaned up the moment the backend reports Closed via
// a poll response — mirroring outputTerminalBackend's deleteTerminalSession
// on Closed/ExitedAt — without requiring an explicit client Close call.
func TestBrowseWSSessionOutputClosedRemovesPanelSession(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	stub := &browseWSAgentStub{
		outputs: []browseWSAgentOutputOutput{
			{Closed: true, CloseReason: "target closed"},
		},
	}
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
	}
	opened := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/ws-sessions",
		[]byte(`{"hostId":"local","url":"wss://example.com/socket"}`), sessionCookie, csrfCookie, headers)
	var result browseWSOpenResponse
	if err := json.Unmarshal(opened.Body.Bytes(), &result); err != nil || result.SessionID == "" {
		t.Fatalf("decode open: result=%#v err=%v", result, err)
	}

	output := browseAuthenticatedRequest(server, http.MethodGet,
		"/api/v1/browse/ws-sessions/"+result.SessionID+"/output?offset=0&wait=0", nil, sessionCookie, csrfCookie, nil)
	if output.Code != http.StatusOK || !strings.Contains(output.Body.String(), "target closed") {
		t.Fatalf("output = %d %s", output.Code, output.Body.String())
	}
	server.browseWSMu.Lock()
	_, exists := server.browseWSSessions[result.SessionID]
	server.browseWSMu.Unlock()
	if exists {
		t.Fatal("session reported Closed by the backend but the Panel-side entry survived")
	}
}

func TestBrowseWSSessionIsBoundToAuthenticatedUser(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	server.browseWSMu.Lock()
	server.browseWSSessions["private"] = panelBrowseWSSession{ID: "private", UserID: "another-user", HostID: "local"}
	server.browseWSMu.Unlock()
	response := browseAuthenticatedRequest(
		server, http.MethodGet,
		"/api/v1/browse/ws-sessions/private/output?offset=0&wait=0", nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "another-user") {
		t.Fatalf("cross-user lookup = %d %s", response.Code, response.Body.String())
	}
}

func TestPruneBrowseWSSessionsRemovesOnlyExpiredEntries(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	now := time.Now().UTC()
	server.browseWSMu.Lock()
	server.browseWSSessions["stale"] = panelBrowseWSSession{ID: "stale", UserID: "user", UpdatedAt: now.Add(-time.Hour)}
	server.browseWSSessions["active"] = panelBrowseWSSession{ID: "active", UserID: "user", UpdatedAt: now}
	server.browseWSMu.Unlock()

	stale := server.pruneBrowseWSSessions(now.Add(-panelBrowseWSIdleTTL))
	if len(stale) != 1 || stale[0].ID != "stale" {
		t.Fatalf("pruned sessions = %#v", stale)
	}
	server.browseWSMu.Lock()
	defer server.browseWSMu.Unlock()
	if _, ok := server.browseWSSessions["active"]; !ok {
		t.Fatal("active browse ws session was pruned")
	}
}

func TestReserveBrowseWSOpenIncludesConcurrentRequests(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	for index := 0; index < maxPanelBrowseWSSessionsByUser; index++ {
		if !server.reserveBrowseWSOpen("user") {
			t.Fatalf("reservation %d was unexpectedly rejected", index)
		}
	}
	if server.reserveBrowseWSOpen("user") {
		t.Fatal("concurrent per-user limit was not enforced")
	}
	if !server.reserveBrowseWSOpen("another-user") {
		t.Fatal("another user should retain an independent slot")
	}
	server.releaseBrowseWSOpen("user")
	if !server.reserveBrowseWSOpen("user") {
		t.Fatal("released slot was not reusable")
	}
}
