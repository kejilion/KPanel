package ai

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func contextFixture(t *testing.T) (*Store, Session, Run) {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	session, err := s.CreateSession(context.Background(), Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(context.Background(), Run{SessionID: session.ID, UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	return s, session, run
}

func contextMessage(t *testing.T, s *Store, run Run, bytes int) Message {
	t.Helper()
	message, err := s.AddMessage(context.Background(), Message{SessionID: run.SessionID, RunID: run.ID, Role: RoleUser, Content: "read the attached input",
		Attachments: []Attachment{{Name: "image.png", MimeType: "image/png", Kind: "image", Data: []byte(strings.Repeat("x", bytes))}}})
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func contextRetry(t *testing.T, s *Store, parent Run) Run {
	t.Helper()
	parent.Status = RunFailed
	if err := s.UpdateRun(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(context.Background(), Run{RetryOf: &parent.ID, SessionID: parent.SessionID, UserID: parent.UserID, ProviderID: parent.ProviderID, ModelID: parent.ModelID})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestContextRetryChainIncludesIntermediateInputAndIndependentRoot(t *testing.T) {
	s, _, root := contextFixture(t)
	ctx := context.Background()
	first := contextMessage(t, s, root, 1024)
	retry := contextRetry(t, s, root)
	second := contextMessage(t, s, retry, 1024)
	retryAgain := contextRetry(t, s, retry)
	snapshot, err := s.ContextForRun(ctx, retryAgain, 131072)
	if err != nil || snapshot.UserMessageCount != 2 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	required := map[string]bool{}
	for _, message := range snapshot.Messages {
		if message.RequiredAttachments && len(message.Attachments) == 1 && len(message.Attachments[0].Data) == 1024 {
			required[message.ID] = true
		}
	}
	if !required[first.ID] || !required[second.ID] {
		t.Fatalf("retry lost ancestors: %v", required)
	}
}

func TestContextSourceSurvivesRestartAndNewSendDoesNotInherit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ai.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, Session{UserID: "admin", ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := s.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	contextMessage(t, s, root, 1024)
	retry := contextRetry(t, s, root)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.RunByID(ctx, retry.ID)
	if err != nil || restored.Status != RunInterrupted || restored.RetryOf == nil || *restored.RetryOf != root.ID {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
	next := contextRetry(t, s, restored)
	snapshot, err := s.ContextForRun(ctx, next, 131072)
	if err != nil || len(snapshot.Messages) != 1 || !snapshot.Messages[0].RequiredAttachments {
		t.Fatalf("retry restore=%#v err=%v", snapshot, err)
	}
	next.Status = RunCompleted
	if err := s.UpdateRun(ctx, next); err != nil {
		t.Fatal(err)
	}
	independent, err := s.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: "p", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	contextMessage(t, s, independent, 1024)
	snapshot, err = s.ContextForRun(ctx, independent, 131072)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range snapshot.Messages {
		if message.RunID != independent.ID && message.RequiredAttachments {
			t.Fatal("new Send inherited failed provenance")
		}
	}
	if independent.RetryOf == nil || *independent.RetryOf != "" {
		t.Fatal("new Send is not an explicit root")
	}
}

func TestContextBudgetFailsWithoutAdvancingSummaryOrLosingRows(t *testing.T) {
	s, session, root := contextFixture(t)
	ctx := context.Background()
	first := contextMessage(t, s, root, 4<<20)
	contextMessage(t, s, root, 4<<20)
	retry := contextRetry(t, s, root)
	contextMessage(t, s, retry, 1)
	next := contextRetry(t, s, retry)
	if _, err := s.ContextForRun(ctx, next, 131072); !errors.Is(err, ErrContextAttachmentLimit) {
		t.Fatalf("over budget: %v", err)
	}
	var cursor string
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT summary_cursor FROM sessions WHERE id=?", session.ID).Scan(&cursor); err != nil || cursor != "" {
		t.Fatalf("cursor=%q err=%v", cursor, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages WHERE session_id=?", session.ID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	// Previously summarized mandatory input may not disappear from preflight.
	if _, err := s.db.ExecContext(ctx, "UPDATE sessions SET summary_cursor=? WHERE id=?", first.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ContextForRun(ctx, next, 131072); !errors.Is(err, ErrContextAttachmentLimit) {
		t.Fatalf("cursor hid mandatory bytes: %v", err)
	}
}

func TestContextRejectsUnknownAndCorruptRetrySources(t *testing.T) {
	for _, source := range []string{"unknown", "missing", "cycle", "other-session"} {
		t.Run(source, func(t *testing.T) {
			s, session, root := contextFixture(t)
			ctx := context.Background()
			var parent any
			switch source {
			case "missing":
				parent = "missing"
			case "cycle":
				parent = root.ID
			case "other-session":
				other, err := s.CreateSession(ctx, Session{UserID: "admin", ProviderID: "p", ModelID: "m"})
				if err != nil {
					t.Fatal(err)
				}
				foreign, err := s.CreateRun(ctx, Run{SessionID: other.ID, UserID: "admin", ProviderID: "p", ModelID: "m"})
				if err != nil {
					t.Fatal(err)
				}
				parent = foreign.ID
			}
			if _, err := s.db.ExecContext(ctx, "UPDATE runs SET retry_of=? WHERE id=?", parent, root.ID); err != nil {
				t.Fatal(err)
			}
			contextMessage(t, s, root, 3)
			if _, err := s.ContextForRun(ctx, root, 131072); !errors.Is(err, ErrContextAttachmentSource) {
				t.Fatalf("source=%s error=%v", source, err)
			}
			if _, err := s.ConversationMessageMetadataPage(ctx, session.ID, 50, ""); err != nil {
				t.Fatalf("source corruption disabled presentation: %v", err)
			}
		})
	}
}

func TestContextSnapshotCountDoesNotConsumeLaterInput(t *testing.T) {
	s, _, run := contextFixture(t)
	ctx := context.Background()
	contextMessage(t, s, run, 3)
	run.Status = RunRunning
	if err := s.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.ContextForRun(ctx, run, 131072)
	if err != nil {
		t.Fatal(err)
	}
	contextMessage(t, s, run, 3)
	completed, err := s.CompleteRunIfUserMessageCount(ctx, run, snapshot.UserMessageCount)
	if err != nil || completed {
		t.Fatalf("later input incorrectly consumed: completed=%v error=%v", completed, err)
	}
	next, err := s.ContextForRun(ctx, run, 131072)
	if err != nil || next.UserMessageCount != 2 {
		t.Fatalf("next=%#v err=%v", next, err)
	}
}

func TestContextRequiredInputsRespectCountTokenAndCancellation(t *testing.T) {
	for _, kind := range []string{"count", "token", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s, session, run := contextFixture(t)
			ctx := context.Background()
			n := 1
			if kind == "count" {
				n = 201
			}
			for i := 0; i < n; i++ {
				contextMessage(t, s, run, 3)
			}
			window := 131072
			if kind == "token" {
				window = 1024
			}
			if kind == "cancel" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			_, err := s.ContextForRun(ctx, run, window)
			if kind == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrContextAttachmentLimit) {
				t.Fatalf("%s: %v", kind, err)
			}
			var cursor string
			if err := s.db.QueryRow("SELECT summary_cursor FROM sessions WHERE id=?", session.ID).Scan(&cursor); err != nil || cursor != "" {
				t.Fatalf("advanced cursor=%q err=%v", cursor, err)
			}
		})
	}
}

func TestContextRetryDepthIsBounded(t *testing.T) {
	s, _, run := contextFixture(t)
	for i := 1; i <= maxRetrySourceRuns; i++ {
		run = contextRetry(t, s, run)
	}
	if _, err := s.ContextForRun(context.Background(), run, 131072); !errors.Is(err, ErrContextAttachmentSource) {
		t.Fatalf("unbounded retry chain: %v", err)
	}
}

func TestProposalTextDoesNotRetainAttachments(t *testing.T) {
	history := []Message{{Role: RoleUser, Content: "remember", Attachments: []Attachment{{Data: []byte("body")}}, RequiredAttachments: true}}
	text := proposalMessageText(history)
	if len(text) != 1 || text[0].Content != "remember" || text[0].Attachments != nil || len(history[0].Attachments) != 1 {
		t.Fatalf("text=%#v", text)
	}
}
