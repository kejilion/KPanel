package panel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDesktopStreamRejectsOutputAfterLogout(t *testing.T) {
	s, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	afterLogout := make(chan struct{})
	host := desktopCredentialHostWithHandler(t, s, func(ctx context.Context, stream io.ReadWriteCloser, _ string) error {
		if _, err := stream.Write([]byte("authorized-frame")); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-afterLogout:
		}
		_, err := stream.Write([]byte("revoked-frame"))
		if err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	})
	body, _ := json.Marshal(map[string]string{"hostId": host})
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	opened := authenticatedRequest(s, http.MethodPost, desktopSessionsPath, body, cookie, csrf, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open: %d %s", opened.Code, opened.Body.String())
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(opened.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "panel.test"
		s.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{"Origin": []string{"http://panel.test"}, "Cookie": []string{cookie.String() + "; " + csrf.String()}}
	ws, _, err := websocket.Dial(ctx, server.URL+desktopSessionsPath+"/"+session.SessionID+"/stream", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	kind, data, err := ws.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || string(data) != "authorized-frame" {
		t.Fatalf("authorized output: %q %v", data, err)
	}
	// No output was in flight at logout: the node emits the next frame only
	// after deletion of the exact browser login has completed successfully.
	if err := s.auth.Logout(cookie.Value); err != nil {
		t.Fatal(err)
	}
	close(afterLogout)
	if _, data, err := ws.Read(ctx); err == nil {
		t.Fatalf("desktop output delivered after logout: %q", data)
	} else if ctx.Err() != nil {
		t.Fatal("revoked desktop did not close before test deadline", err)
	}
}

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
