package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type telemetrySSHLoginSource struct {
	event *contract.SSHLoginEvent
	calls int
}

func (s *telemetrySSHLoginSource) LatestSSHLogin(context.Context) (*contract.SSHLoginEvent, error) {
	s.calls++
	if s.event == nil {
		return nil, nil
	}
	copy := *s.event
	return &copy, nil
}

func TestSystemTelemetryRequiresAuthenticationAndIsReadOnly(t *testing.T) {
	server := testServer(t)

	request := httptest.NewRequest(http.MethodGet, "/v1/system/telemetry", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated telemetry status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/system/telemetry", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("telemetry POST status = %d, want %d; body=%s", response.Code, http.StatusMethodNotAllowed, response.Body.String())
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("telemetry Allow = %q, want %q", allow, http.MethodGet)
	}
}

func TestSystemTelemetryReturnsOnlyFederationSafeFields(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/system/telemetry", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("telemetry status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode telemetry: %v", err)
	}
	allowed := map[string]bool{
		"agentVersion": true, "agentProtocolVersion": true,
		"hostname": true, "os": true, "osId": true, "osLike": true,
		"kernel": true, "architecture": true, "uptimeSeconds": true,
		"load": true, "cpu": true, "memory": true, "disk": true,
		"network": true, "publicNetwork": true, "sshLogin": true, "collectedAt": true,
	}
	for field := range payload {
		if !allowed[field] {
			t.Fatalf("telemetry exposed non-federation field %q; body=%s", field, response.Body.String())
		}
	}
	for _, required := range []string{
		"agentVersion", "agentProtocolVersion", "hostname", "os",
		"uptimeSeconds", "load", "cpu", "memory", "disk", "network",
		"publicNetwork", "collectedAt",
	} {
		if _, ok := payload[required]; !ok {
			t.Fatalf("telemetry omitted required field %q; body=%s", required, response.Body.String())
		}
	}
	for _, forbidden := range []string{
		"management", "sites", "applications", "docker", "credentials",
		"environment", "capabilities",
	} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("telemetry exposed forbidden field %q", forbidden)
		}
	}
}

func TestTelemetryCapabilityRequiresTheSingleKnownQuery(t *testing.T) {
	valid := httptest.NewRequest(http.MethodGet, "/v1/system/telemetry?capabilities=ssh-login-v1", nil)
	if !hasTelemetryCapability(valid, "ssh-login-v1") {
		t.Fatal("known telemetry capability was not accepted")
	}
	for _, target := range []string{
		"/v1/system/telemetry",
		"/v1/system/telemetry?capabilities=ssh-login-v1&extra=x",
		"/v1/system/telemetry?capabilities=other",
		"/v1/system/telemetry?capabilities=ssh-login-v1;extra=x",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if hasTelemetryCapability(request, "ssh-login-v1") {
			t.Fatalf("unexpected telemetry capability acceptance for %q", target)
		}
	}
}

func TestSystemTelemetryUsesClusterSSHLoginSourceOnlyWhenNegotiated(t *testing.T) {
	server := testServer(t)
	source := &telemetrySSHLoginSource{event: &contract.SSHLoginEvent{
		ID: "event-1", OccurredAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
		Username: "admin", RemoteAddress: "203.0.113.9", Method: "publickey",
	}}
	server.sshLoginSource = source
	authorization := "Bearer " + strings.Repeat("x", 32)

	request := httptest.NewRequest(http.MethodGet, "/v1/system/telemetry", nil)
	request.Header.Set("Authorization", authorization)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || source.calls != 0 || strings.Contains(response.Body.String(), "sshLogin") {
		t.Fatalf("telemetry without capability status=%d calls=%d body=%s", response.Code, source.calls, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/system/telemetry?capabilities=ssh-login-v1", nil)
	request.Header.Set("Authorization", authorization)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || source.calls != 1 || !strings.Contains(response.Body.String(), `"sshLogin"`) {
		t.Fatalf("negotiated telemetry status=%d calls=%d body=%s", response.Code, source.calls, response.Body.String())
	}
}
