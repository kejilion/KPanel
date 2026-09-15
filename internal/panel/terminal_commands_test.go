package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/terminalcommands"
)

func TestTerminalCommandsAPIAuthenticationPersistenceAndConflict(t *testing.T) {
	server, tokenPath := newTestServer(t)
	unauthenticated := performRequest(server, http.MethodGet, terminalCommandsPath, nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d", unauthenticated.Code)
	}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	get := authenticatedRequest(server, http.MethodGet, terminalCommandsPath, nil, sessionCookie, csrfCookie, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET terminal commands failed: %d %s", get.Code, get.Body.String())
	}
	var initial terminalcommands.Snapshot
	if err := json.Unmarshal(get.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if !initial.Available || !terminalcommands.ValidResourceVersion(initial.ResourceVersion) || initial.Items == nil {
		t.Fatalf("unexpected initial commands: %#v", initial)
	}

	input := terminalcommands.ReplaceInput{
		ExpectedResourceVersion: initial.ResourceVersion,
		Items: []terminalcommands.Command{{
			ID: "0123456789abcdef0123456789abcdef", Name: "容器列表",
			Command: "docker ps --filter token=audit-secret-command",
		}},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	missingOrigin := authenticatedRequest(server, http.MethodPut, terminalCommandsPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	})
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin response = %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(server, http.MethodPut, terminalCommandsPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF response = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	savedResponse := authenticatedRequest(server, http.MethodPut, terminalCommandsPath, body, sessionCookie, csrfCookie, headers)
	if savedResponse.Code != http.StatusOK {
		t.Fatalf("PUT terminal commands failed: %d %s", savedResponse.Code, savedResponse.Body.String())
	}
	var saved terminalcommands.Snapshot
	if err := json.Unmarshal(savedResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResourceVersion == initial.ResourceVersion || len(saved.Items) != 1 || saved.Items[0].Name != "容器列表" {
		t.Fatalf("terminal commands did not update: %#v", saved)
	}

	loaded := authenticatedRequest(server, http.MethodGet, terminalCommandsPath, nil, sessionCookie, csrfCookie, nil)
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), "audit-secret-command") {
		t.Fatalf("saved terminal command was not returned: %d %s", loaded.Code, loaded.Body.String())
	}
	stale := authenticatedRequest(server, http.MethodPut, terminalCommandsPath, body, sessionCookie, csrfCookie, headers)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "terminal_commands_changed") {
		t.Fatalf("stale update response = %d %s", stale.Code, stale.Body.String())
	}

	events, _ := server.store.ListAudit(20, "")
	auditData, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditData), "terminal.commands.update") || strings.Contains(string(auditData), "audit-secret-command") {
		t.Fatalf("terminal command audit metadata leaked command text: %s", auditData)
	}
}

func TestTerminalCommandsAPIRejectsInvalidRequests(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}

	invalidQuery := authenticatedRequest(server, http.MethodGet, terminalCommandsPath+"?extra=1", nil, sessionCookie, csrfCookie, nil)
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status = %d", invalidQuery.Code)
	}
	wrongMethod := authenticatedRequest(server, http.MethodPost, terminalCommandsPath, []byte(`{}`), sessionCookie, csrfCookie, headers)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != "GET, PUT" {
		t.Fatalf("wrong method response = %d Allow=%q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	initial := server.terminalCommands.Snapshot()
	invalid := terminalcommands.ReplaceInput{
		ExpectedResourceVersion: initial.ResourceVersion,
		Items: []terminalcommands.Command{{
			ID: "0123456789abcdef0123456789abcdef", Name: "Status", Command: "echo\x00secret",
		}},
	}
	body, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(server, http.MethodPut, terminalCommandsPath, body, sessionCookie, csrfCookie, headers)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "items.command") {
		t.Fatalf("invalid command response = %d %s", response.Code, response.Body.String())
	}
}
