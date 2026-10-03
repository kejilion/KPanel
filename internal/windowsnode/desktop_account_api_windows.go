//go:build windows

package windowsnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type nativeDesktopAccountAPI struct{}

const desktopAccountFlags = uint32(0x0001 | 0x0200) // UF_SCRIPT | UF_NORMAL_ACCOUNT

type desktopUserInfo23 struct {
	Name, FullName, Comment *uint16
	Flags                   uint32
	SID                     *windows.SID
}

func desktopNetCall(api string, args ...uintptr) error {
	result, _, _ := windows.NewLazySystemDLL("netapi32.dll").NewProc(api).Call(args...)
	if result == 0 {
		return nil
	}
	if result == 2221 {
		return os.ErrNotExist
	} // NERR_UserNotFound
	return fmt.Errorf("managed desktop account: Windows error %d", result)
}

func (nativeDesktopAccountAPI) lookup(name string) (desktopAccountIdentity, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return desktopAccountIdentity{}, errDesktopAccount
	}
	var buffer *byte
	err = windows.NetUserGetInfo(nil, wide, 23, &buffer)
	if errors.Is(err, syscall.Errno(2221)) {
		return desktopAccountIdentity{}, os.ErrNotExist
	}
	if err != nil || buffer == nil {
		return desktopAccountIdentity{}, errDesktopAccount
	}
	defer windows.NetApiBufferFree(buffer)
	info := (*desktopUserInfo23)(unsafe.Pointer(buffer))
	if info.SID == nil || !info.SID.IsValid() || info.Flags&0x0200 == 0 || !strings.EqualFold(windows.UTF16PtrToString(info.Name), name) {
		return desktopAccountIdentity{}, errDesktopAccount
	}
	return desktopAccountIdentity{SID: info.SID.String(), Marker: strings.TrimPrefix(windows.UTF16PtrToString(info.Comment), "KPanel managed desktop ")}, nil
}

func (nativeDesktopAccountAPI) create(r desktopAccountRecord, password string) error {
	name, _ := windows.UTF16PtrFromString(r.Name)
	comment, _ := windows.UTF16PtrFromString("KPanel managed desktop " + r.Marker)
	secret, err := windows.UTF16FromString(password)
	if err != nil {
		return errDesktopAccount
	}
	defer clear(secret)
	info := struct {
		Name, Password         *uint16
		PasswordAge, Privilege uint32
		HomeDir, Comment       *uint16
		Flags                  uint32
		ScriptPath             *uint16
	}{Name: name, Password: &secret[0], Privilege: 1, Comment: comment, Flags: desktopAccountFlags | 0x0002}
	err = desktopNetCall("NetUserAdd", 0, 1, uintptr(unsafe.Pointer(&info)), 0)
	runtime.KeepAlive(info)
	return err
}

func (nativeDesktopAccountAPI) remove(value string) error {
	name, _ := windows.UTF16PtrFromString(value)
	err := desktopNetCall("NetUserDel", 0, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	return err
}

func desktopSetUser(value string, level uintptr, info unsafe.Pointer) error {
	name, _ := windows.UTF16PtrFromString(value)
	err := desktopNetCall("NetUserSetInfo", 0, uintptr(unsafe.Pointer(name)), level, uintptr(info), 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(info)
	return err
}

func (nativeDesktopAccountAPI) password(name, password string) error {
	secret, err := windows.UTF16FromString(password)
	if err != nil {
		return errDesktopAccount
	}
	defer clear(secret)
	info := struct{ Password *uint16 }{&secret[0]}
	err = desktopSetUser(name, 1003, unsafe.Pointer(&info))
	runtime.KeepAlive(secret)
	return err
}

func (nativeDesktopAccountAPI) expires(name string, when time.Time) error {
	seconds := when.Unix()
	if seconds < 1 || seconds >= int64(^uint32(0)) {
		return errDesktopAccount
	}
	info := uint32(seconds)
	return desktopSetUser(name, 1017, unsafe.Pointer(&info))
}

func (nativeDesktopAccountAPI) disable(name string, disabled bool) error {
	flags := desktopAccountFlags
	if disabled {
		flags |= 0x0002
	}
	return desktopSetUser(name, 1008, unsafe.Pointer(&flags))
}

func (api nativeDesktopAccountAPI) admin(name string) error {
	identity, err := api.lookup(name)
	if err != nil {
		return err
	}
	member, err := windows.StringToSid(identity.SID)
	if err != nil {
		return errDesktopAccount
	}
	// Builtin SIDs work on localized Windows. No domain group is modified.
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinBuiltinAdministratorsSid, windows.WinBuiltinRemoteDesktopUsersSid} {
		groupSID, err := windows.CreateWellKnownSid(kind)
		if err != nil {
			return err
		}
		group, _, _, err := groupSID.LookupAccount("")
		if err != nil {
			return err
		}
		wide, _ := windows.UTF16PtrFromString(group)
		info := struct{ SID *windows.SID }{member}
		result, _, _ := windows.NewLazySystemDLL("netapi32.dll").NewProc("NetLocalGroupAddMembers").Call(0, uintptr(unsafe.Pointer(wide)), 0, uintptr(unsafe.Pointer(&info)), 1)
		runtime.KeepAlive(info)
		if result != 0 && result != 1378 {
			return fmt.Errorf("managed desktop membership: Windows error %d", result)
		}
	}
	return nil
}

func (nativeDesktopAccountAPI) verifyAdmin(value string) error {
	name, _ := windows.UTF16PtrFromString(value)
	var buffer *byte
	var count, total uint32
	err := desktopNetCall("NetUserGetLocalGroups", 0, uintptr(unsafe.Pointer(name)), 0, 1, uintptr(unsafe.Pointer(&buffer)), 65536, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&total)))
	runtime.KeepAlive(name)
	if buffer != nil {
		defer windows.NetApiBufferFree(buffer)
	}
	if err != nil || count != total || count > 1024 || (count != 0 && buffer == nil) {
		return errDesktopAccount
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return errDesktopAccount
	}
	adminName, _, _, err := adminSID.LookupAccount("")
	if err != nil {
		return errDesktopAccount
	}
	for _, group := range unsafe.Slice((**uint16)(unsafe.Pointer(buffer)), count) {
		if strings.EqualFold(windows.UTF16PtrToString(group), adminName) {
			return nil
		}
	}
	return errDesktopAccount
}

func desktopMachineName() (string, error) {
	var name [256]uint16
	size := uint32(len(name))
	if windows.GetComputerName(&name[0], &size) != nil || size == 0 || size >= uint32(len(name)) {
		return "", errDesktopAccount
	}
	return windows.UTF16ToString(name[:size]), nil
}

func desktopSessionsForSID(sid string) ([]uint32, error) {
	var buffer *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &buffer, &count); err != nil {
		return nil, errDesktopAccount
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	if count > 4096 || (count != 0 && buffer == nil) {
		return nil, errDesktopAccount
	}
	var matches []uint32
	for _, session := range unsafe.Slice(buffer, count) {
		if session.SessionID == 0 || session.State == windows.WTSListen {
			continue
		}
		var token windows.Token
		err := windows.WTSQueryUserToken(session.SessionID, &token)
		if errors.Is(err, windows.ERROR_NO_TOKEN) || errors.Is(err, windows.ERROR_CTX_WINSTATION_NOT_FOUND) {
			continue
		}
		if err != nil {
			return nil, errDesktopAccount
		}
		user, err := token.GetTokenUser()
		token.Close()
		if err != nil {
			return nil, errDesktopAccount
		}
		if user.User.Sid.String() == sid {
			matches = append(matches, session.SessionID)
		}
	}
	return matches, nil
}

func (nativeDesktopAccountAPI) logoff(sid string) error {
	// Disabling an account does not log off its desktop. Confirm termination of
	// the exact token SID, including sessions left by a crashed previous broker.
	deadline := time.Now().Add(10 * time.Second)
	requested := make(map[uint32]bool)
	for {
		sessions, err := desktopSessionsForSID(sid)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			return nil
		}
		for _, id := range sessions {
			if requested[id] {
				continue
			}
			result, _, _ := windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSLogoffSession").Call(0, uintptr(id), 0)
			if result == 0 {
				return errDesktopAccount
			}
			requested[id] = true
		}
		if time.Now().After(deadline) {
			return errDesktopAccount
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func managedDesktopPlatformAllowed() error {
	joined, err := DomainJoined()
	if err != nil || joined || windows.RtlGetVersion().ProductType == 2 {
		return errDesktopAccount
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return errDesktopAccount
	}
	defer key.Close()
	edition, _, err := key.GetStringValue("EditionID")
	if err != nil {
		return errDesktopAccount
	}
	switch edition {
	case "Professional", "ProfessionalN", "ProfessionalEducation", "ProfessionalEducationN", "ProfessionalWorkstation", "ProfessionalWorkstationN", "Enterprise", "EnterpriseN", "EnterpriseS", "EnterpriseSN", "Education", "EducationN", "ServerStandard", "ServerDatacenter":
	default:
		return errDesktopAccount
	}
	installation, _, err := key.GetStringValue("InstallationType")
	if err != nil || (installation != "Client" && installation != "Server") {
		return errDesktopAccount
	}
	return nil
}

// EnableManagedDesktopListener is installation-only and requires explicit RDP
// opt-in. Existing firewall rules may expose the RDP listener; the installer/UI
// say so. This never opens a firewall rule, disables NLA/UAC or overrides GPO.
func EnableManagedDesktopListener() (func() error, error) {
	if !IsSystem() || managedDesktopPlatformAllowed() != nil {
		return nil, errDesktopAccount
	}
	policy, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services`, registry.QUERY_VALUE)
	if err == nil {
		defer policy.Close()
		for name, required := range map[string]uint64{"fDenyTSConnections": 0, "UserAuthentication": 1, "SecurityLayer": 2} {
			value, _, e := policy.GetIntegerValue(name)
			if e == nil && value != required || e != nil && !errors.Is(e, registry.ErrNotExist) {
				return nil, errDesktopAccount
			}
		}
	} else if !errors.Is(err, registry.ErrNotExist) {
		return nil, errDesktopAccount
	}
	_, priorErr := readDesktopListenerJournal()
	if priorErr != nil && !errors.Is(priorErr, os.ErrNotExist) {
		return nil, priorErr
	}
	journal, err := ensureDesktopListenerJournal()
	if err != nil {
		return nil, err
	}
	rollback := func() error { return nil }
	if errors.Is(priorErr, os.ErrNotExist) {
		rollback = RestoreManagedDesktopListener
	}
	for i, setting := range desktopListenerSettings {
		key, e := registry.OpenKey(registry.LOCAL_MACHINE, setting.path, registry.QUERY_VALUE|registry.SET_VALUE)
		if e != nil {
			return nil, errors.Join(errDesktopAccount, rollback())
		}
		current, kind, e := key.GetIntegerValue(setting.name)
		original := journal.Original[i]
		if e == nil && kind == registry.DWORD && current == uint64(setting.value) {
			key.Close()
			continue
		}
		unchanged := e == nil && kind == registry.DWORD && original.Existed && current == uint64(original.Previous) || errors.Is(e, registry.ErrNotExist) && !original.Existed
		if !unchanged {
			key.Close()
			return nil, errors.Join(errDesktopAccount, rollback())
		}
		e = key.SetDWordValue(setting.name, setting.value)
		key.Close()
		if e != nil {
			return nil, errors.Join(errDesktopAccount, rollback())
		}
	}
	manager, err := mgr.Connect()
	if err != nil {
		return nil, errors.Join(errDesktopAccount, rollback())
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("TermService")
	if err != nil {
		return nil, errors.Join(errDesktopAccount, rollback())
	}
	defer service.Close()
	status, err := service.Query()
	if err == nil && status.State != svc.Running {
		err = service.Start()
	}
	if err != nil {
		return nil, errors.Join(errDesktopAccount, rollback())
	}
	return rollback, nil
}

// Persist the original settings before the first write. Restore only values
// that still equal our installed values; preserve later administrator edits.
const desktopListenerRegistry = "SYSTEM\\CurrentControlSet\\Control\\Terminal Server"

var desktopListenerSettings = []struct {
	path, name string
	value      uint32
}{
	{desktopListenerRegistry + "\\WinStations\\RDP-Tcp", "UserAuthentication", 1},
	{desktopListenerRegistry + "\\WinStations\\RDP-Tcp", "SecurityLayer", 2},
	{desktopListenerRegistry, "fDenyTSConnections", 0},
}

type desktopListenerOriginal struct {
	Previous uint32
	Existed  bool
}
type desktopListenerJournal struct {
	Version  int
	Original []desktopListenerOriginal
}

func desktopListenerJournalPath() string { return filepath.Join(DataDir(), "desktop-listener.json") }
func readDesktopListenerJournal() (desktopListenerJournal, error) {
	data, err := ReadFile(desktopListenerJournalPath(), 4096, SystemOnly)
	if err != nil {
		return desktopListenerJournal{}, err
	}
	var journal desktopListenerJournal
	if json.Unmarshal(data, &journal) != nil || journal.Version != 1 || len(journal.Original) != len(desktopListenerSettings) {
		return journal, errDesktopAccount
	}
	return journal, nil
}
func ensureDesktopListenerJournal() (desktopListenerJournal, error) {
	journal, err := readDesktopListenerJournal()
	if err == nil {
		return journal, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return journal, err
	}
	journal.Version = 1
	for _, setting := range desktopListenerSettings {
		key, e := registry.OpenKey(registry.LOCAL_MACHINE, setting.path, registry.QUERY_VALUE)
		if e != nil {
			return journal, errDesktopAccount
		}
		value, kind, e := key.GetIntegerValue(setting.name)
		key.Close()
		if e != nil && !errors.Is(e, registry.ErrNotExist) || e == nil && kind != registry.DWORD {
			return journal, errDesktopAccount
		}
		journal.Original = append(journal.Original, desktopListenerOriginal{Previous: uint32(value), Existed: e == nil})
	}
	data, err := json.Marshal(journal)
	if err == nil {
		err = WriteAtomic(desktopListenerJournalPath(), data, SystemOnly)
	}
	return journal, err
}
func RestoreManagedDesktopListener() error {
	if !IsSystem() {
		return errDesktopAccount
	}
	journal, err := readDesktopListenerJournal()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var failures []error
	for i := len(desktopListenerSettings) - 1; i >= 0; i-- {
		setting, original := desktopListenerSettings[i], journal.Original[i]
		key, e := registry.OpenKey(registry.LOCAL_MACHINE, setting.path, registry.QUERY_VALUE|registry.SET_VALUE)
		if e != nil {
			failures = append(failures, e)
			continue
		}
		current, kind, e := key.GetIntegerValue(setting.name)
		if e == nil && kind == registry.DWORD && current == uint64(setting.value) {
			if original.Existed {
				e = key.SetDWordValue(setting.name, original.Previous)
			} else {
				e = key.DeleteValue(setting.name)
			}
		} else if errors.Is(e, registry.ErrNotExist) {
			e = nil
		}
		key.Close()
		failures = append(failures, e)
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return os.Remove(desktopListenerJournalPath())
}
