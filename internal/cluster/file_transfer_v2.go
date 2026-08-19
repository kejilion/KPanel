package cluster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	fileStreamContentType = "application/x-kpanel-noise-stream"
	fileStreamChunkBytes  = 60 << 10
	fileStreamMaxFrame    = noise.MaxMsgLen
	fileRecordData        = byte(1)
	fileRecordEnd         = byte(2)
	fileRecordError       = byte(3)
	fileStreamIdleTimeout = 45 * time.Second
)

type FederationFileOpenRequest struct {
	Path            string `json:"path"`
	ResourceVersion string `json:"resourceVersion"`
}

type FederationFileAuthorization struct {
	request   v2Envelope
	handshake *noise.HandshakeState
	release   func()
}

type remoteV2FileAPI interface {
	OpenFileV2(
		context.Context, string, string, string, noise.DHKey, []byte, time.Time,
		FederationFileOpenRequest,
	) (io.ReadCloser, contract.FileTransferMetadata, error)
}

type remoteV2LinkedFileAPI interface {
	OpenLinkedFileV2(
		context.Context, string, string, string, noise.DHKey, []byte, time.Time,
		FederationFileOpenRequest,
	) (io.ReadCloser, contract.FileTransferMetadata, error)
}

type fileStreamLimiter struct {
	mu            sync.Mutex
	active        int
	activeByPeer  map[string]int
	maxActive     int
	maxActivePeer int
}

func newFileStreamLimiter(maxActive, maxActivePeer int) *fileStreamLimiter {
	return &fileStreamLimiter{
		activeByPeer:  make(map[string]int),
		maxActive:     maxActive,
		maxActivePeer: maxActivePeer,
	}
}

func (l *fileStreamLimiter) acquire(peerID string) (func(), bool) {
	if l == nil || !validID(peerID) {
		return nil, false
	}
	l.mu.Lock()
	if l.maxActive < 1 || l.maxActivePeer < 1 ||
		l.active >= l.maxActive || l.activeByPeer[peerID] >= l.maxActivePeer {
		l.mu.Unlock()
		return nil, false
	}
	l.active++
	l.activeByPeer[peerID]++
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if current := l.activeByPeer[peerID]; current <= 1 {
				delete(l.activeByPeer, peerID)
			} else {
				l.activeByPeer[peerID] = current - 1
			}
			if l.active > 0 {
				l.active--
			}
		})
	}, true
}

func (s *Service) AuthorizeFederationFileV2(
	source string,
	envelope FederationEnvelopeV2,
) (FederationFileOpenRequest, *FederationFileAuthorization, error) {
	now := s.now().UTC()
	if !s.fileSources.Allow(cleanRateSubject(source), now) {
		return FederationFileOpenRequest{}, nil, ErrRateLimited
	}
	if err := s.validateV2Request(v2FileOpenPath, envelope, now); err != nil {
		return FederationFileOpenRequest{}, nil, err
	}
	controller, payload, handshake, err := s.openControllerV2(
		v2FileOpenPath, envelope, now, controllerStateV2Active,
	)
	if err != nil || !ScopeAllowsFiles(controller.Scope) {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	var input FederationFileOpenRequest
	if err := decodeV2Payload(payload, &input); err != nil ||
		!validTransferPath(input.Path) || len(input.ResourceVersion) < 8 || len(input.ResourceVersion) > 256 {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	release, ok := s.fileStreams.acquire(controller.ID)
	if !ok {
		return FederationFileOpenRequest{}, nil, ErrRateLimited
	}
	_ = s.storeV2.TouchController(controller.ID, now)
	return input, &FederationFileAuthorization{
		request: envelope, handshake: handshake, release: release,
	}, nil
}

// AuthorizeLinkedFederationFileV2 authenticates the reverse file-only channel
// created by a bidirectional pairing. It deliberately does not fall back to a
// Controller record: the link grant and its active parent Host must both still
// match before the reused controller key may act as the Noise responder.
func (s *Service) AuthorizeLinkedFederationFileV2(
	source string,
	envelope FederationEnvelopeV2,
) (FederationFileOpenRequest, *FederationFileAuthorization, error) {
	now := s.now().UTC()
	if !s.fileSources.Allow(cleanRateSubject(source), now) {
		return FederationFileOpenRequest{}, nil, ErrRateLimited
	}
	if err := s.validateV2Request(v2FileLinkedOpenPath, envelope, now); err != nil {
		return FederationFileOpenRequest{}, nil, err
	}
	grant, err := s.filePeersV2.ActiveGrant(envelope.ControllerID, now)
	if err != nil || grant.LinkID != envelope.ControllerID || grant.Scope != filePeerReadScope {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	host, err := s.storeV2.Host(grant.HostID)
	if err != nil || host.State != hostStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(host.Scope)) ||
		host.ControllerID != grant.HostControllerID ||
		host.TransactionID != grant.HostTransaction ||
		host.RemoteNodeID != grant.PeerNodeID ||
		host.PeerFingerprint != grant.PeerFingerprint {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	credential, err := s.secretsV2.ReadCredential(host.CredentialFile)
	if err != nil {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	expectedTarget, err := base64.RawURLEncoding.DecodeString(host.TargetPublicKey)
	if err != nil || !bytes.Equal(expectedTarget, credential.TargetPublic) ||
		fingerprintV2(credential.TargetPublic) != host.PeerFingerprint {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	payload, peerStatic, handshake, err := openV2Request(
		http.MethodPost,
		v2FileLinkedOpenPath,
		envelope,
		noiseKeyV2(credential),
		nil,
	)
	if err != nil || !bytes.Equal(peerStatic, credential.TargetPublic) {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	if !s.fileRequests.Allow(grant.LinkID, now) {
		return FederationFileOpenRequest{}, nil, ErrRateLimited
	}
	if err := s.replays.Accept(grant.LinkID, envelope.RequestID, now); err != nil {
		return FederationFileOpenRequest{}, nil, err
	}
	var input FederationFileOpenRequest
	if err := decodeV2Payload(payload, &input); err != nil ||
		!validTransferPath(input.Path) ||
		len(input.ResourceVersion) < 8 || len(input.ResourceVersion) > 256 {
		return FederationFileOpenRequest{}, nil, ErrAuthentication
	}
	release, ok := s.fileStreams.acquire(grant.LinkID)
	if !ok {
		return FederationFileOpenRequest{}, nil, ErrRateLimited
	}
	return input, &FederationFileAuthorization{
		request: envelope, handshake: handshake, release: release,
	}, nil
}

func (a *FederationFileAuthorization) Close() error {
	if a != nil && a.release != nil {
		a.release()
	}
	return nil
}

func (a *FederationFileAuthorization) SealMetadata(
	metadata contract.FileTransferMetadata,
) (FederationEnvelopeV2, *noise.CipherState, error) {
	if a == nil || a.handshake == nil || !validTransferMetadata(metadata) {
		return FederationEnvelopeV2{}, nil, ErrAuthentication
	}
	payload, err := json.Marshal(metadata)
	if err != nil || len(payload) > MaxSummaryBytes {
		return FederationEnvelopeV2{}, nil, ErrAuthentication
	}
	response := a.request
	response.Message = ""
	message, _, encrypt, err := a.handshake.WriteMessage(nil, payload)
	if err != nil || encrypt == nil {
		return FederationEnvelopeV2{}, nil, ErrAuthentication
	}
	response.Message = base64.RawURLEncoding.EncodeToString(message)
	return response, encrypt, nil
}

func validTransferPath(value string) bool {
	if value == "" || value == "/" || len(value) > 4096 ||
		!strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validTransferMetadata(value contract.FileTransferMetadata) bool {
	if (value.Kind != "file" && value.Kind != "directory") ||
		value.Name == "" || value.Name == "." || value.Name == ".." ||
		len(value.Name) > 255 || strings.ContainsAny(value.Name, "/\\") ||
		value.SizeBytes < 0 || value.SizeBytes > 10<<30 ||
		(value.Kind == "file" && value.SizeBytes > 512<<20) ||
		len(value.ResourceVersion) < 8 || len(value.ResourceVersion) > 256 {
		return false
	}
	for _, character := range value.Name {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	for _, prefix := range []string{".kpanel-edit-", ".kpanel-upload-", ".kpanel-copy-", ".kpanel-archive-", ".kpanel-extract-"} {
		if strings.HasPrefix(value.Name, prefix) {
			return false
		}
	}
	return true
}

// WriteFederationFileHeader starts the authenticated binary stream. The first
// frame is the ordinary v2 response envelope containing encrypted metadata.
func WriteFederationFileHeader(output io.Writer, envelope FederationEnvelopeV2) error {
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxV2EnvelopeBytes {
		return ErrAuthentication
	}
	return writeFileFrame(output, body)
}

type FederationFileWriter struct {
	output io.Writer
	cipher *noise.CipherState
	done   bool
}

func NewFederationFileWriter(output io.Writer, cipher *noise.CipherState) *FederationFileWriter {
	return &FederationFileWriter{output: output, cipher: cipher}
}

func (w *FederationFileWriter) Write(content []byte) (int, error) {
	if w == nil || w.done || w.cipher == nil {
		return 0, io.ErrClosedPipe
	}
	written := 0
	for len(content) > 0 {
		count := len(content)
		if count > fileStreamChunkBytes {
			count = fileStreamChunkBytes
		}
		plain := make([]byte, count+1)
		plain[0] = fileRecordData
		copy(plain[1:], content[:count])
		sealed, err := w.cipher.Encrypt(nil, nil, plain)
		if err != nil || len(sealed) > fileStreamMaxFrame {
			return written, ErrAuthentication
		}
		if err := writeFileFrame(w.output, sealed); err != nil {
			return written, err
		}
		written += count
		content = content[count:]
	}
	return written, nil
}

func (w *FederationFileWriter) Finish(transferErr error) error {
	if w == nil || w.done || w.cipher == nil {
		return io.ErrClosedPipe
	}
	w.done = true
	record := []byte{fileRecordEnd}
	if transferErr != nil {
		record = append([]byte{fileRecordError}, []byte("source_transfer_failed")...)
	}
	sealed, err := w.cipher.Encrypt(nil, nil, record)
	if err != nil {
		return ErrAuthentication
	}
	return writeFileFrame(w.output, sealed)
}

func (s *Service) OpenRemoteFileV2(
	ctx context.Context,
	remoteNodeID string,
	input FederationFileOpenRequest,
) (io.ReadCloser, contract.FileTransferMetadata, error) {
	if !validID(remoteNodeID) || !validTransferPath(input.Path) {
		return nil, contract.FileTransferMetadata{}, ErrNotFound
	}
	now := s.now().UTC()
	var record hostRecordV2
	for _, candidate := range s.storeV2.Hosts() {
		if candidate.RemoteNodeID == remoteNodeID {
			record = candidate
			break
		}
	}
	if record.ID != "" {
		if record.State != hostStateV2Active ||
			!ScopeAllowsFiles(normalizedV2Scope(record.Scope)) {
			return nil, contract.FileTransferMetadata{}, ErrNotFound
		}
		credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
		if err != nil {
			return nil, contract.FileTransferMetadata{}, err
		}
		remote, ok := s.remoteV2.(remoteV2FileAPI)
		if !ok {
			return nil, contract.FileTransferMetadata{}, ErrProtocolMismatch
		}
		return remote.OpenFileV2(
			ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
			noiseKeyV2(credential), credential.TargetPublic, now, input,
		)
	}

	// Only the absence of a direct Host permits the reverse route. A failed
	// direct authentication must never be retried under different credentials.
	route, err := s.filePeersV2.ActiveRoute(remoteNodeID, now)
	if err != nil || route.PeerNodeID != remoteNodeID || route.Scope != filePeerReadScope {
		return nil, contract.FileTransferMetadata{}, ErrNotFound
	}
	controller, err := s.storeV2.Controller(route.ControllerID)
	if err != nil || controller.State != controllerStateV2Active ||
		!ScopeAllowsFiles(normalizedV2Scope(controller.Scope)) ||
		controller.TransactionID != route.ControllerTransaction ||
		controller.Fingerprint != route.ControllerFingerprint {
		return nil, contract.FileTransferMetadata{}, ErrNotFound
	}
	controllerPublic, err := base64.RawURLEncoding.DecodeString(controller.PublicKey)
	if err != nil || len(controllerPublic) != 32 ||
		fingerprintV2(controllerPublic) != controller.Fingerprint {
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	remote, ok := s.remoteV2.(remoteV2LinkedFileAPI)
	if !ok {
		return nil, contract.FileTransferMetadata{}, ErrProtocolMismatch
	}
	return remote.OpenLinkedFileV2(
		ctx,
		route.PeerOrigin,
		route.LinkID,
		route.PeerNodeID,
		nodeNoiseKeyV2(s.nodeIdentityV2),
		controllerPublic,
		now,
		input,
	)
}

func (c *RemoteClient) OpenFileV2(
	ctx context.Context,
	origin string,
	controllerID string,
	targetID string,
	controllerKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input FederationFileOpenRequest,
) (io.ReadCloser, contract.FileTransferMetadata, error) {
	return c.openFileV2(
		ctx, origin, v2FileOpenPath, controllerID, targetID,
		controllerKey, targetPublicKey, now, input,
	)
}

func (c *RemoteClient) OpenLinkedFileV2(
	ctx context.Context,
	origin string,
	linkID string,
	targetID string,
	nodeKey noise.DHKey,
	controllerPublicKey []byte,
	now time.Time,
	input FederationFileOpenRequest,
) (io.ReadCloser, contract.FileTransferMetadata, error) {
	return c.openFileV2(
		ctx, origin, v2FileLinkedOpenPath, linkID, targetID,
		nodeKey, controllerPublicKey, now, input,
	)
}

func (c *RemoteClient) openFileV2(
	ctx context.Context,
	origin string,
	requestPath string,
	controllerID string,
	targetID string,
	localKey noise.DHKey,
	targetPublicKey []byte,
	now time.Time,
	input FederationFileOpenRequest,
) (io.ReadCloser, contract.FileTransferMetadata, error) {
	if requestPath != v2FileOpenPath && requestPath != v2FileLinkedOpenPath {
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	payload, err := json.Marshal(input)
	if err != nil || len(payload) > MaxSummaryBytes {
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	requestID, err := randomHex(16)
	if err != nil {
		return nil, contract.FileTransferMetadata{}, err
	}
	envelope, handshake, err := sealV2Request(
		http.MethodPost, requestPath,
		v2Envelope{Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: targetID, Timestamp: now.UTC().Unix(), RequestID: requestID},
		localKey, targetPublicKey, nil, payload,
	)
	if err != nil {
		return nil, contract.FileTransferMetadata{}, err
	}
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxV2EnvelopeBytes {
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	request, err := c.newV2Request(ctx, http.MethodPost, origin, requestPath, bytes.NewReader(body))
	if err != nil {
		return nil, contract.FileTransferMetadata{}, err
	}
	request.Header.Set("Accept", fileStreamContentType)
	response, err := c.streamClient.Do(request)
	if err != nil {
		return nil, contract.FileTransferMetadata{}, classifyRemoteTransportError(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 ||
		!strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), fileStreamContentType) {
		response.Body.Close()
		return nil, contract.FileTransferMetadata{}, &RemoteError{Code: "invalid_response", StatusCode: response.StatusCode}
	}
	streamBody := newIdleReadCloser(response.Body, fileStreamIdleTimeout)
	reader := bufio.NewReader(streamBody)
	header, err := readFileFrame(reader, maxV2EnvelopeBytes)
	if err != nil {
		streamBody.Close()
		return nil, contract.FileTransferMetadata{}, &RemoteError{Code: "invalid_response"}
	}
	var responseEnvelope v2Envelope
	if err := decodeV2Payload(header, &responseEnvelope); err != nil {
		streamBody.Close()
		return nil, contract.FileTransferMetadata{}, &RemoteError{Code: "invalid_response"}
	}
	message, err := decodeV2Message(responseEnvelope.Message)
	if err != nil || validateMatchingV2Response(envelope, responseEnvelope) != nil {
		streamBody.Close()
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	plaintext, _, decrypt, err := handshake.ReadMessage(nil, message)
	if err != nil || decrypt == nil {
		streamBody.Close()
		return nil, contract.FileTransferMetadata{}, ErrAuthentication
	}
	var metadata contract.FileTransferMetadata
	if err := decodeV2Payload(plaintext, &metadata); err != nil || !validTransferMetadata(metadata) {
		streamBody.Close()
		return nil, contract.FileTransferMetadata{}, &RemoteError{Code: "invalid_response"}
	}
	return &federationFileReader{source: reader, body: streamBody, cipher: decrypt}, metadata, nil
}

func validateMatchingV2Response(request, response v2Envelope) error {
	if validateV2Envelope(response, true) != nil || request.Protocol != response.Protocol ||
		request.ControllerID != response.ControllerID || request.TargetID != response.TargetID ||
		request.Timestamp != response.Timestamp || request.RequestID != response.RequestID || response.CodeID != "" {
		return ErrAuthentication
	}
	return nil
}

type federationFileReader struct {
	source *bufio.Reader
	body   io.ReadCloser
	cipher *noise.CipherState
	buffer []byte
	done   bool
}

func (r *federationFileReader) Read(output []byte) (int, error) {
	for len(r.buffer) == 0 {
		if r.done {
			return 0, io.EOF
		}
		frame, err := readFileFrame(r.source, fileStreamMaxFrame)
		if err != nil {
			return 0, io.ErrUnexpectedEOF
		}
		plain, err := r.cipher.Decrypt(nil, nil, frame)
		if err != nil || len(plain) == 0 {
			return 0, ErrAuthentication
		}
		switch plain[0] {
		case fileRecordData:
			if len(plain) == 1 {
				return 0, ErrAuthentication
			}
			r.buffer = plain[1:]
		case fileRecordEnd:
			if len(plain) != 1 {
				return 0, ErrAuthentication
			}
			r.done = true
		case fileRecordError:
			return 0, &RemoteError{Code: "source_transfer_failed"}
		default:
			return 0, ErrAuthentication
		}
	}
	count := copy(output, r.buffer)
	r.buffer = r.buffer[count:]
	return count, nil
}

func (r *federationFileReader) Close() error {
	r.done = true
	return r.body.Close()
}

type idleReadCloser struct {
	body      io.ReadCloser
	timeout   time.Duration
	activity  chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func newIdleReadCloser(body io.ReadCloser, timeout time.Duration) *idleReadCloser {
	reader := &idleReadCloser{
		body: body, timeout: timeout,
		activity: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go reader.watchIdle()
	return reader
}

func (r *idleReadCloser) Read(output []byte) (int, error) {
	r.markActivity()
	count, err := r.body.Read(output)
	if count > 0 {
		r.markActivity()
	}
	return count, err
}

func (r *idleReadCloser) Close() error {
	r.closeOnce.Do(func() {
		close(r.done)
		r.closeErr = r.body.Close()
	})
	return r.closeErr
}

func (r *idleReadCloser) markActivity() {
	select {
	case r.activity <- struct{}{}:
	default:
	}
}

func (r *idleReadCloser) watchIdle() {
	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			_ = r.Close()
			return
		case <-r.activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(r.timeout)
		case <-r.done:
			return
		}
	}
}

func writeFileFrame(output io.Writer, content []byte) error {
	if len(content) == 0 || len(content) > fileStreamMaxFrame {
		return ErrAuthentication
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(content)))
	if _, err := output.Write(header[:]); err != nil {
		return err
	}
	_, err := output.Write(content)
	return err
}

func readFileFrame(input io.Reader, limit int64) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(input, header[:]); err != nil {
		return nil, err
	}
	length := int64(binary.BigEndian.Uint32(header[:]))
	if length < 1 || length > limit {
		return nil, ErrAuthentication
	}
	content := make([]byte, length)
	_, err := io.ReadFull(input, content)
	return content, err
}
