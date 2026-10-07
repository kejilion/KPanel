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
	return composeRemovalIdentityClient(t, "demo", false, false, additionalSources...)
}

func composeRemovalIdentityClient(t *testing.T, name string, rootProject, shared bool, additionalSources ...string) (*Client, ComposeProject, *atomic.Bool) {
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
		if shared {
			items[0].Labels["com.docker.compose.project.working_dir"] = dir
			items[0].Labels["com.docker.compose.project.config_files"] = strings.Join(configPaths, ",")
		}
		if !removed.Load() {
			items = append(items, containerListItem{ID: strings.Repeat("a", 64), Labels: map[string]string{
				"com.docker.compose.project": name, "com.docker.compose.service": "web",
				"com.docker.compose.project.working_dir":  dir,
				"com.docker.compose.project.config_files": strings.Join(configPaths, ","),
			}})
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(server.Close)
	client := testHTTPClient(server)
	client.appRoot = root
	if rootProject {
		client.appRoot = t.TempDir()
		client.webRoot = dir
	}
	project, err := client.ComposeProject(context.Background(), name)
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

func TestComposeRemovalRetainsRootAliasAndMultipleFileIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		root  bool
		multi bool
	}{{"demo", true, false}, {"kejilion-demo", false, false}, {"demo", false, true}, {"kejilion-demo", true, true}} {
		t.Run(fmt.Sprintf("%s-root=%t-multi=%t", scenario.name, scenario.root, scenario.multi), func(t *testing.T) {
			var additional []string
			if scenario.multi {
				additional = append(additional, "services:\n  web:\n    ports: ['127.0.0.1:18080:80']\n")
			}
			client, before, removed := composeRemovalIdentityClient(t, scenario.name, scenario.root, false, additional...)
			var commands [][]string
			client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
				commands = append(commands, append([]string(nil), args...))
				removed.Store(true)
				if containsArgumentSequence(args, "config", "--services") {
					return []byte("web\n"), nil
				}
				if containsArgumentSequence(args, "ps", "--all", "--quiet") {
					return []byte(strings.Repeat("a", 64) + "\n"), nil
				}
				return nil, nil
			}
			if _, err := client.removeComposeProject(context.Background(), MaintenanceInput{
				Action: "compose_remove", Name: before.Name, ExpectedResourceVersion: before.ResourceVersion,
			}, ""); err != nil {
				t.Fatal(err)
			}
			projects := client.ComposeProjects()
			if len(projects) != 1 || projects[0].Name != before.Name {
				t.Fatalf("project identity after removal = %#v", projects)
			}
			after, err := client.ComposeProject(context.Background(), before.Name)
			if err != nil || after.Name != before.Name || after.WorkingDirectory != before.WorkingDirectory || len(after.ConfigFiles) != len(before.ConfigFiles) {
				t.Fatalf("retained project = %#v, %v", after, err)
			}
			for index, file := range before.ConfigFiles {
				if after.ConfigFiles[index] != file {
					t.Fatalf("active file %d changed: %#v", index, after.ConfigFiles)
				}
			}
			if after.EnvironmentFile == nil || !strings.HasPrefix(after.EnvironmentFile.Source, "PASSWORD=keep-private\n") {
				t.Fatal("original environment variables were not preserved")
			}
			if err := client.redeployComposeProject(context.Background(), MaintenanceInput{
				Name: after.Name, ExpectedResourceVersion: after.ResourceVersion,
				ComposeFile: after.ConfigFiles[0].Path, Compose: after.ConfigFiles[0].Source,
			}); err != nil {
				t.Fatal(err)
			}
			start := commands[len(commands)-2]
			if !containsArgumentSequence(start, "--project-name", before.Name, "up", "--detach") {
				t.Fatalf("redeployment identity = %v", start)
			}
			for _, file := range before.ConfigFiles {
				if !containsArgumentSequence(start, "--file", file.Path) {
					t.Fatalf("redeployment omitted %s: %v", file.Path, start)
				}
			}
		})
	}
}

func TestComposeRemovalRecoveryEnvironmentRollbackAndExternalEdits(t *testing.T) {
	for _, scenario := range []string{"existing", "absent", "external-edit"} {
		t.Run(scenario, func(t *testing.T) {
			client, project, _ := composeRemovalTestClient(t, "services:\n  web:\n    restart: always\n")
			path := filepath.Join(project.WorkingDirectory, ".env")
			if scenario == "absent" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				var err error
				project, err = client.ComposeProject(context.Background(), project.Name)
				if err != nil {
					t.Fatal(err)
				}
			}
			client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), "COMPOSE_FILE='") || !containsArgumentSequence(args, "--env-file", path) {
					t.Fatalf("native recovery was not saved before down: %q, %v, %v", data, err, args)
				}
				if scenario == "external-edit" {
					if err := os.WriteFile(path, []byte("PASSWORD=externally-changed\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return nil, errors.New("down failed")
			}
			_, err := client.removeComposeProject(context.Background(), MaintenanceInput{
				Name: project.Name, ExpectedResourceVersion: project.ResourceVersion,
			}, "")
			if err == nil {
				t.Fatal("expected failure")
			}
			data, readErr := os.ReadFile(path)
			if scenario == "absent" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("new environment not rolled back: %v", readErr)
				}
			} else {
				want := "PASSWORD=keep-private\n"
				if scenario == "external-edit" {
					want = "PASSWORD=externally-changed\n"
				}
				if readErr != nil || string(data) != want {
					t.Fatalf("environment overwritten: %q, %v", data, readErr)
				}
			}
		})
	}
}

func TestComposeRemovalPartialDownFailureRetainsRecoveryIdentity(t *testing.T) {
	for _, archive := range []bool{false, true} {
		t.Run(fmt.Sprintf("archive=%t", archive), func(t *testing.T) {
			client, before, removed := composeRemovalIdentityClient(t, "kejilion-demo", true, false, "services:\n  web:\n    restart: always\n")
			client.composeCommand = func(_ context.Context, _ ...string) ([]byte, error) {
				removed.Store(true)
				return nil, errors.New("network cleanup failed after container removal")
			}
			_, err := client.removeComposeProject(context.Background(), MaintenanceInput{
				Name: before.Name, ExpectedResourceVersion: before.ResourceVersion, RemoveComposeFiles: archive,
			}, strings.Repeat("e", 32))
			if err == nil {
				t.Fatal("partial failure was reported as success")
			}
			after, err := client.ComposeProject(context.Background(), before.Name)
			if err != nil || len(after.ConfigFiles) != len(before.ConfigFiles) {
				t.Fatalf("partial failure lost recovery identity: %#v, %v", after, err)
			}
			for index, file := range before.ConfigFiles {
				if after.ConfigFiles[index] != file {
					t.Fatalf("partial failure changed recovery files: %#v", after.ConfigFiles)
				}
			}
		})
	}
}

func TestComposeRemovalSharedConfigurationPreservesOtherProject(t *testing.T) {
	for _, scenario := range []string{"archive", "keep-default", "keep-multiple"} {
		t.Run(scenario, func(t *testing.T) {
			var additional []string
			if scenario == "keep-multiple" {
				additional = append(additional, "services:\n  web:\n    restart: always\n")
			}
			client, project, removed := composeRemovalIdentityClient(t, "demo", false, true, additional...)
			calls := 0
			client.composeCommand = func(_ context.Context, _ ...string) ([]byte, error) { calls++; removed.Store(true); return nil, nil }
			_, err := client.removeComposeProject(context.Background(), MaintenanceInput{
				Name: project.Name, ExpectedResourceVersion: project.ResourceVersion, RemoveComposeFiles: scenario == "archive",
			}, strings.Repeat("f", 32))
			if scenario == "keep-default" {
				if err != nil || calls != 1 {
					t.Fatalf("shared deployment removal = %v, calls=%d", err, calls)
				}
			} else if !errors.Is(err, ErrResourceConflict) || calls != 0 {
				t.Fatalf("shared files were not protected before down: %v, calls=%d", err, calls)
			}
			data, err := os.ReadFile(filepath.Join(project.WorkingDirectory, ".env"))
			if err != nil || string(data) != "PASSWORD=keep-private\n" {
				t.Fatalf("shared environment changed: %q, %v", data, err)
			}
			for _, file := range project.ConfigFiles {
				data, err := os.ReadFile(file.Path)
				if err != nil || string(data) != file.Source {
					t.Fatalf("shared configuration changed: %q, %v", data, err)
				}
			}
		})
	}
}

func TestComposeDiscoveryIgnoresVariablesInsideMultilineSecrets(t *testing.T) {
	data := []byte("SECRET='first line\nCOMPOSE_PROJECT_NAME=wrong\nlast line'\nCOMPOSE_PROJECT_NAME='real-project' # comment\nCOMPOSE_FILE='a\\'b.yml|override.yml'\nCOMPOSE_PATH_SEPARATOR='|'\n")
	values, err := composeDiscoveryVariables(data)
	if err != nil || values["COMPOSE_PROJECT_NAME"] != "real-project" || values["COMPOSE_FILE"] != "a'b.yml|override.yml" {
		t.Fatalf("literal discovery = %#v, %v", values, err)
	}
	if _, err := composeDiscoveryVariables([]byte("COMPOSE_FILE=${FILE}\n")); !errors.Is(err, ErrActionUnsupported) {
		t.Fatalf("dynamic file selection was guessed: %v", err)
	}
}

func TestComposeRemovalSharedConfigurationWithSeparateEnvironmentCanRetainFiles(t *testing.T) {
	_, project, removed := composeRemovalIdentityClient(t, "kejilion-demo", false, false)
	otherDirectory := t.TempDir()
	otherEnvironment := filepath.Join(otherDirectory, ".env")
	if err := os.WriteFile(otherEnvironment, []byte("OTHER=unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		items := []containerListItem{{ID: strings.Repeat("b", 64), Labels: map[string]string{
			"com.docker.compose.project": "other", "com.docker.compose.project.working_dir": otherDirectory,
			"com.docker.compose.project.config_files": project.ConfigFiles[0].Path,
		}}}
		if !removed.Load() {
			items = append(items, containerListItem{ID: strings.Repeat("a", 64), Labels: map[string]string{
				"com.docker.compose.project": project.Name, "com.docker.compose.project.working_dir": project.WorkingDirectory,
				"com.docker.compose.service": "web", "com.docker.compose.project.config_files": project.ConfigFiles[0].Path,
			}})
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(server.Close)
	client := testHTTPClient(server)
	client.appRoot = filepath.Dir(project.WorkingDirectory)
	client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
		if !containsArgumentSequence(args, "--project-name", project.Name, "down", "--remove-orphans") {
			t.Fatalf("wrong target: %v", args)
		}
		removed.Store(true)
		return nil, nil
	}
	if _, err := client.removeComposeProject(context.Background(), MaintenanceInput{
		Name: project.Name, ExpectedResourceVersion: project.ResourceVersion,
	}, ""); err != nil {
		t.Fatal(err)
	}
	after, err := client.ComposeProject(context.Background(), project.Name)
	if err != nil || len(after.ConfigFiles) != 1 || after.ConfigFiles[0] != project.ConfigFiles[0] {
		t.Fatalf("shared file changed: %#v, %v", after, err)
	}
	data, err := os.ReadFile(otherEnvironment)
	if err != nil || string(data) != "OTHER=unchanged\n" {
		t.Fatalf("other environment changed: %q, %v", data, err)
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
