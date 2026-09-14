package panel

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleAutomaticUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.URL.Path == "/api/v1/settings/automatic-update/check" {
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
			return
		}
		if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
			return
		}
		s.forwardAutomaticUpdateMutation(w, r, session.User.ID, "settings.automatic_update.check", nil, "/v1/self-update/check", nil)
		return
	}
	if r.URL.Path == "/api/v1/settings/automatic-update/install" {
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
			return
		}
		if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
			return
		}
		var input struct {
			ExpectedResourceVersion string `json:"expectedResourceVersion"`
		}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
			s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
			return
		}
		body, err := json.Marshal(input)
		if err != nil {
			s.writeProblem(w, r, http.StatusInternalServerError, "automatic_update_request_failed", "Automatic update request failed", "")
			return
		}
		s.forwardAutomaticUpdateMutation(
			w,
			r,
			session.User.ID,
			"settings.automatic_update.install",
			nil,
			"/v1/self-update/install",
			body,
		)
		return
	}

	switch r.Method {
	case http.MethodGet:
		response, err := s.agent.Get(r.Context(), "/v1/self-update", "", requestID(r))
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
			return
		}
		s.writeAgentResponse(w, r, response)
	case http.MethodPut:
		if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
			return
		}
		var input struct {
			Enabled                 bool   `json:"enabled"`
			Channel                 string `json:"channel"`
			ExpectedResourceVersion string `json:"expectedResourceVersion"`
		}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
			s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
			return
		}
		if input.Channel != "stable" && input.Channel != "preview" {
			s.writeValidationProblem(w, r, "channel", "channel must be stable or preview")
			return
		}
		body, err := json.Marshal(input)
		if err != nil {
			s.writeProblem(w, r, http.StatusInternalServerError, "automatic_update_request_failed", "Automatic update request failed", "")
			return
		}
		change := map[string]any{"enabled": input.Enabled, "channel": input.Channel}
		s.forwardAutomaticUpdateMutation(w, r, session.User.ID, "settings.automatic_update.update", change, "/v1/self-update", body)
	default:
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) handleKPanelRelease(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid release information request", "")
		return
	}
	values := r.URL.Query()
	channels, ok := values["channel"]
	if !ok || len(values) != 1 || len(channels) != 1 ||
		(channels[0] != "stable" && channels[0] != "preview") {
		s.writeValidationProblem(w, r, "channel", "channel must be stable or preview")
		return
	}
	response, err := s.agent.Get(
		r.Context(),
		"/v1/self-update/release",
		"channel="+channels[0],
		requestID(r),
	)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) forwardAutomaticUpdateMutation(
	w http.ResponseWriter,
	r *http.Request,
	actorID string,
	action string,
	change map[string]any,
	agentPath string,
	body []byte,
) {
	if err := s.audit(r, actorID, action, "panel", "automatic-update", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.agent.Do(r.Context(), r.Method, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, actorID, action, "panel", "automatic-update", "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result = "success"
	}
	_ = s.audit(r, actorID, action, "panel", "automatic-update", result, change)
	s.writeAgentResponse(w, r, response)
}
