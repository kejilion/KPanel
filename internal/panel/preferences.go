package panel

import (
	"errors"
	"github.com/kejilion/kejilion-panel/internal/store"
	"net/http"
)

const preferencesPath = "/api/v1/settings/preferences"

type preferencesResponse struct {
	ResourceVersion string `json:"resourceVersion"`
	store.Preferences
}

func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, 400, "preferences_request_invalid", "Preferences request is invalid", "")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPut && !s.checkCSRF(w, r, session) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodPut {
		var input struct {
			store.Preferences
			ExpectedResourceVersion string `json:"expectedResourceVersion"`
		}
		if s.decodeJSON(w, r, &input) != nil {
			return
		}
		if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
			s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
			return
		}
		if (input.Locale == "" && input.Desktop == nil && input.Browser == nil && input.GalleryLocation == nil && input.MusicLocation == nil && input.CinemaLocation == nil && input.MusicPlayback == nil) || store.ValidatePreferences(input.Preferences) != nil {
			s.writeProblem(w, r, 422, "preferences_invalid", "Preferences are invalid", "")
			return
		}
		// Audit only section names, never window URLs or file paths.
		change := map[string]any{"locale": input.Locale != "", "desktop": input.Desktop != nil, "browser": input.Browser != nil, "galleryLocation": input.GalleryLocation != nil, "musicLocation": input.MusicLocation != nil, "cinemaLocation": input.CinemaLocation != nil, "musicPlayback": input.MusicPlayback != nil}
		if s.audit(r, session.User.ID, "settings.preferences.update", "panel", "preferences", "intent", change) != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if err := s.store.PatchPreferences(input.ExpectedResourceVersion, input.Preferences); err != nil {
			_ = s.audit(r, session.User.ID, "settings.preferences.update", "panel", "preferences", "failure", change)
			if errors.Is(err, store.ErrConflict) {
				s.writeProblem(w, r, 409, "resource_version_changed", "Preferences changed; refresh and retry", "")
			} else {
				s.writeProblem(w, r, 500, "preferences_update_failed", "Preferences update failed", "")
			}
			return
		}
		_ = s.audit(r, session.User.ID, "settings.preferences.update", "panel", "preferences", "success", change)
	}
	value, version := s.store.Preferences()
	s.writeJSON(w, http.StatusOK, preferencesResponse{ResourceVersion: version, Preferences: value})
}
