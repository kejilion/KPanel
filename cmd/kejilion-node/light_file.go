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
	lightFileUnsupportedRetry = 5 * time.Minute
	lightFileFailureRetry     = 5 * time.Second
	lightFileEventLimit       = 16
	lightFileEventChunkBytes  = 16 << 10
)

type lightNodeFileSession struct {
	requestID  string
	ctx        context.Context
	cancel     context.CancelFunc
	bodyWriter *io.PipeWriter
	bodyOffset int64
	head       bool
	events     chan cluster.FileRelayEvent
	pending    []cluster.FileRelayEvent
	closeOnce  sync.Once
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
}

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
	}
	defer control.resetSessions()
	unsupportedUntil := time.Time{}
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Now().Before(unsupportedUntil) {
			if !waitContext(ctx, time.Until(unsupportedUntil)) {
				return
			}
			continue
		}

		events, _ := control.collectEvents()
		response, err := relay.PollV2(
			ctx, config.Origin, config.NodeID, config.TargetNodeID,
			identity.Key, identity.Peer, time.Now().UTC(),
			cluster.FileRelayPollRequest{RequestIDs: control.requestIDs(), Events: events},
		)
		if err != nil {
			if terminalRelayUnsupported(err) {
				unsupportedUntil = time.Now().Add(lightFileUnsupportedRetry)
			} else {
				slog.Warn("lightweight file relay failed", "error", err)
				if !waitContext(ctx, lightFileFailureRetry) {
					return
				}
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

func (control *lightFileControl) acceptRelayResponse(response cluster.FileRelayPollResponse) {
	control.pendingEvents = nil
	if response.Epoch == "" {
		return
	}
	if control.centerEpoch != "" && control.centerEpoch != response.Epoch {
		control.resetSessions()
	}
	control.centerEpoch = response.Epoch
}

func (control *lightFileControl) resetSessions() {
	for _, session := range control.sessions {
		session.close()
	}
	control.sessions = make(map[string]*lightNodeFileSession)
	control.pendingEvents = nil
	control.processed = make(map[string]cluster.FileRelayEvent)
	control.processedIDs = nil
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
	if len(control.pendingEvents) >= lightFileEventLimit {
		return append([]cluster.FileRelayEvent(nil), control.pendingEvents...), nil
	}
	ids := control.requestIDs()
	for _, requestID := range ids {
		if len(control.pendingEvents) >= lightFileEventLimit {
			break
		}
		session := control.sessions[requestID]
		if session == nil {
			continue
		}
		session.drain()
		for len(session.pending) > 0 && len(control.pendingEvents) < lightFileEventLimit {
			candidate := session.pending[0]
			if !control.pollPayloadFits(append(append([]cluster.FileRelayEvent(nil), control.pendingEvents...), candidate), ids) {
				break
			}
			control.pendingEvents = append(control.pendingEvents, candidate)
			session.pending = session.pending[1:]
			if candidate.Kind == "end" {
				delete(control.sessions, requestID)
				session.close()
				break
			}
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
	requestContext, cancel := context.WithCancel(control.ctx)
	var body io.Reader = http.NoBody
	var bodyWriter *io.PipeWriter
	if command.BodyLength != 0 {
		reader, writer := io.Pipe()
		body, bodyWriter = reader, writer
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
		events: make(chan cluster.FileRelayEvent, 32),
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
	if session == nil || session.bodyWriter == nil || command.Offset != session.bodyOffset {
		return errors.New("file relay request body is unavailable")
	}
	if len(command.Data) > 0 {
		count, err := session.bodyWriter.Write(command.Data)
		if err != nil || count != len(command.Data) {
			if err != nil {
				return err
			}
			return errors.New("file relay request body write was incomplete")
		}
		session.bodyOffset += int64(count)
	}
	if command.Final {
		_ = session.bodyWriter.Close()
		session.bodyWriter = nil
	}
	event := acceptedFileCommand(command)
	control.rememberCommandResult(command.ID, event)
	if !control.queueEvent(event) {
		return errors.New("file relay event queue is full")
	}
	return nil
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
	writer.send(cluster.FileRelayEvent{RequestID: writer.session.requestID, Kind: "response", Status: status, Headers: headers})
}

func (writer *lightFileResponseWriter) Write(data []byte) (int, error) {
	written := len(data)
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.session.head {
		return len(data), nil
	}
	for len(data) > 0 {
		size := min(len(data), lightFileEventChunkBytes)
		chunk := append([]byte(nil), data[:size]...)
		writer.send(cluster.FileRelayEvent{RequestID: writer.session.requestID, Kind: "data", Offset: writer.offset, Data: chunk})
		writer.offset += int64(size)
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

func (writer *lightFileResponseWriter) send(event cluster.FileRelayEvent) {
	select {
	case writer.session.events <- event:
	case <-writer.session.ctx.Done():
	}
}

func (session *lightNodeFileSession) drain() {
	for {
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
	if len(control.pendingEvents) >= lightFileEventLimit {
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
	input := cluster.FileRelayPollRequest{RequestIDs: requestIDs, Events: events}
	content, err := jsonMarshalFileRelayPoll(input)
	return err == nil && len(content) <= cluster.MaxSummaryBytes
}

func jsonMarshalFileRelayPoll(input cluster.FileRelayPollRequest) ([]byte, error) {
	return json.Marshal(input)
}
