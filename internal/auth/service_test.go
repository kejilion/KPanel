package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

func TestDefaultPasswordHashConcurrencyPreservesMemoryHeadroom(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := cap(service.hashSlots); got != 1 {
		t.Fatalf("default password hash concurrency = %d; want 1", got)
	}
}

func TestBootstrapLoginSessionAndLogout(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	tokenBytes, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Bootstrap(
		string(tokenBytes),
		"admin",
		"a-strong-password-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "bootstrap.token")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap token was not consumed: %v", err)
	}
	session, err := service.Authenticate(credentials.Token)
	if err != nil {
		t.Fatal(err)
	}
	if session.User.Username != "admin" {
		t.Fatalf("unexpected user: %#v", session.User)
	}
	if err := service.ValidateCSRF(session, credentials.CSRFToken); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateCSRF(session, "wrong"); !errors.Is(err, ErrInvalidCSRFToken) {
		t.Fatalf("expected CSRF failure, got %v", err)
	}
	if err := service.Logout(credentials.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(credentials.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected invalid session, got %v", err)
	}

	login, err := service.Login("127.0.0.1", "admin", "a-strong-password-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if login.User.ID != credentials.User.ID {
		t.Fatalf("login user changed: %#v", login.User)
	}
}

func TestLoginRateLimit(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(string(token), "admin", "a-strong-password-1"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := service.Login("192.0.2.1", "admin", "wrong", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected invalid credentials, got %v", err)
		}
	}
	if _, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	if _, err := service.Login("192.0.2.2", "admin", "a-strong-password-1", ""); err != nil {
		t.Fatalf("a different IP could not use valid credentials after one IP was limited: %v", err)
	}
}

func TestLoginAppliesHigherDistributedAccountLimit(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(string(token), "admin", "a-strong-password-1"); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 * accountFailureLimitMultiplier {
		ip := fmt.Sprintf("192.0.2.%d", attempt+1)
		if _, err := service.Login(ip, "admin", "wrong", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", attempt, err)
		}
	}
	if _, err := service.Login("198.51.100.1", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected distributed account rate limit, got %v", err)
	}
}

func TestLoginBoundsConcurrentPasswordHashes(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	if err := storage.CreateInitialAdmin(store.User{
		ID: "user-1", Username: "admin", PasswordHash: "stored",
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hasher := &gatedHasher{started: make(chan struct{}, 1), release: make(chan struct{})}
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath:  filepath.Join(directory, "bootstrap.token"),
		SessionTTL:          time.Hour,
		LoginWindow:         time.Minute,
		MaxLoginFailures:    5,
		MaxConcurrentHashes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstResult := make(chan error, 1)
	go func() {
		_, loginErr := service.Login("192.0.2.1", "admin", "wrong", "")
		firstResult <- loginErr
	}()
	select {
	case <-hasher.started:
	case <-time.After(time.Second):
		t.Fatal("first password verification did not start")
	}

	if _, err := service.Login("192.0.2.2", "admin", "wrong", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected concurrent hash limit, got %v", err)
	}
	close(hasher.release)
	if err := <-firstResult; !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unexpected first login result: %v", err)
	}
}

func TestBootstrapSerializesPasswordHashing(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	hasher := &bootstrapGatedHasher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	defer hasher.unblock()
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath:  filepath.Join(directory, "bootstrap.token"),
		SessionTTL:          time.Hour,
		LoginWindow:         time.Minute,
		MaxLoginFailures:    5,
		MaxConcurrentHashes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, bootstrapErr := service.Bootstrap(
				string(token),
				"admin",
				"a-strong-password-1",
			)
			results <- bootstrapErr
		}()
	}
	select {
	case <-hasher.started:
	case <-time.After(time.Second):
		t.Fatal("bootstrap password hash did not start")
	}
	time.Sleep(25 * time.Millisecond)
	if calls := hasher.calls(); calls != 2 {
		t.Fatalf("concurrent bootstrap started %d password hashes; want dummy + one bootstrap", calls)
	}
	hasher.unblock()

	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrBootstrapUnavailable)) ||
		(second == nil && errors.Is(first, ErrBootstrapUnavailable))) {
		t.Fatalf("unexpected concurrent bootstrap results: first=%v second=%v", first, second)
	}
}

func TestChangePasswordRevokesSessionsAndReplacesCredentials(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	hasher := testHasher(t)
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := service.Bootstrap(string(token), "admin", "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.ChangePassword(firstSession.User.ID, "wrong-current", "an-even-stronger-password-2"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("expected invalid current password, got %v", err)
	}
	if _, err := service.Authenticate(firstSession.Token); err != nil {
		t.Fatalf("failed change revoked a session: %v", err)
	}
	if err := service.ChangePassword(firstSession.User.ID, "a-strong-password-1", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected weak password rejection, got %v", err)
	}
	if err := service.ChangePassword(firstSession.User.ID, "a-strong-password-1", "a-strong-password-1"); !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("expected unchanged password rejection, got %v", err)
	}

	before, err := storage.UserByID(firstSession.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return before.UpdatedAt.Add(time.Minute) }
	if err := service.ChangePassword(firstSession.User.ID, "a-strong-password-1", "an-even-stronger-password-2"); err != nil {
		t.Fatal(err)
	}
	after, err := storage.UserByID(firstSession.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash == before.PasswordHash {
		t.Fatal("password hash did not change")
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt.Add(time.Minute)) {
		t.Fatalf("unexpected password update time: %v", after.UpdatedAt)
	}
	for _, credentials := range []Credentials{firstSession, secondSession} {
		if _, err := service.Authenticate(credentials.Token); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("old session remained valid: %v", err)
		}
	}
	if _, err := service.Login("192.0.2.2", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password remained valid: %v", err)
	}
	newSession, err := service.Login("192.0.2.2", "admin", "an-even-stronger-password-2", "")
	if err != nil {
		t.Fatalf("new password was rejected: %v", err)
	}
	if _, err := service.Authenticate(newSession.Token); err != nil {
		t.Fatalf("new session was invalid: %v", err)
	}
}

func TestRecoverPasswordReplacesCredentialsWithoutCurrentPassword(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Bootstrap(string(token), "admin", "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecoverPassword(credentials.User.ID, "short", false); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected weak password rejection, got %v", err)
	}
	if _, err := service.Authenticate(credentials.Token); err != nil {
		t.Fatalf("rejected recovery revoked the session: %v", err)
	}

	recovered, err := service.RecoverPassword(credentials.User.ID, "a-recovered-password-3", false)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Username != "admin" || recovered.TOTPEnabled {
		t.Fatalf("unexpected recovered user: %#v", recovered)
	}
	if _, err := service.Authenticate(credentials.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old session survived recovery: %v", err)
	}
	if _, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password remained valid: %v", err)
	}
	if _, err := service.Login("192.0.2.1", "admin", "a-recovered-password-3", ""); err != nil {
		t.Fatalf("recovered password was rejected: %v", err)
	}
	events, _ := storage.ListAudit(10, "")
	if len(events) != 1 || events[0].ActorType != "cli" || events[0].Action != "auth.password.recover" {
		t.Fatalf("unexpected recovery audit: %#v", events)
	}
}

func TestChangeUsernameRevokesSessionsAndKeepsPassword(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		SessionTTL:         time.Hour, LoginWindow: time.Minute, MaxLoginFailures: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Bootstrap(string(token), "admin", "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login("192.0.2.10", "admin", "a-strong-password-1", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.ChangeUsername(first.User.ID, "wrong-password", "operator"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("expected current password rejection, got %v", err)
	}
	if err := service.ChangeUsername(first.User.ID, "a-strong-password-1", "bad name"); !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("expected username validation, got %v", err)
	}
	if err := service.ChangeUsername(first.User.ID, "a-strong-password-1", "admin"); !errors.Is(err, ErrUsernameUnchanged) {
		t.Fatalf("expected unchanged username rejection, got %v", err)
	}
	if err := service.ChangeUsername(first.User.ID, "a-strong-password-1", "operator"); err != nil {
		t.Fatal(err)
	}
	for _, credentials := range []Credentials{first, second} {
		if _, err := service.Authenticate(credentials.Token); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("old session remained valid: %v", err)
		}
	}
	if _, err := service.Login("192.0.2.11", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old username remained valid: %v", err)
	}
	if _, err := service.Login("192.0.2.11", "operator", "a-strong-password-1", ""); err != nil {
		t.Fatalf("new username or existing password was rejected: %v", err)
	}
}

func TestChangePasswordRejectsInvalidInputs(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	hasher := testHasher(t)
	passwordHash, err := hasher.Hash("a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.CreateInitialAdmin(store.User{
		ID: "user-1", Username: "admin", PasswordHash: passwordHash,
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		MaxLoginFailures:   5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.ChangePassword("user-1", "", "an-even-stronger-password-2"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("empty current password returned %v", err)
	}
	if err := service.ChangePassword("user-1", "a-strong-password-1", string(make([]byte, 257))); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("oversized new password returned %v", err)
	}
	if err := service.ChangePassword("missing-user", "a-strong-password-1", "an-even-stronger-password-2"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("missing user returned %v", err)
	}
}

func TestConcurrentPasswordChangesDoNotOverwriteWinner(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	hasher := testHasher(t)
	oldHash, err := hasher.Hash("a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.CreateInitialAdmin(store.User{
		ID: "user-1", Username: "admin", PasswordHash: oldHash,
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath:  filepath.Join(directory, "bootstrap.token"),
		MaxLoginFailures:    5,
		MaxConcurrentHashes: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		password string
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, password := range []string{"first-new-password-6", "second-new-password-7"} {
		go func(newPassword string) {
			<-start
			results <- result{
				password: newPassword,
				err:      service.ChangePassword("user-1", "a-strong-password-1", newPassword),
			}
		}(password)
	}
	close(start)

	var winningPassword string
	var successes, rejected int
	for range 2 {
		item := <-results
		switch {
		case item.err == nil:
			successes++
			winningPassword = item.password
		case errors.Is(item.err, ErrInvalidCurrentPassword):
			rejected++
		default:
			t.Fatalf("unexpected password change result: %v", item.err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("got %d successes and %d rejected changes", successes, rejected)
	}
	user, err := storage.UserByID("user-1")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := hasher.Verify(winningPassword, user.PasswordHash)
	if err != nil || !valid {
		t.Fatalf("winning password was not stored: valid=%v err=%v", valid, err)
	}
}

func TestPasswordChangeRevokesSessionCreatedByInflightOldPasswordLogin(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	if err := storage.CreateInitialAdmin(store.User{
		ID: "user-1", Username: "admin", PasswordHash: "hash:a-strong-password-1",
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hasher := &inflightLoginHasher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer hasher.unblock()
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath:  filepath.Join(directory, "bootstrap.token"),
		SessionTTL:          time.Hour,
		LoginWindow:         time.Minute,
		MaxLoginFailures:    5,
		MaxConcurrentHashes: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	loginResult := make(chan struct {
		credentials Credentials
		err         error
	}, 1)
	go func() {
		credentials, loginErr := service.Login("192.0.2.1", "admin", "a-strong-password-1", "")
		loginResult <- struct {
			credentials Credentials
			err         error
		}{credentials: credentials, err: loginErr}
	}()
	select {
	case <-hasher.started:
	case <-time.After(time.Second):
		t.Fatal("old-password login verification did not start")
	}

	changeResult := make(chan error, 1)
	go func() {
		changeResult <- service.ChangePassword("user-1", "a-strong-password-1", "an-even-stronger-password-2")
	}()
	select {
	case err := <-changeResult:
		t.Fatalf("password change passed the in-flight login: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	hasher.unblock()

	login := <-loginResult
	if login.err != nil {
		t.Fatalf("in-flight login failed unexpectedly: %v", login.err)
	}
	if err := <-changeResult; err != nil {
		t.Fatalf("password change failed: %v", err)
	}
	if _, err := service.Authenticate(login.credentials.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("in-flight old-password login left a valid session: %v", err)
	}
}

func TestChangePasswordBoundsConcurrentPasswordHashes(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	now := time.Now().UTC()
	if err := storage.CreateInitialAdmin(store.User{
		ID: "user-1", Username: "admin", PasswordHash: "stored",
		Role: "admin", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	hasher := &gatedHasher{started: make(chan struct{}, 1), release: make(chan struct{})}
	service, err := NewService(storage, hasher, Config{
		BootstrapTokenPath:  filepath.Join(directory, "bootstrap.token"),
		MaxLoginFailures:    5,
		MaxConcurrentHashes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- service.ChangePassword("user-1", "a-strong-password-1", "an-even-stronger-password-2")
	}()
	select {
	case <-hasher.started:
	case <-time.After(time.Second):
		t.Fatal("first password verification did not start")
	}
	if err := service.ChangePassword("user-1", "a-strong-password-1", "another-strong-password-8"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected concurrent hash limit, got %v", err)
	}
	close(hasher.release)
	if err := <-firstResult; !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("unexpected first password change result: %v", err)
	}
}

type gatedHasher struct {
	started chan struct{}
	release chan struct{}
}

func (h *gatedHasher) Hash(string) (string, error) {
	return "dummy", nil
}

func (h *gatedHasher) Verify(string, string) (bool, error) {
	select {
	case h.started <- struct{}{}:
	default:
	}
	<-h.release
	return false, nil
}

type bootstrapGatedHasher struct {
	mu          sync.Mutex
	hashCalls   int
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func (h *bootstrapGatedHasher) Hash(string) (string, error) {
	h.mu.Lock()
	h.hashCalls++
	call := h.hashCalls
	h.mu.Unlock()
	if call == 1 {
		return "dummy-hash", nil
	}
	select {
	case h.started <- struct{}{}:
	default:
	}
	<-h.release
	return "bootstrap-hash", nil
}

func (h *bootstrapGatedHasher) Verify(string, string) (bool, error) {
	return false, nil
}

func (h *bootstrapGatedHasher) calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hashCalls
}

func (h *bootstrapGatedHasher) unblock() {
	h.releaseOnce.Do(func() { close(h.release) })
}

type inflightLoginHasher struct {
	mu          sync.Mutex
	verifyCalls int
	releaseOnce sync.Once
	started     chan struct{}
	release     chan struct{}
}

func (h *inflightLoginHasher) Hash(password string) (string, error) {
	return "hash:" + password, nil
}

func (h *inflightLoginHasher) Verify(password, encoded string) (bool, error) {
	if encoded == "hash:a-strong-password-1" {
		h.mu.Lock()
		h.verifyCalls++
		call := h.verifyCalls
		h.mu.Unlock()
		if call == 1 {
			close(h.started)
			<-h.release
		}
	}
	return encoded == "hash:"+password, nil
}

func (h *inflightLoginHasher) unblock() {
	h.releaseOnce.Do(func() { close(h.release) })
}

func TestValidatePasswordRequiresLettersAndDigits(t *testing.T) {
	for password, want := range map[string]bool{
		"only-letters-here": false,
		"123456789012":      false,
		"letters-and-1":     true,
		"UPPER-CASE-9x":     true,
		"short1":            false,
	} {
		if got := validatePassword(password) == nil; got != want {
			t.Errorf("validatePassword(%q) accepted=%v, want %v", password, got, want)
		}
	}
}
