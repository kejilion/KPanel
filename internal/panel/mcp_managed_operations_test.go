package panel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

type managedTestAgent struct {
	calls *atomic.Int32
	fail  bool
}

func (a managedTestAgent) Get(context.Context, string, string, string) (AgentResponse, error) {
	return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[{"id":"nginx","resourceVersion":"sha256:abc","env":{"SECRET":"hidden"}}]}`)}, nil
}
func (a managedTestAgent) Do(_ context.Context, method, path, query, id string, body []byte) (AgentResponse, error) {
	a.calls.Add(1)
	if a.fail {
		return AgentResponse{}, errors.New("lost receipt")
	}
	return AgentResponse{StatusCode: 200, Body: []byte(`{"id":"nginx","state":"running"}`)}, nil
}

func managedTestClient(t *testing.T, s *Server, write bool) (mcpaccess.Client, string) {
	t.Helper()
	_, _ = s.mcp.access.SetEnabled(true, s.mcp.access.Snapshot().ResourceVersion)
	h, err := s.cluster.Host(context.Background(), "local")
	if err != nil {
		t.Fatal(err)
	}
	p, err := managedPolicy([]string{"apps"}, write, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, token, err := s.mcp.access.CreateWithPolicy("Managed", []mcpaccess.HostGrant{{ID: h.ID, Identity: s.mcpHostIdentity(h)}}, p, time.Hour, s.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	return c, token
}

func managedCall(s *Server, token, name string, args any) map[string]any {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	w := mcpRPC(s, token, string(body))
	var response struct {
		Result struct {
			Data map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	return response.Result.Data
}

func TestMCPManagedApprovalAndIdempotentExecution(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := managedTestClient(t, s, true)
	var calls atomic.Int32
	s.agent = managedTestAgent{calls: &calls}
	list := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if !strings.Contains(list.Body.String(), `"host_app_action"`) || strings.Contains(list.Body.String(), `"host_docker_task"`) {
		t.Fatalf("discovery escaped policy: %s", list.Body.String())
	}
	args := map[string]any{"hostId": "local", "requestKey": "request-1", "appId": "builtin-nginx", "action": "restart", "resourceVersion": "sha256:" + strings.Repeat("a", 64)}
	planned := managedCall(s, token, "host_app_action", args)
	if planned["state"] != "pending" || calls.Load() != 0 {
		t.Fatalf("unapproved dispatch: %#v calls=%d", planned, calls.Load())
	}
	id, digest := planned["operationId"].(string), planned["digest"].(string)
	repeated := managedCall(s, token, "host_app_action", args)
	if repeated["operationId"] != id {
		t.Fatal("retry made another plan")
	}
	execArgs := map[string]any{"operationId": id, "digest": digest}
	if got := managedCall(s, token, "operation_execute", execArgs); got["state"] != "pending" || calls.Load() != 0 {
		t.Fatal("client self approved")
	}
	if _, err := s.mcp.operations.Decide(id, digest, "admin", true); err != nil {
		t.Fatal(err)
	}
	if got := managedCall(s, token, "operation_execute", execArgs); got["state"] != "succeeded" {
		t.Fatalf("execution: %#v", got)
	}
	managedCall(s, token, "operation_execute", execArgs)
	if calls.Load() != 1 {
		t.Fatalf("duplicate write: %d", calls.Load())
	}
	_, other := managedTestClient(t, s, true)
	if got := managedCall(s, other, "operation_status", map[string]any{"operationId": id}); got != nil {
		t.Fatal("cross-client operation exposed")
	}
}

func TestMCPManagedLostReceiptDoesNotRetry(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, token := managedTestClient(t, s, true)
	var calls atomic.Int32
	s.agent = managedTestAgent{calls: &calls, fail: true}
	planned := managedCall(s, token, "host_app_action", map[string]any{"hostId": "local", "requestKey": "request-1", "appId": "builtin-nginx", "action": "restart", "resourceVersion": "sha256:" + strings.Repeat("a", 64)})
	if planned == nil {
		t.Fatal("missing plan")
	}
	id, digest := planned["operationId"].(string), planned["digest"].(string)
	_, _ = s.mcp.operations.Decide(id, digest, "admin", true)
	args := map[string]any{"operationId": id, "digest": digest}
	if result := managedCall(s, token, "operation_execute", args); result["state"] != "unknown" {
		t.Fatalf("receipt loss called failure: %#v", result)
	}
	managedCall(s, token, "operation_execute", args)
	if calls.Load() != 1 {
		t.Fatal("repeated ambiguous write")
	}
}

func TestMCPManagedSchemaAndFileScope(t *testing.T) {
	p, err := managedPolicy([]string{"files"}, true, false, []string{"/home/web"})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"path":"/etc/shadow"}`, `{"path":"/home/web/../../etc/shadow"}`, `{"path":"/home/web/index.html","shell":"id"}`} {
		if _, err := managedCatalog()["host_file_read"].prepare(json.RawMessage(args), p); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
	if _, err := managedCatalog()["host_file_read"].prepare(json.RawMessage(`{"path":"/home/web/index.html"}`), p); err != nil {
		t.Fatal(err)
	}
	output, err := mcpManagedOutput([]byte(`{"items":[{"id":"one","resourceVersion":"version","env":{"x":"hidden"},"authorization":"bearer hidden"},{"id":"two"}]}`), 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(output)
	if strings.Contains(string(b), "hidden") || !strings.Contains(string(b), `"nextOffset":1`) || !strings.Contains(string(b), `"resourceVersion":"version"`) {
		t.Fatalf("unsafe or unusable projection: %s", b)
	}
}
