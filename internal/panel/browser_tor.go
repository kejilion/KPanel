package panel

import (
	"context"
	"errors"
	"net/http"
	"regexp"
)

var torBrowserSessionPath = regexp.MustCompile(`^/v1/sessions/[a-f0-9]{64}/vnc$`)

// Only the two fixed native desktop endpoints are reachable. The caller has
// already authenticated the administrator, Origin and first-message CSRF.
// No upstream HTML or JavaScript is served under the panel's origin.
func torBrowserRequest(ctx context.Context, serviceURL, path string) (*http.Request, error) {
	switch {
	case path == "/healthz":
		path = "/"
	case torBrowserSessionPath.MatchString(path):
		path = "/websockify"
	default:
		return nil, errors.New("browser_incompatible")
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, serviceURL+path, nil)
}
