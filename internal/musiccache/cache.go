// Package musiccache stores disposable, versioned file-header observations.
// Audio files remain authoritative; this cache is never a music library database.
package musiccache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/atomicfile"
)

const MaxRecords = 4096
const MaxBytes = 16 << 20
const MaxRecordBytes = 12 << 10
const Retention = 180 * 24 * time.Hour

type Ref struct {
	HostID          string `json:"hostId"`
	Path            string `json:"path"`
	ResourceVersion string `json:"resourceVersion"`
	SizeBytes       int64  `json:"sizeBytes"`
}
type Tags struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
}
type Quality struct {
	Lossless      bool   `json:"lossless,omitempty"`
	Bitrate       int    `json:"bitrate,omitempty"`
	BitrateKind   string `json:"bitrateKind,omitempty"`
	SampleRate    int    `json:"sampleRate,omitempty"`
	BitsPerSample int    `json:"bitsPerSample,omitempty"`
	VBR           bool   `json:"vbr,omitempty"`
}
type Info struct {
	Tags           *Tags    `json:"tags,omitempty"`
	Quality        *Quality `json:"quality,omitempty"`
	EmbeddedAssets *bool    `json:"embeddedAssets,omitempty"`
}
type Record struct {
	Ref        Ref       `json:"ref"`
	Info       Info      `json:"info"`
	Schema     int       `json:"schema"`
	Source     string    `json:"source"`
	DetectedAt time.Time `json:"detectedAt"`
}
type diskEntry struct {
	size     int64
	modified time.Time
}
type Cache struct {
	mu     sync.Mutex
	dir    string
	root   os.FileInfo
	files  map[string]diskEntry
	bytes  int64
	now    func() time.Time
	writer atomicfile.Writer
}

func validText(s string, limit int) bool {
	return len(s) <= limit && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func (r Ref) Valid() bool {
	return validText(r.HostID, 128) && validText(r.Path, 4096) && strings.HasPrefix(r.Path, "/") &&
		validText(r.ResourceVersion, 256) && r.ResourceVersion != "" && r.SizeBytes >= 0
}
func (i Info) Valid() bool {
	if i.Tags == nil && i.Quality == nil {
		return false
	}
	if t := i.Tags; t != nil && (!validText(t.Title, 1024) || !validText(t.Artist, 1024) || !validText(t.Album, 1024)) {
		return false
	}
	if q := i.Quality; q != nil {
		if q.Bitrate < 0 || q.Bitrate > 1000000 || q.SampleRate < 0 || q.SampleRate > 10000000 || q.BitsPerSample < 0 || q.BitsPerSample > 64 {
			return false
		}
		if q.BitrateKind != "" && q.BitrateKind != "frame" && q.BitrateKind != "average" && q.BitrateKind != "pcm" {
			return false
		}
	}
	return true
}
func key(r Ref) string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) + ".json"
}
func cacheName(name string) bool {
	if len(name) != 69 || !strings.HasSuffix(name, ".json") {
		return false
	}
	_, err := hex.DecodeString(name[:64])
	return err == nil
}

// Open indexes only a bounded private directory. Corrupt/expired entries are misses;
// excessive or unowned directory contents disable the optional cache rather than scan forever.
func Open(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid music cache directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	c := &Cache{dir: dir, root: root, files: map[string]diskEntry{}, now: time.Now}
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	entries, err := d.ReadDir(MaxRecords + 33)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > MaxRecords+32 {
		return nil, errors.New("music cache directory limit")
	}
	for _, entry := range entries {
		if !cacheName(entry.Name()) {
			name := entry.Name()
			if strings.HasPrefix(name, ".music-") && strings.HasSuffix(name, ".tmp") {
				st, err := os.Lstat(filepath.Join(dir, name))
				if err != nil || !st.Mode().IsRegular() {
					return nil, errors.New("invalid music cache temporary")
				}
				if err := os.Remove(filepath.Join(dir, name)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, errors.New("unexpected music cache entry")
		}
		st, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() {
			return nil, errors.New("invalid music cache entry")
		}
		if err := os.Chmod(filepath.Join(dir, entry.Name()), 0600); err != nil {
			return nil, err
		}
		c.files[entry.Name()] = diskEntry{st.Size(), st.ModTime()}
		c.bytes += st.Size()
	}
	if err := c.prune(0, false); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Cache) trusted() error {
	st, err := os.Lstat(c.dir)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !os.SameFile(c.root, st) {
		return errors.New("music cache directory changed")
	}
	return nil
}
func (c *Cache) remove(name string) error {
	if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	c.bytes -= c.files[name].size
	delete(c.files, name)
	return nil
}
func (c *Cache) prune(extra int64, adding bool) error {
	for name, entry := range c.files {
		if entry.size > MaxRecordBytes || c.now().Sub(entry.modified) > Retention {
			if err := c.remove(name); err != nil {
				return err
			}
		}
	}
	limit := MaxRecords
	if adding {
		limit--
	}
	for len(c.files) > limit || c.bytes+extra > MaxBytes {
		oldest := ""
		var at time.Time
		for name, entry := range c.files {
			if oldest == "" || entry.modified.Before(at) {
				oldest = name
				at = entry.modified
			}
		}
		if oldest == "" {
			return errors.New("music cache quota")
		}
		if err := c.remove(oldest); err != nil {
			return err
		}
	}
	return nil
}
func (c *Cache) get(ref Ref) (*Record, error) {
	name := key(ref)
	if _, ok := c.files[name]; !ok {
		return nil, nil
	}
	path := filepath.Join(c.dir, name)
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		c.bytes -= c.files[name].size
		delete(c.files, name)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > MaxRecordBytes {
		return nil, errors.New("invalid music cache entry")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(st, opened) {
		return nil, errors.New("music cache entry changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	var record Record
	err = json.Unmarshal(b, &record)
	if len(b) > MaxRecordBytes || err != nil || record.Ref != ref || record.Schema != 1 || record.Source != "file-header" || !record.Info.Valid() || record.DetectedAt.IsZero() || record.DetectedAt.After(c.now().Add(time.Minute)) || c.now().Sub(record.DetectedAt) > Retention {
		return nil, c.remove(name)
	}
	return &record, nil
}
func (c *Cache) Lookup(refs []Ref) ([]Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.trusted(); err != nil {
		return nil, err
	}
	if len(refs) > 32 {
		return nil, errors.New("music cache batch limit")
	}
	result := make([]Record, 0, len(refs))
	for _, ref := range refs {
		if !ref.Valid() {
			return nil, errors.New("invalid music reference")
		}
		r, err := c.get(ref)
		if err != nil {
			return nil, err
		}
		if r != nil {
			result = append(result, *r)
		}
	}
	return result, nil
}
func (c *Cache) Put(ref Ref, info Info) error {
	if !ref.Valid() || !info.Valid() {
		return errors.New("invalid music observation")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.trusted(); err != nil {
		return err
	}
	previous, err := c.get(ref)
	if err != nil {
		return err
	}
	if previous != nil {
		if info.Tags == nil {
			info.Tags = previous.Info.Tags
			info.EmbeddedAssets = previous.Info.EmbeddedAssets
		}
		if info.Quality == nil {
			info.Quality = previous.Info.Quality
		}
	}
	b, err := json.Marshal(Record{ref, info, 1, "file-header", c.now().UTC()})
	if err != nil {
		return err
	}
	if len(b) > MaxRecordBytes {
		return errors.New("music observation too large")
	}
	name := key(ref)
	if err := c.prune(int64(len(b)), true); err != nil {
		return err
	}
	result, err := c.writer.WritePrivate(c.dir, filepath.Join(c.dir, name), ".music-*.tmp", b)
	if err != nil {
		return err
	}
	if result.Committed {
		c.bytes += int64(len(b)) - c.files[name].size
		c.files[name] = diskEntry{int64(len(b)), c.now()}
	}
	return errors.Join(result.DurabilityErr, result.CleanupErr)
}
