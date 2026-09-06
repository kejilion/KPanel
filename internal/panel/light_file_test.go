package panel

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestIsLightFileRelayRequestUsesTheSharedRouteAllowlist(t *testing.T) {
	hostID := strings.Repeat("a", 32)
	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/v1/files?hostId=" + hostID, want: true},
		{path: "/api/v1/files/content?hostId=" + hostID, want: true},
		{path: "/api/v1/files/actions?hostId=" + hostID, want: true},
		{path: "/api/v1/files/transfers?hostId=" + hostID, want: false},
		{path: "/api/v1/files/download-tickets?hostId=" + hostID, want: false},
		{path: "/api/v1/files/content", want: false},
	}
	for _, test := range tests {
		requestURL, err := url.Parse(test.path)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", test.path, err)
		}
		request := &http.Request{Method: http.MethodGet, URL: requestURL}
		if got := isLightFileRelayRequest(request); got != test.want {
			t.Errorf("isLightFileRelayRequest(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

type lightFileLegacyTestRemote struct{ body func() io.ReadCloser }

func (*lightFileLegacyTestRemote) Pair(context.Context, string, cluster.PairRequest) (cluster.PairResponse, error) {
	return cluster.PairResponse{NodeID: strings.Repeat("b", 32), Hostname: "relay-test", PanelVersion: "1.5.0", FederationProtocol: cluster.FederationProtocol}, nil
}
func (*lightFileLegacyTestRemote) Summary(context.Context, string, string, string, ed25519.PrivateKey, time.Time) (cluster.FederationSummary, error) {
	return cluster.FederationSummary{NodeID: strings.Repeat("b", 32), PanelVersion: "1.5.0", FederationProtocol: cluster.FederationProtocol, Telemetry: contract.HostTelemetry{Hostname: "relay-test", CollectedAt: time.Now()}}, nil
}
func (remote *lightFileLegacyTestRemote) SummaryWithCapabilities(ctx context.Context, origin, controller, target string, key ed25519.PrivateKey, now time.Time) (cluster.FederationSummary, string, error) {
	summary, err := remote.Summary(ctx, origin, controller, target, key, now)
	return summary, cluster.FileRelayV1Capability, err
}
func (*lightFileLegacyTestRemote) Revoke(context.Context, string, string, string, ed25519.PrivateKey, time.Time) error {
	return nil
}
func (remote *lightFileLegacyTestRemote) OpenFileRelayV1(context.Context, string, string, string, ed25519.PrivateKey, time.Time, cluster.LightFileRequest) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: remote.body()}, nil
}

type lightFileFailingReader struct{}

func (lightFileFailingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type lightFileObservedBody struct {
	io.ReadCloser
	closed chan struct{}
	once   sync.Once
}

func (body *lightFileObservedBody) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(func() { close(body.closed) })
	return err
}

func TestLightFileRelayTruncationAbortsHTTPAndAuditsFailure(t *testing.T) {
	for _, outcome := range []string{"complete", "truncated", "cancel", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			truncated := outcome == "truncated"
			server, tokenPath := newTestServer(t)
			session, csrf := bootstrapCookies(t, server, tokenPath)
			closed := make(chan struct{})
			remote := &lightFileLegacyTestRemote{body: func() io.ReadCloser {
				if outcome == "panic" {
					panic("ordinary handler failure")
				}
				if outcome == "cancel" {
					reader, writer := io.Pipe()
					t.Cleanup(func() { _ = writer.Close() })
					go func() { _, _ = writer.Write([]byte(strings.Repeat("x", 32<<10))) }()
					return &lightFileObservedBody{ReadCloser: reader, closed: closed}
				}
				var reader io.Reader = strings.NewReader(strings.Repeat("x", 32<<10))
				if truncated {
					reader = io.MultiReader(reader, lightFileFailingReader{})
				}
				return io.NopCloser(reader)
			}}
			service, err := cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Remote: remote, Telemetry: clusterTelemetrySource{agent: &stubAgent{}}})
			if err != nil {
				t.Fatal(err)
			}
			_ = server.cluster.Close()
			server.cluster = service
			host, err := service.AddHost(context.Background(), cluster.AddHostInput{Origin: "https://relay.example", PairingCode: "0123456789abcdef." + strings.Repeat("a", 64)})
			if err != nil || !host.FileManagementAvailable {
				t.Fatalf("legacy relay fixture: %#v, %v", host, err)
			}
			httpServer := httptest.NewServer(server)
			defer httpServer.Close()
			request, err := http.NewRequest(http.MethodPut, httpServer.URL+"/api/v1/files/content?hostId="+host.ID, strings.NewReader("content"))
			if err != nil {
				t.Fatal(err)
			}
			request.Host = "panel.test"
			request.Header.Set("Origin", "http://panel.test")
			request.Header.Set("X-CSRF-Token", csrf.Value)
			request.AddCookie(session)
			request.AddCookie(csrf)
			requestContext, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			request = request.WithContext(requestContext)
			client := &http.Client{Timeout: 3 * time.Second}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "cancel" {
				cancelRequest()
			}
			content, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if outcome == "panic" {
				if response.StatusCode != http.StatusInternalServerError || !strings.Contains(string(content), "internal_error") {
					t.Fatalf("ordinary panic response = %d %s", response.StatusCode, content)
				}
				return
			}
			if truncated && readErr == nil {
				t.Fatal("truncated chunked stream ended successfully")
			}
			if outcome == "cancel" {
				if readErr == nil {
					t.Fatal("cancelled download ended successfully")
				}
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("downstream response body was not closed on browser cancellation")
				}
			}
			if outcome == "complete" && (readErr != nil || len(content) != 32<<10) {
				t.Fatalf("complete response length=%d err=%v", len(content), readErr)
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				events, _ := server.store.ListAudit(50, "")
				found := false
				for _, event := range events {
					if event.Action == "file.remote.relay" {
						found = true
					}
				}
				if found {
					break
				}
				time.Sleep(time.Millisecond)
			}
			events, _ := server.store.ListAudit(50, "")
			want := "success"
			if truncated || outcome == "cancel" {
				want = "failure"
			}
			found := false
			for _, event := range events {
				if event.Action == "file.remote.relay" {
					found = true
					if event.Result != want {
						t.Errorf("relay audit = %s, want %s", event.Result, want)
					}
				}
			}
			if !found {
				t.Fatal("relay audit missing")
			}
		})
	}
}

func TestLightFileWritesReuseOriginSessionAndCSRF(t *testing.T) {
	for _, publicURL := range []string{"http://panel.test", ""} {
		t.Run("PublicURL="+publicURL, func(t *testing.T) {
			server, tokenPath := newTestServerWithPublicURL(t, publicURL)
			session, csrf := bootstrapCookies(t, server, tokenPath)
			for _, method := range []string{http.MethodPost, http.MethodPut} {
				for _, test := range []struct{ name, origin, fetchSite, token, want string }{
					{"cross-origin", "https://attacker.test", "", csrf.Value, "origin_validation_failed"},
					{"missing-origin", "", "", csrf.Value, ""},
					{"cross-site-no-origin", "", "cross-site", csrf.Value, "origin_validation_failed"},
					{"same-origin-no-origin", "", "same-origin", csrf.Value, ""},
					{"valid", "http://panel.test", "same-origin", csrf.Value, "file_host_unavailable"},
					{"missing-csrf", "http://panel.test", "", "", "csrf_validation_failed"},
				} {
					t.Run(method+"/"+test.name, func(t *testing.T) {
						want := test.want
						if want == "" {
							if publicURL != "" {
								want = "origin_validation_failed"
							} else {
								want = "file_host_unavailable"
							}
						}
						response := authenticatedRequest(server, method, "/api/v1/files/content?hostId="+strings.Repeat("a", 32), nil, session, csrf,
							map[string]string{"Origin": test.origin, "Sec-Fetch-Site": test.fetchSite, "X-CSRF-Token": test.token})
						if !strings.Contains(response.Body.String(), want) {
							t.Fatalf("status=%d body=%s, want %s", response.Code, response.Body.String(), want)
						}
					})
				}
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := performRequest(server, method, "/api/v1/files?hostId="+strings.Repeat("a", 32), nil, nil)
				if response.Code != http.StatusUnauthorized {
					t.Fatalf("%s without session = %d", method, response.Code)
				}
				response = authenticatedRequest(server, method, "/api/v1/files?hostId="+strings.Repeat("a", 32), nil, session, csrf, map[string]string{"Origin": "https://attacker.test"})
				if response.Code != http.StatusConflict {
					t.Fatalf("authenticated read must keep existing Origin behavior: %d", response.Code)
				}
			}
			response := performRequest(server, http.MethodPut, "/api/v1/files/content?hostId="+strings.Repeat("a", 32), nil, map[string]string{"Origin": "http://panel.test"})
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("write without session = %d", response.Code)
			}
		})
	}
}

func TestLightFileUploadBodyCloseInterruptsRealHTTPRead(t *testing.T) {
	started := make(chan *lightFileUploadBody, 1)
	readDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := &lightFileUploadBody{
			ctx: context.Background(), body: r.Body, controller: http.NewResponseController(w),
		}
		started <- body
		_, err := io.ReadAll(body)
		readDone <- err
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(server.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = fmt.Fprint(conn, "PUT / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\nx")
	if err != nil {
		t.Fatal(err)
	}
	var body *lightFileUploadBody
	select {
	case body = <-started:
	case <-time.After(time.Second):
		t.Fatal("HTTP upload did not start")
	}
	closed := make(chan struct{})
	go func() { _ = body.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind the HTTP body Read")
	}
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("partial upload was treated as complete")
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP body Read did not terminate")
	}
}
