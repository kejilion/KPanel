package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxSiteAliases        = 20
	maxSiteDomainLength   = 253
	maxSiteUpstreamLength = 2048
	maxSiteUpstreams      = 8
	maxSiteCertificate    = 16 << 10
	maxSitePrivateKey     = 8 << 10
)

var siteIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type optionalString struct {
	Value string
	Set   bool
}

func (value *optionalString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("null is not allowed")
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = decoded
	value.Set = true
	return nil
}

type optionalBool struct {
	Value bool
	Set   bool
}

func (value *optionalBool) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("null is not allowed")
	}
	var decoded bool
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = decoded
	value.Set = true
	return nil
}

type optionalStrings struct {
	Value []string
	Set   bool
}

func (value *optionalStrings) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("null is not allowed")
	}
	var decoded []string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = decoded
	value.Set = true
	return nil
}

type optionalInt struct {
	Value int
	Set   bool
}

func (value *optionalInt) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("null is not allowed")
	}
	var decoded int
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = decoded
	value.Set = true
	return nil
}

type siteWriteInput struct {
	PrimaryDomain           optionalString  `json:"primaryDomain"`
	Aliases                 optionalStrings `json:"aliases"`
	Type                    optionalString  `json:"type"`
	Certificate             optionalString  `json:"certificate"`
	PrivateKey              optionalString  `json:"privateKey"`
	Recipe                  optionalString  `json:"recipe"`
	Upstream                optionalString  `json:"upstream"`
	Upstreams               optionalStrings `json:"upstreams"`
	RedirectTarget          optionalString  `json:"redirectTarget"`
	RedirectCode            optionalInt     `json:"redirectCode"`
	PHPVersion              optionalString  `json:"phpVersion"`
	Enabled                 optionalBool    `json:"enabled"`
	ExpectedResourceVersion optionalString  `json:"expectedResourceVersion"`
}

type siteAgentPayload struct {
	PrimaryDomain           *string   `json:"primaryDomain,omitempty"`
	Aliases                 *[]string `json:"aliases,omitempty"`
	Type                    *string   `json:"type,omitempty"`
	Certificate             *string   `json:"certificate,omitempty"`
	PrivateKey              *string   `json:"privateKey,omitempty"`
	Recipe                  *string   `json:"recipe,omitempty"`
	Upstream                *string   `json:"upstream,omitempty"`
	Upstreams               *[]string `json:"upstreams,omitempty"`
	RedirectTarget          *string   `json:"redirectTarget,omitempty"`
	RedirectCode            *int      `json:"redirectCode,omitempty"`
	PHPVersion              *string   `json:"phpVersion,omitempty"`
	Enabled                 *bool     `json:"enabled,omitempty"`
	ExpectedResourceVersion *string   `json:"expectedResourceVersion,omitempty"`
}

func (s *Server) handleSiteInstallationInput(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	const prefix = "/api/v1/site-installations/"
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
		"/v1/site-installations/"+id+"/input",
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

func (s *Server) handleDiagnosticInput(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	const prefix = "/api/v1/diagnostic-jobs/"
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
		"/v1/diagnostic-jobs/"+id+"/input",
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

func (s *Server) handleSiteCreate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/sites" || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	s.handleSiteWrite(w, r, http.MethodPost, "/v1/sites", "", true)
}

func (s *Server) handleSiteUpdate(w http.ResponseWriter, r *http.Request) {
	agentPath, siteID, ok := allowedSiteUpdatePath(r.URL.Path)
	if !ok || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	s.handleSiteWrite(w, r, http.MethodPatch, agentPath, siteID, false)
}

func (s *Server) handleSiteDelete(w http.ResponseWriter, r *http.Request) {
	agentPath, siteID, ok := allowedSiteUpdatePath(r.URL.Path)
	if !ok || r.URL.RawPath != "" || r.URL.RawQuery != "" {
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
		PrimaryDomain optionalString `json:"primaryDomain"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !input.PrimaryDomain.Set {
		s.writeValidationProblem(w, r, "primaryDomain", "primaryDomain is required for site deletion")
		return
	}
	normalized, valid := normalizePanelSiteDomain(input.PrimaryDomain.Value)
	if !valid {
		s.writeValidationProblem(w, r, "primaryDomain", "primaryDomain must be a valid ASCII domain")
		return
	}
	input.PrimaryDomain.Value = normalized
	payload := map[string]string{"primaryDomain": input.PrimaryDomain.Value}
	body, err := json.Marshal(payload)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"primaryDomain": input.PrimaryDomain.Value}
	if err := s.audit(r, session.User.ID, "site.delete", "site", siteID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodDelete, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "site.delete", "site", siteID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "site.delete", "site", siteID, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleSiteWrite(
	w http.ResponseWriter,
	r *http.Request,
	method string,
	agentPath string,
	siteID string,
	create bool,
) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}

	var input siteWriteInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if field, detail := validateSiteWriteInput(&input, create); field != "" {
		s.writeValidationProblem(w, r, field, detail)
		return
	}
	payload := input.agentPayload()
	body, err := json.Marshal(payload)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}

	action := "site.update"
	targetID := siteID
	if create {
		action = "site.create"
		targetID = input.PrimaryDomain.Value
	}
	change := map[string]any{
		"kind":            input.Type.Value,
		"domain":          input.PrimaryDomain.Value,
		"resourceVersion": input.ExpectedResourceVersion.Value,
	}
	if err := s.audit(r, session.User.ID, action, "site", targetID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}

	response, err := s.hostOps.Do(r.Context(), method, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "site", targetID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, action, "site", targetID, result, change)
	s.writeAgentResponse(w, r, response)
}

func validateSiteWriteInput(input *siteWriteInput, create bool) (field, detail string) {
	if create {
		if input.ExpectedResourceVersion.Set {
			return "expectedResourceVersion", "expectedResourceVersion is not allowed when creating a site"
		}
		if !input.PrimaryDomain.Set || input.PrimaryDomain.Value == "" {
			return "primaryDomain", "primaryDomain is required"
		}
		if !input.Type.Set || input.Type.Value == "" {
			return "type", "type is required"
		}
	} else {
		if !input.ExpectedResourceVersion.Set ||
			!resourceVersionPattern.MatchString(input.ExpectedResourceVersion.Value) {
			return "expectedResourceVersion", "a valid expectedResourceVersion is required"
		}
		if !input.PrimaryDomain.Set || input.PrimaryDomain.Value == "" {
			return "primaryDomain", "primaryDomain is required"
		}
		if !input.Type.Set || input.Type.Value == "" {
			return "type", "type is required"
		}
	}

	if input.PrimaryDomain.Set {
		normalized, valid := normalizePanelSiteDomain(input.PrimaryDomain.Value)
		if !valid {
			return "primaryDomain", "primaryDomain must be a valid ASCII domain"
		}
		input.PrimaryDomain.Value = normalized
	}
	if input.Aliases.Set {
		if len(input.Aliases.Value) > maxSiteAliases {
			return "aliases", "at most 20 aliases are allowed"
		}
		seen := make(map[string]struct{}, len(input.Aliases.Value)+1)
		if input.PrimaryDomain.Set {
			seen[input.PrimaryDomain.Value] = struct{}{}
		}
		for index, alias := range input.Aliases.Value {
			normalized, valid := normalizePanelSiteDomain(alias)
			if !valid {
				return "aliases", "each alias must be a valid ASCII domain"
			}
			if _, duplicate := seen[normalized]; duplicate {
				return "aliases", "aliases must not contain duplicate domains"
			}
			seen[normalized] = struct{}{}
			input.Aliases.Value[index] = normalized
		}
	}
	if input.Type.Set && !validPanelSiteType(input.Type.Value) {
		return "type", "type must be wordpress, recipe, static, php, proxy, proxy_domain, load_balance, or redirect"
	}
	if field, detail := validateSiteCertificateInput(input, create); field != "" {
		return field, detail
	}
	if input.Recipe.Set && input.Recipe.Value != "" && !validPanelSiteRecipe(input.Recipe.Value) {
		return "recipe", "recipe is not supported"
	}
	if input.Upstream.Set {
		if len(input.Upstream.Value) > maxSiteUpstreamLength || hasControlCharacter(input.Upstream.Value) {
			return "upstream", "upstream is too long or contains control characters"
		}
		if input.Upstream.Value != "" && !validPanelSiteUpstream(input.Upstream.Value) {
			return "upstream", "upstream must be an http(s) origin without credentials, query, fragment, or path"
		}
	}
	if input.Upstreams.Set {
		if len(input.Upstreams.Value) > maxSiteUpstreams {
			return "upstreams", "at most 8 upstreams are allowed"
		}
		seen := make(map[string]struct{}, len(input.Upstreams.Value))
		for _, upstream := range input.Upstreams.Value {
			if len(upstream) > maxSiteUpstreamLength || hasControlCharacter(upstream) ||
				!validPanelSiteUpstream(upstream) {
				return "upstreams", "each upstream must be an http(s) origin without credentials, query, fragment, or path"
			}
			if _, duplicate := seen[upstream]; duplicate {
				return "upstreams", "upstreams must not contain duplicates"
			}
			seen[upstream] = struct{}{}
		}
	}
	if input.RedirectTarget.Set {
		if len(input.RedirectTarget.Value) > maxSiteUpstreamLength ||
			hasControlCharacter(input.RedirectTarget.Value) ||
			(input.RedirectTarget.Value != "" && !validPanelSiteUpstream(input.RedirectTarget.Value)) {
			return "redirectTarget", "redirectTarget must be an http(s) origin without credentials, query, fragment, or path"
		}
	}
	if input.RedirectCode.Set && input.RedirectCode.Value != 301 && input.RedirectCode.Value != 302 &&
		input.RedirectCode.Value != 307 && input.RedirectCode.Value != 308 {
		return "redirectCode", "redirectCode must be 301, 302, 307, or 308"
	}
	if input.PHPVersion.Set && input.PHPVersion.Value != "" &&
		input.PHPVersion.Value != "latest" && input.PHPVersion.Value != "7.4" {
		return "phpVersion", "phpVersion must be latest or 7.4"
	}

	switch input.Type.Value {
	case "wordpress":
		if !create {
			return "type", "WordPress installer settings cannot be edited as a generic site"
		}
		if len(input.Aliases.Value) != 0 || input.Upstream.Value != "" ||
			len(input.Upstreams.Value) != 0 || input.RedirectTarget.Value != "" ||
			input.RedirectCode.Set || input.PHPVersion.Value != "" {
			return "type", "WordPress accepts only one primary domain"
		}
	case "recipe":
		if !create {
			return "type", "one-click recipe settings cannot be edited as a generic site"
		}
		if !input.Recipe.Set || input.Recipe.Value == "" {
			return "recipe", "recipe is required"
		}
		if len(input.Aliases.Value) != 0 || input.Upstream.Value != "" ||
			len(input.Upstreams.Value) != 0 || input.RedirectTarget.Value != "" ||
			input.RedirectCode.Set || input.PHPVersion.Value != "" {
			return "type", "one-click recipes accept only one primary domain"
		}
	case "static":
		if create {
			return validateScriptedTemplateFields(input)
		}
		if input.Upstream.Value != "" || len(input.Upstreams.Value) != 0 ||
			input.RedirectTarget.Value != "" || input.RedirectCode.Set || input.PHPVersion.Value != "" {
			return "type", "static sites cannot define runtime, upstream, or redirect settings"
		}
	case "php":
		if create {
			return validateScriptedTemplateFields(input)
		}
		if input.Upstream.Value != "" || len(input.Upstreams.Value) != 0 ||
			input.RedirectTarget.Value != "" || input.RedirectCode.Set {
			return "type", "php sites cannot define upstream or redirect settings"
		}
	case "proxy", "proxy_domain":
		if create && input.Type.Value == "proxy_domain" {
			return validateScriptedTemplateFields(input)
		}
		if !input.Upstream.Set || input.Upstream.Value == "" {
			return "upstream", "upstream is required for proxy sites"
		}
		if input.Type.Value == "proxy_domain" && !validPanelDomainOrigin(input.Upstream.Value) {
			return "upstream", "domain proxy upstream must use an ASCII domain"
		}
		if len(input.Upstreams.Value) != 0 || input.RedirectTarget.Value != "" ||
			input.RedirectCode.Set || input.PHPVersion.Value != "" {
			return "type", "proxy sites cannot define load balancing, redirect, or PHP settings"
		}
	case "load_balance":
		if create {
			return validateScriptedTemplateFields(input)
		}
		if !input.Upstreams.Set || len(input.Upstreams.Value) < 2 {
			return "upstreams", "2 to 8 upstreams are required for load balancing"
		}
		if input.Upstream.Value != "" || input.RedirectTarget.Value != "" ||
			input.RedirectCode.Set || input.PHPVersion.Value != "" {
			return "type", "load balancing sites cannot define single upstream, redirect, or PHP settings"
		}
		for _, upstream := range input.Upstreams.Value {
			if !validPanelHTTPOrigin(upstream) {
				return "upstreams", "load balancing upstreams must use http"
			}
		}
	case "redirect":
		if create {
			return validateScriptedTemplateFields(input)
		}
		if !input.RedirectTarget.Set || input.RedirectTarget.Value == "" {
			return "redirectTarget", "redirectTarget is required for redirect sites"
		}
		if !validPanelDomainOrigin(input.RedirectTarget.Value) {
			return "redirectTarget", "redirectTarget must use an ASCII domain"
		}
		if input.Upstream.Value != "" || len(input.Upstreams.Value) != 0 ||
			input.PHPVersion.Value != "" {
			return "type", "redirect sites cannot define upstream or PHP settings"
		}
	}
	return "", ""
}

func validateScriptedTemplateFields(input *siteWriteInput) (field, detail string) {
	if len(input.Aliases.Value) != 0 || input.Recipe.Value != "" ||
		input.Upstream.Value != "" || len(input.Upstreams.Value) != 0 ||
		input.RedirectTarget.Value != "" || input.RedirectCode.Set ||
		input.PHPVersion.Value != "" {
		return "type", "scripted website details must be entered in the interactive terminal"
	}
	return "", ""
}

func validateSiteCertificateInput(input *siteWriteInput, create bool) (field, detail string) {
	certificate := strings.TrimSpace(input.Certificate.Value)
	privateKey := strings.TrimSpace(input.PrivateKey.Value)
	if !create && (certificate != "" || privateKey != "") {
		return "certificate", "custom certificates can only be supplied when creating a site"
	}
	if certificate == "" && privateKey == "" {
		return "", ""
	}
	if certificate == "" {
		return "certificate", "certificate and privateKey must be supplied together"
	}
	if privateKey == "" {
		return "privateKey", "certificate and privateKey must be supplied together"
	}
	if len([]byte(certificate)) > maxSiteCertificate {
		return "certificate", "certificate is too large"
	}
	if len([]byte(privateKey)) > maxSitePrivateKey {
		return "privateKey", "privateKey is too large"
	}
	if hasUnsupportedPEMControlCharacter(certificate) || hasUnsupportedPEMControlCharacter(privateKey) {
		return "certificate", "certificate material contains unsupported control characters"
	}
	input.Certificate.Value = certificate
	input.PrivateKey.Value = privateKey
	return "", ""
}

func (input siteWriteInput) agentPayload() siteAgentPayload {
	var payload siteAgentPayload
	if input.PrimaryDomain.Set {
		payload.PrimaryDomain = &input.PrimaryDomain.Value
	}
	if input.Aliases.Set {
		payload.Aliases = &input.Aliases.Value
	}
	if input.Type.Set {
		payload.Type = &input.Type.Value
	}
	if input.Certificate.Set {
		payload.Certificate = &input.Certificate.Value
	}
	if input.PrivateKey.Set {
		payload.PrivateKey = &input.PrivateKey.Value
	}
	if input.Recipe.Set {
		payload.Recipe = &input.Recipe.Value
	}
	if input.Upstream.Set {
		payload.Upstream = &input.Upstream.Value
	}
	if input.Upstreams.Set {
		payload.Upstreams = &input.Upstreams.Value
	}
	if input.RedirectTarget.Set {
		payload.RedirectTarget = &input.RedirectTarget.Value
	}
	if input.RedirectCode.Set {
		payload.RedirectCode = &input.RedirectCode.Value
	}
	if input.PHPVersion.Set {
		payload.PHPVersion = &input.PHPVersion.Value
	}
	if input.Enabled.Set {
		payload.Enabled = &input.Enabled.Value
	}
	if input.ExpectedResourceVersion.Set {
		payload.ExpectedResourceVersion = &input.ExpectedResourceVersion.Value
	}
	return payload
}

func validPanelSiteType(value string) bool {
	switch value {
	case "wordpress", "recipe", "static", "php", "proxy", "proxy_domain", "load_balance", "redirect":
		return true
	default:
		return false
	}
}

func validPanelSiteRecipe(value string) bool {
	switch value {
	case "discuz", "kodbox", "maccms", "dujiaoka", "flarum", "typecho", "linkstack", "ai-prompt", "bitwarden", "halo":
		return true
	default:
		return false
	}
}

func allowedSiteUpdatePath(publicPath string) (agentPath, siteID string, allowed bool) {
	const prefix = "/api/v1/sites/"
	siteID = strings.TrimPrefix(publicPath, prefix)
	if siteID == publicPath || !siteIDPattern.MatchString(siteID) {
		return "", "", false
	}
	return "/v1/sites/" + siteID, siteID, true
}

func normalizePanelSiteDomain(value string) (string, bool) {
	if value == "" || strings.TrimSpace(value) != value || len(value) > maxSiteDomainLength ||
		strings.HasSuffix(value, ".") || !strings.Contains(value, ".") || net.ParseIP(value) != nil {
		return "", false
	}
	value = strings.ToLower(value)
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') && character != '-' {
				return "", false
			}
		}
	}
	return value, true
}

func validPanelSiteUpstream(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Hostname() == "" || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	return true
}

func validPanelHTTPOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "http" && validPanelSiteUpstream(value)
}

func validPanelDomainOrigin(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || !validPanelSiteUpstream(value) {
		return false
	}
	normalized, valid := normalizePanelSiteDomain(parsed.Hostname())
	return valid && normalized == strings.ToLower(parsed.Hostname())
}

func hasControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func hasUnsupportedPEMControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 && character != '\n' && character != '\t' {
			return true
		}
		if character == 0x7f {
			return true
		}
	}
	return false
}
