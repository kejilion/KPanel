package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

type nodeHistoryFixture struct{ ticks uint64 }

func (s *nodeHistoryFixture) CollectRuntime(context.Context) (contract.SystemSummary, error) {
	s.ticks++
	return contract.SystemSummary{CPU: contract.CPUSummary{Cores: 4, UsagePercent: 24.5}, Load: contract.LoadSummary{One: 1, Five: 2, Fifteen: 3}, Memory: contract.MemorySummary{UsedBytes: 1 << 30, TotalBytes: 4 << 30, SwapUsedBytes: 1 << 20, SwapTotalBytes: 1 << 30}, Disks: []contract.DiskSummary{{MountPoint: "/", UsedBytes: 20 << 30, TotalBytes: 100 << 30, UsagePercent: 20}}, DiskIO: contract.DiskIOSummary{Available: true, ReadBytes: s.ticks * 600000, WriteBytes: s.ticks * 300000}, Network: contract.NetworkSummary{ReceivedBytes: s.ticks * 900000, SentBytes: s.ticks * 600000, TCPConnections: 12, UDPConnections: 3}}, nil
}
func (s *nodeHistoryFixture) RunningContainerStats(context.Context, int, int) (dockerx.ContainerMetricBatch, error) {
	batch := dockerx.ContainerMetricBatch{Total: 32}
	for i := 0; i < 32; i++ {
		batch.Items = append(batch.Items, dockerx.ContainerMetricSample{Name: fmt.Sprintf("container-%d", i), Image: "nginx:alpine", ContainerStats: dockerx.ContainerStats{ContainerID: fmt.Sprintf("%064x", i+1), MemoryBytes: 128 << 20, MemoryLimit: 1 << 30, NetworkRx: s.ticks * 600000, NetworkTx: s.ticks * 300000, BlockRead: s.ticks * 400000, BlockWrite: s.ticks * 200000, PIDs: 5}})
	}
	return batch, nil
}
func (*nodeHistoryFixture) Probe(context.Context, string) (time.Duration, error) {
	return 15 * time.Millisecond, nil
}
func (*nodeHistoryFixture) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return contract.HostTelemetry{Hostname: "center", CollectedAt: time.Now().UTC()}, nil
}

func TestNodeHistoryMatchesLocalOverAuthenticatedOutboundRelayAndRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	clock := time.Now().UTC().Truncate(time.Second).Add(-6 * time.Hour)
	source := &nodeHistoryFixture{}
	config := monitoring.Config{StateDir: filepath.Join(t.TempDir(), "monitoring"), System: source, Docker: source, OperatorLatency: source, Now: func() time.Time { return clock }}
	history, err := monitoring.New(config)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 72; i++ {
		clock = clock.Add(5 * time.Minute)
		if err := history.Sample(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want, err := history.History(ctx, "6h")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(want)
	if len(encoded) <= cluster.MaxSummaryBytes {
		t.Fatal("fixture must exercise multiple encrypted frames")
	}
	t.Logf("light relay native history: %d bytes, %d containers, %d routes", len(encoded), len(want.Containers), len(want.OperatorLatency))
	// Reopen the same on-node history, including the records collected without
	// any center connection. No center-side data backfill is required.
	history, err = monitoring.New(config)
	if err != nil {
		t.Fatal(err)
	}
	var center *cluster.Service
	var polls atomic.Int64
	var requestBytes atomic.Int64
	var compression atomic.Bool
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
		requestBytes.Add(r.ContentLength)
		// Repeatable network delay; this is a local relay test, not a WAN claim.
		select {
		case <-time.After(20 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
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
	node, err := center.EnrollLightNode("198.51.100.10", cluster.LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "edge", NodeVersion: "1.13.0", TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := base64.RawURLEncoding.DecodeString(node.TerminalPeerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := cluster.NewHistoryRelayClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	nodeConfig := nodeConfig{NodeID: node.NodeID, TargetNodeID: node.TargetNodeID, Origin: server.URL}
	relayContext, stopRelay := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler := agent.NewMonitoringHandler(history)
		runLightFileControl(relayContext, nodeConfig, terminalIdentity{Key: key, Peer: peer}, relay, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !compression.Load() {
				r = r.Clone(r.Context())
				query := r.URL.Query()
				query.Del("gzip")
				r.URL.RawQuery = query.Encode()
			}
			handler.ServeHTTP(w, r)
		}))
	}()
	defer func() { stopRelay(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for polls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// The HTTP request arriving precedes authenticated availability by a few
	// instructions; retry only the documented initial-not-ready state.
	var got contract.MonitoringHistory
	started := time.Now()
	for {
		got, err = center.History(ctx, node.NodeID, "6h", time.Time{}, time.Time{})
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	plainDuration, plainPolls, plainBytes := time.Since(started), polls.Load(), requestBytes.Load()
	t.Logf("plain light history: duration=%v polls=%d wire_request_bytes=%d", plainDuration, plainPolls, plainBytes)
	// Reopening storage changes current-hour restoration bookkeeping; compare
	// against the reopened native reader, exactly as the broker serves it.
	want, err = history.History(ctx, "6h")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("light node did not return the complete native history")
	}
	if len(got.Containers) != 32 || len(got.OperatorLatency) != 9 || !got.Host[0].DiskIOAvailable || got.Storage.HostIntervalSeconds != 60 || got.Storage.RollupRetentionDays != 365 {
		t.Fatal("native monitoring capabilities missing")
	}
	compression.Store(true)
	started = time.Now()
	got, err = center.History(ctx, node.NodeID, "6h", time.Time{}, time.Time{})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("compressed history changed metrics: %v", err)
	}
	compressedPolls, compressedBytes := polls.Load()-plainPolls, requestBytes.Load()-plainBytes
	t.Logf("gzip light history: duration=%v polls=%d wire_request_bytes=%d", time.Since(started), compressedPolls, compressedBytes)
	if compressedBytes >= plainBytes/2 || compressedPolls >= plainPolls/2 {
		t.Fatal("compression did not substantially reduce relay traffic and round trips")
	}
	stopRelay()
	<-done
	selected, err := center.Host(ctx, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := center.DeleteHost(ctx, node.NodeID, cluster.DeleteHostInput{ExpectedResourceVersion: selected.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := center.History(ctx, node.NodeID, "6h", time.Time{}, time.Time{}); err == nil {
		t.Fatal("removed node history still readable")
	}
}

func TestNodeHistoryHandlerIsReadOnlyAndStorageFailureDoesNotCrashBroker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, stateDir := range []string{t.TempDir(), "relative-path"} {
		handler, stop := startNodeMonitoring(ctx, stateDir)
		for _, test := range []struct {
			method, path string
			status       int
		}{{"POST", cluster.HistoryPath, 405}, {"GET", "/v1/files", 404}, {"GET", cluster.HistoryPath + "?range=1h&range=7d", 422}, {"GET", cluster.HistoryPath + "?range=bad", 422}} {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != test.status {
				t.Fatalf("%s: %d", test.path, recorder.Code)
			}
		}
		stop()
	}
}
