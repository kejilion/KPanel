package appmarket

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

func TestImageUpdateRequestCost(t *testing.T) {
	for _, sample := range []struct{ containers, checks int }{{10, 10}, {50, 20}, {200, 20}} {
		t.Run(fmt.Sprintf("containers_%d_checks_%d", sample.containers, sample.checks), func(t *testing.T) {
			var lists, inspections, images, distributions atomic.Int64
			digest := "sha256:" + strings.Repeat("b", 64)
			imageID := "sha256:" + strings.Repeat("d", 64)
			image := "ghcr.io/librespeed/speedtest:latest"
			root := t.TempDir()
			// A fake Engine Unix socket: no real Docker daemon or registry is contacted.
			socket := filepath.Join(root, "e.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write %s", r.Method)
				}
				var value any
				switch {
				case r.URL.Path == "/containers/json":
					lists.Add(1)
					rows := make([]map[string]any, sample.containers)
					for i := range rows {
						name := fmt.Sprintf("fixture-%d", i)
						if i == 0 {
							name = "speedtest"
						}
						rows[i] = map[string]any{"Id": fmt.Sprintf("%064x", i+1), "Names": []string{"/" + name}, "Image": image, "State": "running"}
					}
					value = rows
				case strings.HasPrefix(r.URL.Path, "/containers/"):
					inspections.Add(1)
					id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json")
					name := "/fixture-" + id
					if id == fmt.Sprintf("%064x", 1) {
						name = "/speedtest"
					}
					value = map[string]any{"Id": id, "Name": name, "Image": imageID, "Created": "2026-09-01T00:00:00Z", "Config": map[string]any{"Image": image, "Labels": map[string]string{}}, "State": map[string]any{"Status": "running"}}
				case strings.HasPrefix(r.URL.Path, "/images/"):
					images.Add(1)
					value = map[string]any{"Id": imageID, "RepoDigests": []string{"ghcr.io/librespeed/speedtest@" + digest}}
				case strings.HasPrefix(r.URL.Path, "/distribution/"):
					distributions.Add(1)
					value = map[string]any{"Descriptor": map[string]string{"digest": digest, "mediaType": "application/vnd.oci.image.index.v1+json"}}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(value)
			})}
			go server.Serve(listener)
			defer server.Close()
			client := dockerx.New(socket, root, root)
			client.ConfigureDaemonAccess("", true)
			service, err := New(client, root)
			if err != nil {
				t.Fatal(err)
			}
			service.scriptAppRoot = root
			for _, mode := range []string{"docker", "apps"} {
				lists.Store(0)
				inspections.Store(0)
				images.Store(0)
				distributions.Store(0)
				start := time.Now()
				for range sample.checks {
					var result dockerx.ImageUpdateResult
					if mode == "docker" {
						result, err = client.CheckContainerImageUpdate(context.Background(), fmt.Sprintf("%064x", 1), "v")
					} else {
						result, err = service.CheckUpdate(context.Background(), "builtin-28", "v")
					}
					if err != nil || result.Status != "current" {
						t.Fatalf("%s result=%+v err=%v", mode, result, err)
					}
				}
				expectedList, expectedInspect := int64(0), int64(2*sample.checks)
				if mode == "apps" {
					expectedList = int64(sample.checks)
				}
				if lists.Load() != expectedList || inspections.Load() != expectedInspect || images.Load() != int64(sample.checks) || distributions.Load() != int64(sample.checks) {
					t.Fatalf("counts list=%d inspect=%d", lists.Load(), inspections.Load())
				}
				t.Logf("SIMULATED_ENGINE mode=%s N=%d M=%d list=%d inspect=%d image=%d distribution=%d elapsed_ms=%.2f", mode, sample.containers, sample.checks, lists.Load(), inspections.Load(), images.Load(), distributions.Load(), float64(time.Since(start).Microseconds())/1000)
			}
			lists.Store(0)
			inspections.Store(0)
			inventory, err := service.Inventory(context.Background())
			if err != nil || len(inventory.Items) <= 1 {
				t.Fatalf("readonly lookup changed public catalog: items=%d err=%v", len(inventory.Items), err)
			}
			if lists.Load() != 1 || inspections.Load() != int64(sample.containers) {
				t.Fatalf("public inventory lost full inspect snapshots: list=%d inspect=%d", lists.Load(), inspections.Load())
			}
		})
	}
}
