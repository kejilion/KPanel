package browsercomponent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type fakeRunner struct {
	installed, running, failInstall, workerRunning bool
	networkMissing                                 bool
	unsafeNetwork                                  bool
	runtimeParent                                  string
	project, networkProject                        string
	calls                                          []string
}

func (f *fakeRunner) LookPath(name string) (string, error) { return "/usr/bin/" + name, nil }
func (f *fakeRunner) InitRuntimeDirectoryExists(path string) bool {
	return path == "/run/systemd/system"
}
func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	a := strings.Join(args, " ")
	f.calls = append(f.calls, name+" "+a)
	if name == "systemd-run" {
		return nil, nil
	}
	if name == "systemctl" {
		if f.workerRunning {
			return []byte("ActiveState=active\nSubState=running\nResult=success\n"), nil
		}
		return []byte("ActiveState=inactive\nSubState=dead\nResult=success\n"), nil
	}
	if strings.Contains(a, "container ls") {
		if f.installed {
			return []byte("abcdef012345"), nil
		}
		return nil, nil
	}
	if strings.Contains(a, "container inspect kejilion-panel") {
		return []byte(`[{"Id":"panel-id","Config":{"Labels":{"com.docker.compose.project":"` + f.project + `","com.docker.compose.service":"panel"}},"State":{"Running":true},"Mounts":[{"Source":"` + f.runtimeParent + `","Destination":"/run/kejilion-panel","RW":false}],"NetworkSettings":{"Networks":{}}}]`), nil
	}
	if strings.Contains(a, "network inspect") {
		if f.networkMissing {
			return nil, errors.New("network missing")
		}
		if f.unsafeNetwork {
			return []byte(`[{"Internal":false,"Driver":"bridge"}]`), nil
		}
		return []byte(`[{"Internal":true,"Driver":"bridge","Labels":{"com.docker.compose.project":"` + f.networkProject + `","com.docker.compose.network":"browser-control"},"Containers":{}}]`), nil
	}
	if strings.Contains(a, "network create") {
		f.networkMissing = false
	}
	if strings.Contains(a, "inspect --format") {
		return []byte(`{"Running":true,"Health":{"Status":"healthy"}}`), nil
	}
	if strings.Contains(a, "inspect abcdef012345") {
		state := "exited"
		health := "unhealthy"
		run := "false"
		if f.running {
			state = "running"
			health = "healthy"
			run = "true"
		}
		return []byte(`[{"Id":"abcdef012345","Image":"sha256:fixed","RestartCount":0,"State":{"Running":` + run + `,"Status":"` + state + `","Health":{"Status":"` + health + `"}}}]`), nil
	}
	if strings.Contains(a, " up ") {
		if f.failInstall {
			return nil, errors.New("registry unavailable")
		}
		f.installed = true
		f.running = true
	}
	if strings.Contains(a, " stop ") {
		f.running = false
	}
	if strings.Contains(a, " start ") || strings.Contains(a, " restart ") {
		f.running = true
	}
	if strings.Contains(a, " rm ") {
		f.installed = false
		f.running = false
	}
	return []byte("ok"), nil
}
func fixture(t *testing.T) (*Service, *fakeRunner) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("private Agent state requires root")
	}
	dir := t.TempDir()
	f := &fakeRunner{runtimeParent: dir, project: "kejilion-panel", networkProject: "kejilion-panel"}
	return New(Config{StateDir: filepath.Join(dir, "state"), RuntimeDir: filepath.Join(dir, "runtime"), Executable: "/usr/bin/kejilion-agent", Runner: f}), f
}

func TestInstallCreatesMissingControlNetwork(t *testing.T) {
	s, f := fixture(t)
	f.networkMissing = true
	ctx := context.Background()
	v, err := s.Snapshot(ctx)
	if err != nil || !v.Supported {
		t.Fatal(v, err)
	}
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	calls := strings.Join(f.calls, "\n")
	if !strings.Contains(calls, "network create --internal") || !strings.Contains(calls, "network connect "+controlNetwork+" panel-id") {
		t.Fatal(calls)
	}
}

func TestInstallSupportsPanelComposeProjects(t *testing.T) {
	for _, tc := range []struct {
		project string
		missing bool
	}{{"kpanel", false}, {"kpanel", true}, {"kejilion-panel", true}, {"custom-panel_1", true}} {
		t.Run(fmt.Sprintf("%s-missing-%t", tc.project, tc.missing), func(t *testing.T) {
			s, f := fixture(t)
			f.project, f.networkProject = tc.project, tc.project
			f.networkMissing = tc.missing
			ctx := context.Background()
			v, err := s.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.RunJob(ctx, j.ID); err != nil {
				t.Fatal(err)
			}
			if tc.missing && !strings.Contains(strings.Join(f.calls, "\n"), "com.docker.compose.project="+tc.project) {
				t.Fatal(f.calls)
			}
		})
	}
}

func TestInstallRejectsMismatchedNetworkProject(t *testing.T) {
	s, f := fixture(t)
	f.project, f.networkProject = "kpanel", "other"
	err := s.prepareNetwork(context.Background())
	if err == nil || err.Error() != "browser_network_invalid" {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(f.calls, "\n"), "network connect") {
		t.Fatal(f.calls)
	}
}

func TestInstallRejectsInvalidComposeProject(t *testing.T) {
	for _, project := range []string{"", "-panel", "KPanel", "panel;echo"} {
		s, f := fixture(t)
		f.project = project
		if err := s.prepareNetwork(context.Background()); err == nil || err.Error() != "browser_panel_unsupported" {
			t.Fatal(project, err)
		}
		if strings.Contains(strings.Join(f.calls, "\n"), "network create") {
			t.Fatal(f.calls)
		}
	}
}

func TestInstallRejectsUnsafeControlNetwork(t *testing.T) {
	s, f := fixture(t)
	f.unsafeNetwork = true
	ctx := context.Background()
	v, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(ctx, j.ID); err == nil || err.Error() != "browser_network_invalid" {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(f.calls, "\n"), "network connect") {
		t.Fatal("connected to unsafe network")
	}
}

func TestReconcileRestoresRecreatedPanelOnlyWhileEnabled(t *testing.T) {
	s, f := fixture(t)
	if err := s.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	if err := s.enableDefaultConnection(); err != nil {
		t.Fatal(err)
	}
	f.installed = true
	if err := s.reconcileNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "network connect "+controlNetwork+" panel-id") {
		t.Fatal(f.calls)
	}
	f.calls = nil
	s.config.DockerProbe = func(context.Context) error { return errors.New("daemon stopped") }
	if err := s.reconcileNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatal("reconciliation attempted to activate Docker", f.calls)
	}
	if err := os.Remove(filepath.Join(s.config.StateDir, "enabled")); err != nil {
		t.Fatal(err)
	}
	s.config.DockerProbe = nil
	if err := s.reconcileNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatal("reconciliation continued after uninstall", f.calls)
	}
}
func TestLifecyclePersistsAndRecoversAcrossServiceInstances(t *testing.T) {
	s, f := fixture(t)
	ctx := context.Background()
	for _, action := range []string{"install", "stop", "start", "restart", "uninstall", "install"} {
		v, err := s.Snapshot(ctx)
		if err != nil || !v.Supported {
			t.Fatal(v, err)
		}
		j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: action, ExpectedResourceVersion: v.ResourceVersion})
		if err != nil {
			t.Fatal(action, err)
		}
		if _, err = s.Start(ctx, contract.BrowserComponentRequest{Action: action, ExpectedResourceVersion: v.ResourceVersion}); !errors.Is(err, ErrBusy) {
			t.Fatal("overlap accepted", err)
		}
		worker := New(s.config)
		if err = worker.RunJob(ctx, j.ID); err != nil {
			t.Fatal(action, err)
		}
		v, err = New(s.config).Snapshot(ctx)
		if err != nil || v.Job == nil || v.Job.State != "succeeded" {
			t.Fatal(v, err)
		}
		if action == "uninstall" && v.Installed {
			t.Fatal("container remains installed")
		}
		marker := filepath.Join(s.config.RuntimeDir, "browser-enabled")
		if action == "install" {
			if data, err := os.ReadFile(marker); err != nil || string(data) != "enabled" {
				t.Fatal("missing enabled marker", err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			if err := s.PrepareRuntime(); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "enabled" {
				t.Fatal("marker not restored", err)
			}
		}
		if action == "uninstall" {
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("marker remains after uninstall", err)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(s.config.StateDir, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), componentImage) || !strings.Contains(string(data), s.config.RuntimeDir) || strings.Contains(string(data), "ports:") {
		t.Fatal("incorrect or exposed component bundle")
	}
	if len(f.calls) == 0 {
		t.Fatal("no lifecycle commands")
	}
}

func TestStatusUsesExistingDaemonPolicyBeforeAnyDockerCommand(t *testing.T) {
	s, f := fixture(t)
	s.config.DockerProbe = func(context.Context) error { return errors.New("Docker daemon is stopped") }
	v, err := s.Snapshot(context.Background())
	if err != nil || v.Supported || v.Reason != "docker_unavailable" {
		t.Fatal(v, err)
	}
	if len(f.calls) != 0 {
		t.Fatal("status probe attempted to activate Docker", f.calls)
	}
}
func TestRejectsStaleTargetAndRetainsInstallFailure(t *testing.T) {
	s, f := fixture(t)
	ctx := context.Background()
	v, _ := s.Snapshot(ctx)
	if _, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: "stale"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	f.installed = true
	if err = s.RunJob(ctx, j.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("replaced resource was modified", err)
	}
	f.installed = false
	f.failInstall = true
	v, _ = s.Snapshot(ctx)
	j, err = s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(ctx, j.ID); err == nil {
		t.Fatal("installation failure lost")
	}
	v, _ = s.Snapshot(ctx)
	if v.Job == nil || v.Job.State != "failed" || !strings.Contains(v.Job.Message, "registry unavailable") {
		t.Fatal(v)
	}
}
func TestCredentialRepublicationAndUnsafeFiles(t *testing.T) {
	s, _ := fixture(t)
	previousMask := syscall.Umask(0077)
	defer syscall.Umask(previousMask)
	if err := s.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	token, _ := os.ReadFile(filepath.Join(s.config.StateDir, "token"))
	if len(token) != 64 {
		t.Fatal("invalid credential")
	}
	if err := s.PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	copy, _ := os.ReadFile(filepath.Join(s.config.RuntimeDir, "browser-token"))
	if string(copy) != string(token) {
		t.Fatal("credential rotated")
	}
	directory, _ := os.Stat(s.config.RuntimeDir)
	runtimeToken, _ := os.Stat(filepath.Join(s.config.RuntimeDir, "browser-token"))
	if directory.Mode().Perm() != 0755 || runtimeToken.Mode().Perm() != 0440 || runtimeToken.Sys().(*syscall.Stat_t).Gid != 65532 {
		t.Fatal("unprivileged runtime credential permissions are incorrect")
	}
	if err := os.Chmod(filepath.Join(s.config.StateDir, "token"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareRuntime(); err == nil {
		t.Fatal("unsafe private token accepted")
	}
}

func TestControlFileRetentionAndFailedCommandInspection(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	v, _ := s.Snapshot(ctx)
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	controlDir := filepath.Join(s.config.StateDir, ".job-control")
	if err = os.MkdirAll(controlDir, 0700); err != nil {
		t.Fatal(err)
	}
	pid := filepath.Join(controlDir, s.spec(j.ID).Unit+".pid")
	if err = os.WriteFile(pid, []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Snapshot(ctx)
	if _, err = s.Start(ctx, contract.BrowserComponentRequest{Action: "stop", ExpectedResourceVersion: v.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(pid); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old OpenRC control file retained", err)
	}
	output, err := (commandRunner{}).Run(ctx, "sh", "-c", "printf 'LoadState=not-found\\nActiveState=inactive\\n'; exit 4")
	if err == nil || !strings.Contains(string(output), "LoadState=not-found") {
		t.Fatal("failed systemctl-style output discarded", err)
	}
}
func TestInterruptedWorkerCanBeRetriedButLiveWorkerStaysBusy(t *testing.T) {
	s, f := fixture(t)
	ctx := context.Background()
	v, _ := s.Snapshot(ctx)
	j, err := s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	j.StartedAt = time.Now().Add(-time.Minute)
	if err = s.save(&j); err != nil {
		t.Fatal(err)
	}
	f.workerRunning = true
	v, _ = s.Snapshot(ctx)
	if v.Job.State != "queued" {
		t.Fatal("live worker marked interrupted")
	}
	f.workerRunning = false
	v, _ = s.Snapshot(ctx)
	if v.Job.State != "failed" || v.Job.Stage != "interrupted" {
		t.Fatal(v)
	}
	if _, err = s.Start(ctx, contract.BrowserComponentRequest{Action: "install", ExpectedResourceVersion: v.ResourceVersion}); err != nil {
		t.Fatal("retry blocked", err)
	}
	if err = s.RunJob(ctx, j.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("late obsolete worker accepted", err)
	}
}
