package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccountManagementWriteRequiresCSRFAndNeverAuditsSecret(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := strings.Repeat("a", 64)
	secret := "do-not-store-this-password"
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"set-password","status":"succeeded","changed":true,"message":"ok","resourceVersion":"` + strings.Repeat("b", 64) + `","appliedAt":"2026-08-11T00:00:00Z"}`),
	}}
	server.agent = agent
	body := []byte(`{"action":"set-password","expectedResourceVersion":"` + version + `","username":"root","secret":"` + secret + `"}`)
	withoutCSRF := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/account-actions", body, false)
	if withoutCSRF.Code == http.StatusOK || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("missing CSRF status=%d calls=%#v", withoutCSRF.Code, agent.snapshotCalls())
	}
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/account-actions", body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/account-actions" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	for _, event := range events {
		if event.Action != "system.accounts.set-password" {
			continue
		}
		encoded, _ := json.Marshal(event.Change)
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("secret leaked into audit: %s", encoded)
		}
	}
}
