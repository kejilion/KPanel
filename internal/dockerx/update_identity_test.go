package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (f updateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageUpdateBoundsParallelReadsAndCancels(t *testing.T) {
	started := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	client := testHTTPClient(server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := client.CheckContainerImageUpdate(ctx, strings.Repeat("a", 64), "v"); done <- err }()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("read never started")
		}
	}
	if _, err := client.CheckContainerImageUpdate(ctx, strings.Repeat("b", 64), "v"); !errors.Is(err, ErrImageUpdateBusy) {
		t.Fatalf("unbounded reads: %v", err)
	}
	cancel()
	for range 2 {
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel=%v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("canceled read remains active")
		}
	}
	if _, err := client.CheckContainerImageUpdate(ctx, strings.Repeat("b", 64), "v"); errors.Is(err, ErrImageUpdateBusy) {
		t.Fatal("cancellation leaked read slots")
	}
}

func TestImageUpdateIdentityAndFailureBoundaries(t *testing.T) {
	const index = "application/vnd.oci.image.index.v1+json"
	const manifest = "application/vnd.oci.image.manifest.v1+json"
	old := "sha256:" + strings.Repeat("b", 64)
	newDigest := "sha256:" + strings.Repeat("c", 64)
	for _, tc := range []struct {
		name, image, repo, local, remote, localKind, remoteKind, want string
		status                                                        int
		race, transientRace, managed, stale                           bool
	}{
		{name: "same normalized repository", image: "redis:alpine", repo: "index.docker.io/library/redis", local: old, remote: old, want: "current"},
		{name: "stale browser snapshot uses current identity", image: "redis:alpine", repo: "redis", local: old, remote: old, stale: true, want: "current"},
		{name: "transient snapshot race refreshes once", image: "redis:alpine", repo: "redis", local: old, remote: old, transientRace: true, want: "current"},
		{name: "wrong repository same digest", image: "redis:alpine", repo: "other/redis", local: old, remote: old},
		{name: "missing digest", image: "redis:alpine", repo: "redis", remote: old},
		{name: "index versus platform", image: "redis:alpine", repo: "redis", local: old, remote: newDigest, localKind: manifest, remoteKind: index},
		{name: "missing media type", image: "redis:alpine", repo: "redis", local: old, remote: newDigest},
		{name: "platform identity missing", image: "redis:alpine", repo: "redis", local: old, remote: newDigest, localKind: manifest, remoteKind: manifest},
		{name: "index changed", image: "redis:7", repo: "redis", local: old, remote: newDigest, localKind: index, remoteKind: index, want: "available"},
		{name: "private registry refused", image: "registry.example:5000/team/app:v1", repo: "registry.example:5000/team/app", local: old, status: 401},
		{name: "registry rate limited", image: "redis:alpine", repo: "redis", local: old, status: 429},
		{name: "registry timed out", image: "redis:alpine", repo: "redis", local: old, status: 504},
		{name: "fixed digest", image: "redis@" + old, want: "fixed"},
		{name: "fixed image ID", image: old, want: "fixed"},
		{name: "managed image ID retains tag", image: old, managed: true, repo: "redis", local: old, remote: old, want: "current"},
		{name: "container changes during check", image: "redis:alpine", repo: "redis", local: old, remote: old, race: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			raw := managedInspect(id, "2026-09-06T00:00:00Z", 0)
			raw.Image, raw.Config.Image = "sha256:"+strings.Repeat("d", 64), tc.image
			raw.Config.Labels = nil
			if tc.managed {
				raw.Config.Labels = map[string]string{"io.kejilion.panel.managed": "true", "io.kejilion.panel.image": "redis:alpine"}
			}
			inspections, registryCalls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", r.Method)
				}
				switch {
				case strings.HasPrefix(r.URL.Path, "/containers/"):
					inspections++
					response := raw
					if (tc.race && inspections%2 == 0) || (tc.transientRace && inspections > 1) {
						response.Image = newDigest
					}
					_ = json.NewEncoder(w).Encode(response)
				case strings.HasPrefix(r.URL.Path, "/images/"):
					_ = json.NewEncoder(w).Encode(map[string]any{"RepoDigests": []string{tc.repo + "@" + tc.local}})
				case strings.HasPrefix(r.URL.Path, "/distribution/"):
					registryCalls++
					if tc.status != 0 {
						http.Error(w, "registry unavailable", tc.status)
						return
					}
					digest, kind := tc.remote, tc.remoteKind
					if strings.Contains(r.URL.Path, "@") {
						digest, kind = tc.local, tc.localKind
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": map[string]string{"digest": digest, "mediaType": kind}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := testHTTPClient(server)
			var deadlines []time.Time
			transport := client.httpClient.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			client.httpClient.Timeout = 0
			client.httpClient.Transport = updateRoundTripper(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok {
					t.Error("image check has no total deadline")
				}
				deadlines = append(deadlines, deadline)
				return transport.RoundTrip(r)
			})
			version := client.summaryFromInspect(raw).ResourceVersion
			if tc.stale {
				version = "browser-stale-version"
			}
			result, err := client.CheckContainerImageUpdate(context.Background(), id, version)
			if tc.status != 0 && registryCalls != 1 {
				t.Fatalf("registry failure must not consume snapshot retry: calls=%d", registryCalls)
			}
			if tc.race || tc.transientRace {
				if inspections != 4 || registryCalls != 2 {
					t.Fatalf("retry budget exceeded: inspections=%d registryCalls=%d", inspections, registryCalls)
				}
				for _, deadline := range deadlines {
					if !deadline.Equal(deadlines[0]) {
						t.Fatal("retry extended the shared deadline")
					}
				}
			}
			if tc.stale && (result.ResourceVersion == version || inspections != 2 || registryCalls != 1) {
				t.Fatalf("stale snapshot was not refreshed cheaply: %+v inspect=%d registry=%d", result, inspections, registryCalls)
			}
			if tc.want == "fixed" {
				if !errors.Is(err, ErrImageUpdateFixed) || !errors.Is(err, ErrActionUnsupported) || registryCalls != 0 {
					t.Fatalf("fixed=%+v err=%v calls=%d", result, err, registryCalls)
				}
			} else if tc.want == "" {
				if err == nil {
					t.Fatalf("uncertain check falsely succeeded: %+v", result)
				}
				if tc.race && !errors.Is(err, ErrResourceConflict) {
					t.Fatalf("race=%v", err)
				}
				return
			} else if err != nil {
				t.Fatal(err)
			}
			if result.Status != tc.want {
				t.Fatalf("status=%s want=%s", result.Status, tc.want)
			}
		})
	}
}

func TestImageUpdateUsesRunningImageAfterTagMoves(t *testing.T) {
	id := strings.Repeat("a", 64)
	oldDigest := "sha256:" + strings.Repeat("b", 64)
	newDigest := "sha256:" + strings.Repeat("c", 64)
	raw := managedInspect(id, "2026-09-06T00:00:00Z", 0)
	raw.Config.Labels = nil // An ordinary container has exactly the same image identity contract.
	raw.Image = "sha256:" + strings.Repeat("d", 64)
	var inspectedImage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("update check wrote to Engine: %s", r.Method)
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/containers/"):
			_ = json.NewEncoder(w).Encode(raw)
		case strings.HasPrefix(r.URL.Path, "/images/"):
			inspectedImage = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/images/"), "/json")
			digest := newDigest
			if inspectedImage == raw.Image {
				digest = oldDigest
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"RepoDigests": []string{"docker.io/library/example@" + digest}})
		case strings.HasPrefix(r.URL.Path, "/distribution/"):
			digest := newDigest
			if strings.Contains(r.URL.Path, oldDigest) {
				digest = oldDigest
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": map[string]string{"digest": digest, "mediaType": "application/vnd.oci.image.index.v1+json"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testHTTPClient(server)
	result, err := client.CheckContainerImageUpdate(context.Background(), id, client.summaryFromInspect(raw).ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	if inspectedImage != raw.Image || result.Status != "available" || result.LocalDigest != oldDigest {
		t.Fatalf("inspected=%q running=%q result=%+v; tag now points to a different local image", inspectedImage, raw.Image, result)
	}
}
