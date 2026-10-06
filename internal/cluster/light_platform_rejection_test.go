package cluster

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUnsupportedPlatformsAreRejectedBeforeEnrollment(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	for name, action := range map[string]func() error{
		"single enrollment": func() error {
			_, err := s.EnrollLightNode("198.51.100.1", LightEnrollRequest{Platform: "unsupported"})
			return err
		},
		"batch enrollment": func() error {
			_, _, err := s.EnrollLightNodeBatch("198.51.100.1", s.publicURL,
				LightEnrollRequest{Platform: "unsupported", AttemptID: strings.Repeat("a", 32)})
			return err
		},
		"report": func() error {
			return validateLightPlatform(LightReportRequest{Platform: "unsupported"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, ErrLightPlatformUnsupported) {
				t.Fatalf("unsupported platform was not rejected: %v", err)
			}
		})
	}
	if got := s.LightBatchEnrollments(); got.Total != 0 {
		t.Fatalf("rejected request left batch enrollment state: %#v", got)
	}
}

func TestLinuxLightNodeEnrollmentCommandRemainsAvailable(t *testing.T) {
	s := newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	command, err := s.lightEnrollmentCommand("kpl1.token", "linux node")
	if err != nil || !strings.Contains(command, "kpanel node join 'kpl1.token'") || !strings.Contains(command, "--name 'linux node'") {
		t.Fatalf("Linux command = %q, %v", command, err)
	}
}
