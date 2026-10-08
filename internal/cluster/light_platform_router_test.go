package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestRouterLightNodesResumeReportsAfterRestart(t *testing.T) {
	for _, router := range []struct {
		name, id string
		like     []string
	}{
		{"OpenWrt SNAPSHOT", "openwrt", []string{"lede", "openwrt"}},
		{"LEDE Reboot", "lede", nil},
		{"ImmortalWrt", "immortalwrt", nil},
		{"iStoreOS", "istoreos", nil},
		{"Custom router", "custom-router", []string{"lede", "openwrt"}},
	} {
		for _, mode := range []string{"legacy-snapshot", "new-enrollment"} {
			t.Run(router.id+"/"+mode, func(t *testing.T) {
				now := time.Date(2026, 10, 8, 12, 8, 21, 0, time.UTC)
				clock := &serviceTestClock{now: now}
				config := ServiceConfig{
					DataDir: t.TempDir(), PanelVersion: "test", PublicURL: "https://panel.example",
					Hostname: "center", Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
					Remote: localNodeTestRemote{}, Now: clock.Now,
				}
				service, err := NewService(config)
				if err != nil {
					t.Fatal(err)
				}
				telemetry := serviceTelemetry(now, "router")
				telemetry.OS, telemetry.OSID, telemetry.OSLike = router.name, router.id, router.like
				var enrollment LightEnrollResponse
				if mode == "legacy-snapshot" {
					// v1.24 records have no platform field; the stored OS identifies Linux.
					key := bytes.Repeat([]byte{7}, 32)
					enrollment = LightEnrollResponse{
						NodeID: strings.Repeat("a", 32), ReportingKey: base64.RawURLEncoding.EncodeToString(key),
					}
					if err := service.light.AddHost(lightHostRecord{
						ID: enrollment.NodeID, Name: "router", CreatedAt: now, UpdatedAt: now,
						LastSnapshot: &HostSnapshot{Telemetry: telemetry, ReceivedAt: now}, LastSuccessAt: &now,
					}, key); err != nil {
						t.Fatal(err)
					}
				} else {
					enrollment = enrollLightHostForTest(t, service, "router")
				}
				before, err := service.light.Host(enrollment.NodeID)
				if err != nil {
					t.Fatal(err)
				}
				if err := service.Close(); err != nil {
					t.Fatal(err)
				}
				clock.now = now.Add(10 * time.Minute)
				reopened, err := NewService(config)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = reopened.Close() })
				inventory := reopened.Hosts(context.Background())
				if inventory.RemoteTotal != 1 {
					t.Fatalf("router disappeared after restart: remote total = %d", inventory.RemoteTotal)
				}
				restored, err := reopened.Host(context.Background(), enrollment.NodeID)
				if err != nil || restored.Platform != "linux" {
					t.Fatalf("router lookup after restart: platform=%q, err=%v", restored.Platform, err)
				}
				if !reopened.lightControlAllowed(enrollment.NodeID, "terminal") || !reopened.lightControlAllowed(enrollment.NodeID, "files") {
					t.Fatal("router lost eligibility for its existing authenticated controls")
				}
				telemetry.CollectedAt = clock.Now()
				input := LightReportRequest{Telemetry: telemetry}
				body, auth := signedLightReportForTest(t, clock.Now(), enrollment, input, strings.Repeat("b", 32))
				forged := auth
				forged.Signature = strings.Repeat("A", len(auth.Signature))
				if _, err := reopened.AcceptLightReport(forged, body, input); !errors.Is(err, ErrAuthentication) {
					t.Fatalf("forged router report accepted: %v", err)
				}
				if _, err := reopened.AcceptLightReport(auth, body, input); err != nil {
					t.Fatalf("router could not report with its original credential: %v", err)
				}
				if _, err := reopened.AcceptLightReport(auth, body, input); !errors.Is(err, ErrReplay) {
					t.Fatalf("router report replay accepted: %v", err)
				}
				host, err := reopened.Host(context.Background(), enrollment.NodeID)
				if err != nil || host.State != HostOnline || host.LastSnapshot == nil || host.LastSnapshot.Telemetry.OSID != router.id {
					t.Fatalf("router did not recover: state=%q, snapshot=%v, err=%v", host.State, host.LastSnapshot != nil, err)
				}
				if host.ResourceVersion != before.ResourceVersion {
					t.Fatal("recovery unexpectedly changed the node configuration version")
				}
			})
		}
	}
}

func TestRouterLinuxIdentityDoesNotAdmitWindowsOrUnknownPlatforms(t *testing.T) {
	for name, telemetry := range map[string]contract.HostTelemetry{
		"Windows name":     {OS: "Windows Server", OSID: "openwrt", OSLike: []string{"lede"}},
		"Windows ID":       {OS: "OpenWrt", OSID: "windows", OSLike: []string{"openwrt"}},
		"Windows ancestry": {OS: "iStoreOS", OSID: "istoreos", OSLike: []string{"windows", "openwrt"}},
		"unknown platform": {OS: "Unknown router", OSID: "unknown"},
		"name without ID":  {OS: "OpenWrt router", OSID: "unknown"},
		"substring in ID":  {OS: "Unknown router", OSID: "not-openwrt"},
		"missing identity": {},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateLightPlatform(LightReportRequest{Platform: "linux", Telemetry: telemetry}); !errors.Is(err, ErrLightPlatformUnsupported) {
				t.Fatalf("non-Linux telemetry accepted: %v", err)
			}
			if lightNodePlatformSupported(lightHostRecord{LastSnapshot: &HostSnapshot{Telemetry: telemetry}}) {
				t.Fatal("non-Linux legacy snapshot admitted")
			}
		})
	}
}
