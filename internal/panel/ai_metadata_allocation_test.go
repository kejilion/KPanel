package panel

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/auth"
)

func TestAIHistoryAndSnapshotAllocatedBytes(t *testing.T) {
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 1024, 1020))); err != nil {
		t.Fatal(err)
	}
	attachments := []ai.Attachment{
		{Name: "one.png", MimeType: "image/png", Kind: "image", Data: encoded.Bytes()},
		{Name: "two.png", MimeType: "image/png", Kind: "image", Data: encoded.Bytes()},
	}
	for _, count := range []int{1, 16} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			server, _ := newTestServer(t)
			if err := server.EnableAI(); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			session, err := server.ai.Store.CreateSession(ctx, ai.Session{UserID: "admin", ProviderID: "provider", ModelID: "model"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := server.ai.Store.CreateRun(ctx, ai.Run{SessionID: session.ID, UserID: "admin", ProviderID: "provider", ModelID: "model"})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < count; i++ {
				if _, err := server.ai.Store.AddMessage(ctx, ai.Message{SessionID: session.ID, RunID: run.ID, Role: ai.RoleUser, Content: fmt.Sprint(i), Attachments: attachments}); err != nil {
					t.Fatal(err)
				}
			}
			for _, stream := range []bool{false, true} {
				read := func() {
					response := httptest.NewRecorder()
					if stream {
						streamCtx, cancel := context.WithCancel(ctx)
						defer cancel()
						writer := &aiSnapshotRecorder{ResponseRecorder: response, cancel: cancel}
						server.aiRunEvents(writer, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(streamCtx), auth.Session{User: auth.PublicUser{ID: "admin"}}, run.ID)
					} else {
						server.aiMessages(response, httptest.NewRequest(http.MethodGet, "/", nil), "admin", session.ID)
					}
					if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), fmt.Sprintf(`"size":%d`, encoded.Len())) || strings.Contains(response.Body.String(), "iVBORw0KGgo") {
						t.Fatalf("metadata response status=%d bytes=%d", response.Code, response.Body.Len())
					}
					if stream && !strings.Contains(response.Body.String(), "event: run.snapshot") {
						t.Fatal("missing snapshot")
					}
				}
				read()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				read()
				runtime.ReadMemStats(&after)
				allocated := after.TotalAlloc - before.TotalAlloc
				t.Logf("history=%d sse=%v rawImageBytes=%d allocatedBytes=%d", count, stream, encoded.Len(), allocated)
				// Includes the real Store query, driver, metadata conversion and HTTP
				// response. It does not assert resident or concurrent arena memory.
				if allocated > 1<<20 {
					t.Fatalf("metadata consumer copied attachment bodies: %d bytes", allocated)
				}
			}
		})
	}
}
