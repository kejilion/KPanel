package panel

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

func TestMCPOAuthHTTPDiscoveryRegistrationConsentAndToken(t *testing.T) {
	s, path := newTestServerWithPublicURL(t, "https://panel.test")
	session, csrf := bootstrapCookiesForOrigin(t, s, path, "https://panel.test")
	_, _ = s.mcp.access.SetEnabled(true, s.mcp.access.Snapshot().ResourceVersion)
	request := func(method, path, body, contentType string, authenticated, csrfHeader bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.test"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		if authenticated {
			r.AddCookie(session)
			r.Header.Set("Origin", "https://panel.test")
		}
		if csrfHeader {
			r.AddCookie(csrf)
			r.Header.Set("X-CSRF-Token", csrf.Value)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	challenge := mcpRPC(s, "", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if challenge.Code != 401 || !strings.Contains(challenge.Header().Get("WWW-Authenticate"), "oauth-protected-resource/mcp") {
		t.Fatal("missing resource metadata challenge")
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server"} {
		w := request("GET", path, "", "", false, false)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("metadata: %d", w.Code)
		}
	}
	w := request("POST", "/oauth/register", `{"client_name":"Remote assistant","redirect_uris":["https://client.test/callback"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"]}`, "application/json", false, false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var c struct {
		ID string `json:"client_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	verifier := strings.Repeat("v", 64)
	sum := sha256.Sum256([]byte(verifier))
	params := url.Values{"response_type": {"code"}, "client_id": {c.ID}, "redirect_uri": {"https://client.test/callback"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {"https://panel.test/mcp"}, "state": {"opaque-client-state"}}
	w = request("GET", "/oauth/authorize?"+params.Encode(), "", "", false, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	matched := regexp.MustCompile(`mcpAuthorize=([A-Za-z0-9_-]{43})`).FindStringSubmatch(w.Body.String())
	if len(matched) != 2 {
		t.Fatal("missing consent link")
	}
	consentPath := mcpSettingsPath + "/oauth/" + matched[1]
	if w = request("GET", consentPath, "", "", false, false); w.Code != 401 {
		t.Fatal("anonymous consent access")
	}
	input, _ := json.Marshal(map[string]any{"approve": true, "hostIds": []string{"local"}, "expiresInDays": 7, "expectedResourceVersion": s.mcp.access.Snapshot().ResourceVersion})
	if w = request("POST", consentPath, string(input), "application/json", true, false); w.Code != 403 {
		t.Fatal("consent CSRF bypass")
	}
	w = request("POST", consentPath, string(input), "application/json", true, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var callback struct {
		URL string `json:"redirectUrl"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &callback)
	redirect, _ := url.Parse(callback.URL)
	if redirect.Query().Get("state") != "opaque-client-state" {
		t.Fatal("OAuth state changed")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {c.ID}, "code": {redirect.Query().Get("code")}, "redirect_uri": {"https://client.test/callback"}, "code_verifier": {verifier}, "resource": {"https://panel.test/mcp"}}
	w = request("POST", "/oauth/token", form.Encode(), "application/x-www-form-urlencoded", false, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var tokens mcpaccess.OAuthTokens
	_ = json.Unmarshal(w.Body.Bytes(), &tokens)
	if w = mcpRPC(s, tokens.AccessToken, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kpanel_info","arguments":{}}}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"permission":"inspect"`) {
		t.Fatal("OAuth MCP interoperability", w.Code, w.Body.String())
	}
	if len(s.mcp.access.Snapshot().Clients) != 1 {
		t.Fatal("missing revocable grant")
	}
	w = request("POST", "/oauth/revoke", url.Values{"client_id": {c.ID}, "token": {tokens.RefreshToken}}.Encode(), "application/x-www-form-urlencoded", false, false)
	if w.Code != 200 {
		t.Fatal("revoke", w.Code)
	}
	if w = mcpRPC(s, tokens.AccessToken, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`); w.Code != http.StatusUnauthorized {
		t.Fatal("revoked OAuth token still usable")
	}
}
