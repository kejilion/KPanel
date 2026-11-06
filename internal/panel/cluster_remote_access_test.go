package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestRemoteAccessAPIAuthenticationCSRFConflictAndLocalIsolation(t *testing.T) {
	s, token := newTestServer(t)
	const route = "/api/v1/cluster/remote-access"
	if response := performRequest(s, http.MethodGet, route, nil, nil); response.Code != 401 {
		t.Fatal(response.Code)
	}
	session, csrf := bootstrapCookies(t, s, token)
	read := authenticatedRequest(s, http.MethodGet, route, nil, session, csrf, nil)
	var before cluster.RemoteAccess
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &before) != nil || !before.TerminalEnabled || !before.FilesEnabled {
		t.Fatal(read.Code, read.Body.String())
	}
	body := []byte(`{"terminalEnabled":false,"filesEnabled":false,"expectedResourceVersion":"` + before.ResourceVersion + `"}`)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	for _, omit := range []string{"Origin", "X-CSRF-Token"} {
		partial := map[string]string{}
		for key, value := range headers {
			if key != omit {
				partial[key] = value
			}
		}
		response := authenticatedRequest(s, http.MethodPut, route, body, session, csrf, partial)
		if response.Code != 403 {
			t.Fatal(omit, response.Code, response.Body.String())
		}
		if !s.cluster.RemoteAccess().TerminalEnabled {
			t.Fatal("unauthorized mutation applied")
		}
	}
	response := authenticatedRequest(s, http.MethodPut, route, body, session, csrf, headers)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if s.cluster.RemoteAccess().TerminalEnabled || s.cluster.RemoteAccess().FilesEnabled {
		t.Fatal("policy not changed")
	}
	stale := authenticatedRequest(s, http.MethodPut, route, body, session, csrf, headers)
	if stale.Code != 409 || !strings.Contains(stale.Body.String(), "remote_access_changed") {
		t.Fatal(stale.Code, stale.Body.String())
	}
	inventory := authenticatedRequest(s, http.MethodGet, "/api/v1/cluster/hosts", nil, session, csrf, nil)
	var hosts clusterHostsResponse
	if inventory.Code != 200 || json.Unmarshal(inventory.Body.Bytes(), &hosts) != nil {
		t.Fatal(inventory.Code, inventory.Body.String())
	}
	if len(hosts.Items) != 1 || !hosts.Items[0].IsLocal || !hosts.Items[0].TerminalAvailable || !hosts.Items[0].FileManagementAvailable {
		t.Fatal(hosts.Items)
	}
	for _, invalid := range []string{`{"terminalEnabled":true}`, `{"terminalEnabled":null,"filesEnabled":true}`, `{"terminalEnabled":true,"filesEnabled":true,"extra":true}`} {
		invalidResponse := authenticatedRequest(s, http.MethodPut, route, []byte(invalid), session, csrf, headers)
		if invalidResponse.Code < 400 {
			t.Fatal(invalidResponse.Code, invalidResponse.Body.String())
		}
	}
}

func TestRemoteAccessCleanupKeepsUnconfirmedRemoteSessionAndLocalSession(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	s.terminalSessions["remote"] = panelTerminalSession{ID: "remote", HostID: "unreachable", BackendSessionID: "backend", UserID: session.Value}
	s.terminalSessions["local"] = panelTerminalSession{ID: "local", HostID: cluster.LocalHostID, BackendSessionID: "local-backend"}
	response := authenticatedRequest(s, http.MethodPut, "/api/v1/cluster/remote-access", []byte(`{"terminalEnabled":false,"filesEnabled":true,"expectedResourceVersion":"initial"}`), session, csrf, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"pendingTerminalCleanup":1`) {
		t.Fatal(response.Body.String())
	}
	if !s.terminalSessions["remote"].CloseRequested || s.terminalSessions["local"].CloseRequested {
		t.Fatal(s.terminalSessions)
	}
	read := authenticatedRequest(s, http.MethodGet, "/api/v1/cluster/remote-access", nil, session, csrf, nil)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"pendingTerminalCleanup":1`) {
		t.Fatal("reopening settings lost pending cleanup", read.Code, read.Body.String())
	}
}

func TestRemoteAccessRemembersFailedOpenCleanupWithoutChangingLocalSessions(t *testing.T) {
	s, _ := newTestServer(t)
	opened := cluster.TerminalOpenResponse{SessionID: "known-backend"}
	s.rememberRemoteTerminalOpen("owner", "remote", opened)
	s.rememberRemoteTerminalOpen("owner", "remote", opened)
	s.rememberRemoteTerminalOpen("owner", cluster.LocalHostID, opened)
	s.rememberRemoteTerminalOpen("owner", "remote", cluster.TerminalOpenResponse{})
	if len(s.terminalSessions) != 1 {
		t.Fatal(s.terminalSessions)
	}
	for _, item := range s.terminalSessions {
		if !item.CloseRequested || item.BackendSessionID != opened.SessionID || item.UserID != "owner" || item.HostID != "remote" {
			t.Fatal(item)
		}
	}
}

func TestUnreturnedRemoteOpenUsesBoundedCleanupCapacity(t *testing.T) {
	s, _ := newTestServer(t)
	s.terminalMu.Lock()
	item := panelTerminalSession{ID: "unreturned", UserID: "owner", HostID: "remote", BackendSessionID: "backend", CloseRequested: true}
	item = s.detachUnreturnedTerminalLocked(item)
	if !item.Detached || !item.CloseRequested || item.BackendSessionID != "backend" {
		s.terminalMu.Unlock()
		t.Fatalf("unreturned session was not preserved in cleanup capacity: %+v", item)
	}
	s.terminalSessions[item.ID] = item
	s.terminalMu.Unlock()
	if !s.reserveTerminalOpen(item.UserID) {
		t.Fatal("unreturned session consumed an interactive slot")
	}
	s.releaseTerminalOpen(item.UserID)

	s.terminalMu.Lock()
	for i := 1; i < maxPanelTerminalsByUser; i++ {
		id := fmt.Sprintf("detached-%d", i)
		s.terminalSessions[id] = panelTerminalSession{ID: id, UserID: item.UserID, Detached: true}
	}
	full := s.detachUnreturnedTerminalLocked(panelTerminalSession{ID: "full", UserID: item.UserID, CloseRequested: true})
	s.terminalMu.Unlock()
	if full.Detached || !full.CloseRequested {
		t.Fatal("full cleanup capacity did not retain the interactive reservation")
	}
}
