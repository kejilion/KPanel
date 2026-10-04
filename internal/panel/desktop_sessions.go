package panel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

const desktopSessionsPath = "/api/v1/desktop-sessions"

type panelDesktopSession struct {
	id, hostID, userID, nonce string
	token                     [32]byte
	expires                   time.Time
	claimed                   bool
	cancel                    context.CancelFunc
	prepared                  *cluster.PreparedDesktopStream
	preparing                 bool
	accountMode               string
}

func (s *Server) closeDesktopSessions() {
	s.desktopSessionMu.Lock()
	defer s.desktopSessionMu.Unlock()
	for id, item := range s.desktopSessions {
		if item.cancel != nil {
			item.cancel()
		}
		delete(s.desktopSessions, id)
	}
}

func (s *Server) handleDesktopSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.Header.Get("Origin") == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_desktop_request", "Invalid desktop request", "")
		return
	}
	token, session, ok := s.requireSession(w, r)
	if !ok || !s.checkOrigin(w, r) {
		return
	}
	if r.Method != http.MethodGet && !s.checkCSRF(w, r, session) {
		return
	}
	if strings.HasPrefix(r.URL.Path, desktopSessionsPath+"/credentials/") {
		s.handleDesktopCredentials(w, r, session.User.ID)
		return
	}
	if r.URL.Path == desktopSessionsPath+"/policy" && r.Method == http.MethodPost {
		var input struct {
			HostID  string `json:"hostId"`
			Allowed *bool  `json:"allowed"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		if input.Allowed == nil {
			s.writeProblem(w, r, 400, "invalid_desktop_request", "Missing desktop policy", "")
			return
		}
		if s.audit(r, session.User.ID, "desktop.policy", "cluster_host", input.HostID, "intent", map[string]any{"allowed": *input.Allowed}) != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if err := s.cluster.SetDesktopAllowed(input.HostID, *input.Allowed); err != nil {
			s.writeProblem(w, r, 409, "desktop_policy_failed", "Desktop policy update failed", "")
			return
		}
		_ = s.audit(r, session.User.ID, "desktop.policy", "cluster_host", input.HostID, "success", map[string]any{"allowed": *input.Allowed})
		s.writeJSON(w, 200, map[string]bool{"allowed": *input.Allowed})
		return
	}
	if r.URL.Path == desktopSessionsPath && r.Method == http.MethodPost {
		var input struct {
			HostID                string `json:"hostId"`
			UseSavedCredentials   bool   `json:"useSavedCredentials"`
			UseManagedCredentials bool   `json:"useManagedCredentials"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		if input.UseSavedCredentials && input.UseManagedCredentials {
			s.writeProblem(w, r, 400, "invalid_desktop_request", "Choose one desktop account source", "")
			return
		}
		host, err := s.cluster.Host(r.Context(), input.HostID)
		if err != nil || !host.DesktopAvailable {
			s.writeProblem(w, r, http.StatusConflict, "desktop_unavailable", "Remote desktop is unavailable", "")
			return
		}
		accountMode := "manual"
		if input.UseSavedCredentials {
			accountMode = "saved"
		}
		if input.UseManagedCredentials {
			accountMode = "managed-administrator"
		}
		if s.audit(r, session.User.ID, "desktop.open", "cluster_host", host.ID, "intent", map[string]any{"accountMode": accountMode}) != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		var credentials *desktopcredentials.Credentials
		if input.UseSavedCredentials {
			saved, err := s.savedDesktopCredentials(session.User.ID, host.ID)
			if errors.Is(err, desktopcredentials.ErrMissing) {
				s.writeProblem(w, r, 409, "desktop_credentials_missing", "Saved Windows account must be configured again", "")
				return
			}
			if err != nil {
				s.writeProblem(w, r, 503, "desktop_credentials_unavailable", "Saved desktop credentials unavailable", "")
				return
			}
			credentials = &saved
		}
		id, err := randomTerminalSessionID()
		if err != nil {
			s.writeProblem(w, r, 500, "desktop_session_failed", "Desktop session failed", "")
			return
		}
		nonce := make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			s.writeProblem(w, r, 500, "desktop_session_failed", "Desktop session failed", "")
			return
		}
		now := time.Now()
		item := &panelDesktopSession{id: id, hostID: host.ID, userID: session.User.ID, nonce: hex.EncodeToString(nonce), token: sha256.Sum256([]byte(token)), expires: now.Add(time.Minute), accountMode: accountMode}
		leaseCtx, leaseCancel := context.WithDeadline(context.Background(), session.ExpiresAt)
		item.cancel, item.preparing = leaseCancel, input.UseManagedCredentials
		s.desktopSessionMu.Lock()
		if s.desktopSessions == nil {
			s.desktopSessions = make(map[string]*panelDesktopSession)
		}
		userCount, hostCount := 0, 0
		for key, current := range s.desktopSessions {
			if !current.claimed && now.After(current.expires) {
				if current.cancel != nil {
					current.cancel()
				}
				delete(s.desktopSessions, key)
				continue
			}
			if current.userID == item.userID {
				userCount++
			}
			if current.hostID == item.hostID {
				hostCount++
			}
		}
		if len(s.desktopSessions) >= 4 || userCount >= 2 || hostCount >= 1 {
			s.desktopSessionMu.Unlock()
			leaseCancel()
			s.writeProblem(w, r, 429, "desktop_limit", "Desktop session limit reached", "")
			return
		}
		s.desktopSessions[id] = item
		s.desktopSessionMu.Unlock()
		// Expire unclaimed allocations without requiring another browser request.
		expiry := time.AfterFunc(time.Until(item.expires), func() {
			s.desktopSessionMu.Lock()
			defer s.desktopSessionMu.Unlock()
			if s.desktopSessions[id] == item && !item.claimed {
				item.cancel()
				delete(s.desktopSessions, id)
			}
		})
		context.AfterFunc(leaseCtx, func() {
			expiry.Stop()
			s.desktopSessionMu.Lock()
			defer s.desktopSessionMu.Unlock()
			if s.desktopSessions[id] == item && !item.claimed {
				delete(s.desktopSessions, id)
			}
		})
		if input.UseManagedCredentials {
			// Watch authorization during preparation and the unclaimed lease too.
			// The WebSocket handler takes over the same checks after claim.
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-leaseCtx.Done():
						return
					case <-ticker.C:
						s.desktopSessionMu.Lock()
						claimed := item.claimed
						s.desktopSessionMu.Unlock()
						if claimed {
							return
						}
						_, authErr := s.auth.Authenticate(token)
						if authErr != nil || !s.cluster.ManagedDesktopAvailable(host.ID) {
							leaseCancel()
							return
						}
					}
				}
			}()
			stopRequest := context.AfterFunc(r.Context(), leaseCancel)
			prepared, issued, prepareErr := s.cluster.PrepareManagedDesktop(leaseCtx, host.ID, item.nonce)
			stopRequest()
			_, authErr := s.auth.Authenticate(token)
			currentHost, hostErr := s.cluster.Host(r.Context(), host.ID)
			s.desktopSessionMu.Lock()
			accepted := prepareErr == nil && authErr == nil && hostErr == nil && currentHost.DesktopAvailable &&
				s.cluster.ManagedDesktopAvailable(host.ID) && leaseCtx.Err() == nil && s.desktopSessions[id] == item
			if accepted {
				item.prepared, item.preparing = prepared, false
			} else {
				delete(s.desktopSessions, id)
			}
			s.desktopSessionMu.Unlock()
			if !accepted {
				leaseCancel()
				if prepared != nil {
					_ = prepared.Close()
				}
				issued.Password = ""
				s.writeProblem(w, r, 502, "desktop_managed_unavailable", "Managed Windows account is unavailable", "")
				return
			}
			credentials = &issued
		}
		w.Header().Set("Cache-Control", "no-store")
		response := map[string]any{"sessionId": id, "hostId": host.ID, "nonce": item.nonce, "expiresAt": item.expires}
		if credentials != nil {
			response["credentials"] = credentials
			defer func() { credentials.Password = "" }()
		}
		s.writeJSON(w, http.StatusCreated, response)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, desktopSessionsPath+"/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || (parts[1] != "stream" && parts[1] != "close") {
		s.writeProblem(w, r, 404, "route_not_found", "Route not found", "")
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), session.ExpiresAt)
	defer cancel()
	s.desktopSessionMu.Lock()
	item := s.desktopSessions[parts[0]]
	if item == nil || item.userID != session.User.ID || item.token != sha256.Sum256([]byte(token)) {
		s.desktopSessionMu.Unlock()
		s.writeProblem(w, r, 404, "desktop_not_found", "Desktop session not found", "")
		return
	}
	if parts[1] == "close" && r.Method == http.MethodPost {
		if item.cancel != nil {
			item.cancel()
		}
		delete(s.desktopSessions, item.id)
		s.desktopSessionMu.Unlock()
		s.writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
		return
	}
	if parts[1] != "stream" || r.Method != http.MethodGet || item.claimed || item.preparing || !time.Now().Before(item.expires) {
		s.desktopSessionMu.Unlock()
		s.writeProblem(w, r, 409, "desktop_session_consumed", "Desktop session is expired or in use", "")
		return
	}
	leaseCancel := item.cancel
	item.claimed, item.cancel = true, func() {
		cancel()
		if leaseCancel != nil {
			leaseCancel()
		}
	}
	s.desktopSessionMu.Unlock()
	defer func() {
		item.cancel()
		s.desktopSessionMu.Lock()
		delete(s.desktopSessions, item.id)
		s.desktopSessionMu.Unlock()
		_ = s.audit(r, item.userID, "desktop.close", "cluster_host", item.hostID, "success", map[string]any{"accountMode": item.accountMode})
	}()
	var stream io.ReadWriteCloser
	var err error
	if item.prepared != nil {
		if !s.cluster.ManagedDesktopAvailable(item.hostID) {
			s.writeProblem(w, r, 409, "desktop_managed_unavailable", "Managed Windows account is unavailable", "")
			return
		}
		stream = item.prepared
		err = item.prepared.Claim()
	} else {
		stream, err = s.cluster.OpenDesktopStream(ctx, item.hostID, item.nonce)
	}
	if err != nil {
		s.writeProblem(w, r, 502, "desktop_open_failed", "Desktop connection failed", "")
		return
	}
	defer stream.Close()
	// IronRDP's browser client does not send a subprotocol. Admission instead
	// binds this one-use POST allocation to the exact login, Origin and nonce;
	// the nonce is authenticated again inside the node's RDCleanPath bridge.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(256 << 10)
	context.AfterFunc(ctx, func() { _ = ws.CloseNow(); _ = stream.Close() })
	_ = s.audit(r, item.userID, "desktop.open", "cluster_host", item.hostID, "success", map[string]any{"accountMode": item.accountMode})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := s.auth.Authenticate(token)
				host, hostErr := s.cluster.Host(ctx, item.hostID)
				if err != nil || hostErr != nil || !host.DesktopAvailable ||
					item.accountMode == "managed-administrator" && !s.cluster.ManagedDesktopAvailable(item.hostID) || !time.Now().Before(session.ExpiresAt) {
					cancel()
					return
				}
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buffer := make([]byte, 60<<10)
		for {
			n, err := stream.Read(buffer)
			if n > 0 {
				if _, err := s.auth.Authenticate(token); err != nil {
					return
				}
				writeCtx, stop := context.WithTimeout(ctx, 15*time.Second)
				writeErr := ws.Write(writeCtx, websocket.MessageBinary, buffer[:n])
				stop()
				if writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		kind, data, err := ws.Read(ctx)
		if err != nil || kind != websocket.MessageBinary || len(data) == 0 {
			break
		}
		if _, err := s.auth.Authenticate(token); err != nil {
			break
		}
		if _, err = stream.Write(data); err != nil {
			break
		}
	}
	cancel()
	_ = stream.Close()
	<-done
}
