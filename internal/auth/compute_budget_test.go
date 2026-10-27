package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

type computationSpy struct{ verifies int }

func (*computationSpy) Hash(s string) (string, error)         { return s, nil }
func (h *computationSpy) Verify(s, hash string) (bool, error) { h.verifies++; return s == hash, nil }

func TestLoginComputationBudgetCountsMissingTOTPWithoutCredentialFailures(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	const recovery = "AAAAA-BBBBB-CCCCC"
	if err := db.CreateInitialAdmin(store.User{ID: "admin", Username: "admin", PasswordHash: "correct-password-1", TOTPSecret: "protected", TOTPRecoveryCodeHashes: []string{recoveryCodeHash(recovery)}, Role: "admin", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	h := &computationSpy{}
	s, err := NewService(db, h, Config{BootstrapTokenPath: filepath.Join(dir, "bootstrap"), MaxLoginFailures: 2, LoginWindow: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	for i := 0; i < 8; i++ {
		_, err := s.Login("2001:db8::1", "admin", "correct-password-1", "")
		if i < 4 && !errors.Is(err, ErrTOTPRequired) {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if i >= 4 && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("attempt %d must be admitted before hashing: %v", i, err)
		}
	}
	if h.verifies != 4 {
		t.Fatalf("hash computations=%d; want 4", h.verifies)
	}
	ipKey, accountKey := loginKeys("2001:db8::1", "admin")
	if db.FailedLoginCount(ipKey, now.Add(-time.Minute)) != 0 || db.FailedLoginCount(accountKey, now.Add(-time.Minute)) != 0 {
		t.Fatal("missing TOTP must not become credential failures")
	}
	if _, err := s.Login("2001:db8::2", "missing", "password", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("IPv6 /64 bypassed computation budget: %v", err)
	}
	now = now.Add(time.Minute)
	s.config.MaxLoginFailures = 1
	if _, err := s.Login("2001:db8::1", "admin", "correct-password-1", ""); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("budget did not expire: %v", err)
	}
	if _, err := s.Login("2001:db8::1", "admin", "correct-password-1", recovery); err != nil {
		t.Fatalf("normal second step with one-failure policy must succeed: %v", err)
	}
	if _, err := s.Login("2001:db8::1", "admin", "correct-password-1", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatal("successful authentication reset computation budget")
	}
}

func TestComputationBudgetAllowsTwoStepFlowAndBoundsSourceTable(t *testing.T) {
	s := &Service{config: Config{MaxLoginFailures: 1, LoginWindow: time.Minute}, compute: map[string]computeBudget{}, now: time.Now}
	if !s.admitPasswordComputation("source") || !s.admitPasswordComputation("source") || s.admitPasswordComputation("source") {
		t.Fatal("two-step allowance must be two computations even with one failure allowed")
	}
	for i := 0; i < maxComputeSources; i++ {
		s.compute[string(rune(i))+"key"] = computeBudget{start: s.now()}
	}
	if s.admitPasswordComputation("new-source") {
		t.Fatal("computation source table must fail closed at capacity")
	}
}

func TestBusyHashSlotDoesNotSpendComputationBudget(t *testing.T) {
	for _, reauth := range []bool{false, true} {
		t.Run(map[bool]string{false: "login", true: "reauth"}[reauth], func(t *testing.T) {
			dir := t.TempDir()
			db, err := store.Open(filepath.Join(dir, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			now := time.Now().UTC()
			if err := db.CreateInitialAdmin(store.User{ID: "admin", Username: "admin", PasswordHash: "correct-password-1", Role: "admin", CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			h := &computationSpy{}
			s, err := NewService(db, h, Config{BootstrapTokenPath: filepath.Join(dir, "bootstrap"), MaxLoginFailures: 5, LoginWindow: 15 * time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			s.now = func() time.Time { return now }
			check := func() error {
				if reauth {
					_, err := s.verifyCurrentPassword("admin", "correct-password-1")
					return err
				}
				_, err := s.Login("192.0.2.1", "admin", "correct-password-1", "")
				return err
			}
			s.hashSlots <- struct{}{}
			for i := 0; i < 12; i++ {
				var limited *RateLimitError
				if err := check(); !errors.As(err, &limited) || limited.RetryAfter != time.Second {
					t.Fatalf("busy attempt %d: %v", i, err)
				}
			}
			<-s.hashSlots
			if h.verifies != 0 {
				t.Fatalf("busy requests performed %d computations", h.verifies)
			}
			// Actual successful work still consumes the original ten-computation limit.
			for i := 0; i < 10; i++ {
				if err := check(); err != nil {
					t.Fatalf("available attempt %d: %v", i, err)
				}
			}
			if err := check(); !errors.Is(err, ErrRateLimited) || h.verifies != 10 {
				t.Fatalf("work budget bypass: verifies=%d err=%v", h.verifies, err)
			}
		})
	}
}
