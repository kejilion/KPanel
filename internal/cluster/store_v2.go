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
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	clusterStateV2FileName = "cluster-state-v2.json"
	maxClusterStoreV2Bytes = int64(4 << 20)
	maxPairingCodesV2      = 16
	maxControllersV2       = 256
)

type hostStateV2 string

const (
	hostStateV2PendingPair   hostStateV2 = "pending_pair"
	hostStateV2PendingCommit hostStateV2 = "pending_commit"
	hostStateV2Active        hostStateV2 = "active"
	hostStateV2PendingRevoke hostStateV2 = "pending_revoke"
)

type controllerStateV2 string

const (
	controllerStateV2Provisional controllerStateV2 = "provisional"
	controllerStateV2Active      controllerStateV2 = "active"
	controllerStateV2Revoked     controllerStateV2 = "revoked"
)

type pairingStateV2 string

const (
	pairingStateV2Issued pairingStateV2 = "issued"
	pairingStateV2Bound  pairingStateV2 = "bound"
)

type hostRecordV2 struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name,omitempty"`
	Origin                string            `json:"origin"`
	TransportSecurity     TransportSecurity `json:"transportSecurity"`
	RemoteNodeID          string            `json:"remoteNodeId"`
	ControllerID          string            `json:"controllerId"`
	State                 hostStateV2       `json:"state"`
	TransactionID         string            `json:"transactionId"`
	CredentialFile        string            `json:"credentialFile,omitempty"`
	PairingCredentialFile string            `json:"pairingCredentialFile,omitempty"`
	TargetPublicKey       string            `json:"targetPublicKey"`
	PeerFingerprint       string            `json:"peerFingerprint"`
	FederationProtocol    string            `json:"federationProtocol"`
	Scope                 string            `json:"scope,omitempty"`
	PanelVersion          string            `json:"panelVersion,omitempty"`
	ResourceVersion       string            `json:"resourceVersion"`
	CreatedAt             time.Time         `json:"createdAt"`
	UpdatedAt             time.Time         `json:"updatedAt"`
	LastSnapshot          *HostSnapshot     `json:"lastSnapshot,omitempty"`
	LastAttemptAt         *time.Time        `json:"lastAttemptAt,omitempty"`
	LastSuccessAt         *time.Time        `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures   int               `json:"consecutiveFailures,omitempty"`
	LastErrorCode         string            `json:"lastErrorCode,omitempty"`
	LastError             string            `json:"lastError,omitempty"`
}

type controllerRecordV2 struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	PublicKey     string            `json:"publicKey"`
	Fingerprint   string            `json:"fingerprint"`
	Scope         string            `json:"scope"`
	State         controllerStateV2 `json:"state"`
	TransactionID string            `json:"transactionId"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	LastSeenAt    *time.Time        `json:"lastSeenAt,omitempty"`
	RevokedUntil  *time.Time        `json:"revokedUntil,omitempty"`
}

type pairingCodeRecordV2 struct {
	ID                  string         `json:"id"`
	State               pairingStateV2 `json:"state"`
	CredentialFile      string         `json:"credentialFile"`
	ControllerID        string         `json:"controllerId,omitempty"`
	ControllerName      string         `json:"controllerName,omitempty"`
	ControllerPublicKey string         `json:"controllerPublicKey,omitempty"`
	TransactionID       string         `json:"transactionId,omitempty"`
	ExpiresAt           time.Time      `json:"expiresAt"`
	BoundAt             *time.Time     `json:"boundAt,omitempty"`
	Attempts            int            `json:"attempts"`
}

type persistedStateV2 struct {
	SchemaVersion int                   `json:"schemaVersion"`
	NodeID        string                `json:"nodeId,omitempty"`
	Hosts         []hostRecordV2        `json:"hosts"`
	Controllers   []controllerRecordV2  `json:"controllers"`
	PairingCodes  []pairingCodeRecordV2 `json:"pairingCodes"`
}

type atomicFileOpsV2 struct {
	rename   func(string, string) error
	remove   func(string) error
	syncFile func(*os.File) error
	syncDir  func(string) error
}

func defaultAtomicFileOpsV2() atomicFileOpsV2 {
	return atomicFileOpsV2{
		rename:   os.Rename,
		remove:   os.Remove,
		syncFile: func(file *os.File) error { return file.Sync() },
		syncDir:  syncDirectoryV2,
	}
}

type storeV2 struct {
	mu    sync.RWMutex
	path  string
	state persistedStateV2
	ops   atomicFileOpsV2
}

func openStoreV2(path string) (*storeV2, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) ||
		filepath.Base(filepath.Clean(path)) != clusterStateV2FileName {
		return nil, fmt.Errorf("cluster v2 store path must be an absolute %s path", clusterStateV2FileName)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create cluster v2 store directory: %w", err)
	}
	if err := protectDirectoryV2(directory); err != nil {
		return nil, err
	}
	ops := defaultAtomicFileOpsV2()
	if err := recoverAtomicTargetV2(path, ops); err != nil {
		return nil, fmt.Errorf("recover cluster v2 store: %w", err)
	}
	store := &storeV2{
		path: path,
		state: persistedStateV2{
			SchemaVersion: 2,
			Hosts:         []hostRecordV2{},
			Controllers:   []controllerRecordV2{},
			PairingCodes:  []pairingCodeRecordV2{},
		},
		ops: ops,
	}
	content, err := readRegularFileV2(path, maxClusterStoreV2Bytes, false)
	switch {
	case err == nil:
		loadErr := decodeStrictJSONV2(content, &store.state)
		if loadErr == nil {
			loadErr = validatePersistedStateV2(store.state)
		}
		if loadErr != nil {
			if restoreErr := restoreAtomicBackupV2(path, ops); restoreErr != nil {
				return nil, fmt.Errorf(
					"decode cluster v2 store: %v (backup recovery: %w)",
					loadErr,
					restoreErr,
				)
			}
			content, err = readRegularFileV2(path, maxClusterStoreV2Bytes, false)
			if err != nil {
				return nil, fmt.Errorf("read recovered cluster v2 store: %w", err)
			}
			store.state = persistedStateV2{}
			if err := decodeStrictJSONV2(content, &store.state); err != nil {
				return nil, fmt.Errorf("decode recovered cluster v2 store: %w", err)
			}
			if err := validatePersistedStateV2(store.state); err != nil {
				return nil, err
			}
		} else if err := discardAtomicBackupV2(path, ops); err != nil {
			return nil, fmt.Errorf("finalize cluster v2 store recovery: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read cluster v2 store: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect cluster v2 store: %w", err)
	}
	return store, nil
}

func (s *storeV2) EnsureNodeID(nodeID string) error {
	if !validID(nodeID) {
		return errors.New("cluster v2 node ID is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.NodeID == nodeID {
		return nil
	}
	if s.state.NodeID != "" {
		return ErrIdentityMismatch
	}
	previous := cloneStateV2(s.state)
	s.state.NodeID = nodeID
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *storeV2) NodeID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.NodeID
}

func (s *storeV2) Hosts() []hostRecordV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := cloneHostsV2(s.state.Hosts)
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (s *storeV2) Host(id string) (hostRecordV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Hosts {
		if record.ID == id {
			return cloneHostV2(record), nil
		}
	}
	return hostRecordV2{}, ErrNotFound
}

func (s *storeV2) AddHost(record hostRecordV2) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.Hosts) >= MaxHosts {
		return ErrHostLimit
	}
	for _, current := range s.state.Hosts {
		if strings.EqualFold(current.Origin, record.Origin) ||
			current.RemoteNodeID == record.RemoteNodeID {
			return ErrDuplicate
		}
	}
	previous := cloneStateV2(s.state)
	record.ResourceVersion = hostResourceVersionV2(record)
	s.state.Hosts = append(s.state.Hosts, cloneHostV2(record))
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *storeV2) UpdateHost(record hostRecordV2, expected string) (hostRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Hosts {
		if s.state.Hosts[index].ID != record.ID {
			continue
		}
		if s.state.Hosts[index].ResourceVersion != expected {
			return hostRecordV2{}, ErrConflict
		}
		previous := cloneStateV2(s.state)
		record.CreatedAt = s.state.Hosts[index].CreatedAt
		record.ResourceVersion = hostResourceVersionV2(record)
		s.state.Hosts[index] = cloneHostV2(record)
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return hostRecordV2{}, err
		}
		return cloneHostV2(record), nil
	}
	return hostRecordV2{}, ErrNotFound
}

func (s *storeV2) DeleteHost(id, expected string) (hostRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, record := range s.state.Hosts {
		if record.ID != id {
			continue
		}
		if record.ResourceVersion != expected {
			return hostRecordV2{}, ErrConflict
		}
		previous := cloneStateV2(s.state)
		s.state.Hosts = append(s.state.Hosts[:index], s.state.Hosts[index+1:]...)
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return hostRecordV2{}, err
		}
		return cloneHostV2(record), nil
	}
	return hostRecordV2{}, ErrNotFound
}

func (s *storeV2) Checkpoint(runtimeStateByHost map[string]runtimeState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneStateV2(s.state)
	for index := range s.state.Hosts {
		current, ok := runtimeStateByHost[s.state.Hosts[index].ID]
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
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *storeV2) Controllers() []controllerRecordV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]controllerRecordV2, 0, len(s.state.Controllers))
	for _, record := range s.state.Controllers {
		if record.State != controllerStateV2Revoked {
			result = append(result, cloneControllerV2(record))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *storeV2) Controller(id string) (controllerRecordV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Controllers {
		if record.ID == id {
			return cloneControllerV2(record), nil
		}
	}
	return controllerRecordV2{}, ErrNotFound
}

func (s *storeV2) AddController(record controllerRecordV2) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.Controllers) >= maxControllersV2 {
		return errors.New("cluster v2 controller limit reached")
	}
	for _, current := range s.state.Controllers {
		if current.ID == record.ID {
			if current.TransactionID == record.TransactionID &&
				current.PublicKey == record.PublicKey &&
				current.State == record.State {
				return nil
			}
			return ErrDuplicate
		}
	}
	previous := cloneStateV2(s.state)
	s.state.Controllers = append(s.state.Controllers, cloneControllerV2(record))
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *storeV2) CommitController(id, transactionID string, now time.Time) (controllerRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Controllers {
		record := &s.state.Controllers[index]
		if record.ID != id {
			continue
		}
		if record.TransactionID != transactionID {
			return controllerRecordV2{}, ErrConflict
		}
		if record.State == controllerStateV2Active {
			return cloneControllerV2(*record), nil
		}
		if record.State != controllerStateV2Provisional {
			return controllerRecordV2{}, ErrConflict
		}
		previous := cloneStateV2(s.state)
		record.State = controllerStateV2Active
		record.UpdatedAt = now.UTC()
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return controllerRecordV2{}, err
		}
		return cloneControllerV2(*record), nil
	}
	return controllerRecordV2{}, ErrNotFound
}

func (s *storeV2) RevokeController(
	id string,
	transactionID string,
	now time.Time,
	retainUntil time.Time,
) (controllerRecordV2, error) {
	if !retainUntil.After(now) {
		return controllerRecordV2{}, errors.New("cluster v2 revocation retention is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Controllers {
		record := &s.state.Controllers[index]
		if record.ID != id {
			continue
		}
		if record.State == controllerStateV2Revoked {
			if record.TransactionID == transactionID {
				return cloneControllerV2(*record), nil
			}
			return controllerRecordV2{}, ErrConflict
		}
		previous := cloneStateV2(s.state)
		until := retainUntil.UTC()
		record.State = controllerStateV2Revoked
		record.TransactionID = transactionID
		record.UpdatedAt = now.UTC()
		record.RevokedUntil = &until
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return controllerRecordV2{}, err
		}
		return cloneControllerV2(*record), nil
	}
	return controllerRecordV2{}, ErrNotFound
}

func (s *storeV2) DeleteController(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, record := range s.state.Controllers {
		if record.ID != id {
			continue
		}
		previous := cloneStateV2(s.state)
		s.state.Controllers = append(s.state.Controllers[:index], s.state.Controllers[index+1:]...)
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return err
		}
		return nil
	}
	return ErrNotFound
}

func (s *storeV2) TouchController(id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Controllers {
		record := &s.state.Controllers[index]
		if record.ID != id {
			continue
		}
		if record.State == controllerStateV2Revoked {
			return ErrAuthentication
		}
		if record.LastSeenAt != nil && now.UTC().Sub(*record.LastSeenAt) < 5*time.Minute {
			return nil
		}
		previous := cloneStateV2(s.state)
		value := now.UTC()
		record.LastSeenAt = &value
		record.UpdatedAt = value
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return err
		}
		return nil
	}
	return ErrNotFound
}

func (s *storeV2) GCRevokedControllers(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneStateV2(s.state)
	filtered := s.state.Controllers[:0]
	removed := 0
	for _, record := range s.state.Controllers {
		if record.State == controllerStateV2Revoked &&
			record.RevokedUntil != nil &&
			!record.RevokedUntil.After(now.UTC()) {
			removed++
			continue
		}
		filtered = append(filtered, record)
	}
	s.state.Controllers = filtered
	if removed == 0 {
		return 0, nil
	}
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return 0, err
	}
	return removed, nil
}

func (s *storeV2) PairingCodes() []pairingCodeRecordV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]pairingCodeRecordV2, len(s.state.PairingCodes))
	for index := range s.state.PairingCodes {
		result[index] = clonePairingCodeV2(s.state.PairingCodes[index])
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ExpiresAt.Before(result[j].ExpiresAt)
	})
	return result
}

func (s *storeV2) PairingCode(id string) (pairingCodeRecordV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.PairingCodes {
		if record.ID == id {
			return clonePairingCodeV2(record), nil
		}
	}
	return pairingCodeRecordV2{}, ErrNotFound
}

func (s *storeV2) AddPairingCode(record pairingCodeRecordV2, now time.Time) error {
	if !record.ExpiresAt.After(now.UTC()) {
		return ErrPairingCode
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneStateV2(s.state)
	for _, current := range s.state.PairingCodes {
		if current.ID == record.ID {
			s.state = previous
			return ErrDuplicate
		}
	}
	if len(s.state.PairingCodes) >= maxPairingCodesV2 {
		s.state = previous
		return errors.New("cluster v2 pairing code limit reached")
	}
	record.State = pairingStateV2Issued
	record.Attempts = 0
	record.BoundAt = nil
	record.ControllerID = ""
	record.ControllerName = ""
	record.ControllerPublicKey = ""
	record.TransactionID = ""
	s.state.PairingCodes = append(s.state.PairingCodes, clonePairingCodeV2(record))
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *storeV2) BindPairingCode(
	id string,
	transactionID string,
	controller controllerRecordV2,
	now time.Time,
) (pairingCodeRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.PairingCodes {
		record := &s.state.PairingCodes[index]
		if record.ID != id {
			continue
		}
		if !record.ExpiresAt.After(now.UTC()) {
			return pairingCodeRecordV2{}, ErrPairingCode
		}
		if record.State == pairingStateV2Bound {
			if record.TransactionID == transactionID &&
				record.ControllerID == controller.ID &&
				record.ControllerPublicKey == controller.PublicKey {
				return clonePairingCodeV2(*record), nil
			}
			return pairingCodeRecordV2{}, ErrConflict
		}
		if record.State != pairingStateV2Issued ||
			controller.State != controllerStateV2Provisional ||
			controller.TransactionID != transactionID {
			return pairingCodeRecordV2{}, ErrConflict
		}
		for _, current := range s.state.Controllers {
			if current.ID == controller.ID {
				return pairingCodeRecordV2{}, ErrDuplicate
			}
		}
		if len(s.state.Controllers) >= maxControllersV2 {
			return pairingCodeRecordV2{}, errors.New("cluster v2 controller limit reached")
		}
		previous := cloneStateV2(s.state)
		boundAt := now.UTC()
		record.State = pairingStateV2Bound
		record.ControllerID = controller.ID
		record.ControllerName = controller.Name
		record.ControllerPublicKey = controller.PublicKey
		record.TransactionID = transactionID
		record.ExpiresAt = now.UTC().Add(v2PendingCommitTTL)
		record.BoundAt = &boundAt
		s.state.Controllers = append(s.state.Controllers, cloneControllerV2(controller))
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return pairingCodeRecordV2{}, err
		}
		return clonePairingCodeV2(*record), nil
	}
	return pairingCodeRecordV2{}, ErrPairingCode
}

func (s *storeV2) CommitPairingCode(
	id string,
	transactionID string,
	now time.Time,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for current := range s.state.PairingCodes {
		if s.state.PairingCodes[current].ID == id {
			index = current
			break
		}
	}
	if index < 0 {
		for _, controller := range s.state.Controllers {
			if controller.TransactionID == transactionID &&
				controller.State == controllerStateV2Active {
				return "", nil
			}
		}
		return "", ErrPairingCode
	}
	record := s.state.PairingCodes[index]
	if record.State != pairingStateV2Bound ||
		record.TransactionID != transactionID ||
		!record.ExpiresAt.After(now.UTC()) {
		return "", ErrPairingCode
	}
	controllerIndex := -1
	for current := range s.state.Controllers {
		if s.state.Controllers[current].ID == record.ControllerID {
			controllerIndex = current
			break
		}
	}
	if controllerIndex < 0 ||
		s.state.Controllers[controllerIndex].State != controllerStateV2Provisional ||
		s.state.Controllers[controllerIndex].TransactionID != transactionID {
		return "", ErrConflict
	}
	previous := cloneStateV2(s.state)
	s.state.Controllers[controllerIndex].State = controllerStateV2Active
	s.state.Controllers[controllerIndex].UpdatedAt = now.UTC()
	s.state.PairingCodes = append(s.state.PairingCodes[:index], s.state.PairingCodes[index+1:]...)
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return "", err
	}
	return record.CredentialFile, nil
}

func (s *storeV2) RecordPairingFailure(id string, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.PairingCodes {
		record := &s.state.PairingCodes[index]
		if record.ID != id {
			continue
		}
		if record.State == pairingStateV2Bound {
			return "", ErrPairingCode
		}
		previous := cloneStateV2(s.state)
		record.Attempts++
		removedCredential := ""
		if record.Attempts >= 5 || !record.ExpiresAt.After(now.UTC()) {
			removedCredential = record.CredentialFile
			s.removeProvisionalControllerLocked(record.ControllerID, record.TransactionID)
			s.state.PairingCodes = append(s.state.PairingCodes[:index], s.state.PairingCodes[index+1:]...)
		}
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return "", err
		}
		return removedCredential, ErrPairingCode
	}
	return "", ErrPairingCode
}

func (s *storeV2) GCExpiredPairingCodes(now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneStateV2(s.state)
	var credentials []string
	filtered := s.state.PairingCodes[:0]
	for _, record := range s.state.PairingCodes {
		if !record.ExpiresAt.After(now.UTC()) {
			credentials = append(credentials, record.CredentialFile)
			s.removeProvisionalControllerLocked(record.ControllerID, record.TransactionID)
			continue
		}
		filtered = append(filtered, record)
	}
	s.state.PairingCodes = filtered
	if len(credentials) == 0 {
		return nil, nil
	}
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return nil, err
	}
	return credentials, nil
}

func (s *storeV2) CredentialReferences() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]struct{})
	for _, record := range s.state.Hosts {
		if record.CredentialFile != "" {
			result[record.CredentialFile] = struct{}{}
		}
		if record.PairingCredentialFile != "" {
			result[record.PairingCredentialFile] = struct{}{}
		}
	}
	for _, record := range s.state.PairingCodes {
		result[record.CredentialFile] = struct{}{}
	}
	return result
}

func (s *storeV2) removeProvisionalControllerLocked(id, transactionID string) {
	if id == "" || transactionID == "" {
		return
	}
	filtered := s.state.Controllers[:0]
	for _, controller := range s.state.Controllers {
		if controller.ID == id &&
			controller.TransactionID == transactionID &&
			controller.State == controllerStateV2Provisional {
			continue
		}
		filtered = append(filtered, controller)
	}
	s.state.Controllers = filtered
}

func (s *storeV2) persistValidatedLocked() error {
	if err := validatePersistedStateV2(s.state); err != nil {
		return err
	}
	return s.persistLocked()
}

func (s *storeV2) persistLocked() error {
	content, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cluster v2 store: %w", err)
	}
	content = append(content, '\n')
	if int64(len(content)) > maxClusterStoreV2Bytes {
		return errors.New("cluster v2 store exceeds 4 MiB")
	}
	if err := atomicWriteFileV2(s.path, content, 0o600, true, s.ops); err != nil {
		return fmt.Errorf("persist cluster v2 store: %w", err)
	}
	return nil
}

func validatePersistedStateV2(state persistedStateV2) error {
	if state.SchemaVersion != 2 {
		return fmt.Errorf("unsupported cluster v2 store schema %d", state.SchemaVersion)
	}
	if state.NodeID != "" && !validID(state.NodeID) {
		return errors.New("cluster v2 store contains an invalid node ID")
	}
	if len(state.Hosts) > MaxHosts || len(state.Controllers) > maxControllersV2 ||
		len(state.PairingCodes) > maxPairingCodesV2 {
		return errors.New("cluster v2 store exceeds a record limit")
	}
	hostIDs := make(map[string]struct{}, len(state.Hosts))
	origins := make(map[string]struct{}, len(state.Hosts))
	remoteNodes := make(map[string]struct{}, len(state.Hosts))
	for _, record := range state.Hosts {
		if err := validateHostRecordV2(record); err != nil {
			return err
		}
		if _, ok := hostIDs[record.ID]; ok {
			return errors.New("cluster v2 store contains a duplicate host ID")
		}
		if _, ok := origins[strings.ToLower(record.Origin)]; ok {
			return errors.New("cluster v2 store contains a duplicate host origin")
		}
		if _, ok := remoteNodes[record.RemoteNodeID]; ok {
			return errors.New("cluster v2 store contains a duplicate remote node")
		}
		hostIDs[record.ID] = struct{}{}
		origins[strings.ToLower(record.Origin)] = struct{}{}
		remoteNodes[record.RemoteNodeID] = struct{}{}
	}
	controllerIDs := make(map[string]struct{}, len(state.Controllers))
	for _, record := range state.Controllers {
		if err := validateControllerRecordV2(record); err != nil {
			return err
		}
		if _, ok := controllerIDs[record.ID]; ok {
			return errors.New("cluster v2 store contains a duplicate controller ID")
		}
		controllerIDs[record.ID] = struct{}{}
	}
	pairingIDs := make(map[string]struct{}, len(state.PairingCodes))
	for _, record := range state.PairingCodes {
		if err := validatePairingCodeRecordV2(record); err != nil {
			return err
		}
		if _, ok := pairingIDs[record.ID]; ok {
			return errors.New("cluster v2 store contains a duplicate pairing code ID")
		}
		pairingIDs[record.ID] = struct{}{}
	}
	return nil
}

func validateHostRecordV2(record hostRecordV2) error {
	normalizedOrigin, err := NormalizeV2Origin(record.Origin)
	targetPublicKey, keyErr := base64.RawURLEncoding.DecodeString(record.TargetPublicKey)
	if !validID(record.ID) || !validID(record.RemoteNodeID) ||
		!validID(record.ControllerID) || !validID(record.TransactionID) ||
		err != nil || normalizedOrigin != record.Origin ||
		record.TransportSecurity != v2TransportSecurity(record.Origin) ||
		record.FederationProtocol != FederationProtocolV2 ||
		(record.Scope != "" && record.Scope != SummaryScope && record.Scope != SummaryTerminalScope && record.Scope != SummaryTerminalFilesScope) ||
		keyErr != nil || len(targetPublicKey) != 32 ||
		record.PeerFingerprint != fingerprintV2(targetPublicKey) ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.ConsecutiveFailures < 0 ||
		len(record.Name) > 80 ||
		record.ResourceVersion != hostResourceVersionV2(record) {
		return errors.New("cluster v2 store contains an invalid host record")
	}
	switch record.State {
	case hostStateV2PendingPair:
		if !validPairingCredentialNameV2(record.PairingCredentialFile) ||
			record.CredentialFile != "" {
			return errors.New("cluster v2 pending pair host credential state is invalid")
		}
	case hostStateV2PendingCommit:
		if !validCredentialNameV2(record.CredentialFile) ||
			record.CredentialFile != "host-"+record.ID+".v2key" ||
			!validPairingCredentialNameV2(record.PairingCredentialFile) {
			return errors.New("cluster v2 pending commit host credential state is invalid")
		}
	case hostStateV2Active, hostStateV2PendingRevoke:
		if !validCredentialNameV2(record.CredentialFile) ||
			record.CredentialFile != "host-"+record.ID+".v2key" ||
			record.PairingCredentialFile != "" {
			return errors.New("cluster v2 active host credential state is invalid")
		}
	default:
		return errors.New("cluster v2 host state is invalid")
	}
	return nil
}

func validateControllerRecordV2(record controllerRecordV2) error {
	publicKey, err := base64.RawURLEncoding.DecodeString(record.PublicKey)
	if !validID(record.ID) || !validID(record.TransactionID) ||
		err != nil || len(publicKey) != 32 ||
		record.Fingerprint != fingerprintV2(publicKey) ||
		(record.Scope != SummaryScope && record.Scope != SummaryTerminalScope && record.Scope != SummaryTerminalFilesScope) ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		len(record.Name) > 80 {
		return errors.New("cluster v2 store contains an invalid controller record")
	}
	switch record.State {
	case controllerStateV2Provisional, controllerStateV2Active:
		if record.RevokedUntil != nil {
			return errors.New("cluster v2 controller revocation state is invalid")
		}
	case controllerStateV2Revoked:
		if record.RevokedUntil == nil || record.RevokedUntil.IsZero() {
			return errors.New("cluster v2 controller revocation state is invalid")
		}
	default:
		return errors.New("cluster v2 controller state is invalid")
	}
	return nil
}

func validatePairingCodeRecordV2(record pairingCodeRecordV2) error {
	if len(record.ID) != 16 || !validHex(record.ID) ||
		!validPairingCredentialNameV2(record.CredentialFile) ||
		record.CredentialFile != "pair-"+record.ID+".v2key" ||
		record.ExpiresAt.IsZero() || record.Attempts < 0 || record.Attempts >= 5 {
		return errors.New("cluster v2 store contains an invalid pairing code record")
	}
	switch record.State {
	case pairingStateV2Issued:
		if record.ControllerID != "" || record.ControllerName != "" ||
			record.ControllerPublicKey != "" || record.TransactionID != "" ||
			record.BoundAt != nil {
			return errors.New("cluster v2 issued pairing code state is invalid")
		}
	case pairingStateV2Bound:
		publicKey, err := base64.RawURLEncoding.DecodeString(record.ControllerPublicKey)
		if !validID(record.ControllerID) || !validID(record.TransactionID) ||
			err != nil || len(publicKey) != 32 || record.BoundAt == nil ||
			record.BoundAt.IsZero() || len(record.ControllerName) > 80 {
			return errors.New("cluster v2 bound pairing code state is invalid")
		}
	default:
		return errors.New("cluster v2 pairing code state is invalid")
	}
	return nil
}

func hostResourceVersionV2(record hostRecordV2) string {
	fields := []string{
		record.ID,
		record.Name,
		record.Origin,
		record.RemoteNodeID,
		record.ControllerID,
		string(record.State),
		record.TransactionID,
		record.CredentialFile,
		record.PairingCredentialFile,
		record.TargetPublicKey,
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	// Preserve resource versions for existing summary-only and terminal v2
	// pairings. A new files grant is versioned separately and never upgrades an
	// old credential implicitly.
	if record.Scope == SummaryTerminalScope {
		fields = append(fields, "scope:"+SummaryTerminalScope)
	} else if record.Scope == SummaryTerminalFilesScope {
		fields = append(fields, "scope:"+SummaryTerminalFilesScope)
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fingerprintV2(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneStateV2(source persistedStateV2) persistedStateV2 {
	result := persistedStateV2{
		SchemaVersion: source.SchemaVersion,
		NodeID:        source.NodeID,
		Hosts:         cloneHostsV2(source.Hosts),
		Controllers:   make([]controllerRecordV2, len(source.Controllers)),
		PairingCodes:  make([]pairingCodeRecordV2, len(source.PairingCodes)),
	}
	for index := range source.Controllers {
		result.Controllers[index] = cloneControllerV2(source.Controllers[index])
	}
	for index := range source.PairingCodes {
		result.PairingCodes[index] = clonePairingCodeV2(source.PairingCodes[index])
	}
	return result
}

func cloneHostsV2(source []hostRecordV2) []hostRecordV2 {
	result := make([]hostRecordV2, len(source))
	for index := range source {
		result[index] = cloneHostV2(source[index])
	}
	return result
}

func cloneHostV2(source hostRecordV2) hostRecordV2 {
	source.LastSnapshot = cloneSnapshot(source.LastSnapshot)
	source.LastAttemptAt = cloneTime(source.LastAttemptAt)
	source.LastSuccessAt = cloneTime(source.LastSuccessAt)
	return source
}

func cloneControllerV2(source controllerRecordV2) controllerRecordV2 {
	source.LastSeenAt = cloneTime(source.LastSeenAt)
	source.RevokedUntil = cloneTime(source.RevokedUntil)
	return source
}

func clonePairingCodeV2(source pairingCodeRecordV2) pairingCodeRecordV2 {
	source.BoundAt = cloneTime(source.BoundAt)
	return source
}

func atomicWriteFileV2(
	target string,
	content []byte,
	permission os.FileMode,
	replace bool,
	ops atomicFileOpsV2,
) error {
	if ops.rename == nil || ops.remove == nil ||
		ops.syncFile == nil || ops.syncDir == nil {
		return errors.New("cluster v2 atomic file operations are incomplete")
	}
	directory := filepath.Dir(target)
	if !replace {
		if _, err := os.Lstat(target); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	temp, err := os.CreateTemp(directory, ".cluster-v2-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer ops.remove(tempName)
	closeWithError := func(current error) error {
		if closeErr := temp.Close(); current == nil {
			return closeErr
		}
		return current
	}
	if err := temp.Chmod(permission); err != nil {
		return closeWithError(err)
	}
	if _, err := temp.Write(content); err != nil {
		return closeWithError(err)
	}
	if err := ops.syncFile(temp); err != nil {
		return closeWithError(err)
	}
	if err := temp.Close(); err != nil {
		return err
	}

	backup := target + ".previous"
	hadTarget := false
	if replace {
		info, statErr := os.Lstat(target)
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("cluster v2 atomic target must be a regular file")
			}
			hadTarget = true
		case !errors.Is(statErr, os.ErrNotExist):
			return statErr
		}
	}
	if hadTarget {
		if _, err := os.Lstat(backup); err == nil {
			return errors.New("cluster v2 atomic backup already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := ops.rename(target, backup); err != nil {
			return err
		}
	}
	if err := ops.rename(tempName, target); err != nil {
		if hadTarget {
			_ = ops.rename(backup, target)
		}
		return err
	}
	if err := ops.syncDir(directory); err != nil {
		_ = ops.remove(target)
		if hadTarget {
			_ = ops.rename(backup, target)
		}
		_ = syncDirectoryV2(directory)
		return err
	}
	if hadTarget {
		_ = ops.remove(backup)
		_ = ops.syncDir(directory)
	}
	return nil
}

func recoverAtomicTargetV2(target string, ops atomicFileOpsV2) error {
	backup := target + ".previous"
	targetInfo, targetErr := os.Lstat(target)
	backupInfo, backupErr := os.Lstat(backup)
	if targetErr == nil &&
		(!targetInfo.Mode().IsRegular() || targetInfo.Mode()&os.ModeSymlink != 0) {
		return errors.New("cluster v2 target must be a regular file")
	}
	if backupErr == nil &&
		(!backupInfo.Mode().IsRegular() || backupInfo.Mode()&os.ModeSymlink != 0) {
		return errors.New("cluster v2 backup must be a regular file")
	}
	switch {
	case targetErr == nil && backupErr == nil:
		// A crash may have happened after installing the new target but before
		// syncing the directory. Keep the backup until the caller has strictly
		// decoded and validated the target.
		return nil
	case errors.Is(targetErr, os.ErrNotExist) && backupErr == nil:
		if err := ops.rename(backup, target); err != nil {
			return err
		}
		return ops.syncDir(filepath.Dir(target))
	case targetErr != nil && !errors.Is(targetErr, os.ErrNotExist):
		return targetErr
	case backupErr != nil && !errors.Is(backupErr, os.ErrNotExist):
		return backupErr
	default:
		return nil
	}
}

func restoreAtomicBackupV2(target string, ops atomicFileOpsV2) error {
	backup := target + ".previous"
	info, err := os.Lstat(backup)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cluster v2 backup must be a regular file")
	}
	if err := ops.remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ops.rename(backup, target); err != nil {
		return err
	}
	return ops.syncDir(filepath.Dir(target))
}

func discardAtomicBackupV2(target string, ops atomicFileOpsV2) error {
	backup := target + ".previous"
	info, err := os.Lstat(backup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cluster v2 backup must be a regular file")
	}
	if err := ops.remove(backup); err != nil {
		return err
	}
	return ops.syncDir(filepath.Dir(target))
}

func protectDirectoryV2(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cluster v2 directory must be a real directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect cluster v2 directory: %w", err)
	}
	return nil
}

func readRegularFileV2(path string, maximum int64, requirePrivate bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("cluster v2 file must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maximum {
		return nil, errors.New("cluster v2 file size is invalid")
	}
	if requirePrivate && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return nil, errors.New("cluster v2 file permissions are unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return nil, errors.New("cluster v2 file changed while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, errors.New("cluster v2 file is too large")
	}
	return content, nil
}

func decodeStrictJSONV2(content []byte, destination any) error {
	if err := validateJSONShapeV2(content); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func validateJSONShapeV2(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := scanJSONValueV2(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func scanJSONValueV2(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is invalid")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValueV2(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			if err != nil {
				return err
			}
			return errors.New("JSON object is not terminated")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValueV2(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			if err != nil {
				return err
			}
			return errors.New("JSON array is not terminated")
		}
	default:
		return errors.New("JSON delimiter is invalid")
	}
	return nil
}

func syncDirectoryV2(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	err = handle.Sync()
	if err == nil {
		return nil
	}
	if runtime.GOOS == "windows" &&
		(errors.Is(err, syscall.Errno(1)) ||
			errors.Is(err, syscall.Errno(5)) ||
			errors.Is(err, syscall.Errno(6))) {
		return nil
	}
	return err
}
