package cluster

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestFileStreamProductionHandshakeBudget(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	target, _ := url.Parse(f.server.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = f.server.Client().Transport
	edge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(3500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer edge.Close()
	roots := x509.NewCertPool()
	roots.AddCert(edge.Certificate())
	client, err := NewRemoteClient(RemoteClientConfig{RootCAs: roots, Resolver: staticResolver{"example.com": {net.ParseIP("8.8.8.8")}}, Dialer: func(ctx context.Context, kind, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, kind, edge.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(edge.Listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	response, err := client.OpenFileRelayV2(ctx, "https://example.com:"+port, f.controller, f.service.NodeID(), f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), LightFileRequest{Method: "GET", Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil || string(got) != "ok" {
		t.Fatalf("body=%q err=%v", got, err)
	}
	defer client.fileStreamClient.CloseIdleConnections()
	if client.streamClient.Transport.(*http.Transport).ResponseHeaderTimeout != 3*time.Second {
		t.Fatal("ordinary HTTP budget changed")
	}
}

func TestFileStreamRemoteBusyReturnsCompleteResponse(t *testing.T) {
	for _, mode := range []string{"directory", "blocked-upload", "active-upload"} {
		t.Run(mode, func(t *testing.T) {
			f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("busy handler executed") }))
			input := LightFileRequest{Method: "GET", Path: "/v1/files"}
			if mode == "blocked-upload" {
				reader, writer := io.Pipe()
				defer writer.Close()
				input = LightFileRequest{Method: "POST", Path: "/v1/files/upload", Body: reader, BodyLength: -1}
			}
			if mode == "active-upload" {
				input = LightFileRequest{Method: "POST", Path: "/v1/files/upload", Body: bytes.NewReader(bytes.Repeat([]byte("active upload"), 100000)), BodyLength: -1}
			}
			for range 4 {
				release, ok := f.service.fileStreamHub.limits.acquire(f.controller, input)
				if !ok {
					t.Fatal("reserve")
				}
				defer release()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := f.open(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != 429 || !strings.Contains(string(body), "file_relay_rate_limited") {
				t.Fatalf("status=%d body=%q err=%v", response.StatusCode, body, err)
			}
		})
	}
}

func TestFileStreamDirectoryBudgetDoesNotExpandOtherOperations(t *testing.T) {
	for _, tc := range []struct {
		method, path, query string
		want                int64
	}{
		{"POST", "/v1/files/transfer/import", "kind=directory", contract.MaxFileTransferArchiveBytes},
		{"POST", "/v1/files/transfer/import", "kind=file", contract.MaxFileShareBytes},
		{"POST", "/v1/files/upload", "kind=directory", contract.MaxFileShareBytes},
		{"GET", "/v1/files/transfer/import", "kind=directory", contract.MaxFileShareBytes},
		{"POST", "/v1/files/transfer/import", "kind=directory&kind=file", contract.MaxFileShareBytes},
		{"POST", "/v1/files/transfer/import", "kind=directory&bad=%zz", contract.MaxFileShareBytes},
	} {
		if got := fileRelayBodyLimit(tc.method, tc.path, tc.query); got != tc.want {
			t.Fatalf("%+v limit=%d", tc, got)
		}
		input := LightFileRequest{Method: tc.method, Path: tc.path, RawQuery: tc.query, BodyLength: tc.want}
		if !validFileRelayRequest(input) {
			t.Fatalf("valid boundary rejected: %+v", tc)
		}
		input.BodyLength++
		if validFileRelayRequest(input) {
			t.Fatal("over budget accepted")
		}
	}
	now := time.Now()
	command := FileRelayCommand{ID: strings.Repeat("a", 32), RequestID: strings.Repeat("b", 32), Kind: "request", Method: "POST", Path: "/v1/files/transfer/import", Query: "kind=directory", BodyLength: contract.MaxFileTransferArchiveBytes, ExpiresAt: now.Add(time.Minute).Unix()}
	if err := validateFileRelayCommand(command, now); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyFileRelayBodyUsesOperationBudget(t *testing.T) {
	for _, limit := range []int64{4, 5} {
		relay := newLightFileRelay(time.Now)
		item := relay.node(strings.Repeat("a", 32), true)
		session := &lightFileSession{id: strings.Repeat("b", 32), responseReady: make(chan struct{}), done: make(chan struct{})}
		item.sessions[session.id] = session
		wake := item.wake
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan struct{})
		go func() {
			relay.sendBody(ctx, item, session, strings.NewReader("bytes"), 5, limit)
			close(done)
		}()
		if limit == 5 {
			select {
			case <-wake:
				item.mu.Lock()
				request := item.queued[0]
				item.mu.Unlock()
				if string(request.command.Data) != "bytes" || !request.command.Final {
					t.Fatal("body command changed")
				}
				request.done <- nil
			case <-ctx.Done():
				t.Fatal("allowed body was not queued")
			}
		}
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("body did not stop")
		}
		cancel()
		if (session.failure() != nil) != (limit == 4) {
			t.Fatalf("limit=%d error=%v", limit, session.failure())
		}
	}
}

func TestFileStreamTransportErrorClassification(t *testing.T) {
	var classified *FileStreamError
	if err := fileStreamHTTPError(429, errors.New("private proxy details")); !errors.Is(err, ErrRateLimited) {
		t.Fatal(err)
	}
	err := fileStreamHTTPError(502, errors.New("https://secret.example/private"))
	if !errors.As(err, &classified) || classified.HTTPStatus != 502 || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	err = fileStreamTransportError("upgrade", context.DeadlineExceeded)
	if !errors.As(err, &classified) || classified.Code != "timeout" {
		t.Fatal(err)
	}
}

// A real-time integration checks native ping/pong against the deployed 45s
// idle boundary. Short mode skips the real-time wait.
func TestFileStreamQuietHandlerSurvivesIdleBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("real-time idle boundary")
	}
	t.Parallel()
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(47 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			_, _ = io.WriteString(w, "completed")
		case <-r.Context().Done():
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	response, err := f.open(ctx, LightFileRequest{Method: "GET", Path: "/v1/files"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != "completed" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}

func TestFileStreamHeartbeatsDoNotExtendStalledUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("real-time upload idle boundary")
	}
	t.Parallel()
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			panic(http.ErrAbortHandler)
		}
		t.Error("stalled upload unexpectedly completed")
	}))
	body, writer := io.Pipe()
	defer body.Close()
	defer writer.Close()
	go func() { _, _ = writer.Write([]byte("incomplete")) }()
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	start := time.Now()
	response, err := f.open(ctx, LightFileRequest{Method: "POST", Path: "/v1/files/upload", Body: body, BodyLength: -1})
	if response != nil {
		response.Body.Close()
	}
	if err == nil || time.Since(start) < 40*time.Second || time.Since(start) > 52*time.Second {
		t.Fatalf("elapsed=%v err=%v", time.Since(start), err)
	}
}
