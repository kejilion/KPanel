// Package offers keeps the read-only 广告专栏 (sponsored offers) manifest that is
// published next to the application catalogue on app.kejilion.sh. The Panel
// only downloads a bounded JSON manifest and the banner images it pins by
// SHA-256 from that single origin; nothing here writes to the host, runs
// commands or accepts user input beyond a cached image digest.
package offers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ManifestURL is the only manifest source. Images are resolved against
	// the same origin and must stay below offers/.
	ManifestURL  = "https://app.kejilion.sh/offers/v1.json"
	originScheme = "https"
	originHost   = "app.kejilion.sh"

	SchemaVersion    = 1
	MaxManifestBytes = 64 << 10
	MaxItems         = 24
	MaxFeatured      = 3

	CardWidth     = 960
	CardHeight    = 480
	WideWidth     = 1600
	WideHeight    = 400
	MaxCardBytes  = 200 << 10
	MaxWideBytes  = 300 << 10
	maxVendorRune = 32
	maxAltRune    = 80
	maxURLBytes   = 512
)

var (
	itemIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	imagePathPattern = regexp.MustCompile(`^offers/[a-z0-9][a-z0-9_-]{0,63}[.](webp|png|jpg)$`)
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)

	ErrManifestInvalid = errors.New("offers manifest is invalid")
)

// Image pins one banner file by its path below the official origin and the
// exact SHA-256 of its bytes, so a changed file is never shown unannounced.
type Image struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Images holds the required 2:1 wall banner and, for featured items, the 4:1
// carousel banner.
type Images struct {
	Card Image  `json:"card"`
	Wide *Image `json:"wide,omitempty"`
}

type Item struct {
	ID         string     `json:"id"`
	Vendor     string     `json:"vendor"`
	Alt        string     `json:"alt"`
	Featured   bool       `json:"featured,omitempty"`
	Images     Images     `json:"images"`
	URL        string     `json:"url"`
	ValidFrom  *time.Time `json:"validFrom,omitempty"`
	ValidUntil *time.Time `json:"validUntil,omitempty"`
}

type Manifest struct {
	SchemaVersion int       `json:"schemaVersion"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Items         []Item    `json:"items"`
}

// DecodeManifest strictly parses and validates one manifest document.
func DecodeManifest(content []byte) (Manifest, error) {
	if len(content) == 0 || len(content) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: size is outside 1..%d bytes", ErrManifestInvalid, MaxManifestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, fmt.Errorf("%w: trailing data", ErrManifestInvalid)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ValidateManifest checks every bound the page and the image cache rely on.
func ValidateManifest(manifest Manifest) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrManifestInvalid, fmt.Sprintf(format, args...))
	}
	if manifest.SchemaVersion != SchemaVersion {
		return invalid("schemaVersion must be %d", SchemaVersion)
	}
	if manifest.UpdatedAt.IsZero() {
		return invalid("updatedAt is required")
	}
	if manifest.Items == nil || len(manifest.Items) > MaxItems {
		return invalid("items must be a list of at most %d entries", MaxItems)
	}
	ids := make(map[string]bool, len(manifest.Items))
	paths := make(map[string]string, len(manifest.Items)*2)
	featured := 0
	for index, item := range manifest.Items {
		if !itemIDPattern.MatchString(item.ID) || ids[item.ID] {
			return invalid("item %d has an invalid or duplicated id", index)
		}
		ids[item.ID] = true
		if !boundedText(item.Vendor, maxVendorRune) || !boundedText(item.Alt, maxAltRune) {
			return invalid("item %q needs a vendor of 1..%d and alt text of 1..%d characters", item.ID, maxVendorRune, maxAltRune)
		}
		if _, err := ValidTargetURL(item.URL); err != nil {
			return invalid("item %q has an invalid url", item.ID)
		}
		if item.ValidFrom != nil && item.ValidUntil != nil && !item.ValidUntil.After(*item.ValidFrom) {
			return invalid("item %q validUntil must be after validFrom", item.ID)
		}
		images := []Image{item.Images.Card}
		if item.Featured {
			featured++
			if item.Images.Wide == nil {
				return invalid("featured item %q needs a wide image", item.ID)
			}
		}
		if item.Images.Wide != nil {
			if !item.Featured {
				return invalid("item %q has a wide image but is not featured", item.ID)
			}
			images = append(images, *item.Images.Wide)
		}
		for _, image := range images {
			if !imagePathPattern.MatchString(image.Path) || !digestPattern.MatchString(image.SHA256) {
				return invalid("item %q has an invalid image path or sha256", item.ID)
			}
			// One path always means one file; reusing a digest under two
			// paths is fine, reusing a path for two digests is not.
			if digest, ok := paths[image.Path]; ok && digest != image.SHA256 {
				return invalid("image path %q is pinned to two digests", image.Path)
			}
			paths[image.Path] = image.SHA256
		}
	}
	if featured > MaxFeatured {
		return invalid("at most %d items may be featured", MaxFeatured)
	}
	return nil
}

// ValidTargetURL accepts only absolute https links without credentials.
func ValidTargetURL(raw string) (*url.URL, error) {
	if raw == "" || len(raw) > maxURLBytes || strings.TrimSpace(raw) != raw || !utf8.ValidString(raw) {
		return nil, ErrManifestInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Opaque != "" {
		return nil, ErrManifestInvalid
	}
	for _, character := range raw {
		if character < 0x21 || character == 0x7f {
			return nil, ErrManifestInvalid
		}
	}
	return parsed, nil
}

// Active reports whether the item is inside its publishing window at now.
func (item Item) Active(now time.Time) bool {
	if item.ValidFrom != nil && now.Before(*item.ValidFrom) {
		return false
	}
	return item.ValidUntil == nil || now.Before(*item.ValidUntil)
}

func boundedText(value string, maxRunes int) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	count := utf8.RuneCountInString(value)
	if count < 1 || count > maxRunes {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func imageURL(path string) string {
	return (&url.URL{Scheme: originScheme, Host: originHost, Path: "/" + path}).String()
}
