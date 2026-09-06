package panel

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

func TestNotificationResourcesPanelAgentDockerChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket integration runs on Linux")
	}
	root, err := os.MkdirTemp("", "kpanel-alerts-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	for _, name := range []string{"conf.d", "certs", "html"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	serve := func(socket string, handler http.Handler) *http.Server {
		t.Helper()
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })
		return server
	}
	id := strings.Repeat("a", 64)
	var calls atomic.Int32
	var unavailable atomic.Bool
	dockerSocket := filepath.Join(root, "docker.sock")
	serve(dockerSocket, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" {
			t.Errorf("unexpected write %s", r.Method)
		}
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/containers/json":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": id, "Names": []string{"/important"}}})
		case "/containers/" + id + "/json":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/important", "RestartCount": 7, "State": map[string]string{"Status": "exited"}, "Config": map[string]any{"Env": []string{"TOKEN=secret-fixture"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	docker := dockerx.New(dockerSocket, root, filepath.Join(root, "state"))
	docker.ConfigureDaemonAccess("", true)
	const token = "notification-test-token-000000000000000"
	backend, err := agent.NewServer(agent.Config{Token: []byte(token), Version: "test", ProtocolVersion: "v1", WebRoot: root, StateDir: filepath.Join(root, "state"), Docker: docker})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	agentSocket := filepath.Join(root, "agent.sock")
	serve(agentSocket, backend)
	tokenPath := filepath.Join(root, "token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	source := notificationResourceSource{agent: NewAgentClient(agentSocket, tokenPath, 256<<10)}
	start := time.Now()
	snapshot := source.Resources(context.Background())
	elapsed := time.Since(start)
	if snapshot.ContainerStatus != "ready" || snapshot.CertificateStatus != "ready" || len(snapshot.Containers) != 1 || !snapshot.Containers[0].Known || *snapshot.Containers[0].RestartCount != 7 || snapshot.Containers[0].State != "exited" {
		t.Fatalf("chain projection=%+v", snapshot)
	}
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), "secret-fixture") || strings.Contains(string(data), "TOKEN") || calls.Load() != 2 {
		t.Fatalf("leak/budget: %s calls=%d", data, calls.Load())
	}
	t.Logf("resource_read_elapsed=%v response_bytes=%d docker_requests=%d", elapsed, len(data), calls.Load())
	unavailable.Store(true)
	snapshot = source.Resources(context.Background())
	if snapshot.ContainerStatus != "unknown" || len(snapshot.Containers) != 0 {
		t.Fatal("Engine failure fabricated healthy/empty success")
	}
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	snapshot = source.Resources(context.Background())
	if snapshot.ContainerStatus != "unknown" {
		t.Fatal("Agent credential loss fabricated success")
	}
}
