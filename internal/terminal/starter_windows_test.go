//go:build windows

package terminal

import (
	"bytes"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestWindowsPlatformStarterInteractive(t *testing.T) {
	process, err, handled := platformStarter(24, 100)
	if err != nil || !handled {
		t.Fatalf("native starter: handled=%v err=%v", handled, err)
	}
	t.Cleanup(func() { _ = process.Close() })
	done := make(chan struct{})
	var output bytes.Buffer
	var readErr, waitErr error
	go func() { _, readErr = io.Copy(&output, process); waitErr = process.Wait(); close(done) }()
	// Assert executed output absent from the command text; echoed input cannot
	// satisfy either the arithmetic result or its Chinese prefix.
	if _, err := io.WriteString(process, `$value = 6 * 7; Write-Output ('原生终端结果=' + $value); exit 0`+"\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("production Windows starter did not execute and exit")
	}
	if readErr != nil || waitErr != nil || !bytes.Contains(output.Bytes(), []byte("原生终端结果=42")) {
		t.Fatalf("production starter: read=%v wait=%v output=%q", readErr, waitErr, output.Bytes())
	}
}
