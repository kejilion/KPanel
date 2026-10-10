package bittorrent

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type fixtureResolver struct{}

func (fixtureResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

func newFixtureSeeder(t *testing.T, data []byte, info metainfo.Info) *torrent.Client {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, info.Name), data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.SetListenAddr("127.0.0.1:0")
	cfg.DisableIPv6 = true
	cfg.DataDir = directory
	cfg.NoDHT = true
	cfg.DisableUTP = true
	cfg.DisablePEX = true
	cfg.DisableTrackers = true
	cfg.DisableWebtorrent = true
	cfg.DisableWebseeds = true
	cfg.NoDefaultPortForwarding = true
	cfg.Seed = true
	cfg.MaxAllocPeerRequestDataPerConn = 64 << 10
	cfg.PieceHashersPerTorrent = 1
	cfg.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := torrent.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	spec, err := torrent.TorrentSpecFromMetaInfoErr(&metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info)})
	if err != nil {
		t.Fatal(err)
	}
	seed, _, err := client.AddTorrentSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	seed.VerifyData()
	select {
	case <-seed.Complete().On():
	case <-time.After(15 * time.Second):
		t.Fatal("fixture seeder verification timeout")
	}
	return client
}

func TestStartupPeerRecovery(t *testing.T) {
	now := time.Unix(1_000, 0)
	var recovery startupPeerRecovery
	if recovery.shouldRetry(now, true, 1, 1, 0) || recovery.shouldRetry(now.Add(29*time.Second), true, 1, 1, 0) {
		t.Fatal("startup connection recovered before its grace period")
	}
	if !recovery.shouldRetry(now.Add(30*time.Second), true, 1, 1, 0) {
		t.Fatal("stalled startup connection was not recovered")
	}
	if recovery.shouldRetry(now.Add(31*time.Second), true, 1, 1, 0) || !recovery.shouldRetry(now.Add(61*time.Second), true, 1, 1, 0) || recovery.shouldRetry(now.Add(100*time.Second), true, 1, 1, 0) {
		t.Fatal("startup recovery interval or two-attempt bound changed")
	}
	for _, state := range []struct {
		name     string
		metadata bool
		active   int
		known    int
	}{
		{"metadata-pending", false, 1, 1},
		{"no-connected-peers", true, 0, 1},
		{"no-public-remembered-peers", true, 1, 0},
	} {
		t.Run(state.name, func(t *testing.T) {
			var r startupPeerRecovery
			if r.shouldRetry(now, true, 1, 1, 0) || r.shouldRetry(now.Add(time.Hour), state.metadata, state.active, state.known, 0) || r.shouldRetry(now.Add(2*time.Hour), true, 1, 1, 0) {
				t.Fatal("ineligible connection retained a retry timer")
			}
		})
	}
	var progressing startupPeerRecovery
	progressing.shouldRetry(now, true, 1, 1, 0)
	if progressing.shouldRetry(now.Add(30*time.Second), true, 1, 1, 1) || progressing.shouldRetry(now.Add(time.Hour), true, 1, 1, 0) {
		t.Fatal("a connection that delivered payload was recovered")
	}
	oldPeer, newPeer := new(torrent.PeerConn), new(torrent.PeerConn)
	if startupPeersRemoved([]*torrent.PeerConn{oldPeer}, []*torrent.PeerConn{oldPeer, newPeer}) || !startupPeersRemoved([]*torrent.PeerConn{oldPeer}, []*torrent.PeerConn{newPeer}) {
		t.Fatal("redial did not wait for the exact closed connection to leave")
	}
}

func TestEngineRecoversZeroPayloadStartup(t *testing.T) {
	data := make([]byte, 4<<20)
	for i := range data {
		data[i] = byte(i*31 + i/32771)
	}
	info := testInfo(data)
	private := true
	info.Private = &private
	seeder := newFixtureSeeder(t, data, info)
	stalled, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	stalledDone := make(chan error, 1)
	go func() {
		conn, err := stalled.Accept()
		if err != nil {
			stalledDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
		handshake := make([]byte, 68)
		if _, err = io.ReadFull(conn, handshake); err != nil {
			stalledDone <- err
			return
		}
		if handshake[0] != 19 || string(handshake[1:20]) != "BitTorrent protocol" {
			stalledDone <- errors.New("fixture did not receive plaintext BT handshake")
			return
		}
		clear(handshake[20:28])
		copy(handshake[48:], "-KP0001-stalled-peer!")
		bits := bytes.Repeat([]byte{0xff}, (len(info.Pieces)/20+7)/8)
		message := make([]byte, 5+len(bits))
		binary.BigEndian.PutUint32(message, uint32(1+len(bits)))
		message[4] = 5 // bitfield: this peer advertises every piece.
		copy(message[5:], bits)
		response := append(handshake, message...)
		response = append(response, 0, 0, 0, 1, 1) // unchoke, then withhold all payload.
		if _, err = conn.Write(response); err != nil {
			stalledDone <- err
			return
		}
		_, err = io.Copy(io.Discard, conn)
		stalledDone <- err
	}()
	var announces, dials atomic.Int32
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		announces.Add(1)
		compact := []byte{93, 184, 216, 34, 0, 0}
		binary.BigEndian.PutUint16(compact[4:], uint16(seeder.LocalPort()))
		_, _ = w.Write(bencode.MustMarshal(map[string]any{"interval": 120, "peers": string(compact)}))
	}))
	defer tracker.Close()
	_, trackerPort, _ := net.SplitHostPort(tracker.Listener.Addr().String())
	seedPort := strconv.Itoa(seeder.LocalPort())
	client := remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(address)
		if err != nil || (port != trackerPort && port != seedPort) {
			return nil, remotedownload.ErrAddressBlocked
		}
		target := net.JoinHostPort("127.0.0.1", port)
		if port == seedPort && dials.Add(1) == 1 {
			target = stalled.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}})
	cache, err := OpenCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info), Announce: "http://fixture.test:" + trackerPort + "/announce"}
	source, err := Parse("", bencode.MustMarshal(mi))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(cache, client)
	engine.noDHT = true
	engine.configure = func(cfg *torrent.ClientConfig) {
		cfg.HeaderObfuscationPolicy.Preferred = false
		cfg.HeaderObfuscationPolicy.RequirePreferred = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	result, downloadErr := engine.Download(ctx, source, false, func(Progress) bool { return true })
	if result != nil {
		defer result.Close()
	}
	if downloadErr != nil {
		t.Fatal(downloadErr)
	}
	body := result.Open(ctx)
	got, err := io.ReadAll(body)
	_ = body.Close()
	result.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("recovered payload mismatch: %v, bytes=%d", err, len(got))
	}
	select {
	case err := <-stalledDone:
		// Closing a peer with unread protocol bytes may send a TCP reset.
		// EOF and ECONNRESET both prove closure; a deadline still fails.
		if err != nil && !errors.Is(err, syscall.ECONNRESET) {
			t.Fatalf("first peer did not close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled startup connection remained open")
	}
	if dials.Load() != 2 || announces.Load() != 1 {
		t.Fatalf("recovery bypassed remembered peer or tracker budget: dials=%d announces=%d", dials.Load(), announces.Load())
	}
	entries, err := os.ReadDir(cache.root.Name())
	if err != nil || len(entries) != 0 {
		t.Fatalf("recovered payload staging was not reclaimed: %v", err)
	}
}

func TestEngineDownloadsTorrentAndMagnetFromRealPeer(t *testing.T) {
	data := make([]byte, 4<<20)
	for i := range data {
		data[i] = byte(i*31 + i/32771)
	}
	info := testInfo(data)
	seeder := newFixtureSeeder(t, data, info)
	privateInfo := info
	private := true
	privateInfo.Private = &private
	privateSeeder := newFixtureSeeder(t, data, privateInfo)
	privateMeta := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(privateInfo)}
	privateHash := privateMeta.HashInfoBytes()
	var announces atomic.Int32
	trackerHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		announces.Add(1)
		if len(r.URL.Query().Get("info_hash")) != 20 || r.URL.Query().Get("port") == "0" {
			t.Error("invalid announce")
		}
		compact := []byte{93, 184, 216, 34, 0, 0}
		port := seeder.LocalPort()
		if r.URL.Query().Get("info_hash") == string(privateHash[:]) {
			port = privateSeeder.LocalPort()
		}
		binary.BigEndian.PutUint16(compact[4:], uint16(port))
		w.Write(bencode.MustMarshal(map[string]any{"interval": 120, "peers": string(compact)}))
	}))
	defer trackerHTTP.Close()
	_, trackerPort, _ := net.SplitHostPort(trackerHTTP.Listener.Addr().String())
	client := remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(address)
		if port != trackerPort && port != strconv.Itoa(seeder.LocalPort()) && port != strconv.Itoa(privateSeeder.LocalPort()) {
			return nil, remotedownload.ErrAddressBlocked
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}})
	mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info), Announce: "http://fixture.test:" + trackerPort + "/announce?passkey=secret"}
	privateMeta.Announce = mi.Announce
	for _, kind := range []string{"torrent", "magnet", "private-torrent", "private-magnet"} {
		t.Run(kind, func(t *testing.T) {
			cache, err := OpenCache(filepath.Join(t.TempDir(), "cache"))
			if err != nil {
				t.Fatal(err)
			}
			defer cache.Close()
			engine := NewEngine(cache, client)
			engine.noDHT = true
			engine.configure = func(cfg *torrent.ClientConfig) {
				if !cfg.DisablePEX {
					t.Fatal("private or unknown torrent enables peer exchange")
				}
			}
			var source Source
			selected, selectedInfo := mi, info
			if kind == "private-torrent" || kind == "private-magnet" {
				selected, selectedInfo = privateMeta, privateInfo
			}
			if kind == "torrent" || kind == "private-torrent" {
				source, err = Parse("", bencode.MustMarshal(selected))
			} else {
				source, err = Parse(selected.Magnet(nil, &selectedInfo).String(), nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := engine.Download(ctx, source, true, func(p Progress) bool {
				if p.Bytes > p.Total || p.Peers > 16 {
					t.Error("invalid bounded progress")
				}
				return true
			})
			if kind == "private-magnet" {
				if !errors.Is(err, ErrPrivateMagnet) || result != nil {
					t.Fatalf("private magnet accepted: %v", err)
				}
				entries, _ := os.ReadDir(cache.root.Name())
				if len(entries) != 0 {
					t.Fatal("rejected private magnet left staging")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := result.Open(ctx)
			got, err := io.ReadAll(body)
			body.Close()
			result.Close()
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("payload mismatch: %v, bytes=%d", err, len(got))
			}
			entries, _ := os.ReadDir(cache.root.Name())
			if len(entries) != 0 {
				t.Fatal("completed payload not reclaimed")
			}
		})
	}
	if announces.Load() != 4 {
		t.Fatalf("announces=%d", announces.Load())
	}
}

func TestEngineCancellationCleansStageAndTracker(t *testing.T) {
	entered := make(chan struct{})
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(closed) }))
	defer server.Close()
	client := remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}})
	cache, err := OpenCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	source, err := Parse("magnet:?xt=urn:btih:0123456789012345678901234567890123456789&tr=http%3A%2F%2Ffixture.test%2Fannounce", nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(cache, client)
	engine.noDHT = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := engine.Download(ctx, source, true, func(Progress) bool { return true }); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("tracker not opened")
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not cancel")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("tracker leaked")
	}
	entries, _ := os.ReadDir(cache.root.Name())
	if len(entries) != 0 {
		t.Fatal("cancelled payload retained")
	}
}
