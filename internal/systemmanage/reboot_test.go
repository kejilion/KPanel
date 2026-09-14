package systemmanage

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestScheduleRebootUsesDelayedFixedSystemdUnit(t *testing.T) {
	var command string
	var arguments []string
	runner := &fakeRunner{
		run: func(_ context.Context, name string, values ...string) ([]byte, error) {
			command = name
			arguments = append([]string(nil), values...)
			return nil, nil
		},
	}
	manager, _, _, _ := testManager(t, runner)
	changed, message, err := manager.scheduleReboot(
		context.Background(),
		contract.SystemActionRequest{Action: "reboot"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !strings.Contains(message, "15 秒") {
		t.Fatalf("unexpected result: changed=%v message=%q", changed, message)
	}
	if command != "systemd-run" {
		t.Fatalf("command = %q, want systemd-run", command)
	}
	for _, expected := range []string{
		"--collect",
		"--no-block",
		"--on-active=15s",
		"--timer-property=AccuracySec=1s",
		"--property=NoNewPrivileges=yes",
		"--property=ProtectSystem=strict",
		"--",
		"/usr/bin/systemctl",
		"--no-wall",
		"reboot",
	} {
		if !slices.Contains(arguments, expected) {
			t.Fatalf("reboot arguments missing %q: %#v", expected, arguments)
		}
	}
	if len(arguments) == 0 || arguments[0] != "--unit="+rebootUnitName {
		t.Fatalf("unsafe or missing transient unit name: %#v", arguments)
	}
	if _, _, err := manager.scheduleReboot(
		context.Background(),
		contract.SystemActionRequest{Action: "reboot"},
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate schedule error = %v, want ErrConflict", err)
	}
}

func TestScheduleRebootDoesNotUseConfirmationAsAuthorization(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	if _, _, err := manager.scheduleReboot(
		context.Background(),
		contract.SystemActionRequest{Action: "reboot", Confirmation: "anything"},
	); err != nil {
		t.Fatalf("confirmation text became an authorization gate: %v", err)
	}

	manager, _, _, _ = testManager(t, &fakeRunner{})
	if _, _, err := manager.scheduleReboot(
		context.Background(),
		contract.SystemActionRequest{Action: "reboot", Hostname: "unexpected"},
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unrelated reboot input error = %v, want ErrInvalidInput", err)
	}
}

func TestScheduleRebootDoesNotUseMaintenanceAsAnAuthorizationGate(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	if err := manager.writeMaintenance(contract.SystemMaintenanceSummary{State: "running"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := manager.scheduleReboot(
		context.Background(),
		contract.SystemActionRequest{Action: "reboot"},
	)
	if err != nil {
		t.Fatalf("running maintenance blocked an explicit reboot: %v", err)
	}
}

func TestRebootCapabilityRequiresAnInitBackend(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{missing: map[string]bool{
		"systemd-run": true, "start-stop-daemon": true, "rc-service": true,
	}})
	capability := findCapability(manager.Capabilities(), "system.reboot.write")
	if capability.Enabled || !strings.Contains(capability.Reason, "OpenRC") {
		t.Fatalf("unexpected reboot capability: %#v", capability)
	}
}

func TestScheduleRebootUsesFixedOpenRCWorker(t *testing.T) {
	runner := &fakeRunner{
		missing:     map[string]bool{"systemd-run": true},
		runtimeDirs: map[string]bool{"/run/openrc": true},
	}
	manager, _, _, stateDir := testManager(t, runner)
	changed, message, err := manager.scheduleReboot(
		context.Background(), contract.SystemActionRequest{Action: "reboot"},
	)
	if err != nil || !changed || !strings.Contains(message, "15 秒") {
		t.Fatalf("scheduleReboot() = %v, %q, %v", changed, message, err)
	}
	all := strings.Join(runner.commands, "\n")
	for _, expected := range []string{
		"start-stop-daemon --start --background --make-pidfile",
		"--pidfile " + filepath.Join(stateDir, ".job-control", rebootUnitName+".pid"),
		"/usr/local/libexec/kejilion-agent --chdir /",
		"reboot-run --delay-seconds 15",
	} {
		if !strings.Contains(all, expected) {
			t.Fatalf("OpenRC reboot launch missing %q:\n%s", expected, all)
		}
	}
	if strings.Contains(all, "systemctl") {
		t.Fatalf("OpenRC reboot unexpectedly invoked systemctl:\n%s", all)
	}
}
