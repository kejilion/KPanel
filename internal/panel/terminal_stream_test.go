package panel

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamAgentStub honors the long-poll wait so terminal pumps idle instead of
// spinning, and serves task terminal chunks for job subscriptions.
type streamAgentStub struct {
	terminalAgentStub
	mu      sync.Mutex
	pending []byte
	wake    chan struct{}
	job     []byte
}

func newStreamAgentStub() *streamAgentStub {
	return &streamAgentStub{wake: make(chan struct{}, 1)}
}

func (s *streamAgentStub) push(data string) {
	s.mu.Lock()
	s.pending = append(s.pending, data...)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *streamAgentStub) Get(ctx context.Context, path, rawQuery, requestID string) (AgentResponse, error) {
	if strings.HasPrefix(path, "/v1/app-jobs/") && strings.HasSuffix(path, "/terminal") {
		return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: s.job}, nil
	}
	if !strings.HasSuffix(path, "/output") {
		return s.terminalAgentStub.Get(ctx, path, rawQuery, requestID)
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	for {
		s.mu.Lock()
		if len(s.pending) > 0 || strings.Contains(rawQuery, "wait=0") {
			data := s.pending
			s.pending = nil
			offset := s.next
			s.next += int64(len(data))
			s.mu.Unlock()
			body, _ := json.Marshal(map[string]any{"data": data, "offset": offset, "nextOffset": offset + int64(len(data)), "truncated": false, "closed": false})
			return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return AgentResponse{}, ctx.Err()
		case <-s.wake:
		case <-timer.C:
			body, _ := json.Marshal(map[string]any{"data": nil, "offset": s.next, "nextOffset": s.next, "truncated": false, "closed": false})
			return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
		}
	}
}

type sseReader struct {
	scanner *bufio.Scanner
}

func (r sseReader) next(t *testing.T) (string, string) {
	t.Helper()
	event := ""
	for r.scanner.Scan() {
		line := r.scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			return event, strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("stream ended: %v", r.scanner.Err())
	return "", ""
}

func openTerminalStream(t *testing.T, server *Server, sessionCookie *http.Cookie) (*http.Response, sseReader, string) {
	t.Helper()
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	request, _ := http.NewRequest(http.MethodGet, httpServer.URL+terminalStreamPath, nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream status %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := sseReader{scanner: bufio.NewScanner(response.Body)}
	event, data := reader.next(t)
	var ready struct {
		StreamID string `json:"streamId"`
	}
	if event != "ready" || json.Unmarshal([]byte(data), &ready) != nil || ready.StreamID == "" {
		t.Fatalf("ready event = %s %s", event, data)
	}
	return response, reader, ready.StreamID
}

func TestTerminalStreamPushesHostAndTaskTerminals(t *testing.T) {
	server, tokenPath := newTestServer(t)
	stub := newStreamAgentStub()
	stub.job = []byte(`{"dataBase64":"aGk=","nextOffset":2,"inputOpen":true,"finished":true}`)
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":30,"columns":120}`), sessionCookie, csrfCookie, headers)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	var session terminalOpenResponse
	_ = json.Unmarshal(opened.Body.Bytes(), &session)
	_, reader, streamID := openTerminalStream(t, server, sessionCookie)

	subscribe := func(body string) int {
		return authenticatedRequest(server, http.MethodPost, terminalStreamSubscriptionsPath, []byte(body), sessionCookie, csrfCookie, headers).Code
	}
	if code := authenticatedRequest(server, http.MethodPost, terminalStreamSubscriptionsPath,
		[]byte(`{"streamId":"`+streamID+`","add":[{"kind":"terminal","id":"`+session.SessionID+`","offset":0}]}`),
		sessionCookie, csrfCookie, map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"}).Code; code != http.StatusForbidden {
		t.Fatalf("subscription without CSRF = %d", code)
	}
	if code := subscribe(`{"streamId":"` + streamID + `","add":[{"kind":"terminal","id":"not-a-session","offset":0}]}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("foreign terminal subscription = %d", code)
	}
	if code := subscribe(`{"streamId":"missing","add":[]}`); code != http.StatusNotFound {
		t.Fatalf("unknown stream = %d", code)
	}
	if code := subscribe(`{"streamId":"` + streamID + `","add":[{"kind":"terminal","id":"` + session.SessionID + `","offset":0}]}`); code != http.StatusOK {
		t.Fatalf("subscribe = %d", code)
	}
	stub.push("hello")
	for {
		event, data := reader.next(t)
		var payload terminalStreamEvent
		if event != "output" || json.Unmarshal([]byte(data), &payload) != nil {
			t.Fatalf("event %s %s", event, data)
		}
		if payload.Key == "terminal:"+session.SessionID && payload.Output != nil && string(payload.Output.Data) == "hello" {
			break
		}
	}
	if code := subscribe(`{"streamId":"` + streamID + `","add":[{"kind":"job","job":"app","id":"0123456789abcdef0123456789abcdef","offset":0}]}`); code != http.StatusOK {
		t.Fatalf("job subscribe = %d", code)
	}
	for {
		event, data := reader.next(t)
		var payload terminalStreamEvent
		if event != "output" || json.Unmarshal([]byte(data), &payload) != nil {
			t.Fatalf("event %s %s", event, data)
		}
		if payload.Key == "job:app:0123456789abcdef0123456789abcdef" && payload.Job != nil && payload.Job.Finished && payload.Job.DataBase64 == "aGk=" && payload.Job.InputOpen {
			break
		}
	}
	if code := subscribe(`{"streamId":"` + streamID + `","add":[{"kind":"job","job":"shell","id":"0123456789abcdef0123456789abcdef","offset":0}]}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown job kind accepted: %d", code)
	}
}

func TestTerminalStreamEndsAfterLogout(t *testing.T) {
	previous := terminalStreamHeartbeat
	terminalStreamHeartbeat = 50 * time.Millisecond
	defer func() { terminalStreamHeartbeat = previous }()
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	_, reader, _ := openTerminalStream(t, server, sessionCookie)
	logout := authenticatedRequest(server, http.MethodPost, "/api/v1/auth/logout", nil, sessionCookie, csrfCookie,
		map[string]string{"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value})
	if logout.Code >= 300 {
		t.Fatalf("logout = %d %s", logout.Code, logout.Body.String())
	}
	done := make(chan string, 1)
	go func() {
		event, _ := reader.next(t)
		done <- event
	}()
	select {
	case event := <-done:
		if event != "auth.expired" {
			t.Fatalf("event after logout = %s", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream kept running after logout")
	}
}

func TestTerminalStreamLimitsAndBindsToSession(t *testing.T) {
	hub := newTerminalStreamHub()
	defer hub.closeAll()
	token := [32]byte{1}
	var first *terminalStream
	for index := 0; index < maxTerminalStreamsPerUser; index++ {
		stream, ok := hub.register("user", token)
		if !ok {
			t.Fatalf("stream %d rejected", index)
		}
		if first == nil {
			first = stream
		}
	}
	if _, ok := hub.register("user", token); ok {
		t.Fatal("per-user stream limit ignored")
	}
	if hub.lookup(first.id, "user", [32]byte{2}) != nil {
		t.Fatal("stream reachable from another session token")
	}
	if hub.lookup(first.id, "other", token) != nil {
		t.Fatal("stream reachable from another user")
	}
	hub.remove(first)
	if _, ok := hub.register("user", token); !ok {
		t.Fatal("removed stream still counted")
	}
}

// paneld sets ReadTimeout; net/http cancels a request context when that
// deadline expires during the handler unless the handler clears it. The
// stream must outlive the server read timeout.
func TestTerminalStreamSurvivesServerReadTimeout(t *testing.T) {
	previous := terminalStreamHeartbeat
	terminalStreamHeartbeat = 50 * time.Millisecond
	defer func() { terminalStreamHeartbeat = previous }()
	server, tokenPath := newTestServer(t)
	sessionCookie, _ := bootstrapCookies(t, server, tokenPath)
	httpServer := httptest.NewUnstartedServer(server)
	httpServer.Config.ReadTimeout = 200 * time.Millisecond
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	request, _ := http.NewRequest(http.MethodGet, httpServer.URL+terminalStreamPath, nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("stream ended after %v: %v", 700*time.Millisecond-time.Until(deadline), err)
		}
	}
}
