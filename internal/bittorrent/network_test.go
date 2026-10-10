package bittorrent

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/anacrolix/torrent/tracker"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type peerCacheTestDialer struct{ fail bool }

func (*peerCacheTestDialer) DialerNetwork() string { return "tcp" }
func (d *peerCacheTestDialer) Dial(context.Context, string) (net.Conn, error) {
	if d.fail {
		return nil, io.ErrUnexpectedEOF
	}
	left, right := net.Pipe()
	right.Close()
	return left, nil
}

func TestRememberedPeersArePublicSuccessfulAndBounded(t *testing.T) {
	base := &peerCacheTestDialer{}
	dialer := &rememberedDialer{Dialer: base}
	for port := 1; port <= 130; port++ {
		conn, err := dialer.Dial(context.Background(), "93.184.216.34:"+strconv.Itoa(port))
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
	}
	for _, address := range []string{"93.184.216.34:130", "127.0.0.1:80", "[fe80::1%lo]:80"} {
		conn, _ := dialer.Dial(context.Background(), address)
		conn.Close()
	}
	base.fail = true
	dialer.Dial(context.Background(), "1.1.1.1:80")
	peers := dialer.peers()
	if len(peers) != 128 || peers[0].Addr.String() != "93.184.216.34:3" || peers[127].Addr.String() != "93.184.216.34:130" {
		t.Fatal("peer cache leaked, duplicated or retained unsafe/failed address")
	}
}

type packet struct {
	data    string
	address *net.UDPAddr
}
type fakePacketConn struct {
	reads  []packet
	writes int
}

func (p *fakePacketConn) WriteTo(data []byte, _ net.Addr) (int, error) {
	p.writes++
	return len(data), nil
}
func (p *fakePacketConn) ReadFrom(data []byte) (int, net.Addr, error) {
	if len(p.reads) == 0 {
		return 0, nil, io.EOF
	}
	v := p.reads[0]
	p.reads = p.reads[1:]
	return copy(data, v.data), v.address, nil
}
func (*fakePacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{IP: net.IPv4zero, Port: 12345} }
func (*fakePacketConn) Close() error                     { return nil }
func (*fakePacketConn) SetDeadline(time.Time) error      { return nil }
func (*fakePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakePacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestPublicPacketConnBlocksPrivateAndUnsolicitedDHT(t *testing.T) {
	public := &net.UDPAddr{IP: net.ParseIP("93.184.216.34"), Port: 6881}
	other := &net.UDPAddr{IP: net.ParseIP("1.1.1.1"), Port: 6881}
	private := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 6881}
	fake := &fakePacketConn{reads: []packet{{"d1:y1:re", private}, {"d1:y1:re", other}, {"d1:y99999999:re", public}, {"d1:y1:qe", public}, {"d1:y1:re", public}}}
	conn := &publicPacketConn{PacketConn: fake, dht: true}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "224.0.0.1", "192.0.2.1"} {
		if _, err := conn.WriteTo([]byte("x"), &net.UDPAddr{IP: net.ParseIP(ip), Port: 80}); err == nil {
			t.Fatalf("allowed %s", ip)
		}
	}
	if fake.writes != 0 {
		t.Fatal("private packet reached socket")
	}
	if _, err := conn.WriteTo([]byte("query"), public); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4096)
	n, addr, err := conn.ReadFrom(buffer)
	if err != nil || addr.String() != public.String() || string(buffer[:n]) != "d1:y1:re" || len(fake.reads) != 0 {
		t.Fatal("unsafe DHT reply reached decoder", err)
	}
}

func TestHTTPTrackerRetainsTLSAndResponseBounds(t *testing.T) {
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("d8:intervali120e5:peers0:e")) }))
	defer tls.Close()
	client := remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, tls.Listener.Addr().String())
	}})
	if _, _, err := announceHTTP(context.Background(), client, "https://fixture.test/announce", tracker.AnnounceRequest{Port: 6881}); err == nil {
		t.Fatal("tracker skipped TLS verification")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("d5:peers99999999:xe")) }))
	defer server.Close()
	client = remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}})
	if _, _, err := announceHTTP(context.Background(), client, "http://fixture.test/announce", tracker.AnnounceRequest{Port: 6881}); err == nil {
		t.Fatal("tracker length declaration accepted")
	}
}
