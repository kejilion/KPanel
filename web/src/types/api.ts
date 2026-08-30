export type HealthLevel = 'healthy' | 'warning' | 'critical' | 'unknown'
export type ConsistencyState =
  | 'synced'
  | 'drifted'
  | 'ambiguous'
  | 'conflicted'
  | 'unsupported'
  | 'read_only'
  | 'pending'
  | 'unknown'
export type ResourceAccess = 'managed' | 'read-only' | 'unmanaged'

export interface ApiList<T> {
  items: T[]
  total: number
  nextCursor?: string
}

export interface AgentStatus {
  connected: boolean
  readOnly: boolean
  compatible: boolean
  version?: string
  protocolVersion?: string
  lastSeenAt?: string
  reason?: string
}

export interface User {
  id: string
  username: string
  displayName?: string
  role?: string
  totpEnabled?: boolean
}

export interface AuthStatus {
  setupRequired: boolean
  authenticated: boolean
  user?: User
  csrfToken?: string
  expiresAt?: string
  agent?: AgentStatus
}

export interface SetupRequest {
  token: string
  username: string
  password: string
}

export interface LoginRequest {
  username: string
  password: string
  totpCode?: string
}

export interface AuthSession {
  user: User
  csrfToken?: string
  expiresAt?: string
}

export interface TOTPStatus {
  enabled: boolean
  enabledAt?: string
  recoveryCodesRemaining: number
}

export interface TOTPEnrollment {
  id: string
  secret: string
  otpauthUri: string
  expiresAt: string
}

export interface TOTPRecoveryCodes {
  recoveryCodes: string[]
}

export interface MetricValue {
  value: number
  total?: number
  unit?: string
  percent?: number
  change?: number
}

export interface ServiceStatus {
  id: string
  name: string
  state: 'running' | 'stopped' | 'degraded' | 'unknown'
  version?: string
  detail?: string
}

export interface NetworkSummary {
  receiveBytesPerSecond: number
  transmitBytesPerSecond: number
  rateAvailable: boolean
  totalReceivedBytes?: number
  totalTransmittedBytes?: number
  tcpConnections: number
  udpConnections: number
}

export interface PublicNetworkSummary {
  ipv4?: string
  ipv6?: string
  isp?: string
  country?: string
  countryCode?: string
  region?: string
  city?: string
  timezone?: string
  source?: string
  updatedAt?: string
}

export type ClusterHostState =
  | 'unknown'
  | 'pairing'
  | 'revoking'
  | 'online'
  | 'degraded'
  | 'stale'
  | 'offline'
  | 'auth_failed'
  | 'tls_error'
  | 'incompatible'

export type ClusterTransportSecurity = 'tls' | 'e2e_http'
export type ClusterHostKind = 'panel' | 'light_node'

export interface ClusterTelemetry {
  agentVersion: string
  agentProtocolVersion: string
  hostname: string
  os: string
  osId?: string
  osLike?: string[]
  kernel?: string
  architecture?: string
  uptimeSeconds: number
  load: { one: number; five: number; fifteen: number }
  cpu: {
    model?: string
    cores: number
    frequencyMHz?: number
    usagePercent: number
  }
  memory: {
    totalBytes: number
    availableBytes: number
    usedBytes: number
    usagePercent: number
  }
  disk: {
    totalBytes: number
    usedBytes: number
    usagePercent: number
  }
  network: {
    receivedBytes: number
    sentBytes: number
    tcpConnections: number
    udpConnections: number
  }
  publicNetwork: PublicNetworkSummary
  collectedAt: string
}

export interface ClusterHostSnapshot {
  telemetry: ClusterTelemetry
  receivedAt: string
  latencyMilliseconds: number
  receiveBytesPerSecond: number
  transmitBytesPerSecond: number
}

export interface ClusterHost {
  id: string
  isLocal: boolean
  name: string
  kind?: ClusterHostKind
  origin: string
  transportSecurity: ClusterTransportSecurity
  peerFingerprint?: string
  remoteNodeId: string
  federationProtocol: string
  scope: string
  terminalAvailable: boolean
  fileTransferAvailable?: boolean
  mutualFileTransferAvailable: boolean
  panelVersion?: string
  securityEntrancePath?: string
  state: ClusterHostState
  lastSnapshot?: ClusterHostSnapshot
  lastAttemptAt?: string
  lastSuccessAt?: string
  consecutiveFailures: number
  lastErrorCode?: string
  lastError?: string
  polling: boolean
  nextPollAt?: string
  resourceVersion: string
  createdAt: string
  updatedAt: string
}

export interface ClusterHostList {
  items: ClusterHost[]
  total: number
  remoteTotal: number
  maxHosts: number
  pollIntervalSeconds: number
  nodeId: string
}

export interface ClusterShareSettings {
	enabled: boolean
	title: string
	description: string
	sharePath?: string
	resourceVersion: string
	updatedAt?: string
}

export type PublicClusterShareHostState = 'online' | 'degraded' | 'offline' | 'pending'

export interface PublicClusterShareHost {
	id: string
	name: string
	state: PublicClusterShareHostState
	os?: string
	architecture?: string
	uptimeSeconds?: number
	load: { one: number; five: number; fifteen: number }
	cpu: { cores: number; usagePercent: number }
	memory: { totalBytes: number; usedBytes: number; usagePercent: number }
	disk: { totalBytes: number; usedBytes: number; usagePercent: number }
	network: { receiveBytesPerSecond: number; transmitBytesPerSecond: number }
	location: {
		isp?: string
		country?: string
		countryCode?: string
		region?: string
		city?: string
	}
	collectedAt?: string
}

export interface PublicClusterShareSnapshot {
	title: string
	description?: string
	generatedAt: string
	total: number
	online: number
	attention: number
	items: PublicClusterShareHost[]
}

export interface ClusterPairingCode {
  code: string
  scope:
    | 'cluster.summary.read'
    | 'cluster.summary.read cluster.terminal.open'
    | 'cluster.summary.read cluster.terminal.open cluster.files.read'
  expiresAt: string
}

export interface TerminalSession {
  sessionId: string
  hostId: string
  offset: number
  createdAt: string
}

export interface TerminalOutput {
  data: string
  offset: number
  nextOffset: number
  truncated: boolean
  exitedAt?: string
  exitError?: string
  closed: boolean
}

export interface ClusterLightEnrollment {
  command: string
  expiresAt: string
}

export interface ClusterController {
  id: string
  name?: string
  fingerprint: string
  scope: string
  createdAt: string
  lastSeenAt?: string
}

export interface CapabilityState {
  enabled: boolean
  reason?: string
  methods?: string[]
}

export interface SystemManagement {
  ssh: {
    ports: number[]
    source: 'configured' | 'default' | 'unknown'
    defense: {
      available: boolean
      installed: boolean
      running: boolean
      enabled: boolean
      autostart: boolean
      jail?: string
      banned: number
      message?: string
    }
  }
  dns: {
    servers: string[]
    manager: string
  }
  timezone?: string
  swap: {
    totalBytes: number
    usedBytes: number
    activeDevices: number
    path: string
    fileExists: boolean
    fileActive: boolean
    fileSizeBytes: number
    fileUsedBytes: number
    legacyExists: boolean
    legacyActive: boolean
    legacySizeBytes: number
    otherActiveDevices: number
    otherSwapTotalBytes: number
    otherSwapUsedBytes: number
  }
  packageManager?: string
  packageSources: string[]
  maintenance: {
    id?: string
    state: 'idle' | 'running' | 'succeeded' | 'failed'
    action?: 'update' | 'cleanup' | 'ssh-defense' | 'bbrv3' | 'system-tuning' | 'log-cleanup'
    policy?: string
    stage?: string
    progress: number
    message?: string
    startedAt?: string
    finishedAt?: string
    rebootRequired: boolean
  }
  ipPreference: 'ipv4' | 'system_default' | 'unknown'
  kernelOptimization: {
    enabled: boolean
    profile?: string
    source?: string
  }
  bbr: {
    supported: boolean
    enabled: boolean
    congestionControl?: string
    defaultQDisc?: string
    available: string[]
  }
  bbrv3: {
    available: boolean
    supported: boolean
    installed: boolean
    active: boolean
    architecture?: string
    os?: string
    codename?: string
    runningKernel?: string
    installedKernel?: string
    congestionControl?: string
    defaultQDisc?: string
    rebootRequired: boolean
    reason?: string
  }
  capabilities: Record<string, CapabilityState>
}

export interface SystemOverview {
  hostname: string
  os: string
  osId?: string
  osLike: string[]
  kernel?: string
  architecture?: string
  uptimeSeconds: number
  observedAt: string
  cpu: MetricValue & {
    model?: string
    cores: number
    frequencyMHz?: number
  }
  memory: MetricValue
  disk: MetricValue
  load: MetricValue & {
    one: number
    five: number
    fifteen: number
  }
  network: NetworkSummary
  publicNetwork: PublicNetworkSummary
  management: SystemManagement
  services: ServiceStatus[]
  agent: AgentStatus
  sites?: {
    total: number
    healthy: number
    drifted: number
  }
  containers?: {
    total: number
    running: number
    stopped: number
  }
  apps?: {
    total: number
    installed: number
    running: number
    updateAvailable: number
  }
}

export type SystemLogSource = 'system' | 'service' | 'security' | 'login'
export type SystemLogPriority = 'all' | 'warning' | 'error'
export type SystemLogLimit = 50 | 100 | 200

export interface SystemLogAvailability {
  available: boolean
  reason?: string
}

export interface SystemLogsSummary {
  observedAt: string
  varLog: SystemLogAvailability & { bytes?: number }
  journal: SystemLogAvailability & { bytes?: number }
  sources: {
    journal: SystemLogAvailability
    login: SystemLogAvailability
    security: SystemLogAvailability
  }
  authSource?: string
  maintenance: SystemManagement['maintenance']
}

export interface SystemLogEntry {
  timestamp?: string
  cursor?: string
  priority?: string
  unit?: string
  identifier?: string
  pid?: number
  message: string
}

export type SystemLogQuery =
  | { source: 'system'; limit: SystemLogLimit; priority: SystemLogPriority; unit?: never }
  | { source: 'service'; limit: SystemLogLimit; priority: SystemLogPriority; unit?: never }
  | { source: 'security' | 'login'; limit: SystemLogLimit; priority?: never; unit?: never }

export interface SystemLogEntries {
  source: SystemLogSource
  unit?: string
  authSource?: string
  entries: SystemLogEntry[]
  truncated: boolean
  observedAt: string
}

export type ProcessSort = 'cpu' | 'memory' | 'pid' | 'name' | 'user' | 'state' | 'threads'
export type ProcessOrder = 'asc' | 'desc'

export interface ProcessQuery {
  search?: string
  sort?: ProcessSort
  order?: ProcessOrder
  limit?: number
}

export interface ProcessMetric {
  pid: number
  parentPid: number
  name: string
  state: string
  userId: number
  user?: string
  cpuPercent: number
  memoryBytes: number
  threads: number
  nice: number
  startTimeTicks: number
}

export interface ProcessSummary {
  cpuPercent: number
  memoryUsedBytes: number
  memoryTotalBytes: number
  total: number
  running: number
  sleeping: number
  stopped: number
  zombie: number
}

export interface ProcessSnapshot {
  items: ProcessMetric[]
  total: number
  summary: ProcessSummary
  scanned: number
  truncated: boolean
  sampleDuration: number
  collectedAt: string
}

export interface SystemActionInput {
  action:
    | 'hostname'
    | 'ssh-port'
    | 'ssh-defense'
    | 'dns'
    | 'timezone'
    | 'process-signal'
    | 'swap'
    | 'mirror'
    | 'ip-preference'
    | 'kernel-tuning'
    | 'bbr'
    | 'bbrv3'
    | 'update'
    | 'cleanup'
    | 'log-cleanup'
    | 'reboot'
  hostname?: string
  port?: number
  servers?: string[]
  timezone?: string
  swapSizeMiB?: number
  mirrorPreset?: 'cn-default' | 'cn-edu' | 'abroad' | 'smart'
  preference?: 'ipv4' | 'system_default'
  profile?: 'high' | 'balanced' | 'web' | 'stream' | 'game' | 'off'
  maintenancePolicy?:
    | 'full'
    | 'cache'
    | 'standard'
    | 'install'
    | 'update'
    | 'uninstall'
    | 'retain-7d'
    | 'retain-3d'
    | 'max-500m'
  enabled?: boolean
  pid?: number
  startTimeTicks?: number
  signal?: 'term' | 'kill'
}

export interface SystemActionResult {
  action: string
  status: string
  changed: boolean
  message: string
  taskId?: string
  maintenancePolicy?: SystemActionInput['maintenancePolicy']
  backupPath?: string
  appliedAt: string
}

export interface SystemResourceSnapshot<Entry> {
  resourceVersion: string
  entries: Entry[]
  total: number
  truncated: boolean
}

export interface HostsEntry {
  line: number
  address: string
  hostnames: string[]
  comment?: string
  raw: string
}

export type HostsSnapshot = SystemResourceSnapshot<HostsEntry>

export interface CronEntry {
  line: number
  kind: string
  expression: string
  command: string
  raw: string
}

export type CronSnapshot = SystemResourceSnapshot<CronEntry>

export interface NetworkInterfaceEntry {
  name: string
  state: string
  macAddress?: string
  addresses: string[]
  loopback: boolean
  resourceVersion: string
}

export type NetworkInterfacesSnapshot = SystemResourceSnapshot<NetworkInterfaceEntry>

export interface FirewallRule {
  line: number
  chain: string
  target: string
  protocol: string
  source: string
  destination: string
  options: string[]
  raw: string
}

export interface FirewallSnapshot {
  resourceVersion: string
  backend: string
  inputPolicy: string
  rules: FirewallRule[]
  total: number
  truncated: boolean
  pingAllowed: boolean
  ddosEnabled: boolean
}

export type SystemResourceActionInput =
  | {
      action: 'hosts-add'
      address: string
      hostnames: string[]
      comment?: string
      expectedResourceVersion: string
    }
  | { action: 'hosts-delete'; line: number; expectedResourceVersion: string }
  | {
      action: 'cron-add'
      expression: string
      command: string
      expectedResourceVersion: string
    }
  | {
      action: 'cron-update'
      line: number
      expression: string
      command: string
      expectedResourceVersion: string
    }
  | { action: 'cron-delete'; line: number; expectedResourceVersion: string }
  | {
      action: 'network-interface-state'
      interfaceName: string
      enabled: boolean
      expectedResourceVersion: string
    }
  | {
      action: 'firewall-open-port' | 'firewall-close-port'
      port: number
      expectedResourceVersion: string
    }
  | {
      action: 'firewall-allow-ip' | 'firewall-block-ip' | 'firewall-remove-ip'
      address: string
      expectedResourceVersion: string
    }
  | {
      action:
        | 'firewall-open-all'
        | 'firewall-close-all'
        | 'firewall-enable-ping'
        | 'firewall-disable-ping'
        | 'firewall-enable-ddos'
        | 'firewall-disable-ddos'
      expectedResourceVersion: string
    }

export interface SystemResourceActionResult {
  action: SystemResourceActionInput['action']
  status: string
  changed: boolean
  message: string
  backupPath?: string
  resourceVersion: string
  appliedAt: string
}

export interface PortUsageEntry {
  protocol: string
  state: string
  localAddress: string
  localPort: string
  peerAddress: string
  peerPort: string
  process?: string
  pid?: number
  raw: string
}

export interface PortUsageSnapshot {
  resourceVersion: string
  entries: PortUsageEntry[]
  total: number
  truncated: boolean
  observedAt: string
}

export interface TrafficShutdownSnapshot {
  resourceVersion: string
  enabled: boolean
  health: 'disabled' | 'ready' | 'inconsistent'
  rxBytes: number
  txBytes: number
  rxThresholdGiB: number
  txThresholdGiB: number
  resetDay: number
  observedAt: string
}

export type TrafficShutdownActionInput =
  | {
      action: 'enable'
      expectedResourceVersion: string
      rxThresholdGiB: number
      txThresholdGiB: number
      resetDay: number
    }
  | { action: 'disable'; expectedResourceVersion: string }

export interface TrafficShutdownActionResult {
  action: TrafficShutdownActionInput['action']
  status: string
  changed: boolean
  message: string
  backupPath?: string
  resourceVersion: string
  appliedAt: string
}

export type SystemTuningItemID =
	| 'system-update'
	| 'system-cleanup'
	| 'swap-1g'
	| 'ssh-port-5522'
	| 'ssh-defense'
	| 'firewall-open-all'
	| 'bbr'
	| 'timezone-shanghai'
	| 'dns-auto'
	| 'ipv4-preferred'
	| 'basic-tools'
	| 'kernel-auto'

export interface SystemTuningSnapshot {
	resourceVersion: string
	items: Array<{ id: SystemTuningItemID; state: 'ready' | 'pending' }>
	maintenance: SystemManagement['maintenance']
	observedAt: string
}

export interface SystemTuningActionInput {
	action: 'apply'
	items: SystemTuningItemID[]
	expectedResourceVersion: string
}

export interface SystemTuningActionResult {
	action: 'apply'
	items: SystemTuningItemID[]
	status: string
	changed: boolean
	message: string
	resourceVersion: string
	acceptedAt: string
}

export type DiskManagementAction = 'mount' | 'unmount' | 'format' | 'check' | 'repair'
export type DiskFilesystem = 'ext4' | 'xfs' | 'ntfs' | 'vfat'

export interface DiskOperationAvailability {
	enabled: boolean
	reason?: string
}

export interface DiskMount {
	path: string
	persistent: boolean
	totalBytes?: number
	usedBytes?: number
	availableBytes?: number
	usagePercent?: number
}

export interface DiskDevice {
	id: string
	path: string
	name: string
	type: string
	parentId?: string
	sizeBytes: number
	readOnly: boolean
	removable: boolean
	virtual: boolean
	model?: string
	serial?: string
	transport?: string
	filesystem?: {
		type: string
		version?: string
		label?: string
		uuid?: string
		partUuid?: string
	}
	mounts: DiskMount[]
	protected: boolean
	protectionReasons: string[]
	operations: Record<DiskManagementAction, DiskOperationAvailability>
}

export interface DiskManagementJob {
	id: string
	action: DiskManagementAction
	deviceId: string
	devicePath: string
	status: 'queued' | 'running' | 'succeeded' | 'failed' | 'needs_attention'
	stage: string
	progress: number
	message: string
	recoveryPath?: string
	createdAt: string
	startedAt?: string
	finishedAt?: string
}

export interface DiskManagementSnapshot {
	resourceVersion: string
	platform: {
		kind: 'linux' | 'wsl1' | 'wsl2' | 'container' | 'unknown'
		label: string
		writable: boolean
		reason?: string
	}
	devices: DiskDevice[]
	job?: DiskManagementJob
	observedAt: string
}

export interface DiskManagementActionInput {
	action: DiskManagementAction
	deviceId: string
	expectedResourceVersion: string
	mountPoint?: string
	persist?: boolean
	removePersistence?: boolean
	filesystem?: DiskFilesystem
}

export interface SSHAuthorizedKey {
	id: string
	type: string
	fingerprint: string
	comment?: string
}

export interface SystemAccount {
	username: string
	uid: number
	gid: number
	home: string
	shell: string
	kind: 'root' | 'human' | 'system'
	passwordStatus: 'enabled' | 'locked' | 'unset' | 'unknown'
	role: 'root' | 'standard' | 'administrator' | 'passwordless-admin'
	groups: string[]
	sshKeys: SSHAuthorizedKey[]
}

export interface AccountManagementSnapshot {
	resourceVersion: string
	accounts: SystemAccount[]
	total: number
	truncated: boolean
	sshPolicy: {
		passwordAuthentication: boolean
		publicKeyAuthentication: boolean
		rootLogin: 'enabled' | 'key-only' | 'disabled' | 'custom'
	}
	observedAt: string
}

type AccountActionBase = { expectedResourceVersion: string }

export type AccountManagementActionInput =
	| (AccountActionBase & { action: 'create'; username: string; role: 'standard' | 'administrator' | 'passwordless-admin'; credential: 'password' | 'key'; secret: string })
	| (AccountActionBase & { action: 'set-password'; username: string; secret: string })
	| (AccountActionBase & { action: 'add-key'; username: string; secret: string })
	| (AccountActionBase & { action: 'delete-key'; username: string; keyId: string })
	| (AccountActionBase & { action: 'set-role'; username: string; role: 'standard' | 'administrator' | 'passwordless-admin' })
	| (AccountActionBase & { action: 'set-ssh-policy'; passwordAuthentication: boolean; rootLogin: 'enabled' | 'key-only' | 'disabled' })
	| (AccountActionBase & { action: 'disable-root' })
	| (AccountActionBase & { action: 'create-admin-disable-root'; username: string; credential: 'password' | 'key'; secret: string })
	| (AccountActionBase & { action: 'delete'; username: string; removeHome: boolean })

export interface AccountManagementActionResult {
	action: AccountManagementActionInput['action']
	status: string
	changed: boolean
	message: string
	backupPath?: string
	resourceVersion: string
	appliedAt: string
}

export interface SSHDefenseEvent {
	occurredAt: string
	action: 'found' | 'ban' | 'unban'
	address: string
}

export interface SSHDefenseSnapshot {
	resourceVersion: string
	installed: boolean
	running: boolean
	enabled: boolean
	autostart: boolean
	jail: 'sshd' | 'alpine-sshd'
	profile: 'mild' | 'standard' | 'strict' | 'custom'
	banTimeSeconds: number
	findTimeSeconds: number
	maxRetry: number
	currentFailed: number
	totalFailed: number
	currentBanned: number
	totalBanned: number
	bannedIps: string[]
	bansTruncated: boolean
	trustedAddresses: string[]
	recentEvents: SSHDefenseEvent[]
	maintenance: SystemManagement['maintenance']
	observedAt: string
}

type SSHDefenseActionBase = { expectedResourceVersion: string }

export type SSHDefenseActionInput =
	| (SSHDefenseActionBase & { action: 'enable' | 'disable' | 'uninstall' | 'unban-all' })
	| (SSHDefenseActionBase & { action: 'set-profile'; profile: 'mild' | 'standard' | 'strict' })
	| (SSHDefenseActionBase & { action: 'add-trusted' | 'remove-trusted' | 'unban'; address: string })

export interface SSHDefenseActionResult {
	action: SSHDefenseActionInput['action']
	status: string
	changed: boolean
	message: string
	backupPath?: string
	resourceVersion: string
	appliedAt: string
}

export interface AppMarketCategory {
  key: string
  zh: string
  zh_tw?: string
  en: string
}

export interface AppActionCapability {
  enabled: boolean
  reason?: string
}

export interface AppMarketRuntime {
  installed: boolean
  state: string
  status?: string
  containerId?: string
  containerName?: string
  image?: string
  ports: Array<{
    privatePort: number
    publicPort?: number
    ip?: string
    type: string
  }>
  accessMode: 'direct' | 'domain_only' | 'unknown' | 'not_applicable'
  updateStatus: 'available' | 'current' | 'check_required' | 'unknown' | 'not_installed'
  resourceVersion?: string
  detectedBy: string[]
  warning?: string
}

export interface AppMarketItem {
  id: string
  num?: number
  source: 'builtin' | 'thirdparty'
  token: string
  name_zh: string
  name_zh_tw?: string
  name_en: string
  desc_zh: string
  desc_zh_tw?: string
  desc_en: string
  cat: string
  url?: string
  icon: string
  iconSha256: string
  slug: string
  addedAt?: string
  defaultPort?: number
  installPortConfigurable?: boolean
  installer: 'declarative' | 'kejilion' | 'guided'
  runtime: AppMarketRuntime
  capabilities: Record<string, AppActionCapability>
}

export interface AppInstallPortStatus {
  port: number
  available: boolean
  conflicts: Array<{
    source: 'docker' | 'listener'
    protocol: string
    container?: string
  }>
  checkedAt: string
}

export interface AppMarketInventory {
  schemaVersion: number
  source: string
  scriptSha256: string
  catalogMode: 'live' | 'cached' | 'embedded'
  catalogWarning?: string
  catalogRefreshedAt?: string
  categories: AppMarketCategory[]
  items: AppMarketItem[]
  installed: number
  running: number
  updateAvailable: number
  collectedAt: string
}

export interface AppMutationResult {
  containerId?: string
  action: string
  status: string
  resourceVersion?: string
}

export interface AppInstallJob {
  id: string
  appId: string
  appName: string
  action: 'install' | 'update' | 'uninstall' | 'direct_access' | 'manage'
  interactive?: boolean
  inputOpen?: boolean
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
  stage: string
  progress: number
  message?: string
  logs: string[]
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface AppTerminalChunk {
  dataBase64: string
  nextOffset: number
  inputOpen: boolean
  finished: boolean
}

export interface AppImageUpdateResult {
  containerId: string
  image: string
  status: 'available' | 'current'
  updateAvailable: boolean
  localDigest?: string
  remoteDigest?: string
  resourceVersion: string
  checkedAt: string
}

export interface DiagnosticCategory {
  id: string
  name: string
}

export interface DiagnosticCheck {
  id: string
  category: string
  name: string
  description: string
  sourceUrl: string
  provider?: 'native' | 'script'
  estimatedMinutes: number
  impact: 'light' | 'network' | 'intensive'
}

export interface DiagnosticCatalog {
  categories: DiagnosticCategory[]
  items: DiagnosticCheck[]
}

export interface DiagnosticSummaryMetric {
  key: string
  value: string
}

export interface DiagnosticSummaryDimension {
  metrics: DiagnosticSummaryMetric[]
}

export interface DiagnosticSummary {
  parser?: string
  reportUrl?: string
  dimensions: Record<string, DiagnosticSummaryDimension>
}

export interface DiagnosticJob {
  id: string
  checkId: string
  checkName: string
  category: string
  sourceUrl: string
  provider?: 'native' | 'script'
  estimatedMinutes: number
  impact: 'light' | 'network' | 'intensive'
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  stage: string
  progress: number
  message?: string
  interactive?: boolean
  inputOpen?: boolean
  logs: string[]
  summary?: DiagnosticSummary
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface CertificateSummary {
  status: 'valid' | 'expiring' | 'expired' | 'missing' | 'unknown'
  issuer?: string
  expiresAt?: string
  daysRemaining?: number
}

export interface Site {
  id: string
  primaryDomain: string
  domains: string[]
  type: 'static' | 'proxy' | 'proxy_domain' | 'load_balance' | 'php' | 'wordpress' | 'redirect' | 'unknown'
  enabled: boolean
  health: HealthLevel
  consistency: ConsistencyState
  access: ResourceAccess
  source: 'kejilion' | 'panel' | 'external' | 'unknown'
  rootPath?: string
  upstream?: string
  certificate?: CertificateSummary
  resourceVersion: string
  observedAt?: string
  reason?: string
  allowedActions?: string[]
  warnings?: string[]
  artifacts?: Array<{
    kind: string
    path: string
    hash?: string
  }>
}

export interface SiteAppearance {
  name?: string
}

export interface SiteInput {
  primaryDomain: string
  aliases?: string[]
  type: 'wordpress' | 'recipe' | 'static' | 'php' | 'proxy' | 'proxy_domain' | 'load_balance' | 'redirect'
  recipe?: 'discuz' | 'kodbox' | 'maccms' | 'dujiaoka' | 'flarum' | 'typecho' | 'linkstack' | 'ai-prompt' | 'bitwarden' | 'halo'
  upstream?: string
  upstreams?: string[]
  redirectTarget?: string
  redirectCode?: 301 | 302 | 307 | 308
  phpVersion?: 'latest' | '7.4'
  enabled?: boolean
  expectedResourceVersion?: string
}

export interface SiteInstallationProgress {
  id?: string
  domain?: string
  recipe?: string
  interactive?: boolean
  inputOpen?: boolean
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  stage: string
  progress: number
  message: string
  events?: Array<{
    stage: string
    progress: number
    message: string
    at: string
  }>
}

export interface SiteDeleteResult {
  id: string
  primaryDomain: string
  status: 'deleted'
  mode: 'full'
  resourceVersion: string
  removed: string[]
  databaseDropped: boolean
  warnings?: string[]
}

export interface WebEnvironmentComponent {
  name: 'nginx' | 'mysql' | 'php' | 'php74' | 'redis' | string
  required: boolean
  exists: boolean
  running: boolean
  state: string
  image?: string
  version?: string
  repoDigest?: string
  updateStatus: 'available' | 'current' | 'unknown' | string
  updateReason?: string
}

export interface WebProtectionSummary {
  fail2ban: boolean
  waf: boolean
  cloudflare: boolean
  ddos: boolean
}

export interface WebOptimizationSummary {
  mode: 'standard' | 'high' | 'custom' | string
  gzip: boolean
  brotli: boolean
  zstd: boolean
}

export interface WebEnvironmentSummary {
  protocolVersion: string
  state: 'absent' | 'partial' | 'installed'
  profile: 'none' | 'full' | 'nginx' | 'custom'
  health: 'unknown' | 'healthy' | 'degraded'
  webRoot: string
  diskBytes: number
  siteCount: number
  databaseCount: number
  certificateCount: number
  composeValid: boolean
  nginxValid: boolean
  resourceVersion: string
  scriptVersion: string
  latestBackup?: string
  portConflicts: string[]
  components: WebEnvironmentComponent[]
  protection: WebProtectionSummary
  optimization: WebOptimizationSummary
  observedAt: string
}

export interface WebEnvironmentCatalog {
  protocolVersion: string
  installProfiles: Array<{ id: 'full' | 'nginx'; label: string }>
  protectionActions: string[]
  optimizationActions: string[]
  updateComponents: Array<{ id: string; versions: string[] }>
}

export interface WebEnvironmentBackup {
  id: string
  sizeBytes: number
  createdAt: string
  verified: boolean
  format: 'kejilion-ldnmp-v1' | 'legacy' | string
}

export interface WebEnvironmentActionInput {
  action:
    | 'install'
    | 'protection.configure'
    | 'optimization.apply'
    | 'update.component'
    | 'update.all'
    | 'backup.create'
    | 'backup.delete'
    | 'restore'
    | 'uninstall'
  profile?: 'full' | 'nginx'
  operation?: string
  component?: string
  version?: string
  backupId?: string
  backupBeforeChange?: boolean
  expectedResourceVersion: string
  cloudflareAccount?: string
  cloudflareToken?: string
  cloudflareZoneId?: string
}

export interface WebEnvironmentJob {
  id: string
  action: WebEnvironmentActionInput['action']
  target?: string
  status: 'queued' | 'running' | 'waiting_input' | 'succeeded' | 'failed' | 'needs_attention'
  stage: string
  progress: number
  message: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface DockerPort {
  privatePort: number
  publicPort?: number
  protocol: 'tcp' | 'udp' | 'sctp'
  ip?: string
}

export interface DockerContainer {
  id: string
  name: string
  image: string
  state: 'running' | 'paused' | 'restarting' | 'exited' | 'dead' | 'created' | 'unknown'
  health?: HealthLevel
  access: ResourceAccess
  consistency: ConsistencyState
  project?: string
  service?: string
  ports: DockerPort[]
  networks: string[]
  mounts: Array<{
    type: string
    name?: string
    source?: string
    destination: string
  }>
  createdAt?: string
  startedAt?: string
  cpuPercent?: number
  memoryBytes?: number
  memoryLimitBytes?: number
  reason?: string
  allowedActions?: string[]
  resourceVersion?: string
  statusText?: string
}

export interface DockerImage {
  id: string
  tags: string[]
  digests?: string[]
  sizeBytes: number
  createdAt?: string
  inUse: boolean
  resourceVersion?: string
}

export interface DockerNetwork {
  id: string
  name: string
  driver: string
  scope?: string
  containers?: number
  resourceVersion?: string
}

export interface DockerVolume {
  name: string
  driver: string
  mountpoint?: string
  inUse?: boolean
  resourceVersion?: string
}

export interface DockerInventory {
  available: boolean
  version?: string
  observedAt: string
  containers: DockerContainer[]
  composeProjects?: string[]
	images: DockerImage[]
  networks: DockerNetwork[]
  volumes: DockerVolume[]
  loading?: Partial<Record<'images' | 'networks' | 'volumes', boolean>>
  errors?: Partial<Record<'images' | 'networks' | 'volumes', string>>
}

export interface DockerContainerStats {
  containerId: string
  cpuPercent: number
  memoryBytes: number
  memoryLimitBytes: number
  memoryPercent: number
  networkRxBytes: number
  networkTxBytes: number
  blockReadBytes: number
  blockWriteBytes: number
  pids: number
  collectedAt: string
}

export type MonitoringRange = '1h' | '6h' | '24h' | '7d' | '30d' | '3m' | '6m' | '12m'

export interface MonitoringHistoryQuery {
  start: string
  end: string
}

export interface MonitoringHostPoint {
  collectedAt: string
  cpuPercent: number
  cpuAveragePercent?: number
  cpuSampleCount?: number
  cpuCores: number
  loadOne: number
  loadFive: number
  loadFifteen: number
  memoryUsedBytes: number
  memoryTotalBytes: number
  swapUsedBytes: number
  swapTotalBytes: number
  diskUsedBytes: number
  diskTotalBytes: number
  diskPercent: number
  diskIoAvailable?: boolean
  diskReadBytes?: number
  diskWriteBytes?: number
  diskReadBytesPerSecond?: number
  diskWriteBytesPerSecond?: number
  networkRxBytes: number
  networkTxBytes: number
  networkRxBytesPerSecond: number
  networkTxBytesPerSecond: number
  tcpConnections: number
  udpConnections: number
}

export interface MonitoringContainerPoint {
  collectedAt: string
  cpuPercent: number
  cpuAveragePercent?: number
  cpuSampleCount?: number
  memoryBytes: number
  memoryLimitBytes: number
  memoryPercent: number
  networkRxBytes: number
  networkTxBytes: number
  networkRxBytesPerSecond: number
  networkTxBytesPerSecond: number
  blockReadBytes: number
  blockWriteBytes: number
  blockReadBytesPerSecond?: number
  blockWriteBytesPerSecond?: number
  pids: number
}

export interface MonitoringContainerSeries {
  containerId: string
  name: string
  image: string
  points: MonitoringContainerPoint[]
}

export interface MonitoringOperatorLatencyPoint {
  collectedAt: string
  latencyMilliseconds: number | null
  successCount?: number
  failureCount?: number
}

export interface MonitoringOperatorLatencySeries {
  id: string
  operator: 'telecom' | 'unicom' | 'mobile'
  region: 'beijing' | 'shanghai' | 'guangzhou'
  address: string
  points: MonitoringOperatorLatencyPoint[]
}

export interface MonitoringStorageStatus {
  enabled: boolean
  retentionDays: number
  hostIntervalSeconds: number
  containerIntervalSeconds: number
  operatorLatencyIntervalSeconds?: number
  maxContainers: number
  storageBytes: number
  maxStorageBytes: number
  lastSampleAt?: string
  lastError?: string
  lastContainerTotal: number
  lastContainerRecorded: number
  lastContainerFailed: number
  lastContainerTruncated: number
  lastDockerAvailable: boolean
  operatorLatencyAvailable?: boolean
  lastOperatorLatencyAt?: string
  lastOperatorLatencySuccessful?: number
  lastOperatorLatencyFailed?: number
  storageLimitReached: boolean
  rollupRetentionDays?: number
  rollupStorageBytes?: number
  maxRollupStorageBytes?: number
  lastRollupAt?: string
  rollupStorageLimitReached?: boolean
}

export interface MonitoringHistory {
  range: MonitoringRange
  startedAt: string
  endedAt: string
  bucketSeconds: number
  host: MonitoringHostPoint[]
  containers: MonitoringContainerSeries[]
  operatorLatency?: MonitoringOperatorLatencySeries[]
  storage: MonitoringStorageStatus
  scannedBytes: number
  skippedLines: number
  truncatedSeries: number
}

export interface DockerExecResult {
  containerId: string
  exitCode: number
  output: string
  truncated: boolean
  finishedAt: string
}

export interface DockerBackup {
  id: string
  sizeBytes: number
  createdAt: string
  format: 'kpanel-home-docker-v1'
}

export interface DockerEnvironment {
  available: boolean
  engineVersion?: string
  storageDriver?: string
  dataRoot?: string
  containers: number
  images: number
  mirrorPreset: 'cn' | 'official' | 'custom'
  registryMirrors: string[]
  ipv6Enabled: boolean
  ipv6Cidr?: string
  daemonConfig: 'missing' | 'valid' | 'invalid'
  daemonWarning?: string
  observedAt: string
}

export interface DockerComposeProjectFile {
  path: string
  name: string
  source: string
  resourceVersion: string
}

export interface DockerComposeProject {
  name: string
  workingDirectory: string
  configFiles: DockerComposeProjectFile[]
  environmentFile?: DockerComposeProjectFile
  services: string[]
  resourceVersion: string
}

export interface DockerContainerCreatePort {
  privatePort: number
  publicPort: number
  protocol?: 'tcp' | 'udp'
  hostIp?: string
}

export interface DockerContainerCreateMount {
  type?: 'volume' | 'bind'
  source: string
  target: string
  readOnly?: boolean
}

export interface DockerContainerCreateEnvironment {
  name: string
  value: string
}

export type DockerMaintenanceAction =
  | 'container_create'
  | 'compose_deploy'
  | 'compose_redeploy'
  | 'compose_start'
  | 'compose_stop'
  | 'compose_restart'
  | 'container_access'
  | 'image_pull'
  | 'image_remove'
  | 'network_create'
  | 'network_remove'
  | 'network_connect'
  | 'network_disconnect'
  | 'volume_create'
  | 'volume_remove'
  | 'backup_create'
  | 'backup_restore'
  | 'backup_migrate'
  | 'daemon_mirror'
  | 'daemon_ipv6'
  | 'container_prune'
  | 'image_prune'
  | 'network_prune'
  | 'volume_prune'
  | 'prune'

export interface DockerMaintenanceInput {
  action: DockerMaintenanceAction
  image?: string
  target?: string
  name?: string
  driver?: string
  containerId?: string
  containerResourceVersion?: string
  expectedResourceVersion?: string
  preset?: 'cn' | 'official'
  enabled?: boolean
  ipv6Cidr?: string
  ports?: DockerContainerCreatePort[]
  mounts?: DockerContainerCreateMount[]
  environment?: DockerContainerCreateEnvironment[]
  command?: string[]
  network?: string
  restartPolicy?: 'no' | 'always' | 'unless-stopped' | 'on-failure'
  compose?: string
  composeEnvironment?: string
  composeFile?: string
  allowedIp?: string
  backupId?: string
  migrationHost?: string
  migrationUser?: string
  migrationPort?: number
}

export interface DockerMaintenanceJob {
  id: string
  action: DockerMaintenanceAction
  target?: string
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  stage: string
  progress: number
  message?: string
  resultPath?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export type JobStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'failed_rolled_back'
  | 'failed_needs_attention'
  | 'interrupted'
  | 'cancelled'

export interface JobStage {
  name: string
  status: JobStatus
  startedAt?: string
  finishedAt?: string
  message?: string
}

export interface Job {
  id: string
  action: string
  resourceType?: string
  resourceName?: string
  status: JobStatus
  progress?: number
  actor?: string
  source?: 'web' | 'cli' | 'reconcile' | 'system'
  createdAt: string
  startedAt?: string
  finishedAt?: string
  errorCode?: string
  errorMessage?: string
  stages?: JobStage[]
}

export interface AuditEvent {
  id: string
  occurredAt: string
  actor: string
  source: 'web' | 'cli' | 'reconcile' | 'system' | 'external'
  action: string
  resourceType?: string
  resourceName?: string
  outcome: 'success' | 'failure' | 'denied' | 'observed'
  requestId?: string
  summary?: string
  remoteAddress?: string
}

export interface PanelSettings {
  panelVersion?: string
  agent: AgentStatus
  serverTime?: string
  sessionExpiresAt?: string
  publicUrl?: string
  reconcileIntervalSeconds?: number
  telemetryEnabled?: boolean
}

export interface DesktopIconPosition {
  /** Normalized horizontal coordinate in the current work area, from 0 to 1. */
  x: number
  /** Paged normalized vertical coordinate, from 0 to 512; values above 1 are below the first view. */
  y: number
}

export interface DesktopShortcut {
  id: string
  name: string
  description: string
  targetType: 'url' | 'file' | 'directory'
  url?: string
  path?: string
  iconVersion?: string
  iconURL?: string
  createdAt: string
  updatedAt: string
}

export interface DesktopWorkspace {
  schemaVersion: 3
  resourceVersion: string
  /** False means corrupt/unsupported persisted data was isolated read-only. */
  available: boolean
  warning?: string
  hiddenEntryKeys: string[]
  /** Optional for compatibility with workspaces created before widget visibility was persisted. */
  hiddenWidgetKeys?: string[]
  positions: Record<string, DesktopIconPosition>
  widgetPositions: Record<string, DesktopIconPosition>
  labels: Record<string, string>
  shortcuts: DesktopShortcut[]
}

export interface DesktopWorkspaceUpdate {
  expectedResourceVersion: string
  hiddenEntryKeys: string[]
  hiddenWidgetKeys?: string[]
  positions: Record<string, DesktopIconPosition>
  widgetPositions: Record<string, DesktopIconPosition>
  labels: Record<string, string>
  shortcuts: Array<Pick<DesktopShortcut, 'id' | 'name' | 'description' | 'targetType' | 'url' | 'path'>>
}

export interface DesktopShortcutIconResult {
  iconVersion: string
  iconURL: string
}

export interface SecurityEntranceSettings {
  enabled: boolean
  path?: string
  resourceVersion: string
}

export type FileKind = 'file' | 'directory' | 'symlink' | 'special'

export interface FileEntry {
  name: string
  path: string
  kind: FileKind
  mime?: string
  sizeBytes: number
  mode: string
  owner: string
  group: string
  modifiedAt: string
  resourceVersion: string
  editable: boolean
  previewable: boolean
}

export interface FileEntryBatchResult {
  entries: FileEntry[]
  unavailable: string[]
}

export interface FileDirectory {
  path: string
  entries: FileEntry[]
  offset: number
  nextOffset?: number
  total?: number
  totalKnown?: boolean
  truncated: boolean
  scanTruncated?: boolean
  readAt: string
}

export interface FileDownloadTicket {
  downloadUrl: string
  expiresAt: string
}

export type FileShareExpiry = '7d' | '30d' | 'never'

export interface FileShareAdminView {
  id: string
  createdAt: string
  expiresAt?: string
  linksAvailable: boolean
  sharePath?: string
  directPath?: string
}

export interface FileShareLookup {
  share: FileShareAdminView | null
}

export interface FileShareListItem extends FileShareAdminView {
  path: string
}

export interface FileShareList {
  shares: FileShareListItem[]
}

export interface FileShareCreateInput {
  path: string
  expectedResourceVersion: string
  expectedShareID: string
  expiresIn: FileShareExpiry
}

export interface PublicFileShareView {
  name: string
  mime?: string
  sizeBytes: number
  expiresAt?: string
  directPath: string
  downloadPath: string
}

export interface CrossPanelFileTransferInput {
  sourceNodeId: string
  path: string
  resourceVersion: string
  targetDirectory: string
}

export type CrossPanelFileTransferState =
  | 'connecting'
  | 'transferring'
  | 'committing'
  | 'complete'
  | 'error'

export interface CrossPanelFileTransferEvent {
  state: CrossPanelFileTransferState
  loadedBytes?: number
  totalBytes?: number
  name?: string
  entry?: FileEntry
  code?: string
  detail?: string
}

export interface FileRemoteDownloadInput {
  url: string
  targetDirectory: string
  name?: string
}

export type FileRemoteDownloadState =
  | 'connecting'
  | 'transferring'
  | 'confirming'
  | 'complete'
  | 'error'

export interface FileRemoteDownloadEvent {
  state: FileRemoteDownloadState
  loadedBytes?: number
  totalBytes?: number
  name?: string
  entry?: FileEntry
  code?: string
  detail?: string
}

export type FileRemoteDownloadJobState =
  | 'queued'
  | 'connecting'
  | 'transferring'
  | 'confirming'
  | 'complete'
  | 'cancelled'
  | 'error'
  | 'interrupted'

export interface FileRemoteDownloadJob {
  id: string
  state: FileRemoteDownloadJobState
  source: string
  targetDirectory: string
  name?: string
  loadedBytes?: number
  totalBytes?: number
  entry?: FileEntry
  code?: string
  createdAt: string
  updatedAt: string
  finishedAt?: string
}

export interface FileRemoteDownloadJobList {
  items: FileRemoteDownloadJob[]
}

export type FileAction =
  | 'mkdir'
  | 'rename'
  | 'copy'
  | 'move'
  | 'compress'
  | 'extract'
  | 'trash'
  | 'chmod'
  | 'trash_restore'
  | 'trash_delete'
  | 'trash_empty'

export interface FileActionInput {
  action: FileAction
  sources?: string[]
  trashIds?: string[]
  target?: string
  name?: string
  format?: 'zip' | 'tar' | 'tar.gz'
  mode?: string
  expectedResourceVersion?: string
  expectedResourceVersions?: Record<string, string>
}

export interface FileTrashEntry {
  id: string
  name: string
  originalPath?: string
  kind: FileKind
  sizeBytes: number
  mode: string
  owner: string
  group: string
  deletedAt: string
  resourceVersion: string
  restorable: boolean
}

export interface FileTrashDirectory {
  entries: FileTrashEntry[]
  total: number
  truncated: boolean
  readAt: string
}

export interface FileActionResult {
  action: FileAction
  succeeded: Array<{
    path: string
    destination?: string
    resourceVersion?: string
  }>
  failed: Array<{
    path: string
    detail: string
  }>
}

export interface FileWriteResult {
  entry: FileEntry
}

export interface DockerActionResult {
  jobId?: string
  containerId?: string
  action?: string
  status?: string
  resourceVersion?: string
}
