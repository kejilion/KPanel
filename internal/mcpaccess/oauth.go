package mcpaccess

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrOAuth = errors.New("invalid_grant")

type OAuthRegistration struct {
	ID           string    `json:"client_id"`
	Name         string    `json:"client_name"`
	RedirectURIs []string  `json:"redirect_uris"`
	Method       string    `json:"token_endpoint_auth_method"`
	SecretDigest string    `json:"secretDigest,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}
type OAuthAuthorization struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"clientId"`
	ClientName  string    `json:"clientName"`
	RedirectURI string    `json:"redirectUri"`
	State       string    `json:"state"`
	Challenge   string    `json:"challenge"`
	Resource    string    `json:"resource"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type oauthCode struct {
	Digest, ClientID, RedirectURI, Challenge, Resource, GrantID, FamilyID string
	ExpiresAt                                                             time.Time
	Used                                                                  bool
}
type oauthFamily struct {
	ID, ClientID, GrantID, Resource, AccessDigest, RefreshDigest string
	UsedRefresh                                                  []string
	AccessExpiresAt, ExpiresAt                                   time.Time
	Revoked                                                      bool
}
type oauthState struct {
	Version        int
	Clients        []OAuthRegistration
	Authorizations []OAuthAuthorization
	Codes          []oauthCode
	Families       []oauthFamily
}
type OAuth struct {
	mu        sync.Mutex
	path      string
	state     oauthState
	available bool
	now       func() time.Time
	write     func(string, any) error
}
type OAuthTokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func OpenOAuth(dataDir string) *OAuth {
	o := &OAuth{path: filepath.Join(dataDir, "mcp-access", "oauth.json"), state: oauthState{Version: 1}, available: true, now: time.Now, write: writePrivateJSON}
	data, err := readPrivateJSON(o.path)
	if errors.Is(err, os.ErrNotExist) {
		return o
	}
	if err != nil || json.Unmarshal(data, &o.state) != nil || o.state.Version != 1 || len(o.state.Clients) > 128 || len(o.state.Authorizations) > 64 || len(o.state.Codes) > 64 || len(o.state.Families) > MaxClients {
		o.available = false
		return o
	}
	for _, c := range o.state.Clients {
		if !validHex(c.ID, 32) || !validName(c.Name) || len(c.RedirectURIs) == 0 || len(c.RedirectURIs) > 8 || !slices.Contains([]string{"none", "client_secret_post", "client_secret_basic"}, c.Method) {
			o.available = false
		}
		for _, uri := range c.RedirectURIs {
			if !ValidOAuthRedirect(uri) {
				o.available = false
			}
		}
	}
	for _, f := range o.state.Families {
		if !validHex(f.ID, 32) || !validHex(f.GrantID, 32) || !validHex(f.AccessDigest, 64) || !validHex(f.RefreshDigest, 64) || len(f.UsedRefresh) > 4096 {
			o.available = false
		}
	}
	return o
}

func oauthRandom(prefix string) string {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(data[:])
}
func oauthID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}
func oauthDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func oauthEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func oauthOpaque(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+43 {
		return false
	}
	b, e := base64.RawURLEncoding.DecodeString(value[len(prefix):])
	return e == nil && len(b) == 32
}

func ValidOAuthRedirect(raw string) bool {
	if len(raw) == 0 || len(raw) > 2048 || strings.ContainsAny(raw, "\r\n\\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || strings.Contains(u.Hostname(), "*") || u.User != nil || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" {
		return false
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return false
		}
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	for _, key := range []string{"code", "state", "error", "iss"} {
		if q.Has(key) {
			return false
		}
	}
	return true
}

func (o *OAuth) next() oauthState {
	data, _ := json.Marshal(o.state)
	var state oauthState
	_ = json.Unmarshal(data, &state)
	now := o.now()
	state.Authorizations = slices.DeleteFunc(state.Authorizations, func(a OAuthAuthorization) bool { return !now.Before(a.ExpiresAt) })
	state.Codes = slices.DeleteFunc(state.Codes, func(c oauthCode) bool { return !now.Before(c.ExpiresAt) })
	state.Families = slices.DeleteFunc(state.Families, func(f oauthFamily) bool { return f.Revoked || !now.Before(f.ExpiresAt) })
	state.Clients = slices.DeleteFunc(state.Clients, func(c OAuthRegistration) bool {
		if now.Sub(c.CreatedAt) < 24*time.Hour {
			return false
		}
		for _, f := range state.Families {
			if f.ClientID == c.ID {
				return false
			}
		}
		for _, a := range state.Authorizations {
			if a.ClientID == c.ID {
				return false
			}
		}
		for _, code := range state.Codes {
			if code.ClientID == c.ID {
				return false
			}
		}
		return true
	})
	return state
}
func (o *OAuth) save(state oauthState) error {
	if err := o.write(o.path, state); err != nil {
		o.available = false
		return ErrUnavailable
	}
	o.state = state
	return nil
}

func (o *OAuth) Register(name string, redirects []string, method string) (OAuthRegistration, string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return OAuthRegistration{}, "", ErrUnavailable
	}
	if name == "" {
		name = "MCP client"
	}
	if method == "" {
		method = "client_secret_basic"
	}
	if !validName(name) || len(redirects) == 0 || len(redirects) > 8 || !slices.Contains([]string{"none", "client_secret_post", "client_secret_basic"}, method) {
		return OAuthRegistration{}, "", ErrInvalid
	}
	for _, uri := range redirects {
		if !ValidOAuthRedirect(uri) {
			return OAuthRegistration{}, "", ErrInvalid
		}
	}
	state := o.next()
	if len(state.Clients) >= 128 {
		return OAuthRegistration{}, "", ErrLimited
	}
	c := OAuthRegistration{ID: oauthID(), Name: name, RedirectURIs: slices.Clone(redirects), Method: method, CreatedAt: o.now().UTC()}
	secret := ""
	if method != "none" {
		secret = oauthRandom("kps_")
		c.SecretDigest = oauthDigest(secret)
	}
	state.Clients = append(state.Clients, c)
	if err := o.save(state); err != nil {
		return OAuthRegistration{}, "", err
	}
	c.SecretDigest = ""
	return c, secret, nil
}

func (o *OAuth) client(id string) (OAuthRegistration, bool) {
	for _, c := range o.state.Clients {
		if c.ID == id {
			return c, true
		}
	}
	return OAuthRegistration{}, false
}
func (o *OAuth) clientAuth(id, secret, method string) bool {
	c, ok := o.client(id)
	if !ok || c.Method != method {
		return false
	}
	if method == "none" {
		return secret == ""
	}
	return secret != "" && oauthEqual(c.SecretDigest, oauthDigest(secret))
}

func (o *OAuth) Authorize(clientID, redirect, challenge, resource, stateValue string) (OAuthAuthorization, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return OAuthAuthorization{}, ErrUnavailable
	}
	c, ok := o.client(clientID)
	if !ok || !slices.Contains(c.RedirectURIs, redirect) || !oauthOpaque(challenge, "") || len(stateValue) > 1024 || strings.ContainsAny(stateValue, "\r\n") || resource == "" || len(resource) > 2048 {
		return OAuthAuthorization{}, ErrInvalid
	}
	state := o.next()
	if len(state.Authorizations) >= 64 {
		return OAuthAuthorization{}, ErrLimited
	}
	// An idle registration may have been pruned by next(). Starting an actual
	// authorization renews it before we persist the pending request.
	if !slices.ContainsFunc(state.Clients, func(item OAuthRegistration) bool { return item.ID == c.ID }) {
		c.CreatedAt = o.now().UTC()
		state.Clients = append(state.Clients, c)
	}
	a := OAuthAuthorization{ID: oauthRandom(""), ClientID: c.ID, ClientName: c.Name, RedirectURI: redirect, Challenge: challenge, Resource: resource, State: stateValue, ExpiresAt: o.now().Add(10 * time.Minute).UTC()}
	state.Authorizations = append(state.Authorizations, a)
	if err := o.save(state); err != nil {
		return OAuthAuthorization{}, err
	}
	return a, nil
}
func (o *OAuth) Authorization(id string) (OAuthAuthorization, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return OAuthAuthorization{}, ErrUnavailable
	}
	for _, a := range o.state.Authorizations {
		if oauthEqual(a.ID, id) && o.now().Before(a.ExpiresAt) {
			return a, nil
		}
	}
	return OAuthAuthorization{}, ErrOAuth
}

// Consent is called only after Panel Session/Origin/CSRF checks. create creates
// a normal immutable host/policy grant without disclosing its unused PAT.
func (o *OAuth) Consent(id string, approve bool, create func(OAuthAuthorization) (Client, error)) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return "", ErrUnavailable
	}
	state := o.next()
	index := slices.IndexFunc(state.Authorizations, func(a OAuthAuthorization) bool { return oauthEqual(a.ID, id) })
	if index < 0 {
		return "", ErrOAuth
	}
	a := state.Authorizations[index]
	u, _ := url.Parse(a.RedirectURI)
	q := u.Query()
	if a.State != "" {
		q.Set("state", a.State)
	}
	if approve {
		if len(state.Codes) >= 64 {
			return "", ErrLimited
		}
		grant, err := create(a)
		if err != nil {
			return "", err
		}
		code := oauthRandom("kpc_")
		state.Codes = append(state.Codes, oauthCode{Digest: oauthDigest(code), ClientID: a.ClientID, RedirectURI: a.RedirectURI, Challenge: a.Challenge, Resource: a.Resource, GrantID: grant.ID, ExpiresAt: o.now().Add(5 * time.Minute).UTC()})
		q.Set("code", code)
	} else {
		q.Set("error", "access_denied")
	}
	state.Authorizations = slices.Delete(state.Authorizations, index, index+1)
	if err := o.save(state); err != nil {
		return "", err
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (o *OAuth) Exchange(base *Store, clientID, secret, method, code, redirect, verifier, resource string) (OAuthTokens, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return OAuthTokens{}, ErrUnavailable
	}
	if !o.clientAuth(clientID, secret, method) || !oauthOpaque(code, "kpc_") || len(verifier) < 43 || len(verifier) > 128 {
		return OAuthTokens{}, ErrOAuth
	}
	for _, ch := range verifier {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~", ch)) {
			return OAuthTokens{}, ErrOAuth
		}
	}
	state := o.next()
	idx := slices.IndexFunc(state.Codes, func(c oauthCode) bool { return oauthEqual(c.Digest, oauthDigest(code)) })
	if idx < 0 {
		return OAuthTokens{}, ErrOAuth
	}
	c := state.Codes[idx]
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if c.ClientID != clientID || c.RedirectURI != redirect || c.Resource != resource || !oauthEqual(c.Challenge, challenge) {
		return OAuthTokens{}, ErrOAuth
	}
	if c.Used {
		_ = base.revokeOAuthGrant(c.GrantID)
		return OAuthTokens{}, ErrOAuth
	}
	grant, err := base.LookupClient(c.GrantID)
	if err != nil {
		return OAuthTokens{}, ErrOAuth
	}
	state.Families = slices.DeleteFunc(state.Families, func(f oauthFamily) bool { _, err := base.LookupClient(f.GrantID); return err != nil })
	if len(state.Families) >= MaxClients {
		return OAuthTokens{}, ErrLimited
	}
	f := oauthFamily{ID: oauthID(), ClientID: clientID, GrantID: grant.ID, Resource: resource, ExpiresAt: grant.ExpiresAt}
	tokens := o.issue(&f)
	state.Codes[idx].Used = true
	state.Codes[idx].FamilyID = f.ID
	state.Families = append(state.Families, f)
	if err := o.save(state); err != nil {
		return OAuthTokens{}, err
	}
	return tokens, nil
}
func (o *OAuth) issue(f *oauthFamily) OAuthTokens {
	access, refresh := oauthRandom("kpo_"), oauthRandom("kpr_")
	f.AccessDigest, f.RefreshDigest = oauthDigest(access), oauthDigest(refresh)
	f.AccessExpiresAt = o.now().Add(time.Hour).UTC()
	if f.AccessExpiresAt.After(f.ExpiresAt) {
		f.AccessExpiresAt = f.ExpiresAt
	}
	return OAuthTokens{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int64(f.AccessExpiresAt.Sub(o.now()).Seconds()), Scope: "mcp"}
}

func (o *OAuth) Refresh(base *Store, clientID, secret, method, token, resource string) (OAuthTokens, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return OAuthTokens{}, ErrUnavailable
	}
	if !o.clientAuth(clientID, secret, method) || !oauthOpaque(token, "kpr_") {
		return OAuthTokens{}, ErrOAuth
	}
	state := o.next()
	digest := oauthDigest(token)
	for i := range state.Families {
		f := &state.Families[i]
		if f.ClientID != clientID || f.Resource != resource {
			continue
		}
		if slices.Contains(f.UsedRefresh, digest) {
			_ = base.revokeOAuthGrant(f.GrantID)
			f.Revoked = true
			if err := o.save(state); err != nil {
				return OAuthTokens{}, err
			}
			return OAuthTokens{}, ErrOAuth
		}
		if !oauthEqual(f.RefreshDigest, digest) || f.Revoked {
			continue
		}
		if _, err := base.LookupClient(f.GrantID); err != nil {
			return OAuthTokens{}, ErrOAuth
		}
		if len(f.UsedRefresh) >= 4096 {
			return OAuthTokens{}, ErrLimited
		}
		f.UsedRefresh = append(f.UsedRefresh, digest)
		tokens := o.issue(f)
		if err := o.save(state); err != nil {
			return OAuthTokens{}, err
		}
		return tokens, nil
	}
	return OAuthTokens{}, ErrOAuth
}

func (o *OAuth) access(token, resource string) (oauthFamily, bool) {
	if !o.available || !oauthOpaque(token, "kpo_") {
		return oauthFamily{}, false
	}
	digest := oauthDigest(token)
	for _, f := range o.state.Families {
		if !f.Revoked && f.Resource == resource && oauthEqual(f.AccessDigest, digest) && o.now().Before(f.AccessExpiresAt) && o.now().Before(f.ExpiresAt) {
			return f, true
		}
	}
	return oauthFamily{}, false
}
func (o *OAuth) Begin(base *Store, token, resource string) (Principal, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	f, ok := o.access(token, resource)
	if !ok {
		return Principal{}, nil, ErrUnauthorized
	}
	return base.beginOAuthGrant(f.GrantID)
}
func (o *OAuth) Active(base *Store, token, resource string, p Principal) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	f, ok := o.access(token, resource)
	return ok && f.GrantID == p.ID && base.Active(p)
}
func (o *OAuth) Revoke(base *Store, clientID, secret, method, token string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.available {
		return "", ErrUnavailable
	}
	if !o.clientAuth(clientID, secret, method) {
		return "", ErrOAuth
	}
	if !oauthOpaque(token, "kpo_") && !oauthOpaque(token, "kpr_") {
		return "", nil
	}
	state := o.next()
	digest := oauthDigest(token)
	for i := range state.Families {
		f := &state.Families[i]
		if f.ClientID == clientID && (oauthEqual(f.AccessDigest, digest) || oauthEqual(f.RefreshDigest, digest) || slices.Contains(f.UsedRefresh, digest)) {
			if err := base.revokeOAuthGrant(f.GrantID); err != nil {
				return "", err
			}
			f.Revoked = true
			return f.GrantID, o.save(state)
		}
	}
	return "", nil
}

func (o *OAuth) Reset() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.save(oauthState{Version: 1})
}
