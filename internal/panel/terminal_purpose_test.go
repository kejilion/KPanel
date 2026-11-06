package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSiteManagementTerminalRestoresAndLaunchesOnce(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained", true: "legacy"}[legacy], func(t *testing.T) {
			s, path := newTestServer(t)
			suspendTestTerminalCleanup(s)
			stub := &retainedAgentStub{strictLegacy: legacy}
			s.agent = stub
			cookie, csrf := bootstrapCookies(t, s, path)
			call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
				body, _ := json.Marshal(input)
				return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
			}
			input := terminalOpenRequest{HostID: "local", Rows: 30, Columns: 120, Purpose: "site-management", PrepareOnly: true}
			var intent struct {
				LaunchID string `json:"launchId"`
			}
			prepared := call(input)
			_ = json.Unmarshal(prepared.Body.Bytes(), &intent)
			if prepared.Code != 201 || !validTerminalLaunchID(intent.LaunchID) || stub.opened != 0 {
				t.Fatalf("prepare: %d %s", prepared.Code, prepared.Body)
			}
			input.PrepareOnly, input.Retained, input.LaunchID = false, true, intent.LaunchID
			if legacy {
				rejected := call(input)
				if rejected.Code != 409 || !strings.Contains(rejected.Body.String(), "terminal_retention_unavailable") || stub.opened != 0 || len(stub.inputs) != 0 {
					t.Fatalf("pre-create rejection: %d %s", rejected.Code, rejected.Body)
				}
				input.Retained = false
			}
			var wg sync.WaitGroup
			responses := make(chan *httptest.ResponseRecorder, 2)
			for range 2 {
				wg.Add(1)
				go func() { defer wg.Done(); responses <- call(input) }()
			}
			wg.Wait()
			close(responses)
			var session terminalOpenResponse
			for response := range responses {
				var next terminalOpenResponse
				_ = json.Unmarshal(response.Body.Bytes(), &next)
				if response.Code != 200 && response.Code != 201 || next.Purpose != input.Purpose || next.SessionID == "" || session.SessionID != "" && next.SessionID != session.SessionID {
					t.Fatalf("restore: %d %s", response.Code, response.Body)
				}
				session = next
			}
			if stub.opened != 1 || len(stub.inputs) != 1 || string(stub.inputs[0]) != "k web\n" || session.Retained == legacy {
				t.Fatalf("duplicate or wrong command: opened=%d inputs=%q session=%+v", stub.opened, stub.inputs, session)
			}
			listed := authenticatedRequest(s, http.MethodGet, "/api/v1/terminal-sessions", nil, cookie, csrf, nil)
			if strings.Contains(listed.Body.String(), `"purpose":"site-management"`) == legacy {
				t.Fatalf("retained list: %s", listed.Body)
			}
			stub.failClose = true
			closed := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/close", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
			if closed.Code != 502 || call(input).Code != 409 || stub.opened != 1 {
				t.Fatal("failed close allowed a replacement")
			}
			stub.failClose = false
			closed = authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/close", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
			if closed.Code != 200 || call(input).Code != 409 || stub.opened != 1 {
				t.Fatal("consumed launch intent replayed after close")
			}
		})
	}
}

func TestSiteManagementTerminalRejectsInvalidTargetsAndFencesCancellation(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
	}
	input := terminalOpenRequest{HostID: "local", Rows: 30, Columns: 120, Purpose: "site-management", PrepareOnly: true}
	for _, change := range []func(*terminalOpenRequest){
		func(in *terminalOpenRequest) { in.Purpose = "shell" },
		func(in *terminalOpenRequest) { in.HostID = strings.Repeat("a", 32) },
		func(in *terminalOpenRequest) { in.ShortcutID = panelTestShortcutID },
		func(in *terminalOpenRequest) { in.ShortcutVersion = "stale" },
		func(in *terminalOpenRequest) { in.ContainerID, in.ResourceVersion = strings.Repeat("a", 64), "v1" },
		func(in *terminalOpenRequest) { in.PrepareOnly = false },
	} {
		bad := input
		change(&bad)
		if got := call(bad); got.Code != 422 {
			t.Fatalf("invalid target accepted: %+v => %d %s", bad, got.Code, got.Body)
		}
	}
	body, _ := json.Marshal(input)
	for _, headers := range []map[string]string{nil, {"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrf.Value}} {
		if got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, headers); got.Code != 403 {
			t.Fatalf("unguarded purpose: %d", got.Code)
		}
	}
	var intent struct {
		LaunchID string `json:"launchId"`
	}
	prepared := call(input)
	_ = json.Unmarshal(prepared.Body.Bytes(), &intent)
	input.PrepareOnly, input.LookupOnly, input.LaunchID = false, true, intent.LaunchID
	if got := call(input); got.Code != 404 {
		t.Fatalf("cancel: %d %s", got.Code, got.Body)
	}
	input.LookupOnly = false
	if got := call(input); got.Code != 409 || stub.opened != 0 || len(stub.inputs) != 0 {
		t.Fatalf("late launch escaped cancellation: %d", got.Code)
	}
}

func TestSiteManagementUncertainLaunchCannotReplayOrCrossOwner(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{failInput: true}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
	}
	input := terminalOpenRequest{HostID: "local", Rows: 30, Columns: 120, Purpose: "site-management", PrepareOnly: true}
	var intent struct {
		LaunchID string `json:"launchId"`
	}
	_ = json.Unmarshal(call(input).Body.Bytes(), &intent)
	input.PrepareOnly, input.Retained, input.LaunchID = false, true, intent.LaunchID
	var session terminalOpenResponse
	_ = json.Unmarshal(call(input).Body.Bytes(), &session)
	stub.failInput = false
	if got := call(input); got.Code != 200 || !session.LaunchUncertain || stub.opened != 1 || len(stub.inputs) != 0 {
		t.Fatalf("uncertain launch replayed: %d", got.Code)
	}
	blocked := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/input", []byte(`{"data":"eAo"}`), cookie, csrf, retainedHeaders(csrf))
	if blocked.Code != 409 {
		t.Fatalf("uncertain input not frozen: %d", blocked.Code)
	}
	s.terminalMu.Lock()
	item := s.terminalSessions[session.SessionID]
	item.UserID = "another-owner"
	s.terminalSessions[session.SessionID] = item
	s.terminalMu.Unlock()
	input.LookupOnly = true
	if got := call(input); got.Code != 404 {
		t.Fatalf("cross-owner lookup: %d", got.Code)
	}
}
