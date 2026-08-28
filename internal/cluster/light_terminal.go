package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type lightTerminalRelay struct {
	mu    sync.RWMutex
	nodes map[string]*lightTerminalNode
	now   func() time.Time
	epoch string
}

type lightTerminalNode struct {
	mu         sync.Mutex
	id         string
	sessions   map[string]*lightTerminalSession
	queued     []*lightTerminalCommand
	pending    map[string]*lightTerminalCommand
	wake       chan struct{}
	lastPoll   time.Time
	resetEpoch string
	available  bool
	closed     bool
}

type lightTerminalCommand struct {
	command   TerminalRelayCommand
	done      chan error
	cancel    bool
	delivered bool
}

type lightTerminalSession struct {
	mu        sync.Mutex
	id        string
	buffer    []byte
	base      int64
	next      int64
	notify    chan struct{}
	createdAt time.Time
	updatedAt time.Time
	exitedAt  *time.Time
	exitError string
	closed    bool
}

func newLightTerminalRelay(now func() time.Time) *lightTerminalRelay {
	if now == nil {
		now = time.Now
	}
	epoch, err := randomHex(16)
	if err != nil {
		epoch = fmt.Sprintf("%032x", time.Now().UTC().UnixNano())
	}
	return &lightTerminalRelay{nodes: make(map[string]*lightTerminalNode), now: now, epoch: epoch}
}

func (r *lightTerminalRelay) node(id string, create bool) *lightTerminalNode {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.nodes[id]
	if item == nil && create {
		item = &lightTerminalNode{
			id:       id,
			sessions: make(map[string]*lightTerminalSession),
			pending:  make(map[string]*lightTerminalCommand),
			wake:     make(chan struct{}),
		}
		r.nodes[id] = item
	}
	return item
}

func (r *lightTerminalRelay) available(id string) bool {
	r.mu.RLock()
	item := r.nodes[id]
	r.mu.RUnlock()
	if item == nil {
		return false
	}
	now := r.now().UTC()
	item.mu.Lock()
	defer item.mu.Unlock()
	return item.available && !item.closed && !item.lastPoll.IsZero() && now.Sub(item.lastPoll) <= lightTerminalLiveness
}

func (r *lightTerminalRelay) deleteNode(id string) {
	r.mu.Lock()
	item := r.nodes[id]
	delete(r.nodes, id)
	r.mu.Unlock()
	if item == nil {
		return
	}
	item.mu.Lock()
	item.closed = true
	for _, command := range item.pending {
		completeLightTerminalCommand(command, ErrTerminalUnavailable)
	}
	item.pending = make(map[string]*lightTerminalCommand)
	item.queued = nil
	for _, session := range item.sessions {
		session.markClosed(r.now().UTC())
	}
	item.sessions = make(map[string]*lightTerminalSession)
	close(item.wake)
	item.wake = make(chan struct{})
	item.mu.Unlock()
}

func (r *lightTerminalRelay) closeAll() {
	r.mu.Lock()
	nodes := make([]*lightTerminalNode, 0, len(r.nodes))
	for _, item := range r.nodes {
		nodes = append(nodes, item)
	}
	r.nodes = make(map[string]*lightTerminalNode)
	r.mu.Unlock()
	for _, item := range nodes {
		item.mu.Lock()
		item.closed = true
		for _, command := range item.pending {
			completeLightTerminalCommand(command, terminal.ErrClosed)
		}
		item.pending = make(map[string]*lightTerminalCommand)
		item.queued = nil
		for _, session := range item.sessions {
			session.markClosed(r.now().UTC())
		}
		item.sessions = make(map[string]*lightTerminalSession)
		close(item.wake)
		item.wake = make(chan struct{})
		item.mu.Unlock()
	}
}

func (r *lightTerminalRelay) poll(
	ctx context.Context,
	nodeID string,
	sessionIDs []string,
	events []TerminalRelayEvent,
) (TerminalRelayPollResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	item := r.node(nodeID, true)
	for {
		now := r.now().UTC()
		item.mu.Lock()
		if item.closed {
			item.mu.Unlock()
			return TerminalRelayPollResponse{}, ErrTerminalUnavailable
		}
		item.available = true
		item.lastPoll = now
		item.applyEvents(events, now)
		item.reconcileSessions(sessionIDs, now)
		if command := item.takeCommand(now); command != nil {
			item.resetEpoch = r.epoch
			response := TerminalRelayPollResponse{Epoch: r.epoch, Command: &command.command}
			item.mu.Unlock()
			return response, nil
		}
		if r.epoch != "" && item.resetEpoch != r.epoch && len(sessionIDs) > 0 {
			item.resetEpoch = r.epoch
			response := TerminalRelayPollResponse{Epoch: r.epoch}
			item.mu.Unlock()
			return response, nil
		}
		notify := item.wake
		wait := lightTerminalPollWait
		if item.hasActiveSessions() {
			wait = lightTerminalActivePollWait
		}
		item.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			stopLightTerminalTimer(timer)
			return TerminalRelayPollResponse{}, ctx.Err()
		case <-notify:
			stopLightTerminalTimer(timer)
			// A command may have been queued after the wake channel was
			// captured. Loop once more while preserving the long-poll.
		case <-timer.C:
			item.mu.Lock()
			if !item.closed {
				item.resetEpoch = r.epoch
			}
			item.mu.Unlock()
			return TerminalRelayPollResponse{Epoch: r.epoch}, nil
		}
		if ctx.Err() != nil {
			return TerminalRelayPollResponse{}, ctx.Err()
		}
		events = nil
	}
}

func (item *lightTerminalNode) takeCommand(now time.Time) *lightTerminalCommand {
	for len(item.queued) > 0 {
		command := item.queued[0]
		item.queued = item.queued[1:]
		current, ok := item.pending[command.command.ID]
		if !ok || current != command || command.cancel {
			continue
		}
		if now.Unix() >= command.command.ExpiresAt {
			delete(item.pending, command.command.ID)
			completeLightTerminalCommand(command, ErrTerminalUnavailable)
			continue
		}
		command.delivered = true
		return command
	}
	for commandID, command := range item.pending {
		if command.cancel {
			continue
		}
		if now.Unix() >= command.command.ExpiresAt {
			delete(item.pending, commandID)
			completeLightTerminalCommand(command, ErrTerminalUnavailable)
			continue
		}
		if !command.delivered {
			continue
		}
		return command
	}
	return nil
}

func (item *lightTerminalNode) hasActiveSessions() bool {
	for _, session := range item.sessions {
		if session.isActive() {
			return true
		}
	}
	return false
}

func (item *lightTerminalNode) applyEvents(events []TerminalRelayEvent, now time.Time) {
	for _, event := range events {
		session := item.sessions[event.SessionID]
		if session == nil {
			continue
		}
		switch event.Kind {
		case "output":
			if event.Offset < session.currentNext() {
				continue
			}
			if err := session.appendOutput(event.Offset, event.NextOffset, event.Data, event.Truncated, event.ExitedAt != nil, event.ExitError, event.Closed, now); err != nil {
				session.markExit(err, now)
			}
		case "opened", "accepted", "closed", "error":
			item.finishCommand(event, session, now)
		}
	}
}

func (item *lightTerminalNode) reconcileSessions(sessionIDs []string, now time.Time) {
	keep := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		keep[sessionID] = struct{}{}
	}
	for sessionID, session := range item.sessions {
		if _, ok := keep[sessionID]; ok {
			continue
		}
		if item.hasPendingOpenSession(sessionID) {
			item.cancelPendingSessionCommands(sessionID, true)
			continue
		}
		session.markClosed(now)
		delete(item.sessions, sessionID)
		item.cancelPendingSessionCommands(sessionID, false)
	}
}

func (item *lightTerminalNode) hasPendingOpenSession(sessionID string) bool {
	for _, command := range item.pending {
		if command.command.Path == v2TerminalOpenPath && command.command.SessionID == sessionID && !command.cancel {
			return true
		}
	}
	return false
}

func (item *lightTerminalNode) cancelPendingSessionCommands(sessionID string, preserveOpen bool) {
	for commandID, command := range item.pending {
		if command.command.SessionID != sessionID || (preserveOpen && command.command.Path == v2TerminalOpenPath) {
			continue
		}
		command.cancel = true
		delete(item.pending, commandID)
		completeLightTerminalCommand(command, terminal.ErrClosed)
	}
}

func (item *lightTerminalNode) finishCommand(event TerminalRelayEvent, session *lightTerminalSession, now time.Time) {
	if event.CommandID == "" {
		return
	}
	command := item.pending[event.CommandID]
	if command == nil || command.command.SessionID != event.SessionID || command.cancel {
		return
	}
	validKind := (command.command.Path == v2TerminalOpenPath && event.Kind == "opened") ||
		((command.command.Path == v2TerminalInputPath || command.command.Path == v2TerminalResizePath) && event.Kind == "accepted") ||
		(command.command.Path == v2TerminalClosePath && event.Kind == "closed") || event.Kind == "error"
	if !validKind {
		return
	}
	delete(item.pending, event.CommandID)
	if event.Kind == "closed" {
		session.markClosed(now)
	}
	if event.Kind == "error" {
		message := strings.TrimSpace(event.Error)
		if message == "" {
			message = "light terminal command failed"
		}
		completeLightTerminalCommand(command, errors.New(message))
		return
	}
	if command.command.Path != v2TerminalOpenPath {
		session.touch(now)
	}
	completeLightTerminalCommand(command, nil)
}

func completeLightTerminalCommand(command *lightTerminalCommand, err error) {
	select {
	case command.done <- err:
	default:
	}
}

func (r *lightTerminalRelay) commandNode(nodeID string) (*lightTerminalNode, error) {
	item := r.node(nodeID, false)
	if item == nil {
		return nil, ErrTerminalUnavailable
	}
	now := r.now().UTC()
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed || !item.available || item.lastPoll.IsZero() || now.Sub(item.lastPoll) > lightTerminalLiveness {
		return nil, ErrTerminalUnavailable
	}
	return item, nil
}

func (r *lightTerminalRelay) enqueueAndWait(ctx context.Context, nodeID string, command TerminalRelayCommand) error {
	if ctx == nil {
		ctx = context.Background()
	}
	item, err := r.commandNode(nodeID)
	if err != nil {
		return err
	}
	request := &lightTerminalCommand{command: command, done: make(chan error, 1)}
	item.mu.Lock()
	if item.closed || len(item.pending) >= lightTerminalQueueLimit {
		item.mu.Unlock()
		return terminal.ErrLimit
	}
	item.pending[command.ID] = request
	item.queued = append(item.queued, request)
	close(item.wake)
	item.wake = make(chan struct{})
	item.mu.Unlock()

	return waitLightTerminalCommand(ctx, item, request)
}

func waitLightTerminalCommand(ctx context.Context, item *lightTerminalNode, request *lightTerminalCommand) error {
	timeout := lightTerminalCommandTTL
	if timeout > 12*time.Second {
		timeout = 12 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case err := <-request.done:
		return err
	case <-waitCtx.Done():
		item.mu.Lock()
		request.cancel = true
		if item.pending[request.command.ID] == request {
			delete(item.pending, request.command.ID)
		}
		item.mu.Unlock()
		return waitCtx.Err()
	}
}

func (r *lightTerminalRelay) open(ctx context.Context, nodeID string, rows, columns uint16) (TerminalOpenResponse, error) {
	item, err := r.commandNode(nodeID)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	now := r.now().UTC()
	session := &lightTerminalSession{id: sessionID, notify: make(chan struct{}), createdAt: now, updatedAt: now}
	command, err := newLightTerminalCommand(v2TerminalOpenPath, sessionID, TerminalOpenRequest{Rows: rows, Columns: columns}, now)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	request := &lightTerminalCommand{command: command, done: make(chan error, 1)}
	item.mu.Lock()
	if item.closed || len(item.pending) >= lightTerminalQueueLimit {
		item.mu.Unlock()
		return TerminalOpenResponse{}, terminal.ErrLimit
	}
	if item.activeSessionCount() >= terminal.DefaultMaxOwnerSessions {
		item.mu.Unlock()
		return TerminalOpenResponse{}, terminal.ErrLimit
	}
	item.sessions[sessionID] = session
	item.pending[command.ID] = request
	item.queued = append(item.queued, request)
	close(item.wake)
	item.wake = make(chan struct{})
	item.mu.Unlock()
	if err := waitLightTerminalCommand(ctx, item, request); err != nil {
		item.removeSession(sessionID)
		return TerminalOpenResponse{}, err
	}
	return TerminalOpenResponse{SessionID: sessionID, Offset: 0, CreatedAt: now}, nil
}

func (r *lightTerminalRelay) output(ctx context.Context, nodeID string, input TerminalOutputRequest) (terminal.Output, error) {
	item, err := r.commandNode(nodeID)
	if err != nil {
		return terminal.Output{}, err
	}
	item.mu.Lock()
	session := item.sessions[input.SessionID]
	item.mu.Unlock()
	if session == nil {
		return terminal.Output{}, terminal.ErrNotFound
	}
	return session.output(ctx, input.Offset, time.Duration(input.Wait)*time.Millisecond, r.now().UTC())
}

func (r *lightTerminalRelay) input(ctx context.Context, nodeID string, input TerminalInputRequest) error {
	data, err := decodeTerminalPayload(input.Data)
	if err != nil || len(data) == 0 || len(data) > terminal.MaxInputBytes {
		return errors.New("invalid terminal input")
	}
	if _, err := r.session(nodeID, input.SessionID); err != nil {
		return err
	}
	command, err := newLightTerminalCommand(v2TerminalInputPath, input.SessionID, input, r.now().UTC())
	if err != nil {
		return err
	}
	return r.enqueueAndWait(ctx, nodeID, command)
}

func (r *lightTerminalRelay) resize(ctx context.Context, nodeID string, input TerminalResizeRequest) error {
	if input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
		return errors.New("invalid terminal dimensions")
	}
	if _, err := r.session(nodeID, input.SessionID); err != nil {
		return err
	}
	command, err := newLightTerminalCommand(v2TerminalResizePath, input.SessionID, input, r.now().UTC())
	if err != nil {
		return err
	}
	return r.enqueueAndWait(ctx, nodeID, command)
}

func (r *lightTerminalRelay) close(ctx context.Context, nodeID string, input TerminalCloseRequest) error {
	if _, err := r.session(nodeID, input.SessionID); err != nil {
		return err
	}
	command, err := newLightTerminalCommand(v2TerminalClosePath, input.SessionID, input, r.now().UTC())
	if err != nil {
		return err
	}
	err = r.enqueueAndWait(ctx, nodeID, command)
	if err == nil {
		if item := r.node(nodeID, false); item != nil {
			item.removeSession(input.SessionID)
		}
	}
	return err
}

func (r *lightTerminalRelay) session(nodeID, sessionID string) (*lightTerminalSession, error) {
	item, err := r.commandNode(nodeID)
	if err != nil {
		return nil, err
	}
	item.mu.Lock()
	session := item.sessions[sessionID]
	item.mu.Unlock()
	if session == nil {
		return nil, terminal.ErrNotFound
	}
	return session, nil
}

func newLightTerminalCommand(path, sessionID string, input any, now time.Time) (TerminalRelayCommand, error) {
	id, err := randomHex(16)
	if err != nil {
		return TerminalRelayCommand{}, err
	}
	payload, err := json.Marshal(input)
	if err != nil || len(payload) > MaxSummaryBytes {
		return TerminalRelayCommand{}, ErrAuthentication
	}
	command := TerminalRelayCommand{
		ID: id, Path: path, SessionID: sessionID, Payload: payload,
		ExpiresAt: now.Add(lightTerminalCommandTTL).Unix(),
	}
	if err := validateTerminalRelayCommand(command, now); err != nil {
		return TerminalRelayCommand{}, err
	}
	return command, nil
}

func (item *lightTerminalNode) activeSessionCount() int {
	count := 0
	for _, session := range item.sessions {
		if session.isActive() {
			count++
		}
	}
	return count
}

func (item *lightTerminalNode) removeSession(id string) {
	item.mu.Lock()
	delete(item.sessions, id)
	for commandID, command := range item.pending {
		if command.command.SessionID == id {
			command.cancel = true
			delete(item.pending, commandID)
			completeLightTerminalCommand(command, terminal.ErrClosed)
		}
	}
	item.mu.Unlock()
}

func (session *lightTerminalSession) isActive() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return !session.closed && session.exitedAt == nil
}

func (session *lightTerminalSession) currentNext() int64 {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.next
}

func (session *lightTerminalSession) touch(now time.Time) {
	session.mu.Lock()
	session.updatedAt = now
	session.mu.Unlock()
}

func (session *lightTerminalSession) appendOutput(offset, nextOffset int64, data []byte, truncated, exited bool, exitError string, closed bool, now time.Time) error {
	if offset < 0 || nextOffset < offset || nextOffset-offset != int64(len(data)) {
		return errors.New("invalid terminal relay output offset")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if offset < session.next {
		return nil
	}
	if offset > session.next {
		session.buffer = nil
		session.base = offset
		session.next = offset
		truncated = true
	}
	session.buffer = append(session.buffer, data...)
	session.next = nextOffset
	if len(session.buffer) > lightTerminalBufferBytes {
		drop := len(session.buffer) - lightTerminalBufferBytes
		copy(session.buffer, session.buffer[drop:])
		session.buffer = session.buffer[:lightTerminalBufferBytes]
		session.base += int64(drop)
	}
	if exited {
		when := now
		session.exitedAt = &when
		session.exitError = cleanDisplayText(exitError, 512)
	}
	if closed {
		session.closed = true
	}
	session.updatedAt = now
	close(session.notify)
	session.notify = make(chan struct{})
	return nil
}

func (session *lightTerminalSession) markExit(err error, now time.Time) {
	session.mu.Lock()
	when := now
	session.exitedAt = &when
	if err != nil {
		session.exitError = cleanDisplayText(err.Error(), 512)
	}
	session.updatedAt = now
	close(session.notify)
	session.notify = make(chan struct{})
	session.mu.Unlock()
}

func (session *lightTerminalSession) markClosed(now time.Time) {
	session.mu.Lock()
	if !session.closed {
		session.closed = true
		session.updatedAt = now
		close(session.notify)
		session.notify = make(chan struct{})
	}
	session.mu.Unlock()
}

func (session *lightTerminalSession) output(ctx context.Context, offset int64, wait time.Duration, now time.Time) (terminal.Output, error) {
	if offset < 0 || wait < 0 || wait > 1500*time.Millisecond {
		return terminal.Output{}, terminal.ErrOffset
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		session.mu.Lock()
		if offset > session.next {
			session.mu.Unlock()
			return terminal.Output{}, terminal.ErrOffset
		}
		truncated := offset < session.base
		if truncated {
			offset = session.base
		}
		start := int(offset - session.base)
		available := len(session.buffer) - start
		if available > terminal.MaxOutputBytes {
			available = terminal.MaxOutputBytes
		}
		data := append([]byte(nil), session.buffer[start:start+available]...)
		result := terminal.Output{
			Data: data, Offset: offset, NextOffset: offset + int64(len(data)),
			Truncated: truncated, ExitedAt: cloneTime(session.exitedAt),
			ExitError: session.exitError, Closed: session.closed,
		}
		notify := session.notify
		ready := len(data) > 0 || session.closed || session.exitedAt != nil
		session.updatedAt = now
		session.mu.Unlock()
		if ready || wait == 0 {
			return result, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			stopLightTerminalTimer(timer)
			return terminal.Output{}, ctx.Err()
		case <-notify:
			stopLightTerminalTimer(timer)
		case <-timer.C:
			return result, nil
		}
	}
}

func stopLightTerminalTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
