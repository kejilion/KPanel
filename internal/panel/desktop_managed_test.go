package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

// Uses real HTTP, enrollment and Noise, with synthetic credentials and no SAM.
func TestDesktopManagedHTTPLeaseAdmissionAndRevocation(t *testing.T) {
	for _, ending := range []string{"close", "logout", "policy", "websocket"} {
		t.Run(ending, func(t *testing.T) {
			s, tokenPath := newTestServer(t)
			cookie, csrf := bootstrapCookies(t, s, tokenPath)
			issued := desktopcredentials.Credentials{Username: "kp_rdp_synthetic", Domain: "TESTBOX", Password: "synthetic-not-real"}
			var prepared atomic.Int32
			cleaned := make(chan struct{}, 1)
			host := desktopCredentialHostWithProvider(t, s, func(ctx context.Context, stream io.ReadWriteCloser, _ string) error {
				_, err := io.Copy(stream, stream)
				return err
			}, func(context.Context) (desktopcredentials.Credentials, func() error, error) {
				prepared.Add(1)
				return issued, func() error { cleaned <- struct{}{}; return nil }, nil
			})
			// Managed mode must not depend on the optional saved-password vault.
			s.desktopCredentials = nil
			headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
			post := func(path string, value any) *httptest.ResponseRecorder {
				body, _ := json.Marshal(value)
				return authenticatedRequest(s, http.MethodPost, desktopSessionsPath+path, body, cookie, csrf, headers)
			}
			if status := post("/credentials/status", map[string]string{"hostId": host}); status.Code != 200 || !strings.Contains(status.Body.String(), `"managed":true`) || strings.Contains(status.Body.String(), issued.Password) {
				t.Fatal("managed availability without vault", status.Code, status.Body.String())
			}
			input := map[string]any{"hostId": host, "useManagedCredentials": true}
			body, _ := json.Marshal(input)
			badHeaders := map[string]string{"Origin": "http://evil.test", "X-CSRF-Token": csrf.Value}
			if bad := authenticatedRequest(s, http.MethodPost, desktopSessionsPath, body, cookie, csrf, badHeaders); bad.Code < 400 || prepared.Load() != 0 {
				t.Fatal("invalid origin prepared administrator")
			}
			if bad := post("", map[string]any{"hostId": host, "useManagedCredentials": true, "useSavedCredentials": true}); bad.Code != 400 || prepared.Load() != 0 {
				t.Fatal("ambiguous credential source admitted")
			}
			opened := post("", input)
			if opened.Code != 201 || opened.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(opened.Code, opened.Body.String())
			}
			var session struct {
				SessionID   string                         `json:"sessionId"`
				Credentials desktopcredentials.Credentials `json:"credentials"`
			}
			if json.Unmarshal(opened.Body.Bytes(), &session) != nil || session.Credentials != issued || prepared.Load() != 1 {
				t.Fatal("managed credential handoff")
			}
			if duplicate := post("", input); duplicate.Code != 429 || prepared.Load() != 1 {
				t.Fatal("host quota must precede account mutation")
			}
			switch ending {
			case "close":
				if response := post("/"+session.SessionID+"/close", nil); response.Code != 200 {
					t.Fatal(response.Code)
				}
			case "logout":
				if err := s.auth.Logout(cookie.Value); err != nil {
					t.Fatal(err)
				}
			case "policy":
				if err := s.cluster.SetDesktopAllowed(host, false); err != nil {
					t.Fatal(err)
				}
			case "websocket":
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; s.ServeHTTP(w, r) }))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				header := http.Header{"Origin": {"http://panel.test"}, "Cookie": {cookie.String() + "; " + csrf.String()}}
				ws, _, err := websocket.Dial(ctx, server.URL+desktopSessionsPath+"/"+session.SessionID+"/stream", &websocket.DialOptions{HTTPHeader: header})
				if err != nil {
					t.Fatal(err)
				}
				defer ws.CloseNow()
				if err := ws.Write(ctx, websocket.MessageBinary, []byte("same-lease")); err != nil {
					t.Fatal(err)
				}
				_, echoed, err := ws.Read(ctx)
				if err != nil || string(echoed) != "same-lease" || prepared.Load() != 1 {
					t.Fatal("claim reopened credentials", err)
				}
				ws.CloseNow()
			}
			select {
			case <-cleaned:
			case <-time.After(3 * time.Second):
				t.Fatal("administrator lease not revoked")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				s.desktopSessionMu.Lock()
				remaining := len(s.desktopSessions)
				s.desktopSessionMu.Unlock()
				if remaining == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("revoked lease retained quota")
				}
				time.Sleep(10 * time.Millisecond)
			}
			state, err := os.ReadFile(s.config.StorePath)
			if err != nil || strings.Contains(string(state), issued.Password) || strings.Contains(string(state), issued.Username) {
				t.Fatal("managed account entered ordinary state/audit", err)
			}
		})
	}
}
