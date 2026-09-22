package agent

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func writeTestPhoto(t *testing.T, root string) *filemanager.Manager {
	t.Helper()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	source := image.NewNRGBA(image.Rect(0, 0, 320, 200))
	for y := range 200 {
		for x := range 320 {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.png"), encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return manager
}

func thumbnailPath(t *testing.T, manager *filemanager.Manager) string {
	t.Helper()
	entry, err := manager.Stat("/photo.png")
	if err != nil {
		t.Fatal(err)
	}
	return "/v1/files/content?path=%2Fphoto.png&disposition=inline&mode=thumbnail&version=" + url.QueryEscape(entry.ResourceVersion)
}

func TestThumbnailCacheEvictsLeastRecentlyUsedByBytes(t *testing.T) {
	cache := newThumbnailCache(10)
	cache.put("a", []byte("12345"), "image/png")
	cache.put("b", []byte("12345"), "image/png")
	if _, _, ok := cache.get("a"); !ok {
		t.Fatal("a missing")
	}
	cache.put("c", []byte("12345"), "image/png")
	if _, _, ok := cache.get("b"); ok {
		t.Fatal("least recently used entry was kept")
	}
	if _, _, ok := cache.get("a"); !ok {
		t.Fatal("recently used entry was evicted")
	}
	cache.put("huge", bytes.Repeat([]byte("x"), 11), "image/png")
	if _, _, ok := cache.get("huge"); ok {
		t.Fatal("entry larger than the cache was stored")
	}
}

func TestThumbnailRevalidationAndCacheHitsSkipGeneration(t *testing.T) {
	server := testServer(t)
	manager := writeTestPhoto(t, t.TempDir())
	server.files = manager
	path := thumbnailPath(t, manager)
	first := fileRequest(server, http.MethodGet, path, "")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first thumbnail status=%d etag=%q", first.Code, etag)
	}
	// Occupy every generation slot: any request that needed to decode the
	// image now would wait for the gate and time out.
	for range cap(server.thumbnailGate) {
		server.thumbnailGate <- struct{}{}
	}
	defer func() {
		for range cap(server.thumbnailGate) {
			<-server.thumbnailGate
		}
	}()
	revalidated := fileRequestWithHeaders(server, http.MethodGet, path, "", map[string]string{"If-None-Match": etag})
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("revalidation status=%d", revalidated.Code)
	}
	cached := fileRequest(server, http.MethodGet, path, "")
	if cached.Code != http.StatusOK || !bytes.Equal(cached.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf("cached thumbnail status=%d", cached.Code)
	}
}

func TestStandaloneFileHandlerServesThumbnails(t *testing.T) {
	manager := writeTestPhoto(t, t.TempDir())
	handler := NewFileHandler(manager)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, thumbnailPath(t, manager), nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("light broker thumbnail status=%d body=%s", response.Code, response.Body.String())
	}
}
