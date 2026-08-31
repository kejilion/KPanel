package notification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	stateFileName           = "notification-state.json"
	telegramTokenName       = "telegram-bot-token"
	maxStateBytes     int64 = 4 << 20
	maxTokenBytes     int64 = MaxTelegramTokenBytes + 1
)

type telegramState struct {
	TokenFingerprint string         `json:"tokenFingerprint,omitempty"`
	BotID            int64          `json:"botId,omitempty"`
	BotUsername      string         `json:"botUsername,omitempty"`
	ChatID           int64          `json:"chatId,omitempty"`
	HasChat          bool           `json:"hasChat,omitempty"`
	Status           TelegramStatus `json:"status"`
	LastCheckedAt    *time.Time     `json:"lastCheckedAt,omitempty"`
	LastSuccessAt    *time.Time     `json:"lastSuccessAt,omitempty"`
	LastErrorCode    string         `json:"lastErrorCode,omitempty"`
}

type alertState struct {
	Active         bool      `json:"active,omitempty"`
	Consecutive    int       `json:"consecutive,omitempty"`
	LastAttemptAt  time.Time `json:"lastAttemptAt,omitempty"`
	LastNotifiedAt time.Time `json:"lastNotifiedAt,omitempty"`
	LastEventID    string    `json:"lastEventId,omitempty"`
	PendingEventID string    `json:"pendingEventId,omitempty"`
	LastValue      float64   `json:"lastValue,omitempty"`
}

type persistedState struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	Settings        Settings              `json:"settings"`
	Telegram        telegramState         `json:"telegram"`
	UpdatedAt       time.Time             `json:"updatedAt"`
	ResourceVersion string                `json:"resourceVersion"`
	AlertStates     map[string]alertState `json:"alertStates,omitempty"`
}

type Store struct {
	mu        sync.RWMutex
	directory string
	statePath string
	tokenPath string
	state     persistedState
}

func Open(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" || !filepath.IsAbs(dataDir) {
		return nil, errors.New("notification data directory must be absolute")
	}
	directory := filepath.Join(filepath.Clean(dataDir), "notifications")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create notification directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("protect notification directory: %w", err)
	}
	store := &Store{
		directory: directory,
		statePath: filepath.Join(directory, stateFileName),
		tokenPath: filepath.Join(directory, telegramTokenName),
		state: persistedState{
			SchemaVersion: StateSchemaVersion,
			Settings:      Settings{Rules: DefaultRules()},
			Telegram:      telegramState{Status: TelegramNotConfigured},
			AlertStates:   make(map[string]alertState),
		},
	}
	content, err := readRegularFile(store.statePath, maxStateBytes, false)
	if err == nil {
		if err := decodeState(content, &store.state); err != nil {
			return nil, err
		}
		if err := os.Chmod(store.statePath, 0o600); err != nil {
			return nil, fmt.Errorf("protect notification state: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := store.persistLocked(store.state); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("read notification state: %w", err)
	}
	return store, nil
}

func decodeState(content []byte, state *persistedState) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(state); err != nil {
		return fmt.Errorf("decode notification state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("notification state contains multiple JSON values")
	}
	if state.SchemaVersion != StateSchemaVersion {
		return errors.New("notification state is invalid")
	}
	if err := validateAlertStates(state.AlertStates); err != nil {
		return err
	}
	if err := state.Settings.Rules.Validate(); err != nil {
		return fmt.Errorf("notification state rules are invalid: %w", err)
	}
	if state.ResourceVersion != "" && !validResourceVersion(state.ResourceVersion) {
		return errors.New("notification state resource version is invalid")
	}
	if state.Telegram.Status != "" && !validTelegramStatus(state.Telegram.Status) {
		return errors.New("notification state telegram status is invalid")
	}
	if state.Telegram.BotUsername != "" && !validDisplayText(state.Telegram.BotUsername, 64) {
		return errors.New("notification state bot username is invalid")
	}
	if state.Telegram.TokenFingerprint != "" && !validHexString(state.Telegram.TokenFingerprint, sha256.Size*2) {
		return errors.New("notification state token fingerprint is invalid")
	}
	if state.Telegram.Status == "" {
		state.Telegram.Status = TelegramNotConfigured
	}
	if state.AlertStates == nil {
		state.AlertStates = make(map[string]alertState)
	}
	return nil
}

func (s *Store) stateSnapshot() persistedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePersistedState(s.state)
}

func (s *Store) commitState(next persistedState) error {
	if next.SchemaVersion == 0 {
		next.SchemaVersion = StateSchemaVersion
	}
	if err := decodeStateForCommit(next); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistLocked(next); err != nil {
		return err
	}
	s.state = clonePersistedState(next)
	return nil
}

func decodeStateForCommit(state persistedState) error {
	if state.SchemaVersion != StateSchemaVersion {
		return errors.New("notification state is invalid")
	}
	if err := validateAlertStates(state.AlertStates); err != nil {
		return err
	}
	if err := state.Settings.Rules.Validate(); err != nil {
		return err
	}
	if state.ResourceVersion != "" && !validResourceVersion(state.ResourceVersion) {
		return errors.New("notification state resource version is invalid")
	}
	if !validTelegramStatus(state.Telegram.Status) {
		return errors.New("notification state telegram status is invalid")
	}
	return nil
}

func validateAlertStates(states map[string]alertState) error {
	if len(states) > MaxAlertStates {
		return errors.New("notification state contains too many alert states")
	}
	for key, value := range states {
		if !validAlertKey(key) || value.Consecutive < 0 || value.Consecutive > DefaultSustainSamples ||
			(value.LastEventID != "" && !validDisplayText(value.LastEventID, 160)) ||
			(value.PendingEventID != "" && !validDisplayText(value.PendingEventID, 160)) ||
			math.IsNaN(value.LastValue) || math.IsInf(value.LastValue, 0) {
			return errors.New("notification state alert state is invalid")
		}
	}
	return nil
}

func (s *Store) token() (string, bool, error) {
	content, err := readRegularFile(s.tokenPath, maxTokenBytes, true)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	token := strings.TrimSpace(string(content))
	if token == "" || !ValidBotToken(token) {
		return "", false, ErrTelegramInvalidToken
	}
	return token, true, nil
}

func (s *Store) replaceToken(token string) error {
	return atomicWrite(s.tokenPath, []byte(token+"\n"), 0o600)
}

func (s *Store) restoreToken(token string, present bool) error {
	if present {
		return s.replaceToken(token)
	}
	if err := removeRegularFile(s.tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) persistLocked(state persistedState) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode notification state: %w", err)
	}
	content = append(content, '\n')
	if int64(len(content)) > maxStateBytes {
		return errors.New("notification state exceeds size limit")
	}
	return atomicWrite(s.statePath, content, 0o600)
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".notification-*")
	if err != nil {
		return fmt.Errorf("create temporary notification file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary notification file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary notification file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary notification file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary notification file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		_ = syncDirectory(directory)
		_ = os.Chmod(path, mode)
		return nil
	}
	backupPath := path + ".previous"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("prepare notification replacement: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("replace notification file: %w", err)
	}
	_ = os.Remove(backupPath)
	_ = syncDirectory(directory)
	_ = os.Chmod(path, mode)
	return nil
}

func readRegularFile(path string, limit int64, secret bool) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("notification file is not a regular file")
	}
	if secret && runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("telegram token file is too broadly accessible")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("notification file changed while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("notification file is too large")
	}
	return content, nil
}

func removeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("notification file is not a regular file")
	}
	return os.Remove(path)
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

func clonePersistedState(source persistedState) persistedState {
	alertStates := make(map[string]alertState, len(source.AlertStates))
	for key, value := range source.AlertStates {
		alertStates[key] = value
	}
	source.AlertStates = alertStates
	if source.Telegram.LastCheckedAt != nil {
		value := *source.Telegram.LastCheckedAt
		source.Telegram.LastCheckedAt = &value
	}
	if source.Telegram.LastSuccessAt != nil {
		value := *source.Telegram.LastSuccessAt
		source.Telegram.LastSuccessAt = &value
	}
	return source
}

func tokenFingerprint(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func validTelegramStatus(value TelegramStatus) bool {
	switch value {
	case TelegramNotConfigured, TelegramWaitingChat, TelegramReady, TelegramError:
		return true
	default:
		return false
	}
}

func validResourceVersion(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	return validHexString(strings.TrimPrefix(value, "sha256:"), sha256.Size*2)
}

func validHexString(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validDisplayText(value string, maxBytes int) bool {
	if strings.TrimSpace(value) != value || value == "" || len(value) > maxBytes || strings.ContainsAny(value, "\r\n\t\x00") {
		return false
	}
	for _, character := range value {
		if character < ' ' || character == '\u007f' {
			return false
		}
	}
	return true
}

func validAlertKey(value string) bool {
	return validDisplayText(value, 256)
}
