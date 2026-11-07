package store

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

func cinemaPoint(path string, position float64) CinemaCheckpoint {
	return CinemaCheckpoint{Path: path, HostID: "edge", FileVersion: "v1", Position: position, Duration: 120}
}

func TestCinemaCheckpointsMergePersistAndClone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, version := s.Preferences()
	a := cinemaPoint("/films/a.mp4", 30)
	input := &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{a}}
	if err := s.PatchPreferences(version, Preferences{CinemaPlayback: input}); err != nil {
		t.Fatal(err)
	}
	input.Entries[0].Position = 99
	state, version := s.Preferences()
	if state.CinemaPlayback.Entries[0].Position != 30 {
		t.Fatal("input aliases store")
	}
	state.CinemaPlayback.Entries[0].Position = 100
	b := cinemaPoint("/films/b.mp4", 40)
	if err := s.PatchPreferences(version, Preferences{CinemaPlayback: &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{b}}, MusicPlayback: &MusicPlaybackPreference{Path: "/music/a.mp3", Position: 10, Repeat: "off"}}); err != nil {
		t.Fatal(err)
	}
	state, version = s.Preferences()
	if len(state.CinemaPlayback.Entries) != 2 || state.CinemaPlayback.Entries[1].Position != 30 {
		t.Fatal("upsert lost other film or snapshot aliases store")
	}
	a.Position = 0 // Restart is an explicit update, not max(position).
	if err := s.PatchPreferences(version, Preferences{CinemaPlayback: &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{a}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PatchPreferences(version, Preferences{CinemaPlayback: &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{b}}}); err != ErrConflict {
		t.Fatalf("stale update accepted: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, _ = reopened.Preferences()
	if len(state.CinemaPlayback.Entries) != 2 || state.CinemaPlayback.Entries[0].Position != 0 || state.MusicPlayback.Position != 10 {
		t.Fatalf("restart lost state: %+v", state)
	}
}

func TestCinemaCheckpointBoundsAndIdentity(t *testing.T) {
	valid := cinemaPoint("/films/a.mp4", 30)
	for _, mutate := range []func(*CinemaCheckpoint){
		func(c *CinemaCheckpoint) { c.Path = "/films/../secret" },
		func(c *CinemaCheckpoint) { c.HostID = "bad/host" },
		func(c *CinemaCheckpoint) { c.Position = math.NaN() },
		func(c *CinemaCheckpoint) { c.Position = 121 },
		func(c *CinemaCheckpoint) { c.Duration = math.Inf(1) },
		func(c *CinemaCheckpoint) { c.Duration = 0 },
		func(c *CinemaCheckpoint) { c.FileVersion = "" },
	} {
		item := valid
		mutate(&item)
		if ValidatePreferences(Preferences{CinemaPlayback: &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{item}}}) == nil {
			t.Fatalf("accepted invalid checkpoint: %+v", item)
		}
	}
	if validCinemaPlayback(&CinemaPlaybackPreference{Entries: []CinemaCheckpoint{valid, valid}}) {
		t.Fatal("duplicate accepted")
	}
	previous := &CinemaPlaybackPreference{}
	for i := 0; i < MaxCinemaCheckpoints+4; i++ {
		previous = mergeCinemaPlayback(previous, &CinemaPlaybackPreference{Entries: []CinemaCheckpoint{cinemaPoint(fmt.Sprintf("/films/%d.mp4", i), 1)}})
	}
	if len(previous.Entries) != MaxCinemaCheckpoints || previous.Entries[0].Path != "/films/53.mp4" {
		t.Fatal("not bounded newest-first")
	}
	tooMany := append(previous.Entries, valid)
	if validCinemaPlayback(&CinemaPlaybackPreference{Entries: tooMany}) {
		t.Fatal("oversized patch accepted")
	}
}
