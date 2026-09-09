package appmarket

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestInstalledApplicationsCanOpenNativeScriptManagement(t *testing.T) {
	for _, app := range []struct{ id, token, selector string }{
		{"builtin-55", "frps", "55"},
		{"builtin-56", "frpc", "56"},
		{"builtin-28", "speedtest", "28"},
		{"builtin-64", "it-tools", "64"},
		{"builtin-76", "dosgame", "76"},
		{"thirdparty-CLIProxyAPI", "CLIProxyAPI", "CLIProxyAPI"},
	} {
		for _, state := range []string{"running", "exited"} {
			t.Run(app.id+"/"+state, func(t *testing.T) {
				root := t.TempDir()
				// FRP uses a dedicated script menu; third-party metadata can be dynamic.
				if err := os.WriteFile(filepath.Join(root, "appno.txt"), []byte(app.selector+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				containerID := strings.Repeat("a", 64)
				docker := &fakeDocker{containers: []contract.ContainerSummary{{
					ID: containerID, Name: app.token, State: state, Image: "example/app:latest",
					ResourceVersion: "current-version", AllowedActions: []string{"restart"},
				}}}
				service, err := New(docker, root)
				if err != nil {
					t.Fatal(err)
				}
				service.scriptAppRoot = filepath.Join(root, "apps")
				if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), &fakeJobRunner{}); err != nil {
					t.Fatal(err)
				}
				service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
				service.scriptInteractiveManageFinder = service.scriptInteractiveFinder
				service.scriptManageFinder = service.scriptInteractiveFinder
				item, err := service.Find(context.Background(), app.id)
				if err != nil {
					t.Fatal(err)
				}
				if !item.Capabilities["manage"].Enabled || item.Runtime.ContainerID != containerID {
					t.Fatalf("installed application did not expose native management: %#v", item)
				}
				if _, _, err := service.StartScriptMutation(context.Background(), app.id, "manage", MutationInput{ResourceVersion: "stale"}); !errors.Is(err, ErrConflict) {
					t.Fatalf("stale management request = %v", err)
				}
				if len(service.AppJobs()) != 0 {
					t.Fatal("rejected management request created a job")
				}
				job, handled, err := service.StartScriptMutation(context.Background(), app.id, "manage", MutationInput{ResourceVersion: item.Runtime.ResourceVersion})
				if err != nil || !handled {
					t.Fatalf("management launch: handled=%v err=%v", handled, err)
				}
				record, err := service.jobs.read(job.ID)
				if err != nil {
					t.Fatal(err)
				}
				if record.Selector != app.selector || record.Adapter != "kejilion" || !record.Interactive || record.ExpectedContainerID != containerID {
					t.Fatalf("management job lost fixed selector or container identity: %#v", record)
				}
				env := interactiveAppJobEnvironment(record)
				if slices.Contains(env, "KJ_APP_MARKER_RECOVERY=1") || !slices.Contains(env, "KJ_APP_EXPECTED_CONTAINER_ID="+containerID) {
					t.Fatalf("container management bypassed native identity checks: %#v", env)
				}
			})
		}
	}
}

func TestNativeManageEnvironmentPreservesMarkerRecovery(t *testing.T) {
	env := interactiveAppJobEnvironment(appJobRecord{AppJob: AppJob{Action: "manage"}})
	if !slices.Contains(env, "KJ_APP_MARKER_RECOVERY=1") {
		t.Fatalf("missing recovery environment: %#v", env)
	}
}

func TestUninstalledApplicationsDoNotExposeManagement(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), &fakeJobRunner{}); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	item, err := service.Find(context.Background(), "builtin-55")
	if err != nil {
		t.Fatal(err)
	}
	if item.Capabilities["manage"].Enabled {
		t.Fatal("uninstalled application exposed management")
	}
	if _, _, err := service.StartScriptMutation(context.Background(), item.ID, "manage", MutationInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("uninstalled application management = %v", err)
	}
}

func TestNativeScriptReportsUnsupportedLoopbackBindingsWithoutChangingExposure(t *testing.T) {
	for _, app := range []struct{ id, token string }{
		{"builtin-28", "speedtest"}, {"builtin-64", "it-tools"},
		{"builtin-76", "dosgame"}, {"builtin-65", "n8n"},
	} {
		for _, ip := range []string{"127.0.0.1", "127.0.0.2", "::1"} {
			t.Run(app.id+"/"+ip, func(t *testing.T) {
				root := t.TempDir()
				service, err := New(&fakeDocker{containers: []contract.ContainerSummary{{
					ID: strings.Repeat("a", 64), Name: app.token, State: "running",
					Image: "example/app:latest", ResourceVersion: "current",
					Ports:          []contract.PortBinding{{IP: ip, PublicPort: 8080, PrivatePort: 80, Type: "tcp"}},
					AllowedActions: []string{"stop", "restart"},
				}}}, root)
				if err != nil {
					t.Fatal(err)
				}
				if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "agent"), &fakeJobRunner{}); err != nil {
					t.Fatal(err)
				}
				service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
				service.scriptInteractiveManageFinder = service.scriptInteractiveFinder
				service.scriptManageFinder = service.scriptInteractiveFinder
				// A stale direct marker cannot override actual loopback binding.
				if err := os.WriteFile(filepath.Join(root, app.token+"_access.conf"), []byte("direct\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
				item, err := service.Find(context.Background(), app.id)
				if err != nil {
					t.Fatal(err)
				}
				if item.Runtime.AccessMode != "domain_only" || item.Runtime.Warning == "" {
					t.Fatalf("loopback access truth was lost: %#v", item.Runtime)
				}
				for _, action := range []string{"update", "direct_access", "manage"} {
					_, _, err := service.StartScriptMutation(context.Background(), app.id, action, MutationInput{ResourceVersion: "current", AccessMode: "direct"})
					if !errors.Is(err, ErrForbidden) {
						t.Fatalf("unsupported %s launch = %v", action, err)
					}
				}
				if len(service.AppJobs()) != 0 {
					t.Fatal("unsupported binding created a mutating job")
				}
				for _, action := range []string{"stop", "restart", "uninstall", "check_update", "add_domain"} {
					if !item.Capabilities[action].Enabled {
						t.Fatalf("unaffected %s capability was lost", action)
					}
				}
			})
		}
	}
}

func TestMixedBindingsAreNotReportedAsDomainOnly(t *testing.T) {
	for _, ports := range [][]contract.PortBinding{
		{{IP: "127.0.0.1", PublicPort: 8080}, {IP: "0.0.0.0", PublicPort: 8081}},
		{{IP: "0.0.0.0", PublicPort: 8081}, {IP: "::1", PublicPort: 8080}},
	} {
		if got := runtimeFromContainer(contract.ContainerSummary{Ports: ports}).AccessMode; got != "direct" {
			t.Fatalf("mixed binding access = %q", got)
		}
	}
}
