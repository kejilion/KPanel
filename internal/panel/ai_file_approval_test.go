package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/store"
)

func TestAIFileApprovalUsesAgentComposeIdentity(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		err        error
		approval   bool
	}{
		{"active_custom_source", `{"composeSource":true}`, 200, nil, true},
		{"ordinary_config", `{"composeSource":false}`, 200, nil, false},
		{"old_agent", `{}`, 404, nil, true},
		{"missing_field", `{}`, 200, nil, true},
		{"bad_reply", `{"composeSource":"false"}`, 200, nil, true},
		{"unavailable", ``, 0, errors.New("unavailable"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			agent := &stubAgent{response: AgentResponse{StatusCode: test.status, Body: []byte(test.body)}, err: test.err}
			server := &Server{agent: agent}
			server.hostOps = newHostOperationService(server)
			tools := &panelAITools{server: server}
			got := tools.RequiresApproval(context.Background(), "host_file_write", json.RawMessage(`{"path":"/home/docker/demo/./prod-stack.conf"}`))
			if got != test.approval {
				t.Fatalf("approval=%v want=%v", got, test.approval)
			}
			if len(agent.calls) != 1 || agent.calls[0].method != http.MethodGet || agent.calls[0].path != "/v1/files/write-policy" || agent.calls[0].rawQuery != "path=%2Fhome%2Fdocker%2Fdemo%2Fprod-stack.conf" {
				t.Fatalf("approval must use a read-only normalized Agent lookup: %+v", agent.calls)
			}
		})
	}
}

type approvalRuntimeClient struct {
	mu        sync.Mutex
	calls     int
	arguments json.RawMessage
}

func (c *approvalRuntimeClient) Stream(_ context.Context, _ ai.Provider, _ string, _ ai.CompletionRequest, emit func(ai.CompletionEvent) error) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		return emit(ai.CompletionEvent{Done: true, ToolCalls: []ai.ToolCall{{ID: "write-1", Name: "host_file_write", Arguments: c.arguments}}})
	}
	return emit(ai.CompletionEvent{Done: true, Delta: "done"})
}

func (*approvalRuntimeClient) Models(context.Context, ai.Provider, string) ([]ai.Model, error) {
	return nil, nil
}

func TestAIRuntimeFileApprovalBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    ai.ApprovalMode
		source  bool
		pending bool
	}{
		{"custom_compose_auto", ai.ApprovalAuto, true, true},
		{"ordinary_config_auto", ai.ApprovalAuto, false, false},
		{"ordinary_config_manual", ai.ApprovalManual, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			aiStore, err := ai.OpenStore(filepath.Join(dir, "ai.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer aiStore.Close()
			box, err := ai.OpenSecretBox(filepath.Join(dir, "ai.key"), false)
			if err != nil {
				t.Fatal(err)
			}
			providers, err := ai.NewProviderService(aiStore, box)
			if err != nil {
				t.Fatal(err)
			}
			key := "fixture-only"
			provider, err := providers.Save(ctx, "", ai.ProviderInput{Name: "fixture", Protocol: ai.ProtocolOpenAICompatible, BaseURL: "https://example.com/v1", EndpointScope: ai.EndpointPublic, APIKey: &key, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := aiStore.SaveModels(ctx, provider.ID, []ai.Model{{ModelID: "fixture", DisplayName: "Fixture", ContextWindow: 8000, ToolCalling: true, Enabled: true}}); err != nil {
				t.Fatal(err)
			}
			models, _ := aiStore.ListModels(ctx, provider.ID)
			panelStore, err := store.Open(filepath.Join(dir, "panel.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer panelStore.Close()
			policy, _ := json.Marshal(map[string]bool{"composeSource": test.source})
			agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: policy}}
			server := &Server{agent: agent, store: panelStore}
			server.hostOps = newHostOperationService(server)
			arguments, _ := json.Marshal(map[string]string{"path": "/home/docker/demo/prod-stack.conf", "content": "services: {}", "expectedResourceVersion": "sha256:" + strings.Repeat("a", 64)})
			runtime, err := ai.NewNativeRuntime(aiStore, providers, &approvalRuntimeClient{arguments: arguments}, &panelAITools{server: server}, ai.NewEventHub())
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			session, err := aiStore.CreateSession(ctx, ai.Session{UserID: "admin", ProviderID: provider.ID, ModelID: models[0].ID, ApprovalMode: test.mode})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := aiStore.AddMessage(ctx, ai.Message{SessionID: session.ID, Role: ai.RoleUser, Content: "change the file"}); err != nil {
				t.Fatal(err)
			}
			run, err := aiStore.CreateRun(ctx, ai.Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ModelID: models[0].ID, ApprovalMode: test.mode})
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Run(ctx, run.ID); err != nil {
				t.Fatal(err)
			}
			loaded, _ := aiStore.Run(ctx, "admin", run.ID)
			want := ai.RunCompleted
			if test.pending {
				want = ai.RunPendingApproval
			}
			if loaded.Status != want {
				t.Fatalf("run=%s want=%s", loaded.Status, want)
			}
			writes := 0
			for _, call := range agent.snapshotCalls() {
				if call.method == http.MethodPut && call.path == "/v1/files/content" {
					writes++
				}
				if strings.HasPrefix(call.path, "/v1/docker/") {
					t.Fatal("approval classification triggered a Docker operation")
				}
			}
			if (test.pending && writes != 0) || (!test.pending && writes != 1) {
				t.Fatalf("pending=%v writes=%d", test.pending, writes)
			}
		})
	}
}
