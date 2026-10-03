package panel

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

// An input socket is idle whenever nobody types. Reverse proxies close idle
// upgraded connections: nginx does when the upstream sends nothing for
// proxy_read_timeout (60 s by default, and KPanel's own site template does not
// raise it), Cloudflare after about 100 s. The page then reconnects and prints
// "reconnecting" every time. The Panel must keep the connection busy itself.
//
// idleCutProxy stands in for such a proxy: it relays TCP and closes both sides
// once the upstream has sent nothing for idle.
type idleCutProxy struct {
	addr string
}

func newIdleCutProxy(t *testing.T, target string, idle time.Duration) *idleCutProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	p := &idleCutProxy{addr: listener.Addr().String()}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			upstream, err := net.Dial("tcp", target)
			if err != nil {
				client.Close()
				continue
			}
			closeBoth := func() { client.Close(); upstream.Close() }
			go func() {
				buffer := make([]byte, 32<<10)
				for {
					n, err := client.Read(buffer)
					if n > 0 {
						if _, werr := upstream.Write(buffer[:n]); werr != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				closeBoth()
			}()
			go func() {
				buffer := make([]byte, 32<<10)
				for {
					_ = upstream.SetReadDeadline(time.Now().Add(idle))
					n, err := upstream.Read(buffer)
					if n > 0 {
						if _, werr := client.Write(buffer[:n]); werr != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				closeBoth()
			}()
		}
	}()
	return p
}

const (
	proxyIdleCut     = 400 * time.Millisecond
	testKeepalive    = 100 * time.Millisecond
	idleLongerThanIt = 1200 * time.Millisecond
)

func shortenInputKeepalive(t *testing.T) {
	previous := terminalInputKeepalive
	terminalInputKeepalive = testKeepalive
	t.Cleanup(func() { terminalInputKeepalive = previous })
}

func dialThrough(t *testing.T, ctx context.Context, proxy *idleCutProxy, path string, session, csrf *http.Cookie) *websocket.Conn {
	t.Helper()
	header := http.Header{"Origin": []string{"http://panel.test"}, "Cookie": []string{session.String() + "; " + csrf.String()}}
	ws, _, err := websocket.Dial(ctx, "ws://"+proxy.addr+path, &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{terminalInputSocketProtocol}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	return ws
}

// readReplies reads replies in the background, as the browser does: it answers
// pings only while it reads, so a test must keep reading to stay alive.
func readReplies(ctx context.Context, ws *websocket.Conn) <-chan terminalInputReply {
	replies := make(chan terminalInputReply, 16)
	go func() {
		defer close(replies)
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			var reply terminalInputReply
			if json.Unmarshal(data, &reply) == nil {
				replies <- reply
			}
		}
	}()
	return replies
}

func nextReply(t *testing.T, replies <-chan terminalInputReply) terminalInputReply {
	t.Helper()
	select {
	case reply, ok := <-replies:
		if !ok {
			t.Fatal("the input socket was closed")
		}
		return reply
	case <-time.After(5 * time.Second):
		t.Fatal("no reply")
	}
	return terminalInputReply{}
}

func TestHostTerminalInputSocketSurvivesAnIdleClosingProxy(t *testing.T) {
	shortenInputKeepalive(t)
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
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; server.ServeHTTP(w, r) }))
	defer httpServer.Close()
	proxy := newIdleCutProxy(t, httpServer.Listener.Addr().String(), proxyIdleCut)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ws := dialThrough(t, ctx, proxy, "/api/v1/terminal-sessions/"+session.SessionID+"/input-stream", sessionCookie, csrfCookie)
	replies := readReplies(ctx, ws)
	stream := "00000000000000000000000000000001"
	jobSocketWrite(t, ctx, ws, map[string]string{"type": "auth", "csrf": csrfCookie.Value, "stream": stream})
	if reply := nextReply(t, replies); reply.Type != "ready" {
		t.Fatalf("ready: %+v", reply)
	}

	// Nobody types for longer than the proxy tolerates silence.
	time.Sleep(idleLongerThanIt)

	sendJobFrame(t, ctx, ws, stream, 1, "ls\r")
	if reply := nextReply(t, replies); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("idle host input socket behind a proxy: %+v", reply)
	}
}

func TestTaskTerminalInputSocketSurvivesAnIdleClosingProxy(t *testing.T) {
	shortenInputKeepalive(t)
	server, tokenPath := newTestServer(t)
	agent := newJobInputAgent()
	server.agent = agent
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; server.ServeHTTP(w, r) }))
	defer httpServer.Close()
	proxy := newIdleCutProxy(t, httpServer.Listener.Addr().String(), proxyIdleCut)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ws := dialThrough(t, ctx, proxy, jobTerminalInputPrefix+"app/"+jobTestID+"/input-stream", sessionCookie, csrfCookie)
	replies := readReplies(ctx, ws)
	jobSocketWrite(t, ctx, ws, map[string]string{"type": "auth", "csrf": csrfCookie.Value, "stream": jobStreamOne})
	if reply := nextReply(t, replies); reply.Type != "ready" {
		t.Fatalf("ready: %+v", reply)
	}

	time.Sleep(idleLongerThanIt)

	sendJobFrame(t, ctx, ws, jobStreamOne, 1, "y\r")
	if reply := nextReply(t, replies); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("idle task input socket behind a proxy: %+v", reply)
	}
	if data, _ := agent.received(); !strings.Contains(data, "y\r") {
		t.Fatalf("task received %q", data)
	}
}

// The server's own whole-request timeouts are not the cause: net/http clears
// them when it hands over a hijacked connection. Keep that pinned, since paneld
// serves with ReadTimeout and WriteTimeout and a regression would look the same.
func TestTerminalInputSocketOutlivesServerTimeouts(t *testing.T) {
	server, tokenPath := newTestServer(t)
	agent := newJobInputAgent()
	server.agent = agent
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	httpServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; server.ServeHTTP(w, r) }))
	httpServer.Config.ReadTimeout = 300 * time.Millisecond
	httpServer.Config.WriteTimeout = 500 * time.Millisecond
	httpServer.Start()
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	header := http.Header{"Origin": []string{"http://panel.test"}, "Cookie": []string{sessionCookie.String() + "; " + csrfCookie.String()}}
	ws, _, err := websocket.Dial(ctx, httpServer.URL+jobTerminalInputPrefix+"app/"+jobTestID+"/input-stream", &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{terminalInputSocketProtocol}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	replies := readReplies(ctx, ws)
	jobSocketWrite(t, ctx, ws, map[string]string{"type": "auth", "csrf": csrfCookie.Value, "stream": jobStreamOne})
	if reply := nextReply(t, replies); reply.Type != "ready" {
		t.Fatalf("ready: %+v", reply)
	}
	time.Sleep(900 * time.Millisecond)
	sendJobFrame(t, ctx, ws, jobStreamOne, 1, "y\r")
	if reply := nextReply(t, replies); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("socket after the server timeouts: %+v", reply)
	}
}
