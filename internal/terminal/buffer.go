package terminal

import (
	"context"
	"sync"
	"time"
)

// Buffer mirrors a remote PTY's output with the same offset, truncation and
// exit semantics as a local session. Streaming transports append pushed
// records to it, while browsers keep reading through Output unchanged.
type Buffer struct {
	mu        sync.Mutex
	limit     int
	buffer    []byte
	base      int64
	next      int64
	notify    chan struct{}
	exitedAt  *time.Time
	exitError string
	closed    bool
	failed    error
}

func NewBuffer(limit int, offset int64) *Buffer {
	if limit <= 0 {
		limit = DefaultBufferBytes
	}
	if offset < 0 {
		offset = 0
	}
	return &Buffer{limit: limit, base: offset, next: offset, notify: make(chan struct{})}
}

// Append accepts an ordered record. Overlapping bytes from a re-attached stream
// are skipped; a gap (the remote ring already dropped data) moves the base so
// readers behind it observe truncation instead of silently missing output.
func (b *Buffer) Append(offset int64, data []byte) bool {
	if offset < 0 || len(data) == 0 {
		return offset >= 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	end := offset + int64(len(data))
	if end <= b.next {
		return true
	}
	if offset > b.next {
		b.buffer = b.buffer[:0]
		b.base = offset
		b.next = offset
	}
	if offset < b.next {
		data = data[b.next-offset:]
	}
	b.buffer = append(b.buffer, data...)
	b.next += int64(len(data))
	if len(b.buffer) > b.limit {
		drop := len(b.buffer) - b.limit
		copy(b.buffer, b.buffer[drop:])
		b.buffer = b.buffer[:b.limit]
		b.base += int64(drop)
	}
	b.wakeLocked()
	return true
}

func (b *Buffer) SetState(exitedAt *time.Time, exitError string, closed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := false
	if exitedAt != nil && b.exitedAt == nil {
		value := exitedAt.UTC()
		b.exitedAt = &value
		b.exitError = exitError
		changed = true
	}
	if closed && !b.closed {
		b.closed = true
		changed = true
	}
	if changed {
		b.wakeLocked()
	}
}

// Fail wakes readers with a transport error; the retained output stays
// readable after a later Recover.
func (b *Buffer) Fail(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failed == nil && err != nil {
		b.failed = err
		b.wakeLocked()
	}
}

func (b *Buffer) Recover() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failed != nil {
		b.failed = nil
		b.wakeLocked()
	}
}

func (b *Buffer) Next() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.next
}

func (b *Buffer) Finished() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed || b.exitedAt != nil
}

func (b *Buffer) Output(ctx context.Context, offset int64, wait time.Duration) (Output, error) {
	if wait < 0 || wait > 1500*time.Millisecond {
		return Output{}, ErrOffset
	}
	for {
		output, notify, ready, err := b.read(offset)
		if err != nil || ready || wait == 0 {
			return output, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Output{}, ctx.Err()
		case <-notify:
			timer.Stop()
		case <-timer.C:
			output, _, _, err = b.read(offset)
			return output, err
		}
	}
}

func (b *Buffer) read(offset int64) (Output, <-chan struct{}, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if offset < 0 || offset > b.next {
		return Output{}, nil, false, ErrOffset
	}
	truncated := offset < b.base
	if truncated {
		offset = b.base
	}
	start := int(offset - b.base)
	available := min(len(b.buffer)-start, MaxOutputBytes)
	data := append([]byte(nil), b.buffer[start:start+available]...)
	result := Output{Data: data, Offset: offset, NextOffset: offset + int64(len(data)),
		Truncated: truncated, ExitedAt: b.exitedAt, ExitError: b.exitError, Closed: b.closed}
	if len(data) == 0 && b.failed != nil && !b.closed && b.exitedAt == nil {
		return Output{}, b.notify, true, b.failed
	}
	return result, b.notify, len(data) > 0 || truncated || b.closed || b.exitedAt != nil, nil
}

func (b *Buffer) wakeLocked() {
	close(b.notify)
	b.notify = make(chan struct{})
}
