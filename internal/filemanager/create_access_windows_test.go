//go:build windows

package filemanager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
)

func testAccessSDDL(t *testing.T, everyone bool, directory bool) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	sddl := "D:P(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")"
	if everyone {
		sddl += "(A;" + inherit + ";FRFX;;;WD)"
	}
	return sddl
}

func setTestDACL(t *testing.T, name, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func testDACL(t *testing.T, name string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	// AUTO_INHERITED records how Windows last processed inheritance; creating
	// with an already protected DACL clears it without changing any ACE/right.
	return strings.Replace(sd.String(), "D:PAI(", "D:P(", 1)
}

// This token keeps the process identity but additionally requires a World ACE.
// It cannot read the private fixture, while it can read a file inheriting the
// destination's broad read grant. No account, service or machine ACL is changed.
func testWorldRestrictedReader(t *testing.T) windows.Token {
	t.Helper()
	var original windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY, &original); err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	world, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	sid := windows.SIDAndAttributes{Sid: world}
	var restricted windows.Token
	ok, _, err := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken").Call(uintptr(original), 0, 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&sid)), uintptr(unsafe.Pointer(&restricted)))
	if ok == 0 {
		t.Fatal(err)
	}
	defer restricted.Close()
	var reader windows.Token
	if err := windows.DuplicateTokenEx(restricted, windows.TOKEN_IMPERSONATE|windows.TOKEN_QUERY, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &reader); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

func openAsRestrictedReader(token windows.Token, name string) (*os.File, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.SetThreadToken(nil, token); err != nil {
		return nil, err
	}
	h, openErr := windows.CreateFile(ptr, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err := windows.RevertToSelf(); err != nil {
		panic(err) // Never return an impersonating OS thread to the Go scheduler.
	}
	if openErr != nil {
		return nil, openErr
	}
	return os.NewFile(uintptr(h), name), nil
}

func TestWindowsReplacementTempUsesSourceDACLAtCreation(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	setTestDACL(t, c, testAccessSDDL(t, true, true))
	private := filepath.Join(c, "private.txt")
	mustWrite(t, private, "before")
	setTestDACL(t, private, testAccessSDDL(t, false, false))
	reader := testWorldRestrictedReader(t)

	// Prove the hostile inherited-ACL fixture is meaningful: a reader opened
	// before tightening a DACL continues to see bytes written afterwards.
	unsafePath := filepath.Join(c, "inherited.txt")
	mustWrite(t, unsafePath, "")
	retained, err := openAsRestrictedReader(reader, unsafePath)
	if err != nil {
		t.Fatalf("broad-parent read fixture: %v", err)
	}
	defer retained.Close()
	setTestDACL(t, unsafePath, testAccessSDDL(t, false, false))
	mustWrite(t, unsafePath, "later bytes")
	bytes, err := io.ReadAll(retained)
	if err != nil || string(bytes) != "later bytes" {
		t.Fatalf("retained handle fixture: %q %v", bytes, err)
	}

	source, err := m.rootFS.Open("C/private.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	temp, virtual, err := m.createTempWithSourceAccess("/C", ".kpanel-edit-", source)
	if err != nil {
		t.Fatal(err)
	}
	defer temp.Close()
	native := filepath.Join(c, filepath.Base(virtual))
	if got, want := testDACL(t, native), testDACL(t, private); got != want {
		t.Fatalf("creation DACL %s, want %s", got, want)
	}
	if handle, err := openAsRestrictedReader(reader, native); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if handle != nil {
			handle.Close()
		}
		t.Fatalf("reader obtained a pre-tightening handle: %v", err)
	}
	if _, err := temp.WriteString("private bytes"); err != nil {
		t.Fatal(err)
	}

	// Exercise both public overwrite paths, not just the creation helper.
	want := testDACL(t, private)
	entry, err := m.Stat("/C/private.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteText(context.Background(), entry.Path, contract.FileWriteRequest{Content: "saved", ExpectedResourceVersion: entry.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Upload(context.Background(), "/C", "private.txt", strings.NewReader("uploaded"), 8, true); err != nil {
		t.Fatal(err)
	}
	if got := testDACL(t, private); got != want {
		t.Fatalf("overwrite DACL %s, want %s", got, want)
	}
}

func TestWindowsCrossVolumeMovesAndTrashRetainDACL(t *testing.T) {
	// Distinct logical volumes force EXDEV even on a one-volume test host.
	m, c, d := windowsVolumeManager(t)
	setTestDACL(t, c, testAccessSDDL(t, true, true))
	setTestDACL(t, d, testAccessSDDL(t, true, true))
	ctx := context.Background()
	reader := testWorldRestrictedReader(t)
	for _, action := range []string{"move", "trash_restore"} {
		t.Run(action, func(t *testing.T) {
			base := filepath.Join(d, action)
			mustMkdirAll(t, filepath.Join(base, "nested"))
			mustWrite(t, filepath.Join(base, "nested", "private.txt"), "private payload")
			setTestDACL(t, base, testAccessSDDL(t, false, true))
			setTestDACL(t, filepath.Join(base, "nested"), testAccessSDDL(t, false, true))
			setTestDACL(t, filepath.Join(base, "nested", "private.txt"), testAccessSDDL(t, false, false))
			want := map[string]string{}
			for _, relative := range []string{".", "nested", filepath.Join("nested", "private.txt")} {
				want[relative] = testDACL(t, filepath.Join(base, relative))
			}
			input := contract.FileActionRequest{Action: "move", Sources: []string{"/D/" + action}, Target: "/C"}
			if action == "trash_restore" {
				input.Action = "trash"
			}
			result, err := m.Action(ctx, input)
			if err != nil || len(result.Failed) != 0 || len(result.Succeeded) != 1 {
				t.Fatalf("initial move: %+v %v", result, err)
			}
			if action == "trash_restore" {
				if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("trash did not remove source")
				}
				result, err = m.Action(ctx, contract.FileActionRequest{Action: "trash_restore", TrashIDs: []string{filepath.Base(result.Succeeded[0].Destination)}})
				if err != nil || len(result.Failed) != 0 || len(result.Succeeded) != 1 {
					t.Fatalf("restore: %+v %v", result, err)
				}
			} else {
				base = filepath.Join(c, action)
			}
			for relative, expected := range want {
				if got := testDACL(t, filepath.Join(base, relative)); got != expected {
					t.Errorf("%s DACL %s, want %s", relative, got, expected)
				}
			}
			private := filepath.Join(base, "nested", "private.txt")
			if file, err := openAsRestrictedReader(reader, private); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				if file != nil {
					file.Close()
				}
				t.Fatalf("move exposed private payload: %v", err)
			}
			if data, err := os.ReadFile(private); err != nil || string(data) != "private payload" {
				t.Fatalf("move lost payload: %q %v", data, err)
			}
		})
	}
}

func TestWindowsOrdinaryCopyKeepsDestinationInheritance(t *testing.T) {
	m, c, d := windowsVolumeManager(t)
	setTestDACL(t, c, testAccessSDDL(t, true, true))
	private := filepath.Join(d, "copy.txt")
	mustWrite(t, private, "copied")
	setTestDACL(t, private, testAccessSDDL(t, false, false))
	result, err := m.Action(context.Background(), contract.FileActionRequest{Action: "copy", Sources: []string{"/D/copy.txt"}, Target: "/C"})
	if err != nil || len(result.Failed) != 0 || len(result.Succeeded) != 1 {
		t.Fatalf("copy: %+v %v", result, err)
	}
	file, err := openAsRestrictedReader(testWorldRestrictedReader(t), filepath.Join(c, "copy.txt"))
	if err != nil {
		t.Fatalf("ordinary copy unexpectedly lost destination inheritance: %v", err)
	}
	file.Close()
}
