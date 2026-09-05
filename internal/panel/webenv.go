package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) handleWebEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		Action                  string `json:"action"`
		Profile                 string `json:"profile,omitempty"`
		Operation               string `json:"operation,omitempty"`
		Component               string `json:"component,omitempty"`
		Version                 string `json:"version,omitempty"`
		BackupID                string `json:"backupId,omitempty"`
		BackupBeforeChange      bool   `json:"backupBeforeChange,omitempty"`
		ExpectedResourceVersion string `json:"expectedResourceVersion,omitempty"`
		CloudflareAccount       string `json:"cloudflareAccount,omitempty"`
		CloudflareToken         string `json:"cloudflareToken,omitempty"`
		CloudflareZoneID        string `json:"cloudflareZoneId,omitempty"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	target := input.Component
	if target == "" {
		target = input.Operation
	}
	if target == "" {
		target = input.Profile
	}
	if target == "" {
		target = input.BackupID
	}
	change := map[string]any{
		"profile": input.Profile, "operation": input.Operation, "component": input.Component,
		"version": input.Version, "backupId": input.BackupID,
		"backupBeforeChange": input.BackupBeforeChange,
		"resourceVersion":    input.ExpectedResourceVersion,
	}
	if err := s.audit(r, session.User.ID, "web.environment."+input.Action, "web_environment", target, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.agent.Do(r.Context(), http.MethodPost, "/v1/web-environment/jobs", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "web.environment."+input.Action, "web_environment", target, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result, change := acceptedJobAudit(response, "webenv", change)
	_ = s.audit(r, session.User.ID, "web.environment."+input.Action, "web_environment", target, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleWebEnvironmentBackupDownload(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	agentPath, allowed := allowedAgentPath(r.URL.Path)
	if !allowed {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	client, ok := s.agent.(*AgentClient)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent streaming unavailable", "")
		return
	}
	response, err := client.Open(r.Context(), http.MethodGet, agentPath, r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Last-Modified"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *Server) handleWebEnvironmentInput(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	agentPath, allowed := allowedAgentPath(r.URL.Path)
	if !allowed {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	var input struct {
		Data string `json:"data"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if size := len([]byte(input.Data)); size == 0 || size > 16<<10 || strings.IndexByte(input.Data, 0) >= 0 {
		s.writeValidationProblem(w, r, "data", "terminal input must contain 1 to 16384 bytes without NUL")
		return
	}
	body, _ := json.Marshal(input)
	response, err := s.agent.Do(r.Context(), http.MethodPost, agentPath, "", requestID(r), body)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}
