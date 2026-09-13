package panel

import (
	"context"
	"fmt"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Real Panel route, federation service, AgentClient and an HTTP Agent fixture.
// Only the Agent socket dial is mapped to a local TCP listener on Windows.
func newAgentFileStreamTest(t *testing.T, handler http.Handler, writeTimeout time.Duration) (*cluster.Service, string) {
	t.Helper()
	backend := httptest.NewUnstartedServer(handler)
	backend.Config.WriteTimeout = writeTimeout
	backend.Start()
	t.Cleanup(backend.Close)
	tokenPath := filepath.Join(t.TempDir(), "agent-token")
	if err := os.WriteFile(tokenPath, []byte("diagnostic-only-token"), 0600); err != nil {
		t.Fatal(err)
	}
	client := NewAgentClient("unused-test-socket", tokenPath, 8<<20)
	client.client.Transport.(*http.Transport).DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	t.Cleanup(client.client.CloseIdleConnections)
	server, _ := newTestServer(t)
	server.agent = client
	network := httptest.NewServer(server)
	t.Cleanup(network.Close)
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{Dialer: func(ctx context.Context, kind, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, kind, network.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { center.Close() })
	code, err := server.cluster.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host, err := center.AddHost(ctx, cluster.AddHostInput{Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	return center, host.ID
}

func TestFileStreamAgentDisconnectRejectsTruncatedDownload(t *testing.T) {
	for _, tc := range []struct{ name, path, query string }{{"download", "/v1/files/content", "path=%2Fa.bin"}, {"archive", "/v1/files/archive", "selection=test&name=test.zip"}} {
		t.Run(tc.name, func(t *testing.T) {
			center, host := newAgentFileStreamTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = fmt.Fprint(rw, "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n")
				_ = rw.Flush() // Missing final HTTP chunk deliberately simulates Agent loss.
			}), 0)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			response, err := center.OpenRemotePanelFile(ctx, host, cluster.LightFileRequest{Method: "GET", Path: tc.path, RawQuery: tc.query})
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 200 || string(data) != "abc" || readErr == nil {
				t.Fatalf("reproduction changed: status=%d bytes=%q error=%v", response.StatusCode, data, readErr)
			}
			t.Logf("Agent HTTP stream truncated without terminal chunk; federation returns status=%d bytes=%d readError=%v (truncation correctly rejected)", response.StatusCode, len(data), readErr)
		})
	}
}

func TestFileStreamAgentExportPreservesFailure(t *testing.T) {
	center, host := newAgentFileStreamTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trailer", "X-KPanel-Transfer-Result")
		_, _ = io.WriteString(w, "abc")
		w.Header().Set("X-KPanel-Transfer-Result", "error")
	}), 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := center.OpenRemotePanelFile(ctx, host, cluster.LightFileRequest{Method: "GET", Path: "/v1/files/transfer/export", RawQuery: "path=%2Fa&resourceVersion=12345678"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err == nil {
		t.Fatal("failed export unexpectedly succeeded")
	}
	t.Logf("cross-node export bytes=%d correctly fails: %v", len(data), err)
}
