//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestConfigRewritePreservesUnprivilegedServiceAccess(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise the real service UID/GID boundary")
	}
	directory, err := os.MkdirTemp("/tmp", "kpanel-config-access-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "node.json")
	if err := os.WriteFile(path, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 65534); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigAtomic(path, nodeConfig{SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if info.Mode().Perm() != 0o640 || stat.Uid != 0 || stat.Gid != 65534 {
		t.Fatalf("rewrite changed service access: mode=%o uid=%d gid=%d", info.Mode().Perm(), stat.Uid, stat.Gid)
	}
	command := exec.Command("/bin/cat", path)
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged service cannot read rewritten config: %v %s", err, output)
	}
}
