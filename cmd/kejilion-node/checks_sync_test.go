package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func checksSyncTelemetry(now time.Time) contract.HostTelemetry {
	return contract.HostTelemetry{
		AgentVersion: "1.31.0", AgentProtocolVersion: lightProtocol,
		Hostname: "edge", OS: "Debian GNU/Linux 13", OSID: "debian", OSLike: []string{"debian"},
		Kernel: "6.12.0", Architecture: "amd64", UptimeSeconds: 7200,
		Load:          contract.LoadSummary{One: 0.1, Five: 0.2, Fifteen: 0.3},
		CPU:           contract.CPUSummary{Model: "test cpu", Cores: 4, FrequencyMHz: 2400, UsagePercent: 20},
		Memory:        contract.MemorySummary{TotalBytes: 8 << 30, AvailableBytes: 6 << 30, UsedBytes: 2 << 30, UsagePercent: 25},
		Disk:          contract.DiskCapacitySummary{TotalBytes: 80 << 30, UsedBytes: 20 << 30, UsagePercent: 25},
		Network:       contract.NetworkSummary{ReceivedBytes: 4096, SentBytes: 2048, TCPConnections: 12, UDPConnections: 3},
		PublicNetwork: contract.PublicNetworkSummary{IPv4: "8.8.8.8", ISP: "Example ISP", Country: "Singapore", CountryCode: "SG"},
		CollectedAt:   now,
	}
}

// acceptNodeReport signs a report exactly as the telemetry service does and
// hands it to the center the way the panel's report endpoint does.
func acceptNodeReport(t *testing.T, center *cluster.Service, config nodeConfig) {
	t.Helper()
	secret, err := base64.RawURLEncoding.DecodeString(config.ReportingKey)
	if err != nil {
		t.Fatal(err)
	}
	payload := reportRequest{Telemetry: checksSyncTelemetry(time.Now().UTC()), Capabilities: nodeReportCapabilities(config)}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := signedLightNodeHeaders(config, lightReportPath, body, secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	var input cluster.LightReportRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		t.Fatalf("center cannot decode the node report: %v", err)
	}
	if _, err := center.AcceptLightReport(cluster.LightReportAuth{
		Source: "198.51.100.10", NodeID: headers["X-KPanel-Light-Node-ID"], Timestamp: headers["X-KPanel-Timestamp"],
		RequestID: headers["X-KPanel-Request-ID"], Signature: headers["X-KPanel-Signature"],
	}, body, input); err != nil {
		t.Fatal(err)
	}
}

func TestNodeReportsChecksSyncOnlyAfterCenterAdvertisedIt(t *testing.T) {
	headers := http.Header{}
	if config := enableSSHLoginCapability(nodeConfig{}, headers); config.ChecksSync || nodeReportCapabilities(config) != nil {
		t.Fatal("node advertised checks sync to a center that did not offer it")
	}
	headers.Set(cluster.LightResponseCapabilitiesHeader, strings.Join(cluster.LightCenterCapabilities(), ","))
	config := enableSSHLoginCapability(nodeConfig{Capabilities: []string{"files"}}, headers)
	if got := nodeReportCapabilities(config); !reflect.DeepEqual(got, []string{cluster.MonitoringChecksSyncCapability}) {
		t.Fatalf("node capabilities = %v", got)
	}
	encoded, err := json.Marshal(reportRequest{})
	if err != nil || strings.Contains(string(encoded), "capabilities") {
		t.Fatalf("an old center would receive an unknown field: %s", encoded)
	}
}

func TestNodeChecksAreReplacedOverAuthenticatedRelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source := &nodeHistoryFixture{}
	stateDir := filepath.Join(t.TempDir(), "monitoring")
	history, err := monitoring.New(monitoring.Config{StateDir: stateDir, System: source, Docker: source, OperatorLatency: source})
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Sample(ctx); err != nil {
		t.Fatal(err)
	}
	var center *cluster.Service
	var polls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cluster.HistoryRelayV2Path {
			http.NotFound(w, r)
			return
		}
		var envelope cluster.FederationEnvelopeV2
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			w.WriteHeader(400)
			return
		}
		polls.Add(1)
		response, err := center.HandleFederationV2(r.Context(), "198.51.100.10", r.URL.Path, "", envelope)
		if err != nil {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	center, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), PublicURL: server.URL, Telemetry: source})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
	enrollment, err := center.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	node, err := center.EnrollLightNode("198.51.100.10", cluster.LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "edge", NodeVersion: "1.31.0", TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := base64.RawURLEncoding.DecodeString(node.TerminalPeerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	config := nodeConfig{NodeID: node.NodeID, TargetNodeID: node.TargetNodeID, Origin: server.URL, ReportingKey: node.ReportingKey, ChecksSync: true, Capabilities: []string{"files"}}

	relay, err := cluster.NewHistoryRelayClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	relayContext, stopRelay := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runLightFileControl(relayContext, config, terminalIdentity{Key: key, Peer: peer}, relay, agent.NewMonitoringHandler(history))
	}()
	defer func() { stopRelay(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for !center.LightChecksReadiness(node.NodeID).Connected && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Connected but never advertised: the command must not be sent at all.
	if _, err := center.LightNodeChecks(ctx, node.NodeID); !errors.Is(err, cluster.ErrLightChecksUnsupported) {
		t.Fatalf("unadvertised node read error = %v", err)
	}
	acceptNodeReport(t, center, config)
	if readiness := center.LightChecksReadiness(node.NodeID); !readiness.Supported || !readiness.Connected {
		t.Fatalf("readiness = %#v", readiness)
	}

	current, err := center.LightNodeChecks(ctx, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ResourceVersion != monitoring.ChecksResourceVersion(monitoring.DefaultChecks()) || !reflect.DeepEqual(current.Items, monitoring.DefaultChecks()) {
		t.Fatalf("node checks = %#v", current)
	}
	desired := []monitoring.Check{
		{ID: "site", Kind: "http", Name: "官网", Target: "https://example.com/health"},
		{ID: "db", Kind: "tcp", Name: "数据库", Target: "10.0.0.5:5432"},
	}
	result, err := center.ReplaceLightNodeChecks(ctx, node.NodeID, monitoring.ReplaceChecksInput{ExpectedResourceVersion: current.ResourceVersion, Items: desired})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResourceVersion != monitoring.ChecksResourceVersion(desired) || !reflect.DeepEqual(history.Checks().Items, desired) {
		t.Fatalf("node did not store the center list: %#v", history.Checks())
	}
	reopened, err := monitoring.New(monitoring.Config{StateDir: stateDir, System: source, Docker: source, OperatorLatency: source})
	if err != nil || !reflect.DeepEqual(reopened.Checks().Items, desired) {
		t.Fatalf("node did not persist the center list: %v", err)
	}
	if _, err := center.ReplaceLightNodeChecks(ctx, node.NodeID, monitoring.ReplaceChecksInput{ExpectedResourceVersion: current.ResourceVersion, Items: monitoring.DefaultChecks()}); !errors.Is(err, monitoring.ErrChecksConflict) {
		t.Fatalf("stale replacement error = %v", err)
	}
	invalid := []monitoring.Check{{ID: "bad", Kind: "dns", Name: "bad", Target: "example.com"}}
	if _, err := center.ReplaceLightNodeChecks(ctx, node.NodeID, monitoring.ReplaceChecksInput{ExpectedResourceVersion: result.ResourceVersion, Items: invalid}); !errors.Is(err, cluster.ErrLightChecksRejected) {
		t.Fatalf("invalid replacement error = %v", err)
	}
	if !reflect.DeepEqual(history.Checks().Items, desired) {
		t.Fatal("a rejected list changed the node")
	}
	// The relay keeps serving history after the checks commands.
	if _, err := center.History(ctx, node.NodeID, "1h", time.Time{}, time.Time{}); err != nil {
		t.Fatalf("history after checks sync: %v", err)
	}
	if polls.Load() == 0 {
		t.Fatal("relay was not exercised")
	}
}
