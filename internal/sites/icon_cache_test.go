package sites

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type iconRoundTripFunc func(*http.Request) (*http.Response, error)

func (function iconRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestIconCacheLoadsLinkedIconAndPersistsAcrossRestart(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("a", 32)
	directory := t.TempDir()
	icon := testIconPNG(t, 32, 32)
	var calls atomic.Int32
	cache := newTestIconCache(t, directory, &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch request.URL.Path {
		case "/":
			return iconResponse(
				http.StatusOK,
				"text/html",
				[]byte(`<html><head><title> 科技狮 &amp; KPanel </title><link rel="icon" type="application/octet-stream" href="/assets/favicon.png?v=3"></head></html>`),
			), nil
		case "/assets/favicon.png":
			if request.URL.RawQuery != "v=3" {
				t.Fatalf("icon query = %q", request.URL.RawQuery)
			}
			return iconResponse(http.StatusOK, "application/octet-stream", icon), nil
		default:
			t.Fatalf("unexpected request URL: %s", request.URL)
			return nil, errors.New("unexpected request")
		}
	})

	first, err := cache.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentType != "image/png" || !bytes.Equal(first.Data, icon) {
		t.Fatalf("unexpected first icon: type=%q bytes=%d", first.ContentType, len(first.Data))
	}
	appearance, err := cache.Appearance(context.Background(), id)
	if err != nil || appearance.Name != "科技狮 & KPanel" {
		t.Fatalf("site appearance = %#v, %v", appearance, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("network calls = %d, want 2", calls.Load())
	}
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("fresh cache performed another request: %d", calls.Load())
	}

	restarted := newTestIconCache(t, directory, &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		t.Fatalf("persistent cache attempted network request: %s", request.URL)
		return nil, errors.New("network must not be used")
	})
	persisted, err := restarted.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ContentType != "image/png" || !bytes.Equal(persisted.Data, icon) {
		t.Fatal("persistent cache changed the validated icon")
	}
	persistedAppearance, err := restarted.Appearance(context.Background(), id)
	if err != nil || persistedAppearance.Name != "科技狮 & KPanel" {
		t.Fatalf("persistent appearance = %#v, %v", persistedAppearance, err)
	}
	for _, path := range []string{cache.iconPath(id), cache.metadataPath(id)} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("cache artifact is not a regular file: %s", path)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("cache artifact permissions = %o", info.Mode().Perm())
		}
	}
}

func TestIconCacheServesStaleIconAndBacksOffAfterRefreshFailure(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("b", 32)
	icon := testIconPNG(t, 24, 24)
	var unavailable atomic.Bool
	var calls atomic.Int32
	cache := newTestIconCache(t, t.TempDir(), &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if unavailable.Load() {
			return iconResponse(http.StatusServiceUnavailable, "text/plain", []byte("unavailable")), nil
		}
		if request.URL.Path == "/" {
			return iconResponse(http.StatusOK, "text/html", []byte(`<link rel=icon href=/favicon.png>`)), nil
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	initialCalls := calls.Load()

	now = now.Add(8 * 24 * time.Hour)
	unavailable.Store(true)
	stale, err := cache.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stale.Data, icon) {
		t.Fatal("refresh failure did not preserve the stale icon")
	}
	afterFailure := calls.Load()
	if afterFailure <= initialCalls {
		t.Fatal("expired icon was not refreshed")
	}
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != afterFailure {
		t.Fatal("negative retry window did not suppress repeated refreshes")
	}
}

func TestIconCacheNegativeCachesMissingIcon(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("c", 32)
	var calls atomic.Int32
	cache := newTestIconCache(t, t.TempDir(), &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Path == "/" {
			return iconResponse(http.StatusOK, "text/html", []byte("<html><head><title>只有名称</title></head></html>")), nil
		}
		return iconResponse(http.StatusNotFound, "text/plain", nil), nil
	})
	for range 2 {
		if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconNotFound) {
			t.Fatalf("missing icon error = %v", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("negative cache calls = %d, want homepage and fallback only", calls.Load())
	}
	appearance, err := cache.Appearance(context.Background(), id)
	if err != nil || appearance.Name != "只有名称" {
		t.Fatalf("appearance without icon = %#v, %v", appearance, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("appearance bypassed negative cache: %d", calls.Load())
	}
	now = now.Add(25 * time.Hour)
	if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconNotFound) {
		t.Fatalf("expired negative cache error = %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("expired negative cache calls = %d, want 4", calls.Load())
	}
}

func TestParseSiteAppearanceNameNormalizesAndBoundsTitle(t *testing.T) {
	name := parseSiteAppearanceName([]byte("<TITLE data-x='1'>\n  示例&#32;网站\t\x00中心  </TITLE>"))
	if name != "示例 网站 中心" {
		t.Fatalf("appearance name = %q", name)
	}
	long := strings.Repeat("站", siteAppearanceNameRunes+20)
	if runes := []rune(parseSiteAppearanceName([]byte("<title>" + long + "</title>"))); len(runes) != siteAppearanceNameRunes {
		t.Fatalf("bounded appearance runes = %d", len(runes))
	}
	if name := parseSiteAppearanceName([]byte("<title>broken")); name != "" {
		t.Fatalf("malformed title = %q", name)
	}
}

func TestIconCacheRejectsUntrustedCandidatesAndUsesDiscoveredAlias(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("d", 32)
	site := testIconSite(id)
	site.Domains = []string{"example.com", "www.example.com", "*.example.com"}
	icon := testIconPNG(t, 16, 16)
	var requested []string
	cache := newTestIconCache(t, t.TempDir(), &now, site, func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.String())
		if request.URL.Path == "/" {
			return iconResponse(http.StatusOK, "text/html", []byte(`
				<link rel="icon" href="https://evil.example/icon.png">
				<link rel="icon" href="http://example.com:8080/icon.png">
				<link rel="icon" href="data:image/png;base64,AAAA">
				<link rel="mask-icon" href="/slow-mask.svg">
				<link rel="icon" type="image/svg+xml" href="/slow-icon.svg">
				<link rel="apple-touch-icon" href="https://www.example.com/favicon.png">
			`)), nil
		}
		if request.URL.Hostname() != "www.example.com" || request.URL.Path != "/favicon.png" {
			t.Fatalf("untrusted candidate was requested: %s", request.URL)
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 2 ||
		requested[0] != "http://example.com/" ||
		requested[1] != "https://www.example.com/favicon.png" {
		t.Fatalf("requested URLs = %#v", requested)
	}
}

func TestIconCacheCoalescesConcurrentRefreshes(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("e", 32)
	icon := testIconPNG(t, 20, 20)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	cache := newTestIconCache(t, t.TempDir(), &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Path == "/" {
			once.Do(func() { close(started) })
			<-release
			return iconResponse(http.StatusOK, "text/html", []byte(`<link rel="icon" href="/icon.png">`)), nil
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})

	const readers = 8
	errorsByReader := make(chan error, readers)
	for range readers {
		go func() {
			_, err := cache.Get(context.Background(), id)
			errorsByReader <- err
		}()
	}
	<-started
	close(release)
	for range readers {
		if err := <-errorsByReader; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("concurrent refresh network calls = %d, want 2", calls.Load())
	}
}

func TestIconCacheCallerCancellationDoesNotPoisonSharedRefresh(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("7", 32)
	icon := testIconPNG(t, 20, 20)
	started := make(chan struct{})
	release := make(chan struct{})
	inheritedCancellation := make(chan struct{}, 1)
	var once sync.Once
	var calls atomic.Int32
	cache := newTestIconCache(t, t.TempDir(), &now, testIconSite(id), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Path == "/" {
			once.Do(func() { close(started) })
			select {
			case <-release:
			case <-request.Context().Done():
				inheritedCancellation <- struct{}{}
				return nil, request.Context().Err()
			}
			return iconResponse(http.StatusOK, "text/html", []byte(`<link rel="icon" href="/icon.png">`)), nil
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := cache.Get(ctx, id)
		firstResult <- err
	}()
	<-started

	cache.callsMu.Lock()
	var call *siteIconCall
	for _, active := range cache.calls {
		call = active
	}
	cache.callsMu.Unlock()
	if call == nil {
		t.Fatal("shared refresh was not registered")
	}

	cancel()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller returned %v", err)
	}
	close(release)
	<-call.done
	select {
	case <-inheritedCancellation:
		t.Fatal("shared refresh inherited the caller cancellation")
	default:
	}

	result, err := cache.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Data, icon) {
		t.Fatal("completed shared refresh was not cached")
	}
	if calls.Load() != 2 {
		t.Fatalf("caller cancellation caused another refresh: %d requests", calls.Load())
	}
}

func TestIconCacheRefreshPanicDoesNotCrashOrLeaveCallStuck(t *testing.T) {
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := cache.doRefresh(
			context.Background(),
			"panic-test",
			func(context.Context) (SiteIcon, error) {
				panic("test panic")
			},
		); !errors.Is(err, ErrSiteIconUnavailable) {
			t.Fatalf("panic result = %v", err)
		}
		cache.callsMu.Lock()
		activeCalls := len(cache.calls)
		cache.callsMu.Unlock()
		if activeCalls != 0 {
			t.Fatalf("active calls after panic = %d", activeCalls)
		}
	}
}

func TestIconCacheRejectsUnknownSitesWildcardsAndChangedSourceCache(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("f", 32)
	site := testIconSite(id)
	current := site
	icon := testIconPNG(t, 16, 16)
	var calls atomic.Int32
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		return []contract.SiteSummary{current}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.now = func() time.Time { return now }
	cache.discoveryTTL = 0
	cache.client = &http.Client{Transport: iconRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Path == "/" {
			return iconResponse(http.StatusOK, "text/html", []byte(`<link rel=icon href=/icon.png>`)), nil
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})}
	if _, err := cache.Get(context.Background(), strings.Repeat("0", 32)); !errors.Is(err, ErrSiteIconNotFound) {
		t.Fatalf("unknown site error = %v", err)
	}
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	now = now.Add(time.Minute)
	current.PrimaryDomain = "changed.example.com"
	current.Domains = []string{"changed.example.com"}
	cache.client = &http.Client{Transport: iconRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return iconResponse(http.StatusNotFound, "text/plain", nil), nil
	})}
	if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconNotFound) {
		t.Fatalf("changed source reused old icon: %v", err)
	}

	current.PrimaryDomain = "*.example.com"
	now = now.Add(time.Minute)
	if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconNotFound) {
		t.Fatalf("wildcard site error = %v", err)
	}

	current.PrimaryDomain = "synthetic.example.com"
	current.Domains = nil
	now = now.Add(time.Minute)
	if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconNotFound) {
		t.Fatalf("synthetic fallback domain error = %v", err)
	}
}

func TestValidateSiteIconRejectsActiveAndOversizedContent(t *testing.T) {
	if contentType, err := validateSiteIcon(testIconPNG(t, 64, 64)); err != nil || contentType != "image/png" {
		t.Fatalf("valid PNG result = %q, %v", contentType, err)
	}
	for name, value := range map[string][]byte{
		"svg":      []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html":     []byte("<!doctype html><title>not an icon</title>"),
		"fake png": append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...),
		"oversize": bytes.Repeat([]byte{0}, siteIconBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateSiteIcon(value); err == nil {
				t.Fatal("unsafe icon content was accepted")
			}
		})
	}
}

func TestValidateSiteIconChecksEmbeddedICODimensions(t *testing.T) {
	valid := testPNGICO(t, testIconPNG(t, 16, 16), 16, 16)
	if contentType, err := validateSiteIcon(valid); err != nil ||
		contentType != "image/vnd.microsoft.icon" {
		t.Fatalf("valid ICO result = %q, %v", contentType, err)
	}

	mismatched := testPNGICO(t, testIconPNG(t, siteIconMaxDimension+1, 1), 1, 1)
	if _, err := validateSiteIcon(mismatched); err == nil {
		t.Fatal("ICO with oversized embedded PNG was accepted")
	}
}

func TestIconCacheDiscoveryDeadlineAndFailureBackoff(t *testing.T) {
	id := strings.Repeat("8", 32)
	release := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		calls.Add(1)
		<-release
		once.Do(func() { close(finished) })
		return nil, errors.New("discovery unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}

	const readers = 6
	results := make(chan error, readers)
	for range readers {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			_, getErr := cache.Get(ctx, id)
			results <- getErr
		}()
	}
	for range readers {
		if getErr := <-results; !errors.Is(getErr, ErrSiteIconUnavailable) {
			t.Fatalf("deadline error = %v", getErr)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent discovery calls = %d, want 1", calls.Load())
	}
	close(release)
	<-finished

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.discoveryMu.Lock()
		active := cache.discoveryCall != nil
		cache.discoveryMu.Unlock()
		if !active {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, getErr := cache.Get(context.Background(), id); !errors.Is(getErr, ErrSiteIconUnavailable) {
		t.Fatalf("backed-off discovery error = %v", getErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("failure backoff repeated discovery: %d", calls.Load())
	}
}

func TestIconCacheDiscoveryFailureNeverFetchesUsingStaleSite(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	id := strings.Repeat("9", 32)
	icon := testIconPNG(t, 16, 16)
	var discoveryFails atomic.Bool
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		if discoveryFails.Load() {
			return nil, errors.New("discovery unavailable")
		}
		return []contract.SiteSummary{testIconSite(id)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.now = func() time.Time { return now }
	cache.client = &http.Client{Transport: iconRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/" {
			return iconResponse(http.StatusOK, "text/html", []byte(`<link rel=icon href=/icon.png>`)), nil
		}
		return iconResponse(http.StatusOK, "image/png", icon), nil
	})}
	if _, err := cache.Get(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	now = now.Add(8 * 24 * time.Hour)
	discoveryFails.Store(true)
	cache.client = &http.Client{Transport: iconRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("stale site was fetched after discovery failure: %s", request.URL)
		return nil, errors.New("network must not be used")
	})}
	stale, err := cache.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stale.Data, icon) {
		t.Fatal("validated stale icon was not preserved")
	}

	if err := os.Remove(cache.iconPath(id)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cache.metadataPath(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), id); !errors.Is(err, ErrSiteIconUnavailable) {
		t.Fatalf("stale discovery without a cache returned %v", err)
	}
}

func TestIconCacheLimitsGlobalConcurrentFetches(t *testing.T) {
	items := make([]contract.SiteSummary, 0, 8)
	for index := 0; index < 8; index++ {
		character := string(rune('0' + index))
		id := strings.Repeat(character, 32)
		site := testIconSite(id)
		site.PrimaryDomain = "site-" + character + ".example.com"
		site.Domains = []string{site.PrimaryDomain}
		items = append(items, site)
	}
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		return items, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	started := make(chan struct{}, len(items))
	var active atomic.Int32
	var maximum atomic.Int32
	cache.client = &http.Client{Transport: iconRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		if request.URL.Path == "/" {
			started <- struct{}{}
			<-release
		}
		return iconResponse(http.StatusNotFound, "text/plain", nil), nil
	})}

	results := make(chan error, len(items))
	for _, item := range items {
		go func(id string) {
			_, getErr := cache.Get(context.Background(), id)
			results <- getErr
		}(item.ID)
	}
	for range 4 {
		<-started
	}
	select {
	case <-started:
		t.Fatal("more than four site fetches started concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range items {
		if getErr := <-results; !errors.Is(getErr, ErrSiteIconNotFound) {
			t.Fatalf("fetch result = %v", getErr)
		}
	}
	if maximum.Load() > 4 {
		t.Fatalf("maximum concurrent fetches = %d", maximum.Load())
	}
}

func TestLocalSiteIconTransportRejectsNonWebPorts(t *testing.T) {
	for address, expected := range map[string]string{
		"evil.example:80":  "127.0.0.1:80",
		"evil.example:443": "127.0.0.1:443",
	} {
		if got, err := localSiteIconDialAddress(address); err != nil || got != expected {
			t.Fatalf("local dial target for %s = %q, %v", address, got, err)
		}
	}
	request, err := http.NewRequest(http.MethodGet, "http://example.com:8080/favicon.ico", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newLocalSiteIconClient().Do(request)
	if err == nil || !strings.Contains(err.Error(), "non-web destination") {
		t.Fatalf("non-web destination error = %v", err)
	}
}

func TestIconCacheDefaultTransportUsesRealLoopbackHTTP(t *testing.T) {
	if os.Getenv("KPANEL_SITE_ICON_LOOPBACK_TEST") != "1" {
		t.Skip("set KPANEL_SITE_ICON_LOOPBACK_TEST=1 in an isolated Linux network namespace")
	}
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	listener, err := net.Listen("tcp4", "127.0.0.1:80")
	if err != nil {
		t.Fatalf("listen on isolated loopback port 80: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	icon := testIconPNG(t, 32, 32)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.Host != "fixture.example" {
			t.Errorf("Host header = %q", request.Host)
			response.WriteHeader(http.StatusMisdirectedRequest)
			return
		}
		if request.URL.Path == "/icon.png" {
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write(icon)
			return
		}
		_, _ = response.Write([]byte(`<link rel="icon" href="/icon.png">`))
	})
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	id := strings.Repeat("9", 32)
	site := testIconSite(id)
	site.PrimaryDomain = "fixture.example"
	site.Domains = []string{"fixture.example"}
	cache, err := NewIconCache(t.TempDir(), func() ([]contract.SiteSummary, error) {
		return []contract.SiteSummary{site}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := cache.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "image/png" || !bytes.Equal(got.Data, icon) {
		t.Fatal("real loopback transport did not return the fixture icon")
	}
}

func newTestIconCache(
	t *testing.T,
	directory string,
	now *time.Time,
	site contract.SiteSummary,
	roundTrip iconRoundTripFunc,
) *IconCache {
	t.Helper()
	cache, err := NewIconCache(directory, func() ([]contract.SiteSummary, error) {
		return []contract.SiteSummary{site}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cache.now = func() time.Time { return *now }
	cache.client = &http.Client{
		Transport: roundTrip,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return cache
}

func testIconSite(id string) contract.SiteSummary {
	return contract.SiteSummary{
		ID:            id,
		PrimaryDomain: "example.com",
		Domains:       []string{"example.com"},
		Enabled:       true,
		TLS:           contract.TLSStatus{Enabled: false, Status: "disabled"},
	}
}

func testIconPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func testPNGICO(t *testing.T, payload []byte, width, height int) []byte {
	t.Helper()
	if width < 1 || width > 256 || height < 1 || height > 256 {
		t.Fatalf("invalid ICO directory dimensions: %dx%d", width, height)
	}
	result := make([]byte, 22+len(payload))
	binary.LittleEndian.PutUint16(result[2:4], 1)
	binary.LittleEndian.PutUint16(result[4:6], 1)
	if width < 256 {
		result[6] = byte(width)
	}
	if height < 256 {
		result[7] = byte(height)
	}
	binary.LittleEndian.PutUint16(result[10:12], 1)
	binary.LittleEndian.PutUint16(result[12:14], 32)
	binary.LittleEndian.PutUint32(result[14:18], uint32(len(payload)))
	binary.LittleEndian.PutUint32(result[18:22], 22)
	copy(result[22:], payload)
	return result
}

func iconResponse(status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestIconCachePrunesOldestBoundedEntries(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	cache, err := NewIconCache(directory, func() ([]contract.SiteSummary, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	cache.now = func() time.Time { return now }
	cache.maxEntries = 2
	icon := testIconPNG(t, 8, 8)
	for index, character := range []string{"1", "2", "3"} {
		id := strings.Repeat(character, 32)
		metadata := siteIconMetadata{
			Version:       siteIconCacheVersion,
			SourceKey:     strings.Repeat(character, 64),
			ContentSHA256: hashSiteIcon(icon),
			FetchedAt:     now.Add(time.Duration(index) * time.Hour),
		}
		if err := cache.writeSuccess(id, SiteIcon{ContentType: "image/png", Data: icon}, metadata); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, strings.Repeat("1", 32)+".meta.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oldest entry was not pruned: %v", err)
	}
	for _, character := range []string{"2", "3"} {
		if _, err := os.Stat(filepath.Join(directory, strings.Repeat(character, 32)+".meta.json")); err != nil {
			t.Fatalf("retained entry %s missing: %v", character, err)
		}
	}
}

func TestIconCachePrunesAbandonedArtifactsAfterGracePeriod(t *testing.T) {
	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	cache, err := NewIconCache(directory, func() ([]contract.SiteSummary, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	cache.now = func() time.Time { return now }
	old := now.Add(-2 * siteIconOrphanGrace)
	orphan := cache.iconPath(strings.Repeat("7", 32))
	temporary := filepath.Join(directory, ".site-icon-abandoned")
	for _, path := range []string{orphan, temporary} {
		if err := os.WriteFile(path, []byte("abandoned"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	cache.prune()
	for _, path := range []string{orphan, temporary} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("abandoned artifact was not pruned: %s (%v)", path, err)
		}
	}
}
