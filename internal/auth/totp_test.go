package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

func TestTOTPMatchesRFC6238SHA1Vector(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	code, err := totpAtStep(secret, time.Unix(59, 0).Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("TOTP code = %q, want last six RFC 6238 digits 287082", code)
	}
	if _, ok := matchTOTPStep(secret, code, time.Unix(59, 0)); !ok {
		t.Fatal("current RFC 6238 code did not match")
	}
}

func TestTOTPSecretEncryptionRoundTripAndPermissions(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "secrets", "totp.key")
	sealed, err := sealTOTPSecret(keyPath, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openTOTPSecret(keyPath, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if opened != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypted secret = %q", opened)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(keyPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
		}
	}
	if _, err := openTOTPSecret(keyPath, sealed+"corrupt"); err == nil {
		t.Fatal("corrupted encrypted secret was accepted")
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := openTOTPSecret(keyPath, sealed); err == nil {
		t.Fatal("missing encryption key was silently replaced")
	}
	if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("decrypt recreated missing key: %v", err)
	}
}

func TestTOTPManagementPasswordIsRateLimited(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		TOTPKeyPath:        filepath.Join(directory, "totp.key"),
		LoginWindow:        time.Minute,
		MaxLoginFailures:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
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
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.StartTOTPEnrollment(credentials.User.ID, "wrong-password"); !errors.Is(err, ErrInvalidCurrentPassword) {
			t.Fatalf("attempt %d error = %v", attempt+1, err)
		}
	}
	if _, err := service.StartTOTPEnrollment(credentials.User.ID, "a-strong-password-1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("management reauthentication was not rate limited: %v", err)
	}
}

func TestTOTPEnrollmentLoginRecoveryRotationAndDisable(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service, err := NewService(storage, testHasher(t), Config{
		BootstrapTokenPath: filepath.Join(directory, "bootstrap.token"),
		TOTPKeyPath:        filepath.Join(directory, "totp.key"),
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   8,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	if err := service.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(directory, "bootstrap.token"))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.Bootstrap(string(token), "admin", "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}

	enrollment, err := service.StartTOTPEnrollment(initial.User.ID, "a-strong-password-1")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totpAtStep(enrollment.Secret, now.Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	recoveryCodes, err := service.ConfirmTOTPEnrollment(initial.User.ID, enrollment.ID, code)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveryCodes) != recoveryCodeCount {
		t.Fatalf("recovery code count = %d", len(recoveryCodes))
	}
	if _, err := service.Authenticate(initial.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("enrollment did not revoke existing session: %v", err)
	}
	status, err := service.TOTPStatus(initial.User.ID)
	if err != nil || !status.Enabled || status.RecoveryCodesRemaining != recoveryCodeCount {
		t.Fatalf("unexpected TOTP status %#v, err=%v", status, err)
	}
	keyPath := filepath.Join(directory, "totp.key")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	missingKeyCode, _ := totpAtStep(enrollment.Secret, now.Unix()/30)
	if _, err := service.Login("192.0.2.10", "admin", "a-strong-password-1", missingKeyCode); !errors.Is(err, ErrSecondFactorUnavailable) {
		t.Fatalf("missing encryption key was not reported as unavailable: %v", err)
	}
	missingKeyRecovery, err := service.Login("192.0.2.11", "admin", "a-strong-password-1", recoveryCodes[9])
	if err != nil {
		t.Fatalf("recovery login should work without the encryption key: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", ""); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("password-only login was not challenged: %v", err)
	}
	if _, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", code); !errors.Is(err, ErrInvalidSecondFactor) {
		t.Fatalf("enrollment code replay was accepted: %v", err)
	}

	now = now.Add(30 * time.Second)
	code, _ = totpAtStep(enrollment.Secret, now.Unix()/30)
	login, err := service.Login("192.0.2.1", "admin", "a-strong-password-1", code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login("192.0.2.2", "admin", "a-strong-password-1", code); !errors.Is(err, ErrInvalidSecondFactor) {
		t.Fatalf("TOTP replay was accepted: %v", err)
	}
	recoveryLogin, err := service.Login("192.0.2.3", "admin", "a-strong-password-1", recoveryCodes[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login("192.0.2.4", "admin", "a-strong-password-1", recoveryCodes[0]); !errors.Is(err, ErrInvalidSecondFactor) {
		t.Fatalf("recovery code replay was accepted: %v", err)
	}

	now = now.Add(30 * time.Second)
	code, _ = totpAtStep(enrollment.Secret, now.Unix()/30)
	rotated, err := service.RegenerateRecoveryCodes(initial.User.ID, "a-strong-password-1", code)
	if err != nil || len(rotated) != recoveryCodeCount {
		t.Fatalf("recovery rotation failed: count=%d err=%v", len(rotated), err)
	}
	for _, token := range []string{login.Token, recoveryLogin.Token, missingKeyRecovery.Token} {
		if _, err := service.Authenticate(token); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("recovery rotation did not revoke session: %v", err)
		}
	}
	if _, err := service.Login("192.0.2.5", "admin", "a-strong-password-1", recoveryCodes[1]); !errors.Is(err, ErrInvalidSecondFactor) {
		t.Fatalf("old recovery code survived rotation: %v", err)
	}

	now = now.Add(30 * time.Second)
	code, _ = totpAtStep(enrollment.Secret, now.Unix()/30)
	if err := service.DisableTOTP(initial.User.ID, "a-strong-password-1", code); err != nil {
		t.Fatal(err)
	}
	plainLogin, err := service.Login("192.0.2.6", "admin", "a-strong-password-1", "")
	if err != nil || plainLogin.User.TOTPEnabled {
		t.Fatalf("password login after disable failed: %#v err=%v", plainLogin.User, err)
	}
}
