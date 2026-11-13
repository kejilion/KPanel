package panel

import (
	"context"
	"encoding/json"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"net/http"
	"time"
)

func (s *Server) handleBrowserComponent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, 400, "browser_component_invalid", "Invalid component request", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/browser/component" {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		response, err := s.hostOps.Get(ctx, "/v1/browser/component", "", requestID(r))
		if err != nil {
			s.writeProblem(w, r, 503, "agent_unavailable", "Agent unavailable", "")
			return
		}
		s.writeAgentResponse(w, r, response)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/api/v1/browser/component/actions" {
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.BrowserComponentRequest
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if !contract.ValidBrowserComponentAction(input.Action) || !contract.ValidBrowserComponentKind(input.Kind) || (input.Action != "install" && input.Kind != "") || !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.writeProblem(w, r, 422, "browser_component_invalid", "Invalid component request", "")
		return
	}
	change := map[string]any{"action": input.Action}
	if input.Kind != "" {
		change["kind"] = input.Kind
	}
	if s.audit(r, session.User.ID, "browser.component."+input.Action, "browser", "component", "intent", change) != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit unavailable", "")
		return
	}
	body, _ := json.Marshal(input)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
	defer cancel()
	response, err := s.hostOps.Do(ctx, http.MethodPost, "/v1/browser/component/actions", "", requestID(r), body)
	result := "unknown"
	if err == nil {
		result = "failure"
		if response.StatusCode == 202 {
			result = "accepted"
		}
	}
	_ = s.audit(r, session.User.ID, "browser.component."+input.Action, "browser", "component", result, change)
	if err != nil {
		s.writeProblem(w, r, 503, "browser_component_result_unknown", "Refresh component status before retrying", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}
