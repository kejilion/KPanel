package cluster

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxFileRelayV1BodyBytes        = int64(512 << 20)
	maxConcurrentFileRelayV1Spools = 2
	// FileRelayV1SignedCapability requires a target-side administrator grant
	// and binds both the inner operation and the complete request body.
	FileRelayV1SignedCapability = "file-relay-v1-signed-body"
	FileRelayV1SignatureHeader  = "X-KPanel-File-Signature"
	FileRelayV1BodyDigestHeader = "X-KPanel-File-Body-SHA256"
)

// fileRelayV1Canonical binds the inner file operation to the outer v1 request
// signature. The outer canonical form predates the relay and covers only the
// fixed relay path, so without this binding a party able to modify a request
// in transit could turn an authorized read into a write.
func fileRelayV1Canonical(controllerID, targetID, timestamp, nonce string, input LightFileRequest, bodyDigest string) string {
	return strings.Join([]string{
		"KPanel federation file relay v1",
		http.MethodPost, FileRelayV1Path, controllerID, targetID, timestamp, nonce,
		strings.ToUpper(input.Method), input.Path, input.RawQuery,
		strconv.FormatInt(input.BodyLength, 10), bodyDigest,
	}, "\n")
}

func signFileRelayV1(request *http.Request, privateKey ed25519.PrivateKey, input LightFileRequest, bodyDigest [sha256.Size]byte) error {
	if request == nil || len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("federation file relay signing input is invalid")
	}
	digestText := hex.EncodeToString(bodyDigest[:])
	request.Header.Set(FileRelayV1BodyDigestHeader, digestText)
	canonical := fileRelayV1Canonical(
		request.Header.Get(headerControllerID), request.Header.Get(headerTargetID),
		request.Header.Get(headerTimestamp), request.Header.Get(headerNonce), input, digestText,
	)
	request.Header.Set(FileRelayV1SignatureHeader, base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(canonical))))
	return nil
}

func verifyFileRelayV1Binding(request *http.Request, publicKey ed25519.PublicKey, input LightFileRequest, bodyDigest [sha256.Size]byte) error {
	values := request.Header.Values(FileRelayV1SignatureHeader)
	digestValues := request.Header.Values(FileRelayV1BodyDigestHeader)
	if len(values) != 1 || len(values[0]) > 128 || len(digestValues) != 1 ||
		len(digestValues[0]) != sha256.Size*2 || len(publicKey) != ed25519.PublicKeySize {
		return ErrAuthentication
	}
	digestText := strings.ToLower(strings.TrimSpace(digestValues[0]))
	claimedDigest, err := hex.DecodeString(digestText)
	if err != nil || subtle.ConstantTimeCompare(claimedDigest, bodyDigest[:]) != 1 {
		return ErrAuthentication
	}
	signature, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(values[0]))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrAuthentication
	}
	canonical := fileRelayV1Canonical(
		strings.TrimSpace(request.Header.Get(headerControllerID)), strings.TrimSpace(request.Header.Get(headerTargetID)),
		strings.TrimSpace(request.Header.Get(headerTimestamp)), strings.TrimSpace(request.Header.Get(headerNonce)), input, digestText,
	)
	if !ed25519.Verify(publicKey, []byte(canonical), signature) {
		return ErrAuthentication
	}
	return nil
}

type fileRelayV1Spool struct {
	file    *os.File
	path    string
	release func()
	once    sync.Once
	err     error
}

func (s *fileRelayV1Spool) Read(buffer []byte) (int, error) { return s.file.Read(buffer) }

func (s *fileRelayV1Spool) Close() error {
	s.once.Do(func() {
		if err := s.file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			s.err = err
		}
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.err = errors.Join(s.err, err)
		}
		if s.release != nil {
			s.release()
		}
	})
	return s.err
}

func spoolFileRelayV1Body(tempDir string, slots chan struct{}, reader io.Reader, declaredLength int64) (*fileRelayV1Spool, [sha256.Size]byte, int64, error) {
	var digest [sha256.Size]byte
	if declaredLength < -1 || declaredLength > maxFileRelayV1BodyBytes {
		return nil, digest, 0, errors.New("federation file relay body length is invalid")
	}
	release, err := acquireFileRelaySpoolSlot(slots)
	if err != nil {
		return nil, digest, 0, err
	}
	if reader == nil || reader == http.NoBody {
		reader = strings.NewReader("")
	}
	file, err := os.CreateTemp(tempDir, "kpanel-file-relay-v1-*.tmp")
	if err != nil {
		release()
		return nil, digest, 0, fmt.Errorf("create federation file relay body spool: %w", err)
	}
	spool := &fileRelayV1Spool{file: file, path: file.Name(), release: release}
	removeOnError := true
	defer func() {
		if removeOnError {
			_ = spool.Close()
		}
	}()
	limit := maxFileRelayV1BodyBytes + 1
	if declaredLength >= 0 && declaredLength < limit {
		limit = declaredLength + 1
	}
	hasher := sha256.New()
	length, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(reader, limit))
	if err != nil {
		return nil, digest, 0, fmt.Errorf("read federation file relay body: %w", err)
	}
	if length > maxFileRelayV1BodyBytes || (declaredLength >= 0 && length != declaredLength) {
		return nil, digest, 0, errors.New("federation file relay body length does not match its declaration")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, digest, 0, fmt.Errorf("rewind federation file relay body spool: %w", err)
	}
	copy(digest[:], hasher.Sum(nil))
	removeOnError = false
	return spool, digest, length, nil
}

func acquireFileRelaySpoolSlot(slots chan struct{}) (func(), error) {
	if slots == nil {
		return func() {}, nil
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	default:
		return nil, ErrRateLimited
	}
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
	if !s.fileRequests.Allow(controllerV1Principal(controllerID), now) {
		return LightFileRequest{}, ErrRateLimited
	}
	principal := controllerV1Principal(controllerID)
	if err := s.replays.Check(principal, nonce, now); err != nil {
		return LightFileRequest{}, err
	}
	spool, bodyDigest, bodyLength, err := spoolFileRelayV1Body(s.fileRelaySpoolDir, s.fileRelayV1SpoolSlots, input.Body, input.BodyLength)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			return LightFileRequest{}, ErrRateLimited
		}
		return LightFileRequest{}, ErrAuthentication
	}
	keepSpool := false
	defer func() {
		if !keepSpool {
			_ = spool.Close()
		}
	}()
	input.Body = spool
	input.BodyLength = bodyLength

	// Body spooling can take a long time. Re-read the controller and its grant
	// afterwards so a key replacement or administrator revocation during the
	// upload cannot authorize a request under stale state.
	current, err := s.store.Controller(controllerID)
	if err != nil || current.Scope != SummaryScope {
		return LightFileRequest{}, ErrAuthentication
	}
	currentPublicKey, err := decodePublicKey(current.PublicKey)
	if err != nil || !s.fileRelayV1Grants.Granted(controllerID, current.Fingerprint) {
		return LightFileRequest{}, ErrAuthentication
	}
	authorizedAt := s.now().UTC()
	verifiedID, verifiedNonce, err := VerifyRequest(request, s.store.NodeID(), currentPublicKey, authorizedAt)
	if err != nil || verifiedID != controllerID || verifiedNonce != nonce {
		return LightFileRequest{}, ErrAuthentication
	}
	if err := verifyFileRelayV1Binding(request, currentPublicKey, input, bodyDigest); err != nil {
		return LightFileRequest{}, err
	}
	if err := s.replays.Accept(principal, verifiedNonce, authorizedAt); err != nil {
		return LightFileRequest{}, err
	}
	s.store.TouchController(controllerID, authorizedAt)
	keepSpool = true
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

// OpenRemotePanelFileV1 opens a same-page file-manager request only when the
// target advertises body-signed v1 relay support. Older unsigned relays fail
// closed because their fixed outer signature does not bind the inner body.
func (s *Service) openRemotePanelFileV1(
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
	if !available || !signed {
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
	return nil, ErrFileRelayUnavailable
}

// OpenFileRelayV1 retains the original API shape for callers compiled against
// older releases. It fails closed because the old signature does not bind the
// inner request body; use OpenSignedFileRelayV1 for supported targets.
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
	if !signed {
		return nil, ErrProtocolMismatch
	}
	if c == nil || c.streamClient == nil || !validFileRelayRequest(input) {
		return nil, ErrAuthentication
	}
	if ctx == nil {
		ctx = context.Background()
	}
	spool, bodyDigest, bodyLength, err := spoolFileRelayV1Body(c.spoolDir, c.spoolSlots, input.Body, input.BodyLength)
	if err != nil {
		return nil, err
	}
	keepSpool := false
	defer func() {
		if !keepSpool {
			_ = spool.Close()
		}
	}()
	input.Body = spool
	input.BodyLength = bodyLength
	request, err := c.newRequest(ctx, http.MethodPost, origin, FileRelayV1Path, spool)
	if err != nil {
		return nil, err
	}
	request.ContentLength = bodyLength
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
	if err := signFileRelayV1(request, privateKey, input, bodyDigest); err != nil {
		return nil, err
	}
	response, err := c.streamClient.Do(request)
	if err != nil {
		return nil, classifyRemoteTransportError(err)
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("legacy file relay returned an empty response")
	}
	response.Body = &fileRelayV1ResponseBody{ReadCloser: response.Body, spool: spool}
	keepSpool = true
	return response, nil
}

type fileRelayV1ResponseBody struct {
	io.ReadCloser
	spool *fileRelayV1Spool
	once  sync.Once
	err   error
}

func (b *fileRelayV1ResponseBody) Close() error {
	b.once.Do(func() {
		b.err = errors.Join(b.ReadCloser.Close(), b.spool.Close())
	})
	return b.err
}
