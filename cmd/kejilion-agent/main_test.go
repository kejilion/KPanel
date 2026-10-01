package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/version"
)

func TestValidateAgentHealthAcceptsReadyAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "ok",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
	})
	if err != nil {
		t.Fatalf("validateAgentHealth() error = %v", err)
	}
}

func TestValidateAgentHealthAcceptsMissingOptionalWebRoot(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "degraded",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		Reasons:         []string{"web_root_unavailable"},
	})
	if err != nil {
		t.Fatalf("validateAgentHealth() error = %v", err)
	}
}

func TestValidateAgentHealthRejectsDegradedAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "degraded",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		Reasons:         []string{"docker_unavailable"},
	})
	if err == nil || !strings.Contains(err.Error(), "docker_unavailable") {
		t.Fatalf("validateAgentHealth() error = %v, want degraded reason", err)
	}
}

func TestValidateAgentHealthRejectsReadOnlyAgent(t *testing.T) {
	err := validateAgentHealth(contract.AgentHealth{
		Status:          "ok",
		Version:         version.Version,
		ProtocolVersion: version.ProtocolVersion,
		ReadOnly:        true,
	})
	if err == nil {
		t.Fatal("validateAgentHealth() accepted a read-only Agent")
	}
}

func TestDiskWorkerSubcommandsAcceptOnlyFixedFlags(t *testing.T) {
	for name, arguments := range map[string][]string{
		"inspect positional": {"disk-inspect", "--state-dir", "/tmp/state", "unexpected"},
		"inspect device":     {"disk-inspect", "--device", "/dev/sdb"},
		"run missing id":     {"disk-run", "--state-dir", "/tmp/state"},
		"run device":         {"disk-run", "--state-dir", "/tmp/state", "--id", strings.Repeat("a", 32), "--device", "/dev/sdb"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(arguments); err == nil {
				t.Fatal("unsafe or incomplete worker arguments were accepted")
			}
		})
	}
}

func TestCanonicalStateDirResolvesLinkedAncestors(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	volume := filepath.Join(base, "vol1")
	if err := os.MkdirAll(filepath.Join(volume, "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "docker")
	if err := os.Symlink(volume, linked); err != nil {
		t.Skip("symbolic links unavailable:", err)
	}
	got, err := canonicalStateDir(filepath.Join(linked, "agent"))
	if err != nil || got != filepath.Join(volume, "agent") {
		t.Fatal(got, err)
	}
	if got, err := canonicalStateDir("relative/state"); err != nil || got != "relative/state" {
		t.Fatalf("relative directory changed: %q %v", got, err)
	}
}
