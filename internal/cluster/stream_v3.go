package cluster

// Stream transport v3 keeps the v2 Noise identities and the fixed stream
// endpoint, but lets a full Panel use one authenticated socket per terminal
// session and a small pool of reusable sockets for file requests. Targets
// advertise support explicitly; nothing is probed, so an older Panel is never
// sent a role it would reject after the WebSocket upgrade.

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	PanelStreamCapability = "panel-stream-v3"

	streamRolePanelFile            = "panel-file"
	streamRolePanelTerminal        = "panel-terminal"
	streamRoleLightTerminalControl = "light-terminal-control"
	streamRoleLightTerminalData    = "light-terminal-data"

	panelStreamLegacyTTL  = 2 * time.Minute
	panelFilePoolIdle     = 20 * time.Second
	panelFilePoolMaxAge   = 10 * time.Minute
	panelFilePoolSize     = 2
	streamTerminalIdleTTL = 40 * time.Minute
	lightTerminalDetach   = 2 * time.Minute
)

// streamDialError marks a failure before any request record was sent, which
// is always safe to retry over the v2 transport.
type streamDialError struct{ err error }

func (e *streamDialError) Error() string { return "stream dial failed: " + e.err.Error() }
func (e *streamDialError) Unwrap() error { return e.err }

func isStreamDialError(err error) bool {
	var target *streamDialError
	return errors.As(err, &target)
}

type remoteV2StreamAPI interface {
	dialPanelStream(ctx, parent context.Context, origin, controllerID, targetID string, key noise.DHKey, peer []byte, role string) (*fileStreamConn, error)
}

type remoteV2CapabilityAPI interface {
	SummaryV2WithCapabilities(context.Context, string, string, string, noise.DHKey, []byte, time.Time) (FederationSummary, string, error)
}

func (c *RemoteClient) dialPanelStream(ctx, parent context.Context, origin, controllerID, targetID string, key noise.DHKey, peer []byte, role string) (*fileStreamConn, error) {
	if c == nil || c.wsClient == nil {
		return nil, ErrFileStreamUnsupported
	}
	if normalized, err := NormalizeV2Origin(origin); err != nil || normalized != origin {
		return nil, ErrInvalidOrigin
	}
	return dialFileStreamWithParent(ctx, parent, c.wsClient, origin, controllerID, targetID, key, peer, time.Now(), fileStreamHello{Role: role})
}

type streamHostState struct {
	capable     bool
	legacyUntil time.Time
}

type pooledStreamConn struct {
	conn      *fileStreamConn
	idleSince time.Time
	createdAt time.Time
}

type panelStreams struct {
	ctx       context.Context
	cancel    context.CancelFunc
	limits    *fileStreamLimits
	mu        sync.Mutex
	hosts     map[string]streamHostState
	idle      map[string][]pooledStreamConn
	terminals map[string]*streamTerminal
}

func newPanelStreams() *panelStreams {
	ctx, cancel := context.WithCancel(context.Background())
	streams := &panelStreams{ctx: ctx, cancel: cancel, limits: newFileStreamLimits(),
		hosts: make(map[string]streamHostState), idle: make(map[string][]pooledStreamConn), terminals: make(map[string]*streamTerminal)}
	go streams.janitor()
	return streams
}

func (p *panelStreams) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.expire(time.Now())
		}
	}
}

func (p *panelStreams) expire(now time.Time) {
	var stale []*streamTerminal
	p.mu.Lock()
	for key, t := range p.terminals {
		if now.Sub(t.idleSince()) >= streamTerminalIdleTTL || t.ctx.Err() != nil {
			delete(p.terminals, key)
			stale = append(stale, t)
		}
	}
	for host, conns := range p.idle {
		kept := conns[:0]
		for _, item := range conns {
			if item.conn.ctx.Err() == nil && now.Sub(item.idleSince) < panelFilePoolIdle && now.Sub(item.createdAt) < panelFilePoolMaxAge {
				kept = append(kept, item)
			} else {
				item.conn.close()
			}
		}
		if len(kept) == 0 {
			delete(p.idle, host)
		} else {
			p.idle[host] = kept
		}
	}
	p.mu.Unlock()
	for _, t := range stale {
		t.shutdown()
	}
}

func (p *panelStreams) closeAll() {
	if p == nil {
		return
	}
	p.cancel()
	p.mu.Lock()
	terminals := p.terminals
	idle := p.idle
	p.terminals = make(map[string]*streamTerminal)
	p.idle = make(map[string][]pooledStreamConn)
	p.mu.Unlock()
	for _, t := range terminals {
		t.shutdown()
	}
	for _, conns := range idle {
		for _, item := range conns {
			item.conn.close()
		}
	}
}

// forgetHost drops capability, pooled sockets and terminal handles after a
// host is removed or its authorization changes.
func (p *panelStreams) forgetHost(hostID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	delete(p.hosts, hostID)
	idle := p.idle[hostID]
	delete(p.idle, hostID)
	var terminals []*streamTerminal
	for key, t := range p.terminals {
		if t.hostID == hostID {
			delete(p.terminals, key)
			terminals = append(terminals, t)
		}
	}
	p.mu.Unlock()
	for _, item := range idle {
		item.conn.close()
	}
	for _, t := range terminals {
		t.shutdown()
	}
}

func (p *panelStreams) setCapable(hostID string, capable bool) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.hosts[hostID]
	state.capable = capable
	p.hosts[hostID] = state
}

func (p *panelStreams) usable(hostID string) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.hosts[hostID]
	return state.capable && time.Now().After(state.legacyUntil)
}

func (p *panelStreams) markLegacy(hostID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.hosts[hostID]
	state.legacyUntil = time.Now().Add(panelStreamLegacyTTL)
	p.hosts[hostID] = state
}

func (p *panelStreams) takeIdle(hostID string) (*fileStreamConn, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	conns := p.idle[hostID]
	for len(conns) > 0 {
		item := conns[len(conns)-1]
		conns = conns[:len(conns)-1]
		if item.conn.ctx.Err() == nil && now.Sub(item.idleSince) < panelFilePoolIdle && now.Sub(item.createdAt) < panelFilePoolMaxAge {
			p.idle[hostID] = conns
			return item.conn, item.createdAt
		}
		item.conn.close()
	}
	delete(p.idle, hostID)
	return nil, time.Time{}
}

func (p *panelStreams) putIdle(hostID string, conn *fileStreamConn, createdAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if p.ctx.Err() != nil || conn.ctx.Err() != nil || len(p.idle[hostID]) >= panelFilePoolSize || now.Sub(createdAt) >= panelFilePoolMaxAge {
		conn.close()
		return
	}
	p.idle[hostID] = append(p.idle[hostID], pooledStreamConn{conn: conn, idleSince: now, createdAt: createdAt})
}

func streamTerminalKey(hostID, sessionID string) string { return hostID + "\x00" + sessionID }

func (p *panelStreams) terminal(hostID, sessionID string) *streamTerminal {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.terminals[streamTerminalKey(hostID, sessionID)]
}

func (p *panelStreams) putTerminal(t *streamTerminal) {
	p.mu.Lock()
	p.terminals[streamTerminalKey(t.hostID, t.sessionID)] = t
	p.mu.Unlock()
}

func (p *panelStreams) deleteTerminal(t *streamTerminal) {
	p.mu.Lock()
	if p.terminals[streamTerminalKey(t.hostID, t.sessionID)] == t {
		delete(p.terminals, streamTerminalKey(t.hostID, t.sessionID))
	}
	p.mu.Unlock()
}

// dialPanelHost authenticates one outbound socket to an active paired Panel.
func (s *Service) dialPanelHost(ctx context.Context, hostID, role string) (*fileStreamConn, error) {
	remote, ok := s.remoteV2.(remoteV2StreamAPI)
	if !ok {
		return nil, &streamDialError{ErrFileStreamUnsupported}
	}
	record, err := s.storeV2.Host(hostID)
	if err != nil || record.State != hostStateV2Active {
		return nil, &streamDialError{ErrFileRelayUnavailable}
	}
	scope := normalizedV2Scope(record.Scope)
	if (role == streamRolePanelFile && !ScopeAllowsFiles(scope)) || (role == streamRolePanelTerminal && !ScopeAllowsTerminal(scope)) {
		return nil, &streamDialError{ErrFileRelayUnavailable}
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return nil, &streamDialError{err}
	}
	conn, err := remote.dialPanelStream(ctx, s.streams.ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
		noiseKeyV2(credential), credential.TargetPublic, role)
	if err != nil {
		return nil, &streamDialError{err}
	}
	s.fileStreamHub.track(conn, "host:"+hostID)
	return conn, nil
}

func idempotentStreamRequest(input LightFileRequest) bool {
	return (input.Method == http.MethodGet || input.Method == http.MethodHead) &&
		(input.Body == nil || input.Body == http.NoBody) && input.BodyLength == 0
}

// openPanelFileStream performs one file request over a pooled socket. A
// stale pooled socket that fails before any response is retried once on a
// fresh socket for idempotent requests only.
func (s *Service) openPanelFileStream(ctx context.Context, hostID string, input LightFileRequest) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		release, ok := s.streams.limits.acquire(hostID, input)
		if !ok {
			return nil, ErrRateLimited
		}
		conn, createdAt := (*fileStreamConn)(nil), time.Time{}
		if attempt == 0 {
			conn, createdAt = s.streams.takeIdle(hostID)
		}
		reused := conn != nil
		if conn == nil {
			var err error
			conn, err = s.dialPanelHost(ctx, hostID, streamRolePanelFile)
			if err != nil {
				release()
				return nil, err
			}
			createdAt = time.Now()
		}
		stopCancel := context.AfterFunc(ctx, conn.close)
		response, err := exchangeStreamRequest(conn, input, func(clean bool) {
			stopCancel()
			if clean {
				s.streams.putIdle(hostID, conn, createdAt)
			}
			release()
		})
		if err == nil {
			return response, nil
		}
		if !reused || !idempotentStreamRequest(input) || ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, ErrFileRelayUnavailable
}

// startTerminalStream sends the first record on a fresh socket and waits for
// the target's ready or reject answer.
func startTerminalStream(conn *fileStreamConn, first byte, payload any) (*fileStreamConn, terminalStreamReady, error) {
	if err := conn.writeJSON(first, payload); err != nil {
		conn.close()
		return nil, terminalStreamReady{}, &streamDialError{err}
	}
	kind, data, err := conn.readControl()
	if err != nil {
		conn.close()
		return nil, terminalStreamReady{}, &streamDialError{err}
	}
	switch kind {
	case termReady:
		var ready terminalStreamReady
		if decodeV2Payload(data, &ready) != nil || ready.SessionID == "" || len(ready.SessionID) > 128 || ready.Offset < 0 {
			conn.close()
			return nil, terminalStreamReady{}, ErrAuthentication
		}
		return conn, ready, nil
	case termReject:
		var result terminalStreamResult
		_ = decodeV2Payload(data, &result)
		conn.close()
		if code := terminalStreamError(result.Code); code != nil {
			return nil, terminalStreamReady{}, code
		}
		return nil, terminalStreamReady{}, ErrTerminalUnavailable
	default:
		conn.close()
		return nil, terminalStreamReady{}, ErrAuthentication
	}
}

type panelTerminalFallback struct {
	service *Service
	hostID  string
}

func (f panelTerminalFallback) Output(ctx context.Context, input TerminalOutputRequest) (terminal.Output, error) {
	record, credential, remote, err := f.service.terminalHostCredential(f.hostID)
	if err != nil {
		return terminal.Output{}, err
	}
	return remote.TerminalOutputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, f.service.now().UTC(), input)
}

func (f panelTerminalFallback) Input(ctx context.Context, input TerminalInputRequest) error {
	record, credential, remote, err := f.service.terminalHostCredential(f.hostID)
	if err != nil {
		return err
	}
	return remote.TerminalInputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, f.service.now().UTC(), input)
}

func (f panelTerminalFallback) Resize(ctx context.Context, input TerminalResizeRequest) error {
	record, credential, remote, err := f.service.terminalHostCredential(f.hostID)
	if err != nil {
		return err
	}
	return remote.TerminalResizeV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, f.service.now().UTC(), input)
}

func (f panelTerminalFallback) Close(ctx context.Context, input TerminalCloseRequest) error {
	record, credential, remote, err := f.service.terminalHostCredential(f.hostID)
	if err != nil {
		return err
	}
	return remote.TerminalCloseV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, f.service.now().UTC(), input)
}

func (s *Service) openPanelStreamTerminal(ctx context.Context, hostID string, rows, columns uint16) (TerminalOpenResponse, error) {
	dial := func(ctx context.Context, first byte, payload any) (*fileStreamConn, terminalStreamReady, error) {
		conn, err := s.dialPanelHost(ctx, hostID, streamRolePanelTerminal)
		if err != nil {
			return nil, terminalStreamReady{}, err
		}
		return startTerminalStream(conn, first, payload)
	}
	t, response, err := openStreamTerminal(s.streams.ctx, hostID, dial, panelTerminalFallback{service: s, hostID: hostID}, rows, columns)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	s.streams.putTerminal(t)
	return response, nil
}

func (s *Service) openLightStreamTerminal(ctx context.Context, nodeID string, rows, columns uint16) (TerminalOpenResponse, error) {
	dial := func(ctx context.Context, first byte, payload any) (*fileStreamConn, terminalStreamReady, error) {
		conn, err := s.fileStreamHub.openLightTerminalConn(ctx, nodeID)
		if err != nil {
			return nil, terminalStreamReady{}, &streamDialError{err}
		}
		return startTerminalStream(conn, first, payload)
	}
	t, response, err := openStreamTerminal(s.streams.ctx, nodeID, dial, nil, rows, columns)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	s.streams.putTerminal(t)
	return response, nil
}

// managerTerminalBackend adapts a local PTY manager for the stream server.
type managerTerminalBackend struct{ manager *terminal.Manager }

func (b managerTerminalBackend) Open(_ context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	return b.manager.Open(owner, rows, columns)
}

func (b managerTerminalBackend) Output(ctx context.Context, owner, id string, offset int64, wait time.Duration) (terminal.Output, error) {
	return b.manager.Output(ctx, owner, id, offset, wait)
}

func (b managerTerminalBackend) Input(_ context.Context, owner, id string, data []byte) error {
	return b.manager.Input(owner, id, data)
}

func (b managerTerminalBackend) Resize(_ context.Context, owner, id string, rows, columns uint16) error {
	return b.manager.Resize(owner, id, rows, columns)
}

func (b managerTerminalBackend) Close(_ context.Context, owner, id string) error {
	return b.manager.Close(owner, id)
}

// RunTerminalStream serves one outbound terminal control connection for the
// light node's root broker. It runs beside the polling relay: sessions opened
// over the relay stay there, and the center only prefers the stream while
// this control connection is alive. connected is called once the center has
// accepted the control role, letting the caller reset its backoff.
func (c *TerminalRelayClient) RunTerminalStream(ctx context.Context, origin, node, target string, key noise.DHKey, peer []byte, manager *terminal.Manager, owner string, connected func()) error {
	if c == nil || c.client == nil || manager == nil {
		return ErrAuthentication
	}
	if normalized, err := validateLightOrigin(origin); err != nil || normalized != origin {
		return ErrInvalidOrigin
	}
	control, err := dialFileStream(ctx, c.client, origin, node, target, key, peer, time.Now(), fileStreamHello{Role: streamRoleLightTerminalControl})
	if err != nil {
		return err
	}
	defer control.close()
	kind, data, err := control.read()
	if err != nil || kind != streamOpen || !validID(string(data)) {
		return ErrAuthentication
	}
	if connected != nil {
		connected()
	}
	generation := string(data)
	backend := managerTerminalBackend{manager: manager}
	attachments := newTerminalAttachments(lightTerminalDetach, func(id string) { _ = manager.Close(owner, id) })
	// Established sessions outlive this control socket so a reconnect does not
	// interrupt open shells; the gate only bounds concurrent dial-backs.
	gate := make(chan struct{}, terminal.DefaultMaxOwnerSessions)
	defer control.close()
	for {
		kind, data, err := control.readControl()
		if err != nil {
			return err
		}
		switch kind {
		case streamPing:
			if len(data) != 0 {
				return ErrAuthentication
			}
			if err := control.write(streamPong, nil); err != nil {
				return err
			}
		case streamOpen:
			if !validID(string(data)) {
				return ErrAuthentication
			}
			requestID := string(data)
			select {
			case gate <- struct{}{}:
			default:
				if err := control.write(streamReject, []byte(requestID)); err != nil {
					return err
				}
				continue
			}
			go func() {
				defer func() { <-gate }()
				conn, err := dialFileStreamWithParent(control.ctx, ctx, c.client, origin, node, target, key, peer, time.Now(),
					fileStreamHello{Role: streamRoleLightTerminalData, RequestID: requestID, Generation: generation})
				if err == nil {
					serveTerminalStream(conn, backend, owner, attachments)
				}
			}()
		default:
			return ErrAuthentication
		}
	}
}
