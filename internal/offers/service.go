package offers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

const (
	// RefreshInterval is how long a successful download stays fresh.
	RefreshInterval = 6 * time.Hour
	// RetryInterval spaces automatic attempts after a failure.
	RetryInterval = 10 * time.Minute
	// ForcedRefreshInterval spaces administrator-triggered refreshes.
	ForcedRefreshInterval = time.Minute

	MediaPrefix = "/api/v1/offers/media/"

	StateLive        = "live"
	StateStale       = "stale"
	StateUnavailable = "unavailable"

	refreshTimeout   = 45 * time.Second
	imageWorkers     = 3
	maxStateBytes    = 256 << 10
	stateFileName    = "state.json"
	objectsDirName   = "objects"
	cacheStateSchema = 1
)

var ErrNotFound = errors.New("offers image not found")

// Fetch downloads one URL with a byte limit and returns the body and the
// response Content-Type. Tests replace it; production uses the shared
// remote-download client with redirects rejected.
type Fetch func(ctx context.Context, address string, limit int64) ([]byte, string, error)

type cacheState struct {
	Schema    int       `json:"schema"`
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetchedAt"`
	Manifest  Manifest  `json:"manifest"`
}

// View is the session-only API representation. Image URLs are same-origin
// and content-addressed because the Panel CSP only allows 'self' images.
type View struct {
	SchemaVersion int        `json:"schemaVersion"`
	State         string     `json:"state"`
	Source        string     `json:"source"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
	FetchedAt     *time.Time `json:"fetchedAt,omitempty"`
	Items         []ItemView `json:"items"`
}

type ItemView struct {
	ID       string `json:"id"`
	Vendor   string `json:"vendor"`
	Alt      string `json:"alt"`
	Featured bool   `json:"featured"`
	URL      string `json:"url"`
	Host     string `json:"host"`
	Card     string `json:"card"`
	Wide     string `json:"wide,omitempty"`
}

type Service struct {
	root   string
	fetch  Fetch
	now    func() time.Time
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	unavailable bool
	cached      *cacheState
	lastAttempt time.Time
	lastFailed  bool
	forcedAt    time.Time
	inflight    chan struct{}
}

// Open never fails the Panel: a damaged cache is discarded and an unusable
// cache directory only makes this optional page report "unavailable".
func Open(root string, fetch Fetch) *Service {
	s := &Service{root: root, fetch: fetch, now: time.Now}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.fetch == nil {
		s.fetch = defaultFetch()
	}
	if backup.PrivateDir(root) != nil || backup.PrivateDir(filepath.Join(root, objectsDirName)) != nil {
		s.unavailable = true
		return s
	}
	state, err := s.loadState()
	switch {
	case err == nil:
		s.cached = state
	case !errors.Is(err, os.ErrNotExist):
		slog.Warn("offers cache was discarded", "error", err)
	}
	s.pruneObjects(s.cached)
	return s
}

func (s *Service) Close() { s.cancel() }

// Snapshot returns the current view. Without any cached manifest it waits
// for one bounded download; a stale cache is returned at once while a
// background refresh runs. force asks for an immediate refresh at most once
// per ForcedRefreshInterval.
func (s *Service) Snapshot(ctx context.Context, force bool) View {
	s.mu.Lock()
	if s.unavailable {
		s.mu.Unlock()
		return View{SchemaVersion: SchemaVersion, State: StateUnavailable, Source: ManifestURL, Items: []ItemView{}}
	}
	now := s.now()
	retryDue := s.lastAttempt.IsZero() || now.Sub(s.lastAttempt) >= RetryInterval
	wait := false
	if force && (s.forcedAt.IsZero() || now.Sub(s.forcedAt) >= ForcedRefreshInterval) {
		s.forcedAt = now
		wait = true
	} else if s.cached == nil && (retryDue || s.inflight != nil) {
		// Concurrent first loads share the one download instead of
		// reporting "unavailable" while it is still running.
		wait = true
	}
	stale := s.cached != nil && now.Sub(s.cached.FetchedAt) >= RefreshInterval && retryDue
	var done chan struct{}
	if wait || stale {
		done = s.startRefreshLocked(now)
	}
	s.mu.Unlock()
	if wait {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return s.view()
}

func (s *Service) startRefreshLocked(now time.Time) chan struct{} {
	if s.inflight != nil {
		return s.inflight
	}
	done := make(chan struct{})
	s.inflight = done
	s.lastAttempt = now
	go s.refresh(done)
	return done
}

func (s *Service) refresh(done chan struct{}) {
	ctx, cancel := context.WithTimeout(s.ctx, refreshTimeout)
	defer cancel()
	state, err := s.download(ctx)
	s.mu.Lock()
	if err == nil {
		s.cached = state
		s.lastFailed = false
	} else {
		s.lastFailed = true
	}
	s.mu.Unlock()
	if err == nil {
		// Prune while this refresh still owns the in-flight slot, so a later
		// refresh can never write objects that this prune then deletes.
		s.pruneObjects(state)
	} else if !errors.Is(err, context.Canceled) {
		slog.Warn("offers manifest refresh failed", "source", ManifestURL, "error", err)
	}
	s.mu.Lock()
	s.inflight = nil
	close(done)
	s.mu.Unlock()
}

// download is all-or-nothing: a manifest is adopted only after every image
// it pins is present and valid, so the page never mixes two publications.
func (s *Service) download(ctx context.Context) (*cacheState, error) {
	body, contentType, err := s.fetch(ctx, ManifestURL, MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		return nil, fmt.Errorf("%w: manifest content type %q", ErrManifestInvalid, contentType)
	}
	manifest, err := DecodeManifest(body)
	if err != nil {
		return nil, err
	}
	type pending struct {
		image Image
		kind  imageKind
	}
	var jobs []pending
	seen := make(map[string]bool)
	for _, item := range manifest.Items {
		images := []pending{{item.Images.Card, cardImage}}
		if item.Images.Wide != nil {
			images = append(images, pending{*item.Images.Wide, wideImage})
		}
		for _, job := range images {
			if seen[job.image.SHA256] {
				continue
			}
			seen[job.image.SHA256] = true
			if s.cachedObjectValid(job.image, job.kind) {
				continue
			}
			jobs = append(jobs, job)
		}
	}
	queue := make(chan pending)
	errs := make(chan error, len(jobs))
	var workers sync.WaitGroup
	for range min(imageWorkers, len(jobs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range queue {
				errs <- s.storeImage(ctx, job.image, job.kind)
			}
		}()
	}
	for _, job := range jobs {
		queue <- job
	}
	close(queue)
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	state := &cacheState{Schema: cacheStateSchema, Source: ManifestURL, FetchedAt: s.now().UTC(), Manifest: manifest}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if err := backup.AtomicFile(filepath.Join(s.root, stateFileName), data); err != nil {
		return nil, fmt.Errorf("save offers cache: %w", err)
	}
	return state, nil
}

func (s *Service) storeImage(ctx context.Context, pinned Image, kind imageKind) error {
	_, _, maxBytes := kind.limits()
	data, contentType, err := s.fetch(ctx, imageURL(pinned.Path), maxBytes)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", pinned.Path, err)
	}
	sniffed, err := validateImage(data, pinned, kind)
	if err != nil {
		return fmt.Errorf("%s: %w", pinned.Path, err)
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != sniffed {
		return fmt.Errorf("%s: %w: content type %q", pinned.Path, ErrImageInvalid, contentType)
	}
	if err := backup.AtomicFile(s.objectPath(pinned.SHA256), data); err != nil {
		return fmt.Errorf("cache %s: %w", pinned.Path, err)
	}
	return nil
}

func (s *Service) cachedObjectValid(pinned Image, kind imageKind) bool {
	_, _, maxBytes := kind.limits()
	data, err := backup.ReadFile(s.objectPath(pinned.SHA256), maxBytes)
	if err != nil {
		return false
	}
	_, err = validateImage(data, pinned, kind)
	return err == nil
}

// OpenImage serves only digests referenced by the adopted manifest.
func (s *Service) OpenImage(digest string) ([]byte, string, error) {
	if !digestPattern.MatchString(digest) {
		return nil, "", ErrNotFound
	}
	s.mu.Lock()
	kind, ok := referencedImage(s.cached, digest)
	s.mu.Unlock()
	if !ok {
		return nil, "", ErrNotFound
	}
	_, _, maxBytes := kind.limits()
	data, err := backup.ReadFile(s.objectPath(digest), maxBytes)
	if err != nil {
		return nil, "", ErrNotFound
	}
	sum := sha256.Sum256(data)
	contentType := sniffContentType(data)
	if hex.EncodeToString(sum[:]) != digest || contentType == "" {
		return nil, "", ErrNotFound
	}
	return data, contentType, nil
}

func referencedImage(state *cacheState, digest string) (imageKind, bool) {
	if state == nil {
		return 0, false
	}
	for _, item := range state.Manifest.Items {
		if item.Images.Card.SHA256 == digest {
			return cardImage, true
		}
		if item.Images.Wide != nil && item.Images.Wide.SHA256 == digest {
			return wideImage, true
		}
	}
	return 0, false
}

func (s *Service) view() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := View{SchemaVersion: SchemaVersion, Source: ManifestURL, Items: []ItemView{}}
	if s.cached == nil {
		result.State = StateUnavailable
		return result
	}
	result.State = StateLive
	if s.lastFailed {
		result.State = StateStale
	}
	updatedAt, fetchedAt := s.cached.Manifest.UpdatedAt, s.cached.FetchedAt
	result.UpdatedAt, result.FetchedAt = &updatedAt, &fetchedAt
	now := s.now()
	for _, item := range s.cached.Manifest.Items {
		if !item.Active(now) {
			continue
		}
		target, err := ValidTargetURL(item.URL)
		if err != nil {
			continue
		}
		entry := ItemView{
			ID: item.ID, Vendor: item.Vendor, Alt: item.Alt, Featured: item.Featured, URL: item.URL,
			Host: strings.TrimPrefix(strings.ToLower(target.Hostname()), "www."),
			Card: MediaPrefix + item.Images.Card.SHA256,
		}
		if item.Images.Wide != nil {
			entry.Wide = MediaPrefix + item.Images.Wide.SHA256
		}
		result.Items = append(result.Items, entry)
	}
	return result
}

func (s *Service) loadState() (*cacheState, error) {
	data, err := backup.ReadFile(filepath.Join(s.root, stateFileName), maxStateBytes)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state cacheState
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("decode offers cache: %w", err)
	}
	if state.Schema != cacheStateSchema || state.Source != ManifestURL || state.FetchedAt.IsZero() {
		return nil, errors.New("offers cache has an unknown schema or source")
	}
	if err := ValidateManifest(state.Manifest); err != nil {
		return nil, err
	}
	for _, item := range state.Manifest.Items {
		images := []Image{item.Images.Card}
		if item.Images.Wide != nil {
			images = append(images, *item.Images.Wide)
		}
		for _, image := range images {
			info, err := os.Lstat(s.objectPath(image.SHA256))
			if err != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("offers cache is missing %s", image.Path)
			}
		}
	}
	return &state, nil
}

// pruneObjects keeps only images the adopted manifest references, which
// bounds the cache to MaxItems banners plus MaxFeatured carousel images.
func (s *Service) pruneObjects(state *cacheState) {
	keep := make(map[string]bool)
	if state != nil {
		for _, item := range state.Manifest.Items {
			keep[item.Images.Card.SHA256] = true
			if item.Images.Wide != nil {
				keep[item.Images.Wide.SHA256] = true
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.root, objectsDirName))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(s.root, objectsDirName, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("offers cache prune failed", "name", entry.Name(), "error", err)
		}
	}
}

func (s *Service) objectPath(digest string) string {
	return filepath.Join(s.root, objectsDirName, digest)
}

func defaultFetch() Fetch {
	client := remotedownload.NewClient(remotedownload.Config{
		ResponseHeaderTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second, RejectRedirects: true,
	})
	return func(ctx context.Context, address string, limit int64) ([]byte, string, error) {
		response, err := client.Open(ctx, address)
		if err != nil {
			return nil, "", err
		}
		defer response.Body.Close()
		// Trust never extends to a redirect target, even on the same host.
		if response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL == nil ||
			response.Request.URL.String() != address || response.ContentLength > limit {
			return nil, "", fmt.Errorf("unexpected response from %s", address)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil {
			return nil, "", err
		}
		if int64(len(body)) > limit {
			return nil, "", fmt.Errorf("response from %s exceeds %d bytes", address, limit)
		}
		return body, response.Header.Get("Content-Type"), nil
	}
}
