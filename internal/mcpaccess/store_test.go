package mcpaccess

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func enabledStore(t *testing.T) *Store {
	t.Helper()
	s := Open(t.TempDir())
	if _, err := s.SetEnabled(true, s.Snapshot().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	return s
}
func createClient(t *testing.T, s *Store) (Client, string) {
	t.Helper()
	c, token, err := s.Create("Inspector", []HostGrant{{ID: "local", Identity: strings.Repeat("a", 64)}}, 24*time.Hour, s.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	return c, token
}
func TestCredentialLifecycleAndPersistence(t *testing.T) {
	s := enabledStore(t)
	c, token := createClient(t, s)
	b, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), token) || strings.Contains(string(b), token[4:]) {
		t.Fatal("plaintext credential persisted")
	}
	p, release, err := s.Begin(token)
	if err != nil {
		t.Fatal(err)
	}
	release()
	// Snapshots cannot mutate stored grants.
	v := s.Snapshot()
	v.Clients[0].Hosts[0].Identity = "changed"
	if !s.Active(p) || s.Snapshot().Clients[0].Hosts[0].Identity == "changed" {
		t.Fatal("snapshot aliases state")
	}
	reopened := Open(filepath.Dir(filepath.Dir(s.path)))
	_, done, err := reopened.Begin(token)
	if err != nil {
		t.Fatal(err)
	}
	done()
	if _, err := s.Revoke(c.ID, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := s.Revoke(c.ID, s.Snapshot().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if s.Active(p) {
		t.Fatal("revoked principal remains active")
	}
	if _, _, err := s.Begin(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if len(s.usage) != 0 {
		t.Fatal("revoked client rate entry retained")
	}
}
func TestExpiryDisableResetAndCorruptionFailClosed(t *testing.T) {
	s := enabledStore(t)
	_, token := createClient(t, s)
	p, done, err := s.Begin(token)
	if err != nil {
		t.Fatal(err)
	}
	done()
	s.now = func() time.Time { return p.ExpiresAt }
	if s.Active(p) {
		t.Fatal("expired credential accepted")
	}
	s.now = time.Now
	if _, err := s.SetEnabled(false, s.Snapshot().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if s.Active(p) {
		t.Fatal("disabled credential accepted")
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Enabled || len(s.Snapshot().Clients) != 0 {
		t.Fatal("restore retained grants")
	}
	if err := os.WriteFile(s.path, []byte(`{"version":900,"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	broken := Open(filepath.Dir(filepath.Dir(s.path)))
	if broken.Snapshot().Available {
		t.Fatal("corrupt state accepted")
	}
	if _, err := broken.SetEnabled(true, broken.Snapshot().ResourceVersion); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestCapacityValidationRateAndFailedPersistence(t *testing.T) {
	s := enabledStore(t)
	if _, _, err := s.Create("bad", nil, time.Hour, s.Snapshot().ResourceVersion); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	_, token := createClient(t, s)
	_, a, err := s.Begin(token)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := s.Begin(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Begin(token); !errors.Is(err, ErrLimited) {
		t.Fatal("concurrency bound", err)
	}
	a()
	a()
	b()
	for range 118 {
		_, end, e := s.Begin(token)
		if e != nil {
			t.Fatal(e)
		}
		end()
	}
	if _, _, err := s.Begin(token); !errors.Is(err, ErrLimited) {
		t.Fatal("rate bound", err)
	}
	for range MaxClients - 1 {
		createClient(t, s)
	}
	if _, _, err := s.Create("overflow", []HostGrant{{"local", strings.Repeat("a", 64)}}, time.Hour, s.Snapshot().ResourceVersion); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	s.write = func(string, any) error { return errors.New("disk full") }
	if _, err := s.SetEnabled(false, s.Snapshot().ResourceVersion); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := s.Begin(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("write failure did not fail closed", err)
	}
}
func TestDisabledStoreCreatesNoFiles(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	if s.Snapshot().Enabled || !s.Snapshot().Available {
		t.Fatal("wrong defaults")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("default created state", err)
	}
	for _, token := range []string{"", "session", "kpm_" + strings.Repeat("a", 64), strings.Repeat("b", 10000)} {
		if _, _, err := s.Begin(token); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("invalid token accepted")
		}
	}
}
