package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpContainersCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"containers_list","arguments":{"hostId":"local"}}}`

func mcpSDKRequest(token, body, protocol string) *http.Request {
	var message struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if protocol == "2026-07-28" && json.Unmarshal([]byte(body), &message) == nil && message.Params != nil {
		var envelope map[string]any
		_ = json.Unmarshal([]byte(body), &envelope)
		message.Params["_meta"] = map[string]any{mcp.MetaKeyProtocolVersion: protocol, mcp.MetaKeyClientInfo: map[string]string{"name": "regression", "version": "1"}, mcp.MetaKeyClientCapabilities: map[string]any{}}
		envelope["params"] = message.Params
		encoded, _ := json.Marshal(envelope)
		body = string(encoded)
	}
	r := httptest.NewRequest("POST", "https://panel.test/mcp", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Content-Type", "application/json")
	if protocol != "" {
		r.Header.Set("MCP-Protocol-Version", protocol)
	}
	if protocol == "2026-07-28" {
		r.Header.Set("Mcp-Method", message.Method)
		if name, ok := message.Params["name"].(string); ok {
			r.Header.Set("Mcp-Name", name)
		}
	}
	return r
}

func TestMCPRejectsBatchesBeforeDispatchForEveryProtocol(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := mcpTestClient(t, s, true)
	var calls atomic.Int32
	s.agent = mcpTestAgent{get: func(context.Context, string) (AgentResponse, error) {
		calls.Add(1)
		return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[]}`)}, nil
	}}
	for _, protocol := range []string{"", "2025-03-26", "2025-11-25", "2026-07-28"} {
		t.Run("protocol="+protocol, func(t *testing.T) {
			calls.Store(0)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, mcpSDKRequest(token, "["+mcpContainersCall+","+strings.Replace(mcpContainersCall, `"id":1`, `"id":2`, 1)+"]", protocol))
			if w.Code != http.StatusBadRequest || calls.Load() != 0 {
				t.Fatalf("batch reached SDK: status=%d agentCalls=%d", w.Code, calls.Load())
			}
		})
	}
}

func TestMCPToolPreservesHTTPDeadlineAndCancellation(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			s, path := newTestServerWithPublicURL(t, "https://panel.test")
			bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
			_, token := mcpTestClient(t, s, true)
			entered := make(chan bool, 1)
			forceStop := make(chan struct{})
			s.agent = mcpTestAgent{get: func(ctx context.Context, _ string) (AgentResponse, error) {
				deadline, ok := ctx.Deadline()
				entered <- ok && time.Until(deadline) > 0 && time.Until(deadline) <= 10*time.Second
				select {
				case <-ctx.Done():
				case <-forceStop:
				}
				return AgentResponse{}, context.Canceled
			}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := httptest.NewRecorder()
			done := make(chan struct{})
			defer func() { close(forceStop); <-done }()
			go func() {
				s.ServeHTTP(w, mcpSDKRequest(token, mcpContainersCall, protocol).WithContext(ctx))
				close(done)
			}()
			select {
			case bounded := <-entered:
				if !bounded {
					t.Error("Agent context lost the HTTP request deadline")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Agent call did not start")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("canceled request retained an active tool/session")
			}
			if w.Code != http.StatusGatewayTimeout || len(s.mcp.gate) != 0 {
				t.Fatalf("cancellation did not release request budget: status=%d active=%d", w.Code, len(s.mcp.gate))
			}
			// Both transport and per-client slots must be reusable after cancellation.
			if response := mcpRPC(s, token, mcpInfoCall); response.Code != http.StatusOK {
				t.Fatalf("request budget not recovered: %d", response.Code)
			}
		})
	}
}

func TestMCPRealLoopbackHTTPSProxyAndRebindingDefense(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := mcpTestClient(t, s, true)
	server := httptest.NewServer(s)
	defer server.Close()
	for _, tc := range []struct {
		name, host, forwarded, origin string
		status                        int
	}{
		{"trusted-https-proxy", "panel.test", "https", "https://panel.test", 200},
		{"plain-http", "panel.test", "", "", 403},
		{"dns-rebinding", "evil.test", "", "", 421},
		{"cross-origin", "panel.test", "https", "https://evil.test", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(mcpInfoCall))
			if err != nil {
				t.Fatal(err)
			}
			r.Host = tc.host
			r.Header = mcpSDKRequest(token, mcpInfoCall, "2025-11-25").Header
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-Proto", tc.forwarded)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			response, err := server.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != tc.status || (tc.status == 200 && !strings.Contains(string(body), `"permission":"inspect"`)) {
				t.Fatalf("proxy/rebinding result: status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	// A caller outside the configured proxy CIDRs cannot claim HTTPS.
	r := mcpSDKRequest(token, mcpInfoCall, "2025-11-25")
	r.TLS = nil
	r.RemoteAddr = "192.0.2.20:1234"
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("forged proxy accepted: %d", w.Code)
	}
}
