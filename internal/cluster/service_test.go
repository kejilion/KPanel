package cluster

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type serviceTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *serviceTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *serviceTestClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type serviceTestTelemetry struct {
	now      func() time.Time
	hostname string
}

func (s serviceTestTelemetry) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return serviceTelemetry(s.now().UTC(), s.hostname), nil
}

func serviceTelemetry(now time.Time, hostname string) contract.HostTelemetry {
	return contract.HostTelemetry{
		AgentVersion: "0.27.0", AgentProtocolVersion: "v1alpha1",
		Hostname: hostname, OS: "Debian GNU/Linux 13", OSID: "debian",
		OSLike: []string{"debian"}, Kernel: "6.12.0", Architecture: "amd64",
		UptimeSeconds: 7200,
		Load:          contract.LoadSummary{One: 0.1, Five: 0.2, Fifteen: 0.3},
		CPU: contract.CPUSummary{
			Model: "test cpu", Cores: 4, FrequencyMHz: 2400, UsagePercent: 20,
		},
		Memory: contract.MemorySummary{
			TotalBytes: 8 << 30, AvailableBytes: 6 << 30,
			UsedBytes: 2 << 30, UsagePercent: 25,
		},
		Disk: contract.DiskCapacitySummary{
			TotalBytes: 80 << 30, UsedBytes: 20 << 30, UsagePercent: 25,
		},
		Network: contract.NetworkSummary{
			ReceivedBytes: 4096, SentBytes: 2048, TCPConnections: 12, UDPConnections: 3,
		},
		PublicNetwork: contract.PublicNetworkSummary{
			IPv4: "8.8.8.8", ISP: "Example ISP", Country: "Singapore", CountryCode: "SG",
		},
		CollectedAt: now,
	}
}

type serviceRouteRemote struct {
	mu     sync.RWMutex
	routes map[string]*Service
}

func (r *serviceRouteRemote) Add(origin string, service *Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[origin] = service
}

func (r *serviceRouteRemote) target(origin string) (*Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	target := r.routes[origin]
	if target == nil {
		return nil, &RemoteError{Code: "unreachable"}
	}
	return target, nil
}

func (r *serviceRouteRemote) Pair(
	_ context.Context,
	origin string,
	input PairRequest,
) (PairResponse, error) {
	target, err := r.target(origin)
	if err != nil {
		return PairResponse{}, err
	}
	return target.AcceptPair("198.51.100.10", input)
}

func (r *serviceRouteRemote) Summary(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
) (FederationSummary, error) {
	target, err := r.target(origin)
	if err != nil {
		return FederationSummary{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+summaryPath, nil)
	if err != nil {
		return FederationSummary{}, err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return FederationSummary{}, err
	}
	if err := SignRequest(request, controllerID, targetID, privateKey, now, nonce); err != nil {
		return FederationSummary{}, err
	}
	request.Header.Set(FederationCapabilitiesHeader, SecurityEntrancePathCapability)
	return target.SignedSummary(ctx, request)
}

func (r *serviceRouteRemote) Revoke(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
) error {
	target, err := r.target(origin)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, origin+revokePath, nil)
	if err != nil {
		return err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return err
	}
	if err := SignRequest(request, controllerID, targetID, privateKey, now, nonce); err != nil {
		return err
	}
	return target.SignedRevoke(request)
}

func newServiceForFederationTest(
	t *testing.T,
	remote remoteAPI,
	now func() time.Time,
	hostname string,
) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), PanelVersion: "0.27.0", Hostname: hostname,
		Telemetry: serviceTestTelemetry{now: now, hostname: hostname},
		Remote:    remote, Now: now, Jitter: func(duration time.Duration) time.Duration {
			return duration
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return service
}

func TestGeneratedClusterCredentialNameIsAccepted(t *testing.T) {
	name := strings.Repeat("a", 32) + ".ed25519"
	if !validCredentialName(name) {
		t.Fatalf("generated credential name %q was rejected", name)
	}
}

func TestServiceTwoNodePairSummaryReplayAndRevoke(t *testing.T) {
	requirePOSIXClusterCredentials(t)
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	remote := &serviceRouteRemote{routes: make(map[string]*Service)}
	controller := newServiceForFederationTest(t, remote, clock.Now, "controller")
	target := newServiceForFederationTest(t, remote, clock.Now, "target")
	target.securityEntrancePath = func() string { return "panel-secure1" }
	remote.Add("https://target.example", target)

	code, err := target.CreatePairingCode()
	if err != nil {
		t.Fatalf("CreatePairingCode() error = %v", err)
	}
	host, err := controller.AddHost(context.Background(), AddHostInput{
		Name: "production", Origin: "https://target.example", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.State != HostOnline || host.LastSnapshot == nil {
		record, recordErr := controller.store.Host(host.ID)
		_, credentialErr := controller.secrets.Read(record.CredentialFile)
		t.Fatalf(
			"paired host is not online: %#v (record error: %v, credential error: %v)",
			host, recordErr, credentialErr,
		)
	}
	if host.LastSnapshot.Telemetry.Hostname != "target" {
		t.Fatalf("unexpected remote telemetry: %#v", host.LastSnapshot.Telemetry)
	}
	if host.SecurityEntrancePath != "panel-secure1" {
		t.Fatalf("security entrance path was not synchronized: %#v", host)
	}
	inventory := controller.Hosts(context.Background())
	if inventory.Total != 2 || inventory.RemoteTotal != 1 ||
		len(inventory.Items) != 2 || !inventory.Items[0].IsLocal {
		t.Fatalf("unexpected inventory after pairing: %#v", inventory)
	}
	controllers := target.Controllers()
	if len(controllers) != 1 || controllers[0].Scope != SummaryScope {
		t.Fatalf("unexpected target controllers: %#v", controllers)
	}

	record, err := controller.store.Host(host.ID)
	if err != nil {
		t.Fatalf("Host() record error = %v", err)
	}
	privateKey, err := controller.secrets.Read(record.CredentialFile)
	if err != nil {
		t.Fatalf("Read() credential error = %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://target.example"+summaryPath, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if err := SignRequest(
		request, record.ControllerID, target.NodeID(), privateKey, clock.Now(),
		"0123456789abcdef0123456789abcdef",
	); err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}
	target.requestLimiter = newFixedWindowLimiter(1, time.Minute, 16)
	if !target.requestLimiter.Allow(record.ControllerID, clock.Now()) {
		t.Fatal("failed to seed controller rate limit")
	}
	target.replays.mu.Lock()
	replayEntriesBeforeLimit := len(target.replays.entries)
	target.replays.mu.Unlock()
	if _, err := target.SignedSummary(context.Background(), request); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate-limited SignedSummary() error = %v, want ErrRateLimited", err)
	}
	target.replays.mu.Lock()
	replayEntriesAfterLimit := len(target.replays.entries)
	target.replays.mu.Unlock()
	if replayEntriesAfterLimit != replayEntriesBeforeLimit {
		t.Fatalf(
			"rate-limited request consumed replay capacity: before=%d after=%d",
			replayEntriesBeforeLimit, replayEntriesAfterLimit,
		)
	}
	target.requestLimiter = newFixedWindowLimiter(30, time.Minute, 512)
	chunked := request.Clone(context.Background())
	chunked.ContentLength = -1
	chunked.TransferEncoding = []string{"chunked"}
	if _, err := target.SignedSummary(context.Background(), chunked); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("chunked SignedSummary() error = %v, want ErrAuthentication", err)
	}
	legacySummary, err := target.SignedSummary(context.Background(), request)
	if err != nil {
		t.Fatalf("SignedSummary() error = %v", err)
	}
	if legacySummary.SecurityEntrancePath != "" {
		t.Fatalf("legacy controller received an incompatible optional field: %#v", legacySummary)
	}
	if _, err := target.SignedSummary(context.Background(), request); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed SignedSummary() error = %v, want ErrReplay", err)
	}

	result, err := controller.DeleteHost(context.Background(), host.ID, DeleteHostInput{
		ExpectedResourceVersion: host.ResourceVersion,
	})
	if err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if !result.Deleted || !result.RemoteRevoked || !result.CredentialRemoved {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if len(target.Controllers()) != 0 {
		t.Fatalf("remote controller was not revoked: %#v", target.Controllers())
	}
	if controller.Hosts(context.Background()).RemoteTotal != 0 {
		t.Fatal("deleted host remains in controller inventory")
	}
}

func TestFederationSummarySecurityEntrancePathValidation(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	base := FederationSummary{
		NodeID:             "abcdefabcdefabcdefabcdefabcdefab",
		PanelVersion:       "v0.80.0",
		FederationProtocol: FederationProtocol,
		Telemetry:          serviceTelemetry(now, "target"),
	}
	for _, test := range []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "omitted"},
		{name: "valid", path: "panel-secure1"},
		{name: "leading slash", path: "/panel-secure1", wantErr: true},
		{name: "uppercase", path: "Panel-secure1", wantErr: true},
		{name: "too long", path: strings.Repeat("a", 49), wantErr: true},
		{name: "absolute URL", path: "https://attacker.example", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary := base
			summary.SecurityEntrancePath = test.path
			err := validateFederationSummary(summary, base.NodeID, now)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateFederationSummary() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestSecurityEntrancePathResponseRequiresDeclaredCapability(t *testing.T) {
	service := &Service{securityEntrancePath: func() string { return "panel-secure1" }}
	if got := service.responseSecurityEntrancePath(""); got != "" {
		t.Fatalf("path without capability = %q, want empty", got)
	}
	if got := service.responseSecurityEntrancePath("future-v1, " + SecurityEntrancePathCapability); got != "panel-secure1" {
		t.Fatalf("path with capability = %q", got)
	}
	service.securityEntrancePath = func() string { return "//attacker.example" }
	if got := service.responseSecurityEntrancePath(SecurityEntrancePathCapability); got != "" {
		t.Fatalf("invalid configured path = %q, want empty", got)
	}
	if hasFederationCapability(strings.Repeat("x", 257), SecurityEntrancePathCapability) {
		t.Fatal("oversized capability header was accepted")
	}
}

func TestDeleteHostCommitsStateBeforeCredentialCleanup(t *testing.T) {
	requirePOSIXClusterCredentials(t)
	now := time.Date(2026, 7, 29, 13, 30, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	remote := &serviceScriptedRemote{now: clock.Now}
	service := newServiceForFederationTest(t, remote, clock.Now, "controller")
	seedServiceRemoteHost(t, service, 0, now)

	records := service.store.Hosts()
	if len(records) != 1 {
		t.Fatalf("seeded records = %d, want 1", len(records))
	}
	record := records[0]
	originalRemove := service.secrets.remove
	service.secrets.remove = func(string) error {
		return errors.New("injected credential cleanup failure")
	}

	result, err := service.DeleteHost(context.Background(), record.ID, DeleteHostInput{
		ExpectedResourceVersion: record.ResourceVersion,
	})
	if err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if !result.Deleted || result.CredentialRemoved {
		t.Fatalf("unexpected delete result after cleanup failure: %#v", result)
	}
	if _, err := service.store.Host(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted host record error = %v, want ErrNotFound", err)
	}
	if _, err := service.secrets.Read(record.CredentialFile); err != nil {
		t.Fatalf("orphan credential should remain available for cleanup: %v", err)
	}

	service.secrets.remove = originalRemove
	if err := service.secrets.RemoveOrphans(map[string]struct{}{}); err != nil {
		t.Fatalf("RemoveOrphans() error = %v", err)
	}
	if _, err := service.secrets.Read(record.CredentialFile); err == nil {
		t.Fatal("orphan credential remained after cleanup retry")
	}
}

type serviceBlockingRevokeRemote struct {
	started chan struct{}
	release chan struct{}
}

func (r *serviceBlockingRevokeRemote) Pair(
	context.Context,
	string,
	PairRequest,
) (PairResponse, error) {
	return PairResponse{}, errors.New("unexpected pair")
}

func (r *serviceBlockingRevokeRemote) Summary(
	context.Context,
	string,
	string,
	string,
	ed25519.PrivateKey,
	time.Time,
) (FederationSummary, error) {
	return FederationSummary{}, errors.New("unexpected summary")
}

func (r *serviceBlockingRevokeRemote) Revoke(
	ctx context.Context,
	_ string,
	_ string,
	_ string,
	_ ed25519.PrivateKey,
	_ time.Time,
) error {
	select {
	case r.started <- struct{}{}:
	default:
	}
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestDeleteSerializesRefreshWithoutRecreatingRuntime(t *testing.T) {
	requirePOSIXClusterCredentials(t)
	now := time.Date(2026, 7, 29, 13, 45, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	remote := &serviceBlockingRevokeRemote{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	service := newServiceForFederationTest(t, remote, clock.Now, "controller")
	seedServiceRemoteHost(t, service, 0, now)
	record := service.store.Hosts()[0]

	deleted := make(chan error, 1)
	go func() {
		_, err := service.DeleteHost(context.Background(), record.ID, DeleteHostInput{
			ExpectedResourceVersion: record.ResourceVersion,
		})
		deleted <- err
	}()
	select {
	case <-remote.started:
	case <-time.After(time.Second):
		t.Fatal("DeleteHost() did not reach remote revoke")
	}

	refreshed := make(chan error, 1)
	go func() {
		_, err := service.Refresh(context.Background(), record.ID)
		refreshed <- err
	}()
	select {
	case err := <-refreshed:
		t.Fatalf("Refresh() bypassed the active delete mutation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	close(remote.release)
	if err := <-deleted; err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if err := <-refreshed; !errors.Is(err, ErrNotFound) {
		t.Fatalf("Refresh() after delete error = %v, want ErrNotFound", err)
	}
	service.mu.RLock()
	_, exists := service.runtime[record.ID]
	service.mu.RUnlock()
	if exists {
		t.Fatal("Refresh() recreated runtime state for a deleted host")
	}
}

func TestServiceRejectsPairingItselfAndRevokesProvisionalController(t *testing.T) {
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	remote := &serviceRouteRemote{routes: make(map[string]*Service)}
	service := newServiceForFederationTest(t, remote, clock.Now, "self")
	remote.Add("https://self.example", service)

	code, err := service.CreatePairingCode()
	if err != nil {
		t.Fatalf("CreatePairingCode() error = %v", err)
	}
	_, err = service.AddHost(context.Background(), AddHostInput{
		Origin: "https://self.example", PairingCode: code.Code,
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("AddHost(self) error = %v, want ErrDuplicate", err)
	}
	if len(service.store.Hosts()) != 0 {
		t.Fatal("self pairing persisted a remote host")
	}
	if len(service.Controllers()) != 0 {
		t.Fatalf("self pairing left a provisional controller: %#v", service.Controllers())
	}
}

type serviceScriptedRemote struct {
	mu         sync.Mutex
	nodeID     string
	hostname   string
	now        func() time.Time
	summaryErr error
	calls      int
}

func (r *serviceScriptedRemote) Pair(
	context.Context,
	string,
	PairRequest,
) (PairResponse, error) {
	return PairResponse{
		NodeID: r.nodeID, Hostname: r.hostname,
		PanelVersion: "0.27.0", FederationProtocol: FederationProtocol,
	}, nil
}

func (r *serviceScriptedRemote) Summary(
	context.Context,
	string,
	string,
	string,
	ed25519.PrivateKey,
	time.Time,
) (FederationSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.summaryErr != nil {
		return FederationSummary{}, r.summaryErr
	}
	return FederationSummary{
		NodeID: r.nodeID, PanelVersion: "0.27.0",
		FederationProtocol: FederationProtocol,
		Telemetry:          serviceTelemetry(r.now().UTC(), r.hostname),
	}, nil
}

func (r *serviceScriptedRemote) Revoke(
	context.Context,
	string,
	string,
	string,
	ed25519.PrivateKey,
	time.Time,
) error {
	return nil
}

func (r *serviceScriptedRemote) Fail(err error) {
	r.mu.Lock()
	r.summaryErr = err
	r.mu.Unlock()
}

func TestServicePollingFailureStateAndExponentialBackoff(t *testing.T) {
	requirePOSIXClusterCredentials(t)
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	nodeID := "abcdefabcdefabcdefabcdefabcdefab"
	remote := &serviceScriptedRemote{
		nodeID: nodeID, hostname: "remote", now: clock.Now,
	}
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), PanelVersion: "0.27.0", Hostname: "controller",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "controller"},
		Remote:    remote, Now: clock.Now, PollInterval: 30 * time.Second,
		Jitter: func(duration time.Duration) time.Duration { return duration },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	code := "0123456789abcdef." + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	host, err := service.AddHost(context.Background(), AddHostInput{
		Origin: "https://remote.example", PairingCode: code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.State != HostOnline {
		record, recordErr := service.store.Host(host.ID)
		_, credentialErr := service.secrets.Read(record.CredentialFile)
		t.Fatalf(
			"initial state = %q, want online (record error: %v, credential error: %v)",
			host.State, recordErr, credentialErr,
		)
	}

	remote.Fail(&RemoteError{Code: "unreachable"})
	delays := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for index, wantDelay := range delays {
		clock.Advance(wantDelay)
		service.mu.Lock()
		current := service.runtime[host.ID]
		current.inFlight = true
		service.runtime[host.ID] = current
		service.mu.Unlock()
		service.poll(context.Background(), host.ID)

		got, err := service.Host(context.Background(), host.ID)
		if err != nil {
			t.Fatalf("Host() after failure %d error = %v", index+1, err)
		}
		if got.ConsecutiveFailures != index+1 {
			t.Fatalf("failures = %d, want %d", got.ConsecutiveFailures, index+1)
		}
		if got.NextPollAt == nil || !got.NextPollAt.Equal(clock.Now().Add(wantDelay)) {
			t.Fatalf(
				"next poll after failure %d = %v, want %v",
				index+1, got.NextPollAt, clock.Now().Add(wantDelay),
			)
		}
		wantState := HostDegraded
		if index == 2 {
			wantState = HostOffline
		}
		if got.State != wantState {
			t.Fatalf("state after failure %d = %q, want %q", index+1, got.State, wantState)
		}
	}
}

type serviceBlockingRemote struct {
	mu            sync.Mutex
	active        int
	maxActive     int
	activeByHost  map[string]int
	maxByHost     map[string]int
	callsByHost   map[string]int
	canceledCalls int
	started       chan string
	release       chan struct{}
}

func newServiceBlockingRemote() *serviceBlockingRemote {
	return &serviceBlockingRemote{
		activeByHost: make(map[string]int),
		maxByHost:    make(map[string]int),
		callsByHost:  make(map[string]int),
		started:      make(chan string, MaxHosts),
		release:      make(chan struct{}),
	}
}

func (r *serviceBlockingRemote) Pair(
	context.Context,
	string,
	PairRequest,
) (PairResponse, error) {
	return PairResponse{}, errors.New("unexpected pair")
}

func (r *serviceBlockingRemote) Summary(
	ctx context.Context,
	origin string,
	_ string,
	targetID string,
	_ ed25519.PrivateKey,
	_ time.Time,
) (FederationSummary, error) {
	r.mu.Lock()
	r.active++
	r.activeByHost[origin]++
	r.callsByHost[origin]++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	if r.activeByHost[origin] > r.maxByHost[origin] {
		r.maxByHost[origin] = r.activeByHost[origin]
	}
	r.mu.Unlock()
	select {
	case r.started <- origin:
	default:
	}

	defer func() {
		r.mu.Lock()
		r.active--
		r.activeByHost[origin]--
		r.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		r.mu.Lock()
		r.canceledCalls++
		r.mu.Unlock()
		return FederationSummary{}, ctx.Err()
	case <-r.release:
		now := time.Now().UTC()
		return FederationSummary{
			NodeID: targetID, PanelVersion: "0.27.0",
			FederationProtocol: FederationProtocol,
			Telemetry:          serviceTelemetry(now, origin),
		}, nil
	}
}

func (r *serviceBlockingRemote) Revoke(
	context.Context,
	string,
	string,
	string,
	ed25519.PrivateKey,
	time.Time,
) error {
	return nil
}

func (r *serviceBlockingRemote) metrics() (int, map[string]int, map[string]int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxByHost := make(map[string]int, len(r.maxByHost))
	for key, value := range r.maxByHost {
		maxByHost[key] = value
	}
	callsByHost := make(map[string]int, len(r.callsByHost))
	for key, value := range r.callsByHost {
		callsByHost[key] = value
	}
	return r.maxActive, maxByHost, callsByHost, r.canceledCalls
}

func seedServiceRemoteHost(t *testing.T, service *Service, index int, due time.Time) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	_ = publicKey
	hostID, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex(host) error = %v", err)
	}
	nodeID, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex(node) error = %v", err)
	}
	controllerID, err := randomHex(16)
	if err != nil {
		t.Fatalf("randomHex(controller) error = %v", err)
	}
	credential, err := service.secrets.Write(hostID, privateKey)
	if err != nil {
		t.Fatalf("Write() credential error = %v", err)
	}
	now := time.Now().UTC()
	record := hostRecord{
		ID: hostID, Name: "host", Origin: "https://node" + string(rune('a'+index)) + ".example",
		RemoteNodeID: nodeID, ControllerID: controllerID, CredentialFile: credential,
		FederationProtocol: FederationProtocol, PanelVersion: "0.27.0",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := service.store.AddHost(record); err != nil {
		t.Fatalf("AddHost(record) error = %v", err)
	}
	if _, err := service.secrets.Read(credential); err != nil {
		t.Fatalf("Read() seeded credential error = %v", err)
	}
	service.mu.Lock()
	service.runtime[hostID] = runtimeState{nextPollAt: due}
	service.mu.Unlock()
}

func TestServiceSchedulerBoundsConcurrencyAvoidsOverlapAndCloseCancels(t *testing.T) {
	requirePOSIXClusterCredentials(t)
	remote := newServiceBlockingRemote()
	service, err := NewService(ServiceConfig{
		DataDir: t.TempDir(), PanelVersion: "0.27.0", Hostname: "controller",
		Telemetry: serviceTestTelemetry{now: time.Now, hostname: "controller"},
		Remote:    remote, PollInterval: 20 * time.Millisecond,
		SchedulerTick: 5 * time.Millisecond, CheckpointEvery: time.Hour,
		MaxConcurrency: 2,
		Jitter:         func(duration time.Duration) time.Duration { return duration },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	for index := 0; index < 5; index++ {
		seedServiceRemoteHost(t, service, index, time.Now().Add(-time.Second))
	}
	service.Start(context.Background())

	for index := 0; index < 2; index++ {
		select {
		case <-remote.started:
		case <-time.After(2 * time.Second):
			t.Fatal("scheduler did not start the bounded poll set")
		}
	}
	time.Sleep(40 * time.Millisecond)
	maxActive, maxByHost, callsByHost, _ := remote.metrics()
	if maxActive != 2 {
		t.Fatalf("maximum active polls = %d, want 2", maxActive)
	}
	for origin, maximum := range maxByHost {
		if maximum != 1 {
			t.Fatalf("host %s had %d overlapping polls", origin, maximum)
		}
	}
	for origin, calls := range callsByHost {
		if calls != 1 {
			t.Fatalf("host %s was polled %d times while its first poll was active", origin, calls)
		}
	}

	closed := make(chan error, 1)
	go func() { closed <- service.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not cancel in-flight and queued polls")
	}
	_, _, _, canceled := remote.metrics()
	if canceled != 2 {
		t.Fatalf("canceled remote calls = %d, want 2", canceled)
	}
}

func requirePOSIXClusterCredentials(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("cluster credential permission enforcement requires POSIX file modes")
	}
}
