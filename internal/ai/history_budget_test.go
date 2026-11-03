package ai

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestAIHistoryBudgetTracksUpsertsAndCascadingDeletes(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/ai.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	session, err := store.CreateSession(ctx, Session{UserID: "admin", Title: "bounded history"})
	if err != nil {
		t.Fatal(err)
	}
	message := Message{ID: "budget-message", SessionID: session.ID, Role: RoleUser, Content: "one"}
	if _, err := store.AddMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	message.Content = "updated content"
	if _, err := store.AddMessage(ctx, message); err != nil {
		t.Fatalf("message upsert: %v", err)
	}
	assertHistoryUsage(t, store, 1, 1)
	if err := store.DeleteSession(ctx, "admin", session.ID); err != nil {
		t.Fatal(err)
	}
	assertHistoryUsage(t, store, 0, 0)
}

func TestAIHistoryAggregateByteLimitRejectsSessionAndMessageWrites(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/ai.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE ai_history_usage SET payload_bytes=? WHERE id=1`, MaxAIHistoryBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(ctx, Session{UserID: "admin"}); err == nil || !strings.Contains(err.Error(), "AI history storage limit reached") {
		t.Fatalf("session at aggregate byte cap: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE ai_history_usage SET payload_bytes=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(ctx, Session{UserID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE ai_history_usage SET payload_bytes=? WHERE id=1`, MaxAIHistoryBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "new message"}); err == nil || !strings.Contains(err.Error(), "AI history storage limit reached") {
		t.Fatalf("message at aggregate byte cap: %v", err)
	}
}

func TestAIHistoryMessageCountLimitSerializesConcurrentWriters(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/ai.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	session, err := store.CreateSession(ctx, Session{UserID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE ai_history_usage SET message_count=? WHERE id=1`, MaxAIMessages-1); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for index := range 2 {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			_, err := store.AddMessage(ctx, Message{ID: "parallel-" + string(rune('a'+index)), SessionID: session.ID, Role: RoleUser, Content: "bounded"})
			results <- err
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "AI history storage limit reached") {
			rejected++
		} else {
			t.Fatalf("concurrent writer returned unexpected error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent writes succeeded=%d rejected=%d, want one of each", succeeded, rejected)
	}
	var sessionCount, messageCount, rowCount int
	if err := store.db.QueryRow(`SELECT session_count,message_count FROM ai_history_usage WHERE id=1`).Scan(&sessionCount, &messageCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || messageCount != MaxAIMessages || rowCount != 1 {
		t.Fatalf("usage counters=(%d,%d), persisted messages=%d", sessionCount, messageCount, rowCount)
	}
}

func assertHistoryUsage(t *testing.T, store *Store, sessions, messages int) {
	t.Helper()
	var actualSessions, actualMessages, actualBytes int
	if err := store.db.QueryRow(`SELECT session_count,message_count,payload_bytes FROM ai_history_usage WHERE id=1`).Scan(&actualSessions, &actualMessages, &actualBytes); err != nil {
		t.Fatal(err)
	}
	var expectedSessions, expectedMessages, expectedBytes int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&expectedSessions); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&expectedMessages); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT
		COALESCE((SELECT SUM(length(CAST(id AS BLOB))+length(CAST(user_id AS BLOB))+length(CAST(title AS BLOB))+length(CAST(provider_id AS BLOB))+length(CAST(model_id AS BLOB))+length(CAST(provider_name AS BLOB))+length(CAST(model_name AS BLOB))+length(CAST(summary AS BLOB))+length(CAST(summary_cursor AS BLOB))+length(CAST(approval_mode AS BLOB))+length(CAST(thinking_level AS BLOB))) FROM sessions),0)+
		COALESCE((SELECT SUM(length(CAST(id AS BLOB))+length(CAST(session_id AS BLOB))+length(CAST(run_id AS BLOB))+length(CAST(role AS BLOB))+length(CAST(content AS BLOB))+length(CAST(provider_id AS BLOB))+length(CAST(provider_name AS BLOB))+length(CAST(model_id AS BLOB))+length(CAST(model_name AS BLOB))+length(CAST(tool_call_id AS BLOB))+length(CAST(attachments_json AS BLOB))) FROM messages),0)`).Scan(&expectedBytes); err != nil {
		t.Fatal(err)
	}
	if actualSessions != sessions || actualMessages != messages || actualSessions != expectedSessions || actualMessages != expectedMessages || actualBytes != expectedBytes {
		t.Fatalf("usage counters=(%d,%d,%d), rows=(%d,%d,%d)", actualSessions, actualMessages, actualBytes, expectedSessions, expectedMessages, expectedBytes)
	}
}
