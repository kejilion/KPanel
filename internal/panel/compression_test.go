package panel

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentJSONResponseUsesGzipWhenAccepted(t *testing.T) {
	server, _ := newTestServer(t)
	body := bytes.Repeat([]byte(`{"status":"running"}`), 256)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()

	server.writeAgentResponse(response, request, AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json; charset=utf-8",
		Body:        body,
	})

	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q; want gzip", got)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatal("gzip response does not decode to the original body")
	}
}

func TestAgentJSONResponseHonorsGzipQualityZero(t *testing.T) {
	server, _ := newTestServer(t)
	body := bytes.Repeat([]byte("a"), 2048)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	request.Header.Set("Accept-Encoding", "gzip;q=0, *;q=1")
	response := httptest.NewRecorder()

	server.writeAgentResponse(response, request, AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        body,
	})

	if got := response.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q; want identity", got)
	}
	if !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatal("identity response body changed")
	}
}

func TestStaticHashedAssetUsesPrecompressedVariantAndImmutableCache(t *testing.T) {
	server, _ := newTestServer(t)
	assets := filepath.Join(server.config.WebRoot, "assets")
	if err := os.MkdirAll(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	source := bytes.Repeat([]byte("const value = 'kpanel';\n"), 128)
	path := filepath.Join(assets, "main-deadbeef.js")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(source); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".gz", compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	response := performRequest(server, http.MethodGet, "/assets/main-deadbeef.js", nil, map[string]string{
		"Accept-Encoding": "gzip",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected static status: %d", response.Code)
	}
	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q; want gzip", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestAppearanceBootstrapRevalidatesOnEveryLoad(t *testing.T) {
	server, _ := newTestServer(t)
	if err := os.WriteFile(filepath.Join(server.config.WebRoot, "appearance-init.js"), []byte("/* boot */"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := performRequest(server, http.MethodGet, "/appearance-init.js?v=20260926", nil, nil)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("bootstrap status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
	}
}
