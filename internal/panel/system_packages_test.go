package panel

import (
	"net/http"
	"testing"
)

func TestSystemPackagesActionRequiresCSRFAndProxiesTypedRequest(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusAccepted, ContentType: "application/json", Body: []byte(`{"status":"accepted"}`)}}
	server.agent = agent
	body := []byte(`{"action":"install","items":["curl","htop"],"expectedResourceVersion":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	withoutCSRF := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/packages/actions", body, false)
	if withoutCSRF.Code == http.StatusAccepted {
		t.Fatal("request without CSRF was accepted")
	}
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/packages/actions", body, true)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/packages/actions" {
		t.Fatalf("agent calls = %#v", calls)
	}
}

func TestSystemPackagesActionRejectsArbitraryPackageName(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	body := []byte(`{"action":"install","items":["curl;reboot"],"expectedResourceVersion":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/packages/actions", body, true)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", response.Code, response.Body.String())
	}
}
