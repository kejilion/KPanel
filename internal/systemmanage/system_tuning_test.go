package systemmanage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type splitSystemTuningRunner struct {
	resourceRun   func(context.Context, string, ...string) ([]byte, []byte, error)
	runCalls      int
	resourceCalls int
}

func (runner *splitSystemTuningRunner) Run(context.Context, string, ...string) ([]byte, error) {
	runner.runCalls++
	return []byte("business log mixed with protocol output"), nil
}

func (runner *splitSystemTuningRunner) RunResource(
	ctx context.Context,
	_ int,
	_ []byte,
	name string,
	arguments ...string,
) ([]byte, []byte, error) {
	runner.resourceCalls++
	return runner.resourceRun(ctx, name, arguments...)
}

func (runner *splitSystemTuningRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func systemTuningReceipt(status, selected, version string) []byte {
	var output strings.Builder
	output.WriteString("KPANEL_SYSTEM_TUNING_PROTOCOL 1\nKPANEL_SYSTEM_TUNING_STATUS=" + status + "\n")
	output.WriteString("KPANEL_SYSTEM_TUNING_VERSION=" + version + "\n")
	if selected != "" {
		output.WriteString("KPANEL_SYSTEM_TUNING_SELECTED=" + selected + "\n")
	}
	for _, id := range contract.SystemTuningItemIDs {
		output.WriteString("KPANEL_SYSTEM_TUNING_ITEM=" + id + ":pending\n")
	}
	return []byte(output.String())
}

func TestParseSystemTuningOutput(t *testing.T) {
	var output strings.Builder
	output.WriteString("KPANEL_SYSTEM_TUNING_PROTOCOL 1\nKPANEL_SYSTEM_TUNING_STATUS=ok\n")
	output.WriteString("KPANEL_SYSTEM_TUNING_VERSION=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n")
	for _, item := range contract.SystemTuningItemIDs {
		output.WriteString("KPANEL_SYSTEM_TUNING_ITEM=" + item + ":pending\n")
	}
	snapshot, status, selected, err := parseSystemTuningOutput([]byte(output.String()))
	if err != nil || status != "ok" || selected != "" || len(snapshot.Items) != 12 {
		t.Fatalf("parse result = %#v %q %q %v", snapshot, status, selected, err)
	}
}

func TestSystemTuningPolicyRoundTrip(t *testing.T) {
	version := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	items := []string{"system-update", "bbr", "kernel-auto"}
	parsed, parsedVersion, ok := parseSystemTuningMaintenancePolicy(systemTuningMaintenancePolicy(items, version))
	if !ok || parsedVersion != version || strings.Join(parsed, ",") != strings.Join(items, ",") {
		t.Fatalf("round trip = %v %q %v", parsed, parsedVersion, ok)
	}
}

func TestTrustedSystemTuningRequiresPinnedSources(t *testing.T) {
	content := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"3\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE\nkpanel_system_resource_dispatch\nKPANEL_SYSTEM_RESOURCE_STATUS\nKPANEL_SYSTEM_RESOURCE_VERSION\nKPANEL_SYSTEM_TUNING_PROTOCOL_VERSION=\"1\"\nKJ_SYSTEM_TUNING_NONINTERACTIVE\nkpanel_system_tuning_dispatch\nKPANEL_SYSTEM_TUNING_MIRROR_SHA256\nKPANEL_SYSTEM_TUNING_NETWORK_SHA256\n")
	if !trustedKejilionSystemTuningContent(content) {
		t.Fatal("current protocol markers were rejected")
	}
	if trustedKejilionSystemTuningContent([]byte(strings.ReplaceAll(string(content), "KPANEL_SYSTEM_TUNING_NETWORK_SHA256", "MISSING"))) {
		t.Fatal("unpinned network source was accepted")
	}
}

func systemTuningScriptFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kejilion.sh")
	content := "#!/bin/bash\npermission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"3\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE\nkpanel_system_resource_dispatch\nKPANEL_SYSTEM_RESOURCE_STATUS\nKPANEL_SYSTEM_RESOURCE_VERSION\nKPANEL_SYSTEM_TUNING_PROTOCOL_VERSION=\"1\"\nKJ_SYSTEM_TUNING_NONINTERACTIVE\nkpanel_system_tuning_dispatch\nKPANEL_SYSTEM_TUNING_MIRROR_SHA256\nKPANEL_SYSTEM_TUNING_NETWORK_SHA256\n# " + strings.Repeat("x", 1200)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSystemTuningMaintenanceBuildsOneFixedStepPerSelectedItem(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("trusted script ownership check requires root")
	}
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	script := systemTuningScriptFixture(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	version := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	policy := systemTuningMaintenancePolicy([]string{"system-update", "bbr", "kernel-auto"}, version)
	action, returnedPolicy, steps, err := manager.maintenanceSteps("system-tuning-" + policy)
	if err != nil {
		t.Fatal(err)
	}
	if action != "system-tuning" || returnedPolicy != policy || len(steps) != 3 {
		t.Fatalf("plan = %q %q %#v", action, returnedPolicy, steps)
	}
	for index, item := range []string{"system-update", "bbr", "kernel-auto"} {
		step := steps[index]
		if step.operation != maintenanceOperationSystemTuning || step.command != "env" || step.arguments[len(step.arguments)-2] != "apply-item" || step.arguments[len(step.arguments)-1] != item {
			t.Fatalf("step %d = %#v", index, step)
		}
	}
}

func TestRunSystemTuningStopsAtFirstFailedItem(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("trusted script ownership check requires root")
	}
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	script := systemTuningScriptFixture(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	version := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	items := []string{"bbr", "kernel-auto"}
	policy := systemTuningMaintenancePolicy(items, version)
	started := manager.now().UTC()
	if err := manager.writeMaintenance(contract.SystemMaintenanceSummary{ID: "test-tuning", State: "running", Action: "system-tuning", Policy: policy, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	runner.run = func(_ context.Context, _ string, arguments ...string) ([]byte, error) {
		item := arguments[len(arguments)-1]
		status := "applied"
		if item == "kernel-auto" {
			status = "needs-attention"
		}
		var output strings.Builder
		output.WriteString("KPANEL_SYSTEM_TUNING_PROTOCOL 1\nKPANEL_SYSTEM_TUNING_STATUS=" + status + "\n")
		output.WriteString("KPANEL_SYSTEM_TUNING_VERSION=" + version + "\nKPANEL_SYSTEM_TUNING_SELECTED=" + item + "\n")
		for _, id := range contract.SystemTuningItemIDs {
			output.WriteString("KPANEL_SYSTEM_TUNING_ITEM=" + id + ":pending\n")
		}
		return []byte(output.String()), nil
	}
	if err := manager.RunMaintenance(context.Background(), "system-tuning-"+policy); err == nil {
		t.Fatal("failed item was reported as success")
	}
	status := manager.readMaintenance()
	if status.State != "failed" || status.Stage != "system_tuning_kernel-auto" {
		t.Fatalf("status = %#v", status)
	}
}

func TestRunSystemTuningKeepsAllTwelveItemLogsOutOfProtocolReceipts(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("trusted script ownership check requires root")
	}
	version := strings.Repeat("a", 64)
	runner := &splitSystemTuningRunner{}
	runner.resourceRun = func(_ context.Context, _ string, arguments ...string) ([]byte, []byte, error) {
		item := arguments[len(arguments)-1]
		return systemTuningReceipt("applied", item, version), []byte("native operation log for " + item), nil
	}
	manager, _, _, _ := testManager(t, runner)
	script := systemTuningScriptFixture(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	policy := systemTuningMaintenancePolicy(contract.SystemTuningItemIDs, version)
	started := manager.now().UTC()
	if err := manager.writeMaintenance(contract.SystemMaintenanceSummary{ID: "test-all-tuning", State: "running", Action: "system-tuning", Policy: policy, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunMaintenance(context.Background(), "system-tuning-"+policy); err != nil {
		t.Fatal(err)
	}
	status := manager.readMaintenance()
	if status.State != "succeeded" || status.Stage != "completed" {
		t.Fatalf("status = %#v", status)
	}
	if runner.runCalls != 0 || runner.resourceCalls != len(contract.SystemTuningItemIDs) {
		t.Fatalf("combined calls=%d split calls=%d", runner.runCalls, runner.resourceCalls)
	}
}

func TestRunSystemTuningReportsNativeFailureWithoutProtocolDump(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("trusted script ownership check requires root")
	}
	version := strings.Repeat("b", 64)
	runner := &splitSystemTuningRunner{}
	runner.resourceRun = func(_ context.Context, _ string, arguments ...string) ([]byte, []byte, error) {
		item := arguments[len(arguments)-1]
		return systemTuningReceipt("needs-attention", item, version), nil, errors.New("chattr: Operation not supported while reading flags: exit status 1")
	}
	manager, _, _, _ := testManager(t, runner)
	script := systemTuningScriptFixture(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	policy := systemTuningMaintenancePolicy([]string{"dns-auto"}, version)
	started := manager.now().UTC()
	if err := manager.writeMaintenance(contract.SystemMaintenanceSummary{ID: "test-tuning-error", State: "running", Action: "system-tuning", Policy: policy, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunMaintenance(context.Background(), "system-tuning-"+policy); err == nil {
		t.Fatal("failed DNS tuning item was reported as success")
	}
	status := manager.readMaintenance()
	if status.State != "failed" || !strings.Contains(status.Message, "Operation not supported") || strings.Contains(status.Message, "KPANEL_SYSTEM_TUNING_") {
		t.Fatalf("status = %#v", status)
	}
}
