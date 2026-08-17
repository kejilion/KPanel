//go:build integration && linux

package dockerx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestComposeLifecycleAgainstDocker(t *testing.T) {
	if os.Getenv("KPANEL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set KPANEL_DOCKER_INTEGRATION=1 to run against a real Docker daemon")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := t.TempDir()
	appRoot := filepath.Join(root, "docker")
	webRoot := filepath.Join(root, "web")
	for _, path := range []string{appRoot, webRoot} {
		if err := os.Mkdir(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	projectName := fmt.Sprintf("kpanel-it-%d", os.Getpid())
	client := New("/var/run/docker.sock", webRoot, filepath.Join(root, "state"))
	client.appRoot = appRoot
	client.ConfigureDaemonAccess("/run/docker.pid", true)

	image := strings.TrimSpace(os.Getenv("KPANEL_DOCKER_INTEGRATION_IMAGE"))
	if image == "" {
		image = "alpine:3.22"
	}
	original := integrationComposeSource(image, "original")
	if err := client.deployComposeProject(ctx, MaintenanceInput{
		Action: "compose_deploy", Name: projectName, Compose: original,
	}); err != nil {
		t.Fatalf("deploy real Compose project: %v", err)
	}

	t.Cleanup(func() {
		project, err := client.ComposeProject(context.Background(), projectName)
		if err == nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			_, _ = client.runCompose(cleanupCtx, append(composeProjectBase(project), "down", "--volumes", "--remove-orphans")...)
		}
	})

	project := requireComposeProject(t, client, projectName)
	if len(project.Services) != 1 || project.Services[0] != "worker" ||
		len(project.ConfigFiles) != 1 || project.ConfigFiles[0].Source != original {
		t.Fatalf("discovered project = %#v", project)
	}

	runLifecycle := func(action, operation string) {
		t.Helper()
		current := requireComposeProject(t, client, projectName)
		if err := client.runComposeProjectLifecycle(ctx, MaintenanceInput{
			Action: action, Name: projectName, ExpectedResourceVersion: current.ResourceVersion,
		}, operation); err != nil {
			t.Fatalf("Compose %s: %v", operation, err)
		}
	}
	runLifecycle("compose_stop", "stop")
	runLifecycle("compose_start", "start")
	runLifecycle("compose_restart", "restart")

	project = requireComposeProject(t, client, projectName)
	composePath := project.ConfigFiles[0].Path
	failing := integrationComposeSource("kpanel.invalid/missing:never", "must-rollback")
	failing = strings.Replace(failing, "    image: kpanel.invalid/missing:never\n", "    image: kpanel.invalid/missing:never\n    pull_policy: never\n", 1)
	err := client.redeployComposeProject(ctx, MaintenanceInput{
		Action: "compose_redeploy", Name: projectName, ComposeFile: composePath,
		Compose: failing, ExpectedResourceVersion: project.ResourceVersion,
	})
	if err == nil || !strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatalf("failed redeploy rollback = %v", err)
	}
	restored, readErr := os.ReadFile(composePath)
	if readErr != nil || string(restored) != original {
		t.Fatalf("restored Compose source = %q, %v", restored, readErr)
	}

	project = requireComposeProject(t, client, projectName)
	updated := integrationComposeSource(image, "updated")
	if err := client.redeployComposeProject(ctx, MaintenanceInput{
		Action: "compose_redeploy", Name: projectName, ComposeFile: composePath,
		Compose: updated, ExpectedResourceVersion: project.ResourceVersion,
	}); err != nil {
		t.Fatalf("redeploy real Compose project: %v", err)
	}
	project = requireComposeProject(t, client, projectName)
	if project.ConfigFiles[0].Source != updated {
		t.Fatalf("updated Compose source = %q", project.ConfigFiles[0].Source)
	}

	err = client.runComposeProjectLifecycle(ctx, MaintenanceInput{
		Action: "compose_stop", Name: projectName, ExpectedResourceVersion: "stale-version",
	}, "stop")
	if !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("stale resource version error = %v", err)
	}
}

func requireComposeProject(t *testing.T, client *Client, name string) ComposeProject {
	t.Helper()
	project, err := client.ComposeProject(context.Background(), name)
	if err != nil {
		t.Fatalf("resolve Compose project: %v", err)
	}
	return project
}

func integrationComposeSource(image, revision string) string {
	return fmt.Sprintf(`services:
  worker:
    image: %s
    command: ["sh", "-c", "while true; do sleep 30; done"]
    labels:
      kpanel.integration.revision: %s
`, image, revision)
}
