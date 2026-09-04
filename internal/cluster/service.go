package cluster

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	defaultPollInterval = 30 * time.Second
	maxPollBackoff      = 5 * time.Minute
	staleAfter          = 90 * time.Second
	maxSafeJSONInteger  = uint64(1<<53 - 1)
)

type TelemetrySource interface {
	Telemetry(context.Context) (contract.HostTelemetry, error)
}

type remoteAPI interface {
	Pair(context.Context, string, PairRequest) (PairResponse, error)
	Summary(context.Context, string, string, string, ed25519.PrivateKey, time.Time) (FederationSummary, error)
	Revoke(context.Context, string, string, string, ed25519.PrivateKey, time.Time) error
}

type ServiceConfig struct {
	DataDir         string
	PanelVersion    string
	PublicURL       string
	PrivateCIDRs    []string
	Telemetry       TelemetrySource
	Terminal        TerminalBackend
	Remote          remoteAPI
	Now             func() time.Time
	Hostname        string
	PollInterval    time.Duration
	SchedulerTick   time.Duration
	CheckpointEvery time.Duration
	MaxConcurrency  int
	Jitter          func(time.Duration) time.Duration

	SecurityEntrancePath func() string
}

type TerminalBackend interface {
	Open(context.Context, string, uint16, uint16) (terminal.Snapshot, error)
	Output(context.Context, string, string, int64, time.Duration) (terminal.Output, error)
	Input(context.Context, string, string, []byte) error
	Resize(context.Context, string, string, uint16, uint16) error
	Close(context.Context, string, string) error
}

type runtimeState struct {
	snapshot            *HostSnapshot
	lastAttemptAt       *time.Time
	lastSuccessAt       *time.Time
	consecutiveFailures int
	lastErrorCode       string
	lastError           string
	panelVersion        string
	nextPollAt          time.Time
	inFlight            bool
	nextFilePeerSyncAt  time.Time

	securityEntrancePath string
}

type Service struct {
	store           *Store
	secrets         *secretStore
	storeV2         *storeV2
	filePeersV2     *filePeerStoreV2
	secretsV2       *secretStoreV2
	remote          remoteAPI
	remoteV2        remoteV2API
	telemetry       TelemetrySource
	terminal        TerminalBackend
	lightTerminal   *lightTerminalRelay
	lightFile       *lightFileRelay
	panelFileRelay  *panelFileRelay
	nodeIdentityV2  nodeIdentityV2
	panelVersion    string
	publicURL       string
	light           *lightStore
	hostname        string
	now             func() time.Time
	pollInterval    time.Duration
	schedulerTick   time.Duration
	checkpointEvery time.Duration
	jitter          func(time.Duration) time.Duration
	sem             chan struct{}

	securityEntrancePath func() string

	mutationMu      sync.Mutex
	v2SecretStateMu sync.Mutex
	mu              sync.RWMutex
	runtime         map[string]runtimeState
	localHost       runtimeState
	started         bool
	ctx             context.Context
	cancel          context.CancelFunc
	kick            chan struct{}
	wg              sync.WaitGroup

	replays               *replayGuard
	pairLimiter           *fixedWindowLimiter
	v2SourceLimiter       *fixedWindowLimiter
	requestLimiter        *fixedWindowLimiter
	fileSources           *fixedWindowLimiter
	fileRequests          *fixedWindowLimiter
	panelFileRequests     *fixedWindowLimiter
	fileStreams           *fileStreamLimiter
	terminalSources       *fixedWindowLimiter
	terminalRequests      *fixedWindowLimiter
	lightEnrolls          *fixedWindowLimiter
	lightSources          *fixedWindowLimiter
	lightReports          *fixedWindowLimiter
	lightTerminalRequests *fixedWindowLimiter
	lightFileSources      *fixedWindowLimiter
	lightFileRequests     *fixedWindowLimiter

	localMu       sync.Mutex
	localValue    contract.HostTelemetry
	localExpires  time.Time
	localInFlight chan struct{}
	localError    error
}

func NewService(config ServiceConfig) (*Service, error) {
	if strings.TrimSpace(config.DataDir) == "" || !filepath.IsAbs(config.DataDir) {
		return nil, errors.New("cluster data directory must be absolute")
	}
	if config.Telemetry == nil {
		return nil, errors.New("cluster telemetry source is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.PollInterval == 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.PollInterval < 20*time.Millisecond || config.PollInterval > 10*time.Minute {
		return nil, errors.New("cluster poll interval is invalid")
	}
	if config.SchedulerTick == 0 {
		config.SchedulerTick = time.Second
	}
	if config.SchedulerTick < 5*time.Millisecond || config.SchedulerTick > time.Minute {
		return nil, errors.New("cluster scheduler tick is invalid")
	}
	if config.CheckpointEvery == 0 {
		config.CheckpointEvery = 5 * time.Minute
	}
	if config.CheckpointEvery < 50*time.Millisecond || config.CheckpointEvery > time.Hour {
		return nil, errors.New("cluster checkpoint interval is invalid")
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = 8
	}
	if config.MaxConcurrency < 1 || config.MaxConcurrency > 16 {
		return nil, errors.New("cluster concurrency must be between 1 and 16")
	}
	if config.Jitter == nil {
		config.Jitter = jitterDuration
	}
	if config.Remote == nil {
		remote, err := NewRemoteClient(RemoteClientConfig{PrivateCIDRs: config.PrivateCIDRs})
		if err != nil {
			return nil, err
		}
		config.Remote = remote
	}
	if config.Hostname == "" {
		config.Hostname, _ = os.Hostname()
	}
	config.Hostname = cleanDisplayText(config.Hostname, 253)
	if config.Hostname == "" {
		config.Hostname = "KPanel"
	}
	store, err := OpenStore(filepath.Join(config.DataDir, "cluster-state.json"))
	if err != nil {
		return nil, err
	}
	secrets, err := openSecretStore(filepath.Join(config.DataDir, "cluster-secrets"))
	if err != nil {
		return nil, err
	}
	storeV2, err := openStoreV2(filepath.Join(config.DataDir, clusterStateV2FileName))
	if err != nil {
		return nil, err
	}
	filePeersV2, err := openFilePeerStoreV2(filepath.Join(config.DataDir, filePeerStateV2FileName))
	if err != nil {
		return nil, err
	}
	if err := filePeersV2.Reconcile(storeV2.Controllers(), storeV2.Hosts(), config.Now().UTC()); err != nil {
		return nil, err
	}
	light, err := openLightStore(filepath.Join(config.DataDir, lightStateFileName))
	if err != nil {
		return nil, err
	}
	if err := storeV2.EnsureNodeID(store.NodeID()); err != nil {
		return nil, err
	}
	secretsV2, err := openSecretStoreV2(
		filepath.Join(config.DataDir, clusterSecretsV2DirectoryName),
	)
	if err != nil {
		return nil, err
	}
	nodeIdentity, err := secretsV2.ReadNodeIdentity()
	if errors.Is(err, os.ErrNotExist) {
		key, keyErr := v2NoiseSuite.GenerateKeypair(rand.Reader)
		if keyErr != nil {
			return nil, fmt.Errorf("generate cluster v2 node identity: %w", keyErr)
		}
		nodeIdentity = nodeIdentityV2{
			PrivateKey: append([]byte(nil), key.Private...),
			PublicKey:  append([]byte(nil), key.Public...),
		}
		if err := secretsV2.WriteNodeIdentity(nodeIdentity); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	referencedCredentials := make(map[string]struct{}, len(store.Hosts()))
	for _, record := range store.Hosts() {
		referencedCredentials[record.CredentialFile] = struct{}{}
	}
	if err := secrets.RemoveOrphans(referencedCredentials); err != nil {
		return nil, err
	}
	if err := secretsV2.RemoveOrphans(storeV2.CredentialReferences()); err != nil {
		return nil, err
	}
	remoteV2, _ := config.Remote.(remoteV2API)
	now := config.Now().UTC()
	service := &Service{
		store: store, secrets: secrets,
		storeV2: storeV2, filePeersV2: filePeersV2, secretsV2: secretsV2,
		remote: config.Remote, remoteV2: remoteV2, telemetry: config.Telemetry, terminal: config.Terminal,
		light: light, lightTerminal: newLightTerminalRelay(config.Now), lightFile: newLightFileRelay(config.Now),
		panelFileRelay: newPanelFileRelay(),
		publicURL:      strings.TrimRight(strings.TrimSpace(config.PublicURL), "/"),
		nodeIdentityV2: cloneNodeIdentityV2(nodeIdentity),
		panelVersion:   cleanDisplayText(config.PanelVersion, 64), hostname: config.Hostname,
		now: config.Now, pollInterval: config.PollInterval,
		schedulerTick: config.SchedulerTick, checkpointEvery: config.CheckpointEvery,
		jitter: config.Jitter, sem: make(chan struct{}, config.MaxConcurrency),
		runtime: make(map[string]runtimeState), kick: make(chan struct{}, 1),
		replays:               newReplayGuard(8192, 5*time.Minute),
		pairLimiter:           newFixedWindowLimiter(20, time.Minute, 1024),
		v2SourceLimiter:       newFixedWindowLimiter(120, time.Minute, 2048),
		requestLimiter:        newFixedWindowLimiter(30, time.Minute, 512),
		fileSources:           newFixedWindowLimiter(1200, time.Minute, 2048),
		fileRequests:          newFixedWindowLimiter(256, time.Minute, 512),
		panelFileRequests:     newFixedWindowLimiter(1200, time.Minute, 512),
		fileStreams:           newFileStreamLimiter(8, 2),
		terminalSources:       newFixedWindowLimiter(1200, time.Minute, 2048),
		terminalRequests:      newFixedWindowLimiter(600, time.Minute, 512),
		lightEnrolls:          newFixedWindowLimiter(10, time.Minute, 2048),
		lightSources:          newFixedWindowLimiter(240, time.Minute, 2048),
		lightReports:          newFixedWindowLimiter(180, time.Minute, MaxHosts),
		lightTerminalRequests: newFixedWindowLimiter(300, time.Minute, MaxHosts),
		lightFileSources:      newFixedWindowLimiter(1200, time.Minute, MaxHosts),
		lightFileRequests:     newFixedWindowLimiter(600, time.Minute, MaxHosts),

		securityEntrancePath: config.SecurityEntrancePath,
	}
	for _, record := range store.Hosts() {
		service.runtime[record.ID] = runtimeState{
			snapshot:            cloneSnapshot(record.LastSnapshot),
			lastAttemptAt:       cloneTime(record.LastAttemptAt),
			lastSuccessAt:       cloneTime(record.LastSuccessAt),
			consecutiveFailures: record.ConsecutiveFailures,
			lastErrorCode:       record.LastErrorCode,
			lastError:           record.LastError,
			panelVersion:        record.PanelVersion,
			nextPollAt:          now.Add(config.Jitter(config.PollInterval / 10)),
		}
	}
	for _, record := range storeV2.Hosts() {
		nextFilePeerSyncAt := time.Time{}
		if grant, peerErr := filePeersV2.GrantByHost(record.ID); peerErr == nil &&
			grant.State == filePeerGrantActive {
			nextFilePeerSyncAt = now.Add(config.Jitter(filePeerSyncInterval / 10))
		}
		service.runtime[record.ID] = runtimeState{
			snapshot:            cloneSnapshot(record.LastSnapshot),
			lastAttemptAt:       cloneTime(record.LastAttemptAt),
			lastSuccessAt:       cloneTime(record.LastSuccessAt),
			consecutiveFailures: record.ConsecutiveFailures,
			lastErrorCode:       record.LastErrorCode,
			lastError:           record.LastError,
			panelVersion:        record.PanelVersion,
			nextPollAt:          now.Add(config.Jitter(config.PollInterval / 10)),
			nextFilePeerSyncAt:  nextFilePeerSyncAt,
		}
	}
	return service, nil
}

func (s *Service) Start(parent context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(parent)
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.run()
}

func (s *Service) Close() error {
	s.mu.Lock()
	cancel := s.cancel
	started := s.started
	s.started = false
	s.cancel = nil
	s.mu.Unlock()
	if started && cancel != nil {
		cancel()
		s.wg.Wait()
	}
	if s.lightTerminal != nil {
		s.lightTerminal.closeAll()
	}
	if s.lightFile != nil {
		s.lightFile.closeAll()
	}
	if s.panelFileRelay != nil {
		s.panelFileRelay.closeAll()
	}
	return s.checkpoint()
}

// SetFileRelayHandler supplies the already-authenticated Panel Agent file
// surface to the cluster relay. It is called once during Panel construction;
// the cluster service keeps the relay protocol and the Panel keeps Agent
// authorization/route details in their respective packages.
func (s *Service) SetFileRelayHandler(handler http.Handler) {
	if s == nil || s.panelFileRelay == nil {
		return
	}
	s.panelFileRelay.setHandler(handler)
}

func (s *Service) NodeID() string {
	return s.store.NodeID()
}

func (s *Service) Hosts(ctx context.Context) HostList {
	records := s.store.Hosts()
	recordsV2 := s.storeV2.Hosts()
	lightRecords := s.light.Hosts()
	items := make([]Host, 0, len(records)+len(recordsV2)+len(lightRecords)+1)
	items = append(items, s.localHostSummary(ctx))
	now := s.now().UTC()
	s.mu.RLock()
	for _, record := range records {
		items = append(items, publicHost(record, s.runtime[record.ID], now))
	}
	for _, record := range recordsV2 {
		host := publicHostV2(record, s.runtime[record.ID], now)
		host.MutualFileTransferAvailable = s.hasActiveFilePeerGrant(record.ID, now)
		items = append(items, host)
	}
	for _, record := range lightRecords {
		items = append(items, s.publicLightHost(record, now))
	}
	s.mu.RUnlock()
	return HostList{
		Items: items, Total: len(items),
		RemoteTotal: len(records) + len(recordsV2) + len(lightRecords), MaxHosts: MaxHosts,
		PollIntervalSeconds: max(1, int(s.pollInterval/time.Second)),
		NodeID:              s.store.NodeID(),
	}
}

func (s *Service) Host(ctx context.Context, id string) (Host, error) {
	if id == LocalHostID {
		return s.localHostSummary(ctx), nil
	}
	record, err := s.store.Host(id)
	if err == nil {
		s.mu.RLock()
		current := s.runtime[id]
		s.mu.RUnlock()
		return publicHost(record, current, s.now().UTC()), nil
	}
	recordV2, v2Err := s.storeV2.Host(id)
	if v2Err == nil {
		s.mu.RLock()
		current := s.runtime[id]
		s.mu.RUnlock()
		now := s.now().UTC()
		host := publicHostV2(recordV2, current, now)
		host.MutualFileTransferAvailable = s.hasActiveFilePeerGrant(recordV2.ID, now)
		return host, nil
	}
	lightRecord, lightErr := s.light.Host(id)
	if lightErr != nil {
		if !errors.Is(err, ErrNotFound) {
			return Host{}, err
		}
		if !errors.Is(v2Err, ErrNotFound) {
			return Host{}, v2Err
		}
		return Host{}, lightErr
	}
	return s.publicLightHost(lightRecord, s.now().UTC()), nil
}

func (s *Service) AddHost(ctx context.Context, input AddHostInput) (Host, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	if strings.HasPrefix(strings.TrimSpace(input.PairingCode), v2PairingCodePrefix) {
		return s.addHostV2Locked(ctx, input)
	}
	origin, err := NormalizeOrigin(input.Origin)
	if err != nil {
		return Host{}, err
	}
	name, err := validateOptionalName(input.Name)
	if err != nil {
		return Host{}, err
	}
	code := strings.TrimSpace(input.PairingCode)
	if len(code) != 81 {
		return Host{}, ErrPairingCode
	}
	existing := s.store.Hosts()
	existingV2 := s.storeV2.Hosts()
	if len(existing)+len(existingV2)+len(s.light.Hosts()) >= MaxHosts {
		return Host{}, ErrHostLimit
	}
	for _, item := range existing {
		if strings.EqualFold(item.Origin, origin) {
			return Host{}, ErrDuplicate
		}
	}
	for _, item := range existingV2 {
		if strings.EqualFold(item.Origin, origin) {
			return Host{}, ErrDuplicate
		}
	}
	hostID, err := randomHex(16)
	if err != nil {
		return Host{}, err
	}
	controllerID, err := randomHex(16)
	if err != nil {
		return Host{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Host{}, fmt.Errorf("generate federation credential: %w", err)
	}
	response, err := s.remote.Pair(ctx, origin, PairRequest{
		PairingCode: code, ControllerID: controllerID, ControllerName: s.hostname,
		PublicKey: encodePublicKey(publicKey), FederationProtocol: FederationProtocol,
	})
	if err != nil {
		return Host{}, err
	}
	if err := validatePairResponse(response); err != nil {
		return Host{}, err
	}
	if response.NodeID == s.store.NodeID() {
		_ = s.remote.Revoke(ctx, origin, controllerID, response.NodeID, privateKey, s.now().UTC())
		return Host{}, ErrDuplicate
	}
	for _, item := range existing {
		if item.RemoteNodeID == response.NodeID {
			_ = s.remote.Revoke(ctx, origin, controllerID, response.NodeID, privateKey, s.now().UTC())
			return Host{}, ErrDuplicate
		}
	}
	for _, item := range existingV2 {
		if item.RemoteNodeID == response.NodeID {
			_ = s.remote.Revoke(ctx, origin, controllerID, response.NodeID, privateKey, s.now().UTC())
			return Host{}, ErrDuplicate
		}
	}
	if name == "" {
		name = cleanDisplayText(response.Hostname, 80)
	}
	if name == "" {
		name = strings.TrimPrefix(origin, "https://")
	}
	credentialFile, err := s.secrets.Write(hostID, privateKey)
	if err != nil {
		_ = s.remote.Revoke(ctx, origin, controllerID, response.NodeID, privateKey, s.now().UTC())
		return Host{}, err
	}
	now := s.now().UTC()
	record := hostRecord{
		ID: hostID, Name: name, Origin: origin, RemoteNodeID: response.NodeID,
		ControllerID: controllerID, CredentialFile: credentialFile,
		FederationProtocol: response.FederationProtocol, PanelVersion: response.PanelVersion,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.AddHost(record); err != nil {
		_ = s.secrets.Delete(credentialFile)
		_ = s.remote.Revoke(ctx, origin, controllerID, response.NodeID, privateKey, s.now().UTC())
		return Host{}, err
	}
	s.mu.Lock()
	s.runtime[hostID] = runtimeState{
		panelVersion: response.PanelVersion, nextPollAt: now, inFlight: true,
	}
	s.mu.Unlock()
	s.poll(ctx, hostID)
	return s.Host(ctx, hostID)
}

func (s *Service) RenameHost(id string, input UpdateHostInput) (Host, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	name, err := validateRequiredName(input.Name)
	if err != nil {
		return Host{}, err
	}
	if !validResourceVersion(input.ExpectedResourceVersion) {
		return Host{}, ErrConflict
	}
	if id == LocalHostID {
		s.mu.RLock()
		current := s.localHost
		s.mu.RUnlock()
		now := s.now().UTC()
		local := localPublicHost(
			s.store.NodeID(), s.hostname, s.store.LocalName(),
			s.panelVersion, current, now,
		)
		if local.ResourceVersion != input.ExpectedResourceVersion {
			return Host{}, ErrConflict
		}
		if err := s.store.SetLocalName(name); err != nil {
			return Host{}, err
		}
		return localPublicHost(
			s.store.NodeID(), s.hostname, name,
			s.panelVersion, current, now,
		), nil
	}
	record, err := s.store.RenameHost(
		id, name, input.ExpectedResourceVersion, s.now().UTC(),
	)
	if err == nil {
		s.mu.RLock()
		current := s.runtime[id]
		s.mu.RUnlock()
		return publicHost(record, current, s.now().UTC()), nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Host{}, err
	}
	recordV2, err := s.storeV2.Host(id)
	if err != nil {
		lightRecord, lightErr := s.light.RenameHost(id, name, input.ExpectedResourceVersion, s.now().UTC())
		if lightErr != nil {
			if !errors.Is(err, ErrNotFound) {
				return Host{}, err
			}
			return Host{}, lightErr
		}
		return s.publicLightHost(lightRecord, s.now().UTC()), nil
	}
	recordV2.Name = name
	recordV2.UpdatedAt = s.now().UTC()
	recordV2, err = s.storeV2.UpdateHost(
		recordV2, input.ExpectedResourceVersion,
	)
	if err != nil {
		return Host{}, err
	}
	s.mu.RLock()
	current := s.runtime[id]
	s.mu.RUnlock()
	now := s.now().UTC()
	host := publicHostV2(recordV2, current, now)
	host.MutualFileTransferAvailable = s.hasActiveFilePeerGrant(recordV2.ID, now)
	return host, nil
}

func (s *Service) DeleteHost(
	ctx context.Context,
	id string,
	input DeleteHostInput,
) (DeleteHostResult, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if id == LocalHostID {
		return DeleteHostResult{}, ErrLocalHost
	}
	if !validResourceVersion(input.ExpectedResourceVersion) {
		return DeleteHostResult{}, ErrConflict
	}
	record, err := s.store.Host(id)
	if errors.Is(err, ErrNotFound) {
		if _, v2Err := s.storeV2.Host(id); v2Err == nil {
			return s.deleteHostV2Locked(ctx, id, input)
		}
		return s.deleteLightHostLocked(id, input)
	}
	if err != nil {
		return DeleteHostResult{}, err
	}
	if record.ResourceVersion != input.ExpectedResourceVersion {
		return DeleteHostResult{}, ErrConflict
	}
	key, keyErr := s.secrets.Read(record.CredentialFile)
	remoteRevoked := false
	if keyErr == nil {
		revokeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		remoteRevoked = s.remote.Revoke(
			revokeCtx, record.Origin, record.ControllerID,
			record.RemoteNodeID, key, s.now().UTC(),
		) == nil
		cancel()
	}
	if _, err := s.store.DeleteHost(id, input.ExpectedResourceVersion); err != nil {
		return DeleteHostResult{}, err
	}
	s.mu.Lock()
	delete(s.runtime, id)
	s.mu.Unlock()
	credentialRemoved := s.secrets.Delete(record.CredentialFile) == nil
	return DeleteHostResult{
		Deleted: true, RemoteRevoked: remoteRevoked,
		CredentialRemoved: credentialRemoved,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, id string) (Host, error) {
	if id == LocalHostID {
		s.invalidateLocalTelemetry()
		return s.localHostSummary(ctx), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := s.store.Host(id); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return Host{}, err
		}
		if _, v2Err := s.storeV2.Host(id); v2Err != nil {
			if record, lightErr := s.light.Host(id); lightErr == nil {
				return s.publicLightHost(record, s.now().UTC()), nil
			}
			return Host{}, v2Err
		}
	}
	s.mu.Lock()
	current := s.runtime[id]
	current.nextPollAt = s.now().UTC()
	s.runtime[id] = current
	started := s.started
	s.mu.Unlock()
	if started {
		s.signal()
	}
	return s.Host(ctx, id)
}

func (s *Service) CreatePairingCode() (PairingCode, error) {
	return s.store.CreatePairingCode(s.now().UTC())
}

func (s *Service) CreatePairingCodeV2() (PairingCode, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.createPairingCodeV2()
}

func (s *Service) Controllers() []Controller {
	records := s.store.Controllers()
	recordsV2 := s.storeV2.Controllers()
	result := make([]Controller, 0, len(records)+len(recordsV2))
	for _, item := range records {
		result = append(result, Controller{
			ID: item.ID, Name: item.Name, Fingerprint: item.Fingerprint,
			Scope: item.Scope, CreatedAt: item.CreatedAt, LastSeenAt: cloneTime(item.LastSeenAt),
		})
	}
	for _, item := range recordsV2 {
		if item.State != controllerStateV2Active {
			continue
		}
		result = append(result, Controller{
			ID: item.ID, Name: item.Name, Fingerprint: item.Fingerprint,
			Scope: item.Scope, CreatedAt: item.CreatedAt,
			LastSeenAt: cloneTime(item.LastSeenAt),
		})
	}
	return result
}

func (s *Service) DeleteController(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	if err := s.store.DeleteController(id); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	record, err := s.storeV2.Controller(id)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	_, err = s.storeV2.RevokeController(
		id,
		record.TransactionID,
		now,
		now.Add(v2RevocationRetain),
	)
	if err != nil {
		return err
	}
	if err := s.filePeersV2.DeleteController(id); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func (s *Service) AcceptPair(source string, input PairRequest) (PairResponse, error) {
	now := s.now().UTC()
	if !s.pairLimiter.Allow(cleanRateSubject(source), now) {
		return PairResponse{}, ErrRateLimited
	}
	if !validID(input.ControllerID) ||
		input.FederationProtocol != FederationProtocol ||
		len(input.PairingCode) != 81 {
		return PairResponse{}, ErrPairingCode
	}
	name, err := validateOptionalName(input.ControllerName)
	if err != nil {
		return PairResponse{}, ErrPairingCode
	}
	publicKey, err := decodePublicKey(input.PublicKey)
	if err != nil {
		return PairResponse{}, ErrPairingCode
	}
	controller := controllerRecord{
		ID: input.ControllerID, Name: name, PublicKey: encodePublicKey(publicKey),
		Fingerprint: fingerprint(publicKey), Scope: SummaryScope, CreatedAt: now,
	}
	if err := s.store.ConsumePairingCode(input.PairingCode, controller, now); err != nil {
		return PairResponse{}, err
	}
	return PairResponse{
		NodeID: s.store.NodeID(), Hostname: s.hostname,
		PanelVersion: s.panelVersion, FederationProtocol: FederationProtocol,
	}, nil
}

func (s *Service) SignedSummary(ctx context.Context, request *http.Request) (FederationSummary, error) {
	now := s.now().UTC()
	controllerID, nonce, err := s.authenticate(request, http.MethodGet, summaryPath, now)
	if err != nil {
		return FederationSummary{}, err
	}
	if !s.requestLimiter.Allow(controllerID, now) {
		return FederationSummary{}, ErrRateLimited
	}
	if err := s.replays.Accept(controllerID, nonce, now); err != nil {
		return FederationSummary{}, err
	}
	telemetry, err := s.localTelemetry(ctx)
	if err != nil {
		return FederationSummary{}, err
	}
	telemetry = telemetryForFederation(
		telemetry,
		request.Header.Get(FederationCapabilitiesHeader),
	)
	s.store.TouchController(controllerID, s.now().UTC())
	return FederationSummary{
		NodeID: s.store.NodeID(), PanelVersion: s.panelVersion,
		FederationProtocol: FederationProtocol,
		SecurityEntrancePath: s.responseSecurityEntrancePath(
			request.Header.Get(FederationCapabilitiesHeader),
		),
		Telemetry: telemetry,
	}, nil
}

func telemetryForFederation(value contract.HostTelemetry, capabilities string) contract.HostTelemetry {
	if !hasFederationCapability(capabilities, SSHLoginCapability) {
		value.SSHLogin = nil
	}
	return value
}

func (s *Service) responseSecurityEntrancePath(capabilities string) string {
	if !hasFederationCapability(capabilities, SecurityEntrancePathCapability) ||
		s.securityEntrancePath == nil {
		return ""
	}
	path := s.securityEntrancePath()
	if !validSecurityEntrancePath(path) {
		return ""
	}
	return path
}

func (s *Service) SignedRevoke(request *http.Request) error {
	now := s.now().UTC()
	controllerID, nonce, err := s.authenticate(request, http.MethodDelete, revokePath, now)
	if err != nil {
		return err
	}
	if err := s.replays.Accept(controllerID, nonce, now); err != nil {
		return err
	}
	return s.store.DeleteController(controllerID)
}

func (s *Service) authenticate(
	request *http.Request,
	method string,
	path string,
	now time.Time,
) (string, string, error) {
	if request == nil || request.Method != method || request.URL.Path != path ||
		request.URL.RawQuery != "" || request.URL.RawPath != "" ||
		request.ContentLength != 0 || len(request.TransferEncoding) != 0 {
		return "", "", ErrAuthentication
	}
	controllerID := strings.TrimSpace(request.Header.Get(headerControllerID))
	if !validID(controllerID) {
		return "", "", ErrAuthentication
	}
	record, err := s.store.Controller(controllerID)
	if err != nil || record.Scope != SummaryScope {
		return "", "", ErrAuthentication
	}
	publicKey, err := decodePublicKey(record.PublicKey)
	if err != nil {
		return "", "", ErrAuthentication
	}
	verifiedID, nonce, err := VerifyRequest(
		request, s.store.NodeID(), publicKey, now,
	)
	if err != nil {
		return "", "", err
	}
	if verifiedID != controllerID {
		return "", "", ErrAuthentication
	}
	return controllerID, nonce, nil
}

func (s *Service) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.schedulerTick)
	checkpoint := time.NewTicker(s.checkpointEvery)
	defer ticker.Stop()
	defer checkpoint.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.launchDue()
		case <-s.kick:
			s.launchDue()
		case <-checkpoint.C:
			_ = s.checkpoint()
		}
	}
}

func (s *Service) launchDue() {
	now := s.now().UTC()
	records := s.store.Hosts()
	for _, record := range records {
		s.mu.Lock()
		current, exists := s.runtime[record.ID]
		if !exists {
			current.nextPollAt = now
		}
		if current.inFlight || current.nextPollAt.After(now) {
			s.runtime[record.ID] = current
			s.mu.Unlock()
			continue
		}
		current.inFlight = true
		s.runtime[record.ID] = current
		ctx := s.ctx
		s.wg.Add(1)
		s.mu.Unlock()
		go func(id string) {
			defer s.wg.Done()
			select {
			case s.sem <- struct{}{}:
				defer func() { <-s.sem }()
				if ctx.Err() != nil {
					s.clearInFlight(id)
					return
				}
				s.poll(ctx, id)
			case <-ctx.Done():
				s.clearInFlight(id)
			}
		}(record.ID)
	}
	s.launchDueV2(now)
}

func (s *Service) poll(ctx context.Context, id string) {
	record, err := s.store.Host(id)
	if err != nil {
		s.clearInFlight(id)
		return
	}
	startedAt := s.now().UTC()
	key, err := s.secrets.Read(record.CredentialFile)
	var summary FederationSummary
	if err == nil {
		summary, err = s.remote.Summary(
			ctx, record.Origin, record.ControllerID,
			record.RemoteNodeID, key, startedAt,
		)
	}
	finishedAt := s.now().UTC()
	if err == nil {
		err = validateFederationSummary(summary, record.RemoteNodeID, finishedAt)
	}
	s.mu.Lock()
	current, exists := s.runtime[id]
	if !exists {
		s.mu.Unlock()
		return
	}
	current.inFlight = false
	current.lastAttemptAt = timePointer(finishedAt)
	if err != nil {
		current.consecutiveFailures++
		current.lastErrorCode = remoteErrorCode(err)
		current.lastError = remoteErrorMessage(current.lastErrorCode)
		current.nextPollAt = finishedAt.Add(s.failureDelay(current.consecutiveFailures))
		s.runtime[id] = current
		s.mu.Unlock()
		return
	}
	snapshot := &HostSnapshot{
		Telemetry: cloneTelemetry(summary.Telemetry), ReceivedAt: finishedAt,
		LatencyMilliseconds: elapsedMilliseconds(finishedAt.Sub(startedAt)),
	}
	if previous := current.snapshot; previous != nil && finishedAt.After(previous.ReceivedAt) {
		elapsed := finishedAt.Sub(previous.ReceivedAt).Seconds()
		if summary.Telemetry.Network.ReceivedBytes >= previous.Telemetry.Network.ReceivedBytes {
			snapshot.ReceiveBytesPerSecond = float64(
				summary.Telemetry.Network.ReceivedBytes-previous.Telemetry.Network.ReceivedBytes,
			) / elapsed
		}
		if summary.Telemetry.Network.SentBytes >= previous.Telemetry.Network.SentBytes {
			snapshot.TransmitBytesPerSecond = float64(
				summary.Telemetry.Network.SentBytes-previous.Telemetry.Network.SentBytes,
			) / elapsed
		}
	}
	current.snapshot = snapshot
	current.lastSuccessAt = timePointer(finishedAt)
	current.consecutiveFailures = 0
	current.lastErrorCode = ""
	current.lastError = ""
	current.panelVersion = summary.PanelVersion
	current.securityEntrancePath = summary.SecurityEntrancePath
	current.nextPollAt = finishedAt.Add(s.jitter(s.pollInterval))
	s.runtime[id] = current
	s.mu.Unlock()
}

func (s *Service) clearInFlight(id string) {
	s.mu.Lock()
	current, ok := s.runtime[id]
	if ok {
		current.inFlight = false
		s.runtime[id] = current
	}
	s.mu.Unlock()
}

func (s *Service) failureDelay(failures int) time.Duration {
	exponent := min(max(failures-1, 0), 4)
	delay := s.pollInterval * time.Duration(1<<exponent)
	if delay > maxPollBackoff {
		delay = maxPollBackoff
	}
	return s.jitter(delay)
}

func (s *Service) signal() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) checkpoint() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.RLock()
	runtime := make(map[string]runtimeState, len(s.runtime))
	for id, current := range s.runtime {
		current.snapshot = cloneSnapshot(current.snapshot)
		current.lastAttemptAt = cloneTime(current.lastAttemptAt)
		current.lastSuccessAt = cloneTime(current.lastSuccessAt)
		runtime[id] = current
	}
	s.mu.RUnlock()
	checkpointErr := errors.Join(
		s.store.Checkpoint(runtime),
		s.storeV2.Checkpoint(runtime),
		s.light.Checkpoint(s.now().UTC()),
	)
	return errors.Join(checkpointErr, s.cleanupV2(s.now().UTC()))
}

func (s *Service) localHostSummary(ctx context.Context) Host {
	if ctx == nil {
		ctx = context.Background()
	}
	telemetryCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	startedAt := s.now().UTC()
	telemetry, err := s.localTelemetry(telemetryCtx)
	finishedAt := s.now().UTC()

	s.mu.Lock()
	current := s.localHost
	current.lastAttemptAt = timePointer(finishedAt)
	if err != nil {
		current.consecutiveFailures++
		current.lastErrorCode = "local_agent_unavailable"
		current.lastError = "无法读取本机 Agent 主机摘要"
	} else {
		if current.snapshot == nil ||
			!current.snapshot.Telemetry.CollectedAt.Equal(telemetry.CollectedAt) {
			snapshot := &HostSnapshot{
				Telemetry:           cloneTelemetry(telemetry),
				ReceivedAt:          finishedAt,
				LatencyMilliseconds: elapsedMilliseconds(finishedAt.Sub(startedAt)),
			}
			if previous := current.snapshot; previous != nil && finishedAt.After(previous.ReceivedAt) {
				elapsed := finishedAt.Sub(previous.ReceivedAt).Seconds()
				if telemetry.Network.ReceivedBytes >= previous.Telemetry.Network.ReceivedBytes {
					snapshot.ReceiveBytesPerSecond = float64(
						telemetry.Network.ReceivedBytes-previous.Telemetry.Network.ReceivedBytes,
					) / elapsed
				}
				if telemetry.Network.SentBytes >= previous.Telemetry.Network.SentBytes {
					snapshot.TransmitBytesPerSecond = float64(
						telemetry.Network.SentBytes-previous.Telemetry.Network.SentBytes,
					) / elapsed
				}
			}
			current.snapshot = snapshot
		}
		current.lastSuccessAt = timePointer(finishedAt)
		current.consecutiveFailures = 0
		current.lastErrorCode = ""
		current.lastError = ""
		current.panelVersion = s.panelVersion
	}
	s.localHost = current
	public := localPublicHost(
		s.store.NodeID(), s.hostname, s.store.LocalName(),
		s.panelVersion, current, finishedAt,
	)
	s.mu.Unlock()
	return public
}

func (s *Service) invalidateLocalTelemetry() {
	s.localMu.Lock()
	s.localExpires = time.Time{}
	s.localMu.Unlock()
}

func (s *Service) localTelemetry(ctx context.Context) (contract.HostTelemetry, error) {
	for {
		now := s.now().UTC()
		s.localMu.Lock()
		if now.Before(s.localExpires) && !s.localValue.CollectedAt.IsZero() {
			value := cloneTelemetry(s.localValue)
			s.localMu.Unlock()
			return value, nil
		}
		if pending := s.localInFlight; pending != nil {
			s.localMu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return contract.HostTelemetry{}, ctx.Err()
			}
		}
		pending := make(chan struct{})
		s.localInFlight = pending
		s.localMu.Unlock()

		value, err := s.telemetry.Telemetry(ctx)
		if err == nil {
			err = validateTelemetry(value, now)
		}
		s.localMu.Lock()
		if err == nil {
			s.localValue = cloneTelemetry(value)
			s.localExpires = now.Add(5 * time.Second)
		}
		s.localError = err
		s.localInFlight = nil
		close(pending)
		s.localMu.Unlock()
		return cloneTelemetry(value), err
	}
}

func localPublicHost(
	nodeID string,
	hostname string,
	localName string,
	panelVersion string,
	current runtimeState,
	now time.Time,
) Host {
	name := localName
	if name == "" {
		name = hostname
	}
	if localName == "" && current.snapshot != nil && current.snapshot.Telemetry.Hostname != "" {
		name = current.snapshot.Telemetry.Hostname
	}
	resourceHash := sha256.Sum256([]byte(nodeID + "\x00" + name))
	return Host{
		ID: LocalHostID, IsLocal: true, Name: name,
		Kind:              HostKindPanel,
		TransportSecurity: TransportSecurityTLS,
		RemoteNodeID:      nodeID, FederationProtocol: FederationProtocol,
		Scope: SummaryTerminalFilesScope, TerminalAvailable: true, FileManagementAvailable: true,
		PanelVersion: panelVersion, State: hostState(current, now),
		LastSnapshot:        cloneSnapshot(current.snapshot),
		LastAttemptAt:       cloneTime(current.lastAttemptAt),
		LastSuccessAt:       cloneTime(current.lastSuccessAt),
		ConsecutiveFailures: current.consecutiveFailures,
		LastErrorCode:       current.lastErrorCode, LastError: current.lastError,
		ResourceVersion: "sha256:" + hex.EncodeToString(resourceHash[:]),
		CreatedAt:       now, UpdatedAt: now,
	}
}

func publicHost(record hostRecord, current runtimeState, now time.Time) Host {
	panelVersion := current.panelVersion
	if panelVersion == "" {
		panelVersion = record.PanelVersion
	}
	var next *time.Time
	if !current.nextPollAt.IsZero() {
		next = timePointer(current.nextPollAt)
	}
	return Host{
		ID: record.ID, Name: record.Name, Origin: record.Origin,
		Kind:              HostKindPanel,
		TransportSecurity: TransportSecurityTLS,
		PeerFingerprint:   "",
		RemoteNodeID:      record.RemoteNodeID, FederationProtocol: record.FederationProtocol,
		Scope: SummaryScope, TerminalAvailable: false, FileManagementAvailable: true,
		PanelVersion: panelVersion, State: hostState(current, now),

		SecurityEntrancePath: current.securityEntrancePath,

		LastSnapshot:        cloneSnapshot(current.snapshot),
		LastAttemptAt:       cloneTime(current.lastAttemptAt),
		LastSuccessAt:       cloneTime(current.lastSuccessAt),
		ConsecutiveFailures: current.consecutiveFailures,
		LastErrorCode:       current.lastErrorCode, LastError: current.lastError,
		Polling: current.inFlight, NextPollAt: next,
		ResourceVersion: record.ResourceVersion,
		CreatedAt:       record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func hostState(current runtimeState, now time.Time) HostState {
	switch current.lastErrorCode {
	case "authentication_failed":
		return HostAuthFailed
	case "tls_error":
		return HostTLSFailed
	case "protocol_incompatible", "identity_changed":
		return HostIncompatible
	}
	if current.snapshot == nil {
		if current.consecutiveFailures >= 3 {
			return HostOffline
		}
		if current.consecutiveFailures > 0 {
			return HostDegraded
		}
		return HostUnknown
	}
	if current.consecutiveFailures >= 3 {
		return HostOffline
	}
	if current.lastSuccessAt == nil || now.Sub(*current.lastSuccessAt) > staleAfter {
		return HostStale
	}
	if current.consecutiveFailures > 0 {
		return HostDegraded
	}
	return HostOnline
}

func validatePairResponse(response PairResponse) error {
	if !validID(response.NodeID) || response.FederationProtocol != FederationProtocol {
		return ErrProtocolMismatch
	}
	if cleanDisplayText(response.Hostname, 253) != response.Hostname ||
		cleanDisplayText(response.PanelVersion, 64) != response.PanelVersion {
		return &RemoteError{Code: "invalid_response"}
	}
	return nil
}

func validateFederationSummary(summary FederationSummary, expectedNodeID string, now time.Time) error {
	if summary.NodeID != expectedNodeID {
		return ErrIdentityMismatch
	}
	if summary.FederationProtocol != FederationProtocol {
		return ErrProtocolMismatch
	}
	if cleanDisplayText(summary.PanelVersion, 64) != summary.PanelVersion {
		return &RemoteError{Code: "invalid_response"}
	}
	if summary.SecurityEntrancePath != "" &&
		!validSecurityEntrancePath(summary.SecurityEntrancePath) {
		return &RemoteError{Code: "invalid_response"}
	}
	return validateTelemetry(summary.Telemetry, now)
}

func validSecurityEntrancePath(value string) bool {
	if len(value) < 6 || len(value) > 48 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func elapsedMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	milliseconds := duration.Milliseconds()
	if milliseconds == 0 {
		return 1
	}
	return milliseconds
}

func validateTelemetry(value contract.HostTelemetry, now time.Time) error {
	if value.CollectedAt.IsZero() || value.CollectedAt.Before(now.Add(-10*time.Minute)) ||
		value.CollectedAt.After(now.Add(5*time.Minute)) ||
		cleanDisplayText(value.AgentVersion, 64) != value.AgentVersion ||
		cleanDisplayText(value.AgentProtocolVersion, 64) != value.AgentProtocolVersion ||
		cleanDisplayText(value.Hostname, 253) != value.Hostname ||
		cleanDisplayText(value.OS, 256) != value.OS ||
		cleanDisplayText(value.OSID, 64) != value.OSID ||
		cleanDisplayText(value.Kernel, 128) != value.Kernel ||
		cleanDisplayText(value.Architecture, 64) != value.Architecture ||
		cleanDisplayText(value.CPU.Model, 256) != value.CPU.Model ||
		len(value.OSLike) > 16 || value.CPU.Cores < 0 || value.CPU.Cores > 4096 ||
		!finiteInRange(value.CPU.FrequencyMHz, 0, 1_000_000) ||
		!validPercent(value.CPU.UsagePercent) ||
		!validPercent(value.Memory.UsagePercent) ||
		!validPercent(value.Disk.UsagePercent) ||
		value.UptimeSeconds > 200*366*24*60*60 ||
		value.Memory.TotalBytes > maxSafeJSONInteger ||
		value.Memory.AvailableBytes > value.Memory.TotalBytes ||
		value.Memory.UsedBytes > value.Memory.TotalBytes ||
		value.Memory.SwapTotalBytes > maxSafeJSONInteger ||
		value.Memory.SwapUsedBytes > value.Memory.SwapTotalBytes ||
		value.Disk.TotalBytes > maxSafeJSONInteger ||
		value.Disk.UsedBytes > value.Disk.TotalBytes ||
		value.Network.ReceivedBytes > maxSafeJSONInteger ||
		value.Network.SentBytes > maxSafeJSONInteger ||
		value.Network.TCPConnections < 0 || value.Network.TCPConnections > 10_000_000 ||
		value.Network.UDPConnections < 0 || value.Network.UDPConnections > 10_000_000 ||
		!finiteNonNegative(value.Load.One) ||
		!finiteNonNegative(value.Load.Five) ||
		!finiteNonNegative(value.Load.Fifteen) ||
		value.Load.One > 1_000_000 || value.Load.Five > 1_000_000 ||
		value.Load.Fifteen > 1_000_000 {
		return &RemoteError{Code: "invalid_response"}
	}
	for _, item := range value.OSLike {
		if cleanDisplayText(item, 64) != item {
			return &RemoteError{Code: "invalid_response"}
		}
	}
	if value.SSHLogin != nil && (!contract.ValidSSHLoginEvent(*value.SSHLogin) ||
		value.SSHLogin.OccurredAt.After(now.Add(5*time.Minute))) {
		return &RemoteError{Code: "invalid_response"}
	}
	publicFields := []struct {
		value string
		limit int
	}{
		{value.PublicNetwork.ISP, 160}, {value.PublicNetwork.Country, 64},
		{value.PublicNetwork.CountryCode, 2}, {value.PublicNetwork.Region, 96},
		{value.PublicNetwork.City, 96}, {value.PublicNetwork.Timezone, 96},
		{value.PublicNetwork.Source, 64},
	}
	for _, field := range publicFields {
		if cleanDisplayText(field.value, field.limit) != field.value {
			return &RemoteError{Code: "invalid_response"}
		}
	}
	for _, raw := range []string{value.PublicNetwork.IPv4, value.PublicNetwork.IPv6} {
		if raw != "" {
			if _, err := netip.ParseAddr(raw); err != nil {
				return &RemoteError{Code: "invalid_response"}
			}
		}
	}
	return nil
}

func remoteErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPrivateOrigin):
		return "origin_blocked"
	case errors.Is(err, ErrAuthentication):
		return "authentication_failed"
	case errors.Is(err, ErrProtocolMismatch):
		return "protocol_incompatible"
	case errors.Is(err, ErrIdentityMismatch):
		return "identity_changed"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	}
	var remote *RemoteError
	if errors.As(err, &remote) && remote.Code != "" {
		return remote.Code
	}
	return "unreachable"
}

func remoteErrorMessage(code string) string {
	switch code {
	case "origin_blocked":
		return "目标地址被集群网络安全策略拒绝"
	case "authentication_failed":
		return "远端授权已失效，请重新配对"
	case "tls_error":
		return "远端 HTTPS 证书校验失败"
	case "protocol_incompatible":
		return "远端 KPanel 集群协议不兼容"
	case "identity_changed":
		return "远端 KPanel 身份发生变化，已停止信任"
	case "rate_limited":
		return "远端暂时限制了监控请求"
	case "invalid_response", "response_too_large":
		return "远端返回了无效的监控摘要"
	default:
		return "暂时无法连接远端 KPanel"
	}
}

func validateOptionalName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return validateRequiredName(value)
}

func validateRequiredName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || cleanDisplayText(value, 80) != value {
		return "", errors.New("cluster host name is invalid")
	}
	return value, nil
}

func cleanDisplayText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := make([]rune, 0, min(len(value), limit))
	for _, character := range value {
		if unicode.IsControl(character) {
			return ""
		}
		runes = append(runes, character)
		if len(runes) > limit {
			return ""
		}
	}
	return string(runes)
}

func validResourceVersion(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validPercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func finiteInRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) &&
		value >= minimum && value <= maximum
}

func cleanRateSubject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return "unknown"
	}
	return value
}

func cloneTelemetry(value contract.HostTelemetry) contract.HostTelemetry {
	value.OSLike = append([]string(nil), value.OSLike...)
	if value.SSHLogin != nil {
		copy := *value.SSHLogin
		value.SSHLogin = &copy
	}
	return value
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func jitterDuration(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	var random [1]byte
	if _, err := rand.Read(random[:]); err != nil {
		return base
	}
	// 80% through 120%, inclusive.
	percentage := 80 + int(random[0])%41
	return base * time.Duration(percentage) / 100
}
