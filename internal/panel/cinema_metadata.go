package panel

import (
	"errors"
	"github.com/kejilion/kejilion-panel/internal/cinemametadata"
	"net/http"
)

const cinemaMetadataPath = "/api/v1/cinema/metadata"
const cinemaImagePath = "/api/v1/cinema/image"

func (s *Server) handleCinemaMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input cinemametadata.Query
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Valid() {
		s.writeProblem(w, r, 400, "invalid_cinema_query", "Invalid cinema query or missing TMDB credential", "")
		return
	}
	result, err := s.cinemaMetadata.Search(r.Context(), input)
	if err != nil {
		s.cinemaMetadataProblem(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	s.writeJSON(w, 200, result)
}
func (s *Server) cinemaMetadataProblem(w http.ResponseWriter, r *http.Request, err error) {
	status := 502
	code := "cinema_metadata_unavailable"
	title := "Video metadata source is unavailable"
	if errors.Is(err, cinemametadata.ErrBusy) {
		status = 429
		code = "cinema_metadata_busy"
		title = "Video metadata source is busy; retry later"
		w.Header().Set("Retry-After", "5")
	}
	if errors.Is(err, cinemametadata.ErrCredentials) {
		status = 502
		code = "cinema_source_credentials"
		title = "Video source rejected the request or credentials"
	}
	s.writeProblem(w, r, status, code, title, "")
}
func (s *Server) handleCinemaImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	q := r.URL.Query()
	if len(q) != 3 || len(q["provider"]) != 1 || len(q["path"]) != 1 || len(q["size"]) != 1 || len(q.Get("path")) > 1800 || (q.Get("size") != "poster" && q.Get("size") != "backdrop") || cinemametadata.ImageURL(q.Get("provider"), q.Get("path"), q.Get("size")) == "" {
		s.writeProblem(w, r, 400, "invalid_cinema_image", "Invalid video image", "")
		return
	}
	body, kind, err := s.cinemaMetadata.Image(r.Context(), q.Get("provider"), q.Get("path"), q.Get("size"))
	if err != nil {
		s.cinemaMetadataProblem(w, r, err)
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
