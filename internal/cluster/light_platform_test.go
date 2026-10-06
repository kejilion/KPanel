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
	if err != nil || slices.Contains(response.Capabilities, "windows-node-v1") ||
		slices.Contains(response.Capabilities, "desktop-v1") || slices.Contains(response.Capabilities, "desktop-managed-v1") ||
		!slices.Contains(response.Capabilities, LightHealthCapability) {
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

func TestWindowsHostIsListedOfflineWithoutDerivedCapabilities(t *testing.T) {
	now := time.Now().UTC()
	snapshot := HostSnapshot{Platform: "windows", ReceivedAt: now,
		UnavailableMetrics: []string{"load"}, NodeCapabilities: []string{"monitoring", "terminal", "files"},
		Telemetry: contract.HostTelemetry{OSID: "windows"}}
	record := lightHostRecord{LastSnapshot: &snapshot}
	host := publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "windows" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable ||
		host.LastSnapshot != nil || host.LightHealth != nil {
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
	if host.Platform != "windows" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable ||
		host.TerminalShell != "" || host.PathStyle != "" || host.LastSnapshot != nil || len(host.UnavailableMetrics) != 0 {
		t.Fatalf("restart must retain display but fail closed: %#v", host)
	}
	record.LastSnapshot = &snapshot
	for _, capability := range []string{"terminal", "files", "desktop", "desktop-managed"} {
		if lightPlatformAllows(record, capability, now) {
			t.Fatalf("withdrawn Windows capability %q accepted", capability)
		}
	}
}

func TestWindowsReportMetadataIsRejectedButLegacyLinuxRemainsAccepted(t *testing.T) {
	windows := LightReportRequest{Platform: "windows", Telemetry: contract.HostTelemetry{OSID: "windows"},
		UnavailableMetrics: []string{"load"}, Capabilities: []string{"monitoring", "terminal"}}
	if err := validateLightPlatform(windows); !errors.Is(err, ErrLightPlatformUnsupported) {
		t.Fatalf("Windows report = %v", err)
	}
	windows.Platform = ""
	if err := validateLightPlatform(windows); !errors.Is(err, ErrLightPlatformUnsupported) {
		t.Fatalf("Windows report without platform marker = %v", err)
	}
	if err := validateLightPlatform(LightReportRequest{Telemetry: contract.HostTelemetry{OSID: "debian"}}); err != nil {
		t.Fatalf("legacy Linux rejected: %v", err)
	}
}

func TestUnknownLightPlatformFailsClosedUntilFirstSnapshot(t *testing.T) {
	now := time.Now().UTC()
	unknown := lightHostRecord{}
	for _, capability := range []string{"terminal", "files", "desktop", "desktop-managed"} {
		if lightPlatformAllows(unknown, capability, now) {
			t.Fatalf("unknown platform capability %q was allowed", capability)
		}
	}
	host := publicLightHostWithCapabilities(unknown, now, true, true)
	if host.Platform != "unknown" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable {
		t.Fatalf("unknown host projection = %#v", host)
	}
	linux := lightHostRecord{LastSnapshot: &HostSnapshot{Platform: "linux", Telemetry: contract.HostTelemetry{OSID: "debian"}}}
	if !lightPlatformAllows(linux, "terminal", now) {
		t.Fatal("a reported Linux platform remained blocked")
	}
}

func TestNewLinuxEnrollmentPersistsPlatformBeforeFirstReport(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	service := newLightServiceForTest(t, &serviceTestClock{now: now})
	enrolled := enrollLightHostForTest(t, service, "edge-before-first-report")

	reopened, err := openLightStore(service.light.path)
	if err != nil {
		t.Fatalf("openLightStore() error = %v", err)
	}
	record, err := reopened.Host(enrolled.NodeID)
	if err != nil {
		t.Fatalf("reopened.Host() error = %v", err)
	}
	if record.Platform != "linux" || record.LastSnapshot != nil {
		t.Fatalf("persisted enrollment = %#v", record)
	}
	for _, capability := range []string{"terminal", "files"} {
		if !lightPlatformAllows(record, capability, now) {
			t.Fatalf("new Linux enrollment cannot start %s before its first report", capability)
		}
	}
	host := publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "linux" || host.TerminalShell != "posix" || host.PathStyle != "posix" ||
		!host.TerminalAvailable || !host.FileManagementAvailable {
		t.Fatalf("new Linux enrollment projection = %#v", host)
	}
}
