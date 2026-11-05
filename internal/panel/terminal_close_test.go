package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

type terminalCloseAgent struct {
	terminalAgentStub
	err       error
	status    int
	code      string
	owners    []string
	ids       []string
	deadline  bool
	closeBody []byte
}

func (s *terminalCloseAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if !strings.HasSuffix(path, "/close") {
		return s.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
	}
	var input struct {
		Owner string `json:"owner"`
	}
	_ = json.Unmarshal(body, &input)
	s.owners = append(s.owners, input.Owner)
	s.ids = append(s.ids, path)
	deadline, ok := ctx.Deadline()
	s.deadline = ok && time.Until(deadline) <= 10*time.Second
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	payload := []byte(`{"closed":true}`)
	if s.code != "" {
		payload, _ = json.Marshal(map[string]any{"code": s.code})
	}
	if s.closeBody != nil {
		payload = s.closeBody
	}
	return AgentResponse{StatusCode: status, Body: payload}, s.err
}

func TestTerminalCloseRejectsUnconfirmedSuccessBody(t *testing.T) {
	for _, body := range []string{`{"closed":false}`, `{"accepted":true}`, `{}`, `invalid`} {
		stub := &terminalCloseAgent{closeBody: []byte(body)}
		if err := (clusterTerminalSource{agent: stub}).Close(context.Background(), "owner", "id"); err == nil {
			t.Fatalf("unconfirmed body accepted: %s", body)
		}
	}
}

func TestDetachRetainsUnconfirmedIdentityButFreesInteractiveSlot(t *testing.T) {
	server, tokenPath := newTestServer(t)
	stub := &terminalCloseAgent{err: errors.New("offline")}
	server.agent = stub
	cookie, csrf := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), cookie, csrf, headers)
	var session terminalOpenResponse
	if opened.Code != http.StatusCreated || json.Unmarshal(opened.Body.Bytes(), &session) != nil {
		t.Fatal(opened.Body.String())
	}
	if !session.Detachable {
		t.Fatal("interactive session missing detachable classification")
	}
	path := "/api/v1/terminal-sessions/" + session.SessionID
	if got := authenticatedRequest(server, http.MethodPost, path+"/detach", []byte(`{}`), cookie, csrf, headers); got.Code != http.StatusConflict {
		t.Fatalf("detached active session: %d", got.Code)
	}
	if got := authenticatedRequest(server, http.MethodPost, path+"/close", []byte(`{}`), cookie, csrf, headers); got.Code != http.StatusBadGateway {
		t.Fatal(got.Body.String())
	}
	if got := authenticatedRequest(server, http.MethodPost, path+"/detach", []byte(`{}`), cookie, csrf, headers); got.Code != http.StatusOK {
		t.Fatal(got.Body.String())
	}
	item, ok := server.terminalSessions[session.SessionID]
	if !ok || !item.CloseRequested || !item.Detached || item.BackendSessionID != "backend-terminal" {
		t.Fatalf("identity lost: %+v", item)
	}
	listed := authenticatedRequest(server, http.MethodGet, "/api/v1/terminal-sessions", nil, cookie, csrf, nil)
	if strings.Contains(listed.Body.String(), session.SessionID) {
		t.Fatal("detached session restored to list")
	}
	for i := 0; i < maxPanelTerminalsByUser; i++ {
		if !server.reserveTerminalOpen(item.UserID) {
			t.Fatal("detached session consumed an interactive slot")
		}
	}
	if server.reserveTerminalOpen(item.UserID) {
		t.Fatal("interactive limit not enforced")
	}
	for i := 0; i < maxPanelTerminalsByUser; i++ {
		server.releaseTerminalOpen(item.UserID)
	}
	stub.err = nil
	server.scheduleTerminalCleanup()
	server.terminalMu.Lock()
	done := server.terminalCleanupDone
	server.terminalMu.Unlock()
	<-done
	server.terminalMu.Lock()
	_, ok = server.terminalSessions[session.SessionID]
	server.terminalMu.Unlock()
	if ok {
		t.Fatal("confirmed cleanup did not remove identity")
	}
}

func TestDetachEnforcesOwnershipAndBoundedCleanupRecords(t *testing.T) {
	server, tokenPath := newTestServer(t)
	server.agent = &terminalCloseAgent{}
	cookie, csrf := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), cookie, csrf, headers)
	var session terminalOpenResponse
	if opened.Code != http.StatusCreated || json.Unmarshal(opened.Body.Bytes(), &session) != nil {
		t.Fatal(opened.Body.String())
	}
	item := server.terminalSessions[session.SessionID]
	item.CloseRequested = true
	server.terminalSessions[session.SessionID] = item
	foreign := item
	foreign.ID = "foreign"
	foreign.UserID = "another-user"
	server.terminalSessions[foreign.ID] = foreign
	if got := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/foreign/detach", []byte(`{}`), cookie, csrf, headers); got.Code != http.StatusNotFound {
		t.Fatalf("foreign detach = %d", got.Code)
	}
	protected := item
	protected.ID, protected.Detachable = "protected", false
	server.terminalSessions[protected.ID] = protected
	if terminalSessionResponse(protected).Detachable {
		t.Fatal("protected session incorrectly classified as detachable")
	}
	if got := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/protected/detach", []byte(`{}`), cookie, csrf, headers); got.Code != http.StatusConflict {
		t.Fatalf("non-interactive detach = %d", got.Code)
	}
	for n := 0; n < maxPanelTerminalsByUser; n++ {
		copy := item
		copy.ID = fmt.Sprintf("detached-%d", n)
		copy.Detached = true
		server.terminalSessions[copy.ID] = copy
	}
	got := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/detach", []byte(`{}`), cookie, csrf, headers)
	if got.Code != http.StatusConflict || server.terminalSessions[session.SessionID].Detached {
		t.Fatalf("detach beyond cap = %d", got.Code)
	}
}

func TestTerminalCloseRetainsIdentityAndAuditsFailureBeforeRetry(t *testing.T) {
	for _, failure := range []error{errors.New("Agent disconnected"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			server, tokenPath := newTestServer(t)
			stub := &terminalCloseAgent{err: failure}
			server.agent = stub
			cookie, csrf := bootstrapCookies(t, server, tokenPath)
			headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
			opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), cookie, csrf, headers)
			var session terminalOpenResponse
			if opened.Code != http.StatusCreated || json.Unmarshal(opened.Body.Bytes(), &session) != nil {
				t.Fatal(opened.Body.String())
			}
			path := "/api/v1/terminal-sessions/" + session.SessionID + "/close"
			failed := authenticatedRequest(server, http.MethodPost, path, []byte(`{}`), cookie, csrf, headers)
			if failed.Code != http.StatusBadGateway || !strings.Contains(failed.Body.String(), "terminal_close_failed") {
				t.Fatalf("close=%d %s", failed.Code, failed.Body.String())
			}
			retained, ok := server.terminalSessions[session.SessionID]
			if !ok || retained.BackendSessionID != "backend-terminal" || !stub.deadline {
				t.Fatal("identity or bounded close deadline missing")
			}
			stub.outputStatus = http.StatusNotFound
			stub.outputBody = []byte(`{"code":"terminal_closed"}`)
			_ = authenticatedRequest(server, http.MethodGet, "/api/v1/terminal-sessions/"+session.SessionID+"/output?offset=0&wait=0", nil, cookie, csrf, nil)
			if _, ok := server.terminalSessions[session.SessionID]; !ok {
				t.Fatal("late output erased unconfirmed close")
			}
			events, _, _ := server.store.ListAudit(10, "")
			if len(events) == 0 || events[0].Action != "terminal.close" || events[0].Result != "failure" {
				t.Fatalf("audit=%+v", events)
			}
			stub.err = nil
			success := authenticatedRequest(server, http.MethodPost, path, []byte(`{}`), cookie, csrf, headers)
			if success.Code != http.StatusOK || !strings.Contains(success.Body.String(), `"closed":true`) {
				t.Fatal(success.Body.String())
			}
			if _, ok := server.terminalSessions[session.SessionID]; ok {
				t.Fatal("confirmed close retained index")
			}
			if len(stub.ids) != 2 || stub.ids[0] != stub.ids[1] || stub.owners[0] != stub.owners[1] {
				t.Fatal("retry identity changed")
			}
			events, _, _ = server.store.ListAudit(10, "")
			if events[0].Action != "terminal.close" || events[0].Result != "success" {
				t.Fatalf("audit=%+v", events)
			}
			again := authenticatedRequest(server, http.MethodPost, path, []byte(`{}`), cookie, csrf, headers)
			if again.Code != http.StatusNotFound || len(stub.ids) != 2 {
				t.Fatal("missing public session must not reach backend")
			}
		})
	}
}

func TestRemoteTerminalCloseConfirmsTargetAbsenceAndRetainsTransportFailures(t *testing.T) {
	stub := &terminalCloseAgent{}
	target, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Terminal: clusterTerminalSource{agent: stub}, Telemetry: clusterTelemetrySource{agent: stub}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	transportFailure := false
	loopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if transportFailure {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var envelope cluster.FederationEnvelopeV2
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result, err := target.HandleFederationV2(r.Context(), "198.51.100.10", r.URL.Path, r.Header.Get(cluster.FederationCapabilitiesHeader), envelope)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer loopback.Close()
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, loopback.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Remote: remote, Telemetry: clusterTelemetrySource{agent: stub}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })
	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	// The injected dialer always uses the loopback test server; no external I/O.
	host, err := center.AddHost(context.Background(), cluster.AddHostInput{Name: "target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	server, _ := newTestServer(t)
	server.cluster = center
	for _, failureCode := range []string{"", "terminal_not_found", "terminal_closed"} {
		server.terminalSessions["remote"] = panelTerminalSession{ID: "remote", UserID: "owner", BackendSessionID: "backend", HostID: host.ID}
		transportFailure = true
		failed := httptest.NewRecorder()
		server.handleTerminalOperation(failed, httptest.NewRequest(http.MethodPost, "/", nil), "owner", "remote", "close")
		if failed.Code != http.StatusBadGateway {
			t.Fatal(failed.Code)
		}
		if _, ok := server.terminalSessions["remote"]; !ok {
			t.Fatal("remote transport failure erased identity")
		}
		transportFailure = false
		stub.code = failureCode
		stub.status = http.StatusOK
		if failureCode != "" {
			stub.status = http.StatusNotFound
		}
		closed := httptest.NewRecorder()
		server.handleTerminalOperation(closed, httptest.NewRequest(http.MethodPost, "/", nil), "owner", "remote", "close")
		if closed.Code != http.StatusOK {
			t.Fatalf("remote %s = %d %s", failureCode, closed.Code, closed.Body.String())
		}
		if _, ok := server.terminalSessions["remote"]; ok {
			t.Fatal("confirmed remote close retained index")
		}
	}
	// A disconnected node can later return authenticated confirmation that the
	// old PTY no longer exists, as after a target process restart.
	server.terminalSessions["detached-remote"] = panelTerminalSession{ID: "detached-remote", UserID: "owner", BackendSessionID: "backend", HostID: host.ID, Detachable: true}
	transportFailure = true
	failed := httptest.NewRecorder()
	server.handleTerminalOperation(failed, httptest.NewRequest(http.MethodPost, "/", nil), "owner", "detached-remote", "close")
	if failed.Code != http.StatusBadGateway {
		t.Fatalf("offline close = %d", failed.Code)
	}
	detached := httptest.NewRecorder()
	server.handleTerminalOperation(detached, httptest.NewRequest(http.MethodPost, "/", nil), "owner", "detached-remote", "detach")
	if detached.Code != http.StatusOK {
		t.Fatalf("detach = %d %s", detached.Code, detached.Body.String())
	}
	transportFailure = false
	stub.code, stub.status = "terminal_not_found", http.StatusNotFound
	server.scheduleTerminalCleanup()
	server.terminalMu.Lock()
	done := server.terminalCleanupDone
	server.terminalMu.Unlock()
	<-done
	server.terminalMu.Lock()
	_, retained := server.terminalSessions["detached-remote"]
	server.terminalMu.Unlock()
	if retained {
		t.Fatal("authenticated missing-session response did not clear detached identity")
	}
}

func TestTerminalCloseAcceptsOnlyConfirmedAbsence(t *testing.T) {
	for _, code := range []string{"terminal_not_found", "terminal_closed", "route_not_found"} {
		t.Run(code, func(t *testing.T) {
			server, _ := newTestServer(t)
			stub := &terminalCloseAgent{status: http.StatusNotFound, code: code}
			server.agent = stub
			server.terminalSessions["private"] = panelTerminalSession{ID: "private", UserID: "owner", Owner: "panel:owner", BackendSessionID: "backend", HostID: "local"}
			foreign := httptest.NewRecorder()
			server.handleTerminalOperation(foreign, httptest.NewRequest(http.MethodPost, "/", nil), "other", "private", "close")
			if foreign.Code != http.StatusNotFound || len(stub.ids) != 0 {
				t.Fatal("owner isolation failed")
			}
			response := httptest.NewRecorder()
			server.handleTerminalOperation(response, httptest.NewRequest(http.MethodPost, "/", nil), "owner", "private", "close")
			_, retained := server.terminalSessions["private"]
			if code == "route_not_found" {
				if response.Code != http.StatusBadGateway || !retained {
					t.Fatal("generic 404 treated as confirmed absence")
				}
			} else if response.Code != http.StatusOK || retained {
				t.Fatalf("known absence: %d retained=%v", response.Code, retained)
			}
		})
	}
}
