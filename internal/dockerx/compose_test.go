package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposeDeploymentPersistsProjectAndVerifiesContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/containers/json" {
			_, _ = response.Write([]byte(`[]`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.appRoot = t.TempDir()
	var calls []string
	client.composeCommand = func(_ context.Context, arguments ...string) ([]byte, error) {
		calls = append(calls, strings.Join(arguments, " "))
		switch {
		case containsArgumentSequence(arguments, "config", "--services"):
			return []byte("app\n"), nil
		case containsArgumentSequence(arguments, "up", "--detach"):
			return []byte("started\n"), nil
		case containsArgumentSequence(arguments, "ps", "--all", "--quiet"):
			return []byte(strings.Repeat("a", 64) + "\n"), nil
		default:
			return nil, errors.New("unexpected Compose command")
		}
	}
	input := MaintenanceInput{
		Action: "compose_deploy", Name: "demo",
		Compose:            "services:\n  app:\n    image: nginx:${IMAGE_TAG}\n",
		ComposeEnvironment: composeEnvironmentPointer("IMAGE_TAG=alpine"),
	}
	if err := client.deployComposeProject(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(client.appRoot, "demo", "docker-compose.yml"))
	if err != nil || string(data) != input.Compose {
		t.Fatalf("persisted Compose source = %q, %v", data, err)
	}
	environmentPath := filepath.Join(client.appRoot, "demo", ".env")
	environmentData, environmentErr := os.ReadFile(environmentPath)
	environmentInfo, _ := os.Stat(environmentPath)
	if environmentErr != nil || string(environmentData) != "IMAGE_TAG=alpine\n" || environmentInfo == nil ||
		(runtime.GOOS != "windows" && environmentInfo.Mode().Perm() != 0o600) {
		t.Fatalf("persisted Compose environment = %q, %#v, %v", environmentData, environmentInfo, environmentErr)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "--env-file ") || !strings.Contains(joined, string(filepath.Separator)+".env") ||
		!strings.Contains(joined, "--project-name demo config --services") ||
		!strings.Contains(joined, "--project-name demo up --detach") ||
		!strings.Contains(joined, "--project-name demo ps --all --quiet") {
		t.Fatalf("Compose calls = %q", joined)
	}
}

func TestComposeDeploymentRollsBackFailedStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/containers/json" {
			_, _ = response.Write([]byte(`[]`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.appRoot = t.TempDir()
	rolledBack := false
	client.composeCommand = func(_ context.Context, arguments ...string) ([]byte, error) {
		switch {
		case containsArgumentSequence(arguments, "config", "--services"):
			return []byte("app\n"), nil
		case containsArgumentSequence(arguments, "up", "--detach"):
			return []byte("port is already allocated"), errors.New("exit status 1")
		case containsArgumentSequence(arguments, "down", "--remove-orphans"):
			rolledBack = true
			return nil, nil
		default:
			return nil, errors.New("unexpected Compose command")
		}
	}
	err := client.deployComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_deploy", Name: "demo",
		Compose: "services:\n  app:\n    image: nginx:alpine\n",
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") || !rolledBack {
		t.Fatalf("rollback result: err=%v rolledBack=%v", err, rolledBack)
	}
	if _, statErr := os.Stat(filepath.Join(client.appRoot, "demo")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Compose project directory remains after rollback: %v", statErr)
	}
}

func TestComposeDeploymentKeepsProjectFilesWhenRollbackNeedsAttention(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/containers/json" {
			_, _ = response.Write([]byte(`[]`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.appRoot = t.TempDir()
	client.composeCommand = func(_ context.Context, arguments ...string) ([]byte, error) {
		switch {
		case containsArgumentSequence(arguments, "config", "--services"):
			return []byte("app\n"), nil
		case containsArgumentSequence(arguments, "up", "--detach"):
			return []byte("start failed"), errors.New("exit status 1")
		case containsArgumentSequence(arguments, "down", "--remove-orphans"):
			return []byte("daemon unavailable"), errors.New("exit status 1")
		default:
			return nil, errors.New("unexpected Compose command")
		}
	}
	err := client.deployComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_deploy", Name: "demo",
		Compose: "services:\n  app:\n    image: nginx:alpine\n",
	})
	if err == nil || !strings.Contains(err.Error(), "needs attention") {
		t.Fatalf("rollback attention error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(client.appRoot, "demo", "docker-compose.yml")); statErr != nil {
		t.Fatalf("Compose source needed for recovery was removed: %v", statErr)
	}
}

func TestComposeDeploymentRejectsExistingProjectBeforeWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/containers/json" {
			_, _ = response.Write([]byte(`[]`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.appRoot = t.TempDir()
	if err := os.Mkdir(filepath.Join(client.appRoot, "demo"), 0o750); err != nil {
		t.Fatal(err)
	}
	err := client.validateComposeDeploymentInput(context.Background(), MaintenanceInput{
		Action: "compose_deploy", Name: "demo", Compose: "services:\n  app:\n    image: nginx\n",
	})
	if !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("existing project error = %v", err)
	}
}

func TestComposeProjectReadsDockerLabelSourceOfTruth(t *testing.T) {
	client, composePath := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
	environmentPath := filepath.Join(filepath.Dir(composePath), ".env")
	if err := os.WriteFile(environmentPath, []byte("APP_MODE=production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(project.ConfigFiles) != 1 {
		t.Fatalf("ComposeProject() = %#v", project)
	}
	wantInfo, _ := os.Stat(composePath)
	gotInfo, _ := os.Stat(project.ConfigFiles[0].Path)
	wantEnvironmentInfo, _ := os.Stat(environmentPath)
	var gotEnvironmentInfo os.FileInfo
	if project.EnvironmentFile != nil {
		gotEnvironmentInfo, _ = os.Stat(project.EnvironmentFile.Path)
	}
	if project.Name != "demo" || wantInfo == nil || gotInfo == nil || !os.SameFile(wantInfo, gotInfo) ||
		project.ConfigFiles[0].Source != "services:\n  web:\n    image: nginx:alpine\n" ||
		project.EnvironmentFile == nil || wantEnvironmentInfo == nil || gotEnvironmentInfo == nil ||
		!os.SameFile(wantEnvironmentInfo, gotEnvironmentInfo) ||
		project.EnvironmentFile.Source != "APP_MODE=production\n" ||
		len(project.Services) != 1 || project.Services[0] != "web" || project.ResourceVersion == "" {
		t.Fatalf("ComposeProject() = %#v", project)
	}
}

func composeEnvironmentPointer(source string) *string {
	return &source
}

func TestComposeRedeployValidatesStagesAndReplacesConfiguration(t *testing.T) {
	client, composePath := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
	environmentPath := filepath.Join(filepath.Dir(composePath), ".env")
	if err := os.WriteFile(environmentPath, []byte("APP_TAG=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	composePath = project.ConfigFiles[0].Path
	var calls []string
	client.composeCommand = func(_ context.Context, arguments ...string) ([]byte, error) {
		calls = append(calls, strings.Join(arguments, " "))
		switch {
		case containsArgumentSequence(arguments, "config", "--services"):
			joined := strings.Join(arguments, " ")
			if strings.Count(joined, ".kpanel-compose-") < 2 {
				t.Fatal("Compose validation did not use staged configuration and environment")
			}
			return []byte("web\n"), nil
		case containsArgumentSequence(arguments, "up", "--detach", "--remove-orphans"):
			joined := strings.Join(arguments, " ")
			if strings.Contains(joined, ".kpanel-compose-") || strings.Contains(joined, ".kpanel-env-") {
				t.Fatal("Compose redeploy used staged paths after validation")
			}
			return []byte("updated\n"), nil
		case containsArgumentSequence(arguments, "ps", "--all", "--quiet"):
			return []byte(strings.Repeat("a", 64) + "\n"), nil
		default:
			return nil, errors.New("unexpected Compose command")
		}
	}
	err = client.redeployComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_redeploy", Name: "demo", ComposeFile: composePath,
		Compose:                 "services:\n  web:\n    image: nginx:stable\n",
		ComposeEnvironment:      composeEnvironmentPointer("APP_TAG=new"),
		ExpectedResourceVersion: project.ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, readErr := os.ReadFile(composePath)
	if readErr != nil || string(updated) != "services:\n  web:\n    image: nginx:stable\n" {
		t.Fatalf("updated Compose source = %q, %v", updated, readErr)
	}
	updatedEnvironment, environmentErr := os.ReadFile(environmentPath)
	if environmentErr != nil || string(updatedEnvironment) != "APP_TAG=new\n" {
		t.Fatalf("updated Compose environment = %q, %v", updatedEnvironment, environmentErr)
	}
	if len(calls) != 3 {
		t.Fatalf("Compose calls = %q", calls)
	}
}

func TestComposeRedeployRestoresPreviousConfigurationAfterStartFailure(t *testing.T) {
	original := "services:\n  web:\n    image: nginx:alpine\n"
	client, composePath := composeProjectTestClient(t, original)
	environmentPath := filepath.Join(filepath.Dir(composePath), ".env")
	if err := os.WriteFile(environmentPath, []byte("APP_TAG=original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	composePath = project.ConfigFiles[0].Path
	upCalls := 0
	client.composeCommand = func(_ context.Context, arguments ...string) ([]byte, error) {
		switch {
		case containsArgumentSequence(arguments, "config", "--services"):
			return []byte("web\n"), nil
		case containsArgumentSequence(arguments, "up", "--detach", "--remove-orphans"):
			upCalls++
			if upCalls == 1 {
				return []byte("port conflict"), errors.New("exit status 1")
			}
			return []byte("restored"), nil
		default:
			return nil, errors.New("unexpected Compose command")
		}
	}
	err = client.redeployComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_redeploy", Name: "demo", ComposeFile: composePath,
		Compose:                 "services:\n  web:\n    image: nginx:stable\n",
		ComposeEnvironment:      composeEnvironmentPointer("APP_TAG=failed"),
		ExpectedResourceVersion: project.ResourceVersion,
	})
	if err == nil || !strings.Contains(err.Error(), "previous configuration restored") || upCalls != 2 {
		t.Fatalf("rollback result: err=%v upCalls=%d", err, upCalls)
	}
	restored, readErr := os.ReadFile(composePath)
	if readErr != nil || string(restored) != original {
		t.Fatalf("restored Compose source = %q, %v", restored, readErr)
	}
	restoredEnvironment, environmentErr := os.ReadFile(environmentPath)
	if environmentErr != nil || string(restoredEnvironment) != "APP_TAG=original\n" {
		t.Fatalf("restored Compose environment = %q, %v", restoredEnvironment, environmentErr)
	}
}

func composeProjectTestClient(t *testing.T, source string) (*Client, string) {
	t.Helper()
	root := t.TempDir()
	projectDir := filepath.Join(root, "demo")
	if err := os.Mkdir(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 64)
	labels := map[string]string{
		"com.docker.compose.project":              "demo",
		"com.docker.compose.project.working_dir":  projectDir,
		"com.docker.compose.project.config_files": composePath,
		"com.docker.compose.service":              "web",
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/containers/json":
			_ = json.NewEncoder(response).Encode([]containerListItem{{
				ID: id, Names: []string{"/demo-web-1"}, Image: "nginx:alpine", State: "running", Labels: labels,
			}})
		case request.URL.Path == "/containers/"+id+"/json":
			inspect := managedInspect(id, "2026-08-17T00:00:00Z", 0)
			inspect.Name = "/demo-web-1"
			inspect.Config.Labels = labels
			_ = json.NewEncoder(response).Encode(inspect)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	client := testHTTPClient(server)
	client.appRoot = root
	client.webRoot = filepath.Join(root, "web")
	return client, composePath
}

func containsArgumentSequence(arguments []string, sequence ...string) bool {
	for index := 0; index+len(sequence) <= len(arguments); index++ {
		if strings.Join(arguments[index:index+len(sequence)], "\x00") == strings.Join(sequence, "\x00") {
			return true
		}
	}
	return false
}
