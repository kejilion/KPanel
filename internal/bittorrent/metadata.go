package bittorrent

import (
	"bytes"
	"crypto/sha1"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type File struct {
	Path           string
	Offset, Length int64
}
type Metadata struct {
	Name           string
	Files          []File
	Size           int64
	InfoBytes      []byte
	Hash           metainfo.Hash
	Private, Multi bool
}
type Source struct {
	Spec         *torrent.TorrentSpec
	HTTPTrackers []string
	Metadata     *Metadata
}

func safeName(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 255 || strings.TrimSpace(s) != s || !utf8.ValidString(s) || strings.ContainsAny(s, "/\\") || strings.HasPrefix(s, ".kpanel-") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateInfo(info *metainfo.Info) (Metadata, error) {
	if info.MetaVersion != 0 || !info.HasV1() {
		return Metadata{}, ErrVersion
	}
	if strings.Contains(info.Attr, "l") || len(info.SymlinkPath) > 0 {
		return Metadata{}, ErrMetadata
	}
	if !safeName(info.BestName()) || info.PieceLength < 16<<10 || info.PieceLength > 8<<20 || len(info.Pieces)%20 != 0 || len(info.Pieces)/20 > 65536 || len(info.Files) > 512 || len(info.Files) > 0 && info.Length != 0 {
		return Metadata{}, ErrMetadata
	}
	m := Metadata{Name: info.BestName(), Private: info.Private != nil && *info.Private, Multi: len(info.Files) > 0}
	entries := info.UpvertedFiles()
	if len(entries) == 0 || len(entries) > 512 {
		return Metadata{}, ErrMetadata
	}
	seen := make(map[string]bool)
	pathBytes := 0
	for _, f := range entries {
		parts := f.BestPath()
		if len(info.Files) == 0 {
			parts = []string{m.Name}
		}
		if len(parts) == 0 || len(parts) > 32 || f.Length < 0 || f.Length > contract.MaxFileTransferBytes-m.Size || strings.Contains(f.Attr, "l") || len(f.SymlinkPath) > 0 {
			return Metadata{}, ErrMetadata
		}
		for _, part := range parts {
			if !safeName(part) {
				return Metadata{}, ErrMetadata
			}
		}
		p := strings.Join(parts, "/")
		pathBytes += len(p)
		if len(p) > 4096 || pathBytes > 64<<10 || seen[p] {
			return Metadata{}, ErrMetadata
		}
		seen[p] = true
		m.Files = append(m.Files, File{Path: p, Offset: m.Size, Length: f.Length})
		m.Size += f.Length
	}
	for p := range seen {
		for parent := path.Dir(p); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return Metadata{}, ErrMetadata
			}
		}
	}
	if m.Size <= 0 || (m.Size+info.PieceLength-1)/info.PieceLength != int64(len(info.Pieces)/20) {
		return Metadata{}, ErrMetadata
	}
	return m, nil
}

func parseInfo(data []byte) (Metadata, error) {
	if !validBencode(data) {
		return Metadata{}, ErrMetadata
	}
	var info metainfo.Info
	if err := bencode.Unmarshal(data, &info); err != nil {
		return Metadata{}, ErrMetadata
	}
	m, err := validateInfo(&info)
	if err != nil {
		return m, err
	}
	m.InfoBytes = bytes.Clone(data)
	m.Hash = sha1.Sum(data)
	return m, nil
}

func Parse(raw string, data []byte) (Source, error) {
	var spec *torrent.TorrentSpec
	var metadata *Metadata
	var err error
	if len(data) > 0 {
		if raw != "" || !validBencode(data) {
			return Source{}, ErrMetadata
		}
		var mi metainfo.MetaInfo
		if bencode.Unmarshal(data, &mi) != nil {
			return Source{}, ErrMetadata
		}
		m, e := parseInfo(mi.InfoBytes)
		if e != nil {
			return Source{}, e
		}
		metadata = &m
		spec, err = torrent.TorrentSpecFromMetaInfoErr(&mi)
	} else {
		if len(raw) > remotedownload.MaxURLBytes || !strings.HasPrefix(raw, "magnet:?") {
			return Source{}, ErrMetadata
		}
		u, e := url.Parse(raw)
		if e != nil || u.Fragment != "" || u.Host != "" {
			return Source{}, ErrMetadata
		}
		q, e := url.ParseQuery(u.RawQuery)
		if e != nil || len(q["xt"]) != 1 || !strings.HasPrefix(q.Get("xt"), "urn:btih:") {
			return Source{}, ErrVersion
		}
		for k := range q {
			if k != "xt" && k != "tr" && k != "dn" && k != "xl" {
				return Source{}, ErrMetadata
			}
		}
		spec, err = torrent.TorrentSpecFromMagnetUri(raw)
	}
	if err != nil || spec == nil {
		return Source{}, ErrMetadata
	}
	result := Source{Spec: spec, Metadata: metadata}
	trackers := spec.Trackers
	spec.Trackers = nil
	spec.Sources = nil
	spec.Webseeds = nil
	spec.PeerAddrs = nil
	spec.DhtNodes = nil
	count := 0
	for _, tier := range trackers {
		for _, raw := range tier {
			count++
			if count > 16 || len(raw) > 2048 {
				return Source{}, ErrMetadata
			}
			u, e := url.Parse(raw)
			if e != nil || u.User != nil || u.Fragment != "" {
				return Source{}, ErrMetadata
			}
			if u.Scheme == "udp" {
				if u.Port() == "" {
					return Source{}, ErrMetadata
				}
				port, e := strconv.Atoi(u.Port())
				if e != nil || port < 1 || port > 65535 {
					return Source{}, ErrMetadata
				}
				copyURL := *u
				copyURL.Scheme = "http"
				if _, e = remotedownload.ValidateURL(copyURL.String()); e != nil {
					return Source{}, ErrMetadata
				}
				spec.Trackers = append(spec.Trackers, []string{u.String()})
			} else {
				if _, e = remotedownload.ValidateURL(raw); e != nil {
					return Source{}, ErrMetadata
				}
				result.HTTPTrackers = append(result.HTTPTrackers, raw)
			}
		}
	}
	return result, nil
}
