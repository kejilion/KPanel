package contract

import "time"

// Problem is the stable error envelope used by the public and local APIs.
type Problem struct {
	Type        string            `json:"type,omitempty"`
	Title       string            `json:"title"`
	Status      int               `json:"status"`
	Code        string            `json:"code"`
	Detail      string            `json:"detail,omitempty"`
	RequestID   string            `json:"requestId"`
	Retryable   bool              `json:"retryable"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}

type PageResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type AgentHealth struct {
	Status          string    `json:"status"`
	Version         string    `json:"version"`
	ProtocolVersion string    `json:"protocolVersion"`
	ReadOnly        bool      `json:"readOnly"`
	Reasons         []string  `json:"reasons,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
}

// CoreReady reports whether the Agent can safely serve the Panel's core
// system, Docker, and application APIs. A missing Kejilion web root only
// disables website capabilities and must not prevent standalone installation.
func (health AgentHealth) CoreReady() bool {
	if health.Status == "ok" {
		return len(health.Reasons) == 0
	}
	return health.Status == "degraded" &&
		len(health.Reasons) == 1 &&
		health.Reasons[0] == "web_root_unavailable"
}

type Capability struct {
	ID      string   `json:"id"`
	Enabled bool     `json:"enabled"`
	Reason  string   `json:"reason,omitempty"`
	Methods []string `json:"methods,omitempty"`
}

type SystemSummary struct {
	Hostname      string                  `json:"hostname"`
	OS            string                  `json:"os"`
	OSID          string                  `json:"osId,omitempty"`
	OSLike        []string                `json:"osLike,omitempty"`
	Kernel        string                  `json:"kernel"`
	Architecture  string                  `json:"architecture"`
	UptimeSeconds uint64                  `json:"uptimeSeconds"`
	Load          LoadSummary             `json:"load"`
	CPU           CPUSummary              `json:"cpu"`
	Memory        MemorySummary           `json:"memory"`
	Disks         []DiskSummary           `json:"disks"`
	DiskIO        DiskIOSummary           `json:"diskIo"`
	Network       NetworkSummary          `json:"network"`
	PublicNetwork PublicNetworkSummary    `json:"publicNetwork"`
	Management    SystemManagementSummary `json:"management"`
	CollectedAt   time.Time               `json:"collectedAt"`
}

// SystemManagementSummary describes configuration observed on the host. It is
// deliberately read-only: mutations are exposed as separate typed capabilities.
type SystemManagementSummary struct {
	SSH                SSHConfiguration          `json:"ssh"`
	DNS                DNSConfiguration          `json:"dns"`
	Timezone           string                    `json:"timezone,omitempty"`
	Swap               SwapConfiguration         `json:"swap"`
	PackageManager     string                    `json:"packageManager,omitempty"`
	PackageSources     []string                  `json:"packageSources,omitempty"`
	Maintenance        SystemMaintenanceSummary  `json:"maintenance"`
	IPPreference       string                    `json:"ipPreference"`
	KernelOptimization KernelOptimizationSummary `json:"kernelOptimization"`
	BBR                BBRSummary                `json:"bbr"`
	BBRv3              BBRv3Summary              `json:"bbrv3"`
}

type SystemMaintenanceSummary struct {
	ID             string     `json:"id,omitempty"`
	State          string     `json:"state"`
	Action         string     `json:"action,omitempty"`
	Policy         string     `json:"policy,omitempty"`
	Stage          string     `json:"stage,omitempty"`
	Progress       int        `json:"progress"`
	Message        string     `json:"message,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	RebootRequired bool       `json:"rebootRequired"`
}

type SSHConfiguration struct {
	Ports   []uint16         `json:"ports"`
	Source  string           `json:"source"`
	Defense SSHDefenseStatus `json:"defense"`
}

type SSHDefenseStatus struct {
	Available bool   `json:"available"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
	Autostart bool   `json:"autostart"`
	Jail      string `json:"jail,omitempty"`
	Banned    int    `json:"banned"`
	Message   string `json:"message,omitempty"`
}

type DNSConfiguration struct {
	Servers []string `json:"servers"`
	Manager string   `json:"manager"`
}

type SwapConfiguration struct {
	ActiveDevices       int    `json:"activeDevices"`
	Path                string `json:"path"`
	FileExists          bool   `json:"fileExists"`
	FileActive          bool   `json:"fileActive"`
	FileSizeBytes       uint64 `json:"fileSizeBytes"`
	FileUsedBytes       uint64 `json:"fileUsedBytes"`
	LegacyExists        bool   `json:"legacyExists"`
	LegacyActive        bool   `json:"legacyActive"`
	LegacySizeBytes     uint64 `json:"legacySizeBytes"`
	OtherActiveDevices  int    `json:"otherActiveDevices"`
	OtherSwapTotalBytes uint64 `json:"otherSwapTotalBytes"`
	OtherSwapUsedBytes  uint64 `json:"otherSwapUsedBytes"`
}

type KernelOptimizationSummary struct {
	Enabled bool   `json:"enabled"`
	Profile string `json:"profile,omitempty"`
	Source  string `json:"source,omitempty"`
}

type BBRSummary struct {
	Supported         bool     `json:"supported"`
	Enabled           bool     `json:"enabled"`
	CongestionControl string   `json:"congestionControl,omitempty"`
	DefaultQDisc      string   `json:"defaultQDisc,omitempty"`
	Available         []string `json:"available,omitempty"`
}

type BBRv3Summary struct {
	Available         bool   `json:"available"`
	Supported         bool   `json:"supported"`
	Installed         bool   `json:"installed"`
	Active            bool   `json:"active"`
	Architecture      string `json:"architecture,omitempty"`
	OS                string `json:"os,omitempty"`
	Codename          string `json:"codename,omitempty"`
	RunningKernel     string `json:"runningKernel,omitempty"`
	InstalledKernel   string `json:"installedKernel,omitempty"`
	CongestionControl string `json:"congestionControl,omitempty"`
	DefaultQDisc      string `json:"defaultQDisc,omitempty"`
	RebootRequired    bool   `json:"rebootRequired"`
	Reason            string `json:"reason,omitempty"`
}

// SystemActionRequest is the only mutation envelope accepted by the Agent.
// The Action value selects exactly one typed field set; unknown JSON fields are
// rejected by the HTTP decoder and arbitrary commands are never accepted.
type SystemActionRequest struct {
	Action            string   `json:"action"`
	Hostname          string   `json:"hostname,omitempty"`
	Port              uint16   `json:"port,omitempty"`
	Servers           []string `json:"servers,omitempty"`
	Timezone          string   `json:"timezone,omitempty"`
	SwapSizeMiB       int      `json:"swapSizeMiB,omitempty"`
	MirrorPreset      string   `json:"mirrorPreset,omitempty"`
	Preference        string   `json:"preference,omitempty"`
	Profile           string   `json:"profile,omitempty"`
	MaintenancePolicy string   `json:"maintenancePolicy,omitempty"`
	Confirmation      string   `json:"confirmation,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	PID               int      `json:"pid,omitempty"`
	StartTimeTicks    uint64   `json:"startTimeTicks,omitempty"`
	Signal            string   `json:"signal,omitempty"`
}

type SystemActionResult struct {
	Action            string    `json:"action"`
	Status            string    `json:"status"`
	Changed           bool      `json:"changed"`
	Message           string    `json:"message"`
	BackupPath        string    `json:"backupPath,omitempty"`
	TaskID            string    `json:"taskId,omitempty"`
	MaintenancePolicy string    `json:"maintenancePolicy,omitempty"`
	AppliedAt         time.Time `json:"appliedAt"`
}

type LoadSummary struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type CPUSummary struct {
	Model        string  `json:"model,omitempty"`
	Cores        int     `json:"cores"`
	FrequencyMHz float64 `json:"frequencyMHz,omitempty"`
	UsagePercent float64 `json:"usagePercent"`
}

type MemorySummary struct {
	TotalBytes     uint64  `json:"totalBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	UsagePercent   float64 `json:"usagePercent"`
	SwapTotalBytes uint64  `json:"swapTotalBytes"`
	SwapUsedBytes  uint64  `json:"swapUsedBytes"`
}

type DiskSummary struct {
	Device       string  `json:"device"`
	MountPoint   string  `json:"mountPoint"`
	FileSystem   string  `json:"fileSystem"`
	TotalBytes   uint64  `json:"totalBytes"`
	UsedBytes    uint64  `json:"usedBytes"`
	UsagePercent float64 `json:"usagePercent"`
}

// DiskIOSummary contains monotonically increasing host block-device counters.
// Consumers derive rates from samples so collection remains a single bounded
// read of /proc/diskstats without an additional sleep or process invocation.
type DiskIOSummary struct {
	Available  bool   `json:"available"`
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
}

type NetworkSummary struct {
	ReceivedBytes  uint64 `json:"receivedBytes"`
	SentBytes      uint64 `json:"sentBytes"`
	TCPConnections int    `json:"tcpConnections"`
	UDPConnections int    `json:"udpConnections"`
}

type PublicNetworkSummary struct {
	IPv4        string     `json:"ipv4,omitempty"`
	IPv6        string     `json:"ipv6,omitempty"`
	ISP         string     `json:"isp,omitempty"`
	Country     string     `json:"country,omitempty"`
	CountryCode string     `json:"countryCode,omitempty"`
	Region      string     `json:"region,omitempty"`
	City        string     `json:"city,omitempty"`
	Timezone    string     `json:"timezone,omitempty"`
	Source      string     `json:"source,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// HostTelemetry is the deliberately narrow, read-only host snapshot used by
// KPanel federation. It excludes system configuration, site, application and
// Docker details so a monitoring controller never receives management state
// or credentials.
type HostTelemetry struct {
	AgentVersion         string               `json:"agentVersion"`
	AgentProtocolVersion string               `json:"agentProtocolVersion"`
	Hostname             string               `json:"hostname"`
	OS                   string               `json:"os"`
	OSID                 string               `json:"osId,omitempty"`
	OSLike               []string             `json:"osLike,omitempty"`
	Kernel               string               `json:"kernel,omitempty"`
	Architecture         string               `json:"architecture,omitempty"`
	UptimeSeconds        uint64               `json:"uptimeSeconds"`
	Load                 LoadSummary          `json:"load"`
	CPU                  CPUSummary           `json:"cpu"`
	Memory               MemorySummary        `json:"memory"`
	Disk                 DiskCapacitySummary  `json:"disk"`
	Network              NetworkSummary       `json:"network"`
	PublicNetwork        PublicNetworkSummary `json:"publicNetwork"`
	CollectedAt          time.Time            `json:"collectedAt"`
}

type DiskCapacitySummary struct {
	TotalBytes   uint64  `json:"totalBytes"`
	UsedBytes    uint64  `json:"usedBytes"`
	UsagePercent float64 `json:"usagePercent"`
}

type Origin string

const (
	OriginWeb        Origin = "web"
	OriginCLI        Origin = "cli"
	OriginDiscovered Origin = "discovered"
	OriginExternal   Origin = "external"
)

type Consistency string

const (
	ConsistencyInSync      Consistency = "in_sync"
	ConsistencyDrifted     Consistency = "drifted"
	ConsistencyAmbiguous   Consistency = "ambiguous"
	ConsistencyConflicted  Consistency = "conflicted"
	ConsistencyUnsupported Consistency = "unsupported"
	ConsistencyReadOnly    Consistency = "read_only"
)

type SiteKind string

const (
	SiteStatic       SiteKind = "static"
	SiteReverseProxy SiteKind = "reverse_proxy"
	SiteDomainProxy  SiteKind = "domain_proxy"
	SiteLoadBalance  SiteKind = "load_balance"
	SitePHP          SiteKind = "php"
	SiteWordPress    SiteKind = "wordpress"
	SiteRedirect     SiteKind = "redirect"
	SiteUnknown      SiteKind = "unknown"
)

type SiteSummary struct {
	ID              string      `json:"id"`
	PrimaryDomain   string      `json:"primaryDomain"`
	Domains         []string    `json:"domains"`
	Kind            SiteKind    `json:"kind"`
	Enabled         bool        `json:"enabled"`
	Health          string      `json:"health"`
	TLS             TLSStatus   `json:"tls"`
	Target          string      `json:"target,omitempty"`
	DocumentRoot    string      `json:"documentRoot,omitempty"`
	Origin          Origin      `json:"origin"`
	Consistency     Consistency `json:"consistency"`
	ResourceVersion string      `json:"resourceVersion"`
	AllowedActions  []string    `json:"allowedActions"`
	Artifacts       []Artifact  `json:"artifacts,omitempty"`
	Warnings        []string    `json:"warnings,omitempty"`
	ReconciledAt    time.Time   `json:"reconciledAt"`
}

type TLSStatus struct {
	Enabled   bool       `json:"enabled"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Source    string     `json:"source,omitempty"`
}

type Artifact struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Hash string `json:"hash,omitempty"`
}

type DockerSummary struct {
	Available     bool      `json:"available"`
	ServerVersion string    `json:"serverVersion,omitempty"`
	Containers    int       `json:"containers"`
	Running       int       `json:"running"`
	Paused        int       `json:"paused"`
	Stopped       int       `json:"stopped"`
	Images        int       `json:"images"`
	CollectedAt   time.Time `json:"collectedAt"`
}

type ContainerSummary struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Image             string            `json:"image"`
	State             string            `json:"state"`
	Status            string            `json:"status"`
	Health            string            `json:"health,omitempty"`
	CreatedAt         *time.Time        `json:"createdAt,omitempty"`
	Ports             []PortBinding     `json:"ports"`
	Mounts            []Mount           `json:"mounts"`
	Networks          []string          `json:"networks"`
	ComposeProject    string            `json:"composeProject,omitempty"`
	ComposeService    string            `json:"composeService,omitempty"`
	Ownership         string            `json:"ownership"`
	OwnershipEvidence []string          `json:"ownershipEvidence,omitempty"`
	ResourceVersion   string            `json:"resourceVersion"`
	AllowedActions    []string          `json:"allowedActions"`
	Labels            map[string]string `json:"-"`
}

type PortBinding struct {
	PrivatePort uint16 `json:"privatePort"`
	PublicPort  uint16 `json:"publicPort,omitempty"`
	IP          string `json:"ip,omitempty"`
	Type        string `json:"type"`
}

type Mount struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readOnly"`
}

type JobState string

const (
	JobQueued               JobState = "queued"
	JobRunning              JobState = "running"
	JobSucceeded            JobState = "succeeded"
	JobFailedRolledBack     JobState = "failed_rolled_back"
	JobFailedNeedsAttention JobState = "failed_needs_attention"
	JobInterrupted          JobState = "interrupted"
	JobCancelled            JobState = "cancelled"
)

type Job struct {
	ID          string     `json:"id"`
	Action      string     `json:"action"`
	Origin      Origin     `json:"origin"`
	State       JobState   `json:"state"`
	Progress    int        `json:"progress,omitempty"`
	Stage       string     `json:"stage,omitempty"`
	TargetKind  string     `json:"targetKind,omitempty"`
	TargetID    string     `json:"targetId,omitempty"`
	TargetLabel string     `json:"targetLabel,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	Error       *Problem   `json:"error,omitempty"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	OccurredAt time.Time      `json:"occurredAt"`
	ActorType  string         `json:"actorType"`
	ActorID    string         `json:"actorId,omitempty"`
	SourceIP   string         `json:"sourceIp,omitempty"`
	Action     string         `json:"action"`
	TargetKind string         `json:"targetKind,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	Result     string         `json:"result"`
	RequestID  string         `json:"requestId"`
	Change     map[string]any `json:"change,omitempty"`
}
