package cluster

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newLightServiceForTest(t *testing.T, clock *serviceTestClock) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), PanelVersion: "0.40.0", PublicURL: "https://panel.example",
		Hostname: "center", Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
		Remote: localNodeTestRemote{}, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func enrollLightHostForTest(t *testing.T, service *Service, name string) LightEnrollResponse {
	t.Helper()
	enrollment, err := service.CreateLightEnrollment()
	if err != nil {
		t.Fatalf("CreateLightEnrollment() error = %v", err)
	}
	fields := strings.Fields(enrollment.Command)
	if len(fields) == 0 {
		t.Fatalf("enrollment command is empty: %q", enrollment.Command)
	}
	token := strings.Trim(fields[len(fields)-1], "'")
	response, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: token, Name: name, NodeVersion: "0.40.0",
	})
	if err != nil {
		t.Fatalf("EnrollLightNode() error = %v", err)
	}
	return response
}

func signedLightReportForTest(
	t *testing.T,
	now time.Time,
	response LightEnrollResponse,
	telemetry LightReportRequest,
	requestID string,
) ([]byte, LightReportAuth) {
	t.Helper()
	body, err := json.Marshal(telemetry)
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawURLEncoding.DecodeString(response.ReportingKey)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(now.UTC().Unix(), 10)
	bodyHash := sha256.Sum256(body)
	material := strings.Join([]string{
		"POST", lightReportPath, response.NodeID, timestamp, requestID, hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(material))
	return body, LightReportAuth{
		Source: "198.51.100.10", NodeID: response.NodeID, Timestamp: timestamp, RequestID: requestID,
		Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
	}
}

func TestLightEnrollmentIsHTTPSBoundOneUseAndPreservesValidTokenAfterBadInput(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)

	enrollment, err := service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(enrollment.Command, "curl -fsSL https://kejilion.sh) kpanel node join 'kpl1.") ||
		!enrollment.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("unexpected enrollment: %#v", enrollment)
	}
	token := strings.Trim(strings.Fields(enrollment.Command)[len(strings.Fields(enrollment.Command))-1], "'")
	tokenPayload, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, lightTokenPrefix))
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	malformedToken := lightTokenPrefix + base64.RawURLEncoding.EncodeToString(append(tokenPayload, []byte("\n{}")...))
	if _, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: malformedToken, Name: "edge-invalid", NodeVersion: "0.40.0",
	}); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("multi-value token error = %v, want ErrPairingCode", err)
	}
	if _, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: token, Name: "bad\nname", NodeVersion: "0.40.0",
	}); err == nil {
		t.Fatal("invalid host name was accepted")
	}
	first, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: token, Name: "edge-1", NodeVersion: "0.40.0",
	})
	if err != nil {
		t.Fatalf("valid enrollment after bad input error = %v", err)
	}
	if _, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: token, Name: "edge-2", NodeVersion: "0.40.0",
	}); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("reused enrollment error = %v, want ErrPairingCode", err)
	}
	host, err := service.Host(context.Background(), first.NodeID)
	if err != nil || host.Kind != HostKindLightNode || host.Origin != "" || host.State != HostUnknown {
		t.Fatalf("unexpected light host after enrollment: %#v, %v", host, err)
	}
}

func TestLightEnrollmentCommandCarriesOptionalDisplayName(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)

	enrollment, err := service.CreateLightEnrollmentForOriginAndName("https://panel.example", "英国AMR's")
	if err != nil {
		t.Fatalf("CreateLightEnrollmentForOriginAndName() error = %v", err)
	}
	if !strings.HasSuffix(enrollment.Command, " --name '英国AMR'\\''s'") {
		t.Fatalf("optional display name missing or not shell-quoted: %q", enrollment.Command)
	}
}

func TestLightEnrollmentAcceptsLegacyThirtyMinuteTokenDuringRollingUpgrade(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	id := strings.Repeat("a", 32)
	secret := []byte("0123456789abcdef0123456789abcdef")
	expiresAt := now.Add(30 * time.Minute)
	hash := sha256.Sum256(secret)
	if err := service.light.AddEnrollment(lightEnrollmentRecord{
		ID: id, SecretHash: hex.EncodeToString(hash[:]), ExpiresAt: expiresAt,
	}, now); err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(lightTokenWire{
		Version: 1, Origin: "https://panel.example", ID: id,
		Secret: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	token := lightTokenPrefix + base64.RawURLEncoding.EncodeToString(wire)
	if _, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: token, Name: "legacy-edge", NodeVersion: "0.37.1",
	}); err != nil {
		t.Fatalf("legacy 30-minute token rejected during rolling upgrade: %v", err)
	}

	tooLong, err := json.Marshal(lightTokenWire{
		Version: 1, Origin: "https://panel.example", ID: strings.Repeat("b", 32),
		Secret: base64.RawURLEncoding.EncodeToString(secret), ExpiresAt: now.Add(61 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseLightToken(lightTokenPrefix+base64.RawURLEncoding.EncodeToString(tooLong), now); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("overlong token error = %v, want ErrPairingCode", err)
	}
}

func TestLightReportAuthenticatesBeforeReplayAndUpdatesState(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge-1")
	beforeReport, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	input := LightReportRequest{Telemetry: serviceTelemetry(now, "edge-1")}
	requestID := strings.Repeat("a", 32)
	body, auth := signedLightReportForTest(t, now, enrollment, input, requestID)
	auth.ReportLatencyMilliseconds = "42"

	badAuth := auth
	badAuth.Signature = strings.Repeat("A", len(auth.Signature))
	if _, err := service.AcceptLightReport(badAuth, body, input); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("bad signature error = %v, want ErrAuthentication", err)
	}
	if _, err := service.AcceptLightReport(auth, body, input); err != nil {
		t.Fatalf("valid report after bad signature error = %v", err)
	}
	if _, err := service.AcceptLightReport(auth, body, input); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed report error = %v, want ErrReplay", err)
	}
	clock.Advance(30 * time.Second)
	secondInput := input
	secondInput.Telemetry.CollectedAt = clock.Now()
	secondInput.Telemetry.Network.ReceivedBytes += 30 * 1024
	secondInput.Telemetry.Network.SentBytes += 60 * 1024
	secondBody, secondAuth := signedLightReportForTest(t, clock.Now(), enrollment, secondInput, strings.Repeat("b", 32))
	secondAuth.ReportLatencyMilliseconds = "not-a-number"
	if _, err := service.AcceptLightReport(secondAuth, secondBody, secondInput); err != nil {
		t.Fatalf("second valid report error = %v", err)
	}
	host, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil || host.State != HostOnline || host.LastSnapshot == nil || host.LastSnapshot.Telemetry.Hostname != "edge-1" {
		t.Fatalf("unexpected reported host: %#v, %v", host, err)
	}
	if host.LastSnapshot.ReceiveBytesPerSecond != 1024 || host.LastSnapshot.TransmitBytesPerSecond != 2048 {
		t.Fatalf("unexpected light-node network rates: %#v", host.LastSnapshot)
	}
	if host.LastSnapshot.LatencyMilliseconds != 42 {
		t.Fatalf("invalid or absent latency reset the last known value: %d", host.LastSnapshot.LatencyMilliseconds)
	}
	if host.ResourceVersion != beforeReport.ResourceVersion {
		t.Fatalf("telemetry changed management resourceVersion: %q -> %q", beforeReport.ResourceVersion, host.ResourceVersion)
	}
	clock.Advance(2 * time.Minute)
	host, _ = service.Host(context.Background(), enrollment.NodeID)
	if host.State != HostStale {
		t.Fatalf("host state after two minutes = %s, want stale", host.State)
	}
	clock.Advance(4 * time.Minute)
	host, _ = service.Host(context.Background(), enrollment.NodeID)
	if host.State != HostOffline {
		t.Fatalf("host state after six minutes = %s, want offline", host.State)
	}
}

func TestLightReportIsRecoveredFromCheckpointWithoutChangingResourceVersion(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	dataDir := t.TempDir()
	config := ServiceConfig{
		DataDir: dataDir, PanelVersion: "0.40.0", PublicURL: "https://panel.example",
		Hostname: "center", Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
		Remote: localNodeTestRemote{}, Now: clock.Now,
	}
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	enrollment := enrollLightHostForTest(t, service, "edge-1")
	beforeReport, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	input := LightReportRequest{Telemetry: serviceTelemetry(now, "edge-1")}
	body, auth := signedLightReportForTest(t, now, enrollment, input, strings.Repeat("e", 32))
	if _, err := service.AcceptLightReport(auth, body, input); err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() checkpoint error = %v", err)
	}

	reopened, err := NewService(config)
	if err != nil {
		t.Fatalf("NewService() after checkpoint error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.Host(context.Background(), enrollment.NodeID)
	if err != nil || restored.LastSnapshot == nil || restored.LastSnapshot.Telemetry.Hostname != "edge-1" {
		t.Fatalf("checkpoint did not restore light report: %#v, %v", restored, err)
	}
	if restored.ResourceVersion != beforeReport.ResourceVersion {
		t.Fatalf("checkpoint changed management resourceVersion: %q -> %q", beforeReport.ResourceVersion, restored.ResourceVersion)
	}
}

func TestForgedLightReportsCannotExhaustAValidNodeQuota(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge-1")
	input := LightReportRequest{Telemetry: serviceTelemetry(now, "edge-1")}
	body, auth := signedLightReportForTest(t, now, enrollment, input, strings.Repeat("c", 32))

	for index := 0; index < 180; index++ {
		forged := auth
		forged.RequestID = strings.Repeat("d", 16) + fmt.Sprintf("%016x", index)
		forged.Signature = strings.Repeat("A", len(auth.Signature))
		if _, err := service.AcceptLightReport(forged, body, input); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("forged report %d error = %v, want ErrAuthentication", index, err)
		}
	}
	if _, err := service.AcceptLightReport(auth, body, input); err != nil {
		t.Fatalf("valid report was blocked after forged traffic: %v", err)
	}
}

func TestLightHostRenameAndDeleteUseResourceVersion(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge-1")
	host, _ := service.Host(context.Background(), enrollment.NodeID)
	if _, err := service.RenameHost(host.ID, UpdateHostInput{Name: "edge-2", ExpectedResourceVersion: "sha256:stale"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale rename error = %v, want ErrConflict", err)
	}
	renamed, err := service.RenameHost(host.ID, UpdateHostInput{Name: "edge-2", ExpectedResourceVersion: host.ResourceVersion})
	if err != nil || renamed.Name != "edge-2" {
		t.Fatalf("RenameHost() = %#v, %v", renamed, err)
	}
	result, err := service.DeleteHost(context.Background(), host.ID, DeleteHostInput{ExpectedResourceVersion: renamed.ResourceVersion})
	if err != nil || !result.Deleted || !result.CredentialRemoved {
		t.Fatalf("DeleteHost() = %#v, %v", result, err)
	}
	if _, err := service.Host(context.Background(), host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted light host error = %v, want ErrNotFound", err)
	}
}

func TestLightStoreMutationsRestoreMemoryAfterAtomicWriteFailure(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge-1")
	host, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil {
		t.Fatal(err)
	}

	service.light.ops.syncDir = func(string) error { return errors.New("injected sync failure") }
	if _, err := service.RenameHost(host.ID, UpdateHostInput{
		Name: "edge-broken", ExpectedResourceVersion: host.ResourceVersion,
	}); err == nil {
		t.Fatal("RenameHost() unexpectedly succeeded")
	}
	if _, err := service.DeleteHost(context.Background(), host.ID, DeleteHostInput{
		ExpectedResourceVersion: host.ResourceVersion,
	}); err == nil {
		t.Fatal("DeleteHost() unexpectedly succeeded")
	}

	service.light.ops = defaultAtomicFileOpsV2()
	restored, err := service.Host(context.Background(), host.ID)
	if err != nil || restored.Name != host.Name || restored.ResourceVersion != host.ResourceVersion {
		t.Fatalf("failed mutation changed in-memory host: %#v, %v", restored, err)
	}
}

func TestLightEnrollmentRequiresConfiguredHTTPSPublicOrigin(t *testing.T) {
	clock := &serviceTestClock{now: time.Now().UTC()}
	for _, publicURL := range []string{
		"", "http://panel.example", "https://panel.example/path", "https://user@panel.example",
		"https://panel.example?query=1", "https://panel.example#fragment",
	} {
		service, err := NewService(ServiceConfig{
			DataDir: t.TempDir(), PublicURL: publicURL, Hostname: "center",
			Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
			Remote:    localNodeTestRemote{}, Now: clock.Now,
		})
		if err != nil {
			t.Fatalf("NewService(%q) error = %v", publicURL, err)
		}
		if _, err := service.CreateLightEnrollment(); !errors.Is(err, ErrLightHTTPSOrigin) {
			t.Fatalf("CreateLightEnrollment(%q) error = %v, want ErrLightHTTPSOrigin", publicURL, err)
		}
		_ = service.Close()
	}
}

func TestLightEnrollmentCanUseAuthenticatedExternalHTTPSOrigin(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), PublicURL: "http://127.0.0.1:8080", Hostname: "center",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
		Remote:    localNodeTestRemote{}, Now: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	enrollment, err := service.CreateLightEnrollmentForOrigin("https://panel.example")
	if err != nil {
		t.Fatalf("CreateLightEnrollmentForOrigin() error = %v", err)
	}
	fields := strings.Fields(enrollment.Command)
	token := strings.Trim(fields[len(fields)-1], "'")
	if _, err := service.EnrollLightNodeAtOrigin("198.51.100.10", "https://panel.example", LightEnrollRequest{
		Token: token, Name: "edge-1", NodeVersion: "0.40.0",
	}); err != nil {
		t.Fatalf("EnrollLightNodeAtOrigin() error = %v", err)
	}

	other, err := service.CreateLightEnrollmentForOrigin("https://panel.example")
	if err != nil {
		t.Fatal(err)
	}
	otherFields := strings.Fields(other.Command)
	otherToken := strings.Trim(otherFields[len(otherFields)-1], "'")
	if _, err := service.EnrollLightNodeAtOrigin("198.51.100.10", "https://other.example", LightEnrollRequest{
		Token: otherToken, Name: "edge-2", NodeVersion: "0.40.0",
	}); !errors.Is(err, ErrPairingCode) {
		t.Fatalf("origin mismatch error = %v, want ErrPairingCode", err)
	}
}
