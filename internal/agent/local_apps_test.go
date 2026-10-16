package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every request here is rejected before the Agent writes anything.
func TestLocalAppRoutesValidateBeforeWriting(t *testing.T) {
	server := testServer(t)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	for _, sample := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{http.MethodPost, "/v1/apps/local", `{"token":"notes","content":"echo hi","resourceVersion":""}`, http.StatusUnprocessableEntity, "local_app_invalid"},
		{http.MethodPost, "/v1/apps/local", `{"token":"notes","content":"x","resourceVersion":"sha256:` + strings.Repeat("a", 64) + `"}`, http.StatusBadRequest, "invalid_request"},
		{http.MethodPost, "/v1/apps/local", `{"token":"notes","content":"` + strings.Repeat("x", maxLocalAppRequestBytes) + `"}`, http.StatusBadRequest, "invalid_request"},
		{http.MethodGet, "/v1/apps/local", "", http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.MethodPut, "/v1/apps/local/notes", `{"token":"notes","content":"x","resourceVersion":""}`, http.StatusBadRequest, "resource_version_required"},
		{http.MethodPut, "/v1/apps/local/notes", `{"token":"other","content":"x","resourceVersion":"sha256:` + strings.Repeat("a", 64) + `"}`, http.StatusBadRequest, "invalid_request"},
		{http.MethodGet, "/v1/apps/local/missing-app", "", http.StatusNotFound, "app_not_found"},
		{http.MethodGet, "/v1/apps/local/missing-app/icon", "", http.StatusNotFound, "app_icon_not_found"},
		{http.MethodDelete, "/v1/apps/local/missing-app", `{"resourceVersion":"sha256:` + strings.Repeat("a", 64) + `"}`, http.StatusNotFound, "app_not_found"},
		{http.MethodPatch, "/v1/apps/local/notes", `{}`, http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.MethodGet, "/v1/apps/local/a/b/c", "", http.StatusNotFound, "not_found"},
	} {
		response := send(sample.method, sample.path, sample.body)
		if response.Code != sample.status || !strings.Contains(response.Body.String(), sample.code) {
			t.Errorf("%s %s = %d %s, want %d %s", sample.method, sample.path, response.Code,
				response.Body.String(), sample.status, sample.code)
		}
	}
}
