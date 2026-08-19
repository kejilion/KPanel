package panel

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
)

const panelTestShortcutID = "0123456789abcdef0123456789abcdef"

func TestDesktopWorkspaceAPIAuthenticationValidationAndConflict(t *testing.T) {
	server, tokenPath := newTestServer(t)
	path := "/api/v1/desktop/workspace"
	unauthenticated := performRequest(server, http.MethodGet, path, nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d", unauthenticated.Code)
	}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	get := authenticatedRequest(server, http.MethodGet, path, nil, sessionCookie, csrfCookie, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET workspace failed: %d %s", get.Code, get.Body.String())
	}
	var initial desktopworkspace.Workspace
	if err := json.Unmarshal(get.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if !initial.Available || !desktopworkspace.ValidResourceVersion(initial.ResourceVersion) {
		t.Fatalf("unexpected initial workspace: %#v", initial)
	}
	invalidQuery := authenticatedRequest(server, http.MethodGet, path+"?extra=1", nil, sessionCookie, csrfCookie, nil)
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid workspace query status = %d", invalidQuery.Code)
	}

	input := desktopworkspace.ReplaceInput{
		ExpectedResourceVersion: initial.ResourceVersion,
		HiddenEntryKeys:         []string{"app:builtin-1"},
		Positions: map[string]desktopworkspace.Position{
			"nav:/overview":                   {X: 0.1, Y: 2.2},
			"shortcut:" + panelTestShortcutID: {X: 0.4, Y: 0.5},
		},
		Shortcuts: []desktopworkspace.ShortcutInput{{
			ID: panelTestShortcutID, Name: "Control", Description: "audit-secret-description",
			TargetType: desktopworkspace.ShortcutTargetURL, URL: "https://example.com/audit-secret-url",
		}, {
			ID: "ffffffffffffffffffffffffffffffff", Name: "Config",
			TargetType: desktopworkspace.ShortcutTargetFile, Path: "/etc/audit-secret-path.conf",
		}},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	missingOrigin := authenticatedRequest(server, http.MethodPut, path, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	})
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin response = %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(server, http.MethodPut, path, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF response = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	savedResponse := authenticatedRequest(server, http.MethodPut, path, body, sessionCookie, csrfCookie, headers)
	if savedResponse.Code != http.StatusOK {
		t.Fatalf("PUT workspace failed: %d %s", savedResponse.Code, savedResponse.Body.String())
	}
	var saved desktopworkspace.Workspace
	if err := json.Unmarshal(savedResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResourceVersion == initial.ResourceVersion || len(saved.Shortcuts) != 2 ||
		saved.Shortcuts[1].TargetType != desktopworkspace.ShortcutTargetFile ||
		saved.Shortcuts[1].Path != "/etc/audit-secret-path.conf" ||
		saved.Positions["nav:/overview"] != (desktopworkspace.Position{X: 0.1, Y: 2.2}) {
		t.Fatalf("workspace did not update: %#v", saved)
	}

	stale := authenticatedRequest(server, http.MethodPut, path, body, sessionCookie, csrfCookie, headers)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "desktop_workspace_changed") {
		t.Fatalf("stale workspace response = %d %s", stale.Code, stale.Body.String())
	}

	events, _ := server.store.ListAudit(20, "")
	auditJSON, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(auditJSON, []byte("audit-secret-url")) || bytes.Contains(auditJSON, []byte("audit-secret-description")) ||
		bytes.Contains(auditJSON, []byte("audit-secret-path")) {
		t.Fatalf("workspace audit leaked URL or description: %s", auditJSON)
	}
	if !bytes.Contains(auditJSON, []byte("changedCounts")) {
		t.Fatalf("workspace audit lacks bounded change counts: %s", auditJSON)
	}
}

func TestDesktopWorkspaceUnavailableStateIsReadOnly(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	workspacePath := filepath.Join(server.config.DataDir, "desktop-workspace", "workspace.json")
	corrupt := []byte("{corrupt")
	if err := os.WriteFile(workspacePath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	degraded, err := desktopworkspace.Open(filepath.Dir(workspacePath))
	if err != nil {
		t.Fatal(err)
	}
	server.desktopWorkspace = degraded

	get := authenticatedRequest(server, http.MethodGet, "/api/v1/desktop/workspace", nil, sessionCookie, csrfCookie, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("degraded GET failed: %d %s", get.Code, get.Body.String())
	}
	var workspace desktopworkspace.Workspace
	if err := json.Unmarshal(get.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Available || workspace.Warning == "" {
		t.Fatalf("degraded workspace was not marked unavailable: %#v", workspace)
	}
	body, _ := json.Marshal(desktopworkspace.ReplaceInput{ExpectedResourceVersion: workspace.ResourceVersion})
	put := authenticatedRequest(server, http.MethodPut, "/api/v1/desktop/workspace", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if put.Code != http.StatusServiceUnavailable || !strings.Contains(put.Body.String(), "desktop_workspace_unavailable") {
		t.Fatalf("degraded PUT response = %d %s", put.Code, put.Body.String())
	}
	after, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatal("degraded PUT overwrote corrupt metadata")
	}
}

func TestDesktopShortcutIconAPIValidationCachingAndDeletion(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	workspace := server.desktopWorkspace.Workspace()
	workspace, err := server.desktopWorkspace.Replace(desktopworkspace.ReplaceInput{
		ExpectedResourceVersion: workspace.ResourceVersion,
		Shortcuts: []desktopworkspace.ShortcutInput{{
			ID: panelTestShortcutID, Name: "Console", URL: "https://example.com/",
		}},
	})
	if err != nil || len(workspace.Shortcuts) != 1 {
		t.Fatalf("prepare shortcut: %#v, %v", workspace, err)
	}
	path := "/api/v1/desktop/shortcuts/" + panelTestShortcutID + "/icon"

	unauthenticated := performRequest(server, http.MethodGet, path, nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated icon GET status = %d", unauthenticated.Code)
	}
	writeHeaders := map[string]string{
		"Content-Type": "image/png", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	missingCSRF := authenticatedRequest(server, http.MethodPut, path, []byte("x"), sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "image/png", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing icon CSRF status = %d", missingCSRF.Code)
	}
	forged := authenticatedRequest(server, http.MethodPut, path, []byte(`<svg/>`), sessionCookie, csrfCookie, writeHeaders)
	if forged.Code != http.StatusUnprocessableEntity || !strings.Contains(forged.Body.String(), "validation_failed") {
		t.Fatalf("forged PNG response = %d %s", forged.Code, forged.Body.String())
	}
	svg := authenticatedRequest(server, http.MethodPut, path, []byte(`<svg/>`), sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "image/svg+xml", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if svg.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("SVG status = %d", svg.Code)
	}
	tooLarge := authenticatedRequest(server, http.MethodPut, path, bytes.Repeat([]byte("x"), desktopworkspace.MaxIconBytes+1), sessionCookie, csrfCookie, writeHeaders)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized icon status = %d", tooLarge.Code)
	}

	pngData := panelTestPNG(t)
	put := authenticatedRequest(server, http.MethodPut, path, pngData, sessionCookie, csrfCookie, writeHeaders)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT icon failed: %d %s", put.Code, put.Body.String())
	}
	var result struct {
		IconVersion string `json:"iconVersion"`
		IconURL     string `json:"iconURL"`
	}
	if err := json.Unmarshal(put.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.IconVersion) != 64 || result.IconURL != path+"?v="+result.IconVersion {
		t.Fatalf("unexpected icon response: %#v", result)
	}

	get := authenticatedRequest(server, http.MethodGet, path, nil, sessionCookie, csrfCookie, nil)
	if get.Code != http.StatusOK || !bytes.Equal(get.Body.Bytes(), pngData) {
		t.Fatalf("GET icon response = %d %d bytes", get.Code, get.Body.Len())
	}
	if get.Header().Get("Content-Type") != "image/png" || get.Header().Get("X-Content-Type-Options") != "nosniff" ||
		get.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("unexpected icon headers: %v", get.Header())
	}
	versioned := authenticatedRequest(server, http.MethodGet, result.IconURL, nil, sessionCookie, csrfCookie, nil)
	if versioned.Code != http.StatusOK || !strings.Contains(versioned.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("versioned icon response = %d %v", versioned.Code, versioned.Header())
	}
	notModified := authenticatedRequest(server, http.MethodGet, result.IconURL, nil, sessionCookie, csrfCookie, map[string]string{
		"If-None-Match": get.Header().Get("ETag"),
	})
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional icon response = %d %q", notModified.Code, notModified.Body.String())
	}
	invalidQuery := authenticatedRequest(server, http.MethodGet, path+"?v="+strings.ToUpper(result.IconVersion), nil, sessionCookie, csrfCookie, nil)
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid icon query status = %d", invalidQuery.Code)
	}

	workspaceResponse := authenticatedRequest(server, http.MethodGet, "/api/v1/desktop/workspace", nil, sessionCookie, csrfCookie, nil)
	var enriched desktopworkspace.Workspace
	if workspaceResponse.Code != http.StatusOK || json.Unmarshal(workspaceResponse.Body.Bytes(), &enriched) != nil ||
		len(enriched.Shortcuts) != 1 || enriched.Shortcuts[0].IconURL != result.IconURL {
		t.Fatalf("workspace icon metadata missing: %d %#v", workspaceResponse.Code, enriched)
	}

	deleted := authenticatedRequest(server, http.MethodDelete, path, nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("DELETE icon failed: %d %s", deleted.Code, deleted.Body.String())
	}
	missing := authenticatedRequest(server, http.MethodGet, path, nil, sessionCookie, csrfCookie, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted icon GET status = %d", missing.Code)
	}
}

func panelTestPNG(t *testing.T) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: 32, G: 160, B: 208, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
