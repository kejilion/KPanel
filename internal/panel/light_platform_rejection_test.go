package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestEnrollmentRejectsRemovedPlatformOptions(t *testing.T) {
	server, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, server, tokenPath)
	server.config.PublicURL = "https://panel.test"
	headers := map[string]string{
		"Content-Type": "application/json",
		"Origin":       "https://panel.test",
		"X-CSRF-Token": csrf.Value,
	}
	requests := []struct {
		path string
		body any
	}{
		{
			path: "/api/v1/cluster/light-enrollments",
			body: map[string]any{"name": "test host", "platform": "unsupported"},
		},
		{
			path: "/api/v1/cluster/light-batch-enrollments",
			body: map[string]any{"platform": "unsupported", "maxUses": 2, "expiresInSeconds": 3600},
		},
	}
	for _, request := range requests {
		t.Run(request.path, func(t *testing.T) {
			body, err := json.Marshal(request.body)
			if err != nil {
				t.Fatal(err)
			}
			response := authenticatedRequest(server, http.MethodPost, request.path, body, cookie, csrf, headers)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_json") {
				t.Fatalf("removed platform option response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	response := authenticatedRequest(server, http.MethodGet, "/api/v1/cluster/light-batch-enrollments", nil, cookie, csrf, headers)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":0`) {
		t.Fatalf("rejected requests changed batch enrollment state: %d %s", response.Code, response.Body.String())
	}
}
