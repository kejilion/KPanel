package panel

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestClusterHistorySessionAndStrictQuery(t *testing.T) {
	s, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	path := "/api/v1/monitoring/cluster-history?hostId=" + strings.Repeat("a", 32) + "&range=6h"
	if response := performRequest(s, http.MethodGet, path, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous access: %d", response.Code)
	}
	session, csrf := bootstrapCookiesForOrigin(t, s, tokenPath, "https://panel.test")
	for _, query := range []string{"hostId=../a", "hostId=" + strings.Repeat("a", 32) + "&range=6h&range=7d", "hostId=" + strings.Repeat("a", 32) + "&start=oops", "hostId=" + strings.Repeat("a", 32) + "&command=ls", "hostId=" + strings.Repeat("a", 32) + "&range=%GG"} {
		response := authenticatedRequest(s, http.MethodGet, "/api/v1/monitoring/cluster-history?"+query, nil, session, csrf, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("accepted query %q: %d %s", query, response.Code, response.Body)
		}
	}
	response := authenticatedRequest(s, http.MethodGet, path, nil, session, csrf, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown host: %d %s", response.Code, response.Body)
	}
	// An unreachable node must fail honestly, never fabricate an empty history.
	enrollment, err := s.cluster.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	host, err := s.cluster.EnrollLightNode("198.51.100.10", cluster.LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "empty", NodeVersion: "1.13.0"})
	if err != nil {
		t.Fatal(err)
	}
	path = "/api/v1/monitoring/cluster-history?hostId=" + host.NodeID + "&range=6h"
	response = authenticatedRequest(s, http.MethodGet, path, nil, session, csrf, nil)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"host":[]`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("empty history: %d %s", response.Code, response.Body)
	}
	response = authenticatedRequest(s, http.MethodGet, strings.Replace(path, "range=6h", "range=12m", 1), nil, session, csrf, nil)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("long range must reach the node: %d", response.Code)
	}
	if _, err := s.cluster.Host(context.Background(), host.NodeID); err != nil {
		t.Fatal(err)
	}
}
