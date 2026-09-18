package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSHDefenseRoutesRejectQueriesUnknownFieldsAndInvalidActions(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)
	request := httptest.NewRequest(http.MethodGet, "/v1/system/ssh-defense?raw=1", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_system_resource_url") {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}
	version := strings.Repeat("a", 64)
	for _, test := range []struct {
		name string
		body string
		code string
	}{
		{name: "unknown field", body: `{"action":"disable","expectedResourceVersion":"` + version + `","command":"systemctl"}`, code: "invalid_request"},
		{name: "invalid profile", body: `{"action":"set-profile","expectedResourceVersion":"` + version + `","profile":"maximum"}`, code: "invalid_ssh_defense_action"},
		{name: "unban cidr", body: `{"action":"unban","expectedResourceVersion":"` + version + `","address":"203.0.113.0/24"}`, code: "invalid_ssh_defense_action"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/system/ssh-defense/actions", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code == http.StatusOK || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
