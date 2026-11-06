package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

type fileHandlerBridge struct{ handler http.Handler }

func (b fileHandlerBridge) Get(ctx context.Context, path, query, _ string) (AgentResponse, error) {
	request := httptest.NewRequest(http.MethodGet, path+"?"+query, nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	b.handler.ServeHTTP(recorder, request)
	return AgentResponse{StatusCode: recorder.Code, Body: recorder.Body.Bytes()}, nil
}

func (b fileHandlerBridge) Do(context.Context, string, string, string, string, []byte) (AgentResponse, error) {
	return AgentResponse{}, errors.New("unexpected operation")
}

func qualifiedTestTail(content string) []byte {
	body, _ := json.Marshal(map[string]any{"path": "/home/web/test.log", "content": content, "sizeBytes": len(content), "resourceVersion": "sha256:" + strings.Repeat("a", 64), "truncated": false, "projection": "full-context-v1"})
	return body
}

func TestAIToolUnavailableProjectionIsSafe(t *testing.T) {
	for _, body := range []string{`{"content":"CONFIG_SECRET"`, strings.Repeat("x", (512<<10)+1)} {
		for _, tool := range []struct{ name, arguments string }{{"host_file_read", `{"path":"/tmp/example"}`}, {"host_system_summary", `{}`}} {
			server, _ := newTestServer(t)
			server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: []byte(body)}}
			tools := &panelAITools{server: server}
			result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, tool.name, json.RawMessage(tool.arguments))
			if result != "" || !errors.Is(err, ai.ErrToolResultUnavailable) || strings.Contains(err.Error(), "CONFIG_SECRET") {
				t.Fatalf("tool=%s projection failure result=%q err=%v", tool.name, result, err)
			}
		}
	}
}

func TestApprovedAIFileReadDoesNotExposeKubernetesSecretData(t *testing.T) {
	content := `{"apiVersion":"v1","kind":"Secret","data":{"tls.key":"U1lOVEhFVElDX1BSSVZBVEU="}}`
	body, _ := json.Marshal(map[string]any{"path": "/tmp/kube.json", "content": content})
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	tools := &panelAITools{server: server}
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_read", json.RawMessage(`{"path":"/tmp/kube.json"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "U1lOVEhFVElDX1BSSVZBVEU=") {
		t.Fatal("synthetic secret remains in tool result")
	}
}

func TestApprovedAIReadRedactsUnicodeFoldedJSONKey(t *testing.T) {
	for _, content := range []string{`{"ſecret":"OPAQUE_BODY"}`, "{\"pass\u200bword\":\"OPAQUE_BODY\"}"} {
		body, _ := json.Marshal(map[string]any{"path": "/tmp/fixture.json", "content": content})
		server, _ := newTestServer(t)
		server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
		result, err := (&panelAITools{server: server}).Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_read", json.RawMessage(`{"path":"/tmp/fixture.json"}`))
		if err != nil || strings.Contains(result, "OPAQUE_BODY") {
			t.Fatalf("unicode key exposed result=%q err=%v", result, err)
		}
	}
}

func TestApprovedAIReadRedactsEscapedYAMLCredentialKey(t *testing.T) {
	content := `- "pass\u0077ord": OPAQUE_SYNTHETIC` + "\n  safe: ordinary-visible\n"
	body, _ := json.Marshal(map[string]any{"path": "/tmp/fixture.yaml", "content": content})
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	tools := &panelAITools{server: server}
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_read", json.RawMessage(`{"path":"/tmp/fixture.yaml"}`))
	if err != nil || strings.Contains(result, "OPAQUE_SYNTHETIC") || !strings.Contains(result, "ordinary-visible") {
		t.Fatalf("escaped credential projection failed: %v", err)
	}
}

func TestAIFileProjectionOrdinarySource(t *testing.T) {
	cases := map[string]string{
		"go":                  "func main() {\n    println(\"hello\")\n}\n",
		"nginx":               "server {\n    listen 80;\n}\n",
		"nginx_tls":           "server {\n    listen 443 ssl;\n    server_name example.test;\n    ssl_certificate /etc/nginx/certs/example.test_cert.pem;\n    ssl_certificate_key /etc/nginx/certs/example.test_key.pem;\n    location / {\n        proxy_pass http://127.0.0.1:8080;\n    }\n}\n",
		"nginx_php":           "server {\n    listen 443 ssl;\n    location / { try_files $uri $uri/ /index.php?$args; }\n    location ~ \\.php$ {\n        fastcgi_pass unix:/run/php/php-fpm.sock;\n    }\n}\n",
		"nginx_main":          "worker_processes auto;\nevents {\n    worker_connections 1024;\n}\nhttp {\n    server_tokens off;\n    include /etc/nginx/mime.types;\n}\n",
		"nginx_spaced_tokens": "server {\n    server_tokens\t off;\n    listen 443;\n}\n",
		"javascript":          "function hello() {\n    return 42;\n}\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"path": "/tmp/example", "content": content, "sizeBytes": len(content)})
			safe, err := mcpDisplayJSON(raw)
			if err != nil {
				t.Fatalf("ordinary source projection: %v", err)
			}
			var projected map[string]any
			if err := json.Unmarshal(safe, &projected); err != nil {
				t.Fatal(err)
			}
			if projected["content"] != content {
				t.Fatalf("content changed: %q", projected["content"])
			}
		})
	}
}

func TestAIFileProjectionRejectsMalformedSecret(t *testing.T) {
	for _, content := range []string{`INFO {"passw\u006frd":"CONFIG_SECRET"`, "server {\n" + `"passw\u006frd":"CONFIG_SECRET"` + "\n}", "INFO {\nfalse\n\"CONFIG_SECRET\"\n}"} {
		raw, _ := json.Marshal(map[string]string{"content": content})
		safe, err := mcpDisplayJSON(raw)
		if err == nil && strings.Contains(string(safe), "CONFIG_SECRET") {
			t.Fatalf("secret leaked: %s", safe)
		}
	}
}

func TestApprovedAIReadProjectsNginxTLSConfig(t *testing.T) {
	content := "server {\n    listen 443 ssl;\n    server_name example.test;\n    ssl_certificate /etc/nginx/certs/example.test_cert.pem;\n    ssl_certificate_key /etc/nginx/certs/example.test_key.pem;\n}\n"
	body, err := json.Marshal(map[string]any{"path": "/home/web/conf.d/example.test.conf", "content": content, "sizeBytes": len(content), "resourceVersion": "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	server, _ := newTestServer(t)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	server.agent = agent
	tools := &panelAITools{server: server}
	arguments := json.RawMessage(`{"path":"/home/web/conf.d/example.test.conf"}`)
	if _, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin"}, "host_file_read", arguments); !errors.Is(err, ai.ErrToolRejected) {
		t.Fatalf("read without approval was not rejected: %v", err)
	}
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_read", arguments)
	if err != nil {
		t.Fatalf("approved read failed: %v", err)
	}
	if !strings.Contains(result, "ssl_certificate") || !strings.Contains(result, "example.test_cert.pem") {
		t.Fatalf("certificate path missing: %s", result)
	}
}

func TestApprovedAITailProjectsNginxNoticeLog(t *testing.T) {
	content := "2026/11/05 20:43:00 [notice] 42#42: signal process started\n"
	body := qualifiedTestTail(content)
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	tools := &panelAITools{server: server}
	args := json.RawMessage(`{"path":"/home/web/log/nginx/error.log"}`)
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", args)
	if err != nil || !strings.Contains(result, "signal process started") {
		t.Fatalf("approved log tail result=%q err=%v", result, err)
	}
}

func TestApprovedAITailRealAgentFileHandler(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("stable read leases require Linux")
	}
	root := t.TempDir()
	content := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: fixture\ndata:\n" + strings.Repeat("  filler: value\n", 200) + "  opaque: SYNTHETIC_PRIVATE\n"
	if err := os.WriteFile(filepath.Join(root, "manifest.log"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	handler, stop := agent.NewCachedFileHandler(manager, t.TempDir())
	t.Cleanup(stop)
	server, _ := newTestServer(t)
	server.agent = fileHandlerBridge{handler: handler}
	result, err := (&panelAITools{server: server}).Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", json.RawMessage(`{"path":"/manifest.log","maxBytes":1024}`))
	if err != nil || strings.Contains(result, "SYNTHETIC_PRIVATE") || !strings.Contains(result, "full-context-v1") || !strings.Contains(result, `"truncated":true`) {
		t.Fatalf("unsafe Agent-to-Panel result=%q err=%v", result, err)
	}
}

func TestApprovedAITailAndMCPRedactAuthViaRealAgent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("stable read leases require Linux")
	}
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	handler, stop := agent.NewCachedFileHandler(manager, t.TempDir())
	t.Cleanup(stop)
	bridge := fileHandlerBridge{handler: handler}
	server, _ := newTestServer(t)
	server.agent = bridge
	for _, content := range []string{"auth: OPAQUE_BODY\n", "auths:\n  registry.example:\n    auth: OPAQUE_BODY\n", "https://example.test/?auth=OPAQUE_BODY&safe=visible\n", "https://example.test/?auths=OPAQUE_BODY&safe=visible\n", `{"ſecret":"OPAQUE_BODY"}` + "\n", "{\"pass\u200bword\":\"OPAQUE_BODY\"}\n"} {
		if err := os.WriteFile(filepath.Join(root, "fixture.yaml"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := (&panelAITools{server: server}).Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", json.RawMessage(`{"path":"/fixture.yaml"}`))
		if err != nil || strings.Contains(result, "OPAQUE_BODY") || !strings.Contains(result, "[REDACTED]") {
			t.Fatalf("AI tail projection result=%q err=%v", result, err)
		}
		response, err := bridge.Get(context.Background(), "/v1/files/ai-tail", "path=%2Ffixture.yaml", "")
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("Agent response=%v err=%v", response.StatusCode, err)
		}
		body, err := mcpFileReadProjection("host_file_tail", response.Body, nil)
		if err != nil {
			t.Fatal(err)
		}
		display, err := mcpDisplayJSON(body)
		if err != nil || strings.Contains(string(display), "OPAQUE_BODY") || !strings.Contains(string(display), "[REDACTED]") {
			t.Fatalf("MCP tail projection result=%q err=%v", display, err)
		}
	}
}

func TestApprovedAITailRejectsLegacyUnprojectedAgentResult(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"path": "/tmp/log", "content": "  arbitrary: OPAQUE_SYNTHETIC\n", "truncated": true})
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	tools := &panelAITools{server: server}
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", json.RawMessage(`{"path":"/tmp/log"}`))
	if result != "" || !errors.Is(err, ai.ErrToolResultUnavailable) {
		t.Fatalf("legacy response result=%q err=%v", result, err)
	}
	events, _, err := server.store.ListAudit(10, "")
	if err != nil || len(events) == 0 || events[0].Action != "ai.tool.host_file_tail" || events[0].Result != "failure" {
		t.Fatalf("missing failure audit: %#v err=%v", events, err)
	}
}

func TestApprovedAITailRequiresAuditAvailability(t *testing.T) {
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: qualifiedTestTail("visible log")}}
	if err := server.store.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := (&panelAITools{server: server}).Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", json.RawMessage(`{"path":"/tmp/log"}`))
	if result != "" || err == nil || !strings.Contains(err.Error(), "audit unavailable") {
		t.Fatalf("result=%q err=%v", result, err)
	}
}

func TestMCPTailRequiresAgentProjection(t *testing.T) {
	policy, err := managedPolicy([]string{"files"}, false, false, []string{"/var/log"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := managedCatalog()["host_file_tail"].prepare(json.RawMessage(`{"path":"/var/log/test"}`), policy)
	if err != nil || request.Path != "/v1/files/ai-tail" {
		t.Fatalf("request=%#v err=%v", request, err)
	}
	if _, err := mcpFileReadProjection("host_file_tail", []byte(`{"content":"opaque secret","truncated":true}`), nil); err == nil {
		t.Fatal("legacy raw tail accepted")
	}
	body := qualifiedTestTail("safe result")
	if _, err := mcpFileReadProjection("host_file_tail", body, nil); err != nil {
		t.Fatalf("projected result rejected: %v", err)
	}
	if validRemoteAIFileTailProjection([]byte(`{"data":{"content":"opaque secret"},"redacted":true}`)) {
		t.Fatal("legacy remote accepted")
	}
	remote, _ := json.Marshal(map[string]any{"data": json.RawMessage(body), "observedAt": "2026-11-06T00:00:00Z", "redacted": true, "truncated": false})
	if !validRemoteAIFileTailProjection(remote) {
		t.Fatal("qualified remote rejected")
	}
	remoteExtra := append(append([]byte{}, remote[:len(remote)-1]...), []byte(`,"extra":"OPAQUE_SYNTHETIC"}`)...)
	if validRemoteAIFileTailProjection(remoteExtra) {
		t.Fatal("unexpected remote field accepted")
	}
	if validRemoteAIFileTailProjection([]byte(strings.Replace(string(remote), "2026-11-06T00:00:00Z", "OPAQUE_SYNTHETIC", 1))) {
		t.Fatal("invalid metadata accepted")
	}
	if validAIFileTailProjection([]byte(`{"content":"safe","projection":"full-context-v1"}`)) {
		t.Fatal("incomplete result accepted")
	}
	for _, field := range []string{"path", "content", "sizeBytes", "resourceVersion", "truncated", "projection"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		fields[field] = json.RawMessage("null")
		nullResult, _ := json.Marshal(fields)
		if validAIFileTailProjection(nullResult) {
			t.Fatalf("null field %s accepted", field)
		}
	}
}

func TestMCPTailRejectsLegacyAgentAndAuditsFailure(t *testing.T) {
	for _, response := range []AgentResponse{
		{StatusCode: http.StatusNotFound, Body: []byte(`{"code":"not_found"}`)},
		{StatusCode: http.StatusOK, Body: []byte(`{"content":"SYNTHETIC_PRIVATE","truncated":true}`)},
	} {
		s, path := newTestServerWithPublicURL(t, "https://panel.test")
		bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
		_, _ = s.mcp.access.SetEnabled(true, s.mcp.access.Snapshot().ResourceVersion)
		host, err := s.cluster.Host(context.Background(), "local")
		if err != nil {
			t.Fatal(err)
		}
		policy, err := managedPolicy([]string{"files"}, false, false, []string{"/home/web"})
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := s.mcp.access.CreateWithPolicy("Tail", []mcpaccess.HostGrant{{ID: host.ID, Identity: s.mcpHostIdentity(host)}}, policy, time.Hour, s.mcp.access.Snapshot().ResourceVersion)
		if err != nil {
			t.Fatal(err)
		}
		s.agent = &stubAgent{response: response}
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"host_file_tail","arguments":{"hostId":"local","path":"/home/web/test.log"}}}`
		result := mcpRPC(s, token, body).Body.String()
		if strings.Contains(result, "SYNTHETIC_PRIVATE") || !strings.Contains(result, `"isError":true`) {
			t.Fatalf("unsafe result: %s", result)
		}
		events, _, err := s.store.ListAudit(20, "")
		found := false
		for _, event := range events {
			if event.Action == "mcp.tool.host_file_tail" && event.Result == "failure" {
				found = true
			}
		}
		if err != nil || !found {
			t.Fatalf("missing MCP failure audit: %#v err=%v", events, err)
		}
	}
}
