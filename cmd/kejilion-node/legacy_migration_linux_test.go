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
	} else {
		t.Logf("isolated migration evidence:\n%s", output)
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
	if err := os.WriteFile("/etc/passwd", []byte("kejilion-node:x:65534:65534::/:/usr/sbin/nologin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/etc/group", []byte("kejilion-node:x:65534:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown("/etc/kejilion-node", 0, 65534); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod("/etc/kejilion-node", 0o750); err != nil {
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
	if asset := os.Getenv("KPANEL_HISTORICAL_NODE_ASSET"); asset != "" {
		testHistoricalReleaseProbe(t, asset)
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
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
	// Future generations must still receive their runtime through the new
	// staging path, not merely disable all version-triggered migrations.
	releaseDir, err := os.MkdirTemp("/tmp", lightReleaseStagingPrefix)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(releaseDir)
	releaseBinary := filepath.Join(releaseDir, name)
	if err := os.WriteFile(releaseBinary, content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, lightUpdateChecksumName), []byte(fmt.Sprintf("%x  %s\n", sum, name)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/usr/local/lib/kejilion-node/update.sh", []byte("#!/bin/bash\n# KPANEL_NODE_RUNTIME_GENERATION=1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	releaseCommand := exec.Command(releaseBinary, "-test.run=^TestLegacyUpdateMigrationStagedHelper$")
	releaseCommand.Env = append(os.Environ(), "KPANEL_UPDATE_MIGRATION_TEST=staged")
	if output, err := releaseCommand.CombinedOutput(); err != nil {
		t.Fatalf("new staged migration: %s %v", output, err)
	}
	if actual, err := os.ReadFile("/usr/local/lib/kejilion-node/update.sh"); err != nil || !bytes.Equal(actual, lightNodeUpdater) {
		t.Fatalf("new staging path did not migrate runtime: %s %v", actual, err)
	}
	// Unsafe destinations fail closed and cannot replace another target.
	path := "/usr/local/lib/kejilion-node/update.sh"
	// The script is published before the new binary. A newer installed runtime
	// is authoritative even when an older staged binary is perfectly verified.
	newer := []byte("#!/bin/bash\n# KPANEL_NODE_RUNTIME_GENERATION=999\nexit 0\n")
	if err := os.WriteFile(path, newer, 0o755); err != nil {
		t.Fatal(err)
	}
	run()
	if actual, err := os.ReadFile(path); err != nil || !bytes.Equal(actual, newer) {
		t.Fatalf("newer installed updater was replaced: %s %v", actual, err)
	}
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

func testHistoricalReleaseProbe(t *testing.T, asset string) {
	content, err := os.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	// Published v1.4.0 amd64 asset, verified against its public SHA256SUMS.
	if runtime.GOARCH != "amd64" || fmt.Sprintf("%x", sha256.Sum256(content)) != "6c91acadf01396268734faa8f0f8be5c4d6989aa8d0d75dcd5df36e2b1cba4f6" {
		t.Fatal("historical asset is not the verified v1.4.0 amd64 release")
	}
	updater := "/usr/local/lib/kejilion-node/update.sh"
	sentinel := []byte("#!/bin/bash\n# KPANEL_NODE_RUNTIME_GENERATION=999\nexit 0\n")
	for _, prefix := range []string{lightUpdateStagingPrefix, lightReleaseStagingPrefix} {
		dir, err := os.MkdirTemp("/tmp", prefix)
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		name := "kejilion-node-linux-amd64"
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(content), name)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(updater, sentinel, 0o755); err != nil {
			t.Fatal(err)
		}
		output, err := exec.Command(path, "version").CombinedOutput()
		if err != nil || !strings.Contains(string(output), "1.4.0 light-v1") {
			t.Fatalf("historical probe: %s %v", output, err)
		}
		actual, err := os.ReadFile(updater)
		if err != nil {
			t.Fatal(err)
		}
		preserved := bytes.Equal(actual, sentinel)
		if preserved != (prefix == lightReleaseStagingPrefix) {
			t.Fatalf("historical asset prefix=%s preserved=%v", prefix, preserved)
		}
		t.Logf("historical v1.4.0 asset prefix=%s installed runtime preserved=%v", prefix, preserved)
	}
	if err := os.Remove(updater); err != nil {
		t.Fatal(err)
	}
}

func TestLightRuntimeGeneration(t *testing.T) {
	for _, tc := range []struct {
		content string
		want    uint64
		invalid bool
	}{
		{"#!/bin/bash\n", 0, false},
		{"# KPANEL_NODE_RUNTIME_GENERATION=2\n", 2, false},
		{"# KPANEL_NODE_RUNTIME_GENERATION=0\n", 0, true},
		{"# KPANEL_NODE_RUNTIME_GENERATION=no\n", 0, true},
		{"# KPANEL_NODE_RUNTIME_GENERATION=2\n# KPANEL_NODE_RUNTIME_GENERATION=3\n", 0, true},
	} {
		got, err := lightRuntimeGeneration([]byte(tc.content))
		if (err != nil) != tc.invalid || got != tc.want {
			t.Fatalf("generation(%q) = %d, %v", tc.content, got, err)
		}
	}
}
