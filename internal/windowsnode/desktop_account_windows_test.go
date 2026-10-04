//go:build windows

package windowsnode

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeDesktopAccountAPI struct {
	identity                         desktopAccountIdentity
	created, administrator, disabled bool
	secret                           string
	expiry                           time.Time
	changes, logoffs                 int
	failLogoff                       bool
	beforeCreate                     func()
}

func (f *fakeDesktopAccountAPI) lookup(string) (desktopAccountIdentity, error) {
	if !f.created {
		return desktopAccountIdentity{}, os.ErrNotExist
	}
	return f.identity, nil
}
func (f *fakeDesktopAccountAPI) create(r desktopAccountRecord, password string) error {
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	if f.created {
		return errors.New("exists")
	}
	f.created, f.disabled, f.secret = true, true, password
	f.identity = desktopAccountIdentity{SID: "S-1-5-21-1-2-3-1001", Marker: r.Marker}
	f.changes++
	return nil
}
func (f *fakeDesktopAccountAPI) remove(string) error { f.created = false; f.changes++; return nil }
func (f *fakeDesktopAccountAPI) admin(string) error  { f.administrator = true; f.changes++; return nil }
func (f *fakeDesktopAccountAPI) verifyAdmin(string) error {
	if !f.administrator {
		return errDesktopAccount
	}
	return nil
}
func (f *fakeDesktopAccountAPI) password(_ string, value string) error {
	f.secret = value
	f.changes++
	return nil
}
func (f *fakeDesktopAccountAPI) expires(_ string, value time.Time) error {
	f.expiry = value
	f.changes++
	return nil
}
func (f *fakeDesktopAccountAPI) disable(_ string, value bool) error {
	f.disabled = value
	f.changes++
	return nil
}
func (f *fakeDesktopAccountAPI) logoff(string) error {
	f.logoffs++
	if f.failLogoff {
		return errDesktopAccount
	}
	return nil
}

func managedAccountFixture(t *testing.T) (*DesktopAccountManager, *fakeDesktopAccountAPI) {
	t.Helper()
	var record desktopAccountRecord
	api := &fakeDesktopAccountAPI{}
	err := installDesktopAccount(api, func() (desktopAccountRecord, error) { return record, os.ErrNotExist }, func(r desktopAccountRecord) error { record = r; return nil })
	if err != nil {
		t.Fatal(err)
	}
	return &DesktopAccountManager{api: api, record: record, domain: "TESTBOX"}, api
}

func TestDesktopManagedLeaseRotatesAndRevokesAdministrator(t *testing.T) {
	m, api := managedAccountFixture(t)
	if !api.disabled || !api.administrator || !api.expiry.Before(time.Now()) {
		t.Fatal("installed account must be an expired, disabled administrator")
	}
	oldPassword := api.secret
	credentials, cleanup, err := m.Prepare(context.Background())
	if err != nil || cleanup == nil {
		t.Fatal(err)
	}
	if credentials.Domain != "TESTBOX" || credentials.Password != api.secret || api.secret == oldPassword || api.disabled || time.Until(api.expiry) < 50*time.Second || time.Until(api.expiry) > time.Minute {
		t.Fatal("incorrect short-lived administrator lease")
	}
	if _, cleanup2, err := m.Prepare(context.Background()); err == nil || cleanup2 != nil {
		t.Fatal("concurrent credential lease admitted")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if !api.disabled || !api.expiry.Before(time.Now()) || api.secret == credentials.Password {
		t.Fatal("credential lease not revoked")
	}
	logoffs := api.logoffs
	if cleanup() != nil || api.logoffs != logoffs {
		t.Fatal("cleanup is not idempotent")
	}
	_, next, err := m.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if next() != nil {
		t.Fatal("next lease cleanup")
	}
}

func TestDesktopManagedLeaseNeverChangesReplacementSID(t *testing.T) {
	m, api := managedAccountFixture(t)
	api.identity.SID = "S-1-5-21-1-2-3-1002"
	before, loggedOff := api.changes, api.logoffs
	if _, cleanup, err := m.Prepare(context.Background()); err == nil || cleanup != nil {
		t.Fatal("replacement account adopted")
	}
	if api.changes != before || api.logoffs != loggedOff {
		t.Fatal("replacement account was modified or logged off")
	}
}

func TestDesktopManagedLeaseCleanupFailureBlocksFurtherCredentials(t *testing.T) {
	m, api := managedAccountFixture(t)
	_, cleanup, err := m.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	api.failLogoff = true
	if cleanup() == nil {
		t.Fatal("failed logoff reported success")
	}
	if !api.disabled || !api.expiry.Before(time.Now()) {
		t.Fatal("logoff failure skipped account revocation")
	}
	api.failLogoff = false
	before := api.changes
	if _, cleanup, err := m.Prepare(context.Background()); err == nil || cleanup != nil || before != api.changes {
		t.Fatal("poisoned manager issued another lease")
	}
}

func TestDesktopManagedLeaseCancellationAndPolicyDoNotEnableAccount(t *testing.T) {
	for _, cause := range []string{"cancel", "domain", "admin-removed"} {
		t.Run(cause, func(t *testing.T) {
			m, api := managedAccountFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cause == "cancel" {
				cancel()
			}
			if cause == "domain" {
				m.allowed = func() error { return errDesktopAccount }
			}
			if cause == "admin-removed" {
				api.administrator = false
			}
			if _, cleanup, err := m.Prepare(ctx); err == nil || cleanup != nil || !api.disabled {
				t.Fatal("ineligible request activated account")
			}
		})
	}
}

func TestDesktopAccountInstallIntentRecoversOnlyOwnedAccount(t *testing.T) {
	var record desktopAccountRecord
	api := &fakeDesktopAccountAPI{beforeCreate: func() {
		if !record.valid() || record.SID != "" {
			t.Fatal("account created before protected intent")
		}
	}}
	writes := 0
	write := func(r desktopAccountRecord) error {
		writes++
		if writes == 2 {
			return errors.New("disk fault")
		}
		record = r
		return nil
	}
	if installDesktopAccount(api, func() (desktopAccountRecord, error) { return record, os.ErrNotExist }, write) == nil {
		t.Fatal("write failure ignored")
	}
	if !api.disabled || api.administrator {
		t.Fatal("partial install enabled administrator")
	}
	read := func() (desktopAccountRecord, error) { return record, nil }
	if err := installDesktopAccount(api, read, write); err != nil {
		t.Fatal("owned pending intent could not recover", err)
	}
	if record.SID != api.identity.SID || !api.disabled || !api.administrator {
		t.Fatal("incomplete recovered account")
	}
	api.identity.Marker = strings.Repeat("a", 64)
	before := api.changes
	if installDesktopAccount(api, read, write) == nil || api.changes != before {
		t.Fatal("unrelated account changed")
	}
}

func TestDesktopInstallRollbackPreservesPreviousInstallation(t *testing.T) {
	for _, existed := range []bool{false, true} {
		t.Run(map[bool]string{true: "retry", false: "fresh"}[existed], func(t *testing.T) {
			var prior error = os.ErrNotExist
			if existed {
				prior = nil
			}
			removed, restored := 0, 0
			rollback, err := desktopInstallRollback(prior, prior, func() error { removed++; return nil }, func() error { restored++; return errors.New("registry failure") })
			if err != nil {
				t.Fatal(err)
			}
			err = rollback()
			if existed && (err != nil || removed != 0 || restored != 0) {
				t.Fatal("retry removed existing installation")
			}
			if !existed && (err == nil || removed != 1 || restored != 1) {
				t.Fatal("fresh rollback did not attempt both cleanup operations")
			}
			_ = rollback()
			if removed > 1 || restored > 1 {
				t.Fatal("rollback not idempotent")
			}
		})
	}
	if _, err := desktopInstallRollback(os.ErrPermission, nil, nil, nil); err == nil {
		t.Fatal("unknown prior state admitted")
	}
}
