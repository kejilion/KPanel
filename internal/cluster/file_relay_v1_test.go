package cluster

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
			bytes.NewReader([]byte(`{"path":"/"}`)),
		)
		input := LightFileRequest{Method: method, Path: path, RawQuery: query}
		request.Header.Set(FileRelayV1MethodHeader, method)
		request.Header.Set(FileRelayV1PathHeader, path)
		request.Header.Set(FileRelayV1QueryHeader, query)
		nonce := fmt.Sprintf("%032x", nonceIndex)
		if err := SignRequest(request, controllerID, service.NodeID(), controllerPrivate, now, nonce); err != nil {
			t.Fatal(err)
		}
		if signed {
			if err := signFileRelayV1(request, controllerPrivate, input); err != nil {
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
	if _, err := service.AuthorizeFileRelayV1("198.51.100.10", request); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed file relay error = %v, want ErrReplay", err)
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
