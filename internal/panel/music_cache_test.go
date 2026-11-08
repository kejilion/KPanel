package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/musiccache"
)

func TestMusicCacheAuthenticationBoundsAndDurability(t *testing.T) {
	s, token := newTestServer(t)
	if r := performRequest(s, http.MethodPost, musicCachePath, []byte(`{}`), nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous=%d", r.Code)
	}
	session, csrf := bootstrapCookies(t, s, token)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	input := `{"observation":{"ref":{"hostId":"","path":"/music/song.mp3","resourceVersion":"v1","sizeBytes":1234},"info":{"tags":{"title":"曲名"},"quality":{"bitrate":320}}}}`
	if r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(input), session, csrf, headers); r.Code != 200 {
		t.Fatalf("put=%d %s", r.Code, r.Body.String())
	}
	lookup := `{"refs":[{"hostId":"","path":"/music/song.mp3","resourceVersion":"v1","sizeBytes":1234}]}`
	r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(lookup), session, csrf, headers)
	var result struct {
		Items []musiccache.Record `json:"items"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if r.Code != 200 || len(result.Items) != 1 || result.Items[0].Info.Tags.Title != "曲名" || r.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("read=%d %s", r.Code, r.Body.String())
	}
	for _, body := range []string{`{}`, `{"refs":[]}`, strings.Replace(lookup, `"v1"`, `""`, 1), strings.Replace(input, `320`, `-1`, 1), strings.Replace(input, `"tags"`, `"unrecognized"`, 1), lookup + ` {}`} {
		if r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(body), session, csrf, headers); r.Code != 400 {
			t.Fatalf("invalid=%d %s", r.Code, r.Body.String())
		}
	}
	badOrigin := map[string]string{"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrf.Value}
	if r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(lookup), session, csrf, badOrigin); r.Code != 403 {
		t.Fatalf("origin=%d", r.Code)
	}
	if r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(lookup), session, csrf, map[string]string{"Origin": "http://panel.test", "Content-Type": "application/json"}); r.Code != 403 {
		t.Fatalf("csrf=%d", r.Code)
	}
	if r := authenticatedRequest(s, http.MethodPost, musicCachePath, []byte(`{"refs":["`+strings.Repeat("x", 260<<10)+`"]}`), session, csrf, headers); r.Code != 413 {
		t.Fatalf("size=%d", r.Code)
	}
}
