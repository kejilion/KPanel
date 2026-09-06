package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationResourcesAuthenticationMethodsAndReadBudget(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct {
		method, path string
		auth         bool
		want         int
	}{{"GET", "/v1/notification-resources", false, 401}, {"POST", "/v1/notification-resources", true, 405}, {"GET", "/v1/notification-resources?path=/", true, 400}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth {
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s = %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	notificationResourceGate <- struct{}{}
	defer func() { <-notificationResourceGate }()
	r := httptest.NewRequest(http.MethodGet, "/v1/notification-resources", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatalf("read budget not enforced: %d", w.Code)
	}
}
