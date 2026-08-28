package cluster

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	lightStateFileName         = "cluster-light-state.json"
	lightSecretsDirectory      = "cluster-light-secrets"
	lightTerminalKeysDirectory = "cluster-light-terminal-keys"
	maxLightStateBytes         = int64(4 << 20)
	maxLightEnrollments        = 16
	lightCredentialExtension   = ".lightkey"
	lightTerminalKeyExtension  = ".noisepub"
)

type lightEnrollmentRecord struct {
	ID         string    `json:"id"`
	SecretHash string    `json:"secretHash"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type lightHostRecord struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	CredentialFile      string        `json:"credentialFile"`
	NodeVersion         string        `json:"nodeVersion"`
	ResourceVersion     string        `json:"resourceVersion"`
	CreatedAt           time.Time     `json:"createdAt"`
	UpdatedAt           time.Time     `json:"updatedAt"`
	LastSnapshot        *HostSnapshot `json:"lastSnapshot,omitempty"`
	LastAttemptAt       *time.Time    `json:"lastAttemptAt,omitempty"`
	LastSuccessAt       *time.Time    `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int           `json:"consecutiveFailures,omitempty"`
	LastErrorCode       string        `json:"lastErrorCode,omitempty"`
	LastError           string        `json:"lastError,omitempty"`
}

type lightPersistedState struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Enrollments   []lightEnrollmentRecord `json:"enrollments"`
	Hosts         []lightHostRecord       `json:"hosts"`
}

type lightStore struct {
	mu          sync.RWMutex
	path        string
	secretDir   string
	terminalDir string
	state       lightPersistedState
	dirty       bool
	ops         atomicFileOpsV2
}

func openLightStore(path string) (*lightStore, error) {
	if !filepath.IsAbs(path) || filepath.Base(filepath.Clean(path)) != lightStateFileName {
		return nil, errors.New("light node store path is invalid")
	}
	directory := filepath.Dir(path)
	secretDir := filepath.Join(directory, lightSecretsDirectory)
	terminalDir := filepath.Join(directory, lightTerminalKeysDirectory)
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		return nil, fmt.Errorf("create light node secret directory: %w", err)
	}
	if err := os.MkdirAll(terminalDir, 0o700); err != nil {
		return nil, fmt.Errorf("create light terminal key directory: %w", err)
	}
	if err := protectDirectoryV2(directory); err != nil {
		return nil, err
	}
	if err := protectDirectoryV2(secretDir); err != nil {
		return nil, err
	}
	if err := protectDirectoryV2(terminalDir); err != nil {
		return nil, err
	}
	store := &lightStore{
		path: path, secretDir: secretDir, terminalDir: terminalDir, ops: defaultAtomicFileOpsV2(),
		state: lightPersistedState{SchemaVersion: 1, Enrollments: []lightEnrollmentRecord{}, Hosts: []lightHostRecord{}},
	}
	content, err := readRegularFileV2(path, maxLightStateBytes, false)
	switch {
	case err == nil:
		if err := decodeLightState(content, &store.state); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read light node store: %w", err)
	}
	if err := store.cleanupOrphanCredentials(); err != nil {
		return nil, err
	}
	return store, nil
}

func decodeLightState(content []byte, state *lightPersistedState) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(state); err != nil {
		return fmt.Errorf("decode light node store: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("light node store contains multiple JSON values")
	}
	if state.SchemaVersion != 1 || len(state.Enrollments) > maxLightEnrollments || len(state.Hosts) > MaxHosts {
		return errors.New("light node store is invalid")
	}
	hostIDs := make(map[string]struct{}, len(state.Hosts))
	credentialFiles := make(map[string]struct{}, len(state.Hosts))
	for _, host := range state.Hosts {
		validatedName, nameErr := validateRequiredName(host.Name)
		if !validID(host.ID) || nameErr != nil || validatedName != host.Name || !validResourceVersion(host.ResourceVersion) ||
			!validLightCredentialName(host.CredentialFile) || host.CreatedAt.IsZero() || host.UpdatedAt.IsZero() {
			return errors.New("light node store contains an invalid host")
		}
		if _, exists := hostIDs[host.ID]; exists {
			return errors.New("light node store contains a duplicate host")
		}
		if _, exists := credentialFiles[host.CredentialFile]; exists {
			return errors.New("light node store contains a duplicate credential")
		}
		hostIDs[host.ID] = struct{}{}
		credentialFiles[host.CredentialFile] = struct{}{}
	}
	enrollmentIDs := make(map[string]struct{}, len(state.Enrollments))
	for _, enrollment := range state.Enrollments {
		decodedHash, err := hex.DecodeString(enrollment.SecretHash)
		if !validID(enrollment.ID) || err != nil || len(decodedHash) != sha256.Size || enrollment.ExpiresAt.IsZero() {
			return errors.New("light node store contains an invalid enrollment")
		}
		if _, exists := enrollmentIDs[enrollment.ID]; exists {
			return errors.New("light node store contains a duplicate enrollment")
		}
		enrollmentIDs[enrollment.ID] = struct{}{}
	}
	return nil
}

func (s *lightStore) Hosts() []lightHostRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := append([]lightHostRecord(nil), s.state.Hosts...)
	for index := range result {
		result[index].LastSnapshot = cloneSnapshot(result[index].LastSnapshot)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result
}

func (s *lightStore) Host(id string) (lightHostRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Hosts {
		if record.ID == id {
			record.LastSnapshot = cloneSnapshot(record.LastSnapshot)
			return record, nil
		}
	}
	return lightHostRecord{}, ErrNotFound
}

func (s *lightStore) AddEnrollment(record lightEnrollmentRecord, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]lightEnrollmentRecord(nil), s.state.Enrollments...)
	s.gcEnrollmentsLocked(now)
	decodedHash, err := hex.DecodeString(record.SecretHash)
	if !validID(record.ID) || err != nil || len(decodedHash) != sha256.Size || !record.ExpiresAt.After(now.UTC()) {
		return ErrPairingCode
	}
	for _, existing := range s.state.Enrollments {
		if existing.ID == record.ID {
			return ErrDuplicate
		}
	}
	if len(s.state.Enrollments) >= maxLightEnrollments {
		return ErrRateLimited
	}
	s.state.Enrollments = append(s.state.Enrollments, record)
	if err := s.persistLocked(); err != nil {
		s.state.Enrollments = previous
		return err
	}
	return nil
}

func (s *lightStore) ConsumeEnrollment(id, secretHash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for current, record := range s.state.Enrollments {
		if record.ID == id && record.SecretHash == secretHash && record.ExpiresAt.After(now.UTC()) {
			index = current
			break
		}
	}
	if index < 0 {
		return ErrPairingCode
	}
	previous := append([]lightEnrollmentRecord(nil), s.state.Enrollments...)
	s.state.Enrollments = append(s.state.Enrollments[:index], s.state.Enrollments[index+1:]...)
	if err := s.persistLocked(); err != nil {
		s.state.Enrollments = previous
		return err
	}
	return nil
}

func (s *lightStore) EnrollHost(
	enrollmentID string,
	secretHash string,
	record lightHostRecord,
	reportingKey []byte,
	terminalPublicKey []byte,
	now time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousEnrollments := append([]lightEnrollmentRecord(nil), s.state.Enrollments...)
	previousHosts := append([]lightHostRecord(nil), s.state.Hosts...)
	s.gcEnrollmentsLocked(now)
	enrollmentIndex := -1
	for index, enrollment := range s.state.Enrollments {
		if enrollment.ID == enrollmentID && enrollment.SecretHash == secretHash && enrollment.ExpiresAt.After(now.UTC()) {
			enrollmentIndex = index
			break
		}
	}
	if enrollmentIndex < 0 {
		return ErrPairingCode
	}
	if len(s.state.Hosts) >= MaxHosts {
		return ErrHostLimit
	}
	validatedName, nameErr := validateRequiredName(record.Name)
	if !validID(record.ID) || nameErr != nil || validatedName != record.Name || len(reportingKey) != 32 ||
		(len(terminalPublicKey) != 0 && len(terminalPublicKey) != 32) ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("light node host is invalid")
	}
	for _, existing := range s.state.Hosts {
		if existing.ID == record.ID {
			return ErrDuplicate
		}
	}
	credentialFile := "host-" + record.ID + lightCredentialExtension
	credentialPath := filepath.Join(s.secretDir, credentialFile)
	if err := atomicWriteFileV2(credentialPath, []byte(base64.RawURLEncoding.EncodeToString(reportingKey)), 0o600, false, s.ops); err != nil {
		return fmt.Errorf("write light node credential: %w", err)
	}
	terminalPath := terminalKeyPath(s.terminalDir, record.ID)
	terminalWritten := false
	if len(terminalPublicKey) > 0 {
		if err := atomicWriteFileV2(
			terminalPath,
			[]byte(base64.RawURLEncoding.EncodeToString(terminalPublicKey)),
			0o600,
			false,
			s.ops,
		); err != nil {
			_ = os.Remove(credentialPath)
			return fmt.Errorf("write light node terminal key: %w", err)
		}
		terminalWritten = true
	}
	record.CredentialFile = credentialFile
	record.ResourceVersion = lightResourceVersion(record)
	s.state.Enrollments = append(
		append([]lightEnrollmentRecord(nil), s.state.Enrollments[:enrollmentIndex]...),
		s.state.Enrollments[enrollmentIndex+1:]...,
	)
	s.state.Hosts = append(s.state.Hosts, record)
	if err := s.persistLocked(); err != nil {
		s.state.Enrollments = previousEnrollments
		s.state.Hosts = previousHosts
		_ = os.Remove(credentialPath)
		if terminalWritten {
			_ = os.Remove(terminalPath)
		}
		return err
	}
	return nil
}

func (s *lightStore) AddHost(record lightHostRecord, secret []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.Hosts) >= MaxHosts {
		return ErrHostLimit
	}
	validatedName, nameErr := validateRequiredName(record.Name)
	if !validID(record.ID) || nameErr != nil || validatedName != record.Name || len(secret) != 32 ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("light node host is invalid")
	}
	for _, existing := range s.state.Hosts {
		if existing.ID == record.ID {
			return ErrDuplicate
		}
	}
	name := "host-" + record.ID + lightCredentialExtension
	if err := atomicWriteFileV2(filepath.Join(s.secretDir, name), []byte(base64.RawURLEncoding.EncodeToString(secret)), 0o600, false, s.ops); err != nil {
		return fmt.Errorf("write light node credential: %w", err)
	}
	record.CredentialFile = name
	record.ResourceVersion = lightResourceVersion(record)
	s.state.Hosts = append(s.state.Hosts, record)
	if err := s.persistLocked(); err != nil {
		s.state.Hosts = s.state.Hosts[:len(s.state.Hosts)-1]
		_ = os.Remove(filepath.Join(s.secretDir, name))
		return err
	}
	return nil
}

func (s *lightStore) ReadSecret(record lightHostRecord) ([]byte, error) {
	if !validLightCredentialName(record.CredentialFile) {
		return nil, ErrAuthentication
	}
	content, err := readRegularFileV2(filepath.Join(s.secretDir, record.CredentialFile), 128, true)
	if err != nil {
		return nil, ErrAuthentication
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(content)))
	if err != nil || len(secret) != 32 {
		return nil, ErrAuthentication
	}
	return secret, nil
}

func (s *lightStore) ReadTerminalPublicKey(record lightHostRecord) ([]byte, error) {
	if !validID(record.ID) {
		return nil, ErrAuthentication
	}
	content, err := readRegularFileV2(terminalKeyPath(s.terminalDir, record.ID), 128, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrAuthentication
	}
	key, err := decodeTerminalRelayPublicKey(strings.TrimSpace(string(content)))
	if err != nil || len(key) != 32 {
		return nil, ErrAuthentication
	}
	return key, nil
}

func (s *lightStore) UpdateReport(id string, snapshot HostSnapshot, nodeVersion string, now time.Time) (lightHostRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Hosts {
		if s.state.Hosts[index].ID != id {
			continue
		}
		record := &s.state.Hosts[index]
		if prior := record.LastSnapshot; prior != nil && now.After(prior.ReceivedAt) {
			elapsed := now.Sub(prior.ReceivedAt).Seconds()
			if snapshot.Telemetry.Network.ReceivedBytes >= prior.Telemetry.Network.ReceivedBytes {
				snapshot.ReceiveBytesPerSecond = float64(
					snapshot.Telemetry.Network.ReceivedBytes-prior.Telemetry.Network.ReceivedBytes,
				) / elapsed
			}
			if snapshot.Telemetry.Network.SentBytes >= prior.Telemetry.Network.SentBytes {
				snapshot.TransmitBytesPerSecond = float64(
					snapshot.Telemetry.Network.SentBytes-prior.Telemetry.Network.SentBytes,
				) / elapsed
			}
		}
		record.LastSnapshot = cloneSnapshot(&snapshot)
		record.LastAttemptAt = timePointer(now)
		record.LastSuccessAt = timePointer(now)
		record.ConsecutiveFailures = 0
		record.LastErrorCode = ""
		record.LastError = ""
		record.NodeVersion = cleanDisplayText(nodeVersion, 64)
		s.dirty = true
		result := *record
		result.LastSnapshot = cloneSnapshot(record.LastSnapshot)
		return result, nil
	}
	return lightHostRecord{}, ErrNotFound
}

func (s *lightStore) RenameHost(id, name, expected string, now time.Time) (lightHostRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Hosts {
		record := &s.state.Hosts[index]
		if record.ID != id {
			continue
		}
		if record.ResourceVersion != expected {
			return lightHostRecord{}, ErrConflict
		}
		previous := *record
		record.Name = name
		record.UpdatedAt = now.UTC()
		record.ResourceVersion = lightResourceVersion(*record)
		if err := s.persistLocked(); err != nil {
			s.state.Hosts[index] = previous
			return lightHostRecord{}, err
		}
		return *record, nil
	}
	return lightHostRecord{}, ErrNotFound
}

func (s *lightStore) DeleteHost(id, expected string) (lightHostRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, record := range s.state.Hosts {
		if record.ID != id {
			continue
		}
		if record.ResourceVersion != expected {
			return lightHostRecord{}, false, ErrConflict
		}
		previous := append([]lightHostRecord(nil), s.state.Hosts...)
		s.state.Hosts = append(s.state.Hosts[:index], s.state.Hosts[index+1:]...)
		if err := s.persistLocked(); err != nil {
			s.state.Hosts = previous
			return lightHostRecord{}, false, err
		}
		credentialRemoved := true
		if err := os.Remove(filepath.Join(s.secretDir, record.CredentialFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			credentialRemoved = false
		}
		if err := os.Remove(terminalKeyPath(s.terminalDir, record.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			credentialRemoved = false
		}
		return record, credentialRemoved, nil
	}
	return lightHostRecord{}, false, ErrNotFound
}

func (s *lightStore) cleanupOrphanCredentials() error {
	referenced := make(map[string]struct{}, len(s.state.Hosts))
	referencedTerminalKeys := make(map[string]struct{}, len(s.state.Hosts))
	for _, record := range s.state.Hosts {
		referenced[record.CredentialFile] = struct{}{}
		referencedTerminalKeys[terminalKeyName(record.ID)] = struct{}{}
	}
	entries, err := os.ReadDir(s.secretDir)
	if err != nil {
		return fmt.Errorf("read light node secret directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && validLightCredentialName(name) {
			if _, ok := referenced[name]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(s.secretDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove orphan light node credential: %w", err)
			}
		}
	}
	terminalEntries, err := os.ReadDir(s.terminalDir)
	if err != nil {
		return fmt.Errorf("read light terminal key directory: %w", err)
	}
	for _, entry := range terminalEntries {
		name := entry.Name()
		if entry.Type().IsRegular() && validLightTerminalKeyName(name) {
			if _, ok := referencedTerminalKeys[name]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(s.terminalDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove orphan light terminal key: %w", err)
			}
		}
	}
	return nil
}

func (s *lightStore) gcEnrollmentsLocked(now time.Time) {
	filtered := s.state.Enrollments[:0]
	for _, record := range s.state.Enrollments {
		if record.ExpiresAt.After(now.UTC()) {
			filtered = append(filtered, record)
		}
	}
	s.state.Enrollments = filtered
}

func (s *lightStore) Checkpoint(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.state.Enrollments)
	s.gcEnrollmentsLocked(now)
	if !s.dirty && before == len(s.state.Enrollments) {
		return nil
	}
	return s.persistLocked()
}

func (s *lightStore) persistLocked() error {
	content, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	if err := atomicWriteFileV2(s.path, append(content, '\n'), 0o600, true, s.ops); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

func validLightCredentialName(name string) bool {
	return strings.HasPrefix(name, "host-") && strings.HasSuffix(name, lightCredentialExtension) && !strings.ContainsAny(name, `/\\`) && len(name) == len("host-")+32+len(lightCredentialExtension)
}

func validLightTerminalKeyName(name string) bool {
	return strings.HasPrefix(name, "host-") && strings.HasSuffix(name, lightTerminalKeyExtension) && !strings.ContainsAny(name, `/\\`) && len(name) == len("host-")+32+len(lightTerminalKeyExtension)
}

func terminalKeyName(id string) string {
	return "host-" + id + lightTerminalKeyExtension
}

func terminalKeyPath(directory, id string) string {
	return filepath.Join(directory, terminalKeyName(id))
}

func lightResourceVersion(record lightHostRecord) string {
	material := strings.Join([]string{
		record.ID, record.Name, record.CredentialFile,
		record.CreatedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(sum[:])
}
