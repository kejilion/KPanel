package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const passkeyTestOrigin = "https://panel.example.com"
const passkeyPassword = "a-strong-password-1"

func TestPasskeyOriginCanonicalDefaultPort(t *testing.T) {
	for _, value := range []string{"https://Panel.Example.com:443", "https://panel.example.com/"} {
		origin, rp, err := PasskeyOrigin(value)
		if err != nil || origin != "https://panel.example.com" || rp != "panel.example.com" {
			t.Fatal(value, origin, rp, err)
		}
	}
}

type testAuthenticator struct {
	key    *ecdsa.PrivateKey
	id     []byte
	handle []byte
}

func newTestAuthenticator(t *testing.T, handle string) *testAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &testAuthenticator{key: key, id: id, handle: []byte(handle)}
}
func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }
func (a *testAuthenticator) registration(t *testing.T, options PasskeyOptions, origin string, flags byte) []byte {
	t.Helper()
	o := options.PublicKey.(protocol.PublicKeyCredentialCreationOptions)
	client, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": o.Challenge.String(), "origin": origin, "crossOrigin": false})
	pub, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: a.key.X.FillBytes(make([]byte, 32)), -3: a.key.Y.FillBytes(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	rp := sha256.Sum256([]byte(o.RelyingParty.ID))
	data := append([]byte{}, rp[:]...)
	data = append(data, flags|0x40)
	data = append(data, 0, 0, 0, 0)
	data = append(data, make([]byte, 16)...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(a.id)))
	data = append(data, a.id...)
	data = append(data, pub...)
	att, err := cbor.Marshal(map[string]any{"fmt": "none", "authData": data, "attStmt": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"id": b64(a.id), "rawId": b64(a.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "attestationObject": b64(att), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{}})
	return result
}
func (a *testAuthenticator) assertion(t *testing.T, options PasskeyOptions, origin string, flags byte, count uint32) []byte {
	t.Helper()
	o := options.PublicKey.(protocol.PublicKeyCredentialRequestOptions)
	client, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": o.Challenge.String(), "origin": origin, "crossOrigin": false})
	rp := sha256.Sum256([]byte(o.RelyingPartyID))
	data := append([]byte{}, rp[:]...)
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, count)
	hash := sha256.Sum256(client)
	signed := append(append([]byte{}, data...), hash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"id": b64(a.id), "rawId": b64(a.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "authenticatorData": b64(data), "signature": b64(signature), "userHandle": b64(a.handle)}, "clientExtensionResults": map[string]any{}})
	return result
}
func setupPasskeys(t *testing.T) (*PasskeyService, Credentials, Session) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s, err := NewService(st, testHasher(t), Config{BootstrapTokenPath: filepath.Join(dir, "bootstrap"), MaxLoginFailures: 20})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	token, _ := os.ReadFile(s.config.BootstrapTokenPath)
	credentials, err := s.Bootstrap(string(token), "admin", passkeyPassword)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.Authenticate(credentials.Token)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewPasskeyService(s, passkeyTestOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return p, credentials, session
}
func registerTestPasskey(t *testing.T, p *PasskeyService, session Session, flags byte) *testAuthenticator {
	t.Helper()
	a := newTestAuthenticator(t, session.User.ID)
	o, err := p.BeginRegistration(session, passkeyPassword, "", "Laptop")
	if err != nil {
		t.Fatal(err)
	}
	if err = p.FinishRegistration(session, o.CeremonyID, a.registration(t, o, passkeyTestOrigin, flags)); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPasskeyRegistrationLoginRevocationAndRecovery(t *testing.T) {
	p, initial, session := setupPasskeys(t)
	a := registerTestPasskey(t, p, session, 5)
	if _, err := p.auth.Authenticate(initial.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("registration did not revoke session", err)
	}
	o, err := p.BeginLogin("192.0.2.1", "admin", "browser")
	if err != nil {
		t.Fatal(err)
	}
	response := a.assertion(t, o, passkeyTestOrigin, 5, 1)
	creds, err := p.FinishLogin("192.0.2.1", "browser", o.CeremonyID, response, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.FinishLogin("192.0.2.1", "browser", o.CeremonyID, response, ""); !errors.Is(err, ErrPasskeyInvalid) {
		t.Fatal("replay", err)
	}
	entries, err := p.List(session.User.ID)
	if err != nil || len(entries) != 1 || entries[0].LastUsedAt == nil {
		t.Fatal(entries, err)
	}
	current, err := p.auth.Authenticate(creds.Token)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := p.BeginLogin("192.0.2.1", "admin", "browser")
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(current, b64(a.id), passkeyPassword, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = p.auth.Authenticate(creds.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("delete did not revoke", err)
	}
	if _, err = p.FinishLogin("192.0.2.1", "browser", pending.CeremonyID, a.assertion(t, pending, passkeyTestOrigin, 5, 2), ""); err == nil {
		t.Fatal("deleted credential accepted")
	}
	passwordLogin, err := p.auth.Login("192.0.2.3", "admin", passkeyPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	current, _ = p.auth.Authenticate(passwordLogin.Token)
	registerTestPasskey(t, p, current, 5)
	if _, err = p.auth.RecoverPassword(session.User.ID, "new-strong-password-2", false); err != nil {
		t.Fatal(err)
	}
	entries, err = p.List(session.User.ID)
	if err != nil || len(entries) != 0 {
		t.Fatal("local recovery retained passkeys", entries, err)
	}
}

func TestPasskeyRejectsOriginUVSignatureBindingAndExpiredChallenges(t *testing.T) {
	for _, tc := range []string{"origin", "uv", "signature", "handle", "browser", "expired", "credential-version"} {
		t.Run(tc, func(t *testing.T) {
			p, _, s := setupPasskeys(t)
			a := registerTestPasskey(t, p, s, 5)
			o, err := p.BeginLogin("192.0.2.1", "admin", "browser")
			if err != nil {
				t.Fatal(err)
			}
			origin := passkeyTestOrigin
			flags := byte(5)
			binding := "browser"
			switch tc {
			case "origin":
				origin = "https://evil.example.com"
			case "uv":
				flags = 1
			case "signature":
				a.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			case "handle":
				a.handle = []byte("other-admin")
			case "browser":
				binding = "other-browser"
			case "expired":
				p.auth.now = func() time.Time { return time.Now().Add(4 * time.Minute) }
			case "credential-version":
				if err := p.auth.ChangePassword(s.User.ID, passkeyPassword, "new-strong-password-2"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = p.FinishLogin("192.0.2.1", binding, o.CeremonyID, a.assertion(t, o, origin, flags, 1), ""); err == nil {
				t.Fatal("invalid assertion accepted")
			}
		})
	}
}

func TestPasskeyEnrollmentRejectsMissingUVAndWrongOrigin(t *testing.T) {
	for _, tc := range []string{"uv", "origin", "session"} {
		t.Run(tc, func(t *testing.T) {
			p, initial, s := setupPasskeys(t)
			a := newTestAuthenticator(t, s.User.ID)
			o, err := p.BeginRegistration(s, passkeyPassword, "", "key")
			if err != nil {
				t.Fatal(err)
			}
			origin := passkeyTestOrigin
			flags := byte(5)
			if tc == "uv" {
				flags = 1
			}
			if tc == "origin" {
				origin = "https://evil.example.com"
			}
			if tc == "session" {
				_ = p.auth.Logout(initial.Token)
			}
			if err = p.FinishRegistration(s, o.CeremonyID, a.registration(t, o, origin, flags)); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
}

func TestPasskeyKeepsTOTPAndRecoveryCodeSingleUse(t *testing.T) {
	p, _, s := setupPasskeys(t)
	a := registerTestPasskey(t, p, s, 5)
	enrollment, err := p.auth.StartTOTPEnrollment(s.User.ID, passkeyPassword)
	if err != nil {
		t.Fatal(err)
	}
	step := p.auth.now().Unix()/30 - 1
	code, err := totpAtStep(enrollment.Secret, step)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := p.auth.ConfirmTOTPEnrollment(s.User.ID, enrollment.ID, code)
	if err != nil {
		t.Fatal(err)
	}
	for index, second := range []string{"", codes[0], codes[0]} {
		o, err := p.BeginLogin("192.0.2.1", "admin", "browser")
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.FinishLogin("192.0.2.1", "browser", o.CeremonyID, a.assertion(t, o, passkeyTestOrigin, 5, uint32(index+1)), second)
		if index == 1 && err != nil {
			t.Fatal(err)
		}
		if index != 1 && err == nil {
			t.Fatal("TOTP/recovery bypass")
		}
	}
}

func TestPasskeyConcurrentAssertionOnlyCreatesOneSession(t *testing.T) {
	p, _, s := setupPasskeys(t)
	a := registerTestPasskey(t, p, s, 5)
	o, err := p.BeginLogin("192.0.2.1", "admin", "browser")
	if err != nil {
		t.Fatal(err)
	}
	response := a.assertion(t, o, passkeyTestOrigin, 5, 1)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := p.FinishLogin("192.0.2.1", "browser", o.CeremonyID, response, ""); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful replays=%d", success)
	}
}

func TestPasskeySyncedZeroCountersAndLimits(t *testing.T) {
	p, _, s := setupPasskeys(t)
	a := registerTestPasskey(t, p, s, 0x1d)
	for range 2 {
		o, err := p.BeginLogin("192.0.2.1", "admin", "browser")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.FinishLogin("192.0.2.1", "browser", o.CeremonyID, a.assertion(t, o, passkeyTestOrigin, 0x1d, 0), ""); err != nil {
			t.Fatal(err)
		}
	}
	p.auth.config.MaxLoginFailures = 1
	if _, err := p.BeginLogin("192.0.2.1", "missing", "browser"); !errors.Is(err, ErrRateLimited) {
		t.Fatal("challenge budget", err)
	}
	for _, origin := range []string{"http://panel.example.com", "https://127.0.0.1", "https://panel.example.com/x", "https://user@panel.example.com", "https://panel.example.com#x", "https://localhost", "https://*.example.com"} {
		if _, _, err := PasskeyOrigin(origin); err == nil {
			t.Fatal("accepted", origin)
		}
	}
}
