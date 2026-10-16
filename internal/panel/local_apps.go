package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Local apps are user-authored kejilion.sh configs in /root/apps-local. The
// Panel only authenticates, audits and forwards; the Agent owns validation.
const (
	localAppsPath           = "/api/v1/apps/local"
	maxLocalAppContentBytes = 256 << 10
	maxLocalAppIconBytes    = 64 << 10
)

var (
	localAppTokenPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	localAppIconTagPattern = regexp.MustCompile(`^[a-f0-9]{12}$`)
)

type localAppIconInput struct {
	ContentType string `json:"contentType"`
	Data        []byte `json:"data"`
}

type localAppWriteInput struct {
	Token           string             `json:"token"`
	Content         string             `json:"content"`
	ResourceVersion string             `json:"resourceVersion"`
	Icon            *localAppIconInput `json:"icon,omitempty"`
	RemoveIcon      bool               `json:"removeIcon"`
}

// Local app IDs are "local-<token>", so action routes such as
// /api/v1/apps/local-notes/install never share this prefix.
func isLocalAppRequest(path string) bool {
	return path == localAppsPath || strings.HasPrefix(path, localAppsPath+"/")
}

func (s *Server) handleLocalApps(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	token := ""
	if r.URL.Path != localAppsPath {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, localAppsPath+"/"), "/")
		if len(parts) == 3 && parts[1] == "icon" {
			s.handleLocalAppIcon(w, r, parts[0], parts[2])
			return
		}
		token = parts[0]
		if len(parts) != 1 || !localAppTokenPattern.MatchString(token) {
			s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
			return
		}
	}
	switch {
	case token == "" && r.Method == http.MethodPost:
		s.handleLocalAppWrite(w, r, "")
	case token != "" && r.Method == http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		response, err := s.hostOps.Get(r.Context(), "/v1/apps/local/"+token, "", requestID(r))
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
			return
		}
		s.writeAgentResponse(w, r, response)
	case token != "" && r.Method == http.MethodPut:
		s.handleLocalAppWrite(w, r, token)
	case token != "" && r.Method == http.MethodDelete:
		s.handleLocalAppDelete(w, r, token)
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) handleLocalAppWrite(w http.ResponseWriter, r *http.Request, token string) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input localAppWriteInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	create := token == ""
	if create {
		token = input.Token
	} else if input.Token != "" && input.Token != token {
		s.writeValidationProblem(w, r, "token", "token cannot change while editing")
		return
	}
	input.Token = token
	switch {
	case !localAppTokenPattern.MatchString(token):
		s.writeValidationProblem(w, r, "token", "token must use letters, digits, underscores or hyphens")
		return
	case len(input.Content) == 0 || len(input.Content) > maxLocalAppContentBytes:
		s.writeValidationProblem(w, r, "content", "content must be 1 byte to 256 KiB")
		return
	case create && input.ResourceVersion != "":
		s.writeValidationProblem(w, r, "resourceVersion", "resourceVersion must be empty when creating")
		return
	case !create && !strings.HasPrefix(input.ResourceVersion, "sha256:"):
		s.writeValidationProblem(w, r, "resourceVersion", "resourceVersion is required")
		return
	case input.Icon != nil && (len(input.Icon.Data) == 0 || len(input.Icon.Data) > maxLocalAppIconBytes ||
		(input.Icon.ContentType != "image/png" && input.Icon.ContentType != "image/webp") ||
		!siteIconBodyMatchesContentType(input.Icon.Data, input.Icon.ContentType)):
		s.writeValidationProblem(w, r, "icon", "icon must be a PNG or WebP of at most 64 KiB")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	contentDigest := sha256.Sum256([]byte(input.Content))
	iconChange := "unchanged"
	if input.Icon != nil {
		iconChange = "uploaded"
	} else if input.RemoveIcon {
		iconChange = "removed"
	}
	change := map[string]any{
		"contentSha256": hex.EncodeToString(contentDigest[:]),
		"contentBytes":  len(input.Content),
		"icon":          iconChange,
	}
	action, method, agentPath := "app.local.update", http.MethodPut, "/v1/apps/local/"+token
	if create {
		action, method, agentPath = "app.local.create", http.MethodPost, "/v1/apps/local"
	}
	s.forwardLocalAppMutation(w, r, session.User.ID, action, token, method, agentPath, body, change)
}

func (s *Server) handleLocalAppDelete(w http.ResponseWriter, r *http.Request, token string) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		ResourceVersion string `json:"resourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !strings.HasPrefix(input.ResourceVersion, "sha256:") {
		s.writeValidationProblem(w, r, "resourceVersion", "resourceVersion is required")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	s.forwardLocalAppMutation(w, r, session.User.ID, "app.local.delete", token,
		http.MethodDelete, "/v1/apps/local/"+token, body, map[string]any{})
}

func (s *Server) forwardLocalAppMutation(
	w http.ResponseWriter,
	r *http.Request,
	actorID, action, token, method, agentPath string,
	body []byte,
	change map[string]any,
) {
	targetID := "local-" + token
	if err := s.audit(r, actorID, action, "application", targetID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), method, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, actorID, action, "application", targetID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, actorID, action, "application", targetID, result, change)
	s.writeAgentResponse(w, r, response)
}

// handleLocalAppIcon serves GET /api/v1/apps/local/{token}/icon/{tag}; the
// tag is a content hash that only busts browser caches.
func (s *Server) handleLocalAppIcon(w http.ResponseWriter, r *http.Request, token, tag string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	if !localAppTokenPattern.MatchString(token) || !localAppIconTagPattern.MatchString(tag) {
		writeAppIconFallback(w, r)
		return
	}
	response, err := s.hostOps.Get(r.Context(), "/v1/apps/local/"+token+"/icon", "", requestID(r))
	if err != nil || response.StatusCode != http.StatusOK {
		writeAppIconFallback(w, r)
		return
	}
	contentType := strings.TrimSpace(strings.SplitN(response.ContentType, ";", 2)[0])
	if (contentType != "image/png" && contentType != "image/webp") || len(response.Body) == 0 ||
		len(response.Body) > maxLocalAppIconBytes || !siteIconBodyMatchesContentType(response.Body, contentType) {
		writeAppIconFallback(w, r)
		return
	}
	digest := sha256.Sum256(response.Body)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("Cache-Control", appIconBrowserCache)
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if requestMatchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(response.Body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response.Body)
}
