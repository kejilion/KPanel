package offers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func pngBanner(t *testing.T, width, height int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{shade, 40, 90, 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func jpegBanner(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type fakeOrigin struct {
	mu        sync.Mutex
	files     map[string][]byte
	types     map[string]string
	failures  map[string]error
	requested []string
}

func newFakeOrigin() *fakeOrigin {
	return &fakeOrigin{files: map[string][]byte{}, types: map[string]string{}, failures: map[string]error{}}
}

func (o *fakeOrigin) put(path string, data []byte, contentType string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.files["https://app.kejilion.sh/"+path] = data
	o.types["https://app.kejilion.sh/"+path] = contentType
}

func (o *fakeOrigin) fetch(_ context.Context, address string, limit int64) ([]byte, string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requested = append(o.requested, address)
	if err := o.failures[address]; err != nil {
		return nil, "", err
	}
	data, ok := o.files[address]
	if !ok {
		return nil, "", errors.New("HTTP 404")
	}
	if int64(len(data)) > limit {
		return nil, "", errors.New("too large")
	}
	return append([]byte(nil), data...), o.types[address], nil
}

func (o *fakeOrigin) count(prefix string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	total := 0
	for _, address := range o.requested {
		if strings.HasPrefix(address, prefix) {
			total++
		}
	}
	return total
}

type publication struct {
	manifest Manifest
	card     []byte
	wide     []byte
}

// publish places a two-item manifest (one featured) and its images.
func publish(t *testing.T, origin *fakeOrigin, shade uint8) publication {
	t.Helper()
	card := pngBanner(t, CardWidth, CardHeight, shade)
	wide := pngBanner(t, WideWidth, WideHeight, shade)
	other := jpegBanner(t, CardWidth, CardHeight)
	origin.put("offers/lcayun-card.png", card, "image/png")
	origin.put("offers/lcayun-wide.png", wide, "image/png")
	origin.put("offers/racknerd-card.jpg", other, "image/jpeg")
	manifest := Manifest{
		SchemaVersion: 1,
		UpdatedAt:     time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		Items: []Item{
			{
				ID: "lcayun", Vendor: "莱卡云", Alt: "香港 CN2 GIA 优惠活动", Featured: true,
				Images: Images{
					Card: Image{Path: "offers/lcayun-card.png", SHA256: digestOf(card)},
					Wide: &Image{Path: "offers/lcayun-wide.png", SHA256: digestOf(wide)},
				},
				URL: "https://www.lcayun.com/aff/ZEXUQBIM",
			},
			{
				ID: "racknerd", Vendor: "RackNerd", Alt: "美国 VPS 年付特价",
				Images: Images{Card: Image{Path: "offers/racknerd-card.jpg", SHA256: digestOf(other)}},
				URL:    "https://my.racknerd.com/aff.php?aff=5501&pid=879",
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	origin.put("offers/v1.json", data, "application/json; charset=utf-8")
	return publication{manifest: manifest, card: card, wide: wide}
}

func openTestService(t *testing.T, origin *fakeOrigin, now *time.Time) (*Service, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "offers")
	service := Open(root, origin.fetch)
	service.now = func() time.Time { return *now }
	t.Cleanup(service.Close)
	return service, root
}

func TestSnapshotDownloadsManifestAndServesPinnedImages(t *testing.T) {
	origin := newFakeOrigin()
	published := publish(t, origin, 10)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)

	view := service.Snapshot(context.Background(), false)
	if view.State != StateLive || len(view.Items) != 2 || view.FetchedAt == nil || !view.UpdatedAt.Equal(published.manifest.UpdatedAt) {
		t.Fatalf("view = %+v", view)
	}
	first := view.Items[0]
	if first.Host != "lcayun.com" || !first.Featured || first.Wide != MediaPrefix+digestOf(published.wide) ||
		first.Card != MediaPrefix+digestOf(published.card) || first.URL != "https://www.lcayun.com/aff/ZEXUQBIM" {
		t.Fatalf("featured item = %+v", first)
	}
	if view.Items[1].Wide != "" || view.Items[1].Host != "my.racknerd.com" {
		t.Fatalf("wall item = %+v", view.Items[1])
	}
	data, contentType, err := service.OpenImage(digestOf(published.wide))
	if err != nil || contentType != "image/png" || !bytes.Equal(data, published.wide) {
		t.Fatalf("OpenImage = %d bytes, %q, %v", len(data), contentType, err)
	}
	for _, digest := range []string{strings.Repeat("a", 64), "../state.json", strings.ToUpper(digestOf(published.card))} {
		if _, _, err := service.OpenImage(digest); !errors.Is(err, ErrNotFound) {
			t.Fatalf("OpenImage(%q) error = %v", digest, err)
		}
	}
}

func TestRefreshIsAllOrNothingAndKeepsPreviousPublication(t *testing.T) {
	origin := newFakeOrigin()
	first := publish(t, origin, 10)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)
	if view := service.Snapshot(context.Background(), false); view.State != StateLive {
		t.Fatalf("initial state = %q", view.State)
	}

	// A new publication whose wide image is the wrong size must not replace
	// the adopted one, and the view reports the cache as stale.
	second := publish(t, origin, 200)
	origin.put("offers/lcayun-wide.png", pngBanner(t, WideWidth, WideHeight+1, 200), "image/png")
	manifest := second.manifest
	manifest.Items[0].Images.Wide = &Image{Path: "offers/lcayun-wide.png", SHA256: digestOf(pngBanner(t, WideWidth, WideHeight+1, 200))}
	data, _ := json.Marshal(manifest)
	origin.put("offers/v1.json", data, "application/json")

	now = now.Add(ForcedRefreshInterval)
	view := service.Snapshot(context.Background(), true)
	if view.State != StateStale || len(view.Items) != 2 || view.Items[0].Card != MediaPrefix+digestOf(first.card) {
		t.Fatalf("view after rejected publication = %+v", view)
	}
	if _, _, err := service.OpenImage(digestOf(first.wide)); err != nil {
		t.Fatalf("previous wide image is no longer served: %v", err)
	}
}

func TestFailedRefreshPrunesUnadoptedImages(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "without-cache"
		if cached {
			name = "with-cache"
		}
		t.Run(name, func(t *testing.T) {
			origin := newFakeOrigin()
			first := publish(t, origin, 10)
			now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
			service, root := openTestService(t, origin, &now)
			keep := make(map[string]bool)
			if cached {
				if view := service.Snapshot(context.Background(), false); view.State != StateLive {
					t.Fatalf("initial state = %q", view.State)
				}
				for _, item := range first.manifest.Items {
					keep[item.Images.Card.SHA256] = true
					if item.Images.Wide != nil {
						keep[item.Images.Wide.SHA256] = true
					}
				}
			}
			origin.failures["https://app.kejilion.sh/offers/lcayun-wide.png"] = errors.New("incomplete publication")
			for shade := uint8(20); shade < 23; shade++ {
				rejected := publish(t, origin, shade)
				now = now.Add(ForcedRefreshInterval)
				view := service.Snapshot(context.Background(), true)
				wantState := StateUnavailable
				if cached {
					wantState = StateStale
					if len(view.Items) != 2 || view.Items[0].Card != MediaPrefix+digestOf(first.card) {
						t.Fatalf("rejected publication changed the adopted view: %+v", view)
					}
				}
				if view.State != wantState {
					t.Fatalf("state = %q, want %q", view.State, wantState)
				}
				objects, err := os.ReadDir(filepath.Join(root, objectsDirName))
				if err != nil || len(objects) != len(keep) {
					t.Fatalf("failed refresh retained %d objects, want %d: %v", len(objects), len(keep), err)
				}
				for _, object := range objects {
					if !keep[object.Name()] {
						t.Fatalf("unadopted object survived: %s", object.Name())
					}
				}
				if _, _, err := service.OpenImage(digestOf(rejected.card)); !errors.Is(err, ErrNotFound) {
					t.Fatalf("rejected image is accessible: %v", err)
				}
			}
			if cached {
				if _, _, err := service.OpenImage(digestOf(first.wide)); err != nil {
					t.Fatalf("adopted image was lost: %v", err)
				}
			}
		})
	}
}

func TestSnapshotWithoutCacheReportsUnavailableAndBacksOff(t *testing.T) {
	origin := newFakeOrigin()
	origin.failures["https://app.kejilion.sh/offers/v1.json"] = errors.New("network down")
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)

	if view := service.Snapshot(context.Background(), false); view.State != StateUnavailable || len(view.Items) != 0 {
		t.Fatalf("view = %+v", view)
	}
	service.Snapshot(context.Background(), false)
	if attempts := origin.count(ManifestURL); attempts != 1 {
		t.Fatalf("attempts inside retry window = %d", attempts)
	}
	now = now.Add(RetryInterval)
	delete(origin.failures, "https://app.kejilion.sh/offers/v1.json")
	publish(t, origin, 10)
	if view := service.Snapshot(context.Background(), false); view.State != StateLive || len(view.Items) != 2 {
		t.Fatalf("view after retry = %+v", view)
	}
}

func TestConcurrentFirstLoadsShareTheDownload(t *testing.T) {
	origin := newFakeOrigin()
	publish(t, origin, 10)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	fetch := func(ctx context.Context, address string, limit int64) ([]byte, string, error) {
		if address == ManifestURL {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		}
		return origin.fetch(ctx, address, limit)
	}
	service := Open(filepath.Join(t.TempDir(), "offers"), fetch)
	defer service.Close()

	results := make(chan View, 2)
	go func() { results <- service.Snapshot(context.Background(), false) }()
	<-started
	go func() { results <- service.Snapshot(context.Background(), false) }()
	// The second request arrives while the first download is still running.
	time.Sleep(50 * time.Millisecond)
	close(release)
	for range 2 {
		if view := <-results; view.State != StateLive || len(view.Items) != 2 {
			t.Fatalf("concurrent first load view = %+v", view)
		}
	}
	if attempts := origin.count(ManifestURL); attempts != 1 {
		t.Fatalf("manifest downloads = %d", attempts)
	}
}

func TestForcedRefreshIsRateLimited(t *testing.T) {
	origin := newFakeOrigin()
	publish(t, origin, 10)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)
	service.Snapshot(context.Background(), true)
	service.Snapshot(context.Background(), true)
	if attempts := origin.count(ManifestURL); attempts != 1 {
		t.Fatalf("forced refreshes inside one interval = %d", attempts)
	}
	now = now.Add(ForcedRefreshInterval)
	service.Snapshot(context.Background(), true)
	if attempts := origin.count(ManifestURL); attempts != 2 {
		t.Fatalf("forced refresh after interval attempts = %d", attempts)
	}
	// Unchanged images are reused from the cache instead of downloaded again.
	if images := origin.count("https://app.kejilion.sh/offers/lcayun"); images != 2 {
		t.Fatalf("image downloads = %d", images)
	}
}

func TestStaleCacheRefreshesInBackground(t *testing.T) {
	origin := newFakeOrigin()
	publish(t, origin, 10)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)
	service.Snapshot(context.Background(), false)

	now = now.Add(RefreshInterval)
	if view := service.Snapshot(context.Background(), false); view.State != StateLive || len(view.Items) != 2 {
		t.Fatalf("stale view = %+v", view)
	}
	deadline := time.Now().Add(5 * time.Second)
	for origin.count(ManifestURL) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("stale cache did not start a background refresh")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCacheSurvivesReopenAndPrunesUnreferencedImages(t *testing.T) {
	origin := newFakeOrigin()
	published := publish(t, origin, 10)
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, root := openTestService(t, origin, &now)
	service.Snapshot(context.Background(), false)
	service.Close()

	stray := filepath.Join(root, objectsDirName, strings.Repeat("b", 64))
	if err := os.WriteFile(stray, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	offline := newFakeOrigin()
	offline.failures[ManifestURL] = errors.New("offline")
	reopened := Open(root, offline.fetch)
	reopened.now = func() time.Time { return now }
	defer reopened.Close()
	view := reopened.Snapshot(context.Background(), false)
	if view.State != StateLive || len(view.Items) != 2 {
		t.Fatalf("reopened view = %+v", view)
	}
	if offline.count(ManifestURL) != 0 {
		t.Fatal("fresh cache should not be downloaded again after a restart")
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Fatalf("unreferenced cache object was kept: %v", err)
	}
	if _, _, err := reopened.OpenImage(digestOf(published.card)); err != nil {
		t.Fatalf("cached image after reopen: %v", err)
	}
}

func TestDamagedCacheIsDiscarded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "offers")
	if err := os.MkdirAll(filepath.Join(root, objectsDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stateFileName), []byte(`{"schema":1,"source":"https://evil.test/","fetchedAt":"2026-10-10T00:00:00Z","manifest":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	origin := newFakeOrigin()
	origin.failures[ManifestURL] = errors.New("offline")
	service := Open(root, origin.fetch)
	defer service.Close()
	if view := service.Snapshot(context.Background(), false); view.State != StateUnavailable || len(view.Items) != 0 {
		t.Fatalf("damaged cache view = %+v", view)
	}
}

func TestViewHonoursPublishingWindow(t *testing.T) {
	origin := newFakeOrigin()
	published := publish(t, origin, 10)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	until := time.Date(2026, 10, 31, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	manifest := published.manifest
	manifest.Items[1].ValidFrom, manifest.Items[1].ValidUntil = &from, &until
	data, _ := json.Marshal(manifest)
	origin.put("offers/v1.json", data, "application/json")

	now := from.Add(-time.Second)
	service, _ := openTestService(t, origin, &now)
	if view := service.Snapshot(context.Background(), false); len(view.Items) != 1 {
		t.Fatalf("items before window = %d", len(view.Items))
	}
	now = from
	if view := service.Snapshot(context.Background(), false); len(view.Items) != 2 {
		t.Fatalf("items inside window = %d", len(view.Items))
	}
	now = until
	if view := service.Snapshot(context.Background(), false); len(view.Items) != 1 {
		t.Fatalf("items after window = %d", len(view.Items))
	}
}

func TestDownloadRejectsWrongManifestContentTypeAndImageType(t *testing.T) {
	origin := newFakeOrigin()
	published := publish(t, origin, 10)
	data, _ := json.Marshal(published.manifest)
	origin.put("offers/v1.json", data, "text/html")
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	service, _ := openTestService(t, origin, &now)
	if view := service.Snapshot(context.Background(), false); view.State != StateUnavailable {
		t.Fatalf("html manifest view = %+v", view)
	}

	origin.put("offers/v1.json", data, "application/json")
	origin.put("offers/racknerd-card.jpg", jpegBanner(t, CardWidth, CardHeight), "image/png")
	now = now.Add(RetryInterval)
	if view := service.Snapshot(context.Background(), false); view.State != StateUnavailable {
		t.Fatalf("mislabelled image view = %+v", view)
	}
}
