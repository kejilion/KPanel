package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAttachmentProjectionSQLiteValues(t *testing.T) {
	s, _, _ := contextFixture(t)
	for _, raw := range []string{"", "null", "[]", `[null,{}]`, `[{"data":"%%%","DATA":"Zg==","name":"first","NAME":null}]`, `[{"data":"Z\u000dm\n8=","name":"文档"}]`, `[{"data":"Zg==","data":null,"unknown":{"ignored":1}}]`, `[{"data":"/w=="}]`, `[{"data":"\u002fw=="}]`, `[{"data":1}]`, `[{"data":"%%%"}]`, `[null,null,null,null,null]`, "[", `[] []`} {
		for _, value := range []any{[]byte(raw), raw} {
			want, wantErr := decodeAttachmentMetadata([]byte(raw))
			var encoded []byte
			err := s.db.QueryRow(`SELECT kpanel_ai_attachment_metadata_v1(CAST(? AS BLOB))`, value).Scan(&encoded)
			if (err != nil) != (wantErr != nil) {
				t.Fatalf("input %.100q (%T): SQL=%v parser=%v", raw, value, err, wantErr)
			}
			if err == nil {
				got, err := decodeAttachmentMetadataProjection(encoded)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("input %.100q: metadata=%#v/%v want=%#v", raw, got, err, want)
				}
			}
		}
	}
	var data []byte
	if err := s.db.QueryRow(`SELECT kpanel_ai_attachment_metadata_v1(CAST(NULL AS BLOB))`).Scan(&data); err != nil || string(data) != "null" {
		t.Fatalf("NULL metadata=%q err=%v", data, err)
	}
	// Legacy metadata can be large in its own right. Do not introduce a name
	// limit just to make the allocation tests with ordinary names pass.
	name := strings.Repeat("文", 100000)
	raw, _ := json.Marshal([]storedAttachment{{Name: name, Data: "Zg=="}})
	if err := s.db.QueryRow(`SELECT kpanel_ai_attachment_metadata_v1(CAST(? AS BLOB))`, raw).Scan(&data); err != nil {
		t.Fatal(err)
	}
	items, err := decodeAttachmentMetadataProjection(data)
	if err != nil || len(items) != 1 || items[0].Name != name || items[0].Size != 1 {
		t.Fatalf("long legacy metadata changed: %v", err)
	}
}

func TestAttachmentProjectionStoreErrorsPreserveRows(t *testing.T) {
	s, session, run := contextFixture(t)
	ctx := context.Background()
	contextMessage(t, s, run, 1)
	bad := contextMessage(t, s, run, 1)
	cases := []struct {
		name, data string
		fail       bool
	}{
		{"four", `[null,null,null,null]`, false}, {"five", `[null,null,null,null,null]`, true},
		{"json", "{", true}, {"base64", `[{"data":"%%%"}]`, true},
	}
	for _, n := range []int{maxAttachmentReadBytes - 1, maxAttachmentReadBytes, maxAttachmentReadBytes + 1} {
		cases = append(cases, struct {
			name, data string
			fail       bool
		}{fmt.Sprint(n), "[]" + strings.Repeat(" ", n-2), n > maxAttachmentReadBytes})
	}
	for _, depth := range []int{9998, 9999, 10000} {
		cases = append(cases, struct {
			name, data string
			fail       bool
		}{fmt.Sprint(depth), `[{"data":"Zg==","unknown":` + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + `}]`, depth >= 9999})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range []any{[]byte(tc.data), tc.data} {
				if _, err := s.db.Exec(`UPDATE messages SET attachments_json=? WHERE id=?`, value, bad.ID); err != nil {
					t.Fatal(err)
				}
				page, err := s.ConversationMessageMetadataPage(ctx, session.ID, 50, "")
				if (err != nil) != tc.fail || (err != nil && (page.Items != nil || page.NextCursor != "")) {
					t.Fatalf("page fail=%v page=%#v err=%v", tc.fail, page, err)
				}
				tx, err := s.db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				batch, batchErr := readContextBatch(ctx, tx, session.ID, "", 200, false)
				required, requiredErr := contextRequiredAttachments(ctx, tx, session.ID, session.UserID, run.ID)
				text, textErr := readContextBatch(ctx, tx, session.ID, "", 200, true)
				tx.Rollback()
				if textErr != nil || len(text) != 2 || text[0].Attachments != nil || text[1].Attachments != nil {
					t.Fatalf("text-only path read attachment bodies: items=%#v err=%v", text, textErr)
				}
				if (batchErr != nil) != tc.fail || (batchErr != nil && batch != nil) {
					t.Fatalf("context batch fail=%v items=%#v err=%v", tc.fail, batch, batchErr)
				}
				if (requiredErr != nil) != tc.fail || (requiredErr != nil && required != nil) {
					t.Fatalf("required fail=%v items=%#v err=%v", tc.fail, required, requiredErr)
				}
				var original []byte
				var content, kind string
				if err := s.db.QueryRow(`SELECT attachments_json,content,typeof(attachments_json) FROM messages WHERE id=?`, bad.ID).Scan(&original, &content, &kind); err != nil {
					t.Fatal(err)
				}
				wantKind := "blob"
				if _, ok := value.(string); ok {
					wantKind = "text"
				}
				if !bytes.Equal(original, []byte(tc.data)) || content != bad.Content || kind != wantKind {
					t.Fatal("metadata projection changed original storage")
				}
			}
		})
	}
}

func TestAttachmentProjectionConnectionsOwnResults(t *testing.T) {
	s, _, _ := contextFixture(t)
	ctx := context.Background()
	connections := make([]*sql.Conn, 4)
	for i := range connections {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections[i] = c
		defer c.Close()
	}
	var wg sync.WaitGroup
	for i, c := range connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var retained []Attachment
			for j := 0; j < 2; j++ {
				a, _ := json.Marshal([]storedAttachment{{Name: fmt.Sprintf("文件-%d-%d-a", i, j), Data: "Zg=="}})
				b, _ := json.Marshal([]storedAttachment{{Name: fmt.Sprintf("文件-%d-%d-b", i, j), Data: "Zm8="}})
				rows, err := c.QueryContext(ctx, `WITH v(x) AS (VALUES(CAST(? AS BLOB)),(CAST(? AS BLOB))) SELECT kpanel_ai_attachment_metadata_v1(x) FROM v`, a, b)
				if err != nil {
					t.Error(err)
					return
				}
				for rows.Next() {
					var raw sql.RawBytes
					if err := rows.Scan(&raw); err != nil {
						t.Error(err)
						rows.Close()
						return
					}
					items, err := decodeAttachmentMetadataProjection(raw)
					if err != nil || len(items) != 1 {
						t.Errorf("items=%#v err=%v", items, err)
						rows.Close()
						return
					}
					retained = append(retained, items[0])
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					t.Error(err)
					return
				}
				clear(a)
				clear(b)
			}
			if len(retained) != 4 {
				t.Errorf("connection %d returned %d records", i, len(retained))
			}
			for j, item := range retained {
				suffix := "a"
				if j%2 == 1 {
					suffix = "b"
				}
				if item.Name != fmt.Sprintf("文件-%d-%d-%s", i, j/2, suffix) || item.Size != j%2+1 || item.Data != nil {
					t.Errorf("borrowed result changed: %#v", item)
				}
			}
		}()
	}
	wg.Wait()
}

func TestAttachmentProjectionStoreAllocatedBytes(t *testing.T) {
	var pngData bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1024, 1020))); err != nil {
		t.Fatal(err)
	}
	attachments, err := validateAttachments([]Attachment{{Name: "one.png", Data: pngData.Bytes()}, {Name: "two.png", Data: pngData.Bytes()}})
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 16} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s, session, run := contextFixture(t)
			ctx := context.Background()
			for i := 0; i < count; i++ {
				runID := ""
				if i == count-1 {
					runID = run.ID
				}
				if _, err := s.AddMessage(ctx, Message{ID: fmt.Sprintf("projection-%02d", i), SessionID: session.ID, RunID: runID, Role: RoleUser, CreatedAt: time.UnixMilli(1700000000000 + int64(i)), Attachments: attachments}); err != nil {
					t.Fatal(err)
				}
			}
			readers := []struct {
				name string
				read func() error
			}{
				{"previous_sql_body", func() error {
					rows, err := s.db.QueryContext(ctx, `SELECT attachments_json FROM messages WHERE session_id=? ORDER BY created_at,id`, session.ID)
					if err != nil {
						return err
					}
					defer rows.Close()
					for rows.Next() {
						var raw sql.RawBytes
						if err := rows.Scan(&raw); err != nil {
							return err
						}
						if _, err := decodeAttachmentMetadata(raw); err != nil {
							return err
						}
					}
					return rows.Err()
				}},
				{"history", func() error {
					p, err := s.ConversationMessageMetadataPage(ctx, session.ID, 50, "")
					if err == nil && len(p.Items) != count {
						return fmt.Errorf("page count=%d", len(p.Items))
					}
					return err
				}},
				{"snapshot", func() error {
					p, err := s.ConversationMessageMetadataPage(ctx, session.ID, 1, "")
					if err == nil && (len(p.Items) != 1 || p.Items[0].Attachments[0].Size != pngData.Len()) {
						return fmt.Errorf("snapshot metadata mismatch")
					}
					return err
				}},
				{"context_batch", func() error {
					tx, err := s.db.BeginTx(ctx, nil)
					if err != nil {
						return err
					}
					defer tx.Rollback()
					items, err := readContextBatch(ctx, tx, session.ID, "", 200, false)
					if err == nil && len(items) != count {
						return fmt.Errorf("context count=%d", len(items))
					}
					return err
				}},
				{"context_required", func() error {
					tx, err := s.db.BeginTx(ctx, nil)
					if err != nil {
						return err
					}
					defer tx.Rollback()
					items, err := contextRequiredAttachments(ctx, tx, session.ID, session.UserID, run.ID)
					if err == nil && (len(items) != 1 || items[fmt.Sprintf("projection-%02d", count-1)] != 2*pngData.Len()) {
						return fmt.Errorf("required metadata mismatch")
					}
					return err
				}},
			}
			for _, reader := range readers {
				if err := reader.read(); err != nil {
					t.Fatal(err)
				}
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				if err := reader.read(); err != nil {
					t.Fatal(err)
				}
				runtime.ReadMemStats(&after)
				allocated := after.TotalAlloc - before.TotalAlloc
				t.Logf("history=%d rawImageBytes=%d path=%s allocatedBytes=%d", count, pngData.Len(), reader.name, allocated)
				// Count the SQL driver and projection, not just parsing an existing
				// input slice. No forced GC; this is not a cgroup/RSS assertion.
				if reader.name != "previous_sql_body" && allocated > 512<<10 {
					t.Fatalf("metadata SQL copied attachment bodies: %d bytes", allocated)
				}
			}
		})
	}
}
