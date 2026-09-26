package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppearancePersistsAndReturnsIndependentSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	storage, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, initialVersion := storage.Appearance()
	value := Appearance{Theme: "dark", Wallpaper: "prism", ClassicLevel: "ambient", Colors: &AppearanceColors{
		Brand: "#7856b6", Neutral: "#54475f", Signature: "#c76586",
	}}
	if err := storage.ReplaceAppearance(initialVersion, value); err != nil {
		t.Fatal(err)
	}
	if err := storage.ReplaceAppearance(initialVersion, value); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
	value.Colors.Brand = "#000000"
	saved, version := storage.Appearance()
	if saved.Colors.Brand != "#7856b6" || version == initialVersion {
		t.Fatalf("saved = %#v version=%s", saved, version)
	}
	saved.Colors.Brand = "#ffffff"
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, restoredVersion := reopened.Appearance()
	if restored.Colors.Brand != "#7856b6" || restoredVersion != version {
		t.Fatalf("restored = %#v version=%s", restored, restoredVersion)
	}
}

func TestAppearanceIncludedInPanelBackup(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	now := time.Now().UTC()
	if err := source.CreateInitialAdmin(User{ID: "admin", Username: "admin", PasswordHash: strings.Repeat("h", 32), Role: "admin", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	_, version := source.Appearance()
	choice := Appearance{Theme: "light", Wallpaper: "horizon", ClassicLevel: "clear"}
	if err := source.ReplaceAppearance(version, choice); err != nil {
		t.Fatal(err)
	}
	data, err := source.ExportIdentity()
	if err != nil || ValidateIdentityBackup(data) != nil {
		t.Fatalf("exported appearance backup: %v", err)
	}
	destination, err := Open(filepath.Join(t.TempDir(), "destination.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := destination.RestoreIdentity(data); err != nil {
		t.Fatal(err)
	}
	restored, _ := destination.Appearance()
	if restored == nil || *restored != choice {
		t.Fatalf("restored appearance = %#v", restored)
	}
}
