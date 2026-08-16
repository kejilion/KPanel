package agent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// browseWSManager relays WebSocket traffic a browsed page opens to a target
// site. Unlike browseFetch (one buffered request/response), a WS connection
// is genuinely long-lived and stateful, so this mirrors internal/terminal's
// Manager shape (bounded session count, idle/lifetime reaping, sliding
// output window with long-poll, owner-scoped sessions) rather than
// browseFetch's one-shot call.
//
// Owner scoping mirrors terminal's exact reasoning: session IDs are
// unguessable 128-bit random values, but that alone is defense-in-depth, not
// authorization — two different Panel users (or two different federation
// controllers) share this one Agent's bearer token, so the Agent itself must
// still refuse to let one poll or write into the other's session even if the
// ID somehow leaked (logs, timing, a referrer header). The caller (Panel)
// passes "panel:<userId>" or "federation:<controllerId>" as owner — see
// clusterBrowseWSSource / clusterTerminalSource in internal/panel.
type browseWSManager struct {
	mu         sync.Mutex
	sessions   map[string]*browseWSSession
	stop       chan struct{}
	stopOnce   sync.Once
	httpClient *http.Client
}

type browseWSMessage struct {
	binary bool
	data   []byte
}

type browseWSSession struct {
	mu            sync.Mutex
	owner         string
	conn          *websocket.Conn
	cancel        context.CancelFunc
	messages      []browseWSMessage
	base          int // sequence index of messages[0]; advances as the window slides
	bufferedBytes int
	notify        chan struct{}
	closed        bool
	closeReason   string
	createdAt     time.Time
	updatedAt     time.Time
}

const (
	maxBrowseWSSessions         = 16
	maxBrowseWSBufferedMessages = 256
	maxBrowseWSBufferedBytes    = 4 << 20
	maxBrowseWSMessageBytes     = 1 << 20
	browseWSIdleTimeout         = 5 * time.Minute
	browseWSSessionLifetime     = 30 * time.Minute
	browseWSDialTimeout         = 10 * time.Second
	browseWSWriteTimeout        = 10 * time.Second
	browseWSFinishedGrace       = time.Minute
)

var (
	errBrowseWSNotFound = errors.New("browse ws session not found")
	errBrowseWSLimit    = errors.New("browse ws session limit reached")
)

// browseWSHopByHopHeaders mirrors browseHopByHopHeaders: these are either
// handshake-protocol-controlled by the websocket library itself or must
// never be caller-overridable (Host).
var browseWSHopByHopHeaders = []string{
	"Connection", "Upgrade", "Host", "Sec-Websocket-Key", "Sec-Websocket-Version",
	"Sec-Websocket-Accept", "Sec-Websocket-Extensions", "Sec-Websocket-Protocol",
}

func newBrowseWSManager() *browseWSManager {
	m := &browseWSManager{
		sessions: make(map[string]*browseWSSession), stop: make(chan struct{}),
		httpClient: newBrowseHTTPClient(),
	}
	go m.reapLoop()
	return m
}

func (m *browseWSManager) CloseAll() {
	m.stopOnce.Do(func() { close(m.stop) })
	m.mu.Lock()
	sessions := make([]*browseWSSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()
	for _, session := range sessions {
		session.terminate("agent shutting down")
	}
}

func validateBrowseWSURL(raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > maxBrowseURLBytes {
		return nil, errors.New("url must be a non-empty absolute ws(s) URL within the size limit")
	}
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || target.Hostname() == "" {
		return nil, errors.New("url must be an absolute ws(s) URL")
	}
	if target.Scheme != "ws" && target.Scheme != "wss" {
		return nil, errors.New("url scheme must be ws or wss")
	}
	if target.User != nil {
		return nil, errors.New("url must not contain userinfo")
	}
	return target, nil
}

func browseWSDialHeader(input map[string][]string) (http.Header, error) {
	if len(input) > maxBrowseHeaderCount {
		return nil, errors.New("too many headers")
	}
	header := make(http.Header, len(input))
	for name, values := range input {
		if len(name) > maxBrowseHeaderBytes {
			return nil, errors.New("header name too large")
		}
		for _, value := range values {
			if len(value) > maxBrowseHeaderBytes {
				return nil, errors.New("header value too large")
			}
			header.Add(name, value)
		}
	}
	for _, name := range browseWSHopByHopHeaders {
		header.Del(name)
	}
	return header, nil
}

// Open dials targetURL (already validated by the caller) and starts relaying
// inbound messages into the session's sliding buffer. The dial itself is
// bounded by ctx, but the connection's lifetime is not tied to the request
// that opened it — background context, released only by Close or reaping.
func (m *browseWSManager) Open(ctx context.Context, owner, targetURL string, headers map[string][]string) (string, error) {
	owner = strings.TrimSpace(owner)
	target, err := validateBrowseWSURL(targetURL)
	if err != nil {
		return "", err
	}
	header, err := browseWSDialHeader(headers)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	if len(m.sessions) >= maxBrowseWSSessions {
		m.mu.Unlock()
		return "", errBrowseWSLimit
	}
	m.mu.Unlock()

	dialCtx, dialCancel := context.WithTimeout(ctx, browseWSDialTimeout)
	defer dialCancel()
	conn, _, err := websocket.Dial(dialCtx, target.String(), &websocket.DialOptions{
		HTTPClient: m.httpClient,
		HTTPHeader: header,
	})
	if err != nil {
		return "", err
	}
	conn.SetReadLimit(maxBrowseWSMessageBytes)

	id, err := randomBrowseWSID()
	if err != nil {
		_ = conn.CloseNow()
		return "", err
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	session := &browseWSSession{
		owner: owner, conn: conn, cancel: cancel, notify: make(chan struct{}),
		createdAt: now, updatedAt: now,
	}

	m.mu.Lock()
	if len(m.sessions) >= maxBrowseWSSessions {
		m.mu.Unlock()
		cancel()
		_ = conn.CloseNow()
		return "", errBrowseWSLimit
	}
	m.sessions[id] = session
	m.mu.Unlock()

	go relayBrowseWS(sessionCtx, session)
	return id, nil
}

func relayBrowseWS(ctx context.Context, session *browseWSSession) {
	for {
		typ, data, err := session.conn.Read(ctx)
		if err != nil {
			session.terminate(closeReasonFor(err))
			return
		}
		session.append(browseWSMessage{binary: typ == websocket.MessageBinary, data: data})
	}
}

func closeReasonFor(err error) string {
	if err == nil {
		return ""
	}
	if code := websocket.CloseStatus(err); code != -1 {
		return "closed"
	}
	return "error"
}

func (s *browseWSSession) append(msg browseWSMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.messages = append(s.messages, msg)
	s.bufferedBytes += len(msg.data)
	for (s.bufferedBytes > maxBrowseWSBufferedBytes || len(s.messages) > maxBrowseWSBufferedMessages) && len(s.messages) > 0 {
		s.bufferedBytes -= len(s.messages[0].data)
		s.messages = s.messages[1:]
		s.base++
	}
	s.updatedAt = time.Now().UTC()
	close(s.notify)
	s.notify = make(chan struct{})
}

func (s *browseWSSession) terminate(reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeReason = reason
	s.updatedAt = time.Now().UTC()
	close(s.notify)
	s.notify = make(chan struct{})
	s.mu.Unlock()
	s.cancel()
	_ = s.conn.CloseNow()
}

func (s *browseWSSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// snapshot returns messages from offset onward (clamped into the current
// sliding window), the next offset, whether the session is closed, and the
// current notify channel to wait on for more data.
func (s *browseWSSession) snapshot(offset int) ([]browseWSMessage, int, bool, string, chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if offset < s.base {
		offset = s.base
	}
	localIndex := offset - s.base
	if localIndex > len(s.messages) {
		localIndex = len(s.messages)
	}
	result := append([]browseWSMessage(nil), s.messages[localIndex:]...)
	nextOffset := s.base + len(s.messages)
	return result, nextOffset, s.closed, s.closeReason, s.notify
}

// lookup resolves id to a session owned by owner. A session that exists but
// belongs to a different owner is indistinguishable from one that does not
// exist at all — same reasoning as internal/terminal.Manager.lookup.
func (m *browseWSManager) lookup(owner, id string) (*browseWSSession, error) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session == nil || session.owner != strings.TrimSpace(owner) {
		return nil, errBrowseWSNotFound
	}
	return session, nil
}

// Output long-polls for messages at or after offset, waiting up to wait for
// new data to arrive if none is immediately available.
func (m *browseWSManager) Output(ctx context.Context, owner, id string, offset int, wait time.Duration) ([]browseWSMessage, int, bool, string, error) {
	session, err := m.lookup(owner, id)
	if err != nil {
		return nil, 0, false, "", err
	}
	messages, next, closed, reason, notify := session.snapshot(offset)
	if len(messages) > 0 || closed || wait <= 0 {
		return messages, next, closed, reason, nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-notify:
	case <-timer.C:
	case <-ctx.Done():
		messages, next, closed, reason, _ = session.snapshot(offset)
		return messages, next, closed, reason, nil
	}
	messages, next, closed, reason, _ = session.snapshot(offset)
	return messages, next, closed, reason, nil
}

func (m *browseWSManager) Input(ctx context.Context, owner, id string, binary bool, data []byte) error {
	session, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	if session.isClosed() {
		// Once closed (by either side — explicit Close or the target
		// disconnecting), the underlying conn is gone. Output on a closed
		// session still succeeds (drains any final buffered messages), but
		// there is nothing left to write to.
		return errBrowseWSNotFound
	}
	writeCtx, cancel := context.WithTimeout(ctx, browseWSWriteTimeout)
	defer cancel()
	typ := websocket.MessageText
	if binary {
		typ = websocket.MessageBinary
	}
	if err := session.conn.Write(writeCtx, typ, data); err != nil {
		session.terminate("error")
		return err
	}
	return nil
}

func (m *browseWSManager) Close(owner, id string) error {
	session, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	session.terminate("closed")
	return nil
}

func (m *browseWSManager) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reap(time.Now().UTC())
		case <-m.stop:
			return
		}
	}
}

func (m *browseWSManager) reap(now time.Time) {
	type staleSession struct{ owner, id string }
	var stale []staleSession
	var finished []string
	m.mu.Lock()
	for id, session := range m.sessions {
		session.mu.Lock()
		owner := session.owner
		isClosed := session.closed
		inactive := now.Sub(session.updatedAt) >= browseWSIdleTimeout || now.Sub(session.createdAt) >= browseWSSessionLifetime
		finishedLongEnough := isClosed && now.Sub(session.updatedAt) >= browseWSFinishedGrace
		session.mu.Unlock()
		if finishedLongEnough {
			finished = append(finished, id)
		} else if inactive && !isClosed {
			stale = append(stale, staleSession{owner: owner, id: id})
		}
	}
	for _, id := range finished {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	for _, item := range stale {
		_ = m.Close(item.owner, item.id)
	}
}

func randomBrowseWSID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// maxBrowseWSWait mirrors terminal_poll.go's maxTerminalWait: both are
// long-poll caps sized against the same v2 federation request-rate budget
// (see cluster.terminalSources/terminalRequests, reused for browse WS too).
const maxBrowseWSWait = 1500 * time.Millisecond

type browseWSOpenInput struct {
	Owner   string              `json:"owner"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
}

type browseWSOpenOutput struct {
	SessionID string `json:"sessionId"`
}

type browseWSMessageWire struct {
	Type string `json:"type"` // "text" or "binary"
	Data string `json:"data"` // base64
}

type browseWSOutputOutput struct {
	Messages    []browseWSMessageWire `json:"messages"`
	NextOffset  int                   `json:"nextOffset"`
	Closed      bool                  `json:"closed"`
	CloseReason string                `json:"closeReason,omitempty"`
}

type browseWSInputInput struct {
	Owner string `json:"owner"`
	Type  string `json:"type"` // "text" or "binary"
	Data  string `json:"data"` // base64
}

type browseWSCloseInput struct {
	Owner string `json:"owner"`
}

func (s *Server) browseWSOpen(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	var input browseWSOpenInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	id, err := s.browseWS.Open(r.Context(), input.Owner, input.URL, input.Headers)
	if err != nil {
		status, code := browseWSErrorStatus(err)
		writeProblem(w, requestID, status, code, "浏览 WebSocket 打开失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusCreated, browseWSOpenOutput{SessionID: id})
}

func browseWSErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, errBrowseTargetBlocked):
		return http.StatusForbidden, "browse_target_blocked"
	case errors.Is(err, errBrowseWSLimit):
		return http.StatusServiceUnavailable, "browse_ws_limit_reached"
	case errors.Is(err, errBrowseWSNotFound):
		return http.StatusNotFound, "browse_ws_not_found"
	default:
		return http.StatusBadGateway, "browse_ws_open_failed"
	}
}

func (s *Server) browseWSOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/browse/ws/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "Route not found", "")
		return
	}
	id, action := parts[0], parts[1]
	switch action {
	case "output":
		s.requireMethod(w, r, requestID, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			s.browseWSOutput(w, r, id)
		})
	case "input":
		s.requireMethod(w, r, requestID, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
			s.browseWSInput(w, r, id)
		})
	case "close":
		s.requireMethod(w, r, requestID, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
			s.browseWSClose(w, r, id)
		})
	default:
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "Route not found", "")
	}
}

func (s *Server) browseWSOutput(w http.ResponseWriter, r *http.Request, id string) {
	requestID := requestIDFrom(w)
	owner, offset, wait, err := parseBrowseWSOutputQuery(r.URL.Query())
	if err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), wait+2*time.Second)
	defer cancel()
	messages, next, closed, reason, err := s.browseWS.Output(ctx, owner, id, offset, wait)
	if err != nil {
		status, code := browseWSErrorStatus(err)
		writeProblem(w, requestID, status, code, "浏览 WebSocket 读取失败", safeDetail(err))
		return
	}
	wire := make([]browseWSMessageWire, len(messages))
	for i, msg := range messages {
		typ := "text"
		if msg.binary {
			typ = "binary"
		}
		wire[i] = browseWSMessageWire{Type: typ, Data: base64.StdEncoding.EncodeToString(msg.data)}
	}
	writeJSON(w, http.StatusOK, browseWSOutputOutput{Messages: wire, NextOffset: next, Closed: closed, CloseReason: reason})
}

func (s *Server) browseWSInput(w http.ResponseWriter, r *http.Request, id string) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	var input browseWSInputInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.Type != "text" && input.Type != "binary" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil || len(data) > maxBrowseWSMessageBytes {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	if err := s.browseWS.Input(r.Context(), input.Owner, id, input.Type == "binary", data); err != nil {
		status, code := browseWSErrorStatus(err)
		writeProblem(w, requestID, status, code, "浏览 WebSocket 写入失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
}

func (s *Server) browseWSClose(w http.ResponseWriter, r *http.Request, id string) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	var input browseWSCloseInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if err := s.browseWS.Close(input.Owner, id); err != nil {
		status, code := browseWSErrorStatus(err)
		writeProblem(w, requestID, status, code, "浏览 WebSocket 关闭失败", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
}

func parseBrowseWSOutputQuery(values url.Values) (string, int, time.Duration, error) {
	for key, entries := range values {
		if (key != "owner" && key != "offset" && key != "wait") || len(entries) != 1 {
			return "", 0, 0, errors.New("invalid browse ws query")
		}
	}
	rawOwner, hasOwner := values["owner"]
	if !hasOwner || rawOwner[0] == "" {
		return "", 0, 0, errors.New("owner is required")
	}
	rawOffset, hasOffset := values["offset"]
	if !hasOffset {
		return "", 0, 0, errors.New("offset is required")
	}
	offset, err := strconv.Atoi(rawOffset[0])
	if err != nil || offset < 0 {
		return "", 0, 0, errors.New("offset is invalid")
	}
	var wait time.Duration
	if rawWait, hasWait := values["wait"]; hasWait {
		waitMillis, err := strconv.Atoi(rawWait[0])
		if err != nil || waitMillis < 0 {
			return "", 0, 0, errors.New("wait is invalid")
		}
		wait = time.Duration(waitMillis) * time.Millisecond
		if wait > maxBrowseWSWait {
			return "", 0, 0, errors.New("wait exceeds the maximum")
		}
	}
	return rawOwner[0], offset, wait, nil
}
