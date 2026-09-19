// Package mcpaccess owns delegated MCP credentials. It never owns
// host resources or accepts Panel sessions / Agent credentials.
package mcpaccess

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

const MaxClients = 32
const MaxHosts = 101
const MaxBytes = 512 << 10

var (
	ErrUnavailable  = errors.New("mcp_access_unavailable")
	ErrConflict     = errors.New("resource_version_changed")
	ErrInvalid      = errors.New("invalid_mcp_access")
	ErrUnauthorized = errors.New("mcp_authentication_required")
	ErrLimited      = errors.New("mcp_rate_limited")
)

type HostGrant struct {
	ID       string `json:"id"`
	Identity string `json:"identity"`
}

type Client struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Hosts     []HostGrant `json:"hosts"`
	CreatedAt time.Time   `json:"createdAt"`
	ExpiresAt time.Time   `json:"expiresAt"`
	Policy    *Policy     `json:"policy,omitempty"`
}

type credential struct {
	Client
	Digest string `json:"digest"`
}

type state struct {
	Version int          `json:"version"`
	Enabled bool         `json:"enabled"`
	Clients []credential `json:"clients"`
}

type Snapshot struct {
	Enabled         bool     `json:"enabled"`
	Available       bool     `json:"available"`
	ResourceVersion string   `json:"resourceVersion"`
	Clients         []Client `json:"clients"`
}

type Principal struct {
	Client
	digest string
}

type usage struct {
	minute        int64
	count, active int
}

type Store struct {
	mu        sync.Mutex
	path      string
	state     state
	available bool
	usage     map[string]*usage
	write     func(string, any) error
	now       func() time.Time
}

// Open does not create files for installations that have never enabled MCP.
// Corrupt credentials disable only MCP; they must not prevent Panel startup.
func Open(dataDir string) *Store {
	s := &Store{path: filepath.Join(dataDir, "mcp-access", "access.json"), state: state{Version: 1}, available: true, usage: make(map[string]*usage), write: backup.WriteJSON, now: time.Now}
	b, err := backup.ReadFile(s.path, MaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil || json.Unmarshal(b, &s.state) != nil || !validState(s.state) {
		s.available = false
		s.state = state{Version: 1}
	}
	return s
}

func validState(v state) bool {
	if v.Version != 1 || len(v.Clients) > MaxClients {
		return false
	}
	ids := map[string]bool{}
	for _, c := range v.Clients {
		if !validHex(c.ID, 32) || !validHex(c.Digest, 64) || ids[c.ID] || !validName(c.Name) || !validHosts(c.Hosts) || !c.Policy.Valid() || c.CreatedAt.IsZero() || !c.ExpiresAt.After(c.CreatedAt) || c.ExpiresAt.Sub(c.CreatedAt) > 90*24*time.Hour {
			return false
		}
		ids[c.ID] = true
	}
	return true
}

func validHex(s string, n int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(s) == n && len(b)*2 == n
}
func validName(s string) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) != s || s == "" || utf8.RuneCountInString(s) > 48 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validHosts(hosts []HostGrant) bool {
	if len(hosts) == 0 || len(hosts) > MaxHosts {
		return false
	}
	seen := map[string]bool{}
	for _, h := range hosts {
		if h.ID == "" || len(h.ID) > 128 || !validHex(h.Identity, 64) || seen[h.ID] {
			return false
		}
		seen[h.ID] = true
	}
	return true
}

func (s *Store) version() string {
	b, _ := json.Marshal(s.state)
	v := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(v[:])
}
func (s *Store) snapshot() Snapshot {
	v := Snapshot{Enabled: s.available && s.state.Enabled, Available: s.available, ResourceVersion: s.version(), Clients: []Client{}}
	for _, c := range s.state.Clients {
		v.Clients = append(v.Clients, clone(c.Client))
	}
	return v
}
func clone(c Client) Client {
	c.Hosts = append([]HostGrant(nil), c.Hosts...)
	c.Policy = clonePolicy(c.Policy)
	return c
}
func (s *Store) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot() }

// Enabled is allocation-free for unauthenticated requests to the public route.
func (s *Store) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.available && s.state.Enabled
}

func (s *Store) save(next state) error {
	data, err := json.Marshal(next)
	if err != nil || len(data) > MaxBytes {
		return ErrLimited
	}
	if err := s.write(s.path, next); err != nil {
		// A failed directory sync may mean rename succeeded. Never keep using an
		// in-memory credential set that could disagree with durable revocations.
		s.available = false
		return ErrUnavailable
	}
	s.state = next
	return nil
}
func (s *Store) expected(version string) error {
	if !s.available {
		return ErrUnavailable
	}
	if version != s.version() {
		return ErrConflict
	}
	return nil
}
func (s *Store) SetEnabled(enabled bool, version string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expected(version); err != nil {
		return Snapshot{}, err
	}
	next := s.state
	next.Enabled = enabled
	if err := s.save(next); err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(), nil
}

func (s *Store) Create(name string, hosts []HostGrant, lifetime time.Duration, version string) (Client, string, error) {
	return s.CreateWithPolicy(name, hosts, nil, lifetime, version)
}

func (s *Store) CreateWithPolicy(name string, hosts []HostGrant, policy *Policy, lifetime time.Duration, version string) (Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expected(version); err != nil {
		return Client{}, "", err
	}
	if !s.state.Enabled || !validName(name) || !validHosts(hosts) || !policy.Valid() || lifetime < time.Hour || lifetime > 90*24*time.Hour || len(s.state.Clients) >= MaxClients {
		return Client{}, "", ErrInvalid
	}
	var id [16]byte
	var secret [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Client{}, "", err
	}
	if _, err := rand.Read(secret[:]); err != nil {
		return Client{}, "", err
	}
	token := "kpm_" + hex.EncodeToString(secret[:])
	digest := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	c := Client{ID: hex.EncodeToString(id[:]), Name: name, Hosts: append([]HostGrant(nil), hosts...), CreatedAt: now, ExpiresAt: now.Add(lifetime), Policy: clonePolicy(policy)}
	next := s.state
	next.Clients = append(append([]credential(nil), next.Clients...), credential{Client: c, Digest: hex.EncodeToString(digest[:])})
	if err := s.save(next); err != nil {
		return Client{}, "", err
	}
	return clone(c), token, nil
}

func (s *Store) Revoke(id, version string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expected(version); err != nil {
		return Snapshot{}, err
	}
	return s.revokeLocked(id)
}

// OAuth revocation already authenticates the token. It cannot race a UI
// resource-version check and leave its backing grant usable for old approvals.
func (s *Store) revokeOAuthGrant(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrUnavailable
	}
	_, err := s.revokeLocked(id)
	if errors.Is(err, ErrInvalid) {
		return nil
	}
	return err
}

func (s *Store) revokeLocked(id string) (Snapshot, error) {
	next := s.state
	next.Clients = make([]credential, 0, len(s.state.Clients))
	found := false
	for _, c := range s.state.Clients {
		if c.ID == id {
			found = true
		} else {
			next.Clients = append(next.Clients, c)
		}
	}
	if !found {
		return Snapshot{}, ErrInvalid
	}
	if err := s.save(next); err != nil {
		return Snapshot{}, err
	}
	delete(s.usage, id)
	return s.snapshot(), nil
}

// Reset disables and destroys grants at the Panel restore boundary. Credentials
// are excluded from portable backups; restoring one cannot resurrect access.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.save(state{Version: 1}); err != nil {
		return err
	}
	s.available = true
	s.usage = make(map[string]*usage)
	return nil
}

func (s *Store) find(digest string) (Principal, error) {
	if !s.available || !s.state.Enabled {
		return Principal{}, ErrUnauthorized
	}
	for _, c := range s.state.Clients {
		if subtle.ConstantTimeCompare([]byte(c.Digest), []byte(digest)) == 1 && s.now().Before(c.ExpiresAt) {
			return Principal{Client: clone(c.Client), digest: digest}, nil
		}
	}
	return Principal{}, ErrUnauthorized
}

// Begin bounds each client's request rate and concurrency without unbounded
// per-IP maps. Its release function must be called even when a request fails.
func (s *Store) Begin(token string) (Principal, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(token) != 68 || !strings.HasPrefix(token, "kpm_") || !validHex(token[4:], 64) {
		return Principal{}, nil, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(token))
	p, err := s.find(hex.EncodeToString(digest[:]))
	if err != nil {
		return Principal{}, nil, err
	}
	return s.beginLocked(p)
}

func (s *Store) beginOAuthGrant(id string) (Principal, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.state.Clients {
		if c.ID == id {
			p, err := s.find(c.Digest)
			if err != nil {
				return Principal{}, nil, err
			}
			return s.beginLocked(p)
		}
	}
	return Principal{}, nil, ErrUnauthorized
}

func (s *Store) beginLocked(p Principal) (Principal, func(), error) {
	u := s.usage[p.ID]
	if u == nil {
		u = &usage{}
		s.usage[p.ID] = u
	}
	minute := s.now().Unix() / 60
	if minute != u.minute {
		u.minute = minute
		u.count = 0
	}
	if u.count >= 120 || u.active >= 2 {
		return Principal{}, nil, ErrLimited
	}
	u.count++
	u.active++
	var once sync.Once
	return p, func() { once.Do(func() { s.mu.Lock(); defer s.mu.Unlock(); u.active-- }) }, nil
}
func (s *Store) Active(p Principal) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.find(p.digest)
	return err == nil
}

// LookupClient is for an already authenticated Panel administrator executing
// an approved plan. It is never a public credential exchange endpoint.
func (s *Store) LookupClient(id string) (Principal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.state.Clients {
		if c.ID == id {
			return s.find(c.Digest)
		}
	}
	return Principal{}, ErrUnauthorized
}
