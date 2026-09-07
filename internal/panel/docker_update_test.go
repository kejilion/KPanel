package panel

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestImageChecksDoNotRewriteAuditStoreButMutationsStillDo(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	body := []byte(`{"resourceVersion":"sha256:` + strings.Repeat("b", 64) + `"}`)
	headers := map[string]string{"Origin": "http://panel.test", "X-CSRF-Token": csrf.Value, "Content-Type": "application/json"}
	before, err := os.ReadFile(s.config.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.config.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/docker/containers/" + strings.Repeat("a", 64), "/api/v1/apps/builtin-28"} {
		for _, code := range []int{http.StatusOK, http.StatusBadGateway, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
			agent := &stubAgent{response: AgentResponse{StatusCode: code, ContentType: "application/json", Body: []byte(`{"status":"current"}`)}}
			if code == http.StatusServiceUnavailable {
				agent.err = errors.New("simulated transport failure")
			}
			s.agent = agent
			response := authenticatedRequest(s, http.MethodPost, path+"/check_update", body, session, csrf, headers)
			if response.Code != code || len(agent.snapshotCalls()) != 1 {
				t.Fatalf("%s code=%d calls=%d", path, response.Code, len(agent.snapshotCalls()))
			}
			after, err := os.ReadFile(s.config.StorePath)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(s.config.StorePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !os.SameFile(info, afterInfo) || !info.ModTime().Equal(afterInfo.ModTime()) {
				t.Fatal("read-only check rewrote the identity/audit store")
			}
		}
	}
	for _, path := range []string{"/api/v1/docker/containers/" + strings.Repeat("a", 64), "/api/v1/apps/builtin-28"} {
		s.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{}`)}}
		response := authenticatedRequest(s, http.MethodPost, path+"/restart", body, session, csrf, headers)
		if response.Code != http.StatusOK {
			t.Fatalf("restart status=%d body=%s", response.Code, response.Body.String())
		}
	}
	events, _ := s.store.ListAudit(100, "")
	for _, action := range []string{"docker.restart", "app.restart"} {
		count := 0
		for _, event := range events {
			if event.Action == action {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("mutation %s lost intent/result auditing: %d", action, count)
		}
	}
}
