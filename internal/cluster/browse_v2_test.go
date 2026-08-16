package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type serviceV2BrowseStub struct {
	mu      sync.Mutex
	lastURL string
	result  BrowseFetchResult
	err     error
}

func (s *serviceV2BrowseStub) Fetch(_ context.Context, input BrowseFetchRequest) (BrowseFetchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastURL = input.URL
	if s.err != nil {
		return BrowseFetchResult{}, s.err
	}
	return s.result, nil
}

// TestServiceV2BrowseFetchDrainsChunkedResponseOverAuthenticatedEncryptedChannel
// pairs with browse-only scope, opens a fetch whose body is large enough to
// force several Output polls (each capped at
// maxFederationBrowseFetchOutputBytes), and confirms the controller
// reassembles exactly the bytes the target's Agent produced — with neither
// the target URL nor the response body ever appearing in plaintext on the
// simulated HTTP wire.
func TestServiceV2BrowseFetchDrainsChunkedResponseOverAuthenticatedEncryptedChannel(t *testing.T) {
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	payload := bytes.Repeat([]byte("browse-secret-payload-"), 5000) // ~107KB: several 32KB chunks
	browseStub := &serviceV2BrowseStub{result: BrowseFetchResult{
		StatusCode: 201,
		Headers:    map[string][]string{"X-Upstream": {"1"}},
		Body:       payload,
	}}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.72.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"}, Browse: browseStub,
		Remote: targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.72.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(BuildV2Scope(false, true, false))
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(browse only) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "browse-target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if !host.BrowseAvailable || host.TerminalAvailable {
		t.Fatalf("unexpected paired capabilities: %#v", host)
	}

	opened, err := center.BrowseFetchOpen(context.Background(), host.ID, BrowseFetchRequest{
		URL: "https://example.com/secret-path", Method: "GET",
	})
	if err != nil {
		t.Fatalf("BrowseFetchOpen() error = %v", err)
	}
	if opened.StatusCode != 201 || opened.TotalBytes != len(payload) || opened.Headers["X-Upstream"][0] != "1" {
		t.Fatalf("unexpected open response: %#v", opened)
	}

	var assembled []byte
	offset := 0
	polls := 0
	for {
		polls++
		if polls > 20 {
			t.Fatal("too many Output polls, chunking likely stuck")
		}
		output, err := center.BrowseFetchOutput(context.Background(), host.ID, BrowseFetchOutputRequest{
			SessionID: opened.SessionID, Offset: offset,
		})
		if err != nil {
			t.Fatalf("BrowseFetchOutput() error = %v", err)
		}
		chunk, err := base64.StdEncoding.DecodeString(output.Data)
		if err != nil {
			t.Fatalf("decode output chunk: %v", err)
		}
		assembled = append(assembled, chunk...)
		offset = output.NextOffset
		if output.Done {
			break
		}
	}
	if polls < 2 {
		t.Fatalf("expected the ~107KB body to require multiple polls, got %d", polls)
	}
	if !bytes.Equal(assembled, payload) {
		t.Fatalf("reassembled body mismatch: got %d bytes, want %d", len(assembled), len(payload))
	}
	if browseStub.lastURL != "https://example.com/secret-path" {
		t.Fatalf("target Agent did not receive the requested URL: %q", browseStub.lastURL)
	}

	for _, body := range route.requestBodies() {
		if bytes.Contains(body, []byte("secret-path")) || bytes.Contains(body, []byte("browse-secret-payload")) {
			t.Fatalf("browse fetch plaintext leaked on HTTP wire: %s", body)
		}
	}

	// A fully-drained session must not still be readable. Errors that cross
	// the federation wire arrive as an opaque *RemoteError on the controller
	// side (same as every other v2 client call in this package — see
	// TestServiceV2BrowseFetchRequiresBrowseScope's comment for the one case
	// that stays typed because it never reaches the network), so this only
	// asserts failure, not ErrNotFound's identity.
	if _, err := center.BrowseFetchOutput(context.Background(), host.ID, BrowseFetchOutputRequest{
		SessionID: opened.SessionID, Offset: 0,
	}); err == nil {
		t.Fatal("expected Output on a fully-drained session to fail")
	}

	// Explicit Close on a session that was never fully drained must also
	// make it unreadable.
	secondOpen, err := center.BrowseFetchOpen(context.Background(), host.ID, BrowseFetchRequest{URL: "https://example.com/other"})
	if err != nil {
		t.Fatalf("second BrowseFetchOpen() error = %v", err)
	}
	if err := center.BrowseFetchClose(context.Background(), host.ID, BrowseFetchCloseRequest{SessionID: secondOpen.SessionID}); err != nil {
		t.Fatalf("BrowseFetchClose() error = %v", err)
	}
	if _, err := center.BrowseFetchOutput(context.Background(), host.ID, BrowseFetchOutputRequest{
		SessionID: secondOpen.SessionID, Offset: 0,
	}); err == nil {
		t.Fatal("expected Output on a closed session to fail")
	}
}

// TestServiceV2BrowseFetchRequiresBrowseScope confirms a controller paired
// with terminal-only scope is rejected locally (no federation round trip
// needed to know it will fail) rather than silently reusing terminal access
// for browse.
func TestServiceV2BrowseFetchRequiresBrowseScope(t *testing.T) {
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.72.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"}, Browse: &serviceV2BrowseStub{},
		Remote: targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.72.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(BuildV2Scope(true, false, false))
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(terminal only) error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "terminal-only-target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}

	requestsBefore := len(route.requestBodies())
	if _, err := center.BrowseFetchOpen(context.Background(), host.ID, BrowseFetchRequest{URL: "https://example.com/"}); !errors.Is(err, ErrBrowseUnavailable) {
		t.Fatalf("BrowseFetchOpen() on terminal-only host error = %v, want ErrBrowseUnavailable", err)
	}
	if len(route.requestBodies()) != requestsBefore {
		t.Fatal("scope rejection should be local; it must not reach the network")
	}
}

func TestBrowseFetchSessionV2CapacityAndIsolation(t *testing.T) {
	service := &Service{browseFetchSessions: make(map[string]browseFetchSessionV2)}
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)

	var lastID string
	for i := 0; i < maxBrowseFetchSessionsV2; i++ {
		id, err := service.openBrowseFetchSessionV2("alice", BrowseFetchResult{StatusCode: 200, Body: []byte("x")}, now)
		if err != nil {
			t.Fatalf("openBrowseFetchSessionV2() call %d error = %v", i, err)
		}
		lastID = id
	}
	if _, err := service.openBrowseFetchSessionV2("alice", BrowseFetchResult{StatusCode: 200, Body: []byte("x")}, now); err == nil {
		t.Fatal("expected the session cap to reject a session beyond maxBrowseFetchSessionsV2")
	}

	// Another controller must not be able to read alice's session.
	if _, err := service.browseFetchSessionOutputV2("bob", lastID, 0, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-controller Output error = %v, want ErrNotFound", err)
	}
	// An out-of-range offset is rejected without leaking session existence via a different error.
	if _, err := service.browseFetchSessionOutputV2("alice", lastID, 999, now); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("out-of-range offset error = %v, want ErrAuthentication", err)
	}

	service.closeBrowseFetchSessionV2("alice", lastID)
	if _, err := service.openBrowseFetchSessionV2("alice", BrowseFetchResult{StatusCode: 200, Body: []byte("x")}, now); err != nil {
		t.Fatalf("Close should free a capacity slot for a new session, got error = %v", err)
	}
}

func TestBrowseFetchSessionV2ExpiresAbandonedSessions(t *testing.T) {
	service := &Service{browseFetchSessions: make(map[string]browseFetchSessionV2)}
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	id, err := service.openBrowseFetchSessionV2("alice", BrowseFetchResult{StatusCode: 200, Body: []byte("x")}, now)
	if err != nil {
		t.Fatalf("openBrowseFetchSessionV2() error = %v", err)
	}
	later := now.Add(browseFetchSessionV2TTL + time.Second)
	// A later Open sweeps the expired session lazily (no background reaper).
	if _, err := service.openBrowseFetchSessionV2("alice", BrowseFetchResult{StatusCode: 200, Body: []byte("y")}, later); err != nil {
		t.Fatalf("openBrowseFetchSessionV2() at later time error = %v", err)
	}
	if _, err := service.browseFetchSessionOutputV2("alice", id, 0, later); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session Output error = %v, want ErrNotFound", err)
	}
}
