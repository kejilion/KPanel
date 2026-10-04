//go:build windows

package windowsnode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

var errDesktopAccount = errors.New("managed desktop account unavailable")

type desktopAccountRecord struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	Marker  string `json:"marker"`
	SID     string `json:"sid,omitempty"`
}

type desktopAccountIdentity struct{ SID, Marker string }

// Only the Windows adapter mutates SAM. Tests inject a fake adapter; they never
// create OS accounts, change passwords, configure RDP, or log off a real user.
type desktopAccountAPI interface {
	lookup(string) (desktopAccountIdentity, error)
	create(desktopAccountRecord, string) error
	remove(string) error
	admin(string) error
	verifyAdmin(string) error
	password(string, string) error
	expires(string, time.Time) error
	disable(string, bool) error
	logoff(string) error
}

func desktopAccountPath() string { return filepath.Join(DataDir(), "desktop-account.json") }

func desktopPassword() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	defer clear(data)
	// 256 random bits plus all four Windows password complexity categories.
	return "Kp!7" + hex.EncodeToString(data), nil
}

func (r desktopAccountRecord) valid() bool {
	if r.Version != 1 || !strings.HasPrefix(r.Name, "kp_rdp_") || len(r.Name) != 19 || len(r.Marker) != 64 {
		return false
	}
	_, a := hex.DecodeString(strings.TrimPrefix(r.Name, "kp_rdp_"))
	_, b := hex.DecodeString(r.Marker)
	return a == nil && b == nil
}

func readDesktopAccount() (desktopAccountRecord, error) {
	data, err := ReadFile(desktopAccountPath(), 4096, SystemOnly)
	if err != nil {
		return desktopAccountRecord{}, err
	}
	var r desktopAccountRecord
	if json.Unmarshal(data, &r) != nil || !r.valid() {
		return r, errDesktopAccount
	}
	return r, nil
}

func writeDesktopAccount(r desktopAccountRecord) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return WriteAtomic(desktopAccountPath(), data, SystemOnly)
}

// Capture ownership before installation starts. A retry may recover an older
// intent, but must not delete a previous installation on a later failure.
func BeginDesktopInstallation() (func() error, error) {
	if !IsSystem() {
		return nil, errDesktopAccount
	}
	_, accountErr := readDesktopAccount()
	_, listenerErr := readDesktopListenerJournal()
	return desktopInstallRollback(accountErr, listenerErr, RemoveDesktopAccount, RestoreManagedDesktopListener)
}

func desktopInstallRollback(accountErr, listenerErr error, remove, restore func() error) (func() error, error) {
	for _, err := range []error{accountErr, listenerErr} {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return sync.OnceValue(func() error {
		var failures []error
		if errors.Is(listenerErr, os.ErrNotExist) {
			failures = append(failures, restore())
		}
		if errors.Is(accountErr, os.ErrNotExist) {
			failures = append(failures, remove())
		}
		return errors.Join(failures...)
	}), nil
}

func checkDesktopIdentity(api desktopAccountAPI, r desktopAccountRecord) error {
	identity, err := api.lookup(r.Name)
	if err != nil || r.SID == "" || identity.SID != r.SID || identity.Marker != r.Marker {
		return errDesktopAccount
	}
	return nil
}

// InstallDesktopAccount provisions a disabled, randomly named local
// administrator only after an explicit installer desktop opt-in. A protected
// intent is written first so a crash after NetUserAdd can be recovered without
// adopting an unrelated pre-existing account. No password is persisted.
func InstallDesktopAccount() error {
	if !IsSystem() || managedDesktopPlatformAllowed() != nil {
		return errDesktopAccount
	}
	return installDesktopAccount(nativeDesktopAccountAPI{}, readDesktopAccount, writeDesktopAccount)
}

func installDesktopAccount(api desktopAccountAPI, read func() (desktopAccountRecord, error), write func(desktopAccountRecord) error) error {
	r, err := read()
	if errors.Is(err, os.ErrNotExist) {
		seed := make([]byte, 32)
		if _, err := rand.Read(seed); err != nil {
			return err
		}
		r = desktopAccountRecord{Version: 1, Name: "kp_rdp_" + hex.EncodeToString(seed[:6]), Marker: hex.EncodeToString(seed)}
		clear(seed)
		if err := write(r); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if !r.valid() {
		return errDesktopAccount
	}
	identity, err := api.lookup(r.Name)
	if errors.Is(err, os.ErrNotExist) && r.SID == "" {
		password, passwordErr := desktopPassword()
		if passwordErr != nil {
			return passwordErr
		}
		err = api.create(r, password)
		password = ""
		if err != nil {
			return err
		}
		identity, err = api.lookup(r.Name)
	}
	if err != nil || identity.Marker != r.Marker || identity.SID == "" || (r.SID != "" && r.SID != identity.SID) {
		return errDesktopAccount
	}
	r.SID = identity.SID
	if err := write(r); err != nil {
		return err
	}
	manager := &DesktopAccountManager{api: api, record: r}
	if err := manager.revoke(); err != nil {
		return err
	}
	// Account remains disabled even if group assignment fails midway.
	if err := checkDesktopIdentity(api, r); err != nil {
		return err
	}
	return api.admin(r.Name)
}

type DesktopAccountManager struct {
	mu       sync.Mutex
	api      desktopAccountAPI
	record   desktopAccountRecord
	poisoned bool
	domain   string
	allowed  func() error
}

func OpenDesktopAccountManager() (*DesktopAccountManager, error) {
	if !IsSystem() || managedDesktopPlatformAllowed() != nil {
		return nil, errDesktopAccount
	}
	r, err := readDesktopAccount()
	if err != nil {
		return nil, err
	}
	domain, err := desktopMachineName()
	if err != nil {
		return nil, err
	}
	m := &DesktopAccountManager{api: nativeDesktopAccountAPI{}, record: r, domain: domain, allowed: managedDesktopPlatformAllowed}
	// Recover any lease left by process termination before advertising control.
	if err := m.revoke(); err != nil {
		return nil, err
	}
	if err := m.change(m.api.verifyAdmin); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *DesktopAccountManager) change(f func(string) error) error {
	if err := checkDesktopIdentity(m.api, m.record); err != nil {
		return err
	}
	return f(m.record.Name)
}

func (m *DesktopAccountManager) revoke() error {
	// Try every revocation step even if an earlier one fails. Never operate on a
	// replacement account with the same name. Existing profile data is retained.
	disabled := m.change(func(name string) error { return m.api.disable(name, true) })
	expired := m.change(func(name string) error { return m.api.expires(name, time.Unix(1, 0)) })
	password, randomErr := desktopPassword()
	var rotated error
	if randomErr == nil {
		rotated = m.change(func(name string) error { return m.api.password(name, password) })
	}
	password = ""
	var loggedOff error
	if err := checkDesktopIdentity(m.api, m.record); err != nil {
		loggedOff = err
	} else {
		loggedOff = m.api.logoff(m.record.SID)
	}
	return errors.Join(disabled, expired, randomErr, rotated, loggedOff)
}

func (m *DesktopAccountManager) Prepare(ctx context.Context) (desktopcredentials.Credentials, func() error, error) {
	if !m.mu.TryLock() {
		return desktopcredentials.Credentials{}, nil, errDesktopAccount
	}
	if m.poisoned {
		m.mu.Unlock()
		return desktopcredentials.Credentials{}, nil, errDesktopAccount
	}
	cleanup := sync.OnceValue(func() error {
		err := m.revoke()
		m.poisoned = err != nil
		m.mu.Unlock()
		return err
	})
	fail := func() (desktopcredentials.Credentials, func() error, error) {
		_ = cleanup()
		return desktopcredentials.Credentials{}, nil, errDesktopAccount
	}
	if ctx.Err() != nil || m.domain == "" || m.allowed != nil && m.allowed() != nil || m.revoke() != nil || m.change(m.api.verifyAdmin) != nil {
		return fail()
	}
	password, err := desktopPassword()
	if err != nil {
		return fail()
	}
	if m.change(func(name string) error { return m.api.password(name, password) }) != nil ||
		m.change(func(name string) error { return m.api.expires(name, time.Now().Add(time.Minute)) }) != nil ||
		m.change(func(name string) error { return m.api.disable(name, false) }) != nil || ctx.Err() != nil {
		password = ""
		return fail()
	}
	return desktopcredentials.Credentials{Username: m.record.Name, Domain: m.domain, Password: password}, cleanup, nil
}

func RemoveDesktopAccount() error {
	if !IsSystem() {
		return errDesktopAccount
	}
	r, err := readDesktopAccount()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	api := nativeDesktopAccountAPI{}
	identity, err := api.lookup(r.Name)
	if errors.Is(err, os.ErrNotExist) {
		return os.Remove(desktopAccountPath())
	}
	if err != nil || identity.Marker != r.Marker || (r.SID != "" && identity.SID != r.SID) {
		return errDesktopAccount
	}
	// A pending install intent can only recover its own random marker.
	r.SID = identity.SID
	m := &DesktopAccountManager{api: api, record: r}
	if err := m.revoke(); err != nil {
		return err
	}
	if err := m.change(api.remove); err != nil {
		return err
	}
	return os.Remove(desktopAccountPath())
}
