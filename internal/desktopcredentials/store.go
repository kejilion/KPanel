// Package desktopcredentials stores opt-in RDP credentials separately from
// ordinary Panel state and exports. Only ciphertext is retained in memory.
package desktopcredentials

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"golang.org/x/crypto/chacha20poly1305"
)

var (
	ErrMissing = errors.New("saved desktop credentials not found")
	ErrInvalid = errors.New("invalid desktop credentials")
	ErrLimit   = errors.New("saved desktop credential limit reached")
)

const maxRecords = 1024
const maxStateBytes = 4 << 20

// Binding prevents moving ciphertext between users, hosts, node identities or
// account security revisions. All fields are supplied by authenticated state.
type Binding struct {
	UserID       string `json:"userId"`
	HostID       string `json:"hostId"`
	UserVersion  uint64 `json:"userVersion"`
	HostIdentity string `json:"hostIdentity"`
}

type Credentials struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Password string `json:"password"`
}

func (c Credentials) Validate() error {
	name := func(v string, empty bool) bool {
		return (empty || v != "") && len(v) <= 256 && utf8.ValidString(v) &&
			strings.TrimSpace(v) == v && !strings.ContainsFunc(v, unicode.IsControl)
	}
	if !name(c.Username, false) || !name(c.Domain, true) || len(c.Password) == 0 ||
		len(c.Password) > 1024 || !utf8.ValidString(c.Password) || strings.ContainsRune(c.Password, 0) {
		return ErrInvalid
	}
	return nil
}

func (b Binding) valid() bool {
	return len(b.UserID) > 0 && len(b.UserID) <= 128 && backup.ValidID(b.HostID) &&
		len(b.HostIdentity) > 0 && len(b.HostIdentity) <= 128
}

func (b Binding) aad() []byte {
	data, _ := json.Marshal(b)
	return append([]byte("kpanel-rdp-credentials-v1\x00"), data...)
}

type record struct {
	Binding Binding `json:"binding"`
	Sealed  []byte  `json:"sealed"`
}

type state struct {
	Version int      `json:"version"`
	Records []record `json:"records"`
}

type Store struct {
	mu    sync.Mutex
	root  string
	key   []byte
	state state
}

func Open(root string) (*Store, error) {
	if err := backup.PrivateDir(root); err != nil {
		return nil, err
	}
	s := &Store{root: root, state: state{Version: 1, Records: []record{}}}
	body, stateErr := backup.ReadFile(filepath.Join(root, "credentials.json"), maxStateBytes)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return nil, stateErr
	}
	keyPath := filepath.Join(root, "key")
	key, err := backup.ReadFile(keyPath, chacha20poly1305.KeySize)
	if errors.Is(err, os.ErrNotExist) && errors.Is(stateErr, os.ErrNotExist) {
		key = make([]byte, chacha20poly1305.KeySize)
		if _, err = rand.Read(key); err == nil {
			_, err = backup.CopyFile(keyPath, bytes.NewReader(key), int64(len(key)))
		}
		if err == nil {
			err = backup.SyncDir(root)
		}
	}
	if err != nil || len(key) != chacha20poly1305.KeySize {
		return nil, ErrInvalid
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		return nil, err
	}
	s.key = key
	if stateErr == nil {
		if backup.Decode(body, &s.state) != nil || s.state.Version != 1 || len(s.state.Records) > maxRecords {
			return nil, ErrInvalid
		}
		seen := make(map[[2]string]bool)
		for _, row := range s.state.Records {
			id := [2]string{row.Binding.UserID, row.Binding.HostID}
			if !row.Binding.valid() || seen[id] {
				return nil, ErrInvalid
			}
			seen[id] = true
			if _, err := s.open(row); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}

func (s *Store) open(row record) (Credentials, error) {
	aead, err := chacha20poly1305.NewX(s.key)
	if err != nil || len(row.Sealed) < chacha20poly1305.NonceSizeX+chacha20poly1305.Overhead || len(row.Sealed) > 8192 {
		return Credentials{}, ErrInvalid
	}
	plain, err := aead.Open(nil, row.Sealed[:aead.NonceSize()], row.Sealed[aead.NonceSize():], row.Binding.aad())
	if err != nil {
		return Credentials{}, ErrInvalid
	}
	defer clear(plain)
	var credentials Credentials
	if backup.Decode(plain, &credentials) != nil || credentials.Validate() != nil {
		return Credentials{}, ErrInvalid
	}
	return credentials, nil
}

func (s *Store) Get(binding Binding) (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.state.Records {
		if row.Binding == binding {
			return s.open(row)
		}
	}
	return Credentials{}, ErrMissing
}

func (s *Store) save(records []record) error {
	next := state{Version: 1, Records: records}
	data, err := json.Marshal(next)
	if err != nil || len(data) > maxStateBytes {
		return ErrLimit
	}
	if err := backup.AtomicFile(filepath.Join(s.root, "credentials.json"), data); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *Store) Put(binding Binding, credentials Credentials) error {
	if !binding.valid() || credentials.Validate() != nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := slices.Clone(s.state.Records)
	index := slices.IndexFunc(rows, func(row record) bool {
		return row.Binding.UserID == binding.UserID && row.Binding.HostID == binding.HostID
	})
	if index < 0 && len(rows) >= maxRecords {
		return ErrLimit
	}
	plain, err := json.Marshal(credentials)
	if err != nil {
		return ErrInvalid
	}
	defer clear(plain)
	aead, err := chacha20poly1305.NewX(s.key)
	if err != nil {
		return ErrInvalid
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	row := record{Binding: binding, Sealed: aead.Seal(nonce, nonce, plain, binding.aad())}
	if index < 0 {
		rows = append(rows, row)
	} else {
		rows[index] = row
	}
	return s.save(rows)
}

// Delete also removes stale account/host revisions; it never needs decryption.
func (s *Store) Delete(userID, hostID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := slices.DeleteFunc(slices.Clone(s.state.Records), func(row record) bool { return row.Binding.UserID == userID && row.Binding.HostID == hostID })
	if len(rows) == len(s.state.Records) {
		return nil
	}
	return s.save(rows)
}

func (s *Store) DeleteHost(hostID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := slices.DeleteFunc(slices.Clone(s.state.Records), func(row record) bool { return row.Binding.HostID == hostID })
	if len(rows) == len(s.state.Records) {
		return nil
	}
	return s.save(rows)
}
