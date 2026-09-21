package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/kejilion/kejilion-panel/internal/store"
	"golang.org/x/net/publicsuffix"
)

const passkeyTTL = 3 * time.Minute
const maxPasskeyCeremonies = 64

var ErrPasskeyInvalid = errors.New("passkey verification failed; start again")
var ErrPasskeyUnavailable = errors.New("passkeys require a configured HTTPS domain")
var ErrPasskeyName = errors.New("passkey name must contain 1-64 characters without control characters")

// PasskeyOrigin accepts a single fixed HTTPS origin, never a request Host or a
// parent-domain wildcard. No development exceptions enter production policy.
func PasskeyOrigin(value string) (origin, rpID string, err error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return "", "", ErrPasskeyUnavailable
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") || protocol.ValidateRPID(host) != nil {
		return "", "", ErrPasskeyUnavailable
	}
	if suffix, _ := publicsuffix.PublicSuffix(host); suffix == host {
		return "", "", ErrPasskeyUnavailable
	}
	if len(host) > 253 {
		return "", "", ErrPasskeyUnavailable
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", "", ErrPasskeyUnavailable
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", "", ErrPasskeyUnavailable
			}
		}
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", "", ErrPasskeyUnavailable
		}
	}
	return "https://" + strings.ToLower(u.Host), host, nil
}

type PasskeyService struct {
	auth    *Service
	web     *webauthn.WebAuthn
	Origin  string
	RPID    string
	mu      sync.Mutex
	pending map[string]passkeyCeremony
}

type passkeyCeremony struct {
	data                                     webauthn.SessionData
	userID, username, purpose, binding, name string
	version                                  uint64
	expires                                  time.Time
}

type PasskeyOptions struct {
	CeremonyID string `json:"ceremonyId"`
	PublicKey  any    `json:"publicKey"`
}

type PasskeyInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type passkeyUser struct {
	store.User
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u passkeyUser) WebAuthnName() string                       { return u.Username }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.Username }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func NewPasskeyService(service *Service, value string) (*PasskeyService, error) {
	if value == "" {
		return &PasskeyService{auth: service, pending: make(map[string]passkeyCeremony)}, nil
	}
	origin, id, err := PasskeyOrigin(value)
	if err != nil {
		return nil, err
	}
	web, err := webauthn.New(&webauthn.Config{
		RPID: id, RPDisplayName: "KPanel", RPOrigins: []string{origin},
		AttestationPreference:  protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyTTL},
		},
	})
	if err != nil {
		return nil, err
	}
	return &PasskeyService{auth: service, web: web, Origin: origin, RPID: id, pending: make(map[string]passkeyCeremony)}, nil
}

func (p *PasskeyService) user(user store.User) (passkeyUser, error) {
	result := passkeyUser{User: user}
	for _, entry := range user.Passkeys {
		if entry.RPID != p.RPID {
			continue
		}
		var c webauthn.Credential
		if json.Unmarshal(entry.Credential, &c) != nil || base64.RawURLEncoding.EncodeToString(c.ID) != entry.ID || !c.Flags.UserVerified {
			return result, ErrPasskeyInvalid
		}
		result.credentials = append(result.credentials, c)
	}
	return result, nil
}

func (p *PasskeyService) List(userID string) ([]PasskeyInfo, error) {
	user, err := p.auth.store.UserByID(userID)
	if err != nil {
		return nil, err
	}
	list := make([]PasskeyInfo, 0, len(user.Passkeys))
	for _, c := range user.Passkeys {
		list = append(list, PasskeyInfo{c.ID, c.Name, c.CreatedAt, c.LastUsedAt})
	}
	return list, nil
}

func (p *PasskeyService) save(c passkeyCeremony, options any) (PasskeyOptions, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.auth.now()
	for id, old := range p.pending {
		if !old.expires.After(now) || (old.binding == c.binding && old.purpose == c.purpose) {
			delete(p.pending, id)
		}
	}
	if len(p.pending) >= maxPasskeyCeremonies {
		return PasskeyOptions{}, &RateLimitError{RetryAfter: passkeyTTL}
	}
	id, err := randomToken(32)
	if err != nil {
		return PasskeyOptions{}, err
	}
	c.expires = now.Add(passkeyTTL)
	p.pending[id] = c
	return PasskeyOptions{id, options}, nil
}

func (p *PasskeyService) consume(id, purpose, binding string) (passkeyCeremony, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.pending[id]
	if !ok || len(binding) == 0 || !secureEqual(c.binding, binding) || c.purpose != purpose {
		return passkeyCeremony{}, ErrPasskeyInvalid
	}
	delete(p.pending, id) // Failed assertions are also single use.
	if !c.expires.After(p.auth.now()) {
		return passkeyCeremony{}, ErrPasskeyInvalid
	}
	return c, nil
}

// Management factors share a separate, persistent budget including failed
// second factors: a correct password must not reset the TOTP guessing budget.
func (p *PasskeyService) reauthenticate(userID, password, second string) (store.User, error) {
	s := p.auth
	now := s.now()
	key := "passkey-reauth:" + userID
	if !s.reserveAuthenticationAttempt(key, now.Add(-s.config.LoginWindow), s.config.MaxLoginFailures) {
		return store.User{}, &RateLimitError{RetryAfter: s.config.LoginWindow}
	}
	defer s.releaseAuthenticationAttempt(key)
	user, err := s.verifyCurrentPassword(userID, password)
	if err == nil && user.TOTPSecret != "" {
		err = s.verifyAndConsumeSecondFactor(user, second, now)
		if err != nil && !errors.Is(err, ErrSecondFactorUnavailable) {
			err = ErrInvalidSecondFactor
		}
	}
	if recordErr := s.store.RecordLoginAttempt(store.LoginAttempt{Key: key, OccurredAt: now, Success: err == nil}, now.Add(-s.config.LoginWindow)); recordErr != nil {
		return store.User{}, recordErr
	}
	if err != nil {
		return store.User{}, err
	}
	return s.store.UserByID(userID)
}

func (p *PasskeyService) BeginRegistration(session Session, password, second, name string) (PasskeyOptions, error) {
	if p.web == nil {
		return PasskeyOptions{}, ErrPasskeyUnavailable
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return PasskeyOptions{}, ErrPasskeyName
	}
	p.auth.credentialMu.Lock()
	defer p.auth.credentialMu.Unlock()
	user, err := p.reauthenticate(session.User.ID, password, second)
	if err != nil {
		return PasskeyOptions{}, err
	}
	if len(user.Passkeys) >= 10 {
		return PasskeyOptions{}, store.ErrLimitReached
	}
	u, err := p.user(user)
	if err != nil {
		return PasskeyOptions{}, err
	}
	var exclude []protocol.CredentialDescriptor
	for _, c := range u.credentials {
		exclude = append(exclude, c.Descriptor())
	}
	creation, data, err := p.web.BeginRegistration(u, webauthn.WithExclusions(exclude))
	if err != nil {
		return PasskeyOptions{}, err
	}
	return p.save(passkeyCeremony{data: *data, userID: user.ID, purpose: "register", binding: session.TokenHash, name: name, version: user.CredentialVersion}, creation.Response)
}

func (p *PasskeyService) FinishRegistration(session Session, id string, response []byte) error {
	c, err := p.consume(id, "register", session.TokenHash)
	if err != nil {
		return err
	}
	p.auth.credentialMu.Lock()
	defer p.auth.credentialMu.Unlock()
	user, err := p.auth.store.UserByID(session.User.ID)
	if err != nil || user.ID != c.userID || user.CredentialVersion != c.version {
		return ErrPasskeyInvalid
	}
	u, err := p.user(user)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return ErrPasskeyInvalid
	}
	cred, err := p.web.CreateCredential(u, c.data, parsed)
	if err != nil || !cred.Flags.UserVerified {
		return ErrPasskeyInvalid
	}
	encoded, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	entries := append(user.Passkeys, store.Passkey{ID: base64.RawURLEncoding.EncodeToString(cred.ID), RPID: p.RPID, Name: c.name, Credential: encoded, CreatedAt: p.auth.now()})
	return p.auth.store.ReplaceUserPasskeys(user, entries, session.TokenHash, p.auth.now())
}

func (p *PasskeyService) Delete(session Session, id, password, second string) error {
	p.auth.credentialMu.Lock()
	defer p.auth.credentialMu.Unlock()
	user, err := p.reauthenticate(session.User.ID, password, second)
	if err != nil {
		return err
	}
	entries := make([]store.Passkey, 0, len(user.Passkeys))
	found := false
	for _, c := range user.Passkeys {
		if c.ID == id {
			found = true
		} else {
			entries = append(entries, c)
		}
	}
	if !found {
		return store.ErrNotFound
	}
	return p.auth.store.ReplaceUserPasskeys(user, entries, session.TokenHash, p.auth.now())
}

func (p *PasskeyService) BeginLogin(ip, username, binding string) (PasskeyOptions, error) {
	if p.web == nil {
		return PasskeyOptions{}, ErrPasskeyUnavailable
	}
	if len(username) == 0 || len(username) > 64 || binding == "" {
		return PasskeyOptions{}, ErrInvalidCredentials
	}
	s := p.auth
	now := s.now()
	key := "passkey-begin:" + ip
	if !s.reserveAuthenticationAttempt(key, now.Add(-s.config.LoginWindow), s.config.MaxLoginFailures*2) {
		return PasskeyOptions{}, &RateLimitError{RetryAfter: s.config.LoginWindow}
	}
	defer s.releaseAuthenticationAttempt(key)
	// Count every challenge allocation, even for existing users. This caps work
	// and does not expose whether the username or credentials exist.
	if err := s.store.RecordLoginAttempt(store.LoginAttempt{Key: key, OccurredAt: now}, now.Add(-s.config.LoginWindow)); err != nil {
		return PasskeyOptions{}, err
	}
	dataOptions, data, err := p.web.BeginDiscoverableLogin()
	if err != nil {
		return PasskeyOptions{}, err
	}
	user, _ := s.store.UserByUsername(username)
	return p.save(passkeyCeremony{data: *data, userID: user.ID, username: username, version: user.CredentialVersion, purpose: "login", binding: binding}, dataOptions.Response)
}

func (p *PasskeyService) FinishLogin(ip, binding, id string, response []byte, second string) (Credentials, error) {
	c, err := p.consume(id, "login", binding)
	if err != nil {
		return Credentials{}, err
	}
	s := p.auth
	now := s.now()
	ipKey, accountKey := loginKeys(ip, c.username)
	if !s.reserveLogin(ipKey, accountKey, now.Add(-s.config.LoginWindow)) {
		return Credentials{}, &RateLimitError{RetryAfter: s.config.LoginWindow}
	}
	defer s.releaseLogin(ipKey, accountKey)
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	failure := func(err error) (Credentials, error) {
		if e := s.recordLoginAttempt(ipKey, accountKey, now, false); e != nil {
			return Credentials{}, e
		}
		return Credentials{}, err
	}
	user, err := s.store.UserByID(c.userID)
	if err != nil || user.CredentialVersion != c.version {
		return failure(ErrInvalidCredentials)
	}
	u, err := p.user(user)
	if err != nil {
		return failure(ErrInvalidCredentials)
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return failure(ErrInvalidCredentials)
	}
	_, cred, err := p.web.ValidatePasskeyLogin(func(rawID, handle []byte) (webauthn.User, error) {
		if !bytes.Equal(handle, u.WebAuthnID()) {
			return nil, ErrInvalidCredentials
		}
		return u, nil
	}, c.data, parsed)
	if err != nil || !cred.Flags.UserVerified {
		return failure(ErrInvalidCredentials)
	}
	// Zero counters and syncable credentials are valid. A non-backup credential
	// with a counter regression is refused; synced counters are not proof of cloning.
	if cred.Authenticator.CloneWarning && !cred.Flags.BackupEligible {
		return failure(ErrInvalidCredentials)
	}
	if user.TOTPSecret != "" {
		if strings.TrimSpace(second) == "" {
			return failure(ErrTOTPRequired)
		}
		if err := s.verifyAndConsumeSecondFactor(user, second, now); err != nil {
			if errors.Is(err, ErrSecondFactorUnavailable) {
				return failure(err)
			}
			return failure(ErrInvalidSecondFactor)
		}
	}
	encoded, err := json.Marshal(cred)
	if err != nil {
		return Credentials{}, err
	}
	credentialID := base64.RawURLEncoding.EncodeToString(cred.ID)
	var entry store.Passkey
	for _, v := range user.Passkeys {
		if v.ID == credentialID && v.RPID == p.RPID {
			entry = v
			break
		}
	}
	if entry.ID == "" {
		return failure(ErrInvalidCredentials)
	}
	entry.Credential = encoded
	entry.LastUsedAt = &now
	credentials, record, err := s.newSession(user)
	if err != nil {
		return Credentials{}, err
	}
	if err = s.store.UpdateUserPasskey(user, entry, record); err != nil {
		return failure(ErrInvalidCredentials)
	}
	if err = s.recordLoginAttempt(ipKey, accountKey, now, true); err != nil {
		_ = s.store.DeleteSession(record.TokenHash)
		return Credentials{}, err
	}
	return credentials, nil
}
