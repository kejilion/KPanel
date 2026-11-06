package panel

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/musicmetadata"
)

const musicMetadataPath = "/api/v1/music/metadata"

func (s *Server) handleMusicMetadata(w http.ResponseWriter, r *http.Request) {
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
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input musicmetadata.Query
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !input.Valid() {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_music_tags", "Invalid music tags", "")
		return
	}
	result, err := s.musicMetadata.Match(r.Context(), input)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, musicmetadata.ErrBusy) {
			status = http.StatusTooManyRequests
		}
		s.writeProblem(w, r, status, "music_metadata_unavailable", "Online music metadata is temporarily unavailable", "")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	s.writeJSON(w, http.StatusOK, result)
}
