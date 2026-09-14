package cluster

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	lightBatchStateFileName        = "cluster-light-batch-state.json"
	maxLightBatchStateBytes        = int64(2 << 20)
	maxLightBatchPolicies          = 16
	maxLightBatchAttemptsPerPolicy = MaxHosts
	maxLightBatchAttempts          = maxLightBatchPolicies * maxLightBatchAttemptsPerPolicy
)

type lightBatchAttemptRecord struct {
	ID                string    `json:"id"`
	NodeID            string    `json:"nodeId"`
	Name              string    `json:"name"`
	NodeVersion       string    `json:"nodeVersion"`
	TerminalPublicKey string    `json:"terminalPublicKey,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

type lightBatchEnrollmentRecord struct {
	ID         string                    `json:"id"`
	SecretHash string                    `json:"secretHash"`
	NamePrefix string                    `json:"namePrefix,omitempty"`
	MaxUses    int                       `json:"maxUses"`
	CreatedAt  time.Time                 `json:"createdAt"`
	ExpiresAt  time.Time                 `json:"expiresAt"`
	Attempts   []lightBatchAttemptRecord `json:"attempts"`
}

type lightBatchPersistedState struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Enrollments   []lightBatchEnrollmentRecord `json:"enrollments"`
}

type lightBatchStore struct {
	mu    sync.RWMutex
	path  string
	state lightBatchPersistedState
	ops   atomicFileOpsV2
}

func openLightBatchStore(path string) (*lightBatchStore, error) {
	if !filepath.IsAbs(path) || filepath.Base(filepath.Clean(path)) != lightBatchStateFileName {
		return nil, errors.New("light node batch store path is invalid")
	}
	if err := protectDirectoryV2(filepath.Dir(path)); err != nil {
		return nil, err
	}
	ops := defaultAtomicFileOpsV2()
	if err := recoverAtomicTargetV2(path, ops); err != nil {
		return nil, fmt.Errorf("recover light node batch store: %w", err)
	}
	store := &lightBatchStore{
		path: path,
		state: lightBatchPersistedState{
			SchemaVersion: 1,
			Enrollments:   []lightBatchEnrollmentRecord{},
		},
		ops: ops,
	}
	content, err := readRegularFileV2(path, maxLightBatchStateBytes, false)
	switch {
	case err == nil:
		loadErr := decodeLightBatchState(content, &store.state)
		if loadErr != nil {
			if restoreErr := restoreAtomicBackupV2(path, ops); restoreErr != nil {
				return nil, fmt.Errorf(
					"decode light node batch store: %v (backup recovery: %w)",
					loadErr,
					restoreErr,
				)
			}
			content, err = readRegularFileV2(path, maxLightBatchStateBytes, false)
			if err != nil {
				return nil, fmt.Errorf("read recovered light node batch store: %w", err)
			}
			store.state = lightBatchPersistedState{}
			if err := decodeLightBatchState(content, &store.state); err != nil {
				return nil, fmt.Errorf("decode recovered light node batch store: %w", err)
			}
		} else if err := discardAtomicBackupV2(path, ops); err != nil {
			return nil, fmt.Errorf("finalize light node batch store recovery: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read light node batch store: %w", err)
	}
	return store, nil
}

func decodeLightBatchState(content []byte, state *lightBatchPersistedState) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(state); err != nil {
		return fmt.Errorf("decode light node batch store: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("light node batch store contains multiple JSON values")
	}
	if state.SchemaVersion != 1 || len(state.Enrollments) > maxLightBatchPolicies {
		return errors.New("light node batch store is invalid")
	}
	policyIDs := make(map[string]struct{}, len(state.Enrollments))
	nodeIDs := make(map[string]struct{})
	totalAttempts := 0
	for _, enrollment := range state.Enrollments {
		secretHash, err := hex.DecodeString(enrollment.SecretHash)
		prefix, prefixErr := validateLightBatchNamePrefix(enrollment.NamePrefix)
		if !validID(enrollment.ID) || err != nil || len(secretHash) != sha256.Size ||
			prefixErr != nil || prefix != enrollment.NamePrefix || enrollment.MaxUses < 1 ||
			enrollment.MaxUses > maxLightBatchAttemptsPerPolicy || enrollment.CreatedAt.IsZero() ||
			enrollment.ExpiresAt.IsZero() || !enrollment.ExpiresAt.After(enrollment.CreatedAt) ||
			len(enrollment.Attempts) > enrollment.MaxUses {
			return errors.New("light node batch store contains an invalid enrollment")
		}
		if _, exists := policyIDs[enrollment.ID]; exists {
			return errors.New("light node batch store contains a duplicate enrollment")
		}
		policyIDs[enrollment.ID] = struct{}{}
		attemptIDs := make(map[string]struct{}, len(enrollment.Attempts))
		for _, attempt := range enrollment.Attempts {
			terminalPublicKey, terminalErr := decodeTerminalRelayPublicKey(attempt.TerminalPublicKey)
			validatedName, nameErr := validateRequiredName(attempt.Name)
			if !validID(attempt.ID) || !validID(attempt.NodeID) || nameErr != nil ||
				validatedName != attempt.Name || cleanDisplayText(attempt.NodeVersion, 64) != attempt.NodeVersion ||
				terminalErr != nil || (len(terminalPublicKey) != 0 && len(terminalPublicKey) != 32) ||
				attempt.CreatedAt.IsZero() || attempt.CreatedAt.Before(enrollment.CreatedAt) ||
				attempt.CreatedAt.After(enrollment.ExpiresAt) {
				return errors.New("light node batch store contains an invalid attempt")
			}
			if _, exists := attemptIDs[attempt.ID]; exists {
				return errors.New("light node batch store contains a duplicate attempt")
			}
			if _, exists := nodeIDs[attempt.NodeID]; exists {
				return errors.New("light node batch store contains a duplicate node")
			}
			attemptIDs[attempt.ID] = struct{}{}
			nodeIDs[attempt.NodeID] = struct{}{}
		}
		totalAttempts += len(enrollment.Attempts)
	}
	if totalAttempts > maxLightBatchAttempts {
		return errors.New("light node batch store contains too many attempts")
	}
	return nil
}

func (s *lightBatchStore) Create(record lightBatchEnrollmentRecord, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	secretHash, err := hex.DecodeString(record.SecretHash)
	prefix, prefixErr := validateLightBatchNamePrefix(record.NamePrefix)
	if !validID(record.ID) || err != nil || len(secretHash) != sha256.Size || prefixErr != nil ||
		prefix != record.NamePrefix || record.MaxUses < 1 || record.MaxUses > maxLightBatchAttemptsPerPolicy ||
		record.CreatedAt.IsZero() || !record.ExpiresAt.After(now.UTC()) ||
		!record.ExpiresAt.After(record.CreatedAt) || len(record.Attempts) != 0 {
		return ErrLightBatchInvalid
	}
	previous := cloneLightBatchState(s.state)
	s.gcLocked(now)
	for _, existing := range s.state.Enrollments {
		if existing.ID == record.ID {
			s.state = previous
			return ErrDuplicate
		}
	}
	if len(s.state.Enrollments) >= maxLightBatchPolicies {
		s.state = previous
		return ErrRateLimited
	}
	record.Attempts = []lightBatchAttemptRecord{}
	s.state.Enrollments = append(s.state.Enrollments, record)
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *lightBatchStore) List(now time.Time) []lightBatchEnrollmentRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]lightBatchEnrollmentRecord, 0, len(s.state.Enrollments))
	for _, enrollment := range s.state.Enrollments {
		if !enrollment.ExpiresAt.After(now.UTC()) {
			continue
		}
		enrollment.Attempts = append([]lightBatchAttemptRecord(nil), enrollment.Attempts...)
		result = append(result, enrollment)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (s *lightBatchStore) Reserve(
	policyID string,
	secretHash string,
	attemptID string,
	candidateNodeID string,
	requestedName string,
	nodeVersion string,
	terminalPublicKey string,
	allowNew bool,
	now time.Time,
) (lightBatchAttemptRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for policyIndex := range s.state.Enrollments {
		policy := &s.state.Enrollments[policyIndex]
		if policy.ID != policyID || !policy.ExpiresAt.After(now.UTC()) ||
			len(policy.SecretHash) != len(secretHash) ||
			subtle.ConstantTimeCompare([]byte(policy.SecretHash), []byte(secretHash)) != 1 {
			continue
		}
		for _, attempt := range policy.Attempts {
			if attempt.ID != attemptID {
				continue
			}
			expectedName := lightBatchNodeName(policy.NamePrefix, requestedName, attempt.NodeID)
			if expectedName != attempt.Name || attempt.TerminalPublicKey != terminalPublicKey {
				return lightBatchAttemptRecord{}, false, ErrIdentityMismatch
			}
			return attempt, false, nil
		}
		if !allowNew {
			return lightBatchAttemptRecord{}, false, ErrHostLimit
		}
		if len(policy.Attempts) >= policy.MaxUses {
			return lightBatchAttemptRecord{}, false, ErrPairingCode
		}
		if !validID(attemptID) || !validID(candidateNodeID) {
			return lightBatchAttemptRecord{}, false, ErrPairingCode
		}
		for _, enrollment := range s.state.Enrollments {
			for _, attempt := range enrollment.Attempts {
				if attempt.NodeID == candidateNodeID {
					return lightBatchAttemptRecord{}, false, ErrDuplicate
				}
			}
		}
		previous := cloneLightBatchState(s.state)
		attempt := lightBatchAttemptRecord{
			ID: attemptID, NodeID: candidateNodeID,
			Name:        lightBatchNodeName(policy.NamePrefix, requestedName, candidateNodeID),
			NodeVersion: cleanDisplayText(nodeVersion, 64), TerminalPublicKey: terminalPublicKey,
			CreatedAt: now.UTC(),
		}
		policy.Attempts = append(policy.Attempts, attempt)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return lightBatchAttemptRecord{}, false, err
		}
		return attempt, true, nil
	}
	return lightBatchAttemptRecord{}, false, ErrPairingCode
}

func (s *lightBatchStore) Delete(id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneLightBatchState(s.state)
	s.gcLocked(now)
	for index, enrollment := range s.state.Enrollments {
		if enrollment.ID != id {
			continue
		}
		s.state.Enrollments = append(s.state.Enrollments[:index], s.state.Enrollments[index+1:]...)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return err
		}
		return nil
	}
	if len(previous.Enrollments) != len(s.state.Enrollments) {
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return err
		}
	}
	return ErrNotFound
}

func (s *lightBatchStore) Checkpoint(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.state.Enrollments)
	previous := cloneLightBatchState(s.state)
	s.gcLocked(now)
	if before == len(s.state.Enrollments) {
		return nil
	}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *lightBatchStore) gcLocked(now time.Time) {
	filtered := s.state.Enrollments[:0]
	for _, enrollment := range s.state.Enrollments {
		if enrollment.ExpiresAt.After(now.UTC()) {
			filtered = append(filtered, enrollment)
		}
	}
	s.state.Enrollments = filtered
}

func (s *lightBatchStore) persistLocked() error {
	content, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	if int64(len(content)+1) > maxLightBatchStateBytes {
		return errors.New("light node batch store exceeds its size limit")
	}
	return atomicWriteFileV2(s.path, append(content, '\n'), 0o600, true, s.ops)
}

func cloneLightBatchState(state lightBatchPersistedState) lightBatchPersistedState {
	result := lightBatchPersistedState{
		SchemaVersion: state.SchemaVersion,
		Enrollments:   append([]lightBatchEnrollmentRecord(nil), state.Enrollments...),
	}
	for index := range result.Enrollments {
		result.Enrollments[index].Attempts = append(
			[]lightBatchAttemptRecord(nil),
			state.Enrollments[index].Attempts...,
		)
	}
	return result
}

func lightBatchTerminalKey(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
