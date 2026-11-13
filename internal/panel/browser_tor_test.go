package panel

import (
	"context"
	"strings"
	"testing"
)

func TestTorBrowserOnlyMapsNativeDesktopEndpoints(t *testing.T) {
	for path, expected := range map[string]string{
		"/healthz": "/",
		"/v1/sessions/" + strings.Repeat("a", 64) + "/vnc": "/websockify",
	} {
		request, err := torBrowserRequest(context.Background(), "http://browser:8092", path)
		if err != nil || request.URL.Host != "browser:8092" || request.URL.Path != expected || len(request.Header) != 0 {
			t.Fatal(request, err)
		}
	}
	for _, path := range []string{"/", "/v1/sessions/" + strings.Repeat("a", 64), "/v1/sessions/../../vnc", "/v1/sessions/a/vnc?x=1", "https://evil.test/vnc"} {
		if _, err := torBrowserRequest(context.Background(), "http://browser:8092", path); err == nil {
			t.Fatal("unexpected endpoint", path)
		}
	}
}
