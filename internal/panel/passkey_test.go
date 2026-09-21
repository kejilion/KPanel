package panel

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/auth"
)

func passkeyRequest(s *Server, method, path string, body []byte, cookies []*http.Cookie, headers map[string]string, secure bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://panel.test"+path, bytes.NewReader(body))
	r.RemoteAddr = "192.0.2.1:2345"
	if !secure {
		r.TLS = nil
		r.URL.Scheme = "http"
	} else {
		r.TLS = &tls.ConnectionState{}
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestPasskeyHTTPOriginSessionCSRFAndCookieBoundary(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	s.config.PublicURL = "https://panel.test"
	var err error
	s.passkeys, err = auth.NewPasskeyService(s.auth, s.config.PublicURL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{session, csrf}
	headers := map[string]string{"Origin": "https://panel.test", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value}
	available := passkeyRequest(s, "GET", passkeyPrefix+"/status", nil, nil, nil, true)
	if available.Code != 200 || !strings.Contains(available.Body.String(), `"available":true`) {
		t.Fatal(available.Code, available.Body.String())
	}
	unavailable := passkeyRequest(s, "GET", passkeyPrefix+"/status", nil, nil, nil, false)
	if !strings.Contains(unavailable.Body.String(), `"available":false`) {
		t.Fatal(unavailable.Body.String())
	}
	anonymous := passkeyRequest(s, "GET", passkeyPrefix, nil, nil, nil, true)
	if anonymous.Code != 401 {
		t.Fatal(anonymous.Code)
	}
	body := []byte(`{"password":"a-strong-password-1","name":"Laptop"}`)
	for _, tc := range []string{"origin", "csrf", "session", "transport", "missing-origin"} {
		t.Run(tc, func(t *testing.T) {
			h := map[string]string{}
			for k, v := range headers {
				h[k] = v
			}
			c := cookies
			secure := true
			switch tc {
			case "origin":
				h["Origin"] = "https://evil.example"
			case "csrf":
				delete(h, "X-CSRF-Token")
			case "session":
				c = nil
			case "transport":
				secure = false
			case "missing-origin":
				delete(h, "Origin")
			}
			response := passkeyRequest(s, "POST", passkeyPrefix+"/register/begin", body, c, h, secure)
			if response.Code < 400 {
				t.Fatal("accepted unsafe enrollment", response.Code)
			}
		})
	}
	started := passkeyRequest(s, "POST", passkeyPrefix+"/register/begin", body, cookies, headers, true)
	if started.Code != 200 {
		t.Fatal(started.Code, started.Body.String())
	}
	var o struct {
		CeremonyID string `json:"ceremonyId"`
		PublicKey  struct {
			AuthenticatorSelection struct {
				UserVerification string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	if err = json.Unmarshal(started.Body.Bytes(), &o); err != nil || o.PublicKey.AuthenticatorSelection.UserVerification != "required" {
		t.Fatal(err, started.Body.String())
	}
	failed := passkeyRequest(s, "POST", passkeyPrefix+"/register/finish", []byte(`{"ceremonyId":"`+o.CeremonyID+`","credential":{}}`), cookies, headers, true)
	if failed.Code != 401 {
		t.Fatal(failed.Code, failed.Body.String())
	}
	begin := passkeyRequest(s, "POST", passkeyPrefix+"/login/begin", []byte(`{"username":"missing"}`), nil, headers, true)
	if begin.Code != 200 {
		t.Fatal(begin.Code, begin.Body.String())
	}
	cs := begin.Result().Cookies()
	if len(cs) != 1 || !cs[0].HttpOnly || !cs[0].Secure || cs[0].SameSite != http.SameSiteStrictMode || cs[0].Path != passkeyPrefix {
		t.Fatal("unsafe binding cookie", cs)
	}
	if strings.Contains(begin.Body.String(), "allowCredentials") {
		t.Fatal("begin discloses registered credentials")
	}
	audit, _, err := s.store.ListAudit(100, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(audit)
	for _, secret := range []string{"a-strong-password-1", "Laptop", o.CeremonyID, cs[0].Value} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("audit disclosed sensitive ceremony data")
		}
	}
}

func TestPasskeyHTTPFixedOriginDoesNotTrustDynamicProxyHost(t *testing.T) {
	s, _ := newTestServerWithPublicURL(t, "https://panel.test")
	for _, host := range []string{"panel.test", "evil.example"} {
		req := httptest.NewRequest("GET", "https://"+host+passkeyPrefix+"/status", nil)
		req.TLS = nil
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-Forwarded-Proto", "https")
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		var status struct {
			Available bool `json:"available"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Available != (host == "panel.test") {
			t.Fatal("dynamic host changed passkey RP", host, status.Available)
		}
	}
}

func TestPasskeyHTTPAuditUnavailableFailsBeforeEnrollment(t *testing.T) {
	s, path := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, path)
	s.config.PublicURL = "https://panel.test"
	s.passkeys, _ = auth.NewPasskeyService(s.auth, s.config.PublicURL)
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	w := passkeyRequest(s, "POST", passkeyPrefix+"/register/begin", []byte(`{"password":"a-strong-password-1","name":"key"}`), []*http.Cookie{session, csrf}, map[string]string{"Origin": "https://panel.test", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value}, true)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "audit_unavailable") {
		t.Fatal(w.Code, w.Body.String())
	}
}
