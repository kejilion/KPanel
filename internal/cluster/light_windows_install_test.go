package cluster

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWindowsEnrollmentCommandPinsAndVerifiesBeforeExecution(t *testing.T) {
	s := &Service{panelVersion: "1.24.0-rc.10", windowsNodePublisher: "CN=KPanel Test, O=Publisher", windowsNodeProfileOID: "1.3.6.1.4.1.311.97.1.1"}
	command, err := s.lightEnrollmentCommand("windows", "kpl1.test", "one'; $(Write-Error 'bad')")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		powershellSingleQuote("https://github.com/kejilion/KPanel/releases/download/v1.24.0-rc.10/install-windows.ps1"), "Get-AuthenticodeSignature -LiteralPath $script",
		"$signature.Status -ne 'Valid'", "-cne " + powershellSingleQuote(s.windowsNodePublisher), "-notin $oids",
		"-Version " + powershellSingleQuote("v1.24.0-rc.10"), "-Name " + powershellSingleQuote("one'; $(Write-Error 'bad')"), "GetFolderPath('ProgramFiles')",
	} {
		if !strings.Contains(command, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	if strings.ContainsAny(command, "\r\n") || strings.Contains(command, "Invoke-Expression") || strings.Index(command, "Get-AuthenticodeSignature") > strings.Index(command, "& $script") {
		t.Fatal("bootstrap must be one line and verify before execution")
	}
	s.windowsNodePublisher = ""
	if _, err := s.lightEnrollmentCommand("windows", "", ""); !errors.Is(err, ErrWindowsInstallerUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.lightEnrollmentCommand("macos", "", ""); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatal(err)
	}
}

func TestWindowsEnrollmentCannotConsumePolicyBeforeSigningConfiguration(t *testing.T) {
	now := time.Now().UTC()
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	if _, err := s.CreateLightEnrollmentForPlatform(s.publicURL, "", "windows"); !errors.Is(err, ErrWindowsInstallerUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows"}); !errors.Is(err, ErrWindowsInstallerUnavailable) {
		t.Fatal(err)
	}
	if got := s.LightBatchEnrollments(); got.Total != 0 {
		t.Fatal("failed generation left active policy")
	}
	s.windowsNodePublisher = "CN=Test"
	enrollment, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows"})
	if err != nil || enrollment.Platform != "windows" || !strings.Contains(enrollment.Command, "-Token ") {
		t.Fatalf("%#v, %v", enrollment, err)
	}
}
