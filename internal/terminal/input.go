package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const InputProtocol = "terminal-input-v1"
const InputWindow = 32
const InputFrameBytes = 2048

var ErrInputSequence = errors.New("terminal input sequence is invalid")
var ErrInputUncertain = errors.New("terminal input write is uncertain; open a new terminal")

// InputFrame identifies bytes independently of any browser or Noise socket.
// The PTY owner retains the last window's hashes for the entire PTY lifetime.
type InputFrame struct {
	Stream string `json:"stream"`
	Seq    uint64 `json:"seq"`
	Data   []byte `json:"data"`
}

type reliableInputState struct {
	stream string
	acked  uint64
	failed bool
	hashes [InputWindow][32]byte
}

func (f InputFrame) Valid() bool {
	decoded, err := hex.DecodeString(f.Stream)
	return err == nil && len(decoded) == 16 && ((f.Seq == 0 && len(f.Data) == 0) || (f.Seq > 0 && f.Seq <= 1<<53-1 && len(f.Data) > 0 && len(f.Data) <= InputFrameBytes))
}

// InputSequenced serializes admission and the actual PTY write under the same
// lock. A partial/failed write permanently freezes this input stream: replaying
// that frame could otherwise execute bytes twice. Success means all bytes were
// written, not merely queued. Closed sessions never acknowledge old input.
func (m *Manager) InputSequenced(owner, id string, frame InputFrame) error {
	return m.InputSequencedContext(context.Background(), owner, id, frame)
}

func (m *Manager) InputSequencedContext(ctx context.Context, owner, id string, frame InputFrame) error {
	if !frame.Valid() {
		return ErrInputSequence
	}
	item, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	item.inputMu.Lock()
	defer item.inputMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed || item.exitedAt != nil || item.closeFailed {
		return ErrClosed
	}
	state := &item.inputState
	if state.failed {
		return ErrInputUncertain
	}
	if state.stream != "" && state.stream != frame.Stream {
		return ErrInputSequence
	}
	// seq=0 is an authenticated writer claim, never PTY input. Reattaching
	// the same writer is idempotent, including before its first data frame.
	if frame.Seq == 0 {
		state.stream = frame.Stream
		return nil
	}
	hash := sha256.Sum256(frame.Data)
	if frame.Seq <= state.acked {
		if state.acked-frame.Seq >= InputWindow || state.hashes[frame.Seq%InputWindow] != hash {
			return ErrInputSequence
		}
		return nil
	}
	if frame.Seq != state.acked+1 {
		return ErrInputSequence
	}
	state.stream = frame.Stream
	// Capture/output and Close must remain able to run while a PTY write is
	// blocked by the child. inputMu alone preserves input order and dedup state.
	item.mu.Unlock()
	n, err := item.process.Write(frame.Data)
	item.mu.Lock()
	if err != nil || n != len(frame.Data) {
		state.failed = true
		return ErrInputUncertain
	}
	state.hashes[frame.Seq%InputWindow] = hash
	state.acked = frame.Seq
	item.updatedAt = m.config.Now().UTC()
	return nil
}
