package panel

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

// A proxy may support HTTP/SSE but reject WebSocket upgrades. This fallback
// uses the SAME owner claim, sequence and replay window; it never replays bare
// POST input. One batch per terminal shares admission with its browser socket.
func (s *Server) handleTerminalInputBatch(w http.ResponseWriter, r *http.Request, token string, session auth.Session, id string) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		s.writeProblem(w, r, 400, "invalid_terminal_request", "Invalid terminal input batch", "")
		return
	}
	var input struct {
		Frames []terminal.InputFrame `json:"frames"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 96<<10)
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if len(input.Frames) == 0 || len(input.Frames) > terminal.InputWindow {
		s.writeProblem(w, r, 400, "terminal_input_sequence", "Invalid terminal input batch", "")
		return
	}
	for i, frame := range input.Frames {
		if !frame.Valid() || (frame.Seq == 0 && len(input.Frames) != 1) || (i > 0 && (frame.Stream != input.Frames[0].Stream || frame.Seq != input.Frames[i-1].Seq+1)) {
			s.writeProblem(w, r, 400, "terminal_input_sequence", "Invalid terminal input batch", "")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	connection := &terminalInputConnection{cancel: cancel}
	s.terminalMu.Lock()
	item, ok := s.terminalSessions[id]
	if !ok || item.UserID != session.User.ID || item.CloseRequested || !item.ReliableInput {
		s.terminalMu.Unlock()
		s.writeProblem(w, r, 404, "terminal_not_found", "Terminal session not found", "")
		return
	}
	if item.InputConnection != nil {
		s.terminalMu.Unlock()
		s.writeProblem(w, r, 429, "terminal_stream_limit", "Terminal input is busy", "")
		return
	}
	if !item.InputClaimed && input.Frames[0].Seq != 0 {
		s.terminalMu.Unlock()
		s.writeProblem(w, r, 409, "terminal_input_sequence", "Terminal input writer must be claimed", "")
		return
	}
	item.InputConnection = connection
	s.terminalSessions[id] = item
	s.terminalMu.Unlock()
	defer func() {
		s.terminalMu.Lock()
		if current, exists := s.terminalSessions[id]; exists && current.InputConnection == connection {
			current.InputConnection = nil
			s.terminalSessions[id] = current
		}
		s.terminalMu.Unlock()
	}()
	valid := func() bool {
		if ctx.Err() != nil {
			return false
		}
		if _, err := s.auth.Authenticate(token); err != nil || !time.Now().Before(session.ExpiresAt) {
			return false
		}
		s.terminalMu.Lock()
		defer s.terminalMu.Unlock()
		current, exists := s.terminalSessions[id]
		return exists && current.UserID == session.User.ID && !current.CloseRequested && current.InputConnection == connection
	}
	failure := func(err error) {
		switch {
		case errors.Is(err, terminal.ErrInputSequence):
			s.writeProblem(w, r, 409, "terminal_input_sequence", "Terminal input sequence is invalid", "")
		case errors.Is(err, terminal.ErrInputUncertain):
			s.writeProblem(w, r, 409, "terminal_input_uncertain", "Terminal input is uncertain; open a new terminal", "")
		case errors.Is(err, terminal.ErrClosed), errors.Is(err, terminal.ErrNotFound):
			s.writeProblem(w, r, 404, "terminal_not_found", "Terminal session not found", "")
		default:
			s.writeProblem(w, r, 503, "terminal_input_unavailable", "Terminal input temporarily unavailable", "")
		}
	}
	waiters := make([]func() error, 0, len(input.Frames))
	for _, frame := range input.Frames {
		if !valid() {
			s.writeProblem(w, r, 401, "session_expired", "Session expired", "")
			return
		}
		if item.HostID == cluster.LocalHostID {
			if err := (clusterTerminalSource{agent: s.agent}).InputSequenced(ctx, item.Owner, item.BackendSessionID, frame); err != nil {
				failure(err)
				return
			}
		} else {
			wait, err := s.cluster.BeginTerminalInput(ctx, item.HostID, item.BackendSessionID, frame)
			if err != nil {
				failure(err)
				return
			}
			waiters = append(waiters, wait)
		}
	}
	for _, wait := range waiters {
		if err := wait(); err != nil {
			failure(err)
			return
		}
	}
	if !valid() {
		s.writeProblem(w, r, 401, "session_expired", "Session expired", "")
		return
	}
	s.terminalMu.Lock()
	if current, exists := s.terminalSessions[id]; exists && current.InputConnection == connection {
		current.InputClaimed = true
		current.UpdatedAt = time.Now().UTC()
		s.terminalSessions[id] = current
	}
	s.terminalMu.Unlock()
	s.writeJSON(w, 200, map[string]uint64{"acked": input.Frames[len(input.Frames)-1].Seq})
}
