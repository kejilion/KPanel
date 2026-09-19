package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type detachedSystemResourceAgent struct {
	started chan context.Context
	release chan struct{}
}

func (agent *detachedSystemResourceAgent) Get(
	context.Context, string, string, string,
) (AgentResponse, error) {
	return AgentResponse{}, nil
}

func (agent *detachedSystemResourceAgent) Do(
	ctx context.Context, _, _, _, _ string, _ []byte,
) (AgentResponse, error) {
	agent.started <- ctx
	select {
	case <-agent.release:
		return AgentResponse{
			StatusCode: http.StatusOK, ContentType: "application/json",
			Body: []byte(`{"action":"cron-delete","status":"succeeded","changed":true,"message":"ok","resourceVersion":"` +
				strings.Repeat("b", 64) + `","appliedAt":"2026-08-10T00:00:00Z"}`),
		}, nil
	case <-ctx.Done():
		return AgentResponse{}, ctx.Err()
	}
}

func TestSystemResourceGetUsesExactAuthenticatedProxyPath(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"resourceVersion":"` + strings.Repeat("a", 64) + `","entries":[],"total":0,"truncated":false}`),
	}}
	server.agent = agent
	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/system/hosts", nil)
	unauthenticated.Host = "panel.test"
	unauthenticatedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticatedResponse.Code, agent.snapshotCalls())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/hosts", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet || calls[0].path != "/v1/system/hosts" || calls[0].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/system/hosts?all=true", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("query status=%d calls=%#v", response.Code, agent.snapshotCalls())
	}
}

func TestSystemResourceWriteRequiresOriginSessionCSRFAndRedactsCronCommand(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := strings.Repeat("a", 64)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"cron-add","status":"succeeded","changed":true,"message":"ok","resourceVersion":"` +
			strings.Repeat("b", 64) + `","appliedAt":"2026-08-10T00:00:00Z"}`),
	}}
	server.agent = agent
	secretCommand := "/usr/bin/backup --token SECRET_TOKEN_123"
	body := []byte(`{"action":"cron-add","expression":"0 2 * * *","command":"` + secretCommand +
		`","expectedResourceVersion":"` + version + `"}`)

	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", body, true,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/v1/system/resource-actions" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}

	events, _, _ := server.store.ListAudit(100, "")
	found := 0
	for _, event := range events {
		if event.Action != "system.resource.cron-add" {
			continue
		}
		found++
		encoded, err := json.Marshal(event.Change)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		if strings.Contains(text, secretCommand) || strings.Contains(text, "SECRET_TOKEN_123") ||
			!strings.Contains(text, "commandHash") || !strings.Contains(text, "commandLength") ||
			!strings.Contains(text, `"schedule":"0 2 * * *"`) {
			t.Fatalf("unsafe or incomplete audit change: %s", text)
		}
	}
	if found != 2 {
		t.Fatalf("resource write audit events=%d want=2", found)
	}

	missingCSRF := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", body, false,
	)
	if missingCSRF.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("missing CSRF status=%d calls=%#v", missingCSRF.Code, agent.snapshotCalls())
	}
	invalidCron := []byte(`{"action":"cron-add","expression":"60 * * * *","command":"true","expectedResourceVersion":"` + version + `"}`)
	invalidCronResponse := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", invalidCron, true,
	)
	if invalidCronResponse.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(invalidCronResponse.Body.String(), "expression.minute") || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("invalid cron status=%d body=%s calls=%#v", invalidCronResponse.Code, invalidCronResponse.Body.String(), agent.snapshotCalls())
	}
	query := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions?unsafe=1", body, true,
	)
	if query.Code != http.StatusNotFound || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("query status=%d calls=%#v", query.Code, agent.snapshotCalls())
	}

	crossOriginRequest := newAuthenticatedSiteRequest(
		sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", body, true,
	)
	crossOriginRequest.Header.Set("Origin", "https://evil.example")
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOriginRequest)
	if crossOriginResponse.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("cross-origin status=%d calls=%#v", crossOriginResponse.Code, agent.snapshotCalls())
	}
}

func TestSystemResourceWriteForwardsAgentConflictEnvelope(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agentProblem := []byte(`{"title":"Conflict","status":409,"code":"system_resource_conflict","requestId":"agent-request"}`)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusConflict, ContentType: "application/problem+json", Body: agentProblem,
	}}
	server.agent = agent
	body := []byte(`{"action":"firewall-open-all","expectedResourceVersion":"` + strings.Repeat("a", 64) + `"}`)
	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", body, true,
	)
	if response.Code != http.StatusConflict || response.Header().Get("Content-Type") != "application/problem+json" ||
		response.Body.String() != string(agentProblem) {
		t.Fatalf("status=%d type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestSystemResourceWriteWaitsForAgentReceiptAfterBrowserCancellation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &detachedSystemResourceAgent{
		started: make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	server.agent = agent
	version := strings.Repeat("a", 64)
	body := []byte(`{"action":"cron-delete","line":1,"expectedResourceVersion":"` + version + `"}`)
	request := newAuthenticatedSiteRequest(
		sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/resource-actions", body, true,
	)
	browserContext, cancelBrowser := context.WithCancel(request.Context())
	request = request.WithContext(browserContext)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.ServeHTTP(response, request)
		close(done)
	}()

	agentContext := <-agent.started
	cancelBrowser()
	select {
	case <-agentContext.Done():
		t.Fatalf("Agent context was canceled with browser context: %v", agentContext.Err())
	case <-done:
		t.Fatal("handler returned before the Agent receipt")
	case <-time.After(25 * time.Millisecond):
	}
	close(agent.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after the Agent receipt")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", response.Code, response.Body.String())
	}
	events, _, _ := server.store.ListAudit(100, "")
	results := make(map[string]int, 2)
	for _, event := range events {
		if event.Action == "system.resource.cron-delete" {
			results[event.Result]++
		}
	}
	if results["intent"] != 1 || results["success"] != 1 || len(results) != 2 {
		t.Fatalf("resource audit results=%v want one intent and one success", results)
	}
}

func TestSystemResourceAuditChangeNeverStoresHostsPayload(t *testing.T) {
	change := systemResourceAuditChange(contract.SystemResourceActionRequest{
		Action: "hosts-add", Address: "192.0.2.10", Hostnames: []string{"private.internal"},
		Comment: "sensitive comment", ExpectedResourceVersion: strings.Repeat("a", 64),
	})
	encoded, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"192.0.2.10", "private.internal", "sensitive comment"} {
		if strings.Contains(text, secret) {
			t.Fatalf("audit leaked %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, "payloadHash") || !strings.Contains(text, "payloadLength") {
		t.Fatalf("audit metadata missing: %s", text)
	}
}
