package bittorrent

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type publicDialer struct{ client *remotedownload.Client }

func (d publicDialer) DialerNetwork() string { return "tcp" }
func (d publicDialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	return d.client.DialPublic(ctx, "tcp", address)
}

// The peer engine may consume spare candidates during its initial handshakes.
// Remember only successfully dialled public peers, so increasing the connection
// budget can actually retry them without issuing extra tracker announcements.
type rememberedDialer struct {
	torrent.Dialer
	mu        sync.Mutex
	addresses []netip.AddrPort
}

func (d *rememberedDialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	conn, err := d.Dialer.Dial(ctx, address)
	if err == nil {
		if peer, parseErr := netip.ParseAddrPort(address); parseErr == nil && peer.Port() != 0 && remotedownload.PublicAddress(peer.Addr()) {
			d.mu.Lock()
			found := false
			for _, known := range d.addresses {
				found = found || known == peer
			}
			if !found {
				if len(d.addresses) == 128 {
					copy(d.addresses, d.addresses[1:])
					d.addresses = d.addresses[:127]
				}
				d.addresses = append(d.addresses, peer)
			}
			d.mu.Unlock()
		}
	}
	return conn, err
}

func (d *rememberedDialer) peers() []torrent.PeerInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	peers := make([]torrent.PeerInfo, 0, len(d.addresses))
	for _, address := range d.addresses {
		peers = append(peers, torrent.PeerInfo{Addr: net.TCPAddrFromAddrPort(address)})
	}
	return peers
}

// UDP replies are accepted only from recently contacted public endpoints. DHT
// is a passive lookup client: unsolicited queries never reach its bencode parser.
type publicPacketConn struct {
	net.PacketConn
	dht  bool
	mu   sync.Mutex
	sent map[netip.AddrPort]time.Time
}

func packetAddress(address net.Addr) (netip.AddrPort, bool) {
	u, ok := address.(*net.UDPAddr)
	if !ok || u.Port < 1 || u.Port > 65535 || u.Zone != "" {
		return netip.AddrPort{}, false
	}
	a, ok := netip.AddrFromSlice(u.IP)
	if !ok || !remotedownload.PublicAddress(a) {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(a.Unmap(), uint16(u.Port)), true
}

func (p *publicPacketConn) WriteTo(data []byte, address net.Addr) (int, error) {
	a, ok := packetAddress(address)
	if !ok || len(data) > 4096 {
		return 0, remotedownload.ErrAddressBlocked
	}
	p.mu.Lock()
	if p.sent == nil {
		p.sent = make(map[netip.AddrPort]time.Time)
	}
	now := time.Now()
	if len(p.sent) >= 256 {
		for key, at := range p.sent {
			if now.Sub(at) > 30*time.Second {
				delete(p.sent, key)
			}
		}
	}
	if _, exists := p.sent[a]; !exists && len(p.sent) >= 256 {
		p.mu.Unlock()
		return 0, ErrNoPeers
	}
	p.sent[a] = now
	p.mu.Unlock()
	return p.PacketConn.WriteTo(data, address)
}

func (p *publicPacketConn) ReadFrom(data []byte) (int, net.Addr, error) {
	for {
		n, addr, err := p.PacketConn.ReadFrom(data)
		if err != nil {
			return n, addr, err
		}
		a, ok := packetAddress(addr)
		if !ok {
			continue
		}
		p.mu.Lock()
		at, exists := p.sent[a]
		p.mu.Unlock()
		if !exists || time.Since(at) > 30*time.Second {
			continue
		}
		if p.dht {
			if n > 4096 || !validBencode(data[:n]) {
				continue
			}
			var envelope struct {
				Type string `bencode:"y"`
			}
			if bencode.Unmarshal(data[:n], &envelope) != nil || envelope.Type != "r" && envelope.Type != "e" {
				continue
			}
		}
		return n, addr, nil
	}
}

func listenPublicPacket(network, address string, dht bool) (net.PacketConn, error) {
	conn, err := net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}
	return &publicPacketConn{PacketConn: conn, dht: dht}, nil
}
