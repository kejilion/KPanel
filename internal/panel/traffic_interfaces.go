package panel

import (
	"encoding/json"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	trafficInterfacesPath            = "/api/v1/system/traffic-interfaces"
	maxTrafficInterfacesRequestBytes = 4 << 10
	trafficInterfacesAuditAction     = "system.traffic_interfaces.update"
	trafficInterfacesAuditTargetKind = "traffic-interfaces"
	trafficInterfacesAuditTargetID   = "configuration"
)

// handleTrafficInterfaces reads and replaces the local host's traffic
// interface selection. Other hosts keep their own selection: KPanel hosts in
// their own panel, lightweight nodes through the node CLI.
func (s *Server) handleTrafficInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "traffic_interfaces_request_invalid", "Traffic interfaces request is invalid", "")
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
		response, err := s.hostOps.Get(r.Context(), "/v1/system/traffic-interfaces", "", requestID(r))
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
	var input contract.UpdateTrafficInterfacesInput
	if err := decodeLimitedJSON(w, r, maxTrafficInterfacesRequestBytes, &input); err != nil {
		_ = s.audit(r, session.User.ID, trafficInterfacesAuditAction, trafficInterfacesAuditTargetKind, trafficInterfacesAuditTargetID, "failure", nil)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	// Interface names are host configuration, not secrets; recording them lets
	// the audit trail explain a later change in counted traffic.
	change := map[string]any{"include": input.Include, "exclude": input.Exclude}
	if err := s.audit(r, session.User.ID, trafficInterfacesAuditAction, trafficInterfacesAuditTargetKind, trafficInterfacesAuditTargetID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPut, "/v1/system/traffic-interfaces", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, trafficInterfacesAuditAction, trafficInterfacesAuditTargetKind, trafficInterfacesAuditTargetID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	outcome := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		outcome = "success"
	}
	_ = s.audit(r, session.User.ID, trafficInterfacesAuditAction, trafficInterfacesAuditTargetKind, trafficInterfacesAuditTargetID, outcome, change)
	s.writeAgentResponse(w, r, response)
}
