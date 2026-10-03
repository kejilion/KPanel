package terminal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

// ErrInputNotWritten is returned by an InputSequencer write callback when the
// failure happened before any byte reached the sink. Offering the same frame
// again is safe, so the sequencer keeps its state instead of freezing it.
var ErrInputNotWritten = errors.New("terminal input was not written")

// InputSequencer gives sinks that are not PTY sessions (task terminals feed a
// FIFO read by a separate job process) the same frame contract as
// Manager.InputSequenced: ordered admission, an acknowledgement only after the
// bytes were fully written, replay of the last InputWindow frames answered from
// their hashes, and a permanent freeze once a write may have been partial.
//
// Two differences follow from that sink. A claim from a new stream fences the
// previous writer instead of being refused, because a task outlives the page
// that opened it and a reloaded page must be able to continue. And a failure
// reported as ErrInputNotWritten leaves the stream usable. Its Epoch identifies
// this in-memory state, so a client can tell it was lost to a restart rather
// than replay frames the previous process may already have written.
type InputSequencer struct {
	gate  chan struct{}
	epoch string
	mu    sync.Mutex
	state reliableInputState
}

func NewInputSequencer() *InputSequencer {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic("terminal input epoch: " + err.Error())
	}
	return &InputSequencer{gate: make(chan struct{}, 1), epoch: hex.EncodeToString(id[:])}
}

func (q *InputSequencer) Epoch() string { return q.epoch }

// Claimed reports whether a stream owns the sink. Bare writes must then be
// refused so that every byte keeps flowing through the acknowledged stream.
func (q *InputSequencer) Claimed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state.stream != ""
}

// Admit applies one frame. write receives the bytes only for a new in-order
// frame and is serialized with every other Admit call. ErrClosed and
// ErrNotFound from write (the task finished) are returned without freezing.
func (q *InputSequencer) Admit(ctx context.Context, frame InputFrame, write func([]byte) error) error {
	if !frame.Valid() {
		return ErrInputSequence
	}
	select {
	case q.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-q.gate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	if frame.Seq == 0 {
		if q.state.stream != frame.Stream {
			q.state = reliableInputState{stream: frame.Stream}
		}
		failed := q.state.failed
		q.mu.Unlock()
		if failed {
			return ErrInputUncertain
		}
		return nil
	}
	state := &q.state
	if state.stream != frame.Stream {
		q.mu.Unlock()
		return ErrInputSequence
	}
	if state.failed {
		q.mu.Unlock()
		return ErrInputUncertain
	}
	hash := sha256.Sum256(frame.Data)
	if frame.Seq <= state.acked {
		replay := state.acked-frame.Seq < InputWindow && state.hashes[frame.Seq%InputWindow] == hash
		q.mu.Unlock()
		if !replay {
			return ErrInputSequence
		}
		return nil
	}
	if frame.Seq != state.acked+1 {
		q.mu.Unlock()
		return ErrInputSequence
	}
	q.mu.Unlock()

	err := write(frame.Data)

	q.mu.Lock()
	defer q.mu.Unlock()
	switch {
	case err == nil:
		state.hashes[frame.Seq%InputWindow] = hash
		state.acked = frame.Seq
		return nil
	case errors.Is(err, ErrInputNotWritten), errors.Is(err, ErrClosed), errors.Is(err, ErrNotFound):
		return err
	default:
		state.failed = true
		return ErrInputUncertain
	}
}
