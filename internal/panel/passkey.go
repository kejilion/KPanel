package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const passkeyPrefix = "/api/v1/auth/passkeys"
const passkeyCookie = "__Host-kpanel_passkey"

func (s *Server) passkeysAvailable(r *http.Request) bool {
	if s.passkeys == nil || s.passkeys.Origin == "" {
		return false
	}
	origin, ok := s.requestHTTPSOrigin(r)
	return ok && secureStringEqual(strings.ToLower(origin), s.passkeys.Origin)
}

func (s *Server) handlePasskeys(w http.ResponseWriter, r *http.Request) {
	available := s.passkeysAvailable(r)
	suffix := strings.TrimPrefix(r.URL.Path, passkeyPrefix)
	if r.Method == http.MethodGet && suffix == "/status" {
		s.writeJSON(w, http.StatusOK, map[string]bool{"available": available})
		return
	}
	if r.Method == http.MethodGet && suffix == "" {
		_, session, ok := s.requireSession(w, r)
		if !ok {
			return
		}
		if s.passkeys == nil {
			s.writeJSON(w, http.StatusOK, map[string]any{"available": false, "rpId": "", "credentials": []auth.PasskeyInfo{}})
			return
		}
		entries, err := s.passkeys.List(session.User.ID)
		if err != nil {
			s.writePasskeyProblem(w, r, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"available": available, "rpId": s.passkeys.RPID, "credentials": entries})
		return
	}
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if suffix != "/login/begin" && suffix != "/login/finish" && suffix != "/register/begin" && suffix != "/register/finish" && suffix != "/delete" {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Origin") == "" || !s.checkOrigin(w, r) {
		if r.Header.Get("Origin") == "" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
		}
		return
	}
	if suffix != "/delete" && (!available || !secureStringEqual(strings.ToLower(r.Header.Get("Origin")), s.passkeys.Origin)) {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	if s.passkeys == nil {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	if s.backups != nil && s.backups.Busy() {
		s.writeProblem(w, r, http.StatusConflict, "backup_busy", "Backup restore is in progress", "")
		return
	}
	// Bound WebAuthn JSON before decoding; attestation is not an upload API.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if strings.HasPrefix(suffix, "/login/") {
		s.handlePasskeyLogin(w, r, suffix)
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		Password   string          `json:"password"`
		TOTPCode   string          `json:"totpCode,omitempty"`
		Name       string          `json:"name"`
		ID         string          `json:"id"`
		CeremonyID string          `json:"ceremonyId"`
		Credential json.RawMessage `json:"credential"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	action := "auth.passkey.register"
	if suffix == "/delete" {
		action = "auth.passkey.delete"
	}
	if suffix == "/register/begin" {
		action = "auth.passkey.register.begin"
	}
	// Fail closed before any authentication mutation if audit storage is down.
	if err := s.audit(r, session.User.ID, action, "user", session.User.ID, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	var err error
	var options auth.PasskeyOptions
	switch suffix {
	case "/register/begin":
		options, err = s.passkeys.BeginRegistration(session, input.Password, input.TOTPCode, input.Name)
	case "/register/finish":
		err = s.passkeys.FinishRegistration(session, input.CeremonyID, input.Credential)
	case "/delete":
		err = s.passkeys.Delete(session, input.ID, input.Password, input.TOTPCode)
	}
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "failure", nil)
		s.writePasskeyProblem(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "success", nil)
	if suffix == "/register/begin" {
		s.writeJSON(w, http.StatusOK, options)
		return
	}
	s.clearAuthCookies(w, r)
	s.writeJSON(w, http.StatusOK, map[string]bool{"reauthenticate": true})
}

func (s *Server) handlePasskeyLogin(w http.ResponseWriter, r *http.Request, suffix string) {
	var input struct {
		Username   string          `json:"username"`
		CeremonyID string          `json:"ceremonyId"`
		Credential json.RawMessage `json:"credential"`
		TOTPCode   string          `json:"totpCode,omitempty"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if suffix == "/login/begin" {
		binding := rand.Text()
		options, err := s.passkeys.BeginLogin(s.remoteIP(r), input.Username, passkeyBinding(binding))
		if err != nil {
			s.auditAuthFailure(r, "auth.passkey.login")
			s.writePasskeyProblem(w, r, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Value: binding, Path: "/", MaxAge: 180, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		s.writeJSON(w, http.StatusOK, options)
		return
	}
	cookie, err := r.Cookie(passkeyCookie)
	if err != nil || len(cookie.Value) > 128 {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyInvalid)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: passkeyCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	// Anonymous failures use the shared audit throttle; arbitrary ceremony IDs
	// must not trigger an unbounded durable intent write before verification.
	credentials, err := s.passkeys.FinishLogin(s.remoteIP(r), passkeyBinding(cookie.Value), input.CeremonyID, input.Credential, input.TOTPCode)
	if err != nil {
		s.auditAuthFailure(r, "auth.passkey.login")
		s.writePasskeyProblem(w, r, err)
		return
	}
	if err := s.audit(r, credentials.User.ID, "auth.passkey.login", "session", "", "success", nil); err != nil {
		_ = s.auth.Logout(credentials.Token)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	s.setAuthCookies(w, r, credentials)
	s.writeJSON(w, http.StatusOK, authResponse{User: credentials.User, CSRFToken: credentials.CSRFToken, ExpiresAt: credentials.ExpiresAt})
}

func passkeyBinding(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (s *Server) writePasskeyProblem(w http.ResponseWriter, r *http.Request, err error) {
	var rate *auth.RateLimitError
	switch {
	case errors.As(err, &rate):
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(rate.RetryAfter.Seconds()))))
		s.writeProblem(w, r, http.StatusTooManyRequests, "authentication_rate_limited", "Authentication rate limited", "")
	case errors.Is(err, auth.ErrPasskeyUnavailable):
		s.writeProblem(w, r, http.StatusConflict, "passkey_unavailable", "Passkeys require the configured HTTPS domain", "")
	case errors.Is(err, auth.ErrTOTPRequired):
		s.writeProblem(w, r, http.StatusUnauthorized, "totp_required", "Two-factor authentication required", "")
	case errors.Is(err, auth.ErrInvalidSecondFactor):
		s.writeProblem(w, r, http.StatusUnauthorized, "invalid_second_factor", "Invalid two-factor authentication code", "")
	case errors.Is(err, auth.ErrSecondFactorUnavailable):
		s.writeProblem(w, r, http.StatusServiceUnavailable, "second_factor_unavailable", "Two-factor authentication is temporarily unavailable", "")
	case errors.Is(err, auth.ErrPasskeyName):
		s.writeValidationProblem(w, r, "name", err.Error())
	case errors.Is(err, store.ErrLimitReached):
		s.writeProblem(w, r, http.StatusConflict, "passkey_limit", "A maximum of 10 passkeys is allowed", "")
	case errors.Is(err, auth.ErrInvalidCurrentPassword), errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrPasskeyInvalid), errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrAlreadyExists):
		s.writeProblem(w, r, http.StatusUnauthorized, "invalid_credentials", "Authentication failed; start again", "")
	default:
		s.writeProblem(w, r, http.StatusInternalServerError, "passkey_failed", "Unable to complete passkey operation", "")
	}
}
