//go:build linux

package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func TestAITailStreamsLargeStableLogAndRejectsSensitiveContext(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	var log strings.Builder
	for i := 0; i < 120000; i++ {
		fmt.Fprintf(&log, "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /sample-%06d HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n", i)
	}
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search?token=OPAQUE_BODY HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n")
	if log.Len() <= 8<<20 {
		t.Fatal("fixture did not exercise the large-log path")
	}
	path := filepath.Join(root, "large.log")
	if err := os.WriteFile(path, []byte(log.String()), 0600); err != nil {
		t.Fatal(err)
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Flarge.log&maxBytes=1024", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "sample-119999") || !strings.Contains(response.Body.String(), "[REDACTED LOG RECORD]") || strings.Contains(response.Body.String(), "OPAQUE_BODY") || !strings.Contains(response.Body.String(), `"truncated":true`) {
		t.Fatalf("large tail status=%d body=%s", response.Code, response.Body.String())
	}
	pem := log.String() + "-----BEGIN PRIVATE KEY-----\nOPAQUEBODY\n-----END PRIVATE KEY-----\n" + "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /after HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n"
	if err := os.WriteFile(path, []byte(pem), 0600); err != nil {
		t.Fatal(err)
	}
	response = fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Flarge.log&maxBytes=1024", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "OPAQUEBODY") || !strings.Contains(response.Body.String(), "[REDACTED PRIVATE KEY]") || !strings.Contains(response.Body.String(), "/after") {
		t.Fatalf("large private-key projection status=%d body=%s", response.Code, response.Body.String())
	}
	secret := "apiVersion: v1\nkind: Secret\nstringData:\n  opaque: OPAQUE_BODY\n---\n" + log.String()
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	response = fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Flarge.log&maxBytes=1024", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "OPAQUE_BODY") || !strings.Contains(response.Body.String(), "sample-119999") {
		t.Fatalf("sensitive tail status=%d body=%s", response.Code, response.Body.String())
	}
	jsonl := log.String() + `{"line":"data:"}` + "\n" + `{"line":"  opaque: OPAQUE_BODY"}` + "\n" + `{"line":"kind: Secret"}` + "\n"
	if err := os.WriteFile(path, []byte(jsonl), 0600); err != nil {
		t.Fatal(err)
	}
	response = fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Flarge.log&maxBytes=1024", "")
	if response.Code != http.StatusUnprocessableEntity || strings.Contains(response.Body.String(), "OPAQUE_BODY") {
		t.Fatalf("JSONL secret context status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAITailStreamsManyShortStableLogRecords(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	var log strings.Builder
	for i := 0; i < 60000; i++ {
		fmt.Fprintf(&log, "127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /sample-%06d HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n", i)
	}
	log.WriteString("ordinary author=alice filename=access.log metadata=present\n")
	log.WriteString("user name changed; data value is available; kind response\n")
	log.WriteString("password policy changed; token format documented\n")
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET / HTTP/1.1\" 200 896 \"-\" \"curl/8.0.0\" \"-\"\n")
	log.WriteString(`{"name":"alice","data":"normal"}` + "\n")
	log.WriteString(`{"message":"password policy changed","email":"user@example.test","percent":"30%","path":"C:\\temp"}` + "\n")
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search?author=alice&filename=access.log&metadata=present HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n")
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search?token=OPAQUE_BODY HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n")
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search?auth=OPAQUE_AUTH HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n")
	log.WriteString("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /search?%61uth=OPAQUE_ENCODED HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n")
	if log.Len() >= 8<<20 {
		t.Fatal("fixture must remain under the snapshot threshold")
	}
	if err := os.WriteFile(filepath.Join(root, "many.log"), []byte(log.String()), 0600); err != nil {
		t.Fatal(err)
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Fmany.log&maxBytes=1024", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "sample-059999") || !strings.Contains(response.Body.String(), "author=alice") || !strings.Contains(response.Body.String(), "filename=access.log") || !strings.Contains(response.Body.String(), "metadata=present") || !strings.Contains(response.Body.String(), "user name changed") || !strings.Contains(response.Body.String(), `\"name\":\"alice\"`) || !strings.Contains(response.Body.String(), "password policy changed") || !strings.Contains(response.Body.String(), "user@example.test") || !strings.Contains(response.Body.String(), "30%") || !strings.Contains(response.Body.String(), "curl/8.0.0") || !strings.Contains(response.Body.String(), "[REDACTED LOG RECORD]") || strings.Contains(response.Body.String(), "OPAQUE_BODY") || strings.Contains(response.Body.String(), "OPAQUE_AUTH") || strings.Contains(response.Body.String(), "OPAQUE_ENCODED") {
		t.Fatalf("many-record tail status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAITailStreamsManyOrdinaryJSONRecords(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	value := strings.Repeat(`{"x":0}`+"\n", 50001) + `{"name":"alice","data":"normal","message":"password policy changed"}` + "\n"
	if len(value) >= 8<<20 {
		t.Fatal("fixture must remain under the snapshot threshold")
	}
	if err := os.WriteFile(filepath.Join(root, "many-json.log"), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Fmany-json.log&maxBytes=1024", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "password policy changed") || !strings.Contains(response.Body.String(), `\"name\":\"alice\"`) {
		t.Fatalf("ordinary JSON records status=%d body=%s", response.Code, response.Body.String())
	}
}

func BenchmarkAITailManyOrdinaryJSONRecords(b *testing.B) {
	root := b.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = manager.Close() })
	handler, stop := NewCachedFileHandler(manager, b.TempDir())
	b.Cleanup(stop)
	value := strings.Repeat(`{"name":"alice","data":"normal","message":"password policy changed"}`+"\n", 51000)
	if err := os.WriteFile(filepath.Join(root, "many-json.log"), []byte(value), 0600); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(value)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/files/ai-tail?path=%2Fmany-json.log&maxBytes=1024", nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "password policy changed") {
			b.Fatalf("ordinary JSON records status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
