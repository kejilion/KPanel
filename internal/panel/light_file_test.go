package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestIsLightFileRelayRequestUsesTheSharedRouteAllowlist(t *testing.T) {
	hostID := strings.Repeat("a", 32)
	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/v1/files?hostId=" + hostID, want: true},
		{path: "/api/v1/files/content?hostId=" + hostID, want: true},
		{path: "/api/v1/files/actions?hostId=" + hostID, want: true},
		{path: "/api/v1/files/transfers?hostId=" + hostID, want: false},
		{path: "/api/v1/files/download-tickets?hostId=" + hostID, want: false},
		{path: "/api/v1/files/content", want: false},
	}
	for _, test := range tests {
		requestURL, err := url.Parse(test.path)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", test.path, err)
		}
		request := &http.Request{Method: http.MethodGet, URL: requestURL}
		if got := isLightFileRelayRequest(request); got != test.want {
			t.Errorf("isLightFileRelayRequest(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}
