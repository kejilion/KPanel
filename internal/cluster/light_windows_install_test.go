package cluster

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWindowsEnrollmentCommandPinsAndVerifiesBeforeExecution(t *testing.T) {
	s := &Service{panelVersion: "1.25.0-rc.3"}
	command, err := s.lightEnrollmentCommand("windows", "kpl1.test", "one'; $(Write-Error 'bad')")
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		powershellSingleQuote("https://github.com/kejilion/KPanel/releases/download/v1.25.0-rc.3/install-windows.ps1"),
		powershellSingleQuote("https://github.com/kejilion/KPanel/releases/download/v1.25.0-rc.3/SHA256SUMS"),
		"Assert-BootstrapDirectory $parent.FullName $true", "Assert-BootstrapDirectory $stage $false",
		"O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", "$lines.Count -ne 1", "Get-FileHash -LiteralPath $script", "65536", "1048576",
		"-Version " + powershellSingleQuote("v1.25.0-rc.3"), "-Name " + powershellSingleQuote("one'; $(Write-Error 'bad')"), "GetFolderPath('ProgramFiles')",
	} {
		if !strings.Contains(command, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	if strings.ContainsAny(command, "\r\n") || strings.Contains(command, "Invoke-Expression") || strings.Contains(command, "Authenticode") || strings.Contains(command, "-Publisher") || !strings.Contains(command, "-NoProfile -ExecutionPolicy Bypass -File $script") || strings.Contains(command, " -Token ") || strings.Index(command, "Get-FileHash") > strings.Index(command, "-File $script") {
		t.Fatal("bootstrap must be one line and verify before execution")
	}
	for _, invalid := range []string{"dev", "01.2.3", "1.2.3-rc.0", "1.2.3-dev", "1000000.2.3", "1.2.3;whoami"} {
		s.panelVersion = invalid
		if _, err := s.lightEnrollmentCommand("windows", "", ""); !errors.Is(err, ErrWindowsInstallerUnavailable) {
			t.Fatalf("invalid %q: %v", invalid, err)
		}
	}
	if _, err := s.lightEnrollmentCommand("macos", "", ""); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatal(err)
	}
}

func TestWindowsEnrollmentCannotConsumePolicyBeforeReleaseVersionAvailable(t *testing.T) {
	now := time.Now().UTC()
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	s.panelVersion = "dev"
	if _, err := s.CreateLightEnrollmentForPlatform(s.publicURL, "", "windows"); !errors.Is(err, ErrWindowsInstallerUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows"}); !errors.Is(err, ErrWindowsInstallerUnavailable) {
		t.Fatal(err)
	}
	if got := s.LightBatchEnrollments(); got.Total != 0 {
		t.Fatal("failed generation left active policy")
	}
	s.panelVersion = "1.25.0-rc.3"
	enrollment, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows"})
	if err != nil || enrollment.Platform != "windows" || !strings.Contains(enrollment.Command, "$env:KPANEL_NODE_TOKEN=") {
		t.Fatalf("%#v, %v", enrollment, err)
	}
}

func TestWindowsDesktopEnrollmentRequiresExplicitOptIn(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	s.panelVersion = "1.25.0-rc.3"
	defaultCommand, err := s.lightEnrollmentCommand("windows", "kpl1.test", "")
	if err != nil || strings.Contains(defaultCommand, "-EnableDesktop") {
		t.Fatal("default enabled desktop", err)
	}
	single, err := s.CreateLightEnrollmentWithDesktop(s.publicURL, "", "windows", true)
	if err != nil || !strings.Contains(single.Command, "-EnableDesktop") {
		t.Fatal("single did not opt in", err)
	}
	batch, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows", EnableDesktop: true})
	if err != nil || !strings.Contains(batch.Command, "-EnableDesktop") {
		t.Fatal("batch did not opt in", err)
	}
	if _, err := s.lightEnrollmentCommandWithDesktop("linux", "", "", true); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatal("Linux accepted desktop", err)
	}
}

func TestWindowsBootstrapPowerShellSyntax(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell parser is exercised on native Windows CI")
	}
	command, err := (&Service{panelVersion: "1.25.0-rc.3"}).lightEnrollmentCommand("windows", "kpl1.dummy", "dummy name")
	if err != nil {
		t.Fatal(err)
	}
	// Parse only: never execute an installer or mutate host services.
	parser := "$tokens=$null;$errors=$null;$null=[Management.Automation.Language.Parser]::ParseInput([Console]::In.ReadToEnd(),[ref]$tokens,[ref]$errors);if($errors.Count){$errors|%{[Console]::Error.WriteLine($_.Message)};exit 1}"
	cmd := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", parser)
	cmd.Stdin = strings.NewReader(command)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap syntax: %v\n%s", err, output)
	}
}
