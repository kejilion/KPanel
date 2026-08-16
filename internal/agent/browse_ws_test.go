package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testBrowseWSOwner = "test-owner"

func newBrowseWSTestServer(t *testing.T, handle func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handle(conn)
	}))
	t.Cleanup(server.Close)
	return server
}

// newTestBrowseWSManager points the manager's dialer at server via a fake
// hostname resolved to the httptest server's real loopback address, with the
// blocked-IP check disabled — the same technique browse_test.go uses for
// browseDialer, keeping the production SSRF guard itself exercised only by
// the dedicated test below that uses the real newBrowseWSManager().
func newTestBrowseWSManager(t *testing.T, server *httptest.Server) *browseWSManager {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := &browseDialer{
		resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{addr.IP}, nil },
		blocked: func(net.IP) bool { return false },
	}
	manager := newBrowseWSManager()
	manager.httpClient = &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
	t.Cleanup(manager.CloseAll)
	return manager
}

func wsURLFor(t *testing.T, server *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return "ws://browse-ws-test.invalid:" + port + "/"
}

func pollBrowseWSMessages(t *testing.T, manager *browseWSManager, id string, want int) []browseWSMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var collected []browseWSMessage
	offset := 0
	for time.Now().Before(deadline) {
		messages, next, closed, reason, err := manager.Output(context.Background(), testBrowseWSOwner, id, offset, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		collected = append(collected, messages...)
		offset = next
		if len(collected) >= want {
			return collected
		}
		if closed {
			t.Fatalf("session closed (reason=%q) before %d messages arrived, got %d", reason, want, len(collected))
		}
	}
	t.Fatalf("timed out waiting for %d messages, got %d", want, len(collected))
	return nil
}

func TestBrowseWSManagerRelaysMessagesInOrderAndAcceptsInput(t *testing.T) {
	received := make(chan string, 1)
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		for _, msg := range []string{"one", "two", "three"} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
				return
			}
		}
		if typ, data, err := conn.Read(ctx); err == nil && typ == websocket.MessageText {
			received <- string(data)
		}
	})
	manager := newTestBrowseWSManager(t, server)

	id, err := manager.Open(context.Background(), testBrowseWSOwner, wsURLFor(t, server), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	messages := pollBrowseWSMessages(t, manager, id, 3)
	got := make([]string, len(messages))
	for i, msg := range messages {
		if msg.binary {
			t.Fatalf("message %d unexpectedly binary", i)
		}
		got[i] = string(msg.data)
	}
	want := []string{"one", "two", "three"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message order mismatch: got %v, want %v", got, want)
		}
	}

	if err := manager.Input(context.Background(), testBrowseWSOwner, id, false, []byte("reply")); err != nil {
		t.Fatalf("Input() error = %v", err)
	}
	select {
	case reply := <-received:
		if reply != "reply" {
			t.Fatalf("server received %q, want %q", reply, "reply")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the input message")
	}
}

func TestBrowseWSManagerCloseEndsSessionAndFailsFurtherCalls(t *testing.T) {
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	})
	manager := newTestBrowseWSManager(t, server)

	id, err := manager.Open(context.Background(), testBrowseWSOwner, wsURLFor(t, server), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := manager.Close(testBrowseWSOwner, id); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := manager.Input(context.Background(), testBrowseWSOwner, id, false, []byte("x")); !errors.Is(err, errBrowseWSNotFound) {
		t.Fatalf("Input() after Close error = %v, want errBrowseWSNotFound", err)
	}
	// Output must keep working after Close (draining any final buffered
	// messages, reporting closed:true) rather than erroring — a poll that
	// was already in flight when Close landed should see a clean result,
	// not a race-shaped failure.
	_, _, closed, _, err := manager.Output(context.Background(), testBrowseWSOwner, id, 0, 0)
	if err != nil {
		t.Fatalf("Output() after Close error = %v, want success with closed=true", err)
	}
	if !closed {
		t.Fatal("Output() after Close did not report closed=true")
	}
	if _, err := manager.lookup(testBrowseWSOwner, "unknown-session-id"); !errors.Is(err, errBrowseWSNotFound) {
		t.Fatalf("lookup() of a session that never existed error = %v, want errBrowseWSNotFound", err)
	}
}

func TestBrowseWSManagerRejectsCrossOwnerAccess(t *testing.T) {
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	})
	manager := newTestBrowseWSManager(t, server)

	id, err := manager.Open(context.Background(), "owner-a", wsURLFor(t, server), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, _, _, _, err := manager.Output(context.Background(), "owner-b", id, 0, 0); !errors.Is(err, errBrowseWSNotFound) {
		t.Fatalf("Output() from a different owner error = %v, want errBrowseWSNotFound", err)
	}
	if err := manager.Input(context.Background(), "owner-b", id, false, []byte("x")); !errors.Is(err, errBrowseWSNotFound) {
		t.Fatalf("Input() from a different owner error = %v, want errBrowseWSNotFound", err)
	}
	if err := manager.Close("owner-b", id); !errors.Is(err, errBrowseWSNotFound) {
		t.Fatalf("Close() from a different owner error = %v, want errBrowseWSNotFound", err)
	}
	// The rightful owner must still be able to use it after the failed probes.
	if _, _, _, _, err := manager.Output(context.Background(), "owner-a", id, 0, 0); err != nil {
		t.Fatalf("Output() from the rightful owner error = %v", err)
	}
}

func TestBrowseWSManagerReportsServerInitiatedClose(t *testing.T) {
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	})
	manager := newTestBrowseWSManager(t, server)

	id, err := manager.Open(context.Background(), testBrowseWSOwner, wsURLFor(t, server), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, _, closed, _, err := manager.Output(context.Background(), testBrowseWSOwner, id, 0, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		if closed {
			return
		}
	}
	t.Fatal("session was never reported closed after the target closed the connection")
}

func TestBrowseWSManagerEnforcesSessionLimit(t *testing.T) {
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	})
	manager := newTestBrowseWSManager(t, server)
	target := wsURLFor(t, server)

	for i := 0; i < maxBrowseWSSessions; i++ {
		if _, err := manager.Open(context.Background(), testBrowseWSOwner, target, nil); err != nil {
			t.Fatalf("Open() call %d error = %v", i, err)
		}
	}
	if _, err := manager.Open(context.Background(), testBrowseWSOwner, target, nil); !errors.Is(err, errBrowseWSLimit) {
		t.Fatalf("Open() beyond the limit error = %v, want errBrowseWSLimit", err)
	}
}

func TestBrowseWSManagerBlocksLoopbackTarget(t *testing.T) {
	manager := newBrowseWSManager() // production dialer: real isBlockedIP, no test override
	defer manager.CloseAll()
	if _, err := manager.Open(context.Background(), testBrowseWSOwner, "ws://127.0.0.1:1/", nil); err == nil {
		t.Fatal("expected a loopback target to be blocked")
	}
}

func TestBrowseWSManagerSlidingWindowEvictsOldestOnOverflow(t *testing.T) {
	server := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		for i := 0; i < maxBrowseWSBufferedMessages+10; i++ {
			if err := conn.Write(ctx, websocket.MessageText, []byte("m")); err != nil {
				return
			}
		}
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	})
	manager := newTestBrowseWSManager(t, server)
	id, err := manager.Open(context.Background(), testBrowseWSOwner, wsURLFor(t, server), nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var next int
	for time.Now().Before(deadline) {
		_, n, _, _, err := manager.Output(context.Background(), testBrowseWSOwner, id, 0, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		next = n
		if next >= maxBrowseWSBufferedMessages+10 {
			break
		}
	}
	if next < maxBrowseWSBufferedMessages+10 {
		t.Fatalf("nextOffset = %d, want at least %d", next, maxBrowseWSBufferedMessages+10)
	}
	// Offset 0 has long since aged out of the window; Output must clamp
	// rather than error, and must not return more than the window holds.
	messages, _, _, _, err := manager.Output(context.Background(), testBrowseWSOwner, id, 0, 0)
	if err != nil {
		t.Fatalf("Output(offset=0) after eviction error = %v", err)
	}
	if len(messages) > maxBrowseWSBufferedMessages {
		t.Fatalf("Output returned %d messages, sliding window cap is %d", len(messages), maxBrowseWSBufferedMessages)
	}
}

func TestBrowseWSOpenHandlerRejectsUnauthenticated(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/ws", bytes.NewReader([]byte(`{"owner":"x","url":"ws://example.com/"}`)))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseWSOpenHandlerRejectsWrongMethod(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/browse/ws", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseWSOpenHandlerBlocksLoopbackTarget(t *testing.T) {
	server := testServer(t)
	body, _ := json.Marshal(browseWSOpenInput{Owner: testBrowseWSOwner, URL: "ws://127.0.0.1:1/"})
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/ws", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseWSOperationHandlerUnknownSessionReturns404(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)
	for _, target := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/browse/ws/does-not-exist/output?owner=" + testBrowseWSOwner + "&offset=0"},
		{http.MethodPost, "/v1/browse/ws/does-not-exist/input"},
		{http.MethodPost, "/v1/browse/ws/does-not-exist/close"},
	} {
		var body *bytes.Reader
		switch {
		case strings.HasSuffix(target.path, "/input"):
			payload, _ := json.Marshal(browseWSInputInput{Owner: testBrowseWSOwner, Type: "text", Data: base64.StdEncoding.EncodeToString([]byte("x"))})
			body = bytes.NewReader(payload)
		case strings.HasSuffix(target.path, "/close"):
			payload, _ := json.Marshal(browseWSCloseInput{Owner: testBrowseWSOwner})
			body = bytes.NewReader(payload)
		default:
			body = bytes.NewReader(nil)
		}
		request := httptest.NewRequest(target.method, target.path, body)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d body=%s", target.method, target.path, response.Code, response.Body.String())
		}
	}
}

// TestBrowseWSHandlersFullLifecycle drives Open -> Output -> Input -> Close
// entirely through the HTTP layer against a real (test-scoped) target
// server, proving the wire contract end to end rather than just the
// manager's Go API (already covered above).
func TestBrowseWSHandlersFullLifecycle(t *testing.T) {
	received := make(chan string, 1)
	wsServer := newBrowseWSTestServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		if err := conn.Write(ctx, websocket.MessageText, []byte("hello")); err != nil {
			return
		}
		if typ, data, err := conn.Read(ctx); err == nil && typ == websocket.MessageText {
			received <- string(data)
		}
	})
	server := testServer(t)
	server.browseWS = newTestBrowseWSManager(t, wsServer)
	token := "Bearer " + strings.Repeat("x", 32)

	openBody, _ := json.Marshal(browseWSOpenInput{Owner: testBrowseWSOwner, URL: wsURLFor(t, wsServer)})
	openRequest := httptest.NewRequest(http.MethodPost, "/v1/browse/ws", bytes.NewReader(openBody))
	openRequest.Header.Set("Authorization", token)
	openResponse := httptest.NewRecorder()
	server.ServeHTTP(openResponse, openRequest)
	if openResponse.Code != http.StatusCreated {
		t.Fatalf("open status = %d body=%s", openResponse.Code, openResponse.Body.String())
	}
	var opened browseWSOpenOutput
	if err := json.Unmarshal(openResponse.Body.Bytes(), &opened); err != nil || opened.SessionID == "" {
		t.Fatalf("open response = %s (err=%v)", openResponse.Body.String(), err)
	}

	var output browseWSOutputOutput
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(output.Messages) == 0 {
		outputRequest := httptest.NewRequest(http.MethodGet, "/v1/browse/ws/"+opened.SessionID+"/output?owner="+testBrowseWSOwner+"&offset=0&wait=200", nil)
		outputRequest.Header.Set("Authorization", token)
		outputResponse := httptest.NewRecorder()
		server.ServeHTTP(outputResponse, outputRequest)
		if outputResponse.Code != http.StatusOK {
			t.Fatalf("output status = %d body=%s", outputResponse.Code, outputResponse.Body.String())
		}
		if err := json.Unmarshal(outputResponse.Body.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
	}
	if len(output.Messages) != 1 || output.Messages[0].Type != "text" {
		t.Fatalf("unexpected output messages: %+v", output.Messages)
	}
	decoded, err := base64.StdEncoding.DecodeString(output.Messages[0].Data)
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("decoded message = %q (err=%v)", decoded, err)
	}

	inputBody, _ := json.Marshal(browseWSInputInput{Owner: testBrowseWSOwner, Type: "text", Data: base64.StdEncoding.EncodeToString([]byte("world"))})
	inputRequest := httptest.NewRequest(http.MethodPost, "/v1/browse/ws/"+opened.SessionID+"/input", bytes.NewReader(inputBody))
	inputRequest.Header.Set("Authorization", token)
	inputResponse := httptest.NewRecorder()
	server.ServeHTTP(inputResponse, inputRequest)
	if inputResponse.Code != http.StatusOK {
		t.Fatalf("input status = %d body=%s", inputResponse.Code, inputResponse.Body.String())
	}
	select {
	case got := <-received:
		if got != "world" {
			t.Fatalf("target received %q, want %q", got, "world")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("target never received the input message")
	}

	closeBody, _ := json.Marshal(browseWSCloseInput{Owner: testBrowseWSOwner})
	closeRequest := httptest.NewRequest(http.MethodPost, "/v1/browse/ws/"+opened.SessionID+"/close", bytes.NewReader(closeBody))
	closeRequest.Header.Set("Authorization", token)
	closeResponse := httptest.NewRecorder()
	server.ServeHTTP(closeResponse, closeRequest)
	if closeResponse.Code != http.StatusOK {
		t.Fatalf("close status = %d body=%s", closeResponse.Code, closeResponse.Body.String())
	}
}
