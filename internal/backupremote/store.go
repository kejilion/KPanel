package backupremote

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"golang.org/x/crypto/chacha20poly1305"
)

type Settings struct {
	Version  int       `json:"version"`
	Revision string    `json:"revision"`
	Storages []Storage `json:"storages"`
	Schedule Schedule  `json:"schedule"`
}

type Store struct {
	mu    sync.Mutex
	root  string
	key   []byte
	state Settings
}

func OpenStore(root string) (*Store, error) {
	if err := backup.PrivateDir(root); err != nil {
		return nil, err
	}
	s := &Store{root: root, state: Settings{Version: 1, Revision: backup.NewID(), Storages: []Storage{}, Schedule: Schedule{Modules: []string{"panel"}, Frequency: "daily", Hour: 3, Day: 1, Timezone: "UTC", Keep: 7}}}
	body, stateErr := backup.ReadFile(filepath.Join(root, "settings.enc"), 64<<10)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return nil, stateErr
	}
	key, err := backup.ReadFile(filepath.Join(root, "key"), 32)
	if errors.Is(err, os.ErrNotExist) && errors.Is(stateErr, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			_, err = backup.CopyFile(filepath.Join(root, "key"), bytes.NewReader(key), 32)
		}
	}
	if err != nil || len(key) != 32 {
		return nil, ErrInvalid
	}
	s.key = key
	if stateErr == nil {
		aead, _ := chacha20poly1305.NewX(key)
		if len(body) < aead.NonceSize() {
			return nil, ErrInvalid
		}
		plain, err := aead.Open(nil, body[:aead.NonceSize()], body[aead.NonceSize():], []byte("kpanel-backup-settings-v1"))
		if err != nil {
			return nil, ErrInvalid
		}
		defer clear(plain)
		if backup.Decode(plain, &s.state) != nil || s.state.Version != 1 || !backup.ValidID(s.state.Revision) || len(s.state.Storages) > MaxStorages || s.state.Schedule.Validate() != nil {
			return nil, ErrInvalid
		}
		seen := map[string]bool{}
		for i := range s.state.Storages {
			v := &s.state.Storages[i]
			if v.Validate() != nil || seen[v.ID] {
				return nil, ErrInvalid
			}
			seen[v.ID] = true
		}
		if s.state.Schedule.StorageID != "" && !seen[s.state.Schedule.StorageID] {
			return nil, ErrInvalid
		}
	}
	return s, nil
}

func cloneSettings(v Settings) Settings {
	v.Storages = slices.Clone(v.Storages)
	v.Schedule.Modules = slices.Clone(v.Schedule.Modules)
	return v
}
func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := cloneSettings(s.state)
	for i := range v.Storages {
		v.Storages[i] = v.Storages[i].Public()
	}
	v.Schedule = v.Schedule.Public()
	return v
}
func (s *Store) Storage(id string) (Storage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.Storages {
		if v.ID == id {
			return v, nil
		}
	}
	return Storage{}, backup.ErrNotFound
}
func (s *Store) Plan() Schedule {
	plan, _ := s.PlanWithRevision()
	return plan
}

func (s *Store) PlanWithRevision() (Schedule, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.state).Schedule, s.state.Revision
}

func (s *Store) save(v Settings, changeRevision bool) error {
	if changeRevision {
		v.Revision = backup.NewID()
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) > 60<<10 {
		return ErrInvalid
	}
	defer clear(b)
	aead, _ := chacha20poly1305.NewX(s.key)
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := aead.Seal(nonce, nonce, b, []byte("kpanel-backup-settings-v1"))
	if err := backup.AtomicFile(filepath.Join(s.root, "settings.enc"), sealed); err != nil {
		return err
	}
	s.state = v
	return nil
}

func (s *Store) PutStorage(revision string, input Storage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.state.Revision {
		return ErrConflict
	}
	v := cloneSettings(s.state)
	index := -1
	for i, old := range v.Storages {
		if old.ID == input.ID {
			index = i
			if input.Secret == "" && input.Kind == old.Kind {
				input.Secret = old.Secret
			}
			break
		}
	}
	if input.ID == "" {
		input.ID = backup.NewID()
	} else if index < 0 {
		return backup.ErrNotFound
	}
	if input.Validate() != nil {
		return ErrInvalid
	}
	input.HasSecret = false
	if index < 0 {
		if len(v.Storages) >= MaxStorages {
			return ErrLimit
		}
		v.Storages = append(v.Storages, input)
	} else {
		v.Storages[index] = input
	}
	return s.save(v, true)
}
func (s *Store) DeleteStorage(revision, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.state.Revision {
		return ErrConflict
	}
	if s.state.Schedule.StorageID == id {
		return ErrConflict
	}
	v := cloneSettings(s.state)
	for i, item := range v.Storages {
		if item.ID == id {
			v.Storages = slices.Delete(v.Storages, i, i+1)
			return s.save(v, true)
		}
	}
	return backup.ErrNotFound
}
func (s *Store) PutSchedule(revision string, input Schedule, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.state.Revision {
		return ErrConflict
	}
	if input.Password == "" {
		input.Password = s.state.Schedule.Password
	}
	if input.Validate() != nil {
		return ErrInvalid
	}
	if input.StorageID != "" && !slices.ContainsFunc(s.state.Storages, func(v Storage) bool { return v.ID == input.StorageID }) {
		return ErrInvalid
	}
	input.HasPassword = false
	input.LastRun = s.state.Schedule.LastRun
	input.LastError = ""
	input.NextRun = input.Next(now)
	v := cloneSettings(s.state)
	v.Schedule = input
	return s.save(v, true)
}

// Claim durably advances the scheduled slot before execution: restart never
// duplicates a backup, and missed windows are skipped rather than replayed.
func (s *Store) Claim(now time.Time, busy bool) (Schedule, bool, error) {
	plan, _, run, err := s.ClaimWithRevision(now, busy)
	return plan, run, err
}

// Return the configuration identity under the same lock as the claimed slot.
func (s *Store) ClaimWithRevision(now time.Time, busy bool) (Schedule, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := cloneSettings(s.state)
	p := v.Schedule
	if !p.Enabled || (!p.NextRun.IsZero() && now.Before(p.NextRun)) {
		return p, v.Revision, false, nil
	}
	missed := p.NextRun.IsZero() || now.Sub(p.NextRun) > 5*time.Minute
	if busy && !missed {
		return p, v.Revision, false, nil
	}
	v.Schedule.NextRun = p.Next(now)
	v.Schedule.LastError = ""
	if missed {
		v.Schedule.LastError = "schedule_missed"
	} else {
		v.Schedule.LastRun = now.UTC()
	}
	if err := s.save(v, false); err != nil {
		return p, v.Revision, false, err
	}
	return p, v.Revision, !missed, nil
}
func (s *Store) SetRunError(code string) error {
	return s.SetRunErrorForRevision("", code)
}

// A worker from an earlier configuration cannot overwrite the current
// schedule's outcome after the administrator changes it.
func (s *Store) SetRunErrorForRevision(revision, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != "" && revision != s.state.Revision {
		return ErrConflict
	}
	v := cloneSettings(s.state)
	v.Schedule.LastError = code
	return s.save(v, false)
}

// Pause is used before restoring Panel identity. Re-enabling is explicit.
func (s *Store) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Schedule.Enabled {
		return nil
	}
	v := cloneSettings(s.state)
	v.Schedule.Enabled = false
	return s.save(v, true)
}
