package systemmanage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestHostsSnapshotUsesExactBytesAndReportsLineTruncation(t *testing.T) {
	manager, etcRoot, _, _ := testManager(t, &fakeRunner{})
	var content strings.Builder
	for index := 0; index < contract.SystemHostsMaxLines+1; index++ {
		content.WriteString("192.0.2.")
		content.WriteString(strconv.Itoa(index%250 + 1))
		content.WriteString(" host-")
		content.WriteString(strconv.Itoa(index))
		content.WriteByte('\n')
	}
	raw := content.String()
	mustWrite(t, filepath.Join(etcRoot, "hosts"), raw)
	snapshot, err := manager.Hosts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != resourceHash([]byte(raw)) ||
		snapshot.Total != contract.SystemHostsMaxLines+1 ||
		len(snapshot.Entries) != contract.SystemHostsMaxLines || !snapshot.Truncated {
		t.Fatalf("unexpected hosts snapshot: %#v", snapshot)
	}
	if snapshot.Entries[0].Line != 1 || snapshot.Entries[0].Raw != "192.0.2.1 host-0" {
		t.Fatalf("unexpected first hosts entry: %#v", snapshot.Entries[0])
	}
	if _, err := manager.systemResourceVersion(context.Background(), "hosts", contract.SystemResourceActionRequest{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("truncated hosts write preflight error=%v", err)
	}
}

func TestHostsSnapshotRejectsOversizedResource(t *testing.T) {
	manager, etcRoot, _, _ := testManager(t, &fakeRunner{})
	mustWrite(t, filepath.Join(etcRoot, "hosts"), strings.Repeat("x", contract.SystemHostsMaxBytes+1))
	_, err := manager.Hosts(context.Background())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("oversized hosts error = %v", err)
	}
	capability := findCapability(manager.SystemResourceCapabilities(), "system.hosts.read")
	if capability.Enabled || capability.Reason == "" {
		t.Fatalf("oversized hosts read capability=%#v", capability)
	}
}

func TestCronTreatsMissingCrontabAsExactEmptyBytes(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name == "crontab" && len(arguments) == 1 && arguments[0] == "-l" {
			return nil, errors.New("no crontab for root")
		}
		return nil, nil
	}}
	manager, _, _, _ := testManager(t, runner)
	snapshot, err := manager.Cron(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != resourceHash([]byte{}) || snapshot.Total != 0 ||
		len(snapshot.Entries) != 0 || snapshot.Truncated {
		t.Fatalf("unexpected empty crontab: %#v", snapshot)
	}
}

func TestCronSnapshotClassifiesAndBoundsPhysicalLines(t *testing.T) {
	lines := []string{"# comment", "SHELL=/bin/bash", "0 2 * * * /usr/bin/backup --full", "@reboot /usr/bin/start", ""}
	for len(lines) < contract.SystemCronMaxLines+1 {
		lines = append(lines, "* * * * * true")
	}
	raw := strings.Join(lines, "\n") + "\n"
	runner := &fakeRunner{run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(raw), nil
	}}
	manager, _, _, _ := testManager(t, runner)
	snapshot, err := manager.Cron(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != len(lines) || len(snapshot.Entries) != contract.SystemCronMaxLines || !snapshot.Truncated ||
		snapshot.ResourceVersion != resourceHash([]byte(raw)) {
		t.Fatalf("unexpected bounded crontab: total=%d entries=%d truncated=%v", snapshot.Total, len(snapshot.Entries), snapshot.Truncated)
	}
	if snapshot.Entries[0].Kind != "comment" || snapshot.Entries[1].Kind != "environment" ||
		snapshot.Entries[2].Expression != "0 2 * * *" || snapshot.Entries[2].Command != "/usr/bin/backup --full" ||
		snapshot.Entries[3].Expression != "@reboot" {
		t.Fatalf("unexpected crontab parsing: %#v", snapshot.Entries[:4])
	}
	if _, err := manager.systemResourceVersion(context.Background(), "cron", contract.SystemResourceActionRequest{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("truncated cron write preflight error=%v", err)
	}
}

func TestNetworkSnapshotUsesAdministrativeStateAndPerEntryVersion(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name != "ip" {
			t.Fatalf("unexpected command %s %#v", name, arguments)
		}
		return []byte(`[
			{"ifname":"eth0","addr_info":[{"family":"inet","local":"192.0.2.10","prefixlen":24}]},
			{"ifname":"lo","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8}]}
		]`), nil
	}}
	manager, _, _, _ := testManager(t, runner)
	networkRoot := filepath.Join(manager.sysRoot, "class", "net")
	for name, values := range map[string][2]string{
		"eth0": {"0x1002", "02:00:00:00:00:01"},
		"lo":   {"0x9", "00:00:00:00:00:00"},
	} {
		mustWrite(t, filepath.Join(networkRoot, name, "flags"), values[0]+"\n")
		mustWrite(t, filepath.Join(networkRoot, name, "address"), values[1]+"\n")
	}
	snapshot, err := manager.NetworkInterfaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].Name != "eth0" || snapshot.Entries[0].State != "down" ||
		snapshot.Entries[1].Name != "lo" || snapshot.Entries[1].State != "up" || !snapshot.Entries[1].Loopback {
		t.Fatalf("unexpected network snapshot: %#v", snapshot)
	}
	wantVersion := resourceHash([]byte("lo|up|00:00:00:00:00:00"))
	if snapshot.Entries[1].ResourceVersion != wantVersion {
		t.Fatalf("loopback version=%q want=%q", snapshot.Entries[1].ResourceVersion, wantVersion)
	}
}

func TestNetworkSnapshotReportsExactTotalPastEntryLimit(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name != "ip" {
			t.Fatalf("unexpected command %s %#v", name, arguments)
		}
		return []byte("[]"), nil
	}}
	manager, _, _, _ := testManager(t, runner)
	networkRoot := filepath.Join(manager.sysRoot, "class", "net")
	wantTotal := contract.SystemNetworkMaxEntries + 2
	for index := 0; index < wantTotal; index++ {
		name := fmt.Sprintf("eth%03d", index)
		mustWrite(t, filepath.Join(networkRoot, name, "flags"), "0x1\n")
		mustWrite(t, filepath.Join(networkRoot, name, "address"), fmt.Sprintf("02:00:00:00:%02x:%02x\n", index/256, index%256))
	}
	snapshot, err := manager.NetworkInterfaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != wantTotal || len(snapshot.Entries) != contract.SystemNetworkMaxEntries || !snapshot.Truncated {
		t.Fatalf("total=%d entries=%d truncated=%v", snapshot.Total, len(snapshot.Entries), snapshot.Truncated)
	}
}

func TestNetworkSnapshotRejectsCountBeyondScanBound(t *testing.T) {
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	networkRoot := filepath.Join(manager.sysRoot, "class", "net")
	for index := 0; index <= resourceNetworkScanLimit; index++ {
		if err := os.MkdirAll(filepath.Join(networkRoot, fmt.Sprintf("v%04d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.NetworkInterfaces(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("oversized interface inventory error=%v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("oversized inventory invoked commands: %#v", runner.commands)
	}
}

func TestFirewallSnapshotUsesExactBytesAndFirstPingRule(t *testing.T) {
	raw := strings.Join([]string{
		"# Generated by iptables-save v1.8.9 (nf_tables)",
		"*filter",
		":INPUT DROP [0:0]",
		"-A INPUT -s 192.0.2.1/32 -p icmp -m icmp --icmp-type 8 -j ACCEPT",
		"-A INPUT -p icmp -m icmp --icmp-type 8 -j DROP",
		"-A INPUT -p icmp -m icmp --icmp-type echo-request -j ACCEPT",
		"-A INPUT -p tcp -m tcp --tcp-flags 0x17/0x02 -m limit --limit 500/sec --limit-burst 100 -j ACCEPT",
		"-A INPUT -p tcp -m tcp --tcp-flags 0x17/0x02 -j DROP",
		"-A INPUT -p udp -m udp -m limit --limit 3333/sec --limit-burst 5 -j ACCEPT",
		"-A INPUT -p udp -m udp -j DROP",
		"COMMIT",
		"*nat",
		":INPUT ACCEPT [100:200]",
		"-A INPUT -p icmp -m icmp --icmp-type echo-request -j ACCEPT",
		"COMMIT",
	}, "\n") + "\n"
	runner := &fakeRunner{missing: map[string]bool{"ipset": true}, run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(raw), nil
	}}
	manager, _, _, _ := testManager(t, runner)
	snapshot, err := manager.Firewall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != firewallResourceVersion([]byte(raw)) || snapshot.Backend != "iptables-nft" ||
		snapshot.InputPolicy != "DROP" || snapshot.PingAllowed || !snapshot.DDoSEnabled || snapshot.Total != 7 {
		t.Fatalf("unexpected firewall snapshot: %#v", snapshot)
	}
	if snapshot.Rules[1].Line != 5 || snapshot.Rules[1].Chain != "INPUT" ||
		snapshot.Rules[1].Target != "DROP" || snapshot.Rules[1].Protocol != "icmp" {
		t.Fatalf("unexpected managed Ping rule: %#v", snapshot.Rules[1])
	}
}

func TestFirewallSnapshotParsesCountryRulesWithoutRawTechnicalText(t *testing.T) {
	iptablesRaw := strings.Join([]string{
		"# Generated by iptables-save v1.8.9 (nf_tables)",
		"*filter",
		":INPUT DROP [0:0]",
		"-A INPUT -m set --match-set us_block src -j ACCEPT",
		"-A INPUT -m set --match-set cn_block src -j DROP",
		"-A FORWARD -m set --match-set jp_block src -j DROP",
		"COMMIT",
	}, "\n") + "\n"
	ipsetRaw := "create us_block hash:net family inet\nadd us_block 192.0.2.0/24\nadd us_block 198.51.100.0/24\ncreate cn_block hash:net family inet\nadd cn_block 203.0.113.0/24\n"
	runner := &fakeRunner{run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
		switch name {
		case "iptables-save":
			return []byte(iptablesRaw), nil
		case "ipset":
			return []byte(ipsetRaw), nil
		default:
			return nil, nil
		}
	}}
	manager, _, _, _ := testManager(t, runner)
	snapshot, err := manager.Firewall(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != firewallResourceVersion([]byte(iptablesRaw), []byte(ipsetRaw)) ||
		snapshot.Backend != "iptables-nft" || snapshot.InputPolicy != "DROP" || snapshot.Total != 3 ||
		len(snapshot.CountryRules) != 2 {
		t.Fatalf("unexpected country firewall snapshot: %#v", snapshot)
	}
	want := []contract.SystemFirewallCountryRule{
		{Code: "CN", Decision: "block", Zone: "inbound", NetworkCount: 1},
		{Code: "US", Decision: "allow", Zone: "inbound", NetworkCount: 2},
	}
	if fmt.Sprintf("%#v", snapshot.CountryRules) != fmt.Sprintf("%#v", want) {
		t.Fatalf("country rules=%#v want=%#v", snapshot.CountryRules, want)
	}
}

func TestFirewallDDoSStatusRequiresAllExactInputRules(t *testing.T) {
	inputRules := []string{
		"-A INPUT -p tcp --syn -m limit --limit 500/s --limit-burst 100 -j ACCEPT",
		"-A INPUT -p tcp --syn -j DROP",
		"-A INPUT -p udp -m limit --limit 3000/s -j ACCEPT",
		"-A INPUT -p udp -j DROP",
	}
	var mask uint8
	for _, rule := range inputRules {
		mask |= firewallDDoSRuleSignature(rule)
	}
	if mask != 0b1111 {
		t.Fatalf("exact INPUT DDoS mask=%04b", mask)
	}
	if firewallDDoSRuleSignature("-A DOCKER-USER -p udp -j DROP") != 0 ||
		firewallDDoSRuleSignature("-A INPUT -p udp -s 192.0.2.0/24 -j DROP") != 0 ||
		firewallDDoSRuleSignature("-A INPUT -p tcp --syn -s 192.0.2.0/24 -m limit --limit 500/s --limit-burst 100 -j ACCEPT") != 0 {
		t.Fatal("unrelated or cross-chain rules were recognized as KPanel DDoS rules")
	}
}

func TestFirewallVersionIgnoresGeneratedMetadataAndLiveCounters(t *testing.T) {
	first := []byte(strings.Join([]string{
		"# Generated by iptables-save v1.8.9 (nf_tables) on Mon Aug 10 12:00:00 2026",
		"*filter",
		":INPUT DROP [123:4567]",
		":FORWARD ACCEPT [89:1011]",
		"-A INPUT -p tcp --dport 443 -j ACCEPT",
		"COMMIT",
		"# Completed on Mon Aug 10 12:00:00 2026",
	}, "\r\n") + "\r\n")
	second := []byte(strings.Join([]string{
		"# Generated by iptables-save v1.8.9 (nf_tables) on Mon Aug 10 12:00:03 2026",
		"*filter",
		":INPUT DROP [999:8888]",
		":FORWARD ACCEPT [777:6666]",
		"-A INPUT -p tcp --dport 443 -j ACCEPT",
		"COMMIT",
		"# Completed on Mon Aug 10 12:00:03 2026",
	}, "\n") + "\n")
	changed := bytes.Replace(second, []byte("--dport 443"), []byte("--dport 8443"), 1)

	if firewallResourceVersion(first) != firewallResourceVersion(second) {
		t.Fatalf("metadata/counter-only change altered firewall version:\n%s\n%s", canonicalFirewallRules(first), canonicalFirewallRules(second))
	}
	if firewallResourceVersion(second) == firewallResourceVersion(changed) {
		t.Fatal("rule change did not alter firewall version")
	}
	want := "*filter\n:INPUT DROP [0:0]\n:FORWARD ACCEPT [0:0]\n-A INPUT -p tcp --dport 443 -j ACCEPT\nCOMMIT\n"
	if string(canonicalFirewallRules(first)) != want {
		t.Fatalf("canonical firewall rules=%q want=%q", canonicalFirewallRules(first), want)
	}
}

func TestPrivilegedResourceReadsRejectInsufficientAgentPrivileges(t *testing.T) {
	manager, _, procRoot, _ := testManager(t, &fakeRunner{})
	manager.effectiveUID = func() int { return 1000 }
	if capability := findCapability(manager.SystemResourceCapabilities(), "system.cron.read"); capability.Enabled || capability.Reason == "" {
		t.Fatalf("non-root cron capability=%#v", capability)
	}
	if _, err := manager.Cron(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("non-root cron read error=%v", err)
	}

	manager.effectiveUID = func() int { return 0 }
	mustWrite(t, filepath.Join(procRoot, "self", "status"), "CapEff:\t0000000000000000\n")
	if _, err := manager.Firewall(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("firewall without CAP_NET_ADMIN error=%v", err)
	}
	if runtime.GOOS == "linux" {
		capability := findCapability(manager.SystemResourceCapabilities(), "system.firewall.read")
		if capability.Enabled || !strings.Contains(capability.Reason, "CAP_NET_ADMIN") {
			t.Fatalf("firewall capability without CAP_NET_ADMIN=%#v", capability)
		}
	}
}

func TestSystemResourceReceiptRejectsNoiseAndMalformedMarkers(t *testing.T) {
	version := strings.Repeat("a", 64)
	valid, err := parseSystemResourceReceipt([]byte(
		"KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\n",
	))
	if err != nil || valid.Status != "applied" || valid.Version != version {
		t.Fatalf("valid receipt=%#v err=%v", valid, err)
	}
	failed, err := parseSystemResourceReceipt([]byte(
		"KPANEL_SYSTEM_RESOURCE_STATUS failed\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\n",
	))
	if err != nil || failed.Status != "failed" {
		t.Fatalf("failed receipt=%#v err=%v", failed, err)
	}
	rollbackFailed, err := parseSystemResourceReceipt([]byte(
		"KPANEL_SYSTEM_RESOURCE_STATUS rollback-failed\nKPANEL_SYSTEM_RESOURCE_VERSION " + version +
			"\nKPANEL_SYSTEM_RESOURCE_BACKUP /var/lib/kejilion-panel/system/recovery/system-resource/20260810T120000Z-firewall.Ab12x9\n",
	))
	if runtime.GOOS == "linux" && (err != nil || !strings.Contains(rollbackFailed.Backup, "/system/recovery/")) {
		t.Fatalf("persistent rollback receipt=%#v err=%v", rollbackFailed, err)
	}
	for _, output := range []string{
		"log noise\nKPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\n",
		"KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\n",
		"KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION uppercase\n",
		"KPANEL_SYSTEM_RESOURCE_STATUS rollback-failed\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\nKPANEL_SYSTEM_RESOURCE_BACKUP /tmp/private-snapshot\n",
		"KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + version + "\nKPANEL_SYSTEM_RESOURCE_BACKUP /var/lib/kejilion-panel/system/recovery/system-resource/20260810T120000Z-hosts.Ab12x9\n",
	} {
		if _, err := parseSystemResourceReceipt([]byte(output)); err == nil {
			t.Fatalf("malformed receipt accepted: %q", output)
		}
	}
}

func TestTrustedSystemResourceProtocolMarkers(t *testing.T) {
	legacy := []byte("permission_granted=\"true\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE=1\nkpanel_system_resource_noninteractive() { :; }\nKPANEL_SYSTEM_RESOURCE_STATUS=x\nKPANEL_SYSTEM_RESOURCE_VERSION=x\n")
	if trustedKejilionSystemResourceContent(legacy) {
		t.Fatal("legacy function marker was trusted")
	}
	unversioned := []byte("permission_granted=\"true\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE=1\nkpanel_system_resource_dispatch() { :; }\nKPANEL_SYSTEM_RESOURCE_STATUS=x\nKPANEL_SYSTEM_RESOURCE_VERSION=x\n")
	if trustedKejilionSystemResourceContent(unversioned) {
		t.Fatal("unversioned protocol without cron stdin support was trusted")
	}
	versionOne := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"1\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE=1\nkpanel_system_resource_dispatch() { :; }\nKPANEL_SYSTEM_RESOURCE_STATUS=x\nKPANEL_SYSTEM_RESOURCE_VERSION=x\n")
	if trustedKejilionSystemResourceContent(versionOne) {
		t.Fatal("v1 protocol without stable firewall versioning was trusted")
	}
	versionTwo := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"2\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE=1\nkpanel_system_resource_dispatch() { :; }\nKPANEL_SYSTEM_RESOURCE_STATUS=x\nKPANEL_SYSTEM_RESOURCE_VERSION=x\n")
	if trustedKejilionSystemResourceContent(versionTwo) {
		t.Fatal("v2 protocol with an unsafe shared lock path was trusted")
	}
	current := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"4\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE=1\nkpanel_system_resource_dispatch() { :; }\nKPANEL_SYSTEM_RESOURCE_STATUS=x\nKPANEL_SYSTEM_RESOURCE_VERSION=x\n")
	if !trustedKejilionSystemResourceContent(current) {
		t.Fatal("current protocol markers were rejected")
	}
	versionThree := []byte(strings.Replace(string(current), `PROTOCOL_VERSION="4"`, `PROTOCOL_VERSION="3"`, 1))
	if !trustedKejilionSystemResourceContent(versionThree) {
		t.Fatal("compatible v3 protocol was rejected")
	}
	for _, invalid := range []string{
		strings.Replace(string(current), `PROTOCOL_VERSION="4"`, `PROTOCOL_VERSION="5"`, 1),
		string(current) + "KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"3\"\n",
		strings.Replace(string(current), `permission_granted="true"`, `permission_granted="false"`, 1),
	} {
		if trustedKejilionSystemResourceContent([]byte(invalid)) {
			t.Fatal("unsupported, ambiguous or unlicensed protocol was trusted")
		}
	}
}

func TestSystemResourceWriteRejectsStaleVersionBeforeScript(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	runner := &fakeRunner{}
	manager, etcRoot, _, _ := testManager(t, runner)
	mustWrite(t, filepath.Join(etcRoot, "hosts"), "127.0.0.1 localhost\n")
	script := trustedResourceScript(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	_, err := manager.ExecuteSystemResourceAction(context.Background(), contract.SystemResourceActionRequest{
		Action: "hosts-add", Address: "192.0.2.1", Hostnames: []string{"host.example"},
		ExpectedResourceVersion: strings.Repeat("b", 64),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version error=%v", err)
	}
	for _, command := range runner.commands {
		if strings.HasPrefix(command, "env KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1") {
			t.Fatalf("stale request reached script: %s", command)
		}
	}
}

func TestSystemResourceWriteUsesFixedArgvAndVerifiesReceipt(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	runner := &fakeRunner{}
	manager, etcRoot, _, _ := testManager(t, runner)
	hostsPath := filepath.Join(etcRoot, "hosts")
	original := "127.0.0.1 localhost\n"
	updated := original + "192.0.2.1 host.example # managed\n"
	mustWrite(t, hostsPath, original)
	script := trustedResourceScript(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	runner.run = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		if name != "env" {
			t.Fatalf("unexpected action command=%s %#v", name, arguments)
		}
		mustWrite(t, hostsPath, updated)
		return []byte("KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + resourceHash([]byte(updated)) + "\n"), nil
	}
	result, err := manager.ExecuteSystemResourceAction(context.Background(), contract.SystemResourceActionRequest{
		Action: "hosts-add", Address: "192.0.2.1", Hostnames: []string{"host.example"}, Comment: "managed",
		ExpectedResourceVersion: resourceHash([]byte(original)),
	})
	if err != nil || !result.Changed || result.ResourceVersion != resourceHash([]byte(updated)) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	command := strings.Join(runner.commands, "\n")
	for _, expected := range []string{
		"env KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1 bash " + script + " kpanel system-resource hosts add",
		resourceHash([]byte(original)) + " 192.0.2.1 host.example managed",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("command missing %q:\n%s", expected, command)
		}
	}
	if strings.Contains(command, "sh -c") || strings.Contains(command, "bash -c") {
		t.Fatalf("action used a shell command string: %s", command)
	}
}

func TestNetworkResourceInvocationUsesSingularDomainAndAdminState(t *testing.T) {
	enabled := true
	resource, action, arguments, input := systemResourceInvocation(contract.SystemResourceActionRequest{
		Action: "network-interface-state", InterfaceName: "lo", Enabled: &enabled,
	})
	if resource != "network-interface" || action != "state" || strings.Join(arguments, "|") != "lo|up" || input != nil {
		t.Fatalf("unexpected invocation: %q %q %#v input=%q", resource, action, arguments, input)
	}
}

func TestCronResourceInvocationKeepsCommandOutOfProcessArgv(t *testing.T) {
	secretCommand := "curl -H 'Authorization: Bearer secret-token' https://example.invalid/task"
	resource, action, arguments, input := systemResourceInvocation(contract.SystemResourceActionRequest{
		Action: "cron-update", Line: 7, Expression: "0 2 * * *", Command: secretCommand,
	})
	joined := strings.Join(arguments, " ")
	if resource != "cron" || action != "update" || joined != "7 0 2 * * * --command-stdin" {
		t.Fatalf("unexpected cron invocation: %q %q %#v", resource, action, arguments)
	}
	if strings.Contains(joined, "secret-token") || string(input) != secretCommand+"\n" {
		t.Fatalf("cron command transport argv=%q stdin=%q", joined, input)
	}
}

func TestCountryFirewallResourceInvocationUsesCountryCodeOnly(t *testing.T) {
	resource, action, arguments, input := systemResourceInvocation(contract.SystemResourceActionRequest{
		Action: "firewall-block-country", CountryCode: "US",
	})
	if resource != "firewall" || action != "block-country" || strings.Join(arguments, "|") != "US" || input != nil {
		t.Fatalf("unexpected country invocation: %q %q %#v input=%q", resource, action, arguments, input)
	}
}

func TestCronWriteCarriesCommandOnlyInStdin(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	secretCommand := "curl -H 'Authorization: Bearer secret-token' https://example.invalid/task"
	original := []byte("0 1 * * * /usr/bin/old\n")
	updated := []byte("0 1 * * * /usr/bin/old\n0 2 * * * " + secretCommand + "\n")
	current := append([]byte(nil), original...)
	runner := &fakeRunner{}
	manager, _, _, _ := testManager(t, runner)
	script := trustedResourceScript(t)
	manager.resourceScript = func() (string, error) { return script, nil }
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	runner.run = func(actionContext context.Context, name string, arguments ...string) ([]byte, error) {
		switch name {
		case "crontab":
			return append([]byte(nil), current...), nil
		case "env":
			cancelRequest()
			if actionContext.Err() != nil {
				t.Fatalf("browser cancellation reached privileged transaction: %v", actionContext.Err())
			}
			current = append([]byte(nil), updated...)
			return []byte("KPANEL_SYSTEM_RESOURCE_STATUS applied\nKPANEL_SYSTEM_RESOURCE_VERSION " + resourceHash(updated) + "\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command %s %#v", name, arguments)
		}
	}

	result, err := manager.ExecuteSystemResourceAction(requestContext, contract.SystemResourceActionRequest{
		Action: "cron-add", Expression: "0 2 * * *", Command: secretCommand,
		ExpectedResourceVersion: resourceHash(original),
	})
	if err != nil || !result.Changed || result.ResourceVersion != resourceHash(updated) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if strings.Contains(strings.Join(runner.commands, "\n"), "secret-token") {
		t.Fatalf("cron secret leaked into argv: %#v", runner.commands)
	}
	foundInput := false
	for _, input := range runner.resourceInputs {
		if string(input) == secretCommand+"\n" {
			foundInput = true
		}
	}
	if !foundInput {
		t.Fatalf("cron stdin frame not observed: %#v", runner.resourceInputs)
	}
}

func TestSystemResourceReadCapabilityDoesNotDependOnWriteAdapter(t *testing.T) {
	manager, etcRoot, _, _ := testManager(t, &fakeRunner{})
	mustWrite(t, filepath.Join(etcRoot, "hosts"), "127.0.0.1 localhost\n")
	manager.enabled = false
	read := findCapability(manager.SystemResourceCapabilities(), "system.hosts.read")
	write := findCapability(manager.SystemResourceCapabilities(), "system.hosts.write")
	if !read.Enabled || write.Enabled || write.Reason == "" {
		t.Fatalf("read=%#v write=%#v", read, write)
	}
	capabilities := manager.SystemResourceCapabilities()
	for _, id := range []string{
		"system.hosts.read", "system.hosts.write", "system.cron.read", "system.cron.write",
		"system.network-interfaces.read", "system.network-interfaces.write",
		"system.firewall.read", "system.firewall.write",
	} {
		if findCapability(capabilities, id).ID != id {
			t.Fatalf("capability %q missing: %#v", id, capabilities)
		}
	}
}

func TestSystemResourceCapabilitiesValidateSharedScriptOnce(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	manager, etcRoot, _, _ := testManager(t, &fakeRunner{})
	mustWrite(t, filepath.Join(etcRoot, "hosts"), "127.0.0.1 localhost\n")
	script := trustedResourceScript(t)
	calls := 0
	manager.resourceScript = func() (string, error) {
		calls++
		return script, nil
	}
	_ = manager.SystemResourceCapabilities()
	if calls != 1 {
		t.Fatalf("shared script finder calls=%d want=1", calls)
	}
}

func trustedResourceScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kejilion.sh")
	content := strings.Join([]string{
		"#!/usr/bin/env bash",
		"permission_granted=\"true\"",
		"KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"4\"",
		"KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1",
		"kpanel_system_resource_dispatch() { :; }",
		"KPANEL_SYSTEM_RESOURCE_STATUS=applied",
		"KPANEL_SYSTEM_RESOURCE_VERSION=" + strings.Repeat("a", 64),
		strings.Repeat("# trusted protocol padding\n", 64),
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
