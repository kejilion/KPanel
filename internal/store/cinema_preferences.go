package store

import (
	"math"
	"strings"
	"unicode/utf8"
)

const MaxCinemaCheckpoints = 50

// CinemaPlaybackPreference holds recent resume points, never a second media index.
// PUT entries are per-film upserts; omitted films and unrelated sections survive.
type CinemaPlaybackPreference struct {
	Entries []CinemaCheckpoint `json:"entries"`
}

type CinemaCheckpoint struct {
	Path        string  `json:"path"`
	HostID      string  `json:"hostId"`
	FileVersion string  `json:"fileVersion"`
	Position    float64 `json:"position"`
	Duration    float64 `json:"duration"`
	Completed   bool    `json:"completed"`
}

func validCinemaPlayback(value *CinemaPlaybackPreference) bool {
	if value == nil {
		return true
	}
	if len(value.Entries) > MaxCinemaCheckpoints {
		return false
	}
	seen := map[string]bool{}
	for _, item := range value.Entries {
		location := MusicPlaybackPreference{Path: item.Path, HostID: item.HostID, Position: item.Position, Repeat: "off"}
		key := item.HostID + "\x00" + item.Path
		if item.Path == "" || !validMusicPlayback(&location) || seen[key] || !utf8.ValidString(item.FileVersion) || len(item.FileVersion) == 0 || len(item.FileVersion) > 256 || strings.IndexFunc(item.FileVersion, func(r rune) bool { return r < 32 || r == 127 }) >= 0 || math.IsNaN(item.Duration) || math.IsInf(item.Duration, 0) || item.Duration <= 0 || item.Duration > 31536000 || item.Position > item.Duration {
			return false
		}
		seen[key] = true
	}
	return true
}

func mergeCinemaPlayback(previous, patch *CinemaPlaybackPreference) *CinemaPlaybackPreference {
	entries := append([]CinemaCheckpoint{}, patch.Entries...)
	seen := map[string]bool{}
	for _, item := range entries {
		seen[item.HostID+"\x00"+item.Path] = true
	}
	if previous != nil {
		for _, item := range previous.Entries {
			if !seen[item.HostID+"\x00"+item.Path] && len(entries) < MaxCinemaCheckpoints {
				entries = append(entries, item)
			}
		}
	}
	return &CinemaPlaybackPreference{Entries: entries}
}
