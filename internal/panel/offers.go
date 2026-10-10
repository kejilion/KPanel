package panel

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/offers"
)

const offersPath = "/api/v1/offers"

func isOffersRequest(path string) bool {
	return path == offersPath || strings.HasPrefix(path, offers.MediaPrefix)
}

// handleOffers serves the read-only 广告专栏 manifest and its cached banner
// images. Both are GET-only, require a session and never touch the Agent.
func (s *Server) handleOffers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	media := strings.HasPrefix(r.URL.Path, offers.MediaPrefix)
	refresh := false
	switch {
	case r.URL.RawPath != "":
		s.writeProblem(w, r, http.StatusBadRequest, "offers_request_invalid", "Offers request is invalid", "")
		return
	case media && r.URL.RawQuery != "":
		s.writeProblem(w, r, http.StatusBadRequest, "offers_request_invalid", "Offers request is invalid", "")
		return
	case !media && r.URL.RawQuery == "refresh=1":
		refresh = true
	case !media && r.URL.RawQuery != "":
		s.writeProblem(w, r, http.StatusBadRequest, "offers_request_invalid", "Offers request is invalid", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	if s.offers == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "offers_unavailable", "Offers are unavailable", "")
		return
	}
	if !media {
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusOK, s.offers.Snapshot(r.Context(), refresh))
		return
	}
	digest := strings.TrimPrefix(r.URL.Path, offers.MediaPrefix)
	data, contentType, err := s.offers.OpenImage(digest)
	if err != nil {
		s.writeProblem(w, r, http.StatusNotFound, "offers_image_not_found", "Offers image not found", "")
		return
	}
	// Images are addressed by their SHA-256, so a URL never changes content.
	etag := `"` + digest + `"`
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	if requestMatchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
