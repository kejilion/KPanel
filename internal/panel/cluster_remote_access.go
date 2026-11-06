package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func (s *Server) handleClusterRemoteAccess(w http.ResponseWriter, r *http.Request) {
	s.handleClusterRemoteAccessScope(w, r, "")
}

func (s *Server) writeClusterRemoteAccess(w http.ResponseWriter, hostID string) {
	if hostID != "" {
		s.writeJSON(w, 200, struct {
			cluster.HostRemoteAccess
			PendingTerminalCleanup int `json:"pendingTerminalCleanup"`
		}{s.cluster.HostRemoteAccess(hostID), s.remoteTerminalCleanupPending(hostID)})
		return
	}
	s.writeJSON(w, 200, struct {
		cluster.RemoteAccess
		PendingTerminalCleanup int `json:"pendingTerminalCleanup"`
	}{s.cluster.RemoteAccess(), s.remoteTerminalCleanupPending()})
}

func (s *Server) handleClusterRemoteAccessScope(w http.ResponseWriter, r *http.Request, hostID string) {
	validHost := func() bool {
		if hostID == "" {
			return true
		}
		host, err := s.cluster.Host(r.Context(), hostID)
		if err != nil || host.IsLocal {
			s.writeProblem(w, r, 404, "host_not_found", "Remote host not found", "")
			return false
		}
		return true
	}
	if r.Method == http.MethodGet {
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		if !validHost() {
			return
		}
		s.writeClusterRemoteAccess(w, hostID)
		return
	}
	if r.Method != http.MethodPut {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	if !validHost() {
		return
	}
	var input struct {
		TerminalEnabled         *bool  `json:"terminalEnabled"`
		FilesEnabled            *bool  `json:"filesEnabled"`
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.TerminalEnabled == nil || input.FilesEnabled == nil {
		s.writeProblem(w, r, 422, "remote_access_invalid", "Remote access settings are invalid", "")
		return
	}
	change := map[string]any{"terminalEnabled": *input.TerminalEnabled, "filesEnabled": *input.FilesEnabled}
	if hostID != "" {
		change["hostId"] = hostID
	}
	if err := s.audit(r, session.User.ID, "cluster.remote-access.update", "cluster-settings", "remote-access", "intent", change); err != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	// Registration uses this same lock and rechecks the policy. An open that
	// raced this update can never escape the close-requested set.
	s.terminalMu.Lock()
	var err error
	terminalEnabled := true
	if hostID == "" {
		var value cluster.RemoteAccess
		value, err = s.cluster.UpdateRemoteAccess(input.ExpectedResourceVersion, *input.TerminalEnabled, *input.FilesEnabled)
		terminalEnabled = value.TerminalEnabled
	} else {
		var value cluster.HostRemoteAccess
		value, err = s.cluster.UpdateHostRemoteAccess(hostID, input.ExpectedResourceVersion, *input.TerminalEnabled, *input.FilesEnabled)
		terminalEnabled = value.EffectiveTerminalEnabled
	}
	if err == nil && !terminalEnabled {
		for id, item := range s.terminalSessions {
			if item.HostID == cluster.LocalHostID || hostID != "" && item.HostID != hostID {
				continue
			}
			item.CloseRequested = true
			if item.InputConnection != nil {
				item.InputConnection.cancel()
			}
			s.terminalSessions[id] = item
		}
	}
	s.terminalMu.Unlock()
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.remote-access.update", "cluster-settings", "remote-access", "failure", change)
		if errors.Is(err, cluster.ErrConflict) {
			s.writeProblem(w, r, 409, "remote_access_changed", "Remote access settings changed", "")
		} else {
			s.writeProblem(w, r, 503, "remote_access_storage_unavailable", "Remote access settings unavailable", "")
		}
		return
	}
	if !terminalEnabled {
		if s.closeDisabledRemoteTerminals(r.Context(), hostID) > 0 {
			s.scheduleTerminalCleanup()
		}
	}
	_ = s.audit(r, session.User.ID, "cluster.remote-access.update", "cluster-settings", "remote-access", "success", change)
	s.writeClusterRemoteAccess(w, hostID)
}

// Never lose identities on failed closure. A later save/explicit close retries
// them, while the admission policy already rejects input and reconnection.
func (s *Server) closeDisabledRemoteTerminals(parent context.Context, hostIDs ...string) int {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	s.terminalMu.Lock()
	items := make([]panelTerminalSession, 0)
	for _, item := range s.terminalSessions {
		if item.HostID != cluster.LocalHostID && item.CloseRequested && remoteCleanupIncludes(hostIDs, item.HostID) {
			items = append(items, item)
		}
	}
	s.terminalMu.Unlock()
	queue := make(chan panelTerminalSession, len(items))
	for _, item := range items {
		queue <- item
	}
	close(queue)
	var workers sync.WaitGroup
	for n := 0; n < min(4, len(items)); n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range queue {
				if ctx.Err() != nil {
					return
				}
				err := s.closeTerminalBackend(ctx, item)
				if err == nil || errors.Is(err, terminal.ErrClosed) || errors.Is(err, terminal.ErrNotFound) {
					s.deleteTerminalSession(item.ID)
				}
			}
		}()
	}
	workers.Wait()
	return s.remoteTerminalCleanupPending(hostIDs...)
}

func remoteCleanupIncludes(hostIDs []string, id string) bool {
	return len(hostIDs) == 0 || hostIDs[0] == "" || hostIDs[0] == id
}

func (s *Server) remoteTerminalCleanupPending(hostIDs ...string) int {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	pending := 0
	for _, item := range s.terminalSessions {
		if item.HostID != cluster.LocalHostID && item.CloseRequested && remoteCleanupIncludes(hostIDs, item.HostID) {
			pending++
		}
	}
	return pending
}

func (s *Server) registerTerminalSession(item panelTerminalSession) bool {
	s.terminalMu.Lock()
	denied := item.HostID != cluster.LocalHostID && !s.cluster.HostRemoteAccess(item.HostID).EffectiveTerminalEnabled
	if denied {
		item.CloseRequested = true
		item = s.detachUnreturnedTerminalLocked(item)
	}
	s.terminalSessions[item.ID] = item
	s.terminalMu.Unlock()
	if denied {
		if s.closeDisabledRemoteTerminals(context.Background(), item.HostID) > 0 {
			s.scheduleTerminalCleanup()
		}
	}
	return !denied
}

func (s *Server) rememberRemoteTerminalOpen(user, hostID string, opened cluster.TerminalOpenResponse) {
	if opened.SessionID == "" || hostID == cluster.LocalHostID {
		return
	}
	identity := sha256.Sum256([]byte(user + "\x00" + hostID + "\x00" + opened.SessionID))
	item := panelTerminalSession{ID: hex.EncodeToString(identity[:]), UserID: user, HostID: hostID, BackendSessionID: opened.SessionID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Retained: opened.Retained, CloseRequested: true}
	s.terminalMu.Lock()
	item = s.detachUnreturnedTerminalLocked(item)
	s.terminalSessions[item.ID] = item
	s.terminalMu.Unlock()
	s.scheduleTerminalCleanup()
}

// The client never received these sessions, so it cannot request detachment.
// Retain their backend identities for retry, using the same bounded detached
// capacity as explicit detachment. If that capacity is full, keep consuming an
// interactive slot rather than losing an uncertain backend process.
// Caller holds terminalMu.
func (s *Server) detachUnreturnedTerminalLocked(item panelTerminalSession) panelTerminalSession {
	item.Unreturned = true
	count, own := 0, 0
	for id, session := range s.terminalSessions {
		if id == item.ID || !session.Detached {
			continue
		}
		count++
		if session.UserID == item.UserID {
			own++
		}
	}
	item.Detached = count < maxPanelTerminals && own < maxPanelTerminalsByUser
	return item
}
