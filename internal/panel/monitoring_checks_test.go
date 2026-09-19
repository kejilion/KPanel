package panel

import (
	"net/http"
	"strings"
	"testing"
)

func TestMonitoringChecksProxyRequiresSessionOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := "sha256:" + strings.Repeat("a", 64)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"schemaVersion":1,"resourceVersion":"` + version + `","available":true,"maxItems":16,"items":[]}`),
	}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/monitoring/checks", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status=%d", unauthenticated.Code)
	}

	read := authenticatedRequest(server, http.MethodGet, "/api/v1/monitoring/checks", nil, sessionCookie, csrfCookie, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("authenticated GET status=%d body=%s", read.Code, read.Body.String())
	}

	body := []byte(`{"expectedResourceVersion":"` + version + `","items":[]}`)
	missingOrigin := authenticatedRequest(server, http.MethodPut, "/api/v1/monitoring/checks", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	})
	if missingOrigin.Code != http.StatusForbidden {
		t.Fatalf("missing Origin status=%d", missingOrigin.Code)
	}

	missingCSRF := authenticatedRequest(server, http.MethodPut, "/api/v1/monitoring/checks", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", missingCSRF.Code)
	}

	updated := authenticatedRequest(server, http.MethodPut, "/api/v1/monitoring/checks", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("valid PUT status=%d body=%s", updated.Code, updated.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 2 || calls[0].method != http.MethodGet || calls[1].method != http.MethodPut ||
		calls[1].path != "/v1/monitoring/checks" || string(calls[1].body) != string(body) {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
}

func TestAllowedMonitoringChecksPath(t *testing.T) {
	if path, ok := allowedAgentPath("/api/v1/monitoring/checks"); !ok || path != "/v1/monitoring/checks" {
		t.Fatalf("monitoring checks path=%q ok=%v", path, ok)
	}
}
