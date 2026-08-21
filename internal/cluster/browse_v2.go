package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/flynn/noise"
)

var ErrBrowseUnavailable = errors.New("cluster browse egress is unavailable")

// browseFetchSessionV2 buffers one already-completed fetch (see
// BrowseBackend — the fetch itself is never streamed, it is a single
// buffered request/response) so BrowseFetchOutputRequest can drain it in
// chunks small enough to fit the v2 federation envelope. Scoped to the
// controller that opened it: another controller presenting a guessed or
// reused session ID gets ErrNotFound, not the buffered bytes.
type browseFetchSessionV2 struct {
	controllerID string
	statusCode   int
	headers      map[string][]string
	body         []byte
	expiresAt    time.Time
}

const (
	// maxBrowseFetchSessionsV2 bounds how many fetch results this process
	// holds in memory at once. paneld's container has a 256MB limit (see
	// deploy/compose/compose.yml) and each session can hold up to the
	// Agent's own response cap (maxBrowseFetchResponseBytes, 8MiB) — kept
	// low deliberately so worst case (8 x 8MiB) stays a modest fraction of
	// that budget rather than competing with everything else paneld holds.
	maxBrowseFetchSessionsV2 = 8
	// browseFetchSessionV2TTL bounds how long an abandoned session (opened
	// but never fully drained or closed) lingers before a later Open sweeps
	// it away.
	browseFetchSessionV2TTL = 60 * time.Second
	// maxFederationBrowseFetchOutputBytes mirrors
	// maxFederationTerminalOutputBytes's reasoning: base64-encoded inside a
	// JSON payload, 32KiB of raw bytes stays comfortably under
	// MaxSummaryBytes (64KiB) alongside the rest of the envelope.
	maxFederationBrowseFetchOutputBytes = 32 << 10
)

func (s *Service) openBrowseFetchSessionV2(controllerID string, result BrowseFetchResult, now time.Time) (string, error) {
	s.browseFetchMu.Lock()
	defer s.browseFetchMu.Unlock()
	for id, session := range s.browseFetchSessions {
		if !session.expiresAt.After(now) {
			delete(s.browseFetchSessions, id)
		}
	}
	if len(s.browseFetchSessions) >= maxBrowseFetchSessionsV2 {
		return "", errors.New("cluster v2 browse fetch session limit reached")
	}
	id, err := randomHex(16)
	if err != nil {
		return "", err
	}
	s.browseFetchSessions[id] = browseFetchSessionV2{
		controllerID: controllerID,
		statusCode:   result.StatusCode,
		headers:      result.Headers,
		body:         result.Body,
		expiresAt:    now.Add(browseFetchSessionV2TTL),
	}
	return id, nil
}

func (s *Service) browseFetchSessionOutputV2(controllerID, sessionID string, offset int, now time.Time) (BrowseFetchOutputResponse, error) {
	s.browseFetchMu.Lock()
	defer s.browseFetchMu.Unlock()
	session, ok := s.browseFetchSessions[sessionID]
	if ok && !session.expiresAt.After(now) {
		// Genuinely expired: sweep it. A wrong-owner probe below must not
		// have this same destructive side effect on someone else's session.
		delete(s.browseFetchSessions, sessionID)
		ok = false
	}
	if !ok || session.controllerID != controllerID {
		return BrowseFetchOutputResponse{}, ErrNotFound
	}
	if offset < 0 || offset > len(session.body) {
		return BrowseFetchOutputResponse{}, ErrAuthentication
	}
	end := offset + maxFederationBrowseFetchOutputBytes
	if end > len(session.body) {
		end = len(session.body)
	}
	chunk := session.body[offset:end]
	done := end == len(session.body)
	if done {
		delete(s.browseFetchSessions, sessionID)
	} else {
		session.expiresAt = now.Add(browseFetchSessionV2TTL) // extend on activity
		s.browseFetchSessions[sessionID] = session
	}
	return BrowseFetchOutputResponse{
		Data:       base64.StdEncoding.EncodeToString(chunk),
		NextOffset: end,
		Done:       done,
	}, nil
}

func (s *Service) closeBrowseFetchSessionV2(controllerID, sessionID string) {
	s.browseFetchMu.Lock()
	defer s.browseFetchMu.Unlock()
	if session, ok := s.browseFetchSessions[sessionID]; ok && session.controllerID == controllerID {
		delete(s.browseFetchSessions, sessionID)
	}
}

func (s *Service) openBrowseControllerV2(path string, envelope v2Envelope, now time.Time) (controllerRecordV2, []byte, *noise.HandshakeState, error) {
	controller, payload, handshake, err := s.openControllerV2(path, envelope, now, controllerStateV2Active)
	if err != nil || !ScopeAllowsBrowse(controller.Scope) {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	return controller, payload, handshake, nil
}

func (s *Service) handleBrowseFetchOpenV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseControllerV2(v2BrowseFetchOpenPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseFetchRequest
	if decodeV2Payload(payload, &input) != nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.browse == nil {
		return FederationEnvelopeV2{}, ErrBrowseUnavailable
	}
	result, err := s.browse.Fetch(ctx, input)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	sessionID, err := s.openBrowseFetchSessionV2(controller.ID, result, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, BrowseFetchOpenResponse{
		SessionID: sessionID, StatusCode: result.StatusCode,
		Headers: result.Headers, TotalBytes: len(result.Body),
	})
}

func (s *Service) handleBrowseFetchOutputV2(_ context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseControllerV2(v2BrowseFetchOutputPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseFetchOutputRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || input.Offset < 0 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	output, err := s.browseFetchSessionOutputV2(controller.ID, input.SessionID, input.Offset, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, output)
}

func (s *Service) handleBrowseFetchCloseV2(_ context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseControllerV2(v2BrowseFetchClosePath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseFetchCloseRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	s.closeBrowseFetchSessionV2(controller.ID, input.SessionID)
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"closed": true})
}

func (s *Service) browseHostCredential(id string) (hostRecordV2, v2Credential, remoteV2BrowseAPI, error) {
	record, err := s.storeV2.Host(id)
	remote, ok := s.remoteV2.(remoteV2BrowseAPI)
	if err != nil || record.State != hostStateV2Active || !ScopeAllowsBrowse(normalizedV2Scope(record.Scope)) || !ok {
		return hostRecordV2{}, v2Credential{}, nil, ErrBrowseUnavailable
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return hostRecordV2{}, v2Credential{}, nil, err
	}
	return record, credential, remote, nil
}

// BrowseFetchOpen, BrowseFetchOutput and BrowseFetchClose are the
// controller-role entry points internal/panel calls for a non-local hostID —
// the counterparts to handleBrowseFetchOpenV2/OutputV2/CloseV2 above, which
// run on the host actually performing the fetch.
func (s *Service) BrowseFetchOpen(ctx context.Context, hostID string, input BrowseFetchRequest) (BrowseFetchOpenResponse, error) {
	record, credential, remote, err := s.browseHostCredential(hostID)
	if err != nil {
		return BrowseFetchOpenResponse{}, err
	}
	return remote.BrowseFetchOpenV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) BrowseFetchOutput(ctx context.Context, hostID string, input BrowseFetchOutputRequest) (BrowseFetchOutputResponse, error) {
	record, credential, remote, err := s.browseHostCredential(hostID)
	if err != nil {
		return BrowseFetchOutputResponse{}, err
	}
	return remote.BrowseFetchOutputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) BrowseFetchClose(ctx context.Context, hostID string, input BrowseFetchCloseRequest) error {
	record, credential, remote, err := s.browseHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.BrowseFetchCloseV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}
