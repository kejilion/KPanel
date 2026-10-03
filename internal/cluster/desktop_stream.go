package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

const (
	DesktopCapability             = "desktop-v1"
	ManagedDesktopCapability      = "desktop-managed-v1"
	streamRoleLightDesktopControl = "light-desktop-control"
	streamRoleLightDesktopData    = "light-desktop-data"
	streamDesktopStart            = byte(70)
	streamDesktopManagedStart     = byte(71)
	streamDesktopCredentials      = byte(72)
	streamDesktopClaim            = byte(73)
)

// PreparedDesktopStream keeps a credential lease on the same authenticated
// connection that will carry RDP. Claim is single-use and must precede any I/O.
type PreparedDesktopStream struct {
	*desktopByteStream
	conn            *fileStreamConn
	mu              sync.Mutex
	claimed, closed bool
}

func (p *PreparedDesktopStream) Claim() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.claimed || p.closed {
		return ErrAuthentication
	}
	p.claimed = true
	if err := p.conn.write(streamDesktopClaim, nil); err != nil {
		return err
	}
	p.desktopByteStream = newDesktopByteStream(p.conn)
	return nil
}

func (p *PreparedDesktopStream) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.conn.close()
	return nil
}

func (p *PreparedDesktopStream) Read(data []byte) (int, error) {
	p.mu.Lock()
	stream, closed := p.desktopByteStream, p.closed
	p.mu.Unlock()
	if stream == nil || closed {
		return 0, io.ErrClosedPipe
	}
	return stream.Read(data)
}

func (p *PreparedDesktopStream) Write(data []byte) (int, error) {
	p.mu.Lock()
	stream, closed := p.desktopByteStream, p.closed
	p.mu.Unlock()
	if stream == nil || closed {
		return 0, io.ErrClosedPipe
	}
	return stream.Write(data)
}

func (s *Service) ManagedDesktopAvailable(node string) bool {
	record, err := s.light.Host(node)
	return err == nil && lightHostIsWindows(record) && s.desktopAllowed(node) &&
		lightPlatformAllows(record, "desktop-managed", s.now().UTC()) && s.fileStreamHub.desktopAvailable(node)
}

// DesktopCredentialIdentity binds saved Windows logins to the enrolled Noise
// identity, not a mutable name/address or a client-supplied fingerprint.
func (s *Service) DesktopCredentialIdentity(node string) (string, error) {
	record, err := s.light.Host(node)
	if err != nil {
		return "", err
	}
	key, err := s.light.ReadTerminalPublicKey(record)
	if err != nil || len(key) != 32 {
		return "", ErrIdentityMismatch
	}
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:]), nil
}

func validDesktopNonce(nonce string) bool {
	value, err := hex.DecodeString(nonce)
	return err == nil && len(value) == 32
}

func (s *Service) OpenDesktopStream(ctx context.Context, node, nonce string) (io.ReadWriteCloser, error) {
	conn, err := s.openDesktopConnection(ctx, node, nonce, false)
	if err != nil {
		return nil, err
	}
	return newDesktopByteStream(conn), nil
}

func (s *Service) PrepareManagedDesktop(ctx context.Context, node, nonce string) (*PreparedDesktopStream, desktopcredentials.Credentials, error) {
	if !s.ManagedDesktopAvailable(node) {
		return nil, desktopcredentials.Credentials{}, ErrTerminalUnavailable
	}
	conn, err := s.openDesktopConnection(ctx, node, nonce, true)
	if err != nil {
		return nil, desktopcredentials.Credentials{}, err
	}
	wait, cancel := context.WithTimeout(ctx, streamHandshakeTimeout)
	defer cancel()
	kind, data, err := conn.readContext(wait)
	defer clear(data)
	var response struct {
		Nonce       string                         `json:"nonce"`
		Credentials desktopcredentials.Credentials `json:"credentials"`
	}
	if err != nil || kind != streamDesktopCredentials || len(data) > 4096 || json.Unmarshal(data, &response) != nil ||
		response.Nonce != nonce || response.Credentials.Validate() != nil {
		conn.close()
		return nil, desktopcredentials.Credentials{}, ErrAuthentication
	}
	return &PreparedDesktopStream{conn: conn}, response.Credentials, nil
}

func (s *Service) openDesktopConnection(ctx context.Context, node, nonce string, managed bool) (*fileStreamConn, error) {
	record, err := s.light.Host(node)
	if err != nil || !s.desktopAllowed(node) || !lightHostIsWindows(record) || !lightPlatformAllows(record, "desktop", s.now().UTC()) || !validDesktopNonce(nonce) {
		return nil, ErrTerminalUnavailable
	}
	h := s.fileStreamHub
	requestID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	control := h.desktopControls[node]
	if control == nil || control.conn.ctx.Err() != nil || !s.desktopAllowed(node) {
		h.mu.Unlock()
		return nil, ErrTerminalUnavailable
	}
	pending := &fileStreamPending{streamRoleLightDesktopData, node, control, make(chan *fileStreamConn, 1), time.Now().Add(streamHandshakeTimeout)}
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
		return nil, err
	}
	wait, cancel := context.WithTimeout(ctx, streamHandshakeTimeout)
	defer cancel()
	select {
	case conn := <-pending.ready:
		if conn == nil {
			return nil, ErrRateLimited
		}
		kind := streamDesktopStart
		if managed {
			kind = streamDesktopManagedStart
		}
		if err := conn.write(kind, []byte(nonce)); err != nil {
			conn.close()
			return nil, err
		}
		stopParent := context.AfterFunc(ctx, conn.close)
		context.AfterFunc(conn.ctx, func() { stopParent() })
		return conn, nil
	case <-control.conn.ctx.Done():
		return nil, ErrTerminalUnavailable
	case <-wait.Done():
		return nil, wait.Err()
	}
}

func (h *fileStreamHub) desktopAvailable(node string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.desktopControls[node]
	return c != nil && c.conn.ctx.Err() == nil
}

// Desktop connections never fall back to the terminal/file record interpreters.
// A stopped control connection also closes its desktop sessions and credentials.
func (c *TerminalRelayClient) RunDesktopStream(ctx context.Context, origin, node, target string, key noise.DHKey, peer []byte,
	handler func(context.Context, io.ReadWriteCloser, string) error, connected func()) error {
	return c.RunManagedDesktopStream(ctx, origin, node, target, key, peer, handler, nil, connected)
}

func (c *TerminalRelayClient) RunManagedDesktopStream(ctx context.Context, origin, node, target string, key noise.DHKey, peer []byte,
	handler func(context.Context, io.ReadWriteCloser, string) error,
	prepare func(context.Context) (desktopcredentials.Credentials, func() error, error), connected func()) error {
	if c == nil || c.client == nil || handler == nil {
		return ErrAuthentication
	}
	if normalized, err := validateLightOrigin(origin); err != nil || normalized != origin {
		return ErrInvalidOrigin
	}
	control, err := dialFileStream(ctx, c.client, origin, node, target, key, peer, time.Now(), fileStreamHello{Role: streamRoleLightDesktopControl})
	if err != nil {
		return err
	}
	var workers sync.WaitGroup
	defer func() { control.close(); workers.Wait() }()
	kind, data, err := control.read()
	if err != nil || kind != streamOpen || !validID(string(data)) {
		return ErrAuthentication
	}
	generation := string(data)
	if connected != nil {
		connected()
	}
	gate := make(chan struct{}, 1)
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
				if err := control.write(streamReject, data); err != nil {
					return err
				}
				continue
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-gate }()
				conn, err := dialFileStream(control.ctx, c.client, origin, node, target, key, peer, time.Now(),
					fileStreamHello{Role: streamRoleLightDesktopData, RequestID: requestID, Generation: generation})
				if err != nil {
					return
				}
				defer conn.close()
				kind, data, err := conn.readControl()
				if err != nil || (kind != streamDesktopStart && kind != streamDesktopManagedStart) || !validDesktopNonce(string(data)) {
					return
				}
				nonce := string(data)
				if kind == streamDesktopManagedStart {
					if prepare == nil {
						return
					}
					credentials, cleanup, err := prepare(conn.ctx)
					if cleanup != nil {
						defer func() {
							if cleanup() != nil {
								control.close()
							}
						}()
					}
					if err != nil || cleanup == nil || credentials.Validate() != nil {
						return
					}
					response, err := json.Marshal(struct {
						Nonce       string                         `json:"nonce"`
						Credentials desktopcredentials.Credentials `json:"credentials"`
					}{nonce, credentials})
					credentials.Password = ""
					if err != nil {
						return
					}
					err = conn.write(streamDesktopCredentials, response)
					clear(response)
					if err != nil {
						return
					}
					claimCtx, stop := context.WithTimeout(conn.ctx, 45*time.Second)
					kind, data, err = conn.readContext(claimCtx)
					stop()
					if err != nil || kind != streamDesktopClaim || len(data) != 0 {
						return
					}
				}
				stream := newDesktopByteStream(conn)
				defer stream.Close()
				_ = handler(conn.ctx, stream, nonce)
			}()
		default:
			return ErrAuthentication
		}
	}
}

type desktopByteStream struct {
	conn    *fileStreamConn
	pending []byte
	idle    *time.Timer
	once    sync.Once
}

func newDesktopByteStream(conn *fileStreamConn) *desktopByteStream {
	stream := &desktopByteStream{conn: conn, idle: time.AfterFunc(30*time.Minute, conn.close)}
	lifetime := time.AfterFunc(8*time.Hour, conn.close)
	context.AfterFunc(conn.ctx, func() { lifetime.Stop() })
	context.AfterFunc(conn.ctx, func() { stream.idle.Stop() })
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-conn.ctx.Done():
				return
			case <-ticker.C:
				if conn.write(streamPing, nil) != nil {
					return
				}
			}
		}
	}()
	return stream
}

func (s *desktopByteStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(s.pending) == 0 {
		kind, data, err := s.conn.read()
		if err != nil {
			return 0, err
		}
		switch kind {
		case streamData:
			if len(data) == 0 {
				return 0, ErrAuthentication
			}
			s.pending = data
		case streamPing:
			if len(data) != 0 {
				return 0, ErrAuthentication
			}
			if err := s.conn.write(streamPong, nil); err != nil {
				return 0, err
			}
		case streamPong:
			if len(data) != 0 {
				return 0, ErrAuthentication
			}
		case streamEnd:
			if len(data) != 0 {
				return 0, ErrAuthentication
			}
			return 0, io.EOF
		default:
			return 0, ErrAuthentication
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	s.idle.Reset(30 * time.Minute)
	return n, nil
}

func (s *desktopByteStream) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), fileStreamChunkBytes)
		if err := s.conn.write(streamData, p[:n]); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
		s.idle.Reset(30 * time.Minute)
	}
	return written, nil
}

func (s *desktopByteStream) Close() error {
	s.once.Do(func() { s.idle.Stop(); s.conn.close() })
	return nil
}
