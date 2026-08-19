package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestAppIconProxyRequiresSessionAndReturnsPrivateCacheHeaders(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	body := append([]byte("RIFF\x04\x00\x00\x00WEBP"), []byte("app-icon")...)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "image/webp", Body: body,
	}}
	server.agent = agent

	unauthenticated := performRequest(
		server,
		http.MethodGet,
		"/api/v1/apps/icons/deepseek-harness.webp",
		nil,
		nil,
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	if calls := agent.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("Agent called before authentication: %#v", calls)
	}

	response := authenticatedRequest(
		server,
		http.MethodGet,
		"/api/v1/apps/icons/deepseek-harness.webp",
		nil,
		sessionCookie,
		csrfCookie,
		nil,
	)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatalf("icon response = %d %q", response.Code, response.Body.Bytes())
	}
	digest := sha256.Sum256(body)
	expectedETag := `"` + hex.EncodeToString(digest[:]) + `"`
	if response.Header().Get("Content-Type") != "image/webp" ||
		response.Header().Get("Cache-Control") != appIconBrowserCache ||
		response.Header().Get("ETag") != expectedETag {
		t.Fatalf("icon headers = %#v", response.Header())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet ||
		calls[0].path != "/v1/apps/icons/deepseek-harness.webp" || calls[0].rawQuery != "" {
		t.Fatalf("Agent calls = %#v", calls)
	}

	notModified := authenticatedRequest(
		server,
		http.MethodGet,
		"/api/v1/apps/icons/deepseek-harness.webp",
		nil,
		sessionCookie,
		csrfCookie,
		map[string]string{"If-None-Match": "W/" + expectedETag},
	)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 ||
		notModified.Header().Get("ETag") != expectedETag {
		t.Fatalf("conditional response = %d headers=%#v", notModified.Code, notModified.Header())
	}
}

func TestAppIconProxyFallsBackWhenAgentIconIsUnavailable(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.agent = &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusServiceUnavailable,
		ContentType: "application/problem+json",
		Body:        []byte(`{"code":"app_icon_unavailable"}`),
	}}
	response := authenticatedRequest(
		server,
		http.MethodGet,
		"/api/v1/apps/icons/new-app.webp",
		nil,
		sessionCookie,
		csrfCookie,
		nil,
	)
	if response.Code != http.StatusTemporaryRedirect ||
		response.Header().Get("Location") != appIconFallbackPath ||
		response.Header().Get("Cache-Control") != appIconFallbackBrowserCache {
		t.Fatalf("fallback response = %d headers=%#v", response.Code, response.Header())
	}
}

func TestAppIconProxyRejectsMalformedPathsBeforeAgent(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{}
	server.agent = agent
	for name, target := range map[string]string{
		"missing suffix": "/api/v1/apps/icons/new-app",
		"extra segment":  "/api/v1/apps/icons/extra/new-app.webp",
		"encoded slash":  "/api/v1/apps/icons/new%2Fapp.webp",
		"query":          "/api/v1/apps/icons/new-app.webp?refresh=true",
	} {
		t.Run(name, func(t *testing.T) {
			response := authenticatedRequest(
				server,
				http.MethodGet,
				target,
				nil,
				sessionCookie,
				csrfCookie,
				nil,
			)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if calls := agent.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("Agent called for malformed icon routes: %#v", calls)
	}
}
