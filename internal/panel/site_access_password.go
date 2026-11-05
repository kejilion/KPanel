package panel

import (
	"encoding/json"
	"net/http"
	"strings"
)

type siteAccessAccountInput struct {
	RuleID           string `json:"ruleId"`
	ResourceVersion  string `json:"resourceVersion"`
	Action           string `json:"action"`
	Username         string `json:"username"`
	PreviousUsername string `json:"previousUsername,omitempty"`
	Password         string `json:"password,omitempty"`
}

type siteAccessRuleInput struct {
	Action          string `json:"action"`
	ResourceVersion string `json:"resourceVersion"`
	RuleID          string `json:"ruleId,omitempty"`
	Scope           string `json:"scope,omitempty"`
	Path            string `json:"path,omitempty"`
	Modifier        string `json:"modifier,omitempty"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
}

func (s *Server) handleSiteAccessPasswordRule(w http.ResponseWriter, r *http.Request) {
	const prefix, suffix = "/api/v1/sites/", "/access-passwords/rules"
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || !siteIDPattern.MatchString(id) {
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
	var input siteAccessRuleInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.Action != "add" && input.Action != "delete" {
		s.writeValidationProblem(w, r, "action", "Invalid access password rule action")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"action": input.Action, "ruleId": input.RuleID}
	if err := s.audit(r, session.User.ID, "site.access_password.rule", "site", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, "/v1/sites/"+id+"/access-passwords/rules", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "site.access_password.rule", "site", id, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "site.access_password.rule", "site", id, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleSiteAccessPasswordAccount(w http.ResponseWriter, r *http.Request) {
	const prefix, suffix = "/api/v1/sites/", "/access-passwords/accounts"
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || !siteIDPattern.MatchString(id) {
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
	var input siteAccessAccountInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !siteIDPattern.MatchString(input.RuleID) || (input.Action != "add" && input.Action != "update" && input.Action != "delete") {
		s.writeValidationProblem(w, r, "action", "Invalid access password action or rule")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"action": input.Action, "ruleId": input.RuleID}
	if err := s.audit(r, session.User.ID, "site.access_password.account", "site", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, "/v1/sites/"+id+"/access-passwords/accounts", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "site.access_password.account", "site", id, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "site.access_password.account", "site", id, result, change)
	s.writeAgentResponse(w, r, response)
}
