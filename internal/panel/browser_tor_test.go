package panel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTorBrowserRecognizesStandardComposeAndPreservesCustomServices(t *testing.T) {
	compose, err := os.ReadFile("../../deploy/compose/compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "http://browser:8092") || !strings.Contains(string(compose), "/run/kejilion-panel/browser/browser-token") {
		t.Fatal("standard deployment contract changed")
	}
	for _, url := range []string{"", "http://browser:8092", "http://browser:8092/"} {
		s := &Server{}
		if url != "" {
			s.config.BrowserServiceURL = url
			s.config.BrowserTokenFile = "/run/kejilion-panel/browser/browser-token"
		}
		for path, want := range map[string]string{"/healthz": "/", "/v1/sessions/" + strings.Repeat("b", 64) + "/vnc": "/websockify"} {
			request, err := s.browserServiceRequestWithMarker(context.Background(), path, func() ([]byte, error) { return []byte("tor"), nil })
			if err != nil || request.URL.Path != want || request.URL.Host != "browser:8092" || len(request.Header) != 0 {
				t.Fatal(url, path, request, err)
			}
		}
	}
	token := filepath.Join(t.TempDir(), "token")
	if err = os.WriteFile(token, []byte(strings.Repeat("t", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{config: Config{BrowserServiceURL: "http://custom-browser:8092", BrowserTokenFile: token}}
	request, err := s.browserServiceRequestWithMarker(context.Background(), "/healthz", func() ([]byte, error) { t.Fatal("custom service read managed marker"); return nil, nil })
	if err != nil || request.URL.Path != "/healthz" || request.Header.Get("Authorization") == "" {
		t.Fatal(request, err)
	}
}

func TestTorBrowserOnlyMapsNativeDesktopEndpoints(t *testing.T) {
	for path, expected := range map[string]string{
		"/healthz": "/",
		"/v1/sessions/" + strings.Repeat("a", 64) + "/vnc": "/websockify",
	} {
		request, err := torBrowserRequest(context.Background(), "http://browser:8092", path)
		if err != nil || request.URL.Host != "browser:8092" || request.URL.Path != expected || len(request.Header) != 0 {
			t.Fatal(request, err)
		}
	}
	for _, path := range []string{"/", "/v1/sessions/" + strings.Repeat("a", 64), "/v1/sessions/../../vnc", "/v1/sessions/a/vnc?x=1", "https://evil.test/vnc"} {
		if _, err := torBrowserRequest(context.Background(), "http://browser:8092", path); err == nil {
			t.Fatal("unexpected endpoint", path)
		}
	}
}
