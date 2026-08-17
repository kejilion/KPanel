package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/store"
)

func newTestServer(t *testing.T) (*Server, string) {
	return newTestServerWithPublicURL(t, "http://panel.test")
}

func newTestServerWithPublicURL(t *testing.T, publicURL string) (*Server, string) {
	t.Helper()
	directory := t.TempDir()
	webRoot := filepath.Join(directory, "web")
	dataDir := filepath.Join(directory, "data")
	if err := os.MkdirAll(webRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<!doctype html><title>panel</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.DataDir = dataDir
	config.StorePath = filepath.Join(dataDir, "state.json")
	config.BootstrapTokenPath = filepath.Join(dataDir, "bootstrap.token")
	config.AgentSocket = filepath.Join(directory, "run", "agent.sock")
	config.AgentTokenFile = filepath.Join(directory, "secrets", "agent.token")
	config.WebRoot = webRoot
	config.PublicURL = publicURL
	config.SecureCookie = false
	config.CookieName = "kejilion_session"
	config.SessionTTL = time.Hour
	config.SessionTTLText = "1h"

	storage, err := store.Open(config.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	hasher, err := auth.NewArgon2idHasher(auth.Argon2idParams{
		MemoryKiB: 8 * 1024, Iterations: 1, Threads: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(storage, hasher, auth.Config{
		BootstrapTokenPath: config.BootstrapTokenPath,
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(
		config,
		authService,
		storage,
		NewAgentClient(config.AgentSocket, config.AgentTokenFile, config.MaxAgentBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, config.BootstrapTokenPath
}

func TestAuthenticationHTTPFlow(t *testing.T) {
	server, tokenPath := newTestServer(t)

	status := performRequest(server, http.MethodGet, "/api/v1/auth/bootstrap", nil, nil)
	if status.Code != http.StatusOK || status.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("unexpected bootstrap status: %d headers=%v", status.Code, status.Header())
	}
	if policy := status.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "frame-src 'self' blob:") || strings.Contains(policy, " http:") || strings.Contains(policy, " https:") {
		t.Fatalf("restricted frame policy missing: %q", policy)
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	bootstrap := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
		"Origin":       "http://panel.test",
	})
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", bootstrap.Code, bootstrap.Body.String())
	}
	cookies := bootstrap.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected session and CSRF cookies, got %d", len(cookies))
	}
	var sessionCookie, csrfCookie *http.Cookie
	for _, cookie := range cookies {
		switch cookie.Name {
		case "kejilion_session":
			sessionCookie = cookie
		case "kejilion_csrf":
			csrfCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || csrfCookie == nil || csrfCookie.HttpOnly {
		t.Fatalf("unexpected cookie flags: %#v", cookies)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.Host = "panel.test"
	sessionRequest.AddCookie(sessionCookie)
	sessionRequest.AddCookie(csrfCookie)
	sessionResponse := httptest.NewRecorder()
	server.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK {
		t.Fatalf("session failed: %d %s", sessionResponse.Code, sessionResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRequest.Host = "panel.test"
	logoutRequest.Header.Set("Origin", "http://panel.test")
	logoutRequest.Header.Set("X-CSRF-Token", csrfCookie.Value)
	logoutRequest.AddCookie(sessionCookie)
	logoutRequest.AddCookie(csrfCookie)
	logoutResponse := httptest.NewRecorder()
	server.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("logout failed: %d %s", logoutResponse.Code, logoutResponse.Body.String())
	}

	expiredRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	expiredRequest.Host = "panel.test"
	expiredRequest.AddCookie(sessionCookie)
	expiredResponse := httptest.NewRecorder()
	server.ServeHTTP(expiredResponse, expiredRequest)
	if expiredResponse.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out session accepted: %d", expiredResponse.Code)
	}
}

func TestSecurityEntranceGatesLoginWithoutBreakingSessions(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	_, resourceVersion := server.store.SecurityEntrance()
	body, err := json.Marshal(map[string]any{
		"enabled": true, "path": "panel-secure1", "expectedResourceVersion": resourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := authenticatedRequest(server, http.MethodPut, "/api/v1/settings/security-entry", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		})
	if updated.Code != http.StatusOK {
		t.Fatalf("enable security entry failed: %d %s", updated.Code, updated.Body.String())
	}

	directLogin := performRequest(server, http.MethodGet, "/login", nil, nil)
	if directLogin.Code != http.StatusNotFound {
		t.Fatalf("direct login should be hidden, got %d", directLogin.Code)
	}
	directAPI := loginRequest(server, "a-strong-password")
	if directAPI.Code != http.StatusNotFound {
		t.Fatalf("direct login API should be hidden, got %d", directAPI.Code)
	}

	entry := performRequest(server, http.MethodGet, "/panel-secure1", nil, nil)
	if entry.Code != http.StatusOK || entry.Header().Get("Location") != "" {
		t.Fatalf("unexpected entrance response: %d %s", entry.Code, entry.Header().Get("Location"))
	}
	if entry.Header().Get("Cache-Control") != "no-store" ||
		entry.Header().Get("Referrer-Policy") != "no-referrer" ||
		entry.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(entry.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		!strings.Contains(entry.Body.String(), `http-equiv="refresh" content="0;url=/login"`) ||
		!strings.Contains(entry.Body.String(), `href="/login"`) {
		t.Fatalf("security entrance transition page is incomplete: headers=%v body=%s", entry.Header(), entry.Body.String())
	}
	var entryCookie *http.Cookie
	for _, cookie := range entry.Result().Cookies() {
		if cookie.Name == "kejilion_entry" {
			entryCookie = cookie
		}
	}
	if entryCookie == nil || !entryCookie.HttpOnly || entryCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected entrance cookie: %#v", entry.Result().Cookies())
	}
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "a-strong-password"})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginRequest.Host = "panel.test"
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("Origin", "http://panel.test")
	loginRequest.AddCookie(entryCookie)
	loginResponse := httptest.NewRecorder()
	server.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("entrance cookie did not unlock login API: %d %s", loginResponse.Code, loginResponse.Body.String())
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.Host = "panel.test"
	sessionRequest.AddCookie(sessionCookie)
	sessionRequest.AddCookie(csrfCookie)
	sessionPage := httptest.NewRecorder()
	server.ServeHTTP(sessionPage, sessionRequest)
	if sessionPage.Code != http.StatusOK {
		t.Fatalf("existing session was gated: %d", sessionPage.Code)
	}
}

func TestPasswordChangeGuardsAndValidation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	validBody, err := json.Marshal(map[string]string{
		"currentPassword": "a-strong-password",
		"newPassword":     "a-new-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}

	missingOrigin := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/password", validBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if missingOrigin.Code != http.StatusForbidden ||
		!strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin returned %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}

	missingCSRF := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/password", validBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
		},
	)
	if missingCSRF.Code != http.StatusForbidden ||
		!strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF token returned %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}

	tests := []struct {
		name          string
		current       string
		next          string
		expectedField string
	}{
		{
			name: "incorrect current password", current: "incorrect-password",
			next: "a-new-strong-password", expectedField: "currentPassword",
		},
		{
			name: "weak new password", current: "a-strong-password",
			next: "too-short", expectedField: "newPassword",
		},
		{
			name: "unchanged password", current: "a-strong-password",
			next: "a-strong-password", expectedField: "newPassword",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, marshalErr := json.Marshal(map[string]string{
				"currentPassword": test.current,
				"newPassword":     test.next,
			})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			response := authenticatedRequest(
				server, http.MethodPut, "/api/v1/settings/password", body,
				sessionCookie, csrfCookie, map[string]string{
					"Content-Type": "application/json",
					"Origin":       "http://panel.test",
					"X-CSRF-Token": csrfCookie.Value,
				},
			)
			if response.Code != http.StatusUnprocessableEntity ||
				!strings.Contains(response.Body.String(), `"`+test.expectedField+`"`) {
				t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
			}
		})
	}

	oldLogin := loginRequest(server, "a-strong-password")
	if oldLogin.Code != http.StatusOK {
		t.Fatalf("validation failure changed the password: %d %s", oldLogin.Code, oldLogin.Body.String())
	}
	events, _ := server.store.ListAudit(50, "")
	passwordEvents := 0
	for _, event := range events {
		if event.Action != "auth.password.change" {
			continue
		}
		passwordEvents++
		if event.Change != nil {
			t.Fatalf("password audit event contains change data: %#v", event.Change)
		}
	}
	if passwordEvents != 6 {
		t.Fatalf("expected three intent/failure audit pairs, got %d events", passwordEvents)
	}
}

func TestPasswordChangeInvalidatesSessionsAndCredentials(t *testing.T) {
	server, tokenPath := newTestServer(t)
	firstSession, firstCSRF := bootstrapCookies(t, server, tokenPath)
	secondLogin := loginRequest(server, "a-strong-password")
	if secondLogin.Code != http.StatusOK {
		t.Fatalf("second login failed: %d %s", secondLogin.Code, secondLogin.Body.String())
	}
	secondSession, secondCSRF := authCookies(t, secondLogin)

	body, err := json.Marshal(map[string]string{
		"currentPassword": "a-strong-password",
		"newPassword":     "a-new-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/password", body,
		firstSession, firstCSRF, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": firstCSRF.Value,
		},
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("password change failed: %d %s", response.Code, response.Body.String())
	}
	cleared := map[string]bool{}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Value == "" && cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
	}
	if !cleared["kejilion_session"] || !cleared["kejilion_csrf"] {
		t.Fatalf("authentication cookies were not cleared: %#v", response.Result().Cookies())
	}

	for name, cookies := range map[string][]*http.Cookie{
		"changing session": {firstSession, firstCSRF},
		"second session":   {secondSession, secondCSRF},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		request.Host = "panel.test"
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		sessionResponse := httptest.NewRecorder()
		server.ServeHTTP(sessionResponse, request)
		if sessionResponse.Code != http.StatusUnauthorized {
			t.Fatalf("%s remained valid: %d %s", name, sessionResponse.Code, sessionResponse.Body.String())
		}
	}

	oldLogin := loginRequest(server, "a-strong-password")
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password remained valid: %d %s", oldLogin.Code, oldLogin.Body.String())
	}
	newLogin := loginRequest(server, "a-new-strong-password")
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login failed: %d %s", newLogin.Code, newLogin.Body.String())
	}

	events, _ := server.store.ListAudit(50, "")
	successes := 0
	for _, event := range events {
		if event.Action == "auth.password.change" && event.Result == "success" {
			successes++
			if event.Change != nil {
				t.Fatalf("successful password audit contains change data: %#v", event.Change)
			}
		}
	}
	if successes != 1 {
		t.Fatalf("expected one successful password audit event, got %d", successes)
	}
}

func TestUsernameChangeInvalidatesSessionsAndCredentials(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	body, err := json.Marshal(map[string]string{
		"currentPassword": "a-strong-password",
		"newUsername":     "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingOrigin := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/username", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin returned %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/username", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test",
		},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF token returned %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	response := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/username", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("username change failed: %d %s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	sessionResponse := httptest.NewRecorder()
	server.ServeHTTP(sessionResponse, request)
	if sessionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("changing session remained valid: %d %s", sessionResponse.Code, sessionResponse.Body.String())
	}

	oldLogin := loginRequest(server, "a-strong-password")
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old username remained valid: %d %s", oldLogin.Code, oldLogin.Body.String())
	}
	loginBody, _ := json.Marshal(map[string]string{"username": "operator", "password": "a-strong-password"})
	newLogin := performRequest(server, http.MethodPost, "/api/v1/auth/login", loginBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new username login failed: %d %s", newLogin.Code, newLogin.Body.String())
	}
	events, _ := server.store.ListAudit(50, "")
	for _, event := range events {
		if event.Action == "auth.username.change" && event.Change != nil {
			t.Fatalf("username audit event contains identity data: %#v", event.Change)
		}
	}
}

func TestPasswordChangeRequestBodyBound(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.config.MaxRequestBytes = 1024
	body, err := json.Marshal(map[string]string{
		"currentPassword": strings.Repeat("x", 2048),
		"newPassword":     "a-new-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(
		server, http.MethodPut, "/api/v1/settings/password", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusRequestEntityTooLarge ||
		!strings.Contains(response.Body.String(), "request_too_large") {
		t.Fatalf("oversized body returned %d %s", response.Code, response.Body.String())
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.Host = "panel.test"
	sessionRequest.AddCookie(sessionCookie)
	sessionRequest.AddCookie(csrfCookie)
	sessionResponse := httptest.NewRecorder()
	server.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK {
		t.Fatalf("oversized request changed the session: %d %s", sessionResponse.Code, sessionResponse.Body.String())
	}
}

func TestRejectsCrossOriginBootstrap(t *testing.T) {
	server, tokenPath := newTestServer(t)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	response := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
		"Origin":       "https://evil.example",
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin bootstrap returned %d", response.Code)
	}
}

func TestRejectsMissingOriginAndUnexpectedHost(t *testing.T) {
	server, tokenPath := newTestServer(t)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})

	missingOrigin := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
	})
	if missingOrigin.Code != http.StatusForbidden {
		t.Fatalf("missing Origin returned %d", missingOrigin.Code)
	}

	unexpectedHost := performRequest(server, http.MethodGet, "/api/v1/auth/bootstrap", nil, map[string]string{
		"Host": "attacker.test",
	})
	if unexpectedHost.Code != http.StatusMisdirectedRequest {
		t.Fatalf("unexpected Host returned %d", unexpectedHost.Code)
	}
}

func TestAllowIPHostsSupportsNATWithoutAllowingDNSHosts(t *testing.T) {
	server, tokenPath := newTestServer(t)
	server.config.AllowIPHosts = true

	for _, host := range []string{
		"192.168.1.7:18080",
		"198.51.100.25:18080",
		"[fd00::25]:18080",
	} {
		response := performRequest(server, http.MethodGet, "/api/v1/auth/bootstrap", nil, map[string]string{
			"Host": host,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("IP Host %q returned %d %s", host, response.Code, response.Body.String())
		}
	}
	for _, host := range []string{
		"panel.lan:18080",
		"192.168.1.7:65536",
		"192.168.1.7@evil.test",
	} {
		response := performRequest(server, http.MethodGet, "/api/v1/auth/bootstrap", nil, map[string]string{
			"Host": host,
		})
		if response.Code != http.StatusMisdirectedRequest {
			t.Fatalf("unsafe Host %q returned %d %s", host, response.Code, response.Body.String())
		}
	}

	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	crossOrigin := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
		"Host":         "192.168.1.7:18080",
		"Origin":       "http://198.51.100.25:18080",
	})
	if crossOrigin.Code != http.StatusForbidden {
		t.Fatalf("cross-origin NAT request returned %d %s", crossOrigin.Code, crossOrigin.Body.String())
	}
	sameOrigin := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
		"Host":         "192.168.1.7:18080",
		"Origin":       "http://192.168.1.7:18080",
	})
	if sameOrigin.Code != http.StatusCreated {
		t.Fatalf("same-origin NAT request returned %d %s", sameOrigin.Code, sameOrigin.Body.String())
	}
}

func TestTrustedHTTPSProxyAllowsKFDOriginAndSecureCookies(t *testing.T) {
	server, tokenPath := newTestServer(t)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body))
	request.RemoteAddr = "127.0.0.1:12345"
	request.Host = "panel.example.com"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://panel.example.com")
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("trusted HTTPS proxy was rejected: %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("trusted HTTPS response HSTS = %q", got)
	}
	for _, cookie := range response.Result().Cookies() {
		if !cookie.Secure {
			t.Fatalf("proxy HTTPS cookie is not Secure: %#v", cookie)
		}
	}
}

func TestForwardedOriginRequiresTrustedPeerAndCleanHTTPSHeaders(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		host           string
		forwardedProto string
	}{
		{
			name: "untrusted peer", remoteAddr: "192.0.2.10:12345",
			host: "panel.example.com", forwardedProto: "https",
		},
		{
			name: "multiple forwarded schemes", remoteAddr: "127.0.0.1:12345",
			host: "panel.example.com", forwardedProto: "https, http",
		},
		{
			name: "host with user info", remoteAddr: "127.0.0.1:12345",
			host: "panel.example.com@evil.example", forwardedProto: "https",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, _ := newTestServer(t)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/bootstrap", nil)
			request.RemoteAddr = test.remoteAddr
			request.Host = test.host
			request.Header.Set("X-Forwarded-Proto", test.forwardedProto)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusMisdirectedRequest {
				t.Fatalf("unsafe forwarded origin returned %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSPAFallback(t *testing.T) {
	server, _ := newTestServer(t)
	response := performRequest(server, http.MethodGet, "/sites/example", nil, nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("<title>panel</title>")) {
		t.Fatalf("SPA fallback failed: %d %s", response.Code, response.Body.String())
	}
}

func TestStaticFilesRejectSymbolicLinks(t *testing.T) {
	server, _ := newTestServer(t)
	outside := filepath.Join(filepath.Dir(server.config.WebRoot), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("must-not-leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(server.config.WebRoot, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	response := performRequest(server, http.MethodGet, "/linked/secret.txt", nil, nil)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "must-not-leak") {
		t.Fatalf("symbolic-link target was served: %d %q", response.Code, response.Body.String())
	}
}

func TestRemoteIPTrustsOnlyConfiguredProxies(t *testing.T) {
	server, _ := newTestServer(t)

	trusted := httptest.NewRequest(http.MethodGet, "/", nil)
	trusted.RemoteAddr = "127.0.0.1:12345"
	trusted.Header.Set("X-Real-IP", "198.51.100.25")
	if got := server.remoteIP(trusted); got != "198.51.100.25" {
		t.Fatalf("trusted proxy address was ignored: %q", got)
	}

	untrusted := httptest.NewRequest(http.MethodGet, "/", nil)
	untrusted.RemoteAddr = "192.0.2.10:12345"
	untrusted.Header.Set("X-Real-IP", "198.51.100.25")
	if got := server.remoteIP(untrusted); got != "192.0.2.10" {
		t.Fatalf("untrusted proxy spoofed client address: %q", got)
	}

	forwarded := httptest.NewRequest(http.MethodGet, "/", nil)
	forwarded.RemoteAddr = "127.0.0.1:12345"
	forwarded.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.25")
	if got := server.remoteIP(forwarded); got != "198.51.100.25" {
		t.Fatalf("trusted proxy did not select the rightmost client address: %q", got)
	}

	trustedChain := httptest.NewRequest(http.MethodGet, "/", nil)
	trustedChain.RemoteAddr = "127.0.0.1:12345"
	trustedChain.Header.Set("X-Forwarded-For", "198.51.100.25, 127.0.0.2")
	if got := server.remoteIP(trustedChain); got != "198.51.100.25" {
		t.Fatalf("trusted proxy chain did not skip trusted hops: %q", got)
	}

	untrustedForwarded := httptest.NewRequest(http.MethodGet, "/", nil)
	untrustedForwarded.RemoteAddr = "192.0.2.10:12345"
	untrustedForwarded.Header.Set("X-Forwarded-For", "198.51.100.25")
	if got := server.remoteIP(untrustedForwarded); got != "192.0.2.10" {
		t.Fatalf("untrusted peer spoofed X-Forwarded-For: %q", got)
	}
}

func TestDockerActionFailsClosedWhenIntentAuditCannotPersist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot remove an open lock file on Windows")
	}
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	dataDir := server.config.DataDir
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataDir, []byte("block store directory recreation"), 0o600); err != nil {
		t.Fatal(err)
	}

	containerID := strings.Repeat("a", 64)
	body, err := json.Marshal(map[string]string{
		"resourceVersion": "sha256:" + strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/docker/containers/"+containerID+"/restart",
		bytes.NewReader(body),
	)
	request.Host = "panel.test"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://panel.test")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), "audit_unavailable") {
		t.Fatalf("Docker action did not fail closed: %d %s", response.Code, response.Body.String())
	}
}

func TestAllowedDockerActionPath(t *testing.T) {
	t.Parallel()
	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, action := range []string{"start", "stop", "restart", "pause", "unpause", "remove"} {
		path, gotID, gotAction, ok := allowedDockerActionPath(
			"/api/v1/docker/containers/" + id + "/" + action,
		)
		if !ok || gotID != id || gotAction != action ||
			path != "/v1/docker/containers/"+id+"/"+action {
			t.Fatalf(
				"unexpected %s mapping: path=%q id=%q action=%q ok=%v",
				action,
				path,
				gotID,
				gotAction,
				ok,
			)
		}
	}
	for _, invalid := range []string{
		"/api/v1/docker/containers/not-an-id/restart",
		"/api/v1/docker/containers/" + id + "/delete",
		"/api/v1/docker/containers/" + id + "/restart/extra",
	} {
		if _, _, _, ok := allowedDockerActionPath(invalid); ok {
			t.Errorf("accepted unsafe action path %q", invalid)
		}
	}
}

func TestAllowedDockerReadPaths(t *testing.T) {
	t.Parallel()
	id := strings.Repeat("a", 64)
	for publicPath, expected := range map[string]string{
		"/api/v1/docker/environment":                 "/v1/docker/environment",
		"/api/v1/docker/backups":                     "/v1/docker/backups",
		"/api/v1/docker/compose-projects":            "/v1/docker/compose-projects",
		"/api/v1/docker/compose-projects/demo-stack": "/v1/docker/compose-projects/demo-stack",
		"/api/v1/docker/containers/" + id + "/logs":  "/v1/docker/containers/" + id + "/logs",
		"/api/v1/docker/containers/" + id + "/stats": "/v1/docker/containers/" + id + "/stats",
	} {
		path, ok := allowedAgentPath(publicPath)
		if !ok || path != expected {
			t.Fatalf("allowedAgentPath(%q) = %q, %v; want %q", publicPath, path, ok, expected)
		}
	}
	for _, invalid := range []string{
		"/api/v1/docker/environment/extra",
		"/api/v1/docker/backups/extra",
		"/api/v1/docker/compose-projects/../shadow",
		"/api/v1/docker/compose-projects/Demo",
		"/api/v1/docker/containers/not-an-id/stats",
		"/api/v1/docker/containers/" + id + "/stats/extra",
	} {
		if path, ok := allowedAgentPath(invalid); ok {
			t.Fatalf("allowed unsafe Docker read path %q as %q", invalid, path)
		}
	}
}

func TestAllowedMonitoringHistoryPathIsExact(t *testing.T) {
	path, ok := allowedAgentPath("/api/v1/monitoring/history")
	if !ok || path != "/v1/monitoring/history" {
		t.Fatalf("monitoring mapping = %q, %v", path, ok)
	}
	for _, invalid := range []string{
		"/api/v1/monitoring",
		"/api/v1/monitoring/history/extra",
		"/api/v1/monitoring/history/../../system",
	} {
		if path, ok := allowedAgentPath(invalid); ok {
			t.Fatalf("allowed unsafe monitoring path %q as %q", invalid, path)
		}
	}
}

func TestAllowedSystemProcessesPathIsExact(t *testing.T) {
	path, ok := allowedAgentPath("/api/v1/system/processes")
	if !ok || path != "/v1/system/processes" {
		t.Fatalf("process mapping = %q, %v", path, ok)
	}
	for _, invalid := range []string{
		"/api/v1/system/processes/",
		"/api/v1/system/processes/42",
		"/api/v1/system/processes/../../files",
	} {
		if path, ok := allowedAgentPath(invalid); ok {
			t.Fatalf("allowed unsafe process path %q as %q", invalid, path)
		}
	}
}

func TestAllowedWordPressInstallationPath(t *testing.T) {
	id := strings.Repeat("a", 32)
	for publicPath, expected := range map[string]string{
		"/api/v1/site-installations/" + id:               "/v1/site-installations/" + id,
		"/api/v1/site-installations/" + id + "/terminal": "/v1/site-installations/" + id + "/terminal",
	} {
		path, ok := allowedAgentPath(publicPath)
		if !ok || path != expected {
			t.Fatalf("allowedAgentPath(%q) = %q, %v", publicPath, path, ok)
		}
	}
	for _, invalid := range []string{
		"/api/v1/site-installations/",
		"/api/v1/site-installations/" + strings.Repeat("a", 31),
		"/api/v1/site-installations/" + id + "/extra",
		"/api/v1/site-installations/" + id + "/terminal/extra",
		"/api/v1/site-installations/" + strings.Repeat("g", 32),
	} {
		if _, ok := allowedAgentPath(invalid); ok {
			t.Errorf("allowedAgentPath(%q) unexpectedly allowed", invalid)
		}
	}
}

func TestAllowedApplicationJobPath(t *testing.T) {
	id := strings.Repeat("b", 32)
	for publicPath, expected := range map[string]string{
		"/api/v1/app-jobs/" + id:               "/v1/app-jobs/" + id,
		"/api/v1/app-jobs/" + id + "/terminal": "/v1/app-jobs/" + id + "/terminal",
	} {
		path, ok := allowedAgentPath(publicPath)
		if !ok || path != expected {
			t.Fatalf("allowedAgentPath(%q) = %q, %v", publicPath, path, ok)
		}
	}
	if path, ok := allowedAgentPath("/api/v1/app-jobs"); !ok || path != "/v1/app-jobs" {
		t.Fatalf("application job collection mapping = %q, %v", path, ok)
	}
	for _, invalid := range []string{
		"/api/v1/app-jobs/",
		"/api/v1/app-jobs/" + strings.Repeat("b", 31),
		"/api/v1/app-jobs/" + id + "/extra",
		"/api/v1/app-jobs/" + id + "/terminal/extra",
		"/api/v1/app-jobs/" + strings.Repeat("B", 32),
	} {
		if _, ok := allowedAgentPath(invalid); ok {
			t.Errorf("allowedAgentPath(%q) unexpectedly allowed", invalid)
		}
	}
}

func bootstrapCookies(t *testing.T, server *Server, tokenPath string) (*http.Cookie, *http.Cookie) {
	return bootstrapCookiesForOrigin(t, server, tokenPath, "http://panel.test")
}

func bootstrapCookiesForOrigin(t *testing.T, server *Server, tokenPath, origin string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := performRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json",
		"Origin":       origin,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", response.Code, response.Body.String())
	}
	return authCookies(t, response)
}

func authCookies(t *testing.T, response *httptest.ResponseRecorder) (*http.Cookie, *http.Cookie) {
	t.Helper()
	var sessionCookie, csrfCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		switch cookie.Name {
		case "kejilion_session":
			sessionCookie = cookie
		case "kejilion_csrf":
			csrfCookie = cookie
		}
	}
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("authentication cookies missing: %#v", response.Result().Cookies())
	}
	return sessionCookie, csrfCookie
}

func loginRequest(server *Server, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": password})
	return performRequest(server, http.MethodPost, "/api/v1/auth/login", body, map[string]string{
		"Content-Type": "application/json",
		"Origin":       "http://panel.test",
	})
}

func authenticatedRequest(
	handler http.Handler,
	method, path string,
	body []byte,
	sessionCookie, csrfCookie *http.Cookie,
	headers map[string]string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Host = "panel.test"
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performRequest(handler http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Host = "panel.test"
	for name, value := range headers {
		if name == "Host" {
			request.Host = value
		} else {
			request.Header.Set(name, value)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
