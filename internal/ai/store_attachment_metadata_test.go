package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachmentMetadataPreservesDecodeContract(t *testing.T) {
	for _, encoded := range []string{
		"", "Zg==", "Zm8=", "Zm9v", "Zg==\r\n", "Z\rm\n8=", "Zg", "Zg=", "Zg===", "Zg==Zg==",
		"Zg==\x00", "====", "%%%", "Z g==", "Zg==\nZg==", strings.Repeat("Zm9v", 256) + "Zg==Zg==",
	} {
		t.Run(fmt.Sprintf("%q", encoded[:min(len(encoded), 30)]), func(t *testing.T) {
			data, err := json.Marshal([]storedAttachment{{Name: "旧文件.txt", MimeType: "text/plain", Kind: "text", Data: encoded}})
			if err != nil {
				t.Fatal(err)
			}
			want, decodeErr := base64.StdEncoding.DecodeString(encoded)
			got, err := decodeAttachmentMetadata(data)
			if (err != nil) != (decodeErr != nil) {
				t.Fatalf("metadata error=%v original error=%v", err, decodeErr)
			}
			if err == nil && (len(got) != 1 || got[0].Name != "旧文件.txt" || got[0].MimeType != "text/plain" || got[0].Kind != "text" || got[0].Size != len(want) || got[0].Data != nil) {
				t.Fatalf("metadata=%#v expected size=%d", got, len(want))
			}
		})
	}
	for _, data := range []string{"", "[]", "null", "{", "{}", "[1]", `[{"data":1}]`, `[{"name":1}]`} {
		_, oldErr := decodeAttachments([]byte(data))
		_, newErr := decodeAttachmentMetadata([]byte(data))
		if (oldErr != nil) != (newErr != nil) {
			t.Fatalf("JSON %q: old=%v metadata=%v", data, oldErr, newErr)
		}
	}
}

func TestConversationMetadataPaginationAndOriginalAttachments(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.UnixMilli(1788576000123)
	ids := make([]string, 53)
	for i := range ids {
		ids[i] = fmt.Sprintf("msg_%03d", 100-i)
		_, err := store.AddMessage(ctx, Message{ID: ids[i], SessionID: session.ID, Role: RoleUser, Content: fmt.Sprint(i), CreatedAt: stamp,
			Attachments: []Attachment{{Name: "note.txt", MimeType: "text/plain", Kind: "text", Data: []byte("hello")}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleTool, ToolCallID: "call", Content: "hidden", CreatedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	page, err := store.ConversationMessageMetadataPage(ctx, session.ID, 50, "")
	if err != nil || len(page.Items) != 50 || page.NextCursor != ids[3] {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	for i, item := range page.Items {
		if item.ID != ids[i+3] || len(item.Attachments) != 1 || item.Attachments[0].Size != 5 || item.Attachments[0].Data != nil {
			t.Fatalf("item %d=%#v", i, item)
		}
	}
	earlier, err := store.ConversationMessageMetadataPage(ctx, session.ID, 50, page.NextCursor)
	if err != nil || len(earlier.Items) != 3 || earlier.NextCursor != "" {
		t.Fatalf("earlier=%#v err=%v", earlier, err)
	}
	for i, item := range earlier.Items {
		if item.ID != ids[i] {
			t.Fatalf("earlier item %d=%s", i, item.ID)
		}
	}
	original, err := store.ConversationMessages(ctx, session.ID, 50)
	if err != nil || len(original) != 50 || string(original[0].Attachments[0].Data) != "hello" {
		t.Fatalf("original data changed: %v", err)
	}
}

func TestConversationMetadataRejectsOversizedAndCorruptRows(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "preserve me"})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"{", `[{"data":"%%%"}]`, strings.Repeat(" ", maxAttachmentReadBytes) + "[]"} {
		if _, err := store.db.ExecContext(ctx, "UPDATE messages SET attachments_json=? WHERE id=?", []byte(data), message.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConversationMessageMetadataPage(ctx, session.ID, 50, ""); err == nil {
			t.Fatal("corrupt or oversized row accepted")
		}
		var got []byte
		if err := store.db.QueryRowContext(ctx, "SELECT attachments_json FROM messages WHERE id=?", message.ID).Scan(&got); err != nil || string(got) != data {
			t.Fatalf("original row modified: %v", err)
		}
	}
}

func TestContextSummaryBatchReadsOnlyText(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(ctx, Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := store.AddMessage(ctx, Message{SessionID: session.ID, Role: RoleUser, Content: "retain old text"})
	if err != nil {
		t.Fatal(err)
	}
	// Even an unreadable body is unrelated to a text-only summary; presentation
	// and model attachment reads must still reject this row explicitly.
	if _, err := store.db.ExecContext(ctx, "UPDATE messages SET attachments_json=? WHERE id=?", []byte("{"), message.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.contextSummaryBatch(ctx, session.ID, "", 100)
	if err != nil || len(items) != 1 || items[0].Content != message.Content || items[0].ID != message.ID || items[0].Attachments != nil {
		t.Fatalf("summary=%#v err=%v", items, err)
	}
	if _, err := store.ConversationMessageMetadataPage(ctx, session.ID, 50, ""); err == nil {
		t.Fatal("presentation hid corrupt body")
	}
}
