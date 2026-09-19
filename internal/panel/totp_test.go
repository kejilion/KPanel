package panel

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTOTPHTTPFlowRequiresProtectedSessionAndCannotBeBypassed(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/settings/totp", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated TOTP status = %d", unauthenticated.Code)
	}

	startBody, _ := json.Marshal(map[string]string{"currentPassword": "a-strong-password-1"})
	missingCSRF := authenticatedRequest(server, http.MethodPost, "/api/v1/settings/totp/enrollment", startBody, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("TOTP enrollment without CSRF = %d", missingCSRF.Code)
	}
	wrongOrigin := authenticatedRequest(server, http.MethodPost, "/api/v1/settings/totp/enrollment", startBody, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "https://attacker.example",
		"X-CSRF-Token": csrfCookie.Value,
	})
	if wrongOrigin.Code != http.StatusForbidden {
		t.Fatalf("TOTP enrollment with hostile Origin = %d", wrongOrigin.Code)
	}

	start := authenticatedRequest(server, http.MethodPost, "/api/v1/settings/totp/enrollment", startBody, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if start.Code != http.StatusCreated {
		t.Fatalf("start TOTP enrollment = %d %s", start.Code, start.Body.String())
	}
	var enrollment struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	verifyCode := panelTestTOTP(t, enrollment.Secret, time.Now().Add(-30*time.Second))
	confirmBody, _ := json.Marshal(map[string]string{"enrollmentId": enrollment.ID, "code": verifyCode})
	confirm := authenticatedRequest(server, http.MethodPut, "/api/v1/settings/totp/enrollment", confirmBody, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm TOTP enrollment = %d %s", confirm.Code, confirm.Body.String())
	}
	var recovery struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(confirm.Body.Bytes(), &recovery); err != nil || len(recovery.RecoveryCodes) != 10 {
		t.Fatalf("unexpected recovery response %#v err=%v", recovery, err)
	}

	oldSession := authenticatedRequest(server, http.MethodGet, "/api/v1/settings/totp", nil, sessionCookie, csrfCookie, nil)
	if oldSession.Code != http.StatusUnauthorized {
		t.Fatalf("old session survived TOTP enable: %d", oldSession.Code)
	}
	passwordOnly := loginRequest(server, "a-strong-password-1")
	if passwordOnly.Code != http.StatusUnauthorized || !strings.Contains(passwordOnly.Body.String(), "totp_required") {
		t.Fatalf("password-only login bypassed TOTP: %d %s", passwordOnly.Code, passwordOnly.Body.String())
	}
	if len(passwordOnly.Result().Cookies()) != 0 {
		t.Fatal("password-only TOTP challenge issued authentication cookies")
	}

	keyPath := filepath.Join(filepath.Dir(tokenPath), "totp-encryption.key")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	unavailableBody, _ := json.Marshal(map[string]string{
		"username": "admin", "password": "a-strong-password-1", "totpCode": panelTestTOTP(t, enrollment.Secret, time.Now()),
	})
	unavailable := performRequest(server, http.MethodPost, "/api/v1/auth/login", unavailableBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if unavailable.Code != http.StatusServiceUnavailable || !strings.Contains(unavailable.Body.String(), "second_factor_unavailable") {
		t.Fatalf("missing TOTP key was not surfaced safely: %d %s", unavailable.Code, unavailable.Body.String())
	}
	recoveryWithoutKeyBody, _ := json.Marshal(map[string]string{
		"username": "admin", "password": "a-strong-password-1", "totpCode": recovery.RecoveryCodes[9],
	})
	recoveryWithoutKey := performRequest(server, http.MethodPost, "/api/v1/auth/login", recoveryWithoutKeyBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if recoveryWithoutKey.Code != http.StatusOK {
		t.Fatalf("recovery code did not remain usable when TOTP key was missing: %d %s", recoveryWithoutKey.Code, recoveryWithoutKey.Body.String())
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin", "password": "a-strong-password-1", "totpCode": panelTestTOTP(t, enrollment.Secret, time.Now()),
	})
	login := performRequest(server, http.MethodPost, "/api/v1/auth/login", loginBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("TOTP login = %d %s", login.Code, login.Body.String())
	}

	recoveryBody, _ := json.Marshal(map[string]string{
		"username": "admin", "password": "a-strong-password-1", "totpCode": recovery.RecoveryCodes[0],
	})
	recoveryLogin := performRequest(server, http.MethodPost, "/api/v1/auth/login", recoveryBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if recoveryLogin.Code != http.StatusOK {
		t.Fatalf("recovery login = %d %s", recoveryLogin.Code, recoveryLogin.Body.String())
	}
	replayed := performRequest(server, http.MethodPost, "/api/v1/auth/login", recoveryBody, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if replayed.Code != http.StatusUnauthorized || !strings.Contains(replayed.Body.String(), "invalid_second_factor") {
		t.Fatalf("recovery code replay was not rejected: %d %s", replayed.Code, replayed.Body.String())
	}

	events, _, _ := server.store.ListAudit(100, "")
	auditJSON, _ := json.Marshal(events)
	if strings.Contains(string(auditJSON), enrollment.Secret) || strings.Contains(string(auditJSON), recovery.RecoveryCodes[0]) {
		t.Fatal("TOTP secret or recovery code leaked into audit records")
	}
}

func panelTestTOTP(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
