package panel

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

type historyTestTelemetry struct{}

func (historyTestTelemetry) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return contract.HostTelemetry{Hostname: "fixture", CollectedAt: time.Now().UTC()}, nil
}

type historyTestResolver struct{}

func (historyTestResolver) LookupNetIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

type slowHistoryAgent struct {
	*fileStubAgent
	delay time.Duration
}

func (a *slowHistoryAgent) OpenStream(ctx context.Context, method, path, query, id string, body io.Reader, headers http.Header, length int64) (*http.Response, error) {
	if path != cluster.HistoryPath || method != http.MethodGet || query != "range=6h" || length != 0 {
		return nil, errors.New("history escaped its fixed Agent request")
	}
	select {
	case <-time.After(a.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return a.fileStubAgent.OpenStream(ctx, method, path, query, id, body, headers, length)
}

func TestClusterHistoryPanelHTTPV1V2ParitySlowSourceAndSignedQuery(t *testing.T) {
	target, _ := newTestServerWithPublicURL(t, "https://example.com")
	_ = target.cluster.Close()
	var err error
	target.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	value := contract.MonitoringHistory{Range: "6h", StartedAt: now.Add(-6 * time.Hour), EndedAt: now, BucketSeconds: 60, Host: []contract.MonitoringHostPoint{{CollectedAt: now, CPUPercent: 31.5, DiskIOAvailable: true, DiskReadRate: 12345}}, Containers: []contract.MonitoringContainerSeries{}, OperatorLatency: []contract.MonitoringOperatorLatencySeries{}, Storage: contract.MonitoringStorageStatus{Enabled: true, RetentionDays: 30, RollupRetentionDays: 365, HostIntervalSeconds: 60}}
	for i := 1; i < 60; i++ {
		point := value.Host[0]
		point.CollectedAt = now.Add(-time.Duration(i) * time.Minute)
		value.Host = append(value.Host, point)
	}
	content, _ := json.Marshal(value)
	agent := &slowHistoryAgent{fileStubAgent: &fileStubAgent{stubAgent: &stubAgent{}, streamHeaders: http.Header{"Content-Type": {"application/json"}}, streamResponse: content}}
	target.agent = agent
	var tamper atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tamper.Load() && r.URL.Path == cluster.HistoryV1Path {
			r.URL.RawQuery = "range=7d"
		}
		target.ServeHTTP(w, r)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{RootCAs: roots, Resolver: historyTestResolver{}, Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
	browserCenter, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	_ = browserCenter.cluster.Close()
	browserCenter.cluster = center
	session, csrf := bootstrapCookiesForOrigin(t, browserCenter, tokenPath, "https://panel.test")
	for _, protocol := range []string{"v2", "v1"} {
		if protocol == "v1" && runtime.GOOS == "windows" {
			t.Log("V1 credential mode validation requires Linux; covered by the Linux gate")
			continue
		}
		var code cluster.PairingCode
		if protocol == "v2" {
			code, err = target.cluster.CreatePairingCodeV2()
		} else {
			code, err = target.cluster.CreatePairingCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		host, err := center.AddHost(context.Background(), cluster.AddHostInput{Name: protocol, Origin: "https://example.com", PairingCode: code.Code})
		if err != nil {
			t.Fatal(err)
		}
		agent.delay = 0
		if protocol == "v2" {
			agent.delay = 3100 * time.Millisecond
		}
		result, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{})
		if err != nil {
			t.Fatalf("%s: %v", protocol, err)
		}
		if result.Host[0].CPUPercent != 31.5 || !result.Host[0].DiskIOAvailable || result.Storage.RollupRetentionDays != 365 {
			t.Fatalf("%s history not forwarded intact", protocol)
		}
		agent.delay = 0
		for _, encoding := range []string{"gzip", "gzip;q=0"} {
			response := authenticatedRequest(browserCenter, http.MethodGet, "/api/v1/monitoring/cluster-history?hostId="+host.ID+"&range=6h", nil, session, csrf, map[string]string{"Accept-Encoding": encoding})
			compressed := response.Header().Get("Content-Encoding") == "gzip"
			if response.Code != 200 || compressed != (encoding == "gzip") || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("browser history: %d %v", response.Code, response.Header())
			}
			decoded, err := monitoring.ReadHistoryPayload(response.Body, compressed)
			var browserValue contract.MonitoringHistory
			if err != nil || json.Unmarshal(decoded, &browserValue) != nil || len(browserValue.Host) != len(value.Host) || browserValue.Host[0].CPUPercent != 31.5 {
				t.Fatalf("browser history corrupted: %v", err)
			}
		}
		if protocol == "v1" {
			tamper.Store(true)
			if _, err := center.History(context.Background(), host.ID, "6h", time.Time{}, time.Time{}); !errors.Is(err, cluster.ErrAuthentication) {
				t.Fatalf("tampered signed query: %v", err)
			}
			tamper.Store(false)
		}
		if _, err := center.DeleteHost(context.Background(), host.ID, cluster.DeleteHostInput{ExpectedResourceVersion: host.ResourceVersion}); err != nil {
			t.Fatal(err)
		}
	}
}
