//go:build linux

package hostpty

import (
	"syscall"
	"testing"
)

func TestPTYChildUsesSessionAndParentDeathSignal(t *testing.T) {
	// Keep this assertion next to the platform implementation: direct OpenRC
	// terminals must not outlive the Agent if graceful CloseAll cannot run.
	attributes := ptyProcessAttributes()
	if !attributes.Setsid || !attributes.Setctty || attributes.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("unsafe PTY process attributes: %#v", attributes)
	}
}
