package panel

import (
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDesktopSessionOriginCSRFAndExactLoginBinding(t *testing.T) {
	s, tokenPath := newTestServer(t)
	s.desktopSessions = make(map[string]*panelDesktopSession)
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	session, err := s.auth.Authenticate(cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("e", 32)
	s.desktopSessions[id] = &panelDesktopSession{id: id, hostID: "missing", userID: session.User.ID, token: sha256.Sum256([]byte("another login of the same user")), expires: time.Now().Add(time.Minute)}
	url := desktopSessionsPath + "/" + id + "/close"
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	if got := authenticatedRequest(s, http.MethodPost, url, nil, cookie, csrf, headers); got.Code != 404 {
		t.Fatalf("wrong login close: %d %s", got.Code, got.Body.String())
	}
	s.desktopSessions[id].token = sha256.Sum256([]byte(cookie.Value))
	for _, bad := range []map[string]string{
		{"Content-Type": "application/json", "X-CSRF-Token": csrf.Value},
		{"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrf.Value},
		{"Content-Type": "application/json", "Origin": "http://panel.test"},
	} {
		if got := authenticatedRequest(s, http.MethodPost, url, nil, cookie, csrf, bad); got.Code < 400 {
			t.Fatalf("invalid origin/CSRF accepted: %d", got.Code)
		}
		if s.desktopSessions[id] == nil {
			t.Fatal("failed close removed session")
		}
	}
	canceled := false
	s.desktopSessions[id].cancel = func() { canceled = true }
	if got := authenticatedRequest(s, http.MethodPost, url, nil, cookie, csrf, headers); got.Code != 200 {
		t.Fatalf("close: %d %s", got.Code, got.Body.String())
	}
	if !canceled || s.desktopSessions[id] != nil {
		t.Fatal("close did not cancel/remove session")
	}
	if got := authenticatedRequest(s, http.MethodPost, desktopSessionsPath+"/policy", []byte(`{"hostId":"missing"}`), cookie, csrf, headers); got.Code != 400 {
		t.Fatal("omitted policy bool accepted", got.Code)
	}
}

func TestDesktopSessionConsumedAndExpiredCannotOpenStream(t *testing.T) {
	s, tokenPath := newTestServer(t)
	s.desktopSessions = make(map[string]*panelDesktopSession)
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	session, _ := s.auth.Authenticate(cookie.Value)
	headers := map[string]string{"Origin": "http://panel.test"}
	for _, claimed := range []bool{true, false} {
		id := strings.Repeat("f", 32)
		expires := time.Now().Add(-time.Second)
		if claimed {
			expires = time.Now().Add(time.Minute)
		}
		s.desktopSessions[id] = &panelDesktopSession{id: id, hostID: "missing", userID: session.User.ID, token: sha256.Sum256([]byte(cookie.Value)), expires: expires, claimed: claimed}
		for range 2 {
			got := authenticatedRequest(s, http.MethodGet, desktopSessionsPath+"/"+id+"/stream", nil, cookie, csrf, headers)
			if got.Code != 409 {
				t.Fatalf("expired/consumed claim: %d %s", got.Code, got.Body.String())
			}
		}
	}
}
