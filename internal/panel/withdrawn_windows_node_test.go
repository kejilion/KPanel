package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWindowsLightEnrollmentIsGoneInCurrentPreview(t *testing.T) {
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
			body: map[string]any{"name": "Windows host", "platform": "windows", "enableDesktop": true},
		},
		{
			path: "/api/v1/cluster/light-batch-enrollments",
			body: map[string]any{"platform": "windows", "enableDesktop": true, "maxUses": 2, "expiresInSeconds": 3600},
		},
	}
	for _, request := range requests {
		t.Run(request.path, func(t *testing.T) {
			body, err := json.Marshal(request.body)
			if err != nil {
				t.Fatal(err)
			}
			response := authenticatedRequest(server, http.MethodPost, request.path, body, cookie, csrf, headers)
			if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "cluster_light_platform_unsupported") {
				t.Fatalf("Windows enrollment response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}
