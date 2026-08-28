package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func NormalizeV2Origin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https://") {
		return NormalizeOrigin(raw)
	}
	if raw == "" || len(raw) > 512 || strings.ContainsAny(raw, "\r\n\t\x00") {
		return "", ErrInvalidOrigin
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.ForceQuery {
		return "", ErrInvalidOrigin
	}
	if strings.ContainsAny(parsed.Host, "%/\\?#@,; ") {
		return "", ErrInvalidOrigin
	}
	port := parsed.Port()
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || number == 80 {
		return "", ErrInvalidOrigin
	}
	address, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || address.Zone() != "" || address.Is4In6() {
		return "", ErrInvalidOrigin
	}
	address = address.Unmap()
	host := address.String()
	if address.Is6() {
		host = "[" + host + "]"
	}
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), port), nil
}

func (c *RemoteClient) ValidateV2Origin(
	ctx context.Context,
	origin string,
) (string, error) {
	normalized, err := NormalizeV2Origin(origin)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", ErrInvalidOrigin
	}
	if _, err := c.resolve(ctx, parsed.Hostname()); err != nil {
		return "", err
	}
	return normalized, nil
}

func (c *RemoteClient) PairV2(
	ctx context.Context,
	origin string,
	controllerID string,
	controllerName string,
	transactionID string,
	controllerKey noise.DHKey,
	pairing v2PairingDescriptor,
	now time.Time,
) (v2PairResult, error) {
	var response v2PairResult
	err := c.exchangeV2(
		ctx, origin, v2PairPath, controllerID, pairing.NodeID, pairing.CodeID,
		controllerKey, pairing.TargetPublicKey, pairing.PairingKey, now,
		v2PairPayload{
			ControllerName: controllerName,
			TransactionID:  transactionID,
		}, &response,
	)
	return response, err
}

func (c *RemoteClient) CommitV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	transactionID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
) (v2CommitResult, error) {
	var response v2CommitResult
	err := c.exchangeV2(
		ctx, origin, v2CommitPath, controllerID, targetID, "",
		controllerKey, targetPublicKey, nil, now,
		v2CommitPayload{TransactionID: transactionID}, &response,
	)
	return response, err
}

func (c *RemoteClient) SummaryV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
) (FederationSummary, error) {
	var response FederationSummary
	err := c.exchangeV2(
		ctx, origin, v2SummaryPath, controllerID, targetID, "",
		controllerKey, targetPublicKey, nil, now,
		struct{}{}, &response,
	)
	return response, err
}

func (c *RemoteClient) RevokeV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
) error {
	var response v2RevokeResult
	if err := c.exchangeV2(
		ctx, origin, v2RevokePath, controllerID, targetID, "",
		controllerKey, targetPublicKey, nil, now,
		struct{}{}, &response,
	); err != nil {
		return err
	}
	if !response.Revoked {
		return ErrAuthentication
	}
	return nil
}

func (c *RemoteClient) TerminalOpenV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input TerminalOpenRequest) (TerminalOpenResponse, error) {
	var response TerminalOpenResponse
	err := c.exchangeV2(ctx, origin, v2TerminalOpenPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) TerminalOutputV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input TerminalOutputRequest) (terminal.Output, error) {
	var response terminal.Output
	err := c.exchangeV2(ctx, origin, v2TerminalOutputPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) TerminalInputV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input TerminalInputRequest) error {
	var response struct {
		Accepted bool `json:"accepted"`
	}
	if err := c.exchangeV2(ctx, origin, v2TerminalInputPath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Accepted {
		return ErrAuthentication
	}
	return nil
}

func (c *RemoteClient) TerminalResizeV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input TerminalResizeRequest) error {
	var response struct {
		Accepted bool `json:"accepted"`
	}
	if err := c.exchangeV2(ctx, origin, v2TerminalResizePath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Accepted {
		return ErrAuthentication
	}
	return nil
}

func (c *RemoteClient) TerminalCloseV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input TerminalCloseRequest) error {
	var response struct {
		Closed bool `json:"closed"`
	}
	if err := c.exchangeV2(ctx, origin, v2TerminalClosePath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Closed {
		return ErrAuthentication
	}
	return nil
}

// TerminalRelayClient is the light-node side of the v2 reverse terminal
// transport. It intentionally accepts the already constrained HTTP client
// owned by the node process: the origin comes from the root-owned enrollment
// configuration, while all terminal payloads still use the shared v2 Noise
// envelope and response validation.
type TerminalRelayClient struct {
	client *http.Client
}

func NewTerminalRelayClient(client *http.Client) (*TerminalRelayClient, error) {
	if client == nil {
		return nil, errors.New("terminal relay HTTP client is required")
	}
	return &TerminalRelayClient{client: client}, nil
}

func (c *TerminalRelayClient) PollV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input TerminalRelayPollRequest,
) (TerminalRelayPollResponse, error) {
	var output TerminalRelayPollResponse
	if c == nil || c.client == nil {
		return output, ErrAuthentication
	}
	normalized, err := validateLightOrigin(origin)
	if err != nil || normalized != origin {
		return output, ErrInvalidOrigin
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
		http.MethodPost, v2TerminalRelayPath,
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
		ctx, http.MethodPost, origin+v2TerminalRelayPath, bytes.NewReader(body),
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
	if output.Command != nil && validateTerminalRelayCommand(*output.Command, now.UTC()) != nil {
		return output, ErrAuthentication
	}
	return output, nil
}

func (c *RemoteClient) exchangeV2(
	ctx context.Context,
	origin string,
	path string,
	controllerID string,
	targetID string,
	codeID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	pairingKey []byte,
	now time.Time,
	input any,
	output any,
) error {
	if !v2PathAllowed(http.MethodPost, path) || output == nil {
		return ErrAuthentication
	}
	payload, err := json.Marshal(input)
	if err != nil || len(payload) > MaxSummaryBytes {
		return ErrAuthentication
	}
	requestID, err := randomHex(16)
	if err != nil {
		return err
	}
	envelope, handshake, err := sealV2Request(
		http.MethodPost,
		path,
		v2Envelope{
			Protocol: FederationProtocolV2, ControllerID: controllerID,
			TargetID: targetID, CodeID: codeID,
			Timestamp: now.UTC().Unix(), RequestID: requestID,
		},
		controllerKey,
		targetPublicKey,
		pairingKey,
		payload,
	)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxV2EnvelopeBytes {
		return ErrAuthentication
	}
	request, err := c.newV2Request(
		ctx, http.MethodPost, origin, path, bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	if path == v2SummaryPath {
		request.Header.Set(FederationCapabilitiesHeader, SecurityEntrancePathCapability)
	}
	var response v2Envelope
	if err := c.doJSON(request, maxV2EnvelopeBytes, &response); err != nil {
		return err
	}
	plaintext, err := openV2Response(envelope, response, handshake)
	if err != nil || len(plaintext) > MaxSummaryBytes {
		return ErrAuthentication
	}
	if err := decodeV2Payload(plaintext, output); err != nil {
		return &RemoteError{Code: "invalid_response"}
	}
	return nil
}

func (c *RemoteClient) newV2Request(
	ctx context.Context,
	method string,
	origin string,
	path string,
	body io.Reader,
) (*http.Request, error) {
	normalized, err := NormalizeV2Origin(origin)
	if err != nil || normalized != origin || !v2PathAllowed(method, path) {
		return nil, ErrInvalidOrigin
	}
	request, err := http.NewRequestWithContext(ctx, method, origin+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KPanel federation/"+FederationProtocolV2)
	return request, nil
}
