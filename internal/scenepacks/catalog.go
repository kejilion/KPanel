// Package scenepacks manages optional, untrusted desktop artwork in Panel data.
// It never executes a pack or delegates downloads to the privileged Agent.
package scenepacks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxPacks              = 100
	MaxInstalled          = 10
	MaxFiles              = 200
	MaxPackBytes    int64 = 30 << 20
	MaxFileBytes    int64 = 16 << 20
	MaxTotalBytes   int64 = 150 << 20
	MaxCatalogBytes int64 = 4 << 20
	MaxStateBytes   int64 = 2 << 20
	FilePrefix            = "/api/v1/desktop/scene-packs/"
)

var (
	ErrInvalid     = errors.New("invalid scene pack")
	ErrUnavailable = errors.New("scene pack storage unavailable")
	ErrSource      = errors.New("scene pack source unavailable")
	ErrNotFound    = errors.New("scene pack not found")
	ErrConflict    = errors.New("scene pack changed; refresh and retry")
	ErrBusy        = errors.New("scene pack operation in progress")
	ErrQuota       = errors.New("scene pack storage quota exceeded")
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	versionPattern = regexp.MustCompile(`^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}(?:-[0-9A-Za-z.-]{1,40})?$`)
	filePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,79}$`)
	digestPattern  = regexp.MustCompile(`^[a-f0-9]{64}$`)
	tokenPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
	colorPattern   = regexp.MustCompile(`^#[a-fA-F0-9]{6}$`)
)

type Text map[string]string
type Camera struct {
	ID   string `json:"id"`
	Name Text   `json:"name"`
}
type Theme struct {
	Brand     string `json:"brand"`
	Neutral   string `json:"neutral"`
	Signature string `json:"signature"`
}
type Author struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Pack struct {
	Schema      int      `json:"schema"`
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Runtime     string   `json:"runtime"`
	Name        Text     `json:"name"`
	Description Text     `json:"description"`
	Author      Author   `json:"author"`
	License     string   `json:"license"`
	Tags        []string `json:"tags"`
	Theme       Theme    `json:"theme"`
	Cameras     []Camera `json:"cameras"`
	Entry       string   `json:"entry"`
	Poster      string   `json:"poster"`
	Thumb       string   `json:"thumb"`
	Path        string   `json:"path"`
	Files       []File   `json:"files"`
	SizeBytes   int64    `json:"sizeBytes"`
}
type Catalog struct {
	Schema int    `json:"schema"`
	Packs  []Pack `json:"packs"`
}
type View struct {
	ID               string   `json:"id"`
	Version          string   `json:"version"`
	Name             Text     `json:"name"`
	Description      Text     `json:"description"`
	Author           Author   `json:"author"`
	License          string   `json:"license"`
	Tags             []string `json:"tags"`
	Theme            Theme    `json:"theme"`
	Cameras          []Camera `json:"cameras"`
	SizeBytes        int64    `json:"sizeBytes"`
	Installed        bool     `json:"installed"`
	InstalledVersion *string  `json:"installedVersion"`
	FileBase         *string  `json:"fileBase"`
	ResourceVersion  string   `json:"resourceVersion"`
}
type List struct {
	Source  string   `json:"source"`
	Sources []string `json:"sources"`
	Packs   []View   `json:"packs"`
	Warning string   `json:"warning,omitempty"`
}

func ValidID(id string) bool       { return idPattern.MatchString(id) }
func ValidToken(token string) bool { return tokenPattern.MatchString(token) }
func ValidSource(source string) bool {
	return source == "auto" || source == "github" || source == "mirror"
}
func contentDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func ContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".avif":
		return "image/avif"
	case ".gltf":
		return "model/gltf+json"
	case ".glb":
		return "model/gltf-binary"
	case ".ktx2":
		return "image/ktx2"
	case ".wasm":
		return "application/wasm"
	case ".woff2":
		return "font/woff2"
	case ".webm":
		return "video/webm"
	case ".mp4":
		return "video/mp4"
	case ".txt", ".md":
		return "text/plain; charset=utf-8"
	case ".bin", ".hdr", ".exr", ".basis":
		return "application/octet-stream"
	}
	return ""
}

func ValidPath(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) > 4 || len(name) > 240 || ContentType(name) == "" {
		return false
	}
	for _, part := range parts {
		if !filePattern.MatchString(part) || strings.Contains(part, "..") {
			return false
		}
	}
	return true
}

func validText(text Text, chinese, english int) bool {
	if len(text) < 2 || len(text) > 3 || text["zh-CN"] == "" || text["en-US"] == "" {
		return false
	}
	for language, value := range text {
		limit := chinese
		if language == "en-US" {
			limit = english
		} else if language != "zh-CN" && language != "zh-TW" {
			return false
		}
		if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > limit {
			return false
		}
	}
	return true
}

func validatePack(p Pack) error {
	if p.Schema != 1 || !ValidID(p.ID) || !versionPattern.MatchString(p.Version) || p.Runtime != "kpanel-scene-pack@1" || p.Path != p.ID+"/dist" ||
		!validText(p.Name, 20, 40) || !validText(p.Description, 40, 90) || p.Author.Name == "" || len(p.Author.Name) > 160 || len(p.Author.URL) > 500 || len(p.License) > 160 || p.License == "" ||
		!colorPattern.MatchString(p.Theme.Brand) || !colorPattern.MatchString(p.Theme.Neutral) || !colorPattern.MatchString(p.Theme.Signature) ||
		p.Entry != "index.html" || p.Poster != "poster.webp" || p.Thumb != "thumb.webp" || len(p.Cameras) < 1 || len(p.Cameras) > 6 || len(p.Tags) > 6 || len(p.Files) < 4 || len(p.Files) > MaxFiles {
		return ErrInvalid
	}
	seenCameras := map[string]bool{}
	for _, camera := range p.Cameras {
		if !ValidID(camera.ID) || seenCameras[camera.ID] || !validText(camera.Name, 12, 30) {
			return ErrInvalid
		}
		seenCameras[camera.ID] = true
	}
	for _, tag := range p.Tags {
		if !ValidID(tag) || len(tag) > 20 {
			return ErrInvalid
		}
	}
	files := map[string]bool{}
	var total int64
	for _, file := range p.Files {
		limit := MaxFileBytes
		switch file.Path {
		case "thumb.webp":
			limit = 40 << 10
		case "poster.webp":
			limit = 200 << 10
		case "manifest.json", "index.html":
			limit = 64 << 10
		case "preview.webm":
			limit = 3 << 20
		}
		if !ValidPath(file.Path) || files[file.Path] || file.Size < 1 || file.Size > limit || !digestPattern.MatchString(file.SHA256) {
			return ErrInvalid
		}
		files[file.Path] = true
		total += file.Size
	}
	for name := range files {
		for prefix := path.Dir(name); prefix != "."; prefix = path.Dir(prefix) {
			if files[prefix] {
				return ErrInvalid
			}
		}
	}
	if total > MaxPackBytes || total != p.SizeBytes || !files["index.html"] || !files["manifest.json"] || !files["poster.webp"] || !files["thumb.webp"] {
		return ErrInvalid
	}
	return nil
}

func DecodeCatalog(data []byte) (Catalog, error) {
	var result Catalog
	if int64(len(data)) > MaxCatalogBytes || json.Unmarshal(data, &result) != nil || result.Schema != 1 || result.Packs == nil || len(result.Packs) > MaxPacks {
		return Catalog{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range result.Packs {
		if seen[p.ID] || validatePack(p) != nil {
			return Catalog{}, ErrInvalid
		}
		seen[p.ID] = true
	}
	return result, nil
}
