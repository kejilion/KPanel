package cluster

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
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

func TestFileStreamPanelFullDuplexAndUnknownLength(t *testing.T) {
	content := bytes.Repeat([]byte("binary\x00\xffpayload"), 150000)
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" || r.Header.Get("Authorization") != "" {
			t.Error("header allowlist changed")
		}
		if r.ContentLength != -1 {
			t.Error("upload must verify authenticated EOF")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusCreated)
		w.(http.Flusher).Flush()
		if _, err := io.Copy(w, r.Body); err != nil {
			panic(http.ErrAbortHandler)
		}
	}))
	for _, length := range []int64{int64(len(content)), -1} {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		response, err := f.open(ctx, LightFileRequest{Method: http.MethodPost, Path: "/v1/files/upload", Headers: map[string]string{"Content-Type": "application/octet-stream", "Authorization": "must-not-forward"}, Body: bytes.NewReader(content), BodyLength: length})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		got, err := io.ReadAll(response.Body)
		response.Body.Close()
		cancel()
		if err != nil || !bytes.Equal(got, content) || response.StatusCode != http.StatusCreated {
			t.Fatalf("roundtrip bytes=%d status=%d error=%v", len(got), response.StatusCode, err)
		}
	}
	response, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(got) != `{"items":[]}` {
		t.Fatalf("directory: %q %v", got, err)
	}
	if f.streamCalls.Load() != 3 || f.legacyCalls.Load() != 0 {
		t.Fatalf("stream=%d legacy=%d", f.streamCalls.Load(), f.legacyCalls.Load())
	}
}

func TestFileStreamFailureNeverReplaysAnAction(t *testing.T) {
	var actions atomic.Int32
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions.Add(1)
		_, _ = w.Write([]byte("partial"))
		panic(http.ErrAbortHandler)
	}))
	response, err := f.open(context.Background(), LightFileRequest{Method: http.MethodPost, Path: "/v1/files/actions"})
	if err == nil {
		_, err = io.ReadAll(response.Body)
		response.Body.Close()
	}
	if err == nil || actions.Load() != 1 || f.legacyCalls.Load() != 0 {
		t.Fatalf("err=%v actions=%d legacy=%d", err, actions.Load(), f.legacyCalls.Load())
	}
	wrong, _ := GenerateFederationV2Keypair()
	conn, err := f.dial(context.Background(), wrong, fileStreamHello{Role: "panel"})
	if conn != nil {
		conn.close()
	}
	if err == nil || actions.Load() != 1 || f.legacyCalls.Load() != 0 {
		t.Fatal("wrong identity authorized or replayed")
	}
}

func TestFileStreamTruncatedAndTamperedUploadsNeverReachEOF(t *testing.T) {
	for _, mode := range []string{"missing-end", "bad-size", "tampered", "invalid-path"} {
		t.Run(mode, func(t *testing.T) {
			finished := make(chan error, 1)
			var actions atomic.Int32
			f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actions.Add(1)
				_, err := io.Copy(io.Discard, r.Body)
				finished <- err
			}))
			conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: "panel"})
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/files/upload"
			if mode == "invalid-path" {
				path = "/v1/terminal/open"
			}
			if err := conn.writeJSON(streamRequest, fileStreamRequest{Method: http.MethodPost, Path: path, Length: 5}); err != nil {
				t.Fatal(err)
			}
			if mode == "tampered" {
				sealed, _ := conn.tx.Encrypt(nil, nil, append([]byte{streamData}, []byte("hello")...))
				sealed[len(sealed)-1] ^= 1
				_ = conn.ws.Write(conn.ctx, websocket.MessageBinary, sealed)
			} else {
				_ = conn.write(streamData, []byte("hello"))
				if mode == "bad-size" {
					_ = conn.write(streamEnd, streamSize(4))
				}
			}
			if mode == "missing-end" {
				conn.close()
			}
			if mode == "invalid-path" {
				_, _, err := conn.read()
				if err == nil || actions.Load() != 0 {
					t.Fatal("unrestricted path executed")
				}
			} else {
				select {
				case err := <-finished:
					if err == nil {
						t.Fatal("invalid upload returned clean EOF")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("upload handler leaked")
				}
			}
			conn.close()
		})
	}
}

func TestFileStreamBulkSlotsPreserveDirectoryAndCancel(t *testing.T) {
	closed := make(chan struct{}, 4)
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files" {
			_, _ = w.Write([]byte("directory"))
			return
		}
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		closed <- struct{}{}
	}))
	var responses []*http.Response
	for range 4 {
		response, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files/content"})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	_, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files/content"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("fifth bulk request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := f.open(ctx, LightFileRequest{Method: http.MethodGet, Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(got) != "directory" {
		t.Fatalf("directory blocked: %q %v", got, err)
	}
	for _, response := range responses {
		response.Body.Close()
	}
	for range 4 {
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("cancel did not reach handler")
		}
	}
}

func TestFileStreamEarlyResponseStopsBlockedUpload(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "reject", http.StatusConflict) }))
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := f.open(ctx, LightFileRequest{Method: http.MethodPost, Path: "/v1/files/upload", Body: reader, BodyLength: -1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusConflict {
		t.Fatalf("early response: %d %v", response.StatusCode, err)
	}
	stopped := make(chan struct{})
	go func() {
		for {
			if _, err := writer.Write([]byte("unused")); err != nil {
				close(stopped)
				return
			}
		}
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("upload reader was not closed")
	}
}

func TestFileStreamEarlyResponseDoesNotWaitForUploadWriteLock(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the continuous upload fill TCP/WebSocket buffers while the handler
		// does not consume its request pipe, then return a complete rejection.
		time.Sleep(150 * time.Millisecond)
		http.Error(w, "reject", http.StatusRequestEntityTooLarge)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := time.Now()
	response, err := f.open(ctx, LightFileRequest{Method: "POST", Path: "/v1/files/upload", Body: io.LimitReader(streamZeros{}, 256<<20), BodyLength: 256 << 20})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 413 || time.Since(started) >= 2*time.Second {
		t.Fatalf("early rejection delayed: status=%d elapsed=%v err=%v", response.StatusCode, time.Since(started), err)
	}
}

type streamZeros struct{}

func (streamZeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestFileStreamExportRequiresSuccessfulFinalTrailer(t *testing.T) {
	for _, result := range []string{"ok", "error", ""} {
		t.Run("trailer="+result, func(t *testing.T) {
			f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Trailer", "X-KPanel-Transfer-Result")
				_, _ = w.Write([]byte("complete-size-but-possibly-changed"))
				w.Header().Set("X-KPanel-Transfer-Result", result)
			}))
			response, err := f.open(context.Background(), LightFileRequest{Method: "GET", Path: "/v1/files/transfer/export"})
			if err == nil {
				_, err = io.ReadAll(response.Body)
				response.Body.Close()
			}
			if (err == nil) != (result == "ok") {
				t.Fatalf("trailer=%q error=%v", result, err)
			}
		})
	}
}

func TestFileStreamRevokeClosesActiveSocket(t *testing.T) {
	closed := make(chan struct{})
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	response, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files/content"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteController(f.controller); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("revocation returned clean EOF")
	}
	response.Body.Close()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("revoke left handler running")
	}
	if _, err := f.open(context.Background(), LightFileRequest{Method: http.MethodGet, Path: "/v1/files"}); err == nil {
		t.Fatal("revoked controller reconnected")
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

func TestFileStreamFallbackOnlyBeforeUpgrade(t *testing.T) {
	for _, status := range []int{404, 405, 426, 401, 403, 429, 502} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			key, _ := GenerateFederationV2Keypair()
			peer, _ := GenerateFederationV2Keypair()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			_, err := dialFileStream(context.Background(), server.Client(), server.URL, strings.Repeat("a", 32), strings.Repeat("b", 32), key, peer.Public, time.Now(), fileStreamHello{Role: "panel"})
			want := status == 404 || status == 405 || status == 426
			if errors.Is(err, ErrFileStreamUnsupported) != want {
				t.Fatalf("status=%d fallback=%v error=%v", status, want, err)
			}
		})
	}
}

type fileStreamLatencyTransport struct {
	next  http.RoundTripper
	delay time.Duration
	calls atomic.Int32
}

func (t *fileStreamLatencyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	timer := time.NewTimer(t.delay)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		return nil, r.Context().Err()
	case <-timer.C:
	}
	return t.next.RoundTrip(r)
}

func TestFileStreamControlledRequestLatency(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 1<<20)
	var durations [2]time.Duration
	var calls [2]int32
	for index, legacy := range []bool{true, false} {
		f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(content) }))
		f.legacyMode.Store(legacy)
		transport := &fileStreamLatencyTransport{next: f.client.streamClient.Transport, delay: 25 * time.Millisecond}
		client := *f.client.streamClient
		client.Transport = transport
		f.client.streamClient = &client
		started := time.Now()
		response, err := f.open(context.Background(), LightFileRequest{Method: "GET", Path: "/v1/files/content"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("legacy=%v bytes=%d err=%v", legacy, len(got), err)
		}
		durations[index], calls[index] = time.Since(started), transport.calls.Load()
		if !legacy && f.legacyCalls.Load() != 0 {
			t.Fatal("stream used legacy calls")
		}
	}
	if calls[0] < 32 || calls[1] != 1 {
		t.Fatalf("legacy/stream request counts=%v", calls)
	}
	t.Logf("1 MiB TLS download, artificial 25 ms delay per HTTP request (not WAN benchmark): legacy=%v/%d requests; stream=%v/%d request", durations[0], calls[0], durations[1], calls[1])
}

func TestFileStreamTransportRetainsTLSAddressAndProxyPolicies(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	roots := x509.NewCertPool()
	roots.AddCert(f.server.Certificate())
	guarded, err := NewRemoteClient(RemoteClientConfig{RootCAs: roots, PrivateCIDRs: []string{"127.0.0.1/32"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = guarded.OpenFileRelayV2(context.Background(), f.server.URL, f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), LightFileRequest{Method: "GET", Path: "/v1/files"})
	if err == nil || f.streamCalls.Load() != 0 {
		t.Fatal("stream bypassed loopback restriction")
	}
	_, err = dialFileStream(context.Background(), &http.Client{}, f.server.URL, f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), fileStreamHello{Role: "panel"})
	if err == nil || f.streamCalls.Load() != 0 {
		t.Fatal("stream accepted an untrusted certificate")
	}
	// The broker's HTTPClient proxy configuration must remain in the actual
	// upgrade path. This local forward proxy tunnels a complete Noise request.
	target, _ := url.Parse(f.server.URL)
	forward := httputil.NewSingleHostReverseProxy(target)
	forward.Transport = f.server.Client().Transport
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1); forward.ServeHTTP(w, r) }))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	conn, err := dialFileStream(context.Background(), client, "http://8.8.8.8:1801", f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), fileStreamHello{Role: "panel"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := openStreamRequest(conn, LightFileRequest{Method: "GET", Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != "ok" || proxyCalls.Load() != 1 {
		t.Fatalf("proxy: %q %v calls=%d", body, err, proxyCalls.Load())
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, proxy.URL, 302) }))
	defer redirect.Close()
	client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	_, err = dialFileStream(context.Background(), client, redirect.URL, f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), fileStreamHello{Role: "panel"})
	if err == nil || errors.Is(err, ErrFileStreamUnsupported) || proxyCalls.Load() != 1 {
		t.Fatal("redirect was followed or downgraded")
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
