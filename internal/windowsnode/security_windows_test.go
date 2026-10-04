//go:build windows

package windowsnode

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestLoginSnapshotParentUsesNarrowDirectoryPolicy(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires an elevated process to set the directory owner")
	}
	root := t.TempDir()
	loginDirectory := filepath.Join(root, "login")
	descriptors := map[string]*windows.SECURITY_DESCRIPTOR{}
	for _, item := range []struct {
		name, sddl string
	}{
		{"login", ""},
		{"sibling", ""},
		{"untrusted", "O:BAD:P(A;;FA;;;SY)(A;;FA;;;" + ServiceSID(LoginService) + ")(A;;FW;;;BU)"},
	} {
		path := filepath.Join(root, item.name)
		sd, err := descriptor(LoginSnapshot, true)
		if item.sddl != "" {
			sd, err = windows.SecurityDescriptorFromString(item.sddl)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		name, _ := windows.UTF16PtrFromString(path)
		// Retain READ_CONTROL before sealing the fixture: an administrator is
		// deliberately not a reader of the production LoginSnapshot descriptor.
		h, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(h)
		owner, _, _ := sd.Owner()
		acl, _, _ := sd.DACL()
		if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		actual, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		descriptors[strings.ToLower(path)] = actual
	}
	for _, test := range []struct {
		name, path, allowedDirectory string
		valid                        bool
	}{
		{"exact login parent", loginDirectory, loginDirectory, true},
		{"case insensitive login parent", strings.ToUpper(loginDirectory), loginDirectory, true},
		{"sibling cannot inherit login writer trust", filepath.Join(root, "sibling"), loginDirectory, false},
		{"ancestor cannot inherit login writer trust", loginDirectory, filepath.Join(loginDirectory, "child"), false},
		{"descendant cannot inherit login writer trust", loginDirectory, root, false},
		{"production KnownFolder does not trust fixture", loginDirectory, RuntimePath("login"), false},
		{"untrusted additional writer rejected", filepath.Join(root, "untrusted"), filepath.Join(root, "untrusted"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			access, ancestor := parentPolicy(test.path, test.allowedDirectory)
			err := validateDescriptor(descriptors[strings.ToLower(test.path)], access, ancestor)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}

func TestSystemOnlyHandoffIsSealedAtCreation(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() || IsSystem() {
		t.Skip("requires an elevated non-SYSTEM test process")
	}
	path := filepath.Join(t.TempDir(), "handoff.json")
	sd, err := descriptor(SystemOnly, false)
	if err != nil {
		t.Fatal(err)
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(h), path)
	if _, err := file.Write([]byte("test enrollment handoff")); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if file, err := os.Open(path); err == nil {
		file.Close()
		t.Fatal("administrator could directly reopen SYSTEM-only handoff")
	}
}

func TestServiceSIDMatchesSCMIdentity(t *testing.T) {
	if got := ServiceSID("TrustedInstaller"); got != "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464" {
		t.Fatalf("unexpected service SID %s", got)
	}
	if ServiceSID(TelemetryService) != ServiceSID("kejilionnode") {
		t.Fatal("SID must be case insensitive")
	}
}
func TestProtectedDescriptorBoundaries(t *testing.T) {
	tests := []struct {
		name, sddl      string
		access          Access
		ancestor, valid bool
	}{
		{"system secret", "O:SYD:P(A;;FA;;;SY)", SystemOnly, false, true},
		{"administrator owns sealed secret", "O:BAD:P(A;;FA;;;SY)", SystemOnly, false, true},
		{"administrator cannot read key", "O:SYD:P(A;;FA;;;SY)(A;;FR;;;BA)", SystemOnly, false, false},
		{"users cannot read key", "O:SYD:P(A;;FA;;;SY)(A;;FR;;;BU)", SystemOnly, false, false},
		{"users cannot write program", "O:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)", ProgramRead, false, false},
		{"users may execute program", "O:BAD:P(A;;FA;;;BA)(A;;FRFX;;;BU)", ProgramRead, false, true},
		{"node reads telemetry", "O:SYD:P(A;;FA;;;SY)(A;;FR;;;" + ServiceSID(TelemetryService) + ")", TelemetryRead, false, true},
		{"node cannot write telemetry", "O:SYD:P(A;;FA;;;SY)(A;;FW;;;" + ServiceSID(TelemetryService) + ")", TelemetryRead, false, false},
		{"login writer", "O:" + ServiceSID(LoginService) + "D:P(A;;FA;;;SY)(A;;FA;;;" + ServiceSID(LoginService) + ")(A;;FR;;;" + ServiceSID(TelemetryService) + ")", LoginSnapshot, false, true},
		{"wrong login writer", "O:SYD:P(A;;FA;;;SY)(A;;FA;;;" + ServiceSID(LoginService) + ")", TelemetryRead, false, false},
		{"unsafe parent delete child", "O:SYD:P(A;;FA;;;SY)(A;;0x40;;;BU)", StateDirectory, true, false},
		{"null DACL", "O:SYD:NO_ACCESS_CONTROL", SystemOnly, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(test.sddl)
			if err != nil {
				t.Fatal(err)
			}
			err = validateDescriptor(sd, test.access, test.ancestor)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}
func TestUnsignedExecutableFailsClosed(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if VerifySignature(exe, TrustPolicy{Publisher: "CN=KPanel Test"}) == nil {
		t.Fatal("unsigned executable accepted")
	}
	if VerifySignature(exe, TrustPolicy{}) == nil {
		t.Fatal("missing publisher accepted")
	}
}
func TestNativeReadOnlyPlatformFacts(t *testing.T) {
	if DataDir() == "" || InstallDir() == "" {
		t.Fatal("known folder unavailable")
	}
	if _, err := DomainJoined(); err != nil {
		t.Fatal(err)
	}
}

// An optional already-installed, signed executable exercises the positive
// WinTrust signer extraction without downloading or trusting a test CA.
func TestNativeSignedFixture(t *testing.T) {
	path, publisher := os.Getenv("KPANEL_TEST_SIGNED_EXE"), os.Getenv("KPANEL_TEST_SIGNED_PUBLISHER")
	if path == "" || publisher == "" {
		t.Skip("no signed local fixture specified")
	}
	if err := VerifySignature(path, TrustPolicy{Publisher: publisher}); err != nil {
		t.Fatal(err)
	}
	if VerifySignature(path, TrustPolicy{Publisher: "CN=Wrong publisher"}) == nil {
		t.Fatal("valid chain allowed the wrong publisher")
	}
	if VerifySignature(path, TrustPolicy{Publisher: publisher, ProfileOID: "1.2.3.4.5.6.7.8.9"}) == nil {
		t.Fatal("valid chain allowed the wrong signing profile")
	}
}
