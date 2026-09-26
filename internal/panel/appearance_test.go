package panel

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAppearanceSettingsAuthenticationValidationAndConflict(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	if response := performRequest(server, http.MethodGet, appearancePath, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read = %d", response.Code)
	}
	read := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, appearancePath, nil, false)
	if read.Code != http.StatusOK {
		t.Fatalf("read = %d %s", read.Code, read.Body.String())
	}
	var initial appearanceResponse
	if err := json.Unmarshal(read.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Configured || initial.Theme != "system" || initial.Wallpaper != "classic" {
		t.Fatalf("initial = %#v", initial)
	}
	body, _ := json.Marshal(map[string]any{
		"theme": "dark", "wallpaper": "orbit", "classicLevel": "clear",
		"colors":                  map[string]any{"brand": "#356fc0", "neutral": "#34465c", "signature": "#23a6bd", "signatureLinked": false},
		"expectedResourceVersion": initial.ResourceVersion,
	})
	if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, appearancePath, body, false); response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodPut, appearancePath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrfCookie.Value,
	}); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin write = %d", response.Code)
	}
	updated := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, appearancePath, body, true)
	if updated.Code != http.StatusOK {
		t.Fatalf("update = %d %s", updated.Code, updated.Body.String())
	}
	var saved appearanceResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Configured || saved.Theme != "dark" || saved.Wallpaper != "orbit" || saved.ClassicLevel != "clear" || saved.Colors.Brand != "#356fc0" {
		t.Fatalf("saved = %#v", saved)
	}
	if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, appearancePath, body, true); response.Code != http.StatusConflict {
		t.Fatalf("stale version = %d", response.Code)
	}
	invalid := []byte(`{"theme":"dark","wallpaper":"url(javascript:bad)","classicLevel":"clear","expectedResourceVersion":"` + saved.ResourceVersion + `"}`)
	if response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, appearancePath, invalid, true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid wallpaper = %d", response.Code)
	}
	current := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, appearancePath, nil, false)
	var reread appearanceResponse
	if err := json.Unmarshal(current.Body.Bytes(), &reread); err != nil {
		t.Fatal(err)
	}
	if reread.ResourceVersion != saved.ResourceVersion || reread.Wallpaper != "orbit" {
		t.Fatalf("failed writes changed setting: %#v", reread)
	}
}
