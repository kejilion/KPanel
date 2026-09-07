package panel

import "net/http"

// Validated image checks are observations, just like inventory GETs. They do
// not append mutation intent/result events or rewrite the identity/audit store.
// Callers must retain their session, origin, CSRF and input validation guards.
func (s *Server) forwardImageUpdate(w http.ResponseWriter, r *http.Request, agentPath string, body []byte) {
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, agentPath, "", requestID(r), body)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}
