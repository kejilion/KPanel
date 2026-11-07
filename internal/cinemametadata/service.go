// Package cinemametadata fetches optional, read-only metadata for local videos.
package cinemametadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

var ErrBusy = errors.New("cinema metadata busy")
var ErrCredentials = errors.New("cinema source credentials rejected")
var ErrUnavailable = errors.New("cinema source unavailable")

type Query struct {
	Provider   string `json:"provider"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Language   string `json:"language"`
	Credential string `json:"credential,omitempty"`
	ID         int64  `json:"id,omitempty"`
}
type Item struct {
	Provider      string   `json:"provider"`
	ID            int64    `json:"id"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Year          string   `json:"year,omitempty"`
	Overview      string   `json:"overview"`
	Rating        float64  `json:"rating,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Cast          []string `json:"cast,omitempty"`
	Poster        string   `json:"poster,omitempty"`
	Backdrop      string   `json:"backdrop,omitempty"`
	URL           string   `json:"url"`
}
type Result struct {
	Items []Item `json:"items"`
}
type cached struct {
	result  Result
	expires time.Time
}
type Service struct {
	client     *http.Client
	gate       chan struct{}
	imageGate  chan struct{}
	imageQueue chan struct{}
	mu         sync.Mutex
	cache      map[[32]byte]cached
}

func New() *Service {
	public := remotedownload.NewClient(remotedownload.Config{ConnectTimeout: 3 * time.Second})
	// A proxy is operator configuration, never request input. Direct connections
	// retain the shared public-address policy. Redirects remain disabled in both modes.
	proxyRequest, _ := http.NewRequest(http.MethodGet, "https://api.themoviedb.org", nil)
	proxy, _ := http.ProxyFromEnvironment(proxyRequest)
	dial := public.DialPublic
	var proxyFunc func(*http.Request) (*url.URL, error)
	if proxy != nil && (proxy.Scheme == "http" || proxy.Scheme == "https") {
		proxyFunc = http.ProxyURL(proxy)
		dial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	}
	return &Service{client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{Proxy: proxyFunc, DialContext: dial, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 64 << 10, MaxConnsPerHost: 2, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second}}, gate: make(chan struct{}, 2), imageGate: make(chan struct{}, 2), imageQueue: make(chan struct{}, 32), cache: make(map[[32]byte]cached)}
}
func (q Query) Valid() bool {
	if q.Kind != "movie" && q.Kind != "tv" {
		return false
	}
	if q.Language != "zh-CN" && q.Language != "zh-TW" && q.Language != "en-US" {
		return false
	}
	if q.Provider != "tmdb" && q.Provider != "tvmaze" && q.Provider != "wikipedia" {
		return false
	}
	if q.Provider == "tvmaze" && q.Kind != "tv" {
		return false
	}
	if len(q.Title) > 240 || strings.TrimSpace(q.Title) == "" || strings.IndexFunc(q.Title, unicode.IsControl) >= 0 || q.ID < 0 || q.ID > 1<<40 {
		return false
	}
	if q.Provider == "tmdb" {
		return len(q.Credential) >= 16 && len(q.Credential) <= 2048 && strings.IndexFunc(q.Credential, func(r rune) bool { return r <= 32 || r > 126 }) < 0
	}
	return q.Credential == "" && (q.Provider != "wikipedia" || q.ID == 0)
}
func (s *Service) acquire(ctx context.Context) (func(), error) {
	select {
	case s.gate <- struct{}{}:
		return func() { <-s.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrBusy
	}
}
func (s *Service) request(ctx context.Context, raw, credential string, limit int64) ([]byte, string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	r.Header.Set("User-Agent", "KPanel-Cinema/1 (https://github.com/kejilion/KPanel)")
	r.Header.Set("Accept", "application/json, image/jpeg, image/png")
	if credential != "" {
		r.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := s.client.Do(r)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, "", ErrCredentials
	}
	if response.StatusCode == 429 {
		return nil, "", ErrBusy
	}
	if response.StatusCode != 200 {
		return nil, "", ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, "", ErrUnavailable
	}
	return body, response.Header.Get("Content-Type"), nil
}
func (s *Service) Search(ctx context.Context, q Query) (Result, error) {
	if !q.Valid() {
		return Result{}, errors.New("invalid cinema query")
	}
	// Hash the entire query: credentials never appear in cache keys or logs.
	keyBytes, _ := json.Marshal(q)
	key := sha256.Sum256(keyBytes)
	s.mu.Lock()
	hit, ok := s.cache[key]
	s.mu.Unlock()
	if ok && time.Now().Before(hit.expires) {
		return hit.result, nil
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	var result Result
	switch q.Provider {
	case "tmdb":
		result, err = s.tmdb(ctx, q)
	case "tvmaze":
		result, err = s.tvmaze(ctx, q)
	case "wikipedia":
		result, err = s.wikipedia(ctx, q)
	}
	if err != nil {
		return Result{}, err
	}
	if result.Items == nil {
		result.Items = []Item{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= 64 {
		for k, v := range s.cache {
			if time.Now().After(v.expires) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= 64 {
			for k := range s.cache {
				delete(s.cache, k)
				break
			}
		}
	}
	ttl := 30 * time.Minute
	if len(result.Items) == 0 {
		ttl = time.Minute
	}
	s.cache[key] = cached{result, time.Now().Add(ttl)}
	return result, nil
}
func text(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
func year(s string) string {
	if len(s) >= 4 {
		if _, err := strconv.Atoi(s[:4]); err == nil {
			return s[:4]
		}
	}
	return ""
}

var markup = regexp.MustCompile(`<[^>]*>`)
var wikipediaRelease = regexp.MustCompile(`(?i)\b(?:is|was) (?:an? )?((?:19|20)\d{2})\b|(?:於|于)((?:19|20)\d{2})年(?:上映|发行|發行|首播|播出)|(?:是|為|为)(?:一部|一套|一齣|一出)?((?:19|20)\d{2})年`)
var wikipediaEnglishTitle = regexp.MustCompile(`(?:英語|英语|英文)[：:]\s*([^，,；;）)]+)`)

func wikipediaYear(extract string) string {
	match := wikipediaRelease.FindStringSubmatch(text(extract, 400))
	if len(match) < 2 {
		return ""
	}
	for _, value := range match[1:] {
		if value != "" {
			return value
		}
	}
	return ""
}

func plain(s string) string { return text(html.UnescapeString(markup.ReplaceAllString(s, " ")), 6000) }

var tmdbImage = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|png)$`)
var mazeImage = regexp.MustCompile(`^/uploads/images/(medium_portrait|original_untouched)/[0-9]+/[0-9]+\.(jpg|png)$`)

func imagePath(provider, raw string) string {
	if provider == "tmdb" {
		if tmdbImage.MatchString(raw) {
			return raw
		}
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return ""
	}
	if provider == "tvmaze" && u.Host == "static.tvmaze.com" && u.RawQuery == "" && u.RawPath == "" && mazeImage.MatchString(u.Path) {
		return u.Path
	}
	// Wikimedia adds analytics parameters to API thumbnails; discard all of them.
	if provider == "wikipedia" && (u.Host == "upload.wikimedia.org" || u.Host == "thumb.wikimedia.org") && strings.HasPrefix(u.Path, "/wikipedia/") && len(u.Path) < 1800 && !strings.Contains(u.Path, "..") && !strings.ContainsAny(u.Path, "\\\x00\r\n?#") && (strings.HasSuffix(strings.ToLower(u.Path), ".jpg") || strings.HasSuffix(strings.ToLower(u.Path), ".png")) {
		return u.Path
	}
	return ""
}
func ImageURL(provider, path, size string) string {
	host := ""
	prefix := ""
	switch provider {
	case "tmdb":
		if !tmdbImage.MatchString(path) {
			return ""
		}
		host = "image.tmdb.org"
		if size == "backdrop" {
			prefix = "/t/p/w780"
		} else {
			prefix = "/t/p/w500"
		}
	case "tvmaze":
		host = "static.tvmaze.com"
		if imagePath(provider, "https://"+host+path) != path {
			return ""
		}
	case "wikipedia":
		host = "upload.wikimedia.org"
		if imagePath(provider, "https://"+host+path) != path {
			return ""
		}
	default:
		return ""
	}
	return (&url.URL{Scheme: "https", Host: host, Path: prefix + path}).String()
}
func (s *Service) Image(ctx context.Context, provider, path, size string) ([]byte, string, error) {
	raw := ImageURL(provider, path, size)
	if raw == "" {
		return nil, "", errors.New("invalid cinema image")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case s.imageQueue <- struct{}{}:
		defer func() { <-s.imageQueue }()
	default:
		return nil, "", ErrBusy
	}
	select {
	case s.imageGate <- struct{}{}:
		defer func() { <-s.imageGate }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	body, _, err := s.request(ctx, raw, "", 4<<20)
	if err != nil {
		return nil, "", err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 {
		return nil, "", ErrUnavailable
	}
	if format != "jpeg" && format != "png" {
		return nil, "", ErrUnavailable
	}
	return body, map[string]string{"jpeg": "image/jpeg", "png": "image/png"}[format], nil
}

type tmdbItem struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	Name          string  `json:"name"`
	OriginalTitle string  `json:"original_title"`
	OriginalName  string  `json:"original_name"`
	Overview      string  `json:"overview"`
	Date          string  `json:"release_date"`
	FirstAir      string  `json:"first_air_date"`
	Poster        string  `json:"poster_path"`
	Backdrop      string  `json:"backdrop_path"`
	Rating        float64 `json:"vote_average"`
	Genres        []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Credits struct {
		Cast []struct {
			Name string `json:"name"`
		} `json:"cast"`
	} `json:"credits"`
}

func (s *Service) tmdb(ctx context.Context, q Query) (Result, error) {
	params := url.Values{"language": {q.Language}, "include_adult": {"false"}, "query": {q.Title}}
	credential := q.Credential
	if len(credential) == 32 && !strings.ContainsAny(credential, ". ") {
		params.Set("api_key", credential)
		credential = ""
	}
	path := "/3/search/" + q.Kind
	if q.ID > 0 {
		path = "/3/" + q.Kind + "/" + strconv.FormatInt(q.ID, 10)
		params.Del("query")
		params.Set("append_to_response", "credits")
	}
	body, _, err := s.request(ctx, "https://api.themoviedb.org"+path+"?"+params.Encode(), credential, 1<<20)
	if err != nil {
		return Result{}, err
	}
	var items []tmdbItem
	if q.ID > 0 {
		var item tmdbItem
		err = json.Unmarshal(body, &item)
		items = []tmdbItem{item}
	} else {
		var list struct {
			Results []tmdbItem `json:"results"`
		}
		err = json.Unmarshal(body, &list)
		items = list.Results
	}
	if err != nil {
		return Result{}, ErrUnavailable
	}
	result := Result{}
	for _, v := range items {
		if len(result.Items) >= 8 {
			break
		}
		title := v.Title
		original := v.OriginalTitle
		date := v.Date
		if q.Kind == "tv" {
			title = v.Name
			original = v.OriginalName
			date = v.FirstAir
		}
		if v.ID <= 0 || title == "" {
			continue
		}
		item := Item{Provider: "tmdb", ID: v.ID, Kind: q.Kind, Title: text(title, 240), OriginalTitle: text(original, 240), Year: year(date), Overview: plain(v.Overview), Rating: v.Rating, Poster: imagePath("tmdb", v.Poster), Backdrop: imagePath("tmdb", v.Backdrop), URL: "https://www.themoviedb.org/" + q.Kind + "/" + strconv.FormatInt(v.ID, 10)}
		for _, g := range v.Genres {
			if len(item.Genres) < 8 {
				item.Genres = append(item.Genres, text(g.Name, 80))
			}
		}
		for _, c := range v.Credits.Cast {
			if len(item.Cast) < 8 {
				item.Cast = append(item.Cast, text(c.Name, 120))
			}
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

type mazeShow struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Premiered string   `json:"premiered"`
	Genres    []string `json:"genres"`
	Rating    struct {
		Average float64 `json:"average"`
	} `json:"rating"`
	Image *struct {
		Medium string `json:"medium"`
	} `json:"image"`
}

func (s *Service) tvmaze(ctx context.Context, q Query) (Result, error) {
	raw := "https://api.tvmaze.com/search/shows?q=" + url.QueryEscape(q.Title)
	if q.ID > 0 {
		raw = "https://api.tvmaze.com/shows/" + strconv.FormatInt(q.ID, 10)
	}
	body, _, err := s.request(ctx, raw, "", 1<<20)
	if err != nil {
		return Result{}, err
	}
	var list []struct {
		Show mazeShow `json:"show"`
	}
	if q.ID > 0 {
		var v mazeShow
		err = json.Unmarshal(body, &v)
		list = append(list, struct {
			Show mazeShow `json:"show"`
		}{v})
	} else {
		err = json.Unmarshal(body, &list)
	}
	if err != nil {
		return Result{}, ErrUnavailable
	}
	result := Result{}
	for _, v := range list {
		show := v.Show
		if len(result.Items) >= 8 {
			break
		}
		if show.ID <= 0 || show.Name == "" {
			continue
		}
		item := Item{Provider: "tvmaze", ID: show.ID, Kind: "tv", Title: text(show.Name, 240), Year: year(show.Premiered), Overview: plain(show.Summary), Rating: show.Rating.Average, URL: "https://www.tvmaze.com/shows/" + strconv.FormatInt(show.ID, 10)}
		for _, g := range show.Genres {
			if len(item.Genres) < 8 {
				item.Genres = append(item.Genres, text(g, 80))
			}
		}
		if show.Image != nil {
			item.Poster = imagePath("tvmaze", show.Image.Medium)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
func (s *Service) wikipedia(ctx context.Context, q Query) (Result, error) {
	language := "en"
	if strings.HasPrefix(q.Language, "zh") {
		language = "zh"
	}
	suffix := " film"
	if q.Kind == "tv" {
		suffix = " television series"
	}
	if language == "zh" {
		suffix = " 电影"
		if q.Kind == "tv" {
			suffix = " 电视剧"
		}
	}
	searchTitle := strings.NewReplacer("\"", " ", ":", " ").Replace(q.Title)
	params := url.Values{"action": {"query"}, "format": {"json"}, "formatversion": {"2"}, "generator": {"search"}, "gsrsearch": {"intitle:\"" + searchTitle + "\"" + suffix}, "gsrnamespace": {"0"}, "gsrlimit": {"5"}, "prop": {"extracts|pageimages|info"}, "exintro": {"1"}, "explaintext": {"1"}, "exchars": {"1800"}, "piprop": {"thumbnail"}, "pilicense": {"any"}, "pithumbsize": {"500"}, "inprop": {"url"}}
	body, _, err := s.request(ctx, "https://"+language+".wikipedia.org/w/api.php?"+params.Encode(), "", 1<<20)
	if err != nil {
		return Result{}, err
	}
	var data struct {
		Error json.RawMessage `json:"error"`
		Query struct {
			Pages []struct {
				Index     int    `json:"index"`
				ID        int64  `json:"pageid"`
				Title     string `json:"title"`
				Extract   string `json:"extract"`
				Thumbnail struct {
					Source string `json:"source"`
				} `json:"thumbnail"`
			} `json:"pages"`
		} `json:"query"`
	}
	if json.Unmarshal(body, &data) != nil || len(data.Error) > 0 {
		return Result{}, ErrUnavailable
	}
	result := Result{}
	sort.SliceStable(data.Query.Pages, func(i, j int) bool { return data.Query.Pages[i].Index < data.Query.Pages[j].Index })
	for _, p := range data.Query.Pages {
		if len(result.Items) >= 5 {
			break
		}
		if p.ID <= 0 || p.Title == "" {
			continue
		}
		releaseYear := wikipediaYear(p.Extract)
		alias := ""
		if strings.Contains(text(p.Extract, 200), "《"+q.Title+"》") {
			alias = q.Title
		} else if match := wikipediaEnglishTitle.FindStringSubmatch(text(p.Extract, 200)); len(match) > 1 {
			alias = text(strings.TrimSpace(match[1]), 240)
		}
		result.Items = append(result.Items, Item{Provider: "wikipedia", ID: p.ID, Kind: q.Kind, Title: text(p.Title, 240), OriginalTitle: alias, Year: releaseYear, Overview: plain(p.Extract), Poster: imagePath("wikipedia", p.Thumbnail.Source), URL: "https://" + language + ".wikipedia.org/?curid=" + strconv.FormatInt(p.ID, 10)})
	}
	return result, nil
}
