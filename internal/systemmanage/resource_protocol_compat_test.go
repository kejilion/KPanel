package systemmanage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func resourceProtocolFixture(t *testing.T, version string) string {
	t.Helper()
	path := trustedResourceScript(t)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), `PROTOCOL_VERSION="4"`, `PROTOCOL_VERSION="`+version+`"`, 1))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResourceProtocolConsumersAcceptV3AndV4(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	for _, version := range []string{"3", "4"} {
		t.Run("v"+version, func(t *testing.T) {
			path := resourceProtocolFixture(t, version)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// These independent protocols did not change with country firewall support.
			content = append(content, []byte(`
KPANEL_SYSTEM_TUNING_PROTOCOL_VERSION="1"
KJ_SYSTEM_TUNING_NONINTERACTIVE
kpanel_system_tuning_dispatch
KPANEL_SYSTEM_TUNING_MIRROR_SHA256
KPANEL_SYSTEM_TUNING_NETWORK_SHA256
KPANEL_NETWORK_OPERATIONS_PROTOCOL_VERSION="1"
KJ_NETWORK_OPERATIONS_NONINTERACTIVE
kpanel_network_operations_dispatch
KPANEL_NETWORK_OPERATIONS_STATUS
KPANEL_NETWORK_OPERATIONS_VERSION
KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION="1"
KJ_ACCOUNT_MANAGEMENT_NONINTERACTIVE
kpanel_account_dispatch
KPANEL_ACCOUNT_MANAGEMENT_STATUS
--secret-stdin
KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"
KJ_F2B_NONINTERACTIVE
kpanel_f2b_manager_dispatch
KPANEL_F2B_MANAGER_STATUS
`)...)
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			manager, etcRoot, _, _ := testManager(t, &fakeRunner{})
			manager.resourceScript = func() (string, error) { return path, nil }
			mustWrite(t, filepath.Join(etcRoot, "hosts"), "127.0.0.1 localhost\n")
			if err := os.MkdirAll(filepath.Join(manager.sysRoot, "class", "net"), 0o755); err != nil {
				t.Fatal(err)
			}
			for name, lookup := range map[string]func() (string, error){
				"tuning":                manager.systemTuningScriptPath,
				"port scan and traffic": manager.networkOperationsScriptPath,
				"accounts":              manager.accountManagementScriptPath,
				"ssh defense":           manager.sshDefenseManagerScriptPath,
			} {
				if got, err := lookup(); err != nil || got != path {
					t.Fatalf("%s rejected v%s: path=%q err=%v", name, version, got, err)
				}
			}
			for _, id := range []string{"system.hosts.write", "system.cron.write", "system.network-interfaces.write", "system.firewall.write"} {
				if capability := findCapability(manager.SystemResourceCapabilities(), id); !capability.Enabled {
					t.Fatalf("v%s disabled %s: %#v", version, id, capability)
				}
			}
		})
	}
}

func TestFirewallWriteMatchesInstalledResourceProtocol(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	for _, version := range []string{"3", "4"} {
		for _, withIPSet := range []bool{false, true} {
			name := "v" + version
			if withIPSet {
				name += "-ipset"
			}
			t.Run(name, func(t *testing.T) {
				raw := []byte("*filter\n:INPUT ACCEPT [0:0]\nCOMMIT\n")
				updated := []byte("*filter\n:INPUT ACCEPT [0:0]\n-A INPUT -p icmp -m icmp --icmp-type 8 -j DROP\nCOMMIT\n")
				ipsets := []byte("create us_block hash:net family inet\nadd us_block 192.0.2.0/24\n")
				hash := func(data []byte) string {
					if version == "4" && withIPSet {
						return firewallResourceVersion(data, ipsets)
					}
					return firewallResourceVersion(data)
				}
				expected := hash(raw)
				runner := &fakeRunner{missing: map[string]bool{"ipset": !withIPSet}}
				runner.run = func(_ context.Context, command string, arguments ...string) ([]byte, error) {
					switch command {
					case "iptables-save":
						return raw, nil
					case "ipset":
						return ipsets, nil
					case "env":
						if arguments[6] != "disable-ping" || arguments[7] != expected {
							t.Fatalf("wrong script invocation: %#v", arguments)
						}
						raw = updated
						return []byte("KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + hash(raw) + "\n"), nil
					default:
						t.Fatalf("unexpected command %s", command)
						return nil, nil
					}
				}
				manager, _, _, _ := testManager(t, runner)
				path := resourceProtocolFixture(t, version)
				manager.resourceScript = func() (string, error) { return path, nil }
				snapshot, err := manager.Firewall(context.Background())
				if err != nil || snapshot.ResourceVersion != expected {
					t.Fatalf("snapshot=%#v err=%v want version=%s", snapshot, err, expected)
				}
				result, err := manager.ExecuteSystemResourceAction(context.Background(), contract.SystemResourceActionRequest{
					Action: "firewall-disable-ping", ExpectedResourceVersion: snapshot.ResourceVersion,
				})
				if err != nil || !result.Changed || result.ResourceVersion != hash(updated) {
					t.Fatalf("read/write/readback result=%#v err=%v", result, err)
				}
			})
		}
	}
}

func TestCountryFirewallV3RejectedBeforeScript(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	raw := []byte("*filter\n:INPUT ACCEPT [0:0]\nCOMMIT\n")
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "env" {
			t.Fatal("unsupported country action reached the script")
		}
		if name == "iptables-save" {
			return raw, nil
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	path := resourceProtocolFixture(t, "3")
	manager.resourceScript = func() (string, error) { return path, nil }
	for _, action := range []string{"allow", "block", "remove"} {
		_, err := manager.ExecuteSystemResourceAction(context.Background(), contract.SystemResourceActionRequest{
			Action: "firewall-" + action + "-country", CountryCode: "US", ExpectedResourceVersion: firewallResourceVersion(raw),
		})
		if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "protocol v4") {
			t.Fatalf("expected actionable missing protocol error, got %v", err)
		}
	}
}
