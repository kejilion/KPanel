package systemmanage

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestParseSSHDefenseSnapshot(t *testing.T) {
	version := strings.Repeat("a", 64)
	event := hex.EncodeToString([]byte("2026-08-11 01:02:04,000 fail2ban.actions [1]: NOTICE [sshd] Ban 198.51.100.8"))
	output := strings.Join([]string{
		"KPANEL_F2B_PROTOCOL 1",
		"KPANEL_F2B_MANAGER_STATUS=ok",
		"KPANEL_F2B_MANAGER_VERSION=" + version,
		"KPANEL_F2B_MANAGER_INSTALLED=true",
		"KPANEL_F2B_MANAGER_RUNNING=true",
		"KPANEL_F2B_MANAGER_ENABLED=true",
		"KPANEL_F2B_MANAGER_AUTOSTART=true",
		"KPANEL_F2B_MANAGER_JAIL=sshd",
		"KPANEL_F2B_MANAGER_CURRENT_FAILED=2",
		"KPANEL_F2B_MANAGER_TOTAL_FAILED=18",
		"KPANEL_F2B_MANAGER_CURRENT_BANNED=2",
		"KPANEL_F2B_MANAGER_TOTAL_BANNED=5",
		"KPANEL_F2B_MANAGER_BANTIME=3600",
		"KPANEL_F2B_MANAGER_FINDTIME=600",
		"KPANEL_F2B_MANAGER_MAXRETRY=5",
		"KPANEL_F2B_MANAGER_PROFILE=standard",
		"KPANEL_F2B_MANAGER_BANS_TRUNCATED=false",
		"KPANEL_F2B_MANAGER_BAN=198.51.100.8",
		"KPANEL_F2B_MANAGER_BAN=2001:db8::8",
		"KPANEL_F2B_MANAGER_TRUSTED=127.0.0.1/8",
		"KPANEL_F2B_MANAGER_EVENT_HEX=" + event,
		"",
	}, "\n")
	snapshot, err := parseSSHDefenseSnapshot([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != version || snapshot.Profile != "standard" || snapshot.MaxRetry != 5 || len(snapshot.BannedIPs) != 2 || len(snapshot.TrustedAddresses) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.RecentEvents) != 1 || snapshot.RecentEvents[0].Action != "ban" || snapshot.RecentEvents[0].Address != "198.51.100.8" {
		t.Fatalf("events = %#v", snapshot.RecentEvents)
	}
}

func TestParseSSHDefenseSnapshotRejectsInconsistentAndNoisyOutput(t *testing.T) {
	base := strings.Join([]string{
		"KPANEL_F2B_MANAGER_STATUS=ok",
		"KPANEL_F2B_MANAGER_VERSION=" + strings.Repeat("a", 64),
		"KPANEL_F2B_MANAGER_INSTALLED=false",
		"KPANEL_F2B_MANAGER_RUNNING=false",
		"KPANEL_F2B_MANAGER_ENABLED=false",
		"KPANEL_F2B_MANAGER_AUTOSTART=false",
		"KPANEL_F2B_MANAGER_JAIL=sshd",
		"KPANEL_F2B_MANAGER_CURRENT_FAILED=0",
		"KPANEL_F2B_MANAGER_TOTAL_FAILED=0",
		"KPANEL_F2B_MANAGER_CURRENT_BANNED=0",
		"KPANEL_F2B_MANAGER_TOTAL_BANNED=0",
		"KPANEL_F2B_MANAGER_BANTIME=3600",
		"KPANEL_F2B_MANAGER_FINDTIME=600",
		"KPANEL_F2B_MANAGER_MAXRETRY=5",
		"KPANEL_F2B_MANAGER_PROFILE=standard",
		"KPANEL_F2B_MANAGER_BANS_TRUNCATED=false",
	}, "\n")
	for name, output := range map[string]string{
		"noise":         base + "\ninstallation complete\n",
		"duplicate":     base + "\nKPANEL_F2B_MANAGER_PROFILE=standard\n",
		"bad address":   base + "\nKPANEL_F2B_MANAGER_TRUSTED=localhost\n",
		"bad installed": strings.Replace(base, "INSTALLED=false", "INSTALLED=true\nKPANEL_F2B_MANAGER_RUNNING=true", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSSHDefenseSnapshot([]byte(output)); err == nil {
				t.Fatalf("invalid output accepted: %s", output)
			}
		})
	}
}

func TestParseSSHDefenseSnapshotReturnsStableEmptyCollections(t *testing.T) {
	output := strings.Join([]string{
		"KPANEL_F2B_MANAGER_STATUS=ok",
		"KPANEL_F2B_MANAGER_VERSION=" + strings.Repeat("a", 64),
		"KPANEL_F2B_MANAGER_INSTALLED=false",
		"KPANEL_F2B_MANAGER_RUNNING=false",
		"KPANEL_F2B_MANAGER_ENABLED=false",
		"KPANEL_F2B_MANAGER_AUTOSTART=false",
		"KPANEL_F2B_MANAGER_JAIL=sshd",
		"KPANEL_F2B_MANAGER_CURRENT_FAILED=0",
		"KPANEL_F2B_MANAGER_TOTAL_FAILED=0",
		"KPANEL_F2B_MANAGER_CURRENT_BANNED=0",
		"KPANEL_F2B_MANAGER_TOTAL_BANNED=0",
		"KPANEL_F2B_MANAGER_BANTIME=3600",
		"KPANEL_F2B_MANAGER_FINDTIME=600",
		"KPANEL_F2B_MANAGER_MAXRETRY=5",
		"KPANEL_F2B_MANAGER_PROFILE=standard",
		"KPANEL_F2B_MANAGER_BANS_TRUNCATED=false",
	}, "\n")
	snapshot, err := parseSSHDefenseSnapshot([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BannedIPs == nil || snapshot.TrustedAddresses == nil || snapshot.RecentEvents == nil {
		t.Fatalf("empty collections must be non-nil: %#v", snapshot)
	}
}

func TestTrustedSSHDefenseManagerProtocolRequiresExactVersion(t *testing.T) {
	base := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"4\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE\nkpanel_system_resource_dispatch\nKPANEL_SYSTEM_RESOURCE_STATUS\nKPANEL_SYSTEM_RESOURCE_VERSION\n")
	if trustedKejilionSSHDefenseManagerContent(base) {
		t.Fatal("system-resource-only script was trusted for SSH defense")
	}
	current := append(base, []byte("KPANEL_F2B_MANAGER_PROTOCOL_VERSION=\"1\"\nKJ_F2B_NONINTERACTIVE\nkpanel_f2b_manager_dispatch\nKPANEL_F2B_MANAGER_STATUS\n")...)
	if !trustedKejilionSSHDefenseManagerContent(current) {
		t.Fatal("current SSH defense protocol was rejected")
	}
	legacy := strings.Replace(string(current), `KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"`, `KPANEL_F2B_MANAGER_PROTOCOL_VERSION="0"`, 1)
	if trustedKejilionSSHDefenseManagerContent([]byte(legacy)) {
		t.Fatal("legacy SSH defense protocol was trusted")
	}
}

func TestSSHDefenseManagerInvocationUsesFixedArguments(t *testing.T) {
	request := contract.SSHDefenseActionRequest{
		Action: "add-trusted", ExpectedResourceVersion: strings.Repeat("a", 64), Address: "203.0.113.0/24",
	}
	arguments := strings.Join(sshDefenseManagerInvocation(request), " ")
	if arguments != "add-trusted "+strings.Repeat("a", 64)+" 203.0.113.0/24" {
		t.Fatalf("arguments = %q", arguments)
	}
	for _, fragment := range []string{"sh -c", ";", "$", "`"} {
		if strings.Contains(arguments, fragment) {
			t.Fatalf("unsafe argument fragment %q in %q", fragment, arguments)
		}
	}
}

func TestParseSSHDefenseManagerReceiptRejectsUnknownProtocolFields(t *testing.T) {
	version := strings.Repeat("a", 64)
	valid := []byte("KPANEL_F2B_MANAGER_PROTOCOL 1\nKPANEL_F2B_MANAGER_STATUS=applied\nKPANEL_F2B_MANAGER_VERSION=" + version + "\nKPANEL_F2B_MANAGER_ENABLED=true\n")
	receipt, err := parseSSHDefenseManagerReceipt(valid)
	if err != nil || receipt.Status != "applied" || receipt.Version != version {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	unknown := append(valid, []byte("KPANEL_F2B_MANAGER_COMMAND_OUTPUT=accepted\n")...)
	if _, err := parseSSHDefenseManagerReceipt(unknown); err == nil {
		t.Fatal("unknown protocol field was accepted")
	}
}
