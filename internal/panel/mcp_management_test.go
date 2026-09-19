package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

func TestMCPManagementAuthenticationCSRFConflictAndOneTimeToken(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	session, csrf := bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	request := func(method, path string, value any, authenticate, withCSRF bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(value)
		r := httptest.NewRequest(method, "https://panel.test"+path, strings.NewReader(string(b)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://panel.test")
		if authenticate {
			r.AddCookie(session)
		}
		if withCSRF {
			r.AddCookie(csrf)
			r.Header.Set("X-CSRF-Token", csrf.Value)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodGet, mcpSettingsPath, nil, false, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	version := s.mcp.access.Snapshot().ResourceVersion
	input := map[string]any{"enabled": true, "expectedResourceVersion": version}
	if w := request(http.MethodPut, mcpSettingsPath, input, true, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request(http.MethodPut, mcpSettingsPath, input, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(http.MethodPut, mcpSettingsPath, input, true, true); w.Code != 409 {
		t.Fatal("stale update", w.Code)
	}
	create := map[string]any{"name": "Inspector", "hostIds": []string{"local"}, "expiresInDays": 30, "expectedResourceVersion": s.mcp.access.Snapshot().ResourceVersion}
	w := request(http.MethodPost, mcpSettingsPath+"/clients", create, true, true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Client mcpaccess.Client
		Token  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Token == "" {
		t.Fatal("missing one-time token", err)
	}
	w = request(http.MethodGet, mcpSettingsPath, nil, true, false)
	if strings.Contains(w.Body.String(), result.Token) || strings.Contains(w.Body.String(), "digest") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credential leaked in list/cache")
	}
	w = request(http.MethodDelete, mcpSettingsPath+"/clients/"+result.Client.ID, map[string]string{"expectedResourceVersion": s.mcp.access.Snapshot().ResourceVersion}, true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if mcpRPC(s, result.Token, mcpInfoCall).Code != 401 {
		t.Fatal("revocation failed")
	}
}

func TestMCPBatchesUnknownWritesAndMalformedAgentResponses(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := mcpTestClient(t, s, true)
	for _, body := range []string{"[" + mcpInfoCall + "," + mcpInfoCall + "]", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell","arguments":{"command":"id"}}}`} {
		w := mcpRPC(s, token, body)
		if w.Code == 200 && !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatalf("batch/write accepted: %s", w.Body.String())
		}
	}
	s.agent = mcpTestAgent{get: func(context.Context, string) (AgentResponse, error) {
		return AgentResponse{StatusCode: 200, Body: []byte("invalid SECRET=private")}, nil
	}}
	w := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sites_list","arguments":{"hostId":"local"}}}`)
	if !strings.Contains(w.Body.String(), "invalid_host_response") || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Body.String())
	}
	buffer := &mcpResponseBuffer{header: make(http.Header)}
	if _, err := buffer.Write(make([]byte, mcpMaxResponse+1)); err == nil || !buffer.overflow || buffer.body.Len() != 0 {
		t.Fatal("response limit bypass")
	}
}
