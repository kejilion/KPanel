package panel

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestPreferencesAuthenticatedSharedStateAndFailureBoundaries(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	if r := performRequest(s, http.MethodGet, preferencesPath, nil, nil); r.Code != 401 {
		t.Fatalf("anonymous: %d", r.Code)
	}
	read := authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var initial preferencesResponse
	if err := json.Unmarshal(read.Body.Bytes(), &initial); err != nil || read.Code != 200 || read.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("read: %d %v", read.Code, err)
	}
	body, _ := json.Marshal(map[string]any{"locale": "en-US", "expectedResourceVersion": initial.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, false); r.Code != 403 {
		t.Fatalf("CSRF: %d", r.Code)
	}
	if r := authenticatedRequest(s, http.MethodPut, preferencesPath, body, session, csrf, map[string]string{"Content-Type": "application/json", "Origin": "http://evil.test", "X-CSRF-Token": csrf.Value}); r.Code != 403 {
		t.Fatalf("origin: %d", r.Code)
	}
	saved := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, true)
	if saved.Code != 200 {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, true); r.Code != 409 {
		t.Fatalf("conflict: %d", r.Code)
	}
	var state preferencesResponse
	_ = json.Unmarshal(saved.Body.Bytes(), &state)
	invalid, _ := json.Marshal(map[string]any{"locale": "unsupported", "expectedResourceVersion": state.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, invalid, true); r.Code != 422 {
		t.Fatalf("invalid: %d", r.Code)
	}
	unknown, _ := json.Marshal(map[string]any{"locale": "zh-CN", "launchPending": true, "expectedResourceVersion": state.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, unknown, true); r.Code != 400 {
		t.Fatalf("unknown fields: %d", r.Code)
	}
	read = authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var reread preferencesResponse
	_ = json.Unmarshal(read.Body.Bytes(), &reread)
	if reread.Locale != "en-US" || reread.ResourceVersion != state.ResourceVersion {
		t.Fatal("failed write changed preferences")
	}
}

func TestMediaLocationsThroughPreferencesAPI(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	read := authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var initial preferencesResponse
	if err := json.Unmarshal(read.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"musicLocation": map[string]string{"path": "/private/music", "hostId": "node-1"}, "expectedResourceVersion": initial.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, false); r.Code != 403 {
		t.Fatalf("missing csrf: %d", r.Code)
	}
	saved := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, true)
	if saved.Code != 200 {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	var state preferencesResponse
	if err := json.Unmarshal(saved.Body.Bytes(), &state); err != nil || state.MusicLocation == nil || state.MusicLocation.Path != "/private/music" {
		t.Fatalf("saved state: %v %+v", err, state)
	}
	invalid, _ := json.Marshal(map[string]any{"galleryLocation": map[string]string{"path": "/../etc", "hostId": ""}, "expectedResourceVersion": state.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, invalid, true); r.Code != 422 {
		t.Fatalf("invalid path: %d", r.Code)
	}
	read = authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var reread preferencesResponse
	if err := json.Unmarshal(read.Body.Bytes(), &reread); err != nil || reread.MusicLocation == nil || reread.MusicLocation.HostID != "node-1" || reread.ResourceVersion != state.ResourceVersion {
		t.Fatalf("reread: %v %+v", err, reread)
	}
}

func TestMusicPlaybackPreferencesAPI(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, tokenPath)
	read := authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var initial preferencesResponse
	if err := json.Unmarshal(read.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	checkpoint := map[string]any{"path": "/music/song.mp3", "hostId": "edge-1", "position": 42.5, "playing": true, "repeat": "all", "shuffle": true}
	body, _ := json.Marshal(map[string]any{"musicPlayback": checkpoint, "expectedResourceVersion": initial.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, false); r.Code != 403 {
		t.Fatalf("CSRF: %d", r.Code)
	}
	saved := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, true)
	if saved.Code != 200 {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, body, true); r.Code != 409 {
		t.Fatalf("conflict: %d", r.Code)
	}
	var state preferencesResponse
	if err := json.Unmarshal(saved.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.MusicPlayback == nil || state.MusicPlayback.Path != "/music/song.mp3" || state.MusicPlayback.Position != 42.5 {
		t.Fatalf("checkpoint not saved: %+v", state.MusicPlayback)
	}
	checkpoint["path"] = "/music/../secret.mp3"
	invalid, _ := json.Marshal(map[string]any{"musicPlayback": checkpoint, "expectedResourceVersion": state.ResourceVersion})
	if r := authenticatedSiteRequest(s, session, csrf, http.MethodPut, preferencesPath, invalid, true); r.Code != 422 {
		t.Fatalf("invalid path: %d", r.Code)
	}
	read = authenticatedSiteRequest(s, session, csrf, http.MethodGet, preferencesPath, nil, false)
	var reread preferencesResponse
	if err := json.Unmarshal(read.Body.Bytes(), &reread); err != nil {
		t.Fatal(err)
	}
	if reread.MusicPlayback == nil || reread.MusicPlayback.Path != "/music/song.mp3" || reread.ResourceVersion != state.ResourceVersion {
		t.Fatal("invalid write changed checkpoint")
	}
}
