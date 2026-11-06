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
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/unix"
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

func TestMkdirPublishesIntoWritableParentAfterSettingPermissions(t *testing.T) {
	m, root := newTestManager(t)
	parent := filepath.Join(root, "shared")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	// Watch the attacker-writable parent. Only an already-complete directory
	// may appear there; mkdir followed by chmod would expose IN_CREATE/ATTRIB.
	watcher, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(watcher)
	if _, err := unix.InotifyAddWatch(watcher, parent, unix.IN_CREATE|unix.IN_MOVED_TO|unix.IN_ATTRIB); err != nil {
		t.Fatal(err)
	}
	if _, err := m.mkdir("/shared", "images"); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4096)
	n, err := unix.Read(watcher, buffer)
	if err != nil {
		t.Fatal(err)
	}
	var published bool
	for offset := 0; offset+unix.SizeofInotifyEvent <= n; {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&buffer[offset]))
		if event.Mask&(unix.IN_CREATE|unix.IN_ATTRIB) != 0 {
			t.Fatalf("mutable creation exposed in writable parent: mask=%x", event.Mask)
		}
		published = published || event.Mask&unix.IN_MOVED_TO != 0
		offset += unix.SizeofInotifyEvent + int(event.Len)
	}
	if !published {
		t.Fatal("directory was not atomically published")
	}
	info, err := os.Stat(filepath.Join(parent, "images"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("published mode: %v, %v", info, err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
		t.Fatalf("staging directory leaked: %v, %v", entries, err)
	}
}

func TestMkdirPreservesSetgidParentGroup(t *testing.T) {
	m, root := newTestManager(t)
	parent := filepath.Join(root, "shared-group")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	group := os.Getegid()
	if os.Geteuid() == 0 {
		group = 65534
	}
	if err := os.Chown(parent, -1, group); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0777|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if _, err := m.mkdir("/shared-group", "images"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(parent, "images"))
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if int(stat.Gid) != group || info.Mode()&os.ModeSetgid == 0 || info.Mode().Perm() != 0755 {
		t.Fatalf("destination group inheritance lost: gid=%d mode=%v", stat.Gid, info.Mode())
	}
}
