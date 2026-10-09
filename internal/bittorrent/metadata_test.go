package bittorrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

func testInfo(data []byte) metainfo.Info {
	info := metainfo.Info{Name: "payload.bin", Length: int64(len(data)), PieceLength: 16384}
	for offset := 0; offset < len(data); offset += 16384 {
		sum := sha1.Sum(data[offset:min(offset+16384, len(data))])
		info.Pieces = append(info.Pieces, sum[:]...)
	}
	return info
}
func testTorrent(t *testing.T, info metainfo.Info) []byte {
	t.Helper()
	data, err := bencode.Marshal(metainfo.MetaInfo{InfoBytes: bencode.MustMarshal(info)})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMetadataRejectsAllocationsPathsAndUnsupportedSources(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 16384)
	valid := testInfo(data)
	for _, payload := range [][]byte{[]byte("99999999:x"), []byte(strings.Repeat("l", 40) + strings.Repeat("e", 40)), []byte("d4:infod4:name99999999:xee"), bytes.Repeat([]byte("x"), MaxMetadataBytes+1)} {
		if _, err := Parse("", payload); err == nil {
			t.Fatal("malformed metadata accepted")
		}
	}
	for _, mutate := range []func(*metainfo.Info){
		func(i *metainfo.Info) { i.Name = "../escape" }, func(i *metainfo.Info) { i.PieceLength = 1 << 30 },
		func(i *metainfo.Info) { i.Length = 11 << 30 }, func(i *metainfo.Info) { i.Pieces = nil },
		func(i *metainfo.Info) {
			i.Length = 0
			i.Files = []metainfo.FileInfo{{Length: 16384, Path: []string{"..", "escape"}}}
		},
		func(i *metainfo.Info) {
			i.Length = 0
			i.Files = []metainfo.FileInfo{{Length: 8192, Path: []string{"a"}}, {Length: 8192, Path: []string{"a", "b"}}}
		},
		func(i *metainfo.Info) { i.MetaVersion = 2 },
	} {
		info := valid
		mutate(&info)
		if _, err := Parse("", testTorrent(t, info)); err == nil {
			t.Fatal("unsafe torrent accepted")
		}
	}
	if _, err := Parse("", testTorrent(t, valid)); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"&xs=http://127.0.0.1/a", "&ws=http://127.0.0.1/a", "&x.pe=127.0.0.1:22", "&xt=urn:btih:0123456789012345678901234567890123456789"} {
		if _, err := Parse("magnet:?xt=urn:btih:0123456789012345678901234567890123456789"+suffix, nil); err == nil {
			t.Fatal("alternate source accepted")
		}
	}
}

func TestMetadataGuardBoundsHandshakeAndRequests(t *testing.T) {
	g := newMetadataGuard(metainfo.Hash{}, false)
	d := pp.ExtendedHandshakeMessage{MetadataSize: MaxMetadataBytes + 1, M: map[pp.ExtensionName]pp.ExtensionNumber{"arbitrary-tracking-key": 1, "ut_metadata": 2}}
	g.handshake(nil, &d)
	if d.MetadataSize != -1 || len(d.M) != 1 {
		t.Fatal("unsafe handshake retained")
	}
	select {
	case <-g.failed:
	default:
		t.Fatal("missing rejection")
	}
	g = newMetadataGuard(metainfo.Hash{}, false)
	msg := pp.Message{Type: pp.Extended, ExtendedPayload: []byte("d1:m99999999:xee")}
	g.message(nil, &msg)
	if !msg.Keepalive {
		t.Fatal("unbounded payload reached decoder")
	}
	msg = pp.Message{Type: pp.Request, Length: 99999999}
	g.message(&torrent.PeerConn{}, &msg)
	if !msg.Keepalive {
		t.Fatal("arbitrary length reached expvar")
	}
}

func TestMetadataGuardWithholdsZeroUntilBoundedHashVerified(t *testing.T) {
	info := metainfo.Info{Name: "large.bin", Length: 16384 * 1000, PieceLength: 16384, Pieces: make([]byte, 20000)}
	data := bencode.MustMarshal(info)
	peer := &torrent.PeerConn{LocalLtepProtocolMap: &torrent.LocalLtepProtocolMap{Index: []pp.ExtensionName{pp.ExtensionNamePex, pp.ExtensionNameMetadata}, NumBuiltin: 2}}
	makeMessage := func(piece, total int, tail []byte) pp.Message {
		return pp.Message{Type: pp.Extended, ExtendedID: 2, ExtendedPayload: append(bencode.MustMarshal(pp.ExtendedMetadataRequestMsg{Piece: piece, TotalSize: total, Type: 1}), tail...)}
	}
	for _, kind := range []string{"valid", "negative-piece", "wrong-total", "short-block", "bad-hash", "declared-allocation"} {
		t.Run(kind, func(t *testing.T) {
			payload := bytes.Clone(data)
			hash := sha1.Sum(payload)
			if kind == "declared-allocation" {
				payload = []byte("d4:name99999999:xe")
				hash = sha1.Sum(payload)
			}
			g := newMetadataGuard(hash, false)
			d := pp.ExtendedHandshakeMessage{MetadataSize: len(payload)}
			g.handshake(peer, &d)
			if kind == "bad-hash" {
				payload[len(payload)-3] ^= 1
			}
			for piece := (len(payload) - 1) / 16384; piece >= 0; piece-- {
				tail := payload[piece*16384 : min((piece+1)*16384, len(payload))]
				msg := makeMessage(piece, len(payload), tail)
				switch kind {
				case "negative-piece":
					msg = makeMessage(-1, len(payload), tail)
				case "wrong-total":
					msg = makeMessage(piece, len(payload)+1, tail)
				case "short-block":
					msg = makeMessage(piece, len(payload), tail[:len(tail)-1])
				}
				g.message(peer, &msg)
				if piece == 0 && !msg.Keepalive {
					t.Fatal("piece zero reached unbounded library parser")
				}
			}
			if kind == "valid" {
				select {
				case got := <-g.ready:
					if !bytes.Equal(got.InfoBytes, data) {
						t.Fatal("info changed")
					}
				default:
					t.Fatal("validated metadata not released")
				}
			} else {
				select {
				case <-g.failed:
				default:
					t.Fatal("hostile metadata accepted")
				}
			}
		})
	}
}

func TestCacheIsBoundedAndDoesNotFollowSymlinks(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cache")
	cache, err := OpenCache(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	stage, err := cache.create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = cache.create(ctx); err == nil {
		t.Fatal("second staging allocation admitted")
	}
	info := testInfo(bytes.Repeat([]byte("x"), 16384))
	impl, err := stage.OpenTorrent(context.Background(), &info, metainfo.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	piece := impl.Piece(info.Piece(0))
	if _, err = piece.WriteAt(make([]byte, 2), 16383); err == nil {
		t.Fatal("piece overflow allowed")
	}
	if _, err = piece.WriteAt(bytes.Repeat([]byte("x"), 16384), 0); err != nil {
		t.Fatal(err)
	}
	piece.MarkComplete()
	if stage.verified() != 16384 {
		t.Fatal("verified counter")
	}
	piece.MarkNotComplete()
	if stage.verified() != 0 {
		t.Fatal("failed piece counted")
	}
	stage.Close()
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("staging leaked")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("safe"), 0600)
	if os.Symlink(outside, filepath.Join(directory, "payload-123")) == nil {
		if another, err := OpenCache(directory); err == nil {
			another.Close()
			t.Fatal("symlink staging accepted")
		}
		body, err := os.ReadFile(outside)
		if err != nil || string(body) != "safe" {
			t.Fatal("unowned file changed")
		}
	}
}

func TestTorrentDirectoryStreamIsDeterministic(t *testing.T) {
	cache, err := OpenCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	stage, err := cache.create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	stage.file.Write([]byte("onetwo"))
	result := Result{Metadata: Metadata{Name: "bundle", Multi: true, Size: 6, Files: []File{{Path: "a.txt", Length: 3}, {Path: "sub/b.txt", Offset: 3, Length: 3}}}, stage: stage}
	first := result.Open(context.Background())
	a, err := io.ReadAll(first)
	first.Close()
	if err != nil {
		t.Fatal(err)
	}
	second := result.Open(context.Background())
	b, err := io.ReadAll(second)
	second.Close()
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("reopened TAR changed")
	}
	// Abandon a stream before consuming its header. Close must unblock and
	// join the producer before the task can dispose of its staging file.
	abandoned := result.Open(context.Background()).(*directoryStream)
	closed := make(chan struct{})
	go func() { abandoned.Close(); close(closed) }()
	select {
	case <-closed:
		select {
		case <-abandoned.done:
		default:
			t.Fatal("Close returned before the TAR producer stopped")
		}
	case <-time.After(time.Second):
		t.Fatal("abandoned TAR producer did not stop")
	}
}

func TestPeerPolicyStopsUnhelpfulGrowth(t *testing.T) {
	p := peerPolicy{}
	if p.observe(100, 100<<20, false) != 8 {
		t.Fatal(p)
	}
	if p.observe(105, 100<<20, false) != 4 {
		t.Fatal(p)
	}
	if p.observe(100, 100<<20, false) != 4 {
		t.Fatal("flapping")
	}
	p = peerPolicy{}
	p.observe(100, 100<<20, false)
	if p.observe(200, 100<<20, false) != 16 {
		t.Fatal(p)
	}
	if p.observe(400, 100<<20, true) != 4 {
		t.Fatal("resource pressure ignored")
	}
}
