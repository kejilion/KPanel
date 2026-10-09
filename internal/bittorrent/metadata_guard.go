package bittorrent

import (
	"bytes"
	"crypto/sha1"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

// Callbacks run under the torrent client's lock. Never call client methods here.
// In particular, piece zero stays outside the library until our bounded decoder
// has checked the entire info dictionary. SetInfoBytes happens in the task loop.
type metadataGuard struct {
	mu     sync.Mutex
	hash   metainfo.Hash
	size   int
	data   []byte
	seen   []bool
	ready  chan Metadata
	failed chan error
	known  bool
}

func newMetadataGuard(hash metainfo.Hash, known bool) *metadataGuard {
	return &metadataGuard{hash: hash, known: known, ready: make(chan Metadata, 1), failed: make(chan error, 1)}
}

func (g *metadataGuard) fail() {
	select {
	case g.failed <- ErrMetadata:
	default:
	}
}

func (g *metadataGuard) handshake(_ *torrent.PeerConn, d *pp.ExtendedHandshakeMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for name := range d.M {
		if name != pp.ExtensionNameMetadata {
			delete(d.M, name)
		}
	}
	if len(d.V) > 128 {
		d.V = ""
	}
	d.Reqq = min(max(d.Reqq, 0), 64)
	if g.known {
		d.MetadataSize = 0
		return
	}
	if d.MetadataSize == 0 {
		return
	}
	if d.MetadataSize < 0 || d.MetadataSize > MaxMetadataBytes || g.size != 0 && g.size != d.MetadataSize {
		d.MetadataSize = -1
		g.fail()
		return
	}
	if g.size == 0 {
		g.size = d.MetadataSize
		g.data = make([]byte, g.size)
		g.seen = make([]bool, (g.size+16383)/16384)
	}
}

func (g *metadataGuard) message(c *torrent.PeerConn, msg *pp.Message) {
	g.mu.Lock()
	defer g.mu.Unlock()
	drop := func() { *msg = pp.Message{Keepalive: true} }
	// This download adapter does not serve payload data or accept inbound peers.
	// Dropping requests also avoids unbounded per-length upstream expvar keys.
	if msg.Type == pp.Request || msg.Type == pp.Port {
		drop()
		return
	}
	if msg.Type != pp.Extended {
		return
	}
	name := pp.ExtensionName("")
	if msg.ExtendedID != 0 {
		var err error
		name, _, err = c.LocalLtepProtocolMap.LookupId(msg.ExtendedID)
		if err != nil {
			drop()
			return
		}
	}
	if msg.ExtendedID != 0 && name != pp.ExtensionNameMetadata {
		drop()
		return
	}
	data := msg.ExtendedPayload
	n, err := scanBencode(data)
	if err != nil || len(data) > 64<<10 || name != pp.ExtensionNameMetadata && n != len(data) {
		g.fail()
		drop()
		return
	}
	if name != pp.ExtensionNameMetadata {
		return
	}
	var header pp.ExtendedMetadataRequestMsg
	if bencode.Unmarshal(data[:n], &header) != nil {
		g.fail()
		drop()
		return
	}
	// Never let a negative index reach requestedMetadataPiece in the library.
	if header.Piece < 0 || header.Piece >= 32 || header.Type < 0 || header.Type > 2 {
		g.fail()
		drop()
		return
	}
	if g.known {
		drop()
		return
	}
	if g.size == 0 || header.Piece >= len(g.seen) {
		g.fail()
		drop()
		return
	}
	if header.Type != pp.DataMetadataExtensionMsgType {
		if n != len(data) {
			g.fail()
			drop()
		}
		return
	}
	offset := header.Piece * 16384
	count := min(16384, g.size-offset)
	if header.TotalSize != g.size || len(data)-n != count {
		g.fail()
		drop()
		return
	}
	if g.seen[header.Piece] && !bytes.Equal(g.data[offset:offset+count], data[n:]) {
		g.fail()
		drop()
		return
	}
	copy(g.data[offset:], data[n:])
	g.seen[header.Piece] = true
	if header.Piece == 0 {
		drop()
	}
	for _, seen := range g.seen {
		if !seen {
			return
		}
	}
	if sha1.Sum(g.data) != g.hash {
		g.fail()
		drop()
		return
	}
	m, err := parseInfo(g.data)
	if err != nil {
		g.fail()
		drop()
		return
	}
	g.known = true
	select {
	case g.ready <- m:
	default:
	}
}
