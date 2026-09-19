package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

var (
	ErrBootstrapUnavailable    = errors.New("bootstrap is unavailable")
	ErrInvalidBootstrapToken   = errors.New("invalid bootstrap token")
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrInvalidSession          = errors.New("invalid session")
	ErrInvalidCSRFToken        = errors.New("invalid csrf token")
	ErrRateLimited             = errors.New("too many login attempts")
	ErrInvalidUsername         = errors.New("username must contain 3-32 letters, numbers, dots, underscores, or hyphens")
	ErrWeakPassword            = errors.New("password must contain 12-256 bytes with at least one letter and one digit")
	ErrInvalidCurrentPassword  = errors.New("current password is invalid")
	ErrPasswordUnchanged       = errors.New("new password must differ from current password")
	ErrUsernameUnchanged       = errors.New("new username must differ from current username")
	ErrUsernameUnavailable     = errors.New("username is already in use")
	ErrTOTPRequired            = errors.New("two-factor authentication is required")
	ErrInvalidSecondFactor     = errors.New("invalid two-factor authentication code")
	ErrTOTPAlreadyEnabled      = errors.New("two-factor authentication is already enabled")
	ErrTOTPDisabled            = errors.New("two-factor authentication is not enabled")
	ErrTOTPEnrollmentExpired   = errors.New("two-factor enrollment expired")
	ErrSecondFactorUnavailable = errors.New("two-factor authentication is temporarily unavailable")
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$`)

const accountFailureLimitMultiplier = 10

type Config struct {
	BootstrapTokenPath  string
	TOTPKeyPath         string
	SessionTTL          time.Duration
	LoginWindow         time.Duration
	MaxLoginFailures    int
	MaxConcurrentHashes int
}

type Service struct {
	store        *store.Store
	hasher       PasswordHasher
	config       Config
	now          func() time.Time
	dummyHash    string
	bootstrapMu  sync.Mutex
	credentialMu sync.RWMutex
	loginMu      sync.Mutex
	pending      map[string]int
	hashSlots    chan struct{}
	enrollmentMu sync.Mutex
	enrollments  map[string]pendingTOTPEnrollment
}

type PublicUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	TOTPEnabled bool   `json:"totpEnabled"`
}

type Credentials struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	User      PublicUser
}

type Session struct {
	TokenHash string
	CSRFHash  string
	ExpiresAt time.Time
	User      PublicUser
}

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return ErrRateLimited.Error()
}

func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

func NewService(storage *store.Store, hasher PasswordHasher, config Config) (*Service, error) {
	if storage == nil {
		return nil, errors.New("auth store is required")
	}
	if hasher == nil {
		return nil, errors.New("password hasher is required")
	}
	if config.BootstrapTokenPath == "" {
		return nil, errors.New("bootstrap token path is required")
	}
	if config.TOTPKeyPath == "" {
		config.TOTPKeyPath = filepath.Join(filepath.Dir(config.BootstrapTokenPath), "totp-encryption.key")
	}
	if !filepath.IsAbs(config.TOTPKeyPath) {
		return nil, errors.New("TOTP key path must be absolute")
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 12 * time.Hour
	}
	if config.LoginWindow <= 0 {
		config.LoginWindow = 15 * time.Minute
	}
	if config.MaxLoginFailures <= 0 {
		config.MaxLoginFailures = 5
	}
	if config.MaxConcurrentHashes <= 0 {
		// Argon2id intentionally uses 64 MiB per verification. One default slot
		// keeps headroom inside the production 256 MiB cgroup during login bursts.
		config.MaxConcurrentHashes = 1
	}
	if config.MaxConcurrentHashes > 8 {
		return nil, errors.New("max concurrent password hashes must not exceed 8")
	}

	dummyHash, err := hasher.Hash("kejilion-panel-invalid-password")
	if err != nil {
		return nil, fmt.Errorf("prepare password verifier: %w", err)
	}
	return &Service{
		store:       storage,
		hasher:      hasher,
		config:      config,
		now:         func() time.Time { return time.Now().UTC() },
		dummyHash:   dummyHash,
		pending:     make(map[string]int),
		hashSlots:   make(chan struct{}, config.MaxConcurrentHashes),
		enrollments: make(map[string]pendingTOTPEnrollment),
	}, nil
}

func (s *Service) IsInitialized() bool {
	return s.store.IsInitialized()
}

// EnsureBootstrapToken creates a secret once without ever returning or logging
// its value. An installer with local filesystem access reads the 0600 file.
func (s *Service) EnsureBootstrapToken() error {
	path := s.config.BootstrapTokenPath
	if s.store.IsInitialized() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove obsolete bootstrap token: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create bootstrap token directory: %w", err)
	}
	if content, err := os.ReadFile(path); err == nil {
		if strings.TrimSpace(string(content)) == "" {
			return errors.New("bootstrap token file is empty")
		}
		return os.Chmod(path, 0o600)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read bootstrap token: %w", err)
	}

	token, err := randomToken(32)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create bootstrap token: %w", err)
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write bootstrap token: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync bootstrap token: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close bootstrap token: %w", err)
	}
	return nil
}

func (s *Service) Bootstrap(token, username, password string) (Credentials, error) {
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()

	if s.store.IsInitialized() {
		return Credentials{}, ErrBootstrapUnavailable
	}
	expected, err := os.ReadFile(s.config.BootstrapTokenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, ErrBootstrapUnavailable
		}
		return Credentials{}, fmt.Errorf("read bootstrap token: %w", err)
	}
	if !secureEqual(strings.TrimSpace(string(expected)), strings.TrimSpace(token)) {
		return Credentials{}, ErrInvalidBootstrapToken
	}
	if err := validateUsername(username); err != nil {
		return Credentials{}, err
	}
	if err := validatePassword(password); err != nil {
		return Credentials{}, err
	}

	// Fail fast like Login instead of queueing behind hash work.
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return Credentials{}, &RateLimitError{RetryAfter: time.Second}
	}
	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return Credentials{}, fmt.Errorf("hash password: %w", err)
	}
	now := s.now()
	userID, err := randomToken(18)
	if err != nil {
		return Credentials{}, err
	}
	user := store.User{
		ID:           userID,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "admin",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateInitialAdmin(user); err != nil {
		if errors.Is(err, store.ErrAlreadyInitialized) {
			return Credentials{}, ErrBootstrapUnavailable
		}
		return Credentials{}, err
	}
	s.consumeBootstrapToken()
	return s.createSession(user)
}

func (s *Service) Login(ip, username, password, secondFactor string) (Credentials, error) {
	now := s.now()
	ipKey, accountKey := loginKeys(ip, username)
	since := now.Add(-s.config.LoginWindow)
	if !s.reserveLogin(ipKey, accountKey, since) {
		return Credentials{}, &RateLimitError{RetryAfter: s.config.LoginWindow}
	}
	defer s.releaseLogin(ipKey, accountKey)
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return Credentials{}, &RateLimitError{RetryAfter: time.Second}
	}
	s.credentialMu.RLock()
	defer s.credentialMu.RUnlock()

	user, userErr := s.store.UserByUsername(username)
	hash := s.dummyHash
	if userErr == nil {
		hash = user.PasswordHash
	}
	passwordToVerify := password
	inputInvalid := len(password) < 1 || len(password) > 256 || len(username) > 64
	if inputInvalid {
		passwordToVerify = "invalid-input"
		hash = s.dummyHash
	}
	valid, verifyErr := s.hasher.Verify(passwordToVerify, hash)
	if inputInvalid || userErr != nil || verifyErr != nil || !valid {
		if err := s.recordLoginAttempt(ipKey, accountKey, now, false); err != nil {
			return Credentials{}, err
		}
		return Credentials{}, ErrInvalidCredentials
	}
	if user.TOTPSecret != "" {
		if strings.TrimSpace(secondFactor) == "" {
			return Credentials{}, ErrTOTPRequired
		}
		if err := s.verifyAndConsumeSecondFactor(user, secondFactor, now); err != nil {
			if errors.Is(err, ErrSecondFactorUnavailable) {
				return Credentials{}, err
			}
			if recordErr := s.recordLoginAttempt(ipKey, accountKey, now, false); recordErr != nil {
				return Credentials{}, recordErr
			}
			return Credentials{}, ErrInvalidSecondFactor
		}
	}
	if err := s.recordLoginAttempt(ipKey, accountKey, now, true); err != nil {
		return Credentials{}, err
	}
	return s.createSession(user)
}

func (s *Service) TOTPStatus(userID string) (TOTPStatus, error) {
	user, err := s.store.UserByID(userID)
	if err != nil {
		return TOTPStatus{}, err
	}
	return TOTPStatus{
		Enabled:                user.TOTPSecret != "",
		EnabledAt:              user.TOTPEnabledAt,
		RecoveryCodesRemaining: len(user.TOTPRecoveryCodeHashes),
	}, nil
}

func (s *Service) StartTOTPEnrollment(userID, currentPassword string) (TOTPEnrollment, error) {
	user, err := s.verifyCurrentPassword(userID, currentPassword)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if user.TOTPSecret != "" {
		return TOTPEnrollment{}, ErrTOTPAlreadyEnabled
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		return TOTPEnrollment{}, err
	}
	id, err := randomToken(18)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	now := s.now()
	expiresAt := now.Add(enrollmentTTL)
	s.enrollmentMu.Lock()
	defer s.enrollmentMu.Unlock()
	for key, pending := range s.enrollments {
		if !pending.expiresAt.After(now) || key == userID {
			delete(s.enrollments, key)
		}
	}
	if len(s.enrollments) >= maxPendingEnrollments {
		return TOTPEnrollment{}, &RateLimitError{RetryAfter: time.Minute}
	}
	s.enrollments[userID] = pendingTOTPEnrollment{id: id, secret: secret, expiresAt: expiresAt}
	return TOTPEnrollment{ID: id, Secret: secret, OTPAuthURI: buildOTPAuthURI(user.Username, secret), ExpiresAt: expiresAt}, nil
}

func (s *Service) ConfirmTOTPEnrollment(userID, enrollmentID, code string) ([]string, error) {
	if len(enrollmentID) < 1 || len(enrollmentID) > 128 {
		return nil, ErrTOTPEnrollmentExpired
	}
	if len(code) > 64 {
		return nil, ErrInvalidSecondFactor
	}
	s.enrollmentMu.Lock()
	pending, ok := s.enrollments[userID]
	if !ok || !secureEqual(pending.id, enrollmentID) || !pending.expiresAt.After(s.now()) {
		delete(s.enrollments, userID)
		s.enrollmentMu.Unlock()
		return nil, ErrTOTPEnrollmentExpired
	}
	step, valid := matchTOTPStep(pending.secret, strings.TrimSpace(code), s.now())
	if !valid {
		s.enrollmentMu.Unlock()
		return nil, ErrInvalidSecondFactor
	}
	delete(s.enrollments, userID)
	s.enrollmentMu.Unlock()

	encryptedSecret, err := sealTOTPSecret(s.config.TOTPKeyPath, pending.secret)
	if err != nil {
		return nil, fmt.Errorf("protect TOTP secret: %w", err)
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.store.EnableUserTOTP(userID, encryptedSecret, now, step, hashes); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, ErrTOTPAlreadyEnabled
		}
		return nil, err
	}
	return codes, nil
}

func (s *Service) RegenerateRecoveryCodes(userID, currentPassword, secondFactor string) ([]string, error) {
	user, err := s.verifyCurrentPassword(userID, currentPassword)
	if err != nil {
		return nil, err
	}
	if user.TOTPSecret == "" {
		return nil, ErrTOTPDisabled
	}
	if err := s.verifyAndConsumeSecondFactor(user, secondFactor, s.now()); err != nil {
		if errors.Is(err, ErrSecondFactorUnavailable) {
			return nil, err
		}
		return nil, ErrInvalidSecondFactor
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.store.ReplaceUserRecoveryCodes(userID, hashes, s.now()); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) DisableTOTP(userID, currentPassword, secondFactor string) error {
	user, err := s.verifyCurrentPassword(userID, currentPassword)
	if err != nil {
		return err
	}
	if user.TOTPSecret == "" {
		return ErrTOTPDisabled
	}
	if err := s.verifyAndConsumeSecondFactor(user, secondFactor, s.now()); err != nil {
		if errors.Is(err, ErrSecondFactorUnavailable) {
			return err
		}
		return ErrInvalidSecondFactor
	}
	if err := s.store.DisableUserTOTP(userID, s.now()); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ErrTOTPDisabled
		}
		return err
	}
	return nil
}

func (s *Service) verifyCurrentPassword(userID, currentPassword string) (store.User, error) {
	now := s.now()
	reauthKey := "reauth:" + userID
	if !s.reserveAuthenticationAttempt(reauthKey, now.Add(-s.config.LoginWindow), s.config.MaxLoginFailures) {
		return store.User{}, &RateLimitError{RetryAfter: s.config.LoginWindow}
	}
	defer s.releaseAuthenticationAttempt(reauthKey)
	record := func(success bool) error {
		return s.store.RecordLoginAttempt(store.LoginAttempt{Key: reauthKey, OccurredAt: now, Success: success}, now.Add(-s.config.LoginWindow))
	}
	if len(currentPassword) < 1 || len(currentPassword) > 256 {
		if err := record(false); err != nil {
			return store.User{}, err
		}
		return store.User{}, ErrInvalidCurrentPassword
	}
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return store.User{}, &RateLimitError{RetryAfter: time.Second}
	}
	user, err := s.store.UserByID(userID)
	if err != nil {
		if recordErr := record(false); recordErr != nil {
			return store.User{}, recordErr
		}
		return store.User{}, ErrInvalidCurrentPassword
	}
	valid, err := s.hasher.Verify(currentPassword, user.PasswordHash)
	if err != nil || !valid {
		if recordErr := record(false); recordErr != nil {
			return store.User{}, recordErr
		}
		return store.User{}, ErrInvalidCurrentPassword
	}
	if err := record(true); err != nil {
		return store.User{}, err
	}
	return user, nil
}

func (s *Service) reserveAuthenticationAttempt(key string, since time.Time, limit int) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.store.FailedLoginCount(key, since)+s.pending[key] >= limit {
		return false
	}
	s.pending[key]++
	return true
}

func (s *Service) releaseAuthenticationAttempt(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.pending[key] <= 1 {
		delete(s.pending, key)
		return
	}
	s.pending[key]--
}

func (s *Service) verifyAndConsumeSecondFactor(user store.User, value string, now time.Time) error {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 64 {
		return ErrInvalidSecondFactor
	}
	if recovery := normalizeRecoveryCode(value); recoveryCodePattern.MatchString(recovery) {
		return s.store.ConsumeUserRecoveryCode(user.ID, recoveryCodeHash(recovery), now)
	}
	secret, err := openTOTPSecret(s.config.TOTPKeyPath, user.TOTPSecret)
	if err != nil {
		return fmt.Errorf("%w: protected TOTP secret cannot be opened", ErrSecondFactorUnavailable)
	}
	step, valid := matchTOTPStep(secret, value, now)
	if !valid {
		return ErrInvalidSecondFactor
	}
	return s.store.ConsumeUserTOTPStep(user.ID, user.TOTPSecret, step, now)
}

func (s *Service) reserveLogin(ipKey, accountKey string, since time.Time) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.store.FailedLoginCount(ipKey, since)+s.pending[ipKey] >= s.config.MaxLoginFailures {
		return false
	}
	accountLimit := s.config.MaxLoginFailures * accountFailureLimitMultiplier
	if s.store.FailedLoginCount(accountKey, since)+s.pending[accountKey] >= accountLimit {
		return false
	}
	s.pending[ipKey]++
	s.pending[accountKey]++
	return true
}

func (s *Service) releaseLogin(ipKey, accountKey string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	for _, key := range []string{ipKey, accountKey} {
		if s.pending[key] <= 1 {
			delete(s.pending, key)
		} else {
			s.pending[key]--
		}
	}
}

func (s *Service) Authenticate(token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidSession
	}
	tokenHash := hashSecret(token)
	session, err := s.store.SessionByTokenHash(tokenHash, s.now())
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	user, err := s.store.UserByID(session.UserID)
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	return Session{
		TokenHash: tokenHash,
		CSRFHash:  session.CSRFHash,
		ExpiresAt: session.ExpiresAt,
		User:      publicUser(user),
	}, nil
}

func (s *Service) ValidateCSRF(session Session, token string) error {
	if token == "" || !secureEqual(session.CSRFHash, hashSecret(token)) {
		return ErrInvalidCSRFToken
	}
	return nil
}

func (s *Service) Logout(sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	return s.store.DeleteSession(hashSecret(sessionToken))
}

// ChangePassword verifies and replaces a user's password while preventing an
// in-flight login with the old password from creating a session after the
// replacement. The store performs the hash update and session revocation as one
// persisted state transition.
func (s *Service) ChangePassword(userID, currentPassword, newPassword string) error {
	if len(currentPassword) < 1 || len(currentPassword) > 256 {
		return ErrInvalidCurrentPassword
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return &RateLimitError{RetryAfter: time.Second}
	}

	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	user, err := s.store.UserByID(userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCurrentPassword
		}
		return fmt.Errorf("read user for password change: %w", err)
	}
	valid, err := s.hasher.Verify(currentPassword, user.PasswordHash)
	if err != nil || !valid {
		return ErrInvalidCurrentPassword
	}
	if secureEqual(currentPassword, newPassword) {
		return ErrPasswordUnchanged
	}
	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := s.store.ReplaceUserPassword(user.ID, user.PasswordHash, newHash, s.now()); err != nil {
		if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCurrentPassword
		}
		return fmt.Errorf("replace password: %w", err)
	}
	return nil
}

// RecoverPassword replaces an administrator password after local host access
// has already established authority. The store commits the credential change,
// session revocation, account lockout cleanup, optional TOTP removal, and audit
// event atomically. This method is intentionally not exposed through HTTP.
func (s *Service) RecoverPassword(userID, newPassword string, disableTOTP bool) (PublicUser, error) {
	if err := validatePassword(newPassword); err != nil {
		return PublicUser{}, err
	}

	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return PublicUser{}, &RateLimitError{RetryAfter: time.Second}
	}

	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	user, err := s.store.UserByID(userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return PublicUser{}, ErrInvalidCredentials
		}
		return PublicUser{}, fmt.Errorf("read user for password recovery: %w", err)
	}
	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return PublicUser{}, fmt.Errorf("hash recovered password: %w", err)
	}
	eventID, err := randomToken(18)
	if err != nil {
		return PublicUser{}, fmt.Errorf("create password recovery audit ID: %w", err)
	}
	now := s.now()
	change := map[string]any{"twoFactorDisabled": disableTOTP}
	if err := s.store.RecoverUserPassword(store.PasswordRecovery{
		UserID: user.ID, ExpectedHash: user.PasswordHash, NewHash: newHash,
		DisableTOTP: disableTOTP, UpdatedAt: now,
		AuditEvent: store.AuditEvent{
			ID: eventID, OccurredAt: now, ActorType: "cli", Action: "auth.password.recover",
			TargetKind: "user", TargetID: user.ID, Result: "success", Change: change,
		},
		MaxAuditEntries: store.MaxAuditEntries,
	}); err != nil {
		if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
			return PublicUser{}, ErrInvalidCredentials
		}
		return PublicUser{}, fmt.Errorf("recover password: %w", err)
	}
	if disableTOTP {
		user.TOTPSecret = ""
		user.TOTPEnabledAt = nil
		user.TOTPLastUsedStep = 0
		user.TOTPRecoveryCodeHashes = nil
	}
	return publicUser(user), nil
}

// ChangeUsername verifies the current password before changing the login name.
// The store updates the identity and revokes all sessions atomically so no
// session continues to expose stale account data.
func (s *Service) ChangeUsername(userID, currentPassword, newUsername string) error {
	if len(currentPassword) < 1 || len(currentPassword) > 256 {
		return ErrInvalidCurrentPassword
	}
	if err := validateUsername(newUsername); err != nil {
		return err
	}

	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return &RateLimitError{RetryAfter: time.Second}
	}

	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()

	user, err := s.store.UserByID(userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidCurrentPassword
		}
		return fmt.Errorf("read user for username change: %w", err)
	}
	valid, err := s.hasher.Verify(currentPassword, user.PasswordHash)
	if err != nil || !valid {
		return ErrInvalidCurrentPassword
	}
	if user.Username == newUsername {
		return ErrUsernameUnchanged
	}
	if err := s.store.ReplaceUserUsername(user.ID, user.Username, newUsername, s.now()); err != nil {
		switch {
		case errors.Is(err, store.ErrAlreadyExists):
			return ErrUsernameUnavailable
		case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrNotFound):
			return ErrInvalidCurrentPassword
		default:
			return fmt.Errorf("replace username: %w", err)
		}
	}
	return nil
}

func (s *Service) createSession(user store.User) (Credentials, error) {
	token, err := randomToken(32)
	if err != nil {
		return Credentials{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return Credentials{}, err
	}
	now := s.now()
	expiresAt := now.Add(s.config.SessionTTL)
	if err := s.store.PutSession(store.Session{
		TokenHash: hashSecret(token),
		CSRFHash:  hashSecret(csrfToken),
		UserID:    user.ID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		return Credentials{}, err
	}
	return Credentials{
		Token:     token,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
		User:      publicUser(user),
	}, nil
}

func (s *Service) recordLoginAttempt(ipKey, accountKey string, now time.Time, success bool) error {
	retainSince := now.Add(-s.config.LoginWindow)
	return s.store.RecordLoginAttempts([]store.LoginAttempt{
		{Key: ipKey, OccurredAt: now, Success: success},
		{Key: accountKey, OccurredAt: now, Success: success},
	}, retainSince)
}

func (s *Service) consumeBootstrapToken() {
	if err := os.Remove(s.config.BootstrapTokenPath); err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	_ = os.WriteFile(s.config.BootstrapTokenPath, nil, 0o600)
}

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return ErrInvalidUsername
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 12 || len(password) > 256 {
		return ErrWeakPassword
	}
	// The same composition rule the setup and settings pages show; enforcing
	// it here keeps API and CLI callers from bypassing the UI check.
	hasLetter, hasDigit := false, false
	for _, character := range password {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z':
			hasLetter = true
		case character >= '0' && character <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

func loginKeys(ip, username string) (string, string) {
	ip = strings.TrimSpace(strings.ToLower(ip))
	if len(ip) > 128 {
		ip = ip[:128]
	}
	username = strings.TrimSpace(strings.ToLower(username))
	if len(username) > 64 {
		username = username[:64]
	}
	return "ip:" + ip, "account:" + username
}

func publicUser(user store.User) PublicUser {
	return PublicUser{ID: user.ID, Username: user.Username, Role: user.Role, TOTPEnabled: user.TOTPSecret != ""}
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func secureEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}
