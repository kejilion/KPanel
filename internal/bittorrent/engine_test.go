package bittorrent

import (
	"bytes"
	"context"
	"encoding/binary"
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

func TestEngineDownloadsTorrentAndMagnetFromRealPeer(t *testing.T) {
	data := make([]byte, 4<<20)
	for i := range data {
		data[i] = byte(i*31 + i/32771)
	}
	info := testInfo(data)
	seeder := newFixtureSeeder(t, data, info)
	var announces atomic.Int32
	trackerHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		announces.Add(1)
		if len(r.URL.Query().Get("info_hash")) != 20 || r.URL.Query().Get("port") == "0" {
			t.Error("invalid announce")
		}
		compact := []byte{93, 184, 216, 34, 0, 0}
		binary.BigEndian.PutUint16(compact[4:], uint16(seeder.LocalPort()))
		w.Write(bencode.MustMarshal(map[string]any{"interval": 120, "peers": string(compact)}))
	}))
	defer trackerHTTP.Close()
	_, trackerPort, _ := net.SplitHostPort(trackerHTTP.Listener.Addr().String())
	client := remotedownload.NewClient(remotedownload.Config{Resolver: fixtureResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(address)
		if port != trackerPort && port != strconv.Itoa(seeder.LocalPort()) {
			return nil, remotedownload.ErrAddressBlocked
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}})
	mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info), Announce: "http://fixture.test:" + trackerPort + "/announce?passkey=secret"}
	for _, kind := range []string{"torrent", "magnet"} {
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
			if kind == "torrent" {
				source, err = Parse("", bencode.MustMarshal(mi))
			} else {
				source, err = Parse(mi.Magnet(nil, &info).String(), nil)
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
	if announces.Load() != 2 {
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
