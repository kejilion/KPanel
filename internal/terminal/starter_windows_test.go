//go:build windows

package terminal

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsShellUsesTrustedInboxPath(t *testing.T) {
	shell, system, err := windowsShell()
	if err != nil {
		t.Fatal(err)
	}
	if shell != filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`) {
		t.Fatalf("unexpected shell %s", shell)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SystemRoot", t.TempDir())
	t.Setenv("PSModulePath", t.TempDir())
	actual, _, err := windowsShell()
	if err != nil || actual != shell {
		t.Fatalf("inherited environment selected shell: %s %v", actual, err)
	}
	file := filepath.Join(t.TempDir(), "powershell.exe")
	if err := os.WriteFile(file, []byte("untrusted"), 0600); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;GW;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if err := trustedWindowsShellPath(file); err == nil {
		t.Fatal("accepted user-owned executable")
	}
}
