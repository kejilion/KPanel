package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func testLightHealth(now time.Time, version string) *contract.LightNodeHealth {
	s := contract.LightNodeServiceHealth{LoadState: "loaded", ActiveState: "active", SubState: "running", UnitFileState: "enabled"}
	return &contract.LightNodeHealth{ObservedAt: now, RuntimeVersion: version, Update: contract.LightNodeUpdateHealth{State: "current", CheckedAt: now.Unix() - 5, FinishedAt: now.Unix()}, Services: contract.LightNodeServicesHealth{Timer: s, Telemetry: s, Terminal: s, File: s, SSHLogin: s}}
}

func TestLightHealthIsAuthenticatedEphemeralAndMissingClears(t *testing.T) {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge")
	input := LightReportRequest{Telemetry: serviceTelemetry(now, "edge")}
	input.Health = testLightHealth(now, input.Telemetry.AgentVersion)
	body, auth := signedLightReportForTest(t, now, enrollment, input, strings.Repeat("a", 32))
	bad := auth
	bad.Signature = "bad"
	if _, err := service.AcceptLightReport(bad, body, input); err == nil {
		t.Fatal("forged health accepted")
	}
	if _, err := service.AcceptLightReport(auth, body, input); err != nil {
		t.Fatal(err)
	}
	host, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil || host.LightHealth == nil || host.State != HostOnline {
		t.Fatalf("host %#v %v", host, err)
	}
	host.LightHealth.Update.State = "failed"
	host, _ = service.Host(context.Background(), enrollment.NodeID)
	if host.LightHealth.Update.State != "current" {
		t.Fatal("snapshot alias")
	}
	// Encode the actual persisted state, then prove old strict readers see no
	// extension and restart exposes unknown until another authenticated report.
	encoded, err := json.Marshal(service.light.state)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("lightHealth")) || bytes.Contains(encoded, []byte("runtimeVersion")) {
		t.Fatal("health leaked into legacy state")
	}
	if err := os.WriteFile(service.light.path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := openLightStore(service.light.path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.state.Hosts[0].LastSnapshot.LightHealth != nil {
		t.Fatal("restart retained ephemeral health")
	}
	clock.Advance(30 * time.Second)
	input.Telemetry.CollectedAt = clock.Now()
	input.Health = nil
	body, auth = signedLightReportForTest(t, clock.Now(), enrollment, input, strings.Repeat("b", 32))
	if _, err := service.AcceptLightReport(auth, body, input); err != nil {
		t.Fatal(err)
	}
	host, _ = service.Host(context.Background(), enrollment.NodeID)
	if host.LightHealth != nil || host.State != HostOnline {
		t.Fatal("old node did not clear health or became offline")
	}
}

func TestInvalidLightHealthCannotBreakTelemetryOrRefreshHealth(t *testing.T) {
	for _, mutation := range []func(*contract.LightNodeHealth){
		func(h *contract.LightNodeHealth) { h.Update.ErrorCode = strings.Repeat("secret", 1000) },
		func(h *contract.LightNodeHealth) { h.Services.Timer.LoadState = "custom-command" },
		func(h *contract.LightNodeHealth) { h.ObservedAt = h.ObservedAt.Add(-10 * time.Minute) },
		func(h *contract.LightNodeHealth) { h.RuntimeVersion = "different" },
	} {
		now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
		clock := &serviceTestClock{now: now}
		service := newLightServiceForTest(t, clock)
		enrollment := enrollLightHostForTest(t, service, "edge")
		input := LightReportRequest{Telemetry: serviceTelemetry(now, "edge")}
		input.Health = testLightHealth(now, input.Telemetry.AgentVersion)
		mutation(input.Health)
		body, auth := signedLightReportForTest(t, now, enrollment, input, strings.Repeat("a", 32))
		if _, err := service.AcceptLightReport(auth, body, input); err != nil {
			t.Fatal(err)
		}
		host, _ := service.Host(context.Background(), enrollment.NodeID)
		if host.LightHealth != nil || host.State != HostOnline {
			t.Fatal("invalid health trusted or telemetry rejected")
		}
	}
}
