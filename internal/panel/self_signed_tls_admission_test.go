package panel

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type selfSignedTLSSourceConn struct {
	*selfSignedTLSProbeConn
	address string
}

func TestSelfSignedTLSFastSameSourceBurstDoesNotSpendSlowBudget(t *testing.T) {
	inner := &selfSignedTLSQueueListener{conns: make(chan net.Conn, 40), done: make(chan struct{})}
	listener := newSelfSignedTLSListener(inner, func() *tls.Config { return nil })
	t.Cleanup(func() { _ = listener.Close() })
	clients := make([]net.Conn, 0, 40)
	for i := 0; i < 40; i++ {
		a, b := net.Pipe()
		clients = append(clients, b)
		t.Cleanup(func() { _ = b.Close() })
		server := &selfSignedTLSSourceConn{selfSignedTLSProbeConn: &selfSignedTLSProbeConn{Conn: a, started: make(chan struct{}, 1)}, address: "192.0.2.1:1234"}
		inner.conns <- server
		select {
		case <-server.started:
		case <-time.After(time.Second):
			t.Fatalf("fast connection %d rejected before its first byte", i)
		}
	}
	for _, client := range clients {
		go func(c net.Conn) { _, _ = c.Write([]byte("G")) }(client)
	}
	for range clients {
		result := make(chan net.Conn, 1)
		go func() { c, _ := listener.Accept(); result <- c }()
		select {
		case c := <-result:
			if c == nil {
				t.Fatal("fast connection rejected")
			}
			_ = c.Close()
		case <-time.After(time.Second):
			t.Fatal("fast connection did not reach HTTP")
		}
	}
	listener.classifierMu.Lock()
	defer listener.classifierMu.Unlock()
	if len(listener.sources) != 0 {
		t.Fatal("fast clients charged to slow-source budget")
	}
}

func (c *selfSignedTLSSourceConn) RemoteAddr() net.Addr {
	address, _ := net.ResolveTCPAddr("tcp", c.address)
	return address
}

func TestSelfSignedTLSListenerSourceFairnessAndClose(t *testing.T) {
	inner := &selfSignedTLSQueueListener{conns: make(chan net.Conn, 70), done: make(chan struct{})}
	listener := newSelfSignedTLSListener(inner, func() *tls.Config { return nil })
	t.Cleanup(func() { _ = listener.Close() })
	pending := func(address string) (net.Conn, *selfSignedTLSSourceConn) {
		a, b := net.Pipe()
		t.Cleanup(func() { _ = b.Close() })
		wrapped := &selfSignedTLSSourceConn{selfSignedTLSProbeConn: &selfSignedTLSProbeConn{Conn: a, started: make(chan struct{}, 1)}, address: address}
		inner.conns <- wrapped
		return b, wrapped
	}
	waitStarted := func(c *selfSignedTLSSourceConn) {
		t.Helper()
		select {
		case <-c.started:
		case <-time.After(time.Second):
			t.Fatal("classifier did not start")
		}
	}
	var silent []net.Conn
	for i := 0; i < 16; i++ {
		client, server := pending("192.0.2.1:1234")
		silent = append(silent, client)
		waitStarted(server)
	}
	deadline := time.Now().Add(time.Second)
	for {
		listener.classifierMu.Lock()
		count := listener.sources["192.0.2.1"]
		listener.classifierMu.Unlock()
		if count == 16 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("silent clients did not enter the slow-source budget")
		}
		time.Sleep(time.Millisecond)
	}
	// The mapped form belongs to the same socket-source bucket.
	blocked, _ := pending("[::ffff:192.0.2.1]:9876")
	_ = blocked.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := blocked.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("one source exceeded its pending allowance: %v", err)
	}
	other, server := pending("192.0.2.2:1234")
	waitStarted(server)
	write := make(chan error, 1)
	go func() { _, err := other.Write([]byte("GET / HTTP/1.1\r\n")); write <- err }()
	accepted := make(chan net.Conn, 1)
	go func() { c, _ := listener.Accept(); accepted <- c }()
	select {
	case c := <-accepted:
		if c == nil {
			t.Fatal("other source was refused")
		}
		_ = c.Close()
	case <-time.After(time.Second):
		t.Fatal("silent source blocked another source")
	}
	<-write
	_ = listener.Close()
	for _, c := range silent {
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("Close left an owned classifier alive: %v", err)
		}
	}
}
