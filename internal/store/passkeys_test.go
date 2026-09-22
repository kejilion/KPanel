package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func passkeyFixture(t *testing.T, bound bool) (*Store, User, Session, Passkey) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := time.Now().UTC().Add(-time.Second)
	key := Passkey{ID: base64.RawURLEncoding.EncodeToString([]byte("credential-1")), RPID: "panel.example.com", Name: "我的安全密钥", Credential: json.RawMessage(`{"signCount":0}`), CreatedAt: now}
	u := User{ID: "admin-id", Username: "admin", Role: "admin", PasswordHash: strings.Repeat("h", 64), CreatedAt: now, UpdatedAt: now}
	if bound {
		u.Passkeys = []Passkey{key}
	}
	if err := s.CreateInitialAdmin(u); err != nil {
		t.Fatal(err)
	}
	session := Session{TokenHash: "existing", CSRFHash: "csrf", UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := s.PutSession(session); err != nil {
		t.Fatal(err)
	}
	return s, u, session, key
}

func passkeyLogin(key Passkey, original Session) (Passkey, Session) {
	used := time.Now().UTC()
	key.LastUsedAt = &used
	key.Credential = json.RawMessage(`{"signCount":1}`)
	original.TokenHash = "passkey-login"
	original.CreatedAt = used
	return key, original
}

func TestPasskeyOriginPersistsWithCompareAndSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	initial, version := s.PasskeyOrigin()
	if initial != "" || version != PasskeyOriginResourceVersion("") {
		t.Fatalf("unexpected initial origin: %q %q", initial, version)
	}
	if err := s.ReplacePasskeyOrigin(version, "https://panel.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePasskeyOrigin(version, "https://other.example.com"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale origin write error = %v", err)
	}
	if s.data.SchemaVersion != 2 {
		t.Fatalf("origin setting did not protect store schema: %d", s.data.SchemaVersion)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, gotVersion := reopened.PasskeyOrigin()
	if got != "https://panel.example.com" || gotVersion == version {
		t.Fatalf("origin was not persisted: %q %q", got, gotVersion)
	}
	for _, value := range []string{" https://bad.example.com", "https://bad\n.example.com", strings.Repeat("x", 513)} {
		if err := reopened.ReplacePasskeyOrigin(gotVersion, value); !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("unsafe origin %q error = %v", value, err)
		}
	}
}

func TestHasPasskeysCoversAllUsers(t *testing.T) {
	s, _, session, key := passkeyFixture(t, false)
	if s.HasPasskeys() {
		t.Fatal("empty store reported passkeys")
	}
	u, err := s.UserByID(session.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceUserPasskeys(u, []Passkey{key}, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.HasPasskeys() {
		t.Fatal("stored passkey was not reported")
	}
}

func TestPasskeyRegistrationPersistsSchemaAndRevokesSessions(t *testing.T) {
	s, u, session, key := passkeyFixture(t, false)
	if s.data.SchemaVersion != 1 {
		t.Fatal("empty legacy store changed schema")
	}
	if err := s.ReplaceUserPasskeys(u, []Passkey{key}, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByTokenHash(session.TokenHash, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session survived registration: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.UserByID(u.ID)
	if err != nil || got.CredentialVersion != 1 || len(got.Passkeys) != 1 || reopened.data.SchemaVersion != 2 {
		t.Fatalf("registration not persisted: %#v %v", got, err)
	}
	if err := reopened.PutSession(session); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReplaceUserPasskeys(got, nil, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if reopened.data.SchemaVersion != 2 {
		t.Fatal("removal downgraded schema")
	}
	content, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	var state diskState
	if err := json.Unmarshal(content, &state); err != nil || state.SchemaVersion != 2 || len(state.Users[0].Passkeys) != 0 {
		t.Fatalf("removal state invalid: %v", err)
	}
}

func TestPasskeyReadAndInputSnapshotsAreIndependent(t *testing.T) {
	s, u, _, key := passkeyFixture(t, true)
	u.Passkeys[0].Name = "input-mutated"
	u.Passkeys[0].Credential[0] = 'x'
	for _, lookup := range []func() (User, error){func() (User, error) { return s.UserByID(u.ID) }, func() (User, error) { return s.UserByUsername(u.Username) }} {
		got, err := lookup()
		if err != nil || got.Passkeys[0].Name != key.Name || !json.Valid(got.Passkeys[0].Credential) {
			t.Fatalf("input alias changed store: %#v %v", got, err)
		}
		got.Passkeys[0].Credential[0] = '!'
		got.Passkeys[0].Name = "read-mutated"
	}
	got, _ := s.UserByID(u.ID)
	loginKey, loginSession := passkeyLogin(got.Passkeys[0], Session{UserID: u.ID, CSRFHash: "csrf", ExpiresAt: time.Now().Add(time.Hour)})
	if err := s.UpdateUserPasskey(got, loginKey, loginSession); err != nil {
		t.Fatal(err)
	}
	*loginKey.LastUsedAt = time.Time{}
	got, _ = s.UserByID(u.ID)
	if got.Passkeys[0].LastUsedAt.IsZero() {
		t.Fatal("last-used input alias changed store")
	}
	*got.Passkeys[0].LastUsedAt = time.Time{}
	fresh, _ := s.UserByID(u.ID)
	if fresh.Passkeys[0].LastUsedAt.IsZero() {
		t.Fatal("last-used output alias changed store")
	}
}

func TestPasskeyValidationAndLimits(t *testing.T) {
	_, _, _, valid := passkeyFixture(t, false)
	tests := map[string]func(*Passkey){
		"empty id":           func(k *Passkey) { k.ID = "" },
		"noncanonical id":    func(k *Passkey) { k.ID = "YR" },
		"oversized id":       func(k *Passkey) { k.ID = strings.Repeat("a", 2050) },
		"empty name":         func(k *Passkey) { k.Name = " " },
		"control name":       func(k *Passkey) { k.Name = "key\nname" },
		"invalid utf8 name":  func(k *Passkey) { k.Name = string([]byte{0xff}) },
		"oversized name":     func(k *Passkey) { k.Name = strings.Repeat("钥", 65) },
		"empty rpid":         func(k *Passkey) { k.RPID = "" },
		"url rpid":           func(k *Passkey) { k.RPID = "https://example.com" },
		"ip rpid":            func(k *Passkey) { k.RPID = "192.0.2.1" },
		"bad label":          func(k *Passkey) { k.RPID = "-example.com" },
		"oversized rpid":     func(k *Passkey) { k.RPID = strings.Repeat("a.", 128) },
		"null credential":    func(k *Passkey) { k.Credential = json.RawMessage(`null`) },
		"array credential":   func(k *Passkey) { k.Credential = json.RawMessage(`[]`) },
		"invalid credential": func(k *Passkey) { k.Credential = json.RawMessage(`{`) },
		"oversized credential": func(k *Passkey) {
			k.Credential = json.RawMessage(`{"data":"` + strings.Repeat("a", MaxPasskeyCredentialBytes) + `"}`)
		},
		"zero creation": func(k *Passkey) { k.CreatedAt = time.Time{} },
		"old usage":     func(k *Passkey) { old := k.CreatedAt.Add(-time.Second); k.LastUsedAt = &old },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if !errors.Is(validatePasskey(value), ErrInvalidRecord) {
				t.Fatal("invalid passkey accepted")
			}
		})
	}
	valid.Name = strings.Repeat("钥", 64)
	if err := validatePasskey(valid); err != nil {
		t.Fatalf("valid unicode name rejected: %v", err)
	}
	if err := validatePasskeys([]Passkey{valid, valid}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate id accepted: %v", err)
	}
	values := make([]Passkey, MaxPasskeys)
	for i := range values {
		values[i] = valid
		values[i].ID = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprint(i)))
	}
	if err := validatePasskeys(values); err != nil {
		t.Fatalf("exact limit rejected: %v", err)
	}
	if err := validatePasskeys(append(values, valid)); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("excess credentials accepted: %v", err)
	}
}

func TestPasskeyManagementRequiresCurrentSessionAndSnapshot(t *testing.T) {
	for _, kind := range []string{"expired", "wrong user", "missing", "version", "password"} {
		t.Run(kind, func(t *testing.T) {
			s, u, session, key := passkeyFixture(t, false)
			now := time.Now()
			switch kind {
			case "expired":
				now = session.ExpiresAt
			case "wrong user":
				s.data.Sessions[0].UserID = "other"
			case "missing":
				session.TokenHash = "absent"
			case "version":
				u.CredentialVersion++
			case "password":
				u.PasswordHash = "stale"
			}
			if err := s.ReplaceUserPasskeys(u, []Passkey{key}, session.TokenHash, now); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale management accepted: %v", err)
			}
			got, _ := s.UserByID(u.ID)
			if len(got.Passkeys) != 0 || s.data.SchemaVersion != 1 {
				t.Fatal("failed management changed state")
			}
		})
	}
}

func TestPasskeyLoginAtomicWithRevocationAndStateCAS(t *testing.T) {
	s, u, session, key := passkeyFixture(t, true)
	updated, login := passkeyLogin(key, session)
	if err := s.UpdateUserPasskey(u, updated, login); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByTokenHash(login.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserPasskey(u, updated, login); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale authenticator state accepted: %v", err)
	}
	// Registration based on stale counters cannot overwrite a successful login.
	if err := s.ReplaceUserPasskeys(u, []Passkey{key}, session.TokenHash, time.Now()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale management rolled back counter: %v", err)
	}
	fresh, _ := s.UserByID(u.ID)
	if err := s.ReplaceUserPasskeys(fresh, nil, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserPasskey(fresh, updated, login); !errors.Is(err, ErrConflict) {
		t.Fatalf("revoked passkey logged in: %v", err)
	}
	if _, err := s.SessionByTokenHash(login.TokenHash, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked login survived: %v", err)
	}
}

func TestConcurrentPasskeyLoginCannotSurviveRevocation(t *testing.T) {
	for i := 0; i < 10; i++ {
		s, u, session, key := passkeyFixture(t, true)
		updated, login := passkeyLogin(key, session)
		var wg sync.WaitGroup
		var loginErr, revokeErr error
		wg.Add(2)
		go func() { defer wg.Done(); loginErr = s.UpdateUserPasskey(u, updated, login) }()
		go func() { defer wg.Done(); revokeErr = s.ReplaceUserPasskeys(u, nil, session.TokenHash, time.Now()) }()
		wg.Wait()
		if revokeErr != nil || (loginErr != nil && !errors.Is(loginErr, ErrConflict)) {
			t.Fatalf("unexpected errors login=%v revoke=%v", loginErr, revokeErr)
		}
		if _, err := s.SessionByTokenHash(login.TokenHash, time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatal("login survived concurrent revocation")
		}
	}
}

func TestPasskeyWritesRollbackAfterPersistenceFailure(t *testing.T) {
	for _, operation := range []string{"register", "login", "recover", "restore"} {
		t.Run(operation, func(t *testing.T) {
			s, u, session, key := passkeyFixture(t, operation != "register")
			before := cloneDiskState(s.data)
			diskBefore, err := os.ReadFile(s.path)
			if err != nil {
				t.Fatal(err)
			}
			backup, err := s.ExportIdentity()
			if err != nil {
				t.Fatal(err)
			}
			path := s.path
			s.path = filepath.Join(t.TempDir(), "missing", "state.json")
			switch operation {
			case "register":
				err = s.ReplaceUserPasskeys(u, []Passkey{key}, session.TokenHash, time.Now())
			case "login":
				updated, login := passkeyLogin(key, session)
				err = s.UpdateUserPasskey(u, updated, login)
			case "recover":
				err = s.RecoverUserPassword(PasswordRecovery{UserID: u.ID, ExpectedHash: u.PasswordHash, NewHash: "new-hash", UpdatedAt: time.Now(), AuditEvent: AuditEvent{ID: "recover", Action: "auth.password.recover"}})
			case "restore":
				err = s.RestoreIdentity(backup)
			}
			s.path = path
			if err == nil {
				t.Fatal("write unexpectedly succeeded")
			}
			if !reflect.DeepEqual(before, cloneDiskState(s.data)) {
				t.Fatal("failed write changed memory")
			}
			diskAfter, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(diskBefore, diskAfter) {
				t.Fatalf("failed write changed disk: %v", err)
			}
		})
	}
}

func TestIdentityBackupAndRecoveryNeverRestorePasskeys(t *testing.T) {
	s, u, session, key := passkeyFixture(t, true)
	backup, err := s.ExportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	var state diskState
	if err := json.Unmarshal(backup, &state); err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 1 || len(state.Users[0].Passkeys) != 0 {
		t.Fatal("export contains passkeys")
	}
	if err := ValidateIdentityBackup(backup); err != nil {
		t.Fatal(err)
	}
	state.Users[0].Passkeys = []Passkey{key}
	invalid, _ := json.Marshal(state)
	if err := ValidateIdentityBackup(invalid); err == nil {
		t.Fatal("backup with passkeys accepted")
	}
	if err := s.ReplaceUserPasskeys(u, nil, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreIdentity(backup); err != nil {
		t.Fatal(err)
	}
	got, _ := s.UserByID(u.ID)
	if len(got.Passkeys) != 0 || got.CredentialVersion != 2 || s.data.SchemaVersion != 2 {
		t.Fatalf("restore reactivated older state: %#v", got)
	}
	if err := s.PutSession(session); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceUserPasskeys(got, []Passkey{key}, session.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverUserPassword(PasswordRecovery{UserID: u.ID, ExpectedHash: u.PasswordHash, NewHash: "new-hash", UpdatedAt: time.Now(), AuditEvent: AuditEvent{ID: "recover", Action: "auth.password.recover"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.UserByID(u.ID)
	if len(got.Passkeys) != 0 || got.CredentialVersion != 4 {
		t.Fatalf("recovery retained passkeys: %#v", got)
	}
}

func TestOpenValidatesPasskeySchemaAndCredential(t *testing.T) {
	for _, kind := range []string{"valid", "legacy with key", "invalid key", "future schema"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _, _ := passkeyFixture(t, true)
			state := cloneDiskState(s.data)
			switch kind {
			case "legacy with key":
				state.SchemaVersion = 1
			case "invalid key":
				state.Users[0].Passkeys[0].RPID = "https://example.com"
			case "future schema":
				state.SchemaVersion = 3
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(state)
			if err := os.WriteFile(s.path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(s.path)
			if kind == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				_ = reopened.Close()
			} else if err == nil {
				_ = reopened.Close()
				t.Fatal("invalid state accepted")
			}
		})
	}
}

func TestCredentialVersionTracksSecurityChangesOnly(t *testing.T) {
	s, u, _, _ := passkeyFixture(t, false)
	now := time.Now()
	steps := []struct {
		run     func() error
		version uint64
	}{
		{func() error { return s.ReplaceUserPassword(u.ID, u.PasswordHash, "new-hash", now) }, 1},
		{func() error { return s.ReplaceUserUsername(u.ID, u.Username, "new-name", now) }, 2},
		{func() error { return s.EnableUserTOTP(u.ID, 2, "secret", now, 1, []string{"recovery"}) }, 3},
		{func() error { return s.ConsumeUserTOTPStep(u.ID, "secret", 2, now) }, 3},
		{func() error { return s.ConsumeUserRecoveryCode(u.ID, "recovery", now) }, 3},
		{func() error { return s.ReplaceUserRecoveryCodes(u.ID, []string{"new-recovery"}, now) }, 4},
		{func() error { return s.DisableUserTOTP(u.ID, now) }, 5},
	}
	for i, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		got, _ := s.UserByID(u.ID)
		if got.CredentialVersion != step.version {
			t.Fatalf("step %d version=%d expected=%d", i, got.CredentialVersion, step.version)
		}
	}
	s.data.Users[0].CredentialVersion = ^uint64(0)
	if err := s.ReplaceUserPassword(u.ID, "new-hash", "another", now); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("version overflow accepted: %v", err)
	}
	if s.data.Users[0].PasswordHash != "new-hash" {
		t.Fatal("overflow changed password")
	}
}

func TestPasskeyLoginRetainsSessionLimit(t *testing.T) {
	s, u, session, key := passkeyFixture(t, true)
	for i := 0; i < maxRetainedSessions; i++ {
		extra := session
		extra.TokenHash = fmt.Sprint(i)
		s.data.Sessions = append(s.data.Sessions, extra)
	}
	updated, login := passkeyLogin(key, session)
	if err := s.UpdateUserPasskey(u, updated, login); err != nil {
		t.Fatal(err)
	}
	if len(s.data.Sessions) != maxRetainedSessions {
		t.Fatalf("session count=%d", len(s.data.Sessions))
	}
	if _, err := s.SessionByTokenHash(login.TokenHash, time.Now()); err != nil {
		t.Fatal(err)
	}
}
