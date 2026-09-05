//go:build linux

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestLegacyUpdateMigrationInMountNamespace(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated Linux root")
	}
	if os.Getenv("KPANEL_UPDATE_MIGRATION_TEST") == "namespace" {
		testLegacyMigrationNamespace(t)
		return
	}
	if output, err := exec.Command("unshare", "--mount", "true").CombinedOutput(); err != nil {
		t.Skipf("mount namespace unavailable: %v %s", err, output)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("unshare", "--mount", executable, "-test.run=^TestLegacyUpdateMigrationInMountNamespace$", "-test.v")
	command.Env = append(os.Environ(), "KPANEL_UPDATE_MIGRATION_TEST=namespace")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated migration: %v\n%s", err, output)
	}
}

func TestLegacyUpdateMigrationStagedHelper(t *testing.T) {
	if os.Getenv("KPANEL_UPDATE_MIGRATION_TEST") != "staged" {
		t.Skip("subprocess helper")
	}
	err := maybeMigrateLegacySSHLoginInstall()
	if os.Getenv("KPANEL_UPDATE_EXPECT_ERROR") == "1" {
		if err == nil {
			t.Fatal("unsafe runtime target was accepted")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}

func testLegacyMigrationNamespace(t *testing.T) {
	// These mounts exist only in the child namespace. No host units/configuration
	// are written, and systemctl is replaced by a root-owned recording fixture.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/etc", "/usr/local/lib"} {
		if err := syscall.Mount("tmpfs", path, "tmpfs", 0, "mode=755"); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/etc/systemd/system", "/etc/kejilion-node", "/usr/local/lib/kejilion-node"} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(defaultConfigPath, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp("/tmp", lightUpdateStagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	logPath := filepath.Join(directory, "systemctl.log")
	stub := filepath.Join(directory, "systemctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+logPath+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mount(stub, "/usr/bin/systemctl", "", syscall.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	// An ordinary executable must not install anything through the version bridge.
	if err := maybeMigrateLegacySSHLoginInstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("ordinary binary performed a migration")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	name := "kejilion-node-linux-" + runtime.GOARCH
	staged := filepath.Join(directory, name)
	if err := os.WriteFile(staged, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := filepath.Join(directory, "SHA256SUMS")
	run := func() {
		command := exec.Command(staged, "-test.run=^TestLegacyUpdateMigrationStagedHelper$")
		command.Env = append(os.Environ(), "KPANEL_UPDATE_MIGRATION_TEST=staged")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("staged migration: %v %s", err, output)
		}
	}
	if err := os.WriteFile(manifest, []byte(strings.Repeat("0", 64)+"  "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run()
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("invalid checksum performed a migration")
	}
	if err := os.WriteFile(manifest, []byte(fmt.Sprintf("%x  %s\n", sum, name)), 0o600); err != nil {
		t.Fatal(err)
	}
	run()
	for path, expected := range map[string][]byte{
		"/usr/local/lib/kejilion-node/update.sh":           lightNodeUpdater,
		"/etc/systemd/system/kejilion-node-update.service": lightNodeUpdateService,
		"/etc/systemd/system/kejilion-node-update.timer":   lightNodeUpdateTimer,
	} {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("migrated template mismatch: %s (%v)", path, err)
		}
	}
	log, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(log), "restart --no-block kejilion-node-update.timer") {
		t.Fatalf("timer migration missing: %q %v", log, err)
	}
	// Unsafe destinations fail closed and cannot replace another target.
	path := "/usr/local/lib/kejilion-node/update.sh"
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(defaultConfigPath, path); err != nil {
		t.Fatal(err)
	}
	if err := installLightNodeUpdateIntegration(); err == nil {
		t.Fatal("migration accepted symlink updater")
	}
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(staged, "-test.run=^TestLegacyUpdateMigrationStagedHelper$")
	command.Env = append(os.Environ(), "KPANEL_UPDATE_MIGRATION_TEST=staged", "KPANEL_UPDATE_EXPECT_ERROR=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("partial migration: %v %s", err, output)
	}
	log, err = os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(log), "enable kejilion-node-ssh-login.service") {
		t.Fatalf("update failure blocked existing SSH migration: %q %v", log, err)
	}
}
