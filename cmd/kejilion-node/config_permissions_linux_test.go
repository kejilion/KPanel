//go:build linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestLegacyConfigRepairBeforeFirstRestart(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated Linux root")
	}
	for _, mode := range []os.FileMode{0o600, 0o640, 0o666} {
		t.Run(mode.String(), func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chown(directory, 0, 65534); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(directory, 0o750); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "node.json")
			content := []byte(`{"reportingKey":"unchanged","nodeId":"unchanged"}`)
			if err := os.WriteFile(path, content, mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			err := repairLegacyNodeConfigAccessAt(path, 65534)
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			actual, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(actual, content) {
				t.Fatal("identity content changed")
			}
			if mode == 0o666 {
				if err == nil || info.Mode().Perm() != mode {
					t.Fatal("unsafe configuration was widened")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			stat := info.Sys().(*syscall.Stat_t)
			if stat.Uid != 0 || stat.Gid != 65534 || info.Mode().Perm() != 0o640 {
				t.Fatal("installer access not restored")
			}
			// No second update or JSON rewrite is needed for the next service start.
			// t.TempDir ancestors may be 0700; test real access via the opened fd.
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			command := exec.Command("/bin/cat", "/proc/self/fd/3")
			command.ExtraFiles = []*os.File{file}
			command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
			if output, err := command.CombinedOutput(); err != nil || !bytes.Equal(output, content) {
				t.Fatalf("telemetry cannot read: %v %s", err, output)
			}
		})
	}
}

func TestLegacyConfigRepairRejectsSymlinksAndUnexpectedGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated Linux root")
	}
	directory := t.TempDir()
	if err := os.Chown(directory, 0, 65534); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "node.json")
	target := filepath.Join(directory, "private.json")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := repairLegacyNodeConfigAccessAt(path, 65534); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 65533); err != nil {
		t.Fatal(err)
	}
	if err := repairLegacyNodeConfigAccessAt(path, 65534); err == nil {
		t.Fatal("foreign group accepted")
	}
	if err := os.Chown(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repairLegacyNodeConfigAccessAt(path, 65534); err == nil {
		t.Fatal("unexpected directory accepted")
	}
}

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
