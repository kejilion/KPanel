package panel

import (
	"bytes"
	"context"
	"crypto/sha1"
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
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/kejilion/kejilion-panel/internal/bittorrent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type torrentFixtureResolver struct{}

func (torrentFixtureResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

func TestTorrentAdapterUsesCommonReceiverAndAtomicDirectory(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(strconv.FormatBool(multi), func(t *testing.T) {
			server, _ := newTestServer(t)
			agent, _, targetRoot := realTransferAgent(t, server)
			agent.lostChunk, agent.lostCommit = true, true
			data := make([]byte, 12<<20)
			for i := range data {
				data[i] = byte(i*31 + i/65521)
			}
			private := true
			info := metainfo.Info{Name: "bundle", Length: int64(len(data)), PieceLength: 64 << 10, Private: &private}
			for offset := 0; offset < len(data); offset += 64 << 10 {
				hash := sha1.Sum(data[offset:min(offset+(64<<10), len(data))])
				info.Pieces = append(info.Pieces, hash[:]...)
			}
			seedRoot := t.TempDir()
			if multi {
				info.Length = 0
				info.Files = []metainfo.FileInfo{{Length: 6 << 20, Path: []string{"a.bin"}}, {Length: 6 << 20, Path: []string{"sub", "b.bin"}}}
				if err := os.MkdirAll(filepath.Join(seedRoot, "bundle", "sub"), 0700); err != nil {
					t.Fatal(err)
				}
				os.WriteFile(filepath.Join(seedRoot, "bundle", "a.bin"), data[:6<<20], 0600)
				os.WriteFile(filepath.Join(seedRoot, "bundle", "sub", "b.bin"), data[6<<20:], 0600)
			} else {
				os.WriteFile(filepath.Join(seedRoot, "bundle"), data, 0600)
			}
			cfg := torrent.NewDefaultClientConfig()
			cfg.SetListenAddr("127.0.0.1:0")
			cfg.DisableIPv6 = true
			cfg.DataDir = seedRoot
			cfg.NoDHT = true
			cfg.DisableUTP = true
			cfg.DisablePEX = true
			cfg.DisableTrackers = true
			cfg.DisableWebtorrent = true
			cfg.DisableWebseeds = true
			cfg.NoDefaultPortForwarding = true
			cfg.Seed = true
			cfg.MaxAllocPeerRequestDataPerConn = 64 << 10
			cfg.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
			seedClient, err := torrent.NewClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer seedClient.Close()
			mi := metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info)}
			spec, _ := torrent.TorrentSpecFromMetaInfoErr(&mi)
			seed, _, err := seedClient.AddTorrentSpec(spec)
			if err != nil {
				t.Fatal(err)
			}
			seed.VerifyData()
			select {
			case <-seed.Complete().On():
			case <-time.After(15 * time.Second):
				t.Fatal("seeder timeout")
			}
			tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				peers := []byte{93, 184, 216, 34, 0, 0}
				binary.BigEndian.PutUint16(peers[4:], uint16(seedClient.LocalPort()))
				w.Write(bencode.MustMarshal(map[string]any{"interval": 120, "peers": string(peers)}))
			}))
			defer tracker.Close()
			_, trackerPort, _ := net.SplitHostPort(tracker.Listener.Addr().String())
			mi.Announce = "http://fixture.test:" + trackerPort + "/announce?private=secret"
			client := remotedownload.NewClient(remotedownload.Config{Resolver: torrentFixtureResolver{}, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
				_, port, _ := net.SplitHostPort(address)
				if port != trackerPort && port != strconv.Itoa(seedClient.LocalPort()) {
					return nil, remotedownload.ErrAddressBlocked
				}
				return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			}})
			server.btDownload = bittorrent.NewEngine(server.btCache, client).Download
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result := server.executeFileRemoteDownload(ctx, contract.FileRemoteDownloadRequest{SourceKind: "torrent", Torrent: bencode.MustMarshal(mi), TargetDirectory: "/home", Background: true}, "torrent-test", func(event contract.FileTransferEvent) bool {
				if event.TransferMode == "bt-adaptive" && event.LoadedBytes != 0 {
					t.Error("source progress impersonated receiver acknowledgement")
				}
				return true
			})
			if result.State != "complete" || result.Entry == nil {
				t.Fatalf("result=%#v", result)
			}
			var got []byte
			if multi {
				a, e := os.ReadFile(filepath.Join(targetRoot, "home", "bundle", "a.bin"))
				if e != nil {
					t.Fatal(e)
				}
				b, e := os.ReadFile(filepath.Join(targetRoot, "home", "bundle", "sub", "b.bin"))
				if e != nil {
					t.Fatal(e)
				}
				got = append(a, b...)
				if result.Entry.Kind != "directory" {
					t.Fatal("directory not atomically published")
				}
			} else {
				got, err = os.ReadFile(filepath.Join(targetRoot, "home", "bundle"))
				if err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(got, data) {
				t.Fatal("final bytes differ")
			}
			if agent.commitAttempts != 1 || agent.chunkBytes < 20<<20 {
				t.Fatal("common acknowledgement/commit recovery bypassed")
			}
			entries, _ := os.ReadDir(filepath.Join(server.config.DataDir, "file-transfers", "bt-cache"))
			if len(entries) != 0 {
				t.Fatal("BT staging leaked")
			}
		})
	}
}

func TestRemoteDownloadInputKeepsProtocolBoundary(t *testing.T) {
	for _, tc := range []struct {
		input contract.FileRemoteDownloadRequest
		valid bool
	}{
		{contract.FileRemoteDownloadRequest{URL: "https://example.com/file", Acceleration: "auto"}, true},
		{contract.FileRemoteDownloadRequest{URL: "magnet:?xt=urn:btih:0123456789012345678901234567890123456789", Background: true}, true},
		{contract.FileRemoteDownloadRequest{URL: "https://example.com/metadata", SourceKind: "torrent", Background: true}, true},
		{contract.FileRemoteDownloadRequest{URL: "file:///etc/passwd"}, false},
		{contract.FileRemoteDownloadRequest{URL: "https://example.com/file", Acceleration: "unlimited"}, false},
		{contract.FileRemoteDownloadRequest{URL: "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"}, false},
		{contract.FileRemoteDownloadRequest{URL: "https://example.com/file", Torrent: []byte("bad"), Background: true}, false},
	} {
		_, err := validateRemoteDownloadInput(&tc.input)
		if (err == nil) != tc.valid {
			t.Fatalf("input=%#v err=%v", tc.input, err)
		}
	}
}
