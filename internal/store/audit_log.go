package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Audit history lives in its own SQLite file instead of panel-state.json.
// Rewriting the whole state file for every audit record cost ~150 ms p95 at
// the 10,000 record cap and taxed every other state write the same way
// (docs/development-quality-standard.md §5.2 review, 2026-09-19).
const (
	// MaxAuditEntries is the retention cap shared by every audit writer.
	MaxAuditEntries = 10_000
	// maxAuditEventBytes bounds one stored record. An oversized change
	// summary is replaced by a marker instead of failing the audited action.
	maxAuditEventBytes = 64 << 10
	maxAuditPageSize   = 200
	auditQueryTimeout  = 10 * time.Second
	// auditJournalLimit caps the WAL file left behind after checkpoints.
	auditJournalLimit = 4 << 20
)

var ErrAuditUnavailable = errors.New("audit storage unavailable")

type auditLog struct {
	mu     sync.Mutex
	db     *sql.DB
	path   string
	opened os.FileInfo
	closed bool
}

// auditLogPath keeps the audit database next to its state file and derives
// its name from it, so separate stores in one directory never share history.
func auditLogPath(statePath string) string {
	base := strings.TrimSuffix(filepath.Base(statePath), filepath.Ext(statePath))
	return filepath.Join(filepath.Dir(statePath), base+"-audit.db")
}

func openAuditLog(path string) (*auditLog, error) {
	// Create the file with its final permissions before SQLite touches it.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create audit database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("create audit database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect audit database: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open audit database: %w", err)
	}
	// One connection keeps every PRAGMA below in force and serializes writers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	opened, err := os.Stat(path)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect audit database: %w", err)
	}
	log := &auditLog{db: db, path: path, opened: opened}
	ctx, cancel := context.WithTimeout(context.Background(), auditQueryTimeout)
	defer cancel()
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		// Each audit record is durable before the audited action reports
		// success, matching the fsync the JSON state file performed.
		"PRAGMA synchronous=FULL",
		"PRAGMA busy_timeout=5000",
		fmt.Sprintf("PRAGMA journal_size_limit=%d", auditJournalLimit),
		"PRAGMA cache_size=-2048",
		`CREATE TABLE IF NOT EXISTS audit_events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			occurred_at INTEGER NOT NULL,
			event BLOB NOT NULL
		)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure audit database: %w", err)
		}
	}
	return log, nil
}

// close is safe to race with writers: background work that outlives the
// store gets ErrAuditUnavailable instead of touching a closed handle.
func (l *auditLog) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return l.db.Close()
}

// checkFileLocked fails closed when the database file was removed or
// replaced after open. SQLite would otherwise keep committing to the unlinked
// inode, reporting success for records nobody can ever read back.
func (l *auditLog) checkFileLocked() error {
	if l.closed {
		return fmt.Errorf("%w: audit database is closed", ErrAuditUnavailable)
	}
	current, err := os.Stat(l.path)
	if err != nil || !os.SameFile(l.opened, current) {
		return fmt.Errorf("%w: audit database file is missing or replaced", ErrAuditUnavailable)
	}
	return nil
}

func encodeAuditEvent(event AuditEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	if len(data) <= maxAuditEventBytes {
		return data, nil
	}
	event.Change = map[string]any{"truncated": true, "originalBytes": len(data)}
	return json.Marshal(event)
}

// append stores events in order and trims the oldest beyond maxEntries, all in
// one transaction. A duplicate event ID is an error for normal writers; only
// the panel-state.json migration skips IDs it already holds, which makes it
// safe to repeat after an interrupted start.
func (l *auditLog) append(events []AuditEvent, maxEntries int, skipKnownIDs bool) error {
	if len(events) == 0 {
		return nil
	}
	if maxEntries <= 0 || maxEntries > MaxAuditEntries {
		maxEntries = MaxAuditEntries
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.checkFileLocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), auditQueryTimeout)
	defer cancel()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	defer tx.Rollback()
	for _, event := range events {
		if event.ID == "" {
			return fmt.Errorf("%w: audit event ID is required", ErrInvalidRecord)
		}
		data, err := encodeAuditEvent(event)
		if err != nil {
			return fmt.Errorf("encode audit event: %w", err)
		}
		insert := `INSERT INTO audit_events(id, occurred_at, event) VALUES(?, ?, ?)`
		if skipKnownIDs {
			insert = `INSERT OR IGNORE INTO audit_events(id, occurred_at, event) VALUES(?, ?, ?)`
		} else {
			var known int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE id = ?`, event.ID).Scan(&known); err != nil {
				return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
			}
			if known > 0 {
				return fmt.Errorf("%w: audit event %s", ErrAlreadyExists, event.ID)
			}
		}
		if _, err := tx.ExecContext(ctx, insert, event.ID, event.OccurredAt.UnixNano(), data); err != nil {
			return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM audit_events WHERE seq <= (SELECT max(seq) FROM audit_events) - ?`, maxEntries,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	return nil
}

// list returns newest-first records by write order. cursor is the last event
// ID a caller received; an unknown cursor restarts from the newest record,
// as the JSON-backed listing did.
func (l *auditLog) list(limit int, cursor string) ([]AuditEvent, string, error) {
	if limit <= 0 || limit > maxAuditPageSize {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), auditQueryTimeout)
	defer cancel()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, "", fmt.Errorf("%w: audit database is closed", ErrAuditUnavailable)
	}
	before := int64(-1)
	if cursor != "" {
		err := l.db.QueryRowContext(ctx, `SELECT seq FROM audit_events WHERE id = ?`, cursor).Scan(&before)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
	}
	query := `SELECT event FROM audit_events ORDER BY seq DESC LIMIT ?`
	arguments := []any{limit + 1}
	if before >= 0 {
		query = `SELECT event FROM audit_events WHERE seq < ? ORDER BY seq DESC LIMIT ?`
		arguments = []any{before, limit + 1}
	}
	rows, err := l.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	defer rows.Close()
	items := make([]AuditEvent, 0, limit+1)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
		}
		var event AuditEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, "", fmt.Errorf("%w: decode audit event: %v", ErrAuditUnavailable, err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[limit-1].ID
	}
	return items, next, nil
}
