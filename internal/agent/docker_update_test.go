package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
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

func TestDockerImageUpdateErrorCategories(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{dockerx.ErrImageUpdateDigestMissing, "docker_update_digest_missing"},
		{dockerx.ErrImageUpdateIncomparable, "docker_update_incomparable"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "docker_update_timeout"},
		{&dockerx.APIError{Status: 401}, "docker_update_registry_auth"},
		{&dockerx.APIError{Status: 500, Message: "unauthorized: secret credential"}, "docker_update_registry_auth"},
		{&dockerx.APIError{Status: 429}, "docker_update_rate_limited"},
		{&dockerx.APIError{Status: 500, Message: "toomanyrequests"}, "docker_update_rate_limited"},
		{&dockerx.APIError{Status: 500, Message: "manifest unknown"}, "docker_update_registry_missing"},
		{&dockerx.APIError{Status: 404}, "docker_update_registry_missing"},
		{&dockerx.APIError{Status: 504}, "docker_update_timeout"},
		{&dockerx.APIError{Status: 500, Message: "dial tcp: i/o timeout"}, "docker_update_timeout"},
		{&dockerx.APIError{Status: 500, Message: "unexpected"}, "docker_update_unavailable"},
	} {
		if got := dockerImageUpdateErrorCode(tc.err); got != tc.code {
			t.Errorf("category=%s want=%s", got, tc.code)
		}
	}
}
