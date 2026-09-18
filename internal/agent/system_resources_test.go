package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

func TestSystemResourceHostsRouteRequiresBearerAndRejectsQuery(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc", "hosts"), []byte("127.0.0.1 localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server.systemManager = systemmanage.NewManager(systemmanage.Config{
		Enabled: false, EtcRoot: filepath.Join(root, "etc"),
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/system/hosts", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/system/hosts", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"address":"127.0.0.1"`) {
		t.Fatalf("hosts status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/system/hosts?all=true", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_system_resource_url") {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSystemResourceActionKeepsStrictBodyAndErrorBoundaries(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)
	version := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{
			name:   "unknown field",
			body:   `{"action":"firewall-open-all","expectedResourceVersion":"` + version + `","unknown":true}`,
			status: http.StatusBadRequest, code: "invalid_request",
		},
		{
			name:   "action-inapplicable zero field",
			body:   `{"action":"firewall-open-all","port":0,"expectedResourceVersion":"` + version + `"}`,
			status: http.StatusUnprocessableEntity, code: "invalid_system_resource_action",
		},
		{
			name:   "disabled write",
			body:   `{"action":"firewall-open-all","expectedResourceVersion":"` + version + `"}`,
			status: http.StatusForbidden, code: "system_resource_write_disabled",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/system/resource-actions", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	oversized := `{"action":"firewall-open-all","expectedResourceVersion":"` + version + `","unknown":"` +
		strings.Repeat("x", maxAgentRequestBytes) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/system/resource-actions", strings.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("oversized status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSystemResourceManualAttentionProblemsAreNotRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      string
		retryable bool
	}{
		{name: "rolled back", err: systemmanage.ErrRolledBack, code: "system_resource_rolled_back", retryable: true},
		{name: "rollback failed", err: systemmanage.ErrRollbackFailed, code: "system_resource_rollback_failed", retryable: false},
		{name: "needs attention", err: systemmanage.ErrNeedsAttention, code: "system_resource_needs_attention", retryable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, code, _, retryable := systemResourceProblem(test.err)
			if status != http.StatusServiceUnavailable || code != test.code || retryable != test.retryable {
				t.Fatalf("status=%d code=%q retryable=%v", status, code, retryable)
			}
			response := httptest.NewRecorder()
			writeProblemWithRetryable(response, "request-id", status, code, "title", "detail", retryable)
			var problem contract.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Retryable != test.retryable || problem.Code != test.code {
				t.Fatalf("serialized problem=%#v", problem)
			}
		})
	}
}
