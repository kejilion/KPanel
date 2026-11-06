// Package musicmetadata provides optional, read-only online metadata for local music.
package musicmetadata

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

var ErrBusy = errors.New("music metadata busy")

type Query struct {
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
}
type Result struct {
	Matched bool   `json:"matched"`
	Source  string `json:"source,omitempty"`
	Lyrics  string `json:"lyrics,omitempty"`
	Cover   string `json:"cover,omitempty"`
}
type cached struct {
	result  Result
	expires time.Time
}
type opener interface {
	Open(context.Context, string) (*http.Response, error)
}
type Service struct {
	client opener
	active chan struct{}
	mu     sync.Mutex
	cache  map[Query]cached
}

type metadataClient struct{ client *http.Client }

func (c metadataClient) Open(ctx context.Context, raw string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "KPanel-Music-Metadata/1")
	if r.URL.Host == "c.y.qq.com" {
		r.Header.Set("Referer", "https://y.qq.com/")
	}
	return c.client.Do(r)
}

func New() *Service {
	public := remotedownload.NewClient(remotedownload.Config{ConnectTimeout: 3 * time.Second})
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, DialContext: public.DialPublic, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxConnsPerHost: 2, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second}}
	return &Service{client: metadataClient{client}, active: make(chan struct{}, 2), cache: make(map[Query]cached)}
}

func (q Query) Valid() bool {
	for _, s := range []string{q.Title, q.Artist, q.Album} {
		if len(s) > 256 || strings.IndexFunc(s, unicode.IsControl) >= 0 {
			return false
		}
	}
	return normalize(q.Title) != "" && q.Duration >= 0 && q.Duration <= 24*3600 && !math.IsNaN(q.Duration) && !math.IsInf(q.Duration, 0)
}

// Match bounds the entire operation, response bodies, concurrency and disposable cache.
// No file paths or audio bytes leave the panel; only music tags are searched.
func (s *Service) Match(ctx context.Context, q Query) (Result, error) {
	if !q.Valid() {
		return Result{}, errors.New("invalid music tags")
	}
	q.Title = strings.TrimSpace(q.Title)
	q.Artist = strings.TrimSpace(q.Artist)
	q.Album = strings.TrimSpace(q.Album)
	q.Duration = math.Round(q.Duration)
	s.mu.Lock()
	hit, ok := s.cache[q]
	s.mu.Unlock()
	if ok && time.Now().Before(hit.expires) {
		return hit.result, nil
	}
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
		return Result{}, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	first, firstCancel := context.WithTimeout(ctx, 6*time.Second)
	result, err := s.fetch(first, q)
	firstCancel()
	if !result.Matched && ctx.Err() == nil {
		fallback, fallbackErr := s.fetchQQ(ctx, q)
		if fallbackErr == nil {
			result = fallback
			err = nil
		} else if err == nil {
			err = fallbackErr
		}
	}
	if err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		return Result{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= 16 {
		var oldest Query
		var expiry time.Time
		for k, v := range s.cache {
			if expiry.IsZero() || v.expires.Before(expiry) {
				oldest = k
				expiry = v.expires
			}
		}
		delete(s.cache, oldest)
	}
	ttl := 30 * time.Minute
	if !result.Matched {
		ttl = time.Minute
	}
	s.cache[q] = cached{result, time.Now().Add(ttl)}
	return result, nil
}

type song struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name   string `json:"name"`
		PicURL string `json:"picUrl"`
	} `json:"album"`
}

func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func score(q Query, s song) int {
	// Preserve version words (Live, remix, cover, etc.) instead of stripping suffixes.
	if s.ID <= 0 || normalize(q.Title) != normalize(s.Name) {
		return -1
	}
	if q.Duration > 0 && math.Abs(q.Duration-float64(s.Duration)/1000) > 5 {
		return -1
	}
	artists := make([]string, 0, len(s.Artists))
	for _, a := range s.Artists {
		artists = append(artists, normalize(a.Name))
	}
	sort.Strings(artists)
	if q.Artist != "" {
		parts := strings.FieldsFunc(q.Artist, func(r rune) bool { return strings.ContainsRune("、&;/,，；|", r) })
		for i := range parts {
			parts[i] = normalize(parts[i])
		}
		sort.Strings(parts)
		if normalize(q.Artist) != normalize(strings.Join(artists, "")) && strings.Join(parts, "/") != strings.Join(artists, "/") {
			return -1
		}
	} else if q.Duration == 0 {
		return -1
	} // A bare title cannot distinguish artists.
	value := 1
	if q.Album != "" && normalize(q.Album) == normalize(s.Album.Name) {
		value += 2
	}
	return value
}
func (s *Service) fetch(ctx context.Context, q Query) (Result, error) {
	var search struct {
		Code   int `json:"code"`
		Result struct {
			Songs []song `json:"songs"`
		} `json:"result"`
	}
	params := url.Values{"s": {q.Title + " " + q.Artist}, "type": {"1"}, "limit": {"20"}}
	if err := s.json(ctx, "https://music.163.com/api/search/get?"+params.Encode(), &search); err != nil {
		return Result{}, err
	}
	if search.Code != 200 {
		return Result{}, errors.New("music search unavailable")
	}
	best := -1
	var selected song
	for _, candidate := range search.Result.Songs {
		if n := score(q, candidate); n > best {
			best = n
			selected = candidate
		}
	}
	if best < 0 {
		return Result{}, nil
	}
	id := strconv.FormatInt(selected.ID, 10)
	result := Result{Source: "netease"}
	var lyric struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	lyricErr := s.json(ctx, "https://music.163.com/api/song/lyric?id="+id+"&lv=-1&tv=-1&rv=-1", &lyric)
	if lyricErr == nil && len(lyric.LRC.Lyric) <= 128<<10 {
		result.Lyrics = lyric.LRC.Lyric
	}
	var detail struct {
		Songs []song `json:"songs"`
	}
	detailErr := s.json(ctx, "https://music.163.com/api/song/detail?ids=%5B"+id+"%5D", &detail)
	if detailErr == nil && len(detail.Songs) > 0 && detail.Songs[0].ID == selected.ID {
		if raw := safeCover(detail.Songs[0].Album.PicURL); raw != "" {
			result.Cover, _ = s.cover(ctx, raw)
		}
	}
	result.Matched = result.Lyrics != "" || result.Cover != ""
	if !result.Matched && (lyricErr != nil || detailErr != nil) {
		return Result{}, errors.New("music metadata unavailable")
	}
	return result, nil
}
func safeCover(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || u.Fragment != "" || len(raw) > 2048 {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	if u.Host != "p1.music.126.net" && u.Host != "p2.music.126.net" && u.Host != "p3.music.126.net" && u.Host != "p4.music.126.net" {
		return ""
	}
	u.Scheme = "https"
	u.RawQuery = "param=300y300"
	return u.String()
}
func (s *Service) read(ctx context.Context, raw string, limit int64) ([]byte, error) {
	r, err := s.client.Open(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, errors.New("upstream status")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("upstream response too large")
	}
	return body, nil
}
func (s *Service) json(ctx context.Context, raw string, out any) error {
	body, err := s.read(ctx, raw, 512<<10)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}
func (s *Service) cover(ctx context.Context, raw string) (string, error) {
	body, err := s.read(ctx, raw, 256<<10)
	if err != nil {
		return "", err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 2048 || config.Height > 2048 {
		return "", errors.New("invalid cover")
	}
	if format != "jpeg" && format != "png" {
		return "", errors.New("unsupported cover")
	}
	return fmt.Sprintf("data:image/%s;base64,%s", format, base64.StdEncoding.EncodeToString(body)), nil
}
