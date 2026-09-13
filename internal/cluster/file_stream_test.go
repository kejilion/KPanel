package cluster

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flynn/noise"
)

type streamFixture struct {
	service     *Service
	server      *httptest.Server
	client      *RemoteClient
	key         noise.DHKey
	controller  string
	streamCalls atomic.Int32
	legacyCalls atomic.Int32
	legacyMode  atomic.Bool
}

func newStreamFixture(t *testing.T, handler http.Handler) *streamFixture {
	t.Helper()
	f := &streamFixture{}
	f.service = newLightServiceForTest(t, &serviceTestClock{now: time.Now().UTC()})
	f.service.SetFileRelayHandler(handler)
	key, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	f.key = key
	record := testControllerRecordV2(3, strings.Repeat("f", 32), time.Now().UTC())
	record.PublicKey = base64.RawURLEncoding.EncodeToString(key.Public)
	record.Fingerprint = fingerprintV2(key.Public)
	record.Scope = SummaryTerminalFilesScope
	record.State = controllerStateV2Active
	if err := f.service.storeV2.AddController(record); err != nil {
		t.Fatal(err)
	}
	f.controller = record.ID
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == FileStreamV2Path {
			f.streamCalls.Add(1)
			if f.legacyMode.Load() {
				http.NotFound(w, r)
				return
			}
			f.service.ServeFileStream(w, r, "127.0.0.1")
			return
		}
		f.legacyCalls.Add(1)
		if r.URL.Path == v2FileRelayPath && r.Method == http.MethodPost {
			var envelope FederationEnvelopeV2
			if json.NewDecoder(r.Body).Decode(&envelope) != nil {
				http.Error(w, "invalid", 400)
				return
			}
			response, err := f.service.HandleFederationV2(r.Context(), "127.0.0.1", r.URL.Path, "", envelope)
			if err != nil {
				http.Error(w, err.Error(), 403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(f.server.Certificate())
	f.client, err = NewRemoteClient(RemoteClientConfig{RootCAs: roots, PrivateCIDRs: []string{"127.0.0.1/32"}})
	if err != nil {
		t.Fatal(err)
	}
	// The integration server is loopback, which the production approved-IP
	// dialer deliberately rejects even when allow-listed. Keep its real TLS
	// verification, and test the production address rejection separately.
	f.client.streamClient = f.server.Client()
	return f
}

func (f *streamFixture) open(ctx context.Context, input LightFileRequest) (*http.Response, error) {
	return f.client.OpenFileRelayV2(ctx, f.server.URL, f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), input)
}

func (f *streamFixture) dial(ctx context.Context, key noise.DHKey, hello fileStreamHello) (*fileStreamConn, error) {
	return dialFileStream(ctx, f.client.streamClient, f.server.URL, f.controller, f.service.NodeID(), key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), hello)
}

func TestFileStreamPanelUsesLegacyRelayAndCannotUpgrade(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	response, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(got) != `{"items":[]}` {
		t.Fatalf("legacy directory: %q %v", got, err)
	}
	if f.streamCalls.Load() != 0 || f.legacyCalls.Load() == 0 {
		t.Fatalf("stream=%d legacy=%d", f.streamCalls.Load(), f.legacyCalls.Load())
	}
	conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: "panel"})
	if conn != nil {
		conn.close()
	}
	if err == nil {
		t.Fatal("full Panel role unexpectedly upgraded to the lightweight WebSocket transport")
	}
}

func TestFileStreamLightUsesIndependentOutboundSockets(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("light request ran on center") }))
	enrollment, err := f.service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, _ := GenerateFederationV2Keypair()
	enrolled, err := f.service.EnrollLightNode("198.51.100.10", LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "stream-light", NodeVersion: "1.14.1", TerminalPublicKey: encodeTestKey(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := decodeTerminalRelayPublicKey(enrolled.TerminalPeerPublicKey)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	relay, _ := NewFileRelayClient(f.server.Client())
	done := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("light-directory"))
			return
		}
		if _, err := io.Copy(w, r.Body); err != nil {
			panic(http.ErrAbortHandler)
		}
	})
	go func() {
		done <- relay.RunFileStream(ctx, f.server.URL, enrolled.NodeID, enrolled.TargetNodeID, key, peer, handler)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !f.service.fileStreamHub.available(enrolled.NodeID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !f.service.fileStreamHub.available(enrolled.NodeID) {
		t.Fatal("light control did not register")
	}
	content := bytes.Repeat([]byte("light-file"), 100000)
	for _, input := range []LightFileRequest{
		{Method: http.MethodGet, Path: "/v1/files"},
		{Method: http.MethodPost, Path: "/v1/files/upload", Body: bytes.NewReader(content), BodyLength: -1},
	} {
		requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		response, err := f.service.OpenLightFile(requestCtx, enrolled.NodeID, input)
		if err != nil {
			stop()
			t.Fatal(err)
		}
		got, err := io.ReadAll(response.Body)
		response.Body.Close()
		stop()
		if err != nil {
			t.Fatal(err)
		}
		if input.Method == http.MethodPost && !bytes.Equal(got, content) {
			t.Fatal("light upload content mismatch")
		}
		if input.Method == http.MethodGet && string(got) != "light-directory" {
			t.Fatal("light directory mismatch")
		}
	}
	if f.streamCalls.Load() != 3 || f.legacyCalls.Load() != 0 {
		t.Fatalf("stream=%d legacy=%d", f.streamCalls.Load(), f.legacyCalls.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("light control did not stop")
	}
}

func TestFileStreamLightCallbacksBindIdentityGenerationAndSingleUse(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	type nodeIdentity struct {
		id  string
		key noise.DHKey
	}
	enroll := func(name string) nodeIdentity {
		enrollment, err := f.service.CreateLightEnrollment()
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(enrollment.Command)
		key, _ := GenerateFederationV2Keypair()
		node, err := f.service.EnrollLightNode("198.51.100.10", LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: name, NodeVersion: "1.14.1", TerminalPublicKey: encodeTestKey(key.Public)})
		if err != nil {
			t.Fatal(err)
		}
		return nodeIdentity{node.NodeID, key}
	}
	a, b := enroll("node-a"), enroll("node-b")
	dial := func(node nodeIdentity, hello fileStreamHello) *fileStreamConn {
		conn, err := dialFileStream(ctx, f.server.Client(), f.server.URL, node.id, f.service.NodeID(), node.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), hello)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(conn.close)
		return conn
	}
	control := dial(a, fileStreamHello{Role: "light-control"})
	_, generation, err := control.read()
	if err != nil {
		t.Fatal(err)
	}
	h := f.service.fileStreamHub
	for !h.available(a.id) {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	h.mu.Lock()
	requestID := strings.Repeat("c", 32)
	pending := &fileStreamPending{nodeID: a.id, control: h.controls[a.id], ready: make(chan *fileStreamConn, 1), expires: time.Now().Add(4 * time.Second)}
	h.pending[requestID] = pending
	h.mu.Unlock()
	for _, attempt := range []struct {
		node       nodeIdentity
		generation string
	}{{b, string(generation)}, {a, strings.Repeat("d", 32)}} {
		conn := dial(attempt.node, fileStreamHello{Role: "light-data", RequestID: requestID, Generation: attempt.generation})
		if _, _, err := conn.read(); err == nil {
			t.Fatal("callback with wrong owner/generation accepted")
		}
		h.mu.Lock()
		intact := h.pending[requestID] == pending
		h.mu.Unlock()
		if !intact {
			t.Fatal("invalid callback consumed another request")
		}
	}
	accepted := dial(a, fileStreamHello{Role: "light-data", RequestID: requestID, Generation: string(generation)})
	select {
	case serverConn := <-pending.ready:
		serverConn.close()
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	accepted.close()
	duplicate := dial(a, fileStreamHello{Role: "light-data", RequestID: requestID, Generation: string(generation)})
	if _, _, err := duplicate.read(); err == nil {
		t.Fatal("callback consumed twice")
	}
	replacement := dial(a, fileStreamHello{Role: "light-control"})
	_, newGeneration, err := replacement.read()
	if err != nil || bytes.Equal(newGeneration, generation) {
		t.Fatal("control generation was reused")
	}
	if _, _, err := control.read(); err == nil {
		t.Fatal("old control remained active")
	}
	h.mu.Lock()
	current := h.controls[a.id]
	h.mu.Unlock()
	if current == nil || current.generation != string(newGeneration) {
		t.Fatal("old control cleanup removed replacement")
	}
	h.closePeer("light:" + a.id)
	if h.available(a.id) || !h.prefersStream(a.id) {
		t.Fatal("disconnect forgot stream preference")
	}
	started := time.Now()
	if _, err := f.service.OpenLightFile(ctx, a.id, LightFileRequest{Method: "POST", Path: "/v1/files/actions"}); err == nil || time.Since(started) > time.Second {
		t.Fatalf("disconnected stream fell back: %v", err)
	}
	legacyEnvelope, _, err := sealV2Request("POST", v2FileRelayPath, v2Envelope{Protocol: FederationProtocolV2, ControllerID: a.id, TargetID: f.service.NodeID(), Timestamp: time.Now().Unix(), RequestID: strings.Repeat("e", 32)}, a.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, nil, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	legacyCtx, stop := context.WithCancel(ctx)
	stop()
	_, _ = f.service.HandleFederationV2(legacyCtx, "198.51.100.10", v2FileRelayPath, "", legacyEnvelope)
	if h.prefersStream(a.id) {
		t.Fatal("authenticated legacy rollback did not restore compatibility")
	}
}
