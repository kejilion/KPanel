package panel

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var appIDPattern = regexp.MustCompile(`^(?:builtin|thirdparty)-[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var markerResourceVersionPattern = regexp.MustCompile(`^marker:sha256:[a-f0-9]{64}$`)

type appActionInput struct {
	HostPort        optionalInt    `json:"hostPort"`
	AccessMode      optionalString `json:"accessMode"`
	ResourceVersion optionalString `json:"resourceVersion"`
}

func (s *Server) handleAppAction(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	agentPath, appID, action, ok := allowedAppActionPath(r.URL.Path)
	if !ok {
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
	var input appActionInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if field, detail := validateAppActionInput(action, input); field != "" {
		s.writeValidationProblem(w, r, field, detail)
		return
	}
	payload := make(map[string]any)
	if input.HostPort.Set {
		payload["hostPort"] = input.HostPort.Value
	}
	if input.AccessMode.Set {
		payload["accessMode"] = input.AccessMode.Value
	}
	if input.ResourceVersion.Set {
		payload["resourceVersion"] = input.ResourceVersion.Value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"action": action}
	if input.HostPort.Set {
		change["hostPort"] = input.HostPort.Value
	}
	if input.AccessMode.Set {
		change["accessMode"] = input.AccessMode.Value
	}
	if err := s.audit(r, session.User.ID, "app."+action, "application", appID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "app."+action, "application", appID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result, change := acceptedJobAudit(response, "app", change)
	_ = s.audit(r, session.User.ID, "app."+action, "application", appID, result, change)
	s.writeAgentResponse(w, r, response)
}

func validateAppActionInput(action string, input appActionInput) (field, detail string) {
	switch action {
	case "install":
		if input.ResourceVersion.Set {
			return "resourceVersion", "resourceVersion is not allowed for install"
		}
		if input.HostPort.Set && (input.HostPort.Value < 1 || input.HostPort.Value > 65535) {
			return "hostPort", "hostPort must be between 1 and 65535"
		}
		if input.AccessMode.Set && input.AccessMode.Value != "direct" && input.AccessMode.Value != "domain_only" {
			return "accessMode", "accessMode must be direct or domain_only"
		}
	case "start", "stop", "restart", "check_update", "update", "uninstall":
		if !input.ResourceVersion.Set || !resourceVersionPattern.MatchString(input.ResourceVersion.Value) {
			return "resourceVersion", "a valid resourceVersion is required"
		}
		if input.HostPort.Set || input.AccessMode.Set {
			return "request", "only resourceVersion is allowed for this action"
		}
	case "manage":
		if !input.ResourceVersion.Set ||
			!markerResourceVersionPattern.MatchString(input.ResourceVersion.Value) {
			return "resourceVersion", "a valid marker resourceVersion is required"
		}
		if input.HostPort.Set || input.AccessMode.Set {
			return "request", "only resourceVersion is allowed for this action"
		}
	case "direct_access":
		if !input.ResourceVersion.Set || !resourceVersionPattern.MatchString(input.ResourceVersion.Value) {
			return "resourceVersion", "a valid resourceVersion is required"
		}
		if !input.AccessMode.Set ||
			(input.AccessMode.Value != "direct" && input.AccessMode.Value != "domain_only") {
			return "accessMode", "accessMode must be direct or domain_only"
		}
		if input.HostPort.Set {
			return "hostPort", "hostPort is not allowed for this action"
		}
	default:
		return "action", "unsupported application action"
	}
	return "", ""
}

func (s *Server) handleAppJobInput(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	const prefix = "/api/v1/app-jobs/"
	const suffix = "/input"
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if !siteIDPattern.MatchString(id) {
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
	var input struct {
		Data string `json:"data"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if len(input.Data) == 0 || len(input.Data) > 16<<10 || strings.IndexByte(input.Data, 0) >= 0 {
		s.writeValidationProblem(w, r, "data", "terminal input must contain 1 to 16384 bytes without NUL")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	response, err := s.hostOps.Do(
		r.Context(),
		http.MethodPost,
		"/v1/app-jobs/"+id+"/input",
		"",
		requestID(r),
		body,
	)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleAppJobCancel(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	const prefix = "/api/v1/app-jobs/"
	const suffix = "/cancel"
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if !siteIDPattern.MatchString(id) {
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
	change := map[string]any{"action": "cancel"}
	if err := s.audit(r, session.User.ID, "app.job.cancel", "application_job", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.agent.Do(
		r.Context(),
		http.MethodPost,
		"/v1/app-jobs/"+id+"/cancel",
		"",
		requestID(r),
		nil,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "app.job.cancel", "application_job", id, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "app.job.cancel", "application_job", id, result, change)
	s.writeAgentResponse(w, r, response)
}

func allowedAppActionPath(publicPath string) (agentPath, appID, action string, allowed bool) {
	const prefix = "/api/v1/apps/"
	rest := strings.TrimPrefix(publicPath, prefix)
	if rest == publicPath {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || !appIDPattern.MatchString(parts[0]) {
		return "", "", "", false
	}
	switch parts[1] {
	case "install", "start", "stop", "restart", "check_update", "update", "uninstall", "direct_access", "manage":
		return "/v1/apps/" + parts[0] + "/" + parts[1], parts[0], parts[1], true
	default:
		return "", "", "", false
	}
}
