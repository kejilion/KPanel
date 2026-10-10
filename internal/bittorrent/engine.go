package bittorrent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"slices"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
	"golang.org/x/time/rate"
)

type Progress struct {
	Name                string
	Bytes, Total, Speed int64
	Peers               int
	Mode                string
}
type Engine struct {
	cache     *Cache
	client    *remotedownload.Client
	configure func(*torrent.ClientConfig)
	dialer    torrent.Dialer
	noDHT     bool
}

func NewEngine(cache *Cache, client *remotedownload.Client) *Engine {
	return &Engine{cache: cache, client: client}
}

type peerPolicy struct {
	limit    int
	baseline float64
	probing  bool
	finished bool
}

// Recover only startup connections that never deliver a payload byte. A live
// peer can stop making progress before its first request; keep recovery bounded
// and leave the existing overall deadline and progressing downloads alone.
type startupPeerRecovery struct {
	connectedSince time.Time
	attempts       int
	finished       bool
}

func (r *startupPeerRecovery) shouldRetry(now time.Time, metadataReady bool, active, remembered int, received int64) bool {
	if received > 0 {
		r.finished = true
	}
	if r.finished || r.attempts >= 2 {
		return false
	}
	if !metadataReady || active == 0 || remembered == 0 {
		r.connectedSince = time.Time{}
		return false
	}
	if r.connectedSince.IsZero() {
		r.connectedSince = now
		return false
	}
	if now.Sub(r.connectedSince) < 30*time.Second {
		return false
	}
	r.attempts++
	r.connectedSince = time.Time{}
	return true
}

func startupPeersRemoved(closed, current []*torrent.PeerConn) bool {
	for _, peer := range closed {
		if slices.Contains(current, peer) {
			return false
		}
	}
	return true
}

func (p *peerPolicy) observe(speed float64, remaining int64, pressure bool) int {
	if p.limit == 0 {
		p.limit = 4
	}
	if pressure {
		p.limit = 4
		p.finished = true
		p.probing = false
		return p.limit
	}
	if p.finished {
		return p.limit
	}
	if p.probing {
		if speed < p.baseline*1.15 {
			p.limit /= 2
			p.finished = true
			p.probing = false
			return p.limit
		}
		p.probing = false
	}
	if remaining < 32<<20 || speed <= 0 {
		return p.limit
	}
	if p.limit >= 16 {
		p.finished = true
		return p.limit
	}
	p.baseline = speed
	p.limit *= 2
	p.probing = true
	return p.limit
}

func (e *Engine) Download(ctx context.Context, source Source, smart bool, progress func(Progress) bool) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stage, err := e.cache.create(ctx)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			stage.Close()
		}
	}()
	guard := newMetadataGuard(source.Spec.InfoHash, source.Metadata != nil)
	cfg := torrent.NewDefaultClientConfig()
	cfg.DefaultStorage = stage
	cfg.NoDHT = true
	// The metadata of a magnet may turn out to be private. Keep peer exchange
	// disabled for the entire session so an early handshake cannot leak peers.
	cfg.DisablePEX = true
	cfg.DisableTCP = true
	cfg.DisableUTP = true
	cfg.AcceptPeerConnections = false
	cfg.NoDefaultPortForwarding = true
	cfg.NoUpload = true
	cfg.Seed = false
	cfg.DisableWebtorrent = true
	cfg.DisableWebseeds = true
	cfg.DisableTrackers = true
	cfg.EstablishedConnsPerTorrent = 4
	cfg.HalfOpenConnsPerTorrent = 4
	cfg.TotalHalfOpenConns = 4
	cfg.TorrentPeersHighWater = 128
	cfg.TorrentPeersLowWater = 16
	cfg.MaxUnverifiedBytes = 8 << 20
	cfg.MaxAllocPeerRequestDataPerConn = 64 << 10
	cfg.PieceHashersPerTorrent = 1
	cfg.NominalDialTimeout = 5 * time.Second
	cfg.DialRateLimiter = rate.NewLimiter(4, 4)
	cfg.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg.Logger = silentLogger()
	cfg.Callbacks.ReadMessage = guard.message
	cfg.Callbacks.ReadExtendedHandshake = guard.handshake
	cfg.ConfigureAnacrolixDhtServer = func(c *dht.ServerConfig) { c.Passive = true; c.SendLimiter = rate.NewLimiter(4, 8) }
	cfg.DhtStartingNodes = func(string) dht.StartingNodesGetter {
		return func() ([]dht.Addr, error) {
			lookup, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var result []dht.Addr
			for _, host := range []string{"router.bittorrent.com", "router.utorrent.com", "dht.transmissionbt.com"} {
				ips, _ := net.DefaultResolver.LookupNetIP(lookup, "ip4", host)
				for _, ip := range ips[:min(len(ips), 8)] {
					if remotedownload.PublicAddress(ip) {
						result = append(result, dht.NewAddr(&net.UDPAddr{IP: ip.AsSlice(), Port: 6881}))
					}
				}
			}
			return result, nil
		}
	}
	if e.configure != nil {
		e.configure(cfg)
	}
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, ErrNoPeers
	}
	defer client.Close()
	dialer := e.dialer
	if dialer == nil {
		dialer = publicDialer{e.client}
	}
	remembered := &rememberedDialer{Dialer: dialer}
	client.AddDialer(remembered)
	var dhtServer *dht.Server
	if !e.noDHT && (source.Metadata == nil || !source.Metadata.Private) {
		packet, err := listenPublicPacket("udp4", "0.0.0.0:0", true)
		if err == nil {
			defer packet.Close()
			dhtServer, err = client.NewAnacrolixDhtServer(packet)
			if err == nil {
				client.AddDhtServer(torrent.AnacrolixDhtServerWrapper{Server: dhtServer})
				defer dhtServer.Close()
			}
		}
	}
	spec := *source.Spec
	spec.Trackers = nil
	t, _, err := client.AddTorrentSpec(&spec)
	if err != nil {
		if errors.Is(err, ErrStorage) {
			return nil, ErrStorage
		}
		return nil, ErrMetadata
	}
	waitTrackers := runTrackers(ctx, e.client, source, t, client.PeerID())
	defer func() { cancel(); waitTrackers() }()
	metadata := source.Metadata
	if metadata != nil {
		t.DownloadAll()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastBytes := int64(0)
	lastProgress := time.Now()
	window := lastProgress
	windowBytes := int64(0)
	policy := peerPolicy{limit: 4}
	recovery := startupPeerRecovery{}
	var retryPeers []torrent.PeerInfo
	var retryClosed []*torrent.PeerConn
	mode := "bt-standard"
	if smart {
		mode = "bt-adaptive"
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-guard.failed:
			return nil, err
		case err := <-stage.failed:
			return nil, err
		case m := <-guard.ready:
			// Privacy is unknown until magnet metadata arrives. Do not download
			// or retry peers discovered through DHT after learning it is private.
			// A .torrent supplies this flag before any discovery and is supported.
			if m.Private {
				return nil, ErrPrivateMagnet
			}
			if err = t.SetInfoBytes(m.InfoBytes); err != nil {
				if errors.Is(err, ErrStorage) {
					return nil, ErrStorage
				}
				return nil, ErrMetadata
			}
			metadata = &m
			t.DownloadAll()
		case <-t.Complete().On():
			if metadata == nil {
				return nil, ErrIntegrity
			}
			if stage.file.Sync() != nil {
				return nil, ErrStorage
			}
			keep = true
			return &Result{Metadata: *metadata, stage: stage}, nil
		case now := <-ticker.C:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			stats := t.Stats()
			p := Progress{Mode: mode, Peers: stats.ActivePeers}
			received := stats.ConnStats.BytesReadData.Int64()
			if len(retryPeers) != 0 {
				// Wait until the exact closed connections leave the client's set;
				// otherwise AddPeers can discard a still-connected address. Newly
				// discovered connections do not delay this bounded recovery.
				if received > 0 {
					retryPeers, retryClosed = nil, nil
				} else if startupPeersRemoved(retryClosed, t.PeerConns()) {
					t.AddPeers(retryPeers)
					retryPeers, retryClosed = nil, nil
				}
			}
			knownPeers := remembered.peers()
			if len(retryPeers) == 0 && recovery.shouldRetry(now, metadata != nil, stats.ActivePeers, len(knownPeers), received) {
				retryClosed = t.PeerConns()
				for _, peer := range retryClosed {
					_ = peer.Close()
				}
				retryPeers = knownPeers
			}
			if metadata != nil {
				p.Name = metadata.Name
				p.Total = metadata.Size
				p.Bytes = stage.verified()
			}
			p.Speed = max(0, p.Bytes-lastBytes)
			lastBytes = p.Bytes
			if p.Speed > 0 {
				lastProgress = now
			}
			if now.Sub(lastProgress) > 2*time.Minute {
				return nil, ErrNoPeers
			}
			if smart && metadata != nil && now.Sub(window) >= 8*time.Second {
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)
				elapsed := now.Sub(window)
				pressure := mem.HeapAlloc > 160<<20 || stage.writeNanos.Swap(0) > int64(elapsed)/2
				limit := policy.observe(float64(p.Bytes-windowBytes)/elapsed.Seconds(), metadata.Size-p.Bytes, pressure)
				if previous := t.SetMaxEstablishedConns(limit); limit > previous {
					t.AddPeers(remembered.peers())
				}
				window = now
				windowBytes = p.Bytes
			}
			if !progress(p) {
				return nil, context.Canceled
			}
		}
	}
}

func (s *stagedFile) verified() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.complete) == 0 {
		return 0
	}
	var n int64
	// All non-final pieces have the same length; retain it independently of any
	// unverified downloaded bytes reported by the peer engine.
	for i, done := range s.complete {
		if done {
			n += min(s.pieceLength, s.metadata.Size-int64(i)*s.pieceLength)
		}
	}
	return n
}
