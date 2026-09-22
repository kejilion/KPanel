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

type passkeyStatusResponse struct {
	Available      bool   `json:"available"`
	Origin         string `json:"origin,omitempty"`
	DetectedOrigin string `json:"detectedOrigin,omitempty"`
	Configurable   bool   `json:"configurable"`
	OriginManaged  bool   `json:"originManaged"`
}

type passkeyListResponse struct {
	Available      bool               `json:"available"`
	RPID           string             `json:"rpId"`
	Origin         string             `json:"origin,omitempty"`
	DetectedOrigin string             `json:"detectedOrigin,omitempty"`
	Configurable   bool               `json:"configurable"`
	OriginManaged  bool               `json:"originManaged"`
	Credentials    []auth.PasskeyInfo `json:"credentials"`
}

func (s *Server) detectedPasskeyOrigin(r *http.Request) string {
	detected, ok := s.requestHTTPSOrigin(r)
	if !ok {
		return ""
	}
	origin, _, err := auth.PasskeyOrigin(detected)
	if err != nil {
		return ""
	}
	return origin
}

func (s *Server) passkeysAvailable(r *http.Request, passkeys *auth.PasskeyService) bool {
	if passkeys == nil || passkeys.Origin == "" {
		return false
	}
	origin := s.detectedPasskeyOrigin(r)
	return origin != "" && secureStringEqual(strings.ToLower(origin), passkeys.Origin)
}

func (s *Server) handlePasskeys(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, passkeyPrefix)
	if suffix == "/origin" {
		s.handlePasskeyOrigin(w, r)
		return
	}
	if suffix == "/origin/disable" {
		s.handlePasskeyOriginDisable(w, r)
		return
	}
	// Keep the service pointer stable for the complete operation. Origin
	// changes take the write side of this lock, so an in-flight ceremony cannot
	// finish against the old RP after the new service becomes active.
	s.passkeyMu.RLock()
	defer s.passkeyMu.RUnlock()
	passkeys := s.passkeys
	available := s.passkeysAvailable(r, passkeys)
	if r.Method == http.MethodGet && suffix == "/status" {
		origin := ""
		if passkeys != nil {
			origin = passkeys.Origin
		}
		s.writeJSON(w, http.StatusOK, passkeyStatusResponse{
			Available: available, Origin: origin, DetectedOrigin: s.detectedPasskeyOrigin(r), Configurable: !s.passkeyOriginLocked && !s.store.HasPasskeys(), OriginManaged: s.passkeyOriginLocked,
		})
		return
	}
	if r.Method == http.MethodGet && suffix == "" {
		_, session, ok := s.requireSession(w, r)
		if !ok {
			return
		}
		if passkeys == nil {
			s.writeJSON(w, http.StatusOK, passkeyListResponse{Credentials: []auth.PasskeyInfo{}, DetectedOrigin: s.detectedPasskeyOrigin(r), OriginManaged: s.passkeyOriginLocked})
			return
		}
		entries, err := passkeys.List(session.User.ID)
		if err != nil {
			s.writePasskeyProblem(w, r, err)
			return
		}
		origin := passkeys.Origin
		s.writeJSON(w, http.StatusOK, passkeyListResponse{
			Available: available, RPID: passkeys.RPID, Origin: origin,
			DetectedOrigin: s.detectedPasskeyOrigin(r), Configurable: !s.passkeyOriginLocked && !s.store.HasPasskeys(), OriginManaged: s.passkeyOriginLocked, Credentials: entries,
		})
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
	if suffix != "/delete" && (!available || passkeys == nil || !secureStringEqual(strings.ToLower(r.Header.Get("Origin")), passkeys.Origin)) {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	if passkeys == nil {
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
		s.handlePasskeyLogin(w, r, suffix, passkeys)
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
		options, err = passkeys.BeginRegistration(session, input.Password, input.TOTPCode, input.Name)
	case "/register/finish":
		err = passkeys.FinishRegistration(session, input.CeremonyID, input.Credential)
	case "/delete":
		err = passkeys.Delete(session, input.ID, input.Password, input.TOTPCode)
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

// handlePasskeyOrigin binds the panel to the current browser-facing HTTPS
// origin after the administrator confirms current management factors. The
// origin is derived only from direct TLS or an explicitly trusted proxy; the
// client cannot submit an arbitrary Host value.
func (s *Server) handlePasskeyOrigin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.Header.Get("Origin") == "" || !s.checkOrigin(w, r) {
		if r.Header.Get("Origin") == "" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
		}
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Password string `json:"password"`
		TOTPCode string `json:"totpCode,omitempty"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	detected := s.detectedPasskeyOrigin(r)
	if detected == "" {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	s.passkeyMu.Lock()
	defer s.passkeyMu.Unlock()
	passkeys := s.passkeys
	if passkeys == nil {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	currentOrigin := passkeys.Origin
	locked := s.passkeyOriginLocked
	if secureStringEqual(strings.ToLower(currentOrigin), strings.ToLower(detected)) {
		s.writeJSON(w, http.StatusOK, passkeyStatusResponse{Available: true, Origin: currentOrigin, DetectedOrigin: detected, OriginManaged: locked})
		return
	}
	if locked {
		s.writeProblem(w, r, http.StatusConflict, "passkey_origin_managed", "Passkey origin is managed by server configuration", "")
		return
	}
	if s.store.HasPasskeys() {
		s.writeProblem(w, r, http.StatusConflict, "passkey_origin_rebind_required", "Revoke existing passkeys before changing the HTTPS domain", "")
		return
	}
	change := map[string]any{"origin": detected, "source": "current_https_origin"}
	if err := s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := passkeys.VerifyManagementFactors(session.User.ID, input.Password, input.TOTPCode); err != nil {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "failure", change)
		s.writePasskeyProblem(w, r, err)
		return
	}
	candidate, err := auth.NewPasskeyService(s.auth, detected)
	if err != nil {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "failure", change)
		s.writePasskeyProblem(w, r, err)
		return
	}
	storedOrigin, version := s.store.PasskeyOrigin()
	if storedOrigin != currentOrigin {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "failure", change)
		s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Passkey origin changed; refresh and retry", "")
		return
	}
	if err := s.store.ReplacePasskeyOrigin(version, detected); err != nil {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "failure", change)
		if errors.Is(err, store.ErrConflict) {
			s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Passkey origin changed; refresh and retry", "")
			return
		}
		s.writeProblem(w, r, http.StatusInternalServerError, "passkey_origin_update_failed", "Passkey origin update failed", "")
		return
	}
	s.passkeys = candidate
	_ = s.audit(r, session.User.ID, "settings.passkey_origin.update", "panel", "passkey-origin", "success", change)
	s.writeJSON(w, http.StatusOK, passkeyStatusResponse{Available: true, Origin: detected, DetectedOrigin: detected, OriginManaged: false})
}

// handlePasskeyOriginDisable clears the locally managed Passkey origin after
// the administrator confirms the current management factors. Server-managed
// origins remain immutable and must be changed in the server configuration.
func (s *Server) handlePasskeyOriginDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.Header.Get("Origin") == "" || !s.checkOrigin(w, r) {
		if r.Header.Get("Origin") == "" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
		}
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Password string `json:"password"`
		TOTPCode string `json:"totpCode,omitempty"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}

	s.passkeyMu.Lock()
	defer s.passkeyMu.Unlock()
	passkeys := s.passkeys
	if passkeys == nil || passkeys.Origin == "" {
		s.writePasskeyProblem(w, r, auth.ErrPasskeyUnavailable)
		return
	}
	if s.passkeyOriginLocked {
		s.writeProblem(w, r, http.StatusConflict, "passkey_origin_managed", "Passkey origin is managed by server configuration", "")
		return
	}
	currentOrigin, version := s.store.PasskeyOrigin()
	if currentOrigin == "" || !secureStringEqual(strings.ToLower(currentOrigin), strings.ToLower(passkeys.Origin)) {
		s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Passkey origin changed; refresh and retry", "")
		return
	}
	change := map[string]any{"origin": currentOrigin, "source": "settings"}
	if err := s.audit(r, session.User.ID, "settings.passkey_origin.disable", "panel", "passkey-origin", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	candidate, err := auth.NewPasskeyService(s.auth, "")
	if err != nil {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.disable", "panel", "passkey-origin", "failure", change)
		s.writePasskeyProblem(w, r, err)
		return
	}
	if err := passkeys.Disable(session, input.Password, input.TOTPCode, version); err != nil {
		_ = s.audit(r, session.User.ID, "settings.passkey_origin.disable", "panel", "passkey-origin", "failure", change)
		if errors.Is(err, store.ErrConflict) {
			s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Passkey origin changed; refresh and retry", "")
			return
		}
		s.writePasskeyProblem(w, r, err)
		return
	}
	s.passkeys = candidate
	_ = s.audit(r, session.User.ID, "settings.passkey_origin.disable", "panel", "passkey-origin", "success", change)
	s.clearAuthCookies(w, r)
	s.writeJSON(w, http.StatusOK, map[string]bool{"reauthenticate": true})
}

func (s *Server) handlePasskeyLogin(w http.ResponseWriter, r *http.Request, suffix string, passkeys *auth.PasskeyService) {
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
		options, err := passkeys.BeginLogin(s.remoteIP(r), input.Username, passkeyBinding(binding))
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
	credentials, err := passkeys.FinishLogin(s.remoteIP(r), passkeyBinding(cookie.Value), input.CeremonyID, input.Credential, input.TOTPCode)
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
