package dockerx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestImagePullRunsAsPersistentBackgroundJob(t *testing.T) {
	requested := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/images/create" {
			http.NotFound(response, request)
			return
		}
		requested <- request.URL.RawQuery
		_, _ = response.Write([]byte("{\"status\":\"Pull complete\"}\n"))
	}))
	defer server.Close()

	client := testHTTPClient(server)
	if err := client.ConfigureJobs(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	job, err := client.StartMaintenance(context.Background(), MaintenanceInput{
		Action: "image_pull", Image: "nginx:alpine",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case query := <-requested:
		if query != "fromImage=nginx&tag=alpine" {
			t.Fatalf("pull query = %q", query)
		}
	case <-time.After(time.Second):
		t.Fatal("image pull did not start")
	}
	job = waitForDockerJob(t, client, job.ID)
	if job.Status != "succeeded" || job.Progress != 100 {
		t.Fatalf("unexpected completed job: %#v", job)
	}
	record, err := client.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Input.Action != "image_pull" || record.Input.Image != "" {
		t.Fatalf("completed job retained request details: %#v", record.Input)
	}
}

func TestMaintenancePersistenceFailureDoesNotLeaveBusyOrReplay(t *testing.T) {
	for _, phase := range []string{"queued", "running", "succeeded", "failed"} {
		t.Run(phase, func(t *testing.T) {
			var calls atomic.Int32
			var inject atomic.Bool
			inject.Store(true)
			parent := t.TempDir()
			root := filepath.Join(parent, "jobs")
			blockStorage := func() error {
				if err := os.Rename(root, root+".offline"); err != nil {
					return err
				}
				return os.WriteFile(root, []byte("storage unavailable"), 0600)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if inject.Load() && (phase == "succeeded" || phase == "failed") {
					if err := blockStorage(); err != nil {
						t.Error(err)
					}
				}
				if phase == "failed" {
					http.Error(w, "engine failure", 500)
					return
				}
				_, _ = w.Write([]byte("{}"))
			}))
			defer server.Close()
			client := testHTTPClient(server)
			if err := client.ConfigureJobs(root); err != nil {
				t.Fatal(err)
			}
			record := dockerJobRecord{MaintenanceJob: MaintenanceJob{ID: strings.Repeat("a", 32), Action: "container_prune", Status: "queued", CreatedAt: time.Now()}, Input: MaintenanceInput{Action: "container_prune"}}
			if phase == "queued" {
				if err := blockStorage(); err != nil {
					t.Fatal(err)
				}
				if _, err := client.StartMaintenance(context.Background(), record.Input); !errors.Is(err, ErrDockerJobStorage) {
					t.Fatalf("queued write error = %v", err)
				}
				if client.jobs.hasActive() || calls.Load() != 0 {
					t.Fatal("failed submission executed or stayed busy")
				}
			} else {
				if err := client.jobs.put(record); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(client.jobs.startError(), ErrDockerJobConflict) {
					t.Fatal("normal active job must retain conflict")
				}
				if phase == "running" {
					if err := blockStorage(); err != nil {
						t.Fatal(err)
					}
				}
				client.runMaintenance(record)
				visible, err := client.MaintenanceJob(record.ID)
				if err != nil || visible.Status != "failed" || visible.Stage != "persistence_pending" || visible.FinishedAt == nil {
					t.Fatalf("missing stopped/storage fault: %#v %v", visible, err)
				}
				if phase == "running" && (visible.StartedAt != nil || !strings.Contains(visible.Message, "尚未执行")) {
					t.Fatalf("unstarted action presented as executed: %#v", visible)
				}
				if _, err := client.StartMaintenance(context.Background(), record.Input); !errors.Is(err, ErrDockerJobStorage) {
					t.Fatalf("pending result must be a storage fault, got %v", err)
				}
			}
			if err := os.Remove(root); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(root+".offline", root); err != nil {
				t.Fatal(err)
			}
			inject.Store(false)
			time.Sleep(1100 * time.Millisecond)
			if client.jobs.hasActive() {
				t.Fatal("recovered storage left registry permanently busy")
			}
			if phase != "queued" {
				saved, err := client.jobs.read(record.ID)
				want := phase
				if phase == "running" {
					want = "failed"
				}
				if err != nil || saved.Status != want || saved.Input.Image != "" {
					t.Fatalf("lost terminal result: %#v %v", saved, err)
				}
			}
			wantCalls := int32(1)
			if phase == "queued" || phase == "running" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatalf("host actions=%d want=%d", calls.Load(), wantCalls)
			}
			next, err := client.StartMaintenance(context.Background(), record.Input)
			if err != nil {
				t.Fatalf("new task still blocked after storage recovered: %v", err)
			}
			waitForDockerJob(t, client, next.ID)
			if calls.Load() != wantCalls+1 {
				t.Fatal("new submission replayed old action")
			}
			if err := client.ConfigureJobs(root); err != nil {
				t.Fatal(err)
			}
			if client.jobs.hasActive() {
				t.Fatal("restart resurrected completed execution")
			}
		})
	}
}

func TestDockerRestartDoesNotReplayUnsavedExecution(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		t.Run(state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("restart replayed a host action") }))
			defer server.Close()
			client := testHTTPClient(server)
			root := t.TempDir()
			if err := client.ConfigureJobs(root); err != nil {
				t.Fatal(err)
			}
			record := dockerJobRecord{MaintenanceJob: MaintenanceJob{ID: strings.Repeat("b", 32), Action: "container_prune", Status: state, CreatedAt: time.Now()}, Input: MaintenanceInput{Action: "container_prune", Image: "must-be-cleared"}}
			if err := client.jobs.put(record); err != nil {
				t.Fatal(err)
			}
			if err := client.ConfigureJobs(root); err != nil {
				t.Fatal(err)
			}
			result, err := client.MaintenanceJob(record.ID)
			if err != nil || result.Status != "failed" || result.Stage != "interrupted" || client.jobs.hasActive() {
				t.Fatalf("restart made up terminal success or retained busy: %#v %v", result, err)
			}
			saved, err := client.jobs.read(record.ID)
			if err != nil || saved.Status != "failed" || saved.Input.Image != "" {
				t.Fatalf("interruption did not persist: %#v %v", saved, err)
			}
		})
	}
}

func TestPruneDoesNotUseConfirmationAsAuthorizationAndUsesFixedEndpoints(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.RequestURI())
		mu.Unlock()
		_ = json.NewEncoder(response).Encode(map[string]any{})
	}))
	defer server.Close()

	client := testHTTPClient(server)
	if err := client.ConfigureJobs(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	job, err := client.StartMaintenance(context.Background(), MaintenanceInput{
		Action: "prune",
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForDockerJob(t, client, job.ID)
	if job.Status != "succeeded" {
		t.Fatalf("prune failed: %#v", job)
	}
	want := []string{
		"/containers/prune",
		dockerPruneEndpoint("images"),
		"/networks/prune",
		"/volumes/prune",
		"/build/prune?all=1",
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != len(want) {
		t.Fatalf("prune paths = %#v", paths)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("prune path %d = %q, want %q", index, paths[index], want[index])
		}
	}
}

func TestScopedPruneUsesOnlySelectedDockerEndpoint(t *testing.T) {
	for action, wantPath := range map[string]string{
		"container_prune": "/containers/prune",
		"image_prune":     dockerPruneEndpoint("images"),
		"network_prune":   "/networks/prune",
		"volume_prune":    "/volumes/prune",
	} {
		t.Run(action, func(t *testing.T) {
			requested := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requested <- request.URL.RequestURI()
				_ = json.NewEncoder(response).Encode(map[string]any{})
			}))
			defer server.Close()
			client := testHTTPClient(server)
			if err := client.ConfigureJobs(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			job, err := client.StartMaintenance(context.Background(), MaintenanceInput{
				Action: action, Confirmation: "PRUNE",
			})
			if err != nil {
				t.Fatal(err)
			}
			job = waitForDockerJob(t, client, job.ID)
			if job.Status != "succeeded" {
				t.Fatalf("scoped prune job = %#v", job)
			}
			if path := <-requested; path != wantPath {
				t.Fatalf("scoped prune path = %q, want %q", path, wantPath)
			}
		})
	}
}

func TestMaintenanceRejectsUnsafeNamesAndProtectsPanelResources(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testHTTPClient(server)
	if err := client.ConfigureJobs(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if validImageReference("nginx;touch /tmp/x") || validImageReference("../nginx") ||
		!validImageReference("ghcr.io/example/app:v1") {
		t.Fatal("image reference validation did not enforce the allowlist")
	}
	if _, err := client.StartMaintenance(context.Background(), MaintenanceInput{
		Action: "network_create", Name: "bad/name",
	}); err == nil {
		t.Fatal("unsafe network name was accepted")
	}
}

func TestDockerBackupIncludesAllEcosystemArtifactsAndCreatesPrivateArchive(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testHTTPClient(server)
	client.appRoot = t.TempDir()
	client.stateRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(client.appRoot, "example", "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.appRoot, "example", "compose.yml"), []byte("services: {}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(client.appRoot, "kpanel", "secrets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.appRoot, "kpanel", "secrets", "token"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.appRoot, "kpanel_port.conf"), []byte("8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.ConfigureJobs(filepath.Join(client.stateRoot, "jobs")); err != nil {
		t.Fatal(err)
	}
	job, err := client.StartMaintenance(context.Background(), MaintenanceInput{Action: "backup_create"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForDockerJob(t, client, job.ID)
	if job.Status != "succeeded" || job.ResultPath == "" {
		t.Fatalf("backup job failed: %#v", job)
	}
	info, err := os.Stat(job.ResultPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %v, %v", info, err)
	}
	file, err := os.Open(job.ResultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "docker/example/compose.yml") ||
		!strings.Contains(joined, "docker/kpanel/secrets/token") ||
		!strings.Contains(joined, "docker/kpanel_port.conf") ||
		strings.Contains(joined, "docker/.kpanel-backups/") {
		t.Fatalf("unexpected backup contents:\n%s", joined)
	}
}

func TestDockerDaemonSettingsPreserveExistingKeys(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testHTTPClient(server)
	client.daemonConfigPath = filepath.Join(t.TempDir(), "docker", "daemon.json")
	if err := os.MkdirAll(filepath.Dir(client.daemonConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		client.daemonConfigPath,
		[]byte("{\"log-driver\":\"local\",\"storage-driver\":\"overlay2\"}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	client.restartDocker = func(context.Context) error {
		restarts++
		return nil
	}

	if err := client.updateDaemonMirrors(context.Background(), "cn"); err != nil {
		t.Fatal(err)
	}
	if err := client.updateDaemonIPv6(context.Background(), true, "fd42:6b50:616e:656c::/64"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(client.daemonConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["log-driver"] != "local" || config["storage-driver"] != "overlay2" ||
		config["ipv6"] != true || config["fixed-cidr-v6"] != "fd42:6b50:616e:656c::/64" {
		t.Fatalf("existing daemon keys were not preserved: %#v", config)
	}
	mirrors, ok := config["registry-mirrors"].([]any)
	if !ok || len(mirrors) != len(kejilionDockerMirrors) || restarts != 2 {
		t.Fatalf("mirror/restart result = %#v, restarts=%d", config["registry-mirrors"], restarts)
	}
}

func TestDockerDaemonRestartFailureRollsBack(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testHTTPClient(server)
	client.daemonConfigPath = filepath.Join(t.TempDir(), "daemon.json")
	original := []byte("{\"log-driver\":\"json-file\"}\n")
	if err := os.WriteFile(client.daemonConfigPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	client.restartDocker = func(context.Context) error {
		restarts++
		if restarts == 1 {
			return errors.New("restart failed")
		}
		return nil
	}
	if err := client.updateDaemonMirrors(context.Background(), "cn"); err == nil ||
		!strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatalf("rollback error = %v", err)
	}
	data, err := os.ReadFile(client.daemonConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) || restarts != 2 {
		t.Fatalf("daemon config was not rolled back: %q, restarts=%d", data, restarts)
	}
}

func TestDockerIPv6CIDRValidation(t *testing.T) {
	if !validDockerIPv6CIDR("fd42:6b50:616e:656c::/64") {
		t.Fatal("valid IPv6 /64 was rejected")
	}
	for _, value := range []string{
		"2001:db8:1::/64",
		"fd42:6b50:616e:656c::/48",
		"127.0.0.1/64",
		"not-a-cidr",
	} {
		if validDockerIPv6CIDR(value) {
			t.Fatalf("unsafe IPv6 CIDR was accepted: %s", value)
		}
	}
}

func waitForDockerJob(t *testing.T, client *Client, id string) MaintenanceJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := client.MaintenanceJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != "queued" && job.Status != "running" {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Docker job did not finish: %#v", client.MaintenanceJobs())
	return MaintenanceJob{}
}
