package appmarket

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOfficialIconCacheFetchesOnceAndSurvivesRestart(t *testing.T) {
	data := testAppIconWebP(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/icons/deepseek-harness.webp" ||
			r.Header.Get("Accept") != "image/webp" {
			t.Errorf("unexpected icon request: path=%q accept=%q", r.URL.Path, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	dir := filepath.Join(t.TempDir(), "app-icons")
	cache, err := newOfficialIconCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	configureTestIconOrigin(t, cache, server)
	icon, err := cache.Get(context.Background(), "deepseek-harness", "icons/deepseek-harness.webp")
	if err != nil || !bytes.Equal(icon.Data, data) || icon.ContentType != "image/webp" ||
		len(icon.SHA256) != 64 {
		t.Fatalf("first icon = %#v, %v", icon, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("first fetch requests = %d", requests.Load())
	}

	reopened, err := newOfficialIconCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	configureTestIconOrigin(t, reopened, server)
	icon, err = reopened.Get(context.Background(), "deepseek-harness", "icons/deepseek-harness.webp")
	if err != nil || !bytes.Equal(icon.Data, data) || requests.Load() != 1 {
		t.Fatalf("reopened icon = %#v, %v; requests=%d", icon, err, requests.Load())
	}
	info, err := os.Lstat(filepath.Join(dir, "deepseek-harness.webp"))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("cached icon is unsafe: %#v, %v", info, err)
	}
	reopened.now = func() time.Time { return info.ModTime().Add(dynamicAppIconRefreshTTL + time.Second) }
	reopened.Prefetch(context.Background(), map[string]string{
		"deepseek-harness": "icons/deepseek-harness.webp",
	})
	if requests.Load() != 2 {
		t.Fatalf("stale icon refresh requests = %d, want 2", requests.Load())
	}
	if err := os.WriteFile(filepath.Join(dir, "removed-app.webp"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened.Prune(map[string]string{"deepseek-harness": "icons/deepseek-harness.webp"})
	if _, err := os.Stat(filepath.Join(dir, "removed-app.webp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphaned icon was not pruned: %v", err)
	}
}

func TestOfficialIconCacheCoalescesConcurrentMisses(t *testing.T) {
	data := testAppIconWebP(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	cache, err := newOfficialIconCache(filepath.Join(t.TempDir(), "app-icons"))
	if err != nil {
		t.Fatal(err)
	}
	configureTestIconOrigin(t, cache, server)

	start := make(chan struct{})
	var workers sync.WaitGroup
	errorsSeen := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, getErr := cache.Get(context.Background(), "concurrent-app", "icons/concurrent-app.webp")
			errorsSeen <- getErr
		}()
	}
	close(start)
	workers.Wait()
	close(errorsSeen)
	for getErr := range errorsSeen {
		if getErr != nil {
			t.Fatal(getErr)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("concurrent fetch requests = %d, want 1", requests.Load())
	}
}

func TestOfficialIconCacheRejectsUnsafeResponses(t *testing.T) {
	valid := testAppIconWebP(t)
	tests := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{name: "wrong content type", contentType: "text/plain", body: valid},
		{name: "invalid WebP", contentType: "image/webp", body: []byte("not-webp")},
		{name: "oversized", contentType: "image/webp", body: bytes.Repeat([]byte("x"), maxDynamicAppIconBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				_, _ = w.Write(test.body)
			}))
			defer server.Close()
			cache, err := newOfficialIconCache(filepath.Join(t.TempDir(), "app-icons"))
			if err != nil {
				t.Fatal(err)
			}
			configureTestIconOrigin(t, cache, server)
			if _, err := cache.Get(context.Background(), "unsafe-app", "icons/unsafe-app.webp"); !errors.Is(err, ErrAppIconUnavailable) {
				t.Fatalf("unsafe response error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(cache.dir, "unsafe-app.webp")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe response was cached: %v", err)
			}
		})
	}
}

func TestOfficialIconCacheRejectsCrossOriginRedirectAndUnlistedPath(t *testing.T) {
	data := testAppIconWebP(t)
	var destinationRequests atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationRequests.Add(1)
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(data)
	}))
	defer destination.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()
	cache, err := newOfficialIconCache(filepath.Join(t.TempDir(), "app-icons"))
	if err != nil {
		t.Fatal(err)
	}
	configureTestIconOrigin(t, cache, origin)
	if _, err := cache.Get(context.Background(), "redirected-app", "icons/redirected-app.webp"); !errors.Is(err, ErrAppIconUnavailable) {
		t.Fatalf("cross-origin redirect error = %v", err)
	}
	if destinationRequests.Load() != 0 {
		t.Fatalf("cross-origin destination received %d requests", destinationRequests.Load())
	}
	if _, err := cache.Get(context.Background(), "valid-app", "icons/another-app.webp"); !errors.Is(err, ErrAppIconNotFound) {
		t.Fatalf("mismatched icon path error = %v", err)
	}
}

func TestDynamicCatalogPrefetchesOfficialIconForBuiltinAndThirdParty(t *testing.T) {
	data := testAppIconWebP(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	embedded, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	payload := remotePayloadFromCatalog(embedded)
	payload.Apps = append(payload.Apps,
		App{
			ID: "builtin-117", Num: 117, Source: "builtin", Token: "new-builtin-app",
			NameZH: "新内置应用", NameEN: "New Builtin App", Category: "ai",
			Icon: "icons/new-builtin-app.webp", Slug: "new-builtin-app",
		},
		App{
			ID: "thirdparty-new-app", Source: "thirdparty", Token: "new-thirdparty-app",
			NameZH: "新第三方应用", NameEN: "New Third-party App", Category: "devprod",
			Icon: "icons/new-thirdparty-app.webp", Slug: "new-thirdparty-app",
		},
	)
	payload.Meta.Builtin++
	payload.Meta.ThirdParty++
	remote := Catalog{
		SchemaVersion: 1, Source: OfficialCatalogURL, Upstream: officialCatalogSource,
		Categories: payload.Categories, Apps: payload.Apps,
	}
	service, err := newService(&fakeDocker{}, t.TempDir(), func(context.Context) (Catalog, error) {
		return remote, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigureIconCache(filepath.Join(t.TempDir(), "app-icons")); err != nil {
		t.Fatal(err)
	}
	configureTestIconOrigin(t, service.iconCache, server)
	service.currentCatalog(context.Background())
	snapshot := waitForCatalogState(t, service, "live", "")
	for _, slug := range []string{"new-builtin-app", "new-thirdparty-app"} {
		app := appBySlug(t, snapshot.Catalog, slug)
		if app.Icon != dynamicAppIconPrefix+slug+".webp" || app.IconSHA256 != "" {
			t.Fatalf("dynamic app %q icon = %q hash=%q", slug, app.Icon, app.IconSHA256)
		}
		icon, iconErr := service.Icon(context.Background(), slug)
		if iconErr != nil || !bytes.Equal(icon.Data, data) {
			t.Fatalf("dynamic app %q cached icon error=%v", slug, iconErr)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("prefetch requests = %d, want one per new app", requests.Load())
	}
}

func configureTestIconOrigin(t *testing.T, cache *officialIconCache, server *httptest.Server) {
	t.Helper()
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	client := newOfficialIconHTTPClient(baseURL)
	client.Transport = server.Client().Transport
	cache.baseURL = baseURL
	cache.client = client
}

func appBySlug(t *testing.T, catalog Catalog, slug string) App {
	t.Helper()
	for _, app := range catalog.Apps {
		if app.Slug == slug {
			return app
		}
	}
	t.Fatalf("application slug %q was not found", slug)
	return App{}
}

func testAppIconWebP(t *testing.T) []byte {
	t.Helper()
	const value = "UklGRuYBAABXRUJQVlA4INoBAACwEQCdASqAAIAAPjEYikKiIaEVDZ1EIAMEsYBpcxr/ldjJ7TTTfNtnz/oH+A9gGcA/gH96/" +
		"YzfQOAz6XDyK9WXDLQb6QvFILrj5wHXz3hKCZnqb0FI9B+oEIVtzxuGnsTfC9YoV5+CHdc99m22g0MQROjt4970E1RDO2q4j" +
		"M/TSPZ7wR7+MBq6loHdS7xvQR0SiUSiTwAA/v/YoqY+31u2qXGJyh1JBhg4zb8eGEcC/sjAEPn7/CgbEdPtO420cqGbn5RDDVr" +
		"vrW17RVnTXO9l6epXmTFmXrvAEWimiYz2VBDidhyCNz9yw2w1llKjDSDTI2tW+n8aFErnLjjPfvXEgc4IYKrfvoAAdAKtgEBvi" +
		"ofE83Kw9YRF7DnwMrChnNbGEL69zKHuN+v+Tf9QZiAkltjO0J5KnIr9q8HDv71ZZRudr/wd34qSAVOw49GY5WvOsDcFdwZGR7" +
		"ti+Nv6YZII3Ev+aZls77Nw41TO96Z5AETMPYDO5PHBPE1uyygCXjBheCDBg4KZCVxKDcLipucRbOhnjd4w3veauvC434fehjx/" +
		"GGvgnx/BCDtQc2bs45Hy9hjsJU7TbQNUzt3QJ1txwjy6Df3CoHc5BLqVj7iBXJRAAAEp/JgAAAA="
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
