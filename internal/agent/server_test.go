package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

func TestBearerRequired(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("without token status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("with token status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestAppIconRouteRequiresKnownDynamicSlug(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/apps/icons/unknown-app.webp", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "app_icon_not_found") {
		t.Fatalf("unknown app icon response = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/apps/icons/unknown-app.webp?refresh=true", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("queried app icon response = %d %s", response.Code, response.Body.String())
	}
}

func TestDockerContainerStatsRouteReturnsBoundedBatch(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix Socket integration test")
	}
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/docker/container-stats", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("container stats status=%d body=%s", response.Code, response.Body.String())
	}
	var batch dockerx.ContainerMetricBatch
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil || batch.Items == nil || batch.Total != 0 {
		t.Fatalf("container stats batch=%#v err=%v", batch, err)
	}
}

func TestTerminalRouteRequiresAuthenticationAndExplicitManager(t *testing.T) {
	server := testServer(t)
	body := `{"owner":"panel:test","rows":24,"columns":80}`

	request := httptest.NewRequest(http.MethodPost, "/v1/terminals", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("terminal without token status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/terminals", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "terminal_unavailable") {
		t.Fatalf("terminal without manager status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestAvailableWebRootRequiresDirectory(t *testing.T) {
	root := t.TempDir()
	if err := availableWebRoot(root); err != nil {
		t.Fatalf("availableWebRoot(directory) error = %v", err)
	}
	if err := availableWebRoot(filepath.Join(root, "missing")); err == nil {
		t.Fatal("availableWebRoot() accepted a missing path")
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := availableWebRoot(file); err == nil {
		t.Fatal("availableWebRoot() accepted a regular file")
	}
}

func TestHealthKeepsCoreReadyWithoutWebRoot(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix Socket integration test")
	}
	root := filepath.Join(t.TempDir(), "not-created")
	docker := dockerx.New(fakeDockerSocket(t), root, t.TempDir())
	docker.ConfigureDaemonAccess("", true)
	server, err := NewServer(Config{
		Token: []byte(strings.Repeat("x", 32)), Version: "test", ProtocolVersion: "test",
		WebRoot: root, System: systeminfo.NewCollector(),
		Sites: sites.NewDiscoverer(root), Docker: docker, Files: testFileManager(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var health contract.AgentHealth
	if err := json.Unmarshal(response.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK ||
		health.Status != "degraded" ||
		!health.CoreReady() ||
		len(health.Reasons) != 1 ||
		health.Reasons[0] != "web_root_unavailable" {
		t.Fatalf("unexpected standalone health: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestApplicationJobEndpointsRequireAuthenticationAndStrictIDs(t *testing.T) {
	server := testServer(t)
	if err := server.appMarket.ConfigureJobs(
		filepath.Join(t.TempDir(), "app-jobs"),
		filepath.Join(t.TempDir(), "kejilion-agent"),
	); err != nil {
		t.Fatal(err)
	}
	unauthenticated := httptest.NewRequest(http.MethodGet, "/v1/app-jobs", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, unauthenticated)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/app-jobs", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("job list status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/app-jobs?cursor=unsafe", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("job list query status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/app-jobs/not-an-id", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("invalid job id status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestApplicationTerminalRoutesValidateMethodAndOffset(t *testing.T) {
	server := testServer(t)
	id := strings.Repeat("a", 32)
	token := "Bearer " + strings.Repeat("x", 32)
	for target, expected := range map[string]int{
		"/v1/app-jobs/" + id + "/terminal":                                     http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/terminal?offset=-1":                           http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/terminal?offset=0&extra=1":                    http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/terminal?offset=0&wait=1501":                  http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/terminal?offset=0&inputOpen=invalid":          http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/terminal?offset=0":                            http.StatusNotFound,
		"/v1/app-jobs/" + id + "/terminal?offset=0&wait=1&inputOpen=false":     http.StatusNotFound,
		"/v1/app-jobs/" + id + "/terminal?offset=0&wait=1&inputOpen=false&x=1": http.StatusBadRequest,
		"/v1/app-jobs/" + id + "/cancel":                                       http.StatusMethodNotAllowed,
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("%s status = %d body=%s", target, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/app-jobs/"+id+"/input", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("terminal input GET status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/app-jobs/"+id+"/cancel", nil)
	request.Header.Set("Authorization", token)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing job cancellation status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestSitesPageEndpoint(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/sites", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestSitesPageReturnsEmptyWithoutWebRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	docker := dockerx.New(fakeDockerSocket(t), root, t.TempDir())
	docker.ConfigureDaemonAccess("", true)
	server, err := NewServer(Config{
		Token: []byte(strings.Repeat("x", 32)), Version: "test", ProtocolVersion: "test",
		WebRoot: root, System: systeminfo.NewCollector(),
		Sites: sites.NewDiscoverer(root), Docker: docker, Files: testFileManager(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/sites", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestSystemWriteCapabilitiesRemainExplicitlyDisabled(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var result contract.PageResult[contract.Capability]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"system.hostname.write",
		"system.ssh-port.write",
		"system.ssh-defense.write",
		"system.dns.write",
		"system.timezone.write",
		"system.processes.signal",
		"system.swap.write",
		"system.mirror.write",
		"system.ip-preference.write",
		"system.kernel-tuning.write",
		"system.bbr.write",
		"system.update.write",
		"system.cleanup.write",
		"system.reboot.write",
		"system.hosts.write",
		"system.cron.write",
		"system.network-interfaces.write",
		"system.firewall.write",
		"system.traffic-shutdown.write",
		"system.reinstall",
	} {
		found := false
		for _, capability := range result.Items {
			if capability.ID != required {
				continue
			}
			found = true
			if capability.Enabled || capability.Reason == "" {
				t.Fatalf("capability %q must be disabled with a reason: %#v", required, capability)
			}
		}
		if !found {
			t.Fatalf("capability %q not reported", required)
		}
	}
}

func TestSystemProcessesRejectsUnboundedQueriesBeforeCollection(t *testing.T) {
	server := testServer(t)
	for _, target := range []string{
		"/v1/system/processes?unknown=value",
		"/v1/system/processes?sort=command",
		"/v1/system/processes?limit=257",
		"/v1/system/processes?q=" + strings.Repeat("x", systeminfo.MaxProcessSearchBytes+1),
		"/v1/system/processes?sort=cpu&sort=memory",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("query %q returned %d: %s", target, response.Code, response.Body.String())
		}
	}
}

func TestSystemProcessesBoundsConcurrentSampling(t *testing.T) {
	server := testServer(t)
	server.processesGate <- struct{}{}
	request := httptest.NewRequest(http.MethodGet, "/v1/system/processes?sort=cpu&limit=200", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "process_metrics_busy") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSiteWriteErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid request", sites.ErrInvalidInput, http.StatusBadRequest, "invalid_site_request"},
		{"unsupported artifact", sites.ErrForbidden, http.StatusUnprocessableEntity, "site_action_unsupported"},
		{"conflict", sites.ErrConflict, http.StatusConflict, "resource_conflict"},
		{"validation", sites.ErrUnprocessable, http.StatusUnprocessableEntity, "site_validation_failed"},
		{"needs attention", sites.ErrNeedsAttention, http.StatusServiceUnavailable, "site_needs_attention"},
		{"unavailable", sites.ErrUnavailable, http.StatusServiceUnavailable, "sites_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			(&Server{}).writeSiteError(
				response,
				"request-id",
				fmt.Errorf("wrapped: %w", test.err),
			)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			var problem contract.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != test.wantCode || problem.Status != test.wantStatus {
				t.Fatalf("problem = %#v, want code=%q status=%d", problem, test.wantCode, test.wantStatus)
			}
			if problem.Retryable != (test.wantStatus >= 500) {
				t.Fatalf("retryable = %v for status %d", problem.Retryable, test.wantStatus)
			}
		})
	}
}

func TestSiteWriteRoutesRejectMalformedRequests(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name: "unknown create field", method: http.MethodPost, path: "/v1/sites",
			body:       `{"primaryDomain":"example.com","type":"static","unknown":true}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "create query", method: http.MethodPost, path: "/v1/sites?mode=unsafe",
			body:       `{"primaryDomain":"example.com","type":"static"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "encoded update path", method: http.MethodPatch,
			path: "/v1/sites/" + strings.Repeat("%61", 32), body: `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "update query", method: http.MethodPatch,
			path: "/v1/sites/" + strings.Repeat("a", 32) + "?force=true", body: `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid update id", method: http.MethodPatch,
			path: "/v1/sites/" + strings.Repeat("A", 32), body: `{}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unsupported collection method", method: http.MethodDelete,
			path: "/v1/sites", wantStatus: http.StatusMethodNotAllowed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestExternalContainerLogsReachDockerWithoutOwnershipGate(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix Socket integration test")
	}
	server := testServer(t)
	id := strings.Repeat("a", 64)
	request := httptest.NewRequest(http.MethodGet, "/v1/docker/containers/"+id+"/logs", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("external logs status = %d body=%s", response.Code, response.Body.String())
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"conf.d", "html", "certs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	socket := fakeDockerSocket(t)
	docker := dockerx.New(socket, root, t.TempDir())
	docker.ConfigureDaemonAccess("", true)
	server, err := NewServer(Config{
		Token: []byte(strings.Repeat("x", 32)), Version: "test", ProtocolVersion: "test",
		WebRoot: root, System: systeminfo.NewCollector(),
		Sites: sites.NewDiscoverer(root), Docker: docker, Files: testFileManager(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestDefaultFileManagerUsesWritableAgentStateDirectoryForTrash(t *testing.T) {
	config := defaultFileManagerConfig("/home/docker/kpanel/data/agent")
	if config.TrashVirtual != "/home/docker/kpanel/data/agent/file-trash" {
		t.Fatalf("trash path = %q", config.TrashVirtual)
	}
	found := false
	for _, protected := range config.ProtectedVirtual {
		if protected == "/home/docker/kpanel/data/agent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Agent state directory is not protected: %#v", config.ProtectedVirtual)
	}
}

func testFileManager(t *testing.T) *filemanager.Manager {
	t.Helper()
	manager, err := filemanager.New(filemanager.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close file manager: %v", err)
		}
	})
	return manager
}

func fakeDockerSocket(t *testing.T) string {
	t.Helper()
	if os.PathSeparator == '\\' {
		// Windows' standard library may not expose Unix sockets. Health is allowed
		// to report degraded, which is sufficient for HTTP middleware tests.
		return filepath.Join(t.TempDir(), "missing.sock")
	}
	path := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/_ping":
				_, _ = w.Write([]byte("OK"))
				return
			case r.URL.Path == "/containers/json":
				_, _ = w.Write([]byte(`[]`))
				return
			case strings.HasSuffix(r.URL.Path, "/json"):
				_, _ = w.Write([]byte(`{"Id":"` + strings.Repeat("a", 64) + `","Config":{"Labels":{}},"State":{"Status":"running"}}`))
				return
			}
			http.NotFound(w, r)
		})}
		_ = server.Serve(listener)
	}()
	return path
}

var _ = context.Background
