package panel

// The terminal stream pushes output for every terminal a browser tab shows
// (host terminals, batch terminals and task terminals) over one Server-Sent
// Events response. Pumps keep long-polling the same backends as the polling
// endpoints, so the browser neither waits a round trip between reads nor
// spends one of its six HTTP/1.1 connections per terminal. Subscriptions are
// changed through a CSRF-protected POST; session IDs never appear in URLs.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	terminalStreamPath              = "/api/v1/terminal-stream"
	terminalStreamSubscriptionsPath = "/api/v1/terminal-stream/subscriptions"

	maxTerminalStreams             = 32
	maxTerminalStreamsPerUser      = 8
	maxTerminalStreamSubscriptions = 16
	terminalStreamEventBuffer      = 32

	terminalStreamWriteTimeout  = 20 * time.Second
	terminalStreamCoalesceDelay = 4 * time.Millisecond
	terminalStreamCoalesceBytes = 32 << 10
)

// terminalStreamHeartbeat also bounds how quickly a revoked session stops
// receiving output.
var terminalStreamHeartbeat = 15 * time.Second

var jobTerminalAgentPrefixes = map[string]string{
	"app":         "/v1/app-jobs/",
	"site":        "/v1/site-installations/",
	"diagnostic":  "/v1/diagnostic-jobs/",
	"environment": "/v1/web-environment/jobs/",
}

type terminalStreamHub struct {
	mu      sync.Mutex
	streams map[string]*terminalStream
}

type terminalStream struct {
	id        string
	userID    string
	tokenHash [32]byte
	ctx       context.Context
	cancel    context.CancelFunc
	events    chan terminalStreamEvent
	mu        sync.Mutex
	subs      map[string]context.CancelFunc
}

type jobTerminalChunk struct {
	DataBase64 string `json:"dataBase64"`
	NextOffset int64  `json:"nextOffset"`
	InputOpen  bool   `json:"inputOpen"`
	Finished   bool   `json:"finished"`
}

type terminalStreamEvent struct {
	Key    string            `json:"key"`
	Output *terminal.Output  `json:"output,omitempty"`
	Job    *jobTerminalChunk `json:"job,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type terminalStreamSubscription struct {
	Kind      string `json:"kind"`
	Job       string `json:"job,omitempty"`
	ID        string `json:"id"`
	Offset    int64  `json:"offset"`
	InputOpen bool   `json:"inputOpen,omitempty"`
}

type terminalStreamSubscriptionRequest struct {
	StreamID string                       `json:"streamId"`
	Add      []terminalStreamSubscription `json:"add"`
	Remove   []string                     `json:"remove"`
}

func terminalStreamKey(item terminalStreamSubscription) string {
	if item.Kind == "job" {
		return "job:" + item.Job + ":" + item.ID
	}
	return "terminal:" + item.ID
}

func newTerminalStreamHub() *terminalStreamHub {
	return &terminalStreamHub{streams: make(map[string]*terminalStream)}
}

func (h *terminalStreamHub) register(userID string, tokenHash [32]byte) (*terminalStream, bool) {
	id, err := randomTerminalSessionID()
	if err != nil {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	perUser := 0
	for _, stream := range h.streams {
		if stream.userID == userID {
			perUser++
		}
	}
	if len(h.streams) >= maxTerminalStreams || perUser >= maxTerminalStreamsPerUser {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &terminalStream{id: id, userID: userID, tokenHash: tokenHash, ctx: ctx, cancel: cancel,
		events: make(chan terminalStreamEvent, terminalStreamEventBuffer), subs: make(map[string]context.CancelFunc)}
	h.streams[id] = stream
	return stream, true
}

func (h *terminalStreamHub) remove(stream *terminalStream) {
	stream.cancel()
	h.mu.Lock()
	if h.streams[stream.id] == stream {
		delete(h.streams, stream.id)
	}
	h.mu.Unlock()
}

func (h *terminalStreamHub) lookup(id, userID string, tokenHash [32]byte) *terminalStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	stream := h.streams[id]
	if stream == nil || stream.userID != userID || stream.tokenHash != tokenHash || stream.ctx.Err() != nil {
		return nil
	}
	return stream
}

func (h *terminalStreamHub) closeAll() {
	h.mu.Lock()
	streams := h.streams
	h.streams = make(map[string]*terminalStream)
	h.mu.Unlock()
	for _, stream := range streams {
		stream.cancel()
	}
}

func (t *terminalStream) emit(event terminalStreamEvent) bool {
	select {
	case t.events <- event:
		return true
	case <-t.ctx.Done():
		return false
	}
}

func (s *Server) handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	token, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.URL.Path == terminalStreamSubscriptionsPath {
		s.updateTerminalStreamSubscriptions(w, r, token, session)
		return
	}
	if r.Method != http.MethodGet || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
		return
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		s.writeProblem(w, r, http.StatusInternalServerError, "stream_unavailable", "Streaming unavailable", "")
		return
	}
	stream, ok := s.terminalStreams.register(session.User.ID, sha256.Sum256([]byte(token)))
	if !ok {
		s.writeProblem(w, r, http.StatusTooManyRequests, "terminal_stream_limit", "Terminal stream limit reached", "")
		return
	}
	defer s.terminalStreams.remove(stream)
	stop := context.AfterFunc(r.Context(), stream.cancel)
	defer stop()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	write := func(event string, value any) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(terminalStreamWriteTimeout))
		var payload bytes.Buffer
		if value != nil {
			data, err := json.Marshal(value)
			if err != nil {
				return false
			}
			payload.WriteString("event: ")
			payload.WriteString(event)
			payload.WriteString("\ndata: ")
			payload.Write(data)
			payload.WriteString("\n\n")
		} else {
			payload.WriteString(": heartbeat\n\n")
		}
		if _, err := w.Write(payload.Bytes()); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !write("ready", map[string]string{"streamId": stream.id}) {
		return
	}
	heartbeat := time.NewTicker(terminalStreamHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-stream.ctx.Done():
			return
		case event := <-stream.events:
			if !write("output", event) {
				return
			}
		case <-heartbeat.C:
			// Logout, password change or expiry revoke the session; the stream
			// must stop delivering output as soon as the next heartbeat.
			if _, err := s.auth.Authenticate(token); err != nil || time.Now().After(session.ExpiresAt) {
				_ = write("auth.expired", map[string]string{"message": "Session expired"})
				return
			}
			if !write("", nil) {
				return
			}
		}
	}
}

func (s *Server) updateTerminalStreamSubscriptions(w http.ResponseWriter, r *http.Request, token string, session auth.Session) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
		return
	}
	if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
		return
	}
	userID := session.User.ID
	var input terminalStreamSubscriptionRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if len(input.Add) > maxTerminalStreamSubscriptions || len(input.Remove) > maxTerminalStreamSubscriptions {
		s.writeValidationProblem(w, r, "subscriptions", "too many subscription changes")
		return
	}
	stream := s.terminalStreams.lookup(input.StreamID, userID, sha256.Sum256([]byte(token)))
	if stream == nil {
		s.writeProblem(w, r, http.StatusNotFound, "terminal_stream_not_found", "Terminal stream not found", "")
		return
	}
	for _, item := range input.Add {
		if !s.validTerminalStreamSubscription(item, userID) {
			s.writeValidationProblem(w, r, "add", "invalid terminal subscription")
			return
		}
	}
	stream.mu.Lock()
	for _, key := range input.Remove {
		if cancel := stream.subs[key]; cancel != nil {
			cancel()
			delete(stream.subs, key)
		}
	}
	added := 0
	for _, item := range input.Add {
		key := terminalStreamKey(item)
		if _, exists := stream.subs[key]; !exists && len(stream.subs) >= maxTerminalStreamSubscriptions {
			break
		}
		if cancel := stream.subs[key]; cancel != nil {
			cancel()
		}
		ctx, cancel := context.WithCancel(stream.ctx)
		stream.subs[key] = cancel
		added++
		if item.Kind == "job" {
			go s.pumpJobTerminal(ctx, stream, key, item)
		} else {
			go s.pumpHostTerminal(ctx, stream, key, item)
		}
	}
	stream.mu.Unlock()
	if added < len(input.Add) {
		s.writeProblem(w, r, http.StatusTooManyRequests, "terminal_stream_limit", "Terminal subscription limit reached", "")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
}

func (s *Server) validTerminalStreamSubscription(item terminalStreamSubscription, userID string) bool {
	if item.Offset < 0 || len(item.ID) == 0 || len(item.ID) > 128 {
		return false
	}
	switch item.Kind {
	case "terminal":
		if item.Job != "" {
			return false
		}
		s.terminalMu.Lock()
		session, ok := s.terminalSessions[item.ID]
		s.terminalMu.Unlock()
		return ok && session.UserID == userID
	case "job":
		_, known := jobTerminalAgentPrefixes[item.Job]
		return known && siteIDPattern.MatchString(item.ID)
	default:
		return false
	}
}

// touchTerminalSession keeps the Panel index alive exactly like polling did
// and returns the current session for the subscribed user.
func (s *Server) touchTerminalSession(id, userID string) (panelTerminalSession, bool) {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	item, ok := s.terminalSessions[id]
	if !ok || item.UserID != userID {
		return panelTerminalSession{}, false
	}
	item.UpdatedAt = time.Now().UTC()
	s.terminalSessions[id] = item
	return item, true
}

func waitTerminalStreamBackoff(ctx context.Context, failures int) bool {
	delay := time.Duration(min(failures, 10)) * 500 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Server) pumpHostTerminal(ctx context.Context, stream *terminalStream, key string, item terminalStreamSubscription) {
	offset := item.Offset
	failures := 0
	var sentExit, sentClosed bool
	for ctx.Err() == nil {
		session, ok := s.touchTerminalSession(item.ID, stream.userID)
		if !ok {
			stream.emit(terminalStreamEvent{Key: key, Error: "terminal_not_found"})
			return
		}
		output, err := s.outputTerminalBackend(ctx, session, offset, time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
				s.deleteFinishedTerminalSession(item.ID)
				stream.emit(terminalStreamEvent{Key: key, Error: "terminal_not_found"})
				return
			}
			failures++
			if failures == 1 && !stream.emit(terminalStreamEvent{Key: key, Error: "terminal_output_failed"}) {
				return
			}
			if !waitTerminalStreamBackoff(ctx, failures) {
				return
			}
			continue
		}
		failures = 0
		finished := output.Closed || output.ExitedAt != nil
		if len(output.Data) > 0 && len(output.Data) < terminalStreamCoalesceBytes && !finished {
			timer := time.NewTimer(terminalStreamCoalesceDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if more, moreErr := s.outputTerminalBackend(ctx, session, output.NextOffset, 0); moreErr == nil &&
				more.Offset == output.NextOffset && !more.Truncated && len(output.Data)+len(more.Data) <= terminal.MaxOutputBytes {
				output.Data = append(output.Data, more.Data...)
				output.NextOffset = more.NextOffset
				output.ExitedAt, output.ExitError, output.Closed = more.ExitedAt, more.ExitError, more.Closed
				finished = output.Closed || output.ExitedAt != nil
			}
		}
		changed := len(output.Data) > 0 || output.Truncated ||
			(output.ExitedAt != nil && !sentExit) || (output.Closed && !sentClosed)
		if changed {
			value := output
			if !stream.emit(terminalStreamEvent{Key: key, Output: &value}) {
				return
			}
			sentExit = sentExit || output.ExitedAt != nil
			sentClosed = sentClosed || output.Closed
		}
		offset = output.NextOffset
		if finished && len(output.Data) == 0 {
			s.deleteFinishedTerminalSession(item.ID)
			return
		}
	}
}

func (s *Server) pumpJobTerminal(ctx context.Context, stream *terminalStream, key string, item terminalStreamSubscription) {
	prefix := jobTerminalAgentPrefixes[item.Job]
	offset, inputOpen := item.Offset, item.InputOpen
	first := true
	failures := 0
	for ctx.Err() == nil {
		query := url.Values{"offset": {strconv.FormatInt(offset, 10)}, "wait": {"1000"}, "inputOpen": {strconv.FormatBool(inputOpen)}}
		response, err := s.hostOps.Get(ctx, prefix+item.ID+"/terminal", query.Encode(), newRequestID())
		var chunk jobTerminalChunk
		if err == nil && response.StatusCode == http.StatusNotFound {
			stream.emit(terminalStreamEvent{Key: key, Error: "terminal_not_found"})
			return
		}
		if err == nil && (response.StatusCode < 200 || response.StatusCode >= 300 || json.Unmarshal(response.Body, &chunk) != nil) {
			err = fmt.Errorf("job terminal returned %d", response.StatusCode)
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			if failures == 1 && !stream.emit(terminalStreamEvent{Key: key, Error: "terminal_output_failed"}) {
				return
			}
			if !waitTerminalStreamBackoff(ctx, failures) {
				return
			}
			continue
		}
		failures = 0
		if first || chunk.DataBase64 != "" || chunk.InputOpen != inputOpen || chunk.Finished || chunk.NextOffset != offset {
			value := chunk
			if !stream.emit(terminalStreamEvent{Key: key, Job: &value}) {
				return
			}
		}
		first = false
		offset, inputOpen = chunk.NextOffset, chunk.InputOpen
		if chunk.Finished {
			return
		}
	}
}
