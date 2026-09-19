package panel

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

func (s *Server) mcpOAuthOrigin(r *http.Request) string {
	if origin, ok := s.requestHTTPSOrigin(r); ok {
		return origin
	}
	if s.mcpTransportAllowed(r) {
		return "http://" + r.Host
	}
	return ""
}
func (s *Server) mcpOAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if origin := s.mcpOAuthOrigin(r); origin != "" {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp", scope="mcp"`)
	}
}
func (m *mcpService) oauthBudget(bucket int, limit int) bool {
	m.oauthMu.Lock()
	defer m.oauthMu.Unlock()
	minute := time.Now().Unix() / 60
	if m.oauthMinute != minute {
		m.oauthMinute = minute
		m.oauthCounts = [3]int{}
	}
	if m.oauthCounts[bucket] >= limit {
		return false
	}
	m.oauthCounts[bucket]++
	return true
}
func (s *Server) oauthError(w http.ResponseWriter, err error) {
	status, code := 400, "invalid_request"
	if errors.Is(err, mcpaccess.ErrOAuth) {
		code = "invalid_grant"
	}
	if errors.Is(err, mcpaccess.ErrUnavailable) {
		status, code = 503, "temporarily_unavailable"
	}
	if errors.Is(err, mcpaccess.ErrLimited) {
		status, code = 429, "temporarily_unavailable"
		w.Header().Set("Retry-After", "60")
	}
	s.writeJSON(w, status, map[string]string{"error": code})
}

func (s *Server) handleMCPOAuth(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if !slices.Contains([]string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server", "/oauth/register", "/oauth/authorize", "/oauth/token", "/oauth/revoke"}, path) {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if s.mcp == nil || !s.auth.IsInitialized() || !s.mcp.access.Enabled() || !s.mcpTransportAllowed(r) || r.URL.RawPath != "" {
		http.NotFound(w, r)
		return true
	}
	origin := s.mcpOAuthOrigin(r)
	if r.Method == http.MethodOptions && path != "/oauth/authorize" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.WriteHeader(204)
		return true
	}
	if path != "/oauth/authorize" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if strings.HasPrefix(path, "/.well-known/") && r.Method == http.MethodGet && r.URL.RawQuery == "" {
		if strings.Contains(path, "protected-resource") {
			s.writeJSON(w, 200, map[string]any{"resource": origin + "/mcp", "authorization_servers": []string{origin}, "scopes_supported": []string{"mcp"}, "bearer_methods_supported": []string{"header"}})
		} else {
			s.writeJSON(w, 200, map[string]any{"issuer": origin, "authorization_endpoint": origin + "/oauth/authorize", "token_endpoint": origin + "/oauth/token", "registration_endpoint": origin + "/oauth/register", "revocation_endpoint": origin + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"}, "revocation_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"mcp"}})
		}
		return true
	}
	if path == "/oauth/authorize" && r.Method == http.MethodGet {
		if !s.mcp.oauthBudget(1, 60) {
			s.oauthError(w, mcpaccess.ErrLimited)
			return true
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		for key, values := range query {
			if len(values) != 1 || !slices.Contains([]string{"client_id", "redirect_uri", "response_type", "code_challenge", "code_challenge_method", "resource", "state", "scope"}, key) {
				err = mcpaccess.ErrInvalid
			}
		}
		if err != nil || query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" || query.Get("resource") != origin+"/mcp" || (query.Get("scope") != "" && query.Get("scope") != "mcp") {
			s.oauthError(w, mcpaccess.ErrInvalid)
			return true
		}
		a, err := s.mcp.oauth.Authorize(query.Get("client_id"), query.Get("redirect_uri"), query.Get("code_challenge"), query.Get("resource"), query.Get("state"))
		if err != nil {
			s.oauthError(w, err)
			return true
		}
		// Loading a same-origin document first preserves Strict session cookies
		// without publishing the installation's secret security entrance path.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		_ = mcpOAuthLanding.Execute(w, struct{ Name, Target string }{a.ClientName, "/settings?mcpAuthorize=" + url.QueryEscape(a.ID) + "#mcp-access"})
		return true
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		s.oauthError(w, mcpaccess.ErrInvalid)
		return true
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	defer http.NewResponseController(w).SetReadDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, 24<<10)
	if path == "/oauth/register" {
		if !s.mcp.oauthBudget(0, 12) {
			s.oauthError(w, mcpaccess.ErrLimited)
			return true
		}
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "application/json" {
			s.oauthError(w, mcpaccess.ErrInvalid)
			return true
		}
		var input struct {
			Name      string   `json:"client_name"`
			Redirects []string `json:"redirect_uris"`
			Method    string   `json:"token_endpoint_auth_method"`
			Grants    []string `json:"grant_types"`
			Responses []string `json:"response_types"`
			Scope     string   `json:"scope"`
		}
		decoder := json.NewDecoder(r.Body)
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || (input.Scope != "" && input.Scope != "mcp") {
			s.oauthError(w, mcpaccess.ErrInvalid)
			return true
		}
		for _, grant := range input.Grants {
			if !slices.Contains([]string{"authorization_code", "refresh_token"}, grant) {
				s.oauthError(w, mcpaccess.ErrInvalid)
				return true
			}
		}
		for _, response := range input.Responses {
			if response != "code" {
				s.oauthError(w, mcpaccess.ErrInvalid)
				return true
			}
		}
		client, secret, err := s.mcp.oauth.Register(input.Name, input.Redirects, input.Method)
		if err != nil {
			s.oauthError(w, err)
			return true
		}
		value := map[string]any{"client_id": client.ID, "client_id_issued_at": client.CreatedAt.Unix(), "client_name": client.Name, "redirect_uris": client.RedirectURIs, "token_endpoint_auth_method": client.Method, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "scope": "mcp"}
		if secret != "" {
			value["client_secret"] = secret
			value["client_secret_expires_at"] = 0
		}
		s.writeJSON(w, 201, value)
		return true
	}
	if !s.mcp.oauthBudget(2, 120) {
		s.oauthError(w, mcpaccess.ErrLimited)
		return true
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
		s.oauthError(w, mcpaccess.ErrInvalid)
		return true
	}
	for key, values := range r.PostForm {
		if len(values) != 1 || !slices.Contains([]string{"client_id", "client_secret", "grant_type", "code", "redirect_uri", "code_verifier", "resource", "refresh_token", "scope", "token", "token_type_hint"}, key) {
			s.oauthError(w, mcpaccess.ErrInvalid)
			return true
		}
	}
	clientID, secret, method := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret"), "none"
	if secret != "" {
		method = "client_secret_post"
	}
	if len(r.Header.Values("Authorization")) > 0 {
		id, password, ok := r.BasicAuth()
		if !ok || len(r.Header.Values("Authorization")) != 1 || secret != "" || (clientID != "" && clientID != id) {
			s.oauthError(w, mcpaccess.ErrOAuth)
			return true
		}
		clientID, secret, method = id, password, "client_secret_basic"
	}
	if path == "/oauth/revoke" {
		id, err := s.mcp.oauth.Revoke(s.mcp.access, clientID, secret, method, r.PostForm.Get("token"))
		if err != nil {
			s.oauthError(w, err)
			return true
		}
		if id != "" {
			_ = s.mcp.operations.Invalidate(id)
		}
		w.WriteHeader(200)
		return true
	}
	if path != "/oauth/token" || r.PostForm.Get("resource") != origin+"/mcp" || (r.PostForm.Get("scope") != "" && r.PostForm.Get("scope") != "mcp") {
		s.oauthError(w, mcpaccess.ErrInvalid)
		return true
	}
	var tokens mcpaccess.OAuthTokens
	var err error
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		tokens, err = s.mcp.oauth.Exchange(s.mcp.access, clientID, secret, method, r.PostForm.Get("code"), r.PostForm.Get("redirect_uri"), r.PostForm.Get("code_verifier"), r.PostForm.Get("resource"))
	case "refresh_token":
		tokens, err = s.mcp.oauth.Refresh(s.mcp.access, clientID, secret, method, r.PostForm.Get("refresh_token"), r.PostForm.Get("resource"))
	default:
		err = mcpaccess.ErrInvalid
	}
	if err != nil {
		s.oauthError(w, err)
	} else {
		s.writeJSON(w, 200, tokens)
	}
	return true
}

var mcpOAuthLanding = template.Must(template.New("oauth").Parse(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>KPanel MCP 授权</title><style>body{font:16px/1.7 system-ui,sans-serif;max-width:640px;margin:12vh auto;padding:24px;color:#183528;background:#f6faf7}h1{font-size:24px}a{display:inline-block;padding:12px 20px;border-radius:8px;background:#17623d;color:white;text-decoration:none}p{overflow-wrap:anywhere}a:focus-visible{outline:3px solid #2165c9;outline-offset:4px}</style></head><body><h1>连接 KPanel</h1><p>{{.Name}} 请求连接此面板。接下来选择允许访问的主机和功能；不会自动授予管理权限。</p><p>若已启用安全入口，请先通过您保存的入口登录面板，然后返回此页继续。</p><a href="{{.Target}}">继续到面板审阅授权</a></body></html>`))

// The parent settings router has authenticated Session and Origin/CSRF.
func (s *Server) handleMCPOAuthConsent(w http.ResponseWriter, r *http.Request, actor string) {
	id := strings.TrimPrefix(r.URL.Path, mcpSettingsPath+"/oauth/")
	if id == "" || strings.Contains(id, "/") || !s.mcpTransportAllowed(r) || !s.mcp.access.Enabled() {
		http.NotFound(w, r)
		return
	}
	a, err := s.mcp.oauth.Authorization(id)
	if err != nil {
		s.oauthError(w, err)
		return
	}
	if a.Resource != s.mcpOAuthOrigin(r)+"/mcp" {
		s.oauthError(w, mcpaccess.ErrInvalid)
		return
	}
	if r.Method == http.MethodGet {
		s.writeJSON(w, 200, map[string]any{"id": a.ID, "clientName": a.ClientName, "redirectUri": a.RedirectURI, "expiresAt": a.ExpiresAt})
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Approve                 bool     `json:"approve"`
		HostIDs                 []string `json:"hostIds"`
		Domains                 []string `json:"domains"`
		Write                   bool     `json:"write"`
		FileRoots               []string `json:"fileRoots"`
		ExpiresInDays           int      `json:"expiresInDays"`
		ExpectedResourceVersion string   `json:"expectedResourceVersion"`
	}
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.audit(r, actor, "mcp.oauth.consent", "oauth_client", a.ClientID, "intent", nil); err != nil {
		s.oauthError(w, mcpaccess.ErrUnavailable)
		return
	}
	createdID := ""
	redirect, err := s.mcp.oauth.Consent(id, input.Approve, func(a mcpaccess.OAuthAuthorization) (mcpaccess.Client, error) {
		if input.ExpiresInDays < 1 || input.ExpiresInDays > 90 || len(input.HostIDs) == 0 || len(input.HostIDs) > mcpaccess.MaxHosts {
			return mcpaccess.Client{}, mcpaccess.ErrInvalid
		}
		var policy *mcpaccess.Policy
		if len(input.Domains) > 0 {
			var err error
			policy, err = managedPolicy(input.Domains, input.Write, false, input.FileRoots)
			if err != nil {
				return mcpaccess.Client{}, mcpaccess.ErrInvalid
			}
		} else if input.Write || len(input.FileRoots) > 0 {
			return mcpaccess.Client{}, mcpaccess.ErrInvalid
		}
		grants := []mcpaccess.HostGrant{}
		for _, id := range input.HostIDs {
			host, err := s.cluster.Host(r.Context(), id)
			if err != nil {
				return mcpaccess.Client{}, mcpaccess.ErrInvalid
			}
			grants = append(grants, mcpaccess.HostGrant{ID: host.ID, Identity: s.mcpHostIdentity(host)})
		}
		name := "OAuth: " + string([]rune(a.ClientName)[:min(len([]rune(a.ClientName)), 36)])
		client, _, err := s.mcp.access.CreateWithPolicy(name, grants, policy, time.Duration(input.ExpiresInDays)*24*time.Hour, input.ExpectedResourceVersion)
		if err == nil {
			createdID = client.ID
		}
		return client, err
	})
	if err != nil {
		if createdID != "" {
			_, _ = s.mcp.access.Revoke(createdID, s.mcp.access.Snapshot().ResourceVersion)
		}
		s.oauthError(w, err)
		return
	}
	_ = s.audit(r, actor, "mcp.oauth.consent", "oauth_client", a.ClientID, map[bool]string{true: "approved", false: "denied"}[input.Approve], nil)
	s.writeJSON(w, 200, map[string]string{"redirectUrl": redirect})
}
