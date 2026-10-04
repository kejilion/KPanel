package main

import (
	"slices"
	"testing"
)

func TestWindowsDesktopOptInDoesNotExpandOtherCapabilities(t *testing.T) {
	if _, err := installationCapabilities(windowsInstallRequest{Capabilities: "monitoring,desktop,desktop-managed"}); err == nil {
		t.Fatal("managed administrator admitted without explicit installation opt-in")
	}
	for _, enabled := range []bool{false, true} {
		caps, err := installationCapabilities(windowsInstallRequest{Capabilities: "monitoring", EnableDesktop: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(caps, "desktop") != enabled || slices.Contains(caps, "terminal") || slices.Contains(caps, "files") || slices.Contains(caps, "login") {
			t.Fatalf("explicit desktop changed unrelated capabilities: %v", caps)
		}
	}
}
