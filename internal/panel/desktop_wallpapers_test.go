package panel

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/desktopwallpapers"
)

func wallpaperJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{28, 58, 74, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// wallpaperUpload builds a multipart body; a nil part is left out.
func wallpaperUpload(t *testing.T, metadata string, imageData, thumbData []byte, extra ...string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if metadata != "" {
		_ = writer.WriteField("metadata", metadata)
	}
	for name, data := range map[string][]byte{"image": imageData, "thumb": thumbData} {
		if data == nil {
			continue
		}
		part, err := writer.CreateFormFile(name, name+".jpg")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	for _, name := range extra {
		_ = writer.WriteField(name, "x")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestDesktopWallpaperAPIUploadServeAndDelete(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	image, thumb := wallpaperJPEG(t, 320, 180), wallpaperJPEG(t, 64, 36)
	metadata := `{"name":"山谷日落","focusX":700,"focusY":320,"theme":{"brand":"#e8b86a","neutral":"#1c3a4a","signature":"#4f8fa8"}}`
	body, contentType := wallpaperUpload(t, metadata, image, thumb)
	headers := map[string]string{"Content-Type": contentType, "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}

	if response := performRequest(server, http.MethodGet, desktopWallpapersPath, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodPost, desktopWallpapersPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": contentType, "Origin": "http://panel.test",
	}); response.Code != http.StatusForbidden {
		t.Fatalf("upload without CSRF = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodPost, desktopWallpapersPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": contentType, "Origin": "http://evil.test", "X-CSRF-Token": csrfCookie.Value,
	}); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin upload = %d", response.Code)
	}

	created := authenticatedRequest(server, http.MethodPost, desktopWallpapersPath, body, sessionCookie, csrfCookie, headers)
	if created.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", created.Code, created.Body.String())
	}
	var wallpaper struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Format   string `json:"format"`
		Width    int    `json:"width"`
		ImageURL string `json:"imageURL"`
		ThumbURL string `json:"thumbURL"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &wallpaper); err != nil {
		t.Fatal(err)
	}
	if !desktopwallpapers.ValidID(wallpaper.ID) || wallpaper.Name != "山谷日落" || wallpaper.Format != "jpeg" || wallpaper.Width != 320 ||
		wallpaper.ImageURL != desktopWallpapersPath+"/"+wallpaper.ID+"/image" {
		t.Fatalf("created wallpaper = %s", created.Body.String())
	}

	listed := authenticatedRequest(server, http.MethodGet, desktopWallpapersPath, nil, sessionCookie, csrfCookie, nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), wallpaper.ID) || !strings.Contains(listed.Body.String(), `"maxCount":12`) {
		t.Fatalf("list = %d %s", listed.Code, listed.Body.String())
	}

	served := authenticatedRequest(server, http.MethodGet, wallpaper.ImageURL, nil, sessionCookie, csrfCookie, nil)
	if served.Code != http.StatusOK || served.Header().Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(served.Header().Get("Cache-Control"), "immutable") || served.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("image = %d %v", served.Code, served.Header())
	}
	revalidated := authenticatedRequest(server, http.MethodGet, wallpaper.ThumbURL, nil, sessionCookie, csrfCookie, map[string]string{
		"If-None-Match": `"` + wallpaper.ID + `-thumb"`,
	})
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("thumb revalidation = %d", revalidated.Code)
	}
	if response := performRequest(server, http.MethodGet, wallpaper.ImageURL, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous image = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodGet, wallpaper.ImageURL+"?v=1", nil, sessionCookie, csrfCookie, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("image with query = %d", response.Code)
	}

	deleteHeaders := map[string]string{"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	itemPath := desktopWallpapersPath + "/" + wallpaper.ID
	if response := authenticatedRequest(server, http.MethodDelete, itemPath, nil, sessionCookie, csrfCookie, deleteHeaders); response.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
	if response := authenticatedRequest(server, http.MethodGet, wallpaper.ImageURL, nil, sessionCookie, csrfCookie, nil); response.Code != http.StatusNotFound {
		t.Fatalf("deleted image = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodDelete, itemPath, nil, sessionCookie, csrfCookie, deleteHeaders); response.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodPut, itemPath, nil, sessionCookie, csrfCookie, deleteHeaders); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT item = %d", response.Code)
	}
}

func TestDesktopWallpaperAPIRejectsMalformedUploads(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	image, thumb := wallpaperJPEG(t, 320, 180), wallpaperJPEG(t, 64, 36)
	metadata := `{"name":"海边","focusX":500,"focusY":500}`
	post := func(body []byte, contentType string) int {
		return authenticatedRequest(server, http.MethodPost, desktopWallpapersPath, body, sessionCookie, csrfCookie, map[string]string{
			"Content-Type": contentType, "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		}).Code
	}

	if code := post(image, "image/jpeg"); code != http.StatusUnsupportedMediaType {
		t.Errorf("raw image = %d", code)
	}
	missing, contentType := wallpaperUpload(t, metadata, image, nil)
	if code := post(missing, contentType); code != http.StatusBadRequest {
		t.Errorf("missing thumb = %d", code)
	}
	extra, contentType := wallpaperUpload(t, metadata, image, thumb, "path")
	if code := post(extra, contentType); code != http.StatusBadRequest {
		t.Errorf("unexpected part = %d", code)
	}
	unknownField, contentType := wallpaperUpload(t, `{"name":"a","focusX":0,"focusY":0,"path":"/etc"}`, image, thumb)
	if code := post(unknownField, contentType); code != http.StatusBadRequest {
		t.Errorf("unknown metadata field = %d", code)
	}
	badName, contentType := wallpaperUpload(t, `{"name":"","focusX":0,"focusY":0}`, image, thumb)
	if code := post(badName, contentType); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Errorf("empty name = %d", code)
	}
	svg, contentType := wallpaperUpload(t, metadata, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), thumb)
	if code := post(svg, contentType); code != http.StatusUnprocessableEntity {
		t.Errorf("svg = %d", code)
	}
	tooLarge, contentType := wallpaperUpload(t, metadata, bytes.Repeat([]byte{0xff}, desktopwallpapers.MaxImageBytes+1), thumb)
	if code := post(tooLarge, contentType); code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized image = %d", code)
	}

	server.desktopWallpaperUploads <- struct{}{}
	busy, contentType := wallpaperUpload(t, metadata, image, thumb)
	if code := post(busy, contentType); code != http.StatusTooManyRequests {
		t.Errorf("concurrent upload = %d", code)
	}
	<-server.desktopWallpaperUploads
}

func TestDesktopWallpapersAreNotPartOfPanelBackups(t *testing.T) {
	for _, name := range []string{"desktop-wallpapers/index.json", "desktop-wallpapers/files/0123456789abcdef0123456789abcdef.image"} {
		if panelBackupPath(name) {
			t.Fatalf("%s would be backed up", name)
		}
	}
}
