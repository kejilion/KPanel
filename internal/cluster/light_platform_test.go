package cluster

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestLightCapabilityProbeIsAuthenticatedReadOnlyAndReplaySafe(t *testing.T) {
	now := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	s := newLightServiceForTest(t, &serviceTestClock{now: now})
	enrolled := enrollLightHostForTest(t, s, "probe")
	key, _ := base64.RawURLEncoding.DecodeString(enrolled.ReportingKey)
	body := []byte("{}")
	auth := LightReportAuth{Source: "198.51.100.1", NodeID: enrolled.NodeID,
		Timestamp: strconv.FormatInt(now.Unix(), 10), RequestID: strings.Repeat("e", 32)}
	auth.Signature = LightRequestSignature(key, "POST", LightCapabilitiesPath, auth.NodeID, auth.Timestamp, auth.RequestID, body)
	wrong := auth
	wrong.Signature = LightRequestSignature(key, "POST", LightFileCapabilityPath, auth.NodeID, auth.Timestamp, auth.RequestID, body)
	if _, err := s.ProbeLightCapabilities(wrong, body); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("cross-endpoint signature accepted: %v", err)
	}
	response, err := s.ProbeLightCapabilities(auth, body)
	if err != nil || !slices.Contains(response.Capabilities, WindowsNodeCapability) {
		t.Fatalf("probe = %#v, %v", response, err)
	}
	if _, err := s.ProbeLightCapabilities(auth, body); !errors.Is(err, ErrReplay) {
		t.Fatalf("replay accepted: %v", err)
	}
	record, _ := s.light.Host(enrolled.NodeID)
	public, err := s.light.ReadTerminalPublicKey(record)
	if err != nil || len(public) != 0 || record.LastSnapshot != nil {
		t.Fatalf("read-only probe mutated enrollment: key=%x, snapshot=%#v, err=%v", public, record.LastSnapshot, err)
	}
}

func TestWindowsMetadataDoesNotChangePersistedSnapshotAndRequiresFreshCapabilities(t *testing.T) {
	now := time.Now().UTC()
	snapshot := HostSnapshot{Platform: "windows", ReceivedAt: now,
		UnavailableMetrics: []string{"load"}, NodeCapabilities: []string{"monitoring", "terminal", "files"},
		Telemetry: contract.HostTelemetry{OSID: "windows"}}
	record := lightHostRecord{LastSnapshot: &snapshot}
	host := publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "windows" || host.TerminalShell != "powershell" || !host.TerminalAvailable || !host.FileManagementAvailable {
		t.Fatalf("fresh Windows host = %#v", host)
	}
	copy := cloneSnapshot(&snapshot)
	copy.NodeCapabilities[0] = "desktop"
	copy.UnavailableMetrics[0] = "swap"
	if snapshot.NodeCapabilities[0] != "monitoring" || snapshot.UnavailableMetrics[0] != "load" {
		t.Fatal("metadata aliases source snapshot")
	}
	body, err := json.Marshal(snapshot)
	if err != nil || bytes.Contains(body, []byte("nodeCapabilities")) || bytes.Contains(body, []byte("unavailableMetrics")) || bytes.Contains(body, []byte("platform")) {
		t.Fatalf("metadata was persisted: %s, %v", body, err)
	}
	var restarted HostSnapshot
	if err := json.Unmarshal(body, &restarted); err != nil {
		t.Fatal(err)
	}
	record.LastSnapshot = &restarted
	host = publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "windows" || host.TerminalAvailable || host.FileManagementAvailable || !slices.Contains(host.UnavailableMetrics, "load") {
		t.Fatalf("restart must retain display but fail closed: %#v", host)
	}
	record.LastSnapshot = &snapshot
	if lightPlatformAllows(record, "terminal", now.Add(91*time.Second)) {
		t.Fatal("stale capability accepted")
	}
}

func TestWindowsReportMetadataValidation(t *testing.T) {
	valid := LightReportRequest{Platform: "windows", Telemetry: contract.HostTelemetry{OSID: "windows"},
		UnavailableMetrics: []string{"load"}, Capabilities: []string{"monitoring", "terminal"}}
	if err := validateLightPlatform(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*LightReportRequest){
		func(v *LightReportRequest) { v.UnavailableMetrics = nil },
		func(v *LightReportRequest) { v.Capabilities = []string{"root"} },
		func(v *LightReportRequest) { v.Capabilities = []string{"terminal", "terminal"} },
		func(v *LightReportRequest) { v.Platform = "" },
		func(v *LightReportRequest) { v.Telemetry.OSID = "linux" },
	} {
		input := valid
		mutate(&input)
		if validateLightPlatform(input) == nil {
			t.Fatalf("invalid metadata accepted: %#v", input)
		}
	}
	if err := validateLightPlatform(LightReportRequest{Telemetry: contract.HostTelemetry{OSID: "debian"}}); err != nil {
		t.Fatalf("legacy Linux rejected: %v", err)
	}
}
