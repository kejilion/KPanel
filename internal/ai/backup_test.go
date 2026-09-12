package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestBackupAccessReplacesConfigurationWithoutHistory(t *testing.T) {
	ctx := context.Background()
	open := func() (*Store, *ProviderService) {
		dir := t.TempDir()
		st, err := OpenStore(filepath.Join(dir, "ai.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		box, err := OpenSecretBox(filepath.Join(dir, "ai-secrets.key"), false)
		if err != nil {
			t.Fatal(err)
		}
		p, err := NewProviderService(st, box)
		if err != nil {
			t.Fatal(err)
		}
		return st, p
	}
	source, p := open()
	key := "sk-test-secret-source"
	provider, err := p.Save(ctx, "", ProviderInput{Name: "Source", Protocol: ProtocolOpenAICompatible, BaseURL: "https://api.example.com/v1", EndpointScope: EndpointPublic, Enabled: true, APIKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.SaveModels(ctx, provider.ID, []Model{{ModelID: "model-a", DisplayName: "Model A", ContextWindow: 32000, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	value, err := p.ExportAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	for _, forbidden := range []string{"sessions", "messages", "attachments", "memories", "procedures", "runs"} {
		if bytes.Contains(raw, []byte(`"`+forbidden+`"`)) {
			t.Fatal("history entered access format", forbidden)
		}
	}
	target, q := open()
	oldKey := "sk-old-key"
	old, err := q.Save(ctx, "", ProviderInput{Name: "Old", Protocol: ProtocolOpenAICompatible, BaseURL: "https://api.example.com/v1", EndpointScope: EndpointPublic, APIKey: &oldKey})
	if err != nil {
		t.Fatal(err)
	}
	history, err := target.CreateSession(ctx, Session{UserID: "existing-user", Title: "Keep existing conversation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.RestoreAccess(ctx, value); err != nil {
		t.Fatal(err)
	}
	if session, err := target.Session(ctx, "existing-user", history.ID); err != nil || session.Title != history.Title {
		t.Fatal("target conversation was altered", err)
	}
	if _, err := target.Provider(ctx, old.ID); err == nil {
		t.Fatal("old configuration remained")
	}
	stored, err := target.Provider(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := q.APIKey(stored)
	if err != nil || restored != key {
		t.Fatal("key could not be resealed")
	}
	original, _ := source.Provider(ctx, provider.ID)
	if bytes.Equal(original.EncryptedKey, stored.EncryptedKey) {
		t.Fatal("source encryption was copied")
	}
	value.Models[0].ModelID = "bad\nmodel"
	if err := q.RestoreAccess(ctx, value); err == nil {
		t.Fatal("invalid model accepted")
	}
	if got, err := target.Provider(ctx, provider.ID); err != nil || got.ID == "" {
		t.Fatal("validation failure changed target")
	}
}
