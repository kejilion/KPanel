package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedClient struct {
	mu    sync.Mutex
	calls int
	tool  string
}

func (c *scriptedClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		return emit(CompletionEvent{Done: true, ToolCalls: []ToolCall{{ID: "call_1", Name: c.tool, Arguments: json.RawMessage(`{"resourceVersion":"sha256:test"}`)}}})
	}
	if err := emit(CompletionEvent{Delta: "完成"}); err != nil {
		return err
	}
	return emit(CompletionEvent{Done: true})
}
func (*scriptedClient) Models(context.Context, Provider, string) ([]Model, error) { return nil, nil }

type repeatedFailureClient struct {
	mu       sync.Mutex
	calls    int
	failures int
	vary     bool
}

func (c *repeatedFailureClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call <= c.failures {
		arguments := json.RawMessage(`{"resourceVersion":"sha256:test"}`)
		if c.vary {
			arguments = json.RawMessage(fmt.Sprintf(`{"attempt":%d}`, call))
		}
		return emit(CompletionEvent{Done: true, ToolCalls: []ToolCall{{ID: fmt.Sprintf("call_%d", call), Name: "host_action", Arguments: arguments}}})
	}
	if err := emit(CompletionEvent{Delta: "完成"}); err != nil {
		return err
	}
	return emit(CompletionEvent{Done: true})
}

func (*repeatedFailureClient) Models(context.Context, Provider, string) ([]Model, error) {
	return nil, nil
}

type fakeTools struct {
	mu         sync.Mutex
	executed   int
	readOnly   bool
	approval   *bool
	dryRunErr  error
	executeErr error
}

func TestRedactAndLimitRemovesCommonCredentialsAndPrivateKeys(t *testing.T) {
	input := "password=hunter2 token=abc123 {\"apiKey\":\"key-value\"}\n-----BEGIN PRIVATE KEY-----\nsecret-body\n-----END PRIVATE KEY-----\n"
	result := redactAndLimit(input, 4096)
	for _, secret := range []string{"hunter2", "abc123", "key-value", "secret-body"} {
		if strings.Contains(result, secret) {
			t.Fatalf("redaction leaked %q: %s", secret, result)
		}
	}
}

func (f *fakeTools) Definitions() []ToolDefinition {
	return []ToolDefinition{{Name: "host_action", Description: "test", Schema: json.RawMessage(`{"type":"object"}`), ReadOnly: f.readOnly}}
}
func (f *fakeTools) DryRun(string, json.RawMessage) error { return f.dryRunErr }
func (f *fakeTools) RequiresApproval(string, json.RawMessage) bool {
	if f.approval != nil {
		return *f.approval
	}
	return !f.readOnly
}
func (f *fakeTools) Execute(context.Context, ToolExecutionContext, string, json.RawMessage) (string, error) {
	f.mu.Lock()
	f.executed++
	f.mu.Unlock()
	return `{"status":"ok"}`, f.executeErr
}

func TestNativeRuntimeAddsVisibleProgressBeforeSilentToolCall(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "检查"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, &fakeTools{readOnly: true}, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	messages, _ := store.Messages(context.Background(), session.ID, 50)
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	var progress *Message
	for index := range messages {
		if messages[index].Role == RoleAssistant && messages[index].Content == "正在执行下一项结构化检查。" {
			progress = &messages[index]
			break
		}
	}
	if progress == nil || len(calls) != 1 || progress.CreatedAt.After(calls[0].CreatedAt) {
		t.Fatalf("progress=%#v calls=%#v", progress, calls)
	}
}

func TestNativeRuntimeReplansAfterResourceVersionConflict(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "执行"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, &fakeTools{executeErr: ErrToolConflict}, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	if err := runtime.Resume(context.Background(), run.ID, Decision{ToolCallID: calls[0].ID, Approve: true}); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted {
		t.Fatalf("conflict should be returned to the model for replanning, status=%s", loaded.Status)
	}
	messages, _ := store.Messages(context.Background(), session.ID, 50)
	found := false
	for _, message := range messages {
		found = found || strings.Contains(message.Content, "重新读取状态")
	}
	if !found {
		t.Fatal("resource-version conflict was not added to model context")
	}
}

func TestNativeRuntimeReplansAfterInvalidToolArguments(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "inspect"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{readOnly: true, executeErr: fmt.Errorf("%w: json: unknown field ignored", ErrToolArguments)}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 1 {
		t.Fatalf("invalid arguments should be returned to the model for correction, status=%s executed=%d", loaded.Status, tools.executed)
	}
	messages, _ := store.Messages(context.Background(), session.ID, 50)
	found := false
	for _, message := range messages {
		found = found || strings.Contains(message.Content, "Schema")
	}
	if !found {
		t.Fatal("argument validation failure was not added to model context")
	}
}

func TestCompletionMessagesPreserveStableIDs(t *testing.T) {
	store, _, _, _ := runtimeFixture(t)
	defer store.Close()
	ctx := context.Background()
	session, err := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	userMessage, err := store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := store.SaveToolCall(ctx, ToolCall{ID: "call_1", RunID: run.ID, SessionID: session.ID, Name: "host_read", Arguments: json.RawMessage(`{}`), Status: ToolCompleted})
	if err != nil {
		t.Fatal(err)
	}
	toolMessage, err := store.AddMessage(ctx, Message{SessionID: session.ID, RunID: run.ID, Role: RoleTool, ToolCallID: call.ID, Content: `{"ok":true}`})
	if err != nil {
		t.Fatal(err)
	}

	messages := (&NativeRuntime{store: store}).completionMessages(ctx, []Message{userMessage, toolMessage}, toolMessage.RunID)
	if len(messages) != 3 {
		t.Fatalf("messages=%#v", messages)
	}
	if messages[0].ID != userMessage.ID || messages[0].Role != "user" {
		t.Fatalf("user message identity was not preserved: %#v", messages[0])
	}
	if messages[1].ID == "" || messages[1].ID == toolMessage.ID || len(messages[1].ToolCalls) != 1 {
		t.Fatalf("synthetic tool call message needs a distinct stable identity: %#v", messages[1])
	}
	if messages[2].ID != toolMessage.ID || messages[2].ToolCallID != call.ID {
		t.Fatalf("tool result identity was not preserved: %#v", messages[2])
	}
	if messages[0].CurrentRun || !messages[1].CurrentRun || !messages[2].CurrentRun {
		t.Fatalf("current run markers were not preserved: %#v", messages)
	}
}

func TestNativeRuntimeReplansAfterAgentBusinessRejection(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "inspect"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{readOnly: true, executeErr: &ToolRejectedError{StatusCode: 422, Code: "file_symlink_rejected", RequestID: "req-safe"}}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 1 {
		t.Fatalf("business rejection should be returned to the model for replanning, status=%s executed=%d", loaded.Status, tools.executed)
	}
	messages, _ := store.Messages(context.Background(), session.ID, 50)
	found := false
	for _, message := range messages {
		found = found || strings.Contains(message.Content, "file_symlink_rejected")
	}
	if !found {
		t.Fatal("safe Agent rejection was not added to model context")
	}
}

func TestNativeRuntimeStopsRepeatedIdenticalFailedToolCall(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "inspect"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{readOnly: true, executeErr: fmt.Errorf("%w: invalid", ErrToolArguments)}
	runtime, _ := NewNativeRuntime(store, providerService, &repeatedFailureClient{failures: 3}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err == nil {
		t.Fatal("repeated identical failures did not stop the run")
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunFailed || loaded.ErrorCode != "tool_retry_limit" || tools.executed != MaxSameToolFailures {
		t.Fatalf("status=%s code=%s executed=%d", loaded.Status, loaded.ErrorCode, tools.executed)
	}
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	failed := 0
	for _, call := range calls {
		if call.Status == ToolFailed {
			failed++
		}
	}
	if len(calls) != MaxSameToolFailures+1 || failed != len(calls) {
		t.Fatalf("repeated calls=%#v", calls)
	}
}

func TestNativeRuntimeBoundsDistinctRecoverableToolFailures(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "inspect"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{readOnly: true, executeErr: &ToolRejectedError{StatusCode: 422, Code: "request_rejected"}}
	runtime, _ := NewNativeRuntime(store, providerService, &repeatedFailureClient{failures: MaxRecoverableToolFailures, vary: true}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err == nil {
		t.Fatal("recoverable failure budget did not stop the run")
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunFailed || loaded.ErrorCode != "tool_replan_limit" || tools.executed != MaxRecoverableToolFailures {
		t.Fatalf("status=%s code=%s executed=%d", loaded.Status, loaded.ErrorCode, tools.executed)
	}
}

func TestNativeRuntimeValidatesArgumentsBeforeApprovalOrExecution(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "change"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{dryRunErr: errors.New(`json: unknown field "unexpected"`)}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 0 {
		t.Fatalf("invalid arguments reached approval or execution, status=%s executed=%d", loaded.Status, tools.executed)
	}
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	if len(calls) != 1 || calls[0].Status != ToolFailed || calls[0].RequiresApproval {
		t.Fatalf("invalid call was not rejected before approval: %#v", calls)
	}
}

func TestNativeRuntimeApprovalAndResume(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "执行"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{}
	runtime, err := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunPendingApproval || tools.executed != 0 {
		t.Fatalf("status=%s executed=%d", loaded.Status, tools.executed)
	}
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	if err := runtime.Resume(context.Background(), run.ID, Decision{ToolCallID: calls[0].ID, Approve: true}); err != nil {
		t.Fatal(err)
	}
	loaded, _ = store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 1 {
		t.Fatalf("status=%s executed=%d", loaded.Status, tools.executed)
	}
}

func TestNativeRuntimeReadOnlyToolRunsWithoutApproval(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "读取"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	tools := &fakeTools{readOnly: true}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 1 {
		t.Fatalf("status=%s executed=%d", loaded.Status, tools.executed)
	}
}

func TestNativeRuntimeApprovedClassifiedWriteRunsWithoutApproval(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "执行常规操作"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName, ApprovalMode: ApprovalAuto})
	requiresApproval := false
	tools := &fakeTools{approval: &requiresApproval}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunCompleted || tools.executed != 1 {
		t.Fatalf("status=%s executed=%d", loaded.Status, tools.executed)
	}
	calls, _ := store.ToolCalls(context.Background(), run.ID)
	if len(calls) != 1 || calls[0].RequiresApproval {
		t.Fatalf("classified write unexpectedly required approval: %#v", calls)
	}
}

func TestNativeRuntimeManualModeRequiresApprovalForClassifiedWrite(t *testing.T) {
	store, providerService, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName, ApprovalMode: ApprovalManual})
	_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, Role: RoleUser, Content: "执行常规操作"})
	run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName, ApprovalMode: ApprovalManual})
	requiresApproval := false
	tools := &fakeTools{approval: &requiresApproval}
	runtime, _ := NewNativeRuntime(store, providerService, &scriptedClient{tool: "host_action"}, tools, NewEventHub())
	defer runtime.Close()
	if err := runtime.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := store.Run(context.Background(), "admin", run.ID)
	if loaded.Status != RunPendingApproval || tools.executed != 0 {
		t.Fatalf("manual mode status=%s executed=%d", loaded.Status, tools.executed)
	}
}

type queuedClient struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (c *queuedClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		close(c.started)
		<-c.release
	}
	if err := emit(CompletionEvent{Delta: "answer"}); err != nil {
		return err
	}
	return emit(CompletionEvent{Done: true})
}

func (*queuedClient) Models(context.Context, Provider, string) ([]Model, error) { return nil, nil }

func TestServiceQueuesMessageIntoActiveSessionRun(t *testing.T) {
	store, providers, provider, model := runtimeFixture(t)
	defer store.Close()
	client := &queuedClient{started: make(chan struct{}), release: make(chan struct{})}
	runtime, err := NewNativeRuntime(store, providers, client, &fakeTools{readOnly: true}, NewEventHub())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	service := &Service{Store: store, Providers: providers, Runtime: runtime}
	session, err := service.CreateSession(context.Background(), "admin", provider.ID, model.ID, "queue")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Send(context.Background(), "admin", session.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first model request did not start")
	}
	second, err := service.Send(context.Background(), "admin", session.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("queued message created another run: %s != %s", second.ID, first.ID)
	}
	close(client.release)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		loaded, loadErr := store.Run(context.Background(), "admin", first.ID)
		if loadErr == nil && loaded.Status == RunCompleted {
			client.mu.Lock()
			calls := client.calls
			client.mu.Unlock()
			if calls != 2 {
				t.Fatalf("model calls=%d, want 2", calls)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued run did not complete")
}

type concurrencyClient struct {
	mu          sync.Mutex
	active, max int
	started     chan struct{}
	release     chan struct{}
}

func (c *concurrencyClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.active++
	if c.active > c.max {
		c.max = c.active
	}
	c.mu.Unlock()
	c.started <- struct{}{}
	<-c.release
	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	return emit(CompletionEvent{Delta: "ok", Done: true})
}

func (*concurrencyClient) Models(context.Context, Provider, string) ([]Model, error) { return nil, nil }

func TestNativeRuntimeLimitsGlobalConcurrencyToTwo(t *testing.T) {
	store, providers, provider, model := runtimeFixture(t)
	defer store.Close()
	client := &concurrencyClient{started: make(chan struct{}, 3), release: make(chan struct{})}
	runtime, err := NewNativeRuntime(store, providers, client, &fakeTools{readOnly: true}, NewEventHub())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runs := make([]Run, 0, 3)
	for index := 0; index < 3; index++ {
		session, createErr := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
		if createErr != nil {
			t.Fatal(createErr)
		}
		run, createErr := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
		if createErr != nil {
			t.Fatal(createErr)
		}
		_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, RunID: run.ID, Role: RoleUser, Content: "run"})
		runs = append(runs, run)
	}
	var wait sync.WaitGroup
	errorsOut := make(chan error, 3)
	for _, run := range runs {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()
			errorsOut <- runtime.Run(context.Background(), id)
		}(run.ID)
	}
	for index := 0; index < 2; index++ {
		select {
		case <-client.started:
		case <-time.After(2 * time.Second):
			t.Fatal("two runs did not start")
		}
	}
	select {
	case <-client.started:
		t.Fatal("third run bypassed the global concurrency limit")
	case <-time.After(100 * time.Millisecond):
	}
	close(client.release)
	wait.Wait()
	close(errorsOut)
	for runErr := range errorsOut {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
	client.mu.Lock()
	maximum := client.max
	client.mu.Unlock()
	if maximum != 2 {
		t.Fatalf("maximum concurrent requests=%d, want 2", maximum)
	}
}

type recordingRuntime struct{ started chan string }

func (r *recordingRuntime) Run(_ context.Context, id string) error       { r.started <- id; return nil }
func (*recordingRuntime) Resume(context.Context, string, Decision) error { return nil }
func (*recordingRuntime) Cancel(context.Context, string) error           { return nil }

func TestServiceRetriesInterruptedRunWithOriginalSnapshot(t *testing.T) {
	store, _, provider, model := runtimeFixture(t)
	defer store.Close()
	session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	previous, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	previous.Status = RunInterrupted
	if err := store.UpdateRun(context.Background(), previous); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{started: make(chan string, 1)}
	service := &Service{Store: store, Runtime: runtime}
	retry, err := service.Retry(context.Background(), "admin", previous.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID == previous.ID || retry.ProviderID != previous.ProviderID || retry.ModelID != previous.ModelID {
		t.Fatalf("retry did not preserve immutable provider/model snapshot: %#v", retry)
	}
	select {
	case started := <-runtime.started:
		if started != retry.ID {
			t.Fatalf("started run=%s want=%s", started, retry.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("retry runtime did not start")
	}
}

type retryClient struct {
	mu         sync.Mutex
	calls      int
	emitBefore bool
	failures   int
}

func (c *retryClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if c.emitBefore {
		if err := emit(CompletionEvent{Delta: "partial"}); err != nil {
			return err
		}
	}
	if call <= c.failures {
		return &ProviderError{Status: 503, Retryable: true, Message: "temporary"}
	}
	return emit(CompletionEvent{Delta: "done", Done: true})
}
func (*retryClient) Models(context.Context, Provider, string) ([]Model, error) { return nil, nil }

type learningClient struct{ output string }

func (c *learningClient) Stream(_ context.Context, _ Provider, _ string, _ CompletionRequest, emit func(CompletionEvent) error) error {
	if err := emit(CompletionEvent{Delta: c.output}); err != nil {
		return err
	}
	return emit(CompletionEvent{Done: true})
}
func (*learningClient) Models(context.Context, Provider, string) ([]Model, error) { return nil, nil }

func TestNativeRuntimeSilentlyLearnsReusableProcedureAcrossSessions(t *testing.T) {
	store, providers, provider, model := runtimeFixture(t)
	defer store.Close()
	ctx := context.Background()
	session, _ := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "检查并恢复应用状态"})
	run, _ := store.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, err := store.SaveToolCall(ctx, ToolCall{ID: "learn-1", RunID: run.ID, SessionID: session.ID, Name: "host_action", Arguments: json.RawMessage(`{"resourceVersion":"sha256:test"}`), Status: ToolCompleted})
	if err != nil {
		t.Fatal(err)
	}
	output := `{"decision":"procedure","confidence":0.96,"title":"应用状态恢复","content":"检查并恢复应用状态","condition":"应用异常且需要恢复时","steps":[{"tool":"host_action","arguments":{"resourceVersion":"sha256:test"}}]}`
	requiresApproval := false
	runtime, _ := NewNativeRuntime(store, providers, &learningClient{output: output}, &fakeTools{approval: &requiresApproval}, NewEventHub())
	defer runtime.Close()
	history, _, _ := store.ContextMessages(ctx, session.ID, model.ContextWindow)
	if err := runtime.generateProposal(ctx, run, provider, "test-key", model, history, false); err != nil {
		t.Fatal(err)
	}
	procedures, _ := store.Procedures(ctx, "admin")
	pending, _ := store.Proposals(ctx, "admin", EvolutionPending)
	if len(procedures) != 1 || !procedures[0].Enabled || len(pending) != 0 {
		t.Fatalf("procedures=%#v pending=%#v", procedures, pending)
	}
	if prompt := runtime.systemPrompt(ctx, "admin"); !strings.Contains(prompt, "应用状态恢复") {
		t.Fatalf("learned procedure was not available to another session: %s", prompt)
	}
	if prompt := runtime.systemPrompt(ctx, "admin"); !strings.Contains(prompt, "host_docker_resource_usage") ||
		!strings.Contains(prompt, "Never use host_docker_task for a status or resource query") ||
		!strings.Contains(prompt, "Never invent placeholder fields") ||
		!strings.Contains(prompt, "KPanel keeps only authentication, approval, typed action routing and audit boundaries") ||
		!strings.Contains(prompt, "host_docker_backups") ||
		!strings.Contains(prompt, "host_docker_environment") ||
		!strings.Contains(prompt, "image_prune removes only dangling images") {
		t.Fatalf("Docker resource routing rule is missing: %s", prompt)
	}
	if err := runtime.generateProposal(ctx, run, provider, "test-key", model, history, false); err != nil {
		t.Fatal(err)
	}
	procedures, _ = store.Procedures(ctx, "admin")
	if len(procedures) != 1 {
		t.Fatalf("duplicate learning created %d procedures", len(procedures))
	}
}

func TestNativeRuntimeSkipsLowConfidenceLearning(t *testing.T) {
	store, providers, provider, model := runtimeFixture(t)
	defer store.Close()
	ctx := context.Background()
	session, _ := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "记住当前 CPU 数值"})
	run, _ := store.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	runtime, _ := NewNativeRuntime(store, providers, &learningClient{output: `{"decision":"memory","confidence":0.4,"title":"CPU","content":"当前 CPU 为 10%"}`}, &fakeTools{}, NewEventHub())
	defer runtime.Close()
	history, _, _ := store.ContextMessages(ctx, session.ID, model.ContextWindow)
	if err := runtime.generateProposal(ctx, run, provider, "test-key", model, history, false); err != nil {
		t.Fatal(err)
	}
	memories, _ := store.Memories(ctx, "admin")
	if len(memories) != 0 {
		t.Fatalf("low-confidence transient memory became active: %#v", memories)
	}
}

func TestNativeRuntimeDoesNotSilentlyLearnProtectedProcedure(t *testing.T) {
	store, providers, provider, model := runtimeFixture(t)
	defer store.Close()
	ctx := context.Background()
	session, _ := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	_, _ = store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "执行核心维护"})
	run, _ := store.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
	for _, id := range []string{"protected-1", "protected-2"} {
		_, _ = store.SaveToolCall(ctx, ToolCall{ID: id, RunID: run.ID, SessionID: session.ID, Name: "host_action", Arguments: json.RawMessage(`{}`), Status: ToolCompleted})
	}
	output := `{"decision":"procedure","confidence":0.99,"title":"核心维护","content":"执行维护","condition":"需要维护时","steps":[{"tool":"host_action","arguments":{}}]}`
	runtime, _ := NewNativeRuntime(store, providers, &learningClient{output: output}, &fakeTools{}, NewEventHub())
	defer runtime.Close()
	history, _, _ := store.ContextMessages(ctx, session.ID, model.ContextWindow)
	if err := runtime.generateProposal(ctx, run, provider, "test-key", model, history, false); err != nil {
		t.Fatal(err)
	}
	procedures, _ := store.Procedures(ctx, "admin")
	pending, _ := store.Proposals(ctx, "admin", EvolutionPending)
	if len(procedures) != 0 || len(pending) != 0 {
		t.Fatalf("protected workflow was learned: procedures=%#v pending=%#v", procedures, pending)
	}
}

func TestRuntimeRetriesOnlyBeforeStreamOutput(t *testing.T) {
	for _, test := range []struct {
		name, wantStatus string
		emitBefore       bool
		failures, calls  int
	}{
		{name: "retry before output", wantStatus: string(RunCompleted), failures: 2, calls: 3},
		{name: "no replay after output", wantStatus: string(RunFailed), emitBefore: true, failures: 2, calls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, providers, provider, model := runtimeFixture(t)
			defer store.Close()
			session, _ := store.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
			run, _ := store.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
			_, _ = store.AddMessage(context.Background(), Message{SessionID: session.ID, RunID: run.ID, Role: RoleUser, Content: "test"})
			client := &retryClient{emitBefore: test.emitBefore, failures: test.failures}
			runtime, _ := NewNativeRuntime(store, providers, client, &fakeTools{readOnly: true}, NewEventHub())
			defer runtime.Close()
			_ = runtime.Run(context.Background(), run.ID)
			loaded, _ := store.Run(context.Background(), "admin", run.ID)
			if string(loaded.Status) != test.wantStatus || client.calls != test.calls {
				t.Fatalf("status=%s calls=%d", loaded.Status, client.calls)
			}
		})
	}
}

func runtimeFixture(t *testing.T) (*Store, *ProviderService, Provider, Model) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	box, err := OpenSecretBox(filepath.Join(dir, "ai-secrets.key"), false)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := NewProviderService(store, box)
	key := "test-key"
	provider, err := service.Save(ctx, "", ProviderInput{Name: "mock", Protocol: ProtocolOpenAICompatible, BaseURL: "https://example.com/v1", EndpointScope: EndpointPublic, APIKey: &key, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveModels(ctx, provider.ID, []Model{{ModelID: "mock-model", DisplayName: "Mock", ContextWindow: 8000, ToolCalling: true, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	models, _ := store.ListModels(ctx, provider.ID)
	return store, service, provider, models[0]
}
