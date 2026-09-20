package panel

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) handleSystemPackagesAction(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.SystemPackagesActionRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if field, detail := contract.ValidateSystemPackagesAction(&input); field != "" {
		s.writeValidationProblem(w, r, field, detail)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"action": input.Action, "items": input.Items, "itemCount": len(input.Items)}
	auditAction := "system.packages." + input.Action
	if err := s.audit(r, session.User.ID, auditAction, "system-packages", "selected-items", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	agentContext, cancelAgent := context.WithTimeout(context.WithoutCancel(r.Context()), systemResourceAgentTimeout)
	defer cancelAgent()
	response, err := s.hostOps.Do(agentContext, http.MethodPost, "/v1/system/packages/actions", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, auditAction, "system-packages", "selected-items", "unknown", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, auditAction, "system-packages", "selected-items", result, change)
	s.writeAgentResponse(w, r, response)
}
