package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBrowserStatusRequiresSessionAndKeepsServiceTokenPrivate(t *testing.T) {
	s, path := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, path)
	r := httptest.NewRequest("GET", "/api/v1/browser/status", nil)
	r.Host = "panel.test"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous browser status accepted", w.Code)
	}
	w = authenticatedSiteRequest(s, session, csrf, "GET", "/api/v1/browser/status", nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "browser_unconfigured") {
		t.Fatal(w.Code, w.Body.String())
	}
	secret := strings.Repeat("browser-secret-", 4)
	var legacy atomic.Bool
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing private service credential")
		}
		if legacy.Load() {
			_, _ = w.Write([]byte(`{"ready":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ready":true,"transport":"vnc"}`))
	}))
	defer service.Close()
	s.config.BrowserServiceURL = service.URL
	s.config.BrowserTokenFile = filepath.Join(t.TempDir(), "token")
	if os.WriteFile(s.config.BrowserTokenFile, []byte(secret), 0600) != nil {
		t.Fatal("secret write failed")
	}
	w = authenticatedSiteRequest(s, session, csrf, "GET", "/api/v1/browser/status", nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":true`) || strings.Contains(w.Body.String(), secret) {
		t.Fatal("invalid browser status")
	}
	legacy.Store(true)
	w = authenticatedSiteRequest(s, session, csrf, "GET", "/api/v1/browser/status", nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":false`) || !strings.Contains(w.Body.String(), "browser_incompatible") {
		t.Fatal("legacy browser was not rejected", w.Body.String())
	}
	for _, bad := range []string{"/api/v1/browser/status?token=x", "/api/v1/browser/stream/not-an-id"} {
		w = authenticatedSiteRequest(s, session, csrf, "GET", bad, nil, false)
		if w.Code != 404 {
			t.Fatalf("invalid path accepted: %s (%d)", bad, w.Code)
		}
	}
}

func TestBrowserSocketAuthenticatesBeforeWorkerAndRejectsRawCDP(t *testing.T) {
	s, path := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, path)
	secret := strings.Repeat("b", 48)
	s.config.BrowserTokenFile = filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(s.config.BrowserTokenFile, []byte(secret), 0600)
	var connections, commands atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+secret || strings.Contains(r.URL.Path, session.Value) {
			t.Error("invalid browser service binding")
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"state","tabs":[],"active":""}`))
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var value map[string]any
			_ = json.Unmarshal(data, &value)
			if value["type"] == "Runtime.evaluate" {
				t.Error("raw CDP reached worker")
			}
			commands.Add(1)
		}
	}))
	defer service.Close()
	s.config.BrowserServiceURL = service.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; s.ServeHTTP(w, r) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	headers := http.Header{"Origin": {"http://panel.test"}, "Cookie": {session.Name + "=" + session.Value + "; " + csrf.Name + "=" + csrf.Value}}
	dial := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/browser/stream/"+strings.Repeat("a", 32), &websocket.DialOptions{HTTPHeader: headers})
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	bad := dial()
	_ = bad.Write(ctx, websocket.MessageText, []byte(`{"csrf":"wrong"}`))
	_, _, _ = bad.Read(ctx)
	_ = bad.CloseNow()
	if connections.Load() != 0 {
		t.Fatal("invalid CSRF reached worker")
	}
	ws := dial()
	defer ws.CloseNow()
	hello, _ := json.Marshal(map[string]string{"csrf": csrf.Value})
	_ = ws.Write(ctx, websocket.MessageText, hello)
	_, data, err := ws.Read(ctx)
	if err != nil || !strings.Contains(string(data), "state") {
		t.Fatal("authenticated browser did not connect", err)
	}
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"Runtime.evaluate","text":"1+1"}`))
	_, _, _ = ws.Read(ctx)
	if connections.Load() != 1 || commands.Load() != 0 {
		t.Fatal("invalid command forwarded")
	}
}

func TestBrowserVNCSocketAuthenticatesAndRelaysBinary(t *testing.T) {
	s, path := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, path)
	secret := strings.Repeat("v", 48)
	s.config.BrowserTokenFile = filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(s.config.BrowserTokenFile, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	var connections atomic.Int32
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+secret || !strings.HasSuffix(r.URL.Path, "/vnc") || strings.Contains(r.URL.Path, session.Value) {
			t.Error("invalid VNC service binding")
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if ws.Write(ctx, websocket.MessageBinary, []byte("RFB")) != nil {
			return
		}
		kind, data, err := ws.Read(ctx)
		if err != nil || kind != websocket.MessageBinary || string(data) != "client" {
			t.Errorf("invalid client relay: %q %v", data, err)
		}
	}))
	defer service.Close()
	s.config.BrowserServiceURL = service.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; s.ServeHTTP(w, r) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := http.Header{"Origin": {"http://panel.test"}, "Cookie": {session.Name + "=" + session.Value + "; " + csrf.Name + "=" + csrf.Value}}
	dial := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/browser/vnc/"+strings.Repeat("a", 32), &websocket.DialOptions{HTTPHeader: headers})
		if err != nil {
			t.Fatal(err)
		}
		return ws
	}
	bad := dial()
	_ = bad.Write(ctx, websocket.MessageText, []byte(`{"csrf":"wrong"}`))
	_, _, _ = bad.Read(ctx)
	bad.CloseNow()
	if connections.Load() != 0 {
		t.Fatal("invalid CSRF reached VNC worker")
	}
	ws := dial()
	defer ws.CloseNow()
	hello, _ := json.Marshal(map[string]string{"csrf": csrf.Value})
	if err := ws.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	kind, data, err := ws.Read(ctx)
	if err != nil || kind != websocket.MessageText || string(data) != `{"ready":true}` {
		t.Fatalf("invalid VNC handshake: %q %v", data, err)
	}
	kind, data, err = ws.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || string(data) != "RFB" {
		t.Fatalf("invalid server relay: %q %v", data, err)
	}
	if err := ws.Write(ctx, websocket.MessageBinary, []byte("client")); err != nil {
		t.Fatal(err)
	}
	_, _, _ = ws.Read(ctx)
	if connections.Load() != 1 {
		t.Fatal("unexpected worker connections")
	}
}
