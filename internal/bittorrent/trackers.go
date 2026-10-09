package bittorrent

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	alog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/tracker"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func silentLogger() alog.Logger { l := alog.NewLogger(); l.Handlers = nil; return l }

func compactPeers(data []byte, width int) []torrent.PeerInfo {
	if len(data)%width != 0 {
		return nil
	}
	var peers []torrent.PeerInfo
	for len(data) >= width && len(peers) < 128 {
		ip, ok := netip.AddrFromSlice(data[:width-2])
		port := binary.BigEndian.Uint16(data[width-2 : width])
		data = data[width:]
		if ok && remotedownload.PublicAddress(ip) && port != 0 {
			peers = append(peers, torrent.PeerInfo{Addr: &net.TCPAddr{IP: ip.AsSlice(), Port: int(port)}})
		}
	}
	return peers
}

func announceHTTP(ctx context.Context, client *remotedownload.Client, raw string, request tracker.AnnounceRequest) ([]torrent.PeerInfo, time.Duration, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, 0, ErrMetadata
	}
	q := u.Query()
	q.Set("info_hash", string(request.InfoHash[:]))
	q.Set("peer_id", string(request.PeerId[:]))
	q.Set("port", strconv.Itoa(int(request.Port)))
	q.Set("uploaded", "0")
	q.Set("downloaded", strconv.FormatInt(request.Downloaded, 10))
	q.Set("left", strconv.FormatInt(max(request.Left, 0), 10))
	q.Set("compact", "1")
	q.Set("numwant", "64")
	// A tracker may have a passkey; neither this URL nor its errors are persisted.
	u.RawQuery = q.Encode()
	response, err := client.Open(ctx, u.String())
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.ContentLength > MaxMetadataBytes {
		return nil, 0, ErrMetadata
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxMetadataBytes+1))
	if err != nil || !validBencode(data) {
		return nil, 0, ErrMetadata
	}
	var result struct {
		Peers    bencode.Bytes `bencode:"peers"`
		Peers6   []byte        `bencode:"peers6"`
		Interval int           `bencode:"interval"`
		Failure  string        `bencode:"failure reason"`
	}
	if bencode.Unmarshal(data, &result) != nil || result.Failure != "" {
		return nil, 0, ErrNoPeers
	}
	var compact []byte
	var peers []torrent.PeerInfo
	if len(result.Peers) > 0 && bencode.Unmarshal(result.Peers, &compact) == nil {
		peers = compactPeers(compact, 6)
	} else if len(result.Peers) > 0 {
		var list []struct {
			IP   string `bencode:"ip"`
			Port int    `bencode:"port"`
		}
		if bencode.Unmarshal(result.Peers, &list) != nil || len(list) > 128 {
			return nil, 0, ErrMetadata
		}
		for _, p := range list {
			ip, err := netip.ParseAddr(p.IP)
			if err == nil && remotedownload.PublicAddress(ip) && p.Port > 0 && p.Port <= 65535 {
				peers = append(peers, torrent.PeerInfo{Addr: &net.TCPAddr{IP: ip.AsSlice(), Port: p.Port}})
			}
		}
	}
	peers = append(peers, compactPeers(result.Peers6, 18)...)
	return peers[:min(len(peers), 128)], time.Duration(min(max(result.Interval, 120), 3600)) * time.Second, nil
}

func announceUDP(ctx context.Context, raw string, request tracker.AnnounceRequest) ([]torrent.PeerInfo, time.Duration, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, 0, ErrMetadata
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil || len(ips) == 0 || len(ips) > 8 {
		return nil, 0, ErrNoPeers
	}
	for _, ip := range ips {
		if !remotedownload.PublicAddress(ip) {
			return nil, 0, remotedownload.ErrAddressBlocked
		}
	}
	u.Host = net.JoinHostPort(ips[0].String(), u.Port())
	client, err := tracker.NewClient(u.String(), tracker.NewClientOpts{Logger: silentLogger(), ListenPacket: func(network, address string) (net.PacketConn, error) {
		return listenPublicPacket(network, address, false)
	}})
	if err != nil {
		return nil, 0, ErrNoPeers
	}
	defer client.Close()
	response, err := client.Announce(ctx, request, tracker.AnnounceOpt{})
	if err != nil {
		return nil, 0, ErrNoPeers
	}
	var peers []torrent.PeerInfo
	for _, p := range response.Peers {
		ip, ok := netip.AddrFromSlice(p.IP)
		if ok && remotedownload.PublicAddress(ip) && p.Port > 0 {
			peers = append(peers, torrent.PeerInfo{Addr: &net.TCPAddr{IP: ip.AsSlice(), Port: p.Port}})
		}
		if len(peers) == 128 {
			break
		}
	}
	return peers, time.Duration(min(max(response.Interval, 120), 3600)) * time.Second, nil
}

func runTrackers(ctx context.Context, client *remotedownload.Client, source Source, t *torrent.Torrent, peerID torrent.PeerID) func() {
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	urls := append([]string(nil), source.HTTPTrackers...)
	for _, tier := range source.Spec.Trackers {
		urls = append(urls, tier...)
	}
	for _, raw := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case gate <- struct{}{}:
				case <-ctx.Done():
					return
				}
				request := tracker.AnnounceRequest{InfoHash: source.Spec.InfoHash, PeerId: peerID, Port: 6881, NumWant: 64, Left: 1}
				if info := t.Info(); info != nil {
					request.Downloaded = t.BytesCompleted()
					request.Left = max(0, t.Length()-request.Downloaded)
				}
				attempt, cancel := context.WithTimeout(ctx, 15*time.Second)
				var peers []torrent.PeerInfo
				var interval time.Duration
				var err error
				if len(raw) >= 4 && raw[:4] == "udp:" {
					peers, interval, err = announceUDP(attempt, raw, request)
				} else {
					peers, interval, err = announceHTTP(attempt, client, raw, request)
				}
				cancel()
				<-gate
				if err == nil && ctx.Err() == nil {
					t.AddPeers(peers)
				}
				if interval < 2*time.Minute {
					interval = 2 * time.Minute
				}
				timer := time.NewTimer(interval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	return wg.Wait
}
