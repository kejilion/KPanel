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
	if err != nil || !slices.Contains(response.Capabilities, LightHealthCapability) {
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

func TestUnsupportedLightHostProjectionHasNoDerivedCapabilities(t *testing.T) {
	now := time.Now().UTC()
	snapshot := HostSnapshot{ReceivedAt: now,
		Telemetry: contract.HostTelemetry{OSID: "debian", OSLike: []string{"debian"}}}
	record := lightHostRecord{Platform: "unsupported", LastSnapshot: &snapshot}
	host := publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "unknown" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable ||
		host.LastSnapshot != nil || host.LightHealth != nil {
		t.Fatalf("unsupported host = %#v", host)
	}
	copy := cloneSnapshot(&snapshot)
	copy.Telemetry.OSLike[0] = "alpine"
	if snapshot.Telemetry.OSLike[0] != "debian" {
		t.Fatal("snapshot telemetry aliases source")
	}
	body, err := json.Marshal(snapshot)
	if err != nil || bytes.Contains(body, []byte("\"platform\"")) {
		t.Fatalf("ephemeral platform metadata was persisted: %s, %v", body, err)
	}
	var restarted HostSnapshot
	if err := json.Unmarshal(body, &restarted); err != nil {
		t.Fatal(err)
	}
	record.LastSnapshot = &restarted
	host = publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "unknown" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable || host.LastSnapshot != nil {
		t.Fatalf("restart must retain display but fail closed: %#v", host)
	}
	record.LastSnapshot = &snapshot
	if lightNodePlatformSupported(record) {
		t.Fatal("unsupported platform regained feature access")
	}
}

func TestUnsupportedReportPlatformIsRejectedButLegacyLinuxRemainsAccepted(t *testing.T) {
	unsupported := LightReportRequest{Platform: "unsupported", Telemetry: contract.HostTelemetry{
		OS: "Debian GNU/Linux 13", OSID: "debian",
	}}
	if err := validateLightPlatform(unsupported); !errors.Is(err, ErrLightPlatformUnsupported) {
		t.Fatalf("unsupported report = %v", err)
	}
	unsupported.Platform = "linux"
	if err := validateLightPlatform(unsupported); err != nil {
		t.Fatalf("Linux report rejected: %v", err)
	}
	if err := validateLightPlatform(LightReportRequest{Telemetry: contract.HostTelemetry{
		OS: "Ubuntu 26.04 LTS", OSID: "ubuntu", OSLike: []string{"debian"},
	}}); err != nil {
		t.Fatalf("legacy Linux rejected: %v", err)
	}
}

func TestStoredUnsupportedLightNodeCannotSubmitReports(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	service := newLightServiceForTest(t, &serviceTestClock{now: now})
	legacySnapshot := &HostSnapshot{Telemetry: contract.HostTelemetry{OS: "Windows Server 2025", OSID: "windows"}}
	for index, record := range []lightHostRecord{
		{Platform: "unsupported"},
		{LastSnapshot: legacySnapshot},
	} {
		nodeID := strings.Repeat(string(rune('f'-index)), 32)
		key := bytes.Repeat([]byte{byte(7 + index)}, 32)
		record.ID, record.Name = nodeID, "legacy unsupported"
		record.CreatedAt, record.UpdatedAt = now, now
		if err := service.light.AddHost(record, key); err != nil {
			t.Fatalf("AddHost(%d) error = %v", index, err)
		}
		response := LightEnrollResponse{NodeID: nodeID, ReportingKey: base64.RawURLEncoding.EncodeToString(key)}
		input := LightReportRequest{Telemetry: serviceTelemetry(now, "legacy unsupported")}
		body, auth := signedLightReportForTest(t, now, response, input, strings.Repeat(string(rune('e'+index)), 32))
		if _, err := service.AcceptLightReport(auth, body, input); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("unsupported stored node %d report error = %v", index, err)
		}
	}

	// A platform-less record with no snapshot is a valid v1.24 migration shape.
	// OSLike alone must not let a Windows identity self-assert back into Linux.
	unknownID := strings.Repeat("a", 32)
	unknownKey := bytes.Repeat([]byte{9}, 32)
	if err := service.light.AddHost(lightHostRecord{
		ID: unknownID, Name: "legacy unknown", CreatedAt: now, UpdatedAt: now,
	}, unknownKey); err != nil {
		t.Fatal(err)
	}
	response := LightEnrollResponse{NodeID: unknownID, ReportingKey: base64.RawURLEncoding.EncodeToString(unknownKey)}
	telemetry := serviceTelemetry(now, "legacy unknown")
	telemetry.OS, telemetry.OSID, telemetry.OSLike = "Windows Server 2025", "windows", []string{"debian"}
	input := LightReportRequest{Telemetry: telemetry}
	body, auth := signedLightReportForTest(t, now, response, input, strings.Repeat("d", 32))
	if _, err := service.AcceptLightReport(auth, body, input); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("platform-less record with non-Linux OSLike was accepted: %v", err)
	}
}

func TestUnknownLightPlatformFailsClosedUntilFirstSnapshot(t *testing.T) {
	now := time.Now().UTC()
	unknown := lightHostRecord{}
	if lightNodePlatformSupported(unknown) {
		t.Fatal("unknown platform was allowed")
	}
	host := publicLightHostWithCapabilities(unknown, now, true, true)
	if host.Platform != "unknown" || host.State != HostOffline || host.TerminalAvailable || host.FileManagementAvailable {
		t.Fatalf("unknown host projection = %#v", host)
	}
	linux := lightHostRecord{LastSnapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{
		OS: "Debian GNU/Linux 13", OSID: "debian",
	}}}
	if !lightNodePlatformSupported(linux) {
		t.Fatal("a reported Linux platform remained blocked")
	}
}

func TestLegacyPlatformlessSnapshotRequiresPositiveLinuxIdentity(t *testing.T) {
	legacyLinux := lightHostRecord{LastSnapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{
		OS: "Ubuntu 26.04 LTS", OSID: "ubuntu", OSLike: []string{"debian"},
	}}}
	if !lightNodePlatformSupported(legacyLinux) {
		t.Fatal("legacy Linux snapshot was not recognized")
	}
	legacyOther := lightHostRecord{LastSnapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{
		OS: "Windows Server 2025", OSID: "windows", OSLike: []string{"debian"},
	}}}
	if lightNodePlatformSupported(legacyOther) {
		t.Fatal("legacy non-Linux snapshot was treated as Linux")
	}
	contradictory := lightHostRecord{LastSnapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{
		OSID: "linux", OSLike: []string{"windows"},
	}}}
	if lightNodePlatformSupported(contradictory) {
		t.Fatal("contradictory Windows marker was treated as Linux")
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
	if !lightNodePlatformSupported(record) {
		t.Fatal("new Linux enrollment is blocked before its first report")
	}
	host := publicLightHostWithCapabilities(record, now, true, true)
	if host.Platform != "linux" || host.TerminalShell != "posix" || !host.TerminalAvailable || !host.FileManagementAvailable {
		t.Fatalf("new Linux enrollment projection = %#v", host)
	}
}
