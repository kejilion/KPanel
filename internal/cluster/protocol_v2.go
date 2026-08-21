package cluster

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
	"golang.org/x/crypto/hkdf"
)

const (
	v2PairingCodePrefix  = "kp2."
	v2PairPath           = "/api/v2/federation/pair"
	v2CommitPath         = "/api/v2/federation/commit"
	v2SummaryPath        = "/api/v2/federation/summary"
	v2RevokePath         = "/api/v2/federation/revoke"
	v2TerminalOpenPath   = "/api/v2/federation/terminal/open"
	v2TerminalOutputPath = "/api/v2/federation/terminal/output"
	v2TerminalInputPath  = "/api/v2/federation/terminal/input"
	v2TerminalResizePath = "/api/v2/federation/terminal/resize"
	v2TerminalClosePath  = "/api/v2/federation/terminal/close"

	v2BrowseFetchOpenPath   = "/api/v2/federation/browse/fetch/open"
	v2BrowseFetchOutputPath = "/api/v2/federation/browse/fetch/output"
	v2BrowseFetchClosePath  = "/api/v2/federation/browse/fetch/close"

	v2BrowseWSOpenPath   = "/api/v2/federation/browse/ws/open"
	v2BrowseWSOutputPath = "/api/v2/federation/browse/ws/output"
	v2BrowseWSInputPath  = "/api/v2/federation/browse/ws/input"
	v2BrowseWSClosePath  = "/api/v2/federation/browse/ws/close"

	v2FileOpenPath       = "/api/v2/federation/files/open"
	v2FileLinkPath       = "/api/v2/federation/files/link"
	v2FileLinkedOpenPath = "/api/v2/federation/files/open-linked"

	maxV2PairingCode   = 1024
	maxV2EnvelopeBytes = MaxFederationV2Bytes
)

var v2NoiseSuite = noise.NewCipherSuite(
	noise.DH25519,
	noise.CipherChaChaPoly,
	noise.HashSHA256,
)

type v2PairingDescriptor struct {
	NodeID          string
	TargetPublicKey []byte
	CodeID          string
	PairingKey      []byte
	ExpiresAt       time.Time
}

type v2PairingCodeWire struct {
	Version         int    `json:"v"`
	NodeID          string `json:"nodeId"`
	TargetPublicKey string `json:"targetPublicKey"`
	CodeID          string `json:"codeId"`
	Secret          string `json:"secret"`
	ExpiresAt       int64  `json:"expiresAt"`
}

type v2Envelope struct {
	Protocol     string `json:"protocol"`
	ControllerID string `json:"controllerId"`
	TargetID     string `json:"targetId"`
	CodeID       string `json:"codeId,omitempty"`
	Timestamp    int64  `json:"timestamp"`
	RequestID    string `json:"requestId"`
	Message      string `json:"message"`
}

// FederationEnvelopeV2 is the only outer structure exposed by the v2 HTTP
// endpoint. Its metadata is authenticated as Noise prologue data; Message is
// the encrypted handshake payload and never contains plaintext telemetry or
// pairing secrets.
type FederationEnvelopeV2 = v2Envelope

type v2PairPayload struct {
	ControllerName string `json:"controllerName,omitempty"`
	TransactionID  string `json:"transactionId"`
}

type v2PairResult struct {
	TransactionID      string `json:"transactionId"`
	NodeID             string `json:"nodeId"`
	Hostname           string `json:"hostname"`
	PanelVersion       string `json:"panelVersion"`
	FederationProtocol string `json:"federationProtocol"`
	Scope              string `json:"scope,omitempty"`
}

type TerminalOpenRequest struct {
	Rows    uint16 `json:"rows"`
	Columns uint16 `json:"columns"`
}

type TerminalOpenResponse struct {
	SessionID string    `json:"sessionId"`
	Offset    int64     `json:"offset"`
	CreatedAt time.Time `json:"createdAt"`
}

type TerminalOutputRequest struct {
	SessionID string `json:"sessionId"`
	Offset    int64  `json:"offset"`
	Wait      int    `json:"waitMilliseconds"`
}

type TerminalInputRequest struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type TerminalResizeRequest struct {
	SessionID string `json:"sessionId"`
	Rows      uint16 `json:"rows"`
	Columns   uint16 `json:"columns"`
}

type TerminalCloseRequest struct {
	SessionID string `json:"sessionId"`
}

// BrowseFetchRequest is both the local Agent-backed fetch request and the
// v2 federation Open payload — one shape either way, only the transport
// differs. Body/Headers are omitted when nil/empty rather than sent as
// empty containers, keeping small requests small.
type BrowseFetchRequest struct {
	URL     string              `json:"url"`
	Method  string              `json:"method,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"` // base64
}

// BrowseFetchResult is a fully-buffered fetch outcome — never sent over the
// wire as-is (every v2 round trip is capped at MaxSummaryBytes, far smaller
// than a page asset can be), only held in memory long enough for
// handleBrowseFetchOpenV2 to seed a session that BrowseFetchOutputRequest
// then drains in chunks.
type BrowseFetchResult struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

type BrowseFetchOpenResponse struct {
	SessionID  string              `json:"sessionId"`
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	TotalBytes int                 `json:"totalBytes"`
}

type BrowseFetchOutputRequest struct {
	SessionID string `json:"sessionId"`
	Offset    int    `json:"offset"`
}

type BrowseFetchOutputResponse struct {
	Data       string `json:"data"` // base64 chunk
	NextOffset int    `json:"nextOffset"`
	Done       bool   `json:"done"`
}

type BrowseFetchCloseRequest struct {
	SessionID string `json:"sessionId"`
}

// BrowseWSOpenRequest is the v2 federation Open payload for a WS relay.
// Unlike browse fetch, WS is genuinely a live session on the target
// (being-controlled) side — the controlled node's own internal/agent
// browseWSManager owns the connection and buffering, cluster.Service here
// just forwards Open/Output/Input/Close through (see BrowseWSBackend), no
// separate federation-layer session buffer like browse fetch needed.
type BrowseWSOpenRequest struct {
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
}

type BrowseWSOpenResponse struct {
	SessionID string `json:"sessionId"`
}

type BrowseWSMessage struct {
	Type string `json:"type"` // "text" or "binary"
	Data string `json:"data"` // base64
}

type BrowseWSOutputRequest struct {
	SessionID string `json:"sessionId"`
	Offset    int    `json:"offset"`
	Wait      int    `json:"waitMilliseconds"`
}

type BrowseWSOutputResponse struct {
	Messages    []BrowseWSMessage `json:"messages"`
	NextOffset  int               `json:"nextOffset"`
	Closed      bool              `json:"closed"`
	CloseReason string            `json:"closeReason,omitempty"`
}

type BrowseWSInputRequest struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"` // "text" or "binary"
	Data      string `json:"data"` // base64
}

type BrowseWSCloseRequest struct {
	SessionID string `json:"sessionId"`
}

type v2CommitPayload struct {
	TransactionID string `json:"transactionId"`
}

type v2CommitResult struct {
	TransactionID string `json:"transactionId"`
	Active        bool   `json:"active"`
}

type v2FilePeerLinkRequest struct {
	LinkID string `json:"linkId"`
	NodeID string `json:"nodeId"`
	Origin string `json:"origin"`
}

type v2FilePeerLinkResult struct {
	LinkID string `json:"linkId"`
	NodeID string `json:"nodeId"`
	Linked bool   `json:"linked"`
}

type v2RevokeResult struct {
	Revoked bool `json:"revoked"`
}

func makeV2PairingCode(
	nodeID string,
	targetPublicKey []byte,
	expiresAt time.Time,
) (string, string, []byte, error) {
	if !validID(nodeID) || len(targetPublicKey) != 32 || expiresAt.IsZero() {
		return "", "", nil, ErrPairingCode
	}
	codeID, err := randomHex(8)
	if err != nil {
		return "", "", nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", nil, err
	}
	pairingKey, err := deriveV2PairingKey(
		secret, nodeID, codeID, targetPublicKey,
	)
	if err != nil {
		return "", "", nil, err
	}
	wire := v2PairingCodeWire{
		Version:         2,
		NodeID:          nodeID,
		TargetPublicKey: base64.RawURLEncoding.EncodeToString(targetPublicKey),
		CodeID:          codeID,
		Secret:          base64.RawURLEncoding.EncodeToString(secret),
		ExpiresAt:       expiresAt.UTC().Unix(),
	}
	content, err := json.Marshal(wire)
	if err != nil {
		return "", "", nil, err
	}
	code := v2PairingCodePrefix + base64.RawURLEncoding.EncodeToString(content)
	if len(code) > maxV2PairingCode {
		return "", "", nil, ErrPairingCode
	}
	return code, codeID, pairingKey, nil
}

func parseV2PairingCode(code string, now time.Time) (v2PairingDescriptor, error) {
	code = strings.TrimSpace(code)
	if len(code) <= len(v2PairingCodePrefix) || len(code) > maxV2PairingCode ||
		!strings.HasPrefix(code, v2PairingCodePrefix) {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	content, err := base64.RawURLEncoding.DecodeString(
		strings.TrimPrefix(code, v2PairingCodePrefix),
	)
	if err != nil || len(content) == 0 || len(content) > 768 {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	var wire v2PairingCodeWire
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	targetPublicKey, publicErr := base64.RawURLEncoding.DecodeString(wire.TargetPublicKey)
	secret, secretErr := base64.RawURLEncoding.DecodeString(wire.Secret)
	expiresAt := time.Unix(wire.ExpiresAt, 0).UTC()
	if wire.Version != 2 || !validID(wire.NodeID) ||
		len(wire.CodeID) != 16 || !validHex(wire.CodeID) ||
		publicErr != nil || len(targetPublicKey) != 32 ||
		secretErr != nil || len(secret) != 32 ||
		!expiresAt.After(now.UTC()) ||
		expiresAt.After(now.UTC().Add(10*time.Minute)) {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	pairingKey, err := deriveV2PairingKey(
		secret, wire.NodeID, wire.CodeID, targetPublicKey,
	)
	if err != nil {
		return v2PairingDescriptor{}, ErrPairingCode
	}
	return v2PairingDescriptor{
		NodeID: wire.NodeID, TargetPublicKey: targetPublicKey,
		CodeID: wire.CodeID, PairingKey: pairingKey, ExpiresAt: expiresAt,
	}, nil
}

func deriveV2PairingKey(
	secret []byte,
	nodeID string,
	codeID string,
	targetPublicKey []byte,
) ([]byte, error) {
	if len(secret) != 32 || !validID(nodeID) ||
		len(codeID) != 16 || !validHex(codeID) ||
		len(targetPublicKey) != 32 {
		return nil, ErrPairingCode
	}
	info := strings.Join([]string{
		"KPanel federation v2 pairing PSK",
		nodeID,
		codeID,
		base64.RawURLEncoding.EncodeToString(targetPublicKey),
	}, "\n")
	key := make([]byte, 32)
	if _, err := io.ReadFull(
		hkdf.New(sha256.New, secret, nil, []byte(info)),
		key,
	); err != nil {
		return nil, err
	}
	return key, nil
}

func sealV2Request(
	method string,
	path string,
	envelope v2Envelope,
	localKey noise.DHKey,
	peerPublicKey []byte,
	pairingKey []byte,
	payload []byte,
) (v2Envelope, *noise.HandshakeState, error) {
	envelope.Message = ""
	if err := validateV2Envelope(envelope, false); err != nil ||
		len(localKey.Private) != 32 || len(localKey.Public) != 32 ||
		len(peerPublicKey) != 32 || len(payload) > noise.MaxMsgLen ||
		(len(pairingKey) != 0 && len(pairingKey) != 32) {
		return v2Envelope{}, nil, ErrAuthentication
	}
	handshake, err := newV2Handshake(
		true, localKey, peerPublicKey, pairingKey,
		v2EnvelopePrologue(method, path, envelope, len(pairingKey) > 0),
	)
	if err != nil {
		return v2Envelope{}, nil, ErrAuthentication
	}
	message, _, _, err := handshake.WriteMessage(nil, payload)
	if err != nil {
		return v2Envelope{}, nil, ErrAuthentication
	}
	envelope.Message = base64.RawURLEncoding.EncodeToString(message)
	return envelope, handshake, nil
}

func openV2Request(
	method string,
	path string,
	envelope v2Envelope,
	localKey noise.DHKey,
	pairingKey []byte,
) ([]byte, []byte, *noise.HandshakeState, error) {
	if err := validateV2Envelope(envelope, true); err != nil ||
		len(localKey.Private) != 32 || len(localKey.Public) != 32 ||
		(len(pairingKey) != 0 && len(pairingKey) != 32) {
		return nil, nil, nil, ErrAuthentication
	}
	message, err := decodeV2Message(envelope.Message)
	if err != nil {
		return nil, nil, nil, ErrAuthentication
	}
	envelope.Message = ""
	handshake, err := newV2Handshake(
		false, localKey, nil, pairingKey,
		v2EnvelopePrologue(method, path, envelope, len(pairingKey) > 0),
	)
	if err != nil {
		return nil, nil, nil, ErrAuthentication
	}
	payload, _, _, err := handshake.ReadMessage(nil, message)
	if err != nil {
		return nil, nil, nil, ErrAuthentication
	}
	peerStatic := append([]byte(nil), handshake.PeerStatic()...)
	if len(peerStatic) != 32 {
		return nil, nil, nil, ErrAuthentication
	}
	return payload, peerStatic, handshake, nil
}

func sealV2Response(
	request v2Envelope,
	handshake *noise.HandshakeState,
	payload []byte,
) (v2Envelope, error) {
	if handshake == nil || len(payload) > noise.MaxMsgLen {
		return v2Envelope{}, ErrAuthentication
	}
	response := request
	response.Message = ""
	if err := validateV2Envelope(response, false); err != nil {
		return v2Envelope{}, ErrAuthentication
	}
	message, _, _, err := handshake.WriteMessage(nil, payload)
	if err != nil {
		return v2Envelope{}, ErrAuthentication
	}
	response.Message = base64.RawURLEncoding.EncodeToString(message)
	return response, nil
}

func openV2Response(
	request v2Envelope,
	response v2Envelope,
	handshake *noise.HandshakeState,
) ([]byte, error) {
	if handshake == nil || validateV2Envelope(response, true) != nil ||
		request.Protocol != response.Protocol ||
		request.ControllerID != response.ControllerID ||
		request.TargetID != response.TargetID ||
		request.CodeID != response.CodeID ||
		request.Timestamp != response.Timestamp ||
		request.RequestID != response.RequestID {
		return nil, ErrAuthentication
	}
	message, err := decodeV2Message(response.Message)
	if err != nil {
		return nil, ErrAuthentication
	}
	payload, _, _, err := handshake.ReadMessage(nil, message)
	if err != nil {
		return nil, ErrAuthentication
	}
	return payload, nil
}

func newV2Handshake(
	initiator bool,
	localKey noise.DHKey,
	peerPublicKey []byte,
	pairingKey []byte,
	prologue []byte,
) (*noise.HandshakeState, error) {
	config := noise.Config{
		CipherSuite: v2NoiseSuite,
		Pattern:     noise.HandshakeIK,
		Initiator:   initiator,
		Prologue:    append([]byte(nil), prologue...),
		StaticKeypair: noise.DHKey{
			Private: append([]byte(nil), localKey.Private...),
			Public:  append([]byte(nil), localKey.Public...),
		},
		PeerStatic: append([]byte(nil), peerPublicKey...),
	}
	if len(pairingKey) > 0 {
		config.PresharedKeyPlacement = 0
		config.PresharedKey = append([]byte(nil), pairingKey...)
	}
	return noise.NewHandshakeState(config)
}

func v2EnvelopePrologue(
	method string,
	path string,
	envelope v2Envelope,
	withPSK bool,
) []byte {
	protocolName := "Noise_IK_25519_ChaChaPoly_SHA256"
	if withPSK {
		protocolName = "Noise_IKpsk0_25519_ChaChaPoly_SHA256"
	}
	return []byte(strings.Join([]string{
		"KPanel federation",
		FederationProtocolV2,
		protocolName,
		strings.ToUpper(method),
		path,
		envelope.ControllerID,
		envelope.TargetID,
		envelope.CodeID,
		time.Unix(envelope.Timestamp, 0).UTC().Format(time.RFC3339),
		envelope.RequestID,
	}, "\n"))
}

func validateV2Envelope(envelope v2Envelope, requireMessage bool) error {
	if envelope.Protocol != FederationProtocolV2 ||
		!validID(envelope.ControllerID) ||
		!validID(envelope.TargetID) ||
		envelope.Timestamp < 1 ||
		!validNonce(envelope.RequestID) ||
		(envelope.CodeID != "" &&
			(len(envelope.CodeID) != 16 || !validHex(envelope.CodeID))) ||
		(requireMessage && envelope.Message == "") ||
		(!requireMessage && envelope.Message != "") {
		return ErrAuthentication
	}
	return nil
}

func decodeV2Message(value string) ([]byte, error) {
	if value == "" || len(value) > base64.RawURLEncoding.EncodedLen(noise.MaxMsgLen) {
		return nil, ErrAuthentication
	}
	message, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(message) == 0 || len(message) > noise.MaxMsgLen {
		return nil, ErrAuthentication
	}
	return message, nil
}

func decodeV2Payload(payload []byte, target any) error {
	if target == nil || len(payload) == 0 || len(payload) > MaxSummaryBytes {
		return ErrAuthentication
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrAuthentication
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrAuthentication
	}
	return nil
}

func validHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return value != ""
}

func v2TransportSecurity(origin string) TransportSecurity {
	if strings.HasPrefix(origin, "https://") {
		return TransportSecurityTLS
	}
	return TransportSecurityEncryptedHTTP
}

func v2PathAllowed(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	switch path {
	case v2PairPath, v2CommitPath, v2SummaryPath, v2RevokePath,
		v2TerminalOpenPath, v2TerminalOutputPath, v2TerminalInputPath,
		v2TerminalResizePath, v2TerminalClosePath,
		v2FileOpenPath, v2FileLinkPath, v2FileLinkedOpenPath,
		v2BrowseFetchOpenPath, v2BrowseFetchOutputPath, v2BrowseFetchClosePath,
		v2BrowseWSOpenPath, v2BrowseWSOutputPath, v2BrowseWSInputPath, v2BrowseWSClosePath:
		return true
	default:
		return false
	}
}

func v2TerminalPath(path string) bool {
	switch path {
	case v2TerminalOpenPath, v2TerminalOutputPath, v2TerminalInputPath,
		v2TerminalResizePath, v2TerminalClosePath:
		return true
	default:
		return false
	}
}

// v2BrowseFetchPath shares the terminal request budget (see
// HandleFederationV2/openControllerV2): both are legitimately chatty polling
// loops from an already-authenticated controller, unlike the low-frequency
// pair/commit/summary/revoke calls the default limiter is sized for.
func v2BrowseFetchPath(path string) bool {
	switch path {
	case v2BrowseFetchOpenPath, v2BrowseFetchOutputPath, v2BrowseFetchClosePath:
		return true
	default:
		return false
	}
}

// v2BrowseWSPath gets the same request-budget treatment as
// v2BrowseFetchPath, for the same reason.
func v2BrowseWSPath(path string) bool {
	switch path {
	case v2BrowseWSOpenPath, v2BrowseWSOutputPath, v2BrowseWSInputPath, v2BrowseWSClosePath:
		return true
	default:
		return false
	}
}
