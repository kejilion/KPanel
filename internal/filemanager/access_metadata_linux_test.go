//go:build linux

package filemanager

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/unix"
)

// Linux ACL xattr wire format; exercise the kernel without setfacl dependencies.
func namedUserACL() []byte {
	entries := [][3]uint32{{1, 7, 0xffffffff}, {2, 4, 60000}, {4, 0, 0xffffffff}, {16, 4, 0xffffffff}, {32, 0, 0xffffffff}}
	data := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(data, 2)
	for n, e := range entries {
		offset := 4 + n*8
		binary.LittleEndian.PutUint16(data[offset:], uint16(e[0]))
		binary.LittleEndian.PutUint16(data[offset+2:], uint16(e[1]))
		binary.LittleEndian.PutUint32(data[offset+4:], e[2])
	}
	return data
}

func probeNamedUserRead(t *testing.T, file string, allow bool) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Log("independent uid access probe requires root; metadata assertions still run")
		return
	}
	// t.TempDir creates a private per-test parent; permit traversal only.
	for parent := filepath.Dir(file); parent != "/tmp" && parent != "/"; parent = filepath.Dir(parent) {
		if err := os.Chmod(parent, 0755); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("/bin/cat", file)
	command.Env = append(os.Environ(), "LC_ALL=C")
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 60000, Gid: 60000, Groups: []uint32{}}}
	out, err := command.CombinedOutput()
	if allow {
		if err != nil {
			t.Fatalf("kernel uid read denied: %s: %v", out, err)
		}
	} else if err == nil || !bytes.Contains(out, []byte("Permission denied")) {
		t.Fatalf("kernel uid read result: %s: %v", out, err)
	}
}

func TestReplacementRemovesInheritedACL(t *testing.T) {
	for _, kind := range []string{"text", "office", "receive"} {
		t.Run(kind, func(t *testing.T) {
			m, root := newTestManager(t)
			file := filepath.Join(root, "source.txt")
			content := []byte("before")
			if kind == "office" {
				file = filepath.Join(root, "sample.docx")
				content = officeFixtures(t)["sample.docx"]
			}
			if err := os.WriteFile(file, content, 0640); err != nil {
				t.Fatal(err)
			}
			if err := unix.Setxattr(root, "system.posix_acl_default", namedUserACL(), 0); err != nil {
				t.Fatalf("ACL fixture unavailable: %v", err)
			}
			// Prove the adversarial directory really grants access to a newly made inode.
			witness := filepath.Join(root, "inherited")
			if err := os.WriteFile(witness, []byte("witness"), 0640); err != nil {
				t.Fatal(err)
			}
			probeNamedUserRead(t, witness, true)
			virtual := "/" + filepath.Base(file)
			ctx := context.Background()
			switch kind {
			case "text":
				entry, err := m.Stat(virtual)
				if err != nil {
					t.Fatal(err)
				}
				_, err = m.WriteText(ctx, virtual, contract.FileWriteRequest{ExpectedResourceVersion: entry.ResourceVersion, Content: "after"})
				if err != nil {
					t.Fatal(err)
				}
			case "office":
				doc, err := m.ReadOffice(ctx, virtual)
				if err != nil {
					t.Fatal(err)
				}
				_, err = m.WriteOffice(ctx, virtual, contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: doc.ContentVersion, OfficeEdits: []contract.OfficeEdit{{ID: doc.Sections[0].Items[0].ID, Text: "after"}}})
				if err != nil {
					t.Fatal(err)
				}
			case "receive":
				input := testReceiveInput(5)
				input.Name = filepath.Base(file)
				input.Overwrite = true
				session, err := m.BeginReceive(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				data := []byte("after")
				if _, err = m.WriteReceiveChunk(ctx, session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
					t.Fatal(err)
				}
				if _, err = m.CommitReceive(ctx, session.ID, input.SourceKey, 5, receiveDigest(data)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := unix.Getxattr(file, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
				t.Fatalf("source-absent access ACL retained: %v", err)
			}
			info, _ := os.Stat(file)
			if info.Mode().Perm() != 0640 {
				t.Fatal(info.Mode())
			}
			probeNamedUserRead(t, file, false)
		})
	}
}

func TestRelocationPreservesExactACLAndXattrs(t *testing.T) {
	for _, sourceACL := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[sourceACL], func(t *testing.T) {
			m, root := newTestManager(t)
			source := filepath.Join(root, "source")
			destination := filepath.Join(root, "destination")
			if err := os.Mkdir(source, 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(destination, 0750); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(source, "value")
			if err := os.WriteFile(file, []byte("unchanged"), 0640); err != nil {
				t.Fatal(err)
			}
			if err := unix.Setxattr(destination, "system.posix_acl_default", namedUserACL(), 0); err != nil {
				t.Fatal(err)
			}
			if sourceACL {
				for _, entry := range []struct{ path, name string }{{source, "system.posix_acl_access"}, {source, "system.posix_acl_default"}, {file, "system.posix_acl_access"}} {
					acl := namedUserACL()
					if entry.path == source {
						binary.LittleEndian.PutUint16(acl[14:], 5)
						binary.LittleEndian.PutUint16(acl[30:], 5)
					}
					if err := unix.Setxattr(entry.path, entry.name, acl, 0); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := unix.Setxattr(file, "user.kpanel-access", []byte("same"), 0); err != nil {
				t.Fatal(err)
			}
			budget := &copyBudget{maxEntries: 100, maxBytes: 1 << 20}
			if err := m.copyTreeWithAccess(context.Background(), "/source", "/destination/moved", budget, true); err != nil {
				t.Fatal(err)
			}
			moved := filepath.Join(destination, "moved")
			for _, pair := range [][2]string{{source, moved}, {file, filepath.Join(moved, "value")}} {
				a, err := os.Open(pair[0])
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				b, err := os.Open(pair[1])
				if err != nil {
					t.Fatal(err)
				}
				defer b.Close()
				sourceAttrs, err := fileExtendedAttributes(a)
				if err != nil {
					t.Fatal(err)
				}
				targetAttrs, err := fileExtendedAttributes(b)
				if err != nil {
					t.Fatal(err)
				}
				if len(sourceAttrs) != len(targetAttrs) {
					t.Fatalf("metadata differs: %v / %v", sourceAttrs, targetAttrs)
				}
				for key, value := range sourceAttrs {
					if !bytes.Equal(value, targetAttrs[key]) {
						t.Fatalf("lost %s", key)
					}
				}
			}
			probeNamedUserRead(t, filepath.Join(moved, "value"), sourceACL)
		})
	}
}

func TestContentReplacementDropsCapabilitiesButRelocationKeepsThem(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires CAP_SETFCAP")
	}
	m, root := newTestManager(t)
	file := filepath.Join(root, "executable")
	if err := os.WriteFile(file, []byte("old"), 0750); err != nil {
		t.Fatal(err)
	}
	capability := make([]byte, 20)
	binary.LittleEndian.PutUint32(capability, 0x02000001)
	binary.LittleEndian.PutUint32(capability[4:], 1)
	if err := unix.Setxattr(file, "security.capability", capability, 0); err != nil {
		t.Fatalf("capability fixture unavailable: %v", err)
	}
	if err := m.copyTreeWithAccess(context.Background(), "/executable", "/moved", &copyBudget{maxEntries: 10, maxBytes: 1000}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getxattr(filepath.Join(root, "moved"), "security.capability", nil); err != nil {
		t.Fatal("relocation lost capability", err)
	}
	entry, _ := m.Stat("/executable")
	if _, err := m.WriteText(context.Background(), "/executable", contract.FileWriteRequest{Content: "new", ExpectedResourceVersion: entry.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getxattr(file, "security.capability", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatal("replacement retained capability", err)
	}
}

func TestCrossFilesystemMoveTrashRestoreRetainsAccess(t *testing.T) {
	base := t.TempDir()
	other, err := os.MkdirTemp("/dev/shm", "kpanel-access-")
	if err != nil {
		t.Skipf("second filesystem unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(other) })
	var a, b unix.Stat_t
	if unix.Stat(base, &a) != nil || unix.Stat(other, &b) != nil || a.Dev == b.Dev {
		t.Skip("distinct local filesystems required")
	}
	m, err := New(Config{Root: "/", TrashVirtual: filepath.Join(other, "trash")})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	file := filepath.Join(base, "value")
	if err := os.WriteFile(file, []byte("same data"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(file, "user.kpanel-access", []byte("preserved"), 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(other, "system.posix_acl_default", namedUserACL(), 0); err != nil {
		t.Fatal(err)
	}
	budget := func() *copyBudget { return &copyBudget{maxEntries: 100, maxBytes: 1 << 20} }
	check := func(file string) {
		t.Helper()
		value := make([]byte, 32)
		count, err := unix.Getxattr(file, "user.kpanel-access", value)
		if err != nil || string(value[:max(count, 0)]) != "preserved" {
			t.Fatal("xattr lost", err)
		}
		if _, err := unix.Getxattr(file, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
			t.Fatal("inherited ACL leaked", err)
		}
	}
	moved, err := m.moveOne(context.Background(), file, other, budget())
	if err != nil {
		t.Fatal("cross-filesystem move", err)
	}
	check(moved.Path)
	returned, err := m.moveOne(context.Background(), moved.Path, base, budget())
	if err != nil {
		t.Fatal(err)
	}
	check(returned.Path)
	trashed, err := m.trashOne(context.Background(), file, budget())
	if err != nil {
		t.Fatal("cross-filesystem trash", err)
	}
	check(trashed)
	restored, err := m.restoreTrash(context.Background(), filepath.Base(trashed), "", budget())
	if err != nil {
		t.Fatal("cross-filesystem restore", err)
	}
	check(restored.Path)
	probeNamedUserRead(t, restored.Path, false)
}

func TestCrossFilesystemMoveRejectsReplaceableDestination(t *testing.T) {
	for _, test := range []struct {
		name string
		mode os.FileMode
	}{{"world-writable", 0777}, {"sticky-world-writable", 01777}} {
		t.Run(test.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			targetRoot, err := os.MkdirTemp("/dev/shm", "kpanel-untrusted-target-")
			if err != nil {
				t.Skipf("second filesystem unavailable: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(targetRoot) })
			if err := os.Chmod(targetRoot, test.mode); err != nil {
				t.Fatal(err)
			}
			var sourceDevice, targetDevice unix.Stat_t
			if err := unix.Stat(sourceRoot, &sourceDevice); err != nil {
				t.Fatal(err)
			}
			if err := unix.Stat(targetRoot, &targetDevice); err != nil {
				t.Fatal(err)
			}
			if sourceDevice.Dev == targetDevice.Dev {
				t.Skip("distinct local filesystems required")
			}

			m, err := New(Config{Root: "/"})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			source := filepath.Join(sourceRoot, "keep-me")
			if err := os.WriteFile(source, []byte("source remains intact"), 0600); err != nil {
				t.Fatal(err)
			}

			_, err = m.moveOne(context.Background(), source, targetRoot, &copyBudget{maxEntries: 10, maxBytes: 1 << 20})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("move into replaceable destination: got %v, want ErrConflict", err)
			}
			content, err := os.ReadFile(source)
			if err != nil || string(content) != "source remains intact" {
				t.Fatalf("source was not retained: content=%q err=%v", content, err)
			}
			entries, err := os.ReadDir(targetRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("rejected move left target entries: %v", entries)
			}
		})
	}
}

func TestCrossFilesystemMoveRejectsReplaceableSource(t *testing.T) {
	sourceRoot, err := os.MkdirTemp("/dev/shm", "kpanel-untrusted-source-")
	if err != nil {
		t.Skipf("second filesystem unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sourceRoot) })
	if err := os.Chmod(sourceRoot, 0777); err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	var sourceDevice, targetDevice unix.Stat_t
	if err := unix.Stat(sourceRoot, &sourceDevice); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat(targetRoot, &targetDevice); err != nil {
		t.Fatal(err)
	}
	if sourceDevice.Dev == targetDevice.Dev {
		t.Skip("distinct local filesystems required")
	}

	m, err := New(Config{Root: "/"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	source := filepath.Join(sourceRoot, "keep-me")
	if err := os.WriteFile(source, []byte("source remains intact"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err = m.moveOne(context.Background(), source, targetRoot, &copyBudget{maxEntries: 10, maxBytes: 1 << 20})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("move from replaceable source directory: got %v, want ErrConflict", err)
	}
	content, err := os.ReadFile(source)
	if err != nil || string(content) != "source remains intact" {
		t.Fatalf("source was not retained: content=%q err=%v", content, err)
	}
	entries, err := os.ReadDir(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected move left target entries: %v", entries)
	}
}

func TestCrossFilesystemTrashRejectsReplaceableSource(t *testing.T) {
	sourceRoot, err := os.MkdirTemp("/dev/shm", "kpanel-untrusted-trash-source-")
	if err != nil {
		t.Skipf("second filesystem unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sourceRoot) })
	if err := os.Chmod(sourceRoot, 0777); err != nil {
		t.Fatal(err)
	}
	var sourceDevice, trashDevice unix.Stat_t
	if err := unix.Stat(sourceRoot, &sourceDevice); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat("/", &trashDevice); err != nil {
		t.Fatal(err)
	}
	if sourceDevice.Dev == trashDevice.Dev {
		t.Skip("distinct local filesystems required")
	}

	trashVirtual := filepath.ToSlash(t.TempDir())
	m, err := New(Config{Root: "/", TrashVirtual: trashVirtual})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	source := filepath.Join(sourceRoot, "keep-me")
	if err := os.WriteFile(source, []byte("source remains intact"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err = m.trashOne(context.Background(), source, &copyBudget{maxEntries: 10, maxBytes: 1 << 20})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("trash from replaceable source directory: got %v, want ErrConflict", err)
	}
	content, err := os.ReadFile(source)
	if err != nil || string(content) != "source remains intact" {
		t.Fatalf("source was not retained: content=%q err=%v", content, err)
	}
}
