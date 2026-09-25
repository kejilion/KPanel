// Package desktopwallpapers stores the wallpapers an administrator uploads for
// the desktop and classic modes. Images are prepared in the browser (scaled and
// re-encoded), then checked again here: only still WebP or JPEG is accepted,
// metadata such as EXIF location is removed, every image is fully decoded, and
// the collection is bounded in count and bytes. Wallpapers are panel data, not
// host data, and are deliberately not part of panel backups (see backup_data.go).
package desktopwallpapers

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/webp"
)

const (
	MaxWallpapers     = 12
	MaxImageBytes     = 8 << 20
	MaxThumbBytes     = 256 << 10
	MaxTotalBytes     = 48 << 20
	MaxMetadataBytes  = 4 << 10
	MaxDimension      = 4096
	MaxPixels         = 10_000_000
	MinDimension      = 64
	MaxThumbDimension = 640
	MinThumbDimension = 16
	MaxNameRunes      = 40
	FocusScale        = 1000
)

var (
	ErrUnavailable = errors.New("desktop wallpapers unavailable")
	ErrNotFound    = errors.New("desktop wallpaper not found")
	ErrInvalid     = errors.New("desktop wallpaper image is invalid")
	ErrQuota       = errors.New("desktop wallpaper quota exceeded")
)

// ValidationError reports a metadata field the client can correct.
type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Detail }

var (
	idPattern    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)
)

// ValidID reports whether id has the shape of a stored wallpaper ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Theme is the color scheme suggested from the image, applied when the wallpaper is chosen.
type Theme struct {
	Brand     string `json:"brand"`
	Neutral   string `json:"neutral"`
	Signature string `json:"signature"`
}

// Metadata is what the client sends with an upload.
type Metadata struct {
	Name   string `json:"name"`
	FocusX int    `json:"focusX"`
	FocusY int    `json:"focusY"`
	Theme  *Theme `json:"theme,omitempty"`
}

// Wallpaper describes one stored wallpaper. Its images never change after upload.
type Wallpaper struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Format      string    `json:"format"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	ImageBytes  int64     `json:"imageBytes"`
	ThumbBytes  int64     `json:"thumbBytes"`
	FocusX      int       `json:"focusX"`
	FocusY      int       `json:"focusY"`
	Luminance   int       `json:"luminance"`
	Theme       *Theme    `json:"theme,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	ImageDigest string    `json:"imageDigest"`
}

// Usage is the collection's size against its limits.
type Usage struct {
	Count    int   `json:"count"`
	Bytes    int64 `json:"bytes"`
	MaxCount int   `json:"maxCount"`
	MaxBytes int64 `json:"maxBytes"`
}

type persistedIndex struct {
	Version    int         `json:"version"`
	Wallpapers []Wallpaper `json:"wallpapers"`
}

// Store keeps the index and image files under one private directory.
type Store struct {
	mu         sync.Mutex
	root       string
	filesDir   string
	wallpapers []Wallpaper
}

// Open loads the index, drops entries whose files are missing or invalid, and removes stray files.
func Open(root string) (*Store, error) {
	filesDir := filepath.Join(root, "files")
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, fmt.Errorf("initialize desktop wallpaper directory: %w", err)
	}
	if err := ensurePrivateDirectory(filesDir); err != nil {
		return nil, fmt.Errorf("initialize desktop wallpaper files: %w", err)
	}
	store := &Store{root: root, filesDir: filesDir}
	index, err := store.readIndex()
	if err != nil {
		return nil, err
	}
	kept := make([]Wallpaper, 0, len(index.Wallpapers))
	seen := map[string]bool{}
	for _, wallpaper := range index.Wallpapers {
		if len(kept) >= MaxWallpapers || seen[wallpaper.ID] || validateStored(wallpaper) != nil {
			continue
		}
		if !store.filesPresent(wallpaper) {
			continue
		}
		seen[wallpaper.ID] = true
		kept = append(kept, wallpaper)
	}
	store.wallpapers = kept
	if len(kept) != len(index.Wallpapers) {
		if err := store.writeIndexLocked(kept); err != nil {
			return nil, fmt.Errorf("repair desktop wallpaper index: %w", err)
		}
	}
	if err := store.removeStrayFilesLocked(); err != nil {
		return nil, fmt.Errorf("inspect desktop wallpaper files: %w", err)
	}
	return store, nil
}

// List returns the wallpapers, newest first, and the collection's usage.
func (s *Store) List() ([]Wallpaper, Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := slices.Clone(s.wallpapers)
	slices.Reverse(list)
	return list, s.usageLocked()
}

// Prepared is a checked upload, ready to be committed with Add.
type Prepared struct {
	meta      Metadata
	format    string
	image     []byte
	thumb     []byte
	width     int
	height    int
	luminance int
}

// Prepare validates the metadata and both images outside the store lock. Decoding is
// the expensive part; callers bound how many uploads prepare at once.
func Prepare(meta Metadata, imageData, thumbData []byte) (*Prepared, error) {
	meta, err := normalizeMetadata(meta)
	if err != nil {
		return nil, err
	}
	format, cleanImage, err := sanitize(imageData, MaxImageBytes)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeChecked(format, cleanImage, MinDimension, MaxDimension)
	if err != nil {
		return nil, err
	}
	thumbFormat, cleanThumb, err := sanitize(thumbData, MaxThumbBytes)
	if err != nil {
		return nil, err
	}
	thumb, err := decodeChecked(thumbFormat, cleanThumb, MinThumbDimension, MaxThumbDimension)
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	return &Prepared{
		meta: meta, format: format, image: cleanImage, thumb: cleanThumb,
		width: bounds.Dx(), height: bounds.Dy(), luminance: averageLuminance(thumb),
	}, nil
}

// Add stores a prepared upload. The index write is the commit point.
func (s *Store) Add(prepared *Prepared) (Wallpaper, error) {
	if prepared == nil {
		return Wallpaper{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := s.usageLocked()
	size := int64(len(prepared.image) + len(prepared.thumb))
	if usage.Count >= MaxWallpapers || usage.Bytes+size > MaxTotalBytes {
		return Wallpaper{}, ErrQuota
	}
	id, err := newID()
	if err != nil {
		return Wallpaper{}, err
	}
	digest := sha256Hex(prepared.image)
	wallpaper := Wallpaper{
		ID: id, Name: prepared.meta.Name, Format: prepared.format,
		Width: prepared.width, Height: prepared.height,
		ImageBytes: int64(len(prepared.image)), ThumbBytes: int64(len(prepared.thumb)),
		FocusX: prepared.meta.FocusX, FocusY: prepared.meta.FocusY,
		Luminance: prepared.luminance, Theme: prepared.meta.Theme,
		CreatedAt: time.Now().UTC().Truncate(time.Second), ImageDigest: digest,
	}
	imagePath, thumbPath := s.paths(id)
	if err := writeAtomicPrivateFile(s.filesDir, imagePath, prepared.image); err != nil {
		return Wallpaper{}, fmt.Errorf("persist desktop wallpaper: %w", err)
	}
	if err := writeAtomicPrivateFile(s.filesDir, thumbPath, prepared.thumb); err != nil {
		_ = os.Remove(imagePath)
		return Wallpaper{}, fmt.Errorf("persist desktop wallpaper thumbnail: %w", err)
	}
	next := append(slices.Clone(s.wallpapers), wallpaper)
	if err := s.writeIndexLocked(next); err != nil {
		_ = os.Remove(imagePath)
		_ = os.Remove(thumbPath)
		return Wallpaper{}, fmt.Errorf("persist desktop wallpaper index: %w", err)
	}
	s.wallpapers = next
	return wallpaper, nil
}

// Delete removes a wallpaper; its files are removed after the index no longer lists it.
func (s *Store) Delete(id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.wallpapers, func(wallpaper Wallpaper) bool { return wallpaper.ID == id })
	if index < 0 {
		return ErrNotFound
	}
	next := slices.Delete(slices.Clone(s.wallpapers), index, index+1)
	if err := s.writeIndexLocked(next); err != nil {
		return fmt.Errorf("persist desktop wallpaper index: %w", err)
	}
	s.wallpapers = next
	imagePath, thumbPath := s.paths(id)
	_ = os.Remove(imagePath)
	_ = os.Remove(thumbPath)
	_ = syncDirectory(s.filesDir)
	return nil
}

// File returns a stored image ("image" or "thumb") and its content type.
func (s *Store) File(id, kind string) ([]byte, string, error) {
	if !ValidID(id) || (kind != "image" && kind != "thumb") {
		return nil, "", ErrNotFound
	}
	s.mu.Lock()
	known := slices.ContainsFunc(s.wallpapers, func(wallpaper Wallpaper) bool { return wallpaper.ID == id })
	s.mu.Unlock()
	if !known {
		return nil, "", ErrNotFound
	}
	imagePath, thumbPath := s.paths(id)
	path, limit := imagePath, int64(MaxImageBytes)
	if kind == "thumb" {
		path, limit = thumbPath, MaxThumbBytes
	}
	data, err := readRegular(path, limit)
	if err != nil {
		return nil, "", err
	}
	format := sniff(data)
	if format == "" {
		return nil, "", ErrInvalid
	}
	return data, "image/" + format, nil
}

func (s *Store) usageLocked() Usage {
	var total int64
	for _, wallpaper := range s.wallpapers {
		total += wallpaper.ImageBytes + wallpaper.ThumbBytes
	}
	return Usage{Count: len(s.wallpapers), Bytes: total, MaxCount: MaxWallpapers, MaxBytes: MaxTotalBytes}
}

func (s *Store) paths(id string) (imagePath, thumbPath string) {
	return filepath.Join(s.filesDir, id+".image"), filepath.Join(s.filesDir, id+".thumb")
}

func (s *Store) indexPath() string { return filepath.Join(s.root, "index.json") }

func (s *Store) readIndex() (persistedIndex, error) {
	data, err := readRegular(s.indexPath(), 1<<20)
	if errors.Is(err, ErrNotFound) {
		return persistedIndex{Version: 1}, nil
	}
	if err != nil {
		return persistedIndex{}, fmt.Errorf("read desktop wallpaper index: %w", err)
	}
	var index persistedIndex
	if err := json.Unmarshal(data, &index); err != nil || index.Version != 1 {
		// An unreadable index loses the list, not the panel: start empty and let
		// the stray-file sweep reclaim the space.
		return persistedIndex{Version: 1}, nil
	}
	return index, nil
}

func (s *Store) writeIndexLocked(wallpapers []Wallpaper) error {
	data, err := json.MarshalIndent(persistedIndex{Version: 1, Wallpapers: wallpapers}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicPrivateFile(s.root, s.indexPath(), append(data, '\n'))
}

func (s *Store) filesPresent(wallpaper Wallpaper) bool {
	imagePath, thumbPath := s.paths(wallpaper.ID)
	for _, check := range []struct {
		path string
		size int64
	}{{imagePath, wallpaper.ImageBytes}, {thumbPath, wallpaper.ThumbBytes}} {
		info, err := os.Lstat(check.path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != check.size {
			return false
		}
	}
	return true
}

func (s *Store) removeStrayFilesLocked() error {
	entries, err := os.ReadDir(s.filesDir)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, wallpaper := range s.wallpapers {
		known[wallpaper.ID+".image"] = true
		known[wallpaper.ID+".thumb"] = true
	}
	for _, entry := range entries {
		if known[entry.Name()] || entry.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(s.filesDir, entry.Name()))
	}
	return nil
}

func normalizeMetadata(meta Metadata) (Metadata, error) {
	meta.Name = strings.TrimSpace(meta.Name)
	if meta.Name == "" || !utf8.ValidString(meta.Name) || utf8.RuneCountInString(meta.Name) > MaxNameRunes ||
		strings.IndexFunc(meta.Name, unicode.IsControl) >= 0 {
		return Metadata{}, &ValidationError{Field: "name", Detail: "name requires 1 to 40 visible characters"}
	}
	if meta.FocusX < 0 || meta.FocusX > FocusScale || meta.FocusY < 0 || meta.FocusY > FocusScale {
		return Metadata{}, &ValidationError{Field: "focus", Detail: "focus must be between 0 and 1000"}
	}
	if meta.Theme != nil {
		theme := *meta.Theme
		for _, color := range []string{theme.Brand, theme.Neutral, theme.Signature} {
			if !colorPattern.MatchString(color) {
				return Metadata{}, &ValidationError{Field: "theme", Detail: "theme colors must be lowercase #rrggbb values"}
			}
		}
		meta.Theme = &theme
	}
	return meta, nil
}

func validateStored(wallpaper Wallpaper) error {
	if !ValidID(wallpaper.ID) || (wallpaper.Format != "webp" && wallpaper.Format != "jpeg") ||
		wallpaper.Width < MinDimension || wallpaper.Height < MinDimension ||
		wallpaper.Width > MaxDimension || wallpaper.Height > MaxDimension ||
		wallpaper.ImageBytes <= 0 || wallpaper.ImageBytes > MaxImageBytes ||
		wallpaper.ThumbBytes <= 0 || wallpaper.ThumbBytes > MaxThumbBytes ||
		wallpaper.Luminance < 0 || wallpaper.Luminance > 100 || len(wallpaper.ImageDigest) != 64 {
		return ErrInvalid
	}
	_, err := normalizeMetadata(Metadata{Name: wallpaper.Name, FocusX: wallpaper.FocusX, FocusY: wallpaper.FocusY, Theme: wallpaper.Theme})
	return err
}

func sniff(data []byte) string {
	switch {
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "jpeg"
	}
	return ""
}

// sanitize identifies the format and returns a copy without metadata.
func sanitize(data []byte, limit int) (string, []byte, error) {
	if len(data) == 0 || len(data) > limit {
		return "", nil, ErrInvalid
	}
	switch sniff(data) {
	case "webp":
		clean, err := stripWebP(data)
		return "webp", clean, err
	case "jpeg":
		clean, err := stripJPEG(data)
		return "jpeg", clean, err
	}
	return "", nil, ErrInvalid
}

// stripWebP keeps only the image bitstream chunks. Metadata chunks (ICCP, EXIF, XMP)
// and unknown chunks are dropped, animation is refused, and nothing may follow the
// RIFF container.
func stripWebP(data []byte) ([]byte, error) {
	if len(data) < 20 || int(binary.LittleEndian.Uint32(data[4:8]))+8 != len(data) {
		return nil, ErrInvalid
	}
	var kept [][]byte
	var extended []byte
	bitstreams := 0
	for offset := 12; offset < len(data); {
		if offset+8 > len(data) {
			return nil, ErrInvalid
		}
		fourCC := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := offset + 8 + size
		padded := end + size%2
		if size < 0 || end > len(data) || padded > len(data) {
			return nil, ErrInvalid
		}
		chunk := data[offset:padded]
		switch fourCC {
		case "VP8 ", "VP8L":
			bitstreams++
			kept = append(kept, chunk)
		case "ALPH":
			kept = append(kept, chunk)
		case "VP8X":
			if extended != nil || size != 10 {
				return nil, ErrInvalid
			}
			extended = slices.Clone(chunk)
		case "ANIM", "ANMF":
			return nil, ErrInvalid
		}
		offset = padded
	}
	if bitstreams != 1 {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	out.Write([]byte("RIFF\x00\x00\x00\x00WEBP"))
	if extended != nil {
		// Keep only the alpha flag: ICC, EXIF, XMP and animation are gone.
		extended[8] &= 0x10
		out.Write(extended)
	}
	for _, chunk := range kept {
		out.Write(chunk)
	}
	clean := out.Bytes()
	binary.LittleEndian.PutUint32(clean[4:8], uint32(len(clean)-8))
	return clean, nil
}

// stripJPEG drops application segments other than JFIF (APP1 carries EXIF and XMP)
// and comments before the first scan, and refuses data after the end marker.
func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 4 || data[len(data)-2] != 0xff || data[len(data)-1] != 0xd9 {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	out.Write(data[:2])
	for offset := 2; ; {
		if offset+4 > len(data) || data[offset] != 0xff {
			return nil, ErrInvalid
		}
		marker := data[offset+1]
		if marker == 0xff {
			offset++
			continue
		}
		size := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		end := offset + 2 + size
		if size < 2 || end > len(data) {
			return nil, ErrInvalid
		}
		if marker == 0xda {
			out.Write(data[offset:])
			return out.Bytes(), nil
		}
		if marker != 0xfe && (marker < 0xe1 || marker > 0xef) {
			out.Write(data[offset:end])
		}
		offset = end
	}
}

func decodeChecked(format string, data []byte, minDimension, maxDimension int) (image.Image, error) {
	reader := bytes.NewReader(data)
	var config image.Config
	var err error
	if format == "webp" {
		config, err = webp.DecodeConfig(reader)
	} else {
		config, err = jpeg.DecodeConfig(reader)
	}
	if err != nil || config.Width < minDimension || config.Height < minDimension ||
		config.Width > maxDimension || config.Height > maxDimension ||
		int64(config.Width)*int64(config.Height) > MaxPixels {
		return nil, ErrInvalid
	}
	var decoded image.Image
	if format == "webp" {
		decoded, err = webp.Decode(bytes.NewReader(data))
	} else {
		decoded, err = jpeg.Decode(bytes.NewReader(data))
	}
	if err != nil || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, ErrInvalid
	}
	return decoded, nil
}

// averageLuminance is the mean perceived brightness, 0–100, used to thicken the
// classic-mode veil over bright pictures.
func averageLuminance(img image.Image) int {
	bounds := img.Bounds()
	var sum, count float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			sum += 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return int(sum/count/0xffff*100 + 0.5)
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func readRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, ErrNotFound
	case err != nil:
		return nil, err
	case !info.Mode().IsRegular():
		return nil, errors.New("desktop wallpaper file must be a regular file")
	case info.Size() <= 0 || info.Size() > limit:
		return nil, ErrInvalid
	}
	return os.ReadFile(path)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path must be a non-symlink directory")
	}
	return os.Chmod(path, 0o700)
}

func writeAtomicPrivateFile(directory, target string, data []byte) error {
	file, err := os.CreateTemp(directory, ".desktop-wallpaper-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		if runtime.GOOS != "windows" {
			return err
		}
		// Local Windows development cannot rename over an existing file.
		_ = os.Remove(target)
		if err := os.Rename(temporary, target); err != nil {
			return err
		}
	}
	return syncDirectory(directory)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
