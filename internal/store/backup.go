package store

import (
	"encoding/json"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/backup"
)

// ExportIdentity excludes sessions, login attempts, audit history and live share
// authorizations. Restoring a backup must never reactivate a bearer credential.
func (s *Store) ExportIdentity() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]User, len(s.data.Users))
	copy(users, s.data.Users)
	for i := range users {
		users[i].TOTPRecoveryCodeHashes = nil
	}
	return json.Marshal(diskState{SchemaVersion: 1, Users: users})
}

func ValidateIdentityBackup(data []byte) error {
	var state diskState
	if err := backup.Decode(data, &state); err != nil {
		return err
	}
	if state.SchemaVersion != 1 || len(state.Users) != 1 || len(state.Sessions) != 0 || len(state.Audit) != 0 || len(state.LoginAttempts) != 0 || len(state.FileShares) != 0 || state.ClusterShare.Enabled || state.ClusterShare.Token != "" || state.SecurityEntrance.Enabled {
		return errors.New("invalid panel identity backup")
	}
	u := state.Users[0]
	if u.ID == "" || len(u.ID) > 128 || u.Role != "admin" || u.Username == "" || len(u.Username) > 128 || len(u.PasswordHash) > 1024 || len(u.PasswordHash) < 32 || len(u.TOTPRecoveryCodeHashes) != 0 {
		return errors.New("invalid backup account")
	}
	return nil
}

// RestoreIdentity retains the destination entrance and audit history, while
// revoking all current login sessions and public shares.
func (s *Store) RestoreIdentity(data []byte) error {
	if err := ValidateIdentityBackup(data); err != nil {
		return err
	}
	var incoming diskState
	if err := json.Unmarshal(data, &incoming); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneDiskState(s.data)
	s.data.Users = incoming.Users
	s.data.Sessions = nil
	s.data.LoginAttempts = nil
	s.data.FileShares = nil
	s.data.ClusterShare = ClusterShare{}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}
