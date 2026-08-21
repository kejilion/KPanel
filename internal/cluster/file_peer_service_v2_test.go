package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type serviceV2FileFixtureTransport struct {
	target   *Service
	metadata contract.FileTransferMetadata
	content  []byte
}

type serviceV2StatusPathTransport struct {
	next   http.RoundTripper
	path   string
	status int

	mu    sync.Mutex
	calls int
}

func (t *serviceV2StatusPathTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == t.path {
		t.mu.Lock()
		t.calls++
		t.mu.Unlock()
		return serviceV2HTTPResponse(t.status, map[string]string{"error": "unsupported"}), nil
	}
	return t.next.RoundTrip(request)
}

func (t *serviceV2StatusPathTransport) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

func (t *serviceV2FileFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var envelope FederationEnvelopeV2
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return serviceV2HTTPResponse(http.StatusBadRequest, map[string]string{"error": "invalid"}), nil
	}
	var authorization *FederationFileAuthorization
	var err error
	switch request.URL.Path {
	case v2FileOpenPath:
		_, authorization, err = t.target.AuthorizeFederationFileV2("198.51.100.20", envelope)
	case v2FileLinkedOpenPath:
		_, authorization, err = t.target.AuthorizeLinkedFederationFileV2("198.51.100.20", envelope)
	default:
		return serviceV2HTTPResponse(http.StatusNotFound, map[string]string{"error": "missing"}), nil
	}
	if err != nil {
		return serviceV2HTTPResponse(http.StatusUnauthorized, map[string]string{"error": "rejected"}), nil
	}
	defer authorization.Close()
	sealed, cipher, err := authorization.SealMetadata(t.metadata)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	if err := WriteFederationFileHeader(&body, sealed); err != nil {
		return nil, err
	}
	writer := NewFederationFileWriter(&body, cipher)
	if _, err := writer.Write(t.content); err != nil {
		return nil, err
	}
	if err := writer.Finish(nil); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{fileStreamContentType}},
		Body:       io.NopCloser(bytes.NewReader(body.Bytes())),
		Request:    request,
	}, nil
}

func TestServiceV2SinglePairEnablesFilesInBothDirections(t *testing.T) {
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	input := FederationFileOpenRequest{Path: "/home/KPanel Desktop/app", ResourceVersion: "sha256:source-version"}
	metadata := contract.FileTransferMetadata{
		Name: "app", Kind: "directory", SizeBytes: 20,
		ResourceVersion: input.ResourceVersion,
	}
	content := []byte("bidirectional-content")
	metadata.SizeBytes = int64(len(content))
	centerRemote.streamClient = &http.Client{Transport: &serviceV2FileFixtureTransport{
		target: target, metadata: metadata, content: content,
	}}
	targetRemote.streamClient = &http.Client{Transport: &serviceV2FileFixtureTransport{
		target: center, metadata: metadata, content: content,
	}}

	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2() error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if !host.FileTransferAvailable || !host.MutualFileTransferAvailable {
		t.Fatalf("single pairing did not enable mutual files: %#v", host)
	}
	if target.Hosts(context.Background()).RemoteTotal != 0 {
		t.Fatal("reverse file route must not create a duplicate Host on the target")
	}

	for direction, source := range map[string]struct {
		service *Service
		nodeID  string
	}{
		"center-to-target": {service: center, nodeID: target.NodeID()},
		"target-to-center": {service: target, nodeID: center.NodeID()},
	} {
		reader, opened, err := source.service.OpenRemoteFileV2(context.Background(), source.nodeID, input)
		if err != nil {
			t.Fatalf("%s OpenRemoteFileV2() error = %v", direction, err)
		}
		got, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || !bytes.Equal(got, content) || opened != metadata {
			t.Fatalf("%s result metadata=%#v content=%q err=%v", direction, opened, got, readErr)
		}
	}

	result, err := center.DeleteHost(context.Background(), host.ID, DeleteHostInput{
		ExpectedResourceVersion: host.ResourceVersion,
	})
	if err != nil || !result.Deleted {
		t.Fatalf("DeleteHost() = %#v, %v", result, err)
	}
	if _, _, err := target.OpenRemoteFileV2(context.Background(), center.NodeID(), input); err == nil {
		t.Fatal("revoked reverse route remained usable")
	}
}

func TestServiceV2ExistingPairCanEnableMutualFilesWithoutRepairing(t *testing.T) {
	now := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	if host.MutualFileTransferAvailable {
		t.Fatal("a pre-existing one-way relationship was silently expanded")
	}
	controllersBefore := target.Controllers()
	host, err = center.EnableMutualFileTransfer(context.Background(), host.ID, "http://8.8.4.4:1802")
	if err != nil {
		t.Fatalf("EnableMutualFileTransfer() error = %v", err)
	}
	if !host.MutualFileTransferAvailable {
		t.Fatalf("explicit upgrade did not activate mutual files: %#v", host)
	}
	controllersAfter := target.Controllers()
	if len(controllersBefore) != 1 || len(controllersAfter) != 1 ||
		controllersBefore[0].ID != controllersAfter[0].ID {
		t.Fatalf("explicit upgrade repaired or duplicated the relationship: before=%#v after=%#v", controllersBefore, controllersAfter)
	}
}

func TestServiceV2TemporaryMutualLinkFailureRetriesWithoutBreakingPairing(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.75.0", Hostname: "old-target",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "old-target"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	route.setFailurePath(v2FileLinkPath)
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil {
		t.Fatalf("link failure rolled back primary pairing: %v", err)
	}
	if host.State != HostOnline || host.MutualFileTransferAvailable {
		t.Fatalf("mixed-version host state = %#v", host)
	}
	grant, err := center.filePeersV2.GrantByHost(host.ID)
	if err != nil || grant.State != filePeerGrantPending {
		t.Fatalf("temporary failure did not retain pending intent: %#v, %v", grant, err)
	}
	route.setFailurePath("")
	clock.Advance(filePeerSyncInterval)
	center.pollV2(context.Background(), host.ID)
	recovered, err := center.Host(context.Background(), host.ID)
	if err != nil || !recovered.MutualFileTransferAvailable {
		t.Fatalf("mutual link did not recover: %#v, %v", recovered, err)
	}
}

func TestServiceV2UnsupportedMutualEndpointStopsRetryingWithoutBreakingPairing(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 30, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.75.0", Hostname: "old-target",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "old-target"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	missing := &serviceV2StatusPathTransport{
		next: centerRemote.client.Transport, path: v2FileLinkPath, status: http.StatusNotFound,
	}
	centerRemote.client.Transport = missing
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil || host.State != HostOnline || host.MutualFileTransferAvailable {
		t.Fatalf("unsupported endpoint affected primary pairing: %#v, %v", host, err)
	}
	if _, err := center.filePeersV2.GrantByHost(host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unsupported pending intent was retained: %v", err)
	}
	if calls := missing.Calls(); calls != 1 {
		t.Fatalf("link calls = %d, want 1", calls)
	}
	clock.Advance(2 * filePeerSyncInterval)
	center.pollV2(context.Background(), host.ID)
	if calls := missing.Calls(); calls != 1 {
		t.Fatalf("unsupported endpoint was retried: calls=%d", calls)
	}
}

func TestServiceV2PendingPairRestartPreservesMutualFileIntent(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 45, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerDirectory := filepath.Join(t.TempDir(), "center")
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	route.setFailurePath(v2PairPath)
	centerConfig := ServiceConfig{
		DataDir: centerDirectory, PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	}
	center, err := NewService(centerConfig)
	if err != nil {
		t.Fatal(err)
	}
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil || host.State != HostPairing {
		t.Fatalf("initial pending host = %#v, %v", host, err)
	}
	grant, err := center.filePeersV2.GrantByHost(host.ID)
	if err != nil || grant.State != filePeerGrantPending {
		t.Fatalf("pending mutual intent = %#v, %v", grant, err)
	}
	if err := center.Close(); err != nil {
		t.Fatal(err)
	}

	route.setFailurePath("")
	restarted, err := NewService(centerConfig)
	if err != nil {
		t.Fatalf("restart center: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.pollV2(context.Background(), host.ID)
	recovered, err := restarted.Host(context.Background(), host.ID)
	if err != nil || !recovered.MutualFileTransferAvailable {
		t.Fatalf("resumed pairing lost mutual intent: %#v, %v", recovered, err)
	}
}

func TestServiceV2RefreshesMutualFileOriginAfterPublicURLChange(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 50, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerDirectory := filepath.Join(t.TempDir(), "center")
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	centerConfig := ServiceConfig{
		DataDir: centerDirectory, PublicURL: "https://8.8.4.4:443",
		PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	}
	center, err := NewService(centerConfig)
	if err != nil {
		t.Fatal(err)
	}
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil || !host.MutualFileTransferAvailable {
		t.Fatalf("initial mutual host = %#v, %v", host, err)
	}
	initialRoute, err := target.filePeersV2.ActiveRoute(center.NodeID(), clock.Now())
	if err != nil || initialRoute.PeerOrigin != "https://8.8.4.4" {
		t.Fatalf("canonical initial route = %#v, %v", initialRoute, err)
	}
	if err := center.Close(); err != nil {
		t.Fatal(err)
	}

	centerConfig.PublicURL = "http://1.1.1.1:1803"
	restarted, err := NewService(centerConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	clock.Advance(filePeerSyncInterval / 10)
	restarted.pollV2(context.Background(), host.ID)
	routeRecord, err := target.filePeersV2.ActiveRoute(restarted.NodeID(), clock.Now())
	if err != nil || routeRecord.PeerOrigin != centerConfig.PublicURL {
		t.Fatalf("refreshed route = %#v, %v", routeRecord, err)
	}
}

func TestServiceV2RenewsMutualFilesWhileSummaryFails(t *testing.T) {
	now := time.Date(2026, 8, 16, 10, 55, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = center.Close() })
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil || !host.MutualFileTransferAvailable {
		t.Fatalf("initial mutual host = %#v, %v", host, err)
	}
	initial, err := center.filePeersV2.ActiveGrantByHost(host.ID, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	route.setFailurePath(v2SummaryPath)
	for range 3 {
		clock.Advance(filePeerSyncInterval)
		center.pollV2(context.Background(), host.ID)
	}
	renewed, err := center.filePeersV2.ActiveGrantByHost(host.ID, clock.Now())
	if err != nil || !renewed.ExpiresAt.After(initial.ExpiresAt) {
		t.Fatalf("summary failure expired mutual grant: before=%#v after=%#v err=%v", initial, renewed, err)
	}
	if _, err := target.filePeersV2.ActiveRoute(center.NodeID(), clock.Now()); err != nil {
		t.Fatalf("summary failure expired remote route: %v", err)
	}
}

func TestServiceV2RestartsAndRenewsAnExpiredMutualFileLease(t *testing.T) {
	now := time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.77.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerDirectory := filepath.Join(t.TempDir(), "center")
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	centerConfig := ServiceConfig{
		DataDir: centerDirectory, PanelVersion: "v0.77.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	}
	center, err := NewService(centerConfig)
	if err != nil {
		t.Fatal(err)
	}
	code, err := target.CreatePairingCodeV2(SummaryTerminalFilesScope)
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
		ControllerOrigin: "http://8.8.4.4:1802",
	})
	if err != nil || !host.MutualFileTransferAvailable {
		t.Fatalf("initial mutual pair = %#v, %v", host, err)
	}
	if err := center.Close(); err != nil {
		t.Fatalf("close center: %v", err)
	}

	clock.Advance(filePeerLeaseDuration + time.Minute)
	restarted, err := NewService(centerConfig)
	if err != nil {
		t.Fatalf("restart center: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	pending, err := restarted.filePeersV2.GrantByHost(host.ID)
	if err != nil || pending.State != filePeerGrantPending {
		t.Fatalf("expired grant after restart = %#v, %v", pending, err)
	}
	clock.Advance(filePeerSyncInterval / 10)
	restarted.pollV2(context.Background(), host.ID)
	renewed, err := restarted.Host(context.Background(), host.ID)
	if err != nil || !renewed.MutualFileTransferAvailable {
		t.Fatalf("renewed host = %#v, %v", renewed, err)
	}
	if _, err := target.filePeersV2.ActiveRoute(restarted.NodeID(), clock.Now()); err != nil {
		t.Fatalf("target route was not renewed: %v", err)
	}
}
