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

const lightNodeProcdCronLine = "17 * * * * /usr/local/lib/kejilion-node/update-cron.sh # KPanel lightweight node updater"
const lightNodeProcdCronWrapper = "/usr/local/lib/kejilion-node/update-cron.sh"
const nodeProcdHealthPath = "/run/kejilion-node-monitoring/procd-health.json"

// Reuse the existing privileged file broker and trusted runtime relay. Giving
// the telemetry user unrestricted service-bus access would expose unrelated
// service data; the relay contains only our five fixed status fields.
func startNodeProcdHealth(parent context.Context) func() {
	if !liveProcdRuntime("/") {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			query, stop := context.WithTimeout(ctx, 2*time.Second)
			units := collectProcdHealth(query, "/")
			stop()
			snapshot := contract.LightNodeHealth{ObservedAt: time.Now().UTC(), RuntimeVersion: version.Version,
				Update:   contract.LightNodeUpdateHealth{State: "missing"},
				Services: contract.LightNodeServicesHealth{Timer: units[0], Telemetry: units[1], Terminal: units[2], File: units[3], SSHLogin: units[4]}}
			// A failed publication leaves the previous bounded observation, which
			// readers expire instead of presenting it as a current status.
			_ = publishProcdHealthSnapshot(snapshot)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

func decodeProcdHealthSnapshot(content []byte, now time.Time) *contract.LightNodeHealth {
	if len(content) > 4096 {
		return nil
	}
	var snapshot contract.LightNodeHealth
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&snapshot) != nil || !contract.ValidLightNodeHealth(snapshot, now) ||
		snapshot.ObservedAt.Before(now.Add(-time.Minute)) || snapshot.ObservedAt.After(now.Add(5*time.Second)) {
		return nil
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return nil
	}
	return &snapshot
}

// Detect the running service manager, not a distribution name or an installed
// compatibility command. The init API files are part of the trusted OS image.
func liveProcdRuntime(root string) bool {
	file, err := os.Open(healthRootPath(root, "/proc/1/comm"))
	if err != nil {
		return false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 65))
	return err == nil && len(content) <= 64 && strings.TrimSpace(string(content)) == "procd" &&
		secureMigrationFile(healthRootPath(root, "/etc/rc.common")) &&
		secureMigrationFile(healthRootPath(root, "/lib/functions/procd.sh"))
}

func collectProcdHealth(ctx context.Context, root string) []contract.LightNodeServiceHealth {
	active := make(map[string]*bool, len(healthOpenRCServices)+1)
	ubus, err := migrationTrustedExecutable("ubus", "/bin/ubus", "/sbin/ubus", "/usr/bin/ubus", "/usr/sbin/ubus")
	if err == nil {
		for _, service := range append(append([]string(nil), healthOpenRCServices...), "cron") {
			query, _ := json.Marshal(map[string]string{"name": service})
			command := exec.CommandContext(ctx, ubus, "-t", "1", "call", "service", "list", string(query))
			output := &healthOutput{}
			command.Stdout, command.Stderr = output, io.Discard
			command.WaitDelay = 100 * time.Millisecond
			if command.Run() == nil {
				active[service] = parseProcdRunning(output.Bytes(), service)
			}
		}
	}
	// Unprivileged ubus callers can be denied by the OS. Preserve unknown in
	// that case; never turn a failed observation into a stopped service.
	return procdServiceHealth(root, active)
}

func parseProcdRunning(content []byte, name string) *bool {
	var services map[string]struct {
		Instances map[string]struct {
			Running *bool `json:"running"`
			PID     int   `json:"pid"`
		} `json:"instances"`
	}
	if len(content) > 4096 || json.Unmarshal(content, &services) != nil || services == nil {
		return nil
	}
	service, exists := services[name]
	active := false
	if !exists {
		return &active
	}
	if service.Instances == nil || len(service.Instances) > 16 {
		return nil
	}
	for instance, state := range service.Instances {
		if name != "cron" && instance != "main" {
			continue
		}
		if state.Running == nil || (*state.Running && state.PID <= 1) {
			return nil
		}
		active = active || *state.Running
	}
	return &active
}

func procdServiceHealth(root string, active map[string]*bool) []contract.LightNodeServiceHealth {
	units := make([]contract.LightNodeServiceHealth, len(healthUnits))
	for index, service := range healthOpenRCServices {
		units[index+1] = procdUnitHealth(root, service, active[service])
	}
	timer := unknownServiceHealth()
	installed := executableRegularFile(healthRootPath(root, lightNodeProcdCronWrapper))
	timer.LoadState = healthChoice(installed, "loaded", "not-found")
	if !installed {
		timer.ActiveState, timer.SubState, timer.UnitFileState = "inactive", "dead", "disabled"
	} else if content, err := readBoundedMigrationFile(healthRootPath(root, "/etc/crontabs/root"), 64<<10); err == nil {
		scheduled := false
		for _, line := range strings.Split(string(content), "\n") {
			scheduled = scheduled || line == lightNodeProcdCronLine
		}
		timer.UnitFileState = healthChoice(scheduled && procdServiceEnabled(root, "cron"), "enabled", "disabled")
		if !scheduled || active["cron"] != nil {
			running := scheduled && active["cron"] != nil && *active["cron"]
			timer.ActiveState = healthChoice(running, "active", "inactive")
			timer.SubState = healthChoice(running, "waiting", "dead")
		}
	} else if _, statErr := os.Lstat(healthRootPath(root, "/etc/crontabs/root")); errors.Is(statErr, os.ErrNotExist) {
		timer.ActiveState, timer.SubState, timer.UnitFileState = "inactive", "dead", "disabled"
	}
	units[0] = timer
	return units
}

func procdUnitHealth(root, name string, active *bool) contract.LightNodeServiceHealth {
	state := unknownServiceHealth()
	installed := executableRegularFile(healthRootPath(root, "/etc/init.d/"+name))
	state.LoadState = healthChoice(installed, "loaded", "not-found")
	state.UnitFileState = healthChoice(installed && procdServiceEnabled(root, name), "enabled", "disabled")
	if !installed || active != nil {
		running := installed && active != nil && *active
		state.ActiveState = healthChoice(running, "active", "inactive")
		state.SubState = healthChoice(running, "running", "dead")
	}
	return state
}

func procdServiceEnabled(root, name string) bool {
	entries, err := os.ReadDir(healthRootPath(root, "/etc/rc.d"))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		filename := entry.Name()
		if len(filename) != len(name)+3 || filename[0] != 'S' || filename[1] < '0' || filename[1] > '9' || filename[2] < '0' || filename[2] > '9' || filename[3:] != name {
			continue
		}
		path, err := filepath.EvalSymlinks(healthRootPath(root, "/etc/rc.d/"+filename))
		if err == nil && path == healthRootPath(root, "/etc/init.d/"+name) {
			return true
		}
	}
	return false
}

// OpenWrt commonly keeps /var in RAM. Keep this broker's bounded state beside
// its persistent configuration; neither branding nor a user-supplied path is used.
func nodeStateDirectory(root string) (string, error) {
	if path := platformStateDirectory(); path != "" {
		return path, nil
	}
	if !liveProcdRuntime(root) {
		return healthRootPath(root, "/var/lib/kejilion-node"), nil
	}
	for _, parent := range []string{"/etc", "/etc/kejilion-node"} {
		if !secureMigrationDirectory(healthRootPath(root, parent)) {
			return "", errors.New("unsafe lightweight node state parent")
		}
	}
	path := healthRootPath(root, "/etc/kejilion-node/state")
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if !secureMigrationDirectory(path) {
		return "", errors.New("unsafe lightweight node state directory")
	}
	return path, os.Chmod(path, 0o700)
}
