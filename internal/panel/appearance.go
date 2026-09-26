package panel

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/store"
)

const appearancePath = "/api/v1/settings/appearance"

type appearanceResponse struct {
	Configured      bool   `json:"configured"`
	ResourceVersion string `json:"resourceVersion"`
	store.Appearance
}

func appearanceView(value *store.Appearance, version string) appearanceResponse {
	if value == nil {
		return appearanceResponse{ResourceVersion: version, Appearance: store.Appearance{
			Theme: "system", Wallpaper: "classic", ClassicLevel: "off",
		}}
	}
	return appearanceResponse{Configured: true, ResourceVersion: version, Appearance: *value}
}

func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "appearance_request_invalid", "Appearance request is invalid", "")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
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
	if r.Method == http.MethodGet {
		value, version := s.store.Appearance()
		s.writeJSON(w, http.StatusOK, appearanceView(value, version))
		return
	}
	var input struct {
		store.Appearance
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
		return
	}
	if store.ValidateAppearance(input.Appearance) != nil {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "appearance_invalid", "Appearance setting is invalid", "")
		return
	}
	change := map[string]any{"theme": input.Theme, "wallpaper": input.Wallpaper, "classicLevel": input.ClassicLevel}
	if err := s.audit(r, session.User.ID, "settings.appearance.update", "panel", "appearance", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.store.ReplaceAppearance(input.ExpectedResourceVersion, input.Appearance); err != nil {
		_ = s.audit(r, session.User.ID, "settings.appearance.update", "panel", "appearance", "failure", change)
		if errors.Is(err, store.ErrConflict) {
			s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Appearance changed; refresh and retry", "")
		} else {
			s.writeProblem(w, r, http.StatusInternalServerError, "appearance_update_failed", "Appearance update failed", "")
		}
		return
	}
	_ = s.audit(r, session.User.ID, "settings.appearance.update", "panel", "appearance", "success", change)
	value, version := s.store.Appearance()
	s.writeJSON(w, http.StatusOK, appearanceView(value, version))
}
