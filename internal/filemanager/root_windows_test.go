//go:build windows

package filemanager

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
)

func windowsVolumeManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	c, d := t.TempDir(), t.TempDir()
	m, err := New(Config{Root: "/", VolumeRoots: map[string]string{"C": c, "D": d}, ProtectedVirtual: []string{"/C/Secrets"}, TrashVirtual: "/C/private-trash"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, c, d
}

func TestWindowsVolumeRootsAndCrossVolumeMove(t *testing.T) {
	m, c, d := windowsVolumeManager(t)
	ctx := context.Background()
	listing, err := m.List(ctx, "/", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 2 || listing.Entries[0].Path != "/C" || listing.Entries[1].Path != "/D" {
		t.Fatalf("volumes %#v", listing)
	}
	entry, err := m.Upload(ctx, "/C", "中文.txt", bytes.NewBufferString("中文\r\n"), int64(len("中文\r\n")), false)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Path != "/C/中文.txt" {
		t.Fatalf("native separators leaked: %q", entry.Path)
	}
	result, err := m.Action(ctx, contract.FileActionRequest{Action: "move", Sources: []string{entry.Path}, Target: "/D"})
	if err != nil || len(result.Failed) != 0 || len(result.Succeeded) != 1 {
		t.Fatalf("move %#v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(c, "中文.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source not removed")
	}
	content, err := os.ReadFile(filepath.Join(d, "中文.txt"))
	if err != nil || string(content) != "中文\r\n" {
		t.Fatalf("bytes changed %q %v", content, err)
	}
	if _, err := m.Upload(ctx, "/D", "中文.txt", strings.NewReader("overwrite"), 9, false); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("no-replace failed: %v", err)
	}
}

func TestWindowsRejectsNamespacesAndProtectsCanonicalPaths(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	mustMkdirAll(t, filepath.Join(c, "Secrets"))
	mustWrite(t, filepath.Join(c, "Secrets", "token.txt"), "secret")
	for _, path := range []string{"/C/../Secrets", "/C/file.txt:stream", "/C/NUL.txt", "/C/COM1", "/C/LPT².txt", "/C/file.", "/C/file ", `C:\Users`, `\\server\share`, `/C/\\?\C:\secret`} {
		if _, err := m.Stat(path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("accepted namespace %q: %v", path, err)
		}
	}
	if _, err := m.Stat("/c/seCRETs/token.txt"); !errors.Is(err, ErrProtected) {
		t.Fatalf("case bypass: %v", err)
	}
	if _, err := m.Action(context.Background(), contract.FileActionRequest{Action: "rename", Sources: []string{"/C"}, Target: "/D/x"}); err == nil {
		t.Fatal("volume root rename allowed")
	}
	long := filepath.Join(c, "Secrets")
	ptr, _ := windows.UTF16PtrFromString(long)
	var short [32768]uint16
	n, err := windows.GetShortPathName(ptr, &short[0], uint32(len(short)))
	if err == nil && n < uint32(len(short)) {
		alias := filepath.Base(windows.UTF16ToString(short[:n]))
		if _, err := m.Stat("/C/" + alias + "/token.txt"); !errors.Is(err, ErrProtected) {
			t.Fatalf("short alias protection: %v", err)
		}
	}
}

func TestWindowsRejectsReparseTargets(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "outside")
	if err := os.Symlink(outside, filepath.Join(c, "redirect")); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	if _, err := m.Stat("/C/redirect/secret.txt"); !errors.Is(err, ErrSymlink) {
		t.Fatalf("reparse traversed: %v", err)
	}
	if _, err := m.Upload(context.Background(), "/C/redirect", "write.txt", strings.NewReader("bad"), 3, false); !errors.Is(err, ErrSymlink) {
		t.Fatalf("reparse write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "write.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("wrote outside volume")
	}
}

func TestWindowsAtomicTextSavePreservesDACL(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	target := filepath.Join(c, "private.txt")
	mustWrite(t, target, "before")
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Skipf("test account cannot set DACL on its temp file: %v", err)
	}
	before, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := m.Stat("/C/private.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteText(context.Background(), entry.Path, contract.FileWriteRequest{Content: "after", ExpectedResourceVersion: entry.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	after, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatalf("DACL changed before=%s after=%s", before.String(), after.String())
	}
}

func TestWindowsOpenFileCollisionNeverClobbers(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	mustWrite(t, filepath.Join(c, "a.txt"), "a")
	mustWrite(t, filepath.Join(c, "b.txt"), "b")
	if err := renameNoReplaceRoot(m.rootFS, "/C/a.txt", "/C/b.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("collision: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		data, err := os.ReadFile(filepath.Join(c, name+".txt"))
		if err != nil || string(data) != name {
			t.Fatalf("collision lost %s", name)
		}
	}
}

func TestWindowsHardLinksCannotAliasProtectedFiles(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	mustMkdirAll(t, filepath.Join(c, "Secrets"))
	target := filepath.Join(c, "Secrets", "key.txt")
	mustWrite(t, target, "private")
	if err := os.Link(target, filepath.Join(c, "alias.txt")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if file, _, err := m.Open(context.Background(), "/C/alias.txt"); err == nil {
		file.Close()
		t.Fatal("hard link exposed protected file")
	}
}

func TestWindowsOccupiedRenameIsBoundedAndNonDestructive(t *testing.T) {
	m, c, _ := windowsVolumeManager(t)
	target := filepath.Join(c, "occupied.txt")
	mustWrite(t, target, "keep")
	ptr, _ := windows.UTF16PtrFromString(target)
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	start := time.Now()
	err = renameNoReplaceRoot(m.rootFS, "/C/occupied.txt", "/C/moved.txt")
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || !strings.Contains(err.Error(), "文件被占用") {
		t.Fatalf("occupied error: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("occupied rename exceeded retry budget")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "keep" {
		t.Fatal("occupied source changed")
	}
}
