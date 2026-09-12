package ai

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

// AccessBackup is intentionally not a database dump. Conversation, attachment,
// run, memory and procedure tables cannot enter this representation.
type AccessBackup struct {
	Version   int              `json:"version"`
	Providers []AccessProvider `json:"providers"`
	Models    []Model          `json:"models"`
}

// WithBackupAccess opens configuration storage without starting an AI runtime.
// The caller must have stopped the panel before using this offline adapter.
func WithBackupAccess(dataDir string, fn func(*ProviderService) error) error {
	st, err := OpenStore(filepath.Join(dataDir, "ai.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	count, err := st.EncryptedSecretCount(context.Background())
	if err != nil {
		return err
	}
	secrets, err := OpenSecretBox(filepath.Join(dataDir, "ai-secrets.key"), count > 0)
	if err != nil {
		return err
	}
	providers, err := NewProviderService(st, secrets)
	if err != nil {
		return err
	}
	return fn(providers)
}

type AccessProvider struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Protocol      ProviderProtocol `json:"protocol"`
	APIMode       OpenAIAPIMode    `json:"apiMode"`
	BaseURL       string           `json:"baseUrl"`
	EndpointScope EndpointScope    `json:"endpointScope"`
	Enabled       bool             `json:"enabled"`
	APIKey        string           `json:"apiKey"`
}

func (s *ProviderService) ExportAccess(ctx context.Context) (AccessBackup, error) {
	result := AccessBackup{Version: 1, Providers: []AccessProvider{}, Models: []Model{}}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,name,protocol,api_mode,base_url,endpoint_scope,enabled,encrypted_key,
		api_key_hint,version,created_at,updated_at FROM providers ORDER BY id LIMIT 129`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		key, err := s.APIKey(p)
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Providers = append(result.Providers, AccessProvider{p.ID, p.Name, p.Protocol, p.APIMode, p.BaseURL, p.EndpointScope, p.Enabled, key})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT id,provider_id,model_id,display_name,context_window,tool_calling,vision,reasoning,enabled,is_default,created_at,updated_at FROM models ORDER BY id LIMIT 5001`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var m Model
		var created, updated int64
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelID, &m.DisplayName, &m.ContextWindow, &m.ToolCalling, &m.Vision, &m.Reasoning, &m.Enabled, &m.IsDefault, &created, &updated); err != nil {
			rows.Close()
			return result, err
		}
		m.CreatedAt, m.UpdatedAt = fromMillis(created), fromMillis(updated)
		result.Models = append(result.Models, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	if err := ValidateAccessBackup(result); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func ValidateAccessBackup(value AccessBackup) error {
	if value.Version != 1 || len(value.Providers) > 128 || len(value.Models) > 5000 {
		return errors.New("invalid AI access backup limits or version")
	}
	ids := map[string]bool{}
	validID := func(id string) bool {
		return len(id) > 0 && len(id) <= 128 && strings.IndexFunc(id, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) == -1
	}
	for _, p := range value.Providers {
		if !validID(p.ID) || ids[p.ID] || strings.TrimSpace(p.Name) == "" || len(p.Name) > 80 || len(p.APIKey) > 4096 {
			return errors.New("invalid AI provider backup")
		}
		ids[p.ID] = true
		if _, err := ValidateProviderURL(p.BaseURL, p.EndpointScope); err != nil {
			return err
		}
		if p.Protocol != ProtocolOpenAICompatible && p.Protocol != ProtocolAnthropic && p.Protocol != ProtocolGemini {
			return errors.New("unsupported AI backup protocol")
		}
		if p.Protocol == ProtocolOpenAICompatible && p.APIMode != OpenAIChatCompletions && p.APIMode != OpenAIResponses {
			return errors.New("invalid AI API mode")
		}
		if p.Protocol != ProtocolOpenAICompatible && p.APIMode != "" {
			return errors.New("invalid AI API mode")
		}
	}
	models := map[string]bool{}
	pairs := map[string]bool{}
	defaults := 0
	for _, m := range value.Models {
		pair := m.ProviderID + "\x00" + m.ModelID
		if !validID(m.ID) || models[m.ID] || !ids[m.ProviderID] || pairs[pair] || strings.TrimSpace(m.ModelID) != m.ModelID || m.ModelID == "" || strings.ContainsFunc(m.ModelID, unicode.IsControl) || len(m.ModelID) > 256 || len(m.DisplayName) > 256 || m.ContextWindow < 1024 || m.ContextWindow > 10_000_000 {
			return errors.New("invalid AI model backup")
		}
		models[m.ID] = true
		pairs[pair] = true
		if m.IsDefault {
			defaults++
		}
	}
	if defaults > 1 {
		return errors.New("multiple default AI models")
	}
	return nil
}

// RestoreAccess changes configuration only, in one transaction, and re-encrypts
// keys with the destination key. Existing histories are never deleted or copied.
func (s *ProviderService) RestoreAccess(ctx context.Context, value AccessBackup) error {
	if err := ValidateAccessBackup(value); err != nil {
		return err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE status IN ('running','queued','pending_approval')`).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrBusy
	}
	// Only configuration tables are replaced. History tables do not reference
	// these through cascading foreign keys and remain untouched.
	if _, err := tx.ExecContext(ctx, `DELETE FROM models`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM providers`); err != nil {
		return err
	}
	now := millis(s.store.now())
	for _, p := range value.Providers {
		p.BaseURL, _ = ValidateProviderURL(p.BaseURL, p.EndpointScope)
		var encrypted []byte
		if p.APIKey != "" {
			encrypted, err = s.secrets.Seal(p.ID, p.APIKey)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO providers(id,name,protocol,api_mode,base_url,endpoint_scope,enabled,encrypted_key,api_key_hint,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,1,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,protocol=excluded.protocol,api_mode=excluded.api_mode,base_url=excluded.base_url,endpoint_scope=excluded.endpoint_scope,enabled=excluded.enabled,encrypted_key=excluded.encrypted_key,api_key_hint=excluded.api_key_hint,version=providers.version+1,updated_at=excluded.updated_at`, p.ID, p.Name, p.Protocol, p.APIMode, p.BaseURL, p.EndpointScope, p.Enabled, encrypted, keyHint(p.APIKey), now, now)
		if err != nil {
			return err
		}
	}
	for _, m := range value.Models {
		if m.IsDefault {
			if _, err := tx.ExecContext(ctx, `UPDATE models SET is_default=0`); err != nil {
				return err
			}
			break
		}
	}
	for _, m := range value.Models {
		_, err = tx.ExecContext(ctx, `INSERT INTO models(id,provider_id,model_id,display_name,context_window,tool_calling,vision,reasoning,enabled,is_default,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET provider_id=excluded.provider_id,model_id=excluded.model_id,display_name=excluded.display_name,context_window=excluded.context_window,tool_calling=excluded.tool_calling,vision=excluded.vision,reasoning=excluded.reasoning,enabled=excluded.enabled,is_default=excluded.is_default,updated_at=excluded.updated_at`, m.ID, m.ProviderID, m.ModelID, m.DisplayName, m.ContextWindow, m.ToolCalling, m.Vision, m.Reasoning, m.Enabled, m.IsDefault, now, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
