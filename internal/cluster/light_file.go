package cluster

import (
	"context"
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
	mu    sync.RWMutex
	nodes map[string]*lightFileNode
	now   func() time.Time
	epoch string
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
	status        int
	headers       map[string]string
	responseSeen  bool
	finished      bool
	err           error
	nextOffset    int64
	dataClosed    bool
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
	item := r.node(nodeID, true)
	for {
		now := r.now().UTC()
		item.mu.Lock()
		if item.closed {
			item.mu.Unlock()
			return FileRelayPollResponse{}, ErrFileRelayUnavailable
		}
		item.available = true
		item.lastPoll = now
		item.applyEvents(events, now)
		item.reconcileSessions(requestIDs, now)
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

func (item *lightFileNode) applyEvents(events []FileRelayEvent, now time.Time) {
	for _, event := range events {
		session := item.sessions[event.RequestID]
		if session == nil {
			continue
		}
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
					session.finish(err)
				}
			}
		}
		switch event.Kind {
		case "response":
			session.response(event.Status, event.Headers)
		case "data":
			if err := session.push(event.Offset, event.Data); err != nil {
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
	_ = now
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
		// A request can be created while the node is already holding a long
		// poll, or immediately after its previous poll returned. In both cases
		// the node has not had a chance to observe the new request ID yet. Keep
		// the session until its request command has been delivered once; only
		// then does an omitted ID prove that the node lost the session.
		if item.hasUndeliveredRequest(requestID) {
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

func (item *lightFileNode) hasUndeliveredRequest(requestID string) bool {
	for _, command := range item.pending {
		if command.command.RequestID == requestID && command.command.Kind == "request" &&
			!command.delivered && !command.cancel {
			return true
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
	if !validID(nodeID) || !validFileRelayRequest(input) {
		return nil, ErrFileRelayUnavailable
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
	if err := validateFileRelayCommand(command, now); err != nil {
		return nil, err
	}
	item.mu.Lock()
	if item.closed || len(item.pending) >= lightFileQueueLimit {
		item.mu.Unlock()
		return nil, ErrRateLimited
	}
	item.sessions[requestID] = session
	item.pending[command.ID] = &lightFileCommand{command: command, done: make(chan error, 1)}
	item.queued = append(item.queued, item.pending[command.ID])
	wakeLightFileNode(item)
	item.mu.Unlock()

	if command.BodyLength != 0 && input.Body != nil && input.Body != http.NoBody {
		go r.sendBody(ctx, item, session, input.Body, command.BodyLength)
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
		r.cancel(item, nodeID, requestID)
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
		count, readErr := body.Read(buffer)
		if count > 0 {
			if length >= 0 && offset+int64(count) > length {
				session.finish(errors.New("file relay request body exceeds declared length"))
				r.cancel(item, item.id, session.id)
				return
			}
			final := readErr == io.EOF || (length >= 0 && offset+int64(count) == length)
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
			r.cancel(item, item.id, session.id)
			return
		}
		if ctx.Err() != nil {
			session.finish(ctx.Err())
			r.cancel(item, item.id, session.id)
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
	if item.closed || len(item.pending) >= lightFileQueueLimit {
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
	waitCtx, cancel := context.WithTimeout(ctx, lightFileCommandTTL)
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

func (r *lightFileRelay) cancel(item *lightFileNode, nodeID, requestID string) {
	if item == nil || !validID(requestID) {
		return
	}
	id, err := randomHex(16)
	if err != nil {
		return
	}
	command := FileRelayCommand{ID: id, Kind: "cancel", RequestID: requestID, ExpiresAt: r.now().UTC().Add(lightFileCommandTTL).Unix()}
	if validateFileRelayCommand(command, r.now().UTC()) != nil {
		return
	}
	item.mu.Lock()
	if item.closed || len(item.pending) >= lightFileQueueLimit {
		item.mu.Unlock()
		return
	}
	item.pending[id] = &lightFileCommand{command: command, done: make(chan error, 1)}
	item.queued = append(item.queued, item.pending[id])
	wakeLightFileNode(item)
	item.mu.Unlock()
	_ = nodeID
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

func (session *lightFileSession) push(offset int64, data []byte) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finished || session.dataClosed || offset != session.nextOffset || len(data) == 0 {
		return ErrFileRelayUnavailable
	}
	session.nextOffset += int64(len(data))
	session.data <- append([]byte(nil), data...)
	return nil
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
		close(session.responseReady)
	}
	if !session.dataClosed {
		close(session.data)
		session.dataClosed = true
	}
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
	for len(body.buffer) == 0 {
		chunk, ok := <-body.session.data
		if !ok {
			body.session.mu.Lock()
			err := body.session.err
			body.session.mu.Unlock()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		body.buffer = chunk
	}
	count := copy(output, body.buffer)
	body.buffer = body.buffer[count:]
	return count, nil
}

func (body *lightFileResponseBody) Close() error {
	body.once.Do(func() {
		if body.relay != nil && body.item != nil && !body.session.isFinished() {
			body.session.finish(io.ErrClosedPipe)
			body.relay.cancel(body.item, body.nodeID, body.session.id)
		}
	})
	return nil
}
