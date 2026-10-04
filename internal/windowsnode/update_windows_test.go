//go:build windows

package windowsnode

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
)

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
