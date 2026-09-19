package monitoring

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCheckStoreSeedsDefaultsAndPersistsEmptyReplacement(t *testing.T) {
	root := t.TempDir()
	store, err := openCheckStore(root)
	if err != nil {
		t.Fatal(err)
	}
	initial := store.snapshot()
	if !initial.Available || len(initial.Items) != 9 || !ValidCheckResourceVersion(initial.ResourceVersion) {
		t.Fatalf("unexpected initial checks: %#v", initial)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(filepath.Join(root, "checks.json"))
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("checks mode=%v", info.Mode().Perm())
		}
	}
	cleared, err := store.replace(ReplaceChecksInput{
		ExpectedResourceVersion: initial.ResourceVersion,
		Items:                   []Check{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Items) != 0 || cleared.ResourceVersion == initial.ResourceVersion {
		t.Fatalf("checks were not cleared: %#v", cleared)
	}
	reopened, err := openCheckStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if restored := reopened.snapshot(); len(restored.Items) != 0 || restored.ResourceVersion != cleared.ResourceVersion {
		t.Fatalf("empty configuration did not survive restart: %#v", restored)
	}
}

func TestCheckStoreCorruptFileFailsClosedWithoutOverwritingIt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "checks.json")
	corrupt := []byte(`{"schemaVersion":1,"items":[`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := openCheckStore(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.snapshot()
	if snapshot.Available || snapshot.Warning != "monitoring_checks_unavailable" || len(snapshot.Items) != 0 {
		t.Fatalf("corrupt store did not fail closed: %#v", snapshot)
	}
	if _, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: snapshot.ResourceVersion, Items: DefaultChecks()}); !errors.Is(err, ErrChecksUnavailable) {
		t.Fatalf("corrupt store replacement error=%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(corrupt) {
		t.Fatalf("corrupt source was overwritten: %q", data)
	}
}

func TestCheckStoreValidatesTargetsAndConflicts(t *testing.T) {
	store, err := openCheckStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	version := store.snapshot().ResourceVersion
	valid := []Check{
		{ID: "ping-cloudflare", Kind: "ping", Name: "Cloudflare", Target: "1.1.1.1"},
		{ID: "tcp-web", Kind: "tcp", Name: "TLS", Target: "example.com:443"},
		{ID: "http-health", Kind: "http", Name: "Health", Target: "https://example.com/health"},
	}
	saved, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: version, Items: valid})
	if err != nil || len(saved.Items) != 3 {
		t.Fatalf("valid checks save=%#v err=%v", saved, err)
	}
	if _, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: version, Items: valid}); !errors.Is(err, ErrChecksConflict) {
		t.Fatalf("stale replacement error=%v", err)
	}
	invalid := append([]Check(nil), valid...)
	invalid[0].Target = "example.com"
	if _, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: saved.ResourceVersion, Items: invalid}); validationCheckField(err) != "items.target" {
		t.Fatalf("invalid Ping error=%v", err)
	}
	invalid = append([]Check(nil), valid...)
	invalid[1].Target = "example.com:70000"
	if _, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: saved.ResourceVersion, Items: invalid}); validationCheckField(err) != "items.target" {
		t.Fatalf("invalid TCP error=%v", err)
	}
	invalid = append([]Check(nil), valid...)
	invalid[2].Target = "https://user:secret@example.com/"
	if _, err := store.replace(ReplaceChecksInput{ExpectedResourceVersion: saved.ResourceVersion, Items: invalid}); validationCheckField(err) != "items.target" {
		t.Fatalf("credential URL error=%v", err)
	}
}

func TestGenericTCPAndHTTPChecksMeasureReachability(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
		close(accepted)
	}()
	if _, err := probeCheck(context.Background(), fakeCheckPing{}, Check{Kind: "tcp", Target: listener.Addr().String()}); err != nil {
		t.Fatalf("TCP check failed: %v", err)
	}
	<-accepted

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "KPanel-Monitor/1" {
			t.Errorf("unexpected user agent: %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if latency, err := probeCheck(context.Background(), fakeCheckPing{}, Check{Kind: "http", Target: server.URL}); err != nil || latency <= 0 {
		t.Fatalf("HTTP check latency=%s err=%v", latency, err)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer failing.Close()
	if _, err := probeCheck(context.Background(), fakeCheckPing{}, Check{Kind: "http", Target: failing.URL}); err == nil {
		t.Fatal("HTTP 502 was accepted as reachable")
	}
}

func TestLegacyNineRouteHistoryRemainsReadableWithoutCheckConfig(t *testing.T) {
	current := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	service, err := New(Config{StateDir: root, System: fakeSystemSource{summary: testSummary(1, 2)}, Now: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	value := diskRecord{
		Version: recordVersion, CollectedAt: current.Add(-time.Minute), Host: hostPoint(testSummary(1, 2), current),
		OperatorLatency: []diskOperatorLatencyPoint{{ID: "telecom-beijing", LatencyMilliseconds: 22.5, Reachable: true}},
	}
	if err := service.appendRecord(value); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "checks.json")); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Config{StateDir: root, System: fakeSystemSource{summary: testSummary(1, 2)}, Now: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	history, err := reopened.History(context.Background(), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.OperatorLatency) != 9 || len(history.OperatorLatency[0].Points) != 1 || history.OperatorLatency[0].Points[0].LatencyMilliseconds == nil {
		t.Fatalf("legacy route history was not retained: %#v", history.OperatorLatency)
	}
}

type fakeCheckPing struct{}

func (fakeCheckPing) Probe(context.Context, string) (time.Duration, error) {
	return time.Millisecond, nil
}

func validationCheckField(err error) string {
	var validation *CheckValidationError
	if errors.As(err, &validation) {
		return validation.Field
	}
	return ""
}
