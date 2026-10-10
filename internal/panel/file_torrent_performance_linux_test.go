package panel

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/kejilion/kejilion-panel/internal/bittorrent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
	"golang.org/x/time/rate"
)

// Opt-in, self-contained fixtures: no Internet tracker, DHT or copyrighted data.
// The runtime cgroup includes seeders, Panel, real Agent and filesystem writes.
func TestTorrentTransferPerformance(t *testing.T) {
	if os.Getenv("KPANEL_BT_BENCHMARK") != "1" {
		t.Skip("opt-in real BT/Agent comparison")
	}
	kind := os.Getenv("KPANEL_BT_CASE")
	peerCount := 8
	peerRate := 512 << 10
	if kind == "scarce" {
		peerCount = 1
		peerRate = 2 << 20
	} else if kind != "swarm" {
		t.Fatal("unknown case")
	}
	mode := os.Getenv("KPANEL_BT_MODE")
	if mode != "off" && mode != "auto" {
		t.Fatal("unknown mode")
	}
	server, _ := newTestServer(t)
	_, _, target := realTransferAgent(t, server)
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.bin")
	file, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	private := true
	info := metainfo.Info{Name: "source.bin", Length: 64 << 20, PieceLength: 256 << 10, Private: &private}
	hash := sha256.New()
	block := make([]byte, info.PieceLength)
	for offset := int64(0); offset < info.Length; offset += info.PieceLength {
		for i := range block {
			block[i] = byte(i*31 + i/257)
		}
		binary.LittleEndian.PutUint64(block[:8], uint64(offset))
		if _, err = file.Write(block); err != nil {
			t.Fatal(err)
		}
		hash.Write(block)
		piece := sha1.Sum(block)
		info.Pieces = append(info.Pieces, piece[:]...)
	}
	file.Close()
	want := hex.EncodeToString(hash.Sum(nil))
	block = nil
	mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info)}
	ports := map[string]bool{}
	compact := []byte{}
	for index := 0; index < peerCount; index++ {
		dir := filepath.Join(root, strconv.Itoa(index))
		if err = os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Link(sourcePath, filepath.Join(dir, info.Name)); err != nil {
			t.Fatal(err)
		}
		cfg := torrent.NewDefaultClientConfig()
		cfg.SetListenAddr("127.0.0.1:0")
		cfg.DisableIPv6 = true
		cfg.DataDir = dir
		cfg.NoDHT = true
		cfg.DisableUTP = true
		cfg.DisablePEX = true
		cfg.DisableTrackers = true
		cfg.DisableWebtorrent = true
		cfg.DisableWebseeds = true
		cfg.NoDefaultPortForwarding = true
		cfg.Seed = true
		cfg.PieceHashersPerTorrent = 1
		cfg.MaxAllocPeerRequestDataPerConn = 64 << 10
		cfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(peerRate), 64<<10)
		cfg.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
		client, err := torrent.NewClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		spec, _ := torrent.TorrentSpecFromMetaInfoErr(&mi)
		seed, _, err := client.AddTorrentSpec(spec)
		if err != nil {
			t.Fatal(err)
		}
		seed.VerifyData()
		select {
		case <-seed.Complete().On():
		case <-time.After(20 * time.Second):
			t.Fatal("seeder timeout")
		}
		port := client.LocalPort()
		ports[strconv.Itoa(port)] = true
		peer := []byte{93, 184, 216, 34, 0, 0}
		binary.BigEndian.PutUint16(peer[4:], uint16(port))
		compact = append(compact, peer...)
	}
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bencode.MustMarshal(map[string]any{"interval": 120, "peers": string(compact)}))
	}))
	defer tracker.Close()
	_, port, _ := net.SplitHostPort(tracker.Listener.Addr().String())
	ports[port] = true
	mi.Announce = "http://fixture.test:" + port + "/announce"
	client := remotedownload.NewClient(remotedownload.Config{Resolver: torrentFixtureResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, p, _ := net.SplitHostPort(address)
		if !ports[p] {
			return nil, remotedownload.ErrAddressBlocked
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", p))
	}})
	server.btDownload = bittorrent.NewEngine(server.btCache, client).Download
	runtime.GC()
	var before, after syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &before)
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	maxPeers := 0
	started := time.Now()
	result := server.executeFileRemoteDownload(ctx, contract.FileRemoteDownloadRequest{SourceKind: "torrent", Torrent: bencode.MustMarshal(mi), TargetDirectory: "/home", Background: true, Acceleration: mode}, "bt-benchmark", func(p contract.FileTransferEvent) bool { maxPeers = max(maxPeers, p.Peers); return true })
	elapsed := time.Since(started)
	syscall.Getrusage(syscall.RUSAGE_SELF, &after)
	runtime.ReadMemStats(&memAfter)
	if result.State != "complete" {
		t.Fatalf("result=%#v", result)
	}
	saved, err := os.Open(filepath.Join(target, "home", info.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Close()
	hash = sha256.New()
	n, err := io.Copy(hash, saved)
	if err != nil || n != info.Length || hex.EncodeToString(hash.Sum(nil)) != want {
		t.Fatal("saved hash mismatch", err)
	}
	seconds := func(v syscall.Timeval) float64 { return float64(v.Sec) + float64(v.Usec)/1e6 }
	row, _ := json.Marshal(map[string]any{"case": kind, "mode": mode, "bytes": n, "seconds": elapsed.Seconds(), "cpu_seconds": seconds(after.Utime) + seconds(after.Stime) - seconds(before.Utime) - seconds(before.Stime), "max_rss_kib": after.Maxrss, "allocated_bytes": memAfter.TotalAlloc - memBefore.TotalAlloc, "max_peers": maxPeers, "hash_verified": true, "scope": "combined-seeders-panel-agent-process"})
	fmt.Printf("BT_BENCH %s\n", row)
}
