// Package bittorrent adapts verified BitTorrent data to the common file receiver.
// It owns no user-facing job queue and never writes a user target directory.
package bittorrent

import (
	"errors"
	"strconv"
)

const MaxMetadataBytes = 512 << 10

var (
	ErrMetadata  = errors.New("torrent metadata is invalid or exceeds limits")
	ErrVersion   = errors.New("torrent version is unsupported")
	ErrIntegrity = errors.New("torrent integrity check failed")
	ErrStorage   = errors.New("torrent temporary storage is unavailable or full")
	ErrNoPeers   = errors.New("torrent peers or metadata are unavailable")
)

// scanBencode validates before any reflective decoder allocates. Metadata wire
// messages have a bencoded header followed by raw bytes, hence the consumed count.
func scanBencode(data []byte) (int, error) {
	nodes := 0
	var scan func(int, int) (int, error)
	scan = func(pos, depth int) (int, error) {
		nodes++
		if pos >= len(data) || depth > 32 || nodes > 32768 {
			return 0, ErrMetadata
		}
		switch data[pos] {
		case 'd', 'l':
			dict := data[pos] == 'd'
			pos++
			for pos < len(data) && data[pos] != 'e' {
				if dict && (data[pos] < '0' || data[pos] > '9') {
					return 0, ErrMetadata
				}
				var err error
				pos, err = scan(pos, depth+1)
				if err != nil {
					return 0, err
				}
				if dict {
					pos, err = scan(pos, depth+1)
					if err != nil {
						return 0, err
					}
				}
			}
			if pos == len(data) {
				return 0, ErrMetadata
			}
			return pos + 1, nil
		case 'i':
			start := pos + 1
			pos = start
			for pos < len(data) && data[pos] != 'e' {
				pos++
			}
			if pos == len(data) || pos-start > 20 || pos == start {
				return 0, ErrMetadata
			}
			if _, err := strconv.ParseInt(string(data[start:pos]), 10, 64); err != nil {
				return 0, ErrMetadata
			}
			return pos + 1, nil
		default:
			start := pos
			for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
				pos++
			}
			if pos == start || pos-start > 8 || pos == len(data) || data[pos] != ':' {
				return 0, ErrMetadata
			}
			n, err := strconv.ParseUint(string(data[start:pos]), 10, 32)
			if err != nil || n > uint64(len(data)-pos-1) {
				return 0, ErrMetadata
			}
			return pos + 1 + int(n), nil
		}
	}
	return scan(0, 0)
}

func validBencode(data []byte) bool {
	if len(data) == 0 || len(data) > MaxMetadataBytes {
		return false
	}
	n, err := scanBencode(data)
	return err == nil && n == len(data)
}
