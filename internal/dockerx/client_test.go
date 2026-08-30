package dockerx

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestContainersBoundsParallelInspectWork(t *testing.T) {
	const count = 12
	items := make([]containerListItem, 0, count)
	for index := range count {
		id := fmt.Sprintf("%064x", index+1)
		items = append(items, containerListItem{
			ID: id, Names: []string{"/container-" + strconv.Itoa(index)},
			Image: "example:latest", State: "running",
		})
	}
	var active atomic.Int32
	var maximum atomic.Int32
	var inspections atomic.Int32
	var includedStopped atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			includedStopped.Store(r.URL.Query().Get("all") == "1")
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				observed := maximum.Load()
				if current <= observed || maximum.CompareAndSwap(observed, current) {
					break
				}
			}
			inspections.Add(1)
			time.Sleep(20 * time.Millisecond)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json")
			_ = json.NewEncoder(w).Encode(managedInspect(id, "2026-07-28T00:00:00Z", 0))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	got, err := testHTTPClient(server).Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != count || inspections.Load() != count {
		t.Fatalf("containers=%d inspections=%d; want %d", len(got), inspections.Load(), count)
	}
	if maximum.Load() <= 1 || maximum.Load() > 4 {
		t.Fatalf("maximum parallel inspections = %d; want 2..4", maximum.Load())
	}
	if !includedStopped.Load() {
		t.Fatal("container inventory did not request stopped containers")
	}
}

func TestAllContainerOriginsExposeStateValidActions(t *testing.T) {
	web := t.TempDir()
	apps := t.TempDir()
	state := t.TempDir()
	c := &Client{
		webRoot:   filepath.ToSlash(web),
		appRoot:   filepath.ToSlash(apps),
		stateRoot: filepath.ToSlash(state),
	}
	var raw containerInspect
	raw.ID = strings.Repeat("a", 64)
	raw.Name = "/nginx"
	raw.Config.Image = "nginx:stable"
	raw.Config.Labels = map[string]string{
		"com.docker.compose.project.working_dir": filepath.ToSlash(web),
	}
	raw.State.Status = "running"
	raw.Mounts = []dockerMount{{
		Type: "bind", Source: filepath.ToSlash(filepath.Join(web, "conf.d")),
		Destination: "/etc/nginx/conf.d", RW: true,
	}}
	if err := ensureDir(filepath.Join(web, "conf.d")); err != nil {
		t.Fatal(err)
	}
	got := c.summaryFromInspect(raw)
	if got.Ownership != "kejilion" || !contains(got.AllowedActions, "restart") {
		t.Fatalf("expected safely managed container, got %#v", got)
	}

	raw.HostConfig.Privileged = true
	raw.Config.Labels = nil
	raw.Name = "/external"
	got = c.summaryFromInspect(raw)
	if got.Ownership != "external" || !contains(got.AllowedActions, "restart") ||
		!contains(got.AllowedActions, "exec") || !contains(got.AllowedActions, "remove") {
		t.Fatalf("external privileged container did not remain manageable: %#v", got)
	}
}

func TestDockerSocketContainerRemainsManageable(t *testing.T) {
	root := t.TempDir()
	c := &Client{webRoot: filepath.ToSlash(root), appRoot: "/home/docker", stateRoot: "/var/lib/kejilion-panel"}
	var raw containerInspect
	raw.Config.Labels = map[string]string{"io.kejilion.panel.managed": "true"}
	raw.State.Status = "running"
	raw.Mounts = []dockerMount{{Type: "bind", Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"}}
	if got := c.summaryFromInspect(raw); !contains(got.AllowedActions, "restart") ||
		!contains(got.AllowedActions, "exec") {
		t.Fatalf("Docker Socket mount incorrectly disabled container management: %#v", got)
	}
}

func TestContainerSummaryExposesCreatedAt(t *testing.T) {
	client := &Client{}
	raw := managedInspect(strings.Repeat("9", 64), "2026-01-01T00:00:00Z", 0)
	raw.Created = "2026-08-18T10:00:00.123456789Z"

	summary := client.summaryFromInspect(raw)
	if summary.CreatedAt == nil || !summary.CreatedAt.Equal(time.Date(2026, 8, 18, 10, 0, 0, 123456789, time.UTC)) {
		t.Fatalf("createdAt = %v, want Docker inspect creation time", summary.CreatedAt)
	}
}

func TestContainerListSummaryFallsBackToCreatedAt(t *testing.T) {
	client := &Client{}
	raw := containerListItem{ID: strings.Repeat("a", 64), Names: []string{"/fallback"}, Created: 1_755_520_800}

	summary := client.summaryFromList(raw)
	if summary.CreatedAt == nil || !summary.CreatedAt.Equal(time.Unix(raw.Created, 0).UTC()) {
		t.Fatalf("createdAt = %v, want list timestamp", summary.CreatedAt)
	}
}

func TestDemuxAndRedactLogs(t *testing.T) {
	payload := []byte("token=super-secret\nhttps://user:pass@example.test/path\n")
	var stream bytes.Buffer
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	stream.Write(header)
	stream.Write(payload)
	lines := redactLines(demuxDockerStream(stream.Bytes()), 20)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "super-secret") || strings.Contains(joined, "user:pass") {
		t.Fatalf("secret was not redacted: %s", joined)
	}
}

func TestRedactJSONAndPrefixedSecretKeys(t *testing.T) {
	input := []byte(`{"password":"json-secret","OPENAI_API_KEY":"sk-secret","safe":"visible"}`)
	joined := strings.Join(redactLines(input, 20), "\n")
	if strings.Contains(joined, "json-secret") || strings.Contains(joined, "sk-secret") {
		t.Fatalf("JSON secret was not redacted: %s", joined)
	}
	if !strings.Contains(joined, `"safe":"visible"`) {
		t.Fatalf("non-secret field was unexpectedly changed: %s", joined)
	}
}

func TestDockerLogsUseSharedCredentialRedactionPolicy(t *testing.T) {
	input := []byte(strings.Join([]string{
		`window-private-body`,
		`-----END RSA PRIVATE KEY-----`,
		`safe-after-orphan`,
		`refresh_token=refresh-secret`,
		`AWS_SECRET_ACCESS_KEY=aws-secret`,
		`pwd=pwd-secret`,
		`password=p@ss#2026`,
		`token=abc&def`,
		`Authorization: Basic YmFzaWMtc2VjcmV0`,
		`Authorization: ApiKey arbitrary-auth-secret`,
		`Cookie: session=cookie-secret; csrf=csrf-secret`,
		`https://url-user:url-pass@example.test/path?access_token=query-secret&safe=visible`,
		`https://token-only@example.test/path`,
		`-----BEGIN OPENSSH PRIVATE KEY-----`,
		`cHJpdmF0ZS1rZXktbWF0ZXJpYWw=`,
		`-----END OPENSSH PRIVATE KEY-----`,
	}, "\n"))
	joined := strings.Join(redactLines(input, 20), "\n")
	for _, secret := range []string{
		"refresh-secret", "aws-secret", "pwd-secret", "p@ss", "#2026", "abc", "&def",
		"YmFzaWMtc2VjcmV0", "ApiKey", "arbitrary-auth-secret",
		"cookie-secret", "csrf-secret", "url-user", "url-pass", "query-secret", "token-only",
		"window-private-body", "cHJpdmF0ZS1rZXktbWF0ZXJpYWw",
	} {
		if strings.Contains(joined, secret) {
			t.Fatalf("shared Docker redaction leaked %q: %s", secret, joined)
		}
	}
	if !strings.Contains(joined, "safe=visible") || !strings.Contains(joined, "safe-after-orphan") ||
		!strings.Contains(joined, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("shared Docker redaction removed safe data or marker: %s", joined)
	}
}

func TestContainerLogsSupportsExternalContainer(t *testing.T) {
	id := strings.Repeat("a", 64)
	logRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/" + id + "/json":
			_ = json.NewEncoder(w).Encode(containerInspect{
				ID: id,
			})
		case "/containers/" + id + "/logs":
			logRequests++
			_, _ = w.Write([]byte("external log line\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	logs, err := client.ContainerLogs(context.Background(), id, 20)
	if err != nil {
		t.Fatal(err)
	}
	if logRequests != 1 || strings.Join(logs.Lines, "\n") != "external log line" {
		t.Fatalf("external container logs = %#v, requests=%d", logs, logRequests)
	}
}

func TestClientDoesNotSocketActivateDockerByDefault(t *testing.T) {
	client := New(filepath.Join(t.TempDir(), "docker.sock"), t.TempDir(), t.TempDir())
	client.ConfigureDaemonAccess(filepath.Join(t.TempDir(), "missing.pid"), false)
	if err := client.Ping(context.Background()); !errors.Is(err, ErrDockerNotRunning) {
		t.Fatalf("Ping() error = %v, want ErrDockerNotRunning", err)
	}
}

func TestLifecycleSerializesAndRejectsStaleRestart(t *testing.T) {
	id := strings.Repeat("b", 64)
	var stateMu sync.Mutex
	restartCount := 0
	startedAt := "2026-07-25T00:00:00Z"
	postCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/" + id + "/json":
			stateMu.Lock()
			raw := managedInspect(id, startedAt, restartCount)
			stateMu.Unlock()
			_ = json.NewEncoder(w).Encode(raw)
		case "/containers/" + id + "/restart":
			time.Sleep(20 * time.Millisecond)
			stateMu.Lock()
			postCount++
			restartCount++
			startedAt = "2026-07-25T00:00:01Z"
			stateMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	initial := managedInspect(id, startedAt, restartCount)
	expected := client.summaryFromInspect(initial).ResourceVersion
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := client.Lifecycle(context.Background(), id, "restart", expected)
			errs <- err
		}()
	}
	first, second := <-errs, <-errs
	if !((first == nil && errors.Is(second, ErrResourceConflict)) ||
		(second == nil && errors.Is(first, ErrResourceConflict))) {
		t.Fatalf("concurrent restart errors = (%v, %v), want one success and one conflict", first, second)
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if postCount != 1 {
		t.Fatalf("restart endpoint called %d times, want 1", postCount)
	}
}

func TestLifecyclePauseAndUnpauseUseFixedDockerEndpoints(t *testing.T) {
	id := strings.Repeat("c", 64)
	state := "running"
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/" + id + "/json":
			raw := managedInspect(id, "2026-07-25T00:00:00Z", 0)
			raw.State.Status = state
			raw.State.Running = state == "running"
			_ = json.NewEncoder(w).Encode(raw)
		case "/containers/" + id + "/pause":
			requests = append(requests, r.Method+" "+r.URL.Path)
			state = "paused"
			w.WriteHeader(http.StatusNoContent)
		case "/containers/" + id + "/unpause":
			requests = append(requests, r.Method+" "+r.URL.Path)
			state = "running"
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testHTTPClient(server)
	running := managedInspect(id, "2026-07-25T00:00:00Z", 0)
	pauseResult, err := client.Lifecycle(
		context.Background(),
		id,
		"pause",
		client.summaryFromInspect(running).ResourceVersion,
	)
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	paused := running
	paused.State.Status = "paused"
	paused.State.Running = false
	if pauseResult.ResourceVersion != client.summaryFromInspect(paused).ResourceVersion {
		t.Fatalf("pause resource version was not refreshed")
	}
	if _, err := client.Lifecycle(
		context.Background(),
		id,
		"unpause",
		pauseResult.ResourceVersion,
	); err != nil {
		t.Fatalf("unpause failed: %v", err)
	}
	if got := strings.Join(requests, ","); got !=
		"POST /containers/"+id+"/pause,POST /containers/"+id+"/unpause" {
		t.Fatalf("lifecycle requests = %q", got)
	}
}

func testHTTPClient(server *httptest.Server) *Client {
	root := filepath.ToSlash(server.URL)
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		webRoot:    root,
		appRoot:    root + "/apps",
		stateRoot:  root + "/state",
		now:        time.Now,
	}
}

func managedInspect(id, startedAt string, restartCount int) containerInspect {
	var raw containerInspect
	raw.ID = id
	raw.Name = "/managed"
	raw.Config.Image = "example:latest"
	raw.Config.Labels = map[string]string{"io.kejilion.panel.managed": "true"}
	raw.State.Status = "running"
	raw.State.Running = true
	raw.State.StartedAt = startedAt
	raw.RestartCount = restartCount
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{}
	raw.NetworkSettings.Ports = map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{}
	return raw
}

func ensureDir(path string) error {
	return osMkdirAll(path)
}
