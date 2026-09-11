package cluster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func TestClusterHistoryStreamCancellationAndByteLimit(t *testing.T) {
	for _, field := range []string{"containers", "CONTAINERS", "containerſ"} {
		input := `{"` + field + `":[` + strings.Repeat(`{},`, 32) + `{}]}`
		if err := scanHistoryJSON([]byte(input)); err == nil {
			t.Fatalf("preflight accepted 33 containers under %q", field)
		}
	}
	source, sink := io.Pipe()
	defer sink.Close()
	reader := &federationFileReader{source: bufio.NewReader(source), body: source}
	done := make(chan error, 1)
	go func() { _, err := reader.Read(make([]byte, 1024)); done <- err }()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled stream returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release the stream reader")
	}
	oversized := strings.NewReader(strings.Repeat(" ", int(monitoring.MaxHistoryResponseBytes)+2))
	if _, err := decodeHistoryResponse(oversized, monitoring.Query{Range: "6h"}); err == nil {
		t.Fatal("oversized history accepted")
	}
	if oversized.Len() != 1 {
		t.Fatalf("reader exceeded byte budget: %d unread", oversized.Len())
	}
}

func historyPairFixture(t *testing.T) (*Service, *Service, *RemoteClient, Host, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	clock := &serviceTestClock{now: now}
	target := newLightServiceForTest(t, clock)
	remote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "1.13.0", Hostname: "center", Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"}, Remote: remote, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })
	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{Name: "history", Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	return center, target, remote, host, now
}

func completeHistoryFixture(now time.Time, count int) contract.MonitoringHistory {
	result := contract.MonitoringHistory{Range: "6h", StartedAt: now.Add(-6 * time.Hour), EndedAt: now, BucketSeconds: 60,
		Host: []contract.MonitoringHostPoint{}, Containers: []contract.MonitoringContainerSeries{}, OperatorLatency: []contract.MonitoringOperatorLatencySeries{},
		Storage: contract.MonitoringStorageStatus{Enabled: true, HostIntervalSeconds: 60, ContainerIntervalSeconds: 300, OperatorLatencyIntervalSeconds: 300, RetentionDays: 30, RollupRetentionDays: 365, MaxStorageBytes: 128 << 20, MaxRollupStorageBytes: 128 << 20, MaxContainers: 32}}
	for i := 0; i < count; i++ {
		at := now.Add(time.Duration(i-count) * time.Minute)
		result.Host = append(result.Host, contract.MonitoringHostPoint{CollectedAt: at, CPUPercent: 42.123456789, CPUCores: 4, LoadOne: 1.25, MemoryUsedBytes: 1 << 30, MemoryTotalBytes: 4 << 30, DiskIOAvailable: true, DiskReadRate: 1234.5678, DiskWriteRate: 7654.321, NetworkRxRate: 50000, TCPConnections: 30})
	}
	for j := 0; j < 32; j++ {
		series := contract.MonitoringContainerSeries{ContainerID: fmt.Sprintf("%064x", j+1), Name: fmt.Sprintf("service-%d", j), Image: "nginx:alpine"}
		for _, point := range result.Host {
			series.Points = append(series.Points, contract.MonitoringContainerPoint{CollectedAt: point.CollectedAt, CPUPercent: 42.123456789, MemoryBytes: 1 << 30, NetworkRxRate: 123456.789, BlockReadRate: 345678.12})
		}
		result.Containers = append(result.Containers, series)
	}
	for j := 0; j < 9; j++ {
		series := contract.MonitoringOperatorLatencySeries{ID: fmt.Sprint(j), Operator: "telecom", Region: "beijing", Address: "192.0.2.1"}
		latency := 12.3456
		for _, point := range result.Host {
			series.Points = append(series.Points, contract.MonitoringOperatorLatencyPoint{CollectedAt: point.CollectedAt, LatencyMilliseconds: &latency, SuccessCount: 1})
		}
		result.OperatorLatency = append(result.OperatorLatency, series)
	}
	return result
}

func installHistoryTransport(t *testing.T, remote *RemoteClient, target *Service, value contract.MonitoringHistory, truncate bool) {
	t.Helper()
	remote.historyClient = &http.Client{Transport: fileV2RoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != HistoryV2Path {
			return nil, errors.New("wrong history endpoint")
		}
		var envelope FederationEnvelopeV2
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			return nil, err
		}
		query, auth, err := target.AuthorizeHistoryV2("198.51.100.10", envelope)
		if err != nil {
			return nil, err
		}
		defer auth.Close()
		response := value
		response.Range = query.Range
		if !query.Start.IsZero() {
			response.StartedAt = query.Start
			response.EndedAt = query.End
		}
		contentType := "application/json"
		if query.Gzip {
			contentType = monitoring.GzipHistoryContentType
		}
		sealed, cipher, err := auth.SealResponse(http.StatusOK, contentType)
		if err != nil {
			return nil, err
		}
		var output bytes.Buffer
		if err := WriteFederationFileHeader(&output, sealed); err != nil {
			return nil, err
		}
		writer := NewFederationFileWriter(&output, cipher)
		content, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}
		if err := monitoring.CopyHistoryPayload(writer, bytes.NewReader(content), query.Gzip); err != nil {
			return nil, err
		}
		if !truncate {
			if err := writer.Finish(nil); err != nil {
				return nil, err
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {fileStreamContentType}}, Body: io.NopCloser(bytes.NewReader(output.Bytes()))}, nil
	})}
}

func TestClusterHistoryFullParityLargeEncryptedResponseAndWindows(t *testing.T) {
	center, target, remote, host, now := historyPairFixture(t)
	want := completeHistoryFixture(now, 720)
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= MaxSummaryBytes || int64(len(encoded)) > monitoring.MaxHistoryResponseBytes {
		t.Fatalf("unexpected maximum fixture size %d", len(encoded))
	}
	t.Logf("32 containers, 9 latency series, 720 points each: %d JSON bytes", len(encoded))
	installHistoryTransport(t, remote, target, want, false)
	got, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("remote history differs from source")
	}
	for _, rangeName := range []string{"1h", "24h", "7d", "30d", "3m", "6m", "12m"} {
		small := completeHistoryFixture(now, 2)
		installHistoryTransport(t, remote, target, small, false)
		got, err := center.History(context.Background(), host.ID, rangeName, now.Add(-time.Minute), now)
		if err != nil || got.Range != rangeName || !got.StartedAt.Equal(now.Add(-time.Minute)) {
			t.Fatalf("%s custom window: %v", rangeName, err)
		}
	}
	// Persisted pre-scope v2 hosts have always meant summary access.
	center.storeV2.mu.Lock()
	center.storeV2.state.Hosts[0].Scope = ""
	center.storeV2.mu.Unlock()
	if _, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{}); err != nil {
		t.Fatalf("legacy scope: %v", err)
	}
}

func TestClusterHistoryRejectsTruncatedStreamAndUnboundedJSON(t *testing.T) {
	center, target, remote, host, now := historyPairFixture(t)
	installHistoryTransport(t, remote, target, completeHistoryFixture(now, 2), true)
	if _, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{}); err == nil {
		t.Fatal("missing authenticated END accepted")
	}
	for _, field := range []string{"host", "containers", "points"} {
		value := completeHistoryFixture(now, 2)
		switch field {
		case "host":
			value.Host = nil
		case "containers":
			value.Containers = nil
		case "points":
			value.Containers[0].Points = nil
		}
		content, _ := json.Marshal(value)
		if _, err := decodeHistoryResponse(bytes.NewReader(content), monitoring.Query{Range: "6h"}); err == nil {
			t.Fatalf("null %s accepted for required chart array", field)
		}
	}
	for _, raw := range []string{
		`{"host":[` + strings.Repeat(`{},`, 720) + `{}]}`,
		`{"containers":[` + strings.Repeat(`{},`, 32) + `{}]}`,
		`{"operatorLatency":[` + strings.Repeat(`{},`, 9) + `{}]}`,
		`{"containers":[{"points":[` + strings.Repeat(`{},`, 720) + `{}]}]}`,
		`{"host":[],"HOST":[]}`, `{"a":[[[[[[[[[[]]]]]]]]]]}`, `{} {}`,
	} {
		if _, err := decodeHistoryResponse(strings.NewReader(raw), monitoring.Query{Range: "6h"}); err == nil {
			t.Fatal("unbounded or malformed response accepted")
		}
	}
}

func TestClusterHistoryScopesReplayDeletionAndConcurrency(t *testing.T) {
	center, target, remote, host, now := historyPairFixture(t)
	installHistoryTransport(t, remote, target, completeHistoryFixture(now, 2), false)
	for _, scope := range []string{"cluster.files.read", "unknown", SummaryScope} {
		target.storeV2.mu.Lock()
		target.storeV2.state.Controllers[0].Scope = scope
		target.storeV2.mu.Unlock()
		_, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{})
		if (scope == SummaryScope) != (err == nil) {
			t.Fatalf("scope %q: %v", scope, err)
		}
	}
	center.historyQueries <- struct{}{}
	center.historyQueries <- struct{}{}
	if _, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{}); !errors.Is(err, monitoring.ErrBusy) {
		t.Fatalf("query cap: %v", err)
	}
	<-center.historyQueries
	<-center.historyQueries
	record, _ := center.storeV2.Host(host.ID)
	credential, _ := center.secretsV2.ReadCredential(record.CredentialFile)
	payload, _ := json.Marshal(monitoring.Query{Range: "6h"})
	envelope, _, err := sealV2Request(http.MethodPost, HistoryV2Path, v2Envelope{Protocol: FederationProtocolV2, ControllerID: record.ControllerID, TargetID: record.RemoteNodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("a", 32)}, noiseKeyV2(credential), credential.TargetPublic, nil, payload)
	if err != nil {
		t.Fatal(err)
	}
	_, authorization, err := target.AuthorizeHistoryV2("198.51.100.10", envelope)
	if err != nil {
		t.Fatal(err)
	}
	authorization.Close()
	if _, _, err := target.AuthorizeHistoryV2("198.51.100.10", envelope); err == nil {
		t.Fatalf("replay: %v", err)
	}
	if err := target.DeleteController(record.ControllerID); err != nil {
		t.Fatal(err)
	}
	if _, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{}); err == nil {
		t.Fatal("revoked controller read history")
	}
}

func TestClusterHistoryConsumerFailureReleasesBudget(t *testing.T) {
	center, target, remote, host, now := historyPairFixture(t)
	installHistoryTransport(t, remote, target, completeHistoryFixture(now, 2), false)
	want := errors.New("downstream failed")
	for _, shouldPanic := range []bool{false, true} {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil && (!shouldPanic || recovered != want) {
					t.Fatalf("unexpected panic: %v", recovered)
				}
			}()
			err := center.WithHistory(context.Background(), host.ID, "6h", time.Time{}, time.Time{}, func(contract.MonitoringHistory) error {
				if len(center.historyQueries) != 1 {
					t.Fatal("consumer ran outside query budget")
				}
				if shouldPanic {
					panic(want)
				}
				return want
			})
			if shouldPanic || !errors.Is(err, want) {
				t.Fatalf("consumer error: %v", err)
			}
		}()
		if len(center.historyQueries) != 0 {
			t.Fatal("failed consumer leaked query slot")
		}
	}
}

func TestClusterHistoryRelayIsolationAndImmediateDataAcknowledgment(t *testing.T) {
	request := LightFileRequest{Method: http.MethodGet, Path: HistoryPath, RawQuery: "range=12m"}
	fileRelay := newLightFileRelay(time.Now)
	historyRelay := newLightHistoryRelay(time.Now)
	defer fileRelay.closeAll()
	defer historyRelay.closeAll()
	id := strings.Repeat("c", 32)
	if _, err := fileRelay.Open(context.Background(), id, request); err == nil {
		t.Fatal("file relay accepted history")
	}
	if _, err := historyRelay.Open(context.Background(), id, LightFileRequest{Method: http.MethodGet, Path: "/v1/files"}); err == nil {
		t.Fatal("history relay accepted files")
	}
	command := FileRelayCommand{ID: id, RequestID: id, Kind: "request", Method: request.Method, Path: request.Path, Query: request.RawQuery, ExpiresAt: time.Now().Add(time.Minute).Unix()}
	if validateFileRelayCommand(command, time.Now()) == nil || validHistoryRelayCommand(command, time.Now()) != nil {
		t.Fatal("command modes not isolated")
	}
	command.Method = http.MethodPost
	if validHistoryRelayCommand(command, time.Now()) == nil {
		t.Fatal("history write accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// Unknown completed sessions are ignored, but acknowledged without the
	// normal file relay's 250ms active wait or 25s idle wait.
	if _, err := historyRelay.poll(ctx, id, nil, []FileRelayEvent{{RequestID: id, Kind: "end"}}); err != nil {
		t.Fatalf("history acknowledgment stalled: %v", err)
	}
}

func TestClusterHistoryDedicatedHeaderBudget(t *testing.T) {
	remote, err := NewRemoteClient(RemoteClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if remote.historyClient.Transport.(*http.Transport).ResponseHeaderTimeout != 30*time.Second {
		t.Fatal("history header budget missing")
	}
	if remote.client.Transport.(*http.Transport).ResponseHeaderTimeout != 3*time.Second {
		t.Fatal("summary budget changed")
	}
	for _, status := range []int{404, 405, 426} {
		if !errors.Is(historyResponseStatus(status), ErrHistoryUnsupported) {
			t.Fatalf("old node status %d", status)
		}
	}
}
