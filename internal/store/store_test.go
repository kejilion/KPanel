package store

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
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

func TestFileSharePersistsOnlyTokenHashAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(7 * 24 * time.Hour)
	expectedExpiresAt := expiresAt
	token := "file-share-token-that-must-never-be-persisted"
	want := testFileShare("share-1", token, "/shared.txt", "sha256:v1", now, &expiresAt)
	if _, _, err := storage.CreateFileShare(want, "", now); err != nil {
		t.Fatal(err)
	}
	expiresAt = now
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), token) {
		t.Fatalf("raw file share token was persisted: %s", content)
	}
	if !strings.Contains(string(content), want.TokenHash) {
		t.Fatalf("file share token hash was not persisted: %s", content)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.FileShareByToken(token, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Path != want.Path || got.ResourceVersion != want.ResourceVersion ||
		got.TokenHash != want.TokenHash || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expectedExpiresAt) {
		t.Fatalf("reopened file share = %#v, want %#v", got, want)
	}
	if _, err := reopened.FileShareByToken(token+"-wrong", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong bearer token lookup error = %v, want ErrNotFound", err)
	}
	if _, err := reopened.DeleteFileShare(want.ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	deletedState, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = deletedState.Close() })
	if _, err := deletedState.FileShareByToken(token, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted share survived reopen: %v", err)
	}
}

func TestFileShareCreateRotatesSamePath(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	oldToken := "old-file-share-token"
	oldShare := testFileShare("share-old", oldToken, "/shared.txt", "sha256:v1", now, nil)
	if _, _, err := storage.CreateFileShare(oldShare, "", now); err != nil {
		t.Fatal(err)
	}
	newToken := "new-file-share-token"
	newShare := testFileShare("share-new", newToken, oldShare.Path, "sha256:v2", now.Add(time.Minute), nil)
	created, replaced, err := storage.CreateFileShare(newShare, "", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if replaced.ID != oldShare.ID {
		t.Fatalf("replaced share = %#v, want ID %q", replaced, oldShare.ID)
	}
	if created.ID != newShare.ID {
		t.Fatalf("created file share = %#v, want id %q", created, newShare.ID)
	}
	concurrent := testFileShare("share-concurrent", "concurrent-token", oldShare.Path, newShare.ResourceVersion, now.Add(2*time.Minute), nil)
	concurrent.ResourceVersion = newShare.ResourceVersion
	concurrent.ShareVersion = newShare.ShareVersion
	if _, _, err := storage.CreateFileShare(concurrent, "", now.Add(2*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("same-version create without expected ID error = %v, want ErrConflict", err)
	}
	if _, err := storage.FileShareByToken(oldToken, now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotated bearer token lookup error = %v, want ErrNotFound", err)
	}
	if _, err := storage.FileShareByPath(oldShare.Path, oldShare.ResourceVersion, now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotated path/version lookup error = %v, want ErrNotFound", err)
	}
	got, err := storage.FileShareByToken(newToken, now.Add(time.Minute))
	if err != nil || got.ID != newShare.ID || got.ResourceVersion != newShare.ResourceVersion {
		t.Fatalf("new bearer token lookup = %#v, err=%v", got, err)
	}
}

func TestFileShareExpiresAtBoundaryAndIsPurgedOnCreate(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	expiredToken := "expiring-file-share-token"
	expiring := testFileShare("share-expiring", expiredToken, "/expiring.txt", "sha256:expiring", now, &expiresAt)
	if _, _, err := storage.CreateFileShare(expiring, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.FileShareByToken(expiredToken, expiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("share expired before its deadline: %v", err)
	}
	if _, err := storage.FileShareByToken(expiredToken, expiresAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("share lookup at expiry error = %v, want ErrNotFound", err)
	}

	replacementToken := "replacement-file-share-token"
	replacement := testFileShare("share-replacement", replacementToken, "/replacement.txt", "sha256:replacement", expiresAt, nil)
	if _, _, err := storage.CreateFileShare(replacement, "", expiresAt); err != nil {
		t.Fatal(err)
	}
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	if len(storage.data.FileShares) != 1 || storage.data.FileShares[0].ID != replacement.ID {
		t.Fatalf("expired records were not purged on create: %#v", storage.data.FileShares)
	}
}

func TestFileShareListIsActiveSortedAndIsolated(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	firstExpiry := now.Add(24 * time.Hour)
	first := testFileShare("first", "token-first", "/first.txt", "version-first", now, &firstExpiry)
	second := testFileShare("second", "token-second", "/second.txt", "version-second", now.Add(time.Minute), nil)
	expiredAt := now
	expired := testFileShare("expired", "token-expired", "/expired.txt", "version-expired", now.Add(-time.Hour), &expiredAt)
	storage.mu.Lock()
	storage.data.FileShares = []FileShare{first, expired, second}
	storage.mu.Unlock()

	items := storage.ListFileShares(now)
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("active file shares = %#v, want newest first and expired omitted", items)
	}
	if items[1].ExpiresAt == nil {
		t.Fatal("listed expiry was lost")
	}
	*items[1].ExpiresAt = now
	itemsAgain := storage.ListFileShares(now)
	if itemsAgain[1].ExpiresAt == nil || !itemsAgain[1].ExpiresAt.Equal(firstExpiry) {
		t.Fatalf("mutating listed expiry changed store state: %#v", itemsAgain[1])
	}
}

func TestFileShareLimitIsFixedAndExpiredSlotCanBeReused(t *testing.T) {
	if MaxFileShares != 256 {
		t.Fatalf("MaxFileShares = %d, want fixed lightweight limit 256", MaxFileShares)
	}
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)

	storage.mu.Lock()
	storage.data.FileShares = make([]FileShare, 0, MaxFileShares)
	for index := range MaxFileShares {
		token := fmt.Sprintf("file-share-token-%03d", index)
		storage.data.FileShares = append(storage.data.FileShares, testFileShare(
			fmt.Sprintf("share-%03d", index), token, fmt.Sprintf("/file-%03d.txt", index),
			fmt.Sprintf("sha256:%03d", index), now, nil,
		))
	}
	storage.mu.Unlock()

	overflowToken := "file-share-token-overflow"
	overflow := testFileShare("share-overflow", overflowToken, "/overflow.txt", "sha256:overflow", now, nil)
	if _, _, err := storage.CreateFileShare(overflow, "", now); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("overflow create error = %v, want ErrLimitReached", err)
	}
	if _, err := storage.FileShareByToken(overflowToken, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("overflow share changed in-memory state: %v", err)
	}

	expiredAt := now
	storage.mu.Lock()
	storage.data.FileShares[0].ExpiresAt = &expiredAt
	storage.mu.Unlock()
	created, _, err := storage.CreateFileShare(overflow, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != overflow.ID {
		t.Fatalf("created file share = %#v, want id %q", created, overflow.ID)
	}
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	if len(storage.data.FileShares) != MaxFileShares {
		t.Fatalf("file share count = %d, want %d", len(storage.data.FileShares), MaxFileShares)
	}
}

func TestFileShareWriteFailureRollsBackCreateAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	originalToken := "original-file-share-token"
	original := testFileShare("share-original", originalToken, "/shared.txt", "sha256:v1", now, nil)
	if _, _, err := storage.CreateFileShare(original, "", now); err != nil {
		t.Fatal(err)
	}

	replacementToken := "replacement-file-share-token"
	replacement := testFileShare("share-replacement", replacementToken, original.Path, "sha256:v2", now.Add(time.Minute), nil)
	originalPath := storage.path
	storage.path = filepath.Join(t.TempDir(), "missing", "state.json")
	_, _, createErr := storage.CreateFileShare(replacement, original.ID, now.Add(time.Minute))
	storage.path = originalPath
	if createErr == nil {
		t.Fatal("file share replacement unexpectedly survived an atomic write failure")
	}
	if got, err := storage.FileShareByToken(originalToken, now.Add(time.Minute)); err != nil || got.ID != original.ID {
		t.Fatalf("failed replacement changed original share: %#v, err=%v", got, err)
	}
	if _, err := storage.FileShareByToken(replacementToken, now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed replacement remained in memory: %v", err)
	}

	storage.path = filepath.Join(t.TempDir(), "missing", "state.json")
	_, deleteErr := storage.DeleteFileShare(original.ID)
	storage.path = originalPath
	if deleteErr == nil {
		t.Fatal("file share deletion unexpectedly survived an atomic write failure")
	}
	if got, err := storage.FileShareByToken(originalToken, now.Add(time.Minute)); err != nil || got.ID != original.ID {
		t.Fatalf("failed deletion changed in-memory share: %#v, err=%v", got, err)
	}
}

func TestFileShareCreateUsesExpectedIDAsCAS(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	original := testFileShare("share-original", "token-original", "/shared.txt", "version-original", now, nil)
	if _, _, err := storage.CreateFileShare(original, "", now); err != nil {
		t.Fatal(err)
	}

	candidates := []struct {
		share FileShare
		token string
	}{
		{testFileShare("share-a", "token-a", original.Path, "version-a", now.Add(time.Minute), nil), "token-a"},
		{testFileShare("share-b", "token-b", original.Path, "version-b", now.Add(time.Minute), nil), "token-b"},
	}
	start := make(chan struct{})
	errorsByCandidate := make([]error, len(candidates))
	var wait sync.WaitGroup
	for index := range candidates {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, _, errorsByCandidate[index] = storage.CreateFileShare(
				candidates[index].share, original.ID, now.Add(time.Minute),
			)
		}(index)
	}
	close(start)
	wait.Wait()

	succeeded := 0
	conflicted := 0
	for _, createErr := range errorsByCandidate {
		switch {
		case createErr == nil:
			succeeded++
		case errors.Is(createErr, ErrConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent create error: %v", createErr)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent CAS results: success=%d conflict=%d errors=%v", succeeded, conflicted, errorsByCandidate)
	}
	activeTokens := 0
	for _, candidate := range candidates {
		if _, err := storage.FileShareByToken(candidate.token, now.Add(time.Minute)); err == nil {
			activeTokens++
		}
	}
	if activeTokens != 1 {
		t.Fatalf("active concurrent tokens = %d, want 1", activeTokens)
	}
}

func TestFileShareStaleReplacementUsesStrongShareVersion(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	current := testFileShare("current", "token-current", "/shared.txt", "same-metadata", now, nil)
	if _, _, err := storage.CreateFileShare(current, "", now); err != nil {
		t.Fatal(err)
	}

	replacement := testFileShare("replacement", "token-replacement", current.Path, "same-metadata", now.Add(time.Minute), nil)
	replacement.ResourceVersion = current.ResourceVersion
	strongDigest := sha256.Sum256([]byte("different-inode-and-ctime"))
	replacement.ShareVersion = "sha256:" + fmt.Sprintf("%x", strongDigest[:])
	created, replaced, err := storage.CreateFileShare(replacement, "", now.Add(time.Minute))
	if err != nil || created.ID != replacement.ID || replaced.ID != current.ID {
		t.Fatalf("strong-version stale replacement created=%#v replaced=%#v err=%v", created, replaced, err)
	}

	duplicate := testFileShare("duplicate", "token-duplicate", current.Path, "same-metadata", now.Add(2*time.Minute), nil)
	duplicate.ResourceVersion = replacement.ResourceVersion
	duplicate.ShareVersion = replacement.ShareVersion
	if _, _, err := storage.CreateFileShare(duplicate, "", now.Add(2*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("same strong version without expected ID error=%v, want ErrConflict", err)
	}
}

func TestFileShareCreateRejectsInvalidAndExpiredRecords(t *testing.T) {
	storage, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)

	invalid := testFileShare("invalid", "invalid-token", "/invalid.txt", "invalid-version", now, nil)
	invalid.ID = "not-an-id"
	if _, _, err := storage.CreateFileShare(invalid, "", now); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("invalid record error = %v, want ErrInvalidRecord", err)
	}
	expiredAt := now
	expired := testFileShare("expired", "expired-token", "/expired.txt", "expired-version", now.Add(-time.Hour), &expiredAt)
	if _, _, err := storage.CreateFileShare(expired, "", now); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("expired record error = %v, want ErrInvalidRecord", err)
	}
	if _, err := storage.FileShareByPath("/invalid.txt", "", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid create changed state: %v", err)
	}
}

func TestFileShareRecoveryRejectsInvalidStateAndAcceptsLegacyState(t *testing.T) {
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	validOne := testFileShare("one", "token-one", "/one.txt", "version-one", now, nil)
	validTwo := testFileShare("two", "token-two", "/two.txt", "version-two", now, nil)
	duplicatePath := validTwo
	duplicatePath.Path = validOne.Path
	duplicateID := validTwo
	duplicateID.ID = validOne.ID
	duplicateHash := validTwo
	duplicateHash.TokenHash = validOne.TokenHash
	malformedShareVersion := validTwo
	malformedShareVersion.ShareVersion = "sha256:" + strings.Repeat("g", 64)
	oversizedShare := validTwo
	oversizedShare.SizeBytes = contract.MaxFileShareBytes + 1
	overLimit := make([]FileShare, 0, MaxFileShares+1)
	for index := range MaxFileShares + 1 {
		overLimit = append(overLimit, testFileShare(
			fmt.Sprintf("share-%d", index), fmt.Sprintf("token-%d", index),
			fmt.Sprintf("/file-%d.txt", index), fmt.Sprintf("version-%d", index), now, nil,
		))
	}
	for name, values := range map[string][]FileShare{
		"invalid record":          {{ID: "invalid"}},
		"duplicate path":          {validOne, duplicatePath},
		"duplicate id":            {validOne, duplicateID},
		"duplicate hash":          {validOne, duplicateHash},
		"malformed share version": {malformedShareVersion},
		"oversized share":         {oversizedShare},
		"over limit":              overLimit,
	} {
		t.Run(name, func(t *testing.T) {
			storePath := filepath.Join(t.TempDir(), "state.json")
			content, err := json.Marshal(diskState{SchemaVersion: 1, FileShares: values})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(storePath, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(storePath); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("Open invalid file share state error = %v, want ErrInvalidRecord", err)
			}
		})
	}
	t.Run("missing share version", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "state.json")
		recordContent, err := json.Marshal(validOne)
		if err != nil {
			t.Fatal(err)
		}
		var record map[string]any
		if err := json.Unmarshal(recordContent, &record); err != nil {
			t.Fatal(err)
		}
		delete(record, "shareVersion")
		content, err := json.Marshal(map[string]any{
			"schemaVersion": 1,
			"fileShares":    []map[string]any{record},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(storePath, content, 0o600); err != nil {
			t.Fatal(err)
		}
		opened, openErr := Open(storePath)
		if opened != nil {
			_ = opened.Close()
		}
		if !errors.Is(openErr, ErrInvalidRecord) {
			t.Fatalf("Open state without shareVersion error = %v, want ErrInvalidRecord", openErr)
		}
	})

	legacyPath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(legacyPath, []byte(`{"schemaVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := Open(legacyPath)
	if err != nil {
		t.Fatalf("legacy state without fileShares was rejected: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
}

func testFileShare(
	id string,
	token string,
	filePath string,
	resourceVersion string,
	createdAt time.Time,
	expiresAt *time.Time,
) FileShare {
	digest := sha256.Sum256([]byte(token))
	idDigest := sha256.Sum256([]byte(id))
	resourceDigest := sha256.Sum256([]byte(resourceVersion))
	return FileShare{
		ID: base64.RawURLEncoding.EncodeToString(idDigest[:16]), TokenHash: fmt.Sprintf("%x", digest[:]), Path: filePath,
		ResourceVersion: "sha256:" + fmt.Sprintf("%x", resourceDigest[:]),
		ShareVersion:    "sha256:" + fmt.Sprintf("%x", sha256.Sum256([]byte("share:"+resourceVersion))),
		SizeBytes:       64,
		CreatedAt:       createdAt, ExpiresAt: expiresAt,
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
