package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDockerUpdateRouteRequiresTokenMethodAndVersion(t *testing.T) {
	s := testServer(t)
	path := "/v1/docker/containers/" + strings.Repeat("a", 64) + "/check_update"
	for _, tc := range []struct {
		method, suffix, body string
		token                bool
		status               int
	}{
		{http.MethodPost, "", `{}`, false, http.StatusUnauthorized},
		{http.MethodGet, "", `{}`, true, http.StatusMethodNotAllowed},
		{http.MethodPost, "?extra=1", `{}`, true, http.StatusMethodNotAllowed},
		{http.MethodPost, "", `{"unexpected":true}`, true, http.StatusBadRequest},
		{http.MethodPost, "", `{}`, true, http.StatusBadRequest},
	} {
		r := httptest.NewRequest(tc.method, path+tc.suffix, strings.NewReader(tc.body))
		if tc.token {
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.suffix, w.Code, w.Body.String())
		}
	}
}
