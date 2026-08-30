package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNetworkOperationsRoutesKeepStrictURLAndBodyBoundaries(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)

	request := httptest.NewRequest(http.MethodGet, "/v1/system/port-usage?raw=true", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_system_resource_url") {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}

	version := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "unknown field", body: `{"action":"disable","expectedResourceVersion":"` + version + `","command":"shutdown"}`, status: http.StatusBadRequest, code: "invalid_request"},
		{name: "disable fields", body: `{"action":"disable","expectedResourceVersion":"` + version + `","rxThresholdGiB":100}`, status: http.StatusUnprocessableEntity, code: "invalid_system_resource_action"},
		{name: "disabled write", body: `{"action":"disable","expectedResourceVersion":"` + version + `"}`, status: http.StatusForbidden, code: "system_resource_write_disabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/system/traffic-shutdown/actions", strings.NewReader(test.body))
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
