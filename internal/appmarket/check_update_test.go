package appmarket

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

type racingUpdateDocker struct {
	*fakeDocker
	versions       []string
	inventoryCalls int
	checked        []string
}

func (docker *racingUpdateDocker) Containers(context.Context) ([]contract.ContainerSummary, error) {
	versionIndex := docker.inventoryCalls
	if versionIndex >= len(docker.versions) {
		versionIndex = len(docker.versions) - 1
	}
	docker.inventoryCalls++
	container := docker.fakeDocker.containers[0]
	container.ResourceVersion = docker.versions[versionIndex]
	return []contract.ContainerSummary{container}, nil
}

func (docker *racingUpdateDocker) CheckContainerImageUpdate(
	_ context.Context,
	containerID, expectedVersion string,
) (dockerx.ImageUpdateResult, error) {
	docker.checked = append(docker.checked, expectedVersion)
	if len(docker.checked) == 1 {
		return dockerx.ImageUpdateResult{}, dockerx.ErrResourceConflict
	}
	return dockerx.ImageUpdateResult{
		ContainerID:     containerID,
		Status:          "current",
		ResourceVersion: expectedVersion,
	}, nil
}

func TestCheckUpdateUsesFreshInventoryWithoutMultiplyingSharedRetries(t *testing.T) {
	containerID := strings.Repeat("a", 64)
	docker := &racingUpdateDocker{
		fakeDocker: &fakeDocker{containers: []contract.ContainerSummary{{
			ID: containerID, Name: "speedtest",
			Image: "ghcr.io/librespeed/speedtest:latest",
			State: "running", Status: "Up",
			Ports: []contract.PortBinding{{
				PrivatePort: 8080, PublicPort: 8028, IP: "0.0.0.0", Type: "tcp",
			}},
			AllowedActions: []string{"stop", "restart"},
		}}},
		versions: []string{"current-version", "refreshed-version"},
	}
	service, err := New(docker, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.CheckUpdate(context.Background(), "builtin-28", "browser-stale-version")
	if !errors.Is(err, dockerx.ErrResourceConflict) {
		t.Fatalf("shared check conflict must remain visible after its retry budget: %v", err)
	}
	if len(docker.checked) != 1 || docker.checked[0] != "current-version" || docker.inventoryCalls != 1 {
		t.Fatalf("checked versions = %#v", docker.checked)
	}
}

func TestCheckUpdateStillRequiresBrowserResourceVersion(t *testing.T) {
	service, err := New(&fakeDocker{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CheckUpdate(context.Background(), "builtin-28", "")
	if !errors.Is(err, dockerx.ErrVersionRequired) {
		t.Fatalf("error = %v, want resource version required", err)
	}
}
