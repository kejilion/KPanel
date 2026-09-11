package panel

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

// Both center and a real TLS/Noise node endpoint run in this process. The
// simulated Agent alone uses an in-memory fixture rather than host telemetry.
func historyMemoryFixture(t *testing.T, points int, pad bool) (*Server, *http.Request) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	value := contract.MonitoringHistory{Range: "6h", StartedAt: now.Add(-6 * time.Hour), EndedAt: now, BucketSeconds: 60, Host: []contract.MonitoringHostPoint{}, Containers: []contract.MonitoringContainerSeries{}}
	random := rand.New(rand.NewPCG(123, 456))
	for i := range points {
		at := now.Add(-time.Duration(i) * time.Minute)
		value.Host = append(value.Host, contract.MonitoringHostPoint{CollectedAt: at, CPUPercent: random.Float64() * 100, MemoryUsedBytes: random.Uint64N(1 << 30), DiskIOAvailable: true})
	}
	for i := range 32 {
		series := contract.MonitoringContainerSeries{ContainerID: fmt.Sprint(i), Name: "fixture", Image: "fixture:latest", Points: []contract.MonitoringContainerPoint{}}
		for _, host := range value.Host {
			series.Points = append(series.Points, contract.MonitoringContainerPoint{CollectedAt: host.CollectedAt, CPUPercent: random.Float64() * 100, MemoryBytes: random.Uint64N(1 << 30), NetworkRxRate: random.Float64() * 1e7, BlockReadRate: random.Float64() * 1e6})
		}
		value.Containers = append(value.Containers, series)
	}
	for i := range 9 {
		series := contract.MonitoringOperatorLatencySeries{ID: fmt.Sprint(i), Address: "192.0.2.1"}
		for _, host := range value.Host {
			latency := random.Float64() * 300
			series.Points = append(series.Points, contract.MonitoringOperatorLatencyPoint{CollectedAt: host.CollectedAt, LatencyMilliseconds: &latency})
		}
		value.OperatorLatency = append(value.OperatorLatency, series)
	}
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if pad {
		full := make([]byte, monitoring.MaxHistoryResponseBytes)
		copy(full, content)
		for i := len(content); i < len(full); i++ {
			full[i] = ' '
		}
		t.Logf("history fixture: %d metric bytes, padded to %d decoded bytes", len(content), len(full))
		content = full
	}
	target, _ := newTestServerWithPublicURL(t, "https://example.com")
	_ = target.cluster.Close()
	target.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}})
	if err != nil {
		t.Fatal(err)
	}
	target.agent = &slowHistoryAgent{fileStubAgent: &fileStubAgent{stubAgent: &stubAgent{}, streamHeaders: http.Header{"Content-Type": {"application/json"}}, streamResponse: content}}
	remoteServer := httptest.NewTLSServer(target)
	t.Cleanup(remoteServer.Close)
	roots := x509.NewCertPool()
	roots.AddCert(remoteServer.Certificate())
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{RootCAs: roots, Resolver: historyTestResolver{}, Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", remoteServer.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	_ = center.cluster.Close()
	center.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	code, err := target.cluster.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.cluster.AddHost(context.Background(), cluster.AddHostInput{Name: "memory", Origin: "https://example.com", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	session, csrf := bootstrapCookiesForOrigin(t, center, tokenPath, "https://panel.test")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/monitoring/cluster-history?hostId="+host.ID+"&range=6h", nil)
	request.Host = "panel.test"
	request.AddCookie(session)
	request.AddCookie(csrf)
	request.Header.Set("Accept-Encoding", "gzip")
	return center, request
}

type historyBlockedWriter struct {
	header           http.Header
	entered, release chan struct{}
	enter, unblock   sync.Once
}

func (w *historyBlockedWriter) Header() http.Header { return w.header }
func (w *historyBlockedWriter) WriteHeader(int)     {}
func (w *historyBlockedWriter) Write([]byte) (int, error) {
	w.enter.Do(func() { close(w.entered) })
	<-w.release
	return 0, io.ErrClosedPipe
}
func (w *historyBlockedWriter) SetWriteDeadline(at time.Time) error {
	if !at.After(time.Now()) {
		w.unblock.Do(func() { close(w.release) })
	}
	return nil
}

func TestClusterHistorySlowBrowserBudgetAndCancellation(t *testing.T) {
	center, request := historyMemoryFixture(t, 2, false)
	done := make(chan struct{}, 2)
	var cancels []context.CancelFunc
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		t.Cleanup(cancel)
		writer := &historyBlockedWriter{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
		go func() {
			center.ServeHTTP(writer, request.Clone(ctx))
			done <- struct{}{}
		}()
		select {
		case <-writer.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("history never reached browser write")
		}
	}
	for range 10 {
		recorder := httptest.NewRecorder()
		center.ServeHTTP(recorder, request.Clone(context.Background()))
		if recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("slow clients escaped budget: %d", recorder.Code)
		}
	}
	for _, cancel := range cancels {
		cancel()
	}
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancelled browser write retained its slot")
		}
	}
	recorder := httptest.NewRecorder()
	center.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history did not recover after cancellation: %d", recorder.Code)
	}
	data, err := monitoring.ReadHistoryPayload(recorder.Body, true)
	var result contract.MonitoringHistory
	if err != nil || json.Unmarshal(data, &result) != nil || len(result.Containers) != 32 || len(result.OperatorLatency) != 9 {
		t.Fatalf("recovered history is incomplete: %v", err)
	}
}

// Opt-in integration workload for a hard memory cgroup; no GOMEMLIMIT or
// explicit GC during the measured request workload. Read cgroup memory.peak
// and memory.events outside the process, including fixture/server overhead.
func TestClusterHistoryMemoryEnvelope(t *testing.T) {
	if os.Getenv("KPANEL_HISTORY_MEMORY_TEST") != "1" {
		t.Skip("opt-in constrained-memory workload")
	}
	center, request := historyMemoryFixture(t, 720, true)
	done := make(chan struct{}, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		center.ServeHTTP(w, r)
		done <- struct{}{}
	}))
	defer server.Close()
	runtime.GC() // Exclude fixture construction garbage, never used in production.
	transport := &http.Transport{DisableCompression: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		if err == nil {
			_ = connection.(*net.TCPConn).SetReadBuffer(64 << 10)
		}
		return connection, err
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 40 * time.Second}
	get := func(ctx context.Context, encoding string) *http.Response {
		t.Helper()
		req := request.Clone(ctx)
		req.URL.Scheme, req.URL.Host = "http", server.Listener.Addr().String()
		req.RequestURI = ""
		req.Header.Set("Accept-Encoding", encoding)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	for round := range 4 {
		// Identity responses exceed the TCP send buffers; the browsers read
		// headers but deliberately stop consuming the multi-megabyte body.
		var responses []*http.Response
		var cancels []context.CancelFunc
		for range 2 {
			ctx, cancel := context.WithCancel(context.Background())
			cancels = append(cancels, cancel)
			t.Cleanup(cancel)
			response := get(ctx, "identity")
			if response.StatusCode != 200 {
				t.Fatalf("slow request: %d", response.StatusCode)
			}
			responses = append(responses, response)
		}
		for range 8 {
			response := get(context.Background(), "gzip")
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("burst escaped slots: %d", response.StatusCode)
			}
			<-done
		}
		for i, cancel := range cancels {
			cancel()
			_ = responses[i].Body.Close()
		}
		for range 2 {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("TCP cancellation did not release slot")
			}
		}
		// Consume a compressed response after each burst to check recovery.
		response := get(context.Background(), "gzip")
		if response.StatusCode != 200 || response.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("recovery: %d %v", response.StatusCode, response.Header)
		}
		data, err := monitoring.ReadHistoryPayload(response.Body, true)
		_ = response.Body.Close()
		var result contract.MonitoringHistory
		if err != nil || json.Unmarshal(data, &result) != nil || len(result.Host) != 720 || len(result.Containers) != 32 || len(result.Containers[31].Points) != 720 || len(result.OperatorLatency) != 9 {
			t.Fatalf("large response lost metrics: %v", err)
		}
		<-done
		t.Logf("round %d: 2 stalled TCP clients, 8 rejected burst requests, cancellation and complete gzip recovery passed", round+1)
	}
	// Browsers that never cancel must also release their slots at the server's
	// write deadline. Client timeout is longer, so it cannot cause this pass.
	for range 2 {
		response := get(context.Background(), "identity")
		if response.StatusCode != 200 {
			t.Fatalf("deadline request: %d", response.StatusCode)
		}
		defer response.Body.Close()
	}
	deadline := time.After(35 * time.Second)
	for range 2 {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("server did not bound uncancelled slow clients")
		}
	}
	response := get(context.Background(), "gzip")
	_, err := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("deadline recovery: %d %v", response.StatusCode, err)
	}
	<-done
	t.Log("uncancelled TCP clients released by server write deadline; next query succeeded")
}
