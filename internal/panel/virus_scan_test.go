package panel

import (
	"net/http"
	"testing"
)

func TestVirusScanActionRequiresCSRFAndProxiesTypedRequest(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusAccepted, ContentType: "application/json", Body: []byte(`{"status":"accepted","taskId":"scan-1"}`)}}
	server.agent = agent
	body := []byte(`{"mode":"custom","paths":["/srv/sites"]}`)
	withoutCSRF := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/virus-scan/actions", body, false)
	if withoutCSRF.Code == http.StatusAccepted {
		t.Fatal("request without CSRF was accepted")
	}
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/virus-scan/actions", body, true)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/virus-scan/actions" || string(calls[0].body) != string(body) {
		t.Fatalf("agent calls = %#v", calls)
	}
}

func TestVirusScanActionRejectsInvalidCustomPath(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusAccepted, ContentType: "application/json", Body: []byte(`{"status":"accepted"}`)}}
	server.agent = agent
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/virus-scan/actions", []byte(`{"mode":"custom","paths":["../etc"]}`), true)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if calls := agent.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("invalid request reached Agent: %#v", calls)
	}
}
