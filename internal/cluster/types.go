package cluster

import (
	"context"
	"errors"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	FederationProtocol             = "v1"
	FederationProtocolV2           = "v2"
	LightNodeProtocol              = "light-v1"
	SummaryScope                   = "cluster.summary.read"
	SummaryTerminalScope           = "cluster.summary.read cluster.terminal.open"
	SummaryTerminalFilesScope      = "cluster.summary.read cluster.terminal.open cluster.files.read"
	SummaryTerminalFilesTasksScope = "cluster.summary.read cluster.terminal.open cluster.files.read cluster.system.maintenance"
	LocalHostID                    = "local"
	MaxHosts                       = 100
	MaxSummaryBytes                = 64 << 10
	MaxPairBytes                   = 16 << 10
	MaxFederationV2Bytes           = 96 << 10
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
	ErrLightBatchInvalid      = errors.New("light node batch enrollment settings are invalid")
	ErrPrivateOrigin          = errors.New("cluster origin is outside the configured private network allowlist")
	ErrPairingCode            = errors.New("pairing code is invalid or expired")
	ErrAuthentication         = errors.New("federation authentication failed")
	ErrReplay                 = errors.New("federation request replayed")
	ErrRateLimited            = errors.New("federation request rate limited")
	ErrProtocolMismatch       = errors.New("federation protocol is incompatible")
	ErrMutualFilesUnsupported = errors.New("mutual file transfer is unsupported")
	ErrIdentityMismatch       = errors.New("federation target identity changed")
	ErrLocalHost              = errors.New("local cluster host cannot be modified")
	ErrBatchTaskInvalid       = errors.New("cluster batch task is invalid")
	ErrBatchTaskBusy          = errors.New("cluster batch task capacity reached")
	ErrBatchTaskState         = errors.New("cluster batch task state does not allow this operation")
	ErrBatchTaskUnsupported   = errors.New("cluster batch task action is unsupported")
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
	// Health is ephemeral: old centers must still read persisted snapshots on rollback.
	LightHealth            *contract.LightNodeHealth `json:"-"`
	Telemetry              contract.HostTelemetry    `json:"telemetry"`
	ReceivedAt             time.Time                 `json:"receivedAt"`
	LatencyMilliseconds    int64                     `json:"latencyMilliseconds"`
	ReceiveBytesPerSecond  float64                   `json:"receiveBytesPerSecond"`
	TransmitBytesPerSecond float64                   `json:"transmitBytesPerSecond"`
}

type Host struct {
	LightHealth                 *contract.LightNodeHealth `json:"lightHealth,omitempty"`
	ID                          string                    `json:"id"`
	IsLocal                     bool                      `json:"isLocal"`
	Name                        string                    `json:"name"`
	Kind                        HostKind                  `json:"kind"`
	Origin                      string                    `json:"origin"`
	TransportSecurity           TransportSecurity         `json:"transportSecurity"`
	PeerFingerprint             string                    `json:"peerFingerprint,omitempty"`
	RemoteNodeID                string                    `json:"remoteNodeId"`
	FederationProtocol          string                    `json:"federationProtocol"`
	Scope                       string                    `json:"scope"`
	TerminalAvailable           bool                      `json:"terminalAvailable"`
	FileManagementAvailable     bool                      `json:"fileManagementAvailable"`
	FileTransferAvailable       bool                      `json:"fileTransferAvailable"`
	MutualFileTransferAvailable bool                      `json:"mutualFileTransferAvailable"`
	BatchTaskAvailable          bool                      `json:"batchTaskAvailable"`
	PanelVersion                string                    `json:"panelVersion,omitempty"`
	SecurityEntrancePath        string                    `json:"securityEntrancePath,omitempty"`
	State                       HostState                 `json:"state"`
	LastSnapshot                *HostSnapshot             `json:"lastSnapshot,omitempty"`
	LastAttemptAt               *time.Time                `json:"lastAttemptAt,omitempty"`
	LastSuccessAt               *time.Time                `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures         int                       `json:"consecutiveFailures"`
	LastErrorCode               string                    `json:"lastErrorCode,omitempty"`
	LastError                   string                    `json:"lastError,omitempty"`
	Polling                     bool                      `json:"polling"`
	NextPollAt                  *time.Time                `json:"nextPollAt,omitempty"`
	ResourceVersion             string                    `json:"resourceVersion"`
	CreatedAt                   time.Time                 `json:"createdAt"`
	UpdatedAt                   time.Time                 `json:"updatedAt"`
}

func ScopeAllowsTerminal(scope string) bool {
	return scope == SummaryTerminalScope || scope == SummaryTerminalFilesScope ||
		scope == SummaryTerminalFilesTasksScope
}

func ScopeAllowsFiles(scope string) bool {
	return scope == SummaryTerminalFilesScope || scope == SummaryTerminalFilesTasksScope
}

func ScopeAllowsBatchTasks(scope string) bool {
	return scope == SummaryTerminalFilesTasksScope
}

type BatchAction string

const (
	BatchActionRefresh         BatchAction = "refresh"
	BatchActionSystemUpdate    BatchAction = "system-update"
	BatchActionCleanupCache    BatchAction = "cleanup-cache"
	BatchActionCleanupStandard BatchAction = "cleanup-standard"
	BatchActionLogsRetain7Days BatchAction = "logs-retain-7d"
	BatchActionLogsRetain3Days BatchAction = "logs-retain-3d"
	BatchActionLogsMax500MiB   BatchAction = "logs-max-500m"
	BatchActionReboot          BatchAction = "reboot"
)

type BatchActionRisk string

const (
	BatchActionRiskRead       BatchActionRisk = "read"
	BatchActionRiskWrite      BatchActionRisk = "write"
	BatchActionRiskDisruptive BatchActionRisk = "disruptive"
)

type BatchActionDefinition struct {
	ID                   BatchAction     `json:"id"`
	Risk                 BatchActionRisk `json:"risk"`
	RequiresTaskScope    bool            `json:"requiresTaskScope"`
	SupportsLegacyPanels bool            `json:"supportsLegacyPanels"`
	SupportsLightNodes   bool            `json:"supportsLightNodes"`
}

type BatchTaskLimits struct {
	MaxTasks              int `json:"maxTasks"`
	MaxActiveTasks        int `json:"maxActiveTasks"`
	MaxTargets            int `json:"maxTargets"`
	MaxConcurrency        int `json:"maxConcurrency"`
	MinTimeoutSeconds     int `json:"minTimeoutSeconds"`
	MaxTimeoutSeconds     int `json:"maxTimeoutSeconds"`
	DefaultTimeoutSeconds int `json:"defaultTimeoutSeconds"`
}

type BatchTaskCatalog struct {
	Actions []BatchActionDefinition `json:"actions"`
	Limits  BatchTaskLimits         `json:"limits"`
}

type BatchTaskState string

const (
	BatchTaskQueued         BatchTaskState = "queued"
	BatchTaskRunning        BatchTaskState = "running"
	BatchTaskCancelling     BatchTaskState = "cancelling"
	BatchTaskSucceeded      BatchTaskState = "succeeded"
	BatchTaskPartial        BatchTaskState = "partial"
	BatchTaskFailed         BatchTaskState = "failed"
	BatchTaskCancelled      BatchTaskState = "cancelled"
	BatchTaskNeedsAttention BatchTaskState = "needs_attention"
)

type BatchTargetState string

const (
	BatchTargetQueued         BatchTargetState = "queued"
	BatchTargetSubmitting     BatchTargetState = "submitting"
	BatchTargetRunning        BatchTargetState = "running"
	BatchTargetSucceeded      BatchTargetState = "succeeded"
	BatchTargetFailed         BatchTargetState = "failed"
	BatchTargetCancelled      BatchTargetState = "cancelled"
	BatchTargetUnsupported    BatchTargetState = "unsupported"
	BatchTargetNeedsAttention BatchTargetState = "needs_attention"
)

type BatchTaskTarget struct {
	HostID      string           `json:"hostId"`
	HostName    string           `json:"hostName"`
	HostKind    HostKind         `json:"hostKind"`
	OperationID string           `json:"operationId"`
	ExecutionID string           `json:"executionId,omitempty"`
	State       BatchTargetState `json:"state"`
	Stage       string           `json:"stage,omitempty"`
	Progress    int              `json:"progress"`
	Message     string           `json:"message,omitempty"`
	ErrorCode   string           `json:"errorCode,omitempty"`
	StartedAt   *time.Time       `json:"startedAt,omitempty"`
	FinishedAt  *time.Time       `json:"finishedAt,omitempty"`
}

type BatchTask struct {
	ID              string            `json:"id"`
	ParentTaskID    string            `json:"parentTaskId,omitempty"`
	Action          BatchAction       `json:"action"`
	State           BatchTaskState    `json:"state"`
	Concurrency     int               `json:"concurrency"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	CancelRequested bool              `json:"cancelRequested"`
	Total           int               `json:"total"`
	Completed       int               `json:"completed"`
	Succeeded       int               `json:"succeeded"`
	Failed          int               `json:"failed"`
	NeedsAttention  int               `json:"needsAttention"`
	Cancelled       int               `json:"cancelled"`
	Unsupported     int               `json:"unsupported"`
	Targets         []BatchTaskTarget `json:"targets,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	StartedAt       *time.Time        `json:"startedAt,omitempty"`
	FinishedAt      *time.Time        `json:"finishedAt,omitempty"`
}

type BatchTaskList struct {
	Items []BatchTask `json:"items"`
	Total int         `json:"total"`
}

type CreateBatchTaskInput struct {
	Action            BatchAction `json:"action"`
	HostIDs           []string    `json:"hostIds"`
	Concurrency       int         `json:"concurrency,omitempty"`
	TimeoutSeconds    int         `json:"timeoutSeconds,omitempty"`
	ConfirmDisruptive bool        `json:"confirmDisruptive,omitempty"`
}

type RetryBatchTaskInput struct {
	ConfirmDisruptive bool `json:"confirmDisruptive,omitempty"`
}

type BatchActionInvocation struct {
	Action       BatchAction
	OperationID  string
	ControllerID string
}

type BatchTargetExecution struct {
	ExecutionID string           `json:"executionId,omitempty"`
	State       BatchTargetState `json:"state"`
	Stage       string           `json:"stage,omitempty"`
	Progress    int              `json:"progress"`
	Message     string           `json:"message,omitempty"`
	ErrorCode   string           `json:"errorCode,omitempty"`
}

type BatchActionBackend interface {
	Submit(context.Context, BatchActionInvocation) (BatchTargetExecution, error)
	Status(context.Context, BatchActionInvocation, string) (BatchTargetExecution, error)
}

type LightEnrollment struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CreateLightBatchEnrollmentInput struct {
	NamePrefix       string `json:"namePrefix,omitempty"`
	MaxUses          int    `json:"maxUses,omitempty"`
	ExpiresInSeconds int    `json:"expiresInSeconds,omitempty"`
}

type LightBatchEnrollment struct {
	ID             string    `json:"id"`
	Command        string    `json:"command,omitempty"`
	NamePrefix     string    `json:"namePrefix,omitempty"`
	MaxUses        int       `json:"maxUses"`
	UsedCount      int       `json:"usedCount"`
	RemainingCount int       `json:"remainingCount"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type LightBatchEnrollmentList struct {
	Items []LightBatchEnrollment `json:"items"`
	Total int                    `json:"total"`
}

type LightEnrollRequest struct {
	Token             string `json:"token"`
	Name              string `json:"name,omitempty"`
	NodeVersion       string `json:"nodeVersion"`
	TerminalPublicKey string `json:"terminalPublicKey,omitempty"`
	AttemptID         string `json:"attemptId,omitempty"`
}

type LightEnrollResponse struct {
	NodeID                string `json:"nodeId"`
	ReportingKey          string `json:"reportingKey"`
	ReportInterval        int    `json:"reportIntervalSeconds"`
	TerminalPeerPublicKey string `json:"terminalPeerPublicKey,omitempty"`
	TargetNodeID          string `json:"targetNodeId,omitempty"`
}

// LightFileCapabilityRequest upgrades an already enrolled lightweight node to
// the root file broker without consuming another enrollment token.
type LightFileCapabilityRequest struct {
	TerminalPublicKey string `json:"terminalPublicKey"`
}

type LightFileCapabilityResponse struct {
	TerminalPeerPublicKey string `json:"terminalPeerPublicKey"`
	TargetNodeID          string `json:"targetNodeId"`
}

type LightReportRequest struct {
	Telemetry contract.HostTelemetry    `json:"telemetry"`
	Health    *contract.LightNodeHealth `json:"health,omitempty"`
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
