package terminalcommands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion      = 1
	MaxCommands        = 64
	MaxNameRunes       = 48
	MaxCommandBytes    = 8 << 10
	MaxUpdateBytes     = 768 << 10
	maxPersistedBytes  = 4 << 20
	unavailableWarning = "terminal_commands_unavailable"
)

var (
	ErrConflict      = errors.New("terminal commands changed")
	ErrUnavailable   = errors.New("terminal commands are unavailable")
	errInvalidOnDisk = errors.New("terminal commands data is invalid")
	idPattern        = regexp.MustCompile(`^[a-f0-9]{32}$`)
	resourcePattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Command struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

type Snapshot struct {
	SchemaVersion   int       `json:"schemaVersion"`
	ResourceVersion string    `json:"resourceVersion"`
	Available       bool      `json:"available"`
	Warning         string    `json:"warning,omitempty"`
	Items           []Command `json:"items"`
}

type ReplaceInput struct {
	ExpectedResourceVersion string    `json:"expectedResourceVersion"`
	Items                   []Command `json:"items"`
}

type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Detail
}

type persistedState struct {
	SchemaVersion int       `json:"schemaVersion"`
	Items         []Command `json:"items"`
}

type atomicWriter func(directory, target string, data []byte) error

type Store struct {
	mu          sync.RWMutex
	root        string
	path        string
	state       persistedState
	available   bool
	writeAtomic atomicWriter
}

func Open(root string) (*Store, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return nil, errors.New("terminal commands root must be absolute")
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, fmt.Errorf("initialize terminal commands directory: %w", err)
	}
	store := &Store{
		root:        root,
		path:        filepath.Join(root, "commands.json"),
		state:       emptyState(),
		available:   true,
		writeAtomic: writeAtomicPrivateFile,
	}
	state, err := readState(store.path)
	switch {
	case err == nil:
		store.state = state
		if chmodErr := os.Chmod(store.path, 0o600); chmodErr != nil {
			return nil, fmt.Errorf("protect terminal commands: %w", chmodErr)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(store.state); err != nil {
			return nil, err
		}
	case errors.Is(err, errInvalidOnDisk):
		store.available = false
	default:
		return nil, err
	}
	return store, nil
}

func ValidID(value string) bool {
	return idPattern.MatchString(value)
}

func ValidResourceVersion(value string) bool {
	return resourcePattern.MatchString(value)
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *Store) Replace(input ReplaceInput) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Snapshot{}, ErrUnavailable
	}
	if !ValidResourceVersion(input.ExpectedResourceVersion) {
		return Snapshot{}, &ValidationError{
			Field: "expectedResourceVersion", Detail: "a valid resourceVersion is required",
		}
	}
	if input.ExpectedResourceVersion != resourceVersion(s.state) {
		return Snapshot{}, ErrConflict
	}
	next, err := buildState(input.Items)
	if err != nil {
		return Snapshot{}, err
	}
	if err := s.persistLocked(next); err != nil {
		return Snapshot{}, err
	}
	s.state = next
	return s.snapshotLocked(), nil
}

func (s *Store) snapshotLocked() Snapshot {
	state := s.state
	if !s.available {
		state = emptyState()
	}
	result := Snapshot{
		SchemaVersion: SchemaVersion, ResourceVersion: resourceVersion(state),
		Available: s.available, Items: cloneCommands(state.Items),
	}
	if !s.available {
		result.Warning = unavailableWarning
	}
	return result
}

func (s *Store) persistLocked(state persistedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terminal commands: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxPersistedBytes {
		return &ValidationError{Field: "items", Detail: "terminal commands exceed the storage limit"}
	}
	if err := s.writeAtomic(s.root, s.path, data); err != nil {
		return fmt.Errorf("persist terminal commands: %w", err)
	}
	return nil
}

func emptyState() persistedState {
	return persistedState{SchemaVersion: SchemaVersion, Items: []Command{}}
}

func readState(path string) (persistedState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return persistedState{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return persistedState{}, errors.New("terminal commands must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxPersistedBytes {
		return persistedState{}, errInvalidOnDisk
	}
	file, err := os.Open(path)
	if err != nil {
		return persistedState{}, fmt.Errorf("open terminal commands: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPersistedBytes+1))
	if err != nil {
		return persistedState{}, fmt.Errorf("read terminal commands: %w", err)
	}
	if len(data) == 0 || len(data) > maxPersistedBytes {
		return persistedState{}, errInvalidOnDisk
	}
	var state persistedState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedState{}, errInvalidOnDisk
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return persistedState{}, errInvalidOnDisk
	}
	if err := validateState(state); err != nil {
		return persistedState{}, errInvalidOnDisk
	}
	state.Items = cloneCommands(state.Items)
	return state, nil
}

func buildState(items []Command) (persistedState, error) {
	state := persistedState{SchemaVersion: SchemaVersion, Items: cloneCommands(items)}
	if err := validateState(state); err != nil {
		return persistedState{}, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return persistedState{}, err
	}
	if len(data)+1 > maxPersistedBytes {
		return persistedState{}, &ValidationError{Field: "items", Detail: "terminal commands exceed the storage limit"}
	}
	return state, nil
}

func validateState(state persistedState) error {
	if state.SchemaVersion != SchemaVersion {
		return &ValidationError{Field: "schemaVersion", Detail: "unsupported terminal commands schema"}
	}
	if len(state.Items) > MaxCommands {
		return &ValidationError{Field: "items", Detail: "at most 64 terminal commands are allowed"}
	}
	seen := make(map[string]bool, len(state.Items))
	for _, item := range state.Items {
		if !ValidID(item.ID) || seen[item.ID] {
			return &ValidationError{Field: "items", Detail: "terminal command IDs must be unique 32-character lowercase hexadecimal values"}
		}
		seen[item.ID] = true
		if strings.TrimSpace(item.Name) != item.Name || item.Name == "" ||
			!utf8.ValidString(item.Name) || utf8.RuneCountInString(item.Name) > MaxNameRunes || hasControl(item.Name, false) {
			return &ValidationError{Field: "items.name", Detail: "terminal command name must contain 1 to 48 characters without controls or outer whitespace"}
		}
		if strings.TrimSpace(item.Command) == "" || !utf8.ValidString(item.Command) ||
			len([]byte(item.Command)) > MaxCommandBytes || hasControl(item.Command, true) {
			return &ValidationError{Field: "items.command", Detail: "terminal command must contain 1 to 8192 bytes without unsupported controls"}
		}
	}
	return nil
}

func hasControl(value string, allowFormatting bool) bool {
	for _, character := range value {
		if unicode.IsControl(character) && (!allowFormatting || character != '\n' && character != '\t') {
			return true
		}
	}
	return false
}

func cloneCommands(source []Command) []Command {
	cloned := make([]Command, len(source))
	copy(cloned, source)
	return cloned
}

func resourceVersion(state persistedState) string {
	canonical := persistedState{SchemaVersion: SchemaVersion, Items: cloneCommands(state.Items)}
	data, _ := json.Marshal(canonical)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path must be a non-symlink directory")
	}
	return os.Chmod(path, 0o700)
}

func writeAtomicPrivateFile(directory, target string, data []byte) error {
	file, err := os.CreateTemp(directory, ".terminal-commands-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err == nil {
		_ = syncDirectory(directory)
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	backup := target + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	_ = syncDirectory(directory)
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
