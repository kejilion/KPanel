package store

import (
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sampleDesktop() *DesktopPreferences {
	return &DesktopPreferences{Mode: "desktop", SideSplitRatio: .6, FocusedID: 3,
		Windows:     []DesktopWindow{{ID: 3, Path: "/files?path=%2Fhome", Geometry: DesktopGeometry{Left: 10, Top: 20, Width: 900, Height: 600}, Z: 101}},
		WindowSizes: []DesktopWindowSize{{Path: "/files", Width: 900, Height: 600}},
	}
}

func TestMediaLocationPreferencesPersistIndependently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, version := s.Preferences()
	gallery := &MediaLocation{Path: "/srv/photos"}
	if err = s.PatchPreferences(version, Preferences{GalleryLocation: gallery}); err != nil {
		t.Fatal(err)
	}
	gallery.Path = "/changed"
	_, version = s.Preferences()
	if err = s.PatchPreferences(version, Preferences{MusicLocation: &MediaLocation{Path: "/", HostID: "edge-1"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.PatchPreferences(version, Preferences{CinemaLocation: &MediaLocation{Path: "/films"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write: %v", err)
	}
	got, version := s.Preferences()
	if got.GalleryLocation.Path != "/srv/photos" || got.MusicLocation.Path != "/" {
		t.Fatalf("lost sections: %+v", got)
	}
	got.GalleryLocation.Path = "/alias"
	if err = s.PatchPreferences(version, Preferences{CinemaLocation: &MediaLocation{Path: "/films"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateInitialAdmin(User{ID: "admin", Username: "admin", PasswordHash: strings.Repeat("h", 32), Role: "admin", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportIdentity()
	if err != nil || ValidateIdentityBackup(backup) != nil {
		t.Fatalf("backup: %v", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, _ = s.Preferences()
	if got.GalleryLocation.Path != "/srv/photos" || got.MusicLocation.HostID != "edge-1" || got.CinemaLocation.Path != "/films" {
		t.Fatalf("restart lost locations: %+v", got)
	}
	destination, err := Open(filepath.Join(t.TempDir(), "restore.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err = destination.RestoreIdentity(backup); err != nil {
		t.Fatal(err)
	}
	restored, _ := destination.Preferences()
	if !reflect.DeepEqual(restored, got) {
		t.Fatal("backup lost media locations")
	}
}

func TestMediaLocationPreferencesRejectInvalid(t *testing.T) {
	for _, path := range []string{"", "relative", "/a/../b", "/a//b", "/a/", "/a\\b", "/bad\nname", "/bad\x7fname", "/a ", "/" + strings.Repeat("x", 1025)} {
		if ValidatePreferences(Preferences{MusicLocation: &MediaLocation{Path: path}}) == nil {
			t.Errorf("accepted %q", path)
		}
	}
	for _, p := range []Preferences{
		{GalleryLocation: &MediaLocation{Path: "/"}},
		{GalleryLocation: &MediaLocation{Path: "/photos", HostID: "remote"}},
		{CinemaLocation: &MediaLocation{Path: "/films", HostID: "bad/host"}},
	} {
		if ValidatePreferences(p) == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}

func TestMediaLocationMatchesBrowserUnicodeBounds(t *testing.T) {
	for _, path := range []string{"/music\u0085", "/" + strings.Repeat("😀", 511)} {
		if err := ValidatePreferences(Preferences{MusicLocation: &MediaLocation{Path: path}}); err != nil {
			t.Errorf("rejected browser-valid %q: %v", path, err)
		}
	}
	for _, path := range []string{"/music\ufeff", "/" + strings.Repeat("😀", 512)} {
		if ValidatePreferences(Preferences{MusicLocation: &MediaLocation{Path: path}}) == nil {
			t.Errorf("accepted browser-invalid %q", path)
		}
	}
}

func TestBrowserPreferencesValidatePersistAndKeepOtherSections(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, version := s.Preferences()
	if err = s.PatchPreferences(version, Preferences{Locale: "zh-CN", Desktop: sampleDesktop()}); err != nil {
		t.Fatal(err)
	}
	_, version = s.Preferences()
	if err = s.PatchPreferences(version, Preferences{Browser: &BrowserPreferences{StartURL: "https://example.com/start"}}); err != nil {
		t.Fatal(err)
	}
	v, version := s.Preferences()
	if v.Locale != "zh-CN" || v.Desktop == nil || v.Browser.StartURL != "https://example.com/start" {
		t.Fatal(v)
	}
	v.Browser.StartURL = "https://changed.test"
	v, _ = s.Preferences()
	if v.Browser.StartURL != "https://example.com/start" {
		t.Fatal("preference aliases store")
	}
	for _, url := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://user:secret@example.com", "//example.com", "https://example.com/\n", "https://"} {
		if ValidatePreferences(Preferences{Browser: &BrowserPreferences{StartURL: url}}) == nil {
			t.Fatal("invalid URL accepted", url)
		}
	}
	if err = s.PatchPreferences(version, Preferences{Browser: &BrowserPreferences{}}); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Preferences()
	if v.Browser.StartURL != "" || v.Desktop == nil {
		t.Fatal("could not restore built-in page")
	}
}

func TestPreferencesPersistenceConflictRollbackAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, initial := s.Preferences()
	value := Preferences{Locale: "zh-TW", Desktop: sampleDesktop()}
	if err := s.PatchPreferences(initial, value); err != nil {
		t.Fatal(err)
	}
	if err := s.PatchPreferences(initial, Preferences{Locale: "en-US"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write: %v", err)
	}
	value.Desktop.Windows[0].Path = "/settings"
	got, version := s.Preferences()
	if got.Desktop.Windows[0].Path != "/files?path=%2Fhome" {
		t.Fatal("input aliases store")
	}
	if err := s.PatchPreferences(version, Preferences{Locale: "en-US"}); err != nil {
		t.Fatal(err)
	}
	got, version = s.Preferences()
	if got.Locale != "en-US" || got.Desktop.FocusedID != 3 {
		t.Fatal("locale patch erased desktop")
	}
	got.Desktop.Windows[0].Path = "/settings"
	got, _ = s.Preferences()
	if got.Desktop.Windows[0].Path == "/settings" {
		t.Fatal("snapshot aliases store")
	}
	// Force atomic replacement to fail and verify the in-memory snapshot rolls back.
	originalPath := s.path
	s.path = filepath.Join(t.TempDir(), "missing", "panel.json")
	if err := s.PatchPreferences(version, Preferences{Locale: "zh-CN"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	s.path = originalPath
	after, afterVersion := s.Preferences()
	if !reflect.DeepEqual(after, got) || afterVersion != version {
		t.Fatal("failed save changed state")
	}
	if err := s.CreateInitialAdmin(User{ID: "admin", Username: "admin", PasswordHash: strings.Repeat("h", 32), Role: "admin", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportIdentity()
	if err != nil || ValidateIdentityBackup(backup) != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, restoredVersion := s.Preferences()
	if !reflect.DeepEqual(restored, got) || restoredVersion != version {
		t.Fatal("restart lost preferences")
	}
	destination, err := Open(filepath.Join(t.TempDir(), "restore.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := destination.RestoreIdentity(backup); err != nil {
		t.Fatal(err)
	}
	restored, _ = destination.Preferences()
	if !reflect.DeepEqual(restored, got) {
		t.Fatal("backup lost preferences")
	}
}

func TestPreferencesRejectInvalidAndUnboundedState(t *testing.T) {
	cases := map[string]func(*Preferences){
		"locale":     func(p *Preferences) { p.Locale = "xx" },
		"windows":    func(p *Preferences) { p.Desktop.Windows = make([]DesktopWindow, 9) },
		"sizes":      func(p *Preferences) { p.Desktop.WindowSizes = make([]DesktopWindowSize, 33) },
		"external":   func(p *Preferences) { p.Desktop.Windows[0].Path = "https://example.com" },
		"auth route": func(p *Preferences) { p.Desktop.Windows[0].Path = "/login?password=abc" },
		"duplicate":  func(p *Preferences) { p.Desktop.Windows = append(p.Desktop.Windows, p.Desktop.Windows[0]) },
		"focus":      func(p *Preferences) { p.Desktop.FocusedID = 999 },
		"geometry":   func(p *Preferences) { p.Desktop.Windows[0].Geometry.Width = math.Inf(1) },
		"ratio":      func(p *Preferences) { p.Desktop.SideSplitRatio = math.NaN() },
		"size query": func(p *Preferences) { p.Desktop.WindowSizes[0].Path = "/files?path=/" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := Preferences{Locale: "en-US", Desktop: sampleDesktop()}
			mutate(&p)
			if ValidatePreferences(p) == nil {
				t.Fatal("accepted invalid preferences")
			}
		})
	}
}

// Desktop routes accepted by the shipped catalogue must round-trip as UI state.
func TestPreferencesAcceptCatalogueWindowRoutes(t *testing.T) {
	for _, path := range []string{"/virtual-machines", "/virtual-machines?tab=instances", "/sites/monitor", "/browser", "/browser?tab=history"} {
		p := Preferences{Desktop: sampleDesktop()}
		p.Desktop.Windows[0].Path = path
		p.Desktop.WindowSizes[0].Path = strings.SplitN(path, "?", 2)[0]
		if err := ValidatePreferences(p); err != nil {
			t.Errorf("catalogue route %q: %v", path, err)
		}
	}
	for _, path := range []string{"/virtual-machines/../settings", "/virtual-machines-extra", "/sites/monitor/other", "//virtual-machines", "/browser/child", "/browser-extra", "/browser/../settings", "//browser"} {
		p := Preferences{Desktop: sampleDesktop()}
		p.Desktop.Windows[0].Path = path
		if ValidatePreferences(p) == nil {
			t.Errorf("accepted non-catalogue route %q", path)
		}
	}
}
