package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Preferences is bounded Panel-owned UI state, shared by the single administrator.
// Nil sections have not been configured and may be migrated once by a client.
type Preferences struct {
	Browser         *BrowserPreferences `json:"browser,omitempty"`
	Locale          string              `json:"locale,omitempty"`
	Desktop         *DesktopPreferences `json:"desktop,omitempty"`
	GalleryLocation *MediaLocation      `json:"galleryLocation,omitempty"`
	MusicLocation   *MediaLocation      `json:"musicLocation,omitempty"`
	CinemaLocation  *MediaLocation      `json:"cinemaLocation,omitempty"`
}

type MediaLocation struct {
	Path   string `json:"path"`
	HostID string `json:"hostId"`
}

var mediaHostPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{0,128}$`)

func validMediaLocation(value *MediaLocation, allowRoot bool) bool {
	if value == nil {
		return true
	}
	p := value.Path
	if !utf8.ValidString(p) || len(p) > 4096 || utf8.RuneCountInString(p) > 1024 || strings.TrimSpace(p) != p || !strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.IndexFunc(p, func(r rune) bool { return r < 32 || r == 127 }) >= 0 || !mediaHostPattern.MatchString(value.HostID) {
		return false
	}
	if p == "/" {
		return allowRoot
	}
	for _, part := range strings.Split(p[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

type BrowserPreferences struct {
	StartURL string `json:"startURL"`
}

type DesktopPreferences struct {
	Mode           string              `json:"mode"`
	Windows        []DesktopWindow     `json:"windows"`
	WindowSizes    []DesktopWindowSize `json:"windowSizes"`
	SideSplitRatio float64             `json:"sideSplitRatio"`
	FocusedID      int64               `json:"focusedId"`
}

type DesktopGeometry struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type DesktopWindow struct {
	ID        int64           `json:"id"`
	Path      string          `json:"path"`
	Geometry  DesktopGeometry `json:"geometry"`
	Minimized bool            `json:"minimized"`
	Maximized bool            `json:"maximized"`
	Snap      string          `json:"snap"`
	Z         int             `json:"z"`
}

type DesktopWindowSize struct {
	Path   string  `json:"path"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

var desktopRoutePattern = regexp.MustCompile(`^/(overview|monitoring|processes|browser|ai|ai/s/[A-Za-z0-9_-]{1,128}|sites|sites/environment|sites/monitor|apps|docker|files|gallery|terminal|diagnostics|cluster|system|activity|settings|settings/guides|guides|offers|virtual-machines|jobs|audit|app-script/[A-Za-z0-9_-]{1,128}|command-shortcut/[a-f0-9]{32})$`)

func validDesktopPath(path string) bool {
	return len(path) <= 768 && utf8.ValidString(path) && !strings.Contains(path, "\\") && strings.IndexFunc(path, func(r rune) bool { return r < 32 || r == 127 }) < 0 && desktopRoutePattern.MatchString(strings.SplitN(strings.SplitN(path, "?", 2)[0], "#", 2)[0])
}

func validDimension(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= 16384
}

func ValidatePreferences(value Preferences) error {
	if !validMediaLocation(value.GalleryLocation, false) || (value.GalleryLocation != nil && value.GalleryLocation.HostID != "") || !validMediaLocation(value.MusicLocation, true) || !validMediaLocation(value.CinemaLocation, true) {
		return ErrInvalidRecord
	}
	if b := value.Browser; b != nil && b.StartURL != "" {
		u, err := url.Parse(b.StartURL)
		if len(b.StartURL) > 4096 || !utf8.ValidString(b.StartURL) || strings.IndexFunc(b.StartURL, func(r rune) bool { return r <= 32 || r == 127 }) >= 0 || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return ErrInvalidRecord
		}
	}
	if value.Locale != "" && value.Locale != "zh-CN" && value.Locale != "zh-TW" && value.Locale != "en-US" {
		return ErrInvalidRecord
	}
	d := value.Desktop
	if d == nil {
		return nil
	}
	if (d.Mode != "classic" && d.Mode != "desktop") || len(d.Windows) > 8 || len(d.WindowSizes) > 32 || math.IsNaN(d.SideSplitRatio) || d.SideSplitRatio <= 0 || d.SideSplitRatio >= 1 {
		return ErrInvalidRecord
	}
	ids := map[int64]bool{}
	for _, w := range d.Windows {
		g := w.Geometry
		if w.ID <= 0 || w.ID > 9007199254740991 || ids[w.ID] || !validDesktopPath(w.Path) || !validDimension(g.Width) || !validDimension(g.Height) || math.IsNaN(g.Left) || math.IsNaN(g.Top) || math.Abs(g.Left) > 16384 || math.Abs(g.Top) > 16384 || (w.Snap != "" && w.Snap != "left" && w.Snap != "right") || w.Z < 0 || w.Z > 1000 {
			return ErrInvalidRecord
		}
		ids[w.ID] = true
	}
	if d.FocusedID != 0 && !ids[d.FocusedID] {
		return ErrInvalidRecord
	}
	paths := map[string]bool{}
	for _, size := range d.WindowSizes {
		// Size preferences are canonical catalogue paths, never route queries.
		if (!validDesktopPath(size.Path) && size.Path != "/app-script" && size.Path != "/command-shortcut") || strings.ContainsAny(size.Path, "?#") || paths[size.Path] || !validDimension(size.Width) || !validDimension(size.Height) {
			return ErrInvalidRecord
		}
		paths[size.Path] = true
	}
	return nil
}

func clonePreferences(value Preferences) Preferences {
	if value.GalleryLocation != nil {
		v := *value.GalleryLocation
		value.GalleryLocation = &v
	}
	if value.MusicLocation != nil {
		v := *value.MusicLocation
		value.MusicLocation = &v
	}
	if value.CinemaLocation != nil {
		v := *value.CinemaLocation
		value.CinemaLocation = &v
	}
	if value.Browser != nil {
		b := *value.Browser
		value.Browser = &b
	}
	if value.Desktop != nil {
		d := *value.Desktop
		d.Windows = append([]DesktopWindow{}, d.Windows...)
		d.WindowSizes = append([]DesktopWindowSize{}, d.WindowSizes...)
		value.Desktop = &d
	}
	return value
}

func preferencesVersion(value Preferences) string {
	body, _ := json.Marshal(value)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(body))
}

func (s *Store) Preferences() (Preferences, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePreferences(s.data.Preferences), preferencesVersion(s.data.Preferences)
}

// PatchPreferences preserves omitted sections atomically under the version lock.
func (s *Store) PatchPreferences(expected string, patch Preferences) error {
	if err := ValidatePreferences(patch); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != preferencesVersion(s.data.Preferences) {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	if patch.Browser != nil {
		s.data.Preferences.Browser = clonePreferences(patch).Browser
	}
	if patch.Locale != "" {
		s.data.Preferences.Locale = patch.Locale
	}
	if patch.Desktop != nil {
		s.data.Preferences.Desktop = clonePreferences(patch).Desktop
	}
	if patch.GalleryLocation != nil {
		s.data.Preferences.GalleryLocation = clonePreferences(patch).GalleryLocation
	}
	if patch.MusicLocation != nil {
		s.data.Preferences.MusicLocation = clonePreferences(patch).MusicLocation
	}
	if patch.CinemaLocation != nil {
		s.data.Preferences.CinemaLocation = clonePreferences(patch).CinemaLocation
	}
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}
