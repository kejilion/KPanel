package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSSHDefenseWriteRequiresCSRFAndAuditsTypedChange(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := strings.Repeat("a", 64)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"add-trusted","status":"succeeded","changed":true,"message":"ok","resourceVersion":"` + strings.Repeat("b", 64) + `","appliedAt":"2026-08-11T00:00:00Z"}`),
	}}
	server.agent = agent
	body := []byte(`{"action":"add-trusted","expectedResourceVersion":"` + version + `","address":"203.0.113.0/24"}`)
	withoutCSRF := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/ssh-defense/actions", body, false)
	if withoutCSRF.Code == http.StatusOK || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("missing CSRF status=%d calls=%#v", withoutCSRF.Code, agent.snapshotCalls())
	}
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/ssh-defense/actions", body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/ssh-defense/actions" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	found := false
	for _, event := range events {
		if event.Action != "system.ssh-defense.add-trusted" {
			continue
		}
		encoded, _ := json.Marshal(event.Change)
		if !strings.Contains(string(encoded), "203.0.113.0/24") {
			t.Fatalf("typed address missing from audit: %s", encoded)
		}
		found = true
	}
	if !found {
		t.Fatal("SSH defense audit event was not written")
	}
}
