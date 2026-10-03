package desktopbridge

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"io"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func testCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// fakeRDP exchanges actual TPKT bytes and performs a real TLS handshake. The
// bytes after TLS stand in for browser CredSSP/RDP, not a fake connection status.
func fakeRDP(t *testing.T, cert tls.Certificate, protocol uint32, minTLS, maxTLS uint16, after func(net.Conn) error) (endpoint, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		p := make([]byte, len(testX224))
		if _, err = io.ReadFull(conn, p); err != nil {
			done <- err
			return
		}
		if !bytes.Equal(p, testX224) {
			done <- errors.New("client X224 changed")
			return
		}
		for _, b := range confirmation(protocol) {
			if err = writeFull(conn, []byte{b}); err != nil {
				done <- err
				return
			}
		}
		if protocol != 2 && protocol != 8 {
			done <- nil
			return
		}
		secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: minTLS, MaxVersion: maxTLS})
		if err = secure.Handshake(); err != nil {
			done <- err
			return
		}
		if after != nil {
			err = after(secure)
		}
		done <- err
	}()
	return endpoint{uint16(listener.Addr().(*net.TCPAddr).Port), cert.Certificate}, done
}

func runBridge(t *testing.T, ep endpoint, budget limits) (net.Conn, context.CancelFunc, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, testNonce, func() (endpoint, error) { return ep, nil }, budget) }()
	t.Cleanup(func() { cancel(); client.Close() })
	_ = client.SetDeadline(time.Now().Add(4 * time.Second))
	return client, cancel, done
}

func finishHandshake(t *testing.T, client net.Conn) [][]byte {
	t.Helper()
	if err := writeFull(client, requestDER(t, "localhost", testNonce, testX224)); err != nil {
		t.Fatal(err)
	}
	data, err := readDER(client)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Version      int      `asn1:"explicit,tag:0"`
		X224         []byte   `asn1:"explicit,tag:6"`
		Certificates [][]byte `asn1:"explicit,tag:7"`
		Address      string   `asn1:"utf8,explicit,tag:9"`
	}
	rest, err := asn1.Unmarshal(data, &response)
	if err != nil || len(rest) != 0 || response.Version != 3390 || response.Address != "127.0.0.1" || !bytes.Equal(response.X224, confirmation(2)) {
		t.Fatalf("response %x: %v", data, err)
	}
	return response.Certificates
}

func waitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("bridge or server did not terminate")
		return nil
	}
}

func TestBridgeHandshakeAndBidirectionalBytes(t *testing.T) {
	cert := testCertificate(t)
	clientData := bytes.Repeat([]byte("browser-credssp-rdp"), 6000)
	serverData := bytes.Repeat([]byte("server-desktop"), 6000)
	ep, rdpDone := fakeRDP(t, cert, 2, tls.VersionTLS12, tls.VersionTLS13, func(conn net.Conn) error {
		got := make([]byte, len(clientData))
		if _, err := io.ReadFull(conn, got); err != nil {
			return err
		}
		if !bytes.Equal(got, clientData) {
			return errors.New("client bytes changed")
		}
		return writeFull(conn, serverData)
	})
	client, _, done := runBridge(t, ep, limits{time.Second, time.Second, 3 * time.Second})
	chain := finishHandshake(t, client)
	if len(chain) != 1 || !bytes.Equal(chain[0], cert.Certificate[0]) {
		t.Fatal("not the actual peer DER certificate")
	}
	if err := writeFull(client, clientData); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(serverData))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, serverData) {
		t.Fatal("server bytes changed")
	}
	if err := waitResult(t, rdpDone); err != nil {
		t.Fatal(err)
	}
	if err := waitResult(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeRejectsCertificateAndProtocolDowngrades(t *testing.T) {
	for _, scenario := range []string{"wrong_certificate", "ssl_only", "tls11"} {
		t.Run(scenario, func(t *testing.T) {
			cert := testCertificate(t)
			protocol := uint32(2)
			minTLS, maxTLS := uint16(tls.VersionTLS12), uint16(tls.VersionTLS13)
			want := ErrCertificateMismatch
			if scenario == "ssl_only" {
				protocol = 1
				want = ErrNLARequired
			}
			if scenario == "tls11" {
				minTLS = tls.VersionTLS10
				maxTLS = tls.VersionTLS11
				want = ErrTLS
			}
			ep, rdpDone := fakeRDP(t, cert, protocol, minTLS, maxTLS, nil)
			if scenario == "wrong_certificate" {
				ep.certificates = testCertificate(t).Certificate
			}
			client, _, done := runBridge(t, ep, limits{time.Second, time.Second, 3 * time.Second})
			if err := writeFull(client, requestDER(t, "localhost", testNonce, testX224)); err != nil {
				t.Fatal(err)
			}
			data, err := readDER(client)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, encodeFailure(502)) {
				t.Fatalf("unexpected error frame %x", data)
			}
			if err := waitResult(t, done); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			_ = waitResult(t, rdpDone)
		})
	}
}

func TestBridgeCancellationIdleAndHardExpiry(t *testing.T) {
	for _, scenario := range []string{"cancel", "idle", "expiry", "peer_disconnect"} {
		t.Run(scenario, func(t *testing.T) {
			cert := testCertificate(t)
			ep, rdpDone := fakeRDP(t, cert, 2, tls.VersionTLS12, tls.VersionTLS13, func(conn net.Conn) error { var p [1]byte; _, err := conn.Read(p[:]); return err })
			budget := limits{time.Second, time.Second, 3 * time.Second}
			want := error(context.Canceled)
			if scenario == "idle" {
				budget.idle = 40 * time.Millisecond
				want = ErrIdle
			}
			if scenario == "expiry" {
				budget.duration = 80 * time.Millisecond
				want = ErrExpired
			}
			client, cancel, done := runBridge(t, ep, budget)
			finishHandshake(t, client)
			if scenario == "cancel" {
				cancel()
			}
			if scenario == "peer_disconnect" {
				client.Close()
				want = nil
			}
			err := waitResult(t, done)
			if want != nil && !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			_ = waitResult(t, rdpDone)
		})
	}
}

func TestPartialHandshakeAndUnconsumedErrorAreBounded(t *testing.T) {
	for _, partial := range []bool{true, false} {
		client, server := net.Pipe()
		done := make(chan error, 1)
		go func() {
			done <- serve(context.Background(), server, testNonce, func() (endpoint, error) { t.Error("invalid request must not resolve or dial"); return endpoint{}, nil }, limits{40 * time.Millisecond, time.Second, time.Second})
		}()
		if partial {
			_, _ = client.Write([]byte{0x30})
		} else {
			_, _ = client.Write([]byte{0x30, 0})
		}
		err := waitResult(t, done)
		client.Close()
		if partial && !errors.Is(err, ErrHandshakeTimeout) {
			t.Fatal(err)
		}
	}
}

func TestCertificateValidityAndUsageAreChecked(t *testing.T) {
	der := testCertificate(t).Certificate[0]
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	for _, when := range []time.Time{cert.NotBefore.Add(-time.Second), cert.NotAfter.Add(time.Second)} {
		if verifyCertificate(cert, [][]byte{der}, when) == nil {
			t.Fatal("expired certificate accepted")
		}
	}
	cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if verifyCertificate(cert, [][]byte{der}, time.Now()) == nil {
		t.Fatal("client-only certificate accepted")
	}
	cert.ExtKeyUsage = nil
	cert.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 311, 54, 1, 2}}
	if err := verifyCertificate(cert, [][]byte{der}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestServeNodeConcurrencyLimit(t *testing.T) {
	first, firstServer := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, firstServer, testNonce) }()
	// An uncompleted DER header proves the first call has acquired admission.
	if _, err := first.Write([]byte{0x30}); err != nil {
		t.Fatal(err)
	}
	second, secondServer := net.Pipe()
	if err := Serve(context.Background(), secondServer, testNonce); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	second.Close()
	cancel()
	first.Close()
	_ = waitResult(t, done)
}

type observedStream struct {
	net.Conn
	writes    atomic.Int32
	dataWrite chan struct{}
}

func (s *observedStream) Write(p []byte) (int, error) {
	if s.writes.Add(1) == 2 {
		close(s.dataWrite)
	}
	return s.Conn.Write(p)
}

func TestBlockedOutputCancellationClosesBothDirections(t *testing.T) {
	cert := testCertificate(t)
	ep, rdpDone := fakeRDP(t, cert, 2, tls.VersionTLS12, tls.VersionTLS13, func(conn net.Conn) error {
		if err := writeFull(conn, bytes.Repeat([]byte{42}, 64<<10)); err != nil {
			return err
		}
		var b [1]byte
		_, err := conn.Read(b[:])
		return err
	})
	client, server := net.Pipe()
	stream := &observedStream{Conn: server, dataWrite: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, stream, testNonce, func() (endpoint, error) { return ep, nil }, limits{time.Second, time.Second, 3 * time.Second})
	}()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	finishHandshake(t, client)
	select {
	case <-stream.dataWrite:
	case <-time.After(time.Second):
		t.Fatal("RDP data did not reach blocked browser output")
	}
	// The client stops reading while the server is blocked writing to it.
	started := time.Now()
	cancel()
	if err := waitResult(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("cancellation waited for transport write timeout")
	}
	_ = waitResult(t, rdpDone)
}

func TestCancellationDuringLocalX224Read(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan struct{})
	rdpDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			rdpDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		p := make([]byte, len(testX224))
		if _, err = io.ReadFull(conn, p); err != nil {
			rdpDone <- err
			return
		}
		close(received)
		var b [1]byte
		_, err = conn.Read(b[:])
		rdpDone <- err
	}()
	ep := endpoint{uint16(listener.Addr().(*net.TCPAddr).Port), testCertificate(t).Certificate}
	client, cancel, done := runBridge(t, ep, limits{time.Second, time.Second, 3 * time.Second})
	if err := writeFull(client, requestDER(t, "localhost", testNonce, testX224)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("no X224 request")
	}
	cancel()
	if err := waitResult(t, done); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := waitResult(t, rdpDone); !errors.Is(err, io.EOF) {
		t.Fatalf("local RDP socket remained open: %v", err)
	}
}
