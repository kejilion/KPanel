package cluster

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileRelayV1RequestFromHTTPKeepsTheEnvelopeFixed(t *testing.T) {
	body := []byte(`{"paths":["/tmp/example"]}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"https://target.example"+FileRelayV1Path,
		bytes.NewReader(body),
	)
	request.Header.Set(FileRelayV1MethodHeader, http.MethodPut)
	request.Header.Set(FileRelayV1PathHeader, "/v1/files/content")
	request.Header.Set(FileRelayV1QueryHeader, "path=%2Ftmp%2Fexample")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", "ignored")

	input, err := FileRelayV1RequestFromHTTP(request)
	if err != nil {
		t.Fatalf("FileRelayV1RequestFromHTTP() error = %v", err)
	}
	if input.Method != http.MethodPut || input.Path != "/v1/files/content" ||
		input.RawQuery != "path=%2Ftmp%2Fexample" || input.BodyLength != int64(len(body)) {
		t.Fatalf("decoded file request = %#v", input)
	}
	if input.Headers["Content-Type"] != "application/json" || len(input.Headers) != 1 {
		t.Fatalf("decoded forwarded headers = %#v", input.Headers)
	}
	decoded, err := io.ReadAll(input.Body)
	if err != nil || !bytes.Equal(decoded, body) {
		t.Fatalf("decoded body = %q, error = %v", decoded, err)
	}

	for _, mutate := range []func(*http.Request){
		func(value *http.Request) { value.Method = http.MethodGet },
		func(value *http.Request) { value.URL.Path = "/api/v1/federation/other" },
		func(value *http.Request) { value.URL.RawQuery = "unexpected=1" },
		func(value *http.Request) { value.Header.Set(FileRelayV1PathHeader, "/v1/terminal") },
	} {
		invalid := request.Clone(context.Background())
		invalid.Body = io.NopCloser(bytes.NewReader(body))
		mutate(invalid)
		if _, err := FileRelayV1RequestFromHTTP(invalid); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("invalid envelope error = %v, want ErrAuthentication", err)
		}
	}
}

func TestFileRelayV1SpoolBoundsConcurrentBodiesAndCleansUp(t *testing.T) {
	directory := t.TempDir()
	slots := make(chan struct{}, 1)
	body := "signed body"
	spool, digest, length, err := spoolFileRelayV1Body(directory, slots, strings.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("spoolFileRelayV1Body() error = %v", err)
	}
	if length != int64(len(body)) || digest != sha256.Sum256([]byte(body)) {
		t.Fatalf("spooled body metadata = length %d, digest %x", length, digest)
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
		t.Fatalf("spool files while open = %d, %v", len(entries), err)
	}
	if _, _, _, err := spoolFileRelayV1Body(directory, slots, strings.NewReader(body), int64(len(body))); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second concurrent spool error = %v, want ErrRateLimited", err)
	}
	if err := spool.Close(); err != nil {
		t.Fatalf("close spool: %v", err)
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
		t.Fatalf("spool files after close = %d, %v", len(entries), err)
	}
	if _, _, _, err := spoolFileRelayV1Body(directory, slots, strings.NewReader("long"), 2); err == nil {
		t.Fatal("spool accepted a body that differs from Content-Length")
	}
	if len(slots) != 0 {
		t.Fatal("failed spool did not release its concurrency slot")
	}
	if _, _, _, err := spoolFileRelayV1Body(directory, slots, http.NoBody, maxFileRelayV1BodyBytes+1); err == nil {
		t.Fatal("spool accepted a declared body above the size limit")
	}
}

func TestLegacyFileRelayRequiresVerifiedTLSAndExplicitGrant(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	service := newLightServiceForTest(t, &serviceTestClock{now: now})
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controllerID := strings.Repeat("e", 32)
	code, err := service.CreatePairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptPair("198.51.100.10", PairRequest{
		PairingCode: code.Code, ControllerID: controllerID, ControllerName: "legacy",
		PublicKey: encodePublicKey(public), FederationProtocol: FederationProtocol,
	}); err != nil {
		t.Fatal(err)
	}
	const original = "authorized file contents"
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		input, err := service.AuthorizeFileRelayV1("198.51.100.10", r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		defer input.Body.(io.Closer).Close()
		body, err := io.ReadAll(io.LimitReader(input.Body, 1024))
		if err != nil || string(body) != original || input.RawQuery != "path=%2Ftmp%2Fapproved.txt" {
			t.Errorf("authenticated TLS request changed: %q %v", body, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	origin := "https://example.com:" + port
	// Preserve the approved-address gate and real certificate validation;
	// only the last test dial maps the approved public address to loopback.
	config := RemoteClientConfig{
		Resolver: staticResolver{"example.com": {net.ParseIP("8.8.8.8")}},
		Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	untrusted, err := NewRemoteClient(config)
	if err != nil {
		t.Fatal(err)
	}
	open := func(client *RemoteClient, origin string) (*http.Response, error) {
		return client.OpenSignedFileRelayV1(context.Background(), origin, controllerID, service.NodeID(), private, now, LightFileRequest{
			Method: http.MethodPut, Path: "/v1/files/content", RawQuery: "path=%2Ftmp%2Fapproved.txt",
			Body: strings.NewReader(original), BodyLength: int64(len(original)),
		})
	}
	if response, err := open(untrusted, origin); err == nil {
		response.Body.Close()
		t.Fatal("legacy relay trusted an unverified TLS certificate")
	}
	if calls.Load() != 0 {
		t.Fatal("unverified TLS reached authorization")
	}
	config.RootCAs = x509.NewCertPool()
	config.RootCAs.AddCert(server.Certificate())
	trusted, err := NewRemoteClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := open(trusted, strings.Replace(origin, "https:", "http:", 1)); err == nil {
		response.Body.Close()
		t.Fatal("legacy relay downgraded to HTTP")
	}
	response, err := open(trusted, origin)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("summary-only pairing reached a file write")
	}
	if _, err := service.SetControllerFileRelay(controllerID, true); err != nil {
		t.Fatal(err)
	}
	response, err = open(trusted, origin)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || calls.Load() != 2 {
		t.Fatalf("granted legacy write failed: status=%d requests=%d", response.StatusCode, calls.Load())
	}
	t.Logf("real TLS: untrusted certificate/HTTP rejected before handler; summary-only denied; one granted write delivered %d bytes to its signed path", len(original))
}

func TestAuthorizeFileRelayV1RequiresTargetGrantAndBoundSignature(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	controllerPublic, controllerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controllerID := strings.Repeat("a", 32)
	code, err := service.CreatePairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptPair("198.51.100.10", PairRequest{
		PairingCode: code.Code, ControllerID: controllerID, ControllerName: "controller",
		PublicKey: encodePublicKey(controllerPublic), FederationProtocol: FederationProtocol,
	}); err != nil {
		t.Fatalf("AcceptPair() error = %v", err)
	}

	nonceIndex := 0
	relayRequest := func(method, path, query string, signed bool, tamper func(*http.Request)) *http.Request {
		t.Helper()
		nonceIndex++
		request := httptest.NewRequest(
			http.MethodPost,
			"https://target.example"+FileRelayV1Path,
			http.NoBody,
		)
		input := LightFileRequest{Method: method, Path: path, RawQuery: query, Body: http.NoBody}
		request.Header.Set(FileRelayV1MethodHeader, method)
		request.Header.Set(FileRelayV1PathHeader, path)
		request.Header.Set(FileRelayV1QueryHeader, query)
		nonce := fmt.Sprintf("%032x", nonceIndex)
		if err := SignRequest(request, controllerID, service.NodeID(), controllerPrivate, now, nonce); err != nil {
			t.Fatal(err)
		}
		if signed {
			if err := signFileRelayV1(request, controllerPrivate, input, sha256.Sum256(nil)); err != nil {
				t.Fatal(err)
			}
		}
		if tamper != nil {
			tamper(request)
		}
		return request
	}
	read := func(signed bool, tamper func(*http.Request)) *http.Request {
		return relayRequest(http.MethodGet, "/v1/files", "path=%2F&limit=100", signed, tamper)
	}

	// A v1 pairing authorizes read-only summaries only.
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", read(true, nil)); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("ungranted relay error = %v, want ErrAuthentication", err)
	}
	if controllers := service.Controllers(); len(controllers) != 1 || controllers[0].Scope != SummaryScope ||
		!controllers[0].FileRelayConfigurable {
		t.Fatalf("summary-only controller view = %#v", controllers)
	}

	granted, err := service.SetControllerFileRelay(controllerID, true)
	if err != nil {
		t.Fatalf("SetControllerFileRelay(true) error = %v", err)
	}
	if granted.Scope != SummaryFilesScope || !granted.FileRelayConfigurable {
		t.Fatalf("granted controller view = %#v", granted)
	}
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", read(false, nil)); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("unsigned relay error = %v, want ErrAuthentication", err)
	}
	// Rewriting a signed read into a write must fail even though the outer
	// signature, which covers only the fixed relay path, still verifies.
	tampered := read(true, func(request *http.Request) {
		request.Header.Set(FileRelayV1MethodHeader, http.MethodPut)
		request.Header.Set(FileRelayV1PathHeader, "/v1/files/content")
		request.Header.Set(FileRelayV1QueryHeader, "path=%2Froot%2F.profile")
	})
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", tampered); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("tampered relay error = %v, want ErrAuthentication", err)
	}
	request := read(true, nil)
	input, err := service.AuthorizeFileRelayV1("198.51.100.10", request)
	if err != nil {
		t.Fatalf("AuthorizeFileRelayV1() error = %v", err)
	}
	if input.Method != http.MethodGet || input.Path != "/v1/files" {
		t.Fatalf("authorized file request = %#v", input)
	}
	defer input.Body.(io.Closer).Close()
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", request); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed file relay error = %v, want ErrReplay", err)
	}
	bodyRequest := httptest.NewRequest(http.MethodPost, "https://target.example"+FileRelayV1Path, strings.NewReader("original"))
	bodyInput := LightFileRequest{Method: http.MethodPut, Path: "/v1/files/content", RawQuery: "path=%2Ftmp%2Fapproved", Body: strings.NewReader("original"), BodyLength: int64(len("original"))}
	bodyRequest.Header.Set(FileRelayV1MethodHeader, bodyInput.Method)
	bodyRequest.Header.Set(FileRelayV1PathHeader, bodyInput.Path)
	bodyRequest.Header.Set(FileRelayV1QueryHeader, bodyInput.RawQuery)
	if err := SignRequest(bodyRequest, controllerID, service.NodeID(), controllerPrivate, now, fmt.Sprintf("%032x", nonceIndex+1)); err != nil {
		t.Fatal(err)
	}
	if err := signFileRelayV1(bodyRequest, controllerPrivate, bodyInput, sha256.Sum256([]byte("original"))); err != nil {
		t.Fatal(err)
	}
	bodyRequest.Body = io.NopCloser(strings.NewReader("tampered"))
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", bodyRequest); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("body-tampered relay error = %v, want ErrAuthentication", err)
	}

	withdrawn, err := service.SetControllerFileRelay(controllerID, false)
	if err != nil || withdrawn.Scope != SummaryScope {
		t.Fatalf("SetControllerFileRelay(false) = %#v, %v", withdrawn, err)
	}
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", read(true, nil)); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("withdrawn relay error = %v, want ErrAuthentication", err)
	}

	// Revocation removes the grant; a later pairing with the same ID starts
	// summary-only again.
	if _, err := service.SetControllerFileRelay(controllerID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteController(controllerID); err != nil {
		t.Fatalf("DeleteController() error = %v", err)
	}
	if service.fileRelayV1Grants.Granted(controllerID, fingerprint(controllerPublic)) {
		t.Fatal("revoked controller kept its file relay grant")
	}
	if _, err := service.SetControllerFileRelay(controllerID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant for revoked controller error = %v, want ErrNotFound", err)
	}
}

func newGrantedV1FileRelayController(t *testing.T, now time.Time) (*Service, string, ed25519.PrivateKey) {
	t.Helper()
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controllerID := strings.Repeat("b", 32)
	code, err := service.CreatePairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptPair("198.51.100.11", PairRequest{
		PairingCode: code.Code, ControllerID: controllerID, ControllerName: "controller",
		PublicKey: encodePublicKey(publicKey), FederationProtocol: FederationProtocol,
	}); err != nil {
		t.Fatalf("AcceptPair() error = %v", err)
	}
	if _, err := service.SetControllerFileRelay(controllerID, true); err != nil {
		t.Fatalf("SetControllerFileRelay(true) error = %v", err)
	}
	return service, controllerID, privateKey
}

func signedV1FileUploadRequest(t *testing.T, service *Service, controllerID string, privateKey ed25519.PrivateKey, now time.Time, nonce string, body []byte) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://target.example"+FileRelayV1Path, bytes.NewReader(body))
	input := LightFileRequest{
		Method: http.MethodPut, Path: "/v1/files/content", RawQuery: "path=%2Ftmp%2Fapproved",
		Headers: map[string]string{"Content-Type": "text/plain"}, Body: bytes.NewReader(body), BodyLength: int64(len(body)),
	}
	request.Header.Set(FileRelayV1MethodHeader, input.Method)
	request.Header.Set(FileRelayV1PathHeader, input.Path)
	request.Header.Set(FileRelayV1QueryHeader, input.RawQuery)
	if err := SignRequest(request, controllerID, service.NodeID(), privateKey, now, nonce); err != nil {
		t.Fatal(err)
	}
	if err := signFileRelayV1(request, privateKey, input, sha256.Sum256(body)); err != nil {
		t.Fatal(err)
	}
	return request
}

type fileRelayCountingReader struct {
	reader io.Reader
	reads  int
}

func (r *fileRelayCountingReader) Read(buffer []byte) (int, error) {
	r.reads++
	return r.reader.Read(buffer)
}

type fileRelayGateReader struct {
	reader  *bytes.Reader
	started chan struct{}
	release chan struct{}
	blocked bool
}

func (r *fileRelayGateReader) Read(buffer []byte) (int, error) {
	if !r.blocked {
		r.blocked = true
		close(r.started)
		<-r.release
	}
	return r.reader.Read(buffer)
}

func TestAuthorizeFileRelayV1RejectsReplayBeforeReadingBody(t *testing.T) {
	now := time.Date(2026, 9, 4, 13, 0, 0, 0, time.UTC)
	service, controllerID, privateKey := newGrantedV1FileRelayController(t, now)
	body := []byte("signed upload body")
	request := signedV1FileUploadRequest(t, service, controllerID, privateKey, now, fmt.Sprintf("%032x", 1), body)
	authorized, err := service.AuthorizeFileRelayV1("198.51.100.11", request)
	if err != nil {
		t.Fatalf("AuthorizeFileRelayV1() error = %v", err)
	}
	if closer, ok := authorized.Body.(io.Closer); ok {
		defer closer.Close()
	}

	reader := &fileRelayCountingReader{reader: bytes.NewReader(body)}
	replay := request.Clone(context.Background())
	replay.Body = io.NopCloser(reader)
	if _, err := service.AuthorizeFileRelayV1("198.51.100.11", replay); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed upload error = %v, want ErrReplay", err)
	}
	if reader.reads != 0 {
		t.Fatalf("replayed request body was read %d times before rejection", reader.reads)
	}
}

func TestAuthorizeFileRelayV1RechecksGrantAfterSpoolingBody(t *testing.T) {
	now := time.Date(2026, 9, 4, 14, 0, 0, 0, time.UTC)
	service, controllerID, privateKey := newGrantedV1FileRelayController(t, now)
	body := []byte("slow signed upload body")
	request := signedV1FileUploadRequest(t, service, controllerID, privateKey, now, fmt.Sprintf("%032x", 2), body)
	gate := &fileRelayGateReader{reader: bytes.NewReader(body), started: make(chan struct{}), release: make(chan struct{})}
	request.Body = io.NopCloser(gate)
	type authorizationResult struct {
		input LightFileRequest
		err   error
	}
	done := make(chan authorizationResult, 1)
	go func() {
		input, err := service.AuthorizeFileRelayV1("198.51.100.11", request)
		done <- authorizationResult{input: input, err: err}
	}()
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("authorization did not begin spooling the request body")
	}
	if _, err := service.SetControllerFileRelay(controllerID, false); err != nil {
		t.Fatalf("revoke file relay grant during body upload: %v", err)
	}
	close(gate.release)
	result := <-done
	if !errors.Is(result.err, ErrAuthentication) {
		if closer, ok := result.input.Body.(io.Closer); ok {
			_ = closer.Close()
		}
		t.Fatalf("authorization after revocation error = %v, want ErrAuthentication", result.err)
	}
	entries, err := os.ReadDir(service.fileRelaySpoolDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("revoked request left %d spool files", len(entries))
	}
}

func TestFileRelayV1GrantIsBoundToTheControllerKey(t *testing.T) {
	store, err := openFileRelayV1GrantStore(filepath.Join(t.TempDir(), fileRelayV1GrantFileName))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("c", 32)
	if err := store.Set(id, "SHA256:first", true, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if !store.Granted(id, "SHA256:first") || store.Granted(id, "SHA256:second") {
		t.Fatal("grant is not bound to the granted controller key")
	}
	reopened, err := openFileRelayV1GrantStore(store.path)
	if err != nil {
		t.Fatalf("reopen grant store: %v", err)
	}
	if !reopened.Granted(id, "SHA256:first") {
		t.Fatal("grant was not persisted")
	}
	if err := reopened.Delete(id); err != nil || reopened.Granted(id, "SHA256:first") {
		t.Fatalf("Delete() error = %v", err)
	}
}
