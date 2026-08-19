package appmarket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/image/webp"
)

const (
	maxDynamicAppIconBytes     = 128 << 10
	maxDynamicAppIconDimension = 1024
	maxDynamicAppIconPixels    = 1 << 20
	dynamicAppIconRefreshTTL   = 24 * time.Hour
	dynamicAppIconFetchTimeout = 4 * time.Second
	dynamicAppIconFetchWorkers = 4
)

var (
	ErrAppIconNotFound    = errors.New("application icon not found")
	ErrAppIconUnavailable = errors.New("application icon unavailable")
)

type AppIcon struct {
	ContentType string
	Data        []byte
	SHA256      string
}

type appIconFetchCall struct {
	done chan struct{}
	icon AppIcon
	err  error
}

type officialIconCache struct {
	dir        string
	baseURL    *url.URL
	client     *http.Client
	now        func() time.Time
	fetchSlots chan struct{}

	callsMu sync.Mutex
	calls   map[string]*appIconFetchCall
}

func newOfficialIconCache(dir string) (*officialIconCache, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || filepath.Dir(dir) == dir {
		return nil, errors.New("application icon cache requires a dedicated absolute path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create application icon cache directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("application icon cache path must be a non-symlink directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure application icon cache directory: %w", err)
	}
	baseURL, err := url.Parse(OfficialCatalogURL)
	if err != nil {
		return nil, fmt.Errorf("parse official application catalog URL: %w", err)
	}
	return &officialIconCache{
		dir: dir, baseURL: baseURL, client: newOfficialIconHTTPClient(baseURL),
		now: time.Now, fetchSlots: make(chan struct{}, dynamicAppIconFetchWorkers),
		calls: make(map[string]*appIconFetchCall),
	}, nil
}

func newOfficialIconHTTPClient(baseURL *url.URL) *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	} else {
		transport = transport.Clone()
	}
	transport.DisableCompression = true
	transport.MaxIdleConns = dynamicAppIconFetchWorkers
	transport.MaxIdleConnsPerHost = dynamicAppIconFetchWorkers
	transport.MaxConnsPerHost = dynamicAppIconFetchWorkers
	transport.MaxResponseHeaderBytes = 32 << 10
	return &http.Client{
		Transport: transport,
		Timeout:   dynamicAppIconFetchTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 2 || request.URL.Scheme != baseURL.Scheme ||
				request.URL.Host != baseURL.Host {
				return errors.New("application icon redirect was rejected")
			}
			return nil
		},
	}
}

func (c *officialIconCache) Prefetch(ctx context.Context, sources map[string]string) {
	if len(sources) == 0 {
		return
	}
	type iconSource struct {
		slug string
		path string
	}
	jobs := make(chan iconSource)
	var workers sync.WaitGroup
	var failures atomic.Int32
	var firstFailure error
	var firstFailureOnce sync.Once
	for range min(dynamicAppIconFetchWorkers, len(sources)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				if _, err := c.ensure(ctx, job.slug, job.path, true); err != nil &&
					!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					failures.Add(1)
					firstFailureOnce.Do(func() { firstFailure = err })
				}
			}
		}()
	}
send:
	for slug, source := range sources {
		select {
		case jobs <- iconSource{slug: slug, path: source}:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	workers.Wait()
	if failures.Load() > 0 {
		slog.Warn("application icon prefetch partially failed", "failures", failures.Load(), "error", firstFailure)
	}
}

func (c *officialIconCache) Prune(sources map[string]string) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		slog.Warn("application icon cache prune failed", "error", err)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		remove := false
		if len(name) > len(".webp") && filepath.Ext(name) == ".webp" {
			slug := name[:len(name)-len(".webp")]
			_, keep := sources[slug]
			remove = tokenPattern.MatchString(slug) && !keep
		} else if len(name) > len(".app-icon-") && name[:len(".app-icon-")] == ".app-icon-" {
			remove = true
		}
		if remove {
			if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("application icon cache entry prune failed", "name", name, "error", err)
			}
		}
	}
}

func (c *officialIconCache) Get(ctx context.Context, slug, source string) (AppIcon, error) {
	if !tokenPattern.MatchString(slug) || !remoteIconPattern.MatchString(source) ||
		source != "icons/"+slug+".webp" {
		return AppIcon{}, ErrAppIconNotFound
	}
	icon, err := c.ensure(ctx, slug, source, false)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return AppIcon{}, err
		}
		return AppIcon{}, fmt.Errorf("%w: %w", ErrAppIconUnavailable, err)
	}
	return icon, nil
}

func (c *officialIconCache) ensure(
	ctx context.Context,
	slug string,
	source string,
	refreshStale bool,
) (AppIcon, error) {
	cached, modifiedAt, cachedErr := c.read(slug)
	if cachedErr == nil && (!refreshStale || c.now().UTC().Sub(modifiedAt.UTC()) < dynamicAppIconRefreshTTL) {
		return cached, nil
	}
	key := slug + "\x00" + source
	icon, err := c.doFetch(ctx, key, func() (AppIcon, error) {
		current, currentModifiedAt, currentErr := c.read(slug)
		if currentErr == nil && (!refreshStale || c.now().UTC().Sub(currentModifiedAt.UTC()) < dynamicAppIconRefreshTTL) {
			return current, nil
		}
		select {
		case c.fetchSlots <- struct{}{}:
			defer func() { <-c.fetchSlots }()
		case <-ctx.Done():
			return AppIcon{}, ctx.Err()
		}
		fresh, fetchErr := c.fetch(ctx, source)
		if fetchErr != nil {
			if currentErr == nil {
				return current, nil
			}
			return AppIcon{}, fetchErr
		}
		if writeErr := c.write(slug, fresh.Data); writeErr != nil {
			return AppIcon{}, writeErr
		}
		return fresh, nil
	})
	return icon, err
}

func (c *officialIconCache) doFetch(
	ctx context.Context,
	key string,
	fetch func() (AppIcon, error),
) (AppIcon, error) {
	c.callsMu.Lock()
	if call, ok := c.calls[key]; ok {
		c.callsMu.Unlock()
		select {
		case <-call.done:
			return call.icon, call.err
		case <-ctx.Done():
			return AppIcon{}, ctx.Err()
		}
	}
	call := &appIconFetchCall{done: make(chan struct{})}
	c.calls[key] = call
	c.callsMu.Unlock()

	call.icon, call.err = fetch()
	c.callsMu.Lock()
	delete(c.calls, key)
	close(call.done)
	c.callsMu.Unlock()
	return call.icon, call.err
}

func (c *officialIconCache) fetch(ctx context.Context, source string) (AppIcon, error) {
	if !remoteIconPattern.MatchString(source) {
		return AppIcon{}, errors.New("application icon source is invalid")
	}
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: source})
	if requestURL.Scheme != c.baseURL.Scheme || requestURL.Host != c.baseURL.Host {
		return AppIcon{}, errors.New("application icon source escaped the official origin")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return AppIcon{}, err
	}
	request.Header.Set("Accept", "image/webp")
	request.Header.Set("User-Agent", "KPanel application icon cache")
	response, err := c.client.Do(request)
	if err != nil {
		return AppIcon{}, fmt.Errorf("fetch application icon: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AppIcon{}, fmt.Errorf("fetch application icon: HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "image/webp" {
		return AppIcon{}, errors.New("application icon returned an invalid content type")
	}
	if response.ContentLength > maxDynamicAppIconBytes {
		return AppIcon{}, errors.New("application icon exceeds 128 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDynamicAppIconBytes+1))
	if err != nil {
		return AppIcon{}, fmt.Errorf("read application icon: %w", err)
	}
	if len(data) == 0 || len(data) > maxDynamicAppIconBytes {
		return AppIcon{}, errors.New("application icon is empty or exceeds 128 KiB")
	}
	if err := validateDynamicAppIcon(data); err != nil {
		return AppIcon{}, err
	}
	return newAppIcon(data), nil
}

func (c *officialIconCache) read(slug string) (AppIcon, time.Time, error) {
	path := c.path(slug)
	before, err := os.Lstat(path)
	if err != nil {
		return AppIcon{}, time.Time{}, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		before.Size() <= 0 || before.Size() > maxDynamicAppIconBytes {
		return AppIcon{}, time.Time{}, errors.New("cached application icon is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return AppIcon{}, time.Time{}, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() {
		return AppIcon{}, time.Time{}, errors.New("cached application icon changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDynamicAppIconBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxDynamicAppIconBytes {
		return AppIcon{}, time.Time{}, errors.New("cached application icon is unreadable")
	}
	if err := validateDynamicAppIcon(data); err != nil {
		return AppIcon{}, time.Time{}, err
	}
	return newAppIcon(data), after.ModTime(), nil
}

func (c *officialIconCache) write(slug string, data []byte) error {
	if !tokenPattern.MatchString(slug) {
		return errors.New("application icon slug is invalid")
	}
	info, err := os.Lstat(c.dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("application icon cache directory is unavailable or unsafe")
	}
	file, err := os.CreateTemp(c.dir, ".app-icon-*")
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
	if err := os.Rename(temporary, c.path(slug)); err != nil {
		return err
	}
	if directory, openErr := os.Open(c.dir); openErr == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (c *officialIconCache) path(slug string) string {
	return filepath.Join(c.dir, slug+".webp")
}

func validateDynamicAppIcon(data []byte) error {
	if len(data) < 12 || !bytes.Equal(data[:4], []byte("RIFF")) ||
		!bytes.Equal(data[8:12], []byte("WEBP")) {
		return errors.New("application icon is not WebP")
	}
	config, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 ||
		config.Width > maxDynamicAppIconDimension || config.Height > maxDynamicAppIconDimension ||
		config.Width > maxDynamicAppIconPixels/config.Height {
		return errors.New("application icon dimensions are invalid")
	}
	if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
		return errors.New("application icon data is invalid")
	}
	return nil
}

func newAppIcon(data []byte) AppIcon {
	digest := sha256.Sum256(data)
	return AppIcon{
		ContentType: "image/webp",
		Data:        append([]byte(nil), data...),
		SHA256:      hex.EncodeToString(digest[:]),
	}
}

func (s *Service) ConfigureIconCache(dir string) error {
	cache, err := newOfficialIconCache(dir)
	if err != nil {
		return err
	}
	s.catalogMu.Lock()
	s.iconCache = cache
	s.catalogMu.Unlock()
	return nil
}

func (s *Service) Icon(ctx context.Context, slug string) (AppIcon, error) {
	if !tokenPattern.MatchString(slug) {
		return AppIcon{}, ErrAppIconNotFound
	}
	s.catalogMu.Lock()
	cache := s.iconCache
	source, known := s.dynamicIconSources[slug]
	s.catalogMu.Unlock()
	if cache == nil || !known {
		return AppIcon{}, ErrAppIconNotFound
	}
	return cache.Get(ctx, slug, source)
}
