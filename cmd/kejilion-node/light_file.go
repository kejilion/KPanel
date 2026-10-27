package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

const (
	lightFileUnsupportedRetryMin = time.Second
	lightFileUnsupportedRetryMax = 5 * time.Minute
	lightFileFailureRetry        = time.Minute
	lightFileEventLimit          = 16
	// Match the full Panel relay's maximum session lifetime, including while
	// the center is disconnected and cannot send a cancellation.
	lightFileSessionTTL = 2 * time.Hour
	// Two chunks plus event metadata fit in the existing 64 KiB JSON envelope.
	lightFileEventChunkBytes = 23 << 10
)

type lightNodeFileSession struct {
	requestID   string
	ctx         context.Context
	cancel      context.CancelFunc
	bodyWriter  *io.PipeWriter
	bodyOffset  int64
	bodyDiscard bool
	bodyCommand *cluster.FileRelayCommand
	bodyResult  chan lightFileBodyResult
	handlerDone chan struct{}
	head        bool
	events      chan cluster.FileRelayEvent
	pending     []cluster.FileRelayEvent
	closeOnce   sync.Once
}

type lightFileBodyResult struct {
	written int
	err     error
}

type lightFileControl struct {
	config        nodeConfig
	identity      terminalIdentity
	relay         *cluster.FileRelayClient
	handler       http.Handler
	ctx           context.Context
	sessions      map[string]*lightNodeFileSession
	pendingEvents []cluster.FileRelayEvent
	processed     map[string]cluster.FileRelayEvent
	processedIDs  []string
	centerEpoch   string
	unsupported   uint8
	failures      uint8
	lastCollected string
	flowMode      uint8
}

const (
	flowLegacy uint8 = iota
	flowProbe
	flowSelective
)

func runLightFileControl(
	ctx context.Context,
	config nodeConfig,
	identity terminalIdentity,
	relay *cluster.FileRelayClient,
	handler http.Handler,
) {
	if relay == nil || handler == nil {
		return
	}
	control := &lightFileControl{
		config: config, identity: identity, relay: relay, handler: handler, ctx: ctx,
		sessions:  make(map[string]*lightNodeFileSession),
		processed: make(map[string]cluster.FileRelayEvent),
		flowMode:  flowProbe,
	}
	defer control.resetSessions()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := requireWindowsCapability(ctx, config, "files"); err != nil {
			control.resetSessions()
			if !waitContext(ctx, time.Minute) {
				return
			}
			continue
		}
		events, _ := control.collectEvents()
		response, err := relay.PollV2(
			ctx, config.Origin, config.NodeID, config.TargetNodeID,
			identity.Key, identity.Peer, time.Now().UTC(),
			cluster.FileRelayPollRequest{RequestIDs: control.requestIDs(), Events: control.wireEvents(events)},
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			delay := control.retryDelay(err)
			slog.Warn("lightweight file relay failed", "error", err, "retryAfter", delay)
			if !waitContext(ctx, delay) {
				return
			}
			continue
		}
		control.acceptRelayResponse(response)
		if response.Command == nil {
			continue
		}
		if err := control.executeCommand(*response.Command); err != nil {
			control.queueCommandError(*response.Command, err)
		}
	}
}

func (control *lightFileControl) retryDelay(err error) time.Duration {
	// An HTTP 404/405/426 before the first authenticated relay response can
	// mean either an old center or a transient reverse-proxy response. Start
	// with a short reconnect and exponentially settle at the legacy discovery
	// interval. This keeps an unsupported center quiet without turning one
	// cold-path proxy failure into a five-minute first connection.
	if terminalRelayUnsupported(err) && control.centerEpoch == "" {
		delay := lightFileUnsupportedRetryMin << min(control.unsupported, 9)
		if delay > lightFileUnsupportedRetryMax {
			delay = lightFileUnsupportedRetryMax
		}
		if control.unsupported < 9 {
			control.unsupported++
		}
		return delay
	}
	// Recover one transient loss promptly, but back off persistent failures.
	// Pending events and command IDs are retained until authenticated success.
	delay := time.Second << min(control.failures, 6)
	if control.failures < 6 {
		control.failures++
	}
	return min(delay, lightFileFailureRetry)
}

func (control *lightFileControl) acceptRelayResponse(response cluster.FileRelayPollResponse) {
	control.unsupported = 0
	control.failures = 0
	if response.Epoch != "" && control.centerEpoch != "" && control.centerEpoch != response.Epoch {
		control.resetSessions()
		control.flowMode = flowProbe
		control.centerEpoch = response.Epoch
		return
	}
	marker := false
	for _, event := range response.Events {
		if cluster.IsFileRelayFlowMarker(event) {
			marker = true
		}
	}
	if control.flowMode == flowProbe && !marker {
		control.flowMode = flowLegacy
	} else if control.flowMode != flowLegacy && marker {
		control.flowMode = flowSelective
	} else if control.flowMode == flowSelective && !marker {
		// Do not silently acknowledge data if a capable peer stops returning
		// receipts. End the old sessions and probe the next poll instead.
		control.resetSessions()
		control.flowMode = flowProbe
		control.centerEpoch = response.Epoch
		return
	}
	pending := control.pendingEvents
	control.pendingEvents = nil
	blocked := make(map[string]bool)
	var unacknowledged []cluster.FileRelayEvent
	for index, event := range pending {
		acknowledged := control.flowMode == flowLegacy
		if !acknowledged {
			want := cluster.FileRelayFlowReceipt(index, event.RequestID)
			for _, receipt := range response.Events {
				if receipt.Kind == want.Kind && receipt.CommandID == want.CommandID && receipt.RequestID == want.RequestID &&
					receipt.Status == 0 && receipt.Offset == 0 && len(receipt.Data) == 0 && len(receipt.Headers) == 0 && receipt.Error == "" {
					acknowledged = true
					break
				}
			}
		}
		if blocked[event.RequestID] {
			acknowledged = false
		}
		session := control.sessions[event.RequestID]
		if acknowledged {
			if event.Kind == "end" && session != nil {
				delete(control.sessions, event.RequestID)
				session.close()
			}
		} else {
			blocked[event.RequestID] = true
			unacknowledged = append(unacknowledged, event)
		}
	}
	// Prepend in reverse order so several retried chunks keep their original
	// order ahead of newly produced output.
	for i := len(unacknowledged) - 1; i >= 0; i-- {
		event := unacknowledged[i]
		if session := control.sessions[event.RequestID]; session != nil {
			session.pending = append([]cluster.FileRelayEvent{event}, session.pending...)
		} else {
			control.pendingEvents = append([]cluster.FileRelayEvent{event}, control.pendingEvents...)
		}
	}
	if response.Epoch != "" {
		control.centerEpoch = response.Epoch
	}
}

func (control *lightFileControl) wireEvents(events []cluster.FileRelayEvent) []cluster.FileRelayEvent {
	if control.flowMode == flowLegacy {
		return events
	}
	return append(append([]cluster.FileRelayEvent(nil), events...), cluster.FileRelayFlowMarker())
}

func (control *lightFileControl) eventLimit() int {
	if control.flowMode == flowLegacy {
		return lightFileEventLimit
	}
	return lightFileEventLimit - 1 // reserve space for negotiation
}

func (control *lightFileControl) resetSessions() {
	for _, session := range control.sessions {
		session.close()
	}
	control.sessions = make(map[string]*lightNodeFileSession)
	control.pendingEvents = nil
	control.processed = make(map[string]cluster.FileRelayEvent)
	control.processedIDs = nil
	control.lastCollected = ""
}

func (control *lightFileControl) requestIDs() []string {
	ids := make([]string, 0, len(control.sessions))
	for id := range control.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (control *lightFileControl) collectEvents() ([]cluster.FileRelayEvent, error) {
	for id, session := range control.sessions {
		if session.ctx != nil && session.ctx.Err() != nil {
			session.close()
			delete(control.sessions, id)
		}
	}
	if (control.flowMode != flowLegacy && len(control.pendingEvents) > 0) || len(control.pendingEvents) >= control.eventLimit() {
		return append([]cluster.FileRelayEvent(nil), control.pendingEvents...), nil
	}
	ids := control.requestIDs()
	// Rotate the starting session as well as taking only one event per session
	// per pass. A full data batch must not keep a later directory request waiting
	// until a large download finishes.
	start := sort.SearchStrings(ids, control.lastCollected)
	if start < len(ids) && ids[start] == control.lastCollected {
		start++
	}
	used := make(map[string]int)
	for len(control.pendingEvents) < control.eventLimit() {
		progressed := false
		for index := 0; index < len(ids) && len(control.pendingEvents) < control.eventLimit(); index++ {
			requestID := ids[(start+index)%len(ids)]
			// Preserve the existing two-chunk single-stream throughput while
			// giving separate active streams one event each before another pass.
			perSessionLimit := 1
			if len(ids) == 1 {
				perSessionLimit = 2
			}
			if control.flowMode != flowLegacy && used[requestID] >= perSessionLimit {
				continue
			}
			session := control.sessions[requestID]
			if session == nil {
				continue
			}
			session.drain()
			control.collectBodyResult(session)
			if len(session.pending) == 0 {
				continue
			}
			candidate := session.pending[0]
			if !control.pollPayloadFits(append(append([]cluster.FileRelayEvent(nil), control.pendingEvents...), candidate), ids) {
				continue
			}
			control.pendingEvents = append(control.pendingEvents, candidate)
			session.pending[0] = cluster.FileRelayEvent{}
			session.pending = session.pending[1:]
			control.lastCollected = requestID
			used[requestID]++
			progressed = true
			if candidate.Kind == "end" && control.flowMode == flowLegacy {
				delete(control.sessions, requestID)
				session.close()
			}
		}
		if !progressed {
			break
		}
	}
	return append([]cluster.FileRelayEvent(nil), control.pendingEvents...), nil
}

func (control *lightFileControl) executeCommand(command cluster.FileRelayCommand) error {
	if event, ok := control.processed[command.ID]; ok {
		control.queueEvent(event)
		return nil
	}
	switch command.Kind {
	case "request":
		return control.startRequest(command)
	case "body":
		return control.writeRequestBody(command)
	case "cancel":
		return control.cancelRequest(command)
	default:
		return errors.New("unsupported file relay command")
	}
}

func (control *lightFileControl) startRequest(command cluster.FileRelayCommand) error {
	if existing := control.sessions[command.RequestID]; existing != nil {
		event := acceptedFileCommand(command)
		control.rememberCommandResult(command.ID, event)
		control.queueEvent(event)
		return nil
	}
	if len(control.sessions) >= cluster.MaxFileRelayRequests {
		return errors.New("file relay request session limit reached")
	}
	requestContext, cancel := context.WithTimeout(control.ctx, lightFileSessionTTL)
	if command.Path == cluster.HistoryPath {
		cancel()
		requestContext, cancel = context.WithTimeout(control.ctx, cluster.HistoryTimeout)
	}
	var body io.Reader = http.NoBody
	var bodyWriter *io.PipeWriter
	if command.BodyLength != 0 {
		reader, writer := io.Pipe()
		body, bodyWriter = reader, writer
		// io.Pipe does not observe the request context itself. Capture this
		// immutable writer so parent cancellation releases both reads and writes.
		context.AfterFunc(requestContext, func() { _ = writer.CloseWithError(requestContext.Err()) })
	}
	requestURL := &url.URL{Scheme: "http", Host: "light-node", Path: command.Path, RawQuery: command.Query}
	request, err := http.NewRequestWithContext(requestContext, command.Method, requestURL.String(), body)
	if err != nil {
		cancel()
		return err
	}
	request.ContentLength = command.BodyLength
	for key, value := range command.Headers {
		request.Header.Set(key, value)
	}
	session := &lightNodeFileSession{
		requestID: command.RequestID, ctx: requestContext, cancel: cancel,
		bodyWriter: bodyWriter, head: command.Method == http.MethodHead,
		events:     make(chan cluster.FileRelayEvent, lightFileEventLimit),
		bodyResult: make(chan lightFileBodyResult, 1), handlerDone: make(chan struct{}),
	}
	control.sessions[command.RequestID] = session
	go serveLightFileRequest(control.handler, request, session)
	event := acceptedFileCommand(command)
	control.rememberCommandResult(command.ID, event)
	if !control.queueEvent(event) {
		return errors.New("file relay event queue is full")
	}
	return nil
}

func (control *lightFileControl) writeRequestBody(command cluster.FileRelayCommand) error {
	session := control.sessions[command.RequestID]
	if session != nil && session.bodyCommand != nil {
		if session.bodyCommand.ID == command.ID {
			return nil // Retry while the same bounded write is still pending.
		}
		return errors.New("file relay request body write is pending")
	}
	if session == nil || command.Offset != session.bodyOffset {
		return errors.New("file relay request body is unavailable")
	}
	if err := session.ctx.Err(); err != nil {
		return err
	}
	if session.bodyDiscard {
		session.bodyOffset += int64(len(command.Data))
		event := acceptedFileCommand(command)
		control.rememberCommandResult(command.ID, event)
		if !control.queueEvent(event) {
			return errors.New("file relay event queue is full")
		}
		return nil
	}
	if session.bodyWriter == nil {
		return errors.New("file relay request body is unavailable")
	}
	// Only the control loop owns offsets, replay receipts and session maps.
	// One worker per session keeps a slow handler from blocking polls/cancel,
	// without acknowledging bytes before consumption or buffering an upload.
	command.Data = append([]byte(nil), command.Data...)
	session.bodyCommand = &command
	bodyWriter := session.bodyWriter
	go func() {
		count, err := bodyWriter.Write(command.Data)
		if err == nil && command.Final {
			err = bodyWriter.Close()
		}
		session.bodyResult <- lightFileBodyResult{written: count, err: err}
	}()
	return nil
}

func (control *lightFileControl) collectBodyResult(session *lightNodeFileSession) {
	if session.bodyCommand == nil || len(session.pending) >= lightFileEventLimit {
		return
	}
	select {
	case result := <-session.bodyResult:
		command := *session.bodyCommand
		session.bodyCommand = nil
		event := acceptedFileCommand(command)
		// A handler may deliberately return 4xx without consuming the body.
		// Preserve that response rather than replacing it with a pipe error.
		finished := false
		select {
		case <-session.handlerDone:
			finished = true
		default:
		}
		if (result.err != nil || result.written != len(command.Data)) && !finished {
			event.Kind, event.Error = "error", "file relay request body write failed"
			session.close()
		} else {
			if finished {
				// The remainder is discarded after an intentional early response.
				// Acknowledged offsets must still match the sender's next block.
				session.bodyOffset += int64(len(command.Data))
				session.bodyDiscard = !command.Final
			} else {
				session.bodyOffset += int64(result.written)
			}
			if command.Final || finished {
				session.bodyWriter = nil
			}
		}
		control.rememberCommandResult(command.ID, event)
		session.pending = append(session.pending, event)
	default:
	}
}

func (control *lightFileControl) cancelRequest(command cluster.FileRelayCommand) error {
	if session := control.sessions[command.RequestID]; session != nil {
		session.close()
		delete(control.sessions, command.RequestID)
	}
	event := acceptedFileCommand(command)
	control.rememberCommandResult(command.ID, event)
	control.queueEvent(event)
	return nil
}

func acceptedFileCommand(command cluster.FileRelayCommand) cluster.FileRelayEvent {
	return cluster.FileRelayEvent{CommandID: command.ID, RequestID: command.RequestID, Kind: "accepted"}
}

func serveLightFileRequest(handler http.Handler, request *http.Request, session *lightNodeFileSession) {
	writer := &lightFileResponseWriter{session: session, headers: make(http.Header)}
	defer func() {
		// Direct handler invocation does not provide net/http's usual request
		// body cleanup. Unblock a pending body command if the handler exits early.
		close(session.handlerDone)
		_ = request.Body.Close()
		if recover() != nil && writer.status == 0 {
			writer.WriteHeader(http.StatusInternalServerError)
		}
		writer.finish()
	}()
	handler.ServeHTTP(writer, request)
}

type lightFileResponseWriter struct {
	session *lightNodeFileSession
	headers http.Header
	status  int
	offset  int64
	ended   bool
	err     error
}

func (writer *lightFileResponseWriter) Header() http.Header {
	return writer.headers
}

func (writer *lightFileResponseWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	if status < 100 || status > 999 {
		status = http.StatusInternalServerError
	}
	writer.status = status
	headers := make(map[string]string, len(writer.headers))
	for key, values := range writer.headers {
		if len(values) > 0 && values[0] != "" {
			headers[key] = values[0]
		}
	}
	writer.err = writer.send(cluster.FileRelayEvent{RequestID: writer.session.requestID, Kind: "response", Status: status, Headers: headers})
}

func (writer *lightFileResponseWriter) Write(data []byte) (int, error) {
	written := 0
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.err != nil {
		return 0, writer.err
	}
	if writer.session.head {
		return len(data), nil
	}
	for len(data) > 0 {
		size := min(len(data), lightFileEventChunkBytes)
		chunk := append([]byte(nil), data[:size]...)
		if err := writer.send(cluster.FileRelayEvent{RequestID: writer.session.requestID, Kind: "data", Offset: writer.offset, Data: chunk}); err != nil {
			writer.err = err
			return written, err
		}
		writer.offset += int64(size)
		written += size
		data = data[size:]
	}
	return written, nil
}

func (writer *lightFileResponseWriter) finish() {
	if writer.ended {
		return
	}
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	writer.ended = true
	writer.send(cluster.FileRelayEvent{RequestID: writer.session.requestID, Kind: "end"})
}

func (writer *lightFileResponseWriter) send(event cluster.FileRelayEvent) error {
	if err := writer.session.ctx.Err(); err != nil {
		return err
	}
	select {
	case writer.session.events <- event:
		return nil
	case <-writer.session.ctx.Done():
		return writer.session.ctx.Err()
	}
}

func (session *lightNodeFileSession) drain() {
	for len(session.pending) < lightFileEventLimit {
		select {
		case event := <-session.events:
			session.pending = append(session.pending, event)
		default:
			return
		}
	}
}

func (session *lightNodeFileSession) close() {
	session.closeOnce.Do(func() {
		if session.bodyWriter != nil {
			_ = session.bodyWriter.CloseWithError(context.Canceled)
		}
		if session.cancel != nil {
			session.cancel()
		}
	})
}

func (control *lightFileControl) rememberCommandResult(commandID string, event cluster.FileRelayEvent) {
	if _, exists := control.processed[commandID]; exists {
		return
	}
	control.processed[commandID] = event
	control.processedIDs = append(control.processedIDs, commandID)
	if len(control.processedIDs) <= 128 {
		return
	}
	delete(control.processed, control.processedIDs[0])
	control.processedIDs = control.processedIDs[1:]
}

func (control *lightFileControl) queueCommandError(command cluster.FileRelayCommand, err error) {
	message := "file relay command failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = strings.TrimSpace(err.Error())
	}
	if len(message) > 512 {
		message = message[:512]
	}
	event := cluster.FileRelayEvent{CommandID: command.ID, RequestID: command.RequestID, Kind: "error", Error: message}
	control.rememberCommandResult(command.ID, event)
	control.queueEvent(event)
}

func (control *lightFileControl) queueEvent(event cluster.FileRelayEvent) bool {
	if len(control.pendingEvents) >= control.eventLimit() {
		return false
	}
	candidate := append(append([]cluster.FileRelayEvent(nil), control.pendingEvents...), event)
	if !control.pollPayloadFits(candidate, control.requestIDs()) {
		return false
	}
	control.pendingEvents = append(control.pendingEvents, event)
	return true
}

func (control *lightFileControl) pollPayloadFits(events []cluster.FileRelayEvent, requestIDs []string) bool {
	input := cluster.FileRelayPollRequest{RequestIDs: requestIDs, Events: control.wireEvents(events)}
	content, err := jsonMarshalFileRelayPoll(input)
	return err == nil && len(content) <= cluster.MaxSummaryBytes
}

func jsonMarshalFileRelayPoll(input cluster.FileRelayPollRequest) ([]byte, error) {
	return json.Marshal(input)
}
