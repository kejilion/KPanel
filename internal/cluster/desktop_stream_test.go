package cluster

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func desktopFixture(t *testing.T) (*streamFixture, LightEnrollResponse, noise.DHKey, []byte) {
	t.Helper()
	f := newStreamFixture(t, nil)
	enrollment, err := f.service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, _ := GenerateFederationV2Keypair()
	node, err := f.service.EnrollLightNode("198.51.100.10", LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "windows", Platform: "windows", NodeVersion: "1.24.0", TerminalPublicKey: encodeTestKey(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.light.UpdateReport(node.NodeID, HostSnapshot{Platform: "windows", ReceivedAt: f.service.now(), NodeCapabilities: []string{"monitoring", "desktop"}, UnavailableMetrics: []string{"load"}, Telemetry: contract.HostTelemetry{OSID: "windows"}}, "1.24.0", f.service.now())
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := decodeTerminalRelayPublicKey(node.TerminalPeerPublicKey)
	return f, node, key, peer
}

func waitDesktop(t *testing.T, f *streamFixture, node string) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if f.service.fileStreamHub.desktopAvailable(node) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("desktop control did not register")
}

func TestDesktopStreamTransferRevocationAndRoleIsolation(t *testing.T) {
	f, node, key, peer := desktopFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	relay, _ := NewTerminalRelayClient(f.server.Client())
	nonce := strings.Repeat("a", 64)
	done := make(chan error, 1)
	handlerDone := make(chan struct{})
	go func() {
		done <- relay.RunDesktopStream(ctx, f.server.URL, node.NodeID, node.TargetNodeID, key, peer, func(ctx context.Context, stream io.ReadWriteCloser, got string) error {
			defer close(handlerDone)
			if got != nonce {
				t.Error("nonce changed in transit")
			}
			_, err := io.Copy(stream, stream)
			return err
		}, nil)
	}()
	waitDesktop(t, f, node.NodeID)
	for _, role := range []string{streamRoleLightTerminalControl, "light-control"} {
		conn, err := dialFileStream(ctx, f.server.Client(), f.server.URL, node.NodeID, node.TargetNodeID, key, peer, time.Now(), fileStreamHello{Role: role})
		if conn != nil {
			conn.close()
		}
		if err == nil {
			t.Fatalf("desktop-only node admitted to %s", role)
		}
	}
	if _, err := f.service.OpenDesktopStream(ctx, node.NodeID, "bad"); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatal("invalid nonce accepted", err)
	}
	stream, err := f.service.OpenDesktopStream(ctx, node.NodeID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	want := bytes.Repeat([]byte("RDP\x00\xff"), 210000)
	wrote := make(chan error, 1)
	go func() { _, err := stream.Write(want); wrote <- err }()
	got := make([]byte, len(want))
	if _, err := io.ReadFull(stream, got); err != nil || !bytes.Equal(got, want) {
		t.Fatal("binary stream mismatch", err)
	}
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	if err := f.service.SetDesktopAllowed(node.NodeID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Read(make([]byte, 1)); err == nil {
		t.Fatal("revoked stream remained readable")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control did not stop")
	}
	select {
	case <-handlerDone:
	default:
		t.Fatal("control returned before handler joined")
	}
	host, err := f.service.Host(ctx, node.NodeID)
	if err != nil || host.DesktopAvailable || host.DesktopUnavailableReason != "desktop_disabled_by_center" {
		t.Fatalf("revoked host = %#v %v", host, err)
	}
	if _, err := f.service.OpenDesktopStream(ctx, node.NodeID, nonce); err == nil {
		t.Fatal("revocation allowed new session")
	}
}

func TestDesktopPolicyPersistsAndCorruptionFailsClosed(t *testing.T) {
	f, node, _, _ := desktopFixture(t)
	if err := f.service.SetDesktopAllowed(node.NodeID, false); err != nil {
		t.Fatal(err)
	}
	path := f.service.desktopPolicy.path
	f.service.desktopPolicy = openDesktopPolicy(filepath.Dir(path))
	if f.service.desktopAllowed(node.NodeID) {
		t.Fatal("restart lost revocation")
	}
	if err := os.WriteFile(path, []byte(`{"bad":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	f.service.desktopPolicy = openDesktopPolicy(filepath.Dir(path))
	if f.service.desktopAllowed(node.NodeID) || f.service.SetDesktopAllowed(node.NodeID, true) == nil {
		t.Fatal("corrupt policy allowed access")
	}
}
