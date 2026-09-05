package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Service struct {
	Store     *Store
	Providers *ProviderService
	Runtime   AgentRuntime
	Events    *EventHub
	client    ModelClient
}

func Open(dataDir string, tools ToolExecutor) (*Service, error) {
	store, err := OpenStore(filepath.Join(dataDir, "ai.db"))
	if err != nil {
		return nil, err
	}
	fail := true
	defer func() {
		if fail {
			_ = store.Close()
		}
	}()
	count, err := store.EncryptedSecretCount(context.Background())
	if err != nil {
		return nil, err
	}
	secrets, err := OpenSecretBox(filepath.Join(dataDir, "ai-secrets.key"), count > 0)
	if err != nil {
		return nil, err
	}
	providers, err := NewProviderService(store, secrets)
	if err != nil {
		return nil, err
	}
	events := NewEventHub()
	client := NewHTTPModelClient()
	runtime, err := NewNativeRuntime(store, providers, client, tools, events)
	if err != nil {
		return nil, err
	}
	fail = false
	return &Service{Store: store, Providers: providers, Runtime: runtime, Events: events, client: client}, nil
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	if closer, ok := s.Runtime.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	return s.Store.Close()
}

func (s *Service) SyncModels(ctx context.Context, providerID string) ([]Model, error) {
	provider, err := s.Store.Provider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	key, err := s.Providers.APIKey(provider)
	if err != nil {
		return nil, err
	}
	items, err := s.client.Models(ctx, provider, key)
	if err != nil {
		return nil, err
	}
	existing, _ := s.Store.ListModels(ctx, provider.ID)
	known := make(map[string]Model, len(existing))
	for _, item := range existing {
		known[item.ModelID] = item
	}
	for index := range items {
		items[index].ProviderID = provider.ID
		items[index].Vision = true
		if previous, ok := known[items[index].ModelID]; ok {
			items[index].Reasoning = previous.Reasoning || inferredReasoning(provider.Protocol, items[index].ModelID)
		} else {
			items[index].Reasoning = inferredReasoning(provider.Protocol, items[index].ModelID)
		}
	}
	if err := s.Store.SaveModels(ctx, provider.ID, items); err != nil {
		return nil, err
	}
	return s.Store.ListModels(ctx, provider.ID)
}

func (s *Service) DeleteProvider(ctx context.Context, id string) error {
	cancelled, err := s.Store.DeleteProviderAndCancelPending(ctx, id)
	if err != nil {
		return err
	}
	if s.Events != nil {
		for _, run := range cancelled {
			s.Events.Publish(RunEvent{Type: "run.cancelled", RunID: run.ID, Data: run})
		}
	}
	return nil
}

func (s *Service) DeleteSession(ctx context.Context, userID, id string) error {
	active, err := s.Store.ActiveRun(ctx, id, userID)
	if err == nil {
		if s.Runtime == nil {
			return errors.New("AI runtime is unavailable")
		}
		if err := s.Runtime.Cancel(ctx, active.ID); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return s.Store.DeleteSession(ctx, userID, id)
}

func inferredReasoning(protocol ProviderProtocol, modelID string) bool {
	name := strings.ToLower(modelID)
	if protocol == ProtocolGemini {
		return strings.Contains(name, "2.5") || strings.Contains(name, "gemini-3")
	}
	return strings.Contains(name, "gpt-5") || strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") || strings.HasPrefix(name, "o4") || strings.Contains(name, "claude-") || strings.Contains(name, "gemini-2.5") || strings.Contains(name, "gemini-3") || strings.Contains(name, "deepseek-r1") || strings.Contains(name, "deepseek-v4") || strings.Contains(name, "qwen3")
}

func (s *Service) CreateSession(ctx context.Context, userID, providerID, modelID, title string) (Session, error) {
	title = strings.TrimSpace(title)
	if len(title) > 120 || strings.IndexFunc(title, func(r rune) bool { return r < 0x20 }) >= 0 {
		return Session{}, errors.New("session title is invalid")
	}
	provider, err := s.Store.Provider(ctx, providerID)
	if err != nil || !provider.Enabled {
		return Session{}, errors.New("provider is unavailable")
	}
	model, err := s.Store.Model(ctx, modelID)
	if err != nil || model.ProviderID != providerID || !model.Enabled {
		return Session{}, errors.New("model is unavailable")
	}
	return s.Store.CreateSession(ctx, Session{UserID: userID, Title: title, ProviderID: provider.ID, ProviderName: provider.Name, ModelID: model.ID, ModelName: model.DisplayName})
}

func (s *Service) Send(ctx context.Context, userID, sessionID, content string) (Run, error) {
	return s.SendWithAttachments(ctx, userID, sessionID, content, nil)
}

func (s *Service) SendWithAttachments(ctx context.Context, userID, sessionID, content string, attachments []Attachment) (Run, error) {
	content = strings.TrimSpace(content)
	if (content == "" && len(attachments) == 0) || len(content) > MaxUserMessageBytes {
		return Run{}, errors.New("message must contain text or attachments and text must not exceed 16 KiB")
	}
	session, err := s.Store.Session(ctx, userID, sessionID)
	if err != nil {
		return Run{}, err
	}
	if !session.ModelAvailable {
		return Run{}, errors.New("session model is unavailable")
	}
	attachments, err = validateAttachments(attachments)
	if err != nil {
		return Run{}, err
	}
	if session.Title == "新会话" {
		titleSource := content
		if titleSource == "" && len(attachments) > 0 {
			titleSource = "分析 " + attachments[0].Name
		}
		if err := s.Store.SetInitialSessionTitle(ctx, userID, session.ID, sessionTitleFromMessage(titleSource)); err != nil {
			return Run{}, err
		}
	}
	active, activeErr := s.Store.ActiveRun(ctx, session.ID, userID)
	if activeErr == nil {
		if err := s.validateAttachmentModel(ctx, active.ModelID, attachments); err != nil {
			return Run{}, err
		}
		if _, err := s.Store.AddMessage(ctx, Message{SessionID: session.ID, RunID: active.ID, Role: RoleUser, Content: content, Attachments: attachments}); err != nil {
			return Run{}, err
		}
		return active, nil
	}
	if !errors.Is(activeErr, ErrNotFound) {
		return Run{}, activeErr
	}
	if err := s.validateAttachmentModel(ctx, session.ModelID, attachments); err != nil {
		return Run{}, err
	}
	run, err := s.Store.CreateRun(ctx, Run{SessionID: session.ID, UserID: userID, ProviderID: session.ProviderID, ProviderName: session.ProviderName, ModelID: session.ModelID, ModelName: session.ModelName, ApprovalMode: session.ApprovalMode, ThinkingLevel: session.ThinkingLevel})
	if err != nil {
		return Run{}, err
	}
	if _, err := s.Store.AddMessage(ctx, Message{SessionID: session.ID, RunID: run.ID, Role: RoleUser, Content: content, Attachments: attachments}); err != nil {
		run.Status, run.ErrorCode, run.ErrorMessage = RunFailed, "message_store_failed", err.Error()
		_ = s.Store.UpdateRun(ctx, run)
		return Run{}, err
	}
	go func() {
		if err := s.Runtime.Run(context.Background(), run.ID); err != nil && !errors.Is(err, context.Canceled) {
			_ = err
		}
	}()
	return run, nil
}

func (s *Service) validateAttachmentModel(ctx context.Context, modelID string, attachments []Attachment) error {
	for _, item := range attachments {
		if item.Kind != "image" {
			continue
		}
		model, err := s.Store.Model(ctx, modelID)
		if err != nil {
			return err
		}
		if !model.Vision {
			return errors.New("selected model is not configured for image input")
		}
		break
	}
	return nil
}

func validateAttachments(items []Attachment) ([]Attachment, error) {
	if len(items) > 4 {
		return nil, errors.New("a message can contain at most 4 attachments")
	}
	allowedText := map[string]bool{".txt": true, ".log": true, ".md": true, ".json": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".conf": true, ".csv": true, ".xml": true, ".html": true, ".css": true, ".js": true, ".ts": true, ".vue": true, ".go": true, ".py": true, ".sh": true}
	allowedImages := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}
	total := 0
	validated := make([]Attachment, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(filepath.Base(item.Name))
		if name == "" || name == "." || len(name) > 160 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, errors.New("attachment name is invalid")
		}
		if len(item.Data) == 0 {
			return nil, errors.New("attachment is empty")
		}
		total += len(item.Data)
		if total > 8<<20 {
			return nil, errors.New("attachments exceed the 8 MiB message limit")
		}
		detected := strings.Split(http.DetectContentType(item.Data), ";")[0]
		kind := ""
		if allowedImages[detected] {
			if len(item.Data) > 4<<20 {
				return nil, errors.New("each image must not exceed 4 MiB")
			}
			kind = "image"
		} else if allowedText[strings.ToLower(filepath.Ext(name))] && len(item.Data) <= 512<<10 && utf8.Valid(item.Data) && !strings.ContainsRune(string(item.Data), '\x00') {
			kind, detected = "text", "text/plain"
		} else {
			return nil, errors.New("unsupported attachment; use PNG, JPEG, WebP, GIF, or UTF-8 text/code/config files")
		}
		validated = append(validated, Attachment{Name: name, MimeType: detected, Size: len(item.Data), Kind: kind, Data: append([]byte(nil), item.Data...)})
	}
	return validated, nil
}

func sessionTitleFromMessage(content string) string {
	content = strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, strings.TrimSpace(content))
	content = strings.Join(strings.Fields(content), " ")
	for _, separator := range []string{"。", "！", "？", "\n", ". ", "! ", "? "} {
		if index := strings.Index(content, separator); index > 0 {
			content = strings.TrimSpace(content[:index])
		}
	}
	characters := []rune(content)
	if len(characters) > 36 {
		content = strings.TrimSpace(string(characters[:36])) + "…"
	}
	if content == "" {
		return "新会话"
	}
	return content
}

func (s *Service) Resume(runID string, decision Decision) {
	go func() { _ = s.Runtime.Resume(context.Background(), runID, decision) }()
}

func (s *Service) Retry(ctx context.Context, userID, runID string) (Run, error) {
	previous, err := s.Store.Run(ctx, userID, runID)
	if err != nil {
		return Run{}, err
	}
	if previous.Status != RunInterrupted && previous.Status != RunFailed {
		return Run{}, ErrConflict
	}
	run, err := s.Store.CreateRun(ctx, Run{RetryOf: &previous.ID, SessionID: previous.SessionID, UserID: userID, ProviderID: previous.ProviderID, ProviderName: previous.ProviderName, ModelID: previous.ModelID, ModelName: previous.ModelName, ApprovalMode: previous.ApprovalMode, ThinkingLevel: previous.ThinkingLevel})
	if err != nil {
		return Run{}, err
	}
	go func() { _ = s.Runtime.Run(context.Background(), run.ID) }()
	return run, nil
}

func (s *Service) Propose(ctx context.Context, userID, runID string) error {
	run, err := s.Store.Run(ctx, userID, runID)
	if err != nil {
		return err
	}
	runtime, ok := s.Runtime.(interface {
		Propose(context.Context, string) error
	})
	if !ok {
		return errors.New("runtime does not support evolution proposals")
	}
	return runtime.Propose(ctx, run.ID)
}

func (s *Service) TestProvider(ctx context.Context, id string) error {
	provider, err := s.Store.Provider(ctx, id)
	if err != nil {
		return err
	}
	key, err := s.Providers.APIKey(provider)
	if err != nil {
		return err
	}
	if provider.Protocol != ProtocolAnthropic {
		_, err = s.client.Models(ctx, provider, key)
		return err
	}
	models, err := s.Store.ListModels(ctx, provider.ID)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("add an Anthropic model before testing")
	}
	var got bool
	err = s.client.Stream(ctx, provider, key, CompletionRequest{Model: models[0].ModelID, System: "Reply OK.", Messages: []ChatMessage{{Role: "user", Content: "OK"}}}, func(event CompletionEvent) error { got = got || event.Delta != ""; return nil })
	if err != nil {
		return err
	}
	if !got {
		return fmt.Errorf("provider returned no content")
	}
	return nil
}
