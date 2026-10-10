package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/offers"
)

func offersTestService(t *testing.T) (*offers.Service, string) {
	t.Helper()
	var banner bytes.Buffer
	if err := png.Encode(&banner, image.NewRGBA(image.Rect(0, 0, offers.CardWidth, offers.CardHeight))); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(banner.Bytes())
	digest := hex.EncodeToString(sum[:])
	manifest, err := json.Marshal(offers.Manifest{
		SchemaVersion: 1,
		UpdatedAt:     time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		Items: []offers.Item{{
			ID: "racknerd", Vendor: "RackNerd", Alt: "美国 VPS 年付特价",
			Images: offers.Images{Card: offers.Image{Path: "offers/racknerd.png", SHA256: digest}},
			URL:    "https://my.racknerd.com/aff.php?aff=5501",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		offers.ManifestURL: manifest,
		"https://app.kejilion.sh/offers/racknerd.png": banner.Bytes(),
	}
	fetch := func(_ context.Context, address string, _ int64) ([]byte, string, error) {
		data, ok := files[address]
		if !ok {
			return nil, "", errors.New("HTTP 404")
		}
		if address == offers.ManifestURL {
			return data, "application/json", nil
		}
		return data, "image/png", nil
	}
	service := offers.Open(filepath.Join(t.TempDir(), "offers"), fetch)
	t.Cleanup(service.Close)
	return service, digest
}

func TestOffersAPIRequiresSessionAndServesCachedBanners(t *testing.T) {
	server, tokenPath := newTestServer(t)
	service, digest := offersTestService(t)
	server.offers.Close()
	server.offers = service
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	imagePath := offers.MediaPrefix + digest

	for _, path := range []string{offersPath, imagePath} {
		if response := performRequest(server, http.MethodGet, path, nil, nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d", path, response.Code)
		}
	}
	if response := authenticatedRequest(server, http.MethodPost, offersPath, nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST offers = %d", response.Code)
	}
	for _, path := range []string{offersPath + "?refresh=yes", offersPath + "?url=https://evil.test/", imagePath + "?v=1"} {
		if response := authenticatedRequest(server, http.MethodGet, path, nil, sessionCookie, csrfCookie, nil); response.Code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d", path, response.Code)
		}
	}

	response := authenticatedRequest(server, http.MethodGet, offersPath+"?refresh=1", nil, sessionCookie, csrfCookie, nil)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET offers = %d %q: %s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var view offers.View
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.State != offers.StateLive || len(view.Items) != 1 || view.Items[0].Card != imagePath || view.Items[0].Host != "my.racknerd.com" {
		t.Fatalf("offers view = %+v", view)
	}

	response = authenticatedRequest(server, http.MethodGet, imagePath, nil, sessionCookie, csrfCookie, nil)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" ||
		response.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("GET image = %d %v", response.Code, response.Header())
	}
	response = authenticatedRequest(server, http.MethodGet, imagePath, nil, sessionCookie, csrfCookie, map[string]string{"If-None-Match": `"` + digest + `"`})
	if response.Code != http.StatusNotModified {
		t.Fatalf("conditional image = %d", response.Code)
	}
	for _, path := range []string{offers.MediaPrefix + "0000", offers.MediaPrefix + "../state.json", offers.MediaPrefix} {
		if response := authenticatedRequest(server, http.MethodGet, path, nil, sessionCookie, csrfCookie, nil); response.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d", path, response.Code)
		}
	}
}
