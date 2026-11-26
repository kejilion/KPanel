package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

type fakeChecksCluster struct {
	hosts        []cluster.Host
	readiness    map[string]cluster.LightChecksReadiness
	current      map[string]monitoring.CheckSnapshot
	replaceErr   error
	attempts     int
	replacements []monitoring.ReplaceChecksInput
}

func (f *fakeChecksCluster) Hosts(context.Context) cluster.HostList {
	return cluster.HostList{Items: f.hosts}
}

func (f *fakeChecksCluster) LightChecksReadiness(id string) cluster.LightChecksReadiness {
	return f.readiness[id]
}

func (f *fakeChecksCluster) LightNodeChecks(_ context.Context, id string) (monitoring.CheckSnapshot, error) {
	return f.current[id], nil
}

func (f *fakeChecksCluster) ReplaceLightNodeChecks(_ context.Context, id string, input monitoring.ReplaceChecksInput) (monitoring.CheckSnapshot, error) {
	f.attempts++
	if f.replaceErr != nil {
		return monitoring.CheckSnapshot{}, f.replaceErr
	}
	f.replacements = append(f.replacements, input)
	result := monitoring.CheckSnapshot{SchemaVersion: 1, MaxItems: monitoring.MaxChecks, Available: true, Items: input.Items}
	result.ResourceVersion = monitoring.ChecksResourceVersion(input.Items)
	f.current[id] = result
	return result, nil
}

func checksSnapshot(t *testing.T, items []monitoring.Check) monitoring.CheckSnapshot {
	t.Helper()
	result := monitoring.CheckSnapshot{SchemaVersion: 1, MaxItems: monitoring.MaxChecks, Available: true, Items: items}
	result.ResourceVersion = monitoring.ChecksResourceVersion(items)
	if err := monitoring.ValidateCheckSnapshot(result); err != nil {
		t.Fatalf("fixture snapshot is invalid: %v", err)
	}
	return result
}

func lightHost(id, state string) cluster.Host {
	return cluster.Host{ID: id, Name: id, Kind: cluster.HostKindLightNode, Platform: "linux", State: cluster.HostState(state)}
}

func TestChecksDistributionPushesOnlyDivergedReachableNodes(t *testing.T) {
	desired := checksSnapshot(t, []monitoring.Check{{ID: "site", Kind: "http", Name: "官网", Target: "https://example.com/"}})
	other := checksSnapshot(t, monitoring.DefaultChecks())
	fake := &fakeChecksCluster{
		hosts: []cluster.Host{
			lightHost("stale", "online"), lightHost("same", "online"), lightHost("offline", "offline"),
			lightHost("old-node", "online"), {ID: "panel", Kind: cluster.HostKindPanel, State: cluster.HostOnline},
		},
		readiness: map[string]cluster.LightChecksReadiness{
			"stale": {Supported: true, Connected: true}, "same": {Supported: true, Connected: true},
			"offline": {Supported: true, Connected: false}, "old-node": {},
		},
		current: map[string]monitoring.CheckSnapshot{"stale": other, "same": desired, "offline": other},
	}
	d := openChecksDistribution(filepath.Join(t.TempDir(), "distribution.json"), fake, func(context.Context) (monitoring.CheckSnapshot, error) { return desired, nil })
	if err := d.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	d.Reconcile(context.Background())
	if len(fake.replacements) != 1 || fake.replacements[0].ExpectedResourceVersion != other.ResourceVersion {
		t.Fatalf("replacements = %#v", fake.replacements)
	}
	d.Reconcile(context.Background())
	if len(fake.replacements) != 1 {
		t.Fatal("a synced node was replaced again")
	}
	status := d.Status(context.Background())
	states := map[string]string{}
	for _, node := range status.Nodes {
		states[node.HostID] = node.State
	}
	want := map[string]string{"stale": "synced", "same": "synced", "offline": "waiting", "old-node": "unsupported"}
	for id, state := range want {
		if states[id] != state {
			t.Errorf("%s state = %q, want %q", id, states[id], state)
		}
	}
	if len(states) != len(want) {
		t.Fatalf("non-light hosts were listed: %#v", states)
	}
}

func TestChecksDistributionRestartsOnNewListAndGivesUpAfterThreeAttempts(t *testing.T) {
	desired := checksSnapshot(t, []monitoring.Check{{ID: "site", Kind: "http", Name: "官网", Target: "https://example.com/"}})
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := &fakeChecksCluster{
		hosts:      []cluster.Host{lightHost("edge", "online")},
		readiness:  map[string]cluster.LightChecksReadiness{"edge": {Supported: true, Connected: true}},
		current:    map[string]monitoring.CheckSnapshot{"edge": checksSnapshot(t, monitoring.DefaultChecks())},
		replaceErr: cluster.ErrLightChecksUnavailable,
	}
	d := openChecksDistribution(filepath.Join(t.TempDir(), "distribution.json"), fake, func(context.Context) (monitoring.CheckSnapshot, error) { return desired, nil })
	d.now = func() time.Time { return now }
	if err := d.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < checksDistributionAttempts; attempt++ {
		d.Reconcile(context.Background())
		now = now.Add(time.Hour)
	}
	if state := d.Status(context.Background()).Nodes[0]; state.State != "failed" || state.ErrorCode != "unavailable" {
		t.Fatalf("state after three failures = %#v", state)
	}
	if fake.attempts != checksDistributionAttempts {
		t.Fatalf("attempts = %d", fake.attempts)
	}
	attemptsBefore := fake.attempts
	d.Reconcile(context.Background())
	if fake.attempts != attemptsBefore {
		t.Fatal("a failed node kept retrying without a new list")
	}
	// Marking the node as reconnected resets the budget.
	fake.readiness["edge"] = cluster.LightChecksReadiness{}
	d.Reconcile(context.Background())
	fake.readiness["edge"] = cluster.LightChecksReadiness{Supported: true, Connected: true}
	d.Reconcile(context.Background())
	if fake.attempts != attemptsBefore+1 {
		t.Fatal("a reconnected node was not retried")
	}
	// A new center list starts every node over.
	fake.replaceErr = nil
	desired = checksSnapshot(t, []monitoring.Check{{ID: "db", Kind: "tcp", Name: "数据库", Target: "10.0.0.5:5432"}})
	d.Reconcile(context.Background())
	if state := d.Status(context.Background()).Nodes[0]; state.State != "synced" {
		t.Fatalf("state after a new list = %#v", state)
	}
}

func TestChecksDistributionDoesNothingWhenDisabled(t *testing.T) {
	fake := &fakeChecksCluster{
		hosts:     []cluster.Host{lightHost("edge", "online")},
		readiness: map[string]cluster.LightChecksReadiness{"edge": {Supported: true, Connected: true}},
	}
	d := openChecksDistribution(filepath.Join(t.TempDir(), "distribution.json"), fake, func(context.Context) (monitoring.CheckSnapshot, error) {
		t.Fatal("a disabled center read its own checks")
		return monitoring.CheckSnapshot{}, nil
	})
	d.Reconcile(context.Background())
	if len(fake.replacements) != 0 || len(d.Status(context.Background()).Nodes) != 0 {
		t.Fatal("a disabled center touched a node")
	}
}

func TestChecksDistributionSettingSurvivesRestartAndDefaultsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "distribution.json")
	desired := func(context.Context) (monitoring.CheckSnapshot, error) { return checksSnapshot(t, nil), nil }
	if openChecksDistribution(path, &fakeChecksCluster{}, desired).Enabled() {
		t.Fatal("distribution defaulted on")
	}
	if err := openChecksDistribution(path, &fakeChecksCluster{}, desired).SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !openChecksDistribution(path, &fakeChecksCluster{}, desired).Enabled() {
		t.Fatal("enabled setting was not persisted")
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"enabled":"yes"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if openChecksDistribution(path, &fakeChecksCluster{}, desired).Enabled() {
		t.Fatal("an invalid setting turned distribution on")
	}
}

func TestChecksDistributionEndpointRequiresOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.checksDistribution = openChecksDistribution(filepath.Join(t.TempDir(), "distribution.json"), &fakeChecksCluster{}, func(context.Context) (monitoring.CheckSnapshot, error) {
		return checksSnapshot(t, nil), nil
	})
	body := []byte(`{"enabled":true}`)
	if response := authenticatedRequest(server, http.MethodPut, checksDistributionPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value,
	}); response.Code != http.StatusForbidden {
		t.Fatalf("missing Origin status=%d", response.Code)
	}
	if response := authenticatedRequest(server, http.MethodPut, checksDistributionPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	}); response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", response.Code)
	}
	response := authenticatedRequest(server, http.MethodPut, checksDistributionPath, body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if response.Code != http.StatusOK || !server.checksDistribution.Enabled() {
		t.Fatalf("valid PUT status=%d body=%s", response.Code, response.Body.String())
	}
	var status ChecksDistributionStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || !status.Enabled {
		t.Fatalf("status = %s", response.Body.String())
	}
	read := authenticatedRequest(server, http.MethodGet, checksDistributionPath, nil, sessionCookie, csrfCookie, nil)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"enabled":true`) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
}
