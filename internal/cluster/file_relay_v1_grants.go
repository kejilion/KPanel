package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A legacy v1 controller was paired for read-only summaries. The target
// administrator grants its file-manager relay separately; the grant lives in a
// sidecar so older Panels, which strictly decode cluster-state.json, can still
// read their own state after a rollback and simply ignore the grant.
const (
	fileRelayV1GrantFileName    = "cluster-v1-file-relay.json"
	maxFileRelayV1GrantBytes    = 256 << 10
	maxFileRelayV1GrantsPerNode = MaxHosts
)

type fileRelayV1Grant struct {
	ControllerID string    `json:"controllerId"`
	Fingerprint  string    `json:"fingerprint"`
	GrantedAt    time.Time `json:"grantedAt"`
}

type persistedFileRelayV1Grants struct {
	SchemaVersion int                `json:"schemaVersion"`
	Grants        []fileRelayV1Grant `json:"grants"`
}

type fileRelayV1GrantStore struct {
	mu    sync.RWMutex
	path  string
	state persistedFileRelayV1Grants
	ops   atomicFileOpsV2
}

func openFileRelayV1GrantStore(path string) (*fileRelayV1GrantStore, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) ||
		filepath.Base(filepath.Clean(path)) != fileRelayV1GrantFileName {
		return nil, fmt.Errorf("cluster v1 file relay grant path must be an absolute %s path", fileRelayV1GrantFileName)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create cluster v1 file relay grant directory: %w", err)
	}
	ops := defaultAtomicFileOpsV2()
	if err := recoverAtomicTargetV2(path, ops); err != nil {
		return nil, fmt.Errorf("recover cluster v1 file relay grants: %w", err)
	}
	store := &fileRelayV1GrantStore{
		path:  path,
		state: persistedFileRelayV1Grants{SchemaVersion: 1, Grants: []fileRelayV1Grant{}},
		ops:   ops,
	}
	content, err := readRegularFileV2(path, maxFileRelayV1GrantBytes, false)
	switch {
	case err == nil:
		loadErr := decodeFileRelayV1Grants(content, &store.state)
		if loadErr != nil {
			if restoreErr := restoreAtomicBackupV2(path, ops); restoreErr != nil {
				return nil, fmt.Errorf("decode cluster v1 file relay grants: %v (backup recovery: %w)", loadErr, restoreErr)
			}
			content, err = readRegularFileV2(path, maxFileRelayV1GrantBytes, false)
			if err != nil {
				return nil, fmt.Errorf("read recovered cluster v1 file relay grants: %w", err)
			}
			store.state = persistedFileRelayV1Grants{}
			if err := decodeFileRelayV1Grants(content, &store.state); err != nil {
				return nil, fmt.Errorf("decode recovered cluster v1 file relay grants: %w", err)
			}
		} else if err := discardAtomicBackupV2(path, ops); err != nil {
			return nil, fmt.Errorf("finalize cluster v1 file relay grant recovery: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		// No file until the first grant: an untouched installation keeps the
		// exact set of files older versions expect.
	default:
		return nil, fmt.Errorf("read cluster v1 file relay grants: %w", err)
	}
	return store, nil
}

func decodeFileRelayV1Grants(content []byte, state *persistedFileRelayV1Grants) error {
	if err := decodeStrictJSONV2(content, state); err != nil {
		return err
	}
	if state.SchemaVersion != 1 || len(state.Grants) > maxFileRelayV1GrantsPerNode {
		return errors.New("cluster v1 file relay grants are invalid")
	}
	if state.Grants == nil {
		state.Grants = []fileRelayV1Grant{}
	}
	seen := make(map[string]struct{}, len(state.Grants))
	for _, grant := range state.Grants {
		if !validID(grant.ControllerID) || !validFileRelayV1Fingerprint(grant.Fingerprint) || grant.GrantedAt.IsZero() {
			return errors.New("cluster v1 file relay grant is invalid")
		}
		if _, exists := seen[grant.ControllerID]; exists {
			return errors.New("cluster v1 file relay grant is duplicated")
		}
		seen[grant.ControllerID] = struct{}{}
	}
	return nil
}

func validFileRelayV1Fingerprint(value string) bool {
	return strings.HasPrefix(value, "SHA256:") && len(value) > len("SHA256:") && len(value) <= 128
}

// Granted binds the grant to the controller's current key. A controller that
// re-pairs with a different key under the same ID starts without file access.
func (s *fileRelayV1GrantStore) Granted(controllerID, fingerprint string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, grant := range s.state.Grants {
		if grant.ControllerID == controllerID {
			return grant.Fingerprint == fingerprint
		}
	}
	return false
}

func (s *fileRelayV1GrantStore) Set(controllerID, fingerprint string, enabled bool, now time.Time) error {
	if s == nil {
		return errors.New("cluster v1 file relay grants are unavailable")
	}
	if !validID(controllerID) || (enabled && !validFileRelayV1Fingerprint(fingerprint)) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]fileRelayV1Grant, 0, len(s.state.Grants)+1)
	changed := false
	for _, grant := range s.state.Grants {
		if grant.ControllerID == controllerID {
			if enabled && grant.Fingerprint == fingerprint {
				return nil
			}
			changed = true
			continue
		}
		next = append(next, grant)
	}
	if enabled {
		if len(next) >= maxFileRelayV1GrantsPerNode {
			return ErrHostLimit
		}
		next = append(next, fileRelayV1Grant{ControllerID: controllerID, Fingerprint: fingerprint, GrantedAt: now.UTC()})
		changed = true
	}
	if !changed {
		return nil
	}
	sort.Slice(next, func(i, j int) bool { return next[i].ControllerID < next[j].ControllerID })
	previous := s.state
	s.state = persistedFileRelayV1Grants{SchemaVersion: 1, Grants: next}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *fileRelayV1GrantStore) Delete(controllerID string) error {
	return s.Set(controllerID, "", false, time.Time{})
}

func (s *fileRelayV1GrantStore) persistLocked() error {
	content, err := jsonMarshalIndentV2(s.state)
	if err != nil {
		return fmt.Errorf("encode cluster v1 file relay grants: %w", err)
	}
	if int64(len(content)) > maxFileRelayV1GrantBytes {
		return errors.New("cluster v1 file relay grants exceed their size limit")
	}
	if err := atomicWriteFileV2(s.path, content, 0o600, true, s.ops); err != nil {
		return fmt.Errorf("persist cluster v1 file relay grants: %w", err)
	}
	return nil
}
