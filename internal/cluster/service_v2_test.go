package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type serviceV2TerminalStub struct {
	mu      sync.Mutex
	owner   string
	input   []byte
	resized bool
	closed  bool
	output  []byte
}

func (s *serviceV2TerminalStub) Open(_ context.Context, owner string, _, _ uint16) (terminal.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owner = owner
	return terminal.Snapshot{ID: "remote-terminal", CreatedAt: time.Now().UTC()}, nil
}

func (s *serviceV2TerminalStub) Output(_ context.Context, owner, id string, offset int64, _ time.Duration) (terminal.Output, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner || id != "remote-terminal" {
		return terminal.Output{}, terminal.ErrNotFound
	}
	return terminal.Output{Data: append([]byte(nil), s.output...), Offset: offset, NextOffset: offset + int64(len(s.output))}, nil
}

func (s *serviceV2TerminalStub) Input(_ context.Context, owner, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner || id != "remote-terminal" {
		return terminal.ErrNotFound
	}
	s.input = append([]byte(nil), data...)
	return nil
}

func (s *serviceV2TerminalStub) Resize(_ context.Context, owner, id string, _, _ uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner || id != "remote-terminal" {
		return terminal.ErrNotFound
	}
	s.resized = true
	return nil
}

func (s *serviceV2TerminalStub) Close(_ context.Context, owner, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner || id != "remote-terminal" {
		return terminal.ErrNotFound
	}
	s.closed = true
	return nil
}

type serviceV2RoundTripper struct {
	mu            sync.Mutex
	target        *Service
	bodies        [][]byte
	failPath      string
	blockPath     string
	blockReached  chan struct{}
	blockRelease  <-chan struct{}
	blockSignaled bool
}

func (t *serviceV2RoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.bodies = append(t.bodies, append([]byte(nil), body...))
	fail := request.URL.Path == t.failPath
	block := request.URL.Path == t.blockPath
	reached := t.blockReached
	release := t.blockRelease
	if block && !t.blockSignaled && reached != nil {
		close(reached)
		t.blockSignaled = true
	}
	t.mu.Unlock()
	if fail {
		return nil, errors.New("simulated federation transport failure")
	}
	if block && release != nil {
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}
	var envelope FederationEnvelopeV2
	if err := decodeStrictJSONV2(body, &envelope); err != nil {
		return serviceV2HTTPResponse(
			http.StatusBadRequest, map[string]string{"error": "invalid"},
		), nil
	}
	response, err := t.target.HandleFederationV2(
		request.Context(), "198.51.100.10", request.URL.Path, envelope,
	)
	if err != nil {
		status := http.StatusUnauthorized
		switch {
		case errors.Is(err, ErrRateLimited):
			status = http.StatusTooManyRequests
		case errors.Is(err, ErrConflict), errors.Is(err, ErrReplay):
			status = http.StatusConflict
		case errors.Is(err, ErrProtocolMismatch):
			status = http.StatusUpgradeRequired
		}
		return serviceV2HTTPResponse(
			status, map[string]string{"error": "rejected"},
		), nil
	}
	return serviceV2HTTPResponse(http.StatusOK, response), nil
}

func (t *serviceV2RoundTripper) requestBodies() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([][]byte, len(t.bodies))
	for index := range t.bodies {
		result[index] = append([]byte(nil), t.bodies[index]...)
	}
	return result
}

func (t *serviceV2RoundTripper) setFailurePath(path string) {
	t.mu.Lock()
	t.failPath = path
	t.mu.Unlock()
}

func (t *serviceV2RoundTripper) block(
	path string,
	reached chan struct{},
	release <-chan struct{},
) {
	t.mu.Lock()
	t.blockPath = path
	t.blockReached = reached
	t.blockRelease = release
	t.blockSignaled = false
	t.mu.Unlock()
}

func serviceV2HTTPResponse(status int, value any) *http.Response {
	content, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(content)),
	}
}

func newServiceV2Remote(t *testing.T) (*RemoteClient, *serviceV2RoundTripper) {
	t.Helper()
	client, err := NewRemoteClient(RemoteClientConfig{})
	if err != nil {
		t.Fatalf("NewRemoteClient() error = %v", err)
	}
	transport := &serviceV2RoundTripper{}
	client.client = &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	return client, transport
}

func TestServiceV2PairsPollsAndRevokesOverEncryptedHTTP(t *testing.T) {
	now := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "target"),
		PanelVersion: "v0.27.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "target-v2",
		},
		Remote: targetRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "center"),
		PanelVersion: "v0.27.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "center-v2",
		},
		Remote: centerRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "public-ip-node", Origin: "http://8.8.8.8:1801",
		PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.State != HostOnline ||
		host.TransportSecurity != TransportSecurityEncryptedHTTP ||
		host.FederationProtocol != FederationProtocolV2 ||
		host.LastSnapshot == nil ||
		host.LastSnapshot.Telemetry.Hostname != "target-v2" ||
		!strings.HasPrefix(host.PeerFingerprint, "sha256:") {
		t.Fatalf("unexpected paired host: %#v", host)
	}
	controllers := target.Controllers()
	if len(controllers) != 1 ||
		controllers[0].ID == "" ||
		controllers[0].Fingerprint == "" {
		t.Fatalf("unexpected target controllers: %#v", controllers)
	}
	requestBodies := route.requestBodies()
	for _, body := range requestBodies {
		if bytes.Contains(body, []byte(code.Code)) ||
			bytes.Contains(body, []byte("center-v2")) ||
			bytes.Contains(body, []byte("target-v2")) {
			t.Fatalf("federation plaintext leaked on HTTP wire: %s", body)
		}
	}
	if len(requestBodies) < 3 {
		t.Fatalf("encrypted pair/commit/summary requests missing: %d", len(requestBodies))
	}
	var replayedSummary FederationEnvelopeV2
	if err := decodeStrictJSONV2(
		requestBodies[len(requestBodies)-1],
		&replayedSummary,
	); err != nil {
		t.Fatalf("decode encrypted summary request: %v", err)
	}
	if _, err := target.HandleFederationV2(
		context.Background(),
		"198.51.100.10",
		v2SummaryPath,
		replayedSummary,
	); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed encrypted summary error = %v, want ErrReplay", err)
	}
	if err := target.DeleteController(controllers[0].ID); err != nil {
		t.Fatalf("manually revoke target controller: %v", err)
	}

	result, err := center.DeleteHost(context.Background(), host.ID, DeleteHostInput{
		ExpectedResourceVersion: host.ResourceVersion,
	})
	if err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if !result.Deleted || !result.RemoteRevoked ||
		!result.CredentialRemoved {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if _, err := center.Host(context.Background(), host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted center host error = %v, want ErrNotFound", err)
	}
	if controllers := target.Controllers(); len(controllers) != 0 {
		t.Fatalf("revoked controller remained visible: %#v", controllers)
	}
}

func TestServiceV2TerminalLifecycleUsesAuthenticatedEncryptedChannel(t *testing.T) {
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	terminalStub := &serviceV2TerminalStub{output: []byte("remote-ready\r\n")}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.38.3", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"}, Terminal: terminalStub,
		Remote: targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.38.3", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{Name: "terminal-target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if !host.TerminalAvailable || host.Scope != SummaryTerminalScope {
		t.Fatalf("terminal capability missing after v2 pairing: %#v", host)
	}

	opened, err := center.TerminalOpen(context.Background(), host.ID, TerminalOpenRequest{Rows: 30, Columns: 120})
	if err != nil || opened.SessionID != "remote-terminal" {
		t.Fatalf("TerminalOpen() = %#v, %v", opened, err)
	}
	output, err := center.TerminalOutput(context.Background(), host.ID, TerminalOutputRequest{SessionID: opened.SessionID, Offset: 0})
	if err != nil || string(output.Data) != "remote-ready\r\n" {
		t.Fatalf("TerminalOutput() = %#v, %v", output, err)
	}
	// Terminal polling has a dedicated bounded limiter. It must not inherit the
	// low-frequency summary limit and disconnect a healthy shell after 30 polls.
	for index := 0; index < 40; index++ {
		if _, err := center.TerminalOutput(context.Background(), host.ID, TerminalOutputRequest{SessionID: opened.SessionID, Offset: 0}); err != nil {
			t.Fatalf("TerminalOutput() poll %d error = %v", index+1, err)
		}
	}
	terminalStub.mu.Lock()
	terminalStub.output = bytes.Repeat([]byte("x"), terminal.MaxOutputBytes)
	terminalStub.mu.Unlock()
	bounded, err := center.TerminalOutput(context.Background(), host.ID, TerminalOutputRequest{SessionID: opened.SessionID, Offset: 0})
	if err != nil || len(bounded.Data) != maxFederationTerminalOutputBytes || bounded.NextOffset != int64(maxFederationTerminalOutputBytes) {
		t.Fatalf("bounded encrypted output = %d bytes, next offset %d, error %v", len(bounded.Data), bounded.NextOffset, err)
	}
	command := []byte("printf terminal-secret\r\n")
	if err := center.TerminalInput(context.Background(), host.ID, TerminalInputRequest{SessionID: opened.SessionID, Data: base64.RawStdEncoding.EncodeToString(command)}); err != nil {
		t.Fatalf("TerminalInput() error = %v", err)
	}
	if err := center.TerminalResize(context.Background(), host.ID, TerminalResizeRequest{SessionID: opened.SessionID, Rows: 40, Columns: 140}); err != nil {
		t.Fatalf("TerminalResize() error = %v", err)
	}
	if err := center.TerminalClose(context.Background(), host.ID, TerminalCloseRequest{SessionID: opened.SessionID}); err != nil {
		t.Fatalf("TerminalClose() error = %v", err)
	}

	terminalStub.mu.Lock()
	if string(terminalStub.input) != string(command) || !terminalStub.resized || !terminalStub.closed || !strings.HasPrefix(terminalStub.owner, "federation:") {
		terminalStub.mu.Unlock()
		t.Fatalf("unexpected target terminal lifecycle: %#v", terminalStub)
	}
	terminalStub.mu.Unlock()
	for _, body := range route.requestBodies() {
		if bytes.Contains(body, command) || bytes.Contains(body, []byte("terminal-secret")) {
			t.Fatalf("terminal plaintext leaked on HTTP wire: %s", body)
		}
	}
}

// TestServiceV2PairingGrantsExactlyTheRequestedScope pairs against two
// independent targets using differently-scoped pairing codes and asserts the
// resulting Host.BrowseAvailable/TerminalAvailable reflect exactly what each
// pairing code's creator chose — not a hardcoded default, and not silently
// "granted because terminal was granted" or vice versa. Two independent
// target/center pairs are used (rather than pairing the same target twice)
// because AddHost rejects re-adding the same remote node under one center.
func TestServiceV2PairingGrantsExactlyTheRequestedScope(t *testing.T) {
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	pairWithScope := func(t *testing.T, name string, scope string) Host {
		t.Helper()
		targetRemote, _ := newServiceV2Remote(t)
		target, err := NewService(ServiceConfig{
			DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.72.0", Hostname: name + "-target",
			Telemetry: serviceTestTelemetry{now: clock.Now, hostname: name + "-target"},
			Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
		})
		if err != nil {
			t.Fatalf("target NewService() error = %v", err)
		}
		t.Cleanup(func() { _ = target.Close() })

		centerRemote, centerRoute := newServiceV2Remote(t)
		centerRoute.target = target
		center, err := NewService(ServiceConfig{
			DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.72.0", Hostname: name + "-center",
			Telemetry: serviceTestTelemetry{now: clock.Now, hostname: name + "-center"},
			Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
		})
		if err != nil {
			t.Fatalf("center NewService() error = %v", err)
		}
		t.Cleanup(func() { _ = center.Close() })

		code, err := target.CreatePairingCodeV2(scope)
		if err != nil {
			t.Fatalf("CreatePairingCodeV2(%q) error = %v", scope, err)
		}
		host, err := center.AddHost(context.Background(), AddHostInput{
			Name: name, Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		})
		if err != nil {
			t.Fatalf("AddHost(%s) error = %v", name, err)
		}
		return host
	}

	terminalOnly := pairWithScope(t, "terminal-only", BuildV2Scope(true, false, false))
	if !terminalOnly.TerminalAvailable || terminalOnly.BrowseAvailable || terminalOnly.BrowseWSAvailable {
		t.Fatalf("terminal-only pairing granted unexpected capabilities: %#v", terminalOnly)
	}

	terminalAndBrowse := pairWithScope(t, "terminal-and-browse", BuildV2Scope(true, true, false))
	if !terminalAndBrowse.TerminalAvailable || !terminalAndBrowse.BrowseAvailable || terminalAndBrowse.BrowseWSAvailable {
		t.Fatalf("terminal+browse pairing has unexpected capabilities: %#v", terminalAndBrowse)
	}
	if terminalAndBrowse.Scope != "cluster.summary.read cluster.terminal.open cluster.browse.fetch" {
		t.Fatalf("unexpected stored scope: %q", terminalAndBrowse.Scope)
	}

	browseOnly := pairWithScope(t, "browse-only", BuildV2Scope(false, true, false))
	if browseOnly.TerminalAvailable || !browseOnly.BrowseAvailable || browseOnly.BrowseWSAvailable {
		t.Fatalf("browse-only pairing granted unexpected capabilities: %#v", browseOnly)
	}

	browseWSOnly := pairWithScope(t, "browse-ws-only", BuildV2Scope(false, false, true))
	if browseWSOnly.TerminalAvailable || browseWSOnly.BrowseAvailable || !browseWSOnly.BrowseWSAvailable {
		t.Fatalf("browse-ws-only pairing granted unexpected capabilities: %#v", browseWSOnly)
	}
	if browseWSOnly.Scope != "cluster.summary.read cluster.browse.ws" {
		t.Fatalf("unexpected stored scope: %q", browseWSOnly.Scope)
	}
}

func TestServiceV2DeleteConvergesLocallyWhenRemoteRevokeFails(t *testing.T) {
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "target"),
		PanelVersion: "v0.27.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "target-v2",
		},
		Remote: targetRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "center"),
		PanelVersion: "v0.27.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "center-v2",
		},
		Remote: centerRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "retry-revoke", Origin: "http://8.8.8.8:1801",
		PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	record, err := center.storeV2.Host(host.ID)
	if err != nil {
		t.Fatalf("read v2 host before delete: %v", err)
	}

	route.setFailurePath(v2RevokePath)
	result, err := center.DeleteHost(context.Background(), host.ID, DeleteHostInput{
		ExpectedResourceVersion: host.ResourceVersion,
	})
	if err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if !result.Deleted || result.RemoteRevoked || !result.CredentialRemoved {
		t.Fatalf("unexpected local-only delete result: %#v", result)
	}
	if _, err := center.Host(context.Background(), host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("locally deleted host error = %v, want ErrNotFound", err)
	}
	if list := center.Hosts(context.Background()); list.RemoteTotal != 0 {
		t.Fatalf("locally deleted host remained in inventory: %#v", list)
	}
	if controllers := target.Controllers(); len(controllers) != 1 {
		t.Fatalf("unreachable target should retain one manually revocable controller: %#v", controllers)
	}
	if converged, err := center.finalizeLocalHostV2(record, false); err != nil ||
		!converged.Deleted {
		t.Fatalf("idempotent local finalize = %#v, %v", converged, err)
	}
}

func TestServiceV2ResumesPendingCommitAfterCenterRestart(t *testing.T) {
	now := time.Date(2026, 7, 29, 11, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "target"),
		PanelVersion: "v0.27.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "target-v2",
		},
		Remote: targetRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerDirectory := filepath.Join(t.TempDir(), "center")
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	centerConfig := ServiceConfig{
		DataDir:      centerDirectory,
		PanelVersion: "v0.27.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "center-v2",
		},
		Remote: centerRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	}
	center, err := NewService(centerConfig)
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	route.setFailurePath(v2CommitPath)
	pending, err := center.AddHost(context.Background(), AddHostInput{
		Name: "restart-resume", Origin: "http://8.8.8.8:1801",
		PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if pending.State != HostPairing {
		t.Fatalf("host state before restart = %q, want %q", pending.State, HostPairing)
	}
	if controllers := target.Controllers(); len(controllers) != 0 {
		t.Fatalf("provisional controller was exposed as active: %#v", controllers)
	}
	if err := center.Close(); err != nil {
		t.Fatalf("close center before restart: %v", err)
	}

	clock.Advance(v2PairingTTL + time.Minute)
	if err := target.checkpoint(); err != nil {
		t.Fatalf("target checkpoint after one-time code expiry: %v", err)
	}
	route.setFailurePath("")
	restarted, err := NewService(centerConfig)
	if err != nil {
		t.Fatalf("restart center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.pollV2(context.Background(), pending.ID)
	active, err := restarted.Host(context.Background(), pending.ID)
	if err != nil {
		t.Fatalf("resumed host unavailable: %v", err)
	}
	if active.State != HostOnline || active.LastSnapshot == nil {
		t.Fatalf("resumed host did not become online: %#v", active)
	}
	if controllers := target.Controllers(); len(controllers) != 1 {
		t.Fatalf("active controller missing after resumed commit: %#v", controllers)
	}
}

func TestServiceV2PairCodeCreationIsAtomicWithCheckpointCleanup(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	remote, _ := newServiceV2Remote(t)
	service, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "node"),
		PanelVersion: "v0.27.0",
		Hostname:     "atomic-node",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "atomic-node",
		},
		Remote: remote,
		Now:    clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	const codeCount = 8
	start := make(chan struct{})
	var wait sync.WaitGroup
	var resultMu sync.Mutex
	codes := make([]PairingCode, 0, codeCount)
	var concurrentErr error
	for range codeCount {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			code, err := service.CreatePairingCodeV2(SummaryTerminalScope)
			resultMu.Lock()
			defer resultMu.Unlock()
			concurrentErr = errors.Join(concurrentErr, err)
			if err == nil {
				codes = append(codes, code)
			}
		}()
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			resultMu.Lock()
			concurrentErr = errors.Join(concurrentErr, service.checkpoint())
			resultMu.Unlock()
		}()
	}
	close(start)
	wait.Wait()
	if concurrentErr != nil {
		t.Fatalf("concurrent create/checkpoint error = %v", concurrentErr)
	}
	if len(codes) != codeCount {
		t.Fatalf("created pairing codes = %d, want %d", len(codes), codeCount)
	}
	for _, code := range codes {
		descriptor, err := parseV2PairingCode(code.Code, now)
		if err != nil {
			t.Fatalf("parse pairing code: %v", err)
		}
		record, err := service.storeV2.PairingCode(descriptor.CodeID)
		if err != nil {
			t.Fatalf("pairing state missing for %s: %v", descriptor.CodeID, err)
		}
		if _, err := service.secretsV2.ReadCredential(record.CredentialFile); err != nil {
			t.Fatalf("pairing credential missing for %s: %v", descriptor.CodeID, err)
		}
	}
}

func TestServiceV2SlowSummaryDoesNotBlockPairingCodeCreation(t *testing.T) {
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "target"),
		PanelVersion: "v0.27.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "target-v2",
		},
		Remote: targetRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "center"),
		PanelVersion: "v0.27.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "center-v2",
		},
		Remote: centerRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("target CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "slow-summary", Origin: "http://8.8.8.8:1801",
		PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}

	reached := make(chan struct{})
	release := make(chan struct{})
	route.block(v2SummaryPath, reached, release)
	pollDone := make(chan struct{})
	go func() {
		center.pollV2(context.Background(), host.ID)
		close(pollDone)
	}()
	select {
	case <-reached:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("slow summary request was not reached")
	}

	created := make(chan error, 1)
	go func() {
		_, err := center.CreatePairingCodeV2(SummaryTerminalScope)
		created <- err
	}()
	select {
	case err := <-created:
		if err != nil {
			close(release)
			t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) during slow summary: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("slow summary blocked an unrelated pairing-code mutation")
	}
	close(release)
	select {
	case <-pollDone:
	case <-time.After(time.Second):
		t.Fatal("poll did not finish after slow summary was released")
	}
}

func TestServiceV2CredentialTransitionIsAtomicWithCleanup(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "target"),
		PanelVersion: "v0.27.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "target-v2",
		},
		Remote: targetRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir:      filepath.Join(t.TempDir(), "center"),
		PanelVersion: "v0.27.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{
			now: clock.Now, hostname: "center-v2",
		},
		Remote: centerRemote, Now: clock.Now,
		Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(SummaryTerminalScope) error = %v", err)
	}
	route.setFailurePath(v2PairPath)
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "atomic-transition", Origin: "http://8.8.8.8:1801",
		PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.State != HostPairing {
		t.Fatalf("host state = %q, want pairing", host.State)
	}
	route.setFailurePath("")

	center.v2SecretStateMu.Lock()
	lockHeld := true
	defer func() {
		if lockHeld {
			center.v2SecretStateMu.Unlock()
		}
	}()
	pollDone := make(chan struct{})
	go func() {
		center.pollV2(context.Background(), host.ID)
		close(pollDone)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		records := target.storeV2.PairingCodes()
		if len(records) == 1 && records[0].State == pairingStateV2Bound {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("remote pair did not reach the bound state")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := center.secretsV2.ReadCredential(
		"host-" + host.ID + ".v2key",
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("host credential was written outside the state transition lock: %v", err)
	}

	checkpointDone := make(chan error, 1)
	go func() {
		checkpointDone <- center.checkpoint()
	}()
	select {
	case err := <-checkpointDone:
		t.Fatalf("checkpoint crossed the guarded transition: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	center.v2SecretStateMu.Unlock()
	lockHeld = false
	select {
	case <-pollDone:
	case <-time.After(2 * time.Second):
		t.Fatal("poll did not finish after the transition lock was released")
	}
	select {
	case err := <-checkpointDone:
		if err != nil {
			t.Fatalf("checkpoint after transition: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("checkpoint did not finish after the transition lock was released")
	}

	active, err := center.Host(context.Background(), host.ID)
	if err != nil {
		t.Fatalf("Host() after transition: %v", err)
	}
	if active.State != HostOnline {
		t.Fatalf("host state after transition = %q, want online", active.State)
	}
	record, err := center.storeV2.Host(host.ID)
	if err != nil {
		t.Fatalf("store Host() after transition: %v", err)
	}
	if _, err := center.secretsV2.ReadCredential(record.CredentialFile); err != nil {
		t.Fatalf("referenced host credential was removed by cleanup: %v", err)
	}
}
