package cluster

import (
	"context"
	"encoding/json"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func (s *Service) TerminalSupportsSequencedInput(hostID, sessionID string) bool {
	t := s.streams.terminal(hostID, sessionID)
	return t != nil && t.reliable
}

// BeginTerminalInput returns only after writing the frame to the authenticated
// stream, without waiting for its ACK. Callers dispatch in order and bound the
// number of outstanding waiters. No unsequenced fallback is ever attempted.
func (s *Service) BeginTerminalInput(ctx context.Context, hostID, sessionID string, frame terminal.InputFrame) (func() error, error) {
	if !frame.Valid() {
		return nil, terminal.ErrInputSequence
	}
	t := s.streams.terminal(hostID, sessionID)
	if t == nil || !t.reliable {
		return nil, ErrTerminalUnavailable
	}
	payload, _ := json.Marshal(frame)
	return t.beginRequest(ctx, termInputSequenced, payload)
}
