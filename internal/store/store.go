package store

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxStoreBytes int64 = 32 << 20
	MaxFileShares       = 256
)

var (
	ErrAlreadyInitialized = errors.New("store already initialized")
	ErrConflict           = errors.New("record changed")
	ErrAlreadyExists      = errors.New("record already exists")
	ErrNotFound           = errors.New("record not found")
	ErrLimitReached       = errors.New("record limit reached")
	ErrInvalidRecord      = errors.New("invalid record")
	ErrStoreLocked        = errors.New("store is already open by another process")
)

type processLock interface {
	Close() error
}

type User struct {
	ID                     string     `json:"id"`
	Username               string     `json:"username"`
	PasswordHash           string     `json:"passwordHash"`
	Role                   string     `json:"role"`
	TOTPSecret             string     `json:"totpSecret,omitempty"`
	TOTPEnabledAt          *time.Time `json:"totpEnabledAt,omitempty"`
	TOTPLastUsedStep       int64      `json:"totpLastUsedStep,omitempty"`
	TOTPRecoveryCodeHashes []string   `json:"totpRecoveryCodeHashes,omitempty"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type Session struct {
	TokenHash string    `json:"tokenHash"`
	CSRFHash  string    `json:"csrfHash"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	OccurredAt time.Time      `json:"occurredAt"`
	ActorType  string         `json:"actorType"`
	ActorID    string         `json:"actorId,omitempty"`
	SourceIP   string         `json:"sourceIp,omitempty"`
	Action     string         `json:"action"`
	TargetKind string         `json:"targetKind,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	Result     string         `json:"result"`
	RequestID  string         `json:"requestId"`
	Change     map[string]any `json:"change,omitempty"`
}

type LoginAttempt struct {
	Key        string    `json:"key"`
	OccurredAt time.Time `json:"occurredAt"`
	Success    bool      `json:"success"`
}

type SecurityEntrance struct {
	Enabled   bool      `json:"enabled"`
	Path      string    `json:"path,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// ClusterShare stores the opt-in public cluster page configuration. Token is
// deliberately kept out of audit events and can be rotated to invalidate an
// existing link without changing the cluster inventory.
type ClusterShare struct {
	Enabled     bool      `json:"enabled"`
	Token       string    `json:"token,omitempty"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// FileShare is a bounded, revocable authorization for one exact filesystem
// resource. Path remains an Agent-owned fact. ResourceVersion preserves normal
// filemanager concurrency semantics while ShareVersion adds the Agent's strong
// Linux object identity for anonymous access. Only the bearer token digest is
// persisted; the token itself must never enter state or audit records.
type FileShare struct {
	ID              string     `json:"id"`
	TokenHash       string     `json:"tokenHash"`
	Path            string     `json:"path"`
	ResourceVersion string     `json:"resourceVersion"`
	ShareVersion    string     `json:"shareVersion"`
	CreatedAt       time.Time  `json:"createdAt"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
}

type PasswordRecovery struct {
	UserID          string
	ExpectedHash    string
	NewHash         string
	DisableTOTP     bool
	UpdatedAt       time.Time
	AuditEvent      AuditEvent
	MaxAuditEntries int
}

type diskState struct {
	SchemaVersion    int              `json:"schemaVersion"`
	Users            []User           `json:"users"`
	Sessions         []Session        `json:"sessions"`
	Audit            []AuditEvent     `json:"audit"`
	LoginAttempts    []LoginAttempt   `json:"loginAttempts"`
	SecurityEntrance SecurityEntrance `json:"securityEntrance,omitempty"`
	ClusterShare     ClusterShare     `json:"clusterShare,omitempty"`
	FileShares       []FileShare      `json:"fileShares,omitempty"`
}

// Store is a small, single-node persistence layer. It deliberately stores only
// panel identity/session/audit and panel-local security settings; host resources
// remain owned by the Agent.
type Store struct {
	mu            sync.RWMutex
	path          string
	data          diskState
	processLock   processLock
	syncDirectory func(string) error
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	lock, err := acquireProcessLock(path + ".lock")
	if err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			_ = lock.Close()
		}
	}()

	s := &Store{
		path:          path,
		data:          diskState{SchemaVersion: 1},
		processLock:   lock,
		syncDirectory: syncDirectory,
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxStoreBytes {
		return nil, fmt.Errorf("store file exceeds %d bytes", maxStoreBytes)
	}
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(content) == 0 {
			return nil, errors.New("store file is empty")
		}
		if err := json.Unmarshal(content, &s.data); err != nil {
			return nil, fmt.Errorf("decode store: %w", err)
		}
		if s.data.SchemaVersion != 1 {
			return nil, fmt.Errorf("unsupported store schema version %d", s.data.SchemaVersion)
		}
		if err := validateFileShares(s.data.FileShares); err != nil {
			return nil, fmt.Errorf("validate file shares: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read store: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect store: %w", err)
	}

	opened = true
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.processLock == nil {
		return nil
	}
	err := s.processLock.Close()
	s.processLock = nil
	return err
}

func (s *Store) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Users) > 0
}

func (s *Store) CreateInitialAdmin(user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data.Users) != 0 {
		return ErrAlreadyInitialized
	}
	previous := cloneDiskState(s.data)
	s.data.Users = append(s.data.Users, user)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) UserByUsername(username string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.data.Users {
		if strings.EqualFold(user.Username, username) {
			return user, nil
		}
	}
	return User{}, ErrNotFound
}

func (s *Store) UserByID(id string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.data.Users {
		if user.ID == id {
			return user, nil
		}
	}
	return User{}, ErrNotFound
}

// ReplaceUserPassword atomically updates a user's password hash and revokes all
// of their sessions. expectedHash prevents concurrent password changes from
// overwriting one another.
func (s *Store) ReplaceUserPassword(userID, expectedHash, newHash string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userIndex := -1
	for index := range s.data.Users {
		if s.data.Users[index].ID == userID {
			userIndex = index
			break
		}
	}
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].PasswordHash != expectedHash {
		return ErrConflict
	}

	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].PasswordHash = newHash
	s.data.Users[userIndex].UpdatedAt = updatedAt
	sessions := make([]Session, 0, len(s.data.Sessions))
	for _, session := range s.data.Sessions {
		if session.UserID != userID {
			sessions = append(sessions, session)
		}
	}
	s.data.Sessions = sessions
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

// RecoverUserPassword applies a local administrator recovery as one persisted
// transition. It keeps account identity and panel settings, revokes the user's
// sessions, clears authentication failures, and records the
// recovery before any success is reported to the caller.
func (s *Store) RecoverUserPassword(input PasswordRecovery) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userIndex := s.userIndexLocked(input.UserID)
	if userIndex < 0 {
		return ErrNotFound
	}
	user := s.data.Users[userIndex]
	if user.PasswordHash != input.ExpectedHash {
		return ErrConflict
	}

	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].PasswordHash = input.NewHash
	s.data.Users[userIndex].UpdatedAt = input.UpdatedAt
	if input.DisableTOTP {
		s.data.Users[userIndex].TOTPSecret = ""
		s.data.Users[userIndex].TOTPEnabledAt = nil
		s.data.Users[userIndex].TOTPLastUsedStep = 0
		s.data.Users[userIndex].TOTPRecoveryCodeHashes = nil
	}
	s.revokeUserSessionsLocked(user.ID)

	// A local recovery establishes a new credential boundary. Clear persisted
	// rate-limit history so the operator can immediately use the new password;
	// subsequent network failures are recorded and limited normally.
	s.data.LoginAttempts = nil

	s.data.Audit = append(s.data.Audit, input.AuditEvent)
	if input.MaxAuditEntries > 0 && len(s.data.Audit) > input.MaxAuditEntries {
		s.data.Audit = append([]AuditEvent(nil), s.data.Audit[len(s.data.Audit)-input.MaxAuditEntries:]...)
	}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) Usernames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	usernames := make([]string, 0, len(s.data.Users))
	for _, user := range s.data.Users {
		usernames = append(usernames, user.Username)
	}
	sort.Strings(usernames)
	return usernames
}

// ReplaceUserUsername atomically changes a user's sign-in name and revokes all
// of their sessions. Usernames are unique case-insensitively, matching login
// lookup semantics. expectedUsername prevents stale settings pages from
// overwriting a concurrent account change.
func (s *Store) ReplaceUserUsername(userID, expectedUsername, newUsername string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].Username != expectedUsername {
		return ErrConflict
	}
	for index, user := range s.data.Users {
		if index != userIndex && strings.EqualFold(user.Username, newUsername) {
			return ErrAlreadyExists
		}
	}

	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].Username = newUsername
	s.data.Users[userIndex].UpdatedAt = updatedAt
	s.revokeUserSessionsLocked(userID)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

// EnableUserTOTP persists the encrypted authenticator secret and one-time
// recovery hashes in the same atomic transition that revokes existing sessions.
func (s *Store) EnableUserTOTP(userID, encryptedSecret string, enabledAt time.Time, lastUsedStep int64, recoveryHashes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].TOTPSecret != "" {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].TOTPSecret = encryptedSecret
	s.data.Users[userIndex].TOTPEnabledAt = &enabledAt
	s.data.Users[userIndex].TOTPLastUsedStep = lastUsedStep
	s.data.Users[userIndex].TOTPRecoveryCodeHashes = append([]string(nil), recoveryHashes...)
	s.data.Users[userIndex].UpdatedAt = enabledAt
	s.revokeUserSessionsLocked(userID)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

// ConsumeUserTOTPStep prevents reuse of an authenticator code after successful
// verification, including concurrent login attempts that matched the same step.
func (s *Store) ConsumeUserTOTPStep(userID, expectedSecret string, step int64, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].TOTPSecret == "" ||
		!secureStringEqual(s.data.Users[userIndex].TOTPSecret, expectedSecret) ||
		step <= s.data.Users[userIndex].TOTPLastUsedStep {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].TOTPLastUsedStep = step
	s.data.Users[userIndex].UpdatedAt = updatedAt
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func secureStringEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

// ConsumeUserRecoveryCode removes a matching hash exactly once. Comparison is
// constant-time for each stored value so the plaintext code never needs storage.
func (s *Store) ConsumeUserRecoveryCode(userID, codeHash string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	hashes := s.data.Users[userIndex].TOTPRecoveryCodeHashes
	matched := -1
	for index, candidate := range hashes {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(codeHash)) == 1 {
			matched = index
		}
	}
	if matched < 0 {
		return ErrNotFound
	}
	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].TOTPRecoveryCodeHashes = append(append([]string(nil), hashes[:matched]...), hashes[matched+1:]...)
	s.data.Users[userIndex].UpdatedAt = updatedAt
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) ReplaceUserRecoveryCodes(userID string, recoveryHashes []string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].TOTPSecret == "" {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].TOTPRecoveryCodeHashes = append([]string(nil), recoveryHashes...)
	s.data.Users[userIndex].UpdatedAt = updatedAt
	s.revokeUserSessionsLocked(userID)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) DisableUserTOTP(userID string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	userIndex := s.userIndexLocked(userID)
	if userIndex < 0 {
		return ErrNotFound
	}
	if s.data.Users[userIndex].TOTPSecret == "" {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Users[userIndex].TOTPSecret = ""
	s.data.Users[userIndex].TOTPEnabledAt = nil
	s.data.Users[userIndex].TOTPLastUsedStep = 0
	s.data.Users[userIndex].TOTPRecoveryCodeHashes = nil
	s.data.Users[userIndex].UpdatedAt = updatedAt
	s.revokeUserSessionsLocked(userID)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) userIndexLocked(userID string) int {
	for index := range s.data.Users {
		if s.data.Users[index].ID == userID {
			return index
		}
	}
	return -1
}

func (s *Store) revokeUserSessionsLocked(userID string) {
	sessions := make([]Session, 0, len(s.data.Sessions))
	for _, session := range s.data.Sessions {
		if session.UserID != userID {
			sessions = append(sessions, session)
		}
	}
	s.data.Sessions = sessions
}

func (s *Store) PutSession(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneDiskState(s.data)
	filtered := s.data.Sessions[:0]
	for _, item := range s.data.Sessions {
		if item.TokenHash != session.TokenHash && item.ExpiresAt.After(time.Now().UTC()) {
			filtered = append(filtered, item)
		}
	}
	s.data.Sessions = append(filtered, session)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) SessionByTokenHash(tokenHash string, now time.Time) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, session := range s.data.Sessions {
		if session.TokenHash == tokenHash && session.ExpiresAt.After(now) {
			return session, nil
		}
	}
	return Session{}, ErrNotFound
}

func (s *Store) DeleteSession(tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneDiskState(s.data)
	filtered := s.data.Sessions[:0]
	found := false
	for _, session := range s.data.Sessions {
		if session.TokenHash == tokenHash {
			found = true
			continue
		}
		filtered = append(filtered, session)
	}
	s.data.Sessions = filtered
	if !found {
		return nil
	}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) AppendAudit(event AuditEvent, maxEntries int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneDiskState(s.data)
	s.data.Audit = append(s.data.Audit, event)
	if maxEntries > 0 && len(s.data.Audit) > maxEntries {
		s.data.Audit = append([]AuditEvent(nil), s.data.Audit[len(s.data.Audit)-maxEntries:]...)
	}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

// ListAudit returns newest-first records. Cursor is the last event ID received.
func (s *Store) ListAudit(limit int, cursor string) ([]AuditEvent, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	items := append([]AuditEvent(nil), s.data.Audit...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})

	start := 0
	if cursor != "" {
		for i, event := range items {
			if event.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	if start >= len(items) {
		return []AuditEvent{}, ""
	}
	end := min(start+limit, len(items))
	next := ""
	if end < len(items) {
		next = items[end-1].ID
	}
	return items[start:end], next
}

func (s *Store) RecordLoginAttempt(attempt LoginAttempt, retainSince time.Time) error {
	return s.RecordLoginAttempts([]LoginAttempt{attempt}, retainSince)
}

func (s *Store) RecordLoginAttempts(attempts []LoginAttempt, retainSince time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneDiskState(s.data)
	filtered := s.data.LoginAttempts[:0]
	for _, item := range s.data.LoginAttempts {
		if item.OccurredAt.After(retainSince) {
			filtered = append(filtered, item)
		}
	}
	s.data.LoginAttempts = append(filtered, attempts...)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func (s *Store) FailedLoginCount(key string, since time.Time) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	latestSuccess := since
	for _, item := range s.data.LoginAttempts {
		if item.Key == key && item.Success && item.OccurredAt.After(latestSuccess) {
			latestSuccess = item.OccurredAt
		}
	}
	count := 0
	for _, item := range s.data.LoginAttempts {
		if item.Key == key && !item.Success && item.OccurredAt.After(latestSuccess) {
			count++
		}
	}
	return count
}

func (s *Store) SecurityEntrance() (SecurityEntrance, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value := s.data.SecurityEntrance
	return value, SecurityEntranceResourceVersion(value)
}

func (s *Store) ReplaceSecurityEntrance(expectedResourceVersion string, value SecurityEntrance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedResourceVersion != SecurityEntranceResourceVersion(s.data.SecurityEntrance) {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.SecurityEntrance = value
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func SecurityEntranceResourceVersion(value SecurityEntrance) string {
	payload := fmt.Sprintf("%t\x00%s\x00%s", value.Enabled, value.Path, value.UpdatedAt.UTC().Format(time.RFC3339Nano))
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func (s *Store) ClusterShare() (ClusterShare, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value := s.data.ClusterShare
	return value, ClusterShareResourceVersion(value)
}

func (s *Store) ReplaceClusterShare(expectedResourceVersion string, value ClusterShare) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedResourceVersion != ClusterShareResourceVersion(s.data.ClusterShare) {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.ClusterShare = value
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func ClusterShareResourceVersion(value ClusterShare) string {
	payload := fmt.Sprintf(
		"%t\x00%s\x00%s\x00%s\x00%s",
		value.Enabled,
		value.Token,
		value.Title,
		value.Description,
		value.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func (s *Store) FileShareByPath(filePath, resourceVersion string, now time.Time) (FileShare, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.data.FileShares {
		if value.Path == filePath && (resourceVersion == "" || value.ResourceVersion == resourceVersion) &&
			fileShareActive(value, now) {
			return cloneFileShare(value), nil
		}
	}
	return FileShare{}, ErrNotFound
}

func (s *Store) FileShareByToken(token string, now time.Time) (FileShare, error) {
	digest := sha256.Sum256([]byte(token))
	tokenHash := fmt.Sprintf("%x", digest[:])
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.data.FileShares {
		if subtle.ConstantTimeCompare([]byte(value.TokenHash), []byte(tokenHash)) == 1 && fileShareActive(value, now) {
			return cloneFileShare(value), nil
		}
	}
	return FileShare{}, ErrNotFound
}

func (s *Store) ListFileShares(now time.Time) []FileShare {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]FileShare, 0, len(s.data.FileShares))
	for _, value := range s.data.FileShares {
		if fileShareActive(value, now) {
			items = append(items, cloneFileShare(value))
		}
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].CreatedAt.Equal(items[right].CreatedAt) {
			return items[left].ID < items[right].ID
		}
		return items[left].CreatedAt.After(items[right].CreatedAt)
	})
	return items
}

// CreateFileShare atomically replaces any existing share for the same path.
// This lets the administrator regenerate a bearer URL without retaining the
// original token in persistent state.
func (s *Store) CreateFileShare(value FileShare, expectedID string, now time.Time) (FileShare, FileShare, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateFileShare(value); err != nil || (value.ExpiresAt != nil && !value.ExpiresAt.After(now)) {
		return FileShare{}, FileShare{}, ErrInvalidRecord
	}

	var currentForPath FileShare
	for _, current := range s.data.FileShares {
		if current.Path == value.Path && fileShareActive(current, now) {
			currentForPath = current
			break
		}
	}
	if (expectedID == "" && currentForPath.ID != "" && currentForPath.ShareVersion == value.ShareVersion) ||
		(expectedID != "" && currentForPath.ID != expectedID) {
		return FileShare{}, FileShare{}, ErrConflict
	}

	for _, current := range s.data.FileShares {
		if current.ID == value.ID || subtle.ConstantTimeCompare([]byte(current.TokenHash), []byte(value.TokenHash)) == 1 {
			return FileShare{}, FileShare{}, ErrAlreadyExists
		}
	}

	active := make([]FileShare, 0, len(s.data.FileShares)+1)
	var replaced FileShare
	for _, current := range s.data.FileShares {
		if !fileShareActive(current, now) {
			continue
		}
		if current.Path == value.Path {
			replaced = cloneFileShare(current)
			continue
		}
		active = append(active, cloneFileShare(current))
	}
	if len(active) >= MaxFileShares {
		return FileShare{}, FileShare{}, ErrLimitReached
	}

	previous := cloneDiskState(s.data)
	active = append(active, cloneFileShare(value))
	s.data.FileShares = active
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return FileShare{}, FileShare{}, err
	}
	return cloneFileShare(value), replaced, nil
}

func (s *Store) DeleteFileShare(id string) (FileShare, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, value := range s.data.FileShares {
		if value.ID != id {
			continue
		}
		previous := cloneDiskState(s.data)
		s.data.FileShares = append(
			append([]FileShare(nil), s.data.FileShares[:index]...),
			s.data.FileShares[index+1:]...,
		)
		if err := s.persistLocked(); err != nil {
			s.data = previous
			return FileShare{}, err
		}
		return cloneFileShare(value), nil
	}
	return FileShare{}, ErrNotFound
}

func fileShareActive(value FileShare, now time.Time) bool {
	return value.ExpiresAt == nil || value.ExpiresAt.After(now)
}

func cloneFileShare(value FileShare) FileShare {
	if value.ExpiresAt != nil {
		expiresAt := *value.ExpiresAt
		value.ExpiresAt = &expiresAt
	}
	return value
}

func validateFileShares(values []FileShare) error {
	if len(values) > MaxFileShares {
		return fmt.Errorf("%w: file share limit exceeded", ErrInvalidRecord)
	}
	ids := make(map[string]struct{}, len(values))
	tokenHashes := make(map[string]struct{}, len(values))
	paths := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := validateFileShare(value); err != nil {
			return fmt.Errorf("%w: file share %d is invalid", ErrInvalidRecord, index)
		}
		if _, exists := ids[value.ID]; exists {
			return fmt.Errorf("%w: duplicate file share id", ErrInvalidRecord)
		}
		if _, exists := tokenHashes[value.TokenHash]; exists {
			return fmt.Errorf("%w: duplicate file share token hash", ErrInvalidRecord)
		}
		if _, exists := paths[value.Path]; exists {
			return fmt.Errorf("%w: duplicate file share path", ErrInvalidRecord)
		}
		ids[value.ID] = struct{}{}
		tokenHashes[value.TokenHash] = struct{}{}
		paths[value.Path] = struct{}{}
	}
	return nil
}

func validateFileShare(value FileShare) error {
	id, err := base64.RawURLEncoding.DecodeString(value.ID)
	if err != nil || len(value.ID) != 22 || len(id) != 16 ||
		base64.RawURLEncoding.EncodeToString(id) != value.ID {
		return ErrInvalidRecord
	}
	if len(value.TokenHash) != 64 || value.TokenHash != strings.ToLower(value.TokenHash) {
		return ErrInvalidRecord
	}
	tokenHash, err := hex.DecodeString(value.TokenHash)
	if err != nil || len(tokenHash) != sha256.Size {
		return ErrInvalidRecord
	}
	if value.Path == "" || len(value.Path) > 4096 || !strings.HasPrefix(value.Path, "/") ||
		strings.ContainsAny(value.Path, "\\\x00") || path.Clean(value.Path) != value.Path {
		return ErrInvalidRecord
	}
	if len(value.ResourceVersion) != 71 || !strings.HasPrefix(value.ResourceVersion, "sha256:") {
		return ErrInvalidRecord
	}
	resourceDigest, err := hex.DecodeString(strings.TrimPrefix(value.ResourceVersion, "sha256:"))
	if err != nil || len(resourceDigest) != sha256.Size || value.ResourceVersion != strings.ToLower(value.ResourceVersion) {
		return ErrInvalidRecord
	}
	if len(value.ShareVersion) != 71 || !strings.HasPrefix(value.ShareVersion, "sha256:") {
		return ErrInvalidRecord
	}
	shareDigest, err := hex.DecodeString(strings.TrimPrefix(value.ShareVersion, "sha256:"))
	if err != nil || len(shareDigest) != sha256.Size || value.ShareVersion != strings.ToLower(value.ShareVersion) {
		return ErrInvalidRecord
	}
	if value.CreatedAt.IsZero() || (value.ExpiresAt != nil && !value.ExpiresAt.After(value.CreatedAt)) {
		return ErrInvalidRecord
	}
	return nil
}

func (s *Store) persistLocked() error {
	content, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}
	content = append(content, '\n')
	if int64(len(content)) > maxStoreBytes {
		return fmt.Errorf("encoded store exceeds %d bytes", maxStoreBytes)
	}

	dir := filepath.Dir(s.path)
	temp, err := os.CreateTemp(dir, ".panel-store-*")
	if err != nil {
		return fmt.Errorf("create temporary store: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect temporary store: %w", err)
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary store: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary store: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary store: %w", err)
	}

	// os.Rename atomically replaces the target on Linux, which is the production
	// platform. The backup fallback keeps local Windows development functional.
	if err := os.Rename(tempPath, s.path); err == nil {
		// The rename is the logical commit point. A directory fsync strengthens
		// crash durability, but failure must not make callers retry an operation
		// that is already visible in memory and on disk.
		_ = s.syncDirectory(dir)
		return nil
	}

	backupPath := s.path + ".previous"
	_ = os.Remove(backupPath)
	if err := os.Rename(s.path, backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("prepare store replacement: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		_ = os.Rename(backupPath, s.path)
		return fmt.Errorf("replace store: %w", err)
	}
	_ = os.Remove(backupPath)
	_ = s.syncDirectory(dir)
	return nil
}

func cloneDiskState(source diskState) diskState {
	users := append([]User(nil), source.Users...)
	for index := range users {
		users[index].TOTPRecoveryCodeHashes = append([]string(nil), users[index].TOTPRecoveryCodeHashes...)
		if users[index].TOTPEnabledAt != nil {
			enabledAt := *users[index].TOTPEnabledAt
			users[index].TOTPEnabledAt = &enabledAt
		}
	}
	return diskState{
		SchemaVersion:    source.SchemaVersion,
		Users:            users,
		Sessions:         append([]Session(nil), source.Sessions...),
		Audit:            append([]AuditEvent(nil), source.Audit...),
		LoginAttempts:    append([]LoginAttempt(nil), source.LoginAttempts...),
		SecurityEntrance: source.SecurityEntrance,
		ClusterShare:     source.ClusterShare,
		FileShares:       cloneFileShares(source.FileShares),
	}
}

func cloneFileShares(source []FileShare) []FileShare {
	result := make([]FileShare, len(source))
	for index, value := range source {
		result[index] = cloneFileShare(value)
	}
	return result
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open store directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync store directory: %w", err)
	}
	return nil
}
