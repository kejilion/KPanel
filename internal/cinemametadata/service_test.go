package cinemametadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestWikipediaReleaseYearsAndHardResultCap(t *testing.T) {
	for _, v := range []struct{ intro, year string }{{"Blade Runner 2049 is a 2017 American film", "2017"}, {"2001: A Space Odyssey is a 1968 film", "1968"}, {"1917 is a 2019 war film", "2019"}, {"《盗梦空间》是一部於2010年上映的电影", "2010"}, {"A story about 2001 and 1999", ""}} {
		if got := wikipediaYear(v.intro); got != v.year {
			t.Fatalf("intro=%s year=%s", v.intro, got)
		}
	}
	s := New()
	pages := make([]map[string]any, 1000)
	for i := range pages {
		pages[i] = map[string]any{"pageid": i + 1, "title": "title", "extract": "A film"}
	}
	body, _ := json.Marshal(map[string]any{"query": map[string]any{"pages": pages}})
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return fixture(string(body)), nil })
	result, err := s.Search(context.Background(), query("wikipedia"))
	if err != nil || len(result.Items) != 5 {
		t.Fatalf("result cap=%d err=%v", len(result.Items), err)
	}
}
func TestImageQueueDoesNotStarveQueries(t *testing.T) {
	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan struct{}, 2)
	finish := make(chan struct{})
	defer close(finish)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "image.tmdb.org" {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-finish:
				return fixture(b.String()), nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return fixture(`{"query":{"pages":[]}}`), nil
	})
	results := make(chan error, 3)
	for i := 0; i < 2; i++ {
		go func() { _, _, err := s.Image(ctx, "tmdb", "/p.png", "poster"); results <- err }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("image requests never started")
		}
	}
	third, cancelThird := context.WithCancel(ctx)
	go func() { _, _, err := s.Image(third, "tmdb", "/p.png", "poster"); results <- err }()
	if _, err := s.Search(ctx, query("wikipedia")); err != nil {
		t.Fatalf("images starved metadata: %v", err)
	}
	cancelThird()
	select {
	case err := <-results:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued image rejected instead of waiting: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("queued cancellation did not finish")
	}
}
func query(provider string) Query {
	kind := "movie"
	if provider == "tvmaze" {
		kind = "tv"
	}
	return Query{Provider: provider, Title: "Inception", Kind: kind, Language: "en-US"}
}
func TestValidationAndImageTargets(t *testing.T) {
	q := query("wikipedia")
	if !q.Valid() {
		t.Fatal("valid query rejected")
	}
	for _, v := range []Query{{Provider: "evil", Title: "x", Kind: "movie", Language: "en-US"}, {Provider: "tmdb", Title: "x", Kind: "movie", Language: "en-US"}, {Provider: "wikipedia", Title: "x\n", Kind: "movie", Language: "en-US"}, {Provider: "tvmaze", Title: "x", Kind: "movie", Language: "en-US"}} {
		if v.Valid() {
			t.Fatalf("invalid query accepted: %+v", v)
		}
	}
	for _, p := range []string{"//evil.test/x.jpg", "/../../x.jpg", "/x.svg", "/x.jpg?url=http://127.0.0.1", "/x.jpg#secret", "/x%2f.jpg"} {
		if ImageURL("tmdb", p, "poster") != "" {
			t.Fatal(p)
		}
	}
	if ImageURL("tmdb", "/poster.jpg", "poster") != "https://image.tmdb.org/t/p/w500/poster.jpg" {
		t.Fatal("fixed image target")
	}
	if imagePath("tvmaze", "https://static.tvmaze.com/uploads/images/medium_portrait/1/2.jpg") == "" {
		t.Fatal("TVmaze image rejected")
	}
	if imagePath("tvmaze", "https://evil.test/uploads/images/medium_portrait/1/2.jpg") != "" {
		t.Fatal("unsafe image host")
	}
}
func TestTMDBSearchDetailCredentialsAndCache(t *testing.T) {
	s := New()
	var calls atomic.Int32
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != "api.themoviedb.org" || r.URL.Query().Get("include_adult") != "false" {
			t.Fatal(r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test.read.access.token" {
			t.Fatal("missing bearer")
		}
		if strings.HasPrefix(r.URL.Path, "/3/search/") {
			return fixture(`{"results":[{"id":27205,"title":"Inception","original_title":"Inception","release_date":"2010-07-16","overview":"A dream","poster_path":"/p.jpg","backdrop_path":"//evil/x.jpg","vote_average":8.3}]}`), nil
		}
		if r.URL.Query().Get("append_to_response") != "credits" {
			t.Fatal("missing credits")
		}
		return fixture(`{"id":27205,"title":"Inception","genres":[{"name":"Sci-Fi"}],"credits":{"cast":[{"name":"Leonardo DiCaprio"}]}}`), nil
	})
	q := query("tmdb")
	q.Credential = "test.read.access.token"
	result, err := s.Search(context.Background(), q)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	item := result.Items[0]
	if item.Year != "2010" || item.Backdrop != "" || item.Poster != "/p.jpg" {
		t.Fatalf("%+v", item)
	}
	_, _ = s.Search(context.Background(), q)
	if calls.Load() != 1 {
		t.Fatal("cache miss")
	}
	q.ID = 27205
	result, err = s.Search(context.Background(), q)
	if err != nil || len(result.Items[0].Cast) != 1 || len(result.Items[0].Genres) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	q.Credential = "another.read.access.token"
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("secret"))}, nil
	})
	if _, err = s.Search(context.Background(), q); !errors.Is(err, ErrCredentials) {
		t.Fatal("credential-scoped cache", err)
	}
}
func TestKeyTVmazeWikipediaAndSourceFailures(t *testing.T) {
	s := New()
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.themoviedb.org" {
			if r.URL.Query().Get("api_key") != strings.Repeat("a", 32) || r.Header.Get("Authorization") != "" {
				t.Fatal("API key authentication")
			}
			return fixture(`{"results":[]}`), nil
		}
		if r.URL.Host == "api.tvmaze.com" {
			return fixture(`[{"show":{"id":1,"name":"Under the Dome","summary":"<p>A &amp; B</p>","genres":["Drama"],"premiered":"2013-06-24","rating":{"average":6.5},"image":{"medium":"https://static.tvmaze.com/uploads/images/medium_portrait/1/2.jpg"}}}]`), nil
		}
		if r.URL.Host != "zh.wikipedia.org" || r.URL.Query().Get("gsrlimit") != "5" {
			t.Fatal(r.URL)
		}
		return fixture(`{"query":{"pages":[{"pageid":42,"title":"盗梦空间","extract":"简介","thumbnail":{"source":"https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/a.jpg/500px-a.jpg"}}]}}`), nil
	})
	q := query("tmdb")
	q.Credential = strings.Repeat("a", 32)
	if _, err := s.Search(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	result, err := s.Search(context.Background(), query("tvmaze"))
	if err != nil || len(result.Items) != 1 || strings.Contains(result.Items[0].Overview, "<p>") || !strings.Contains(result.Items[0].Overview, "A & B") {
		t.Fatalf("%+v %v", result, err)
	}
	q = query("wikipedia")
	q.Language = "zh-CN"
	result, err = s.Search(context.Background(), q)
	if err != nil || result.Items[0].ID != 42 || result.Items[0].Poster == "" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, body := range []string{"invalid", `{"error":{"info":"private error"}}`, strings.Repeat("x", (1<<20)+1)} {
		s = New()
		s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return fixture(body), nil })
		if _, err = s.Search(context.Background(), query("wikipedia")); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
func TestImageGateCancellationAndCacheBound(t *testing.T) {
	s := New()
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("image leaked credential")
		}
		return fixture(b.String()), nil
	})
	data, kind, err := s.Image(context.Background(), "tmdb", "/a.png", "poster")
	if err != nil || len(data) == 0 || kind != "image/png" {
		t.Fatalf("%s %v", kind, err)
	}
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return fixture("<svg onload=evil>"), nil })
	if _, _, err = s.Image(context.Background(), "tmdb", "/a.png", "poster"); err == nil {
		t.Fatal("active content accepted")
	}
	s.gate <- struct{}{}
	s.gate <- struct{}{}
	if _, err = s.Search(context.Background(), query("wikipedia")); !errors.Is(err, ErrBusy) {
		t.Fatal("gate", err)
	}
	<-s.gate
	<-s.gate
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		return fixture(`{"query":{"pages":[]}}`), nil
	})
	for i := 0; i < 70; i++ {
		q := query("wikipedia")
		q.Title = strings.Repeat("a", i+1)
		if _, err = s.Search(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.cache) > 64 {
		t.Fatal("unbounded cache")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := query("wikipedia")
	q.Title = "cancelled"
	if _, err = s.Search(ctx, q); err == nil {
		t.Fatal("cancelled request accepted")
	}
}
func TestLiveCinemaSources(t *testing.T) {
	if os.Getenv("KPANEL_TEST_LIVE_CINEMA") != "1" {
		t.Skip("opt-in live source verification")
	}
	for _, source := range []string{"tvmaze", "wikipedia"} {
		t.Run(source, func(t *testing.T) {
			s := New()
			q := query(source)
			if source == "tvmaze" {
				q.Title = "The Expanse"
			}
			result, err := s.Search(context.Background(), q)
			if err != nil || len(result.Items) == 0 {
				t.Fatalf("source=%s items=%d err=%v", source, len(result.Items), err)
			}
			item := result.Items[0]
			if source == "wikipedia" && (item.Title != "Inception" || item.Year != "2010" || item.Poster == "") {
				t.Fatalf("movie match incomplete: %+v", item)
			}
			data, _ := json.Marshal(item)
			t.Log(string(data))
			if item.Poster != "" {
				body, kind, err := s.Image(context.Background(), source, item.Poster, "poster")
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("image=%s bytes=%d", kind, len(body))
			}
		})
	}
}
