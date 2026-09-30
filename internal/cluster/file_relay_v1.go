package cluster

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	// FileRelayV1SignedCapability is advertised only to a v1 controller that
	// this target has explicitly granted file management. It replaces the
	// earlier unconditional file-relay-v1 hint, which older targets still send.
	FileRelayV1SignedCapability = "file-relay-v1-signed"
	FileRelayV1SignatureHeader  = "X-KPanel-File-Signature"
)

// fileRelayV1Canonical binds the inner file operation to the outer v1 request
// signature. The outer canonical form predates the relay and covers only the
// fixed relay path, so without this binding a party able to modify a request
// in transit could turn an authorized read into a write.
func fileRelayV1Canonical(controllerID, targetID, timestamp, nonce string, input LightFileRequest) string {
	return strings.Join([]string{
		"KPanel federation file relay v1",
		http.MethodPost, FileRelayV1Path, controllerID, targetID, timestamp, nonce,
		strings.ToUpper(input.Method), input.Path, input.RawQuery,
	}, "\n")
}

func signFileRelayV1(request *http.Request, privateKey ed25519.PrivateKey, input LightFileRequest) error {
	if request == nil || len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("federation file relay signing input is invalid")
	}
	canonical := fileRelayV1Canonical(
		request.Header.Get(headerControllerID), request.Header.Get(headerTargetID),
		request.Header.Get(headerTimestamp), request.Header.Get(headerNonce), input,
	)
	request.Header.Set(FileRelayV1SignatureHeader, base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(canonical))))
	return nil
}

func verifyFileRelayV1Binding(request *http.Request, publicKey ed25519.PublicKey, input LightFileRequest) error {
	values := request.Header.Values(FileRelayV1SignatureHeader)
	if len(values) != 1 || len(values[0]) > 128 || len(publicKey) != ed25519.PublicKeySize {
		return ErrAuthentication
	}
	signature, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(values[0]))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrAuthentication
	}
	canonical := fileRelayV1Canonical(
		strings.TrimSpace(request.Header.Get(headerControllerID)), strings.TrimSpace(request.Header.Get(headerTargetID)),
		strings.TrimSpace(request.Header.Get(headerTimestamp)), strings.TrimSpace(request.Header.Get(headerNonce)), input,
	)
	if !ed25519.Verify(publicKey, []byte(canonical), signature) {
		return ErrAuthentication
	}
	return nil
}

// FileRelayV1RequestFromHTTP decodes the fixed v1 federation envelope into
// the same constrained file request used by the lightweight and v2 relays.
// The outer request is authenticated separately; this function only accepts
// the file-manager allowlist and the explicitly forwarded headers.
func FileRelayV1RequestFromHTTP(request *http.Request) (LightFileRequest, error) {
	if request == nil || request.URL == nil ||
		request.Method != http.MethodPost || request.URL.Path != FileRelayV1Path ||
		request.URL.RawPath != "" || request.URL.RawQuery != "" {
		return LightFileRequest{}, ErrAuthentication
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	input := LightFileRequest{
		Method:     strings.TrimSpace(request.Header.Get(FileRelayV1MethodHeader)),
		Path:       strings.TrimSpace(request.Header.Get(FileRelayV1PathHeader)),
		RawQuery:   request.Header.Get(FileRelayV1QueryHeader),
		Headers:    make(map[string]string),
		Body:       body,
		BodyLength: request.ContentLength,
	}
	for _, name := range []string{
		"Accept", "Content-Type", "If-Modified-Since", "If-None-Match", "If-Range", "Range",
	} {
		if value := request.Header.Get(name); value != "" {
			input.Headers[name] = value
		}
	}
	if input.Body == http.NoBody {
		input.BodyLength = 0
	}
	if !validFileRelayRequest(input) {
		return LightFileRequest{}, ErrAuthentication
	}
	return input, nil
}

// AuthorizeFileRelayV1 authenticates an existing v1 controller before the
// target Panel invokes its local, allowlisted Agent file surface. A v1 pairing
// authorizes read-only summaries; file management additionally requires the
// target administrator's explicit grant and a signature over the inner request.
func (s *Service) AuthorizeFileRelayV1(
	source string,
	request *http.Request,
) (LightFileRequest, error) {
	if s == nil || s.store == nil {
		return LightFileRequest{}, ErrAuthentication
	}
	input, err := FileRelayV1RequestFromHTTP(request)
	if err != nil {
		return LightFileRequest{}, err
	}
	now := s.now().UTC()
	if !s.fileSources.Allow(cleanRateSubject(source), now) {
		return LightFileRequest{}, ErrRateLimited
	}
	controllerID := strings.TrimSpace(request.Header.Get(headerControllerID))
	if !validID(controllerID) {
		return LightFileRequest{}, ErrAuthentication
	}
	record, err := s.store.Controller(controllerID)
	if err != nil || record.Scope != SummaryScope {
		return LightFileRequest{}, ErrAuthentication
	}
	publicKey, err := decodePublicKey(record.PublicKey)
	if err != nil {
		return LightFileRequest{}, ErrAuthentication
	}
	verifiedID, nonce, err := VerifyRequest(request, s.store.NodeID(), publicKey, now)
	if err != nil {
		return LightFileRequest{}, err
	}
	if verifiedID != controllerID {
		return LightFileRequest{}, ErrAuthentication
	}
	if !s.fileRelayV1Grants.Granted(controllerID, record.Fingerprint) {
		return LightFileRequest{}, ErrAuthentication
	}
	if err := verifyFileRelayV1Binding(request, publicKey, input); err != nil {
		return LightFileRequest{}, err
	}
	if !s.fileRequests.Allow(controllerID, now) {
		return LightFileRequest{}, ErrRateLimited
	}
	if err := s.replays.Accept("file:v1:"+controllerID, nonce, now); err != nil {
		return LightFileRequest{}, err
	}
	s.store.TouchController(controllerID, now)
	return input, nil
}

type remoteV1PanelFileAPI interface {
	OpenFileRelayV1(
		context.Context, string, string, string, ed25519.PrivateKey, time.Time,
		LightFileRequest,
	) (*http.Response, error)
}

type remoteV1SignedPanelFileAPI interface {
	OpenSignedFileRelayV1(
		context.Context, string, string, string, ed25519.PrivateKey, time.Time,
		LightFileRequest,
	) (*http.Response, error)
}

// OpenRemotePanelFileV1 opens a same-page file-manager request against a
// legacy paired Panel after its summary advertises a v1 relay capability.
// This preserves old pair records and credentials; no second pairing is
// needed. Current targets advertise the signed capability only after their
// administrator grants file management; older targets still advertise the
// unsigned hint and keep their earlier behavior.
func (s *Service) OpenRemotePanelFileV1(
	ctx context.Context,
	hostID string,
	input LightFileRequest,
) (*http.Response, error) {
	if s == nil || !validID(hostID) || !validFileRelayRequest(input) {
		return nil, ErrFileRelayUnavailable
	}
	record, err := s.store.Host(hostID)
	if err != nil || record.FederationProtocol != FederationProtocol {
		return nil, ErrFileRelayUnavailable
	}
	s.mu.RLock()
	available := s.runtime[hostID].fileManagementAvailable
	signed := s.runtime[hostID].fileRelayV1Signed
	s.mu.RUnlock()
	if !available {
		return nil, ErrFileRelayUnavailable
	}
	privateKey, err := s.secrets.Read(record.CredentialFile)
	if err != nil {
		return nil, ErrFileRelayUnavailable
	}
	if signed {
		remote, ok := s.remote.(remoteV1SignedPanelFileAPI)
		if !ok {
			return nil, ErrProtocolMismatch
		}
		return remote.OpenSignedFileRelayV1(
			ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
			privateKey, s.now().UTC(), input,
		)
	}
	remote, ok := s.remote.(remoteV1PanelFileAPI)
	if !ok {
		return nil, ErrProtocolMismatch
	}
	return remote.OpenFileRelayV1(
		ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
		privateKey, s.now().UTC(), input,
	)
}

// OpenFileRelayV1 starts one authenticated request against a legacy paired
// Panel. The transport remains HTTPS and the target performs the final route
// and Agent authorization checks before executing the operation.
func (c *RemoteClient) OpenFileRelayV1(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
	input LightFileRequest,
) (*http.Response, error) {
	return c.openFileRelayV1(ctx, origin, controllerID, targetID, privateKey, now, input, false)
}

// OpenSignedFileRelayV1 is OpenFileRelayV1 with the inner file operation bound
// to the request signature, as required by targets that grant v1 file access.
func (c *RemoteClient) OpenSignedFileRelayV1(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
	input LightFileRequest,
) (*http.Response, error) {
	return c.openFileRelayV1(ctx, origin, controllerID, targetID, privateKey, now, input, true)
}

func (c *RemoteClient) openFileRelayV1(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
	input LightFileRequest,
	signed bool,
) (*http.Response, error) {
	if c == nil || c.streamClient == nil || !validFileRelayRequest(input) {
		return nil, ErrAuthentication
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body := input.Body
	if body == nil || body == http.NoBody {
		body = http.NoBody
		input.BodyLength = 0
	}
	request, err := c.newRequest(ctx, http.MethodPost, origin, FileRelayV1Path, body)
	if err != nil {
		return nil, err
	}
	request.ContentLength = input.BodyLength
	request.Header.Set(FileRelayV1MethodHeader, input.Method)
	request.Header.Set(FileRelayV1PathHeader, input.Path)
	request.Header.Set(FileRelayV1QueryHeader, input.RawQuery)
	for key, value := range input.Headers {
		request.Header.Set(key, value)
	}
	nonce, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	if err := SignRequest(request, controllerID, targetID, privateKey, now, nonce); err != nil {
		return nil, err
	}
	if signed {
		if err := signFileRelayV1(request, privateKey, input); err != nil {
			return nil, err
		}
	}
	response, err := c.streamClient.Do(request)
	if err != nil {
		return nil, classifyRemoteTransportError(err)
	}
	if response == nil || response.Body == nil {
		if response != nil {
			_ = response.Body.Close()
		}
		return nil, errors.New("legacy file relay returned an empty response")
	}
	return response, nil
}
