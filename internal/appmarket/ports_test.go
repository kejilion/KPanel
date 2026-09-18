package appmarket

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestInstallPortStatusCombinesDockerAndHostListeners(t *testing.T) {
	service, err := New(&fakeDocker{containers: []contract.ContainerSummary{{
		Name: "existing-app",
		Ports: []contract.PortBinding{{
			PublicPort: 18080,
			Type:       "tcp",
		}},
	}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.listeningPorts = func(context.Context) (map[uint16][]string, error) {
		return map[uint16][]string{18080: {"tcp6", "udp"}}, nil
	}

	status, err := service.CheckInstallPort(context.Background(), "builtin-13", 18080)
	if err != nil {
		t.Fatal(err)
	}
	if status.Available || len(status.Conflicts) != 3 {
		t.Fatalf("occupied status = %#v", status)
	}
	if status.Conflicts[0].Container != "existing-app" {
		t.Fatalf("Docker conflict was not reported first: %#v", status.Conflicts)
	}
}

func TestStartInstallRejectsOccupiedPanelSelectedPort(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.listeningPorts = func(context.Context) (map[uint16][]string, error) {
		return map[uint16][]string{18081: {"tcp"}}, nil
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "/usr/local/bin/k", nil
	}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}

	_, err = service.StartInstall(
		context.Background(),
		"builtin-4",
		InstallInput{HostPort: 18081},
	)
	if !errors.Is(err, ErrPortConflict) {
		t.Fatalf("occupied port error = %v, want ErrPortConflict", err)
	}
	if len(service.AppJobs()) != 0 {
		t.Fatal("occupied port created a background job")
	}
}

func TestCustomInstallerRejectsPanelPortOverride(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.listeningPorts = func(context.Context) (map[uint16][]string, error) {
		return map[uint16][]string{}, nil
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "/usr/local/bin/k", nil
	}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}

	_, err = service.StartInstall(
		context.Background(),
		"builtin-114",
		InstallInput{HostPort: 18082},
	)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("custom installer port error = %v, want ErrUnsupported", err)
	}
}

func TestListeningSocketPortFiltersBySocketState(t *testing.T) {
	cases := []struct {
		name string
		line string
		tcp  bool
		port uint16
		ok   bool
	}{
		{"tcp listen", "0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 1", true, 8080, true},
		{"tcp established", "1: 0100007F:1F90 0100007F:D2A4 01 00000000:00000000 00:00000000 00000000 0 0 2", true, 0, false},
		{"udp bound server", "2: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000 0 0 3", false, 53, true},
		{"udp connected client", "3: 0A00000F:C350 08080808:0035 01 00000000:00000000 00:00000000 00000000 0 0 4", false, 0, false},
		{"short row", "4: 00000000:0035", false, 0, false},
	}
	for _, item := range cases {
		port, ok := listeningSocketPort(item.line, item.tcp)
		if port != item.port || ok != item.ok {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", item.name, port, ok, item.port, item.ok)
		}
	}
}
