package cluster

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWindowsLightNodeAndDesktopPathsAreRemoved(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	for name, action := range map[string]func() error{
		"single Windows platform": func() error { return validateLightPlatformChoice("windows", false) },
		"single desktop opt-in":   func() error { return validateLightPlatformChoice("linux", true) },
		"batch Windows platform": func() error {
			_, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{Platform: "windows"})
			return err
		},
		"batch desktop opt-in": func() error {
			_, err := s.CreateLightBatchEnrollment(CreateLightBatchEnrollmentInput{EnableDesktop: true})
			return err
		},
		"single Windows enrollment": func() error {
			_, err := s.EnrollLightNode("198.51.100.1", LightEnrollRequest{Platform: "windows"})
			return err
		},
		"batch Windows enrollment": func() error {
			_, _, err := s.EnrollLightNodeBatch("198.51.100.1", s.publicURL,
				LightEnrollRequest{Platform: "windows", AttemptID: strings.Repeat("a", 32)})
			return err
		},
		"Windows report": func() error {
			return validateLightPlatform(LightReportRequest{Platform: "windows"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, ErrLightPlatformUnsupported) {
				t.Fatalf("withdrawn feature was not rejected: %v", err)
			}
		})
	}
	if err := validateLightPlatform(LightReportRequest{DesktopUnavailableReason: "unsupported"}); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("legacy desktop report error = %v, want ErrProtocolMismatch", err)
	}
	if got := s.LightBatchEnrollments(); got.Total != 0 {
		t.Fatalf("rejected Windows request left batch enrollment: %#v", got)
	}
}

func TestLinuxLightNodeEnrollmentRemainsUnchanged(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	command, err := s.lightEnrollmentCommand("kpl1.token", "linux node")
	if err != nil || !strings.Contains(command, "kpanel node join 'kpl1.token'") || !strings.Contains(command, "--name 'linux node'") {
		t.Fatalf("Linux command = %q, %v", command, err)
	}
	if err := validateLightPlatformChoice("macos", false); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("unknown platform error = %v", err)
	}
}
