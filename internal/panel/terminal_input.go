package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const terminalInputSocketProtocol = "kpanel-terminal-input-v1"

// terminalInputKeepalive is how often an input socket pings its page; a page
// that does not answer within the same time is dropped. An input socket is idle
// whenever nobody types, and reverse proxies close idle upgraded connections
// (nginx after 60 s without upstream data by default, Cloudflare after about
// 100 s), so the ping is also what keeps it open behind one. Tests shorten it.
var terminalInputKeepalive = 20 * time.Second

type terminalInputConnection struct{ cancel context.CancelFunc }
type terminalInputMessage struct {
	Type   string               `json:"type"`
	CSRF   string               `json:"csrf,omitempty"`
	Stream string               `json:"stream,omitempty"`
	Frame  *terminal.InputFrame `json:"frame,omitempty"`
}
type terminalInputReply struct {
	Type      string `json:"type"`
	Seq       uint64 `json:"seq,omitempty"`
	Code      string `json:"code,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	Window    int    `json:"window,omitempty"`
	Epoch     string `json:"epoch,omitempty"`
}
type terminalInputWaiter struct {
	seq  uint64
	wait func() error
}

func decodeTerminalSocketJSON(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid terminal JSON")
	}
	return nil
}

// The browser may send its next frame as soon as it receives an ACK, before
// Write returns here. Make that capacity available before publishing the ACK.
func publishTerminalInputReply(slots chan struct{}, reply terminalInputReply, write func(terminalInputReply) bool) bool {
	if reply.Type == "ack" {
		<-slots
	}
	return write(reply) && reply.Type != "error"
}

// terminalInputTarget is where an authenticated browser input stream is
// delivered: the PTY session of a host terminal, or a task terminal. The socket
// and batch handlers own authentication, framing, windowing and ordering; the
// target owns admission, liveness and the hop to whoever applies the frames.
type terminalInputTarget struct {
	// admit runs before the upgrade and registers conn. A batch whose first
	// frame is not a claim passes requireClaimed. A non-zero status is the
	// problem to answer with.
	admit func(conn *terminalInputConnection, requireClaimed bool) (release func(), status int, code, title string)
	// alive revalidates the target for conn and records activity.
	alive func(conn *terminalInputConnection) bool
	// claim binds the writer (seq=0) and returns the epoch of the state that
	// holds it, when that state can be lost independently of the browser.
	claim func(ctx context.Context, frame terminal.InputFrame) (epoch string, err error)
	// send starts delivering one data frame; wait reports whether it was applied.
	send func(ctx context.Context, frame terminal.InputFrame) (wait func() error, err error)
	// immediate reports that send has already applied the frame when it returns,
	// so a batch can stop at the first failure instead of queueing the rest.
	immediate func() bool
	// claimed records a successful claim.
	claimed func(conn *terminalInputConnection)
	// keepalive, when positive, pings the browser so that a proxy does not
	// close an idle socket and a vanished page frees its slot instead of
	// waiting for TCP to notice.
	keepalive time.Duration
}

func (s *Server) handleTerminalInputSocket(w http.ResponseWriter, r *http.Request, token string, session auth.Session, id string) {
	s.serveTerminalInputSocket(w, r, token, session, s.hostTerminalInputTarget(session, id))
}

func (s *Server) serveTerminalInputSocket(w http.ResponseWriter, r *http.Request, token string, session auth.Session, target terminalInputTarget) {
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Origin") == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal input stream", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	connection := &terminalInputConnection{cancel: cancel}
	release, status, code, title := target.admit(connection, false)
	if status != 0 {
		s.writeProblem(w, r, status, code, title, "")
		return
	}
	defer release()
	// checkOrigin above is authoritative, including the configured trusted
	// proxy/PublicURL rules. No credentials are passed in URL or subprotocol.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{terminalInputSocketProtocol}, InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	if ws.Subprotocol() != terminalInputSocketProtocol {
		return
	}
	ws.SetReadLimit(4096)
	read := func(readCtx context.Context) (terminalInputMessage, error) {
		kind, data, readErr := ws.Read(readCtx)
		var message terminalInputMessage
		if readErr != nil {
			return message, readErr
		}
		if kind != websocket.MessageText {
			return message, errors.New("invalid terminal input frame")
		}
		if err := decodeTerminalSocketJSON(data, &message); err != nil {
			return message, err
		}
		return message, nil
	}
	write := func(reply terminalInputReply) bool {
		data, _ := json.Marshal(reply)
		writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return ws.Write(writeCtx, websocket.MessageText, data) == nil
	}
	helloCtx, helloCancel := context.WithTimeout(ctx, 5*time.Second)
	hello, err := read(helloCtx)
	helloCancel()
	if err != nil || hello.Type != "auth" || hello.Frame != nil || !(terminal.InputFrame{Stream: hello.Stream}).Valid() || !secureStringEqual(hello.CSRF, s.csrfCookieValue(r)) || s.auth.ValidateCSRF(session, hello.CSRF) != nil {
		write(terminalInputReply{Type: "error", Code: "csrf_validation_failed"})
		return
	}
	valid := func() bool {
		if _, err := s.auth.Authenticate(token); err != nil || !time.Now().Before(session.ExpiresAt) {
			return false
		}
		return target.alive(connection)
	}
	if !valid() {
		return
	}
	claimCtx, claimCancel := context.WithTimeout(ctx, 10*time.Second)
	epoch, err := target.claim(claimCtx, terminal.InputFrame{Stream: hello.Stream})
	claimCancel()
	if err != nil {
		write(terminalInputReply{Type: "error", Code: "terminal_input_claim", Retryable: !errors.Is(err, terminal.ErrInputSequence) && !errors.Is(err, terminal.ErrInputUncertain) && !errors.Is(err, terminal.ErrNotFound) && !errors.Is(err, terminal.ErrClosed)})
		return
	}
	target.claimed(connection)
	if !valid() || !write(terminalInputReply{Type: "ready", Window: terminal.InputWindow, Epoch: epoch}) {
		return
	}
	if target.keepalive > 0 {
		go func() {
			ticker := time.NewTicker(target.keepalive)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pingCtx, stop := context.WithTimeout(ctx, target.keepalive)
					err := ws.Ping(pingCtx)
					stop()
					if err != nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	pending := make(chan terminalInputWaiter, terminal.InputWindow)
	slots := make(chan struct{}, terminal.InputWindow)
	failures := make(chan terminalInputReply, 1)
	// Exactly one dispatcher performs ordered sends. The reply loop below can
	// wait for frame 1 while this reader sends frames 2..32 to the PTY owner.
	go func() {
		defer cancel()
		fail := func(code string, retryable bool) {
			select {
			case failures <- terminalInputReply{Type: "error", Code: code, Retryable: retryable}:
			default:
			}
			<-ctx.Done()
		}
		for {
			message, err := read(ctx)
			if err != nil {
				return
			}
			if message.Type != "input" || message.CSRF != "" || message.Stream != "" || message.Frame == nil || !message.Frame.Valid() || message.Frame.Seq == 0 || message.Frame.Stream != hello.Stream {
				fail("terminal_input_sequence", false)
				return
			}
			select {
			case slots <- struct{}{}:
			default:
				fail("terminal_input_window", false)
				return
			}
			if !valid() {
				fail("session_expired", false)
				return
			}
			frame := *message.Frame
			wait, err := target.send(ctx, frame)
			if err != nil {
				fail("terminal_input_unavailable", true)
				return
			}
			select {
			case pending <- terminalInputWaiter{seq: frame.Seq, wait: wait}:
			case <-ctx.Done():
				return
			}
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Wait independently so auth/expiry is checked even if the PTY or remote
	// acknowledgement stalls. Every helper is bounded by the socket/window.
	acks := make(chan terminalInputReply, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case next := <-pending:
				err := next.wait()
				reply := terminalInputReply{Type: "ack", Seq: next.seq}
				if err != nil {
					reply.Type, reply.Code, reply.Retryable = "error", "terminal_input_unavailable", true
					switch {
					case errors.Is(err, terminal.ErrInputSequence):
						reply.Code, reply.Retryable = "terminal_input_sequence", false
					case errors.Is(err, terminal.ErrInputUncertain):
						reply.Code, reply.Retryable = "terminal_input_uncertain", false
					case errors.Is(err, terminal.ErrClosed), errors.Is(err, terminal.ErrNotFound):
						reply.Code, reply.Retryable = "terminal_not_found", false
					}
				}
				select {
				case acks <- reply:
				case <-ctx.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}
	}()
	for {
		select {
		case failure := <-failures:
			write(failure)
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !valid() {
				write(terminalInputReply{Type: "error", Code: "session_expired"})
				return
			}
		case reply := <-acks:
			if !valid() {
				write(terminalInputReply{Type: "error", Code: "session_expired"})
				return
			}
			if !publishTerminalInputReply(slots, reply, write) {
				return
			}
		}
	}
}

// hostTerminalInputTarget delivers frames to the PTY owner of a host terminal
// session, local or on a cluster host.
func (s *Server) hostTerminalInputTarget(session auth.Session, id string) terminalInputTarget {
	var item panelTerminalSession
	local := func() bool { return item.HostID == cluster.LocalHostID }
	return terminalInputTarget{
		admit: func(conn *terminalInputConnection, requireClaimed bool) (func(), int, string, string) {
			s.terminalMu.Lock()
			current, ok := s.terminalSessions[id]
			if !ok || current.UserID != session.User.ID || current.CloseRequested || !current.ReliableInput {
				s.terminalMu.Unlock()
				return nil, http.StatusNotFound, "terminal_not_found", "Terminal input stream unavailable"
			}
			// At most one socket (including unauthenticated handshakes) per
			// existing terminal: the existing 16 global / 4 user PTY quotas
			// also bound sockets.
			if current.InputConnection != nil {
				s.terminalMu.Unlock()
				return nil, http.StatusTooManyRequests, "terminal_stream_limit", "Terminal input stream already connected"
			}
			if requireClaimed && !current.InputClaimed {
				s.terminalMu.Unlock()
				return nil, http.StatusConflict, "terminal_input_sequence", "Terminal input writer must be claimed"
			}
			current.InputConnection = conn
			s.terminalSessions[id] = current
			s.terminalMu.Unlock()
			item = current
			return func() {
				s.terminalMu.Lock()
				if now, exists := s.terminalSessions[id]; exists && now.InputConnection == conn {
					now.InputConnection = nil
					s.terminalSessions[id] = now
				}
				s.terminalMu.Unlock()
			}, 0, "", ""
		},
		alive: func(conn *terminalInputConnection) bool {
			s.terminalMu.Lock()
			defer s.terminalMu.Unlock()
			current, exists := s.terminalSessions[id]
			if !exists || current.UserID != session.User.ID || current.CloseRequested || current.InputConnection != conn {
				return false
			}
			current.UpdatedAt = time.Now().UTC()
			s.terminalSessions[id] = current
			return true
		},
		claim: func(ctx context.Context, frame terminal.InputFrame) (string, error) {
			if local() {
				return "", (clusterTerminalSource{agent: s.agent}).InputSequenced(ctx, item.Owner, item.BackendSessionID, frame)
			}
			wait, err := s.cluster.BeginTerminalInput(ctx, item.HostID, item.BackendSessionID, frame)
			if err == nil {
				err = wait()
			}
			return "", err
		},
		send: func(ctx context.Context, frame terminal.InputFrame) (func() error, error) {
			if local() {
				inputErr := (clusterTerminalSource{agent: s.agent}).InputSequenced(ctx, item.Owner, item.BackendSessionID, frame)
				return func() error { return inputErr }, nil
			}
			return s.cluster.BeginTerminalInput(ctx, item.HostID, item.BackendSessionID, frame)
		},
		immediate: local,
		keepalive: terminalInputKeepalive,
		claimed: func(conn *terminalInputConnection) {
			s.terminalMu.Lock()
			if current, exists := s.terminalSessions[id]; exists && current.InputConnection == conn {
				current.InputClaimed = true
				s.terminalSessions[id] = current
			}
			s.terminalMu.Unlock()
		},
	}
}
