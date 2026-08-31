package cluster

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	v2PairingTTL       = 5 * time.Minute
	v2PendingCommitTTL = 24 * time.Hour
	v2RequestSkew      = 2 * time.Minute
	v2RevocationRetain = 10 * time.Minute
)

type remoteV2API interface {
	PairV2(
		context.Context,
		string,
		string,
		string,
		string,
		noise.DHKey,
		v2PairingDescriptor,
		time.Time,
	) (v2PairResult, error)
	CommitV2(
		context.Context,
		string,
		string,
		string,
		string,
		noise.DHKey,
		[]byte,
		time.Time,
	) (v2CommitResult, error)
	SummaryV2(
		context.Context,
		string,
		string,
		string,
		noise.DHKey,
		[]byte,
		time.Time,
	) (FederationSummary, error)
	RevokeV2(
		context.Context,
		string,
		string,
		string,
		noise.DHKey,
		[]byte,
		time.Time,
	) error
}

type remoteV2TerminalAPI interface {
	TerminalOpenV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, TerminalOpenRequest) (TerminalOpenResponse, error)
	TerminalOutputV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, TerminalOutputRequest) (terminal.Output, error)
	TerminalInputV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, TerminalInputRequest) error
	TerminalResizeV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, TerminalResizeRequest) error
	TerminalCloseV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, TerminalCloseRequest) error
}

type v2OriginValidator interface {
	ValidateV2Origin(context.Context, string) (string, error)
}

func (s *Service) cleanupV2(now time.Time) error {
	s.v2SecretStateMu.Lock()
	defer s.v2SecretStateMu.Unlock()
	return s.cleanupV2Locked(now)
}

func (s *Service) cleanupV2Locked(now time.Time) error {
	expired, pairingErr := s.storeV2.GCExpiredPairingCodes(now)
	var cleanupErr error
	for _, name := range expired {
		cleanupErr = errors.Join(cleanupErr, s.secretsV2.Delete(name))
	}
	_, controllerErr := s.storeV2.GCRevokedControllers(now)
	orphanErr := s.secretsV2.RemoveOrphans(s.storeV2.CredentialReferences())
	return errors.Join(pairingErr, controllerErr, cleanupErr, orphanErr)
}

func (s *Service) createPairingCodeV2() (PairingCode, error) {
	s.v2SecretStateMu.Lock()
	defer s.v2SecretStateMu.Unlock()

	now := s.now().UTC()
	if err := s.cleanupV2Locked(now); err != nil {
		return PairingCode{}, err
	}

	expiresAt := now.Add(v2PairingTTL)
	code, codeID, pairingKey, err := makeV2PairingCode(
		s.store.NodeID(), s.nodeIdentityV2.PublicKey, expiresAt,
	)
	if err != nil {
		return PairingCode{}, err
	}
	credentialFile, err := s.secretsV2.WritePairingCredential(
		codeID,
		v2Credential{
			TargetPublic: append([]byte(nil), s.nodeIdentityV2.PublicKey...),
			PairingKey:   append([]byte(nil), pairingKey...),
		},
	)
	if err != nil {
		return PairingCode{}, err
	}
	if err := s.storeV2.AddPairingCode(pairingCodeRecordV2{
		ID:             codeID,
		CredentialFile: credentialFile,
		ExpiresAt:      expiresAt,
	}, now); err != nil {
		_ = s.secretsV2.Delete(credentialFile)
		return PairingCode{}, err
	}
	_ = s.secretsV2.RemoveOrphans(s.storeV2.CredentialReferences())
	return PairingCode{
		Code: code, Scope: SummaryTerminalFilesScope, ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) addHostV2Locked(
	ctx context.Context,
	input AddHostInput,
) (Host, error) {
	if s.remoteV2 == nil {
		return Host{}, ErrProtocolMismatch
	}
	origin, err := NormalizeV2Origin(input.Origin)
	if err != nil {
		return Host{}, err
	}
	if validator, ok := s.remoteV2.(v2OriginValidator); ok {
		validated, err := validator.ValidateV2Origin(ctx, origin)
		if err != nil {
			return Host{}, err
		}
		if validated != origin {
			return Host{}, ErrInvalidOrigin
		}
	}
	name, err := validateOptionalName(input.Name)
	if err != nil {
		return Host{}, err
	}
	now := s.now().UTC()
	pairing, err := parseV2PairingCode(input.PairingCode, now)
	if err != nil {
		return Host{}, err
	}
	if pairing.NodeID == s.store.NodeID() {
		return Host{}, ErrDuplicate
	}
	legacy := s.store.Hosts()
	records := s.storeV2.Hosts()
	if len(legacy)+len(records)+len(s.light.Hosts()) >= MaxHosts {
		return Host{}, ErrHostLimit
	}
	for _, record := range legacy {
		if strings.EqualFold(record.Origin, origin) ||
			record.RemoteNodeID == pairing.NodeID {
			return Host{}, ErrDuplicate
		}
	}
	for _, record := range records {
		if strings.EqualFold(record.Origin, origin) ||
			record.RemoteNodeID == pairing.NodeID {
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
	transactionID, err := randomHex(16)
	if err != nil {
		return Host{}, err
	}
	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		return Host{}, fmt.Errorf("generate cluster v2 controller identity: %w", err)
	}
	s.v2SecretStateMu.Lock()
	pairingCredentialFile, err := s.secretsV2.WritePairingCredential(
		pairing.CodeID,
		v2Credential{
			ControllerPrivate: append([]byte(nil), controllerKey.Private...),
			ControllerPublic:  append([]byte(nil), controllerKey.Public...),
			TargetPublic:      append([]byte(nil), pairing.TargetPublicKey...),
			PairingKey:        append([]byte(nil), pairing.PairingKey...),
		},
	)
	if err != nil {
		s.v2SecretStateMu.Unlock()
		return Host{}, err
	}
	record := hostRecordV2{
		ID: hostID, Name: name, Origin: origin,
		TransportSecurity: v2TransportSecurity(origin),
		RemoteNodeID:      pairing.NodeID, ControllerID: controllerID,
		State: hostStateV2PendingPair, TransactionID: transactionID,
		PairingCredentialFile: pairingCredentialFile,
		TargetPublicKey:       base64.RawURLEncoding.EncodeToString(pairing.TargetPublicKey),
		PeerFingerprint:       fingerprintV2(pairing.TargetPublicKey),
		FederationProtocol:    FederationProtocolV2,
		Scope:                 SummaryScope,
		CreatedAt:             now, UpdatedAt: now,
	}
	// Persist the administrator's mutual-file intent before the Host record. If
	// the process stops between these two writes, startup reconciliation removes
	// the harmless orphaned pending grant; the opposite ordering could leave a
	// successfully resumed pairing permanently one-way.
	filePeerOrigin := input.ControllerOrigin
	if filePeerOrigin == "" {
		filePeerOrigin = s.publicURL
	}
	filePeerPrepared := false
	if filePeerOrigin != "" {
		if normalized, normalizeErr := NormalizeV2Origin(filePeerOrigin); normalizeErr == nil {
			record.ResourceVersion = hostResourceVersionV2(record)
			if _, prepareErr := s.filePeersV2.PrepareGrant(record, normalized, now); prepareErr == nil {
				filePeerPrepared = true
			}
		}
	}
	if err := s.storeV2.AddHost(record); err != nil {
		if filePeerPrepared {
			_ = s.deleteFilePeerGrant(record.ID)
		}
		_ = s.secretsV2.Delete(pairingCredentialFile)
		s.v2SecretStateMu.Unlock()
		return Host{}, err
	}
	s.v2SecretStateMu.Unlock()
	s.mu.Lock()
	s.runtime[hostID] = runtimeState{
		nextPollAt: now, inFlight: true,
	}
	s.mu.Unlock()
	s.pollV2Locked(ctx, hostID)
	if !filePeerPrepared && (input.ControllerOrigin != "" || s.publicURL != "") {
		_ = s.enableFilePeerV2Locked(ctx, hostID, input.ControllerOrigin)
	}
	return s.Host(ctx, hostID)
}

func (s *Service) advanceV2Host(
	ctx context.Context,
	record hostRecordV2,
) (hostRecordV2, error) {
	for {
		switch record.State {
		case hostStateV2PendingPair:
			credential, err := s.secretsV2.ReadCredential(
				record.PairingCredentialFile,
			)
			if err != nil {
				return record, err
			}
			codeID := pairingCodeIDFromCredential(record.PairingCredentialFile)
			if codeID == "" {
				return record, ErrAuthentication
			}
			response, err := s.remoteV2.PairV2(
				ctx, record.Origin, record.ControllerID, s.hostname,
				record.TransactionID, noiseKeyV2(credential),
				v2PairingDescriptor{
					NodeID:          record.RemoteNodeID,
					TargetPublicKey: append([]byte(nil), credential.TargetPublic...),
					CodeID:          codeID,
					PairingKey:      append([]byte(nil), credential.PairingKey...),
				},
				s.now().UTC(),
			)
			if err != nil {
				return record, err
			}
			if err := validateV2PairResult(
				response, record.RemoteNodeID, record.TransactionID,
			); err != nil {
				return record, err
			}
			if response.Scope == "" {
				response.Scope = SummaryScope
			}
			hostCredential := v2Credential{
				ControllerPrivate: append([]byte(nil), credential.ControllerPrivate...),
				ControllerPublic:  append([]byte(nil), credential.ControllerPublic...),
				TargetPublic:      append([]byte(nil), credential.TargetPublic...),
			}
			s.v2SecretStateMu.Lock()
			credentialFile, err := s.ensureV2HostCredential(
				record.ID, hostCredential,
			)
			if err != nil {
				s.v2SecretStateMu.Unlock()
				return record, err
			}
			if record.Name == "" {
				record.Name = cleanDisplayText(response.Hostname, 80)
			}
			record.State = hostStateV2PendingCommit
			record.CredentialFile = credentialFile
			record.PanelVersion = response.PanelVersion
			record.Scope = response.Scope
			record.UpdatedAt = s.now().UTC()
			record, err = s.storeV2.UpdateHost(record, record.ResourceVersion)
			if err != nil {
				_ = s.secretsV2.RemoveOrphans(
					s.storeV2.CredentialReferences(),
				)
				s.v2SecretStateMu.Unlock()
				return record, err
			}
			s.v2SecretStateMu.Unlock()
		case hostStateV2PendingCommit:
			credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
			if err != nil {
				return record, err
			}
			response, err := s.remoteV2.CommitV2(
				ctx, record.Origin, record.ControllerID,
				record.RemoteNodeID, record.TransactionID,
				noiseKeyV2(credential), credential.TargetPublic,
				s.now().UTC(),
			)
			if err != nil {
				return record, err
			}
			if !response.Active ||
				response.TransactionID != record.TransactionID {
				return record, ErrAuthentication
			}
			pairingCredentialFile := record.PairingCredentialFile
			record.State = hostStateV2Active
			record.PairingCredentialFile = ""
			record.UpdatedAt = s.now().UTC()
			record, err = s.storeV2.UpdateHost(record, record.ResourceVersion)
			if err != nil {
				return record, err
			}
			_ = s.secretsV2.Delete(pairingCredentialFile)
			return record, nil
		case hostStateV2Active, hostStateV2PendingRevoke:
			return record, nil
		default:
			return record, ErrProtocolMismatch
		}
	}
}

func (s *Service) ensureV2HostCredential(
	hostID string,
	credential v2Credential,
) (string, error) {
	name, err := s.secretsV2.WriteHostCredential(hostID, credential)
	if err == nil {
		return name, nil
	}
	expected := "host-" + hostID + ".v2key"
	existing, readErr := s.secretsV2.ReadCredential(expected)
	if readErr == nil && v2CredentialsEqual(existing, credential) {
		return expected, nil
	}
	return "", err
}

func (s *Service) deleteHostV2Locked(
	ctx context.Context,
	id string,
	input DeleteHostInput,
) (DeleteHostResult, error) {
	record, err := s.storeV2.Host(id)
	if err != nil {
		return DeleteHostResult{}, err
	}
	if record.ResourceVersion != input.ExpectedResourceVersion {
		return DeleteHostResult{}, ErrConflict
	}
	if record.State == hostStateV2PendingPair {
		if _, err := s.storeV2.DeleteHost(id, record.ResourceVersion); err != nil {
			return DeleteHostResult{}, err
		}
		s.mu.Lock()
		delete(s.runtime, id)
		s.mu.Unlock()
		removed := s.secretsV2.Delete(record.PairingCredentialFile) == nil
		return DeleteHostResult{
			Deleted: true, CredentialRemoved: removed,
		}, nil
	}
	if record.State == hostStateV2PendingCommit {
		pairingCredentialFile := record.PairingCredentialFile
		record.State = hostStateV2PendingRevoke
		record.PairingCredentialFile = ""
		record.UpdatedAt = s.now().UTC()
		record, err = s.storeV2.UpdateHost(record, record.ResourceVersion)
		if err != nil {
			return DeleteHostResult{}, err
		}
		_ = s.secretsV2.Delete(pairingCredentialFile)
	} else if record.State == hostStateV2Active {
		record.State = hostStateV2PendingRevoke
		record.UpdatedAt = s.now().UTC()
		record, err = s.storeV2.UpdateHost(record, record.ResourceVersion)
		if err != nil {
			return DeleteHostResult{}, err
		}
	}
	// The parent Host is no longer active before sidecar cleanup. Linked file
	// authorization therefore fails closed even if deleting the grant hits a
	// storage error.
	if err := s.deleteFilePeerGrant(record.ID); err != nil {
		return DeleteHostResult{}, err
	}
	result, err := s.revokeAndFinalizeV2(ctx, record)
	if err != nil {
		return s.finalizeLocalHostV2(record, false)
	}
	return result, nil
}

func (s *Service) revokeAndFinalizeV2(
	ctx context.Context,
	record hostRecordV2,
) (DeleteHostResult, error) {
	if s.remoteV2 == nil {
		return DeleteHostResult{}, ErrProtocolMismatch
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return DeleteHostResult{}, err
	}
	revokeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	err = s.remoteV2.RevokeV2(
		revokeCtx, record.Origin, record.ControllerID,
		record.RemoteNodeID, noiseKeyV2(credential),
		credential.TargetPublic, s.now().UTC(),
	)
	cancel()
	if err != nil {
		return DeleteHostResult{}, err
	}
	return s.finalizeLocalHostV2(record, true)
}

func (s *Service) finalizeLocalHostV2(
	record hostRecordV2,
	remoteRevoked bool,
) (DeleteHostResult, error) {
	if err := s.deleteFilePeerGrant(record.ID); err != nil {
		return DeleteHostResult{}, err
	}
	if _, err := s.storeV2.DeleteHost(record.ID, record.ResourceVersion); err != nil &&
		!errors.Is(err, ErrNotFound) {
		return DeleteHostResult{}, err
	}
	s.mu.Lock()
	delete(s.runtime, record.ID)
	s.mu.Unlock()
	credentialRemoved := s.secretsV2.Delete(record.CredentialFile) == nil
	return DeleteHostResult{
		Deleted: true, RemoteRevoked: remoteRevoked,
		CredentialRemoved: credentialRemoved,
	}, nil
}

func (s *Service) launchDueV2(now time.Time) {
	for _, record := range s.storeV2.Hosts() {
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
				s.pollV2(ctx, id)
			case <-ctx.Done():
				s.clearInFlight(id)
			}
		}(record.ID)
	}
}

func (s *Service) pollV2(ctx context.Context, id string) {
	s.pollV2Locked(ctx, id)
}

func (s *Service) pollV2Locked(ctx context.Context, id string) {
	record, err := s.storeV2.Host(id)
	if err != nil {
		s.clearInFlight(id)
		return
	}
	startedAt := s.now().UTC()
	if record.State == hostStateV2PendingRevoke {
		_, err = s.revokeAndFinalizeV2(ctx, record)
		if err == nil {
			return
		}
		if _, finalizeErr := s.finalizeLocalHostV2(record, false); finalizeErr == nil {
			return
		} else {
			err = errors.Join(err, finalizeErr)
		}
	} else {
		record, err = s.advanceV2Host(ctx, record)
	}
	filePeerSyncAttempted := false
	if err == nil && record.State == hostStateV2Active {
		if !ScopeAllowsFiles(normalizedV2Scope(record.Scope)) {
			_ = s.deleteFilePeerGrant(record.ID)
		} else {
			s.mu.RLock()
			nextFilePeerSyncAt := s.runtime[id].nextFilePeerSyncAt
			s.mu.RUnlock()
			if nextFilePeerSyncAt.IsZero() || !nextFilePeerSyncAt.After(s.now().UTC()) {
				if grant, grantErr := s.filePeersV2.GrantByHost(record.ID); grantErr == nil {
					filePeerSyncAttempted = true
					if grant, grantErr = s.prepareFilePeerGrantForSync(record, grant); grantErr == nil {
						_ = s.syncFilePeerV2(ctx, record, grant)
					}
				}
			}
		}
	}
	// File-peer lease maintenance is intentionally independent from telemetry.
	// A summary collection failure must not expire an otherwise healthy file
	// route after 30 minutes.
	var summary FederationSummary
	if err == nil && record.State == hostStateV2Active {
		credential, readErr := s.secretsV2.ReadCredential(record.CredentialFile)
		if readErr != nil {
			err = readErr
		} else {
			summary, err = s.remoteV2.SummaryV2(
				ctx, record.Origin, record.ControllerID,
				record.RemoteNodeID, noiseKeyV2(credential),
				credential.TargetPublic, startedAt,
			)
			if err == nil {
				err = validateFederationSummaryV2(
					summary, record.RemoteNodeID, s.now().UTC(),
				)
			}
		}
	}
	finishedAt := s.now().UTC()
	s.mu.Lock()
	current, exists := s.runtime[id]
	if !exists {
		s.mu.Unlock()
		return
	}
	current.inFlight = false
	current.lastAttemptAt = timePointer(finishedAt)
	if filePeerSyncAttempted {
		current.nextFilePeerSyncAt = finishedAt.Add(filePeerSyncInterval)
	}
	if err != nil {
		current.consecutiveFailures++
		current.lastErrorCode = remoteErrorCode(err)
		current.lastError = remoteErrorMessage(current.lastErrorCode)
		current.nextPollAt = finishedAt.Add(
			s.failureDelay(current.consecutiveFailures),
		)
		s.runtime[id] = current
		s.mu.Unlock()
		return
	}
	snapshot := &HostSnapshot{
		Telemetry: cloneTelemetry(summary.Telemetry), ReceivedAt: finishedAt,
		LatencyMilliseconds: max(0, finishedAt.Sub(startedAt).Milliseconds()),
	}
	if previous := current.snapshot; previous != nil &&
		finishedAt.After(previous.ReceivedAt) {
		elapsed := finishedAt.Sub(previous.ReceivedAt).Seconds()
		if summary.Telemetry.Network.ReceivedBytes >=
			previous.Telemetry.Network.ReceivedBytes {
			snapshot.ReceiveBytesPerSecond = float64(
				summary.Telemetry.Network.ReceivedBytes-
					previous.Telemetry.Network.ReceivedBytes,
			) / elapsed
		}
		if summary.Telemetry.Network.SentBytes >=
			previous.Telemetry.Network.SentBytes {
			snapshot.TransmitBytesPerSecond = float64(
				summary.Telemetry.Network.SentBytes-
					previous.Telemetry.Network.SentBytes,
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

func publicHostV2(
	record hostRecordV2,
	current runtimeState,
	now time.Time,
) Host {
	name := record.Name
	if name == "" {
		name = strings.TrimPrefix(
			strings.TrimPrefix(record.Origin, "https://"),
			"http://",
		)
	}
	state := hostState(current, now)
	if record.State == hostStateV2PendingPair ||
		record.State == hostStateV2PendingCommit {
		state = HostPairing
	} else if record.State == hostStateV2PendingRevoke {
		state = HostRevoking
	}
	panelVersion := current.panelVersion
	if panelVersion == "" {
		panelVersion = record.PanelVersion
	}
	var next *time.Time
	if !current.nextPollAt.IsZero() {
		next = timePointer(current.nextPollAt)
	}
	return Host{
		ID: record.ID, Name: name, Origin: record.Origin,
		Kind:                  HostKindPanel,
		TransportSecurity:     record.TransportSecurity,
		PeerFingerprint:       record.PeerFingerprint,
		RemoteNodeID:          record.RemoteNodeID,
		FederationProtocol:    FederationProtocolV2,
		Scope:                 normalizedV2Scope(record.Scope),
		TerminalAvailable:     ScopeAllowsTerminal(normalizedV2Scope(record.Scope)),
		FileTransferAvailable: ScopeAllowsFiles(normalizedV2Scope(record.Scope)),
		PanelVersion:          panelVersion, State: state,

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

func (s *Service) HandleFederationV2(
	ctx context.Context,
	source string,
	path string,
	capabilities string,
	envelope FederationEnvelopeV2,
) (FederationEnvelopeV2, error) {
	now := s.now().UTC()
	sourceLimiter := s.v2SourceLimiter
	if v2TerminalPath(path) || path == v2TerminalRelayPath {
		sourceLimiter = s.terminalSources
	}
	if !sourceLimiter.Allow(cleanRateSubject(source), now) {
		return FederationEnvelopeV2{}, ErrRateLimited
	}
	if err := s.validateV2Request(path, envelope, now); err != nil {
		return FederationEnvelopeV2{}, err
	}
	switch path {
	case v2PairPath:
		return s.handlePairV2(envelope, source, now)
	case v2CommitPath:
		return s.handleCommitV2(envelope, now)
	case v2SummaryPath:
		return s.handleSummaryV2(ctx, capabilities, envelope, now)
	case v2RevokePath:
		return s.handleRevokeV2(envelope, now)
	case v2FileLinkPath:
		return s.handleFilePeerLinkV2(ctx, envelope, now)
	case v2TerminalOpenPath:
		return s.handleTerminalOpenV2(ctx, envelope, now)
	case v2TerminalOutputPath:
		return s.handleTerminalOutputV2(ctx, envelope, now)
	case v2TerminalInputPath:
		return s.handleTerminalInputV2(ctx, envelope, now)
	case v2TerminalResizePath:
		return s.handleTerminalResizeV2(ctx, envelope, now)
	case v2TerminalClosePath:
		return s.handleTerminalCloseV2(ctx, envelope, now)
	case v2TerminalRelayPath:
		return s.handleTerminalRelayV2(ctx, envelope, now)
	default:
		return FederationEnvelopeV2{}, ErrAuthentication
	}
}

func (s *Service) handleTerminalRelayV2(
	ctx context.Context,
	envelope v2Envelope,
	now time.Time,
) (FederationEnvelopeV2, error) {
	record, err := s.light.Host(envelope.ControllerID)
	if err != nil || s.lightTerminal == nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	terminalPublicKey, err := s.light.ReadTerminalPublicKey(record)
	if err != nil || len(terminalPublicKey) != 32 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	payload, peerStatic, handshake, err := openV2Request(
		http.MethodPost, v2TerminalRelayPath, envelope,
		nodeNoiseKeyV2(s.nodeIdentityV2), nil,
	)
	if err != nil || !bytes.Equal(peerStatic, terminalPublicKey) {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if !s.lightTerminalRequests.Allow(envelope.ControllerID, now) {
		return FederationEnvelopeV2{}, ErrRateLimited
	}
	if err := s.replays.Accept("light:"+envelope.ControllerID, envelope.RequestID, now); err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalRelayPollRequest
	if err := decodeV2Payload(payload, &input); err != nil || validateTerminalRelayPoll(input) != nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	response, err := s.lightTerminal.poll(ctx, envelope.ControllerID, input.SessionIDs, input.Events)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, response)
}

func (s *Service) handlePairV2(
	envelope v2Envelope,
	source string,
	now time.Time,
) (FederationEnvelopeV2, error) {
	if !s.pairLimiter.Allow(cleanRateSubject(source), now) {
		return FederationEnvelopeV2{}, ErrRateLimited
	}
	record, err := s.storeV2.PairingCode(envelope.CodeID)
	if err != nil || !record.ExpiresAt.After(now) {
		return FederationEnvelopeV2{}, ErrPairingCode
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil ||
		!bytes.Equal(credential.TargetPublic, s.nodeIdentityV2.PublicKey) {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	payload, peerStatic, handshake, err := openV2Request(
		http.MethodPost, v2PairPath, envelope,
		nodeNoiseKeyV2(s.nodeIdentityV2), credential.PairingKey,
	)
	if err != nil {
		s.recordPairingFailureV2(envelope.CodeID, now)
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if !s.requestLimiter.Allow(envelope.ControllerID, now) {
		return FederationEnvelopeV2{}, ErrRateLimited
	}
	if err := s.replays.Accept(
		envelope.ControllerID, envelope.RequestID, now,
	); err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input v2PairPayload
	if err := decodeV2Payload(payload, &input); err != nil ||
		!validID(input.TransactionID) {
		s.recordPairingFailureV2(envelope.CodeID, now)
		return FederationEnvelopeV2{}, ErrPairingCode
	}
	name, err := validateOptionalName(input.ControllerName)
	if err != nil {
		s.recordPairingFailureV2(envelope.CodeID, now)
		return FederationEnvelopeV2{}, ErrPairingCode
	}
	controller := controllerRecordV2{
		ID: envelope.ControllerID, Name: name,
		PublicKey:   base64.RawURLEncoding.EncodeToString(peerStatic),
		Fingerprint: fingerprintV2(peerStatic), Scope: SummaryTerminalFilesScope,
		State:         controllerStateV2Provisional,
		TransactionID: input.TransactionID,
		CreatedAt:     now, UpdatedAt: now,
	}
	if _, err := s.storeV2.BindPairingCode(
		envelope.CodeID, input.TransactionID, controller, now,
	); err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, v2PairResult{
		TransactionID: input.TransactionID,
		NodeID:        s.store.NodeID(), Hostname: s.hostname,
		PanelVersion:       s.panelVersion,
		FederationProtocol: FederationProtocolV2,
		Scope:              SummaryTerminalFilesScope,
	})
}

func (s *Service) handleCommitV2(
	envelope v2Envelope,
	now time.Time,
) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openControllerV2(
		v2CommitPath, envelope, now,
		controllerStateV2Provisional, controllerStateV2Active,
	)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input v2CommitPayload
	if err := decodeV2Payload(payload, &input); err != nil ||
		input.TransactionID != controller.TransactionID {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	pairingCodeID := ""
	for _, record := range s.storeV2.PairingCodes() {
		if record.ControllerID == controller.ID &&
			record.TransactionID == input.TransactionID {
			pairingCodeID = record.ID
			break
		}
	}
	credentialFile, err := s.storeV2.CommitPairingCode(
		pairingCodeID, input.TransactionID, now,
	)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	if credentialFile != "" {
		_ = s.secretsV2.Delete(credentialFile)
	}
	return sealV2JSONResponse(envelope, handshake, v2CommitResult{
		TransactionID: input.TransactionID,
		Active:        true,
	})
}

func (s *Service) handleSummaryV2(
	ctx context.Context,
	capabilities string,
	envelope v2Envelope,
	now time.Time,
) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openControllerV2(
		v2SummaryPath, envelope, now, controllerStateV2Active,
	)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input struct{}
	if err := decodeV2Payload(payload, &input); err != nil {
		return FederationEnvelopeV2{}, err
	}
	telemetry, err := s.localTelemetry(ctx)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	telemetry = telemetryForFederation(telemetry, capabilities)
	_ = s.storeV2.TouchController(controller.ID, now)
	return sealV2JSONResponse(envelope, handshake, FederationSummary{
		NodeID: s.store.NodeID(), PanelVersion: s.panelVersion,
		FederationProtocol:   FederationProtocolV2,
		SecurityEntrancePath: s.responseSecurityEntrancePath(capabilities),
		Telemetry:            telemetry,
	})
}

func (s *Service) handleRevokeV2(
	envelope v2Envelope,
	now time.Time,
) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openControllerV2(
		v2RevokePath, envelope, now,
		controllerStateV2Provisional,
		controllerStateV2Active,
		controllerStateV2Revoked,
	)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input struct{}
	if err := decodeV2Payload(payload, &input); err != nil {
		return FederationEnvelopeV2{}, err
	}
	if controller.State != controllerStateV2Revoked {
		if _, err := s.storeV2.RevokeController(
			controller.ID, controller.TransactionID, now,
			now.Add(v2RevocationRetain),
		); err != nil {
			return FederationEnvelopeV2{}, err
		}
	}
	if err := s.filePeersV2.DeleteController(controller.ID); err != nil &&
		!errors.Is(err, ErrNotFound) {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(
		envelope, handshake, v2RevokeResult{Revoked: true},
	)
}

func (s *Service) openControllerV2(
	path string,
	envelope v2Envelope,
	now time.Time,
	allowed ...controllerStateV2,
) (controllerRecordV2, []byte, *noise.HandshakeState, error) {
	controller, err := s.storeV2.Controller(envelope.ControllerID)
	if err != nil || !containsControllerStateV2(allowed, controller.State) {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	payload, peerStatic, handshake, err := openV2Request(
		http.MethodPost, path, envelope,
		nodeNoiseKeyV2(s.nodeIdentityV2), nil,
	)
	if err != nil {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	expected, err := base64.RawURLEncoding.DecodeString(controller.PublicKey)
	if err != nil || !bytes.Equal(peerStatic, expected) {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	requestLimiter := s.requestLimiter
	if v2TerminalPath(path) {
		requestLimiter = s.terminalRequests
	} else if path == v2FileOpenPath {
		requestLimiter = s.fileRequests
	}
	if !requestLimiter.Allow(controller.ID, now) {
		return controllerRecordV2{}, nil, nil, ErrRateLimited
	}
	if err := s.replays.Accept(
		controller.ID, envelope.RequestID, now,
	); err != nil {
		return controllerRecordV2{}, nil, nil, err
	}
	return controller, payload, handshake, nil
}

func (s *Service) validateV2Request(
	path string,
	envelope v2Envelope,
	now time.Time,
) error {
	if !v2PathAllowed(http.MethodPost, path) ||
		validateV2Envelope(envelope, true) != nil ||
		envelope.TargetID != s.store.NodeID() {
		return ErrAuthentication
	}
	requestedAt := time.Unix(envelope.Timestamp, 0).UTC()
	if requestedAt.Before(now.Add(-v2RequestSkew)) ||
		requestedAt.After(now.Add(v2RequestSkew)) {
		return ErrAuthentication
	}
	if path == v2PairPath {
		if envelope.CodeID == "" {
			return ErrAuthentication
		}
	} else if envelope.CodeID != "" {
		return ErrAuthentication
	}
	return nil
}

func (s *Service) recordPairingFailureV2(codeID string, now time.Time) {
	credentialFile, _ := s.storeV2.RecordPairingFailure(codeID, now)
	if credentialFile != "" {
		_ = s.secretsV2.Delete(credentialFile)
	}
}

func validateV2PairResult(
	response v2PairResult,
	expectedNodeID string,
	expectedTransactionID string,
) error {
	if response.NodeID != expectedNodeID {
		return ErrIdentityMismatch
	}
	if response.FederationProtocol != FederationProtocolV2 ||
		response.TransactionID != expectedTransactionID {
		return ErrProtocolMismatch
	}
	if response.Scope != "" && response.Scope != SummaryScope && response.Scope != SummaryTerminalScope && response.Scope != SummaryTerminalFilesScope {
		return ErrProtocolMismatch
	}
	if cleanDisplayText(response.Hostname, 253) != response.Hostname ||
		cleanDisplayText(response.PanelVersion, 64) != response.PanelVersion {
		return &RemoteError{Code: "invalid_response"}
	}
	return nil
}

func normalizedV2Scope(scope string) string {
	if scope == SummaryTerminalScope || scope == SummaryTerminalFilesScope {
		return scope
	}
	return SummaryScope
}

func validateFederationSummaryV2(
	summary FederationSummary,
	expectedNodeID string,
	now time.Time,
) error {
	if summary.NodeID != expectedNodeID {
		return ErrIdentityMismatch
	}
	if summary.FederationProtocol != FederationProtocolV2 {
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

func sealV2JSONResponse(
	envelope v2Envelope,
	handshake *noise.HandshakeState,
	value any,
) (FederationEnvelopeV2, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > MaxSummaryBytes {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	return sealV2Response(envelope, handshake, payload)
}

func noiseKeyV2(credential v2Credential) noise.DHKey {
	return noise.DHKey{
		Private: append([]byte(nil), credential.ControllerPrivate...),
		Public:  append([]byte(nil), credential.ControllerPublic...),
	}
}

func nodeNoiseKeyV2(identity nodeIdentityV2) noise.DHKey {
	return noise.DHKey{
		Private: append([]byte(nil), identity.PrivateKey...),
		Public:  append([]byte(nil), identity.PublicKey...),
	}
}

func pairingCodeIDFromCredential(name string) string {
	if !validPairingCredentialNameV2(name) {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(name, "pair-"), ".v2key")
}

func v2CredentialsEqual(left, right v2Credential) bool {
	return bytes.Equal(left.ControllerPrivate, right.ControllerPrivate) &&
		bytes.Equal(left.ControllerPublic, right.ControllerPublic) &&
		bytes.Equal(left.TargetPublic, right.TargetPublic) &&
		bytes.Equal(left.PairingKey, right.PairingKey)
}

func containsControllerStateV2(
	states []controllerStateV2,
	target controllerStateV2,
) bool {
	for _, state := range states {
		if state == target {
			return true
		}
	}
	return false
}
