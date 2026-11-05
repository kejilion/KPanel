package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccessPasswordReadPathIsExact(t *testing.T) {
	id := strings.Repeat("a", 32)
	public := "/api/v1/sites/" + id + "/access-passwords"
	if got, ok := allowedAgentPath(public); !ok || got != "/v1/sites/"+id+"/access-passwords" {
		t.Fatalf("unexpected mapping %q, %v", got, ok)
	}
	for _, invalid := range []string{public + "/", "/api/v1/sites/not-a-site/access-passwords", public + "/extra"} {
		if got, ok := allowedAgentPath(invalid); ok {
			t.Fatalf("unexpected mapping for %q: %q", invalid, got)
		}
	}
}

func TestAccessPasswordAccountRequiresCSRFAndDoesNotAuditPassword(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"items":[]}`)}}
	server.agent = agent
	id := strings.Repeat("a", 32)
	rule := strings.Repeat("b", 32)
	url := "/api/v1/sites/" + id + "/access-passwords/accounts"
	body := []byte(`{"ruleId":"` + rule + `","resourceVersion":"sha256:` + strings.Repeat("c", 64) + `","action":"add","username":"alice","password":"synthetic-password"}`)
	denied := authenticatedSiteRequest(server, session, csrf, http.MethodPost, url, body, false)
	if denied.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("CSRF result: %d", denied.Code)
	}
	response := authenticatedSiteRequest(server, session, csrf, http.MethodPost, url, body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/sites/"+id+"/access-passwords/accounts" || !strings.Contains(string(calls[0].body), "synthetic-password") {
		t.Fatalf("unexpected forwarding: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	for _, event := range events {
		encoded, err := json.Marshal(event.Change)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "synthetic-password") || strings.Contains(string(encoded), "alice") {
			t.Fatal("account secret or username was audited")
		}
	}
}

func TestAccessPasswordRuleRequiresCSRFAndDoesNotAuditSecret(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"items":[]}`)}}
	server.agent = agent
	id := strings.Repeat("a", 32)
	url := "/api/v1/sites/" + id + "/access-passwords/rules"
	body := []byte(`{"action":"add","resourceVersion":"sha256:` + strings.Repeat("c", 64) + `","scope":"server","username":"alice","password":"synthetic-password"}`)
	denied := authenticatedSiteRequest(server, session, csrf, http.MethodPost, url, body, false)
	if denied.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("CSRF result: %d", denied.Code)
	}
	response := authenticatedSiteRequest(server, session, csrf, http.MethodPost, url, body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	if calls := agent.snapshotCalls(); len(calls) != 1 || calls[0].path != "/v1/sites/"+id+"/access-passwords/rules" {
		t.Fatalf("unexpected forwarding: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	for _, event := range events {
		encoded, err := json.Marshal(event.Change)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "synthetic-password") || strings.Contains(string(encoded), "alice") {
			t.Fatal("account secret or username was audited")
		}
	}
}

func TestSiteCreateForwardsAccessPasswordWithoutAuditingIt(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusAccepted, ContentType: "application/json", Body: []byte(`{"status":"queued"}`)}}
	server.agent = agent
	body := []byte(`{"primaryDomain":"example.com","type":"static","accessPassword":{"scope":"server","username":"alice","password":"synthetic-password"}}`)
	response := authenticatedSiteRequest(server, session, csrf, http.MethodPost, "/api/v1/sites", body, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || !strings.Contains(string(calls[0].body), "synthetic-password") {
		t.Fatalf("creation input not forwarded")
	}
	events, _, _ := server.store.ListAudit(20, "")
	for _, event := range events {
		data, err := json.Marshal(event.Change)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "synthetic-password") || strings.Contains(string(data), "alice") {
			t.Fatal("creation secret or username was audited")
		}
	}
}
