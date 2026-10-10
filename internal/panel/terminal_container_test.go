package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type containerTerminalAgentStub struct {
	terminalAgentStub
	body        []byte
	openStatus  int
	openProblem string
}

func (s *containerTerminalAgentStub) Do(ctx context.Context, method, path, contentType, requestID string, body []byte) (AgentResponse, error) {
	if method == http.MethodPost && path == "/v1/terminals" {
		s.body = append([]byte(nil), body...)
		if s.openStatus != 0 {
			data, _ := json.Marshal(map[string]any{"code": s.openProblem, "status": s.openStatus, "title": "fixture"})
			return AgentResponse{StatusCode: s.openStatus, ContentType: "application/json", Body: data}, nil
		}
	}
	return s.terminalAgentStub.Do(ctx, method, path, contentType, requestID, body)
}

func TestContainerTerminalPanelBindsTargetAndPreservesGuards(t *testing.T) {
	s, tokenPath := newTestServer(t)
	stub := &containerTerminalAgentStub{}
	s.agent = stub
	sessionCookie, csrfCookie := bootstrapCookies(t, s, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	body := []byte(`{"hostId":"local","rows":30,"columns":120,"containerId":"` + strings.Repeat("a", 64) + `","resourceVersion":"exact-version"}`)
	missingCSRF := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, sessionCookie, csrfCookie, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"})
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d", missingCSRF.Code)
	}
	if len(stub.body) != 0 {
		t.Fatal("unauthorized request reached Agent")
	}
	opened := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, sessionCookie, csrfCookie, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	var target struct {
		Owner           string `json:"owner"`
		ContainerID     string `json:"containerId"`
		ResourceVersion string `json:"resourceVersion"`
		Rows, Columns   int
	}
	if err := json.Unmarshal(stub.body, &target); err != nil {
		t.Fatal(err)
	}
	if target.Owner == "" || target.ContainerID != strings.Repeat("a", 64) || target.ResourceVersion != "exact-version" || target.Rows != 30 || target.Columns != 120 {
		t.Fatalf("Agent target = %#v", target)
	}
	var public terminalOpenResponse
	_ = json.Unmarshal(opened.Body.Bytes(), &public)
	if public.SessionID == "backend-terminal" || public.SessionID == "" {
		t.Fatal("backend ID escaped public session ownership")
	}
}

func TestContainerTerminalPanelRejectsPartialTargetsAndMapsAdmissionErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, container, version, problem string
		backend, expected                 int
	}{
		{"missing-version", strings.Repeat("a", 64), "", "", 0, http.StatusUnprocessableEntity},
		{"missing-container", "", "v", "", 0, http.StatusUnprocessableEntity},
		{"invalid-container", "../host", "v", "", 0, http.StatusUnprocessableEntity},
		{"version-conflict", strings.Repeat("a", 64), "v", "resource_conflict", 409, 409},
		{"shared-limit", strings.Repeat("a", 64), "v", "terminal_limit", 429, 429},
		{"unsupported", strings.Repeat("a", 64), "v", "container_terminal_unavailable", 409, 409},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, tokenPath := newTestServer(t)
			stub := &containerTerminalAgentStub{openStatus: scenario.backend, openProblem: scenario.problem}
			s.agent = stub
			sessionCookie, csrfCookie := bootstrapCookies(t, s, tokenPath)
			body, _ := json.Marshal(map[string]any{"hostId": "local", "rows": 24, "columns": 80, "containerId": scenario.container, "resourceVersion": scenario.version})
			response := authenticatedRequest(s, http.MethodPost, "/api/v1/terminal-sessions", body, sessionCookie, csrfCookie, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value})
			if response.Code != scenario.expected {
				t.Fatalf("open = %d %s", response.Code, response.Body.String())
			}
			if scenario.backend == 0 && len(stub.body) != 0 {
				t.Fatal("invalid target reached Agent")
			}
		})
	}
}
