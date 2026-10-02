package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type socketInputProcess struct {
	mu    sync.Mutex
	input bytes.Buffer
	done  chan struct{}
	once  sync.Once
}

func (p *socketInputProcess) Read([]byte) (int, error) { <-p.done; return 0, io.EOF }
func (p *socketInputProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.Write(data)
}
func (p *socketInputProcess) Close() error                { p.once.Do(func() { close(p.done) }); return nil }
func (p *socketInputProcess) Kill() error                 { return p.Close() }
func (p *socketInputProcess) Wait() error                 { <-p.done; return nil }
func (p *socketInputProcess) Resize(uint16, uint16) error { return nil }
func (p *socketInputProcess) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.String()
}

type socketInputAgent struct {
	terminalAgentStub
	manager *terminal.Manager
}

func (a *socketInputAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if strings.HasSuffix(path, "/input-protocol") {
		return AgentResponse{StatusCode: 200, Body: []byte(`{"protocol":"terminal-input-v1"}`)}, nil
	}
	if path == "/v1/terminals" {
		var input struct {
			Owner         string
			Rows, Columns uint16
		}
		_ = json.Unmarshal(body, &input)
		opened, err := a.manager.Open(input.Owner, input.Rows, input.Columns)
		if err != nil {
			return AgentResponse{}, err
		}
		data, _ := json.Marshal(opened)
		return AgentResponse{StatusCode: 201, Body: data}, nil
	}
	if strings.HasSuffix(path, "/input-sequenced") {
		var input struct {
			Owner string
			Frame terminal.InputFrame
		}
		_ = json.Unmarshal(body, &input)
		id := strings.Split(path, "/")[3]
		err := a.manager.InputSequenced(input.Owner, id, input.Frame)
		if err != nil {
			code := "terminal_input_sequence"
			if errors.Is(err, terminal.ErrInputUncertain) {
				code = "terminal_input_uncertain"
			}
			if errors.Is(err, terminal.ErrClosed) {
				code = "terminal_closed"
			}
			payload, _ := json.Marshal(map[string]string{"code": code})
			return AgentResponse{StatusCode: 409, Body: payload}, nil
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"accepted":true}`)}, nil
	}
	return a.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
}

func TestTerminalInputSocketClaimsBeforeDataAndReplaysLostACK(t *testing.T) {
	server, tokenPath := newTestServer(t)
	process := &socketInputProcess{done: make(chan struct{})}
	manager := terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return process, nil }})
	defer manager.CloseAll()
	server.agent = &socketInputAgent{manager: manager}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	opened := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions", []byte(`{"hostId":"local","rows":24,"columns":80}`), sessionCookie, csrfCookie, headers)
	if opened.Code != 201 {
		t.Fatalf("open: %d %s", opened.Code, opened.Body.String())
	}
	var session terminalOpenResponse
	_ = json.Unmarshal(opened.Body.Bytes(), &session)
	transport := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/input-transport", []byte(`{}`), sessionCookie, csrfCookie, headers)
	if transport.Code != 200 || !strings.Contains(transport.Body.String(), terminal.InputProtocol) {
		t.Fatalf("capability: %s", transport.Body.String())
	}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; server.ServeHTTP(w, r) }))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		header := http.Header{"Origin": []string{origin}, "Cookie": []string{sessionCookie.String() + "; " + csrfCookie.String()}}
		return websocket.Dial(ctx, httpServer.URL+"/api/v1/terminal-sessions/"+session.SessionID+"/input-stream", &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{terminalInputSocketProtocol}})
	}
	if connection, response, err := dial("https://evil.example"); err == nil {
		connection.CloseNow()
		t.Fatal("foreign origin accepted")
	} else if response == nil || response.StatusCode != 403 {
		t.Fatalf("foreign origin status: %v %v", response, err)
	}
	if connection, response, err := dial(""); err == nil {
		connection.CloseNow()
		t.Fatal("missing origin accepted")
	} else if response == nil || response.StatusCode != 400 {
		t.Fatalf("missing origin status: %v %v", response, err)
	}
	server.terminalMu.Lock()
	owned := server.terminalSessions[session.SessionID]
	foreign := owned
	foreign.UserID = "another-user"
	server.terminalSessions[session.SessionID] = foreign
	server.terminalMu.Unlock()
	if connection, response, err := dial("http://panel.test"); err == nil {
		connection.CloseNow()
		t.Fatal("foreign user terminal accepted")
	} else if response == nil || response.StatusCode != 404 {
		t.Fatalf("owner mismatch status: %v %v", response, err)
	}
	server.terminalMu.Lock()
	server.terminalSessions[session.SessionID] = owned
	server.terminalMu.Unlock()
	write := func(ws *websocket.Conn, value any) {
		data, _ := json.Marshal(value)
		if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
	}
	read := func(ws *websocket.Conn) terminalInputReply {
		_, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var reply terminalInputReply
		if err := json.Unmarshal(data, &reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}
	stream := "00000000000000000000000000000001"
	ws, _, err := dial("http://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	write(ws, map[string]string{"type": "auth", "csrf": "wrong", "stream": stream})
	if reply := read(ws); reply.Code != "csrf_validation_failed" {
		t.Fatalf("bad csrf: %+v", reply)
	}
	ws.CloseNow()
	if process.String() != "" {
		t.Fatal("input before CSRF verification")
	}
	server.terminalMu.Lock()
	claimed := server.terminalSessions[session.SessionID].InputClaimed
	server.terminalMu.Unlock()
	if claimed {
		t.Fatal("bad CSRF claimed the writer")
	}
	connect := func() *websocket.Conn {
		var connection *websocket.Conn
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			connection, _, err = dial("http://panel.test")
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
		write(connection, map[string]string{"type": "auth", "csrf": csrfCookie.Value, "stream": stream})
		if reply := read(connection); reply.Type != "ready" || reply.Window != terminal.InputWindow {
			t.Fatalf("ready: %+v", reply)
		}
		return connection
	}
	ws = connect()
	defer ws.CloseNow()
	if duplicate, response, err := dial("http://panel.test"); err == nil {
		duplicate.CloseNow()
		t.Fatal("second socket bypassed per-session capacity")
	} else if response == nil || response.StatusCode != 429 {
		t.Fatalf("socket capacity: %v %v", response, err)
	}
	raw := authenticatedRequest(server, http.MethodPost, "/api/v1/terminal-sessions/"+session.SessionID+"/input", []byte(`{"data":"eA=="}`), sessionCookie, csrfCookie, headers)
	if raw.Code != 409 {
		t.Fatalf("raw input accepted before first sequenced data: %d", raw.Code)
	}
	frame := terminal.InputFrame{Stream: stream, Seq: 1, Data: []byte("中文\x03\r")}
	write(ws, terminalInputMessage{Type: "input", Frame: &frame})
	deadline := time.Now().Add(time.Second)
	for process.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// Drop the browser socket after PTY write without consuming its ACK.
	ws.CloseNow()
	ws = connect()
	defer ws.CloseNow()
	write(ws, terminalInputMessage{Type: "input", Frame: &frame})
	if reply := read(ws); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("replay ack: %+v", reply)
	}
	if process.String() != string(frame.Data) {
		t.Fatalf("lost ACK replay duplicated bytes: %q", process.String())
	}
	oversize := terminal.InputFrame{Stream: stream, Seq: 2, Data: bytes.Repeat([]byte("x"), terminal.InputFrameBytes+1)}
	write(ws, terminalInputMessage{Type: "input", Frame: &oversize})
	if reply := read(ws); reply.Code != "terminal_input_sequence" {
		t.Fatalf("oversize input accepted: %+v", reply)
	}
	ws.CloseNow()
	ws = connect()
	defer ws.CloseNow()
	if process.String() != string(frame.Data) {
		t.Fatal("rejected oversized frame reached PTY")
	}
	logout := authenticatedRequest(server, http.MethodPost, "/api/v1/auth/logout", nil, sessionCookie, csrfCookie, headers)
	if logout.Code != 200 && logout.Code != 204 {
		t.Fatalf("logout: %d", logout.Code)
	}
	if reply := read(ws); reply.Code != "session_expired" {
		t.Fatalf("logout did not revoke socket: %+v", reply)
	}
}
