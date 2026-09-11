package cluster

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

type HistoryAuthorization struct{ *FederationFileAuthorization }
type historyStreamMetadata struct {
	Status int `json:"status"`
}

func (s *Service) AuthorizeHistoryV2(source string, envelope FederationEnvelopeV2) (monitoring.Query, *HistoryAuthorization, error) {
	now := s.now().UTC()
	if !s.v2SourceLimiter.Allow(cleanRateSubject(source), now) {
		return monitoring.Query{}, nil, ErrRateLimited
	}
	if err := s.validateV2Request(HistoryV2Path, envelope, now); err != nil {
		return monitoring.Query{}, nil, err
	}
	controller, payload, handshake, err := s.openControllerV2(HistoryV2Path, envelope, now, controllerStateV2Active)
	if err != nil || !historyScopeAllowed(controller.Scope) {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	var query monitoring.Query
	if decodeV2Payload(payload, &query) != nil || query.Validate() != nil {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	release, ok := s.historyStreams.acquire(controller.ID)
	if !ok {
		return monitoring.Query{}, nil, monitoring.ErrBusy
	}
	return query, &HistoryAuthorization{&FederationFileAuthorization{request: envelope, handshake: handshake, release: release}}, nil
}

func (a *HistoryAuthorization) SealStatus(status int) (FederationEnvelopeV2, *noise.CipherState, error) {
	if a == nil || a.handshake == nil || status < 200 || status > 599 {
		return FederationEnvelopeV2{}, nil, ErrAuthentication
	}
	payload, err := json.Marshal(historyStreamMetadata{Status: status})
	if err != nil {
		return FederationEnvelopeV2{}, nil, err
	}
	message, _, cipher, err := a.handshake.WriteMessage(nil, payload)
	if err != nil || cipher == nil {
		return FederationEnvelopeV2{}, nil, ErrAuthentication
	}
	response := a.request
	response.Message = base64.RawURLEncoding.EncodeToString(message)
	return response, cipher, nil
}

func (c *RemoteClient) OpenHistoryV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, peer []byte, now time.Time, query monitoring.Query) (*http.Response, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	requestID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	envelope, handshake, err := sealV2Request(http.MethodPost, HistoryV2Path,
		v2Envelope{Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: targetID, Timestamp: now.Unix(), RequestID: requestID}, key, peer, nil, payload)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	request, err := c.newV2Request(ctx, http.MethodPost, origin, HistoryV2Path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", fileStreamContentType)
	response, err := c.historyClient.Do(request)
	if err != nil {
		return nil, classifyRemoteTransportError(err)
	}
	if err := historyResponseStatus(response.StatusCode); err != nil {
		response.Body.Close()
		return nil, err
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), fileStreamContentType) {
		response.Body.Close()
		return nil, ErrHistoryUnavailable
	}
	stream := newIdleReadCloser(response.Body, 30*time.Second)
	ok := false
	defer func() {
		if !ok {
			_ = stream.Close()
		}
	}()
	reader := bufio.NewReader(stream)
	header, err := readFileFrame(reader, maxV2EnvelopeBytes)
	if err != nil {
		return nil, err
	}
	var reply v2Envelope
	if decodeV2Payload(header, &reply) != nil || validateMatchingV2Response(envelope, reply) != nil {
		return nil, ErrAuthentication
	}
	message, err := decodeV2Message(reply.Message)
	if err != nil {
		return nil, err
	}
	plaintext, _, cipher, err := handshake.ReadMessage(nil, message)
	if err != nil || cipher == nil {
		return nil, ErrAuthentication
	}
	var metadata historyStreamMetadata
	if decodeV2Payload(plaintext, &metadata) != nil {
		return nil, ErrAuthentication
	}
	if err := historyResponseStatus(metadata.Status); err != nil {
		return nil, err
	}
	ok = true
	return &http.Response{StatusCode: metadata.Status, Header: http.Header{"Content-Type": {"application/json"}}, Body: &federationFileReader{source: reader, body: stream, cipher: cipher}}, nil
}

// V1's generic signature binds only the route. Bind the query digest into its
// signing path for this new endpoint; the actual HTTP route remains fixed.
func historyV1SignedRequest(request *http.Request) *http.Request {
	clone := request.Clone(request.Context())
	digest := sha256.Sum256([]byte(request.URL.RawQuery))
	clone.URL.Path = HistoryV1Path + "/" + hex.EncodeToString(digest[:])
	clone.URL.RawPath, clone.URL.RawQuery = "", ""
	return clone
}

func (c *RemoteClient) OpenHistoryV1(ctx context.Context, origin, controllerID, targetID string, key ed25519.PrivateKey, now time.Time, query monitoring.Query) (*http.Response, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	request, err := c.newRequest(ctx, http.MethodGet, origin, HistoryV1Path, http.NoBody)
	if err != nil {
		return nil, err
	}
	request.URL.RawQuery = query.Encode()
	nonce, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	signed := historyV1SignedRequest(request)
	if err := SignRequest(signed, controllerID, targetID, key, now, nonce); err != nil {
		return nil, err
	}
	request.Header = signed.Header
	response, err := c.historyClient.Do(request)
	if err != nil {
		return nil, classifyRemoteTransportError(err)
	}
	return response, nil
}

func (s *Service) AuthorizeHistoryV1(source string, request *http.Request) (monitoring.Query, func(), error) {
	if request == nil || request.Method != http.MethodGet || request.URL.Path != HistoryV1Path || request.URL.RawPath != "" || request.ContentLength != 0 || len(request.TransferEncoding) != 0 {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	query, err := monitoring.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return monitoring.Query{}, nil, err
	}
	now := s.now().UTC()
	if !s.v2SourceLimiter.Allow(cleanRateSubject(source), now) {
		return monitoring.Query{}, nil, ErrRateLimited
	}
	id := request.Header.Get(headerControllerID)
	controller, err := s.store.Controller(id)
	if err != nil || controller.Scope != SummaryScope {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	key, err := decodePublicKey(controller.PublicKey)
	if err != nil {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	verified, nonce, err := VerifyRequest(historyV1SignedRequest(request), s.store.NodeID(), key, now)
	if err != nil || verified != id {
		return monitoring.Query{}, nil, ErrAuthentication
	}
	if !s.requestLimiter.Allow(id, now) {
		return monitoring.Query{}, nil, ErrRateLimited
	}
	if err := s.replays.Accept("history:v1:"+id, nonce, now); err != nil {
		return monitoring.Query{}, nil, err
	}
	release, ok := s.historyStreams.acquire(id)
	if !ok {
		return monitoring.Query{}, nil, monitoring.ErrBusy
	}
	return query, release, nil
}

func (s *Service) handleHistoryRelayV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	record, err := s.light.Host(envelope.ControllerID)
	if err != nil || s.lightHistory == nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	key, err := s.light.ReadTerminalPublicKey(record)
	if err != nil || len(key) != 32 {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	payload, peer, handshake, err := openV2Request(http.MethodPost, HistoryRelayV2Path, envelope, nodeNoiseKeyV2(s.nodeIdentityV2), nil)
	if err != nil || !bytes.Equal(peer, key) {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	if !s.historyRelayRequests.Allow(record.ID, now) {
		return FederationEnvelopeV2{}, ErrRateLimited
	}
	if err := s.replays.Accept("light:history:"+record.ID, envelope.RequestID, now); err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input FileRelayPollRequest
	if decodeV2Payload(payload, &input) != nil || validateFileRelayPoll(input) != nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	response, err := s.lightHistory.poll(ctx, record.ID, input.RequestIDs, input.Events)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	// The relation may have been removed while the long poll was waiting.
	if _, err := s.light.Host(record.ID); err != nil {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	return sealV2JSONResponse(envelope, handshake, response)
}
