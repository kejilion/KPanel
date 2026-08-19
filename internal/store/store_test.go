package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStorePersistsIdentitySessionAndAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "secret-hash",
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendAudit(AuditEvent{
		ID: "event-1", OccurredAt: now, ActorType: "user", ActorID: user.ID,
		Action: "auth.login", Result: "success", RequestID: "request-1",
	}, 100); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if !reopened.IsInitialized() {
		t.Fatal("store lost initialized state")
	}
	gotUser, err := reopened.UserByUsername("ADMIN")
	if err != nil || gotUser.ID != user.ID {
		t.Fatalf("unexpected user %#v, err=%v", gotUser, err)
	}
	if _, err := reopened.SessionByTokenHash("token-hash", now); err != nil {
		t.Fatal(err)
	}
	events, next := reopened.ListAudit(10, "")
	if len(events) != 1 || events[0].ID != "event-1" || next != "" {
		t.Fatalf("unexpected audit page: %#v next=%q", events, next)
	}
}

func TestTOTPStatePersistsConsumesOnceAndRevokesSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	user := User{ID: "user-1", Username: "admin", PasswordHash: "hash", Role: "admin", CreatedAt: now, UpdatedAt: now}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{TokenHash: "before", CSRFHash: "csrf", UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := storage.EnableUserTOTP(user.ID, "encrypted", now, 122, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.SessionByTokenHash("before", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("TOTP enable did not revoke session: %v", err)
	}
	if err := storage.ConsumeUserTOTPStep(user.ID, "encrypted", 123, now); err != nil {
		t.Fatal(err)
	}
	if err := storage.ConsumeUserTOTPStep(user.ID, "encrypted", 123, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("TOTP step replay was accepted: %v", err)
	}
	if err := storage.ConsumeUserTOTPStep(user.ID, "replaced-secret", 124, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale TOTP secret was accepted: %v", err)
	}
	if err := storage.ConsumeUserRecoveryCode(user.ID, "one", now); err != nil {
		t.Fatal(err)
	}
	if err := storage.ConsumeUserRecoveryCode(user.ID, "one", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("recovery hash replay was accepted: %v", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TOTPSecret != "encrypted" || got.TOTPLastUsedStep != 123 || len(got.TOTPRecoveryCodeHashes) != 1 {
		t.Fatalf("unexpected persisted TOTP state %#v", got)
	}
	if err := reopened.DisableUserTOTP(user.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ = reopened.UserByID(user.ID)
	if got.TOTPSecret != "" || got.TOTPEnabledAt != nil || len(got.TOTPRecoveryCodeHashes) != 0 {
		t.Fatalf("TOTP state survived disable %#v", got)
	}
}

func TestSecurityEntrancePersistsAndRejectsStaleWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	_, initialVersion := storage.SecurityEntrance()
	value := SecurityEntrance{Enabled: true, Path: "panel-a1b2c3", UpdatedAt: now}
	if err := storage.ReplaceSecurityEntrance(initialVersion, value); err != nil {
		t.Fatal(err)
	}
	if err := storage.ReplaceSecurityEntrance(initialVersion, SecurityEntrance{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write was not rejected: %v", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, version := reopened.SecurityEntrance()
	if got != value || version == initialVersion {
		t.Fatalf("unexpected persisted entrance: %#v version=%q", got, version)
	}
}

func TestClusterSharePersistsRejectsConflictsAndRollsBackWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	initial, version := storage.ClusterShare()
	if initial.Enabled || initial.Token != "" {
		t.Fatalf("unexpected initial cluster share %#v", initial)
	}
	want := ClusterShare{
		Enabled: true, Token: strings.Repeat("a", 64), Title: "My fleet",
		Description: "Public status", UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := storage.ReplaceClusterShare(version, want); err != nil {
		t.Fatal(err)
	}
	if err := storage.ReplaceClusterShare(version, want); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale cluster share update error = %v, want ErrConflict", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	storage, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	restored, restoredVersion := storage.ClusterShare()
	if restored != want {
		t.Fatalf("restored cluster share = %#v, want %#v", restored, want)
	}

	originalPath := storage.path
	storage.path = filepath.Join(t.TempDir(), "missing", "state.json")
	changed := want
	changed.Enabled = false
	changed.UpdatedAt = changed.UpdatedAt.Add(time.Second)
	err = storage.ReplaceClusterShare(restoredVersion, changed)
	storage.path = originalPath
	if err == nil {
		t.Fatal("cluster share update unexpectedly survived an atomic write failure")
	}
	afterFailure, _ := storage.ClusterShare()
	if afterFailure != want {
		t.Fatalf("failed write changed in-memory cluster share: %#v", afterFailure)
	}
}

func TestStoreRejectsOversizedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxStoreBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("oversized store was accepted")
	}
}

func TestStoreAllowsOnlyOneWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("second writer was not rejected: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("store did not unlock after Close: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSuccessfulLoginResetsEarlierFailureCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	key := "account:admin"
	if err := storage.RecordLoginAttempts([]LoginAttempt{
		{Key: key, OccurredAt: now.Add(-3 * time.Minute), Success: false},
		{Key: key, OccurredAt: now.Add(-2 * time.Minute), Success: true},
		{Key: key, OccurredAt: now.Add(-time.Minute), Success: false},
	}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if count := storage.FailedLoginCount(key, now.Add(-time.Hour)); count != 1 {
		t.Fatalf("expected one failure after the latest success, got %d", count)
	}
}

func TestSuccessfulLoginResetUsesAttemptTimeNotAppendOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	key := "account:admin"
	if err := storage.RecordLoginAttempts([]LoginAttempt{
		{Key: key, OccurredAt: now.Add(-time.Minute), Success: false},
		{Key: key, OccurredAt: now.Add(-3 * time.Minute), Success: false},
		{Key: key, OccurredAt: now.Add(-2 * time.Minute), Success: true},
	}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if count := storage.FailedLoginCount(key, now.Add(-time.Hour)); count != 1 {
		t.Fatalf("expected only the failure newer than the latest success, got %d", count)
	}
}

func TestDirectorySyncFailureDoesNotTurnACommittedWriteIntoFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	storage.syncDirectory = func(string) error {
		return errors.New("injected directory sync failure")
	}
	now := time.Now().UTC()
	err = storage.CreateInitialAdmin(User{
		ID: "user-1", Username: "admin", PasswordHash: "hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("committed write returned an ambiguous failure: %v", err)
	}
	if !storage.IsInitialized() {
		t.Fatal("committed in-memory state was rolled back")
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.IsInitialized() {
		t.Fatal("committed on-disk state was lost")
	}
}

func TestReplaceUserPasswordPersistsHashAndRevokesOnlyUserSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	updatedAt := now.Add(time.Minute)
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "old-hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}

	storage.mu.Lock()
	storage.data.Users = append(storage.data.Users, User{
		ID: "user-2", Username: "operator", PasswordHash: "other-hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	})
	err = storage.persistLocked()
	storage.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	for _, session := range []Session{
		{TokenHash: "user-token-1", CSRFHash: "csrf-1", UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "user-token-2", CSRFHash: "csrf-2", UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "other-token", CSRFHash: "csrf-3", UserID: "user-2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := storage.PutSession(session); err != nil {
			t.Fatal(err)
		}
	}

	if err := storage.ReplaceUserPassword(user.ID, "old-hash", "new-hash", updatedAt); err != nil {
		t.Fatal(err)
	}
	gotUser, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser.PasswordHash != "new-hash" || !gotUser.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("password metadata was not replaced: %#v", gotUser)
	}
	for _, tokenHash := range []string{"user-token-1", "user-token-2"} {
		if _, err := storage.SessionByTokenHash(tokenHash, now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("session %q was not revoked: %v", tokenHash, err)
		}
	}
	if _, err := storage.SessionByTokenHash("other-token", now); err != nil {
		t.Fatalf("another user's session was revoked: %v", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	gotUser, err = reopened.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser.PasswordHash != "new-hash" || !gotUser.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("password replacement was not persisted: %#v", gotUser)
	}
	if _, err := reopened.SessionByTokenHash("user-token-1", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked session reappeared after reopen: %v", err)
	}
	if _, err := reopened.SessionByTokenHash("other-token", now); err != nil {
		t.Fatalf("preserved session was lost after reopen: %v", err)
	}
}

func TestRecoverUserPasswordPreservesPanelStateAndRecordsAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	enabledAt := now.Add(-time.Hour)
	user := User{
		ID: "user-1", Username: "Admin", PasswordHash: "old-hash", Role: "admin",
		TOTPSecret: "encrypted-secret", TOTPEnabledAt: &enabledAt, TOTPLastUsedStep: 42,
		TOTPRecoveryCodeHashes: []string{"recovery-hash"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecordLoginAttempts([]LoginAttempt{
		{Key: "account:admin", OccurredAt: now, Success: false},
		{Key: "reauth:user-1", OccurredAt: now, Success: false},
		{Key: "ip:192.0.2.1", OccurredAt: now, Success: false},
	}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	entrance := SecurityEntrance{Enabled: true, Path: "private-entry", UpdatedAt: now}
	_, resourceVersion := storage.SecurityEntrance()
	if err := storage.ReplaceSecurityEntrance(resourceVersion, entrance); err != nil {
		t.Fatal(err)
	}

	updatedAt := now.Add(time.Minute)
	event := AuditEvent{
		ID: "recovery-event", OccurredAt: updatedAt, ActorType: "cli",
		Action: "auth.password.recover", TargetKind: "user", TargetID: user.ID, Result: "success",
	}
	if err := storage.RecoverUserPassword(PasswordRecovery{
		UserID: user.ID, ExpectedHash: user.PasswordHash, NewHash: "new-hash",
		UpdatedAt: updatedAt, AuditEvent: event, MaxAuditEntries: 10_000,
	}); err != nil {
		t.Fatal(err)
	}

	recovered, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.PasswordHash != "new-hash" || !recovered.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("password recovery was not applied: %#v", recovered)
	}
	if recovered.TOTPSecret != user.TOTPSecret || recovered.TOTPLastUsedStep != user.TOTPLastUsedStep ||
		len(recovered.TOTPRecoveryCodeHashes) != 1 {
		t.Fatalf("password-only recovery changed TOTP state: %#v", recovered)
	}
	if _, err := storage.SessionByTokenHash("token-hash", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("existing session survived recovery: %v", err)
	}
	if count := storage.FailedLoginCount("account:admin", now.Add(-time.Hour)); count != 0 {
		t.Fatalf("account failures were not cleared: %d", count)
	}
	if count := storage.FailedLoginCount("reauth:user-1", now.Add(-time.Hour)); count != 0 {
		t.Fatalf("reauth failures were not cleared: %d", count)
	}
	if count := storage.FailedLoginCount("ip:192.0.2.1", now.Add(-time.Hour)); count != 0 {
		t.Fatalf("IP failures were not cleared: %d", count)
	}
	gotEntrance, _ := storage.SecurityEntrance()
	if gotEntrance != entrance {
		t.Fatalf("security entrance changed: %#v", gotEntrance)
	}
	events, _ := storage.ListAudit(10, "")
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("recovery audit was not committed: %#v", events)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err = reopened.UserByID(user.ID)
	if err != nil || recovered.PasswordHash != "new-hash" {
		t.Fatalf("recovery did not persist: %#v, %v", recovered, err)
	}
}

func TestRecoverUserPasswordCanExplicitlyDisableTOTP(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	enabledAt := now
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "old-hash", Role: "admin",
		TOTPSecret: "encrypted-secret", TOTPEnabledAt: &enabledAt, TOTPLastUsedStep: 42,
		TOTPRecoveryCodeHashes: []string{"recovery-hash"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecoverUserPassword(PasswordRecovery{
		UserID: user.ID, ExpectedHash: user.PasswordHash, NewHash: "new-hash", DisableTOTP: true,
		UpdatedAt: now.Add(time.Minute), AuditEvent: AuditEvent{ID: "event", Action: "auth.password.recover"},
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.TOTPSecret != "" || recovered.TOTPEnabledAt != nil || recovered.TOTPLastUsedStep != 0 ||
		len(recovered.TOTPRecoveryCodeHashes) != 0 {
		t.Fatalf("TOTP state was not fully disabled: %#v", recovered)
	}
}

func TestRecoverUserPasswordPersistenceFailureRollsBackEntireTransition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "old-hash", Role: "admin",
		TOTPSecret: "encrypted-secret", TOTPRecoveryCodeHashes: []string{"recovery-hash"},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecordLoginAttempt(LoginAttempt{
		Key: "account:admin", OccurredAt: now, Success: false,
	}, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	originalPath := storage.path
	storage.path = filepath.Join(t.TempDir(), "missing", "state.json")
	err = storage.RecoverUserPassword(PasswordRecovery{
		UserID: user.ID, ExpectedHash: user.PasswordHash, NewHash: "new-hash", DisableTOTP: true,
		UpdatedAt: now.Add(time.Minute), AuditEvent: AuditEvent{ID: "event", Action: "auth.password.recover"},
	})
	storage.path = originalPath
	if err == nil {
		t.Fatal("expected persistence failure")
	}

	unchanged, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.PasswordHash != user.PasswordHash || unchanged.TOTPSecret != user.TOTPSecret ||
		len(unchanged.TOTPRecoveryCodeHashes) != 1 {
		t.Fatalf("failed recovery changed the user: %#v", unchanged)
	}
	if _, err := storage.SessionByTokenHash("token-hash", now); err != nil {
		t.Fatalf("failed recovery revoked the session: %v", err)
	}
	if count := storage.FailedLoginCount("account:admin", now.Add(-time.Hour)); count != 1 {
		t.Fatalf("failed recovery cleared login attempts: %d", count)
	}
	events, _ := storage.ListAudit(10, "")
	if len(events) != 0 {
		t.Fatalf("failed recovery recorded an audit event: %#v", events)
	}
}

func TestReplaceUserUsernamePersistsIdentityAndRevokesSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "password-hash", Role: "admin",
		TOTPSecret: "encrypted-secret", CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	updatedAt := now.Add(time.Minute)
	if err := storage.ReplaceUserUsername(user.ID, "admin", "operator", updatedAt); err != nil {
		t.Fatal(err)
	}
	updated, err := storage.UserByUsername("OPERATOR")
	if err != nil {
		t.Fatal(err)
	}
	if updated.PasswordHash != user.PasswordHash || updated.TOTPSecret != user.TOTPSecret || !updated.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unrelated account credentials changed: %#v", updated)
	}
	if _, err := storage.UserByUsername("admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old username remained available: %v", err)
	}
	if _, err := storage.SessionByTokenHash("token-hash", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session was not revoked: %v", err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.UserByUsername("operator"); err != nil {
		t.Fatalf("username change was not persisted: %v", err)
	}
}

func TestReplaceUserUsernameRejectsDuplicateAndStaleIdentity(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	if err := storage.CreateInitialAdmin(User{ID: "user-1", Username: "admin", PasswordHash: "hash", Role: "admin", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	storage.data.Users = append(storage.data.Users, User{ID: "user-2", Username: "operator", PasswordHash: "hash", Role: "admin", CreatedAt: now, UpdatedAt: now})
	err = storage.persistLocked()
	storage.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.ReplaceUserUsername("user-1", "stale", "new-admin", now.Add(time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale identity conflict, got %v", err)
	}
	if err := storage.ReplaceUserUsername("user-1", "admin", "OPERATOR", now.Add(time.Minute)); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected case-insensitive duplicate rejection, got %v", err)
	}
	current, err := storage.UserByID("user-1")
	if err != nil || current.Username != "admin" {
		t.Fatalf("rejected change modified identity: %#v, %v", current, err)
	}
}

func TestReplaceUserPasswordConflictLeavesStateUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "current-hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	err = storage.ReplaceUserPassword(user.ID, "stale-hash", "new-hash", now.Add(time.Minute))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	gotUser, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser.PasswordHash != user.PasswordHash || !gotUser.UpdatedAt.Equal(user.UpdatedAt) {
		t.Fatalf("conflict changed the user: %#v", gotUser)
	}
	if _, err := storage.SessionByTokenHash("token-hash", now); err != nil {
		t.Fatalf("conflict revoked the session: %v", err)
	}
}

func TestReplaceUserPasswordPersistenceFailureRollsBackMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	user := User{
		ID: "user-1", Username: "admin", PasswordHash: "old-hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(Session{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	originalPath := storage.path
	storage.path = filepath.Join(t.TempDir(), "missing", "state.json")
	err = storage.ReplaceUserPassword(user.ID, "old-hash", "new-hash", now.Add(time.Minute))
	storage.path = originalPath
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	gotUser, readErr := storage.UserByID(user.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if gotUser.PasswordHash != user.PasswordHash || !gotUser.UpdatedAt.Equal(user.UpdatedAt) {
		t.Fatalf("failed replacement changed in-memory user: %#v", gotUser)
	}
	if _, err := storage.SessionByTokenHash("token-hash", now); err != nil {
		t.Fatalf("failed replacement revoked in-memory session: %v", err)
	}
}

func TestConcurrentReplaceUserPasswordUsesCompareAndSwap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	now := time.Now().UTC()
	if err := storage.CreateInitialAdmin(User{
		ID: "user-1", Username: "admin", PasswordHash: "old-hash", Role: "admin",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, hash := range []string{"first-hash", "second-hash"} {
		go func(newHash string) {
			ready.Done()
			<-start
			results <- storage.ReplaceUserPassword("user-1", "old-hash", newHash, now.Add(time.Minute))
		}(hash)
	}
	ready.Wait()
	close(start)

	var successes, conflicts int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected replacement result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("got %d successes and %d conflicts", successes, conflicts)
	}
}
