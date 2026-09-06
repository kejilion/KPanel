package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSystemLogCleanupResponsePreservesMaintenanceTaskIdentity(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"action":"log-cleanup","status":"accepted","changed":true,"message":"queued","taskId":"20260824T090000.000000000Z","maintenancePolicy":"retain-3d","appliedAt":"2026-08-24T09:00:00Z"}`),
	}}
	server.agent = agent
	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/actions",
		[]byte(`{"action":"log-cleanup","maintenancePolicy":"retain-3d"}`), true,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("system action returned %d: %s", response.Code, response.Body.String())
	}
	var result contract.SystemActionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "20260824T090000.000000000Z" || result.Action != "log-cleanup" ||
		result.MaintenancePolicy != "retain-3d" || result.Status != "accepted" {
		t.Fatalf("maintenance task identity was not preserved: %#v", result)
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	var forwarded contract.SystemActionRequest
	if err := json.Unmarshal(calls[0].body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded.Action != result.Action || forwarded.MaintenancePolicy != result.MaintenancePolicy {
		t.Fatalf("forwarded=%#v result=%#v", forwarded, result)
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
		{"retain seven day logs", contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-7d"}, true},
		{"retain three day logs", contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-3d"}, true},
		{"cap logs at 500 MiB", contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "max-500m"}, true},
		{"unknown log cleanup policy", contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "forever"}, false},
		{"log cleanup with unrelated field", contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-7d", Hostname: "ignored"}, false},
		{"reboot", contract.SystemActionRequest{Action: "reboot"}, true},
		{"reboot with legacy confirmation", contract.SystemActionRequest{Action: "reboot", Confirmation: "REBOOT"}, true},
		{"reboot with unrelated field", contract.SystemActionRequest{Action: "reboot", Confirmation: "REBOOT", Hostname: "ignored"}, false},
		{"process signal missing identity", contract.SystemActionRequest{Action: "process-signal", PID: 42, Signal: "term"}, false},
		{"process signal unknown signal", contract.SystemActionRequest{Action: "process-signal", PID: 42, StartTimeTicks: 99, Signal: "stop"}, false},
		{"process signal unrelated field", contract.SystemActionRequest{Action: "process-signal", PID: 42, StartTimeTicks: 99, Signal: "term", Hostname: "ignored"}, false},
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

func TestSystemLogReadPathsAreExactAndPreserveStrictQuery(t *testing.T) {
	for publicPath, expected := range map[string]string{
		"/api/v1/system/logs/summary": "/v1/system/logs/summary",
		"/api/v1/system/logs":         "/v1/system/logs",
	} {
		path, ok := allowedAgentPath(publicPath)
		if !ok || path != expected {
			t.Fatalf("allowedAgentPath(%q) = %q, %v; want %q", publicPath, path, ok, expected)
		}
	}
	for _, path := range []string{
		"/api/v1/system/logs/summary/extra",
		"/api/v1/system/logs/../summary",
		"/api/v1/system/logs/entries",
	} {
		if mapped, ok := allowedAgentPath(path); ok {
			t.Fatalf("unsafe system log path %q mapped to %q", path, mapped)
		}
	}

	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"source":"service","entries":[],"truncated":false,"observedAt":"2026-08-24T09:00:00Z"}`),
	}}
	server.agent = agent
	query := "source=service&limit=50&priority=warning"
	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodGet, "/api/v1/system/logs?"+query, nil, true,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("logs proxy status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet || calls[0].path != "/v1/system/logs" || calls[0].rawQuery != query {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	if strings.Contains(calls[0].rawQuery, "path=") {
		t.Fatal("unexpected arbitrary path reached Agent")
	}
	encoded := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodGet, "/api/v1/system%2flogs?source=system&limit=50&priority=all", nil, true,
	)
	if encoded.Code != http.StatusNotFound || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("encoded system log path status=%d calls=%#v", encoded.Code, agent.snapshotCalls())
	}
}

func TestOverviewReadPathsAreExactAndRequireSession(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	for _, suffix := range []string{"runtime", "management/config", "management/ssh-defense", "management/bbrv3"} {
		path := "/api/v1/system/" + suffix
		want := "/v1/system/" + suffix
		if mapped, ok := allowedAgentPath(path); !ok || mapped != want {
			t.Fatalf("%s mapped to %s, %v", path, mapped, ok)
		}
		for _, invalid := range []string{path + "/extra", path + "/../actions"} {
			if _, ok := allowedAgentPath(invalid); ok {
				t.Fatalf("unexpected allowed route %s", invalid)
			}
		}
		server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"state":{},"observedAt":"2026-09-07T00:00:00Z"}`)}}
		unauthorized := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = "panel.test"
		server.ServeHTTP(unauthorized, request)
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated read %s: %d", path, unauthorized.Code)
		}
		response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, path, nil, true)
		if response.Code != http.StatusOK {
			t.Fatalf("authenticated read %s: %d", path, response.Code)
		}
	}
}

func TestSystemLogCleanupAuditContainsOnlyPolicy(t *testing.T) {
	change := systemActionAuditChange(contract.SystemActionRequest{
		Action: "log-cleanup", MaintenancePolicy: "retain-3d", Hostname: "ignored",
	})
	if len(change) != 2 || change["action"] != "log-cleanup" || change["maintenancePolicy"] != "retain-3d" {
		t.Fatalf("unexpected log cleanup audit change: %#v", change)
	}
	if _, leaked := change["hostname"]; leaked {
		t.Fatal("log cleanup audit leaked unrelated field")
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

func TestSystemActionAuditChangeRecordsOnlyProcessIdentityAndSignal(t *testing.T) {
	change := systemActionAuditChange(contract.SystemActionRequest{
		Action: "process-signal", PID: 42, StartTimeTicks: 99, Signal: "term", Hostname: "ignored",
	})
	if len(change) != 4 || change["pid"] != 42 || change["startTimeTicks"] != uint64(99) || change["signal"] != "term" {
		t.Fatalf("unexpected process audit change: %#v", change)
	}
	if _, leaked := change["hostname"]; leaked {
		t.Fatal("process audit change leaked an unrelated field")
	}
}
