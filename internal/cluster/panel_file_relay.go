package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/flynn/noise"
)

const (
	panelFileRelayPollWait     = 2 * time.Second
	panelFileRelaySessionTTL   = 2 * time.Hour
	panelFileRelayQueueLimit   = 128
	panelFileRelayQueueTimeout = 30 * time.Second
)

// panelFileRelay is the target-side broker for a paired Panel. It executes
// only the Panel's constrained file handler; it never exposes the general
// HTTP router or lets a peer choose an arbitrary URL.
type panelFileRelay struct {
	mu       sync.Mutex
	handler  http.Handler
	sessions map[string]*panelFileRelaySession
}

type panelFileRelaySession struct {
	id      string
	handler http.Handler
	ctx     context.Context
	stop    context.CancelFunc

	events chan FileRelayEvent

	commandMu sync.Mutex
	mu        sync.Mutex
	processed map[string]FileRelayEvent

	started          bool
	requestCommandID string
	bodyWriter       *io.PipeWriter
	bodyLength       int64
	bodyOffset       int64
	bodyClosed       bool
	finished         bool
	expiresAt        time.Time
}

func newPanelFileRelay() *panelFileRelay {
	return &panelFileRelay{sessions: make(map[string]*panelFileRelaySession)}
}

func (r *panelFileRelay) setHandler(handler http.Handler) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
}

func (r *panelFileRelay) available() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handler != nil
}

func (r *panelFileRelay) closeAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	sessions := make([]*panelFileRelaySession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.sessions = make(map[string]*panelFileRelaySession)
	r.mu.Unlock()
	for _, session := range sessions {
		session.abort(ErrFileRelayUnavailable)
	}
}

func (r *panelFileRelay) poll(
	ctx context.Context,
	input FileRelayPollRequest,
	now time.Time,
) (FileRelayPollResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validatePanelFileRelayPoll(input, now); err != nil {
		return FileRelayPollResponse{}, err
	}
	r.mu.Lock()
	r.gcLocked(now.UTC())
	handler := r.handler
	session := r.sessions[input.SessionID]
	if session == nil && input.Command != nil && input.Command.Kind == "request" && handler != nil {
		session = newPanelFileRelaySession(input.SessionID, handler, now.UTC())
		r.sessions[input.SessionID] = session
	}
	r.mu.Unlock()
	if handler == nil || session == nil {
		return FileRelayPollResponse{}, ErrFileRelayUnavailable
	}
	if input.Command != nil {
		if err := session.handleCommand(ctx, *input.Command, now.UTC()); err != nil {
			r.removeSession(input.SessionID, session)
			session.abort(err)
			return FileRelayPollResponse{}, err
		}
	}
	events, err := session.collect(ctx)
	if err != nil {
		r.removeSession(input.SessionID, session)
		session.abort(err)
		return FileRelayPollResponse{}, err
	}
	if len(events) > 0 && (events[0].Kind == "end" || events[0].Kind == "error") {
		r.mu.Lock()
		if r.sessions[input.SessionID] == session {
			delete(r.sessions, input.SessionID)
		}
		r.mu.Unlock()
		session.abort(nil)
	}
	return FileRelayPollResponse{Events: events}, nil
}

func (r *panelFileRelay) removeSession(id string, session *panelFileRelaySession) {
	if r == nil || session == nil {
		return
	}
	r.mu.Lock()
	if r.sessions[id] == session {
		delete(r.sessions, id)
	}
	r.mu.Unlock()
}

func (r *panelFileRelay) gcLocked(now time.Time) {
	for id, session := range r.sessions {
		if session.expired(now) {
			delete(r.sessions, id)
			go session.abort(ErrFileRelayUnavailable)
		}
	}
}

func newPanelFileRelaySession(id string, handler http.Handler, now time.Time) *panelFileRelaySession {
	ctx, stop := context.WithTimeout(context.Background(), panelFileRelaySessionTTL)
	return &panelFileRelaySession{
		id: id, handler: handler, ctx: ctx, stop: stop,
		events:    make(chan FileRelayEvent, panelFileRelayQueueLimit),
		processed: make(map[string]FileRelayEvent), expiresAt: now.Add(panelFileRelaySessionTTL),
	}
}

func (s *panelFileRelaySession) expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.expiresAt.After(now)
}

func (s *panelFileRelaySession) touch(now time.Time) {
	s.mu.Lock()
	s.expiresAt = now.Add(panelFileRelaySessionTTL)
	s.mu.Unlock()
}

func (s *panelFileRelaySession) handleCommand(ctx context.Context, command FileRelayCommand, now time.Time) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	s.touch(now)

	s.mu.Lock()
	if previous, ok := s.processed[command.ID]; ok {
		s.mu.Unlock()
		return s.queue(previous)
	}
	s.mu.Unlock()

	switch command.Kind {
	case "request":
		return s.handleRequest(command)
	case "body":
		return s.handleBody(ctx, command)
	case "cancel":
		return s.handleCancel(command)
	default:
		return s.rememberError(command.ID, "unsupported file relay command")
	}
}

func (s *panelFileRelaySession) handleRequest(command FileRelayCommand) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return s.rememberError(command.ID, "file relay request already started")
	}
	s.mu.Unlock()

	request, bodyWriter, err := s.newRequest(command)
	if err != nil {
		return s.rememberError(command.ID, "file relay request could not be created")
	}
	s.mu.Lock()
	s.started = true
	s.requestCommandID = command.ID
	s.bodyWriter = bodyWriter
	s.bodyLength = command.BodyLength
	s.mu.Unlock()
	accepted := FileRelayEvent{RequestID: s.id, CommandID: command.ID, Kind: "accepted"}
	if err := s.remember(command.ID, accepted); err != nil {
		return err
	}
	go s.serve(request)
	return nil
}

func (s *panelFileRelaySession) newRequest(command FileRelayCommand) (*http.Request, *io.PipeWriter, error) {
	var body io.ReadCloser = http.NoBody
	var bodyWriter *io.PipeWriter
	if command.BodyLength != 0 {
		bodyReader, writer := io.Pipe()
		body = bodyReader
		bodyWriter = writer
	}
	request, err := http.NewRequestWithContext(
		s.ctx, command.Method, "http://kpanel-file-relay"+command.Path, body,
	)
	if err != nil {
		if bodyWriter != nil {
			_ = bodyWriter.CloseWithError(err)
		}
		return nil, nil, err
	}
	request.URL.RawQuery = command.Query
	request.Header = relayRequestHeaders(command.Headers)
	request.Header.Set("X-Request-ID", s.id)
	if command.BodyLength >= 0 {
		request.ContentLength = command.BodyLength
	} else {
		request.ContentLength = -1
	}
	return request, bodyWriter, nil
}

func relayRequestHeaders(input map[string]string) http.Header {
	output := make(http.Header)
	for key, value := range input {
		switch http.CanonicalHeaderKey(key) {
		case "Accept", "Content-Type", "If-Modified-Since", "If-None-Match", "If-Range", "Range":
			output.Set(http.CanonicalHeaderKey(key), value)
		}
	}
	return output
}

func (s *panelFileRelaySession) handleBody(ctx context.Context, command FileRelayCommand) error {
	s.mu.Lock()
	if !s.started || s.bodyWriter == nil || s.bodyClosed {
		s.mu.Unlock()
		return s.rememberError(command.ID, "file relay request body is not available")
	}
	if command.Offset != s.bodyOffset {
		s.mu.Unlock()
		return s.rememberError(command.ID, "file relay request body offset is invalid")
	}
	if s.bodyLength == 0 || (s.bodyLength >= 0 && command.Offset+int64(len(command.Data)) > s.bodyLength) ||
		(s.bodyLength >= 0 && command.Final && command.Offset+int64(len(command.Data)) != s.bodyLength) ||
		(s.bodyLength >= 0 && !command.Final && command.Offset+int64(len(command.Data)) == s.bodyLength) {
		s.mu.Unlock()
		return s.rememberError(command.ID, "file relay request body length is invalid")
	}
	bodyWriter := s.bodyWriter
	s.mu.Unlock()

	if len(command.Data) > 0 {
		writeResult := make(chan struct {
			written int
			err     error
		}, 1)
		go func() {
			written, err := bodyWriter.Write(command.Data)
			writeResult <- struct {
				written int
				err     error
			}{written: written, err: err}
		}()
		var result struct {
			written int
			err     error
		}
		select {
		case result = <-writeResult:
		case <-ctx.Done():
			_ = bodyWriter.CloseWithError(ctx.Err())
			return ctx.Err()
		case <-s.ctx.Done():
			return ErrFileRelayUnavailable
		}
		written, err := result.written, result.err
		if err != nil || written != len(command.Data) {
			return s.rememberError(command.ID, "file relay request body write failed")
		}
	}
	s.mu.Lock()
	s.bodyOffset += int64(len(command.Data))
	if command.Final {
		s.bodyClosed = true
	}
	s.mu.Unlock()
	if command.Final {
		if err := bodyWriter.Close(); err != nil {
			return s.rememberError(command.ID, "file relay request body close failed")
		}
	}
	return s.remember(command.ID, FileRelayEvent{RequestID: s.id, CommandID: command.ID, Kind: "accepted"})
}

func (s *panelFileRelaySession) handleCancel(command FileRelayCommand) error {
	accepted := FileRelayEvent{RequestID: s.id, CommandID: command.ID, Kind: "accepted"}
	if err := s.remember(command.ID, accepted); err != nil {
		return err
	}
	s.abort(io.ErrClosedPipe)
	return nil
}

func (s *panelFileRelaySession) remember(commandID string, event FileRelayEvent) error {
	s.mu.Lock()
	s.processed[commandID] = event
	s.mu.Unlock()
	return s.queue(event)
}

func (s *panelFileRelaySession) rememberError(commandID, message string) error {
	if message == "" {
		message = "file relay command failed"
	}
	event := FileRelayEvent{
		RequestID: s.id, CommandID: commandID, Kind: "error", Error: message,
	}
	s.mu.Lock()
	s.processed[commandID] = event
	s.mu.Unlock()
	_ = s.queue(event)
	s.abort(errors.New(message))
	return nil
}

func (s *panelFileRelaySession) serve(request *http.Request) {
	defer request.Body.Close()
	writer := &panelFileRelayResponseWriter{
		session: s, header: make(http.Header), requestCommandID: s.requestCommandID,
	}
	defer func() {
		if recover() != nil {
			writer.finish(errors.New("remote file handler panicked"))
			return
		}
		writer.finish(nil)
	}()
	s.handler.ServeHTTP(writer, request)
}

func (s *panelFileRelaySession) queue(event FileRelayEvent) error {
	if validateFileRelayEvent(event) != nil {
		return ErrAuthentication
	}
	timer := time.NewTimer(panelFileRelayQueueTimeout)
	defer stopPanelFileRelayTimer(timer)
	select {
	case s.events <- event:
		return nil
	case <-s.ctx.Done():
		return ErrFileRelayUnavailable
	case <-timer.C:
		return ErrFileRelayUnavailable
	}
}

func (s *panelFileRelaySession) collect(ctx context.Context) ([]FileRelayEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(panelFileRelayPollWait)
	defer stopPanelFileRelayTimer(timer)
	select {
	case event := <-s.events:
		return []FileRelayEvent{event}, nil
	case <-s.ctx.Done():
		return nil, ErrFileRelayUnavailable
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return []FileRelayEvent{}, nil
	}
}

func (s *panelFileRelaySession) abort(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	bodyWriter := s.bodyWriter
	bodyClosed := s.bodyClosed
	s.bodyClosed = true
	s.mu.Unlock()
	if bodyWriter != nil && !bodyClosed {
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
		} else {
			_ = bodyWriter.Close()
		}
	}
	s.stop()
}

func (s *panelFileRelaySession) finishedState() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished
}

type panelFileRelayResponseWriter struct {
	session          *panelFileRelaySession
	header           http.Header
	status           int
	wroteHeader      bool
	finished         bool
	queueErr         error
	offset           int64
	requestCommandID string
}

func (w *panelFileRelayResponseWriter) Header() http.Header {
	return w.header
}

func (w *panelFileRelayResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}
	w.status = status
	w.wroteHeader = true
	event := FileRelayEvent{
		RequestID: w.session.id, Kind: "response", Status: status,
		Headers: relayResponseHeaders(w.header),
	}
	if err := w.session.queue(event); err != nil {
		w.queueErr = err
	}
}

func (w *panelFileRelayResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.queueErr != nil {
		return 0, w.queueErr
	}
	written := 0
	for len(data) > 0 {
		count := len(data)
		if count > lightFileChunkBytes {
			count = lightFileChunkBytes
		}
		event := FileRelayEvent{
			RequestID: w.session.id, Kind: "data", Offset: w.offset,
			Data: append([]byte(nil), data[:count]...),
		}
		if err := w.session.queue(event); err != nil {
			w.queueErr = err
			return written, err
		}
		w.offset += int64(count)
		written += count
		data = data[count:]
	}
	return written, nil
}

func (w *panelFileRelayResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
}

func relayResponseHeaders(input http.Header) map[string]string {
	output := make(map[string]string)
	for _, key := range []string{
		"Accept-Ranges", "Content-Disposition", "Content-Length", "Content-Range",
		"Content-Security-Policy", "Content-Type", "ETag", "Last-Modified",
		"X-Content-Type-Options", "X-KPanel-File-Metadata",
	} {
		if value := input.Get(key); value != "" {
			output[key] = value
		}
	}
	return output
}

func (w *panelFileRelayResponseWriter) finish(err error) {
	if w.finished {
		return
	}
	w.finished = true
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.queueErr == nil && err != nil {
		_ = w.session.queue(FileRelayEvent{
			RequestID: w.session.id, CommandID: w.requestCommandID,
			Kind: "error", Error: "remote file handler failed",
		})
	} else if w.queueErr == nil {
		_ = w.session.queue(FileRelayEvent{RequestID: w.session.id, Kind: "end"})
	}
	w.session.mu.Lock()
	w.session.finished = true
	w.session.mu.Unlock()
}

func stopPanelFileRelayTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// remoteV2PanelFileAPI is implemented by the v2 remote client. Keeping this
// interface on the cluster service makes unit tests independent of network
// sockets while the Panel still uses the same paired credentials as telemetry.
type remoteV2PanelFileAPI interface {
	OpenFileRelayV2(
		context.Context, string, string, string, noise.DHKey, []byte, time.Time,
		LightFileRequest,
	) (*http.Response, error)
}

// OpenRemotePanelFile opens a same-page file-manager request against an
// active paired Panel. The caller remains responsible for browser session and
// CSRF checks; this method only uses the stored v2 controller credential.
func (s *Service) OpenRemotePanelFile(
	ctx context.Context,
	hostID string,
	input LightFileRequest,
) (*http.Response, error) {
	if s == nil || !validID(hostID) || !validFileRelayRequest(input) {
		return nil, ErrFileRelayUnavailable
	}
	record, err := s.storeV2.Host(hostID)
	if err != nil || record.State != hostStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(record.Scope)) {
		return nil, ErrFileRelayUnavailable
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return nil, ErrFileRelayUnavailable
	}
	remote, ok := s.remoteV2.(remoteV2PanelFileAPI)
	if !ok {
		return nil, ErrProtocolMismatch
	}
	return remote.OpenFileRelayV2(
		ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
		noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input,
	)
}

// OpenFileRelayV2 starts the paired Panel side of the HTTP-like file relay.
// Each poll is a fresh authenticated Noise exchange, while the response body
// remains a normal io.ReadCloser for the Panel HTTP handler.
func (c *RemoteClient) OpenFileRelayV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input LightFileRequest,
) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !validFileRelayRequest(input) {
		return nil, ErrAuthentication
	}
	if input.Body == nil || input.Body == http.NoBody {
		input.BodyLength = 0
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	commandID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	command := FileRelayCommand{
		ID: commandID, Kind: "request", RequestID: sessionID,
		Method: input.Method, Path: input.Path, Query: input.RawQuery,
		Headers: cloneFileRelayHeaders(input.Headers), BodyLength: input.BodyLength,
		ExpiresAt: now.UTC().Add(lightFileCommandTTL).Unix(),
	}
	if err := validateFileRelayCommand(command, now.UTC()); err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	runContext, cancel := context.WithCancel(ctx)
	ready := make(chan fileRelayOpenResult, 1)
	go c.runFileRelay(runContext, cancel, origin, controllerID, targetID, controllerKey, targetPublicKey, now.UTC(), input, sessionID, command, writer, ready)
	select {
	case result := <-ready:
		if result.err != nil {
			cancel()
			_ = reader.Close()
			return nil, result.err
		}
		return &http.Response{
			StatusCode: result.status,
			Status:     fmt.Sprintf("%d", result.status),
			Header:     fileRelayHTTPHeaders(result.headers),
			Body:       &remoteFileRelayBody{reader: reader, cancel: cancel},
		}, nil
	case <-ctx.Done():
		cancel()
		_ = reader.Close()
		return nil, ctx.Err()
	}
}

type fileRelayOpenResult struct {
	status  int
	headers map[string]string
	err     error
}

func (c *RemoteClient) runFileRelay(
	ctx context.Context,
	cancel context.CancelFunc,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input LightFileRequest,
	sessionID string,
	initial FileRelayCommand,
	output *io.PipeWriter,
	ready chan<- fileRelayOpenResult,
) {
	defer cancel()
	readySent := false
	deliver := func(result fileRelayOpenResult) {
		if readySent {
			return
		}
		readySent = true
		ready <- result
	}
	fail := func(err error) {
		if err == nil {
			err = ErrFileRelayUnavailable
		}
		deliver(fileRelayOpenResult{err: err})
		_ = output.CloseWithError(err)
	}

	knownCommands := map[string]struct{}{initial.ID: {}}
	nextCommand := &initial
	accepted := false
	bodyDone := initial.BodyLength == 0
	bodyOffset := int64(0)
	responseSeen := false
	responseOffset := int64(0)
	for {
		pollInput := FileRelayPollRequest{SessionID: sessionID, Command: nextCommand}
		pollNow := time.Now().UTC()
		if pollNow.IsZero() {
			pollNow = now
		}
		response, err := c.pollFileRelayV2(
			ctx, origin, controllerID, targetID, controllerKey, targetPublicKey,
			pollNow, pollInput,
		)
		if err != nil {
			fail(err)
			return
		}
		nextCommand = nil
		for _, event := range response.Events {
			if event.RequestID != sessionID {
				fail(ErrAuthentication)
				return
			}
			switch event.Kind {
			case "accepted":
				if _, ok := knownCommands[event.CommandID]; !ok {
					fail(ErrAuthentication)
					return
				}
				if event.CommandID == initial.ID {
					accepted = true
				}
			case "response":
				if responseSeen {
					fail(ErrAuthentication)
					return
				}
				responseSeen = true
				deliver(fileRelayOpenResult{status: event.Status, headers: event.Headers})
			case "data":
				if !responseSeen || event.Offset != responseOffset {
					fail(ErrAuthentication)
					return
				}
				written, writeErr := output.Write(event.Data)
				if writeErr != nil || written != len(event.Data) {
					fail(contextError(ctx, writeErr))
					return
				}
				responseOffset += int64(written)
			case "end":
				if !responseSeen {
					fail(ErrAuthentication)
					return
				}
				_ = output.Close()
				return
			case "error":
				if event.Error == "" {
					fail(ErrAuthentication)
					return
				}
				err := errors.New(event.Error)
				if !responseSeen {
					deliver(fileRelayOpenResult{err: err})
				}
				_ = output.CloseWithError(err)
				return
			}
		}
		if !accepted {
			fail(ErrAuthentication)
			return
		}
		if !bodyDone {
			data, final, readErr := readFileRelayBodyChunk(input.Body, initial.BodyLength, bodyOffset)
			if readErr != nil {
				fail(readErr)
				return
			}
			bodyCommandID, idErr := randomHex(16)
			if idErr != nil {
				fail(idErr)
				return
			}
			bodyCommand := FileRelayCommand{
				ID: bodyCommandID, Kind: "body", RequestID: sessionID,
				Offset: bodyOffset, Data: data, Final: final,
				ExpiresAt: time.Now().UTC().Add(lightFileCommandTTL).Unix(),
			}
			if err := validateFileRelayCommand(bodyCommand, time.Now().UTC()); err != nil {
				fail(err)
				return
			}
			knownCommands[bodyCommand.ID] = struct{}{}
			nextCommand = &bodyCommand
			bodyOffset += int64(len(data))
			if final {
				bodyDone = true
			}
		}
	}
}

func readFileRelayBodyChunk(body io.Reader, length, offset int64) ([]byte, bool, error) {
	if body == nil || body == http.NoBody {
		return nil, true, nil
	}
	for {
		buffer := make([]byte, lightFileChunkBytes)
		count, err := body.Read(buffer)
		if count < 0 || count > len(buffer) {
			return nil, false, errors.New("file relay request body read is invalid")
		}
		if length >= 0 && offset+int64(count) > length {
			return nil, false, errors.New("file relay request body exceeds declared length")
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, err
		}
		if count == 0 && err == nil {
			continue
		}
		final := errors.Is(err, io.EOF) || (length >= 0 && offset+int64(count) == length)
		if final && length >= 0 && offset+int64(count) != length {
			return nil, false, errors.New("file relay request body is incomplete")
		}
		return append([]byte(nil), buffer[:count]...), final, nil
	}
}

func (c *RemoteClient) pollFileRelayV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input FileRelayPollRequest,
) (FileRelayPollResponse, error) {
	var output FileRelayPollResponse
	if c == nil || c.streamClient == nil || validatePanelFileRelayPoll(input, now.UTC()) != nil ||
		!fileRelayPayloadFits(input) {
		return output, ErrAuthentication
	}
	payload, err := jsonMarshalFileRelay(input)
	if err != nil {
		return output, err
	}
	requestID, err := randomHex(16)
	if err != nil {
		return output, err
	}
	envelope, handshake, err := sealV2Request(
		http.MethodPost, v2FileRelayPath,
		v2Envelope{Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: targetID, Timestamp: now.UTC().Unix(), RequestID: requestID},
		controllerKey, targetPublicKey, nil, payload,
	)
	if err != nil {
		return output, err
	}
	body, err := jsonMarshalV2Envelope(envelope)
	if err != nil {
		return output, err
	}
	request, err := c.newV2Request(ctx, http.MethodPost, origin, v2FileRelayPath, bytes.NewReader(body))
	if err != nil {
		return output, err
	}
	var responseEnvelope v2Envelope
	if err := c.doJSONWith(c.streamClient, request, maxV2EnvelopeBytes, &responseEnvelope); err != nil {
		return output, err
	}
	plaintext, err := openV2Response(envelope, responseEnvelope, handshake)
	if err != nil || len(plaintext) > MaxSummaryBytes || decodeV2Payload(plaintext, &output) != nil ||
		validatePanelFileRelayResponse(output) != nil {
		return output, ErrAuthentication
	}
	return output, nil
}

func jsonMarshalFileRelay(input FileRelayPollRequest) ([]byte, error) {
	payload, err := json.Marshal(input)
	if err != nil || len(payload) > MaxSummaryBytes {
		return nil, ErrAuthentication
	}
	return payload, nil
}

func jsonMarshalV2Envelope(envelope v2Envelope) ([]byte, error) {
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxV2EnvelopeBytes {
		return nil, ErrAuthentication
	}
	return body, nil
}

type remoteFileRelayBody struct {
	reader *io.PipeReader
	cancel context.CancelFunc
	once   sync.Once
}

func (b *remoteFileRelayBody) Read(output []byte) (int, error) {
	return b.reader.Read(output)
}

func (b *remoteFileRelayBody) Close() error {
	b.once.Do(func() {
		b.cancel()
		_ = b.reader.Close()
	})
	return nil
}
