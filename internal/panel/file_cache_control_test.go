package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFileContentCacheControlOnlyCachesVersionedThumbnails(t *testing.T) {
	cases := []struct {
		target string
		status int
		want   string
	}{
		{"/api/v1/files/content?path=%2Fa.png&mode=thumbnail&version=v1", http.StatusOK, "private, max-age=3600"},
		{"/api/v1/files/content?path=%2Fa.png&mode=thumbnail&version=v1", http.StatusNotModified, "private, max-age=3600"},
		{"/api/v1/files/content?path=%2Fa.png&mode=thumbnail&version=v1", http.StatusNotFound, "private, no-store"},
		{"/api/v1/files/content?path=%2Fa.png&mode=thumbnail", http.StatusOK, "private, no-store"},
		{"/api/v1/files/content?path=%2Fa.png&disposition=inline", http.StatusOK, "private, no-store"},
		{"/api/v1/files/content?path=%2Fa.txt&mode=text&version=v1", http.StatusOK, "private, no-store"},
	}
	for _, item := range cases {
		recorder := httptest.NewRecorder()
		setFileContentCacheControl(recorder, httptest.NewRequest(http.MethodGet, item.target, nil), item.status)
		if got := recorder.Header().Get("Cache-Control"); got != item.want {
			t.Fatalf("%s (%d): Cache-Control = %q, want %q", item.target, item.status, got, item.want)
		}
		if item.want == "private, no-store" && recorder.Header().Get("Pragma") != "no-cache" {
			t.Fatalf("%s: uncached response lost Pragma", item.target)
		}
	}
}
