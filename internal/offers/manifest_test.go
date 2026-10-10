package offers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func validManifest() Manifest {
	digest := strings.Repeat("a", 64)
	return Manifest{
		SchemaVersion: 1,
		UpdatedAt:     time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		Items: []Item{{
			ID: "dmit", Vendor: "DMIT", Alt: "美国 CN2 GIA 入门款", Featured: true,
			Images: Images{
				Card: Image{Path: "offers/dmit-card.webp", SHA256: digest},
				Wide: &Image{Path: "offers/dmit-wide.webp", SHA256: strings.Repeat("b", 64)},
			},
			URL: "https://www.dmit.io/aff.php?aff=4966&pid=100",
		}},
	}
}

func TestDecodeManifestAcceptsPublishedShape(t *testing.T) {
	data, err := json.Marshal(validManifest())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeManifest(data)
	if err != nil || len(manifest.Items) != 1 || manifest.Items[0].Images.Wide == nil {
		t.Fatalf("DecodeManifest = %+v, %v", manifest, err)
	}
}

func TestDecodeManifestRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	data, _ := json.Marshal(validManifest())
	for name, content := range map[string]string{
		"unknown field": strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"script":"x"`, 1),
		"trailing data": string(data) + `{}`,
		"empty":         "",
		"too large":     `{"schemaVersion":1,"pad":"` + strings.Repeat("x", MaxManifestBytes) + `"}`,
	} {
		if _, err := DecodeManifest([]byte(content)); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
}

func TestValidateManifestBounds(t *testing.T) {
	cases := map[string]func(*Manifest){
		"schema":    func(m *Manifest) { m.SchemaVersion = 2 },
		"updatedAt": func(m *Manifest) { m.UpdatedAt = time.Time{} },
		"nil items": func(m *Manifest) { m.Items = nil },
		"id":        func(m *Manifest) { m.Items[0].ID = "DMIT" },
		"duplicate id": func(m *Manifest) {
			m.Items = append(m.Items, m.Items[0])
			m.Items[1].Featured = false
			m.Items[1].Images.Wide = nil
		},
		"empty vendor":        func(m *Manifest) { m.Items[0].Vendor = "" },
		"long vendor":         func(m *Manifest) { m.Items[0].Vendor = strings.Repeat("云", maxVendorRune+1) },
		"padded alt":          func(m *Manifest) { m.Items[0].Alt = " 文案" },
		"control alt":         func(m *Manifest) { m.Items[0].Alt = "文案\n换行" },
		"http url":            func(m *Manifest) { m.Items[0].URL = "http://www.dmit.io/" },
		"javascript url":      func(m *Manifest) { m.Items[0].URL = "javascript:alert(1)" },
		"credentials url":     func(m *Manifest) { m.Items[0].URL = "https://user:pass@www.dmit.io/" },
		"space url":           func(m *Manifest) { m.Items[0].URL = "https://www.dmit.io/a b" },
		"featured no wide":    func(m *Manifest) { m.Items[0].Images.Wide = nil },
		"wide not featured":   func(m *Manifest) { m.Items[0].Featured = false },
		"path traversal":      func(m *Manifest) { m.Items[0].Images.Card.Path = "offers/../index.html" },
		"path outside offers": func(m *Manifest) { m.Items[0].Images.Card.Path = "icons/dmit.webp" },
		"gif":                 func(m *Manifest) { m.Items[0].Images.Card.Path = "offers/dmit.gif" },
		"digest":              func(m *Manifest) { m.Items[0].Images.Card.SHA256 = strings.Repeat("A", 64) },
		"path two digests":    func(m *Manifest) { m.Items[0].Images.Wide.Path = m.Items[0].Images.Card.Path },
		"window":              func(m *Manifest) { now := time.Now(); m.Items[0].ValidFrom, m.Items[0].ValidUntil = &now, &now },
		"too many featured": func(m *Manifest) {
			for index := range MaxFeatured {
				item := m.Items[0]
				item.ID = "extra-" + string(rune('a'+index))
				m.Items = append(m.Items, item)
			}
		},
		"too many items": func(m *Manifest) {
			for index := range MaxItems {
				item := m.Items[0]
				item.ID = "wall-" + strings.Repeat("x", index+1)
				item.Featured, item.Images.Wide = false, nil
				m.Items = append(m.Items, item)
			}
		},
	}
	for name, mutate := range cases {
		manifest := validManifest()
		mutate(&manifest)
		if err := ValidateManifest(manifest); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
}

func TestValidateImageChecksDigestFormatAndExactSize(t *testing.T) {
	card := pngBanner(t, CardWidth, CardHeight, 10)
	pinned := Image{Path: "offers/card.png", SHA256: digestOf(card)}
	if contentType, err := validateImage(card, pinned, cardImage); err != nil || contentType != "image/png" {
		t.Fatalf("valid card = %q, %v", contentType, err)
	}
	if _, err := validateImage(card, pinned, wideImage); !errors.Is(err, ErrImageInvalid) {
		t.Fatalf("card accepted as wide: %v", err)
	}
	if _, err := validateImage(card, Image{Path: "offers/card.webp", SHA256: digestOf(card)}, cardImage); !errors.Is(err, ErrImageInvalid) {
		t.Fatalf("png accepted under a webp path: %v", err)
	}
	if _, err := validateImage(card, Image{Path: "offers/card.png", SHA256: strings.Repeat("0", 64)}, cardImage); !errors.Is(err, ErrImageInvalid) {
		t.Fatalf("digest mismatch accepted: %v", err)
	}
	truncated := card[:len(card)-20]
	if _, err := validateImage(truncated, Image{Path: "offers/card.png", SHA256: digestOf(truncated)}, cardImage); !errors.Is(err, ErrImageInvalid) {
		t.Fatalf("truncated image accepted: %v", err)
	}
}
