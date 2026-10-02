package panel

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type terminalInputBatchReply struct {
	Acked uint64 `json:"acked"`
	Epoch string `json:"epoch,omitempty"`
}

// A proxy may support HTTP/SSE but reject WebSocket upgrades. This fallback
// uses the SAME owner claim, sequence and replay window; it never replays bare
// POST input. One batch per terminal shares admission with its browser socket.
func (s *Server) handleTerminalInputBatch(w http.ResponseWriter, r *http.Request, token string, session auth.Session, id string) {
	s.serveTerminalInputBatch(w, r, token, session, s.hostTerminalInputTarget(session, id))
}

func (s *Server) serveTerminalInputBatch(w http.ResponseWriter, r *http.Request, token string, session auth.Session, target terminalInputTarget) {
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
	release, status, code, title := target.admit(connection, input.Frames[0].Seq != 0)
	if status != 0 {
		s.writeProblem(w, r, status, code, title, "")
		return
	}
	defer release()
	valid := func() bool {
		if ctx.Err() != nil {
			return false
		}
		if _, err := s.auth.Authenticate(token); err != nil || !time.Now().Before(session.ExpiresAt) {
			return false
		}
		return target.alive(connection)
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
	var epoch string
	waiters := make([]func() error, 0, len(input.Frames))
	for _, frame := range input.Frames {
		if !valid() {
			s.writeProblem(w, r, 401, "session_expired", "Session expired", "")
			return
		}
		if frame.Seq == 0 {
			claimed, err := target.claim(ctx, frame)
			if err != nil {
				failure(err)
				return
			}
			epoch = claimed
			continue
		}
		wait, err := target.send(ctx, frame)
		if err != nil {
			failure(err)
			return
		}
		if target.immediate() {
			if err := wait(); err != nil {
				failure(err)
				return
			}
			continue
		}
		waiters = append(waiters, wait)
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
	target.claimed(connection)
	s.writeJSON(w, 200, terminalInputBatchReply{Acked: input.Frames[len(input.Frames)-1].Seq, Epoch: epoch})
}
