package ai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestRetryCreationValidatesParentAndKeepsSourceImmutable(t *testing.T) {
	s, session, root := contextFixture(t)
	ctx := context.Background()
	if _, err := s.CreateRun(ctx, Run{RetryOf: &root.ID, SessionID: session.ID, UserID: "admin"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("active parent accepted: %v", err)
	}
	root.Status = RunFailed
	if err := s.UpdateRun(ctx, root); err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSession(ctx, Session{UserID: "other", ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []Run{{RetryOf: &root.ID, SessionID: session.ID, UserID: "other"}, {RetryOf: &root.ID, SessionID: other.ID, UserID: "other"}} {
		if _, err := s.CreateRun(ctx, input); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign parent accepted: %v", err)
		}
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs").Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial retries count=%d err=%v", count, err)
	}
	retry := contextRetry(t, s, root)
	empty := ""
	retry.RetryOf = &empty
	retry.Status = RunFailed
	if err := s.UpdateRun(ctx, retry); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.RunByID(ctx, retry.ID)
	if err != nil || loaded.RetryOf == nil || *loaded.RetryOf != root.ID {
		t.Fatalf("source changed via UpdateRun: %#v %v", loaded, err)
	}
}

func TestRetryMigrationPreservesLegacyUnknownAndIsIdempotent(t *testing.T) {
	s, session, root := contextFixture(t)
	ctx := context.Background()
	message := contextMessage(t, s, root, 3)
	var before []byte
	if err := s.db.QueryRowContext(ctx, "SELECT attachments_json FROM messages WHERE id=?", message.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE runs DROP COLUMN retry_of"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=10"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := s.RunByID(ctx, root.ID)
	if err != nil || loaded.RetryOf != nil {
		t.Fatalf("legacy source fabricated: %#v %v", loaded, err)
	}
	var after []byte
	if err := s.db.QueryRowContext(ctx, "SELECT attachments_json FROM messages WHERE id=?", message.ID).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatalf("legacy attachment changed: %v", err)
	}
	var versions int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=10 AND applied_at>0").Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("version10=%d %v", versions, err)
	}
	if _, err := s.ContextForRun(ctx, loaded, 131072); !errors.Is(err, ErrContextAttachmentSource) {
		t.Fatalf("unknown source accepted: %v", err)
	}
	if _, err := s.ConversationMessageMetadataPage(ctx, session.ID, 50, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE messages SET attachments_json=X'' WHERE id=?", message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ContextForRun(ctx, loaded, 131072); err != nil {
		t.Fatalf("legacy text-only rejected: %v", err)
	}
}

func TestRetryMigrationReadOnlyAndDDLFailureLeaveOldSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "ALTER TABLE runs DROP COLUMN retry_of"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=10"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	readOnlyStore := &Store{db: ro, now: time.Now}
	if err := readOnlyStore.migrate(ctx); err == nil {
		t.Fatal("read-only migration accepted")
	}
	ro.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A controlled schema conflict fails after version10's transactional INSERT.
	// This tests rollback; actual SQLITE_FULL is exercised separately.
	if _, err := db.ExecContext(ctx, "ALTER TABLE runs RENAME TO runs_original"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE VIEW runs AS SELECT * FROM runs_original"); err != nil {
		t.Fatal(err)
	}
	broken := &Store{db: db, now: time.Now}
	if err := broken.migrate(ctx); err == nil {
		t.Fatal("DDL failure accepted")
	}
	var versions int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=10").Scan(&versions); err != nil || versions != 0 {
		t.Fatalf("partial migration retained: %d %v", versions, err)
	}
	if _, err := db.ExecContext(ctx, "DROP VIEW runs"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE runs_original RENAME TO runs"); err != nil {
		t.Fatal(err)
	}
	if err := broken.migrate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRetryMigrationSQLiteFullRollsBackAtomically(t *testing.T) {
	s, _, root := contextFixture(t)
	ctx := context.Background()
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	message := contextMessage(t, s, root, 3)
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE runs DROP COLUMN retry_of"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version=10"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "CREATE TABLE migration_full_probe(data BLOB)"); err != nil {
		t.Fatal(err)
	}
	// Force the engine's real SQLITE_FULL inside the migration transaction.
	// The trigger is test-only, and the page quota avoids filling the host disk.
	if _, err := s.db.ExecContext(ctx, `CREATE TEMP TRIGGER migration_full BEFORE INSERT ON main.schema_migrations
		WHEN NEW.version=10 BEGIN INSERT INTO migration_full_probe VALUES(zeroblob(1048576)); END`); err != nil {
		t.Fatal(err)
	}
	var pages int
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", pages+1)); err != nil {
		t.Fatal(err)
	}
	err := s.migrate(ctx)
	var coded interface{ Code() int }
	if !errors.As(err, &coded) || coded.Code()&255 != 13 {
		t.Fatalf("expected real SQLITE_FULL (13), got %v", err)
	}
	for _, query := range []string{"SELECT COUNT(*) FROM schema_migrations WHERE version=10", "SELECT COUNT(*) FROM migration_full_probe", "SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name='retry_of'"} {
		var count int
		if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial migration query=%s count=%d err=%v", query, count, err)
		}
	}
	var content string
	if err := s.db.QueryRowContext(ctx, "SELECT content FROM messages WHERE id=?", message.ID).Scan(&content); err != nil || content != message.Content {
		t.Fatalf("original message changed: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER migration_full"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", pages+1024)); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
}
