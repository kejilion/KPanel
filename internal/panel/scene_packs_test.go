package panel

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/scenepacks"
	"github.com/kejilion/kejilion-panel/internal/store"
)

// The source is the tracked repository catalog; only network transport is replaced.
func repositorySceneFetch(t *testing.T) scenepacks.Fetch {
	t.Helper()
	root, err := filepath.Abs("../../scene-packs")
	if err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context, address string, limit int64) ([]byte, error) {
		const prefix = "https://raw.githubusercontent.com/kejilion/KPanel/main/scene-packs/"
		if !strings.HasPrefix(address, prefix) {
			return nil, scenepacks.ErrSource
		}
		name := strings.TrimPrefix(address, prefix)
		if !filepath.IsLocal(filepath.FromSlash(name)) || strings.Contains(name, "..") {
			return nil, scenepacks.ErrSource
		}
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	}
}

func TestScenePackHTTPAuthenticationIsolationAndLifecycle(t *testing.T) {
	s, tokenPath := newTestServer(t)
	s.scenePacks.Close()
	s.scenePacks = scenepacks.Open(filepath.Join(t.TempDir(), "scene-packs"), repositorySceneFetch(t))
	session, csrf := bootstrapCookies(t, s, tokenPath)
	headers := map[string]string{"Origin": "http://panel.test", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		return authenticatedRequest(s, method, path, data, session, csrf, headers)
	}
	for _, path := range []string{scenePacksPath, scenePacksPath + "/orbital-station/thumb"} {
		if r := performRequest(s, "GET", path, nil, nil); r.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", path, r.Code)
		}
	}
	list := call("GET", scenePacksPath, nil)
	if list.Code != 200 {
		t.Fatalf("catalog: %d %s", list.Code, list.Body.String())
	}
	var catalog scenepacks.List
	if err := json.Unmarshal(list.Body.Bytes(), &catalog); err != nil || len(catalog.Packs) != 3 {
		t.Fatalf("catalog %s %v", list.Body.String(), err)
	}
	p := catalog.Packs[0]
	input := map[string]string{"expectedResourceVersion": p.ResourceVersion}
	data, _ := json.Marshal(input)
	for _, denied := range []map[string]string{
		{"Origin": "https://evil.example", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value},
		{"Origin": "http://panel.test", "Content-Type": "application/json"},
	} {
		r := authenticatedRequest(s, "POST", scenePacksPath+"/"+p.ID+"/install", data, session, csrf, denied)
		if r.Code != 403 {
			t.Fatalf("origin/CSRF bypass %d %s", r.Code, r.Body.String())
		}
	}
	if r := call("PUT", scenePacksPath+"/source", map[string]string{"source": "http://127.0.0.1"}); r.Code != 400 {
		t.Fatal("accepted arbitrary source", r.Code)
	}
	if r := call("POST", scenePacksPath+"/"+p.ID+"/install", map[string]string{"expectedResourceVersion": "sha256:" + strings.Repeat("0", 64)}); r.Code != 409 {
		t.Fatal("accepted stale install", r.Code)
	}
	r := call("POST", scenePacksPath+"/"+p.ID+"/install", input)
	if r.Code != 200 {
		t.Fatalf("install: %d %s", r.Code, r.Body.String())
	}
	var installed scenepacks.View
	if err := json.Unmarshal(r.Body.Bytes(), &installed); err != nil || installed.FileBase == nil {
		t.Fatalf("installed: %s %v", r.Body.String(), err)
	}
	base := *installed.FileBase
	for i := 0; i < cap(s.scenePackStreams); i++ {
		s.scenePackStreams <- struct{}{}
	}
	const queuedRequests = 8
	queuedResponses := make(chan *httptest.ResponseRecorder, queuedRequests)
	for i := 0; i < queuedRequests; i++ {
		go func() {
			queuedResponses <- performRequest(s, "GET", base+"scene.js", nil, map[string]string{"Origin": "null"})
		}()
	}
	deadline := time.Now().Add(time.Second)
	for len(s.scenePackStreamQueue) < queuedRequests && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(s.scenePackStreamQueue) != queuedRequests {
		t.Fatalf("only %d scene file requests were queued", len(s.scenePackStreamQueue))
	}
	select {
	case response := <-queuedResponses:
		t.Fatalf("scene file request failed instead of waiting: %d %s", response.Code, response.Body.String())
	case <-time.After(20 * time.Millisecond):
	}
	for i := 0; i < cap(s.scenePackStreams); i++ {
		<-s.scenePackStreams
	}
	for i := 0; i < queuedRequests; i++ {
		select {
		case response := <-queuedResponses:
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || response.Body.Len() == 0 {
				t.Fatalf("queued scene file response: %d %s", response.Code, response.Body.String())
			}
		case <-time.After(time.Second):
			t.Fatal("queued scene file request did not resume")
		}
	}
	for len(s.scenePackStreams) > 0 {
		<-s.scenePackStreams
	}
	_, rv := s.store.SecurityEntrance()
	if err := s.store.ReplaceSecurityEntrance(rv, store.SecurityEntrance{Enabled: true, Path: "private-entry", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// An opaque frame cannot send a session or entrance cookie. The unguessable
	// file capability still loads only artwork, including with security entry enabled.
	for _, name := range []string{"index.html", "scene.js"} {
		r := performRequest(s, "GET", base+name, nil, map[string]string{"Origin": "null"})
		if r.Code != 200 {
			t.Fatalf("sandbox file %s: %d %s", name, r.Code, r.Body.String())
		}
		csp := r.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "sandbox allow-scripts") || !strings.Contains(csp, "connect-src http://panel.test"+base+";") || strings.Contains(csp, "connect-src 'self'") || strings.Contains(csp, "allow-same-origin") {
			t.Fatalf("unsafe CSP %q", csp)
		}
		if r.Header().Get("Access-Control-Allow-Origin") != "*" || r.Header().Get("Access-Control-Allow-Credentials") != "" || r.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("capability CORS/referrer policy", r.Header())
		}
	}
	for _, path := range []string{base + "state.json", base + "../state.json", base + "%2e%2e/state.json", strings.Replace(base, "/files/", "/files/0", 1) + "index.html"} {
		if r := performRequest(s, "GET", path, nil, nil); r.Code == 200 {
			t.Fatalf("accepted unlisted or invalid path %s", path)
		}
	}
	if r := performRequest(s, "GET", "/api/v1/auth/session", nil, nil); r.Code == 200 || r.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("capability weakened session boundary")
	}
	if r := call("GET", scenePacksPath, nil); r.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("management CORS enabled")
	}
	if r := call("DELETE", scenePacksPath+"/"+p.ID, input); r.Code != 409 {
		t.Fatal("accepted stale delete", r.Code)
	}
	if r := call("DELETE", scenePacksPath+"/"+p.ID, map[string]string{"expectedResourceVersion": installed.ResourceVersion}); r.Code != 204 {
		t.Fatalf("delete %d %s", r.Code, r.Body.String())
	}
	if r := performRequest(s, "GET", base+"index.html", nil, nil); r.Code != 404 {
		t.Fatal("deleted capability remained usable", r.Code)
	}
}

func TestScenePackWritesFailClosedWhenAuditUnavailable(t *testing.T) {
	s, tokenPath := newTestServer(t)
	s.scenePacks.Close()
	s.scenePacks = scenepacks.Open(filepath.Join(t.TempDir(), "scene-packs"), repositorySceneFetch(t))
	session, csrf := bootstrapCookies(t, s, tokenPath)
	// Preserve the fixture, then block the audit directory after authentication.
	if err := os.Rename(s.config.DataDir, s.config.DataDir+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.config.DataDir, []byte("blocked audit directory"), 0600); err != nil {
		t.Fatal(err)
	}
	r := authenticatedRequest(s, "PUT", scenePacksPath+"/source", []byte(`{"source":"mirror"}`), session, csrf, map[string]string{"Origin": "http://panel.test", "Content-Type": "application/json", "X-CSRF-Token": csrf.Value})
	if r.Code != 503 || !strings.Contains(r.Body.String(), "audit_unavailable") {
		t.Fatalf("audit failure %d %s", r.Code, r.Body.String())
	}
	list, err := s.scenePacks.List(context.Background())
	if err != nil || list.Source != "auto" {
		t.Fatalf("write escaped audit gate %+v %v", list, err)
	}
}

func TestScenePackStreamQueueIsBoundedAndCancelable(t *testing.T) {
	s, _ := newTestServer(t)
	for i := 0; i < cap(s.scenePackStreams); i++ {
		s.scenePackStreams <- struct{}{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/scene-file", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	queued := make(chan bool, 1)
	go func() { queued <- s.scenePackStream(response, request) }()
	deadline := time.Now().Add(time.Second)
	for len(s.scenePackStreamQueue) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(s.scenePackStreamQueue) != 1 {
		cancel()
		t.Fatal("request did not enter the bounded queue")
	}
	cancel()
	select {
	case acquired := <-queued:
		if acquired {
			t.Fatal("canceled request acquired a stream")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request remained blocked")
	}
	if len(s.scenePackStreamQueue) != 0 {
		t.Fatal("canceled request kept its queue slot")
	}

	for i := 0; i < cap(s.scenePackStreamQueue); i++ {
		s.scenePackStreamQueue <- struct{}{}
	}
	response = httptest.NewRecorder()
	if s.scenePackStream(response, httptest.NewRequest(http.MethodGet, "/scene-file", nil)) {
		t.Fatal("request acquired a stream after the queue was full")
	}
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "scene_pack_busy") {
		t.Fatalf("full queue response: %d %s", response.Code, response.Body.String())
	}
	for len(s.scenePackStreamQueue) > 0 {
		<-s.scenePackStreamQueue
	}
	for len(s.scenePackStreams) > 0 {
		<-s.scenePackStreams
	}
}

// Opt-in local integration fixture: real Panel auth, scene store and HTTP/CSP;
// the upstream repository transport is replaced by this exact checkout. No
// Agent or host actions are available. The listener expires after 20 minutes.
func TestScenePackLocalPreview(t *testing.T) {
	if os.Getenv("KPANEL_SCENE_PREVIEW") != "1" {
		t.Skip("opt-in loopback integration preview")
	}
	origin := os.Getenv("KPANEL_SCENE_PREVIEW_ORIGIN")
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("preview origin must be a loopback HTTP origin with a port")
	}
	port, err := strconv.Atoi(os.Getenv("KPANEL_SCENE_PREVIEW_PORT"))
	if err != nil || port < 1024 || port > 65535 {
		t.Fatal("invalid preview port")
	}
	s, tokenPath := newTestServerWithPublicURL(t, origin)
	s.scenePacks.Close()
	s.scenePacks = scenepacks.Open(filepath.Join(t.TempDir(), "scene-packs"), repositorySceneFetch(t))
	bootstrapCookiesForOrigin(t, s, tokenPath, origin)
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second}
	defer server.Close()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	t.Logf("scene integration fixture ready at http://%s (repository transport fixture; no Agent)", listener.Addr())
	select {
	case err := <-finished:
		if err != http.ErrServerClosed {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Minute):
	}
}
