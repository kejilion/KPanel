package cluster

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// This separate store does not change the persisted federation schema. A
// damaged policy disables desktop access until an administrator repairs it.
type desktopPolicy struct {
	mu        sync.RWMutex
	path      string
	disabled  map[string]bool
	available bool
}

func openDesktopPolicy(dataDir string) *desktopPolicy {
	p := &desktopPolicy{path: filepath.Join(dataDir, "cluster-desktop-policy.json"), disabled: make(map[string]bool), available: true}
	if err := recoverAtomicTargetV2(p.path, defaultAtomicFileOpsV2()); err != nil {
		p.available = false
		return p
	}
	content, err := readRegularFileV2(p.path, 16<<10, false)
	if errors.Is(err, os.ErrNotExist) {
		return p
	}
	if err != nil || json.Unmarshal(content, &p.disabled) != nil || p.disabled == nil || len(p.disabled) > MaxHosts {
		p.available = false
		return p
	}
	for id, disabled := range p.disabled {
		if !validID(id) || !disabled {
			p.available = false
		}
	}
	return p
}

func (s *Service) desktopAllowed(hostID string) bool {
	p := s.desktopPolicy
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.available && !p.disabled[hostID]
}

func (s *Service) SetDesktopAllowed(hostID string, allowed bool) error {
	record, err := s.light.Host(hostID)
	if err != nil {
		return err
	}
	if !lightHostIsWindows(record) {
		return ErrProtocolMismatch
	}
	p := s.desktopPolicy
	p.mu.Lock()
	if !p.available {
		p.mu.Unlock()
		return ErrAuthentication
	}
	updated := make(map[string]bool)
	for id := range p.disabled {
		if _, err := s.light.Host(id); err == nil {
			updated[id] = true
		}
	}
	if allowed {
		delete(updated, hostID)
	} else {
		updated[hostID] = true
	}
	content, err := json.Marshal(updated)
	if err == nil {
		err = atomicWriteFileV2(p.path, content, 0o600, true, defaultAtomicFileOpsV2())
	}
	if err == nil {
		p.disabled = updated
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	if !allowed {
		h := s.fileStreamHub
		h.mu.Lock()
		for conn, node := range h.desktopConnections {
			if node == hostID {
				conn.close()
			}
		}
		h.mu.Unlock()
	}
	return nil
}
