package cluster

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/flynn/noise"
)

type remoteV2FilePeerAPI interface {
	LinkFilePeerV2(
		context.Context, string, string, string, noise.DHKey, []byte,
		time.Time, v2FilePeerLinkRequest,
	) (v2FilePeerLinkResult, error)
}

func (s *Service) EnableMutualFileTransfer(
	ctx context.Context,
	hostID string,
	controllerOrigin string,
) (Host, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.enableFilePeerV2Locked(ctx, hostID, controllerOrigin); err != nil {
		return Host{}, err
	}
	return s.Host(ctx, hostID)
}

func (s *Service) enableFilePeerV2Locked(
	ctx context.Context,
	hostID string,
	controllerOrigin string,
) error {
	record, err := s.storeV2.Host(hostID)
	if err != nil {
		return err
	}
	if record.State != hostStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(record.Scope)) {
		return ErrProtocolMismatch
	}
	if controllerOrigin == "" {
		controllerOrigin = s.publicURL
	}
	controllerOrigin, err = NormalizeV2Origin(controllerOrigin)
	if err != nil {
		return err
	}
	grant, err := s.filePeersV2.PrepareGrant(record, controllerOrigin, s.now().UTC())
	if err != nil {
		return err
	}
	if err := s.syncFilePeerV2(ctx, record, grant); err != nil {
		return err
	}
	s.mu.Lock()
	current := s.runtime[hostID]
	current.nextFilePeerSyncAt = s.now().UTC().Add(filePeerSyncInterval)
	s.runtime[hostID] = current
	s.mu.Unlock()
	return nil
}

func (s *Service) handleFilePeerLinkV2(
	ctx context.Context,
	envelope v2Envelope,
	now time.Time,
) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openControllerV2(
		v2FileLinkPath,
		envelope,
		now,
		controllerStateV2Active,
	)
	if err != nil || !ScopeAllowsFiles(normalizedV2Scope(controller.Scope)) {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	var input v2FilePeerLinkRequest
	if err := decodeV2Payload(payload, &input); err != nil ||
		!validID(input.LinkID) || !validID(input.NodeID) ||
		input.NodeID == s.store.NodeID() {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	origin, err := NormalizeV2Origin(input.Origin)
	if err != nil || origin != input.Origin {
		return FederationEnvelopeV2{}, ErrInvalidOrigin
	}
	if validator, ok := s.remoteV2.(v2OriginValidator); ok {
		validated, validateErr := validator.ValidateV2Origin(ctx, origin)
		if validateErr != nil {
			return FederationEnvelopeV2{}, validateErr
		}
		if validated != origin {
			return FederationEnvelopeV2{}, ErrInvalidOrigin
		}
	}
	// Clear routes whose parent Controller has already been revoked before
	// enforcing per-peer uniqueness. This lets a legitimate re-pair recover as
	// soon as storage is writable instead of waiting for the old lease to expire.
	if err := s.filePeersV2.Reconcile(
		s.storeV2.Controllers(), s.storeV2.Hosts(), now,
	); err != nil {
		return FederationEnvelopeV2{}, err
	}
	if _, err := s.filePeersV2.GrantRoute(
		controller,
		input.LinkID,
		input.NodeID,
		origin,
		now,
	); err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, v2FilePeerLinkResult{
		LinkID: input.LinkID,
		NodeID: s.store.NodeID(),
		Linked: true,
	})
}

func (s *Service) syncFilePeerV2(
	ctx context.Context,
	record hostRecordV2,
	grant filePeerGrantV2,
) error {
	remote, ok := s.remoteV2.(remoteV2FilePeerAPI)
	if !ok || record.State != hostStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(record.Scope)) ||
		grant.HostID != record.ID || grant.HostControllerID != record.ControllerID ||
		grant.HostTransaction != record.TransactionID ||
		grant.PeerNodeID != record.RemoteNodeID ||
		grant.PeerFingerprint != record.PeerFingerprint {
		return ErrProtocolMismatch
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return err
	}
	result, err := remote.LinkFilePeerV2(
		ctx,
		record.Origin,
		record.ControllerID,
		record.RemoteNodeID,
		noiseKeyV2(credential),
		credential.TargetPublic,
		s.now().UTC(),
		v2FilePeerLinkRequest{
			LinkID: grant.LinkID,
			NodeID: s.store.NodeID(),
			Origin: grant.LocalOrigin,
		},
	)
	if err != nil {
		// A missing endpoint is a stable mixed-version result. Remove only a
		// never-activated intent so periodic polling does not keep generating
		// unsupported requests; an administrator can retry after upgrading.
		if grant.State == filePeerGrantPending && errors.Is(err, ErrMutualFilesUnsupported) {
			_ = s.deleteFilePeerGrant(record.ID)
		}
		return err
	}
	if !result.Linked || result.LinkID != grant.LinkID ||
		result.NodeID != record.RemoteNodeID {
		return ErrAuthentication
	}
	_, err = s.filePeersV2.ActivateGrant(grant.LinkID, s.now().UTC())
	return err
}

func (s *Service) prepareFilePeerGrantForSync(
	record hostRecordV2,
	grant filePeerGrantV2,
) (filePeerGrantV2, error) {
	origin := grant.LocalOrigin
	if s.publicURL != "" {
		normalized, err := NormalizeV2Origin(s.publicURL)
		if err != nil {
			return filePeerGrantV2{}, err
		}
		origin = normalized
	}
	return s.filePeersV2.PrepareGrant(record, origin, s.now().UTC())
}

func (c *RemoteClient) LinkFilePeerV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input v2FilePeerLinkRequest,
) (v2FilePeerLinkResult, error) {
	var response v2FilePeerLinkResult
	err := c.exchangeV2(
		ctx,
		origin,
		v2FileLinkPath,
		controllerID,
		targetID,
		"",
		controllerKey,
		targetPublicKey,
		nil,
		now,
		input,
		&response,
	)
	var remoteError *RemoteError
	if errors.As(err, &remoteError) &&
		(remoteError.StatusCode == http.StatusNotFound ||
			remoteError.StatusCode == http.StatusMethodNotAllowed ||
			remoteError.StatusCode == http.StatusUpgradeRequired) {
		return v2FilePeerLinkResult{}, ErrMutualFilesUnsupported
	}
	return response, err
}

func (s *Service) hasActiveFilePeerGrant(hostID string, now time.Time) bool {
	_, err := s.filePeersV2.ActiveGrantByHost(hostID, now)
	return err == nil
}

func (s *Service) deleteFilePeerGrant(hostID string) error {
	err := s.filePeersV2.DeleteHost(hostID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
