package panel

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestSystemActionRequiresProtectedSessionAndForwardsTypedRequest(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"hostname","status":"succeeded","changed":false,"message":"unchanged","appliedAt":"2026-07-26T03:00:00Z"}`),
	}}
	server.agent = agent
	body := []byte(`{"action":"hostname","hostname":"Web-01.Example"}`)

	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/actions", body, true,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("system action returned %d: %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/v1/system/actions" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	var forwarded contract.SystemActionRequest
	if err := json.Unmarshal(calls[0].body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded.Action != "hostname" || forwarded.Hostname != "web-01.example" {
		t.Fatalf("unexpected forwarded request: %#v", forwarded)
	}

	missingCSRF := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/actions", body, false,
	)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF returned %d", missingCSRF.Code)
	}
	if len(agent.snapshotCalls()) != 1 {
		t.Fatal("request without CSRF reached the Agent")
	}
}

func TestValidateSystemAction(t *testing.T) {
	enabled := true
	tests := []struct {
		name  string
		input contract.SystemActionRequest
		valid bool
	}{
		{"hostname", contract.SystemActionRequest{Action: "hostname", Hostname: "web-01.example"}, true},
		{"hostname injection", contract.SystemActionRequest{Action: "hostname", Hostname: "web;reboot"}, false},
		{"SSH port", contract.SystemActionRequest{Action: "ssh-port", Port: 2222}, true},
		{"enable SSH defense", contract.SystemActionRequest{Action: "ssh-defense", Enabled: &enabled}, true},
		{"missing SSH defense state", contract.SystemActionRequest{Action: "ssh-defense"}, false},
		{"empty DNS", contract.SystemActionRequest{Action: "dns"}, false},
		{"DNS", contract.SystemActionRequest{Action: "dns", Servers: []string{"1.1.1.1", "2606:4700:4700::1111"}}, true},
		{"hosts add", contract.SystemActionRequest{Action: "hosts", HostsOperation: "add", HostsEntry: "192.0.2.10 example.com"}, true},
		{"hosts injection", contract.SystemActionRequest{Action: "hosts", HostsOperation: "add", HostsEntry: "192.0.2.10 example.com;reboot"}, false},
		{"cron add", contract.SystemActionRequest{Action: "cron", CronOperation: "add", CronEntry: "0 2 * * * k update"}, true},
		{"cron multiline", contract.SystemActionRequest{Action: "cron", CronOperation: "add", CronEntry: "0 2 * * * k update\nreboot"}, false},
		{"network up", contract.SystemActionRequest{Action: "network-interface", NetworkOperation: "up", InterfaceName: "eth0"}, true},
		{"network injection", contract.SystemActionRequest{Action: "network-interface", NetworkOperation: "down", InterfaceName: "eth0;reboot"}, false},
		{"firewall port", contract.SystemActionRequest{Action: "firewall", FirewallOperation: "port-open", FirewallPort: 443}, true},
		{"firewall missing port", contract.SystemActionRequest{Action: "firewall", FirewallOperation: "port-close"}, false},
		{"firewall IP", contract.SystemActionRequest{Action: "firewall", FirewallOperation: "ip-block", FirewallAddress: "203.0.113.0/24"}, true},
		{"firewall countries", contract.SystemActionRequest{Action: "firewall", FirewallOperation: "country-block", CountryCodes: []string{"cn", "US"}}, true},
		{"firewall invalid country", contract.SystemActionRequest{Action: "firewall", FirewallOperation: "country-allow", CountryCodes: []string{"CHN"}}, false},
		{"timezone traversal", contract.SystemActionRequest{Action: "timezone", Timezone: "../../etc"}, false},
		{"small swap", contract.SystemActionRequest{Action: "swap", SwapSizeMiB: 128}, true},
		{"negative swap", contract.SystemActionRequest{Action: "swap", SwapSizeMiB: -1}, false},
		{"disable swap", contract.SystemActionRequest{Action: "swap", SwapSizeMiB: 0}, true},
		{"mainland mirror", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "cn-default"}, true},
		{"education mirror", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "cn-edu"}, true},
		{"abroad mirror", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "abroad"}, true},
		{"smart mirror", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "smart"}, true},
		{"legacy mirror rejected by Panel", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "aliyun"}, false},
		{"unknown mirror", contract.SystemActionRequest{Action: "mirror", MirrorPreset: "custom"}, false},
		{"high performance kernel profile", contract.SystemActionRequest{Action: "kernel-tuning", Profile: "high"}, true},
		{"stream kernel profile", contract.SystemActionRequest{Action: "kernel-tuning", Profile: "stream"}, true},
		{"game kernel profile", contract.SystemActionRequest{Action: "kernel-tuning", Profile: "game"}, true},
		{"unknown kernel profile", contract.SystemActionRequest{Action: "kernel-tuning", Profile: "automatic"}, false},
		{"BBR", contract.SystemActionRequest{Action: "bbr", Enabled: &enabled}, true},
		{"missing BBR state", contract.SystemActionRequest{Action: "bbr"}, false},
		{"install BBRv3", contract.SystemActionRequest{Action: "bbrv3", MaintenancePolicy: "install"}, true},
		{"update BBRv3", contract.SystemActionRequest{Action: "bbrv3", MaintenancePolicy: "update"}, true},
		{"uninstall BBRv3", contract.SystemActionRequest{Action: "bbrv3", MaintenancePolicy: "uninstall"}, true},
		{"unknown BBRv3 policy", contract.SystemActionRequest{Action: "bbrv3", MaintenancePolicy: "latest"}, false},
		{"BBRv3 with unrelated field", contract.SystemActionRequest{Action: "bbrv3", MaintenancePolicy: "install", Hostname: "ignored"}, false},
		{"system update", contract.SystemActionRequest{Action: "update", MaintenancePolicy: "full"}, true},
		{"unknown update policy", contract.SystemActionRequest{Action: "update", MaintenancePolicy: "security"}, false},
		{"cache cleanup", contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "cache"}, true},
		{"standard cleanup", contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "standard"}, true},
		{"unknown cleanup policy", contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "deep"}, false},
		{"reboot", contract.SystemActionRequest{Action: "reboot"}, true},
		{"reboot with legacy confirmation", contract.SystemActionRequest{Action: "reboot", Confirmation: "REBOOT"}, true},
		{"reboot with unrelated field", contract.SystemActionRequest{Action: "reboot", Confirmation: "REBOOT", Hostname: "ignored"}, false},
		{"arbitrary command", contract.SystemActionRequest{Action: "shell"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, _ := validateSystemAction(&test.input)
			if (field == "") != test.valid {
				t.Fatalf("valid=%v, field=%q", test.valid, field)
			}
		})
	}
}

func TestSystemActionAuditChangeRecordsRebootIntentWithoutConfirmationText(t *testing.T) {
	change := systemActionAuditChange(contract.SystemActionRequest{
		Action: "reboot", Confirmation: "REBOOT",
	})
	if len(change) != 2 || change["action"] != "reboot" || change["requested"] != true {
		t.Fatalf("unexpected reboot audit change: %#v", change)
	}
	if _, leaked := change["confirmation"]; leaked {
		t.Fatal("audit change leaked confirmation text")
	}
}

func TestSystemActionAuditChangeContainsOnlyTypedFields(t *testing.T) {
	input := contract.SystemActionRequest{Action: "ssh-port", Port: 2222, Hostname: "ignored"}
	change := systemActionAuditChange(input)
	if len(change) != 2 || change["action"] != "ssh-port" || change["port"] != uint16(2222) {
		t.Fatalf("unexpected audit change: %#v", change)
	}
	if _, leaked := change["hostname"]; leaked {
		t.Fatal("audit change leaked an unrelated field")
	}
}

func TestSystemActionAuditChangeRecordsSSHDefenseState(t *testing.T) {
	enabled := true
	change := systemActionAuditChange(contract.SystemActionRequest{
		Action: "ssh-defense", Enabled: &enabled,
	})
	if len(change) != 2 || change["action"] != "ssh-defense" || change["enabled"] != true {
		t.Fatalf("unexpected SSH defense audit change: %#v", change)
	}
}
