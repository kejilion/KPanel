package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
)

// FileRelayClient is the lightweight node side of the outbound file-manager
// relay. It shares the v2 Noise identity and poll transport with the terminal
// broker, but has an independent endpoint and command validation surface.
type FileRelayClient struct {
	client *http.Client
}

func NewFileRelayClient(client *http.Client) (*FileRelayClient, error) {
	if client == nil {
		return nil, errors.New("file relay HTTP client is required")
	}
	return &FileRelayClient{client: client}, nil
}

func (c *FileRelayClient) PollV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input FileRelayPollRequest,
) (FileRelayPollResponse, error) {
	var output FileRelayPollResponse
	if c == nil || c.client == nil {
		return output, ErrAuthentication
	}
	normalized, err := validateLightOrigin(origin)
	if err != nil || normalized != origin {
		return output, ErrInvalidOrigin
	}
	if err := validateFileRelayPoll(input); err != nil || !fileRelayPayloadFits(input) {
		return output, ErrAuthentication
	}
	payload, err := json.Marshal(input)
	if err != nil || len(payload) > MaxSummaryBytes {
		return output, ErrAuthentication
	}
	requestID, err := randomHex(16)
	if err != nil {
		return output, err
	}
	envelope, handshake, err := sealV2Request(
		http.MethodPost, v2FileRelayPath,
		v2Envelope{
			Protocol: FederationProtocolV2, ControllerID: controllerID,
			TargetID: targetID, Timestamp: now.UTC().Unix(), RequestID: requestID,
		}, controllerKey, targetPublicKey, nil, payload,
	)
	if err != nil {
		return output, err
	}
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxV2EnvelopeBytes {
		return output, ErrAuthentication
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, origin+v2FileRelayPath, bytes.NewReader(body),
	)
	if err != nil {
		return output, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KPanel federation/"+FederationProtocolV2)
	response, err := c.client.Do(request)
	if err != nil {
		return output, classifyRemoteTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		code := "remote_error"
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "authentication_failed"
		case http.StatusTooManyRequests:
			code = "rate_limited"
		case http.StatusUpgradeRequired:
			code = "protocol_incompatible"
		}
		return output, &RemoteError{Code: code, StatusCode: response.StatusCode}
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.HasPrefix(mediaType, "application/json") {
		return output, &RemoteError{Code: "invalid_response"}
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxV2EnvelopeBytes+1))
	if err != nil || int64(len(content)) > maxV2EnvelopeBytes {
		return output, &RemoteError{Code: "invalid_response"}
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var responseEnvelope v2Envelope
	if err := decoder.Decode(&responseEnvelope); err != nil {
		return output, &RemoteError{Code: "invalid_response"}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return output, &RemoteError{Code: "invalid_response"}
	}
	plaintext, err := openV2Response(envelope, responseEnvelope, handshake)
	if err != nil || len(plaintext) > MaxSummaryBytes || decodeV2Payload(plaintext, &output) != nil {
		return output, ErrAuthentication
	}
	if output.Epoch != "" && !validID(output.Epoch) {
		return output, ErrAuthentication
	}
	if output.Command != nil && validateFileRelayCommand(*output.Command, now.UTC()) != nil {
		return output, ErrAuthentication
	}
	return output, nil
}
