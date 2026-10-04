//go:build windows

package windowsnode

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestChecksumRejectsTamperAndUntrustedFile(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("elevated temporary ACL fixture required")
	}
	path := filepath.Join(t.TempDir(), "unsigned.exe")
	data := []byte("unsigned temporary fixture; never executed")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sd, err := descriptor(ProgramRead, false)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, _ := sd.Owner()
	acl, _, _ := sd.DACL()
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	expected := hex.EncodeToString(digest[:])
	if err := VerifyChecksum(path, expected); err != nil {
		t.Fatal(err)
	}
	t.Run("reparse", func(t *testing.T) {
		link := filepath.Join(filepath.Dir(path), "link.exe")
		if err := os.Symlink(path, link); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
		if VerifyChecksum(link, expected) == nil {
			t.Fatal("reparse point accepted despite matching target digest")
		}
	})
	for _, bad := range []string{"", strings.ToUpper(expected), strings.Repeat("0", 64)} {
		if VerifyChecksum(path, bad) == nil {
			t.Fatal("invalid checksum accepted")
		}
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if VerifyChecksum(path, expected) == nil {
		t.Fatal("tampered file accepted")
	}
	unsafeSD, err := windows.SecurityDescriptorFromString("O:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	unsafeACL, _, _ := unsafeSD.DACL()
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, unsafeACL, nil); err != nil {
		t.Fatal(err)
	}
	tampered := sha256.Sum256([]byte("tampered"))
	if VerifyChecksum(path, hex.EncodeToString(tampered[:])) == nil {
		t.Fatal("writable-by-users file accepted despite matching checksum")
	}
}
