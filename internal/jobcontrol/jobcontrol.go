package jobcontrol

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const controlDirectoryName = ".job-control"

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,159}$`)
var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var ioClassPattern = regexp.MustCompile(`^[0-3](?::[0-7])?$`)
var initRuntimeDirectoryExists = directoryExists

type Backend string

const (
	BackendSystemd Backend = "systemd"
	BackendOpenRC  Backend = "openrc"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type initRuntimeDirectoryProbe interface {
	InitRuntimeDirectoryExists(string) bool
}

type Spec struct {
	Unit              string
	Executable        string
	Arguments         []string
	Environment       []string
	StateDir          string
	SystemdProperties []string
	UMask             string
	Nice              int
	IOClass           string
	NoNewPrivileges   bool
	StopTimeout       time.Duration
}

type State struct {
	Backend    Backend
	Known      bool
	Running    bool
	Successful bool
	Detail     string
}

func RunForeground(ctx context.Context, runner Runner, spec Spec) ([]byte, error) {
	name, arguments, backend, err := ForegroundInvocation(runner, spec)
	if err != nil {
		return nil, err
	}
	output, runErr := runner.Run(ctx, name, arguments...)
	if backend == BackendOpenRC {
		return output, commandError("run OpenRC foreground task", output, runErr)
	}
	return output, commandError("run systemd foreground task", output, runErr)
}

func ForegroundInvocation(runner Runner, spec Spec) (string, []string, Backend, error) {
	if err := validateSpec(spec); err != nil {
		return "", nil, "", err
	}
	backend, err := Detect(runner)
	if err != nil {
		return "", nil, "", err
	}
	if backend == BackendOpenRC {
		return spec.Executable, append([]string(nil), spec.Arguments...), backend, nil
	}
	arguments := []string{"--unit=" + spec.Unit, "--collect", "--pipe", "--wait", "--quiet"}
	for _, property := range spec.SystemdProperties {
		arguments = append(arguments, "--property="+property)
	}
	for _, environment := range spec.Environment {
		arguments = append(arguments, "--setenv="+environment)
	}
	arguments = append(arguments, "--", spec.Executable)
	arguments = append(arguments, spec.Arguments...)
	return "systemd-run", arguments, backend, nil
}

func Detect(runner Runner) (Backend, error) {
	if runner == nil {
		return "", errors.New("background task runner is unavailable")
	}
	_, systemdErr := runner.LookPath("systemd-run")
	_, startStopErr := runner.LookPath("start-stop-daemon")
	_, rcServiceErr := runner.LookPath("rc-service")
	systemdAvailable := systemdErr == nil
	openRCAvailable := startStopErr == nil && rcServiceErr == nil
	if runtimeDirectoryExists(runner, "/run/systemd/system") {
		if systemdAvailable {
			return BackendSystemd, nil
		}
		return "", errors.New("systemd is running but systemd-run is unavailable")
	}
	if runtimeDirectoryExists(runner, "/run/openrc") {
		if openRCAvailable {
			return BackendOpenRC, nil
		}
		return "", errors.New("OpenRC is running but start-stop-daemon or rc-service is unavailable")
	}
	// Test and build containers may expose the tools without running an init
	// manager. Preserve deterministic invocation construction when only one
	// complete backend is present. A live but incomplete manager above always
	// fails closed instead of falling through to tools from another backend.
	if systemdAvailable {
		return BackendSystemd, nil
	}
	if openRCAvailable {
		return BackendOpenRC, nil
	}
	return "", errors.New("neither systemd nor OpenRC background task control is available")
}

func runtimeDirectoryExists(runner Runner, path string) bool {
	if probe, ok := runner.(initRuntimeDirectoryProbe); ok {
		return probe.InitRuntimeDirectoryExists(path)
	}
	return initRuntimeDirectoryExists(path)
}

func Available(runner Runner) error {
	_, err := Detect(runner)
	return err
}

func Launch(ctx context.Context, runner Runner, spec Spec) error {
	if err := validateSpec(spec); err != nil {
		return err
	}
	backend, err := Detect(runner)
	if err != nil {
		return err
	}
	if backend == BackendSystemd {
		arguments := []string{"--unit=" + spec.Unit, "--collect", "--no-block"}
		for _, property := range spec.SystemdProperties {
			arguments = append(arguments, "--property="+property)
		}
		for _, environment := range spec.Environment {
			arguments = append(arguments, "--setenv="+environment)
		}
		arguments = append(arguments, "--", spec.Executable)
		arguments = append(arguments, spec.Arguments...)
		output, runErr := runner.Run(ctx, "systemd-run", arguments...)
		return commandError("launch systemd background task", output, runErr)
	}

	paths, err := prepareControlPaths(spec)
	if err != nil {
		return err
	}
	current, inspectErr := inspectOpenRC(ctx, runner, spec, paths)
	if inspectErr != nil {
		return fmt.Errorf("inspect existing OpenRC background task: %w", inspectErr)
	}
	if current.Running {
		return fmt.Errorf("background task %s is already running", spec.Unit)
	}
	if err := os.Remove(paths.pid); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale OpenRC pid file: %w", err)
	}
	startStopDaemon, err := runner.LookPath("start-stop-daemon")
	if err != nil {
		return errors.New("OpenRC start-stop-daemon is unavailable")
	}
	arguments := []string{
		"--start",
		"--background",
		"--make-pidfile",
		"--pidfile", paths.pid,
		"--exec", spec.Executable,
		"--chdir", "/",
		"--umask", normalizedUMask(spec.UMask),
		"--wait", "100",
		"--stdout", os.DevNull,
		"--stderr", os.DevNull,
	}
	if spec.Nice != 0 {
		arguments = append(arguments, "--nicelevel", strconv.Itoa(spec.Nice))
	}
	if spec.IOClass != "" {
		arguments = append(arguments, "--ionice", spec.IOClass)
	}
	if spec.NoNewPrivileges {
		arguments = append(arguments, "--no-new-privs")
	}
	for _, environment := range spec.Environment {
		arguments = append(arguments, "--env", environment)
	}
	arguments = append(arguments, "--")
	arguments = append(arguments, spec.Arguments...)
	output, runErr := runner.Run(ctx, startStopDaemon, arguments...)
	return commandError("launch OpenRC background task", output, runErr)
}

func Inspect(ctx context.Context, runner Runner, spec Spec) (State, error) {
	if err := validateSpec(spec); err != nil {
		return State{}, err
	}
	backend, err := Detect(runner)
	if err != nil {
		return State{}, err
	}
	if backend == BackendOpenRC {
		paths, pathErr := controlPaths(spec)
		if pathErr != nil {
			return State{}, pathErr
		}
		return inspectOpenRC(ctx, runner, spec, paths)
	}

	output, runErr := runner.Run(
		ctx,
		"systemctl",
		"show",
		spec.Unit,
		"--property=LoadState",
		"--property=ActiveState",
		"--property=SubState",
		"--property=Result",
		"--property=ExecMainStatus",
		"--no-pager",
	)
	values := parseProperties(output)
	active := strings.TrimSpace(values["ActiveState"])
	if active == "" {
		plain := strings.TrimSpace(string(output))
		switch plain {
		case "active", "activating", "reloading", "deactivating", "inactive", "failed", "dead":
			active = plain
			values["ActiveState"] = plain
		}
	}
	state := State{
		Backend: BackendSystemd,
		Known:   active != "" || strings.TrimSpace(values["LoadState"]) != "",
		Running: active == "active" || active == "activating" || active == "reloading",
		Successful: strings.EqualFold(strings.TrimSpace(values["Result"]), "success") &&
			strings.TrimSpace(values["ExecMainStatus"]) == "0",
		Detail: propertyDetail(values),
	}
	if runErr != nil && !state.Known {
		return state, commandError("inspect systemd background task", output, runErr)
	}
	return state, nil
}

func Stop(ctx context.Context, runner Runner, spec Spec) error {
	if err := validateSpec(spec); err != nil {
		return err
	}
	backend, err := Detect(runner)
	if err != nil {
		return err
	}
	if backend == BackendSystemd {
		output, runErr := runner.Run(ctx, "systemctl", "stop", "--no-block", spec.Unit)
		return commandError("stop systemd background task", output, runErr)
	}

	paths, err := controlPaths(spec)
	if err != nil {
		return err
	}
	state, inspectErr := inspectOpenRC(ctx, runner, spec, paths)
	if inspectErr != nil {
		return fmt.Errorf("inspect OpenRC background task before stop: %w", inspectErr)
	}
	if !state.Running {
		return removePIDFile(paths.pid)
	}
	startStopDaemon, err := runner.LookPath("start-stop-daemon")
	if err != nil {
		return errors.New("OpenRC start-stop-daemon is unavailable")
	}
	timeout := int(spec.StopTimeout.Round(time.Second) / time.Second)
	if timeout < 1 {
		timeout = 10
	}
	if timeout > 600 {
		timeout = 600
	}
	arguments := []string{
		"--stop",
		"--stop-group",
		"--retry", fmt.Sprintf("TERM/%d/KILL/5", timeout),
		"--pidfile", paths.pid,
	}
	output, runErr := runner.Run(ctx, startStopDaemon, arguments...)
	if err := commandError("stop OpenRC background task", output, runErr); err != nil {
		return err
	}
	return removePIDFile(paths.pid)
}

func Cleanup(spec Spec) error {
	paths, err := controlPaths(spec)
	if err != nil {
		return err
	}
	return removePIDFile(paths.pid)
}

type controlPathSet struct {
	directory string
	pid       string
}

func prepareControlPaths(spec Spec) (controlPathSet, error) {
	paths, err := controlPaths(spec)
	if err != nil {
		return controlPathSet{}, err
	}
	if err := os.MkdirAll(paths.directory, 0o750); err != nil {
		return controlPathSet{}, fmt.Errorf("create background task control directory: %w", err)
	}
	info, err := os.Lstat(paths.directory)
	if err != nil {
		return controlPathSet{}, fmt.Errorf("inspect background task control directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return controlPathSet{}, errors.New("background task control path is not a trusted directory")
	}
	if !trustedOwner(info) {
		return controlPathSet{}, errors.New("background task control directory ownership or mode is unsafe")
	}
	if err := os.Chmod(paths.directory, 0o750); err != nil {
		return controlPathSet{}, fmt.Errorf("secure background task control directory: %w", err)
	}
	info, err = os.Lstat(paths.directory)
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		return controlPathSet{}, errors.New("background task control directory ownership or mode is unsafe")
	}
	return paths, nil
}

func controlPaths(spec Spec) (controlPathSet, error) {
	if err := validateSpec(spec); err != nil {
		return controlPathSet{}, err
	}
	directory := filepath.Join(spec.StateDir, controlDirectoryName)
	return controlPathSet{
		directory: directory,
		pid:       filepath.Join(directory, spec.Unit+".pid"),
	}, nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func inspectOpenRC(
	ctx context.Context,
	runner Runner,
	spec Spec,
	paths controlPathSet,
) (State, error) {
	state := State{Backend: BackendOpenRC}
	if _, err := os.Lstat(paths.pid); errors.Is(err, os.ErrNotExist) {
		state.Known = true
		state.Detail = "pidfile=absent"
		return state, nil
	} else if err != nil {
		return state, fmt.Errorf("inspect OpenRC pid file: %w", err)
	}
	info, err := os.Lstat(paths.pid)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0o022 != 0 || !trustedOwner(info) {
		return state, errors.New("OpenRC pid file ownership or mode is unsafe")
	}
	startStopDaemon, err := runner.LookPath("start-stop-daemon")
	if err != nil {
		return state, errors.New("OpenRC start-stop-daemon is unavailable")
	}
	output, runErr := runner.Run(
		ctx,
		startStopDaemon,
		"--stop",
		"--test",
		"--quiet",
		"--pidfile", paths.pid,
	)
	if runErr == nil {
		state.Known = true
		state.Running = true
		state.Detail = "pidfile=active"
		return state, nil
	}
	if ctx.Err() != nil {
		return state, ctx.Err()
	}
	state.Known = true
	state.Detail = "pidfile=stale"
	if detail := strings.TrimSpace(string(output)); detail != "" {
		state.Detail += " / " + truncate(detail, 300)
	}
	return state, nil
}

func validateSpec(spec Spec) error {
	if !unitPattern.MatchString(spec.Unit) {
		return errors.New("background task unit name is invalid")
	}
	if !canonicalDedicatedPath(spec.StateDir) {
		return errors.New("background task state directory must be a dedicated absolute path")
	}
	if !canonicalExecutable(spec.Executable) {
		return errors.New("background task executable must be a canonical absolute path")
	}
	for _, value := range append(append(append([]string{}, spec.Arguments...), spec.SystemdProperties...), spec.Environment...) {
		if strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, "\r\n") {
			return errors.New("background task argument contains an unsafe control character")
		}
	}
	for _, environment := range spec.Environment {
		name, _, ok := strings.Cut(environment, "=")
		if !ok || !environmentNamePattern.MatchString(name) {
			return errors.New("background task environment is invalid")
		}
	}
	if spec.UMask != "" {
		if _, err := strconv.ParseUint(spec.UMask, 8, 12); err != nil || len(spec.UMask) > 4 {
			return errors.New("background task umask is invalid")
		}
	}
	if spec.IOClass != "" && !ioClassPattern.MatchString(spec.IOClass) {
		return errors.New("background task ionice class is invalid")
	}
	return nil
}

func canonicalDedicatedPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return false
	}
	volume := filepath.VolumeName(value)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	return value != root
}

func canonicalExecutable(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func normalizedUMask(value string) string {
	if value == "" {
		return "0027"
	}
	return value
}

func parseProperties(output []byte) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	return values
}

func propertyDetail(values map[string]string) string {
	parts := make([]string, 0, 5)
	for _, key := range []string{"LoadState", "ActiveState", "SubState", "Result", "ExecMainStatus"} {
		if value := strings.TrimSpace(values[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " / ")
}

func commandError(action string, output []byte, err error) error {
	if err == nil {
		return nil
	}
	detail := truncate(strings.TrimSpace(string(output)), 300)
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %s: %w", action, detail, err)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func removePIDFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove OpenRC pid file: %w", err)
	}
	return nil
}
