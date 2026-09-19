package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNetworkOperationsGetUsesExactAuthenticatedProxyPath(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"entries":[]}`)}}
	server.agent = agent

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/port-usage", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/port-usage" || calls[0].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/system/traffic-shutdown?unsafe=1", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("query status=%d calls=%#v", response.Code, agent.snapshotCalls())
	}
}

func TestTrafficShutdownWriteUsesAuthCSRFAndAuditsTypedThresholds(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	version := strings.Repeat("a", 64)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"enable","status":"succeeded","changed":true,"message":"ok","resourceVersion":"` + strings.Repeat("b", 64) + `","appliedAt":"2026-08-10T00:00:00Z"}`),
	}}
	server.agent = agent
	body := []byte(`{"action":"enable","expectedResourceVersion":"` + version + `","rxThresholdGiB":100,"txThresholdGiB":200,"resetDay":5}`)
	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/traffic-shutdown/actions", body, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/system/traffic-shutdown/actions" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	found := 0
	for _, event := range events {
		if event.Action != "system.traffic-shutdown.enable" {
			continue
		}
		found++
		if event.Change["rxThresholdGiB"] != float64(100) && event.Change["rxThresholdGiB"] != uint64(100) {
			t.Fatalf("unexpected audit change: %#v", event.Change)
		}
	}
	if found != 2 {
		t.Fatalf("traffic shutdown audit events=%d want=2", found)
	}
}
