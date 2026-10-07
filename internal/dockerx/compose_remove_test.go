package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func composeRemovalTestClient(t *testing.T, additionalSources ...string) (*Client, ComposeProject, *atomic.Bool) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"docker-compose.yml": "services:\n  web:\n    image: nginx:alpine\n",
		".env":               "PASSWORD=keep-private\n", "data/database": "keep-data",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var removed atomic.Bool
	configPaths := []string{filepath.Join(dir, "docker-compose.yml")}
	for index, source := range additionalSources {
		path := filepath.Join(dir, fmt.Sprintf("override-%d.yml", index))
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		configPaths = append(configPaths, path)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		items := []containerListItem{{ID: strings.Repeat("b", 64), Labels: map[string]string{
			"com.docker.compose.project": "unrelated",
		}}}
		if !removed.Load() {
			items = append(items, containerListItem{ID: strings.Repeat("a", 64), Labels: map[string]string{
				"com.docker.compose.project": "demo", "com.docker.compose.service": "web",
				"com.docker.compose.project.working_dir":  dir,
				"com.docker.compose.project.config_files": strings.Join(configPaths, ","),
			}})
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(server.Close)
	client := testHTTPClient(server)
	client.appRoot = root
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	return client, project, &removed
}

func TestComposeRemovalPreservesDataAndArchivesConfiguration(t *testing.T) {
	for _, removeVolumes := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep-volumes", true: "remove-volumes"}[removeVolumes], func(t *testing.T) {
			client, project, removed := composeRemovalTestClient(t)
			var calls [][]string
			client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
				calls = append(calls, append([]string(nil), args...))
				removed.Store(true)
				return nil, nil
			}
			id := strings.Repeat("c", 32)
			input := MaintenanceInput{Action: "compose_remove", Name: "demo",
				ExpectedResourceVersion: project.ResourceVersion, RemoveComposeFiles: true, RemoveVolumes: removeVolumes}
			if err := client.validateMaintenanceInput(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			path, err := client.removeComposeProject(context.Background(), input, id)
			if err != nil || path != project.WorkingDirectory {
				t.Fatalf("remove = %q, %v", path, err)
			}
			if len(calls) != 1 || !containsArgumentSequence(calls[0], "--project-name", "demo", "down", "--remove-orphans") ||
				containsArgumentSequence(calls[0], "--volumes") != removeVolumes || containsArgumentSequence(calls[0], "--rmi") {
				t.Fatalf("unexpected commands: %v", calls)
			}
			file := project.ConfigFiles[0]
			backup := file.Path + ".kpanel-removed-" + id
			data, err := os.ReadFile(backup)
			if err != nil || string(data) != file.Source {
				t.Fatalf("configuration backup = %q, %v", data, err)
			}
			if _, err := os.Lstat(file.Path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("active configuration still exists: %v", err)
			}
			if len(client.ComposeProjects()) != 0 {
				t.Fatal("deleted project is still discovered")
			}
			for name, want := range map[string]string{".env": "PASSWORD=keep-private\n", "data/database": "keep-data"} {
				data, err := os.ReadFile(filepath.Join(project.WorkingDirectory, name))
				if err != nil || string(data) != want {
					t.Fatalf("preserved %s = %q, %v", name, data, err)
				}
			}
			// Restoring the original filename restores shared discovery, without a Panel database.
			if err := os.Rename(backup, file.Path); err != nil {
				t.Fatal(err)
			}
			if len(client.ComposeProjects()) != 1 {
				t.Fatal("restored configuration was not rediscovered")
			}
		})
	}
}

func TestComposeRemovalArchivesAllActiveConfigurationFiles(t *testing.T) {
	client, project, removed := composeRemovalTestClient(t, "services:\n  web:\n    restart: always\n")
	client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
		removed.Store(true)
		return nil, nil
	}
	id := strings.Repeat("e", 32)
	if _, err := client.removeComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_remove", Name: "demo", ExpectedResourceVersion: project.ResourceVersion, RemoveComposeFiles: true,
	}, id); err != nil {
		t.Fatal(err)
	}
	for _, file := range project.ConfigFiles {
		data, err := os.ReadFile(file.Path + ".kpanel-removed-" + id)
		if err != nil || string(data) != file.Source {
			t.Fatalf("backup %s = %q, %v", file.Path, data, err)
		}
	}
}

func TestComposeRemovalRunsAsPersistedBackgroundJob(t *testing.T) {
	client, project, removed := composeRemovalTestClient(t)
	client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) { removed.Store(true); return nil, nil }
	stateDir := t.TempDir()
	if err := client.ConfigureJobs(stateDir); err != nil {
		t.Fatal(err)
	}
	job, err := client.StartMaintenance(context.Background(), MaintenanceInput{
		Action: "compose_remove", Name: "demo", ExpectedResourceVersion: project.ResourceVersion, RemoveComposeFiles: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, err := client.MaintenanceJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "succeeded" {
			data, err := os.ReadFile(filepath.Join(stateDir, job.ID+".json"))
			if err != nil || strings.Contains(string(data), "keep-private") || !strings.Contains(current.Message, "数据卷、挂载目录和镜像保留") {
				t.Fatalf("persisted removal = %s, %v, %#v", data, err, current)
			}
			return
		}
		if current.Status == "failed" {
			t.Fatalf("background removal failed: %#v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background removal did not finish")
}

func TestComposeRemovalCanRetainConfigurationForRedeployment(t *testing.T) {
	client, project, removed := composeRemovalTestClient(t)
	// Already removed containers must not make an otherwise unchanged project stale.
	removed.Store(true)
	client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) { return nil, nil }
	input := MaintenanceInput{Action: "compose_remove", Name: "demo", ExpectedResourceVersion: project.ResourceVersion}
	if _, err := client.removeComposeProject(context.Background(), input, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(project.ConfigFiles[0].Path)
	if err != nil || string(data) != project.ConfigFiles[0].Source || len(client.ComposeProjects()) != 1 {
		t.Fatalf("configuration not retained: %q, %v", data, err)
	}
}

func TestComposeRemovalFailuresPreserveRecoveryFiles(t *testing.T) {
	for _, failure := range []string{"stale", "down", "remaining-container", "external-edit", "existing-backup"} {
		t.Run(failure, func(t *testing.T) {
			client, project, removed := composeRemovalTestClient(t)
			file := project.ConfigFiles[0]
			id := strings.Repeat("d", 32)
			input := MaintenanceInput{Action: "compose_remove", Name: "demo",
				ExpectedResourceVersion: project.ResourceVersion, RemoveComposeFiles: true}
			if failure == "stale" {
				input.ExpectedResourceVersion = "stale"
			}
			if failure == "existing-backup" {
				if err := os.WriteFile(file.Path+".kpanel-removed-"+id, []byte("previous-backup"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			want := file.Source
			client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
				calls++
				if failure == "down" {
					return nil, errors.New("Docker unavailable")
				}
				if failure == "external-edit" {
					want += "# edited externally\n"
					if err := os.WriteFile(file.Path, []byte(want), 0o600); err != nil {
						return nil, err
					}
				}
				removed.Store(failure != "remaining-container")
				return nil, nil
			}
			if _, err := client.removeComposeProject(context.Background(), input, id); err == nil {
				t.Fatal("expected removal failure")
			}
			data, err := os.ReadFile(file.Path)
			if err != nil || string(data) != want {
				t.Fatalf("recovery configuration overwritten: %q, %v", data, err)
			}
			if (failure == "stale" || failure == "existing-backup") && calls != 0 {
				t.Fatalf("precondition failed after executing %d commands", calls)
			}
		})
	}
}

func TestComposeArchiveGuardRestoresConfigurationAndRejectsExternalReplacement(t *testing.T) {
	_, project, _ := composeRemovalTestClient(t)
	guard, err := newComposeEditGuard(project)
	if err != nil {
		t.Fatal(err)
	}
	source := project.ConfigFiles[0].Path
	backup := source + ".backup"
	guard.files[backup] = composeEditFile{}
	if err := guard.move(source, backup); err != nil {
		t.Fatal(err)
	}
	if err := guard.move(backup, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guard.move(source, backup); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("external replacement was not rejected: %v", err)
	}
	data, _ := os.ReadFile(backup)
	if string(data) != "external" {
		t.Fatal("external backup overwritten")
	}
}
