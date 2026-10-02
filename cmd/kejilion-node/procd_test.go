package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestProcdHealthRelayExpiresAndRejectsInvalidContent(t *testing.T) {
	now := time.Now().UTC()
	unknown := unknownServiceHealth()
	snapshot := contract.LightNodeHealth{ObservedAt: now, RuntimeVersion: "1.2.3", Update: contract.LightNodeUpdateHealth{State: "missing"},
		Services: contract.LightNodeServicesHealth{Timer: unknown, Telemetry: unknown, Terminal: unknown, File: unknown, SSHLogin: unknown}}
	data, _ := json.Marshal(snapshot)
	if decodeProcdHealthSnapshot(data, now) == nil {
		t.Fatal("valid snapshot rejected")
	}
	if decodeProcdHealthSnapshot(data, now.Add(61*time.Second)) != nil {
		t.Fatal("stale snapshot accepted")
	}
	if decodeProcdHealthSnapshot(data, now.Add(-6*time.Second)) != nil {
		t.Fatal("future snapshot accepted")
	}
	for _, invalid := range [][]byte{append(append([]byte(nil), data...), []byte("{}")...), []byte(strings.Repeat(" ", 4097)), []byte(strings.Replace(string(data), "\"services\"", "\"credential\"", 1))} {
		if decodeProcdHealthSnapshot(invalid, now) != nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}

func TestProcdRunningDistinguishesUnavailableAndStopped(t *testing.T) {
	for _, tc := range []struct{ name, content, service, want string }{
		{"running", `{"kejilion-node":{"instances":{"main":{"running":true,"pid":123}}}}`, "kejilion-node", "active"},
		{"stopped", `{"kejilion-node":{"instances":{"main":{"running":false}}}}`, "kejilion-node", "inactive"},
		{"unregistered", `{}`, "kejilion-node", "inactive"},
		{"wrong instance", `{"kejilion-node":{"instances":{"other":{"running":true,"pid":123}}}}`, "kejilion-node", "inactive"},
		{"cron instance", `{"cron":{"instances":{"instance1":{"running":true,"pid":125}}}}`, "cron", "active"},
		{"denied", `{"error":"permission denied"}`, "kejilion-node", "unknown"},
		{"no state", `{"kejilion-node":{"instances":{"main":{"pid":123}}}}`, "kejilion-node", "unknown"},
		{"no pid", `{"kejilion-node":{"instances":{"main":{"running":true}}}}`, "kejilion-node", "unknown"},
		{"wrong type", `{"kejilion-node":{"instances":{"main":{"running":"true","pid":123}}}}`, "kejilion-node", "unknown"},
		{"null", `null`, "kejilion-node", "unknown"},
		{"oversize", strings.Repeat(" ", 4097), "kejilion-node", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, got := parseProcdRunning([]byte(tc.content), tc.service), "unknown"
			if state != nil {
				got = healthChoice(*state, "active", "inactive")
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func procdFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted OS fixture requires root in an isolated Linux runner")
	}
	root := t.TempDir()
	for _, directory := range []string{"proc/1", "etc/kejilion-node", "etc/init.d", "etc/rc.d", "etc/crontabs", "lib/functions", "usr/local/lib/kejilion-node"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{"proc/1/comm": "procd\n", "etc/rc.common": "#!/bin/sh\n", "lib/functions/procd.sh": "# native API\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestProcdDetectionAndPersistentStateFailClosed(t *testing.T) {
	root := procdFixture(t)
	if !liveProcdRuntime(root) {
		t.Fatal("native runtime not detected")
	}
	state, err := nodeStateDirectory(root)
	if err != nil || state != filepath.Join(root, "etc/kejilion-node/state") {
		t.Fatalf("state %q: %v", state, err)
	}
	info, err := os.Stat(state)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("state permissions: %v %v", info, err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, state); err != nil {
		t.Fatal(err)
	}
	if _, err := nodeStateDirectory(root); err == nil {
		t.Fatal("symlink state accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "proc/1/comm"), []byte("systemd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if liveProcdRuntime(root) {
		t.Fatal("installed procd scripts mistaken for live runtime")
	}
	if got, err := nodeStateDirectory(root); err != nil || got != filepath.Join(root, "var/lib/kejilion-node") {
		t.Fatalf("existing platform state changed: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc/1/comm"), []byte("procd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "etc/rc.common"), 0o666); err != nil {
		t.Fatal(err)
	}
	if liveProcdRuntime(root) {
		t.Fatal("writable init API trusted")
	}
}

func TestProcdHealthRequiresActualRunningProcessAndCronMembership(t *testing.T) {
	root := procdFixture(t)
	for _, service := range append(append([]string(nil), healthOpenRCServices...), "cron") {
		if err := os.WriteFile(filepath.Join(root, "etc/init.d", service), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../init.d/"+service, filepath.Join(root, "etc/rc.d", "S95"+service)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(healthRootPath(root, lightNodeProcdCronWrapper), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cron := filepath.Join(root, "etc/crontabs/root")
	if err := os.WriteFile(cron, []byte(lightNodeProcdCronLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, stopped := true, false
	got := procdServiceHealth(root, map[string]*bool{"cron": &running, "kejilion-node": &running, "kejilion-node-terminal": &stopped})
	if got[0].ActiveState != "active" || got[0].SubState != "waiting" || got[0].UnitFileState != "enabled" {
		t.Fatalf("timer: %+v", got[0])
	}
	if got[1].ActiveState != "active" || got[2].ActiveState != "inactive" || got[3].ActiveState != "unknown" {
		t.Fatalf("services: %+v", got)
	}
	if err := os.WriteFile(cron, []byte("# "+lightNodeProcdCronLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = procdServiceHealth(root, map[string]*bool{"cron": &running})
	if got[0].ActiveState != "inactive" || got[0].UnitFileState != "disabled" {
		t.Fatal("commented schedule counted as enabled")
	}
	if err := os.Chmod(cron, 0o666); err != nil {
		t.Fatal(err)
	}
	got = procdServiceHealth(root, map[string]*bool{"cron": &running})
	if got[0].ActiveState != "unknown" {
		t.Fatal("untrusted crontab became an authoritative status")
	}
	if err := os.Remove(cron); err != nil {
		t.Fatal(err)
	}
	got = procdServiceHealth(root, map[string]*bool{"cron": &running})
	if got[0].ActiveState != "inactive" || got[0].UnitFileState != "disabled" {
		t.Fatal("deleted crontab did not disable the schedule")
	}
}
