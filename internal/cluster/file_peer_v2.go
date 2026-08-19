package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	filePeerStateV2FileName = "cluster-file-peers-v2.json"
	maxFilePeerStoreV2Bytes = int64(256 << 10)
	filePeerSyncInterval    = 10 * time.Minute
	filePeerLeaseDuration   = 30 * time.Minute
	filePeerReadScope       = "cluster.files.read"
)

type filePeerGrantStateV2 string

const (
	filePeerGrantPending filePeerGrantStateV2 = "pending"
	filePeerGrantActive  filePeerGrantStateV2 = "active"
)

// filePeerGrantV2 authorizes the node referenced by an existing Host to read
// this panel's files. It deliberately contains no key material: every use must
// still resolve and authenticate the parent Host and its credential.
type filePeerGrantV2 struct {
	LinkID              string               `json:"linkId"`
	HostID              string               `json:"hostId"`
	HostTransaction     string               `json:"hostTransaction"`
	HostControllerID    string               `json:"hostControllerId"`
	PeerNodeID          string               `json:"peerNodeId"`
	PeerFingerprint     string               `json:"peerFingerprint"`
	LocalOrigin         string               `json:"localOrigin"`
	Scope               string               `json:"scope"`
	State               filePeerGrantStateV2 `json:"state"`
	CreatedAt           time.Time            `json:"createdAt"`
	UpdatedAt           time.Time            `json:"updatedAt"`
	LastSynchronizedAt  time.Time            `json:"lastSynchronizedAt"`
	NextSynchronization time.Time            `json:"nextSynchronizationAt"`
	ExpiresAt           time.Time            `json:"expiresAt"`
}

// filePeerRouteV2 records how this panel can reach an existing Controller as a
// file source. Controller public keys remain in cluster-state-v2.json and are
// rechecked on every use.
type filePeerRouteV2 struct {
	LinkID                string    `json:"linkId"`
	ControllerID          string    `json:"controllerId"`
	ControllerTransaction string    `json:"controllerTransaction"`
	ControllerFingerprint string    `json:"controllerFingerprint"`
	PeerNodeID            string    `json:"peerNodeId"`
	PeerOrigin            string    `json:"peerOrigin"`
	Scope                 string    `json:"scope"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	ExpiresAt             time.Time `json:"expiresAt"`
}

type persistedFilePeersV2 struct {
	SchemaVersion int               `json:"schemaVersion"`
	Routes        []filePeerRouteV2 `json:"routes"`
	Grants        []filePeerGrantV2 `json:"grants"`
}

type filePeerStoreV2 struct {
	mu    sync.RWMutex
	path  string
	state persistedFilePeersV2
	ops   atomicFileOpsV2
}

func openFilePeerStoreV2(path string) (*filePeerStoreV2, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) ||
		filepath.Base(filepath.Clean(path)) != filePeerStateV2FileName {
		return nil, fmt.Errorf(
			"cluster file peer store path must be an absolute %s path",
			filePeerStateV2FileName,
		)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create cluster file peer store directory: %w", err)
	}
	if err := protectDirectoryV2(directory); err != nil {
		return nil, err
	}
	ops := defaultAtomicFileOpsV2()
	if err := recoverAtomicTargetV2(path, ops); err != nil {
		return nil, fmt.Errorf("recover cluster file peer store: %w", err)
	}
	store := &filePeerStoreV2{
		path: path,
		state: persistedFilePeersV2{
			SchemaVersion: 1,
			Routes:        []filePeerRouteV2{},
			Grants:        []filePeerGrantV2{},
		},
		ops: ops,
	}
	content, err := readRegularFileV2(path, maxFilePeerStoreV2Bytes, false)
	switch {
	case err == nil:
		loadErr := decodeStrictJSONV2(content, &store.state)
		if loadErr == nil {
			loadErr = validateFilePeerStateV2(store.state)
		}
		if loadErr != nil {
			if restoreErr := restoreAtomicBackupV2(path, ops); restoreErr != nil {
				return nil, fmt.Errorf(
					"decode cluster file peer store: %v (backup recovery: %w)",
					loadErr,
					restoreErr,
				)
			}
			content, err = readRegularFileV2(path, maxFilePeerStoreV2Bytes, false)
			if err != nil {
				return nil, fmt.Errorf("read recovered cluster file peer store: %w", err)
			}
			store.state = persistedFilePeersV2{}
			if err := decodeStrictJSONV2(content, &store.state); err != nil {
				return nil, fmt.Errorf("decode recovered cluster file peer store: %w", err)
			}
			if err := validateFilePeerStateV2(store.state); err != nil {
				return nil, err
			}
		} else if err := discardAtomicBackupV2(path, ops); err != nil {
			return nil, fmt.Errorf("finalize cluster file peer store recovery: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read cluster file peer store: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect cluster file peer store: %w", err)
	}
	return store, nil
}

func (s *filePeerStoreV2) PrepareGrant(
	host hostRecordV2,
	localOrigin string,
	now time.Time,
) (filePeerGrantV2, error) {
	if err := validateFilePeerIntentHostV2(host); err != nil {
		return filePeerGrantV2{}, err
	}
	hostCanAuthorize := validateFilePeerHostV2(host) == nil
	origin, err := NormalizeV2Origin(localOrigin)
	if err != nil || origin != localOrigin {
		return filePeerGrantV2{}, ErrInvalidOrigin
	}
	if now.IsZero() {
		return filePeerGrantV2{}, errors.New("cluster file peer grant time is invalid")
	}
	now = now.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for index, current := range s.state.Grants {
		if current.HostID != host.ID {
			if current.PeerNodeID == host.RemoteNodeID {
				return filePeerGrantV2{}, ErrDuplicate
			}
			continue
		}
		if !grantMatchesHostV2(current, host) {
			return filePeerGrantV2{}, ErrIdentityMismatch
		}
		if !hostCanAuthorize {
			if current.State == filePeerGrantPending && current.LocalOrigin == origin {
				return current, nil
			}
			if now.Before(current.UpdatedAt) {
				return filePeerGrantV2{}, ErrConflict
			}
			previous := cloneFilePeerStateV2(s.state)
			current.LocalOrigin = origin
			current = pendingFilePeerGrantV2(current, now)
			s.state.Grants[index] = current
			if err := s.persistValidatedLocked(); err != nil {
				s.state = previous
				return filePeerGrantV2{}, err
			}
			return current, nil
		}
		if current.LocalOrigin == origin &&
			(current.State != filePeerGrantActive || current.ExpiresAt.After(now)) {
			return current, nil
		}
		if now.Before(current.UpdatedAt) {
			return filePeerGrantV2{}, ErrConflict
		}
		previous := cloneFilePeerStateV2(s.state)
		current.LocalOrigin = origin
		if current.State == filePeerGrantActive && current.ExpiresAt.After(now) {
			// An origin refresh is not an authorization refresh. Preserve the
			// current lease until the peer acknowledges the new link payload so a
			// dropped acknowledgement cannot interrupt an otherwise valid grant.
			s.state.Grants[index] = current
			if err := s.persistValidatedLocked(); err != nil {
				s.state = previous
				return filePeerGrantV2{}, err
			}
			return current, nil
		}
		current = pendingFilePeerGrantV2(current, now)
		s.state.Grants[index] = current
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return filePeerGrantV2{}, err
		}
		return current, nil
	}
	if len(s.state.Grants) >= MaxHosts {
		return filePeerGrantV2{}, ErrHostLimit
	}
	linkID, err := randomHex(16)
	if err != nil {
		return filePeerGrantV2{}, err
	}
	for _, current := range s.state.Grants {
		if current.LinkID == linkID {
			return filePeerGrantV2{}, ErrDuplicate
		}
	}
	record := filePeerGrantV2{
		LinkID:              linkID,
		HostID:              host.ID,
		HostTransaction:     host.TransactionID,
		HostControllerID:    host.ControllerID,
		PeerNodeID:          host.RemoteNodeID,
		PeerFingerprint:     host.PeerFingerprint,
		LocalOrigin:         origin,
		Scope:               filePeerReadScope,
		State:               filePeerGrantPending,
		CreatedAt:           now,
		UpdatedAt:           now,
		NextSynchronization: now,
	}
	if err := validateFilePeerGrantV2(record); err != nil {
		return filePeerGrantV2{}, err
	}
	previous := cloneFilePeerStateV2(s.state)
	s.state.Grants = append(s.state.Grants, record)
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return filePeerGrantV2{}, err
	}
	return record, nil
}

func (s *filePeerStoreV2) ActivateGrant(
	linkID string,
	now time.Time,
) (filePeerGrantV2, error) {
	if !validID(linkID) || now.IsZero() {
		return filePeerGrantV2{}, ErrNotFound
	}
	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, current := range s.state.Grants {
		if current.LinkID != linkID {
			continue
		}
		if now.Before(current.UpdatedAt) {
			return filePeerGrantV2{}, ErrConflict
		}
		previous := cloneFilePeerStateV2(s.state)
		current.State = filePeerGrantActive
		current.UpdatedAt = now
		current.LastSynchronizedAt = now
		current.NextSynchronization = now.Add(filePeerSyncInterval)
		current.ExpiresAt = now.Add(filePeerLeaseDuration)
		s.state.Grants[index] = current
		if err := s.persistValidatedLocked(); err != nil {
			s.state = previous
			return filePeerGrantV2{}, err
		}
		return current, nil
	}
	return filePeerGrantV2{}, ErrNotFound
}

func (s *filePeerStoreV2) GrantByHost(hostID string) (filePeerGrantV2, error) {
	if !validID(hostID) {
		return filePeerGrantV2{}, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Grants {
		if record.HostID == hostID {
			return record, nil
		}
	}
	return filePeerGrantV2{}, ErrNotFound
}

func (s *filePeerStoreV2) ActiveGrant(
	linkID string,
	now time.Time,
) (filePeerGrantV2, error) {
	if !validID(linkID) || now.IsZero() {
		return filePeerGrantV2{}, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Grants {
		if record.LinkID == linkID && activeFilePeerGrantV2(record, now.UTC()) {
			return record, nil
		}
	}
	return filePeerGrantV2{}, ErrNotFound
}

func (s *filePeerStoreV2) ActiveGrantByHost(
	hostID string,
	now time.Time,
) (filePeerGrantV2, error) {
	if !validID(hostID) || now.IsZero() {
		return filePeerGrantV2{}, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Grants {
		if record.HostID == hostID && activeFilePeerGrantV2(record, now.UTC()) {
			return record, nil
		}
	}
	return filePeerGrantV2{}, ErrNotFound
}

func (s *filePeerStoreV2) GrantRoute(
	controller controllerRecordV2,
	linkID string,
	peerNodeID string,
	peerOrigin string,
	now time.Time,
) (filePeerRouteV2, error) {
	if err := validateFilePeerControllerV2(controller); err != nil {
		return filePeerRouteV2{}, err
	}
	origin, err := NormalizeV2Origin(peerOrigin)
	if err != nil || origin != peerOrigin {
		return filePeerRouteV2{}, ErrInvalidOrigin
	}
	if !validID(linkID) || !validID(peerNodeID) || now.IsZero() {
		return filePeerRouteV2{}, ErrAuthentication
	}
	now = now.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneFilePeerStateV2(s.state)
	filtered := make([]filePeerRouteV2, 0, len(s.state.Routes))
	for _, current := range s.state.Routes {
		if current.ControllerID != controller.ID && !current.ExpiresAt.After(now) {
			continue
		}
		filtered = append(filtered, current)
	}
	s.state.Routes = filtered

	for index, current := range s.state.Routes {
		if current.ControllerID == controller.ID {
			if !routeMatchesControllerV2(current, controller) ||
				current.PeerNodeID != peerNodeID {
				s.state = previous
				return filePeerRouteV2{}, ErrIdentityMismatch
			}
			if now.Before(current.UpdatedAt) {
				s.state = previous
				return filePeerRouteV2{}, ErrConflict
			}
			for otherIndex, other := range s.state.Routes {
				if otherIndex == index {
					continue
				}
				if other.LinkID == linkID || other.PeerNodeID == peerNodeID {
					s.state = previous
					return filePeerRouteV2{}, ErrDuplicate
				}
			}
			current.LinkID = linkID
			current.PeerOrigin = origin
			current.UpdatedAt = now
			current.ExpiresAt = now.Add(filePeerLeaseDuration)
			s.state.Routes[index] = current
			if err := s.persistValidatedLocked(); err != nil {
				s.state = previous
				return filePeerRouteV2{}, err
			}
			return current, nil
		}
		if current.LinkID == linkID || current.PeerNodeID == peerNodeID {
			s.state = previous
			return filePeerRouteV2{}, ErrDuplicate
		}
	}
	if len(s.state.Routes) >= maxControllersV2 {
		s.state = previous
		return filePeerRouteV2{}, ErrHostLimit
	}
	record := filePeerRouteV2{
		LinkID:                linkID,
		ControllerID:          controller.ID,
		ControllerTransaction: controller.TransactionID,
		ControllerFingerprint: controller.Fingerprint,
		PeerNodeID:            peerNodeID,
		PeerOrigin:            origin,
		Scope:                 filePeerReadScope,
		CreatedAt:             now,
		UpdatedAt:             now,
		ExpiresAt:             now.Add(filePeerLeaseDuration),
	}
	if err := validateFilePeerRouteV2(record); err != nil {
		s.state = previous
		return filePeerRouteV2{}, err
	}
	s.state.Routes = append(s.state.Routes, record)
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return filePeerRouteV2{}, err
	}
	return record, nil
}

func (s *filePeerStoreV2) ActiveRoute(
	peerNodeID string,
	now time.Time,
) (filePeerRouteV2, error) {
	if !validID(peerNodeID) || now.IsZero() {
		return filePeerRouteV2{}, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Routes {
		if record.PeerNodeID == peerNodeID && record.ExpiresAt.After(now.UTC()) {
			return record, nil
		}
	}
	return filePeerRouteV2{}, ErrNotFound
}

func (s *filePeerStoreV2) DeleteController(controllerID string) error {
	if !validID(controllerID) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneFilePeerStateV2(s.state)
	filtered := s.state.Routes[:0]
	removed := false
	for _, record := range s.state.Routes {
		if record.ControllerID == controllerID {
			removed = true
			continue
		}
		filtered = append(filtered, record)
	}
	if !removed {
		return ErrNotFound
	}
	s.state.Routes = filtered
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *filePeerStoreV2) DeleteHost(hostID string) error {
	if !validID(hostID) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneFilePeerStateV2(s.state)
	filtered := s.state.Grants[:0]
	removed := false
	for _, record := range s.state.Grants {
		if record.HostID == hostID {
			removed = true
			continue
		}
		filtered = append(filtered, record)
	}
	if !removed {
		return ErrNotFound
	}
	s.state.Grants = filtered
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *filePeerStoreV2) Reconcile(
	controllers []controllerRecordV2,
	hosts []hostRecordV2,
	now time.Time,
) error {
	if now.IsZero() {
		return errors.New("cluster file peer reconciliation time is invalid")
	}
	now = now.UTC()
	activeControllers := make(map[string]controllerRecordV2, len(controllers))
	for _, controller := range controllers {
		if validateFilePeerControllerV2(controller) == nil {
			activeControllers[controller.ID] = controller
		}
	}
	validHosts := make(map[string]hostRecordV2, len(hosts))
	for _, host := range hosts {
		if validateHostRecordV2(host) == nil {
			validHosts[host.ID] = host
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneFilePeerStateV2(s.state)
	routes := make([]filePeerRouteV2, 0, len(s.state.Routes))
	for _, route := range s.state.Routes {
		controller, ok := activeControllers[route.ControllerID]
		if !ok || !routeMatchesControllerV2(route, controller) ||
			!route.ExpiresAt.After(now) {
			continue
		}
		routes = append(routes, route)
	}
	grants := make([]filePeerGrantV2, 0, len(s.state.Grants))
	for _, grant := range s.state.Grants {
		host, ok := validHosts[grant.HostID]
		if !ok || !grantMatchesHostV2(grant, host) {
			continue
		}
		switch host.State {
		case hostStateV2PendingPair, hostStateV2PendingCommit:
			if grant.State == filePeerGrantActive {
				transitionAt := now
				if transitionAt.Before(grant.UpdatedAt) {
					transitionAt = grant.UpdatedAt
				}
				grant = pendingFilePeerGrantV2(grant, transitionAt)
			}
		case hostStateV2Active:
			if !ScopeAllowsFiles(normalizedV2Scope(host.Scope)) {
				continue
			}
			if grant.State == filePeerGrantActive && !grant.ExpiresAt.After(now) {
				grant = pendingFilePeerGrantV2(grant, now)
			}
		default:
			continue
		}
		grants = append(grants, grant)
	}
	s.state.Routes = routes
	s.state.Grants = grants
	if filePeerStatesEqualV2(previous, s.state) {
		return nil
	}
	if err := s.persistValidatedLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *filePeerStoreV2) persistValidatedLocked() error {
	if err := validateFilePeerStateV2(s.state); err != nil {
		return err
	}
	return s.persistLocked()
}

func (s *filePeerStoreV2) persistLocked() error {
	state := orderedFilePeerStateV2(s.state)
	content, err := marshalFilePeerStateV2(state)
	if err != nil {
		return err
	}
	if err := atomicWriteFileV2(s.path, content, 0o600, true, s.ops); err != nil {
		return fmt.Errorf("persist cluster file peer store: %w", err)
	}
	return nil
}

func marshalFilePeerStateV2(state persistedFilePeersV2) ([]byte, error) {
	content, err := jsonMarshalIndentV2(state)
	if err != nil {
		return nil, fmt.Errorf("encode cluster file peer store: %w", err)
	}
	if int64(len(content)) > maxFilePeerStoreV2Bytes {
		return nil, errors.New("cluster file peer store exceeds its size limit")
	}
	return content, nil
}

// Kept as a small seam so tests exercise the exact production representation
// without exposing the sidecar schema outside the cluster package.
func jsonMarshalIndentV2(value any) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func validateFilePeerStateV2(state persistedFilePeersV2) error {
	if state.SchemaVersion != 1 || state.Routes == nil || state.Grants == nil ||
		len(state.Routes) > maxControllersV2 || len(state.Grants) > MaxHosts {
		return errors.New("cluster file peer store has an invalid schema or record count")
	}
	routeControllers := make(map[string]struct{}, len(state.Routes))
	routeLinks := make(map[string]struct{}, len(state.Routes))
	routePeers := make(map[string]struct{}, len(state.Routes))
	for _, route := range state.Routes {
		if err := validateFilePeerRouteV2(route); err != nil {
			return err
		}
		if _, exists := routeControllers[route.ControllerID]; exists {
			return errors.New("cluster file peer store contains a duplicate route controller")
		}
		if _, exists := routeLinks[route.LinkID]; exists {
			return errors.New("cluster file peer store contains a duplicate route link")
		}
		if _, exists := routePeers[route.PeerNodeID]; exists {
			return errors.New("cluster file peer store contains a duplicate route peer")
		}
		routeControllers[route.ControllerID] = struct{}{}
		routeLinks[route.LinkID] = struct{}{}
		routePeers[route.PeerNodeID] = struct{}{}
	}
	grantHosts := make(map[string]struct{}, len(state.Grants))
	grantLinks := make(map[string]struct{}, len(state.Grants))
	grantPeers := make(map[string]struct{}, len(state.Grants))
	for _, grant := range state.Grants {
		if err := validateFilePeerGrantV2(grant); err != nil {
			return err
		}
		if _, exists := grantHosts[grant.HostID]; exists {
			return errors.New("cluster file peer store contains a duplicate grant host")
		}
		if _, exists := grantLinks[grant.LinkID]; exists {
			return errors.New("cluster file peer store contains a duplicate grant link")
		}
		if _, exists := grantPeers[grant.PeerNodeID]; exists {
			return errors.New("cluster file peer store contains a duplicate grant peer")
		}
		grantHosts[grant.HostID] = struct{}{}
		grantLinks[grant.LinkID] = struct{}{}
		grantPeers[grant.PeerNodeID] = struct{}{}
	}
	return nil
}

func validateFilePeerGrantV2(record filePeerGrantV2) error {
	origin, originErr := NormalizeV2Origin(record.LocalOrigin)
	if !validID(record.LinkID) || !validID(record.HostID) ||
		!validID(record.HostTransaction) || !validID(record.HostControllerID) ||
		!validID(record.PeerNodeID) || !validFingerprintV2(record.PeerFingerprint) ||
		originErr != nil || origin != record.LocalOrigin || record.Scope != filePeerReadScope ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) || record.NextSynchronization.IsZero() {
		return errors.New("cluster file peer store contains an invalid grant")
	}
	switch record.State {
	case filePeerGrantPending:
		if !record.LastSynchronizedAt.IsZero() || !record.ExpiresAt.IsZero() {
			return errors.New("cluster file peer pending grant lease is invalid")
		}
	case filePeerGrantActive:
		if record.LastSynchronizedAt.IsZero() ||
			!record.NextSynchronization.Equal(record.LastSynchronizedAt.Add(filePeerSyncInterval)) ||
			!record.ExpiresAt.Equal(record.LastSynchronizedAt.Add(filePeerLeaseDuration)) ||
			!record.UpdatedAt.Equal(record.LastSynchronizedAt) {
			return errors.New("cluster file peer active grant lease is invalid")
		}
	default:
		return errors.New("cluster file peer grant state is invalid")
	}
	return nil
}

func validateFilePeerRouteV2(record filePeerRouteV2) error {
	origin, originErr := NormalizeV2Origin(record.PeerOrigin)
	if !validID(record.LinkID) || !validID(record.ControllerID) ||
		!validID(record.ControllerTransaction) || !validID(record.PeerNodeID) ||
		!validFingerprintV2(record.ControllerFingerprint) ||
		originErr != nil || origin != record.PeerOrigin || record.Scope != filePeerReadScope ||
		record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() ||
		record.UpdatedAt.Before(record.CreatedAt) ||
		!record.ExpiresAt.Equal(record.UpdatedAt.Add(filePeerLeaseDuration)) {
		return errors.New("cluster file peer store contains an invalid route")
	}
	return nil
}

func validateFilePeerHostV2(host hostRecordV2) error {
	if validateHostRecordV2(host) != nil || host.State != hostStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(host.Scope)) {
		return ErrAuthentication
	}
	return nil
}

func validateFilePeerIntentHostV2(host hostRecordV2) error {
	if validateHostRecordV2(host) != nil {
		return ErrAuthentication
	}
	switch host.State {
	case hostStateV2PendingPair, hostStateV2PendingCommit:
		return nil
	case hostStateV2Active:
		if ScopeAllowsFiles(normalizedV2Scope(host.Scope)) {
			return nil
		}
	}
	return ErrAuthentication
}

func validateFilePeerControllerV2(controller controllerRecordV2) error {
	if validateControllerRecordV2(controller) != nil ||
		controller.State != controllerStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(controller.Scope)) {
		return ErrAuthentication
	}
	return nil
}

func activeFilePeerGrantV2(record filePeerGrantV2, now time.Time) bool {
	return record.State == filePeerGrantActive && record.ExpiresAt.After(now)
}

func pendingFilePeerGrantV2(record filePeerGrantV2, now time.Time) filePeerGrantV2 {
	record.State = filePeerGrantPending
	record.UpdatedAt = now.UTC()
	record.LastSynchronizedAt = time.Time{}
	record.NextSynchronization = now.UTC()
	record.ExpiresAt = time.Time{}
	return record
}

func grantMatchesHostV2(grant filePeerGrantV2, host hostRecordV2) bool {
	return grant.HostID == host.ID &&
		grant.HostTransaction == host.TransactionID &&
		grant.HostControllerID == host.ControllerID &&
		grant.PeerNodeID == host.RemoteNodeID &&
		grant.PeerFingerprint == host.PeerFingerprint
}

func routeMatchesControllerV2(
	route filePeerRouteV2,
	controller controllerRecordV2,
) bool {
	return route.ControllerID == controller.ID &&
		route.ControllerTransaction == controller.TransactionID &&
		route.ControllerFingerprint == controller.Fingerprint
}

func validFingerprintV2(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	digest := strings.TrimPrefix(value, "sha256:")
	return len(digest) == 64 && validHex(digest)
}

func cloneFilePeerStateV2(source persistedFilePeersV2) persistedFilePeersV2 {
	routes := make([]filePeerRouteV2, len(source.Routes))
	copy(routes, source.Routes)
	grants := make([]filePeerGrantV2, len(source.Grants))
	copy(grants, source.Grants)
	return persistedFilePeersV2{
		SchemaVersion: source.SchemaVersion,
		Routes:        routes,
		Grants:        grants,
	}
}

func orderedFilePeerStateV2(source persistedFilePeersV2) persistedFilePeersV2 {
	result := cloneFilePeerStateV2(source)
	sort.Slice(result.Routes, func(i, j int) bool {
		return result.Routes[i].ControllerID < result.Routes[j].ControllerID
	})
	sort.Slice(result.Grants, func(i, j int) bool {
		return result.Grants[i].HostID < result.Grants[j].HostID
	})
	return result
}

func filePeerStatesEqualV2(left, right persistedFilePeersV2) bool {
	if left.SchemaVersion != right.SchemaVersion ||
		len(left.Routes) != len(right.Routes) ||
		len(left.Grants) != len(right.Grants) {
		return false
	}
	for index := range left.Routes {
		if left.Routes[index] != right.Routes[index] {
			return false
		}
	}
	for index := range left.Grants {
		if left.Grants[index] != right.Grants[index] {
			return false
		}
	}
	return true
}
