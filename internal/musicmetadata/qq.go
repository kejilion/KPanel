package musicmetadata

import (
	"context"
	"errors"
	"html"
	"net/url"
	"regexp"
)

var qqMID = regexp.MustCompile(`^[A-Za-z0-9]{14}$`)

func (s *Service) fetchQQ(ctx context.Context, q Query) (Result, error) {
	var search struct {
		Code int `json:"code"`
		Data struct {
			Song struct {
				List []struct {
					ID       int64  `json:"songid"`
					MID      string `json:"songmid"`
					Name     string `json:"songname"`
					Duration int64  `json:"interval"`
					Singers  []struct {
						Name string `json:"name"`
					} `json:"singer"`
					Album    string `json:"albumname"`
					AlbumMID string `json:"albummid"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}
	params := url.Values{"w": {q.Title + " " + q.Artist}, "format": {"json"}, "p": {"1"}, "n": {"20"}}
	if err := s.json(ctx, "https://c.y.qq.com/soso/fcgi-bin/client_search_cp?"+params.Encode(), &search); err != nil {
		return Result{}, err
	}
	if search.Code != 0 {
		return Result{}, errors.New("QQ search unavailable")
	}
	best, index := -1, -1
	for i, candidate := range search.Data.Song.List {
		if !qqMID.MatchString(candidate.MID) {
			continue
		}
		item := song{ID: candidate.ID, Name: html.UnescapeString(candidate.Name), Duration: candidate.Duration * 1000, Artists: candidate.Singers}
		item.Album.Name = html.UnescapeString(candidate.Album)
		if n := score(q, item); n > best {
			best = n
			index = i
		}
	}
	if index < 0 {
		return Result{}, nil
	}
	selected := search.Data.Song.List[index]
	result := Result{Source: "qq"}
	var lyric struct {
		Code  int    `json:"code"`
		Lyric string `json:"lyric"`
	}
	lyricErr := s.json(ctx, "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg?songmid="+selected.MID+"&format=json&nobase64=1&g_tk=5381", &lyric)
	if lyricErr == nil && lyric.Code == 0 && len(lyric.Lyric) <= 128<<10 {
		result.Lyrics = html.UnescapeString(lyric.Lyric)
	}
	if lyric.Code != 0 {
		lyricErr = errors.New("QQ lyrics unavailable")
	}
	var coverErr error
	if qqMID.MatchString(selected.AlbumMID) {
		result.Cover, coverErr = s.cover(ctx, "https://y.gtimg.cn/music/photo_new/T002R300x300M000"+selected.AlbumMID+".jpg")
	}
	result.Matched = result.Lyrics != "" || result.Cover != ""
	if !result.Matched && (lyricErr != nil || coverErr != nil) {
		return Result{}, errors.New("QQ metadata unavailable")
	}
	return result, nil
}
