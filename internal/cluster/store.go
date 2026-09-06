package cluster

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

const maxClusterStoreBytes int64 = 4 << 20

type hostRecord struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Origin              string        `json:"origin"`
	RemoteNodeID        string        `json:"remoteNodeId"`
	ControllerID        string        `json:"controllerId"`
	CredentialFile      string        `json:"credentialFile"`
	FederationProtocol  string        `json:"federationProtocol"`
	PanelVersion        string        `json:"panelVersion,omitempty"`
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

type controllerRecord struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	PublicKey   string     `json:"publicKey"`
	Fingerprint string     `json:"fingerprint"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt,omitempty"`
}

type pairingCodeRecord struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	ExpiresAt time.Time `json:"expiresAt"`
	Attempts  int       `json:"attempts"`
}

type persistedState struct {
	SchemaVersion int                 `json:"schemaVersion"`
	NodeID        string              `json:"nodeId"`
	LocalName     string              `json:"localName,omitempty"`
	Hosts         []hostRecord        `json:"hosts"`
	Controllers   []controllerRecord  `json:"controllers"`
	PairingCodes  []pairingCodeRecord `json:"pairingCodes"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	state persistedState
}

func OpenStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return nil, errors.New("cluster store path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create cluster store directory: %w", err)
	}
	store := &Store{path: path, state: persistedState{SchemaVersion: 1}}
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, errors.New("cluster store must be a regular file")
		}
		if len(content) == 0 || int64(len(content)) > maxClusterStoreBytes {
			return nil, errors.New("cluster store size is invalid")
		}
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&store.state); err != nil {
			return nil, fmt.Errorf("decode cluster store: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("decode cluster store: multiple JSON values")
		}
		if store.state.SchemaVersion != 1 {
			return nil, fmt.Errorf("unsupported cluster store schema %d", store.state.SchemaVersion)
		}
	case errors.Is(err, os.ErrNotExist):
		nodeID, idErr := randomHex(16)
		if idErr != nil {
			return nil, idErr
		}
		store.state.NodeID = nodeID
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read cluster store: %w", err)
	}
	if !validID(store.state.NodeID) {
		return nil, errors.New("cluster node ID is invalid")
	}
	if len(store.state.Hosts) > MaxHosts {
		return nil, errors.New("cluster store exceeds the host limit")
	}
	if err := validatePersistedState(store.state); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect cluster store: %w", err)
	}
	return store, nil
}

func (s *Store) NodeID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.NodeID
}

func (s *Store) LocalName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.LocalName
}

func (s *Store) SetLocalName(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	s.state.LocalName = name
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *Store) Hosts() []hostRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := cloneHosts(s.state.Hosts)
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (s *Store) Host(id string) (hostRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.state.Hosts {
		if item.ID == id {
			return cloneHost(item), nil
		}
	}
	return hostRecord{}, ErrNotFound
}

func (s *Store) AddHost(record hostRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.Hosts) >= MaxHosts {
		return ErrHostLimit
	}
	for _, item := range s.state.Hosts {
		if strings.EqualFold(item.Origin, record.Origin) || item.RemoteNodeID == record.RemoteNodeID {
			return ErrDuplicate
		}
	}
	previous := cloneState(s.state)
	record.ResourceVersion = hostResourceVersion(record)
	s.state.Hosts = append(s.state.Hosts, cloneHost(record))
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *Store) RenameHost(id, name, expected string, now time.Time) (hostRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Hosts {
		item := &s.state.Hosts[index]
		if item.ID != id {
			continue
		}
		if item.ResourceVersion != expected {
			return hostRecord{}, ErrConflict
		}
		previous := cloneState(s.state)
		item.Name = name
		item.UpdatedAt = now.UTC()
		item.ResourceVersion = hostResourceVersion(*item)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return hostRecord{}, err
		}
		return cloneHost(*item), nil
	}
	return hostRecord{}, ErrNotFound
}

func (s *Store) DeleteHost(id, expected string) (hostRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.state.Hosts {
		if item.ID != id {
			continue
		}
		if item.ResourceVersion != expected {
			return hostRecord{}, ErrConflict
		}
		previous := cloneState(s.state)
		s.state.Hosts = append(s.state.Hosts[:index], s.state.Hosts[index+1:]...)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return hostRecord{}, err
		}
		return cloneHost(item), nil
	}
	return hostRecord{}, ErrNotFound
}

func (s *Store) Checkpoint(runtime map[string]runtimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	for index := range s.state.Hosts {
		current, ok := runtime[s.state.Hosts[index].ID]
		if !ok {
			continue
		}
		s.state.Hosts[index].LastSnapshot = cloneSnapshot(current.snapshot)
		s.state.Hosts[index].LastAttemptAt = cloneTime(current.lastAttemptAt)
		s.state.Hosts[index].LastSuccessAt = cloneTime(current.lastSuccessAt)
		s.state.Hosts[index].ConsecutiveFailures = current.consecutiveFailures
		s.state.Hosts[index].LastErrorCode = current.lastErrorCode
		s.state.Hosts[index].LastError = current.lastError
		if current.panelVersion != "" {
			s.state.Hosts[index].PanelVersion = current.panelVersion
		}
	}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *Store) CreatePairingCode(now time.Time) (PairingCode, error) {
	id, err := randomHex(8)
	if err != nil {
		return PairingCode{}, err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return PairingCode{}, fmt.Errorf("generate pairing code: %w", err)
	}
	secret := hex.EncodeToString(secretBytes)
	sum := sha256.Sum256([]byte(id + "." + secret))
	expires := now.UTC().Add(5 * time.Minute)

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	s.removeExpiredCodesLocked(now)
	s.state.PairingCodes = append(s.state.PairingCodes, pairingCodeRecord{
		ID: id, Hash: hex.EncodeToString(sum[:]), ExpiresAt: expires,
	})
	if len(s.state.PairingCodes) > 16 {
		s.state.PairingCodes = append([]pairingCodeRecord(nil), s.state.PairingCodes[len(s.state.PairingCodes)-16:]...)
	}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return PairingCode{}, err
	}
	return PairingCode{Code: id + "." + secret, Scope: SummaryScope, ExpiresAt: expires}, nil
}

func (s *Store) ConsumePairingCode(code string, controller controllerRecord, now time.Time) error {
	code = strings.TrimSpace(code)
	id, secret, ok := strings.Cut(code, ".")
	if !ok || len(id) != 16 || len(secret) != 64 {
		return ErrPairingCode
	}
	if _, err := hex.DecodeString(id + secret); err != nil {
		return ErrPairingCode
	}
	sum := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(sum[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneState(s.state)
	s.removeExpiredCodesLocked(now)
	index := -1
	for i := range s.state.PairingCodes {
		if s.state.PairingCodes[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ErrPairingCode
	}
	record := &s.state.PairingCodes[index]
	if subtle.ConstantTimeCompare([]byte(record.Hash), []byte(hash)) != 1 {
		record.Attempts++
		if record.Attempts >= 5 {
			s.state.PairingCodes = append(s.state.PairingCodes[:index], s.state.PairingCodes[index+1:]...)
		}
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return err
		}
		return ErrPairingCode
	}
	for _, item := range s.state.Controllers {
		if item.ID == controller.ID {
			return ErrDuplicate
		}
	}
	s.state.PairingCodes = append(s.state.PairingCodes[:index], s.state.PairingCodes[index+1:]...)
	s.state.Controllers = append(s.state.Controllers, controller)
	if len(s.state.Controllers) > 256 {
		s.state = previous
		return errors.New("cluster controller limit reached")
	}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *Store) Controllers() []controllerRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := append([]controllerRecord(nil), s.state.Controllers...)
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *Store) Controller(id string) (controllerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.state.Controllers {
		if item.ID == id {
			return item, nil
		}
	}
	return controllerRecord{}, ErrNotFound
}

func (s *Store) DeleteController(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.state.Controllers {
		if item.ID != id {
			continue
		}
		previous := cloneState(s.state)
		s.state.Controllers = append(s.state.Controllers[:index], s.state.Controllers[index+1:]...)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return err
		}
		return nil
	}
	return ErrNotFound
}

func (s *Store) TouchController(id string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Controllers {
		item := &s.state.Controllers[index]
		if item.ID != id || (item.LastSeenAt != nil && now.Sub(*item.LastSeenAt) < 5*time.Minute) {
			continue
		}
		previous := cloneState(s.state)
		value := now.UTC()
		item.LastSeenAt = &value
		if err := s.persistLocked(); err != nil {
			s.state = previous
		}
		return
	}
}

func (s *Store) removeExpiredCodesLocked(now time.Time) {
	filtered := s.state.PairingCodes[:0]
	for _, item := range s.state.PairingCodes {
		if item.ExpiresAt.After(now) {
			filtered = append(filtered, item)
		}
	}
	s.state.PairingCodes = filtered
}

func (s *Store) persistLocked() error {
	content, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cluster store: %w", err)
	}
	content = append(content, '\n')
	if int64(len(content)) > maxClusterStoreBytes {
		return errors.New("cluster store exceeds 4 MiB")
	}
	directory := filepath.Dir(s.path)
	temp, err := os.CreateTemp(directory, ".cluster-state-*")
	if err != nil {
		return fmt.Errorf("create cluster store temporary file: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err == nil {
		return nil
	}
	backup := s.path + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(s.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("prepare cluster store replacement: %w", err)
	}
	if err := os.Rename(name, s.path); err != nil {
		_ = os.Rename(backup, s.path)
		return fmt.Errorf("replace cluster store: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}

func hostResourceVersion(record hostRecord) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		record.ID, record.Name, record.Origin, record.RemoteNodeID,
		record.FederationProtocol, record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneState(source persistedState) persistedState {
	return persistedState{
		SchemaVersion: source.SchemaVersion, NodeID: source.NodeID, LocalName: source.LocalName,
		Hosts:        cloneHosts(source.Hosts),
		Controllers:  append([]controllerRecord(nil), source.Controllers...),
		PairingCodes: append([]pairingCodeRecord(nil), source.PairingCodes...),
	}
}

func cloneHosts(source []hostRecord) []hostRecord {
	result := make([]hostRecord, len(source))
	for index := range source {
		result[index] = cloneHost(source[index])
	}
	return result
}

func cloneHost(source hostRecord) hostRecord {
	source.LastSnapshot = cloneSnapshot(source.LastSnapshot)
	source.LastAttemptAt = cloneTime(source.LastAttemptAt)
	source.LastSuccessAt = cloneTime(source.LastSuccessAt)
	return source
}

func cloneSnapshot(source *HostSnapshot) *HostSnapshot {
	if source == nil {
		return nil
	}
	value := *source
	if source.LightHealth != nil {
		health := *source.LightHealth
		value.LightHealth = &health
	}
	value.Telemetry.OSLike = append([]string(nil), source.Telemetry.OSLike...)
	return &value
}

func cloneTime(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random identifier: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validatePersistedState(state persistedState) error {
	if state.LocalName != "" {
		name, err := validateRequiredName(state.LocalName)
		if err != nil || name != state.LocalName {
			return errors.New("cluster store contains an invalid local name")
		}
	}
	hostIDs := make(map[string]struct{}, len(state.Hosts))
	origins := make(map[string]struct{}, len(state.Hosts))
	nodes := make(map[string]struct{}, len(state.Hosts))
	for _, host := range state.Hosts {
		normalized, err := NormalizeOrigin(host.Origin)
		if !validID(host.ID) || !validID(host.RemoteNodeID) ||
			!validID(host.ControllerID) || !validCredentialName(host.CredentialFile) ||
			host.FederationProtocol != FederationProtocol || err != nil ||
			normalized != host.Origin || host.ResourceVersion != hostResourceVersion(host) {
			return errors.New("cluster store contains an invalid host record")
		}
		if _, exists := hostIDs[host.ID]; exists {
			return errors.New("cluster store contains a duplicate host ID")
		}
		if _, exists := origins[strings.ToLower(host.Origin)]; exists {
			return errors.New("cluster store contains a duplicate host origin")
		}
		if _, exists := nodes[host.RemoteNodeID]; exists {
			return errors.New("cluster store contains a duplicate remote node")
		}
		hostIDs[host.ID] = struct{}{}
		origins[strings.ToLower(host.Origin)] = struct{}{}
		nodes[host.RemoteNodeID] = struct{}{}
	}
	controllerIDs := make(map[string]struct{}, len(state.Controllers))
	for _, controller := range state.Controllers {
		publicKey, err := decodePublicKey(controller.PublicKey)
		if !validID(controller.ID) || controller.Scope != SummaryScope ||
			err != nil || controller.Fingerprint != fingerprint(publicKey) {
			return errors.New("cluster store contains an invalid controller record")
		}
		if _, exists := controllerIDs[controller.ID]; exists {
			return errors.New("cluster store contains a duplicate controller ID")
		}
		controllerIDs[controller.ID] = struct{}{}
	}
	for _, code := range state.PairingCodes {
		if len(code.ID) != 16 || len(code.Hash) != 64 || code.Attempts < 0 || code.Attempts >= 5 {
			return errors.New("cluster store contains an invalid pairing code")
		}
		if _, err := hex.DecodeString(code.ID + code.Hash); err != nil {
			return errors.New("cluster store contains an invalid pairing code")
		}
	}
	return nil
}
