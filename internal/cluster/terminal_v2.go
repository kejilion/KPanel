package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

var ErrTerminalUnavailable = errors.New("cluster terminal is unavailable")

// Keep the JSON-encoded PTY bytes, Noise ciphertext and outer base64 envelope
// below MaxFederationV2Bytes. Local Agent reads may use the larger terminal
// chunk; federation advances through the remaining buffered output on the next
// offset request without dropping data.
const maxFederationTerminalOutputBytes = 32 << 10

func (s *Service) handleTerminalOpenV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openTerminalControllerV2(v2TerminalOpenPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalOpenRequest
	if decodeV2Payload(payload, &input) != nil || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.terminal == nil {
		return FederationEnvelopeV2{}, ErrTerminalUnavailable
	}
	snapshot, err := s.terminal.Open(ctx, "federation:"+controller.ID, input.Rows, input.Columns)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	_ = s.storeV2.TouchController(controller.ID, now)
	return sealV2JSONResponse(envelope, handshake, TerminalOpenResponse{SessionID: snapshot.ID, Offset: snapshot.Offset, CreatedAt: snapshot.CreatedAt})
}

func (s *Service) handleTerminalOutputV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openTerminalControllerV2(v2TerminalOutputPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalOutputRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || input.Offset < 0 || input.Wait < 0 || input.Wait > 1000 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.terminal == nil {
		return FederationEnvelopeV2{}, ErrTerminalUnavailable
	}
	output, err := s.terminal.Output(ctx, "federation:"+controller.ID, input.SessionID, input.Offset, time.Duration(input.Wait)*time.Millisecond)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	if len(output.Data) > maxFederationTerminalOutputBytes {
		output.Data = output.Data[:maxFederationTerminalOutputBytes]
		output.NextOffset = output.Offset + int64(len(output.Data))
	}
	return sealV2JSONResponse(envelope, handshake, output)
}

func (s *Service) handleTerminalInputV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openTerminalControllerV2(v2TerminalInputPath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalInputRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	data, err := decodeTerminalPayload(input.Data)
	if err != nil || len(data) == 0 || len(data) > terminal.MaxInputBytes {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.terminal == nil {
		return FederationEnvelopeV2{}, ErrTerminalUnavailable
	}
	if err := s.terminal.Input(ctx, "federation:"+controller.ID, input.SessionID, data); err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"accepted": true})
}

func decodeTerminalPayload(value string) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func (s *Service) handleTerminalResizeV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openTerminalControllerV2(v2TerminalResizePath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalResizeRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" || input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.terminal == nil {
		return FederationEnvelopeV2{}, ErrTerminalUnavailable
	}
	if err := s.terminal.Resize(ctx, "federation:"+controller.ID, input.SessionID, input.Rows, input.Columns); err != nil {
		return FederationEnvelopeV2{}, err
	}
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"accepted": true})
}

func (s *Service) handleTerminalCloseV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openTerminalControllerV2(v2TerminalClosePath, envelope, now)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input TerminalCloseRequest
	if decodeV2Payload(payload, &input) != nil || input.SessionID == "" {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if s.terminal == nil {
		return FederationEnvelopeV2{}, ErrTerminalUnavailable
	}
	if err := s.terminal.Close(ctx, "federation:"+controller.ID, input.SessionID); err != nil {
		return FederationEnvelopeV2{}, err
	}
	_ = s.storeV2.TouchController(controller.ID, now)
	return sealV2JSONResponse(envelope, handshake, map[string]bool{"closed": true})
}

func (s *Service) openTerminalControllerV2(path string, envelope v2Envelope, now time.Time) (controllerRecordV2, []byte, *noise.HandshakeState, error) {
	controller, payload, handshake, err := s.openControllerV2(path, envelope, now, controllerStateV2Active)
	if err != nil || !ScopeAllowsTerminal(controller.Scope) {
		return controllerRecordV2{}, nil, nil, ErrAuthentication
	}
	return controller, payload, handshake, nil
}

func (s *Service) terminalHostCredential(id string) (hostRecordV2, v2Credential, remoteV2TerminalAPI, error) {
	record, err := s.storeV2.Host(id)
	remote, ok := s.remoteV2.(remoteV2TerminalAPI)
	if err != nil || record.State != hostStateV2Active || !ScopeAllowsTerminal(normalizedV2Scope(record.Scope)) || !ok {
		return hostRecordV2{}, v2Credential{}, nil, ErrTerminalUnavailable
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return hostRecordV2{}, v2Credential{}, nil, err
	}
	return record, credential, remote, nil
}

func (s *Service) TerminalOpen(ctx context.Context, hostID string, input TerminalOpenRequest) (TerminalOpenResponse, error) {
	if _, err := s.light.Host(hostID); err == nil {
		if s.lightTerminal == nil {
			return TerminalOpenResponse{}, ErrTerminalUnavailable
		}
		if input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
			return TerminalOpenResponse{}, errors.New("invalid terminal dimensions")
		}
		return s.lightTerminal.open(ctx, hostID, input.Rows, input.Columns)
	}
	record, credential, remote, err := s.terminalHostCredential(hostID)
	if err != nil {
		return TerminalOpenResponse{}, err
	}
	return remote.TerminalOpenV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) TerminalOutput(ctx context.Context, hostID string, input TerminalOutputRequest) (terminal.Output, error) {
	if _, err := s.light.Host(hostID); err == nil {
		if s.lightTerminal == nil {
			return terminal.Output{}, ErrTerminalUnavailable
		}
		return s.lightTerminal.output(ctx, hostID, input)
	}
	record, credential, remote, err := s.terminalHostCredential(hostID)
	if err != nil {
		return terminal.Output{}, err
	}
	return remote.TerminalOutputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) TerminalInput(ctx context.Context, hostID string, input TerminalInputRequest) error {
	if _, err := s.light.Host(hostID); err == nil {
		if s.lightTerminal == nil {
			return ErrTerminalUnavailable
		}
		return s.lightTerminal.input(ctx, hostID, input)
	}
	record, credential, remote, err := s.terminalHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.TerminalInputV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) TerminalResize(ctx context.Context, hostID string, input TerminalResizeRequest) error {
	if _, err := s.light.Host(hostID); err == nil {
		if s.lightTerminal == nil {
			return ErrTerminalUnavailable
		}
		return s.lightTerminal.resize(ctx, hostID, input)
	}
	record, credential, remote, err := s.terminalHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.TerminalResizeV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}

func (s *Service) TerminalClose(ctx context.Context, hostID string, input TerminalCloseRequest) error {
	if _, err := s.light.Host(hostID); err == nil {
		if s.lightTerminal == nil {
			return ErrTerminalUnavailable
		}
		return s.lightTerminal.close(ctx, hostID, input)
	}
	record, credential, remote, err := s.terminalHostCredential(hostID)
	if err != nil {
		return err
	}
	return remote.TerminalCloseV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}
