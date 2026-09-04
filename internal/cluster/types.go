package cluster

import (
	"errors"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	FederationProtocol        = "v1"
	FederationProtocolV2      = "v2"
	LightNodeProtocol         = "light-v1"
	SummaryScope              = "cluster.summary.read"
	SummaryTerminalScope      = "cluster.summary.read cluster.terminal.open"
	SummaryTerminalFilesScope = "cluster.summary.read cluster.terminal.open cluster.files.read"
	LocalHostID               = "local"
	MaxHosts                  = 100
	MaxSummaryBytes           = 64 << 10
	MaxPairBytes              = 16 << 10
	MaxFederationV2Bytes      = 96 << 10
)

type HostKind string

const (
	HostKindPanel     HostKind = "panel"
	HostKindLightNode HostKind = "light_node"
)

type TransportSecurity string

const (
	TransportSecurityTLS           TransportSecurity = "tls"
	TransportSecurityEncryptedHTTP TransportSecurity = "e2e_http"
)

var (
	ErrNotFound               = errors.New("cluster record not found")
	ErrConflict               = errors.New("cluster record changed")
	ErrDuplicate              = errors.New("cluster host already exists")
	ErrHostLimit              = errors.New("cluster host limit reached")
	ErrInvalidOrigin          = errors.New("invalid cluster origin")
	ErrLightHTTPSOrigin       = errors.New("light node requires an HTTPS origin")
	ErrPrivateOrigin          = errors.New("cluster origin is outside the configured private network allowlist")
	ErrPairingCode            = errors.New("pairing code is invalid or expired")
	ErrAuthentication         = errors.New("federation authentication failed")
	ErrReplay                 = errors.New("federation request replayed")
	ErrRateLimited            = errors.New("federation request rate limited")
	ErrProtocolMismatch       = errors.New("federation protocol is incompatible")
	ErrMutualFilesUnsupported = errors.New("mutual file transfer is unsupported")
	ErrIdentityMismatch       = errors.New("federation target identity changed")
	ErrLocalHost              = errors.New("local cluster host cannot be modified")
)

type HostState string

const (
	HostUnknown      HostState = "unknown"
	HostPairing      HostState = "pairing"
	HostRevoking     HostState = "revoking"
	HostOnline       HostState = "online"
	HostDegraded     HostState = "degraded"
	HostStale        HostState = "stale"
	HostOffline      HostState = "offline"
	HostAuthFailed   HostState = "auth_failed"
	HostTLSFailed    HostState = "tls_error"
	HostIncompatible HostState = "incompatible"
)

type HostSnapshot struct {
	Telemetry              contract.HostTelemetry `json:"telemetry"`
	ReceivedAt             time.Time              `json:"receivedAt"`
	LatencyMilliseconds    int64                  `json:"latencyMilliseconds"`
	ReceiveBytesPerSecond  float64                `json:"receiveBytesPerSecond"`
	TransmitBytesPerSecond float64                `json:"transmitBytesPerSecond"`
}

type Host struct {
	ID                          string            `json:"id"`
	IsLocal                     bool              `json:"isLocal"`
	Name                        string            `json:"name"`
	Kind                        HostKind          `json:"kind"`
	Origin                      string            `json:"origin"`
	TransportSecurity           TransportSecurity `json:"transportSecurity"`
	PeerFingerprint             string            `json:"peerFingerprint,omitempty"`
	RemoteNodeID                string            `json:"remoteNodeId"`
	FederationProtocol          string            `json:"federationProtocol"`
	Scope                       string            `json:"scope"`
	TerminalAvailable           bool              `json:"terminalAvailable"`
	FileManagementAvailable     bool              `json:"fileManagementAvailable"`
	FileTransferAvailable       bool              `json:"fileTransferAvailable"`
	MutualFileTransferAvailable bool              `json:"mutualFileTransferAvailable"`
	PanelVersion                string            `json:"panelVersion,omitempty"`
	SecurityEntrancePath        string            `json:"securityEntrancePath,omitempty"`
	State                       HostState         `json:"state"`
	LastSnapshot                *HostSnapshot     `json:"lastSnapshot,omitempty"`
	LastAttemptAt               *time.Time        `json:"lastAttemptAt,omitempty"`
	LastSuccessAt               *time.Time        `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures         int               `json:"consecutiveFailures"`
	LastErrorCode               string            `json:"lastErrorCode,omitempty"`
	LastError                   string            `json:"lastError,omitempty"`
	Polling                     bool              `json:"polling"`
	NextPollAt                  *time.Time        `json:"nextPollAt,omitempty"`
	ResourceVersion             string            `json:"resourceVersion"`
	CreatedAt                   time.Time         `json:"createdAt"`
	UpdatedAt                   time.Time         `json:"updatedAt"`
}

func ScopeAllowsTerminal(scope string) bool {
	return scope == SummaryTerminalScope || scope == SummaryTerminalFilesScope
}

func ScopeAllowsFiles(scope string) bool {
	return scope == SummaryTerminalFilesScope
}

type LightEnrollment struct {
	Command   string    `json:"command"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type LightEnrollRequest struct {
	Token             string `json:"token"`
	Name              string `json:"name,omitempty"`
	NodeVersion       string `json:"nodeVersion"`
	TerminalPublicKey string `json:"terminalPublicKey,omitempty"`
}

type LightEnrollResponse struct {
	NodeID                string `json:"nodeId"`
	ReportingKey          string `json:"reportingKey"`
	ReportInterval        int    `json:"reportIntervalSeconds"`
	TerminalPeerPublicKey string `json:"terminalPeerPublicKey,omitempty"`
	TargetNodeID          string `json:"targetNodeId,omitempty"`
}

type LightReportRequest struct {
	Telemetry contract.HostTelemetry `json:"telemetry"`
}

type LightReportResponse struct {
	AcceptedAt time.Time `json:"acceptedAt"`
	NextReport int       `json:"nextReportSeconds"`
}

type AddHostInput struct {
	Name             string `json:"name,omitempty"`
	Origin           string `json:"origin"`
	PairingCode      string `json:"pairingCode"`
	ControllerOrigin string `json:"-"`
}

type UpdateHostInput struct {
	Name                    string `json:"name"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type DeleteHostInput struct {
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type DeleteHostResult struct {
	Deleted           bool `json:"deleted"`
	RemoteRevoked     bool `json:"remoteRevoked"`
	CredentialRemoved bool `json:"credentialRemoved"`
}

type HostList struct {
	Items               []Host `json:"items"`
	Total               int    `json:"total"`
	RemoteTotal         int    `json:"remoteTotal"`
	MaxHosts            int    `json:"maxHosts"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	NodeID              string `json:"nodeId"`
}

type PairingCode struct {
	Code      string    `json:"code"`
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Controller struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	Fingerprint string     `json:"fingerprint"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt,omitempty"`
}

type PairRequest struct {
	PairingCode        string `json:"pairingCode"`
	ControllerID       string `json:"controllerId"`
	ControllerName     string `json:"controllerName,omitempty"`
	PublicKey          string `json:"publicKey"`
	FederationProtocol string `json:"federationProtocol"`
}

type PairResponse struct {
	NodeID             string `json:"nodeId"`
	Hostname           string `json:"hostname"`
	PanelVersion       string `json:"panelVersion"`
	FederationProtocol string `json:"federationProtocol"`
}

type FederationSummary struct {
	NodeID               string                 `json:"nodeId"`
	PanelVersion         string                 `json:"panelVersion"`
	FederationProtocol   string                 `json:"federationProtocol"`
	SecurityEntrancePath string                 `json:"securityEntrancePath,omitempty"`
	Telemetry            contract.HostTelemetry `json:"telemetry"`
}
