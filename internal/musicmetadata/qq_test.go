package musicmetadata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestQQFallbackAndValidation(t *testing.T) {
	for _, firstFails := range []bool{false, true} {
		s := New()
		s.client = openFunc(func(_ context.Context, u string) (*http.Response, error) {
			switch {
			case strings.Contains(u, "music.163.com"):
				if firstFails {
					return nil, errors.New("offline")
				}
				return response(`{"code":200,"result":{"songs":[]}}`), nil
			case strings.Contains(u, "/soso/"):
				return response(`{"code":0,"data":{"song":{"list":[{"songid":1,"songmid":"0039MnYb0qxYhV","songname":"Song","singer":[{"name":"Artist"}],"interval":200,"albummid":"000MkMni19ClKG"}]}}}`), nil
			case strings.Contains(u, "/lyric/"):
				return response(`{"code":0,"lyric":"[00:01]QQ &amp; lyric"}`), nil
			case u == "https://y.gtimg.cn/music/photo_new/T002R300x300M000000MkMni19ClKG.jpg":
				var buf bytes.Buffer
				_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
				return response(buf.String()), nil
			default:
				t.Fatalf("unexpected URL %s", u)
				return nil, errors.New("unexpected URL")
			}
		})
		r, err := s.Match(context.Background(), Query{Title: "Song", Artist: "Artist", Duration: 200})
		if err != nil || r.Source != "qq" || r.Lyrics != "[00:01]QQ & lyric" || r.Cover == "" {
			t.Fatalf("QQ fallback failed: source=%s err=%v", r.Source, err)
		}
	}
	for _, mid := range []string{"../etc/passwd", "https://evil.test", strings.Repeat("A", 100)} {
		if qqMID.MatchString(mid) {
			t.Fatalf("accepted unsafe MID %q", mid)
		}
	}
}

func TestArtistNamesWithSeparators(t *testing.T) {
	for _, artist := range []string{"AC/DC", "Simon & Garfunkel"} {
		s := song{ID: 1, Name: "Song", Duration: 200000}
		s.Artists = append(s.Artists, struct {
			Name string `json:"name"`
		}{artist})
		if score(Query{Title: "Song", Artist: artist, Duration: 200}, s) < 0 {
			t.Fatalf("rejected %q", artist)
		}
	}
}

func TestLiveQQ(t *testing.T) {
	if os.Getenv("KPANEL_MUSIC_LIVE_TEST") != "1" {
		t.Skip("opt-in live source probe")
	}
	q := Query{Title: "晴天", Artist: "周杰伦", Duration: 269}
	s := New()
	r, err := s.fetchQQ(context.Background(), q)
	if err != nil || r.Source != "qq" || r.Lyrics == "" || r.Cover == "" {
		t.Fatalf("QQ live source=%s lyrics=%d cover=%d err=%v", r.Source, len(r.Lyrics), len(r.Cover), err)
	}
	t.Logf("source=%s lyricsBytes=%d coverBytes=%d", r.Source, len(r.Lyrics), len(r.Cover))
}
