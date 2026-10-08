package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileWritePolicyUsesAgentAuthAndReadOnlyMethod(t *testing.T) {
	server := testServer(t)
	for _, test := range []struct {
		method, query string
		auth          bool
		status        int
	}{
		{http.MethodGet, "path=%2Fhome%2Fdocker%2Fdemo%2Fapp.conf", false, 401},
		{http.MethodPost, "path=%2Fhome%2Fdocker%2Fdemo%2Fapp.conf", true, 405},
		{http.MethodGet, "path=x&path=y", true, 400},
		{http.MethodGet, "path=x&unknown=y", true, 400},
	} {
		request := httptest.NewRequest(test.method, "/v1/files/write-policy?"+test.query, nil)
		if test.auth {
			request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("method=%s query=%s status=%d body=%s", test.method, test.query, response.Code, response.Body.String())
		}
	}
}
