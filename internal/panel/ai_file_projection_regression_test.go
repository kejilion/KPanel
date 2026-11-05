package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/ai"
)

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

func TestAIFileProjectionOrdinarySource(t *testing.T) {
	cases := map[string]string{
		"go":         "func main() {\n    println(\"hello\")\n}\n",
		"nginx":      "server {\n    listen 80;\n}\n",
		"nginx_tls":  "server {\n    listen 443 ssl;\n    server_name example.test;\n    ssl_certificate /etc/nginx/certs/example.test_cert.pem;\n    ssl_certificate_key /etc/nginx/certs/example.test_key.pem;\n    location / {\n        proxy_pass http://127.0.0.1:8080;\n    }\n}\n",
		"nginx_php":  "server {\n    listen 443 ssl;\n    location / { try_files $uri $uri/ /index.php?$args; }\n    location ~ \\.php$ {\n        fastcgi_pass unix:/run/php/php-fpm.sock;\n    }\n}\n",
		"nginx_main": "worker_processes auto;\nevents {\n    worker_connections 1024;\n}\nhttp {\n    server_tokens off;\n    include /etc/nginx/mime.types;\n}\n",
		"javascript": "function hello() {\n    return 42;\n}\n",
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
	body, _ := json.Marshal(map[string]any{"path": "/home/web/log/nginx/error.log", "content": content})
	server, _ := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	tools := &panelAITools{server: server}
	args := json.RawMessage(`{"path":"/home/web/log/nginx/error.log"}`)
	result, err := tools.Execute(context.Background(), ai.ToolExecutionContext{UserID: "admin", Approved: true}, "host_file_tail", args)
	if err != nil || !strings.Contains(result, "signal process started") {
		t.Fatalf("approved log tail result=%q err=%v", result, err)
	}
}
