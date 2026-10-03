package panel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const desktopSessionsPath = "/api/v1/desktop-sessions"

type panelDesktopSession struct {
	id, hostID, userID, nonce string
	token                     [32]byte
	expires                   time.Time
	claimed                   bool
	cancel                    context.CancelFunc
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
	if r.URL.Path == desktopSessionsPath && r.Method == http.MethodPost {
		var input struct {
			HostID string `json:"hostId"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		host, err := s.cluster.Host(r.Context(), input.HostID)
		if err != nil || !host.DesktopAvailable {
			s.writeProblem(w, r, http.StatusConflict, "desktop_unavailable", "Remote desktop is unavailable", "")
			return
		}
		if s.audit(r, session.User.ID, "desktop.open", "cluster_host", host.ID, "intent", nil) != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
			return
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
		item := &panelDesktopSession{id: id, hostID: host.ID, userID: session.User.ID, nonce: hex.EncodeToString(nonce), token: sha256.Sum256([]byte(token)), expires: now.Add(time.Minute)}
		s.desktopSessionMu.Lock()
		if s.desktopSessions == nil {
			s.desktopSessions = make(map[string]*panelDesktopSession)
		}
		userCount, hostCount := 0, 0
		for key, current := range s.desktopSessions {
			if !current.claimed && now.After(current.expires) {
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
		if len(s.desktopSessions) >= 16 || userCount >= 4 || hostCount >= 2 {
			s.desktopSessionMu.Unlock()
			s.writeProblem(w, r, 429, "desktop_limit", "Desktop session limit reached", "")
			return
		}
		s.desktopSessions[id] = item
		s.desktopSessionMu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusCreated, map[string]any{"sessionId": id, "hostId": host.ID, "nonce": item.nonce, "expiresAt": item.expires})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, desktopSessionsPath+"/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || (parts[1] != "stream" && parts[1] != "close") {
		s.writeProblem(w, r, 404, "route_not_found", "Route not found", "")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
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
	if parts[1] != "stream" || r.Method != http.MethodGet || item.claimed || !time.Now().Before(item.expires) {
		s.desktopSessionMu.Unlock()
		s.writeProblem(w, r, 409, "desktop_session_consumed", "Desktop session is expired or in use", "")
		return
	}
	item.claimed, item.cancel = true, cancel
	s.desktopSessionMu.Unlock()
	defer func() {
		s.desktopSessionMu.Lock()
		delete(s.desktopSessions, item.id)
		s.desktopSessionMu.Unlock()
		_ = s.audit(r, item.userID, "desktop.close", "cluster_host", item.hostID, "success", nil)
	}()
	stream, err := s.cluster.OpenDesktopStream(ctx, item.hostID, item.nonce)
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
	_ = s.audit(r, item.userID, "desktop.open", "cluster_host", item.hostID, "success", nil)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := s.auth.Authenticate(token)
				host, hostErr := s.cluster.Host(ctx, item.hostID)
				if err != nil || hostErr != nil || !host.DesktopAvailable || !time.Now().Before(session.ExpiresAt) {
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
		if _, err = stream.Write(data); err != nil {
			break
		}
	}
	cancel()
	_ = stream.Close()
	<-done
}
