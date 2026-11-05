package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type retainedAgentStub struct {
	terminalAgentStub
	unsupported  bool
	strictLegacy bool
	failInput    bool
	failClose    bool
	beforeInput  func()
}

type announcedTerminalBody struct {
	io.Reader
	entered chan struct{}
	once    sync.Once
}

func (b *announcedTerminalBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	return b.Reader.Read(p)
}

func TestShortcutCloseFencesLaunchThatArrivesAfterLookup(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "temporary", true: "retained"}[retained], func(t *testing.T) {
			s, path := newTestServer(t)
			stub := &retainedAgentStub{}
			s.agent = stub
			cookie, csrf := bootstrapCookies(t, s, path)
			version := saveRetainedShortcut(t, s)
			call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
				body, _ := json.Marshal(input)
				return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
			}
			input := terminalOpenRequest{HostID: "local", Rows: 24, Columns: 80, ShortcutID: panelTestShortcutID, PrepareOnly: true}
			var intent struct {
				LaunchID string `json:"launchId"`
			}
			prepared := call(input)
			_ = json.Unmarshal(prepared.Body.Bytes(), &intent)
			if prepared.Code != 201 || !validTerminalLaunchID(intent.LaunchID) || stub.opened != 0 {
				t.Fatalf("prepare: %d %s", prepared.Code, prepared.Body)
			}
			input.PrepareOnly, input.Retained, input.ShortcutVersion, input.LaunchID = false, retained, version, intent.LaunchID
			original, _ := json.Marshal(input)
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			entered := make(chan struct{})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/terminal-sessions", &announcedTerminalBody{Reader: reader, entered: entered})
			request.Host = "panel.test"
			request.AddCookie(cookie)
			request.AddCookie(csrf)
			for key, value := range retainedHeaders(csrf) {
				request.Header.Set(key, value)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { result := httptest.NewRecorder(); s.ServeHTTP(result, request); done <- result }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("original request did not enter decode")
			}
			lookup := input
			lookup.LookupOnly = true
			if got := call(lookup); got.Code != 404 {
				t.Fatalf("close lookup: %d %s", got.Code, got.Body)
			}
			_, _ = writer.Write(original)
			_ = writer.Close()
			select {
			case got := <-done:
				if got.Code != 409 || !strings.Contains(got.Body.String(), "terminal_launch_intent_invalid") || stub.opened != 0 || len(stub.inputs) != 0 {
					t.Fatalf("late launch escaped close fence: %d %s opened=%d", got.Code, got.Body, stub.opened)
				}
			case <-time.After(time.Second):
				t.Fatal("late launch did not finish")
			}
			// A fresh user action can launch, but replaying its consumed intent
			// after confirmed close must not create another process.
			prepare := input
			prepare.PrepareOnly, prepare.LaunchID = true, ""
			fresh := call(prepare)
			_ = json.Unmarshal(fresh.Body.Bytes(), &intent)
			input.LaunchID = intent.LaunchID
			opened := call(input)
			var session terminalOpenResponse
			_ = json.Unmarshal(opened.Body.Bytes(), &session)
			if fresh.Code != 201 || opened.Code != 201 {
				t.Fatalf("fresh launch: %d %d", fresh.Code, opened.Code)
			}
			closed := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/close", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
			if closed.Code != 200 {
				t.Fatalf("close: %d", closed.Code)
			}
			if replay := call(input); replay.Code != 409 || stub.opened != 1 || len(stub.inputs) != 1 {
				t.Fatalf("consumed intent replayed: %d opened=%d", replay.Code, stub.opened)
			}
		})
	}
}

func TestShortcutOldCloseIdentityCannotResolveReplacement(t *testing.T) {
	s, path := newTestServer(t)
	s.agent = &retainedAgentStub{}
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
	}
	input := terminalOpenRequest{HostID: "local", Rows: 24, Columns: 80, ShortcutID: panelTestShortcutID, PrepareOnly: true}
	prepare := func() string {
		var intent struct {
			LaunchID string `json:"launchId"`
		}
		response := call(input)
		if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &intent) != nil {
			t.Fatal(response.Body.String())
		}
		return intent.LaunchID
	}
	oldID := prepare()
	request := input
	request.PrepareOnly, request.Retained, request.ShortcutVersion, request.LaunchID = false, true, version, oldID
	opened := call(request)
	var old terminalOpenResponse
	if opened.Code != 201 || json.Unmarshal(opened.Body.Bytes(), &old) != nil {
		t.Fatal(opened.Body.String())
	}
	for index := 1; index <= maxPanelTerminals; index++ {
		request.LaunchID = prepare()
		resumed := call(request)
		if index < maxPanelTerminals && resumed.Code != 200 || index == maxPanelTerminals && resumed.Code != 429 {
			t.Fatalf("resume %d: %d %s", index, resumed.Code, resumed.Body)
		}
	}
	request.LookupOnly, request.LaunchID = true, oldID
	if response := call(request); response.Code != 200 {
		t.Fatalf("old identity was evicted: %d %s", response.Code, response.Body)
	}
	request.LookupOnly = false
	closed := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+old.SessionID+"/close", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
	if closed.Code != 200 {
		t.Fatal(closed.Body.String())
	}
	newID := prepare()
	request.LaunchID = newID
	if opened = call(request); opened.Code != 201 {
		t.Fatal(opened.Body.String())
	}
	request.LookupOnly, request.LaunchID = true, oldID
	if response := call(request); response.Code != 404 {
		t.Fatalf("old identity resolved replacement: %d %s", response.Code, response.Body)
	}
	request.LaunchID = newID
	if response := call(request); response.Code != 200 {
		t.Fatalf("new identity did not resolve: %d %s", response.Code, response.Body)
	}
}

func TestShortcutLaunchIntentsAreBoundedExpireAndCancel(t *testing.T) {
	s, path := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, s, path)
	input := terminalOpenRequest{HostID: "local", Rows: 24, Columns: 80, ShortcutID: panelTestShortcutID, PrepareOnly: true}
	call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
	}
	var first struct {
		LaunchID string `json:"launchId"`
	}
	for i := range maxPanelTerminalsByUser {
		got := call(input)
		if got.Code != 201 {
			t.Fatalf("prepare %d: %d", i, got.Code)
		}
		if i == 0 {
			_ = json.Unmarshal(got.Body.Bytes(), &first)
		}
	}
	if got := call(input); got.Code != 429 {
		t.Fatalf("unbounded intents: %d", got.Code)
	}
	cancel := input
	cancel.PrepareOnly, cancel.LookupOnly, cancel.LaunchID = false, true, first.LaunchID
	if got := call(cancel); got.Code != 404 {
		t.Fatalf("cancel: %d", got.Code)
	}
	if got := call(input); got.Code != 201 {
		t.Fatalf("cancel did not free capacity: %d", got.Code)
	}
	s.terminalLaunchMu.Lock()
	for id, intent := range s.terminalLaunchIntents {
		intent.ExpiresAt = time.Now().Add(-time.Second)
		s.terminalLaunchIntents[id] = intent
	}
	s.terminalLaunchMu.Unlock()
	if got := call(input); got.Code != 201 || len(s.terminalLaunchIntents) != 1 {
		t.Fatalf("expiry: %d count=%d", got.Code, len(s.terminalLaunchIntents))
	}
}

func TestShortcutLaunchIntentCannotCrossIdentityOrExpiry(t *testing.T) {
	for _, mismatch := range []string{"user", "host", "shortcut", "expiry"} {
		t.Run(mismatch, func(t *testing.T) {
			s, path := newTestServer(t)
			stub := &retainedAgentStub{}
			s.agent = stub
			cookie, csrf := bootstrapCookies(t, s, path)
			version := saveRetainedShortcut(t, s)
			call := func(input terminalOpenRequest) *httptest.ResponseRecorder {
				body, _ := json.Marshal(input)
				return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
			}
			input := terminalOpenRequest{HostID: "local", Rows: 24, Columns: 80, ShortcutID: panelTestShortcutID, PrepareOnly: true}
			var prepared struct {
				LaunchID string `json:"launchId"`
			}
			got := call(input)
			_ = json.Unmarshal(got.Body.Bytes(), &prepared)
			if got.Code != 201 {
				t.Fatalf("prepare: %d", got.Code)
			}
			input.PrepareOnly, input.Retained, input.ShortcutVersion, input.LaunchID = false, true, version, prepared.LaunchID
			bad := input
			s.terminalLaunchMu.Lock()
			original := s.terminalLaunchIntents[prepared.LaunchID]
			changed := original
			switch mismatch {
			case "user":
				changed.UserID = "other-user"
			case "host":
				bad.HostID = "other-host"
			case "shortcut":
				bad.ShortcutID = strings.Repeat("c", 32)
			case "expiry":
				changed.ExpiresAt = time.Now().Add(-time.Second)
			}
			s.terminalLaunchIntents[prepared.LaunchID] = changed
			s.terminalLaunchMu.Unlock()
			if mismatch != "expiry" {
				lookup := bad
				lookup.LookupOnly = true
				if got := call(lookup); got.Code != 404 {
					t.Fatalf("foreign lookup: %d", got.Code)
				}
			}
			if got := call(bad); got.Code != 409 || stub.opened != 0 || len(stub.inputs) != 0 {
				t.Fatalf("invalid intent launched: %d opened=%d", got.Code, stub.opened)
			}
			s.terminalLaunchMu.Lock()
			_, exists := s.terminalLaunchIntents[prepared.LaunchID]
			s.terminalLaunchIntents[prepared.LaunchID] = original
			s.terminalLaunchMu.Unlock()
			if !exists {
				t.Fatal("foreign lookup consumed another identity's intent")
			}
			if got := call(input); got.Code != 201 || stub.opened != 1 || len(stub.inputs) != 1 {
				t.Fatalf("valid intent was corrupted: %d opened=%d", got.Code, stub.opened)
			}
		})
	}
}

func (a *retainedAgentStub) Do(ctx context.Context, method, path, query, id string, body []byte) (AgentResponse, error) {
	if path == "/v1/terminals/capabilities/input-protocol" {
		if a.strictLegacy {
			return AgentResponse{StatusCode: http.StatusNotFound}, nil
		}
		data, _ := json.Marshal(map[string]any{"protocol": terminal.InputProtocol, "retained": !a.unsupported})
		return AgentResponse{StatusCode: http.StatusOK, Body: data}, nil
	}
	if path == "/v1/terminals" && a.strictLegacy && strings.Contains(string(body), `"retained"`) {
		return AgentResponse{StatusCode: 400, Body: []byte(`{"code":"invalid_request","title":"Unknown field"}`)}, nil
	}
	if strings.HasSuffix(path, "/input") && a.beforeInput != nil {
		a.beforeInput()
	}
	if strings.HasSuffix(path, "/input") && a.failInput {
		return AgentResponse{}, errors.New("write outcome unknown")
	}
	if strings.HasSuffix(path, "/close") && a.failClose {
		return AgentResponse{}, errors.New("node offline")
	}
	response, err := a.terminalAgentStub.Do(ctx, method, path, query, id, body)
	if path == "/v1/terminals" && err == nil && !a.unsupported {
		var input struct {
			Retained bool `json:"retained"`
		}
		_ = json.Unmarshal(body, &input)
		var payload map[string]any
		_ = json.Unmarshal(response.Body, &payload)
		payload["retained"] = input.Retained
		response.Body, _ = json.Marshal(payload)
	}
	return response, err
}

func retainedHeaders(csrf *http.Cookie) map[string]string {
	return map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
}
func saveRetainedShortcut(t *testing.T, s *Server) string {
	t.Helper()
	state := s.desktopWorkspace.Workspace()
	saved, err := s.desktopWorkspace.Replace(desktopworkspace.ReplaceInput{ExpectedResourceVersion: state.ResourceVersion, Shortcuts: []desktopworkspace.ShortcutInput{{ID: panelTestShortcutID, Name: "Test command", TargetType: desktopworkspace.ShortcutTargetCommand, Command: "printf secret-test-command"}}})
	if err != nil {
		t.Fatal(err)
	}
	return saved.ResourceVersion
}
func retainedShortcutBody(version string) []byte {
	data, _ := json.Marshal(map[string]any{"hostId": "local", "rows": 24, "columns": 80, "retained": true, "shortcutId": panelTestShortcutID, "shortcutVersion": version})
	return data
}
func TestRetainedTerminalListIsAuthenticatedOwnedAndDoesNotExtendLifetime(t *testing.T) {
	s, path := newTestServer(t)
	s.agent = &retainedAgentStub{}
	cookie, csrf := bootstrapCookies(t, s, path)
	if got := performRequest(s, http.MethodGet, "/api/v1/terminal-sessions", nil, nil); got.Code != 401 {
		t.Fatalf("unauthenticated list: %d", got.Code)
	}
	opened := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":24,"columns":80,"retained":true}`), cookie, csrf, retainedHeaders(csrf))
	if opened.Code != 201 {
		t.Fatalf("open: %d %s", opened.Code, opened.Body)
	}
	var item terminalOpenResponse
	_ = json.Unmarshal(opened.Body.Bytes(), &item)
	if !item.Retained || item.ExpiresAt == nil {
		t.Fatalf("missing retention: %+v", item)
	}
	s.terminalMu.Lock()
	original := s.terminalSessions[item.SessionID]
	s.terminalSessions["foreign"] = panelTerminalSession{ID: "foreign", UserID: "another", Retained: true}
	s.terminalSessions["temporary"] = panelTerminalSession{ID: "temporary", UserID: original.UserID}
	s.terminalMu.Unlock()
	got := authenticatedRequest(s, http.MethodGet, "/api/v1/terminal-sessions", nil, cookie, csrf, nil)
	var listed struct {
		Items []terminalOpenResponse `json:"items"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &listed)
	if got.Code != 200 || len(listed.Items) != 1 || listed.Items[0].SessionID != item.SessionID {
		t.Fatalf("list: %d %s", got.Code, got.Body)
	}
	if strings.Contains(got.Body.String(), "backend-terminal") || strings.Contains(got.Body.String(), "panel:") {
		t.Fatal("backend identity leaked")
	}
	s.terminalMu.Lock()
	updated := s.terminalSessions[item.SessionID].UpdatedAt
	s.terminalMu.Unlock()
	if !updated.Equal(original.UpdatedAt) {
		t.Fatal("listing extended lifetime")
	}
	bad := authenticatedRequest(s, http.MethodGet, "/api/v1/terminal-sessions?owner=another", nil, cookie, csrf, nil)
	if bad.Code != 400 {
		t.Fatalf("query: %d", bad.Code)
	}
}
func TestRetainedShortcutConcurrentRefreshExecutesOnce(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
			if got.Code != 200 && got.Code != 201 {
				t.Errorf("open: %d %s", got.Code, got.Body)
				return
			}
			var result terminalOpenResponse
			_ = json.Unmarshal(got.Body.Bytes(), &result)
			ids <- result.SessionID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for next := range ids {
		if id != "" && next != id {
			t.Fatal("refresh created a duplicate")
		}
		id = next
	}
	if stub.opened != 1 || len(stub.inputs) != 1 || string(stub.inputs[0]) != "printf secret-test-command\n" {
		t.Fatalf("opened=%d inputs=%q", stub.opened, stub.inputs)
	}
	got := authenticatedRequest(s, http.MethodGet, "/api/v1/terminal-sessions", nil, cookie, csrf, nil)
	if strings.Contains(got.Body.String(), "secret-test-command") {
		t.Fatal("command leaked into registry response")
	}
	events, _, _ := s.store.ListAudit(20, "")
	audit, _ := json.Marshal(events)
	if strings.Contains(string(audit), "secret-test-command") {
		t.Fatal("command leaked into audit")
	}
}
func TestRetainedShortcutValidationAndUncertainLaunchNeverReplay(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{failInput: true}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	for _, headers := range []map[string]string{nil, {"Content-Type": "application/json", "Origin": "http://panel.test"}} {
		got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, headers)
		if got.Code != 403 {
			t.Fatalf("unguarded open: %d", got.Code)
		}
	}
	got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody("stale"), cookie, csrf, retainedHeaders(csrf))
	if got.Code != 409 || stub.opened != 0 {
		t.Fatalf("stale shortcut: %d opened%d", got.Code, stub.opened)
	}
	got = authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	var item terminalOpenResponse
	_ = json.Unmarshal(got.Body.Bytes(), &item)
	if got.Code != 201 || !item.LaunchUncertain {
		t.Fatalf("uncertain: %d %s", got.Code, got.Body)
	}
	if !item.Detachable || !item.WorkspaceVisible {
		t.Fatal("named terminal must support cleanup without losing its workspace identity")
	}
	stub.failInput = false
	retry := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	if retry.Code != 200 || stub.opened != 1 || len(stub.inputs) != 0 {
		t.Fatalf("uncertain replayed: %d opened%d inputs%d", retry.Code, stub.opened, len(stub.inputs))
	}
	input := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+item.SessionID+"/input", []byte(`{"data":"eAo"}`), cookie, csrf, retainedHeaders(csrf))
	if input.Code != 409 {
		t.Fatalf("uncertain input not fenced: %d", input.Code)
	}
	stub.failClose = true
	closed := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+item.SessionID+"/close", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
	if closed.Code != 502 {
		t.Fatalf("close: %d", closed.Code)
	}
	detached := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+item.SessionID+"/detach", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
	if detached.Code != 200 {
		t.Fatalf("detach: %d %s", detached.Code, detached.Body.String())
	}
	retry = authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	if retry.Code != 409 || stub.opened != 1 {
		t.Fatal("failed close replaced a live session")
	}
	s.terminalMu.Lock()
	done := s.terminalCleanupDone
	s.terminalMu.Unlock()
	if done != nil {
		<-done
	}
}
func TestRetainedAgentNegotiationRejectsOldAgent(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{unsupported: true}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":24,"columns":80,"retained":true}`), cookie, csrf, retainedHeaders(csrf))
	if got.Code != 409 || !strings.Contains(got.Body.String(), "terminal_retention_unavailable") || stub.opened != 0 || stub.closed != 0 || len(s.terminalSessions) != 0 {
		t.Fatalf("old Agent silently downgraded: %d closed%d", got.Code, stub.closed)
	}
}

func TestTemporaryShortcutUsesSavedCommandAndReusesUncertainLaunch(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{unsupported: true, failInput: true}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	body := []byte(strings.Replace(string(retainedShortcutBody(version)), `"retained":true`, `"retained":false`, 1))
	first := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
	var opened terminalOpenResponse
	_ = json.Unmarshal(first.Body.Bytes(), &opened)
	if first.Code != 201 || opened.Retained || !opened.LaunchUncertain {
		t.Fatalf("temporary launch: %d %s", first.Code, first.Body)
	}
	stub.failInput = false
	// Switching back to background mode must resume even a temporary session.
	retry := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	var resumed terminalOpenResponse
	_ = json.Unmarshal(retry.Body.Bytes(), &resumed)
	if retry.Code != 200 || resumed.SessionID != opened.SessionID || stub.opened != 1 || len(stub.inputs) != 0 {
		t.Fatalf("temporary launch replayed: %d %s", retry.Code, retry.Body)
	}
}

func TestStrictLegacyAgentExplicitTemporaryShortcutExecutesSavedCommandOnce(t *testing.T) {
	s, path := newTestServer(t)
	stub := &retainedAgentStub{strictLegacy: true, unsupported: true}
	s.agent = stub
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	rejected := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	if rejected.Code != 409 || !strings.Contains(rejected.Body.String(), "terminal_retention_unavailable") || stub.opened != 0 || len(stub.inputs) != 0 {
		t.Fatalf("strict legacy rejection: %d opened=%d inputs=%d", rejected.Code, stub.opened, len(stub.inputs))
	}
	body := []byte(strings.Replace(string(retainedShortcutBody(version)), `"retained":true`, `"retained":false`, 1))
	for i := range 2 {
		got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
		if got.Code != []int{201, 200}[i] {
			t.Fatalf("temporary shortcut: %d %s", got.Code, got.Body)
		}
	}
	if stub.opened != 1 || len(stub.inputs) != 1 || string(stub.inputs[0]) != "printf secret-test-command\n" {
		t.Fatalf("temporary command opened=%d inputs=%q", stub.opened, stub.inputs)
	}
}

func TestShortcutCloseLookupNeverLaunchesAndRecoversOnlyOwnedIdentity(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "temporary", true: "retained"}[retained], func(t *testing.T) {
			s, path := newTestServer(t)
			stub := &retainedAgentStub{}
			s.agent = stub
			cookie, csrf := bootstrapCookies(t, s, path)
			lookup, _ := json.Marshal(terminalOpenRequest{HostID: "local", Rows: 30, Columns: 120, ShortcutID: panelTestShortcutID, LookupOnly: true})
			call := func(body []byte) *httptest.ResponseRecorder {
				return authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, cookie, csrf, retainedHeaders(csrf))
			}
			if got := call(lookup); got.Code != 404 || stub.opened != 0 {
				t.Fatalf("lookup spawned: %d opened=%d", got.Code, stub.opened)
			}
			version := saveRetainedShortcut(t, s)
			body := retainedShortcutBody(version)
			if !retained {
				body = []byte(strings.Replace(string(body), `"retained":true`, `"retained":false`, 1))
			}
			var opened terminalOpenResponse
			first := call(body)
			_ = json.Unmarshal(first.Body.Bytes(), &opened)
			if first.Code != 201 {
				t.Fatalf("launch: %d %s", first.Code, first.Body)
			}
			s.terminalMu.Lock()
			item := s.terminalSessions[opened.SessionID]
			item.CloseRequested = true
			item.CreatedAt = time.Now().Add(-terminal.DefaultLifetime - time.Minute)
			s.terminalSessions[item.ID] = item
			s.terminalMu.Unlock()
			var recovered terminalOpenResponse
			got := call(lookup)
			_ = json.Unmarshal(got.Body.Bytes(), &recovered)
			if got.Code != 200 || recovered.SessionID != opened.SessionID || !recovered.CloseRequested || stub.opened != 1 || len(stub.inputs) != 1 {
				t.Fatalf("lookup failed or replayed: %d %s", got.Code, got.Body)
			}
			changedHost := []byte(strings.Replace(string(lookup), `"hostId":"local"`, `"hostId":"another-host"`, 1))
			if got := call(changedHost); got.Code != 409 {
				t.Fatalf("host mismatch: %d", got.Code)
			}
			s.terminalMu.Lock()
			item.UserID = "another-user"
			s.terminalSessions[item.ID] = item
			s.terminalMu.Unlock()
			if got := call(lookup); got.Code != 404 || strings.Contains(got.Body.String(), opened.SessionID) {
				t.Fatalf("foreign identity leaked: %d %s", got.Code, got.Body)
			}
		})
	}
}
func TestRetainedRegistrySkipsIdleButKeepsUnconfirmedExpiry(t *testing.T) {
	s, _ := newTestServer(t)
	now := time.Now().UTC()
	s.terminalSessions["retained"] = panelTerminalSession{ID: "retained", Retained: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	if got := s.pruneTerminalSessions(now.Add(-panelTerminalIdleTTL)); len(got) != 0 {
		t.Fatal("background session idle-reaped")
	}
	s.terminalSessions["expired"] = panelTerminalSession{ID: "expired", Retained: true, CreatedAt: now.Add(-terminal.DefaultLifetime - time.Minute), UpdatedAt: now}
	got := s.pruneTerminalSessions(now.Add(-panelTerminalIdleTTL))
	if len(got) != 1 || !s.terminalSessions["expired"].CloseRequested {
		t.Fatal("expiry lost unconfirmed cleanup identity")
	}
}

func TestRetainedShortcutInputFrozenUntilLaunchAcknowledged(t *testing.T) {
	s, path := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, s, path)
	version := saveRetainedShortcut(t, s)
	checked := false
	s.agent = &retainedAgentStub{beforeInput: func() {
		s.terminalMu.Lock()
		var id string
		for _, item := range s.terminalSessions {
			if item.ShortcutID == panelTestShortcutID {
				id = item.ID
				if !item.LaunchUncertain {
					t.Error("launch exposed before acknowledgement")
				}
			}
		}
		s.terminalMu.Unlock()
		if id == "" {
			t.Fatal("launch not registered")
		}
		got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions/"+id+"/input-transport", []byte(`{}`), cookie, csrf, retainedHeaders(csrf))
		if got.Code != 409 {
			t.Errorf("concurrent claim: %d %s", got.Code, got.Body)
		}
		checked = true
	}}
	got := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", retainedShortcutBody(version), cookie, csrf, retainedHeaders(csrf))
	var result terminalOpenResponse
	_ = json.Unmarshal(got.Body.Bytes(), &result)
	if !checked || got.Code != 201 || result.LaunchUncertain {
		t.Fatalf("ack: checked=%v response=%d %s", checked, got.Code, got.Body)
	}
}
