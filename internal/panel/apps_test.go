package panel

import (
	"net/http"
	"strings"
	"testing"
)

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
	}); field != "resourceVersion" {
		t.Fatalf("container resourceVersion was accepted for marker recovery on %q", field)
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
