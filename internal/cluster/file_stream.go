package cluster

// The file stream carries the existing, restricted file HTTP contract. A fresh
// Noise IK handshake authenticates each socket; ordered AEAD records then carry
// bytes directly, without JSON/base64 data chunks or per-chunk acknowledgements.
import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/flynn/noise"
)

const (
	FileStreamV2Path       = "/api/v2/federation/files/stream"
	fileStreamProtocol     = "kpanel-file-stream-v1"
	streamRequest          = byte(10)
	streamResponse         = byte(11)
	streamData             = byte(12)
	streamEnd              = byte(13)
	streamOpen             = byte(15)
	streamPing             = byte(16)
	streamPong             = byte(17)
	streamReject           = byte(18)
	streamMaxUpload        = int64(512 << 20)
	streamHandshakeTimeout = 8 * time.Second
)

var ErrFileStreamUnsupported = errors.New("file stream protocol is unsupported")

type fileStreamHello struct {
	Role       string `json:"role"`
	RequestID  string `json:"requestId,omitempty"`
	Generation string `json:"generation,omitempty"`
}

type fileStreamRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   string            `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Length  int64             `json:"length"`
}

type fileStreamResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
}

type fileStreamConn struct {
	ws             *websocket.Conn
	ctx            context.Context
	stop           context.CancelFunc
	tx, rx         *noise.CipherState
	writeMu        sync.Mutex
	idle           *time.Timer
	completed      atomic.Bool
	responseStatus atomic.Int32
}

func newFileStreamConn(parent context.Context, ws *websocket.Conn, tx, rx *noise.CipherState) *fileStreamConn {
	ctx, stop := context.WithCancel(parent)
	c := &fileStreamConn{ws: ws, ctx: ctx, stop: stop, tx: tx, rx: rx}
	c.idle = time.AfterFunc(fileStreamIdleTimeout, stop)
	ws.SetReadLimit(noise.MaxMsgLen)
	context.AfterFunc(ctx, func() { c.idle.Stop(); _ = ws.CloseNow() })
	return c
}

func (c *fileStreamConn) close() { c.stop(); _ = c.ws.CloseNow() }

func (c *fileStreamConn) limitFileLifetime() {
	timer := time.AfterFunc(2*time.Hour, c.stop)
	context.AfterFunc(c.ctx, func() { timer.Stop() })
}

func (c *fileStreamConn) write(kind byte, data []byte) error {
	if len(data) > fileStreamChunkBytes {
		return ErrAuthentication
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	plain := make([]byte, len(data)+1)
	plain[0] = kind
	copy(plain[1:], data)
	sealed, err := c.tx.Encrypt(nil, nil, plain)
	if err != nil {
		c.close()
		return ErrAuthentication
	}
	ctx, cancel := context.WithTimeout(c.ctx, fileStreamIdleTimeout)
	defer cancel()
	err = c.ws.Write(ctx, websocket.MessageBinary, sealed)
	if err != nil {
		c.close()
	} else {
		c.idle.Reset(fileStreamIdleTimeout)
	}
	return err
}

func (c *fileStreamConn) writeJSON(kind byte, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.write(kind, payload)
}

// Exactly one goroutine owns rx. WebSocket close, TCP EOF and missing end are
// errors, never successful file EOF. The explicit end also authenticates size.
func (c *fileStreamConn) read() (byte, []byte, error) {
	return c.readContext(c.ctx)
}

func (c *fileStreamConn) readControl() (byte, []byte, error) {
	ctx, cancel := context.WithTimeout(c.ctx, fileStreamIdleTimeout)
	defer cancel()
	return c.readContext(ctx)
}

func (c *fileStreamConn) readContext(ctx context.Context) (byte, []byte, error) {
	kind, sealed, err := c.ws.Read(ctx)
	if err != nil {
		c.close()
		return 0, nil, errors.Join(io.ErrUnexpectedEOF, err)
	}
	if kind != websocket.MessageBinary {
		c.close()
		return 0, nil, ErrAuthentication
	}
	plain, err := c.rx.Decrypt(nil, nil, sealed)
	if err != nil || len(plain) < 1 || len(plain) > fileStreamChunkBytes+1 {
		c.close()
		return 0, nil, ErrAuthentication
	}
	c.idle.Reset(fileStreamIdleTimeout)
	return plain[0], plain[1:], nil
}

func streamSize(size int64) []byte {
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, uint64(size))
	return value
}

func validStreamEnd(data []byte, size int64) bool {
	return len(data) == 8 && binary.BigEndian.Uint64(data) == uint64(size)
}

func streamWriteBody(c *fileStreamConn, body io.Reader, length int64) error {
	var total int64
	buffer := make([]byte, fileStreamChunkBytes)
	if body == nil {
		body = http.NoBody
	}
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > streamMaxUpload || (length >= 0 && total > length) {
				return ErrAuthentication
			}
			if writeErr := c.write(streamData, buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err == io.EOF {
			if length >= 0 && total != length {
				return io.ErrUnexpectedEOF
			}
			return c.write(streamEnd, streamSize(total))
		}
		if err != nil {
			return err
		}
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
	}
}

// openStreamRequest takes ownership of c and of a closable upload body. The
// upload and response run independently, allowing an early rejection to cancel
// a blocked upload. No body bytes are consumed before authentication succeeds.
func openStreamRequest(c *fileStreamConn, input LightFileRequest) (*http.Response, error) {
	c.limitFileLifetime()
	if !validFileRelayRequest(input) {
		c.close()
		return nil, ErrAuthentication
	}
	if input.Body == nil || input.Body == http.NoBody {
		input.BodyLength = 0
	}
	if body, ok := input.Body.(io.Closer); ok {
		context.AfterFunc(c.ctx, func() { _ = body.Close() })
	}
	meta := fileStreamRequest{input.Method, input.Path, input.RawQuery, input.Headers, input.BodyLength}
	if err := c.writeJSON(streamRequest, meta); err != nil {
		c.close()
		return nil, err
	}
	go func() {
		if err := streamWriteBody(c, input.Body, input.BodyLength); err != nil {
			c.close()
		}
	}()
	kind, payload, err := c.read()
	var response fileStreamResponse
	if err != nil || kind != streamResponse || decodeV2Payload(payload, &response) != nil ||
		response.Status < 200 || response.Status > 599 || !validFileRelayHeaders(response.Headers) {
		c.close()
		if err != nil {
			return nil, err
		}
		return nil, ErrAuthentication
	}
	c.responseStatus.Store(int32(response.Status))
	return &http.Response{StatusCode: response.Status, Header: fileRelayHTTPHeaders(response.Headers),
		ContentLength: -1, Body: &fileStreamBody{conn: c}}, nil
}

type fileStreamBody struct {
	conn   *fileStreamConn
	buffer []byte
	total  int64
	done   bool
}

func (b *fileStreamBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.done {
		return 0, io.EOF
	}
	if len(b.buffer) == 0 {
		kind, data, err := b.conn.read()
		if err != nil {
			return 0, err
		}
		switch kind {
		case streamData:
			if len(data) == 0 {
				b.conn.close()
				return 0, ErrAuthentication
			}
			b.buffer = data
			b.total += int64(len(data))
		case streamEnd:
			if !validStreamEnd(data, b.total) {
				b.conn.close()
				return 0, ErrAuthentication
			}
			b.done = true
			b.conn.completed.Store(true)
			b.conn.close()
			return 0, io.EOF
		default:
			b.conn.close()
			return 0, ErrAuthentication
		}
	}
	n := copy(p, b.buffer)
	b.buffer = b.buffer[n:]
	return n, nil
}

func (b *fileStreamBody) Close() error { b.conn.close(); return nil }

// serveStreamRequest is shared by full Panels and the lightweight file broker.
// The pipe provides backpressure, and only an authenticated end closes it with
// EOF. A disconnected or truncated upload therefore cannot publish a partial file.
func serveStreamRequest(c *fileStreamConn, handler http.Handler, limits *fileStreamLimits, peer string, readOnly bool) {
	c.limitFileLifetime()
	defer c.close()
	kind, payload, err := c.read()
	var meta fileStreamRequest
	if err != nil || kind != streamRequest || decodeV2Payload(payload, &meta) != nil {
		return
	}
	input := LightFileRequest{Method: meta.Method, Path: meta.Path, RawQuery: meta.Query, Headers: meta.Headers, BodyLength: meta.Length}
	if !validFileRelayRequest(input) || (readOnly && (meta.Method != http.MethodGet || meta.Path != "/v1/files/transfer/export" || meta.Length != 0)) {
		return
	}
	release, ok := limits.acquire(peer, input)
	if !ok {
		return
	}
	defer release()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.CloseWithError(io.ErrUnexpectedEOF)
	stopPipe := context.AfterFunc(c.ctx, func() { _ = reader.CloseWithError(c.ctx.Err()); _ = writer.CloseWithError(c.ctx.Err()) })
	defer stopPipe()
	request, err := http.NewRequestWithContext(c.ctx, meta.Method, "http://kpanel-file-stream"+meta.Path, reader)
	if err != nil {
		return
	}
	request.URL.RawQuery = meta.Query
	request.Header = relayRequestHeaders(meta.Headers)
	// Even a declared length must reach authenticated EOF before the local Agent
	// accepts the upload. Passing ContentLength would let transports truncate the
	// body to that many bytes without checking the final encrypted record.
	request.ContentLength = -1
	uploadDone := make(chan struct{})
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		defer c.close()
		var total int64
		for {
			kind, data, readErr := c.read()
			if readErr != nil {
				_ = writer.CloseWithError(readErr)
				return
			}
			if kind == streamEnd {
				if !validStreamEnd(data, total) || (meta.Length >= 0 && total != meta.Length) {
					_ = writer.CloseWithError(io.ErrUnexpectedEOF)
					return
				}
				_ = writer.Close()
				close(uploadDone)
				// Keep reading only to detect peer closure while the handler sends
				// its response. No further request records are valid on this socket.
				_, _, _ = c.read()
				return
			}
			if kind != streamData || len(data) == 0 {
				_ = writer.CloseWithError(ErrAuthentication)
				return
			}
			total += int64(len(data))
			if total > streamMaxUpload || (meta.Length >= 0 && total > meta.Length) {
				_ = writer.CloseWithError(ErrAuthentication)
				return
			}
			if _, err := writer.Write(data); err != nil {
				return
			}
		}
	}()
	if meta.Length == 0 {
		select {
		case <-uploadDone:
		case <-c.ctx.Done():
			return
		}
		request.Body = http.NoBody
		request.ContentLength = 0
	}
	w := &fileStreamResponseWriter{conn: c, header: make(http.Header)}
	// Handler panics must terminate the encrypted stream without emitting end.
	func() {
		defer func() {
			if recover() != nil {
				w.err = io.ErrUnexpectedEOF
			}
		}()
		handler.ServeHTTP(w, request)
	}()
	if w.err != nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	if meta.Path == "/v1/files/transfer/export" && w.status >= 200 && w.status < 300 && w.header.Get("X-KPanel-Transfer-Result") != "ok" {
		return
	}
	if w.err != nil || c.write(streamEnd, streamSize(w.total)) != nil {
		return
	}
	c.completed.Store(true)
	select {
	case <-peerDone:
	case <-c.ctx.Done():
	}
}

type fileStreamResponseWriter struct {
	conn   *fileStreamConn
	header http.Header
	wrote  bool
	status int
	total  int64
	err    error
}

func (w *fileStreamResponseWriter) Header() http.Header { return w.header }
func (w *fileStreamResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	if status < 200 || status > 599 {
		status = http.StatusInternalServerError
	}
	w.wrote = true
	w.status = status
	w.conn.responseStatus.Store(int32(status))
	w.err = w.conn.writeJSON(streamResponse, fileStreamResponse{status, relayResponseHeaders(w.header)})
}
func (w *fileStreamResponseWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	if w.err != nil {
		return 0, w.err
	}
	n := 0
	for len(p) > 0 {
		size := min(len(p), fileStreamChunkBytes)
		if w.err = w.conn.write(streamData, p[:size]); w.err != nil {
			return n, w.err
		}
		w.total += int64(size)
		n += size
		p = p[size:]
	}
	return n, nil
}
func (w *fileStreamResponseWriter) Flush() { w.WriteHeader(http.StatusOK) }

// Bulk transfers cannot consume the slots reserved for directory/metadata work.
// Limits apply to upgraded sockets, which http.Transport no longer counts.
type fileStreamLimits struct{ bulk, short *fileStreamLimiter }

func newFileStreamLimits() *fileStreamLimits {
	return &fileStreamLimits{newFileStreamLimiter(16, 4), newFileStreamLimiter(16, 4)}
}
func (l *fileStreamLimits) acquire(peer string, input LightFileRequest) (func(), bool) {
	switch input.Path {
	case "/v1/files/content", "/v1/files/upload", "/v1/files/archive", "/v1/files/transfer/export", "/v1/files/transfer/import":
		return l.bulk.acquire(peer)
	default:
		return l.short.acquire(peer)
	}
}
