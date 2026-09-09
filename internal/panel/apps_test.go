package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAppManageHTTPForwardsBothResourceKinds(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	for _, prefix := range []string{"sha256:", "marker:sha256:"} {
		for _, status := range []int{http.StatusAccepted, http.StatusConflict} {
			version := prefix + strings.Repeat("a", 64)
			agent := &stubAgent{response: AgentResponse{StatusCode: status, ContentType: "application/json", Body: []byte(`{"status":"forwarded"}`)}}
			server.agent = agent
			body, err := json.Marshal(map[string]string{"resourceVersion": version})
			if err != nil {
				t.Fatal(err)
			}
			response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/apps/builtin-64/manage", body, true)
			if response.Code != status {
				t.Fatalf("%s: status = %d, want %d: %s", prefix, response.Code, status, response.Body.String())
			}
			calls := agent.snapshotCalls()
			if len(calls) != 1 || calls[0].path != "/v1/apps/builtin-64/manage" || string(calls[0].body) != string(body) {
				t.Fatalf("resource version must reach Agent unchanged: %#v", calls)
			}
		}
	}
}

func TestAllowedAppActionPath(t *testing.T) {
	id := "builtin-64"
	for _, expectedAction := range []string{"install", "manage"} {
		path, gotID, action, ok := allowedAppActionPath("/api/v1/apps/" + id + "/" + expectedAction)
		if !ok || gotID != id || action != expectedAction ||
			path != "/v1/apps/"+id+"/"+expectedAction {
			t.Fatalf("valid app path rejected: %q %q %q %v", path, gotID, action, ok)
		}
	}
	for _, value := range []string{
		"/api/v1/apps/not valid/install",
		"/api/v1/apps/" + id + "/exec",
		"/api/v1/apps/" + id + "/install/extra",
	} {
		if _, _, _, ok := allowedAppActionPath(value); ok {
			t.Fatalf("unsafe app path accepted: %s", value)
		}
	}
}

func TestAllowedAppInstallPortReadPath(t *testing.T) {
	t.Parallel()
	const id = "builtin-13"
	path, ok := allowedAgentPath("/api/v1/apps/" + id + "/install-port")
	if !ok || path != "/v1/apps/"+id+"/install-port" {
		t.Fatalf("install port path = %q, %v", path, ok)
	}
	for _, invalid := range []string{
		"/api/v1/apps/not-valid/install-port",
		"/api/v1/apps/" + id + "/install-port/extra",
		"/api/v1/apps/" + id + "/other",
	} {
		if _, ok := allowedAgentPath(invalid); ok {
			t.Fatalf("unsafe install port path was accepted: %q", invalid)
		}
	}
}

func TestValidateAppActionInput(t *testing.T) {
	validVersion := "sha256:" + strings.Repeat("a", 64)
	if field, _ := validateAppActionInput("install", appActionInput{
		HostPort:   optionalInt{Value: 80, Set: true},
		AccessMode: optionalString{Value: "domain_only", Set: true},
	}); field != "" {
		t.Fatalf("script-compatible privileged port was rejected on %s", field)
	}
	if field, _ := validateAppActionInput("update", appActionInput{
		ResourceVersion: optionalString{Value: validVersion, Set: true},
	}); field != "" {
		t.Fatalf("valid update rejected on %s", field)
	}
	if field, _ := validateAppActionInput("manage", appActionInput{
		ResourceVersion: optionalString{Value: "marker:" + validVersion, Set: true},
	}); field != "" {
		t.Fatalf("valid script management request rejected on %s", field)
	}
	if field, _ := validateAppActionInput("manage", appActionInput{
		ResourceVersion: optionalString{Value: validVersion, Set: true},
	}); field != "" {
		t.Fatalf("container script management rejected on %q", field)
	}
	for _, invalid := range []string{"", "sha256:short", "marker:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64)} {
		if field, _ := validateAppActionInput("manage", appActionInput{ResourceVersion: optionalString{Value: invalid, Set: true}}); field != "resourceVersion" {
			t.Fatalf("invalid management version accepted: %q", invalid)
		}
	}
	if field, _ := validateAppActionInput("manage", appActionInput{ResourceVersion: optionalString{Value: validVersion, Set: true}, HostPort: optionalInt{Value: 80, Set: true}}); field != "request" {
		t.Fatalf("management accepted extra install parameters on %q", field)
	}
	if field, _ := validateAppActionInput("direct_access", appActionInput{
		ResourceVersion: optionalString{Value: validVersion, Set: true},
		AccessMode:      optionalString{Value: "public", Set: true},
	}); field != "accessMode" {
		t.Fatalf("unsafe access mode rejected on %q", field)
	}
}

func TestApplicationJobCancellationRequiresSessionOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	id := strings.Repeat("a", 32)

	response := authenticatedRequest(
		server,
		http.MethodPost,
		"/api/v1/app-jobs/"+id+"/cancel",
		nil,
		sessionCookie,
		csrfCookie,
		map[string]string{"X-CSRF-Token": csrfCookie.Value},
	)
	if response.Code != http.StatusForbidden ||
		!strings.Contains(response.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing origin status = %d body=%s", response.Code, response.Body.String())
	}

	response = authenticatedRequest(
		server,
		http.MethodPost,
		"/api/v1/app-jobs/"+id+"/cancel",
		nil,
		sessionCookie,
		csrfCookie,
		map[string]string{"Origin": "http://panel.test"},
	)
	if response.Code != http.StatusForbidden ||
		!strings.Contains(response.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF status = %d body=%s", response.Code, response.Body.String())
	}

	response = authenticatedRequest(
		server,
		http.MethodPost,
		"/api/v1/app-jobs/not-an-id/cancel",
		nil,
		sessionCookie,
		csrfCookie,
		map[string]string{
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusNotFound {
		t.Fatalf("invalid cancellation path status = %d body=%s", response.Code, response.Body.String())
	}
}
