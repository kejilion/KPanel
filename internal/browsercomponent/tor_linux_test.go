package browsercomponent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestTorSelectionRequiresUninstallAndSurvivesRuntimeRecovery(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("Tor prototype is amd64 only")
	}
	s, f := fixture(t)
	ctx := context.Background()
	run := func(action, kind string) {
		t.Helper()
		v, err := s.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: action, Kind: kind, ExpectedResourceVersion: v.ResourceVersion})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.RunJob(ctx, j.ID); err != nil {
			t.Fatal(err)
		}
	}
	run("install", "tor")
	v, _ := s.Snapshot(ctx)
	if v.Kind != "tor" || !v.Healthy {
		t.Fatal(v)
	}
	if err := s.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(s.config.RuntimeDir, "browser-enabled"))
	if err != nil || string(marker) != "tor" {
		t.Fatal(string(marker), err)
	}
	if _, err = s.Start(ctx, contract.BrowserComponentRequest{Action: "install", Kind: "chromium", ExpectedResourceVersion: v.ResourceVersion}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	run("install", "") // Repair retains the actual installed kind for legacy clients.
	if f.kind != "tor" {
		t.Fatal(f.kind)
	}
	recipe, _ := os.ReadFile(filepath.Join(s.config.StateDir, "tor.Dockerfile"))
	ignore, _ := os.ReadFile(filepath.Join(s.config.StateDir, ".dockerignore"))
	if !strings.Contains(string(recipe), "gpgv --keyring") || !strings.HasPrefix(string(ignore), "*\n") {
		t.Fatal("build must verify upstream and exclude credentials")
	}
	run("uninstall", "")
	if _, err = os.Stat(filepath.Join(s.config.RuntimeDir, "browser-enabled")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	run("install", "chromium")
	if f.kind != "chromium" {
		t.Fatal(f.kind)
	}
}

func TestTorFailedBuildDoesNotEnableConnection(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("Tor prototype is amd64 only")
	}
	s, f := fixture(t)
	f.failInstall = true
	v, _ := s.Snapshot(context.Background())
	j, err := s.Start(context.Background(), contract.BrowserComponentRequest{Action: "install", Kind: "tor", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(context.Background(), j.ID); err == nil {
		t.Fatal("expected build failure")
	}
	if _, err = os.Stat(filepath.Join(s.config.RuntimeDir, "browser-enabled")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestTorReconcilerRestoresRecreatedPanelAndStopsAfterUninstall(t *testing.T) {
	s, f := fixture(t)
	ctx := context.Background()
	if err := s.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	if err := s.enableDefaultConnection("tor"); err != nil {
		t.Fatal(err)
	}
	f.installed = true
	f.running = true
	f.kind = "tor"
	if err := s.reconcileNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "network connect "+controlNetwork+" panel-id") {
		t.Fatal(f.calls)
	}
	v, _ := s.Snapshot(ctx)
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "uninstall", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err = s.reconcileNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatal("reconnected after uninstall", f.calls)
	}
}
