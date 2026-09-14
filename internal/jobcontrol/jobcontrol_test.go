package jobcontrol

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	paths map[string]string
	calls [][]string
	run   func(string, ...string) ([]byte, error)
}

func (runner *fakeRunner) LookPath(name string) (string, error) {
	if path := runner.paths[name]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeRunner) Run(_ context.Context, name string, arguments ...string) ([]byte, error) {
	runner.calls = append(runner.calls, append([]string{name}, arguments...))
	if runner.run != nil {
		return runner.run(name, arguments...)
	}
	return nil, nil
}

func testSpec(t *testing.T) Spec {
	t.Helper()
	root := t.TempDir()
	return Spec{
		Unit:        "kpanel-test-0123456789abcdef",
		Executable:  filepath.Join(root, "kejilion-agent"),
		Arguments:   []string{"maintenance-run", "--state-dir", root, "update"},
		StateDir:    root,
		UMask:       "0027",
		Nice:        10,
		IOClass:     "2:7",
		StopTimeout: 8 * time.Second,
	}
}

func TestDetectPrefersSystemdAndFallsBackToOpenRC(t *testing.T) {
	previous := initRuntimeDirectoryExists
	initRuntimeDirectoryExists = func(string) bool { return false }
	t.Cleanup(func() { initRuntimeDirectoryExists = previous })
	systemd := &fakeRunner{paths: map[string]string{
		"systemd-run":       filepath.Join(t.TempDir(), "systemd-run"),
		"start-stop-daemon": filepath.Join(t.TempDir(), "start-stop-daemon"),
		"rc-service":        filepath.Join(t.TempDir(), "rc-service"),
	}}
	if got, err := Detect(systemd); err != nil || got != BackendSystemd {
		t.Fatalf("Detect(systemd) = %q, %v", got, err)
	}
	openrc := &fakeRunner{paths: map[string]string{
		"start-stop-daemon": filepath.Join(t.TempDir(), "start-stop-daemon"),
		"rc-service":        filepath.Join(t.TempDir(), "rc-service"),
	}}
	if got, err := Detect(openrc); err != nil || got != BackendOpenRC {
		t.Fatalf("Detect(openrc) = %q, %v", got, err)
	}
}

func TestDetectFailsClosedForIncompleteLiveOpenRC(t *testing.T) {
	previous := initRuntimeDirectoryExists
	initRuntimeDirectoryExists = func(path string) bool { return path == "/run/openrc" }
	t.Cleanup(func() { initRuntimeDirectoryExists = previous })
	runner := &fakeRunner{paths: map[string]string{
		"systemd-run": filepath.Join(t.TempDir(), "systemd-run"),
	}}
	if backend, err := Detect(runner); err == nil || backend != "" || !strings.Contains(err.Error(), "OpenRC is running") {
		t.Fatalf("Detect(incomplete live OpenRC) = %q, %v", backend, err)
	}
}

func TestDetectUsesLiveOpenRCRuntimeBeforeStraySystemdTool(t *testing.T) {
	previous := initRuntimeDirectoryExists
	initRuntimeDirectoryExists = func(path string) bool { return path == "/run/openrc" }
	t.Cleanup(func() { initRuntimeDirectoryExists = previous })
	runner := &fakeRunner{paths: map[string]string{
		"systemd-run":       filepath.Join(t.TempDir(), "systemd-run"),
		"start-stop-daemon": filepath.Join(t.TempDir(), "start-stop-daemon"),
		"rc-service":        filepath.Join(t.TempDir(), "rc-service"),
	}}
	if got, err := Detect(runner); err != nil || got != BackendOpenRC {
		t.Fatalf("Detect(live OpenRC) = %q, %v", got, err)
	}
}

func TestLaunchSystemdPreservesPropertiesAndFixedWorker(t *testing.T) {
	runner := &fakeRunner{paths: map[string]string{
		"systemd-run": filepath.Join(t.TempDir(), "systemd-run"),
	}}
	spec := testSpec(t)
	spec.SystemdProperties = []string{"Type=oneshot", "TimeoutStartSec=45min"}
	spec.Environment = []string{"LC_ALL=C", "KPANEL_TEST=ready"}
	if err := Launch(context.Background(), runner, spec); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[0]
	for _, expected := range []string{
		"systemd-run", "--unit=" + spec.Unit, "--collect", "--no-block",
		"--property=Type=oneshot", "--property=TimeoutStartSec=45min", "--",
		"--setenv=LC_ALL=C", "--setenv=KPANEL_TEST=ready",
		spec.Executable, "maintenance-run", "update",
	} {
		if !slices.Contains(call, expected) {
			t.Fatalf("systemd call missing %q: %#v", expected, call)
		}
	}
}

func TestRunForegroundUsesDirectWorkerOnOpenRC(t *testing.T) {
	previous := initRuntimeDirectoryExists
	initRuntimeDirectoryExists = func(path string) bool { return path == "/run/openrc" }
	t.Cleanup(func() { initRuntimeDirectoryExists = previous })
	runner := &fakeRunner{paths: map[string]string{
		"start-stop-daemon": filepath.Join(t.TempDir(), "start-stop-daemon"),
		"rc-service":        filepath.Join(t.TempDir(), "rc-service"),
	}}
	spec := testSpec(t)
	runner.run = func(name string, arguments ...string) ([]byte, error) {
		if name != spec.Executable || !slices.Equal(arguments, spec.Arguments) {
			t.Fatalf("foreground call = %q %#v", name, arguments)
		}
		return []byte("result"), nil
	}
	output, err := RunForeground(context.Background(), runner, spec)
	if err != nil || string(output) != "result" {
		t.Fatalf("RunForeground() = %q, %v", output, err)
	}
}

func TestLaunchInspectAndStopOpenRCUsesDedicatedProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenRC pidfile permissions require Unix")
	}
	previous := initRuntimeDirectoryExists
	initRuntimeDirectoryExists = func(path string) bool { return path == "/run/openrc" }
	t.Cleanup(func() { initRuntimeDirectoryExists = previous })
	startStopDaemon := filepath.Join(t.TempDir(), "start-stop-daemon")
	runner := &fakeRunner{paths: map[string]string{
		"start-stop-daemon": startStopDaemon,
		"rc-service":        filepath.Join(t.TempDir(), "rc-service"),
	}}
	spec := testSpec(t)
	active := false
	runner.run = func(_ string, arguments ...string) ([]byte, error) {
		if slices.Contains(arguments, "--test") {
			if active {
				return nil, nil
			}
			return nil, errors.New("not running")
		}
		if slices.Contains(arguments, "--start") {
			active = true
			pidPath := argumentAfter(arguments, "--pidfile")
			if err := os.WriteFile(pidPath, []byte("123\n"), 0o600); err != nil {
				return nil, err
			}
			return nil, nil
		}
		if slices.Contains(arguments, "--stop") {
			active = false
			return nil, nil
		}
		return nil, errors.New("unexpected invocation")
	}
	if err := Launch(context.Background(), runner, spec); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), runner, spec)
	if err != nil || !state.Known || !state.Running || state.Backend != BackendOpenRC {
		t.Fatalf("Inspect() = %#v, %v", state, err)
	}
	if err := Stop(context.Background(), runner, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(spec.StateDir, controlDirectoryName, spec.Unit+".pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pid file remained after stop: %v", err)
	}
	joined := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	for _, expected := range []string{"--background", "--make-pidfile", "--stop-group", "TERM/8/KILL/5"} {
		if !strings.Contains(all, expected) {
			t.Fatalf("OpenRC calls missing %q:\n%s", expected, all)
		}
	}
	if !strings.Contains(all, "--stdout "+os.DevNull) || !strings.Contains(all, "--stderr "+os.DevNull) {
		t.Fatalf("OpenRC background output must not create unbounded control logs:\n%s", all)
	}
	if strings.Contains(all, "--stop --stop-group --retry TERM/8/KILL/5 --pidfile "+
		filepath.Join(spec.StateDir, controlDirectoryName, spec.Unit+".pid")+" --exec") {
		t.Fatalf("OpenRC stop should trust the root-owned pidfile across binary replacement:\n%s", all)
	}
}

func TestSpecRejectsUnsafeNamesAndPaths(t *testing.T) {
	spec := testSpec(t)
	for _, mutate := range []func(*Spec){
		func(value *Spec) { value.Unit = "bad/unit" },
		func(value *Spec) { value.StateDir = "." },
		func(value *Spec) { value.Executable = "kejilion-agent" },
		func(value *Spec) { value.Arguments = []string{"ok", "bad\nargument"} },
		func(value *Spec) { value.Environment = []string{"BAD-NAME=value"} },
	} {
		candidate := spec
		mutate(&candidate)
		if err := validateSpec(candidate); err == nil {
			t.Fatalf("unsafe spec was accepted: %#v", candidate)
		}
	}
}

func argumentAfter(arguments []string, name string) string {
	for index := range arguments {
		if arguments[index] == name && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}
