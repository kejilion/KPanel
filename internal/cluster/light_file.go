package cluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var ErrFileRelayUnavailable = errors.New("cluster file relay is unavailable")

// OpenLightFile starts one authenticated file-manager request to a paired
// lightweight node. The node is reached only through its outbound relay; the
// panel never dials an inbound port on the lightweight host.
func (s *Service) OpenLightFile(ctx context.Context, nodeID string, input LightFileRequest) (*http.Response, error) {
	if s == nil || s.light == nil || s.lightFile == nil {
		return nil, ErrFileRelayUnavailable
	}
	record, err := s.light.Host(nodeID)
	if err != nil {
		return nil, ErrFileRelayUnavailable
	}
	key, err := s.light.ReadTerminalPublicKey(record)
	if err != nil || len(key) != 32 {
		return nil, ErrFileRelayUnavailable
	}
	if s.fileStreamHub.prefersStream(nodeID) {
		return s.fileStreamHub.openLight(ctx, nodeID, input)
	}
	return s.lightFile.Open(ctx, nodeID, input)
}

// OpenLightFileTransfer opens the authenticated export stream used by the
// cross-panel file transfer path. The metadata remains in the Agent response
// header, so the caller can stream the body without buffering a directory or
// regular file in the panel process.
func (s *Service) OpenLightFileTransfer(
	ctx context.Context,
	nodeID string,
	input FederationFileOpenRequest,
) (io.ReadCloser, contract.FileTransferMetadata, error) {
	if !validID(nodeID) || !validTransferPath(input.Path) || input.ResourceVersion == "" {
		return nil, contract.FileTransferMetadata{}, ErrNotFound
	}
	query := url.Values{
		"path": []string{input.Path}, "resourceVersion": []string{input.ResourceVersion},
	}
	response, err := s.OpenLightFile(ctx, nodeID, LightFileRequest{
		Method: http.MethodGet, Path: "/v1/files/transfer/export", RawQuery: query.Encode(),
		Body: http.NoBody, BodyLength: 0,
	})
	if err != nil {
		return nil, contract.FileTransferMetadata{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, contract.FileTransferMetadata{}, fmt.Errorf("light file transfer source returned HTTP %d", response.StatusCode)
	}
	encoded := response.Header.Get(fileTransferMetadataHeader)
	metadataPayload, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
	var metadata contract.FileTransferMetadata
	if decodeErr != nil || json.Unmarshal(metadataPayload, &metadata) != nil || !validTransferMetadata(metadata) {
		_ = response.Body.Close()
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	return response.Body, metadata, nil
}

// LightFileRequest is the browser-facing file-manager request after Panel
// authentication. It is intentionally narrower than http.Request so the
// outbound node relay cannot become a general reverse proxy.
type LightFileRequest struct {
	Method     string
	Path       string
	RawQuery   string
	Headers    map[string]string
	Body       io.Reader
	BodyLength int64
}

type lightFileRelay struct {
	history bool
	mu      sync.RWMutex
	nodes   map[string]*lightFileNode
	now     func() time.Time
	epoch   string
}

type lightFileNode struct {
	mu         sync.Mutex
	id         string
	sessions   map[string]*lightFileSession
	queued     []*lightFileCommand
	pending    map[string]*lightFileCommand
	wake       chan struct{}
	lastPoll   time.Time
	resetEpoch string
	available  bool
	closed     bool
}

type lightFileCommand struct {
	command   FileRelayCommand
	done      chan error
	cancel    bool
	delivered bool
}

type lightFileSession struct {
	mu            sync.Mutex
	id            string
	responseReady chan struct{}
	data          chan []byte
	done          chan struct{}
	space         chan struct{}
	lastProgress  time.Time
	status        int
	headers       map[string]string
	responseSeen  bool
	finished      bool
	err           error
	nextOffset    int64
	recentData    []lightFileChunkReceipt
}

type lightFileChunkReceipt struct {
	offset int64
	size   int
	digest [sha256.Size]byte
}

func newLightFileRelay(now func() time.Time) *lightFileRelay {
	if now == nil {
		now = time.Now
	}
	epoch, err := randomHex(16)
	if err != nil {
		epoch = fmt.Sprintf("%032x", time.Now().UTC().UnixNano())
	}
	return &lightFileRelay{nodes: make(map[string]*lightFileNode), now: now, epoch: epoch}
}

func (r *lightFileRelay) node(id string, create bool) *lightFileNode {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.nodes[id]
	if item == nil && create {
		item = &lightFileNode{
			id: id, sessions: make(map[string]*lightFileSession),
			pending: make(map[string]*lightFileCommand), wake: make(chan struct{}),
		}
		r.nodes[id] = item
	}
	return item
}

func (r *lightFileRelay) available(id string) bool {
	r.mu.RLock()
	item := r.nodes[id]
	r.mu.RUnlock()
	if item == nil {
		return false
	}
	now := r.now().UTC()
	item.mu.Lock()
	defer item.mu.Unlock()
	return item.available && !item.closed && !item.lastPoll.IsZero() && now.Sub(item.lastPoll) <= lightFileLiveness
}

func (r *lightFileRelay) deleteNode(id string) {
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
		completeLightFileCommand(command, ErrFileRelayUnavailable)
	}
	item.pending = make(map[string]*lightFileCommand)
	item.queued = nil
	for _, session := range item.sessions {
		session.finish(ErrFileRelayUnavailable)
	}
	item.sessions = make(map[string]*lightFileSession)
	close(item.wake)
	item.wake = make(chan struct{})
	item.mu.Unlock()
}

func (r *lightFileRelay) closeAll() {
	r.mu.Lock()
	nodes := make([]*lightFileNode, 0, len(r.nodes))
	for _, item := range r.nodes {
		nodes = append(nodes, item)
	}
	r.nodes = make(map[string]*lightFileNode)
	r.mu.Unlock()
	for _, item := range nodes {
		item.mu.Lock()
		item.closed = true
		for _, command := range item.pending {
			completeLightFileCommand(command, ErrFileRelayUnavailable)
		}
		item.pending = make(map[string]*lightFileCommand)
		item.queued = nil
		for _, session := range item.sessions {
			session.finish(ErrFileRelayUnavailable)
		}
		item.sessions = make(map[string]*lightFileSession)
		close(item.wake)
		item.wake = make(chan struct{})
		item.mu.Unlock()
	}
}

func (r *lightFileRelay) poll(
	ctx context.Context,
	nodeID string,
	requestIDs []string,
	events []FileRelayEvent,
) (FileRelayPollResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	response, err := r.pollEvents(ctx, nodeID, requestIDs, events)
	// Pace the acknowledgement, not delivery to the browser. This includes
	// time already spent waiting for commands/backpressure, and protects old
	// nodes too: a sequential broker stays below the existing 600 polls/minute
	// limit without sleeping another 250 ms after every ready data batch.
	if err == nil && !r.history {
		if remaining := lightFilePollInterval - time.Since(started); remaining > 0 {
			timer := time.NewTimer(remaining)
			defer stopLightFileTimer(timer)
			select {
			case <-ctx.Done():
				return FileRelayPollResponse{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return response, err
}

func (r *lightFileRelay) pollEvents(
	ctx context.Context,
	nodeID string,
	requestIDs []string,
	events []FileRelayEvent,
) (FileRelayPollResponse, error) {
	item := r.node(nodeID, true)
	applySnapshot := true
	for {
		now := r.now().UTC()
		item.mu.Lock()
		if item.closed {
			item.mu.Unlock()
			return FileRelayPollResponse{}, ErrFileRelayUnavailable
		}
		item.available = true
		item.lastPoll = now
		if applySnapshot {
			// Reconcile once, before a data event can yield the node lock.
			// Reapplying this old snapshot after wake could discard a request
			// delivered by a newer overlapping poll. Terminal events can carry
			// the last data for a just-removed node ID.
			observed := append([]string(nil), requestIDs...)
			for _, event := range events {
				observed = append(observed, event.RequestID)
			}
			item.reconcileSessions(observed, now)
			if err := item.applyEvents(ctx, events, now); err != nil {
				item.mu.Unlock()
				return FileRelayPollResponse{}, err
			}
			applySnapshot = false
		}
		if item.closed {
			item.mu.Unlock()
			return FileRelayPollResponse{}, ErrFileRelayUnavailable
		}
		if command := item.takeCommand(now); command != nil {
			item.resetEpoch = r.epoch
			response := FileRelayPollResponse{Epoch: r.epoch, Command: &command.command}
			item.mu.Unlock()
			return response, nil
		}
		if r.epoch != "" && item.resetEpoch != r.epoch && len(requestIDs) > 0 {
			item.resetEpoch = r.epoch
			response := FileRelayPollResponse{Epoch: r.epoch}
			item.mu.Unlock()
			return response, nil
		}
		// The producer may already have another bounded batch waiting. File
		// acknowledgements are paced by poll; history keeps its existing policy.
		if len(events) > 0 {
			item.resetEpoch = r.epoch
			item.mu.Unlock()
			return FileRelayPollResponse{Epoch: r.epoch}, nil
		}
		notify := item.wake
		wait := lightFilePollWait
		if len(requestIDs) > 0 || len(item.pending) > 0 {
			wait = lightFileActivePollWait
		}
		item.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			stopLightFileTimer(timer)
			return FileRelayPollResponse{}, ctx.Err()
		case <-notify:
			stopLightFileTimer(timer)
		case <-timer.C:
			item.mu.Lock()
			if !item.closed {
				item.resetEpoch = r.epoch
			}
			item.mu.Unlock()
			return FileRelayPollResponse{Epoch: r.epoch}, nil
		}
		if ctx.Err() != nil {
			return FileRelayPollResponse{}, ctx.Err()
		}
		events = nil
	}
}

func (item *lightFileNode) takeCommand(now time.Time) *lightFileCommand {
	for len(item.queued) > 0 {
		command := item.queued[0]
		item.queued = item.queued[1:]
		current, ok := item.pending[command.command.ID]
		if !ok || current != command || command.cancel {
			continue
		}
		if now.Unix() >= command.command.ExpiresAt {
			delete(item.pending, command.command.ID)
			completeLightFileCommand(command, ErrFileRelayUnavailable)
			continue
		}
		command.delivered = true
		return command
	}
	for commandID, command := range item.pending {
		if command.cancel || !command.delivered {
			continue
		}
		if now.Unix() >= command.command.ExpiresAt {
			delete(item.pending, commandID)
			completeLightFileCommand(command, ErrFileRelayUnavailable)
			continue
		}
		return command
	}
	return nil
}

func (item *lightFileNode) applyEvents(ctx context.Context, events []FileRelayEvent, now time.Time) error {
	for _, event := range events {
		session := item.sessions[event.RequestID]
		if event.CommandID != "" {
			command := item.pending[event.CommandID]
			if command != nil && command.command.RequestID == event.RequestID {
				delete(item.pending, event.CommandID)
				switch event.Kind {
				case "ready", "accepted":
					completeLightFileCommand(command, nil)
				case "error":
					err := errors.New(event.Error)
					completeLightFileCommand(command, err)
					if session != nil {
						session.finish(err)
					}
				}
			}
		}
		if session == nil {
			continue
		}
		session.progress(now)
		switch event.Kind {
		case "response":
			session.response(event.Status, event.Headers)
		case "data":
			// Backpressure belongs to this request. Never hold the node lock
			// while waiting for the browser to consume a chunk.
			item.mu.Unlock()
			err := session.push(ctx, event.Offset, event.Data)
			item.mu.Lock()
			if err != nil {
				// Losing the broker HTTP connection does not cancel the browser's
				// stream. Keep accepted chunks and let the next authenticated poll
				// resend this batch; do not apply a later end event prematurely.
				if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
					return err
				}
				session.finish(err)
			}
		case "end":
			session.finish(nil)
		case "error":
			if event.CommandID == "" {
				session.finish(errors.New(event.Error))
			}
		}
		if event.Kind == "end" || (event.Kind == "error" && session.isFinished()) {
			delete(item.sessions, event.RequestID)
		}
	}
	return nil
}

func (item *lightFileNode) reconcileSessions(requestIDs []string, _ time.Time) {
	keep := make(map[string]struct{}, len(requestIDs))
	for _, requestID := range requestIDs {
		keep[requestID] = struct{}{}
	}
	for requestID, session := range item.sessions {
		if _, ok := keep[requestID]; ok {
			continue
		}
		// A returned poll response can be lost before the node receives it.
		// Keep a read request until the node acknowledges it, allowing
		// takeCommand to redeliver the same ID. Mutations whose receipt is
		// uncertain must still fail: a restarted node loses its dedupe state.
		// The existing command TTL and session watchdog still bound cleanup.
		if item.canAwaitRequestAcknowledgement(requestID) {
			continue
		}
		session.finish(ErrFileRelayUnavailable)
		delete(item.sessions, requestID)
		for commandID, command := range item.pending {
			if command.command.RequestID != requestID {
				continue
			}
			command.cancel = true
			delete(item.pending, commandID)
			completeLightFileCommand(command, ErrFileRelayUnavailable)
		}
	}
}

func (item *lightFileNode) canAwaitRequestAcknowledgement(requestID string) bool {
	for _, command := range item.pending {
		if command.command.RequestID == requestID && command.command.Kind == "request" &&
			!command.cancel {
			return !command.delivered || command.command.Method == http.MethodGet || command.command.Method == http.MethodHead
		}
	}
	return false
}

func completeLightFileCommand(command *lightFileCommand, err error) {
	select {
	case command.done <- err:
	default:
	}
}

func (r *lightFileRelay) commandNode(nodeID string) (*lightFileNode, error) {
	item := r.node(nodeID, false)
	if item == nil {
		return nil, ErrFileRelayUnavailable
	}
	now := r.now().UTC()
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed || !item.available || item.lastPoll.IsZero() || now.Sub(item.lastPoll) > lightFileLiveness {
		return nil, ErrFileRelayUnavailable
	}
	return item, nil
}

func (r *lightFileRelay) Open(
	ctx context.Context,
	nodeID string,
	input LightFileRequest,
) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	validRequest := validFileRelayRequest(input)
	if r.history {
		validRequest = validHistoryRelayRequest(input)
	}
	if !validID(nodeID) || !validRequest {
		return nil, ErrFileRelayUnavailable
	}
	if input.Body != nil && input.Body != http.NoBody && input.BodyLength != 0 {
		// Only finite in-memory readers may omit Close. Accepting an arbitrary
		// blocking Reader would make cancellation unable to reclaim its goroutine.
		switch input.Body.(type) {
		case io.ReadCloser:
		case *bytes.Reader, *bytes.Buffer, *strings.Reader:
		default:
			return nil, errors.New("file relay streaming request body must support Close")
		}
	}
	item, err := r.commandNode(nodeID)
	if err != nil {
		return nil, err
	}
	requestID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	now := r.now().UTC()
	session := &lightFileSession{
		id: requestID, responseReady: make(chan struct{}), data: make(chan []byte, 8),
		done: make(chan struct{}), space: make(chan struct{}, 1), lastProgress: now,
	}
	command := FileRelayCommand{
		ID: requestID, Kind: "request", RequestID: requestID,
		Method: input.Method, Path: input.Path, Query: input.RawQuery,
		Headers: cloneFileRelayHeaders(input.Headers), BodyLength: input.BodyLength,
		ExpiresAt: now.Add(lightFileCommandTTL).Unix(),
	}
	if input.Body == nil || input.Body == http.NoBody {
		command.BodyLength = 0
	}
	validateCommand := validateFileRelayCommand
	if r.history {
		validateCommand = validHistoryRelayCommand
	}
	if err := validateCommand(command, now); err != nil {
		return nil, err
	}
	item.mu.Lock()
	item.pruneCommands(now)
	if item.closed || len(item.pending) >= lightFileQueueLimit || len(item.sessions) >= lightFileQueueLimit {
		item.mu.Unlock()
		return nil, ErrRateLimited
	}
	request := &lightFileCommand{command: command, done: make(chan error, 1)}
	item.sessions[requestID] = session
	item.pending[command.ID] = request
	item.queued = append(item.queued, request)
	wakeLightFileNode(item)
	item.mu.Unlock()

	requestContext, stopRequest := context.WithCancel(ctx)
	go r.watchSession(requestContext, stopRequest, item, session, input.Body)
	if err := waitLightFileCommandWithin(ctx, item, request, lightFileCommandAckWait); err != nil {
		session.finish(contextError(ctx, err))
		return nil, contextError(ctx, err)
	}
	// Do not consume an upload body until the authenticated node has accepted
	// the request. This keeps connection recovery separate from file transfer
	// and avoids buffering work for a broker that is not actually reachable.
	if command.BodyLength != 0 && input.Body != nil && input.Body != http.NoBody {
		go r.sendBody(requestContext, item, session, input.Body, command.BodyLength)
	}
	select {
	case <-session.responseReady:
		status, headers, responseErr := session.responseValues()
		if responseErr != nil {
			return nil, responseErr
		}
		return &http.Response{
			StatusCode: status, Status: fmt.Sprintf("%d", status), Header: fileRelayHTTPHeaders(headers),
			Body: &lightFileResponseBody{relay: r, item: item, session: session, nodeID: nodeID},
		}, nil
	case <-ctx.Done():
		session.finish(ctx.Err())
		return nil, ctx.Err()
	}
}

func validFileRelayRequest(input LightFileRequest) bool {
	if input.Method != http.MethodGet && input.Method != http.MethodHead && input.Method != http.MethodPost && input.Method != http.MethodPut {
		return false
	}
	if !v2FileRelayRequestPath(input.Path) || len(input.RawQuery) > lightFileMaxQueryBytes || strings.ContainsAny(input.RawQuery, "\r\n\x00") || !validFileRelayHeaders(input.Headers) {
		return false
	}
	return input.BodyLength >= -1 && input.BodyLength <= 512<<20
}

func cloneFileRelayHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		result[key] = value
	}
	return result
}

func fileRelayHTTPHeaders(headers map[string]string) http.Header {
	result := make(http.Header, len(headers))
	for key, value := range headers {
		result.Set(key, value)
	}
	return result
}

func wakeLightFileNode(item *lightFileNode) {
	close(item.wake)
	item.wake = make(chan struct{})
}

func (r *lightFileRelay) sendBody(ctx context.Context, item *lightFileNode, session *lightFileSession, body io.Reader, length int64) {
	buffer := make([]byte, lightFileChunkBytes)
	offset := int64(0)
	for {
		if ctx.Err() != nil || session.isFinished() {
			return
		}
		count, readErr := body.Read(buffer)
		if count < 0 || count > len(buffer) {
			session.finish(errors.New("file relay request body read is invalid"))
			return
		}
		if count > 0 {
			if offset+int64(count) > 512<<20 {
				session.finish(errors.New("file relay request body exceeds limit"))
				return
			}
			if length >= 0 && offset+int64(count) > length {
				session.finish(errors.New("file relay request body exceeds declared length"))
				return
			}
			final := readErr == io.EOF || (length >= 0 && offset+int64(count) == length)
			if readErr == io.EOF && length >= 0 && offset+int64(count) != length {
				session.finish(errors.New("file relay request body is incomplete"))
				return
			}
			command, err := r.newBodyCommand(session.id, offset, buffer[:count], final)
			if err != nil {
				session.finish(err)
				return
			}
			if enqueueErr := r.enqueueAndWait(ctx, item, command); enqueueErr != nil {
				session.finish(contextError(ctx, enqueueErr))
				return
			}
			offset += int64(count)
			if final {
				if length >= 0 && offset != length {
					session.finish(errors.New("file relay request body is incomplete"))
				}
				return
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if length >= 0 && offset != length {
					session.finish(errors.New("file relay request body is incomplete"))
					return
				}
				command, err := r.newBodyCommand(session.id, offset, nil, true)
				if err != nil {
					session.finish(err)
					return
				}
				if enqueueErr := r.enqueueAndWait(ctx, item, command); enqueueErr != nil {
					session.finish(contextError(ctx, enqueueErr))
				}
				return
			}
			session.finish(readErr)
			return
		}
		if ctx.Err() != nil {
			session.finish(ctx.Err())
			return
		}
	}
}

func contextError(ctx context.Context, candidate error) error {
	if candidate != nil {
		return candidate
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrFileRelayUnavailable
}

func (r *lightFileRelay) newBodyCommand(requestID string, offset int64, data []byte, final bool) (FileRelayCommand, error) {
	id, err := randomHex(16)
	if err != nil {
		return FileRelayCommand{}, err
	}
	command := FileRelayCommand{
		ID: id, Kind: "body", RequestID: requestID, Offset: offset,
		Data: append([]byte(nil), data...), Final: final,
		ExpiresAt: r.now().UTC().Add(lightFileCommandTTL).Unix(),
	}
	if err := validateFileRelayCommand(command, r.now().UTC()); err != nil {
		return FileRelayCommand{}, err
	}
	return command, nil
}

func (r *lightFileRelay) enqueueAndWait(ctx context.Context, item *lightFileNode, command FileRelayCommand) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := &lightFileCommand{command: command, done: make(chan error, 1)}
	item.mu.Lock()
	if item.closed || ctx.Err() != nil || item.sessions[command.RequestID] == nil || item.sessions[command.RequestID].isFinished() {
		item.mu.Unlock()
		return contextError(ctx, nil)
	}
	if len(item.pending) >= lightFileQueueLimit {
		item.mu.Unlock()
		return ErrRateLimited
	}
	item.pending[command.ID] = request
	item.queued = append(item.queued, request)
	wakeLightFileNode(item)
	item.mu.Unlock()
	return waitLightFileCommand(ctx, item, request)
}

func waitLightFileCommand(ctx context.Context, item *lightFileNode, request *lightFileCommand) error {
	return waitLightFileCommandWithin(ctx, item, request, lightFileCommandTTL)
}

func waitLightFileCommandWithin(ctx context.Context, item *lightFileNode, request *lightFileCommand, timeout time.Duration) error {
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

// watchSession owns request cleanup, including after response headers have
// already been returned. Neither a silent node nor an unread body can retain
// a session indefinitely. Streaming upload readers must unblock on Close.
func (r *lightFileRelay) watchSession(ctx context.Context, stop context.CancelFunc, item *lightFileNode, session *lightFileSession, body io.Reader) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		stop()
		r.cancel(item, session.id, session.failure() != nil)
		if closer, ok := body.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			session.finish(ctx.Err())
			return
		case <-session.done:
			return
		case <-ticker.C:
			now := r.now().UTC()
			item.mu.Lock()
			offline := item.closed || now.Sub(item.lastPoll) > lightFileLiveness
			item.mu.Unlock()
			if offline {
				session.finish(ErrFileRelayUnavailable)
				return
			}
			session.mu.Lock()
			idle := now.Sub(session.lastProgress) >= lightFileCommandTTL
			session.mu.Unlock()
			if idle {
				session.finish(context.DeadlineExceeded)
				return
			}
		}
	}
}

func (session *lightFileSession) progress(now time.Time) {
	session.mu.Lock()
	session.lastProgress = now
	session.mu.Unlock()
}

func (session *lightFileSession) failure() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.err
}

func (item *lightFileNode) pruneCommands(now time.Time) {
	for id, command := range item.pending {
		if command.cancel || now.Unix() >= command.command.ExpiresAt {
			delete(item.pending, id)
			completeLightFileCommand(command, ErrFileRelayUnavailable)
		}
	}
	queued := item.queued[:0]
	for _, command := range item.queued {
		if item.pending[command.command.ID] == command {
			queued = append(queued, command)
		}
	}
	clear(item.queued[len(queued):])
	item.queued = queued
}

func (r *lightFileRelay) cancel(item *lightFileNode, requestID string, notify bool) {
	if item == nil || !validID(requestID) {
		return
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	delete(item.sessions, requestID)
	for commandID, pending := range item.pending {
		if pending.command.RequestID == requestID {
			delete(item.pending, commandID)
			completeLightFileCommand(pending, ErrFileRelayUnavailable)
		}
	}
	item.pruneCommands(r.now().UTC())
	if !notify || item.closed || len(item.pending) >= lightFileQueueLimit {
		return
	}
	id, err := randomHex(16)
	if err != nil {
		return
	}
	command := FileRelayCommand{ID: id, Kind: "cancel", RequestID: requestID, ExpiresAt: r.now().UTC().Add(lightFileCommandTTL).Unix()}
	item.pending[id] = &lightFileCommand{command: command, done: make(chan error, 1)}
	item.queued = append(item.queued, item.pending[id])
	wakeLightFileNode(item)
}

func stopLightFileTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (session *lightFileSession) response(status int, headers map[string]string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.responseSeen || session.finished {
		return
	}
	session.status = status
	session.headers = cloneFileRelayHeaders(headers)
	session.responseSeen = true
	close(session.responseReady)
}

func (session *lightFileSession) responseValues() (int, map[string]string, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.status, cloneFileRelayHeaders(session.headers), session.err
}

func (session *lightFileSession) push(ctx context.Context, offset int64, data []byte) error {
	if len(data) == 0 || len(data) > lightFileChunkBytes {
		return ErrFileRelayUnavailable
	}
	digest := sha256.Sum256(data)
	for {
		session.mu.Lock()
		if session.finished {
			session.mu.Unlock()
			return ErrFileRelayUnavailable
		}
		if offset != session.nextOffset {
			// A lost HTTP acknowledgement causes the node to resend its same
			// event batch in a freshly authenticated envelope. Match complete
			// chunks without retaining file bytes or accepting changed overlaps.
			for _, receipt := range session.recentData {
				if receipt.offset == offset && receipt.size == len(data) && receipt.digest == digest {
					session.mu.Unlock()
					return nil
				}
			}
			session.mu.Unlock()
			return ErrFileRelayUnavailable
		}
		select {
		case session.data <- append([]byte(nil), data...):
			session.nextOffset += int64(len(data))
			if len(session.recentData) == lightFileEventLimit {
				copy(session.recentData, session.recentData[1:])
				session.recentData = session.recentData[:lightFileEventLimit-1]
			}
			session.recentData = append(session.recentData, lightFileChunkReceipt{offset: offset, size: len(data), digest: digest})
			session.mu.Unlock()
			return nil
		default:
			session.mu.Unlock()
		}
		select {
		case <-session.done:
			return ErrFileRelayUnavailable
		case <-ctx.Done():
			return ctx.Err()
		case <-session.space:
		}
	}
}

func (session *lightFileSession) finish(err error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finished {
		return
	}
	session.finished = true
	if err != nil {
		session.err = err
	}
	if !session.responseSeen {
		if session.err == nil {
			session.err = ErrFileRelayUnavailable
		}
		close(session.responseReady)
	}
	close(session.done)
}

func (session *lightFileSession) isFinished() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.finished
}

type lightFileResponseBody struct {
	relay   *lightFileRelay
	item    *lightFileNode
	session *lightFileSession
	nodeID  string
	once    sync.Once
	buffer  []byte
}

func (body *lightFileResponseBody) Read(output []byte) (int, error) {
	if len(output) == 0 {
		return 0, nil
	}
	if err := body.session.failure(); err != nil {
		return 0, err
	}
	for len(body.buffer) == 0 {
		body.session.mu.Lock()
		err, finished := body.session.err, body.session.finished
		body.session.mu.Unlock()
		if err != nil {
			return 0, err
		}
		select {
		case body.buffer = <-body.session.data:
		default:
			if finished {
				return 0, io.EOF
			}
			select {
			case body.buffer = <-body.session.data:
			case <-body.session.done:
				continue
			}
		}
		select {
		case body.session.space <- struct{}{}:
		default:
		}
	}
	count := copy(output, body.buffer)
	body.buffer = body.buffer[count:]
	return count, nil
}

func (body *lightFileResponseBody) Close() error {
	body.once.Do(func() {
		if body.relay != nil && body.item != nil && !body.session.isFinished() {
			body.session.finish(io.ErrClosedPipe)
		}
	})
	return nil
}
