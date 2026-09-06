//go:build integration && linux

package dockerx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A disposable loopback registry supplies two zero-layer images. No public image,
// credentials, existing container, volume or tag is used by this integration test.
func TestImageUpdateAgainstEngineAfterLocalTagMoves(t *testing.T) {
	if os.Getenv("KPANEL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set KPANEL_DOCKER_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	name := fmt.Sprintf("kpanel-update-it-%d", time.Now().UnixNano())
	var current atomic.Int32
	configs := make([][]byte, 2)
	manifests := make([][]byte, 2)
	configDigests := make([]string, 2)
	digests := make([]string, 2)
	for i := range configs {
		configs[i], _ = json.Marshal(map[string]any{"architecture": runtime.GOARCH, "os": "linux", "config": map[string]any{"Cmd": []string{"/noop"}, "Labels": map[string]string{"test": fmt.Sprintf("%s-%d", name, i)}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{}}})
		configDigests[i] = fmt.Sprintf("sha256:%x", sha256.Sum256(configs[i]))
		manifests[i], _ = json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+json", "config": map[string]any{"mediaType": "application/vnd.docker.container.image.v1+json", "size": len(configs[i]), "digest": configDigests[i]}, "layers": []any{}})
		digests[i] = fmt.Sprintf("sha256:%x", sha256.Sum256(manifests[i]))
	}
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read only", 405)
			return
		}
		if r.URL.Path == "/v2/" {
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			_, _ = w.Write([]byte("{}"))
			return
		}
		for i := range configs {
			if strings.HasSuffix(r.URL.Path, "/blobs/"+configDigests[i]) {
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(configs[i])
				return
			}
			if strings.HasSuffix(r.URL.Path, "/manifests/"+digests[i]) || (strings.HasSuffix(r.URL.Path, "/manifests/latest") && int(current.Load()) == i) {
				w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
				w.Header().Set("Docker-Content-Digest", digests[i])
				w.Header().Set("Content-Length", fmt.Sprint(len(manifests[i])))
				_, _ = w.Write(manifests[i])
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer registry.Close()
	image := strings.TrimPrefix(registry.URL, "http://") + "/" + name + ":latest"
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
		for _, ref := range []string{image, configDigests[0], configDigests[1]} {
			_ = exec.CommandContext(cleanup, "docker", "image", "rm", ref).Run()
		}
		if output, _ := exec.CommandContext(cleanup, "docker", "ps", "-aq", "--filter", "name=^/"+name+"$").Output(); len(strings.TrimSpace(string(output))) != 0 {
			t.Error("test container remains")
		}
	})
	run("pull", image)
	id := run("create", "--name", name, image)
	client := New("/var/run/docker.sock", t.TempDir(), t.TempDir())
	client.ConfigureDaemonAccess("/run/docker.pid", true)
	raw, err := client.inspect(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	version := client.summaryFromInspect(raw).ResourceVersion
	initial, err := client.CheckContainerImageUpdate(ctx, id, version)
	if err != nil || initial.Status != "current" {
		t.Fatalf("initial=%+v %v", initial, err)
	}
	current.Store(1)
	run("pull", image)
	result, err := client.CheckContainerImageUpdate(ctx, id, version)
	if err != nil || result.Status != "available" || result.LocalDigest != digests[0] || result.RemoteDigest != digests[1] {
		t.Fatalf("after pull=%+v %v", result, err)
	}
	after, err := client.inspect(ctx, id)
	if err != nil || after.Image != raw.Image || after.ID != raw.ID {
		t.Fatalf("check mutated container: %+v %v", after, err)
	}
	t.Logf("real Engine current -> local tag moved -> available; container=%s unchanged; only disposable fixture resources", id)
}
