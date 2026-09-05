package ai

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound      = errors.New("AI record not found")
	ErrConflict      = errors.New("AI record changed")
	ErrBusy          = errors.New("AI run is already active")
	ErrToolConflict  = errors.New("host resource version changed")
	ErrToolArguments = errors.New("tool arguments are invalid")
	ErrToolRejected  = errors.New("host operation was rejected")
)

// ToolRejectedError carries only the Agent problem fields that are safe to
// return to the model. Agent detail text is intentionally never included.
type ToolRejectedError struct {
	StatusCode int
	Code       string
	RequestID  string
}

func (e *ToolRejectedError) Error() string {
	if e == nil {
		return ErrToolRejected.Error()
	}
	message := fmt.Sprintf("Agent request rejected (HTTP %d", e.StatusCode)
	if e.Code != "" {
		message += ", " + e.Code
	}
	if e.RequestID != "" {
		message += ", requestId: " + e.RequestID
	}
	return message + ")"
}

func (*ToolRejectedError) Unwrap() error { return ErrToolRejected }

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func OpenStore(path string) (*Store, error) {
	if attachmentMetadataProjectionErr != nil {
		return nil, fmt.Errorf("register AI attachment metadata projection: %w", attachmentMetadataProjectionErr)
	}
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return nil, errors.New("AI database path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create AI database directory: %w", err)
	}
	dsn := path + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=wal&_synchronous=normal"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open AI database: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	store := &Store{db: db, now: time.Now}
	failed := true
	defer func() {
		if failed {
			_ = db.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return nil, fmt.Errorf("configure AI database: %w", err)
		}
	}
	if err := store.migrate(ctx); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `UPDATE runs SET status = 'interrupted', error_code = 'panel_restarted',
		error_message = 'Panel restarted while the run was active', updated_at = ?
		WHERE status IN ('queued', 'running')`, millis(store.now())); err != nil {
		return nil, fmt.Errorf("recover interrupted AI runs: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect AI database: %w", err)
	}
	failed = false
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start AI migration: %w", err)
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, protocol TEXT NOT NULL, api_mode TEXT NOT NULL DEFAULT 'chat_completions', base_url TEXT NOT NULL,
			endpoint_scope TEXT NOT NULL, enabled INTEGER NOT NULL, encrypted_key BLOB, api_key_hint TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS models (
			id TEXT PRIMARY KEY, provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
			model_id TEXT NOT NULL, display_name TEXT NOT NULL, context_window INTEGER NOT NULL,
			tool_calling INTEGER NOT NULL, vision INTEGER NOT NULL DEFAULT 1, reasoning INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL, is_default INTEGER NOT NULL,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(provider_id, model_id))`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL,
			provider_id TEXT NOT NULL, model_id TEXT NOT NULL, provider_name TEXT NOT NULL, model_name TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '', summary_cursor TEXT NOT NULL DEFAULT '', approval_mode TEXT NOT NULL DEFAULT 'manual', thinking_level TEXT NOT NULL DEFAULT 'medium', pinned INTEGER NOT NULL, archived INTEGER NOT NULL,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, last_message_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS sessions_user_activity ON sessions(user_id, archived, pinned, last_message_at DESC)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL, content TEXT NOT NULL,
			provider_id TEXT NOT NULL DEFAULT '', provider_name TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL DEFAULT '', model_name TEXT NOT NULL DEFAULT '', tool_call_id TEXT NOT NULL DEFAULT '', attachments_json BLOB NOT NULL DEFAULT X'',
			created_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS messages_session_created ON messages(session_id, created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE, user_id TEXT NOT NULL,
			provider_id TEXT NOT NULL, provider_name TEXT NOT NULL, model_id TEXT NOT NULL, model_name TEXT NOT NULL,
			approval_mode TEXT NOT NULL DEFAULT 'manual', thinking_level TEXT NOT NULL DEFAULT 'medium', status TEXT NOT NULL, step INTEGER NOT NULL, input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL,
			error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS runs_session_status ON runs(session_id, status, created_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS runs_one_active_per_session ON runs(session_id)
			WHERE status IN ('queued','running','pending_approval')`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL, name TEXT NOT NULL, arguments BLOB NOT NULL,
			arguments_preview TEXT NOT NULL, provider_data BLOB NOT NULL DEFAULT X'', result_preview TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
			requires_approval INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL,
			enabled INTEGER NOT NULL, retired INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS procedures (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL, condition_text TEXT NOT NULL,
			steps BLOB NOT NULL, enabled INTEGER NOT NULL, retired INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS procedure_versions (
			id TEXT PRIMARY KEY, procedure_id TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL, title TEXT NOT NULL, condition_text TEXT NOT NULL, steps BLOB NOT NULL,
			version INTEGER NOT NULL, created_at INTEGER NOT NULL, UNIQUE(procedure_id,version))`,
		`CREATE TABLE IF NOT EXISTS evolution_proposals (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, session_id TEXT NOT NULL DEFAULT '', run_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL, payload BLOB NOT NULL,
			status TEXT NOT NULL, version INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(2, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(3, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(4, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(5, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(6, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(7, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(8, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(9, 0)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(10, 0)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply AI schema: %w", err)
		}
	}
	for _, column := range []struct{ table, name, definition string }{
		{"sessions", "summary_cursor", "TEXT NOT NULL DEFAULT ''"},
		{"memories", "retired", "INTEGER NOT NULL DEFAULT 0"},
		{"procedures", "retired", "INTEGER NOT NULL DEFAULT 0"},
		{"providers", "api_mode", "TEXT NOT NULL DEFAULT 'chat_completions'"},
		{"tool_calls", "provider_data", "BLOB NOT NULL DEFAULT X''"},
		{"sessions", "approval_mode", "TEXT NOT NULL DEFAULT 'manual'"},
		{"runs", "approval_mode", "TEXT NOT NULL DEFAULT 'manual'"},
		{"models", "vision", "INTEGER NOT NULL DEFAULT 1"},
		{"models", "reasoning", "INTEGER NOT NULL DEFAULT 0"},
		{"sessions", "thinking_level", "TEXT NOT NULL DEFAULT 'medium'"},
		{"messages", "attachments_json", "BLOB NOT NULL DEFAULT X''"},
		{"runs", "thinking_level", "TEXT NOT NULL DEFAULT 'medium'"},
		{"runs", "retry_of", "TEXT DEFAULT NULL"},
	} {
		if err := ensureAIColumn(ctx, tx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 1 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 2 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 3 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE providers SET api_mode = '' WHERE protocol <> ?`, ProtocolOpenAICompatible); err != nil {
		return fmt.Errorf("normalize provider API mode: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 4 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 5 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 6 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 7 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE models SET vision = 1
		WHERE EXISTS (SELECT 1 FROM schema_migrations WHERE version = 8 AND applied_at = 0)`); err != nil {
		return fmt.Errorf("default existing AI models to vision capable: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 8 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tool_calls SET created_at = updated_at
		WHERE created_at <= 0 AND EXISTS (SELECT 1 FROM schema_migrations WHERE version = 9 AND applied_at = 0)`); err != nil {
		return fmt.Errorf("repair AI tool call timeline: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 9 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET applied_at = ? WHERE version = 10 AND applied_at = 0`, millis(s.now())); err != nil {
		return fmt.Errorf("record AI migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit AI migration: %w", err)
	}
	return nil
}

func ensureAIColumn(ctx context.Context, tx *sql.Tx, table, name, definition string) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return fmt.Errorf("inspect AI schema: %w", err)
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("inspect AI schema column: %w", err)
		}
		found = found || columnName == name
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+name+` `+definition); err != nil {
		return fmt.Errorf("migrate AI schema column %s.%s: %w", table, name, err)
	}
	return nil
}

func (s *Store) EncryptedSecretCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM providers WHERE length(encrypted_key) > 0`).Scan(&count)
	return count, err
}

func (s *Store) ListProviders(ctx context.Context) ([]Provider, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,protocol,api_mode,base_url,endpoint_scope,enabled,encrypted_key,
		api_key_hint,version,created_at,updated_at FROM providers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Provider{}
	for rows.Next() {
		item, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Provider(ctx context.Context, id string) (Provider, error) {
	item, err := scanProvider(s.db.QueryRowContext(ctx, `SELECT id,name,protocol,api_mode,base_url,endpoint_scope,enabled,
		encrypted_key,api_key_hint,version,created_at,updated_at FROM providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, ErrNotFound
	}
	return item, err
}

func (s *Store) ProviderHasActiveRun(ctx context.Context, id string) (bool, error) {
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE provider_id=? AND status IN ('queued','running','pending_approval')`, id).Scan(&active)
	return active > 0, err
}

func (s *Store) ProviderHasExecutingRun(ctx context.Context, id string) (bool, error) {
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE provider_id=? AND status IN ('queued','running')`, id).Scan(&active)
	return active > 0, err
}

func (s *Store) SaveProvider(ctx context.Context, provider Provider, expectedVersion int64) (Provider, error) {
	if provider.Protocol == ProtocolOpenAICompatible && provider.APIMode == "" {
		provider.APIMode = OpenAIChatCompletions
	}
	if provider.Protocol != ProtocolOpenAICompatible {
		provider.APIMode = ""
	}
	now := s.now().UTC()
	if provider.CreatedAt.IsZero() {
		if provider.ID == "" {
			provider.ID = newID("prv")
		}
		provider.Version = 1
		provider.APIKeySet = len(provider.EncryptedKey) > 0
		provider.CreatedAt = now
		provider.UpdatedAt = now
		_, err := s.db.ExecContext(ctx, `INSERT INTO providers(id,name,protocol,api_mode,base_url,endpoint_scope,enabled,
			encrypted_key,api_key_hint,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			provider.ID, provider.Name, provider.Protocol, provider.APIMode, provider.BaseURL, provider.EndpointScope, provider.Enabled,
			provider.EncryptedKey, provider.APIKeyHint, provider.Version, millis(now), millis(now))
		return provider, err
	}
	provider.Version = expectedVersion + 1
	provider.APIKeySet = len(provider.EncryptedKey) > 0
	provider.UpdatedAt = now
	result, err := s.db.ExecContext(ctx, `UPDATE providers SET name=?,protocol=?,api_mode=?,base_url=?,endpoint_scope=?,enabled=?,
		encrypted_key=?,api_key_hint=?,version=?,updated_at=? WHERE id=? AND version=?`,
		provider.Name, provider.Protocol, provider.APIMode, provider.BaseURL, provider.EndpointScope, provider.Enabled,
		provider.EncryptedKey, provider.APIKeyHint, provider.Version, millis(now), provider.ID, expectedVersion)
	if err != nil {
		return Provider{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return Provider{}, ErrConflict
	}
	return provider, nil
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	_, err := s.DeleteProviderAndCancelPending(ctx, id)
	return err
}

func (s *Store) DeleteProviderAndCancelPending(ctx context.Context, id string) ([]Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var executing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE provider_id=? AND status IN ('queued','running')`, id).Scan(&executing); err != nil {
		return nil, err
	}
	if executing > 0 {
		return nil, ErrBusy
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,session_id,user_id,provider_id,provider_name,model_id,model_name,
		approval_mode,thinking_level,status,step,input_tokens,output_tokens,error_code,error_message,created_at,updated_at,finished_at,retry_of
		FROM runs WHERE provider_id=? AND status=?`, id, RunPendingApproval)
	if err != nil {
		return nil, err
	}
	pending := []Run{}
	for rows.Next() {
		run, scanErr := s.scanRun(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		pending = append(pending, run)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE tool_calls SET status=?,result_preview=?,updated_at=?
		WHERE status=? AND run_id IN (SELECT id FROM runs WHERE provider_id=? AND status=?)`,
		ToolRejected, "Provider deleted before approval", millis(now), ToolPendingApproval, id, RunPendingApproval); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET status=?,error_code=?,error_message=?,updated_at=?,finished_at=?
		WHERE provider_id=? AND status=?`, RunCancelled, "provider_deleted", "Provider was deleted before approval",
		millis(now), millis(now), id, RunPendingApproval); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM providers WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return nil, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for index := range pending {
		pending[index].Status = RunCancelled
		pending[index].ErrorCode = "provider_deleted"
		pending[index].ErrorMessage = "Provider was deleted before approval"
		pending[index].UpdatedAt = now
		pending[index].FinishedAt = now
	}
	return pending, nil
}

func (s *Store) SaveModels(ctx context.Context, providerID string, models []Model) error {
	active, err := s.ProviderHasExecutingRun(ctx, providerID)
	if err != nil {
		return err
	}
	if active {
		return ErrBusy
	}
	if len(models) > 5000 {
		return errors.New("provider returned too many models")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().UTC()
	for _, model := range models {
		model.ModelID = strings.TrimSpace(model.ModelID)
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		if model.ModelID == "" || len(model.ModelID) > 256 || strings.IndexFunc(model.ModelID, unicode.IsControl) >= 0 {
			return errors.New("model ID is invalid")
		}
		if model.DisplayName == "" {
			model.DisplayName = model.ModelID
		}
		if len(model.DisplayName) > 256 || strings.IndexFunc(model.DisplayName, unicode.IsControl) >= 0 {
			return errors.New("model display name is invalid")
		}
		if model.ID == "" {
			model.ID = newID("mdl")
		}
		if model.ContextWindow <= 0 {
			model.ContextWindow = 32_000
		}
		if model.ContextWindow < 1024 || model.ContextWindow > 10_000_000 {
			return errors.New("model context window is invalid")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO models(id,provider_id,model_id,display_name,context_window,
			tool_calling,vision,reasoning,enabled,is_default,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(provider_id,model_id) DO UPDATE SET display_name=excluded.display_name,
			context_window=excluded.context_window,tool_calling=excluded.tool_calling,vision=excluded.vision,reasoning=excluded.reasoning,enabled=excluded.enabled,
			is_default=excluded.is_default,updated_at=excluded.updated_at`, model.ID, providerID, model.ModelID,
			model.DisplayName, model.ContextWindow, model.ToolCalling, model.Vision, model.Reasoning, model.Enabled, model.IsDefault, millis(now), millis(now))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListModels(ctx context.Context, providerID string) ([]Model, error) {
	query := `SELECT id,provider_id,model_id,display_name,context_window,tool_calling,vision,reasoning,enabled,is_default,created_at,updated_at FROM models`
	args := []any{}
	if providerID != "" {
		query += ` WHERE provider_id=?`
		args = append(args, providerID)
	}
	query += ` ORDER BY is_default DESC, display_name COLLATE NOCASE`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Model{}
	for rows.Next() {
		var item Model
		var created, updated int64
		if err := rows.Scan(&item.ID, &item.ProviderID, &item.ModelID, &item.DisplayName, &item.ContextWindow,
			&item.ToolCalling, &item.Vision, &item.Reasoning, &item.Enabled, &item.IsDefault, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, item.UpdatedAt = fromMillis(created), fromMillis(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Model(ctx context.Context, id string) (Model, error) {
	var item Model
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT id,provider_id,model_id,display_name,context_window,tool_calling,vision,reasoning,
		enabled,is_default,created_at,updated_at FROM models WHERE id=?`, id).Scan(&item.ID, &item.ProviderID,
		&item.ModelID, &item.DisplayName, &item.ContextWindow, &item.ToolCalling, &item.Vision, &item.Reasoning, &item.Enabled, &item.IsDefault, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, ErrNotFound
	}
	item.CreatedAt, item.UpdatedAt = fromMillis(created), fromMillis(updated)
	return item, err
}

func (s *Store) CreateSession(ctx context.Context, session Session) (Session, error) {
	now := s.now().UTC()
	session.ID = newID("ses")
	session.CreatedAt, session.UpdatedAt, session.LastMessageAt = now, now, now
	if session.Title == "" {
		session.Title = "新会话"
	}
	if !session.ApprovalMode.Valid() {
		session.ApprovalMode = ApprovalManual
	}
	if !session.ThinkingLevel.Valid() {
		session.ThinkingLevel = ThinkingMedium
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id,user_id,title,provider_id,model_id,provider_name,
		model_name,summary,approval_mode,thinking_level,pinned,archived,created_at,updated_at,last_message_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.ID, session.UserID, session.Title, session.ProviderID, session.ModelID, session.ProviderName,
		session.ModelName, session.Summary, session.ApprovalMode, session.ThinkingLevel, session.Pinned, session.Archived, millis(now), millis(now), millis(now))
	session.ModelAvailable = true
	return session, err
}

func (s *Store) Sessions(ctx context.Context, userID, search string, archived bool, limit int) ([]Session, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	pattern := "%" + strings.TrimSpace(search) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.user_id,s.title,s.provider_id,s.model_id,s.provider_name,
		s.model_name,s.summary,s.approval_mode,s.thinking_level,s.pinned,s.archived,s.created_at,s.updated_at,s.last_message_at,
		EXISTS(SELECT 1 FROM providers p JOIN models m ON m.provider_id=p.id WHERE p.id=s.provider_id AND m.id=s.model_id AND p.enabled=1 AND m.enabled=1),
		EXISTS(SELECT 1 FROM runs r WHERE r.session_id=s.id AND r.status IN ('queued','running','pending_approval')),
		COALESCE((SELECT r.id FROM runs r WHERE r.session_id=s.id AND r.status IN ('queued','running','pending_approval') ORDER BY r.created_at DESC LIMIT 1),''),
		COALESCE((SELECT r.id FROM runs r WHERE r.session_id=s.id ORDER BY r.created_at DESC LIMIT 1),''),
		COALESCE((SELECT r.status FROM runs r WHERE r.session_id=s.id ORDER BY r.created_at DESC LIMIT 1),'')
		FROM sessions s WHERE s.user_id=? AND s.archived=? AND s.title LIKE ?
		ORDER BY s.pinned DESC,s.last_message_at DESC LIMIT ?`, userID, archived, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Session{}
	for rows.Next() {
		item, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Session(ctx context.Context, userID, id string) (Session, error) {
	item, err := scanSession(s.db.QueryRowContext(ctx, `SELECT s.id,s.user_id,s.title,s.provider_id,s.model_id,
		s.provider_name,s.model_name,s.summary,s.approval_mode,s.thinking_level,s.pinned,s.archived,s.created_at,s.updated_at,s.last_message_at,
		EXISTS(SELECT 1 FROM providers p JOIN models m ON m.provider_id=p.id WHERE p.id=s.provider_id AND m.id=s.model_id AND p.enabled=1 AND m.enabled=1),
		EXISTS(SELECT 1 FROM runs r WHERE r.session_id=s.id AND r.status IN ('queued','running','pending_approval')),
		COALESCE((SELECT r.id FROM runs r WHERE r.session_id=s.id AND r.status IN ('queued','running','pending_approval') ORDER BY r.created_at DESC LIMIT 1),''),
		COALESCE((SELECT r.id FROM runs r WHERE r.session_id=s.id ORDER BY r.created_at DESC LIMIT 1),''),
		COALESCE((SELECT r.status FROM runs r WHERE r.session_id=s.id ORDER BY r.created_at DESC LIMIT 1),'')
		FROM sessions s WHERE s.user_id=? AND s.id=?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return item, err
}

func (s *Store) UpdateSession(ctx context.Context, userID, id, title, providerID, modelID, providerName, modelName string, pinned, archived *bool, approvalMode *ApprovalMode, thinkingLevel *ThinkingLevel) (Session, error) {
	if len(title) > 120 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return Session{}, errors.New("session title is invalid")
	}
	item, err := s.Session(ctx, userID, id)
	if err != nil {
		return Session{}, err
	}
	if title != "" {
		item.Title = title
	}
	if providerID != "" && modelID != "" {
		item.ProviderID, item.ModelID = providerID, modelID
		item.ProviderName, item.ModelName = providerName, modelName
	}
	if pinned != nil {
		item.Pinned = *pinned
	}
	if archived != nil {
		item.Archived = *archived
	}
	if approvalMode != nil {
		if !approvalMode.Valid() {
			return Session{}, errors.New("approval mode is invalid")
		}
		item.ApprovalMode = *approvalMode
	}
	if thinkingLevel != nil {
		if !thinkingLevel.Valid() {
			return Session{}, errors.New("thinking level is invalid")
		}
		item.ThinkingLevel = *thinkingLevel
	}
	item.UpdatedAt = s.now().UTC()
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET title=?,provider_id=?,model_id=?,provider_name=?,model_name=?,
		approval_mode=?,thinking_level=?,pinned=?,archived=?,updated_at=? WHERE id=? AND user_id=?`, item.Title, item.ProviderID, item.ModelID,
		item.ProviderName, item.ModelName, item.ApprovalMode, item.ThinkingLevel, item.Pinned, item.Archived, millis(item.UpdatedAt), id, userID)
	return item, err
}

func (s *Store) SetInitialSessionTitle(ctx context.Context, userID, id, title string) error {
	if title == "" || len(title) > 120 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return errors.New("session title is invalid")
	}
	now := s.now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET title=?,updated_at=?
		WHERE id=? AND user_id=? AND title='新会话'
		AND NOT EXISTS(SELECT 1 FROM messages WHERE session_id=? AND role='user' AND tool_call_id='')`,
		title, millis(now), id, userID, id)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, userID, id string) error {
	var active int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE session_id=? AND status IN ('queued','running','pending_approval')`, id).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return ErrBusy
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddMessage(ctx context.Context, message Message) (Message, error) {
	if message.ID == "" {
		message.ID = newID("msg")
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = s.now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	attachments, err := encodeAttachments(message.Attachments)
	if err != nil {
		return Message{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO messages(id,session_id,run_id,role,content,provider_id,provider_name,
		model_id,model_name,tool_call_id,attachments_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET content=excluded.content`, message.ID, message.SessionID,
		message.RunID, message.Role, message.Content, message.ProviderID, message.ProviderName, message.ModelID,
		message.ModelName, message.ToolCallID, attachments, millis(message.CreatedAt))
	if err != nil {
		return Message{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET last_message_at=?,updated_at=? WHERE id=?`, millis(message.CreatedAt), millis(message.CreatedAt), message.SessionID)
	if err != nil {
		return Message{}, err
	}
	return message, tx.Commit()
}

func (s *Store) Messages(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	page, err := s.MessagesPage(ctx, sessionID, limit, "")
	return page.Items, err
}

func (s *Store) ConversationMessages(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	page, err := s.ConversationMessagesPage(ctx, sessionID, limit, "")
	return page.Items, err
}

func (s *Store) ConversationMessagesPage(ctx context.Context, sessionID string, limit int, before string) (Page[Message], error) {
	return s.messagesPage(ctx, sessionID, limit, before, true, false)
}

func (s *Store) MessagesPage(ctx context.Context, sessionID string, limit int, before string) (Page[Message], error) {
	return s.messagesPage(ctx, sessionID, limit, before, false, false)
}

// ConversationMessageMetadataPage is the history presentation path. Attachment
// contents remain in storage and are never retained in the returned page.
func (s *Store) ConversationMessageMetadataPage(ctx context.Context, sessionID string, limit int, before string) (Page[Message], error) {
	return s.messagesPage(ctx, sessionID, limit, before, true, true)
}

func (s *Store) messagesPage(ctx context.Context, sessionID string, limit int, before string, conversationOnly, metadataOnly bool) (Page[Message], error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	attachmentColumn := "attachments_json"
	if metadataOnly {
		// Validate inside SQLite so the driver copies only metadata into Go.
		attachmentColumn = attachmentMetadataProjectionSQL()
	}
	columns := `id,session_id,run_id,role,content,provider_id,provider_name,model_id,model_name,tool_call_id,` + attachmentColumn + `,length(CAST(attachments_json AS BLOB)),created_at,messages.rowid`
	selection := ` FROM messages WHERE session_id=?`
	args := []any{sessionID}
	if conversationOnly {
		selection += ` AND tool_call_id='' AND role IN ('user','assistant')`
	}
	if before != "" {
		selection += ` AND (created_at,rowid)<(SELECT created_at,rowid FROM messages WHERE id=? AND session_id=?)`
		args = append(args, before, sessionID)
	}
	selection += ` ORDER BY created_at DESC,rowid DESC LIMIT ?`
	args = append(args, limit+1)
	query := `SELECT ` + columns + selection
	if metadataOnly {
		// The existing index breaks timestamp ties by id, while pagination uses
		// rowid. Sort only rowids: carrying encoded attachments through SQLite's
		// temporary sort can exhaust /tmp during overlapping history reads.
		query = `WITH selected AS MATERIALIZED (SELECT rowid AS message_rowid` + selection + `)
			SELECT ` + columns + ` FROM selected CROSS JOIN messages ON messages.rowid=selected.message_rowid`
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Page[Message]{}, err
	}
	defer rows.Close()
	items := []Message{}
	rowIDs := make(map[string]int64)
	for rows.Next() {
		var item Message
		var created, rowID int64
		// Decode before Next; neither metadata nor decoded Data aliases this row.
		var attachments sql.RawBytes
		var attachmentBytes int64
		if err := rows.Scan(&item.ID, &item.SessionID, &item.RunID, &item.Role, &item.Content, &item.ProviderID,
			&item.ProviderName, &item.ModelID, &item.ModelName, &item.ToolCallID, &attachments, &attachmentBytes, &created, &rowID); err != nil {
			return Page[Message]{}, err
		}
		if metadataOnly {
			if attachmentBytes > maxAttachmentReadBytes {
				return Page[Message]{}, errors.New("message attachment record exceeds the read limit")
			}
			item.Attachments, err = decodeAttachmentMetadataProjection(attachments)
		} else {
			item.Attachments, err = decodeAttachments(attachments)
		}
		if err != nil {
			return Page[Message]{}, err
		}
		item.CreatedAt = fromMillis(created)
		rowIDs[item.ID] = rowID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page[Message]{}, err
	}
	if metadataOnly {
		// Do not depend on the join's output order or sort attachment bodies in
		// SQL. Only the bounded metadata page remains here.
		sort.Slice(items, func(i, j int) bool {
			if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].CreatedAt.After(items[j].CreatedAt)
			}
			return rowIDs[items[i].ID] > rowIDs[items[j].ID]
		})
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return Page[Message]{Items: items, NextCursor: next}, nil
}

// ContextMessages compacts old turns once approximate usage exceeds 70% of
// the model context window, then persists a bounded summary and recent turns.
func (s *Store) ContextMessages(ctx context.Context, sessionID string, contextWindow int) ([]Message, string, error) {
	snapshot, err := s.contextSnapshot(ctx, sessionID, "", contextWindow, false)
	return snapshot.Messages, snapshot.Summary, err
}

// Summary compaction only uses message text; do not load attachment bodies that
// will immediately be discarded by appendContextSummary.
func (s *Store) contextSummaryBatch(ctx context.Context, sessionID, cursor string, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,role,content FROM messages WHERE session_id=? AND
		(?='' OR (created_at,id)>(SELECT created_at,id FROM messages WHERE id=? AND session_id=?))
		ORDER BY created_at ASC,id ASC LIMIT ?`, sessionID, cursor, cursor, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Message, 0, limit)
	for rows.Next() {
		var item Message
		if err := rows.Scan(&item.ID, &item.Role, &item.Content); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func appendContextSummary(existing string, messages []Message, limit int) string {
	var summary strings.Builder
	if existing != "" {
		summary.WriteString(existing)
		summary.WriteString("\n")
	}
	for _, item := range messages {
		content := strings.TrimSpace(item.Content)
		if len(content) > 240 {
			end := 240
			for end > 0 && !utf8.RuneStart(content[end]) {
				end--
			}
			content = content[:end] + "…"
		}
		summary.WriteString(string(item.Role))
		summary.WriteString(": ")
		summary.WriteString(content)
		summary.WriteString("\n")
	}
	value := summary.String()
	if len(value) > limit {
		start := len(value) - limit
		for start < len(value) && !utf8.RuneStart(value[start]) {
			start++
		}
		value = value[start:]
	}
	return value
}

func (s *Store) CreateRun(ctx context.Context, run Run) (Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	retryOf := ""
	if run.RetryOf != nil {
		retryOf = *run.RetryOf
	}
	if retryOf != "" {
		var status RunStatus
		err := tx.QueryRowContext(ctx, `SELECT r.status FROM runs r JOIN sessions s ON s.id=r.session_id
			WHERE r.id=? AND r.user_id=? AND r.session_id=? AND s.user_id=?`, retryOf, run.UserID, run.SessionID, run.UserID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		if err != nil {
			return Run{}, err
		}
		if status != RunFailed && status != RunInterrupted {
			return Run{}, ErrConflict
		}
	}
	run.RetryOf = &retryOf
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE session_id=? AND status IN ('queued','running','pending_approval')`, run.SessionID).Scan(&active); err != nil {
		return Run{}, err
	}
	if active > 0 {
		return Run{}, ErrBusy
	}
	if !run.ApprovalMode.Valid() {
		run.ApprovalMode = ApprovalManual
	}
	if !run.ThinkingLevel.Valid() {
		run.ThinkingLevel = ThinkingMedium
	}
	now := s.now().UTC()
	run.ID, run.Status, run.CreatedAt, run.UpdatedAt = newID("run"), RunQueued, now, now
	_, err = tx.ExecContext(ctx, `INSERT INTO runs(id,session_id,user_id,provider_id,provider_name,model_id,
		model_name,approval_mode,thinking_level,status,step,input_tokens,output_tokens,error_code,error_message,created_at,updated_at,finished_at,retry_of)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?)`, run.ID, run.SessionID, run.UserID, run.ProviderID, run.ProviderName,
		run.ModelID, run.ModelName, run.ApprovalMode, run.ThinkingLevel, run.Status, 0, 0, 0, "", "", millis(now), millis(now), retryOf)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return Run{}, ErrBusy
	}
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) Run(ctx context.Context, userID, id string) (Run, error) {
	return s.scanRun(s.db.QueryRowContext(ctx, `SELECT id,session_id,user_id,provider_id,provider_name,model_id,model_name,
		approval_mode,thinking_level,status,step,input_tokens,output_tokens,error_code,error_message,created_at,updated_at,finished_at,retry_of
		FROM runs WHERE id=? AND user_id=?`, id, userID))
}

func (s *Store) RunByID(ctx context.Context, id string) (Run, error) {
	return s.scanRun(s.db.QueryRowContext(ctx, `SELECT id,session_id,user_id,provider_id,provider_name,model_id,model_name,
		approval_mode,thinking_level,status,step,input_tokens,output_tokens,error_code,error_message,created_at,updated_at,finished_at,retry_of FROM runs WHERE id=?`, id))
}

func (s *Store) ActiveRun(ctx context.Context, sessionID, userID string) (Run, error) {
	return s.scanRun(s.db.QueryRowContext(ctx, `SELECT id,session_id,user_id,provider_id,provider_name,model_id,model_name,
		approval_mode,thinking_level,status,step,input_tokens,output_tokens,error_code,error_message,created_at,updated_at,finished_at,retry_of
		FROM runs WHERE session_id=? AND user_id=? AND status IN ('queued','running','pending_approval')
		ORDER BY created_at DESC LIMIT 1`, sessionID, userID))
}

func (s *Store) UserMessageCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=? AND role='user' AND tool_call_id=''`, sessionID).Scan(&count)
	return count, err
}

func (s *Store) CompleteRunIfUserMessageCount(ctx context.Context, run Run, expectedUserMessages int) (bool, error) {
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE runs SET status='completed',step=?,input_tokens=?,output_tokens=?,
		error_code='',error_message='',updated_at=?,finished_at=? WHERE id=? AND status='running'
		AND (SELECT COUNT(*) FROM messages WHERE session_id=? AND role='user' AND tool_call_id='')=?`,
		run.Step, run.Usage.InputTokens, run.Usage.OutputTokens, millis(now), millis(now), run.ID, run.SessionID, expectedUserMessages)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	return changed == 1, err
}

func (s *Store) scanRun(row scanner) (Run, error) {
	var item Run
	var created, updated, finished int64
	err := row.Scan(&item.ID, &item.SessionID, &item.UserID,
		&item.ProviderID, &item.ProviderName, &item.ModelID, &item.ModelName, &item.ApprovalMode, &item.ThinkingLevel, &item.Status, &item.Step,
		&item.Usage.InputTokens, &item.Usage.OutputTokens, &item.ErrorCode, &item.ErrorMessage, &created, &updated, &finished, &item.RetryOf)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	item.Usage.TotalTokens = item.Usage.InputTokens + item.Usage.OutputTokens
	item.CreatedAt, item.UpdatedAt, item.FinishedAt = fromMillis(created), fromMillis(updated), fromMillis(finished)
	return item, err
}

func (s *Store) UpdateRun(ctx context.Context, run Run) error {
	now := s.now().UTC()
	finished := int64(0)
	if run.Status == RunCompleted || run.Status == RunFailed || run.Status == RunCancelled || run.Status == RunInterrupted {
		finished = millis(now)
		run.FinishedAt = now
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status=?,step=?,input_tokens=?,output_tokens=?,error_code=?,
		error_message=?,updated_at=?,finished_at=? WHERE id=?`, run.Status, run.Step, run.Usage.InputTokens,
		run.Usage.OutputTokens, run.ErrorCode, run.ErrorMessage, millis(now), finished, run.ID)
	return err
}

func (s *Store) SaveToolCall(ctx context.Context, call ToolCall) (ToolCall, error) {
	now := s.now().UTC()
	if call.ID == "" {
		call.ID = newID("tool")
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = now
	}
	call.UpdatedAt = now
	providerData := []byte(call.ProviderData)
	if providerData == nil {
		providerData = []byte{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO tool_calls(id,run_id,session_id,name,arguments,arguments_preview,
		provider_data,result_preview,status,requires_approval,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET result_preview=excluded.result_preview,status=excluded.status,updated_at=excluded.updated_at`,
		call.ID, call.RunID, call.SessionID, call.Name, []byte(call.Arguments), call.ArgumentsPreview, providerData, call.ResultPreview,
		call.Status, call.RequiresApproval, millis(call.CreatedAt), millis(now))
	return call, err
}

func (s *Store) ToolCalls(ctx context.Context, runID string) ([]ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,run_id,session_id,name,arguments,arguments_preview,provider_data,result_preview,
		status,requires_approval,created_at,updated_at FROM tool_calls WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ToolCall{}
	for rows.Next() {
		var item ToolCall
		var raw, providerData []byte
		var created, updated int64
		if err := rows.Scan(&item.ID, &item.RunID, &item.SessionID, &item.Name, &raw, &item.ArgumentsPreview, &providerData,
			&item.ResultPreview, &item.Status, &item.RequiresApproval, &created, &updated); err != nil {
			return nil, err
		}
		item.Arguments = append(json.RawMessage(nil), raw...)
		item.ProviderData = append(json.RawMessage(nil), providerData...)
		item.CreatedAt, item.UpdatedAt = fromMillis(created), fromMillis(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ToolCall(ctx context.Context, runID, id string) (ToolCall, error) {
	var item ToolCall
	var raw, providerData []byte
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT id,run_id,session_id,name,arguments,arguments_preview,provider_data,result_preview,
		status,requires_approval,created_at,updated_at FROM tool_calls WHERE run_id=? AND id=?`, runID, id).Scan(
		&item.ID, &item.RunID, &item.SessionID, &item.Name, &raw, &item.ArgumentsPreview, &providerData, &item.ResultPreview,
		&item.Status, &item.RequiresApproval, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ToolCall{}, ErrNotFound
	}
	item.Arguments = append(json.RawMessage(nil), raw...)
	item.ProviderData = append(json.RawMessage(nil), providerData...)
	item.CreatedAt, item.UpdatedAt = fromMillis(created), fromMillis(updated)
	return item, err
}

type scanner interface{ Scan(...any) error }

func scanProvider(row scanner) (Provider, error) {
	var item Provider
	var created, updated int64
	err := row.Scan(&item.ID, &item.Name, &item.Protocol, &item.APIMode, &item.BaseURL, &item.EndpointScope, &item.Enabled,
		&item.EncryptedKey, &item.APIKeyHint, &item.Version, &created, &updated)
	item.APIKeySet = len(item.EncryptedKey) > 0
	item.CreatedAt, item.UpdatedAt = fromMillis(created), fromMillis(updated)
	return item, err
}

func scanSession(row scanner) (Session, error) {
	var item Session
	var created, updated, last int64
	err := row.Scan(&item.ID, &item.UserID, &item.Title, &item.ProviderID, &item.ModelID, &item.ProviderName,
		&item.ModelName, &item.Summary, &item.ApprovalMode, &item.ThinkingLevel, &item.Pinned, &item.Archived, &created, &updated, &last,
		&item.ModelAvailable, &item.Running, &item.ActiveRunID, &item.LastRunID, &item.LastRunStatus)
	item.CreatedAt, item.UpdatedAt, item.LastMessageAt = fromMillis(created), fromMillis(updated), fromMillis(last)
	return item, err
}

type storedAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Kind     string `json:"kind"`
	Data     string `json:"data"`
}

// A validated message contains at most 8 MiB raw data and four short metadata
// records. 12 MiB leaves room for Base64 and JSON while bounding legacy rows.
const maxAttachmentReadBytes = 12 << 20

func decodeAttachmentMetadata(data []byte) ([]Attachment, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > maxAttachmentReadBytes {
		return nil, errors.New("message attachment record exceeds the read limit")
	}
	// One extra slot rejects overflow, including a fifth null/empty object.
	// A fixed array bounds allocation even for legacy rows with many items.
	var stored [5]attachmentMetadataJSON
	stored[4].overflow = true
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode message attachments: %w", err)
	}
	items := []Attachment{}
	for _, item := range stored {
		if !item.present {
			break
		}
		size, err := item.Data.decodedSize()
		if err != nil {
			return nil, fmt.Errorf("decode message attachment data: %w", err)
		}
		items = append(items, Attachment{Name: item.Name, MimeType: item.MimeType, Kind: item.Kind, Size: size})
	}
	return items, nil
}

func encodeAttachments(items []Attachment) ([]byte, error) {
	if len(items) == 0 {
		return []byte{}, nil
	}
	stored := make([]storedAttachment, 0, len(items))
	for _, item := range items {
		stored = append(stored, storedAttachment{Name: item.Name, MimeType: item.MimeType, Kind: item.Kind, Data: base64.StdEncoding.EncodeToString(item.Data)})
	}
	return json.Marshal(stored)
}

func decodeAttachments(data []byte) ([]Attachment, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var stored []storedAttachment
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode message attachments: %w", err)
	}
	items := make([]Attachment, 0, len(stored))
	for _, item := range stored {
		raw, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil {
			return nil, fmt.Errorf("decode message attachment data: %w", err)
		}
		items = append(items, Attachment{Name: item.Name, MimeType: item.MimeType, Kind: item.Kind, Size: len(raw), Data: raw})
	}
	return items, nil
}

func newID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func millis(value time.Time) int64 { return value.UTC().UnixMilli() }

func fromMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}
