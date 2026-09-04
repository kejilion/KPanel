package cluster

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestAuthorizeFileRelayV1ReusesTheExistingPanelCredential(t *testing.T) {
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

	body := []byte(`{"path":"/"}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"https://target.example"+FileRelayV1Path,
		bytes.NewReader(body),
	)
	request.Header.Set(FileRelayV1MethodHeader, http.MethodGet)
	request.Header.Set(FileRelayV1PathHeader, "/v1/files")
	request.Header.Set(FileRelayV1QueryHeader, "path=%2F&limit=100")
	nonce := strings.Repeat("b", 32)
	if err := SignRequest(request, controllerID, service.NodeID(), controllerPrivate, now, nonce); err != nil {
		t.Fatal(err)
	}
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
}
