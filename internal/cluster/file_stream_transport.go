package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"
)

type fileStreamHub struct {
	mu          sync.Mutex
	ctx         context.Context
	stop        context.CancelFunc
	connections map[*fileStreamConn]string
	controls    map[string]*fileStreamControl
	streamNodes map[string]bool
	pending     map[string]*fileStreamPending
	requests    map[*fileStreamLease]string
	preauth     chan struct{}
	sockets     *fileStreamLimiter
	limits      *fileStreamLimits
}

type fileStreamLease struct {
	ctx  context.Context
	stop context.CancelFunc
}
type fileStreamControl struct {
	conn       *fileStreamConn
	generation string
}
type fileStreamPending struct {
	nodeID  string
	control *fileStreamControl
	ready   chan *fileStreamConn
	expires time.Time
}

func newFileStreamHub() *fileStreamHub {
	ctx, stop := context.WithCancel(context.Background())
	return &fileStreamHub{ctx: ctx, stop: stop, connections: make(map[*fileStreamConn]string),
		controls: make(map[string]*fileStreamControl), streamNodes: make(map[string]bool), pending: make(map[string]*fileStreamPending),
		requests: make(map[*fileStreamLease]string), preauth: make(chan struct{}, 64),
		sockets: newFileStreamLimiter(128, 10), limits: newFileStreamLimits()}
}

func (h *fileStreamHub) closePeer(peer string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c, owner := range h.connections {
		if owner == peer {
			c.close()
		}
	}
	for lease, owner := range h.requests {
		if owner == peer {
			lease.stop()
		}
	}
}

func (h *fileStreamHub) closeAll() {
	h.stop()
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.connections {
		c.close()
	}
	for lease := range h.requests {
		lease.stop()
	}
}

func (h *fileStreamHub) available(node string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	control := h.controls[node]
	return control != nil && control.conn.ctx.Err() == nil
}

func (h *fileStreamHub) prefersStream(node string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.streamNodes[node]
}

// An authenticated legacy poll received after the stream disconnects is an
// explicit compatibility signal (for example a binary rollback). Old cached
// poll liveness alone must never downgrade a new stream-capable broker.
func (h *fileStreamHub) observeLegacy(node string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if control := h.controls[node]; control == nil || control.conn.ctx.Err() != nil {
		delete(h.streamNodes, node)
	}
}

func (h *fileStreamHub) forgetNode(node string) {
	h.closePeer("light:" + node)
	h.mu.Lock()
	delete(h.streamNodes, node)
	h.mu.Unlock()
}

func (h *fileStreamHub) requestContext(ctx context.Context, peer string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := context.WithCancel(ctx)
	lease := &fileStreamLease{ctx, stop}
	h.mu.Lock()
	h.requests[lease] = peer
	h.mu.Unlock()
	stopParent := context.AfterFunc(h.ctx, stop)
	context.AfterFunc(ctx, func() { stopParent(); h.mu.Lock(); delete(h.requests, lease); h.mu.Unlock() })
	return ctx, func() { stop(); stopParent(); h.mu.Lock(); delete(h.requests, lease); h.mu.Unlock() }
}

// Dial preserves the caller's TLS roots, redirect policy, approved-IP dialer
// (Panel) or environment proxy (light broker). No alternate raw dial path exists.
func dialFileStream(ctx context.Context, client *http.Client, origin, controllerID, targetID string, key noise.DHKey, peer []byte, now time.Time, hello fileStreamHello) (*fileStreamConn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(hello)
	if err != nil {
		return nil, err
	}
	envelope, handshake, err := sealV2Request(http.MethodGet, FileStreamV2Path,
		v2Envelope{Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: targetID, Timestamp: now.UTC().Unix(), RequestID: requestID}, key, peer, nil, payload)
	if err != nil {
		return nil, err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, streamHandshakeTimeout)
	defer cancel()
	ws, response, err := websocket.Dial(handshakeCtx, origin+FileStreamV2Path, &websocket.DialOptions{
		HTTPClient: client, Subprotocols: []string{fileStreamProtocol}, CompressionMode: websocket.CompressionDisabled,
		HTTPHeader: http.Header{"User-Agent": []string{"KPanel federation/" + FederationProtocolV2}},
	})
	if err != nil {
		if response != nil {
			switch response.StatusCode {
			case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUpgradeRequired:
				return nil, ErrFileStreamUnsupported
			}
		}
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = ws.CloseNow()
		}
	}()
	if ws.Subprotocol() != fileStreamProtocol {
		return nil, ErrProtocolMismatch
	}
	ws.SetReadLimit(maxV2EnvelopeBytes)
	helloBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if err = ws.Write(handshakeCtx, websocket.MessageBinary, helloBytes); err != nil {
		return nil, err
	}
	kind, reply, err := ws.Read(handshakeCtx)
	if err != nil {
		return nil, err
	}
	if kind != websocket.MessageBinary {
		return nil, ErrAuthentication
	}
	// The responder's second IK message is already bound to the full request
	// prologue; no duplicated unauthenticated response envelope is necessary.
	plain, tx, rx, err := handshake.ReadMessage(nil, reply)
	if err != nil || !bytes.Equal(plain, []byte(fileStreamProtocol)) || tx == nil || rx == nil {
		return nil, ErrAuthentication
	}
	failed = false
	return newFileStreamConn(ctx, ws, tx, rx), nil
}

// ServeFileStream authenticates a GET upgrade before any file operation is
// sent. Requests and keys are bound to this exact route and Noise prologue.
type FileStreamResult struct {
	Upgraded      bool
	Authenticated bool
	Completed     bool
	StatusCode    int
	PeerID        string
	Role          string
}

func (s *Service) ServeFileStream(w http.ResponseWriter, r *http.Request, source string) (result FileStreamResult) {
	h := s.fileStreamHub
	if r.Method != http.MethodGet || r.URL.Path != FileStreamV2Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || h == nil {
		http.NotFound(w, r)
		return
	}
	if !s.fileSources.Allow(cleanRateSubject(source), s.now().UTC()) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	select {
	case h.preauth <- struct{}{}:
	default:
		http.Error(w, "busy", http.StatusTooManyRequests)
		return
	}
	var preauthOnce sync.Once
	releasePreauth := func() { preauthOnce.Do(func() { <-h.preauth }) }
	defer releasePreauth()
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{fileStreamProtocol}, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	result.Upgraded = true
	if ws.Subprotocol() != fileStreamProtocol {
		return
	}
	ws.SetReadLimit(maxV2EnvelopeBytes)
	ctx, cancel := context.WithTimeout(h.ctx, streamHandshakeTimeout)
	defer cancel()
	kind, data, err := ws.Read(ctx)
	var envelope v2Envelope
	if err != nil || kind != websocket.MessageBinary || decodeV2Payload(data, &envelope) != nil {
		return
	}
	// Registration and revocation cleanup use the same lock. Revocation first
	// changes the store, then closes registered sockets; no stale auth can enter.
	h.mu.Lock()
	hello, handshake, owner, err := s.authorizeFileStream(envelope)
	if err != nil {
		h.mu.Unlock()
		return
	}
	result.Authenticated, result.PeerID, result.Role = true, envelope.ControllerID, hello.Role
	release, ok := h.sockets.acquire(envelope.ControllerID)
	if !ok {
		h.mu.Unlock()
		return
	}
	defer release()
	message, rx, tx, err := handshake.WriteMessage(nil, []byte(fileStreamProtocol))
	if err != nil || tx == nil || rx == nil {
		h.mu.Unlock()
		return
	}
	c := newFileStreamConn(h.ctx, ws, tx, rx)
	defer func() { result.Completed = c.completed.Load(); result.StatusCode = int(c.responseStatus.Load()) }()
	h.connections[c] = owner
	h.mu.Unlock()
	defer func() { c.close(); h.mu.Lock(); delete(h.connections, c); h.mu.Unlock() }()
	if err := ws.Write(ctx, websocket.MessageBinary, message); err != nil {
		return
	}
	cancel()
	releasePreauth()
	switch hello.Role {
	case "panel", "linked":
		s.panelFileRelay.mu.Lock()
		handler := s.panelFileRelay.handler
		s.panelFileRelay.mu.Unlock()
		if handler == nil {
			return
		}
		serveStreamRequest(c, handler, h.limits, envelope.ControllerID, hello.Role == "linked")
	case "light-control":
		h.serveControl(c, envelope.ControllerID, envelope.RequestID)
	case "light-data":
		h.mu.Lock()
		pending := h.pending[hello.RequestID]
		if pending == nil || pending.nodeID != envelope.ControllerID || pending.control != h.controls[pending.nodeID] ||
			pending.control.generation != hello.Generation || !pending.expires.After(time.Now()) {
			h.mu.Unlock()
			return
		}
		delete(h.pending, hello.RequestID)
		pending.ready <- c
		h.mu.Unlock()
		<-c.ctx.Done()
	}
	return
}

func (s *Service) authorizeFileStream(envelope v2Envelope) (fileStreamHello, *noise.HandshakeState, string, error) {
	fail := func() (fileStreamHello, *noise.HandshakeState, string, error) {
		return fileStreamHello{}, nil, "", ErrAuthentication
	}
	now := s.now().UTC()
	if validateV2Envelope(envelope, true) != nil || envelope.TargetID != s.NodeID() || envelope.CodeID != "" ||
		time.Unix(envelope.Timestamp, 0).Before(now.Add(-v2RequestSkew)) || time.Unix(envelope.Timestamp, 0).After(now.Add(v2RequestSkew)) {
		return fail()
	}
	key := nodeNoiseKeyV2(s.nodeIdentityV2)
	var expected []byte
	role, owner := "", ""
	if controller, err := s.storeV2.Controller(envelope.ControllerID); err == nil {
		if controller.State != controllerStateV2Active || !ScopeAllowsFiles(controller.Scope) {
			return fail()
		}
		expected, _ = base64.RawURLEncoding.DecodeString(controller.PublicKey)
		role, owner = "panel", "controller:"+controller.ID
	} else if node, err := s.light.Host(envelope.ControllerID); err == nil {
		expected, _ = s.light.ReadTerminalPublicKey(node)
		role, owner = "light", "light:"+node.ID
	} else {
		grant, err := s.filePeersV2.ActiveGrant(envelope.ControllerID, now)
		if err != nil || grant.LinkID != envelope.ControllerID || grant.Scope != filePeerReadScope {
			return fail()
		}
		host, err := s.storeV2.Host(grant.HostID)
		if err != nil || host.State != hostStateV2Active || !ScopeAllowsFiles(normalizedV2Scope(host.Scope)) ||
			host.ControllerID != grant.HostControllerID || host.TransactionID != grant.HostTransaction ||
			host.RemoteNodeID != grant.PeerNodeID || host.PeerFingerprint != grant.PeerFingerprint {
			return fail()
		}
		credential, err := s.secretsV2.ReadCredential(host.CredentialFile)
		if err != nil {
			return fail()
		}
		public, err := base64.RawURLEncoding.DecodeString(host.TargetPublicKey)
		if err != nil || !bytes.Equal(public, credential.TargetPublic) || fingerprintV2(public) != host.PeerFingerprint {
			return fail()
		}
		key, expected = noiseKeyV2(credential), credential.TargetPublic
		role, owner = "linked", "host:"+host.ID
	}
	plain, peer, handshake, err := openV2Request(http.MethodGet, FileStreamV2Path, envelope, key, nil)
	if err != nil || len(expected) != 32 || !bytes.Equal(peer, expected) {
		return fail()
	}
	var hello fileStreamHello
	if decodeV2Payload(plain, &hello) != nil {
		return fail()
	}
	if role == "light" {
		if hello.Role != "light-control" && hello.Role != "light-data" {
			return fail()
		}
	} else if hello.Role != role {
		return fail()
	}
	if hello.Role == "light-data" {
		if !validID(hello.RequestID) || !validID(hello.Generation) {
			return fail()
		}
	} else if hello.RequestID != "" || hello.Generation != "" {
		return fail()
	}
	if !s.panelFileRequests.Allow(envelope.ControllerID, now) || s.replays.Accept(envelope.ControllerID, envelope.RequestID, now) != nil {
		return fail()
	}
	return hello, handshake, owner, nil
}

func (h *fileStreamHub) serveControl(c *fileStreamConn, node, generation string) {
	control := &fileStreamControl{c, generation}
	if c.write(streamOpen, []byte(generation)) != nil {
		return
	}
	h.mu.Lock()
	if c.ctx.Err() != nil {
		h.mu.Unlock()
		return
	}
	if previous := h.controls[node]; previous != nil {
		previous.conn.close()
	}
	h.controls[node] = control
	h.streamNodes[node] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.controls[node] == control {
			delete(h.controls, node)
		}
		h.mu.Unlock()
	}()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if c.write(streamPing, nil) != nil {
					return
				}
			}
		}
	}()
	for {
		kind, data, err := c.readControl()
		if err != nil {
			return
		}
		if kind == streamReject && validID(string(data)) {
			h.mu.Lock()
			pending := h.pending[string(data)]
			if pending != nil && pending.nodeID == node && pending.control == control {
				delete(h.pending, string(data))
				pending.ready <- nil
			}
			h.mu.Unlock()
			continue
		}
		if kind != streamPong || len(data) != 0 {
			return
		}
	}
}

func (h *fileStreamHub) openLight(ctx context.Context, node string, input LightFileRequest) (*http.Response, error) {
	if !validFileRelayRequest(input) {
		return nil, ErrAuthentication
	}
	release, ok := h.limits.acquire(node, input)
	if !ok {
		return nil, ErrRateLimited
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	ctx, stop := h.requestContext(ctx, "light:"+node)
	requestID, err := randomHex(16)
	if err != nil {
		stop()
		return nil, err
	}
	h.mu.Lock()
	control := h.controls[node]
	if control == nil || control.conn.ctx.Err() != nil {
		h.mu.Unlock()
		stop()
		return nil, ErrFileRelayUnavailable
	}
	pending := &fileStreamPending{node, control, make(chan *fileStreamConn, 1), time.Now().Add(streamHandshakeTimeout)}
	h.pending[requestID] = pending
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, requestID)
		select {
		case abandoned := <-pending.ready:
			if abandoned != nil {
				abandoned.close()
			}
		default:
		}
		h.mu.Unlock()
	}()
	if err := control.conn.write(streamOpen, []byte(requestID)); err != nil {
		stop()
		return nil, err
	}
	wait, cancel := context.WithTimeout(ctx, streamHandshakeTimeout)
	defer cancel()
	var c *fileStreamConn
	select {
	case c = <-pending.ready:
		if c == nil {
			stop()
			return nil, ErrRateLimited
		}
	case <-control.conn.ctx.Done():
		stop()
		return nil, ErrFileRelayUnavailable
	case <-wait.Done():
		stop()
		return nil, wait.Err()
	}
	stopConn := context.AfterFunc(ctx, c.close)
	response, err := openStreamRequest(c, input)
	if err != nil {
		stopConn()
		stop()
		return nil, err
	}
	ownedRelease := release
	response.Body = &streamOwnedBody{ReadCloser: response.Body, done: func() { stopConn(); stop(); ownedRelease() }}
	context.AfterFunc(c.ctx, func() { stop(); ownedRelease() })
	release = nil
	return response, nil
}

type streamOwnedBody struct {
	io.ReadCloser
	once sync.Once
	done func()
}

func (b *streamOwnedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.Close()
	}
	return n, err
}
func (b *streamOwnedBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.done); return err }

func (c *RemoteClient) openUnifiedFile(ctx context.Context, origin, controller, target string, key noise.DHKey, peer []byte, now time.Time, role string, input LightFileRequest) (*http.Response, error) {
	c.fileStreamOnce.Do(func() { c.fileStreamLimits = newFileStreamLimits() })
	if normalized, err := NormalizeV2Origin(origin); err != nil || normalized != origin {
		return nil, ErrInvalidOrigin
	}
	if !validFileRelayRequest(input) {
		return nil, ErrAuthentication
	}
	release, ok := c.fileStreamLimits.acquire(controller, input)
	if !ok {
		return nil, ErrRateLimited
	}
	conn, err := dialFileStream(ctx, c.streamClient, origin, controller, target, key, peer, now, fileStreamHello{Role: role})
	if err != nil {
		release()
		return nil, err
	}
	response, err := openStreamRequest(conn, input)
	if err != nil {
		release()
		return nil, err
	}
	response.Body = &streamOwnedBody{ReadCloser: response.Body, done: release}
	context.AfterFunc(conn.ctx, release)
	return response, nil
}

// RunFileStream serves one authenticated control connection. Reconnection is
// handled by the broker; only an explicit pre-upgrade unsupported response
// permits its legacy polling loop. Already opened requests are never replayed.
func (c *FileRelayClient) RunFileStream(ctx context.Context, origin, node, target string, key noise.DHKey, peer []byte, handler http.Handler) error {
	if c == nil || c.client == nil || c.history || handler == nil {
		return ErrAuthentication
	}
	if normalized, err := validateLightOrigin(origin); err != nil || normalized != origin {
		return ErrInvalidOrigin
	}
	control, err := dialFileStream(ctx, c.client, origin, node, target, key, peer, time.Now(), fileStreamHello{Role: "light-control"})
	if err != nil {
		return err
	}
	defer control.close()
	kind, data, err := control.read()
	if err != nil || kind != streamOpen || !validID(string(data)) {
		return ErrAuthentication
	}
	generation := string(data)
	gate := make(chan struct{}, 8)
	limits := newFileStreamLimits()
	var workers sync.WaitGroup
	defer workers.Wait()
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
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-gate }()
				conn, err := dialFileStream(control.ctx, c.client, origin, node, target, key, peer, time.Now(), fileStreamHello{Role: "light-data", RequestID: requestID, Generation: generation})
				if err == nil {
					serveStreamRequest(conn, handler, limits, target, false)
				}
			}()
		default:
			return ErrAuthentication
		}
	}
}
