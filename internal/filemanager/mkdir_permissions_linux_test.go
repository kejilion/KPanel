//go:build linux

package filemanager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestUploadedDirectoryPermissionsWithPrivateAgentUmask(t *testing.T) {
	// A public parent lets a separate unprivileged process exercise traversal,
	// rather than hiding broken directory permissions behind the root test user.
	root, err := os.MkdirTemp("", "kpanel-upload-permissions-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	previous := syscall.Umask(0077)
	defer syscall.Umask(previous)
	ctx := context.Background()
	for _, item := range []struct{ parent, name string }{{"/", "site"}, {"/site", "images"}, {"/site/images", "nested"}} {
		if _, err := m.Action(ctx, contract.FileActionRequest{Action: "mkdir", Target: item.parent, Name: item.name}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(root, item.parent, item.name))
		if err != nil || info.Mode().Perm() != 0755 {
			t.Fatalf("directory must be 0755: %v, %v", info, err)
		}
	}
	const content = "static-image-fixture"
	if _, err := m.Upload(ctx, "/site/images/nested", "image.png", strings.NewReader(content), int64(len(content)), false); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(root, "site", "images", "nested", "image.png")
	info, err := os.Stat(image)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("uploaded file must remain 0644: %v, %v", info, err)
	}
	if os.Geteuid() == 0 {
		command := exec.Command("/bin/cat", image)
		command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
		output, err := command.CombinedOutput()
		if err != nil || string(output) != content {
			t.Fatalf("unprivileged web user cannot read uploaded image: %s, %v", output, err)
		}
	}
}

func TestMkdirDoesNotChangeExistingDirectoryOrFollowLinks(t *testing.T) {
	m, root := newTestManager(t)
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("private", filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.mkdir("/", "private"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("existing directory: %v", err)
	}
	if _, err := m.mkdir("/", "linked"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("existing symlink: %v", err)
	}
	if _, err := m.mkdir("/linked", "images"); err == nil {
		t.Fatal("mkdir followed a linked parent")
	}
	info, err := os.Stat(private)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("existing permissions changed: %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(private, "images")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created below a linked parent: %v", err)
	}
}
