package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/version"
)

const updateHealthPath = "/etc/kejilion-node/update-status.json"
const maxUpdateHealthBytes = 1024

var healthUnits = []string{"kejilion-node-update.timer", "kejilion-node.service", "kejilion-node-terminal.service", "kejilion-node-file.service", "kejilion-node-ssh-login.service"}

var healthOpenRCServices = []string{"kejilion-node", "kejilion-node-terminal", "kejilion-node-file", "kejilion-node-ssh-login"}

func unknownServiceHealth() contract.LightNodeServiceHealth {
	return contract.LightNodeServiceHealth{LoadState: "unknown", ActiveState: "unknown", SubState: "unknown", UnitFileState: "unknown"}
}

// One fixed command, one shared deadline, bounded stdout and discarded stderr.
// Failure never prevents the core report and never turns into not-installed.
func collectLightHealth(ctx context.Context) *contract.LightNodeHealth {
	now := time.Now().UTC()
	health := &contract.LightNodeHealth{ObservedAt: now, RuntimeVersion: version.Version, Update: readUpdateHealth(updateHealthPath, now)}
	units := make([]contract.LightNodeServiceHealth, len(healthUnits))
	for i := range units {
		units[i] = unknownServiceHealth()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// A live OpenRC marker is authoritative even when a stray systemctl
	// compatibility command is present on the host.
	if liveOpenRCRuntime("/") {
		if openRCHealthAvailable("/") {
			units = collectOpenRCHealth(ctx, "/")
		}
		health.Services = contract.LightNodeServicesHealth{Timer: units[0], Telemetry: units[1], Terminal: units[2], File: units[3], SSHLogin: units[4]}
		return health
	}
	args := append([]string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,SubState,UnitFileState"}, healthUnits...)
	command := exec.CommandContext(ctx, "systemctl", args...)
	output := &healthOutput{}
	command.Stdout = output
	command.Stderr = io.Discard
	command.WaitDelay = 100 * time.Millisecond
	if err := command.Run(); err == nil {
		units = parseServiceHealth(output.Bytes())
	}
	health.Services = contract.LightNodeServicesHealth{Timer: units[0], Telemetry: units[1], Terminal: units[2], File: units[3], SSHLogin: units[4]}
	return health
}

func openRCHealthAvailable(root string) bool {
	if !liveOpenRCRuntime(root) {
		return false
	}
	_, err := exec.LookPath("rc-service")
	return err == nil
}

func liveOpenRCRuntime(root string) bool {
	info, err := os.Stat(healthRootPath(root, "/run/openrc"))
	return err == nil && info.IsDir()
}

func collectOpenRCHealth(ctx context.Context, root string) []contract.LightNodeServiceHealth {
	active := make(map[string]bool, len(healthOpenRCServices)+1)
	for _, service := range append(append([]string(nil), healthOpenRCServices...), "crond") {
		command := exec.CommandContext(ctx, "rc-service", service, "status")
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		command.WaitDelay = 100 * time.Millisecond
		active[service] = command.Run() == nil
	}
	return openRCServiceHealth(root, active)
}

func openRCServiceHealth(root string, active map[string]bool) []contract.LightNodeServiceHealth {
	units := make([]contract.LightNodeServiceHealth, len(healthUnits))
	periodic := healthRootPath(root, "/etc/periodic/hourly/kejilion-node-update")
	periodicInstalled := executableRegularFile(periodic)
	cronEnabled := healthPathExists(healthRootPath(root, "/etc/runlevels/default/crond"))
	units[0] = contract.LightNodeServiceHealth{
		LoadState:     healthChoice(periodicInstalled, "loaded", "not-found"),
		ActiveState:   healthChoice(periodicInstalled && active["crond"], "active", "inactive"),
		SubState:      healthChoice(periodicInstalled && active["crond"], "waiting", "dead"),
		UnitFileState: healthChoice(periodicInstalled && cronEnabled, "enabled", "disabled"),
	}
	for index, service := range healthOpenRCServices {
		installed := executableRegularFile(healthRootPath(root, "/etc/init.d/"+service))
		enabled := healthPathExists(healthRootPath(root, "/etc/runlevels/default/"+service))
		units[index+1] = contract.LightNodeServiceHealth{
			LoadState:     healthChoice(installed, "loaded", "not-found"),
			ActiveState:   healthChoice(installed && active[service], "active", "inactive"),
			SubState:      healthChoice(installed && active[service], "running", "dead"),
			UnitFileState: healthChoice(installed && enabled, "enabled", "disabled"),
		}
	}
	return units
}

func healthRootPath(root, absolute string) string {
	if root == "/" {
		return absolute
	}
	return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(absolute, "/")))
}

func executableRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o111 != 0
}

func healthPathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func healthChoice(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

type healthOutput struct{ buffer bytes.Buffer }

func (b *healthOutput) Bytes() []byte { return b.buffer.Bytes() }
func (b *healthOutput) Len() int      { return b.buffer.Len() }

func (b *healthOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, errors.New("health output limit")
	}
	return b.buffer.Write(p)
}

func parseServiceHealth(content []byte) []contract.LightNodeServiceHealth {
	units := make([]contract.LightNodeServiceHealth, len(healthUnits))
	for i := range units {
		units[i] = unknownServiceHealth()
	}
	if len(content) > 4096 {
		return units
	}
	seen := map[string]bool{}
	for _, block := range strings.Split(strings.TrimSpace(string(content)), "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				fields[k] = v
			}
		}
		id := fields["Id"]
		for i, unit := range healthUnits {
			if unit != id {
				continue
			}
			if seen[id] {
				units[i] = unknownServiceHealth()
				continue
			}
			seen[id] = true
			candidate := contract.LightNodeServiceHealth{LoadState: fields["LoadState"], ActiveState: fields["ActiveState"], SubState: fields["SubState"], UnitFileState: fields["UnitFileState"]}
			// Normalize future/unsupported states independently, retaining known
			// missing-unit and disabled-timer information.
			known := unknownServiceHealth()
			if healthEnum(candidate.LoadState, "loaded", "not-found", "masked", "error", "bad-setting") {
				known.LoadState = candidate.LoadState
			}
			if healthEnum(candidate.ActiveState, "active", "inactive", "failed", "activating", "deactivating", "reloading") {
				known.ActiveState = candidate.ActiveState
			}
			if healthEnum(candidate.SubState, "running", "waiting", "dead", "failed", "exited", "start", "stop", "auto-restart", "elapsed") {
				known.SubState = candidate.SubState
			}
			if healthEnum(candidate.UnitFileState, "enabled", "enabled-runtime", "disabled", "masked", "masked-runtime", "static", "indirect", "generated", "transient", "alias", "linked", "linked-runtime") {
				known.UnitFileState = candidate.UnitFileState
			}
			units[i] = known
		}
	}
	return units
}

func healthEnum(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func readUpdateHealth(path string, now time.Time) contract.LightNodeUpdateHealth {
	content, err := readUpdateHealthFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return contract.LightNodeUpdateHealth{State: "missing"}
	}
	if errors.Is(err, os.ErrPermission) {
		return contract.LightNodeUpdateHealth{State: "unavailable"}
	}
	if err != nil {
		return contract.LightNodeUpdateHealth{State: "invalid"}
	}
	return parseUpdateHealth(content, now)
}

func parseUpdateHealth(content []byte, now time.Time) contract.LightNodeUpdateHealth {
	invalid := contract.LightNodeUpdateHealth{State: "invalid"}
	if len(content) > maxUpdateHealthBytes {
		return invalid
	}
	var value contract.LightNodeUpdateHealth
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || !contract.ValidLightNodeUpdateHealth(value, now) {
		return invalid
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return invalid
	}
	if value.State == "running" && now.Unix()-value.CheckedAt > 1800 {
		value.State = "interrupted"
		value.ErrorCode = "interrupted"
	}
	return value
}
