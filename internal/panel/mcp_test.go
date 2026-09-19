package panel

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
	"github.com/kejilion/kejilion-panel/internal/store"
)

func mcpTestClient(t *testing.T, s *Server, local bool) (mcpaccess.Client, string) {
	t.Helper()
	if _, err := s.mcp.access.SetEnabled(true, s.mcp.access.Snapshot().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	h, err := s.cluster.Host(context.Background(), "local")
	if err != nil {
		t.Fatal(err)
	}
	grant := mcpaccess.HostGrant{ID: h.ID, Identity: s.mcpHostIdentity(h)}
	if !local {
		grant.ID = "removed-host"
	}
	c, token, err := s.mcp.access.Create("Test client", []mcpaccess.HostGrant{grant}, time.Hour, s.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	return c, token
}
func mcpRPC(s *Server, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://panel.test/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

const mcpInfoCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kpanel_info","arguments":{}}}`

func TestMCPTransportProtocolAndCredentialIsolation(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	session, _ := bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	if w := mcpRPC(s, "", mcpInfoCall); w.Code != 404 {
		t.Fatalf("default endpoint: %d", w.Code)
	}
	c, token := mcpTestClient(t, s, true)
	if w := mcpRPC(s, "", mcpInfoCall); w.Code != 401 {
		t.Fatalf("unauthenticated: %d %s", w.Code, w.Body.String())
	}
	if w := mcpRPC(s, session.Value, mcpInfoCall); w.Code != 401 {
		t.Fatal("Panel session accepted as MCP credential")
	}
	if w := performRequest(s, "GET", "/api/v1/auth/session", nil, map[string]string{"Authorization": "Bearer " + token}); w.Code != 401 {
		t.Fatal("MCP credential accepted as Panel session")
	}
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, mcpInfoCall,
	} {
		w := mcpRPC(s, token, body)
		if w.Code != 200 || strings.Contains(w.Body.String(), `"error"`) || strings.Contains(w.Body.String(), `"isError":true`) || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("RPC failed: %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Mcp-Session-Id") != "" {
			t.Fatal("server retained protocol session")
		}
	}
	// Security entrance remains a browser boundary; exact /mcp uses its own auth.
	_, revision := s.store.SecurityEntrance()
	if err := s.store.ReplaceSecurityEntrance(revision, store.SecurityEntrance{Enabled: true, Path: "hidden-entry"}); err != nil {
		t.Fatal(err)
	}
	if w := mcpRPC(s, token, mcpInfoCall); w.Code != 200 {
		t.Fatalf("security entrance blocked authenticated MCP: %s", w.Body.String())
	}
	if _, err := s.mcp.access.Revoke(c.ID, s.mcp.access.Snapshot().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if w := mcpRPC(s, token, mcpInfoCall); w.Code != 401 {
		t.Fatal("revoked token accepted")
	}
}

func TestMCPOriginTLSRequestLimitsAndHostScope(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := mcpTestClient(t, s, false)
	for _, tc := range []struct {
		name, origin, query string
		insecure            bool
		expected            int
	}{
		{"cross-origin", "https://evil.example", "", false, 403},
		{"null-origin", "null", "", false, 403},
		{"query-token", "", "?token=x", false, 400},
		{"plain-http", "", "", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://panel.test/mcp"+tc.query, strings.NewReader(mcpInfoCall))
			r.Header.Set("Authorization", "Bearer "+token)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.insecure {
				r.TLS = nil
				s.config.SecureCookie = true
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.expected {
				t.Fatalf("got %d want %d", w.Code, tc.expected)
			}
		})
	}
	if w := mcpRPC(s, token, strings.Repeat("a", mcpMaxRequest+1)); w.Code != 413 {
		t.Fatalf("request limit: %d", w.Code)
	}
	list := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if strings.Contains(list.Body.String(), "containers_list") || !strings.Contains(list.Body.String(), "host_summary") {
		t.Fatalf("scope discovery %s", list.Body.String())
	}
	w := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"host_summary","arguments":{"hostId":"local"}}}`)
	if !strings.Contains(w.Body.String(), "host_not_authorized") {
		t.Fatalf("scope bypass %s", w.Body.String())
	}
	w = mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"hosts_list","arguments":{}}}`)
	if !strings.Contains(w.Body.String(), `"partial":true`) || !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatalf("removed hosts %s", w.Body.String())
	}
}

type mcpTestAgent struct {
	get func(context.Context, string) (AgentResponse, error)
}

func (a mcpTestAgent) Get(ctx context.Context, path, query, id string) (AgentResponse, error) {
	return a.get(ctx, path)
}
func (a mcpTestAgent) Do(context.Context, string, string, string, string, []byte) (AgentResponse, error) {
	panic("MCP must never write to Agent")
}

func TestMCPProjectionPaginationAndRevokeDuringRead(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	c, token := mcpTestClient(t, s, true)
	s.agent = mcpTestAgent{get: func(ctx context.Context, path string) (AgentResponse, error) {
		if path != "/v1/docker/containers" {
			t.Fatalf("unexpected Agent path %s", path)
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[{"id":"b","name":"two","state":"running","mounts":[{"source":"/root/private"}],"env":{"SECRET":"do-not-expose"}},{"id":"a","name":"one","image":"nginx","state":"running"}]}`)}, nil
	}}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"containers_list","arguments":{"hostId":"local","limit":1}}}`
	w := mcpRPC(s, token, body)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "do-not-expose") || !strings.Contains(w.Body.String(), `"nextOffset":1`) || !strings.Contains(w.Body.String(), `"id":"a"`) {
		t.Fatalf("projection failed: %s", w.Body.String())
	}
	for _, args := range []string{`{"hostId":"local","command":"id"}`, `{"hostId":"local","limit":51}`, `{"hostId":"local","offset":-1}`, `null`} {
		w = mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"containers_list","arguments":`+args+`}}`)
		if !strings.Contains(w.Body.String(), "invalid_arguments") {
			t.Fatalf("invalid input accepted: %s", w.Body.String())
		}
	}
	s.agent = mcpTestAgent{get: func(context.Context, string) (AgentResponse, error) {
		_, err := s.mcp.access.Revoke(c.ID, s.mcp.access.Snapshot().ResourceVersion)
		if err != nil {
			t.Fatal(err)
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[{"id":"hidden-after-revoke"}]}`)}, nil
	}}
	w = mcpRPC(s, token, body)
	if w.Code != 401 || strings.Contains(w.Body.String(), "hidden-after-revoke") {
		t.Fatalf("revoked in-flight result leaked: %d %s", w.Code, w.Body.String())
	}
}

func TestMCPTransportLoopbackAndIdentityBinding(t *testing.T) {
	s, path := newTestServer(t)
	bootstrapCookies(t, s, path)
	for _, tc := range []struct {
		host, peer string
		allowed    bool
	}{
		{"127.0.0.1:8123", "127.0.0.1:1234", true}, {"localhost:8123", "[::1]:1234", true},
		{"evil.test", "127.0.0.1:1234", false}, {"127.0.0.1:8123", "192.0.2.1:1234", false},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/mcp", nil)
		r.RemoteAddr = tc.peer
		if s.mcpTransportAllowed(r) != tc.allowed {
			t.Fatal(tc)
		}
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		if s.mcpTransportAllowed(r) {
			t.Fatal("untrusted forwarded HTTP accepted")
		}
		r.TLS = &tls.ConnectionState{}
		if !s.mcpTransportAllowed(r) {
			t.Fatal("TLS rejected")
		}
	}
	h := cluster.Host{ID: "remote", PeerFingerprint: "first", CreatedAt: time.Now()}
	a := s.mcpHostIdentity(h)
	h.PeerFingerprint = "second"
	if a == s.mcpHostIdentity(h) {
		t.Fatal("identity not bound to fingerprint")
	}
	if panelBackupPath("mcp-access/access.json") {
		t.Fatal("credentials included in portable backup")
	}
}
