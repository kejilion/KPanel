package cluster

import (
	"bytes"
	"context"
	"encoding/json"
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

func (c *RemoteClient) BrowseFetchOpenV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseFetchRequest) (BrowseFetchOpenResponse, error) {
	var response BrowseFetchOpenResponse
	err := c.exchangeV2(ctx, origin, v2BrowseFetchOpenPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) BrowseFetchOutputV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseFetchOutputRequest) (BrowseFetchOutputResponse, error) {
	var response BrowseFetchOutputResponse
	err := c.exchangeV2(ctx, origin, v2BrowseFetchOutputPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) BrowseFetchCloseV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseFetchCloseRequest) error {
	var response struct {
		Closed bool `json:"closed"`
	}
	if err := c.exchangeV2(ctx, origin, v2BrowseFetchClosePath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Closed {
		return ErrAuthentication
	}
	return nil
}

func (c *RemoteClient) BrowseWSOpenV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseWSOpenRequest) (BrowseWSOpenResponse, error) {
	var response BrowseWSOpenResponse
	err := c.exchangeV2(ctx, origin, v2BrowseWSOpenPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) BrowseWSOutputV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseWSOutputRequest) (BrowseWSOutputResponse, error) {
	var response BrowseWSOutputResponse
	err := c.exchangeV2(ctx, origin, v2BrowseWSOutputPath, controllerID, targetID, "", key, target, nil, now, input, &response)
	return response, err
}

func (c *RemoteClient) BrowseWSInputV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseWSInputRequest) error {
	var response struct {
		Accepted bool `json:"accepted"`
	}
	if err := c.exchangeV2(ctx, origin, v2BrowseWSInputPath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Accepted {
		return ErrAuthentication
	}
	return nil
}

func (c *RemoteClient) BrowseWSCloseV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, target []byte, now time.Time, input BrowseWSCloseRequest) error {
	var response struct {
		Closed bool `json:"closed"`
	}
	if err := c.exchangeV2(ctx, origin, v2BrowseWSClosePath, controllerID, targetID, "", key, target, nil, now, input, &response); err != nil {
		return err
	}
	if !response.Closed {
		return ErrAuthentication
	}
	return nil
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
