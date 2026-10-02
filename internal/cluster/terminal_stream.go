package cluster

// The terminal stream carries one PTY session over an already authenticated
// Noise stream socket. Output is pushed as offset-tagged records, so a
// keystroke echo costs one network traversal instead of a fresh handshake per
// poll. Input, resize and close are acknowledged, preserving the existing
// "accepted means written to the PTY" contract.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	termOpen           = byte(30)
	termAttach         = byte(31)
	termReady          = byte(32)
	termOutput         = byte(33)
	termState          = byte(34)
	termInput          = byte(35)
	termResize         = byte(36)
	termClose          = byte(37)
	termReply          = byte(38)
	termReject         = byte(39)
	termInputProbe     = byte(40)
	termInputSequenced = byte(41)

	terminalStreamCoalesce      = 3 * time.Millisecond
	terminalStreamCoalesceBytes = 32 << 10
	terminalStreamPing          = 15 * time.Second
	terminalStreamReplyTimeout  = 10 * time.Second
	terminalStreamReconnect     = 2 * time.Minute
	terminalStreamRecordBytes   = fileStreamChunkBytes - 8
	terminalStreamReadyTimeout  = 15 * time.Second
)

var errTerminalStreamDisconnected = errors.New("terminal stream disconnected")

// Optional so legacy backends and peers retain their unchanged contract.
type SequencedTerminalBackend interface {
	SupportsSequencedInput(context.Context) bool
	InputSequenced(context.Context, string, string, terminal.InputFrame) error
}

func validTerminalInputProtocol(ctx context.Context, backend TerminalBackend, protocol string) bool {
	b, ok := backend.(SequencedTerminalBackend)
	return ok && protocol == terminal.InputProtocol && b.SupportsSequencedInput(ctx)
}

// terminalStreamFallbackAfter bounds how long a Panel terminal retries the
// stream before continuing the same session over v2 requests.
var terminalStreamFallbackAfter = 6 * time.Second

type terminalStreamOpen struct {
	Rows          uint16 `json:"rows"`
	Columns       uint16 `json:"columns"`
	InputProtocol string `json:"inputProtocol,omitempty"`
}

type terminalStreamAttach struct {
	SessionID     string `json:"sessionId"`
	Offset        int64  `json:"offset"`
	InputProtocol string `json:"inputProtocol,omitempty"`
}

type terminalStreamReady struct {
	SessionID     string    `json:"sessionId"`
	Offset        int64     `json:"offset"`
	CreatedAt     time.Time `json:"createdAt"`
	InputProtocol string    `json:"inputProtocol,omitempty"`
}

type terminalStreamState struct {
	ExitedAt  *time.Time `json:"exitedAt,omitempty"`
	ExitError string     `json:"exitError,omitempty"`
	Closed    bool       `json:"closed"`
}

type terminalStreamResult struct {
	Code string `json:"code,omitempty"`
}

func terminalStreamCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, terminal.ErrNotFound):
		return "terminal_not_found"
	case errors.Is(err, terminal.ErrClosed):
		return "terminal_closed"
	case errors.Is(err, terminal.ErrLimit):
		return "terminal_limit"
	case errors.Is(err, terminal.ErrInputSequence):
		return "terminal_input_sequence"
	case errors.Is(err, terminal.ErrInputUncertain):
		return "terminal_input_uncertain"
	default:
		return "terminal_failed"
	}
}

func terminalStreamError(code string) error {
	switch code {
	case "":
		return nil
	case "terminal_not_found":
		return terminal.ErrNotFound
	case "terminal_closed":
		return terminal.ErrClosed
	case "terminal_limit":
		return terminal.ErrLimit
	case "terminal_input_sequence":
		return terminal.ErrInputSequence
	case "terminal_input_uncertain":
		return terminal.ErrInputUncertain
	default:
		return ErrTerminalUnavailable
	}
}

func sequenced(seq uint64, payload []byte) []byte {
	record := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint64(record, seq)
	copy(record[8:], payload)
	return record
}

func splitSequenced(data []byte) (uint64, []byte, bool) {
	if len(data) < 8 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint64(data), data[8:], true
}

// terminalAttachments closes PTYs that no center has re-attached within a
// grace period. Light nodes use it so a restarted center cannot leave root
// shells running until the 30 minute idle reaper.
type terminalAttachments struct {
	mu     sync.Mutex
	grace  time.Duration
	count  map[string]int
	timers map[string]*time.Timer
	close  func(string)
}

func newTerminalAttachments(grace time.Duration, closeFn func(string)) *terminalAttachments {
	return &terminalAttachments{grace: grace, count: make(map[string]int), timers: make(map[string]*time.Timer), close: closeFn}
}

func (a *terminalAttachments) attach(id string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.count[id]++
	if timer := a.timers[id]; timer != nil {
		timer.Stop()
		delete(a.timers, id)
	}
}

func (a *terminalAttachments) detach(id string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.count[id] > 1 {
		a.count[id]--
		return
	}
	delete(a.count, id)
	a.timers[id] = time.AfterFunc(a.grace, func() {
		a.mu.Lock()
		if a.count[id] > 0 {
			a.mu.Unlock()
			return
		}
		delete(a.timers, id)
		a.mu.Unlock()
		a.close(id)
	})
}

// serveTerminalStream runs on the PTY owner (target Panel or light node). It
// returns the session ID it served, empty when opening or attaching failed.
func serveTerminalStream(c *fileStreamConn, backend TerminalBackend, owner string, attachments *terminalAttachments) string {
	defer c.close()
	kind, payload, err := c.readControl()
	if err != nil || backend == nil {
		return ""
	}
	// Probe precedes any PTY creation. Old targets close this fresh socket;
	// retrying an ordinary open is safe because no open was sent on it.
	if kind == termInputProbe {
		if string(payload) != "{}" {
			return ""
		}
		protocol := ""
		if reliable, ok := backend.(SequencedTerminalBackend); ok && reliable.SupportsSequencedInput(c.ctx) {
			protocol = terminal.InputProtocol
		}
		if c.writeJSON(termInputProbe, terminalStreamReady{InputProtocol: protocol}) != nil {
			return ""
		}
		kind, payload, err = c.readControl()
		if err != nil {
			return ""
		}
	}
	var sessionID string
	var inputProtocol string
	var offset int64
	var createdAt time.Time
	switch kind {
	case termOpen:
		var input terminalStreamOpen
		if decodeV2Payload(payload, &input) != nil || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
			return ""
		}
		inputProtocol = input.InputProtocol
		if inputProtocol != "" && !validTerminalInputProtocol(c.ctx, backend, inputProtocol) {
			return ""
		}
		snapshot, openErr := backend.Open(c.ctx, owner, input.Rows, input.Columns)
		if openErr != nil {
			_ = c.writeJSON(termReject, terminalStreamResult{Code: terminalStreamCode(openErr)})
			return ""
		}
		sessionID, offset, createdAt = snapshot.ID, snapshot.Offset, snapshot.CreatedAt
	case termAttach:
		var input terminalStreamAttach
		if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || len(input.SessionID) > 128 || input.Offset < 0 {
			return ""
		}
		inputProtocol = input.InputProtocol
		if inputProtocol != "" && !validTerminalInputProtocol(c.ctx, backend, inputProtocol) {
			return ""
		}
		if _, probeErr := backend.Output(c.ctx, owner, input.SessionID, input.Offset, 0); probeErr != nil {
			_ = c.writeJSON(termReject, terminalStreamResult{Code: terminalStreamCode(probeErr)})
			return ""
		}
		sessionID, offset, createdAt = input.SessionID, input.Offset, time.Now().UTC()
	default:
		return ""
	}
	if c.writeJSON(termReady, terminalStreamReady{SessionID: sessionID, Offset: offset, CreatedAt: createdAt, InputProtocol: inputProtocol}) != nil {
		return ""
	}
	attachments.attach(sessionID)
	defer attachments.detach(sessionID)
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		pumpTerminalStream(c, backend, owner, sessionID, offset)
	}()
	defer func() { c.close(); <-pumpDone }()
	// Input can block on PTY back-pressure. Keep the authenticated control
	// reader available for close/ping/resize and serialize only the PTY writes.
	type inputJob struct {
		seq   uint64
		frame *terminal.InputFrame
		raw   []byte
	}
	inputCtx, stopInput := context.WithCancel(c.ctx)
	defer stopInput()
	jobs := make(chan inputJob, terminal.InputWindow)
	inputSlots := make(chan struct{}, terminal.InputWindow)
	go func() {
		for {
			select {
			case <-inputCtx.Done():
				return
			case job := <-jobs:
				if inputCtx.Err() != nil {
					return
				}
				var err error
				if job.frame != nil {
					err = backend.(SequencedTerminalBackend).InputSequenced(inputCtx, owner, sessionID, *job.frame)
				} else {
					err = backend.Input(inputCtx, owner, sessionID, job.raw)
				}
				replyTerminalStream(c, job.seq, err)
				<-inputSlots
			}
		}
	}()
	enqueue := func(job inputJob) {
		select {
		case inputSlots <- struct{}{}:
			jobs <- job
		default:
			replyTerminalStream(c, job.seq, terminal.ErrLimit)
		}
	}
	for {
		kind, data, err := c.read()
		if err != nil {
			return sessionID
		}
		switch kind {
		case termInputSequenced:
			seq, body, ok := splitSequenced(data)
			var frame terminal.InputFrame
			if !ok || inputProtocol != terminal.InputProtocol || decodeV2Payload(body, &frame) != nil || !frame.Valid() {
				return sessionID
			}
			enqueue(inputJob{seq: seq, frame: &frame})
		case streamPing:
			if len(data) != 0 || c.write(streamPong, nil) != nil {
				return sessionID
			}
		case streamPong:
		case termInput:
			seq, input, ok := splitSequenced(data)
			if !ok || len(input) == 0 || len(input) > terminal.MaxInputBytes {
				return sessionID
			}
			enqueue(inputJob{seq: seq, raw: input})
		case termResize:
			seq, body, ok := splitSequenced(data)
			var input terminalStreamOpen
			if !ok || decodeV2Payload(body, &input) != nil || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
				return sessionID
			}
			replyTerminalStream(c, seq, backend.Resize(c.ctx, owner, sessionID, input.Rows, input.Columns))
		case termClose:
			seq, body, ok := splitSequenced(data)
			if !ok || len(body) != 0 {
				return sessionID
			}
			closeErr := backend.Close(c.ctx, owner, sessionID)
			if errors.Is(closeErr, terminal.ErrNotFound) || errors.Is(closeErr, terminal.ErrClosed) {
				closeErr = nil
			}
			if closeErr == nil {
				stopInput()
			}
			replyTerminalStream(c, seq, closeErr)
		default:
			return sessionID
		}
	}
}

func replyTerminalStream(c *fileStreamConn, seq uint64, err error) {
	payload, _ := json.Marshal(terminalStreamResult{Code: terminalStreamCode(err)})
	_ = c.write(termReply, sequenced(seq, payload))
}

func pumpTerminalStream(c *fileStreamConn, backend TerminalBackend, owner, sessionID string, offset int64) {
	var sent terminalStreamState
	failures := 0
	for c.ctx.Err() == nil {
		output, err := backend.Output(c.ctx, owner, sessionID, offset, time.Second)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrOffset) {
				_ = c.writeJSON(termState, terminalStreamState{Closed: true})
				return
			}
			failures++
			if failures > 20 {
				c.close()
				return
			}
			timer := time.NewTimer(time.Duration(min(failures, 10)) * 100 * time.Millisecond)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		failures = 0
		finished := output.Closed || output.ExitedAt != nil
		data, next := output.Data, output.NextOffset
		if len(data) > 0 && len(data) < terminalStreamCoalesceBytes && !finished {
			timer := time.NewTimer(terminalStreamCoalesce)
			select {
			case <-c.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if more, moreErr := backend.Output(c.ctx, owner, sessionID, next, 0); moreErr == nil && more.Offset == next && !more.Truncated {
				data = append(data, more.Data...)
				next = more.NextOffset
				output.ExitedAt, output.ExitError, output.Closed = more.ExitedAt, more.ExitError, more.Closed
				finished = output.Closed || output.ExitedAt != nil
			}
		}
		recordOffset := output.Offset
		emitted := len(data)
		for len(data) > 0 {
			size := min(len(data), terminalStreamRecordBytes)
			if c.write(termOutput, sequenced(uint64(recordOffset), data[:size])) != nil {
				return
			}
			recordOffset += int64(size)
			data = data[size:]
		}
		offset = next
		state := terminalStreamState{ExitedAt: output.ExitedAt, ExitError: output.ExitError, Closed: output.Closed}
		if (state.ExitedAt != nil && sent.ExitedAt == nil) || (state.Closed && !sent.Closed) {
			if c.writeJSON(termState, state) != nil {
				return
			}
			sent = state
		}
		if finished && emitted == 0 {
			return
		}
	}
}

// streamTerminalDialer opens a fresh authenticated socket and sends the given
// first record (open or attach). It returns the ready payload.
type streamTerminalDialer func(ctx context.Context, first byte, payload any) (*fileStreamConn, terminalStreamReady, error)

// streamTerminalFallback continues an established session over the v2 POST
// transport when the stream cannot be re-established. Both transports address
// the same target PTY owner, so the session survives the switch.
type streamTerminalFallback interface {
	Output(context.Context, TerminalOutputRequest) (terminal.Output, error)
	Input(context.Context, TerminalInputRequest) error
	Resize(context.Context, TerminalResizeRequest) error
	Close(context.Context, TerminalCloseRequest) error
}

type streamTerminal struct {
	sendMu    sync.Mutex
	reliable  bool
	hostID    string
	sessionID string
	buffer    *terminal.Buffer
	dial      streamTerminalDialer
	fallback  streamTerminalFallback
	ctx       context.Context
	cancel    context.CancelFunc
	seq       atomic.Uint64

	mu       sync.Mutex
	conn     *fileStreamConn
	pending  map[uint64]chan string
	degraded bool
	closing  bool
	lastUsed time.Time
}

// openStreamTerminal opens the session within the caller's ctx; the stream
// then lives under parent until closed.
func openStreamTerminal(ctx, parent context.Context, hostID string, dial streamTerminalDialer, fallback streamTerminalFallback, rows, columns uint16) (*streamTerminal, TerminalOpenResponse, error) {
	probe, capability, probeErr := dial(ctx, termInputProbe, struct{}{})
	var conn *fileStreamConn
	var ready terminalStreamReady
	var err error
	if probeErr == nil {
		conn, ready, err = startTerminalStream(ctx, probe, termOpen, terminalStreamOpen{Rows: rows, Columns: columns, InputProtocol: capability.InputProtocol})
	} else {
		conn, ready, err = dial(ctx, termOpen, terminalStreamOpen{Rows: rows, Columns: columns})
	}
	if err != nil {
		return nil, TerminalOpenResponse{}, err
	}
	ctx, cancel := context.WithCancel(parent)
	t := &streamTerminal{hostID: hostID, sessionID: ready.SessionID, buffer: terminal.NewBuffer(terminal.DefaultBufferBytes, ready.Offset),
		dial: dial, fallback: fallback, ctx: ctx, cancel: cancel, pending: make(map[uint64]chan string), lastUsed: time.Now(), reliable: ready.InputProtocol == terminal.InputProtocol}
	t.attach(conn)
	return t, TerminalOpenResponse{SessionID: ready.SessionID, Offset: ready.Offset, CreatedAt: ready.CreatedAt}, nil
}

func (t *streamTerminal) attach(conn *fileStreamConn) {
	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()
	t.buffer.Recover()
	go t.read(conn)
	go t.ping(conn)
}

func (t *streamTerminal) ping(conn *fileStreamConn) {
	ticker := time.NewTicker(terminalStreamPing)
	defer ticker.Stop()
	for {
		select {
		case <-conn.ctx.Done():
			return
		case <-ticker.C:
			if conn.write(streamPing, nil) != nil {
				return
			}
		}
	}
}

func (t *streamTerminal) read(conn *fileStreamConn) {
	for {
		kind, data, err := conn.read()
		if err != nil {
			t.disconnected(conn)
			return
		}
		switch kind {
		case termOutput:
			offset, payload, ok := splitSequenced(data)
			if !ok || len(payload) == 0 || offset > 1<<62 {
				conn.close()
				continue
			}
			t.buffer.Append(int64(offset), payload)
		case termState:
			var state terminalStreamState
			if decodeV2Payload(data, &state) != nil {
				conn.close()
				continue
			}
			t.buffer.SetState(state.ExitedAt, state.ExitError, state.Closed)
		case termReply:
			seq, payload, ok := splitSequenced(data)
			var result terminalStreamResult
			if !ok || decodeV2Payload(payload, &result) != nil {
				conn.close()
				continue
			}
			t.mu.Lock()
			reply := t.pending[seq]
			delete(t.pending, seq)
			t.mu.Unlock()
			if reply != nil {
				reply <- result.Code
			}
		case streamPing:
			_ = conn.write(streamPong, nil)
		case streamPong:
		default:
			conn.close()
		}
	}
}

func (t *streamTerminal) disconnected(conn *fileStreamConn) {
	t.mu.Lock()
	if t.conn != conn {
		t.mu.Unlock()
		return
	}
	t.conn = nil
	for seq, reply := range t.pending {
		delete(t.pending, seq)
		close(reply)
	}
	closing := t.closing
	t.mu.Unlock()
	if closing || t.ctx.Err() != nil || t.buffer.Finished() {
		return
	}
	t.buffer.Fail(errTerminalStreamDisconnected)
	go t.reconnect()
}

func (t *streamTerminal) reconnect() {
	window := terminalStreamReconnect
	if t.fallback != nil && !t.reliable {
		window = terminalStreamFallbackAfter
	}
	deadline := time.Now().Add(window)
	delay := 250 * time.Millisecond
	for t.ctx.Err() == nil && time.Now().Before(deadline) {
		protocol := ""
		if t.reliable {
			protocol = terminal.InputProtocol
		}
		conn, ready, err := t.dial(t.ctx, termAttach, terminalStreamAttach{SessionID: t.sessionID, Offset: t.buffer.Next(), InputProtocol: protocol})
		if err == nil && ready.InputProtocol != protocol {
			conn.close()
			err = ErrTerminalUnavailable
		}
		if err == nil {
			t.mu.Lock()
			if t.closing || t.ctx.Err() != nil {
				t.mu.Unlock()
				conn.close()
				return
			}
			t.mu.Unlock()
			t.attach(conn)
			return
		}
		if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
			t.buffer.SetState(nil, "", true)
			t.buffer.Recover()
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-t.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 4*time.Second)
	}
	if t.fallback != nil && !t.reliable && t.ctx.Err() == nil {
		t.mu.Lock()
		t.degraded = true
		t.mu.Unlock()
		t.buffer.Recover()
	}
}

func (t *streamTerminal) touch() {
	t.mu.Lock()
	t.lastUsed = time.Now()
	t.mu.Unlock()
}

func (t *streamTerminal) idleSince() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastUsed
}

func (t *streamTerminal) isDegraded() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.degraded
}

func (t *streamTerminal) output(ctx context.Context, input TerminalOutputRequest) (terminal.Output, error) {
	t.touch()
	if t.isDegraded() {
		return t.fallback.Output(ctx, input)
	}
	output, err := t.buffer.Output(ctx, input.Offset, time.Duration(input.Wait)*time.Millisecond)
	if errors.Is(err, errTerminalStreamDisconnected) {
		if t.isDegraded() {
			return t.fallback.Output(ctx, input)
		}
		return terminal.Output{}, ErrTerminalUnavailable
	}
	return output, err
}

func (t *streamTerminal) request(ctx context.Context, kind byte, payload []byte) error {
	wait, err := t.beginRequest(ctx, kind, payload)
	if err != nil {
		return err
	}
	return wait()
}

// Admission and socket writes are synchronous and ordered; waiting for the
// owner's ACK is separate so the next frame can cross the network immediately.
func (t *streamTerminal) beginRequest(ctx context.Context, kind byte, payload []byte) (func() error, error) {
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.touch()
	t.mu.Lock()
	conn := t.conn
	limit := terminal.InputWindow + 4
	if kind == termInputSequenced {
		limit = terminal.InputWindow
	}
	if conn == nil || t.closing || len(t.pending) >= limit {
		t.mu.Unlock()
		return nil, ErrTerminalUnavailable
	}
	seq := t.seq.Add(1)
	reply := make(chan string, 1)
	t.pending[seq] = reply
	t.mu.Unlock()
	// Cancellation can win while the peer keeps the connection alive without
	// replying. Release the waiter on every exit, not just timeout/disconnect.
	cleanup := func() {
		t.mu.Lock()
		delete(t.pending, seq)
		t.mu.Unlock()
	}
	stopCleanup := context.AfterFunc(ctx, cleanup)
	if err := ctx.Err(); err != nil {
		stopCleanup()
		cleanup()
		return nil, err
	}
	if err := conn.write(kind, sequenced(seq, payload)); err != nil {
		stopCleanup()
		cleanup()
		return nil, ErrTerminalUnavailable
	}
	return func() error {
		defer stopCleanup()
		defer cleanup()
		timer := time.NewTimer(terminalStreamReplyTimeout)
		defer timer.Stop()
		select {
		case code, ok := <-reply:
			if !ok {
				return ErrTerminalUnavailable
			}
			return terminalStreamError(code)
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return ErrTerminalUnavailable
		}
	}, nil
}

func (t *streamTerminal) input(ctx context.Context, input TerminalInputRequest) error {
	if t.isDegraded() {
		return t.fallback.Input(ctx, input)
	}
	data, err := decodeTerminalPayload(input.Data)
	if err != nil || len(data) == 0 || len(data) > terminal.MaxInputBytes {
		return ErrAuthentication
	}
	return t.request(ctx, termInput, data)
}

func (t *streamTerminal) resize(ctx context.Context, input TerminalResizeRequest) error {
	if t.isDegraded() {
		return t.fallback.Resize(ctx, input)
	}
	if input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
		return ErrAuthentication
	}
	payload, _ := json.Marshal(terminalStreamOpen{Rows: input.Rows, Columns: input.Columns})
	return t.request(ctx, termResize, payload)
}

// close confirms remote termination before releasing the local handle. A
// failed confirmation keeps the handle so the caller may retry.
func (t *streamTerminal) close(ctx context.Context, input TerminalCloseRequest) error {
	var err error
	if t.isDegraded() {
		err = t.fallback.Close(ctx, input)
	} else {
		err = t.request(ctx, termClose, nil)
		if errors.Is(err, ErrTerminalUnavailable) && t.fallback != nil {
			err = t.fallback.Close(ctx, input)
		}
	}
	if err != nil && !errors.Is(err, terminal.ErrNotFound) && !errors.Is(err, terminal.ErrClosed) {
		return err
	}
	t.shutdown()
	return nil
}

func (t *streamTerminal) shutdown() {
	t.mu.Lock()
	t.closing = true
	conn := t.conn
	t.conn = nil
	t.mu.Unlock()
	t.cancel()
	if conn != nil {
		conn.close()
	}
	t.buffer.SetState(nil, "", true)
}
