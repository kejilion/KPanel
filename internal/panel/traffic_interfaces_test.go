package panel

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrafficInterfacesProxyRequiresSessionOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := "sha256:" + strings.Repeat("a", 64)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"selection":{"include":[],"exclude":[]},"interfaces":[],"resourceVersion":"` + version + `"}`),
	}}
	server.agent = agent

	if response := performRequest(server, http.MethodGet, trafficInterfacesPath, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status=%d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodGet, trafficInterfacesPath, nil, sessionCookie, csrfCookie, nil); response.Code != http.StatusOK {
		t.Fatalf("authenticated GET status=%d body=%s", response.Code, response.Body.String())
	}

	body := []byte(`{"include":["eth0"],"exclude":[],"expectedResourceVersion":"` + version + `"}`)
	missingOrigin := authenticatedRequest(server, http.MethodPut, trafficInterfacesPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	})
	if missingOrigin.Code != http.StatusForbidden {
		t.Fatalf("missing Origin status=%d", missingOrigin.Code)
	}
	missingCSRF := authenticatedRequest(server, http.MethodPut, trafficInterfacesPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", missingCSRF.Code)
	}
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	unknown := authenticatedRequest(server, http.MethodPut, trafficInterfacesPath, []byte(`{"include":[],"extra":1}`), sessionCookie, csrfCookie, headers)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", unknown.Code)
	}
	updated := authenticatedRequest(server, http.MethodPut, trafficInterfacesPath, body, sessionCookie, csrfCookie, headers)
	if updated.Code != http.StatusOK {
		t.Fatalf("valid PUT status=%d body=%s", updated.Code, updated.Body.String())
	}

	calls := agent.snapshotCalls()
	if len(calls) != 2 || calls[0].method != http.MethodGet || calls[0].path != "/v1/system/traffic-interfaces" ||
		calls[1].method != http.MethodPut || calls[1].path != "/v1/system/traffic-interfaces" || string(calls[1].body) != string(body) {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	results := map[string]int{}
	for _, event := range events {
		if event.Action == "system.traffic_interfaces.update" {
			results[event.Result]++
		}
	}
	if results["intent"] != 1 || results["success"] != 1 || results["failure"] != 1 {
		t.Fatalf("audit results = %#v", results)
	}
}
