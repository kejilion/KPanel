package panel

import (
	"encoding/json"
	"github.com/kejilion/kejilion-panel/internal/cinemametadata"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestCinemaRouteBoundaries(t *testing.T) {
	s, token := newTestServer(t)
	for _, path := range []string{cinemaMetadataPath, cinemaImagePath + "?provider=tmdb&path=%2Fa.jpg&size=poster"} {
		method := http.MethodGet
		if path == cinemaMetadataPath {
			method = http.MethodPost
		}
		if r := performRequest(s, method, path, nil, nil); r.Code != 401 {
			t.Fatalf("missing session %s: %d", path, r.Code)
		}
	}
	session, csrf := bootstrapCookies(t, s, token)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	body := []byte(`{"provider":"tmdb","title":"Inception","kind":"movie","language":"en-US"}`)
	if r := authenticatedRequest(s, http.MethodPost, cinemaMetadataPath, body, session, &http.Cookie{Name: "absent", Value: ""}, nil); r.Code != 403 {
		t.Fatalf("missing CSRF: %d", r.Code)
	}
	if r := authenticatedRequest(s, http.MethodPost, cinemaMetadataPath, body, session, csrf, map[string]string{"Origin": "https://evil.test"}); r.Code != 403 {
		t.Fatalf("cross origin: %d", r.Code)
	}
	if r := authenticatedRequest(s, http.MethodPost, cinemaMetadataPath, body, session, csrf, headers); r.Code != 400 {
		t.Fatalf("missing credential: %d %s", r.Code, r.Body)
	}
	for _, path := range []string{"http://127.0.0.1/secret.jpg", "//evil.test/a.jpg", "/../a.jpg", "/a.svg"} {
		r := authenticatedRequest(s, http.MethodGet, cinemaImagePath+"?provider=tmdb&size=poster&path="+path, nil, session, csrf, nil)
		if r.Code != 400 {
			t.Fatalf("image accepted: %d %s", r.Code, path)
		}
	}
	oversized := []byte(`{"title":"` + strings.Repeat("a", 5000) + `"}`)
	if r := authenticatedRequest(s, http.MethodPost, cinemaMetadataPath, oversized, session, csrf, headers); r.Code < 400 {
		t.Fatalf("oversized accepted: %d", r.Code)
	}
}

func TestCinemaLiveRoute(t *testing.T) {
	if os.Getenv("KPANEL_TEST_LIVE_CINEMA") != "1" {
		t.Skip("opt-in live route verification")
	}
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	headers := map[string]string{"Origin": "http://panel.test", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value}
	for _, query := range []cinemametadata.Query{
		{Provider: "wikipedia", Title: "Inception", Kind: "movie", Language: "en-US"},
		{Provider: "wikipedia", Title: "盗梦空间", Kind: "movie", Language: "zh-CN"},
		{Provider: "tvmaze", Title: "The Expanse", Kind: "tv", Language: "en-US"},
	} {
		t.Run(query.Provider+"/"+query.Language, func(t *testing.T) {
			body, _ := json.Marshal(query)
			r := authenticatedRequest(s, http.MethodPost, cinemaMetadataPath, body, session, csrf, headers)
			if r.Code != 200 {
				t.Fatalf("metadata %d %s", r.Code, r.Body)
			}
			var envelope struct {
				Data cinemametadata.Result `json:"data"`
			}
			if json.Unmarshal(r.Body.Bytes(), &envelope) != nil || len(envelope.Data.Items) == 0 {
				t.Fatalf("response %s", r.Body)
			}
			var item cinemametadata.Item
			for _, candidate := range envelope.Data.Items {
				if candidate.Poster != "" {
					item = candidate
					break
				}
			}
			if item.Poster == "" {
				t.Fatal("no poster returned")
			}
			params := url.Values{"provider": {item.Provider}, "path": {item.Poster}, "size": {"poster"}}
			image := authenticatedRequest(s, http.MethodGet, cinemaImagePath+"?"+params.Encode(), nil, session, csrf, nil)
			if image.Code != 200 || !strings.HasPrefix(image.Header().Get("Content-Type"), "image/") {
				t.Fatalf("image %d %s", image.Code, image.Body)
			}
			t.Logf("title=%s year=%s image=%s bytes=%d", item.Title, item.Year, image.Header().Get("Content-Type"), image.Body.Len())
		})
	}
}
