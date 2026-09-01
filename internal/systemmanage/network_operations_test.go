package systemmanage

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func networkOperationsFixtureScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kejilion.sh")
	content := strings.Join([]string{
		`permission_granted="true"`,
		`KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="4"`,
		`KPANEL_NETWORK_OPERATIONS_PROTOCOL_VERSION="1"`,
		`KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1`,
		`KJ_NETWORK_OPERATIONS_NONINTERACTIVE=1`,
		`kpanel_system_resource_dispatch`,
		`KPANEL_SYSTEM_RESOURCE_STATUS`,
		`KPANEL_SYSTEM_RESOURCE_VERSION`,
		`kpanel_network_operations_dispatch`,
		`KPANEL_NETWORK_OPERATIONS_STATUS`,
		`KPANEL_NETWORK_OPERATIONS_VERSION`,
		strings.Repeat("# padding\n", 140),
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func networkTrafficOutput(version string, enabled bool, health string, rxThreshold, txThreshold uint64, day int) []byte {
	return []byte(strings.Join([]string{
		"KPANEL_NETWORK_OPERATIONS_STATUS=ok",
		"KPANEL_NETWORK_OPERATIONS_VERSION=" + version,
		"KPANEL_NETWORK_OPERATIONS_ENABLED=" + strconvBool(enabled),
		"KPANEL_NETWORK_OPERATIONS_HEALTH=" + health,
		"KPANEL_NETWORK_OPERATIONS_RX_BYTES=1073741824",
		"KPANEL_NETWORK_OPERATIONS_TX_BYTES=2147483648",
		"KPANEL_NETWORK_OPERATIONS_RX_THRESHOLD_GIB=" + uintString(rxThreshold),
		"KPANEL_NETWORK_OPERATIONS_TX_THRESHOLD_GIB=" + uintString(txThreshold),
		"KPANEL_NETWORK_OPERATIONS_RESET_DAY=" + intString(day),
		"",
	}, "\n"))
}

func strconvBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func uintString(value uint64) string { return strconv.FormatUint(value, 10) }
func intString(value int) string     { return strconv.Itoa(value) }

func TestPortUsageProtocolParsesBoundedIPv6AndProcessData(t *testing.T) {
	version := strings.Repeat("a", 64)
	raw := `udp UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:(("nginx",pid=798910,fd=16),("nginx",pid=798909,fd=16)) ino:2669375 sk:1011 cgroup:/system.slice/docker-example.scope <->`
	output := []byte(strings.Join([]string{
		"KPANEL_NETWORK_OPERATIONS_STATUS=ok",
		"KPANEL_NETWORK_OPERATIONS_VERSION=" + version,
		"KPANEL_NETWORK_OPERATIONS_TOTAL=1",
		"KPANEL_NETWORK_OPERATIONS_TRUNCATED=false",
		"KPANEL_NETWORK_OPERATIONS_PORT_HEX=" + hex.EncodeToString([]byte(raw)),
	}, "\n"))
	snapshot, err := parsePortUsageOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Entries[0]
	if entry.LocalAddress != "0.0.0.0" || entry.LocalPort != "443" || entry.PeerAddress != "0.0.0.0" || entry.PeerPort != "*" || entry.Process != "nginx" || entry.PID != 798910 {
		t.Fatalf("unexpected port entry: %#v", entry)
	}

	output = append(output, []byte("\nKPANEL_NETWORK_OPERATIONS_PORT_HEX=00")...)
	if _, err := parsePortUsageOutput(output); err == nil {
		t.Fatal("accepted a NUL port record")
	}
}

func TestTrafficShutdownProtocolRejectsInconsistentReadyState(t *testing.T) {
	version := strings.Repeat("a", 64)
	if _, err := parseTrafficShutdownOutput(networkTrafficOutput(version, true, "ready", 100, 200, 5)); err != nil {
		t.Fatal(err)
	}
	if _, err := parseTrafficShutdownOutput(networkTrafficOutput(version, false, "ready", 100, 200, 5)); err == nil {
		t.Fatal("accepted ready state while disabled")
	}
}

func TestTrafficShutdownWriteUsesFixedScriptProtocolAndReadback(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("trusted script execution requires a root-owned fixture")
	}
	versionA, versionB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	call := 0
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name != "env" {
			return nil, nil
		}
		joined := strings.Join(arguments, " ")
		if strings.Contains(joined, "traffic-shutdown status") {
			call++
			if call == 1 {
				return networkTrafficOutput(versionA, false, "disabled", 0, 0, 0), nil
			}
			return networkTrafficOutput(versionB, true, "ready", 100, 200, 5), nil
		}
		if strings.Contains(joined, "traffic-shutdown enable "+versionA+" 100 200 5") {
			return []byte("KPANEL_NETWORK_OPERATIONS_STATUS=applied\nKPANEL_NETWORK_OPERATIONS_VERSION=" + versionB + "\n"), nil
		}
		t.Fatalf("unexpected command: %s %#v", name, arguments)
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	script := networkOperationsFixtureScript(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	rx, tx, day := uint64(100), uint64(200), 5
	result, err := manager.ExecuteTrafficShutdownAction(context.Background(), contract.TrafficShutdownActionRequest{
		Action: "enable", ExpectedResourceVersion: versionA,
		RXThresholdGiB: &rx, TXThresholdGiB: &tx, ResetDay: &day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.ResourceVersion != versionB || len(runner.commands) != 3 {
		t.Fatalf("unexpected result=%#v commands=%#v", result, runner.commands)
	}
	for _, command := range runner.commands {
		if strings.ContainsAny(command, ";|`") || !strings.Contains(command, "KJ_NETWORK_OPERATIONS_NONINTERACTIVE=1 bash ") {
			t.Fatalf("unsafe or unexpected invocation: %q", command)
		}
	}
}

func TestNetworkOperationsTrustRequiresDedicatedV1Marker(t *testing.T) {
	legacy := []byte(`permission_granted="true"
KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="4"
KJ_SYSTEM_RESOURCE_NONINTERACTIVE
kpanel_system_resource_dispatch
KPANEL_SYSTEM_RESOURCE_STATUS
KPANEL_SYSTEM_RESOURCE_VERSION
KJ_NETWORK_OPERATIONS_NONINTERACTIVE
kpanel_network_operations_dispatch
KPANEL_NETWORK_OPERATIONS_STATUS
KPANEL_NETWORK_OPERATIONS_VERSION`)
	if trustedKejilionNetworkOperationsContent(legacy) {
		t.Fatal("trusted a script without the dedicated network-operations marker")
	}
}
