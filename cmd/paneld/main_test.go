package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/version"
)

func TestValidateAgentHealthAcceptsReadyAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "ok",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
	})
	if err != nil {
		t.Fatalf("validateAgentHealth() error = %v", err)
	}
}

func TestValidateAgentHealthAcceptsMissingOptionalWebRoot(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "degraded",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		Reasons:         []string{"web_root_unavailable"},
	})
	if err != nil {
		t.Fatalf("validateAgentHealth() error = %v", err)
	}
}

func TestValidateAgentHealthRejectsDegradedAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "degraded",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		Reasons:         []string{"docker_unavailable"},
	})
	if err == nil || !strings.Contains(err.Error(), "docker_unavailable") {
		t.Fatalf("validateAgentHealth() error = %v, want degraded reason", err)
	}
}

func TestValidateAgentHealthRejectsReadOnlyAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "ok",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		ReadOnly:        true,
	})
	if err == nil {
		t.Fatal("validateAgentHealth() accepted a read-only Agent")
	}
}

func TestRunPasswordResetReplacesPasswordAndDisablesTOTP(t *testing.T) {
	directory, user := seedPasswordResetState(t)
	readCount := 0
	reader := func(string) ([]byte, error) {
		readCount++
		return []byte("a-recovered-password-3"), nil
	}
	var output bytes.Buffer
	if err := runPasswordResetWithReader([]string{"--disable-2fa"}, &output, reader); err != nil {
		t.Fatal(err)
	}
	if readCount != 2 {
		t.Fatalf("password reader called %d times; want 2", readCount)
	}
	if !strings.Contains(output.String(), "Administrator: admin") ||
		!strings.Contains(output.String(), "All existing sessions were revoked") ||
		!strings.Contains(output.String(), "recovery codes were disabled") {
		t.Fatalf("unexpected recovery output: %q", output.String())
	}

	storage, err := store.Open(filepath.Join(directory, "panel-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	recovered, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := auth.NewArgon2idHasher(auth.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}
	valid, err := hasher.Verify("a-recovered-password-3", recovered.PasswordHash)
	if err != nil || !valid {
		t.Fatalf("recovered password did not verify: %v", err)
	}
	if recovered.TOTPSecret != "" || len(recovered.TOTPRecoveryCodeHashes) != 0 {
		t.Fatalf("TOTP state was preserved despite explicit reset: %#v", recovered)
	}
	if _, err := storage.SessionByTokenHash("session-hash", time.Now().UTC()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("existing session survived recovery: %v", err)
	}
}

func TestRunPasswordResetRejectsMismatchedConfirmationWithoutChangingState(t *testing.T) {
	directory, user := seedPasswordResetState(t)
	first := []byte("a-recovered-password-3")
	second := []byte("a-different-password")
	passwords := [][]byte{first, second}
	reader := func(string) ([]byte, error) {
		password := passwords[0]
		passwords = passwords[1:]
		return password, nil
	}
	if err := runPasswordResetWithReader(nil, io.Discard, reader); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected mismatch error: %v", err)
	}
	if !bytes.Equal(first, make([]byte, len(first))) || !bytes.Equal(second, make([]byte, len(second))) {
		t.Fatal("password buffers were not cleared")
	}

	storage, err := store.Open(filepath.Join(directory, "panel-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	unchanged, err := storage.UserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.PasswordHash != user.PasswordHash {
		t.Fatal("mismatched confirmation changed the password")
	}
	if _, err := storage.SessionByTokenHash("session-hash", time.Now().UTC()); err != nil {
		t.Fatalf("mismatched confirmation revoked the session: %v", err)
	}
}

func TestRunPasswordResetRequiresStoppedPanel(t *testing.T) {
	directory, _ := seedPasswordResetState(t)
	storage, err := store.Open(filepath.Join(directory, "panel-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	readCalled := false
	err = runPasswordResetWithReader(nil, io.Discard, func(string) ([]byte, error) {
		readCalled = true
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "stop the panel service") {
		t.Fatalf("unexpected locked-state error: %v", err)
	}
	if readCalled {
		t.Fatal("password was requested while the panel state was locked")
	}
}

func TestRunPasswordResetRejectsNonInteractiveInput(t *testing.T) {
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	err = runPasswordReset(nil, input, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("unexpected non-interactive error: %v", err)
	}
}

func TestRunPasswordResetHelpWorksWithoutInteractiveInput(t *testing.T) {
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var output bytes.Buffer
	err = runPasswordReset([]string{"--help"}, input, &output)
	if err == nil || !strings.Contains(output.String(), "disable-2fa") {
		t.Fatalf("unexpected help result: err=%v output=%q", err, output.String())
	}
}

func TestRunPasswordResetDoesNotAcceptPasswordArguments(t *testing.T) {
	readCalled := false
	err := runPasswordResetWithReader([]string{"password-from-shell-history"}, io.Discard, func(string) ([]byte, error) {
		readCalled = true
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected reset-password argument") {
		t.Fatalf("unexpected positional password error: %v", err)
	}
	if readCalled {
		t.Fatal("password reader was called for a positional password argument")
	}
}

func seedPasswordResetState(t *testing.T) (string, store.User) {
	t.Helper()
	directory := t.TempDir()
	storePath := filepath.Join(directory, "panel-state.json")
	t.Setenv("KEJILION_PANEL_DATA_DIR", directory)
	t.Setenv("KEJILION_PANEL_STORE_PATH", storePath)
	t.Setenv("KEJILION_PANEL_BOOTSTRAP_TOKEN_FILE", filepath.Join(directory, "bootstrap.token"))
	t.Setenv("KEJILION_PANEL_TOTP_KEY_FILE", filepath.Join(directory, "totp-encryption.key"))
	t.Setenv("KEJILION_PANEL_AGENT_SOCKET", filepath.Join(directory, "agent.sock"))
	t.Setenv("KEJILION_PANEL_AGENT_TOKEN_FILE", filepath.Join(directory, "agent.token"))
	t.Setenv("KEJILION_PANEL_WEB_ROOT", filepath.Join(t.TempDir(), "web"))

	storage, err := store.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := auth.NewArgon2idHasher(auth.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := hasher.Hash("the-original-password")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	enabledAt := now
	user := store.User{
		ID: "user-1", Username: "admin", PasswordHash: passwordHash, Role: "admin",
		TOTPSecret: "encrypted-secret", TOTPEnabledAt: &enabledAt,
		TOTPRecoveryCodeHashes: []string{"recovery-hash"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := storage.CreateInitialAdmin(user); err != nil {
		t.Fatal(err)
	}
	if err := storage.PutSession(store.Session{
		TokenHash: "session-hash", CSRFHash: "csrf-hash", UserID: user.ID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	return directory, user
}
