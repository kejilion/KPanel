package panel

import (
	"net/http"
	"strings"
	"testing"
)

func TestMusicMetadataAuthenticationAndInput(t *testing.T) {
	s, tokenPath := newTestServer(t)
	if r := performRequest(s, http.MethodPost, musicMetadataPath, []byte(`{}`), nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated=%d", r.Code)
	}
	session, csrf := bootstrapCookies(t, s, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	for _, body := range []string{`{}`, `{"title":"Song","duration":-1}`, `{"title":"Song","url":"http://127.0.0.1"}`, `{"title":"Song"} {}`, `{"title":"` + strings.Repeat("x", 257) + `"}`} {
		r := authenticatedRequest(s, http.MethodPost, musicMetadataPath, []byte(body), session, csrf, headers)
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid input returned %d: %s", r.Code, r.Body.String())
		}
	}
	r := authenticatedRequest(s, http.MethodPost, musicMetadataPath, []byte(`{"title":"Song"}`), session, csrf, map[string]string{"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrf.Value})
	if r.Code != http.StatusForbidden {
		t.Fatalf("cross origin=%d", r.Code)
	}
	r = authenticatedRequest(s, http.MethodPost, musicMetadataPath, []byte(`{"title":"`+strings.Repeat("x", 5000)+`"}`), session, csrf, headers)
	if r.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body=%d", r.Code)
	}
}
