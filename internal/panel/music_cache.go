package panel

import (
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/musiccache"
)

const musicCachePath = "/api/v1/music/cache"

func (s *Server) handleMusicCache(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var input struct {
		Refs        []musiccache.Ref `json:"refs"`
		Observation *struct {
			Ref  musiccache.Ref  `json:"ref"`
			Info musiccache.Info `json:"info"`
		} `json:"observation"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	valid := len(input.Refs) > 0 && len(input.Refs) <= 32 && input.Observation == nil || len(input.Refs) == 0 && input.Observation != nil
	for _, ref := range input.Refs {
		valid = valid && ref.Valid()
	}
	if o := input.Observation; o != nil {
		valid = valid && o.Ref.Valid() && o.Info.Valid()
	}
	if !valid {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_music_cache", "Invalid music cache request", "")
		return
	}
	if s.musicCache == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "music_cache_unavailable", "Music cache temporarily unavailable", "")
		return
	}
	if o := input.Observation; o != nil {
		if err := s.musicCache.Put(o.Ref, o.Info); err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "music_cache_unavailable", "Music cache temporarily unavailable", "")
			return
		}
	}
	items, err := s.musicCache.Lookup(input.Refs)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "music_cache_unavailable", "Music cache temporarily unavailable", "")
		return
	}
	s.writeJSON(w, http.StatusOK, struct {
		Items []musiccache.Record `json:"items"`
	}{items})
}
