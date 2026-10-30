//go:build windows

package windowsnode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestDurableMoveRetriesRunningImage(t *testing.T) {
	if os.Getenv("KPANEL_TEST_MOVE_IMAGE_CHILD") == "1" {
		fmtReady := []byte("image-ready\n")
		if _, err := os.Stdout.Write(fmtReady); err != nil {
			t.Fatal(err)
		}
		var release [1]byte
		_, _ = os.Stdin.Read(release[:])
		return
	}
	root := t.TempDir()
	live, old := filepath.Join(root, "live.exe"), filepath.Join(root, "old.exe")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, image, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("verified previous binary"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, live, "-test.run=^TestDurableMoveRetriesRunningImage$")
	child.Env = append(os.Environ(), "KPANEL_TEST_MOVE_IMAGE_CHILD=1")
	release, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = release.Close()
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	ready, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || ready != "image-ready\n" {
		t.Fatalf("child did not map its image: %q, %v", ready, err)
	}
	source, _ := windows.UTF16PtrFromString(old)
	target, _ := windows.UTF16PtrFromString(live)
	if err := windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("expected mapped-image access denial, got %v", err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = release.Close()
	}()
	if err := durableMove(old, live); err != nil {
		t.Fatalf("rollback did not wait for mapped image release: %v", err)
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(live)
	if err != nil || string(got) != "verified previous binary" {
		t.Fatalf("previous binary not restored: %q, %v", got, err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("move left source behind: %v", err)
	}
}

func TestDurableMovePermanentFailurePreservesSource(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "old.exe"), filepath.Join(root, "live.exe")
	if err := os.WriteFile(source, []byte("verified previous binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("read-only destination"), 0600); err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(target)
	if err := windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_NORMAL) })
	started := time.Now()
	if err := durableMove(source, target); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("permanent access denial was not preserved: %v", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("permanent failure exceeded bounded retry budget")
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "verified previous binary" {
		t.Fatalf("failed move discarded source: %q, %v", got, err)
	}
	got, err = os.ReadFile(target)
	if err != nil || string(got) != "read-only destination" {
		t.Fatalf("failed move changed destination: %q, %v", got, err)
	}
	if attributes, err := windows.GetFileAttributes(name); err != nil || attributes&windows.FILE_ATTRIBUTE_READONLY == 0 {
		t.Fatalf("failed move changed destination attributes: %v", err)
	}
}

type memoryUpdateStore struct {
	files    map[string]string
	journal  []byte
	failMove int
	moves    int
	failHash string
}

func (s *memoryUpdateStore) ReadJournal(string) ([]byte, error) {
	if s.journal == nil {
		return nil, os.ErrNotExist
	}
	return s.journal, nil
}
func (s *memoryUpdateStore) WriteJournal(_ string, data []byte) error {
	s.journal = append([]byte(nil), data...)
	return nil
}
func (s *memoryUpdateStore) Hash(path string) (string, error) {
	if path == s.failHash {
		return "", errors.New("untrusted file ACL")
	}
	v, ok := s.files[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}
func (s *memoryUpdateStore) Move(from, to string) error {
	s.moves++
	if s.moves == s.failMove {
		return errors.New("simulated power interruption")
	}
	value, ok := s.files[from]
	if !ok {
		return os.ErrNotExist
	}
	s.files[to] = value
	delete(s.files, from)
	return nil
}
func (s *memoryUpdateStore) Remove(path string) error {
	if path == "journal" {
		s.journal = nil
		return nil
	}
	delete(s.files, path)
	return nil
}
func testUpdater() (u *Updater, s *memoryUpdateStore) {
	s = &memoryUpdateStore{files: map[string]string{}}
	u = &Updater{Directory: `C:\protected`, Journal: "journal", Store: s, Stop: func() error { return nil }, Start: func() error { return nil }, Healthy: func(context.Context) error { return nil }}
	return
}
func TestRecoveryEveryInterruptedUpdatePhase(t *testing.T) {
	previous, next := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, phase := range []string{"prepared", "stopped", "between-renames", "swapped", "committed"} {
		t.Run(phase, func(t *testing.T) {
			u, s := testUpdater()
			live, old, staged := u.paths()
			s.files[live] = previous
			s.files[staged] = next
			tx := UpdateTransaction{Phase: phase, PreviousSHA256: previous, NextSHA256: next}
			if phase == "between-renames" {
				tx.Phase = "stopped"
				delete(s.files, live)
				s.files[old] = previous
			}
			if phase == "swapped" || phase == "committed" {
				s.files[live] = next
				s.files[old] = previous
				delete(s.files, staged)
			}
			s.journal, _ = json.Marshal(tx)
			if err := u.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := previous
			if phase == "committed" {
				want = next
			}
			if s.files[live] != want || s.journal != nil {
				t.Fatalf("recovery lost stable entry: files=%v journal=%s", s.files, s.journal)
			}
		})
	}
}
func TestApplyRollsBackOnSecondRenameFailure(t *testing.T) {
	u, s := testUpdater()
	live, _, staged := u.paths()
	s.files[live] = strings.Repeat("a", 64)
	s.files[staged] = strings.Repeat("b", 64)
	s.failMove = 2
	err := u.Apply(context.Background(), strings.Repeat("b", 64))
	if err == nil || s.files[live] != strings.Repeat("a", 64) || s.journal != nil {
		t.Fatalf("failed restore: %v, %+v", err, s)
	}
}
func TestUpdateNeverStopsBeforeProtectedFileAndHashValidation(t *testing.T) {
	for _, mismatch := range []string{"hash", "staged-access", "live-access"} {
		t.Run(mismatch, func(t *testing.T) {
			u, s := testUpdater()
			live, _, staged := u.paths()
			s.files[live] = strings.Repeat("a", 64)
			s.files[staged] = strings.Repeat("b", 64)
			stopped := false
			u.Stop = func() error { stopped = true; return nil }
			expected := strings.Repeat("b", 64)
			if mismatch == "hash" {
				expected = strings.Repeat("c", 64)
			} else {
				s.failHash = staged
				if mismatch == "live-access" {
					s.failHash = live
				}
			}
			if u.Apply(context.Background(), expected) == nil || stopped || s.journal != nil {
				t.Fatal("unverified release reached service stop")
			}
		})
	}
}
func TestRecoveryRejectsMissingOrChangedRollback(t *testing.T) {
	u, s := testUpdater()
	live, old, _ := u.paths()
	s.files[live] = strings.Repeat("b", 64)
	s.files[old] = strings.Repeat("c", 64)
	s.journal, _ = json.Marshal(UpdateTransaction{Phase: "swapped", PreviousSHA256: strings.Repeat("a", 64), NextSHA256: strings.Repeat("b", 64)})
	if u.Recover(context.Background()) == nil || s.journal == nil {
		t.Fatal("corrupt rollback was accepted or evidence discarded")
	}
}
func TestReleaseURLAndVersionBoundaries(t *testing.T) {
	for _, tag := range []string{"v01.2.3", "v1.2.3-rc.3", "v1.2.3-dev", "v1000000.2.3"} {
		if stableTag.MatchString(tag) {
			t.Fatalf("noncanonical stable tag accepted: %s", tag)
		}
	}
	for _, endpoint := range []string{"http://github.com/x", "https://github.com.evil.test/x", "https://user:pw@github.com/x", "https://github.com:8443/x", "https://localhost/x"} {
		u, _ := url.Parse(endpoint)
		if releaseURL(u) {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	if newerVersion("v1.23.0", "1.24.0-rc.10") || newerVersion("v1.24.0", "1.24.0") || !newerVersion("v1.24.0", "1.24.0-rc.10") {
		t.Fatal("update version order is unsafe")
	}
}
