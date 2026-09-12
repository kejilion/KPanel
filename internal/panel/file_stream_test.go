package panel

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestFederationFileStreamRouteAndAudit(t *testing.T) {
	server, _ := newTestServer(t)
	server.cluster.SetFileRelayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fail") == "1" {
			http.Error(w, "rejected", 409)
			return
		}
		_, _ = w.Write([]byte("file-result"))
	}))
	network := httptest.NewServer(server)
	defer network.Close()
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{Dialer: func(ctx context.Context, networkName, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, networkName, network.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
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
	for _, query := range []string{"", "fail=1"} {
		response, err := center.OpenRemotePanelFile(ctx, host.ID, cluster.LightFileRequest{Method: "GET", Path: "/v1/files", RawQuery: query})
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	// The fixture's unavailable telemetry can consume the global auth audit
	// cooldown during pairing; isolate the following stream-auth assertion.
	server.auditMu.Lock()
	server.lastGlobalAuthAudit = time.Time{}
	server.auditMu.Unlock()
	ws, _, err := websocket.Dial(ctx, network.URL+cluster.FileStreamV2Path, &websocket.DialOptions{Subprotocols: []string{"kpanel-file-stream-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Write(ctx, websocket.MessageBinary, []byte(`{}`))
	_, _, _ = ws.Read(ctx)
	ws.CloseNow()
	deadline := time.Now().Add(time.Second)
	for {
		events, _ := server.store.ListAudit(100, "")
		var success, rejected, unauth int
		for _, event := range events {
			if event.Action != "cluster.federation.v2.files.stream" {
				continue
			}
			if event.TargetKind == "authentication" && event.Result == "failure" {
				unauth++
			} else if event.Result == "success" {
				success++
			} else if event.Result == "failure" {
				rejected++
			}
			if strings.Contains(event.TargetID, "file-result") {
				t.Fatal("file content entered audit")
			}
		}
		if success == 1 && rejected == 1 && unauth == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream audit success=%d failure=%d unauth=%d", success, rejected, unauth)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
