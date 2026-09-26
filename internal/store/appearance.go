package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
)

var appearanceWallpaperPattern = regexp.MustCompile(`^(classic|orbit|horizon|rift|prism|pack:[a-z0-9][a-z0-9-]{0,39}|custom:[0-9a-f]{32})$`)
var appearanceColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func ValidateAppearance(value Appearance) error {
	if value.Theme != "system" && value.Theme != "light" && value.Theme != "dark" {
		return ErrInvalidRecord
	}
	if !appearanceWallpaperPattern.MatchString(value.Wallpaper) {
		return ErrInvalidRecord
	}
	if value.ClassicLevel != "off" && value.ClassicLevel != "ambient" && value.ClassicLevel != "clear" {
		return ErrInvalidRecord
	}
	if value.Colors != nil && (!appearanceColorPattern.MatchString(value.Colors.Brand) ||
		!appearanceColorPattern.MatchString(value.Colors.Neutral) ||
		!appearanceColorPattern.MatchString(value.Colors.Signature)) {
		return ErrInvalidRecord
	}
	return nil
}

func cloneAppearance(value *Appearance) *Appearance {
	if value == nil {
		return nil
	}
	copy := *value
	if value.Colors != nil {
		colors := *value.Colors
		copy.Colors = &colors
	}
	return &copy
}

func appearanceVersion(value *Appearance) string {
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func (s *Store) Appearance() (*Appearance, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAppearance(s.data.Appearance), appearanceVersion(s.data.Appearance)
}

func (s *Store) ReplaceAppearance(expectedVersion string, value Appearance) error {
	if err := ValidateAppearance(value); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedVersion != appearanceVersion(s.data.Appearance) {
		return ErrConflict
	}
	previous := cloneDiskState(s.data)
	s.data.Appearance = cloneAppearance(&value)
	if err := s.persistLocked(); err != nil {
		s.data = previous
		return err
	}
	return nil
}
