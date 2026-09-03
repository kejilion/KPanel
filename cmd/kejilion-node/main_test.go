package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func lightTokenForTest(t *testing.T, origin string, expiresAt time.Time) string {
	t.Helper()
	wire := tokenWire{
		Version: 1, Origin: origin, ID: strings.Repeat("a", 32),
		Secret: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), ExpiresAt: expiresAt.Unix(),
	}
	content, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return lightTokenPrefix + base64.RawURLEncoding.EncodeToString(content)
}

func TestOriginFromTokenRequiresStrictHTTPSOriginAndFutureExpiry(t *testing.T) {
	valid := lightTokenForTest(t, "https://panel.example:8443", time.Now().UTC().Add(time.Minute))
	if origin, err := originFromToken(valid); err != nil || origin != "https://panel.example:8443" {
		t.Fatalf("originFromToken(valid) = %q, %v", origin, err)
	}
	rollingUpgrade := lightTokenForTest(t, "https://panel.example", time.Now().UTC().Add(30*time.Minute))
	if origin, err := originFromToken(rollingUpgrade); err != nil || origin != "https://panel.example" {
		t.Fatalf("originFromToken(rolling upgrade token) = %q, %v", origin, err)
	}
	for _, token := range []string{
		lightTokenForTest(t, "http://panel.example", time.Now().UTC().Add(time.Minute)),
		lightTokenForTest(t, "https://user:pass@panel.example", time.Now().UTC().Add(time.Minute)),
		lightTokenForTest(t, "https://panel.example/path", time.Now().UTC().Add(time.Minute)),
		lightTokenForTest(t, "https://panel.example", time.Now().UTC().Add(-time.Minute)),
		lightTokenForTest(t, "https://panel.example", time.Now().UTC().Add(61*time.Minute)),
		"kpl1.invalid",
	} {
		if _, err := originFromToken(token); err == nil {
			t.Fatalf("originFromToken(%q) accepted an unsafe token", token)
		}
	}
}

func TestNodeConfigRoundTripIsStrictAndRejectsNonRegularTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.json")
	config := nodeConfig{
		SchemaVersion: 1, Origin: "https://panel.example", NodeID: strings.Repeat("b", 32),
		ReportingKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), ReportInterval: 30,
	}
	if err := writeConfigAtomic(path, config); err != nil {
		t.Fatalf("writeConfigAtomic() error = %v", err)
	}
	loaded, secret, err := readConfig(path)
	if err != nil || loaded != config || len(secret) != 32 {
		t.Fatalf("readConfig() = %#v, %d, %v", loaded, len(secret), err)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("configuration is not a regular file: %#v, %v", info, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content[:len(content)-2], []byte(`,"unexpected":true}`+"\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readConfig(path); err == nil {
		t.Fatal("readConfig() accepted an unknown field")
	}
	directoryTarget := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(directoryTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigAtomic(directoryTarget, config); err == nil {
		t.Fatal("writeConfigAtomic() accepted a directory target")
	}
}

func TestLightNodeLearnsSSHCapabilityFromReportResponse(t *testing.T) {
	config := nodeConfig{
		SchemaVersion: 1, Origin: "https://panel.example", NodeID: strings.Repeat("b", 32),
		ReportingKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), ReportInterval: 30,
	}
	headers := http.Header{}
	headers.Set(cluster.LightResponseCapabilitiesHeader, cluster.SSHLoginCapability)
	updated := enableSSHLoginCapability(config, headers)
	if !updated.SSHLogin {
		t.Fatalf("enableSSHLoginCapability() = %#v", updated)
	}
	legacy := enableSSHLoginCapability(updated, nil)
	if legacy.SSHLogin {
		t.Fatalf("missing response capability kept SSH login enabled: %#v", legacy)
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		if !shouldRetryLightReportWithoutSSHLogin(status) {
			t.Fatalf("status %d was not marked for compatibility retry", status)
		}
	}
	if shouldRetryLightReportWithoutSSHLogin(http.StatusUnauthorized) {
		t.Fatal("authentication failure was marked for compatibility retry")
	}
}

func TestLightReportResponseMatchesPanelContract(t *testing.T) {
	var response reportResponse
	decoder := json.NewDecoder(strings.NewReader(`{"acceptedAt":"2026-09-03T00:00:00Z","nextReportSeconds":30}`))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("light report response was rejected: %v", err)
	}
	if response.AcceptedAt.IsZero() || response.NextReport != 30 {
		t.Fatalf("unexpected light report response: %#v", response)
	}
}

func TestTerminalConfigRoundTripIsStrictAndRootOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.json")
	config := terminalConfig{
		SchemaVersion: 1,
		PrivateKey:    base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
		PublicKey:     base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
		PeerPublicKey: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)),
	}
	if err := writeTerminalConfigAtomic(path, config); err != nil {
		t.Fatalf("writeTerminalConfigAtomic() error = %v", err)
	}
	loaded, identity, err := readTerminalConfig(path)
	if err != nil || loaded != config || !bytes.Equal(identity.Key.Private, bytes.Repeat([]byte{1}, 32)) ||
		!bytes.Equal(identity.Key.Public, bytes.Repeat([]byte{2}, 32)) || !bytes.Equal(identity.Peer, bytes.Repeat([]byte{3}, 32)) {
		t.Fatalf("readTerminalConfig() = %#v, %#v, %v", loaded, identity, err)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("terminal configuration is not a regular file: %#v, %v", info, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readTerminalConfig(path); err == nil {
			t.Fatal("readTerminalConfig() accepted group-readable private identity")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content[:len(content)-2], []byte(`,"unexpected":true}`+"\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readTerminalConfig(path); err == nil {
		t.Fatal("readTerminalConfig() accepted an unknown field")
	}
}

func TestNodeHTTPClientDoesNotFollowRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	var output map[string]any
	err := postRawJSON(t.Context(), redirect.URL, []byte(`{}`), nil, &output)
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("postRawJSON() error = %v, want redirect rejection", err)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target received %d requests", targetCalls)
	}
}

func TestSignedLightNodeHeadersCarryPreviousReportLatencyOutOfBand(t *testing.T) {
	config := nodeConfig{NodeID: strings.Repeat("a", 32)}
	latency := int64(42)
	headers, err := signedLightNodeHeaders(config, lightReportPath, []byte(`{"telemetry":{}}`), make([]byte, 32), &latency)
	if err != nil {
		t.Fatalf("signedLightNodeHeaders() error = %v", err)
	}
	if headers[cluster.LightReportLatencyHeader] != "42" {
		t.Fatalf("report latency header = %q, want 42", headers[cluster.LightReportLatencyHeader])
	}
	withoutLatency, err := signedLightNodeHeaders(config, lightReportPath, []byte(`{"telemetry":{}}`), make([]byte, 32), nil)
	if err != nil {
		t.Fatalf("signedLightNodeHeaders() without latency error = %v", err)
	}
	if _, ok := withoutLatency[cluster.LightReportLatencyHeader]; ok {
		t.Fatal("first report unexpectedly carried an unknown latency")
	}
}

func TestElapsedMillisecondsDoesNotTruncateSuccessfulSubMillisecondRequestsToZero(t *testing.T) {
	if got := elapsedMilliseconds(500 * time.Microsecond); got != 1 {
		t.Fatalf("elapsedMilliseconds(500us) = %d, want 1", got)
	}
	if got := elapsedMilliseconds(0); got != 0 {
		t.Fatalf("elapsedMilliseconds(0) = %d, want 0", got)
	}
}

func TestRunRejectsUnsupportedCommands(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("run(nil) succeeded")
	}
	if err := run([]string{"shell"}); err == nil {
		t.Fatal("run(shell) exposed an unsupported command")
	}
	if err := run([]string{"run", "unexpected"}); err == nil {
		t.Fatal("run accepted an unexpected argument")
	}
	if _, err := validateHTTPSOrigin("https://panel.example/?token=secret"); err == nil {
		t.Fatal("validateHTTPSOrigin accepted a query string")
	}
	if !errors.Is(newHTTPClient().CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("HTTP client redirect policy changed")
	}
}
