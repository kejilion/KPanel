package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

// TestPanelSessionIsNotAcceptedOnBrowseEndpoints is the central claim of the
// origin split. A page rewritten by Scramjet runs on the browse origin; if a
// panel session could authenticate a browse endpoint the two credentials would
// be interchangeable and separating the origins would buy nothing. The panel's
// cookies are host-only so a browser would never send them here at all — this
// asserts the server refuses them even when they are presented directly.
func TestPanelSessionIsNotAcceptedOnBrowseEndpoints(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	body, _ := json.Marshal(browseFetchRequest{URL: "https://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		panelSession, panelCSRF, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin,
			"X-CSRF-Token": panelCSRF.Value,
		})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("panel session was accepted on a browse endpoint: %d %s", response.Code, response.Body.String())
	}
}

// TestBrowseSessionIsNotAcceptedOnPanelEndpoints is the same claim in the
// direction that actually matters for impact: this is what an escaped page
// would hold, and it must not open /api/v1/audit, /api/v1/files or anything
// else on the panel.
func TestBrowseSessionIsNotAcceptedOnPanelEndpoints(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	browseSession, browseCSRF := bootstrapBrowseCookies(t, server, tokenPath)

	for _, path := range []string{"/api/v1/audit", "/api/v1/files", "/api/v1/cluster/hosts"} {
		response := browseAuthenticatedRequest(server, http.MethodGet, path, nil,
			browseSession, browseCSRF, map[string]string{
				"Host": browseTestPanelHost, "X-CSRF-Token": browseCSRF.Value,
			})
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s accepted a browse credential: %d %s", path, response.Code, response.Body.String())
		}
	}
}

// TestBrowseOriginServesNothingButTheShellAndBrowseEndpoints locks
// serveBrowseOrigin down as an allowlist. Every panel route reachable here
// would be reachable by browsed content, so the failure mode of a mistake in
// that switch is total.
func TestBrowseOriginServesNothingButTheShellAndBrowseEndpoints(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	browseSession, browseCSRF := bootstrapBrowseCookies(t, server, tokenPath)

	panelPaths := []string{
		"/", "/overview", "/settings",
		"/api/v1/health", "/api/v1/audit", "/api/v1/files", "/api/v1/files/content",
		"/api/v1/cluster/hosts", "/api/v1/auth/session", "/api/v1/settings/allowed-hosts",
		"/api/v1/terminal-sessions", "/api/v1/browse/handoff",
	}
	for _, path := range panelPaths {
		response := browseAuthenticatedRequest(server, http.MethodGet, path, nil,
			browseSession, browseCSRF, map[string]string{"X-CSRF-Token": browseCSRF.Value})
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s is reachable on the browse origin: %d %s", path, response.Code, response.Body.String())
		}
	}
}

// TestBrowseOriginIsNotAcceptedAsAPanelOrigin guards the specific mistake the
// two settings fields exist to prevent. checkHost admits the browse origin so
// ServeHTTP can route it, and it would be easy to conclude the allowlist should
// admit it too — but originHostIsAllowlisted feeds Origin validation on panel
// endpoints, and accepting the browse origin there would let browsed content
// call the panel API cross-origin.
func TestBrowseOriginIsNotAcceptedAsAPanelOrigin(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	body, _ := json.Marshal(map[string]any{
		"hosts": []string{}, "browseOrigin": browseTestHost,
		"expectedResourceVersion": "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	response := browseAuthenticatedRequest(server, http.MethodPut, "/api/v1/settings/allowed-hosts", body,
		panelSession, panelCSRF, map[string]string{
			"Host": browseTestPanelHost, "Content-Type": "application/json",
			"Origin": browseTestOrigin, "X-CSRF-Token": panelCSRF.Value,
		})
	if response.Code != http.StatusForbidden {
		t.Fatalf("panel endpoint accepted the browse origin: %d %s", response.Code, response.Body.String())
	}
}

// TestBrowseHandoffTicketIsSingleUse: the ticket travels in a URL, so it can
// land in a Referer header or a history entry. Being one-shot and short-lived
// is what keeps that from mattering.
func TestBrowseHandoffTicketIsSingleUse(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	minted := browseRequest(server, http.MethodPost, "/api/v1/browse/handoff", nil,
		panelSession, panelCSRF, map[string]string{
			"Host": browseTestPanelHost, "Origin": browseTestPanelOrigin,
			"X-CSRF-Token": panelCSRF.Value,
		})
	if minted.Code != http.StatusCreated {
		t.Fatalf("handoff = %d %s", minted.Code, minted.Body.String())
	}
	var handoff browseHandoffResponse
	if err := json.Unmarshal(minted.Body.Bytes(), &handoff); err != nil {
		t.Fatal(err)
	}
	ticket := handoff.URL[len(handoff.URL)-43:]

	first := browsePerformRequest(server, http.MethodGet, browseEnterPath+"?t="+ticket, nil, nil)
	if first.Code != http.StatusSeeOther {
		t.Fatalf("first redemption = %d %s", first.Code, first.Body.String())
	}
	second := browsePerformRequest(server, http.MethodGet, browseEnterPath+"?t="+ticket, nil, nil)
	if second.Code != http.StatusForbidden {
		t.Fatalf("ticket was reusable: %d", second.Code)
	}
}

// TestBrowseFeatureFailsClosedWithoutABrowseOrigin covers the operator-facing
// half: with nothing configured the handoff reports a distinct code the UI
// turns into "go set it", and the shell is not quietly served beside the panel
// instead.
func TestBrowseFeatureFailsClosedWithoutABrowseOrigin(t *testing.T) {
	server, tokenPath := newTestServerWithPublicURL(t, browseTestPanelOrigin)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	response := browseRequest(server, http.MethodPost, "/api/v1/browse/handoff", nil,
		panelSession, panelCSRF, map[string]string{
			"Host": browseTestPanelHost, "Origin": browseTestPanelOrigin,
			"X-CSRF-Token": panelCSRF.Value,
		})
	if response.Code != http.StatusPreconditionFailed {
		t.Fatalf("handoff = %d %s", response.Code, response.Body.String())
	}
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &problem)
	if problem.Code != "browse_origin_not_configured" {
		t.Fatalf("code = %q", problem.Code)
	}

	for _, path := range []string{"/browser-app/app.html", "/scramjet-sw.js", "/scram/browse-transport.js"} {
		shell := browsePerformRequest(server, http.MethodGet, path, nil,
			map[string]string{"Host": browseTestPanelHost})
		if shell.Code != http.StatusNotFound {
			t.Fatalf("%s served on the panel origin: %d", path, shell.Code)
		}
	}
}

// TestLogoutRevokesBrowseSessions: the browse credential is independent by
// construction, which is exactly why logging out has to reach across and
// invalidate it explicitly.
func TestLogoutRevokesBrowseSessions(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)
	browseSession, browseCSRF := browseHandoffCookies(t, server, panelSession, panelCSRF)

	before := browseAuthenticatedRequest(server, http.MethodGet, "/api/v1/browse/hosts", nil,
		browseSession, browseCSRF, nil)
	if before.Code != http.StatusOK {
		t.Fatalf("browse session unusable before logout: %d %s", before.Code, before.Body.String())
	}

	logout := browseRequest(server, http.MethodPost, "/api/v1/auth/logout", nil,
		panelSession, panelCSRF, map[string]string{
			"Host": browseTestPanelHost, "Origin": browseTestPanelOrigin,
			"X-CSRF-Token": panelCSRF.Value,
		})
	if logout.Code != http.StatusOK {
		t.Fatalf("logout = %d %s", logout.Code, logout.Body.String())
	}

	after := browseAuthenticatedRequest(server, http.MethodGet, "/api/v1/browse/hosts", nil,
		browseSession, browseCSRF, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("browse session survived logout: %d %s", after.Code, after.Body.String())
	}
}

// TestNormalizeBrowseOriginRejectsCollisionsWithThePanel covers the validation
// that keeps the two fields from being pointed at the same name — the one
// operator mistake that silently removes the isolation entirely.
func TestNormalizeBrowseOriginRejectsCollisionsWithThePanel(t *testing.T) {
	const publicURL = "https://panel.example.com"

	if _, err := normalizeBrowseOrigin("panel.example.com", nil, publicURL); err == nil {
		t.Fatal("browse origin equal to publicUrl's host was accepted")
	}
	if _, err := normalizeBrowseOrigin("extra.example.com", []string{"extra.example.com"}, publicURL); err == nil {
		t.Fatal("browse origin duplicated in the panel allowlist was accepted")
	}
	if _, err := normalizeBrowseOrigin("browse.example.com", nil, ""); err == nil {
		t.Fatal("browse origin accepted without a configured publicUrl")
	}
	if _, err := normalizeBrowseOrigin("not a hostname", nil, publicURL); err == nil {
		t.Fatal("malformed hostname was accepted")
	}

	// A pasted URL is the likeliest operator input; accept it by taking the host.
	origin, err := normalizeBrowseOrigin("https://Browse.Example.com/", nil, publicURL)
	if err != nil {
		t.Fatalf("valid browse origin rejected: %v", err)
	}
	if origin != "browse.example.com" {
		t.Fatalf("origin = %q, want the lowercased host", origin)
	}

	// Empty means "feature off" and must stay a legal value.
	if origin, err := normalizeBrowseOrigin("  ", nil, publicURL); err != nil || origin != "" {
		t.Fatalf("empty browse origin = %q, %v", origin, err)
	}
}

// TestAllowedHostsResourceVersionCoversBrowseOrigin keeps optimistic
// concurrency honest: the two fields are saved by one form, so a change to
// either has to invalidate a concurrent editor's version.
func TestAllowedHostsResourceVersionCoversBrowseOrigin(t *testing.T) {
	at := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	base := store.AllowedHosts{Hosts: []string{"panel.example.com"}, UpdatedAt: at}
	changed := store.AllowedHosts{Hosts: []string{"panel.example.com"}, BrowseOrigin: "browse.example.com", UpdatedAt: at}

	if store.AllowedHostsResourceVersion(base) == store.AllowedHostsResourceVersion(changed) {
		t.Fatal("resourceVersion ignores browseOrigin, so concurrent edits would silently overwrite")
	}

	// Same argument for the third field on that form, which widens the browse
	// egress to the LAN and so must never be lost to a stale concurrent save.
	relaxed := changed
	relaxed.BrowseAllowPrivateNetworks = true
	if store.AllowedHostsResourceVersion(changed) == store.AllowedHostsResourceVersion(relaxed) {
		t.Fatal("resourceVersion ignores browseAllowPrivateNetworks")
	}
}

// TestAllowedHostsSettingsRoundTripsThePrivateNetworkOptIn covers the third
// field on the allowlist form end to end, including that saving the other two
// without mentioning it turns it back off — the setting widens the browse
// egress, so "absent" has to mean "off" rather than "keep whatever was there".
func TestAllowedHostsSettingsRoundTripsThePrivateNetworkOptIn(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	save := func(t *testing.T, payload map[string]any) allowedHostsResponse {
		t.Helper()
		body, _ := json.Marshal(payload)
		response := browseAuthenticatedRequest(server, http.MethodPut, "/api/v1/settings/allowed-hosts", body,
			panelSession, panelCSRF, map[string]string{
				"Host": browseTestPanelHost, "Content-Type": "application/json",
				"Origin": browseTestPanelOrigin, "X-CSRF-Token": panelCSRF.Value,
			})
		if response.Code != http.StatusOK {
			t.Fatalf("save = %d %s", response.Code, response.Body.String())
		}
		var saved allowedHostsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}

	_, version := server.store.AllowedHosts()
	saved := save(t, map[string]any{
		"hosts": []string{}, "browseOrigin": browseTestHost,
		"browseAllowPrivateNetworks": true,
		"expectedResourceVersion":    version,
	})
	if !saved.BrowseAllowPrivateNetworks {
		t.Fatal("response did not echo the opt-in")
	}
	if !server.store.BrowseAllowPrivateNetworks() {
		t.Fatal("store did not record the opt-in")
	}

	saved = save(t, map[string]any{
		"hosts": []string{}, "browseOrigin": browseTestHost,
		"expectedResourceVersion": saved.ResourceVersion,
	})
	if saved.BrowseAllowPrivateNetworks || server.store.BrowseAllowPrivateNetworks() {
		t.Fatal("omitting browseAllowPrivateNetworks left the LAN egress on")
	}
}

// TestBrowseHandoffURLFollowsThePanelScheme locks the loopback verification
// path documented in docs/deployment.md: the handoff must not send a panel
// reached over http://localhost to an https port that is not listening.
func TestBrowseHandoffURLFollowsThePanelScheme(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	panelSession, panelCSRF := bootstrapPanelCookies(t, server, tokenPath)

	minted := browseRequest(server, http.MethodPost, "/api/v1/browse/handoff", nil,
		panelSession, panelCSRF, map[string]string{
			"Host": browseTestPanelHost, "Origin": browseTestPanelOrigin,
			"X-CSRF-Token": panelCSRF.Value,
		})
	if minted.Code != http.StatusCreated {
		t.Fatalf("handoff = %d %s", minted.Code, minted.Body.String())
	}
	var handoff browseHandoffResponse
	if err := json.Unmarshal(minted.Body.Bytes(), &handoff); err != nil {
		t.Fatal(err)
	}
	// newBrowseTestServer's publicUrl is http://localhost.
	if !strings.HasPrefix(handoff.URL, "http://"+browseTestHost+browseEnterPath) {
		t.Fatalf("handoff url = %q, want an http:// URL on the browse origin", handoff.URL)
	}

	// The default stays https for every deployment that is not plainly http.
	server.config.PublicURL = "https://panel.example.com"
	if scheme := server.browseOriginScheme(); scheme != "https" {
		t.Fatalf("browseOriginScheme() = %q, want https", scheme)
	}
}

// TestPanelFrameSrcNamesTheBrowseOrigin covers the other half of the iframe
// permission. frame-ancestors on the browse origin is useless on its own: the
// panel page has to be allowed to frame that origin too, and the default
// "frame-src 'self' blob:" does not allow it now that the shell lives
// elsewhere.
func TestPanelFrameSrcNamesTheBrowseOrigin(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapPanelCookies(t, server, tokenPath)

	response := browseAuthenticatedRequest(server, http.MethodGet, "/api/v1/auth/session", nil,
		sessionCookie, csrfCookie, map[string]string{"Host": browseTestPanelHost})
	policy := response.Header().Get("Content-Security-Policy")
	want := "frame-src 'self' blob: http://" + browseTestHost + ";"
	if !strings.Contains(policy, want) {
		t.Fatalf("panel CSP = %q, want it to contain %q", policy, want)
	}

	// With no browse origin configured the policy must not grow a stray entry.
	current, version := server.store.AllowedHosts()
	current.BrowseOrigin = ""
	current.UpdatedAt = time.Now().UTC()
	if err := server.store.ReplaceAllowedHosts(version, current); err != nil {
		t.Fatal(err)
	}
	policy = server.panelContentSecurityPolicy()
	if !strings.Contains(policy, "frame-src 'self' blob:;") {
		t.Fatalf("panel CSP without a browse origin = %q", policy)
	}
}
