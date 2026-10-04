package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestOfficeWritePreservesAuthenticationCSRFTransportAndAudit(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{StatusCode: 200, ContentType: "application/json", Body: []byte(`{"entry":{"path":"/demo.docx"}}`)}}}
	server.agent = agent
	input := contract.FileWriteRequest{ExpectedResourceVersion: "v1", ExpectedContentVersion: strings.Repeat("a", 64), OfficeEdits: []contract.OfficeEdit{{ID: "p1", Text: "Saved"}}}
	body, _ := json.Marshal(input)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	unauthenticated := performRequest(server, http.MethodPut, "/api/v1/files/content?path=%2Fdemo.docx", body, headers)
	if unauthenticated.Code != 401 {
		t.Fatal("anonymous write accepted", unauthenticated.Code)
	}
	withoutCSRF := authenticatedRequest(server, http.MethodPut, "/api/v1/files/content?path=%2Fdemo.docx", body, session, csrf, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"})
	if withoutCSRF.Code != 403 || len(agent.snapshotCalls()) != 0 {
		t.Fatal("CSRF check bypassed", withoutCSRF.Code)
	}
	response := authenticatedRequest(server, http.MethodPut, "/api/v1/files/content?path=%2Fdemo.docx", body, session, csrf, headers)
	if response.Code != 200 {
		t.Fatalf("write: %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files/content" || !strings.Contains(string(calls[0].body), `"officeEdits"`) || !strings.Contains(string(calls[0].body), input.ExpectedContentVersion) {
		t.Fatalf("transport: %#v", calls)
	}
	events, _, _ := server.store.ListAudit(20, "")
	intent, success := false, false
	for _, event := range events {
		if event.Action == "file.write" {
			intent = intent || event.Result == "intent"
			success = success || event.Result == "success"
		}
	}
	if !intent || !success {
		t.Fatal("write audit missing", events)
	}
}
