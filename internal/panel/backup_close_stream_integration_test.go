package panel

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

// This candidate-combination regression is opt-in and uses only loopback TLS,
// t.TempDir stores and the public authenticated lightweight-node stream client.
// It never connects to an Agent, a Docker daemon or an existing Panel.
func TestCombinedBackupCloseStopsHealthyLightControl(t *testing.T) {
	if os.Getenv("KPB_CLOSE_ORDER_INTEGRATION") != "1" {
		t.Skip("set KPB_CLOSE_ORDER_INTEGRATION=1 in an isolated combined candidate tree")
	}
	server, _ := newTestServerWithPublicURL(t, "https://panel.test")
	var active atomic.Int32
	httpServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active.Add(1)
		defer active.Add(-1)
		server.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	enrollment, err := server.cluster.CreateLightEnrollmentForOrigin("https://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := server.cluster.EnrollLightNodeAtOrigin("198.51.100.10", "https://panel.test", cluster.LightEnrollRequest{
		Token: strings.Trim(fields[len(fields)-1], "'"), Name: "close-order-light", NodeVersion: "1.14.1",
		TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := base64.RawURLEncoding.DecodeString(enrolled.TerminalPeerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := cluster.NewFileRelayClient(httpServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	clientCtx, cancelClient := context.WithCancel(context.Background())
	defer cancelClient()
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- relay.RunFileStream(clientCtx, httpServer.URL, enrolled.NodeID, enrolled.TargetNodeID, key, peer,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "control-ready")
			}))
	}()

	// A completed authenticated data request proves the real control socket is
	// registered. No reflection, private cluster hub access or synthetic wait.
	ready := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-clientDone:
			t.Fatalf("control exited before readiness: %v", err)
		default:
		}
		requestCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		response, err := server.cluster.OpenLightFile(requestCtx, enrolled.NodeID,
			cluster.LightFileRequest{Method: http.MethodGet, Path: "/v1/files"})
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			ready = readErr == nil && response.StatusCode == http.StatusOK && string(body) == "control-ready"
		}
		cancel()
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("real lightweight control/data roundtrip did not become ready")
	}
	deadline = time.Now().Add(time.Second)
	for active.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := active.Load(); got != 1 {
		t.Fatalf("expected one idle healthy control request, got %d", got)
	}
	t.Logf("authenticated loopback TLS control/data roundtrip ready; active control requests=%d", active.Load())

	// Match paneld: HTTP shutdown precedes the application Close. Hijacked
	// WebSockets survive HTTP shutdown and must be closed by cluster.Service.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	err = httpServer.Config.Shutdown(shutdownCtx)
	cancelShutdown()
	if err != nil {
		t.Fatalf("HTTP shutdown: %v", err)
	}
	select {
	case err := <-clientDone:
		t.Fatalf("control unexpectedly exited during HTTP shutdown: %v", err)
	default:
	}
	if got := active.Load(); got != 1 {
		t.Fatalf("HTTP shutdown did not retain the live control request: %d", got)
	}

	closed := make(chan error, 1)
	start := time.Now()
	go func() { closed <- server.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Panel.Close failed: %v", err)
		}
		if got := active.Load(); got != 0 {
			// The wrapper's defer runs just after ServeHTTP's request Done.
			deadline := time.Now().Add(time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if got := active.Load(); got != 0 {
				t.Errorf("Panel.Close returned while HTTP control still active: %d", got)
			}
		}
		select {
		case <-clientDone:
		case <-time.After(time.Second):
			t.Error("Panel.Close left the lightweight client connected")
		}
		t.Logf("Panel.Close stopped healthy control and drained requests in %s", time.Since(start))
	case <-time.After(2 * time.Second):
		t.Errorf("Panel.Close blocked while healthy light-control remained connected; active=%d", active.Load())
		// Release the expected old-order failure so red evidence exits cleanly.
		cancelClient()
		select {
		case err := <-closed:
			t.Logf("test cleanup: peer cancellation released the blocked old-order Close: %v", err)
		case <-time.After(2 * time.Second):
			panic(fmt.Sprintf("Close remained stuck after peer cancellation; active=%d", active.Load()))
		}
	}
}
