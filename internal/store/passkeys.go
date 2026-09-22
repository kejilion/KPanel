package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxPasskeys               = 10
	MaxPasskeyCredentialBytes = 16 << 10
)

// PasskeyOrigin returns the persisted browser-facing origin and a stable
// version for optimistic updates. The origin is panel configuration, not a
// credential, so it is kept outside the per-user passkey records.
func (s *Store) PasskeyOrigin() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.PasskeyOrigin, PasskeyOriginResourceVersion(s.data.PasskeyOrigin)
}

// ReplacePasskeyOrigin persists a single canonical origin with compare-and-set
// semantics. Panel validation performs the full WebAuthn origin policy check;
// this layer still rejects unbounded/control values before touching disk.
func (s *Store) ReplacePasskeyOrigin(expectedResourceVersion, value string) error {
	if len(value) > 512 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return ErrInvalidRecord
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedResourceVersion != PasskeyOriginResourceVersion(s.data.PasskeyOrigin) {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.PasskeyOrigin = value
	if value != "" && s.data.SchemaVersion < 2 {
		s.data.SchemaVersion = 2
	}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func PasskeyOriginResourceVersion(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", digest[:])
}

// HasPasskeys reports whether changing the global RP would strand any stored
// credential, including credentials belonging to another panel user.
func (s *Store) HasPasskeys() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.data.Users {
		if len(user.Passkeys) > 0 {
			return true
		}
	}
	return false
}

// Passkey contains public credential material only. WebAuthn parsing and
// signature verification belong to auth; the store enforces bounded records.
type Passkey struct {
	ID         string          `json:"id"`
	RPID       string          `json:"rpId"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
	CreatedAt  time.Time       `json:"createdAt"`
	LastUsedAt *time.Time      `json:"lastUsedAt,omitempty"`
}

func clonePasskeys(values []Passkey) []Passkey {
	if values == nil {
		return nil
	}
	result := make([]Passkey, len(values))
	for i, value := range values {
		value.Credential = append(json.RawMessage(nil), value.Credential...)
		if value.LastUsedAt != nil {
			used := *value.LastUsedAt
			value.LastUsedAt = &used
		}
		result[i] = value
	}
	return result
}

func cloneUser(user User) User {
	user.Passkeys = clonePasskeys(user.Passkeys)
	user.TOTPRecoveryCodeHashes = append([]string(nil), user.TOTPRecoveryCodeHashes...)
	if user.TOTPEnabledAt != nil {
		enabled := *user.TOTPEnabledAt
		user.TOTPEnabledAt = &enabled
	}
	return user
}

func validatePasskeys(values []Passkey) error {
	if len(values) > MaxPasskeys {
		return ErrLimitReached
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if err := validatePasskey(value); err != nil {
			return err
		}
		if seen[value.ID] {
			return ErrAlreadyExists
		}
		seen[value.ID] = true
	}
	return nil
}

func validatePasskey(value Passkey) error {
	if len(value.ID) == 0 || len(value.ID) > 2048 {
		return ErrInvalidRecord
	}
	id, err := base64.RawURLEncoding.DecodeString(value.ID)
	if err != nil || len(id) == 0 || base64.RawURLEncoding.EncodeToString(id) != value.ID {
		return ErrInvalidRecord
	}
	if !validPasskeyRPID(value.RPID) || !utf8.ValidString(value.Name) || strings.TrimSpace(value.Name) == "" || utf8.RuneCountInString(value.Name) > 64 {
		return ErrInvalidRecord
	}
	for _, char := range value.Name {
		if unicode.IsControl(char) {
			return ErrInvalidRecord
		}
	}
	if len(value.Credential) == 0 || len(value.Credential) > MaxPasskeyCredentialBytes || !json.Valid(value.Credential) {
		return ErrInvalidRecord
	}
	// Reject null, arrays and primitives; a credential is an object.
	if bytes.TrimSpace(value.Credential)[0] != '{' || value.CreatedAt.IsZero() || (value.LastUsedAt != nil && value.LastUsedAt.Before(value.CreatedAt)) {
		return ErrInvalidRecord
	}
	return nil
}

func validPasskeyRPID(value string) bool {
	if value == "" || len(value) > 253 || value != strings.ToLower(value) || net.ParseIP(value) != nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func advanceCredentialVersion(user *User) error {
	if user.CredentialVersion == ^uint64(0) {
		return ErrLimitReached
	}
	user.CredentialVersion++
	return nil
}

// ReplaceUserPasskeys commits a management change only while the authenticated
// session and the reauthenticated credential snapshot still hold. Every session
// is revoked in the same persisted transition.
func (s *Store) ReplaceUserPasskeys(expected User, values []Passkey, sessionHash string, now time.Time) error {
	if err := validatePasskeys(values); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.userIndexLocked(expected.ID)
	if index < 0 {
		return ErrNotFound
	}
	current := s.data.Users[index]
	if current.CredentialVersion != expected.CredentialVersion || !secureStringEqual(current.PasswordHash, expected.PasswordHash) {
		return ErrConflict
	}
	validSession := false
	for _, session := range s.data.Sessions {
		if session.TokenHash == sessionHash && session.UserID == expected.ID && session.ExpiresAt.After(now) {
			validSession = true
			break
		}
	}
	if !validSession {
		return ErrConflict
	}
	// A management snapshot may predate a successful login without changing
	// CredentialVersion. Never overwrite the retained authenticator counter.
	for _, value := range values {
		for _, stored := range current.Passkeys {
			if value.ID == stored.ID && (value.RPID != stored.RPID || !value.CreatedAt.Equal(stored.CreatedAt) || !bytes.Equal(value.Credential, stored.Credential) || !sameOptionalTime(value.LastUsedAt, stored.LastUsedAt)) {
				return ErrConflict
			}
		}
	}
	if err := advanceCredentialVersion(&current); err != nil {
		return err
	}
	previous := cloneDiskState(s.data)
	current.Passkeys = clonePasskeys(values)
	current.UpdatedAt = now
	s.data.Users[index] = current
	if len(values) != 0 {
		s.data.SchemaVersion = 2
	}
	s.revokeUserSessionsLocked(expected.ID)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

// UpdateUserPasskey atomically persists the verified authenticator state and its
// new login session. A credential removal, reset or competing state update must
// win over a login that was verified using an older snapshot.
func (s *Store) UpdateUserPasskey(expected User, value Passkey, session Session) error {
	if err := validatePasskey(value); err != nil {
		return err
	}
	if session.UserID != expected.ID || session.TokenHash == "" || session.CSRFHash == "" || session.CreatedAt.IsZero() || !session.ExpiresAt.After(session.CreatedAt) || value.LastUsedAt == nil {
		return ErrInvalidRecord
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.userIndexLocked(expected.ID)
	if index < 0 {
		return ErrNotFound
	}
	current := s.data.Users[index]
	if current.CredentialVersion != expected.CredentialVersion || !secureStringEqual(current.PasswordHash, expected.PasswordHash) {
		return ErrConflict
	}
	credentialIndex := -1
	for i, stored := range current.Passkeys {
		if stored.ID != value.ID {
			continue
		}
		for _, old := range expected.Passkeys {
			if old.ID == stored.ID && old.RPID == stored.RPID && bytes.Equal(old.Credential, stored.Credential) && sameOptionalTime(old.LastUsedAt, stored.LastUsedAt) {
				credentialIndex = i
				break
			}
		}
		if stored.RPID != value.RPID || stored.Name != value.Name || !stored.CreatedAt.Equal(value.CreatedAt) || (stored.LastUsedAt != nil && value.LastUsedAt.Before(*stored.LastUsedAt)) {
			return ErrConflict
		}
		break
	}
	if credentialIndex < 0 {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Users[index].Passkeys[credentialIndex] = clonePasskeys([]Passkey{value})[0]
	s.putSessionLocked(session)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
