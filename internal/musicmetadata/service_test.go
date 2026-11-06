package musicmetadata

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
	"testing"
	"time"
)

type openFunc func(context.Context, string) (*http.Response, error)

func (f openFunc) Open(ctx context.Context, u string) (*http.Response, error) { return f(ctx, u) }
func response(s string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s))}
}
func fixture(t *testing.T) (*Service, *int) {
	t.Helper()
	calls := new(int)
	s := New()
	s.client = openFunc(func(ctx context.Context, u string) (*http.Response, error) {
		*calls++
		switch {
		case strings.Contains(u, "/search/get?"):
			return response(`{"code":200,"result":{"songs":[{"id":1,"name":"Song (Live)","duration":200000,"artists":[{"name":"Artist"}]},{"id":2,"name":"Song","duration":200000,"artists":[{"name":"Artist"}],"album":{"name":"Album"}}]}}`), nil
		case strings.Contains(u, "/song/lyric?id=2"):
			return response(`{"lrc":{"lyric":"[00:01.00]test"}}`), nil
		case strings.Contains(u, "/song/detail?"):
			return response(`{"songs":[{"id":2,"album":{"picUrl":"http://p1.music.126.net/test.png"}}]}`), nil
		case u == "https://p1.music.126.net/test.png?param=300y300":
			var buf bytes.Buffer
			_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
			return response(buf.String()), nil
		default:
			t.Fatalf("unexpected URL %q", u)
			return nil, errors.New("unexpected URL")
		}
	})
	return s, calls
}
func TestMatchAndCache(t *testing.T) {
	s, calls := fixture(t)
	q := Query{Title: "Song", Artist: "Artist", Album: "Album", Duration: 200}
	r, err := s.Match(context.Background(), q)
	if err != nil || !r.Matched || r.Lyrics != "[00:01.00]test" || !strings.HasPrefix(r.Cover, "data:image/png;base64,") {
		t.Fatalf("match failed: matched=%v err=%v", r.Matched, err)
	}
	_, err = s.Match(context.Background(), q)
	if err != nil || *calls != 4 {
		t.Fatalf("cache calls=%d err=%v", *calls, err)
	}
}
func TestRejectMismatchedVersionsAndArtists(t *testing.T) {
	s, _ := fixture(t)
	for _, q := range []Query{{Title: "Song", Artist: "Other", Duration: 200}, {Title: "Song", Artist: "Artist", Duration: 230}, {Title: "Song"}} {
		r, err := s.Match(context.Background(), q)
		if err != nil || r.Matched {
			t.Fatalf("unexpected match %+v %v", q, err)
		}
	}
}
func TestInputCoverAndResponseLimits(t *testing.T) {
	for _, q := range []Query{{Title: ""}, {Title: strings.Repeat("a", 257)}, {Title: "bad\nname"}, {Title: "Song", Duration: -1}} {
		if q.Valid() {
			t.Fatal("accepted invalid tags")
		}
	}
	for _, u := range []string{"http://127.0.0.1/x", "https://p1.music.126.net.evil.test/x", "https://p1.music.126.net:443/x", "https://user@p1.music.126.net/x", "file:///x"} {
		if safeCover(u) != "" {
			t.Fatalf("accepted %s", u)
		}
	}
	s := New()
	s.client = openFunc(func(context.Context, string) (*http.Response, error) {
		return response(strings.Repeat("x", (512<<10)+1)), nil
	})
	if _, err := s.Match(context.Background(), Query{Title: "Song", Artist: "Artist"}); err == nil {
		t.Fatal("accepted oversized JSON")
	}
	s.client = openFunc(func(context.Context, string) (*http.Response, error) {
		return response(`<svg onload="alert(1)"/>`), nil
	})
	if _, err := s.cover(context.Background(), "https://p1.music.126.net/x"); err == nil {
		t.Fatal("accepted SVG")
	}
}
func TestConcurrencyCancellationAndCacheBound(t *testing.T) {
	s, _ := fixture(t)
	s.active <- struct{}{}
	s.active <- struct{}{}
	if _, err := s.Match(context.Background(), Query{Title: "Song"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy: %v", err)
	}
	<-s.active
	<-s.active
	s.client = openFunc(func(ctx context.Context, _ string) (*http.Response, error) { <-ctx.Done(); return nil, ctx.Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Match(ctx, Query{Title: "Song"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error: %v", err)
	}
	s.client = openFunc(func(context.Context, string) (*http.Response, error) {
		return response(`{"code":200,"result":{"songs":[]}}`), nil
	})
	for i := range 20 {
		_, err := s.Match(context.Background(), Query{Title: strings.Repeat("s", i+1)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(s.cache) != 16 {
		t.Fatalf("cache entries=%d", len(s.cache))
	}
}
func TestPartialMetadataSurvives(t *testing.T) {
	s, _ := fixture(t)
	base := s.client
	s.client = openFunc(func(ctx context.Context, u string) (*http.Response, error) {
		if strings.Contains(u, "/song/detail?") {
			return nil, errors.New("offline")
		}
		return base.Open(ctx, u)
	})
	r, err := s.Match(context.Background(), Query{Title: "Song", Artist: "Artist"})
	if err != nil || r.Lyrics == "" || r.Cover != "" {
		t.Fatalf("partial result lost: %v", err)
	}
}

// Opt-in probe: source availability is distinct from deterministic matching tests.
func TestLiveNetEase(t *testing.T) {
	if os.Getenv("KPANEL_MUSIC_LIVE_TEST") != "1" {
		t.Skip("opt-in live source probe")
	}
	for _, q := range []Query{{Title: "十年", Artist: "陈奕迅", Duration: 205}, {Title: "倔强", Artist: "五月天", Duration: 261}} {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		r, err := New().Match(ctx, q)
		cancel()
		if err != nil || !r.Matched || r.Lyrics == "" || r.Cover == "" {
			t.Fatalf("live %s: matched=%v lyrics=%d cover=%d err=%v", q.Title, r.Matched, len(r.Lyrics), len(r.Cover), err)
		}
		b, _ := json.Marshal(map[string]any{"title": q.Title, "source": r.Source, "lyricsBytes": len(r.Lyrics), "coverBytes": len(r.Cover)})
		t.Log(string(b))
	}
}
