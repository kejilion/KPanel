package desktopbridge

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Errors deliberately contain stable codes only: credentials, certificate
// contents, the authentication PDU and OS error text never enter logs.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrUnsupported         Error = "desktop_platform_unsupported"
	ErrDisabled            Error = "desktop_rdp_disabled"
	ErrServiceStopped      Error = "desktop_rdp_service_stopped"
	ErrConfiguration       Error = "desktop_rdp_configuration_unavailable"
	ErrCertificate         Error = "desktop_rdp_certificate_unavailable"
	ErrCertificateMismatch Error = "desktop_rdp_certificate_mismatch"
	ErrConnection          Error = "desktop_rdp_connection_failed"
	ErrTLS                 Error = "desktop_rdp_tls_failed"
	ErrNLARequired         Error = "desktop_nla_required"
	ErrRequest             Error = "desktop_invalid_request"
	ErrDestination         Error = "desktop_invalid_destination"
	ErrAuthentication      Error = "desktop_session_mismatch"
	ErrNegotiation         Error = "desktop_rdp_negotiation_failed"
	ErrBusy                Error = "desktop_session_busy"
	ErrHandshakeTimeout    Error = "desktop_handshake_timeout"
	ErrIdle                Error = "desktop_session_idle"
	ErrExpired             Error = "desktop_session_expired"
	ErrDisconnected        Error = "desktop_disconnected"
)

func Code(err error) string {
	if err == nil {
		return ""
	}
	var code Error
	if errors.As(err, &code) {
		return string(code)
	}
	return string(ErrDisconnected)
}

type endpoint struct {
	port         uint16
	certificates [][]byte
}
type limits struct{ handshake, idle, duration time.Duration }

var defaults = limits{15 * time.Second, 30 * time.Minute, 8 * time.Hour}
var sessionSlot = make(chan struct{}, 1)

// Probe only queries Windows configuration, service state and public
// certificates. It neither enables RDP nor changes the listener or firewall.
func Probe() error { _, err := localEndpoint(); return err }

// Serve owns and closes stream. The caller has already authenticated the Noise
// desktop role and supplied its bound nonce. Only one session is admitted per
// process, independently of central admission. Close must unblock stream I/O.
func Serve(ctx context.Context, stream io.ReadWriteCloser, nonce string) error {
	select {
	case sessionSlot <- struct{}{}:
		defer func() { <-sessionSlot }()
	default:
		_ = stream.Close()
		return ErrBusy
	}
	return serve(ctx, stream, nonce, localEndpoint, defaults)
}

func serve(parent context.Context, stream io.ReadWriteCloser, nonce string, resolve func() (endpoint, error), budget limits) error {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	defer stream.Close()
	stopStream := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stopStream()
	expiry := time.AfterFunc(budget.duration, func() { cancel(ErrExpired) })
	defer expiry.Stop()
	handshakeTimer := time.AfterFunc(budget.handshake, func() { cancel(ErrHandshakeTimeout) })
	defer handshakeTimer.Stop()
	tcp, err := handshake(ctx, stream, nonce, resolve, budget.handshake)
	if err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		// This write is still bounded by the handshake timer if the peer refuses
		// to read the failure. It never contains authentication or OS details.
		status := 502
		if errors.Is(err, ErrRequest) || errors.Is(err, ErrDestination) || errors.Is(err, ErrNegotiation) {
			status = 400
		}
		if errors.Is(err, ErrAuthentication) {
			status = 401
		}
		if errors.Is(err, ErrDisabled) || errors.Is(err, ErrServiceStopped) || errors.Is(err, ErrConfiguration) {
			status = 503
		}
		_ = writeFull(stream, encodeFailure(status))
		return err
	}
	defer tcp.Close()
	handshakeTimer.Stop()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return pipe(ctx, cancel, stream, tcp, budget.idle)
}

func handshake(ctx context.Context, stream io.ReadWriter, nonce string, resolve func() (endpoint, error), timeout time.Duration) (net.Conn, error) {
	data, err := readDER(stream)
	if err != nil {
		return nil, ErrRequest
	}
	req, err := decodeRequest(data, nonce)
	if err != nil {
		return nil, err
	}
	local, err := resolve()
	if err != nil {
		return nil, err
	}
	if local.port == 0 || len(local.certificates) == 0 {
		return nil, ErrConfiguration
	}
	// Never resolve or dial request.destination. Even test endpoints use loopback.
	raw, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(local.port))))
	if err != nil {
		return nil, ErrConnection
	}
	ok := false
	defer func() {
		if !ok {
			_ = raw.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	_ = raw.SetDeadline(time.Now().Add(timeout))
	if err := writeFull(raw, req.x224); err != nil {
		return nil, ErrConnection
	}
	x224, err := readX224Response(raw, req.protocols)
	if err != nil {
		if errors.Is(err, ErrNLARequired) {
			return nil, err
		}
		return nil, ErrNegotiation
	}
	secure := tls.Client(raw, &tls.Config{
		MinVersion: tls.VersionTLS12,
		// The RDP listener commonly uses a self-signed machine certificate and
		// has no loopback SAN. Replace public-PKI/hostname verification with an
		// exact, independently loaded LocalMachine certificate pin. TLS still
		// verifies possession of the corresponding private key.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 || len(state.PeerCertificates) > 8 {
				return ErrCertificateMismatch
			}
			return verifyCertificate(state.PeerCertificates[0], local.certificates, time.Now())
		},
	})
	if err := secure.HandshakeContext(ctx); err != nil {
		if errors.Is(err, ErrCertificateMismatch) {
			return nil, ErrCertificateMismatch
		}
		return nil, ErrTLS
	}
	chain := make([][]byte, 0, len(secure.ConnectionState().PeerCertificates))
	for _, cert := range secure.ConnectionState().PeerCertificates {
		if len(cert.Raw) > 16<<10 {
			return nil, ErrCertificate
		}
		chain = append(chain, cert.Raw)
	}
	response, err := encodeResponse(x224, chain)
	if err != nil {
		return nil, err
	}
	if err := writeFull(stream, response); err != nil {
		return nil, ErrDisconnected
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		return nil, ErrConnection
	}
	ok = true
	return secure, nil
}

func verifyCertificate(cert *x509.Certificate, pins [][]byte, now time.Time) error {
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return ErrCertificateMismatch
	}
	usageOK := len(cert.ExtKeyUsage) == 0 && len(cert.UnknownExtKeyUsage) == 0
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
			usageOK = true
		}
	}
	for _, usage := range cert.UnknownExtKeyUsage {
		if usage.String() == "1.3.6.1.4.1.311.54.1.2" {
			usageOK = true
		}
	}
	if !usageOK {
		return ErrCertificateMismatch
	}
	for _, pin := range pins {
		if bytes.Equal(cert.Raw, pin) {
			return nil
		}
	}
	return ErrCertificateMismatch
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if n < 0 || n > len(p) {
			return io.ErrShortWrite
		}
		p = p[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func pipe(ctx context.Context, cancel context.CancelCauseFunc, stream io.ReadWriteCloser, tcp net.Conn, idle time.Duration) error {
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			// Closing the raw socket avoids waiting for a TLS close_notify write to
			// a stalled peer during cancellation. A disconnect preserves the RDP session.
			if secure, ok := tcp.(*tls.Conn); ok {
				_ = secure.NetConn().Close()
			} else {
				_ = tcp.Close()
			}
			_ = stream.Close()
		})
	}
	defer closeBoth()
	stop := context.AfterFunc(ctx, closeBoth)
	defer stop()
	done := make(chan error, 2)
	copyBytes := func(dst io.Writer, src io.Reader) {
		buffer := make([]byte, 32<<10)
		for {
			n, err := src.Read(buffer)
			if n > 0 {
				last.Store(time.Now().UnixNano())
				if writeErr := writeFull(dst, buffer[:n]); writeErr != nil {
					done <- writeErr
					return
				}
			}
			if err != nil {
				done <- err
				return
			}
			if n == 0 {
				done <- io.ErrNoProgress
				return
			}
		}
	}
	go copyBytes(tcp, stream)
	go copyBytes(stream, tcp)
	ticker := time.NewTicker(min(idle/2, time.Minute))
	defer ticker.Stop()
	var result error
	for {
		select {
		case result = <-done:
			closeBoth()
			<-done // Closing both transports releases the other bounded copy.
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			if errors.Is(result, io.EOF) {
				return nil
			}
			return ErrDisconnected
		case <-ctx.Done():
			closeBoth()
			<-done
			<-done
			return context.Cause(ctx)
		case <-ticker.C:
			if time.Since(time.Unix(0, last.Load())) >= idle {
				cancel(ErrIdle)
			}
		}
	}
}
