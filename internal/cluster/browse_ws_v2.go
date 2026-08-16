package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/flynn/noise"
)

var ErrBrowseWSUnavailable = errors.New("cluster browse ws egress is unavailable")

const (
	// maxFederationBrowseWSOutputBytes mirrors maxFederationBrowseFetchOutputBytes:
	// base64-encoded inside a JSON payload, staying under this keeps the
	// whole envelope comfortably under MaxSummaryBytes (64KiB).
	maxFederationBrowseWSOutputBytes = 32 << 10
	// maxFederationBrowseWSMessageBytes bounds a single WS message that can
	// cross federation at all — unlike browse fetch's byte-range chunking,
	// a WS message is an indivisible unit (splitting it would change its
	// meaning to the target site's application protocol), so a message
	// larger than one federation round trip can hold cannot be chunked the
	// way a fetch body can. Local (non-federated) sessions are not subject
	// to this — only internal/agent's own maxBrowseWSMessageBytes (1MiB)
	// applies there. A message over this cap sent by the target is dropped
	// (not delivered, offset advances past it) rather than wedging the poll
	// loop forever trying to fit it; input the controller tries to send
	// over this cap is rejected outright so the caller knows immediately.
	maxFederationBrowseWSMessageBytes = 40 << 10
)

func (s *Service) openBrowseWSControllerV2(path string, envelope v2Envelope, now time.Time) (controllerRecordV2, []byte, *noise.HandshakeState, error) {
	controller, payload, handshake, err := s.openControllerV2(path, envelope, now, controllerStateV2Active)
	if err != nil || !ScopeAllowsBrowseWS(controller.Scope) {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	return controller, payload, handshake, nil
}

func (s *Service) handleBrowseWSOpenV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseWSControllerV2(v2BrowseWSOpenPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseWSOpenRequest
	if decodeV2Payload(payload, &input) != nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.browseWS == nil {
		return FederationEnvelopeV2{}, ErrBrowseWSUnavailable
	}
	id, err := s.browseWS.Open(ctx, "federation:"+controller.ID, input.URL, input.Headers)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, BrowseWSOpenResponse{SessionID: id})
}

func (s *Service) handleBrowseWSOutputV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseWSControllerV2(v2BrowseWSOutputPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseWSOutputRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || input.Offset < 0 || input.Wait < 0 || input.Wait > 1500 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.browseWS == nil {
		return FederationEnvelopeV2{}, ErrBrowseWSUnavailable
	}
	frames, nextOffset, closed, reason, err := s.browseWS.Output(
		ctx, "federation:"+controller.ID, input.SessionID, input.Offset, time.Duration(input.Wait)*time.Millisecond,
	)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	messages, truncatedNext := truncateBrowseWSFrames(frames, nextOffset)
	// Only report closed once every buffered frame has actually been
	// delivered — otherwise a controller that sees closed:true would stop
	// polling before draining the tail this call had to leave for next time.
	reportClosed := closed && truncatedNext == nextOffset
	reportReason := ""
	if reportClosed {
		reportReason = reason
	}
	return sealV2JSONResponse(envelope, handshake, BrowseWSOutputResponse{
		Messages: messages, NextOffset: truncatedNext, Closed: reportClosed, CloseReason: reportReason,
	})
}

func (s *Service) handleBrowseWSInputV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseWSControllerV2(v2BrowseWSInputPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseWSInputRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || (input.Type != "text" && input.Type != "binary") {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil || len(data) > maxFederationBrowseWSMessageBytes {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.browseWS == nil {
		return FederationEnvelopeV2{}, ErrBrowseWSUnavailable
	}
	if err := s.browseWS.Input(ctx, "federation:"+controller.ID, input.SessionID, input.Type == "binary", data); err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"accepted": true})
}

func (s *Service) handleBrowseWSCloseV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openBrowseWSControllerV2(v2BrowseWSClosePath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input BrowseWSCloseRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.browseWS == nil {
		return FederationEnvelopeV2{}, ErrBrowseWSUnavailable
	}
	_ = s.browseWS.Close(ctx, "federation:"+controller.ID, input.SessionID)
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"closed": true})
}

// truncateBrowseWSFrames selects a prefix of frames that fits within
// maxFederationBrowseWSOutputBytes once base64-encoded, dropping (not
// deferring) any single frame that alone exceeds
// maxFederationBrowseWSMessageBytes — see that constant's doc comment.
// fullNextOffset is what the backend reported for all of frames; the
// returned offset accounts for exactly what was actually included so the
// next poll picks up precisely where this one left off (whether that's
// because of a normal budget cut or a dropped oversized frame).
func truncateBrowseWSFrames(frames []BrowseWSFrame, fullNextOffset int) ([]BrowseWSMessage, int) {
	var size int
	messages := make([]BrowseWSMessage, 0, len(frames))
	included := 0
	for _, frame := range frames {
		if len(frame.Data) > maxFederationBrowseWSMessageBytes {
			// Too big to ever cross federation in one piece: skip it,
			// still counts as "included" so the offset advances past it.
			included++
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(frame.Data)
		const perEntryOverhead = 32 // {"type":"binary","data":""} structural bytes
		entrySize := len(encoded) + perEntryOverhead
		if len(messages) > 0 && size+entrySize > maxFederationBrowseWSOutputBytes {
			break
		}
		typ := "text"
		if frame.Binary {
			typ = "binary"
		}
		messages = append(messages, BrowseWSMessage{Type: typ, Data: encoded})
		size += entrySize
		included++
	}
	return messages, fullNextOffset - (len(frames) - included)
}

func (s *Service) browseWSHostCredential(id string) (hostRecordV2, v2Credential, remoteV2BrowseWSAPI, error) {
	record, err := s.storeV2.Host(id)
	remote, ok := s.remoteV2.(remoteV2BrowseWSAPI)
	if err != nil || record.State != hostStateV2Active || !ScopeAllowsBrowseWS(normalizedV2Scope(record.Scope)) || !ok {
		return hostRecordV2{}, v2Credential{}, nil, ErrBrowseWSUnavailable
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return hostRecordV2{}, v2Credential{}, nil, err
	}
	return record, credential, remote, nil
}

// BrowseWSOpen, BrowseWSOutput, BrowseWSInput and BrowseWSClose are the
// controller-role entry points internal/panel calls for a non-local hostID.
func (s *Service) BrowseWSOpen(ctx context.Context, hostID string, input BrowseWSOpenRequest) (BrowseWSOpenResponse, error) {
	record, credential, remote, err := s.browseWSHostCredential(hostID)
	if err != nil {
		return BrowseWSOpenResponse{}, err
	}
	return remote.BrowseWSOpenV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) BrowseWSOutput(ctx context.Context, hostID string, input BrowseWSOutputRequest) (BrowseWSOutputResponse, error) {
	record, credential, remote, err := s.browseWSHostCredential(hostID)
	if err != nil {
		return BrowseWSOutputResponse{}, err
	}
	return remote.BrowseWSOutputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) BrowseWSInput(ctx context.Context, hostID string, input BrowseWSInputRequest) error {
	record, credential, remote, err := s.browseWSHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.BrowseWSInputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) BrowseWSClose(ctx context.Context, hostID string, input BrowseWSCloseRequest) error {
	record, credential, remote, err := s.browseWSHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.BrowseWSCloseV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}
