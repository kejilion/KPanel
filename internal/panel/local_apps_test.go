package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalAppWritesAreAuthenticatedValidatedAuditedAndForwarded(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusCreated, ContentType: "application/json",
		Body: []byte(`{"token":"notes","content":"x","resourceVersion":"sha256:` + strings.Repeat("a", 64) + `","installed":false,"containerName":"notes"}`),
	}}
	server.agent = agent
	version := "sha256:" + strings.Repeat("a", 64)

	rejected := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/apps/local", `{"token":"../x","content":"a","resourceVersion":""}`},
		{http.MethodPost, "/api/v1/apps/local", `{"token":"notes","content":"","resourceVersion":""}`},
		{http.MethodPost, "/api/v1/apps/local", `{"token":"notes","content":"a","resourceVersion":"` + version + `"}`},
		{http.MethodPut, "/api/v1/apps/local/notes", `{"token":"notes","content":"a","resourceVersion":""}`},
		{http.MethodPut, "/api/v1/apps/local/notes", `{"token":"other","content":"a","resourceVersion":"` + version + `"}`},
		{http.MethodPost, "/api/v1/apps/local", `{"token":"notes","content":"a","resourceVersion":"","icon":{"contentType":"image/svg+xml","data":"PHN2Zy8+"}}`},
		{http.MethodDelete, "/api/v1/apps/local/notes", `{}`},
		{http.MethodPost, "/api/v1/apps/local", `{"token":"notes","content":"a","resourceVersion":"","unexpected":true}`},
	}
	for _, sample := range rejected {
		response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, sample.method, sample.path, []byte(sample.body), true)
		if response.Code < 400 || response.Code >= 500 {
			t.Errorf("%s %s %s returned %d", sample.method, sample.path, sample.body, response.Code)
		}
	}
	if calls := agent.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("invalid writes reached the Agent: %#v", calls)
	}
	if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/apps/local",
		[]byte(`{"token":"notes","content":"a","resourceVersion":""}`), false); response.Code != http.StatusForbidden {
		t.Fatalf("write without CSRF returned %d", response.Code)
	}

	png := "iVBORw0KGgo=" // PNG signature; the Agent decodes and validates the image.
	for _, sample := range []struct {
		method, path, body, agentMethod, agentPath, action string
	}{
		{http.MethodPost, "/api/v1/apps/local", `{"token":"notes","content":"local app_id=\"notes\"","resourceVersion":"","icon":{"contentType":"image/png","data":"` + png + `"}}`,
			http.MethodPost, "/v1/apps/local", "app.local.create"},
		{http.MethodPut, "/api/v1/apps/local/notes", `{"token":"notes","content":"b","resourceVersion":"` + version + `","removeIcon":true}`,
			http.MethodPut, "/v1/apps/local/notes", "app.local.update"},
		{http.MethodDelete, "/api/v1/apps/local/notes", `{"resourceVersion":"` + version + `"}`,
			http.MethodDelete, "/v1/apps/local/notes", "app.local.delete"},
	} {
		before := len(agent.snapshotCalls())
		response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, sample.method, sample.path, []byte(sample.body), true)
		if response.Code != http.StatusCreated {
			t.Fatalf("%s %s returned %d: %s", sample.method, sample.path, response.Code, response.Body.String())
		}
		calls := agent.snapshotCalls()
		if len(calls) != before+1 || calls[before].method != sample.agentMethod || calls[before].path != sample.agentPath {
			t.Fatalf("%s was not forwarded exactly: %#v", sample.action, calls)
		}
		var forwarded map[string]any
		if err := json.Unmarshal(calls[before].body, &forwarded); err != nil {
			t.Fatal(err)
		}
		if sample.action != "app.local.delete" && forwarded["token"] != "notes" {
			t.Fatalf("%s forwarded the wrong token: %#v", sample.action, forwarded)
		}
	}

	events := mustAuditEvents(t, server)
	seen := map[string]int{}
	for _, event := range events {
		if strings.HasPrefix(event.Action, "app.local.") && event.TargetID == "local-notes" {
			seen[event.Action+"/"+event.Result]++
			if strings.Contains(string(mustJSON(t, event)), "local app_id") {
				t.Fatalf("audit stored the config content: %#v", event)
			}
		}
	}
	for _, key := range []string{
		"app.local.create/intent", "app.local.create/success",
		"app.local.update/intent", "app.local.update/success",
		"app.local.delete/intent", "app.local.delete/success",
	} {
		if seen[key] != 1 {
			t.Fatalf("audit %s = %d, events %#v", key, seen[key], seen)
		}
	}
}

func TestLocalAppReadAndIconRoutes(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "image/png", Body: []byte("\x89PNG\r\n\x1a\nrest"),
	}}
	server.agent = agent

	icon := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet,
		"/api/v1/apps/local/notes/icon/0123456789ab", nil, false)
	if icon.Code != http.StatusOK || icon.Header().Get("Content-Type") != "image/png" ||
		icon.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("icon route returned %d %v", icon.Code, icon.Header())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/apps/local/notes/icon" {
		t.Fatalf("icon was not read from the Agent route: %#v", calls)
	}
	for _, path := range []string{
		"/api/v1/apps/local/notes/icon/not-a-hash",
		"/api/v1/apps/local/bad.name/icon/0123456789ab",
	} {
		if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, path, nil, false); response.Code != http.StatusTemporaryRedirect {
			t.Fatalf("%s returned %d, want fallback redirect", path, response.Code)
		}
	}

	agent.response = AgentResponse{StatusCode: http.StatusOK, ContentType: "image/svg+xml", Body: []byte("<svg/>")}
	if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet,
		"/api/v1/apps/local/notes/icon/0123456789ab", nil, false); response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("non-raster icon was served: %d", response.Code)
	}

	agent.response = AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"token":"notes"}`)}
	read := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, "/api/v1/apps/local/notes", nil, false)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"notes"`) {
		t.Fatalf("read returned %d %s", read.Code, read.Body.String())
	}

	anonymous := httptest.NewRecorder()
	anonymousRequest := httptest.NewRequest(http.MethodGet, "/api/v1/apps/local/notes", nil)
	anonymousRequest.Host = "panel.test"
	server.ServeHTTP(anonymous, anonymousRequest)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read returned %d", anonymous.Code)
	}
}

// A local app may be named "icons"; its action routes must not be taken by
// the icon route, which lives under /api/v1/apps/local/{token}/icon/.
func TestLocalAppActionRoutesDoNotCollideWithLocalAppPaths(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusAccepted, ContentType: "application/json", Body: []byte(`{"id":"` + strings.Repeat("c", 32) + `"}`),
	}}
	server.agent = agent
	for _, id := range []string{"local-icons", "local-local"} {
		before := len(agent.snapshotCalls())
		authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/apps/"+id+"/install", []byte(`{}`), true)
		calls := agent.snapshotCalls()
		if len(calls) != before+1 || calls[before].path != "/v1/apps/"+id+"/install" {
			t.Fatalf("%s install was not routed as an app action: %#v", id, calls)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
