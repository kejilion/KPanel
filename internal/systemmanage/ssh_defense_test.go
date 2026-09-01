package systemmanage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseSSHDefenseStatus(t *testing.T) {
	status, err := parseSSHDefenseStatus([]byte(
		"KPANEL_F2B_PROTOCOL 1\n" +
			`KPANEL_F2B_STATUS {"installed":true,"running":true,"enabled":true,"autostart":true,"jail":"sshd","banned":3}` +
			"\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || !status.Running || !status.Enabled || !status.Autostart ||
		status.Jail != "sshd" || status.Banned != 3 {
		t.Fatalf("SSH defense status = %#v", status)
	}

	for name, output := range map[string]string{
		"missing marker": `{"enabled":true}`,
		"unknown field":  `KPANEL_F2B_STATUS {"jail":"sshd","extra":true}`,
		"unknown jail":   `KPANEL_F2B_STATUS {"jail":"custom"}`,
		"negative count": `KPANEL_F2B_STATUS {"jail":"sshd","banned":-1}`,
		"trailing JSON":  `KPANEL_F2B_STATUS {"jail":"sshd"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSSHDefenseStatus([]byte(output)); err == nil {
				t.Fatalf("invalid status accepted: %q", output)
			}
		})
	}
}

func TestSSHDefenseStatusUsesFixedKejilionProtocol(t *testing.T) {
	runner := &fakeRunner{
		run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
			if name != "env" {
				t.Fatalf("SSH defense status command = %s %#v", name, arguments)
			}
			return []byte(
				"KPANEL_F2B_PROTOCOL 1\n" +
					`KPANEL_F2B_STATUS {"installed":true,"running":true,"enabled":true,"autostart":true,"jail":"sshd","banned":2}` +
					"\n",
			), nil
		},
	}
	manager, _, _, _ := testManager(t, runner)
	manager.f2bScript = func() (string, error) {
		return "/home/docker/kpanel/bin/kejilion.sh", nil
	}
	status := manager.SSHDefenseStatus(context.Background())
	if !status.Available || !status.Enabled || status.Banned != 2 {
		t.Fatalf("SSH defense status = %#v", status)
	}
	command := strings.Join(runner.commands, "\n")
	for _, required := range []string{
		"env KJ_F2B_NONINTERACTIVE=1",
		"bash /home/docker/kpanel/bin/kejilion.sh f2b status",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("SSH defense command missing %q: %s", required, command)
		}
	}
	for _, forbidden := range []string{"sh -c", "bash -c", "fail2ban-client"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("SSH defense status bypassed the script protocol: %s", command)
		}
	}
}

func TestSSHDefenseStatusReportsUnavailableProtocol(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	manager.f2bScript = func() (string, error) {
		return "", errors.New("old script")
	}
	status := manager.SSHDefenseStatus(context.Background())
	if status.Available || status.Message == "" {
		t.Fatalf("unavailable SSH defense status = %#v", status)
	}
}

func TestSSHDefenseRunsAsPersistentFixedMaintenanceTask(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	runner := &fakeRunner{}
	manager, _, _, stateDir := testManager(t, runner)
	manager.executable = filepath.Join(stateDir, "kejilion-agent")
	script := filepath.Join(stateDir, "kejilion.sh")
	content := strings.Repeat("# padding\n", 160) + strings.Join([]string{
		`permission_granted="true"`, `KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="4"`,
		"KJ_SYSTEM_RESOURCE_NONINTERACTIVE", "kpanel_system_resource_dispatch", "KPANEL_SYSTEM_RESOURCE_STATUS", "KPANEL_SYSTEM_RESOURCE_VERSION",
		`KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"`, "KJ_F2B_NONINTERACTIVE", "kpanel_f2b_manager_dispatch", "KPANEL_F2B_MANAGER_STATUS",
	}, "\n")
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	manager.resourceScript = func() (string, error) {
		return script, nil
	}
	if _, err := manager.sshDefenseManagerScriptPath(); err != nil {
		t.Fatalf("trusted SSH defense script: %v", err)
	}

	changed, message, err := manager.startMaintenance(
		context.Background(),
		"ssh-defense",
		"enable",
	)
	if err != nil || !changed || message == "" {
		t.Fatalf("start SSH defense: changed=%v message=%q err=%v", changed, message, err)
	}
	if err := manager.RunMaintenance(context.Background(), "ssh-defense-enable"); err != nil {
		t.Fatal(err)
	}
	status := manager.MaintenanceStatus()
	if status.Action != "ssh-defense" || status.Policy != "enable" ||
		status.State != "succeeded" || status.Progress != 100 {
		t.Fatalf("SSH defense maintenance state = %#v", status)
	}
	command := strings.Join(runner.commands, "\n")
	for _, required := range []string{
		"--unit=kejilion-panel-maintenance-",
		manager.executable + " maintenance-run --state-dir " + stateDir + " ssh-defense-enable",
		"env KJ_F2B_NONINTERACTIVE=1 LC_ALL=C.UTF-8 LANG=C.UTF-8 bash " +
			script + " f2b manager enable",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("SSH defense task missing %q:\n%s", required, command)
		}
	}
	for _, forbidden := range []string{"sh -c", "bash -c"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("SSH defense task used a shell fragment %q:\n%s", forbidden, command)
		}
	}
}
