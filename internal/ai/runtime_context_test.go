package ai

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type contextRecordingClient struct {
	mu       sync.Mutex
	requests []CompletionRequest
}

func (c *contextRecordingClient) Stream(_ context.Context, _ Provider, _ string, request CompletionRequest, emit func(CompletionEvent) error) error {
	c.mu.Lock()
	c.requests = append(c.requests, request)
	c.mu.Unlock()
	return emit(CompletionEvent{Delta: "done", Done: true})
}
func (*contextRecordingClient) Models(context.Context, Provider, string) ([]Model, error) {
	return nil, nil
}

type contextAppendingTools struct {
	fakeTools
	once        sync.Once
	appendInput func()
}

func (tools *contextAppendingTools) Definitions() []ToolDefinition {
	tools.once.Do(tools.appendInput)
	return tools.fakeTools.Definitions()
}

func TestNativeRuntimeDoesNotCompleteInputAddedAfterContextSnapshot(t *testing.T) {
	s, providers, provider, model := runtimeFixture(t)
	defer s.Close()
	ctx := context.Background()
	session, err := s.CreateSession(ctx, Session{UserID: "admin", ProviderID: provider.ID, ModelID: model.ID})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ModelID: model.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessage(ctx, Message{SessionID: session.ID, RunID: run.ID, Role: RoleUser, Content: "first"}); err != nil {
		t.Fatal(err)
	}
	client := &contextRecordingClient{}
	tools := &contextAppendingTools{appendInput: func() {
		if _, err := s.AddMessage(ctx, Message{SessionID: session.ID, RunID: run.ID, Role: RoleUser, Content: "later"}); err != nil {
			t.Error(err)
		}
	}}
	runtime, err := NewNativeRuntime(s, providers, client, tools, NewEventHub())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.requests) != 2 {
		t.Fatalf("provider calls=%d; later input lost", len(client.requests))
	}
	for _, message := range client.requests[0].Messages {
		if message.Content == "later" {
			t.Fatal("fixture appended before snapshot")
		}
	}
	found := false
	for _, message := range client.requests[1].Messages {
		found = found || message.Content == "later"
	}
	if !found {
		t.Fatal("next request missed appended input")
	}
	stored, err := s.RunByID(ctx, run.ID)
	if err != nil || stored.Status != RunCompleted {
		t.Fatalf("run=%#v err=%v", stored, err)
	}
}

func TestNativeRuntimeAttachmentFailureIsVisibleBeforeProvider(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "unknown"}[unknown], func(t *testing.T) {
			s, providers, provider, model := runtimeFixture(t)
			defer s.Close()
			ctx := context.Background()
			session, err := s.CreateSession(ctx, Session{UserID: "admin", ProviderID: provider.ID, ModelID: model.ID})
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: provider.ID, ModelID: model.ID})
			if err != nil {
				t.Fatal(err)
			}
			if unknown {
				contextMessage(t, s, run, 3)
				if _, err := s.db.ExecContext(ctx, "UPDATE runs SET retry_of=NULL WHERE id=?", run.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				contextMessage(t, s, run, 4<<20)
				contextMessage(t, s, run, 4<<20)
				contextMessage(t, s, run, 1)
			}
			client := &contextRecordingClient{}
			events := NewEventHub()
			ch, unsubscribe := events.Subscribe(run.ID)
			defer unsubscribe()
			runtime, err := NewNativeRuntime(s, providers, client, &fakeTools{}, events)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			err = runtime.Run(ctx, run.ID)
			want, code := ErrContextAttachmentLimit, "context_attachment_limit"
			if unknown {
				want, code = ErrContextAttachmentSource, "context_attachment_source_unknown"
			}
			if !errors.Is(err, want) {
				t.Fatalf("runtime error=%v", err)
			}
			stored, err := s.RunByID(ctx, run.ID)
			if err != nil || stored.Status != RunFailed || stored.ErrorCode != code || stored.ErrorMessage == "" {
				t.Fatalf("terminal=%#v err=%v", stored, err)
			}
			client.mu.Lock()
			calls := len(client.requests)
			client.mu.Unlock()
			if calls != 0 {
				t.Fatalf("provider called %d times", calls)
			}
			found := false
			for len(ch) > 0 {
				event := <-ch
				found = found || event.Type == "run.failed"
			}
			if !found {
				t.Fatal("failure event missing")
			}
		})
	}
}
