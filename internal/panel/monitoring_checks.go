package panel

import (
	"encoding/json"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func (s *Server) handleMonitoringChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "monitoring_checks_request_invalid", "Monitoring checks request is invalid", "")
		return
	}
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		response, err := s.hostOps.Get(r.Context(), "/v1/monitoring/checks", "", requestID(r))
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
			return
		}
		s.writeAgentResponse(w, r, response)
		return
	}
	if !s.checkCSRF(w, r, session) {
		return
	}
	var input monitoring.ReplaceChecksInput
	if err := decodeLimitedJSON(w, r, monitoring.MaxCheckUpdateBytes, &input); err != nil {
		_ = s.audit(r, session.User.ID, "monitoring.checks.update", "monitoring-checks", "collection", "failure", nil)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"itemCount": len(input.Items)}
	if err := s.audit(r, session.User.ID, "monitoring.checks.update", "monitoring-checks", "collection", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPut, "/v1/monitoring/checks", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "monitoring.checks.update", "monitoring-checks", "collection", "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	outcome := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		outcome = "success"
	}
	_ = s.audit(r, session.User.ID, "monitoring.checks.update", "monitoring-checks", "collection", outcome, change)
	s.writeAgentResponse(w, r, response)
}
