package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDockerUpdateUsesAuthenticatedActionRoute(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	path := "/api/v1/docker/containers/" + strings.Repeat("a", 64) + "/check_update"
	if _, _, action, ok := allowedDockerActionPath(path); !ok || action != "check_update" {
		t.Fatal("missing check route")
	}
	for _, tc := range []struct {
		name, origin  string
		session, csrf bool
	}{
		{"session", "http://panel.test", false, true},
		{"csrf", "http://panel.test", true, false},
		{"origin", "https://untrusted.test", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"resourceVersion":"sha256:`+strings.Repeat("b", 64)+`"}`))
			r.Host = "panel.test"
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			if tc.session {
				r.AddCookie(session)
			}
			if tc.csrf {
				r.AddCookie(csrf)
				r.Header.Set("X-CSRF-Token", csrf.Value)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
				t.Fatalf("boundary escaped: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
