package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func auditTestEvent(id string, at time.Time) AuditEvent {
	return AuditEvent{ID: id, OccurredAt: at, ActorType: "user", Action: "file.upload", Result: "success", RequestID: "r-" + id}
}

func writeLegacyState(t *testing.T, path string, events []AuditEvent) {
	t.Helper()
	legacy := map[string]any{"schemaVersion": 1, "users": []any{}, "sessions": []any{}, "loginAttempts": []any{}, "audit": events}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func auditIDs(events []AuditEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

func TestOpenMigratesLegacyAuditOutOfStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel-state.json")
	now := time.Now().UTC()
	writeLegacyState(t, path, []AuditEvent{
		auditTestEvent("old-1", now.Add(-3*time.Minute)),
		auditTestEvent("old-2", now.Add(-2*time.Minute)),
		auditTestEvent("old-3", now.Add(-time.Minute)),
	})
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	events, next, err := storage.ListAudit(10, "")
	if err != nil || strings.Join(auditIDs(events), ",") != "old-3,old-2,old-1" || next != "" {
		t.Fatalf("migrated audit = %v next=%q err=%v", auditIDs(events), next, err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"audit"`) {
		t.Fatalf("state file still carries audit history: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "panel-state-audit.db")); err != nil {
		t.Fatalf("audit database missing: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	events, _, err = reopened.ListAudit(10, "")
	if err != nil || len(events) != 3 {
		t.Fatalf("reopened audit = %v err=%v", auditIDs(events), err)
	}
}

func TestInterruptedMigrationRepeatsWithoutDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel-state.json")
	now := time.Now().UTC()
	legacy := []AuditEvent{auditTestEvent("a", now), auditTestEvent("b", now), auditTestEvent("c", now)}
	// A previous start imported the records but crashed before clearing them
	// from the state file.
	partial, err := openAuditLog(auditLogPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := partial.append(legacy, MaxAuditEntries, true); err != nil {
		t.Fatal(err)
	}
	if err := partial.close(); err != nil {
		t.Fatal(err)
	}
	writeLegacyState(t, path, legacy)
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	events, _, err := storage.ListAudit(10, "")
	if err != nil || strings.Join(auditIDs(events), ",") != "c,b,a" {
		t.Fatalf("repeated migration = %v err=%v", auditIDs(events), err)
	}
}

func TestAuditRetentionAndCursorPaging(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	for index := range 9 {
		if err := storage.AppendAudit(auditTestEvent(fmt.Sprintf("e%d", index), now), 7); err != nil {
			t.Fatal(err)
		}
	}
	var pages []string
	cursor := ""
	for range 4 {
		events, next, err := storage.ListAudit(3, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, strings.Join(auditIDs(events), ","))
		if next == "" {
			break
		}
		cursor = next
	}
	if got := strings.Join(pages, "|"); got != "e8,e7,e6|e5,e4,e3|e2" {
		t.Fatalf("retained pages = %s", got)
	}
	if events, _, _ := storage.ListAudit(3, "unknown-cursor"); strings.Join(auditIDs(events), ",") != "e8,e7,e6" {
		t.Fatalf("unknown cursor did not restart from newest: %v", auditIDs(events))
	}
}

func TestOversizedAuditChangeIsReplacedByMarker(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	event := auditTestEvent("big", time.Now().UTC())
	event.Change = map[string]any{"blob": strings.Repeat("x", maxAuditEventBytes)}
	if err := storage.AppendAudit(event, 0); err != nil {
		t.Fatal(err)
	}
	events, _, err := storage.ListAudit(1, "")
	if err != nil || len(events) != 1 || events[0].Change["truncated"] != true || events[0].Action != "file.upload" {
		t.Fatalf("oversized audit event = %#v err=%v", events, err)
	}
}

func TestAuditDatabaseIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	info, err := os.Stat(auditLogPath(path))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("audit database mode = %v err=%v", info, err)
	}
}

// Audit writes must not wait for the state lock, which is what used to stall
// session checks behind every audited action.
func TestAppendAuditDoesNotTakeTheStateLock(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	storage.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- storage.AppendAudit(auditTestEvent("while-locked", time.Now().UTC()), 0) }()
	select {
	case err := <-done:
		storage.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		storage.mu.Unlock()
		t.Fatal("AppendAudit blocked on the state lock")
	}
}

func TestAppendAuditFailsClosedWhenDatabaseFileIsRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if err := os.Remove(auditLogPath(path)); err != nil {
		t.Skipf("open database file cannot be removed on this platform: %v", err)
	}
	if err := storage.AppendAudit(auditTestEvent("after-remove", time.Now().UTC()), 0); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("append to removed audit database = %v", err)
	}
}

// Background work can outlive the store during shutdown; it must get an
// error rather than a panic or a race on the closed handle.
func TestAuditAfterCloseReturnsUnavailable(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			_ = storage.AppendAudit(auditTestEvent(fmt.Sprintf("bg-%d", time.Now().UnixNano()), time.Now().UTC()), 0)
		}
	}()
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
	if err := storage.AppendAudit(auditTestEvent("after-close", time.Now().UTC()), 0); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("append after close = %v", err)
	}
	if _, _, err := storage.ListAudit(10, ""); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("list after close = %v", err)
	}
}

func TestAppendAuditRejectsDuplicateIDs(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	event := auditTestEvent("same", time.Now().UTC())
	if err := storage.AppendAudit(event, 0); err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendAudit(event, 0); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate audit ID = %v", err)
	}
}
