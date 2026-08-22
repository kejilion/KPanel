import type {
	AccountManagementActionInput,
	AccountManagementActionResult,
	AccountManagementSnapshot,
	SSHDefenseActionInput,
	SSHDefenseActionResult,
	SSHDefenseSnapshot,
  ApiList,
  AgentStatus,
  AppInstallPortStatus,
  AppMarketInventory,
  AppImageUpdateResult,
  AppMutationResult,
  AuditEvent,
  AuthSession,
  AuthStatus,
  ClusterController,
  ClusterHost,
  ClusterHostList,
  ClusterLightEnrollment,
  ClusterPairingCode,
  ClusterShareSettings,
  CrossPanelFileTransferEvent,
  CrossPanelFileTransferInput,
  DockerInventory,
  DockerActionResult,
  DockerBackup,
  DockerComposeProject,
  DockerContainerStats,
  DockerExecResult,
  DockerEnvironment,
  DockerMaintenanceInput,
  DockerMaintenanceJob,
  DesktopShortcutIconResult,
  DesktopWorkspace,
  DesktopWorkspaceUpdate,
  FileActionInput,
  FileActionResult,
  FileTrashDirectory,
  FileDirectory,
  FileDownloadTicket,
  FileEntry,
  FileEntryBatchResult,
  FileShareAdminView,
  FileShareCreateInput,
  FileShareList,
  FileShareLookup,
  FileWriteResult,
  FirewallSnapshot,
  HostsSnapshot,
  DiagnosticCatalog,
  DiagnosticJob,
  Job,
  AppInstallJob,
  AppTerminalChunk,
  LoginRequest,
  MonitoringHistory,
  MonitoringHistoryQuery,
  MonitoringRange,
  NetworkInterfacesSnapshot,
	PortUsageSnapshot,
  PanelSettings,
  ProcessQuery,
  ProcessSnapshot,
  PublicClusterShareSnapshot,
  PublicFileShareView,
  SecurityEntranceSettings,
  SetupRequest,
  Site,
  SiteAppearance,
  SiteDeleteResult,
  SiteInput,
  SiteInstallationProgress,
  SystemActionInput,
  SystemActionResult,
  SystemResourceActionInput,
  SystemResourceActionResult,
  SystemOverview,
	SystemTuningActionInput,
	SystemTuningActionResult,
	SystemTuningSnapshot,
	TrafficShutdownActionInput,
	TrafficShutdownActionResult,
	TrafficShutdownSnapshot,
  CronSnapshot,
  TOTPEnrollment,
  TOTPRecoveryCodes,
  TOTPStatus,
  TerminalOutput,
  TerminalSession,
  WebEnvironmentActionInput,
  WebEnvironmentBackup,
  WebEnvironmentCatalog,
  WebEnvironmentJob,
  WebEnvironmentSummary,
} from '@/types/api'

type QueryValue = string | number | boolean | undefined
export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  query?: Record<string, QueryValue>
  signal?: AbortSignal
  unwrapEnvelope?: boolean
}
interface ApiEnvelope<T> {
  data?: T
  csrfToken?: string
  error?: {
    code?: string
    message?: string
    details?: unknown
  }
  message?: string
}

interface ProblemPayload {
  title?: string
  status?: number
  code?: string
  detail?: string
  requestId?: string
  retryable?: boolean
  fieldErrors?: Record<string, string>
}

interface RawAgentHealth {
  status: string
  version?: string
  protocolVersion?: string
  readOnly?: boolean
  reasons?: string[]
  checkedAt?: string
}

interface RawSystemSummary {
  hostname: string
  os: string
  osId?: string
  osLike?: string[]
  kernel?: string
  architecture?: string
  uptimeSeconds: number
  load: { one: number; five: number; fifteen: number }
  cpu: { model?: string; cores: number; frequencyMHz?: number; usagePercent: number }
  memory: {
    totalBytes: number
    availableBytes: number
    usedBytes: number
    usagePercent: number
    swapTotalBytes?: number
    swapUsedBytes?: number
  }
  disks: Array<{
    device: string
    mountPoint: string
    fileSystem: string
    totalBytes: number
    usedBytes: number
    usagePercent: number
  }>
  network: {
    receivedBytes: number
    sentBytes: number
    tcpConnections?: number
    udpConnections?: number
  }
  publicNetwork?: {
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
  management?: {
    ssh?: {
      ports?: number[]
      source?: string
      defense?: {
        available?: boolean
        installed?: boolean
        running?: boolean
        enabled?: boolean
        autostart?: boolean
        jail?: string
        banned?: number
        message?: string
      }
    }
    dns?: { servers?: string[]; manager?: string }
    timezone?: string
    swap?: {
      activeDevices?: number
      path?: string
      fileExists?: boolean
      fileActive?: boolean
      fileSizeBytes?: number
      fileUsedBytes?: number
      legacyExists?: boolean
      legacyActive?: boolean
      legacySizeBytes?: number
      otherActiveDevices?: number
      otherSwapTotalBytes?: number
      otherSwapUsedBytes?: number
    }
    packageManager?: string
    packageSources?: string[]
    maintenance?: {
      id?: string
      state?: string
      action?: string
      policy?: string
      stage?: string
      progress?: number
      message?: string
      startedAt?: string
      finishedAt?: string
      rebootRequired?: boolean
    }
    ipPreference?: string
    kernelOptimization?: { enabled?: boolean; profile?: string; source?: string }
    bbr?: {
      supported?: boolean
      enabled?: boolean
      congestionControl?: string
      defaultQDisc?: string
      available?: string[]
    }
    bbrv3?: {
      available?: boolean
      supported?: boolean
      installed?: boolean
      active?: boolean
      architecture?: string
      os?: string
      codename?: string
      runningKernel?: string
      installedKernel?: string
      congestionControl?: string
      defaultQDisc?: string
      rebootRequired?: boolean
      reason?: string
    }
  }
  collectedAt: string
}

interface RawPublicNetworkSummary {
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

interface RawSite {
  id: string
  primaryDomain: string
  domains?: string[]
  kind: string
  enabled: boolean
  health?: string
  tls?: {
    enabled: boolean
    status?: string
    expiresAt?: string
    source?: string
  }
  target?: string
  documentRoot?: string
  origin?: string
  consistency?: string
  resourceVersion?: string
  allowedActions?: string[]
  warnings?: string[]
  artifacts?: Array<{ kind: string; path: string; hash?: string }>
  reconciledAt?: string
}

interface RawDockerSummary {
  available: boolean
  serverVersion?: string
  containers: number
  running: number
  paused?: number
  stopped: number
  images: number
  collectedAt: string
}

interface RawContainer {
  id: string
  name: string
  image: string
  state: string
  status?: string
  health?: string
  createdAt?: string
  ports?: Array<{
    privatePort: number
    publicPort?: number
    ip?: string
    type?: string
    protocol?: string
  }>
  composeProject?: string
  composeService?: string
  ownership?: string
  resourceVersion?: string
  allowedActions?: string[]
  mounts?: Array<{ type?: string; name?: string; source?: string; destination?: string }>
  networks?: string[]
}

interface RawDockerImage {
  id: string
  repoTags?: string[]
  repoDigests?: string[]
  createdAt?: number
  sizeBytes: number
  containers?: number
  resourceVersion?: string
}

interface RawDockerNetwork {
  id: string
  name: string
  driver: string
  scope?: string
  containerCount?: number
  resourceVersion?: string
}

interface RawDockerVolume {
  name: string
  driver: string
  mountpoint?: string
  resourceVersion?: string
}

interface RawJob {
  id: string
  action: string
  origin?: string
  state: string
  progress?: number
  stage?: string
  targetKind?: string
  targetId?: string
  targetLabel?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  error?: ProblemPayload
}

interface RawAuditEvent {
  id: string
  occurredAt: string
  actorType?: string
  actorId?: string
  sourceIp?: string
  action: string
  targetKind?: string
  targetId?: string
  result?: string
  requestId?: string
}

interface RawSiteInstallJob {
  id: string
  domain: string
  recipe?: string
  interactive?: boolean
  inputOpen?: boolean
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  stage?: string
  progress?: number
  message?: string
  events?: Array<{
    stage: string
    progress: number
    message: string
    at: string
  }>
  site?: RawSite
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details?: unknown
  readonly requestId?: string

  constructor(message: string, status = 0, code = 'request_failed', details?: unknown, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
    this.requestId = requestId
  }
}

let csrfToken = ''
let previousNetworkSample:
  | { receivedBytes: number; sentBytes: number; collectedAtMs: number }
  | undefined
let previousNetworkRate = { receive: 0, transmit: 0, available: false }

function networkRates(system: RawSystemSummary): { receive: number; transmit: number; available: boolean } {
  const current = {
    receivedBytes: system.network.receivedBytes,
    sentBytes: system.network.sentBytes,
    collectedAtMs: Date.parse(system.collectedAt),
  }
  const previous = previousNetworkSample
  if (
    previous &&
    current.collectedAtMs === previous.collectedAtMs &&
    current.receivedBytes === previous.receivedBytes &&
    current.sentBytes === previous.sentBytes
  ) {
    return previousNetworkRate
  }
  if (
    !previous ||
    !Number.isFinite(current.collectedAtMs) ||
    current.collectedAtMs <= previous.collectedAtMs ||
    current.receivedBytes < previous.receivedBytes ||
    current.sentBytes < previous.sentBytes
  ) {
    if (!previous || current.collectedAtMs > previous.collectedAtMs) {
      previousNetworkSample = current
      previousNetworkRate = { receive: 0, transmit: 0, available: false }
    }
    return previousNetworkRate
  }
  const elapsedSeconds = (current.collectedAtMs - previous.collectedAtMs) / 1_000
  previousNetworkSample = current
  previousNetworkRate = {
    receive: (current.receivedBytes - previous.receivedBytes) / elapsedSeconds,
    transmit: (current.sentBytes - previous.sentBytes) / elapsedSeconds,
    available: true,
  }
  return previousNetworkRate
}

export type SystemResourceSnapshot = Pick<
  SystemOverview,
  | 'hostname'
  | 'os'
  | 'osId'
  | 'osLike'
  | 'kernel'
  | 'architecture'
  | 'uptimeSeconds'
  | 'observedAt'
  | 'cpu'
  | 'memory'
  | 'disk'
  | 'load'
  | 'network'
> & {
  /** Host-configured IANA timezone, independent from public-IP geolocation. */
  timezone?: string
}

function normalizeSystemResources(system: RawSystemSummary): SystemResourceSnapshot {
  const rootDisk = system.disks.find((disk) => disk.mountPoint === '/') || system.disks[0]
  const rates = networkRates(system)
  return {
    hostname: system.hostname,
    os: system.os,
    osId: system.osId,
    osLike: system.osLike || [],
    kernel: system.kernel,
    architecture: system.architecture,
    timezone: system.management?.timezone,
    uptimeSeconds: system.uptimeSeconds,
    observedAt: system.collectedAt,
    cpu: {
      value: system.cpu.usagePercent,
      percent: system.cpu.usagePercent,
      unit: '%',
      model: system.cpu.model,
      cores: system.cpu.cores,
      frequencyMHz: system.cpu.frequencyMHz,
    },
    memory: {
      value: system.memory.usedBytes,
      total: system.memory.totalBytes,
      percent: system.memory.usagePercent,
      unit: 'bytes',
    },
    disk: {
      value: rootDisk?.usedBytes || 0,
      total: rootDisk?.totalBytes,
      percent: rootDisk?.usagePercent,
      unit: 'bytes',
    },
    load: {
      value: system.load.one,
      unit: String(system.cpu.cores),
      one: system.load.one,
      five: system.load.five,
      fifteen: system.load.fifteen,
    },
    network: {
      receiveBytesPerSecond: rates.receive,
      transmitBytesPerSecond: rates.transmit,
      rateAvailable: rates.available,
      totalReceivedBytes: system.network.receivedBytes,
      totalTransmittedBytes: system.network.sentBytes,
      tcpConnections: system.network.tcpConnections || 0,
      udpConnections: system.network.udpConnections || 0,
    },
  }
}

function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  const base = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/+$/, '')
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  const url = `${base}${normalizedPath}`
  if (!query) return url

  const params = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined) params.set(key, String(value))
  })
  const serialized = params.toString()
  return serialized ? `${url}?${serialized}` : url
}

async function rawFileResponse(
  path: string,
  options: { method?: 'GET' | 'POST'; body?: BodyInit; query?: Record<string, QueryValue>; headers?: HeadersInit } = {},
): Promise<Response> {
  const headers = new Headers(options.headers)
  if (options.method === 'POST' && csrfToken) headers.set('X-CSRF-Token', csrfToken)
  let response: Response
  try {
    response = await fetch(buildUrl(path, options.query), {
      method: options.method || 'GET',
      credentials: 'same-origin',
      cache: 'no-store',
      headers,
      body: options.body,
    })
  } catch (error) {
    throw new ApiError('无法连接到面板服务，请检查服务状态后重试。', 0, 'network_error', error)
  }
  if (!response.ok) {
    const payload = await parsePayload(response)
    const problem = payload && typeof payload === 'object' ? (payload as ProblemPayload) : undefined
    throw new ApiError(
      problem?.detail || problem?.title || '文件操作失败。',
      response.status,
      problem?.code || 'file_request_failed',
      payload,
      problem?.requestId,
    )
  }
  return response
}

function pickCsrfToken(headers: Headers, payload: unknown): void {
  const headerToken = headers.get('x-csrf-token')
  if (headerToken) {
    csrfToken = headerToken
    return
  }

  if (payload && typeof payload === 'object') {
    const envelope = payload as ApiEnvelope<unknown>
    if (typeof envelope.csrfToken === 'string') csrfToken = envelope.csrfToken
    const data = envelope.data
    if (data && typeof data === 'object' && 'csrfToken' in data) {
      const nestedToken = (data as { csrfToken?: unknown }).csrfToken
      if (typeof nestedToken === 'string') csrfToken = nestedToken
    }
  }
}

async function parsePayload(response: Response): Promise<unknown> {
  if (response.status === 204) return undefined
  const contentType = response.headers.get('content-type') || ''
  if (!contentType.includes('json')) {
    const text = await response.text()
    return text || undefined
  }
  return response.json()
}

async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const method = options.method || 'GET'
  const headers = new Headers({ Accept: 'application/json' })
  const multipart = typeof FormData !== 'undefined' && options.body instanceof FormData
  if (options.body !== undefined && !multipart) headers.set('Content-Type', 'application/json')
  if (method !== 'GET' && csrfToken) headers.set('X-CSRF-Token', csrfToken)

  let response: Response
  try {
    response = await fetch(buildUrl(path, options.query), {
      method,
      credentials: 'same-origin',
      cache: 'no-store',
      headers,
      body: options.body === undefined
        ? undefined
        : multipart
          ? options.body as FormData
          : JSON.stringify(options.body),
      signal: options.signal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ApiError('无法连接到面板服务，请检查服务状态后重试。', 0, 'network_error', error)
  }

  const payload = await parsePayload(response)
  if (path === '/auth/bootstrap' || path === '/auth/login' || path === '/auth/session') {
    pickCsrfToken(response.headers, payload)
  }

  if (!response.ok) {
    const envelope = payload && typeof payload === 'object' ? (payload as ApiEnvelope<unknown>) : undefined
    const problem = payload && typeof payload === 'object' ? (payload as ProblemPayload) : undefined
    const fieldError = problem?.fieldErrors
      ? Object.values(problem.fieldErrors).find((value) => value.trim() !== '')
      : undefined
    const textError = typeof payload === 'string'
      ? /<!doctype|<html[\s>]/i.test(payload)
        ? `请求被反向代理或安全网关拒绝（HTTP ${response.status}）`
        : payload
      : ''
    const message =
      envelope?.error?.message ||
      envelope?.message ||
      problem?.detail ||
      fieldError ||
      problem?.title ||
      textError ||
      `请求失败（HTTP ${response.status}）`
    throw new ApiError(
      message,
      response.status,
      envelope?.error?.code || problem?.code || `http_${response.status}`,
      envelope?.error?.details || problem?.fieldErrors,
      problem?.requestId || response.headers.get('x-request-id') || undefined,
    )
  }

  if (options.unwrapEnvelope !== false && payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as ApiEnvelope<T>).data as T
  }
  return payload as T
}

export function normalizeList<T>(value: ApiList<T> | T[] | null | undefined): ApiList<T> {
  if (Array.isArray(value)) return { items: value, total: value.length }
  if (!value) return { items: [], total: 0 }
  const items = Array.isArray(value.items) ? value.items : []
  return { ...value, items, total: Number.isFinite(value.total) ? value.total : items.length }
}

function normalizeAgent(raw: RawAgentHealth): AgentStatus {
  const status = raw.status?.toLowerCase() || 'unknown'
  const connected = !['offline', 'unavailable', 'unreachable', 'error'].includes(status)
  const compatible = !['incompatible', 'protocol_mismatch'].includes(status)
  return {
    connected,
    compatible,
    readOnly: Boolean(raw.readOnly || !connected || !compatible),
    version: raw.version,
    protocolVersion: raw.protocolVersion,
    lastSeenAt: raw.checkedAt,
    reason: raw.reasons?.join('；'),
  }
}

function normalizeMaintenance(
  raw?: NonNullable<RawSystemSummary['management']>['maintenance'],
): SystemOverview['management']['maintenance'] {
  return {
    id: raw?.id,
    state: ['running', 'succeeded', 'failed'].includes(raw?.state || '')
      ? (raw?.state as 'running' | 'succeeded' | 'failed')
      : 'idle',
    action: raw?.action === 'update' || raw?.action === 'cleanup' ? raw.action : undefined,
    policy:
      raw?.policy === 'full' || raw?.policy === 'cache' || raw?.policy === 'standard'
        ? raw.policy
        : undefined,
    stage: raw?.stage,
    progress: raw?.progress || 0,
    message: raw?.message,
    startedAt: raw?.startedAt,
    finishedAt: raw?.finishedAt,
    rebootRequired: Boolean(raw?.rebootRequired),
  }
}

function normalizeAccountManagementSnapshot(snapshot: AccountManagementSnapshot): AccountManagementSnapshot {
  return {
    ...snapshot,
    accounts: (Array.isArray(snapshot.accounts) ? snapshot.accounts : []).map((account) => ({
      ...account,
      groups: Array.isArray(account.groups) ? account.groups : [],
      sshKeys: Array.isArray(account.sshKeys) ? account.sshKeys : [],
    })),
  }
}

async function createSite(
  body: SiteInput,
  onProgress?: (progress: SiteInstallationProgress) => void,
): Promise<Site> {
  onProgress?.({
    status: 'running',
    stage: 'submitting',
    progress: 2,
    message: '正在提交建站配置并检查现有产物。',
  })
  const result = await request<RawSite | RawSiteInstallJob>('/sites', { method: 'POST', body })
  const backgroundTypes: SiteInput['type'][] = [
    'wordpress',
    'recipe',
    'proxy',
    'static',
    'php',
    'proxy_domain',
    'load_balance',
    'redirect',
  ]
  if (
    !backgroundTypes.includes(body.type) ||
    !('status' in result) ||
    !('id' in result)
  ) {
    onProgress?.({
      status: 'succeeded',
      stage: 'completed',
      progress: 100,
      message: '网站配置已创建并通过 Nginx 校验。',
    })
    return normalizeSite(result as RawSite)
  }
  let job = result as RawSiteInstallJob
  for (let attempt = 0; attempt <= 900; attempt += 1) {
    const progress = normalizeSiteInstallationProgress(job)
    onProgress?.(progress)
    if (job.status === 'succeeded') {
      if (!job.site) throw new ApiError('一键建站已完成，但网站对账结果缺失。', 503, 'site_result_missing')
      return normalizeSite(job.site)
    }
    if (job.status === 'failed') {
      throw new ApiError(
        job.message || '一键建站失败，请核对实际产物。',
        422,
        'site_install_failed',
      )
    }
    if (attempt === 900) break
    await new Promise((resolve) => setTimeout(resolve, 2_000))
    try {
      job = await request<RawSiteInstallJob>(
        `/site-installations/${encodeURIComponent(job.id)}`,
      )
    } catch (reason) {
      if (!isTransientAgentError(reason)) throw reason
      onProgress?.({
        ...progress,
        status: 'running',
        stage: 'reconnecting',
        message: 'Agent 暂时不可用，后台建站任务不受影响，正在自动重连。',
      })
    }
  }
  throw new ApiError('一键建站状态等待超时，请在网站列表中核对实际产物。', 504, 'site_install_timeout')
}

export function isTransientAgentError(reason: unknown): boolean {
  return reason instanceof ApiError && (
    reason.status === 0 ||
    reason.status === 502 ||
    reason.status === 503 ||
    reason.status === 504 ||
    reason.code === 'agent_unavailable' ||
    reason.code === 'network_error'
  )
}

function normalizeSiteInstallationProgress(job: RawSiteInstallJob): SiteInstallationProgress {
  const progress: SiteInstallationProgress = {
    id: job.id,
    status: job.status,
    stage: job.stage || job.status,
    progress: Math.min(100, Math.max(0, job.progress ?? (job.status === 'queued' ? 0 : 1))),
    message: job.message || '一键建站任务正在执行。',
  }
  if (job.domain) progress.domain = job.domain
  if (job.recipe) progress.recipe = job.recipe
  if (job.interactive !== undefined) progress.interactive = Boolean(job.interactive)
  if (job.inputOpen !== undefined) progress.inputOpen = Boolean(job.inputOpen)
  if (job.events?.length) {
    progress.events = job.events.map((event) => ({
      stage: event.stage,
      progress: Math.min(100, Math.max(0, event.progress)),
      message: event.message,
      at: event.at,
    }))
  }
  return progress
}

function normalizeSite(raw: RawSite): Site {
  const kindMap: Record<string, Site['type']> = {
    static: 'static',
    reverse_proxy: 'proxy',
    proxy: 'proxy',
    domain_proxy: 'proxy_domain',
    load_balance: 'load_balance',
    php: 'php',
    wordpress: 'wordpress',
    redirect: 'redirect',
  }
  const consistencyMap: Record<string, Site['consistency']> = {
    in_sync: 'synced',
    drifted: 'drifted',
    ambiguous: 'ambiguous',
    conflicted: 'conflicted',
    unsupported: 'unsupported',
    read_only: 'read_only',
  }
  const sourceMap: Record<string, Site['source']> = {
    web: 'panel',
    panel: 'panel',
    cli: 'kejilion',
    discovered: 'kejilion',
    external: 'external',
  }
  const actions = raw.allowedActions || []
  const certificateStatus = raw.tls?.enabled ? raw.tls.status || 'unknown' : 'missing'
  return {
    id: raw.id,
    primaryDomain: raw.primaryDomain,
    domains: raw.domains || [raw.primaryDomain],
    type: kindMap[raw.kind] || 'unknown',
    enabled: raw.enabled,
    health: (['healthy', 'warning', 'critical'].includes(raw.health || '')
      ? raw.health
      : 'unknown') as Site['health'],
    consistency: consistencyMap[raw.consistency || ''] || 'unknown',
    access:
      actions.length > 0 || (raw.origin === 'web' && raw.consistency === 'in_sync')
        ? 'managed'
        : raw.origin === 'external'
          ? 'unmanaged'
          : 'read-only',
    source: sourceMap[raw.origin || ''] || 'unknown',
    rootPath: raw.documentRoot,
    upstream: raw.target,
    certificate: {
      status: (['valid', 'expiring', 'expired', 'missing'].includes(certificateStatus)
        ? certificateStatus
        : 'unknown') as NonNullable<Site['certificate']>['status'],
      issuer: raw.tls?.source,
      expiresAt: raw.tls?.expiresAt,
    },
    resourceVersion: raw.resourceVersion || '',
    observedAt: raw.reconciledAt,
    reason: raw.warnings?.[0],
    warnings: raw.warnings,
    allowedActions: actions,
    artifacts: raw.artifacts,
  }
}

function normalizeContainer(raw: RawContainer): DockerInventory['containers'][number] {
  const actions = raw.allowedActions || []
  const state = ['running', 'paused', 'restarting', 'exited', 'dead', 'created'].includes(raw.state)
    ? raw.state
    : 'unknown'
  return {
    id: raw.id,
    name: raw.name,
    image: raw.image,
    state: state as DockerInventory['containers'][number]['state'],
    health: (['healthy', 'warning', 'critical'].includes(raw.health || '') ? raw.health : 'unknown') as
      | 'healthy'
      | 'warning'
      | 'critical'
      | 'unknown',
    access: actions.length > 0 ? 'managed' : raw.ownership === 'external' ? 'unmanaged' : 'read-only',
    consistency: raw.ownership === 'ambiguous' ? 'ambiguous' : 'synced',
    project: raw.composeProject,
    service: raw.composeService,
    createdAt: raw.createdAt,
    ports: (raw.ports || []).map((port) => ({
      privatePort: port.privatePort,
      publicPort: port.publicPort,
      ip: port.ip,
      protocol: (port.protocol || port.type || 'tcp') as 'tcp' | 'udp' | 'sctp',
    })),
    networks: raw.networks || [],
    mounts: (raw.mounts || []).map((mount) => ({
      type: mount.type || 'unknown',
      name: mount.name,
      source: mount.source,
      destination: mount.destination || '',
    })),
    allowedActions: actions,
    resourceVersion: raw.resourceVersion,
    statusText: raw.status,
  }
}

function normalizeJob(raw: RawJob): Job {
  const knownStates: Job['status'][] = [
    'queued',
    'running',
    'succeeded',
    'failed_rolled_back',
    'failed_needs_attention',
    'interrupted',
    'cancelled',
  ]
  return {
    id: raw.id,
    action: raw.action,
    resourceType: raw.targetKind,
    resourceName: raw.targetLabel || raw.targetId,
    status: knownStates.includes(raw.state as Job['status']) ? (raw.state as Job['status']) : 'failed',
    progress: raw.progress,
    actor: raw.origin,
    source: (['web', 'cli', 'reconcile', 'system'].includes(raw.origin || '') ? raw.origin : 'system') as Job['source'],
    createdAt: raw.createdAt,
    startedAt: raw.startedAt,
    finishedAt: raw.finishedAt,
    errorCode: raw.error?.code,
    errorMessage: raw.error?.detail || raw.error?.title,
    stages: raw.stage
      ? [
          {
            name: raw.stage,
            status: raw.state === 'running' ? 'running' : raw.state === 'succeeded' ? 'succeeded' : 'failed',
          },
        ]
      : undefined,
  }
}

export const api = {
  auth: {
    status: async (signal?: AbortSignal): Promise<AuthStatus> => {
      const bootstrap = await request<{ required: boolean }>('/auth/bootstrap', { signal })
      if (bootstrap.required) return { setupRequired: true, authenticated: false }
      try {
        const session = await request<AuthSession>('/auth/session', { signal })
        return {
          setupRequired: false,
          authenticated: true,
          user: session.user,
          csrfToken: session.csrfToken,
          expiresAt: session.expiresAt,
        }
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) {
          return { setupRequired: false, authenticated: false }
        }
        throw error
      }
    },
    setup: async (body: SetupRequest): Promise<AuthStatus> => {
      const session = await request<AuthSession>('/auth/bootstrap', { method: 'POST', body })
      return {
        setupRequired: false,
        authenticated: true,
        user: session.user,
        csrfToken: session.csrfToken,
        expiresAt: session.expiresAt,
      }
    },
    login: async (body: LoginRequest): Promise<AuthStatus> => {
      const session = await request<AuthSession>('/auth/login', { method: 'POST', body })
      return {
        setupRequired: false,
        authenticated: true,
        user: session.user,
        csrfToken: session.csrfToken,
        expiresAt: session.expiresAt,
      }
    },
    logout: () => request<void>('/auth/logout', { method: 'POST' }),
  },
  desktop: {
    workspace: (signal?: AbortSignal): Promise<DesktopWorkspace> =>
      request<DesktopWorkspace>('/desktop/workspace', { signal }),
    updateWorkspace: (body: DesktopWorkspaceUpdate): Promise<DesktopWorkspace> =>
      request<DesktopWorkspace>('/desktop/workspace', { method: 'PUT', body }),
    shortcutIconURL: (id: string, version?: string): string =>
      buildUrl(`/desktop/shortcuts/${encodeURIComponent(id)}/icon`, version ? { v: version } : undefined),
    uploadShortcutIcon: (id: string, file: File): Promise<DesktopShortcutIconResult> =>
      new Promise<DesktopShortcutIconResult>((resolve, reject) => {
        const xhr = new XMLHttpRequest()
        xhr.open('PUT', buildUrl(`/desktop/shortcuts/${encodeURIComponent(id)}/icon`))
        xhr.withCredentials = true
        xhr.responseType = 'json'
        xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream')
        if (csrfToken) xhr.setRequestHeader('X-CSRF-Token', csrfToken)
        xhr.onerror = () => reject(new ApiError('图标上传连接中断。', 0, 'network_error'))
        xhr.onload = () => {
          const payload = xhr.response
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(payload as DesktopShortcutIconResult)
            return
          }
          const problem = payload && typeof payload === 'object' ? (payload as ProblemPayload) : undefined
          reject(new ApiError(
            problem?.detail || problem?.title || '图标上传失败。',
            xhr.status,
            problem?.code || 'desktop_icon_upload_failed',
            payload,
            problem?.requestId,
          ))
        }
        xhr.send(file)
      }),
    removeShortcutIcon: (id: string): Promise<void> =>
      request<void>(`/desktop/shortcuts/${encodeURIComponent(id)}/icon`, { method: 'DELETE' }),
  },
  agent: {
    health: async (signal?: AbortSignal) => normalizeAgent(await request<RawAgentHealth>('/agent/health', { signal })),
    capabilities: async (signal?: AbortSignal) => {
      type Capability = { id: string; enabled: boolean; reason?: string; methods?: string[] }
      const result = await request<ApiList<Capability> | Capability[]>('/capabilities', { signal })
      return normalizeList(result).items
    },
  },
  overview: {
    get: async (
      signal?: AbortSignal,
      onUpdate?: (overview: SystemOverview) => void,
    ): Promise<SystemOverview> => {
      type Capability = { id: string; enabled: boolean; reason?: string; methods?: string[] }
      // Keep the CPU sample isolated from the optional requests triggered by this page.
      const system = await request<RawSystemSummary>('/system/summary', { signal })
      const agentRequest = request<RawAgentHealth>('/agent/health', { signal })
      const capabilitiesRequest = request<ApiList<Capability> | Capability[]>('/capabilities', { signal })
        .catch(() => undefined)
      const sitesRequest = request<ApiList<RawSite> | RawSite[]>('/sites', { signal })
        .catch(() => undefined)
      const dockerRequest = request<RawDockerSummary>('/docker/summary', { signal })
        .catch(() => undefined)
      const containersRequest = request<ApiList<RawContainer> | RawContainer[]>('/docker/containers', { signal })
        .catch(() => undefined)
      const publicNetworkRequest = request<RawPublicNetworkSummary>('/system/public-network', { signal })
        .catch(() => undefined)
      const appsRequest = request<AppMarketInventory>('/apps', { signal })
        .catch(() => undefined)
      const agent = await agentRequest
      let capabilitiesResult: ApiList<Capability> | Capability[] | undefined
      let sitesResult: ApiList<RawSite> | RawSite[] | undefined
      let appsResult: AppMarketInventory | undefined
      let dockerResult: RawDockerSummary | undefined
      let containersResult: ApiList<RawContainer> | RawContainer[] | undefined
      let publicNetwork: RawPublicNetworkSummary | undefined = system.publicNetwork
      let capabilities: SystemOverview['management']['capabilities'] = {}
      let sitesSummary: SystemOverview['sites']
      let containersSummary: SystemOverview['containers']
      let appsSummary: SystemOverview['apps']
      let services: SystemOverview['services'] = []
      let previousOverview: SystemOverview | undefined
      let revision = 0
      let builtRevision = -1
      const knownServices = [
        { id: 'nginx', name: 'Nginx' },
        { id: 'mysql', name: 'MySQL' },
        { id: 'php', name: 'PHP' },
        { id: 'php74', name: 'PHP 7.4' },
        { id: 'redis', name: 'Redis' },
      ]
      const refreshServices = () => {
        if (!dockerResult) {
          services = []
          return
        }
        const containers = normalizeList(containersResult).items
        services = [
          {
            id: 'docker',
            name: 'Docker Engine',
            state: dockerResult.available ? 'running' : 'stopped',
            version: dockerResult.serverVersion,
          },
          ...knownServices.flatMap((known) => {
            const container = containers.find((item) => item.name.replace(/^\/+/, '') === known.id)
            if (!container) return []
            const state: SystemOverview['services'][number]['state'] =
              container.state === 'running'
                ? 'running'
                : container.state === 'paused' || container.state === 'restarting'
                  ? 'degraded'
                  : ['exited', 'dead', 'created'].includes(container.state)
                    ? 'stopped'
                    : 'unknown'
            return [{ id: known.id, name: known.name, state, detail: container.image }]
          }),
        ]
      }
      const publicNetworkSummary = (): SystemOverview['publicNetwork'] => ({
        ipv4: publicNetwork?.ipv4,
        ipv6: publicNetwork?.ipv6,
        isp: publicNetwork?.isp,
        country: publicNetwork?.country,
        countryCode: publicNetwork?.countryCode,
        region: publicNetwork?.region,
        city: publicNetwork?.city,
        timezone: publicNetwork?.timezone,
        source: publicNetwork?.source,
        updatedAt: publicNetwork?.updatedAt,
      })
      const build = (): SystemOverview => {
        if (previousOverview && builtRevision === revision) return previousOverview
        if (previousOverview) {
          previousOverview = {
            ...previousOverview,
            publicNetwork: publicNetworkSummary(),
            management:
              previousOverview.management.capabilities === capabilities
                ? previousOverview.management
                : { ...previousOverview.management, capabilities },
            services,
            sites: sitesSummary,
            containers: containersSummary,
            apps: appsSummary,
          }
          builtRevision = revision
          return previousOverview
        }
        const overview: SystemOverview = {
          ...normalizeSystemResources(system),
          publicNetwork: publicNetworkSummary(),
          management: {
          ssh: {
            ports: system.management?.ssh?.ports || [],
            source:
              system.management?.ssh?.source === 'configured' || system.management?.ssh?.source === 'default'
                ? system.management.ssh.source
                : 'unknown',
            defense: {
              available: Boolean(system.management?.ssh?.defense?.available),
              installed: Boolean(system.management?.ssh?.defense?.installed),
              running: Boolean(system.management?.ssh?.defense?.running),
              enabled: Boolean(system.management?.ssh?.defense?.enabled),
              autostart: Boolean(system.management?.ssh?.defense?.autostart),
              jail: system.management?.ssh?.defense?.jail,
              banned: Math.max(0, Number(system.management?.ssh?.defense?.banned) || 0),
              message: system.management?.ssh?.defense?.message,
            },
          },
          dns: {
            servers: system.management?.dns?.servers || [],
            manager: system.management?.dns?.manager || 'unknown',
          },
          timezone: system.management?.timezone,
          swap: {
            totalBytes: system.memory.swapTotalBytes || 0,
            usedBytes: system.memory.swapUsedBytes || 0,
            activeDevices: system.management?.swap?.activeDevices || 0,
            path: system.management?.swap?.path || '/swapfile',
            fileExists: Boolean(system.management?.swap?.fileExists),
            fileActive: Boolean(system.management?.swap?.fileActive),
            fileSizeBytes: system.management?.swap?.fileSizeBytes || 0,
            fileUsedBytes: system.management?.swap?.fileUsedBytes || 0,
            legacyExists: Boolean(system.management?.swap?.legacyExists),
            legacyActive: Boolean(system.management?.swap?.legacyActive),
            legacySizeBytes: system.management?.swap?.legacySizeBytes || 0,
            otherActiveDevices: system.management?.swap?.otherActiveDevices || 0,
            otherSwapTotalBytes: system.management?.swap?.otherSwapTotalBytes || 0,
            otherSwapUsedBytes: system.management?.swap?.otherSwapUsedBytes || 0,
          },
          packageManager: system.management?.packageManager,
          packageSources: system.management?.packageSources || [],
          maintenance: normalizeMaintenance(system.management?.maintenance),
          ipPreference:
            system.management?.ipPreference === 'ipv4' || system.management?.ipPreference === 'system_default'
              ? system.management.ipPreference
              : 'unknown',
          kernelOptimization: {
            enabled: Boolean(system.management?.kernelOptimization?.enabled),
            profile: system.management?.kernelOptimization?.profile,
            source: system.management?.kernelOptimization?.source,
          },
          bbr: {
            supported: Boolean(system.management?.bbr?.supported),
            enabled: Boolean(system.management?.bbr?.enabled),
            congestionControl: system.management?.bbr?.congestionControl,
            defaultQDisc: system.management?.bbr?.defaultQDisc,
            available: system.management?.bbr?.available || [],
          },
          bbrv3: {
            available: Boolean(system.management?.bbrv3?.available),
            supported: Boolean(system.management?.bbrv3?.supported),
            installed: Boolean(system.management?.bbrv3?.installed),
            active: Boolean(system.management?.bbrv3?.active),
            architecture: system.management?.bbrv3?.architecture,
            os: system.management?.bbrv3?.os,
            codename: system.management?.bbrv3?.codename,
            runningKernel: system.management?.bbrv3?.runningKernel,
            installedKernel: system.management?.bbrv3?.installedKernel,
            congestionControl: system.management?.bbrv3?.congestionControl,
            defaultQDisc: system.management?.bbrv3?.defaultQDisc,
            rebootRequired: Boolean(system.management?.bbrv3?.rebootRequired),
            reason: system.management?.bbrv3?.reason,
          },
          capabilities,
        },
        services,
        agent: normalizeAgent(agent),
        sites: sitesSummary,
        containers: containersSummary,
        apps: appsSummary,
        }
        previousOverview = overview
        builtRevision = revision
        return overview
      }

      onUpdate?.(build())
      const emit = () => onUpdate?.(build())
      await Promise.allSettled([
        capabilitiesRequest.then((value) => {
          if (value === undefined) return
          capabilitiesResult = value
          capabilities = Object.fromEntries(
            normalizeList(capabilitiesResult).items.map((capability) => [
              capability.id,
              {
                enabled: capability.enabled,
                reason: capability.reason,
                methods: capability.methods,
              },
            ]),
          )
          revision += 1
          emit()
        }),
        sitesRequest.then((value) => {
          if (value === undefined) return
          sitesResult = value
          const sites = normalizeList(sitesResult).items.map(normalizeSite)
          sitesSummary = {
            total: sites.length,
            healthy: sites.filter((site) => site.health === 'healthy').length,
            drifted: sites.filter((site) => site.consistency !== 'synced').length,
          }
          revision += 1
          emit()
        }),
        dockerRequest.then((value) => {
          if (value === undefined) return
          dockerResult = value
          containersSummary = {
            total: dockerResult.containers,
            running: dockerResult.running,
            stopped: dockerResult.stopped,
          }
          refreshServices()
          revision += 1
          emit()
        }),
        containersRequest.then((value) => {
          if (value === undefined) return
          containersResult = value
          refreshServices()
          revision += 1
          emit()
        }),
        publicNetworkRequest.then((value) => {
          if (value === undefined) return
          publicNetwork = value
          revision += 1
          emit()
        }),
        appsRequest.then((value) => {
          if (value === undefined) return
          appsResult = value
          appsSummary = {
            total: appsResult.items.length,
            installed: appsResult.installed,
            running: appsResult.running,
            updateAvailable: appsResult.updateAvailable,
          }
          revision += 1
          emit()
        }),
      ])
      return build()
    },
  },
  cluster: {
    hosts: (signal?: AbortSignal): Promise<ClusterHostList> =>
      request<ClusterHostList>('/cluster/hosts', { signal }),
    shareSettings: (signal?: AbortSignal): Promise<ClusterShareSettings> =>
      request<ClusterShareSettings>('/cluster/share', { signal }),
    updateShare: (body: {
      enabled: boolean
      title: string
      description: string
      expectedResourceVersion: string
    }): Promise<ClusterShareSettings> =>
      request<ClusterShareSettings>('/cluster/share', { method: 'PUT', body }),
    resetShareToken: (expectedResourceVersion: string): Promise<ClusterShareSettings> =>
      request<ClusterShareSettings>('/cluster/share/token', {
        method: 'POST',
        body: { expectedResourceVersion },
      }),
    publicShare: (token: string, signal?: AbortSignal): Promise<PublicClusterShareSnapshot> =>
      request<PublicClusterShareSnapshot>(`/public/cluster-share/${encodeURIComponent(token)}`, { signal }),
    host: (id: string, signal?: AbortSignal): Promise<ClusterHost> =>
      request<ClusterHost>(`/cluster/hosts/${encodeURIComponent(id)}`, { signal }),
    add: (body: { name?: string; origin: string; pairingCode: string }): Promise<ClusterHost> =>
      request<ClusterHost>('/cluster/hosts', { method: 'POST', body }),
    rename: (
      id: string,
      body: { name: string; expectedResourceVersion: string },
    ): Promise<ClusterHost> =>
      request<ClusterHost>(`/cluster/hosts/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        body,
      }),
    remove: (
      id: string,
      expectedResourceVersion: string,
    ): Promise<{ deleted: boolean; remoteRevoked: boolean; credentialRemoved?: boolean }> =>
      request<{ deleted: boolean; remoteRevoked: boolean; credentialRemoved?: boolean }>(
        `/cluster/hosts/${encodeURIComponent(id)}`,
        { method: 'DELETE', body: { expectedResourceVersion } },
      ),
    refresh: (id: string): Promise<ClusterHost> =>
      request<ClusterHost>(`/cluster/hosts/${encodeURIComponent(id)}/refresh`, {
        method: 'POST',
      }),
    enableMutualFiles: (id: string): Promise<ClusterHost> =>
      request<ClusterHost>(`/cluster/hosts/${encodeURIComponent(id)}/mutual-files`, {
        method: 'POST',
      }),
    createPairingCode: (): Promise<ClusterPairingCode> =>
      request<ClusterPairingCode>('/cluster/pairing-codes/v2', { method: 'POST' }),
    createLightEnrollment: (): Promise<ClusterLightEnrollment> =>
      request<ClusterLightEnrollment>('/cluster/light-enrollments', { method: 'POST' }),
    controllers: async (signal?: AbortSignal): Promise<ApiList<ClusterController>> =>
      normalizeList(
        await request<ApiList<ClusterController> | ClusterController[]>(
          '/cluster/controllers',
          { signal },
        ),
      ),
    revokeController: (id: string): Promise<{ deleted: boolean }> =>
      request<{ deleted: boolean }>(`/cluster/controllers/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
  },
  terminals: {
    open: (hostId: string, rows: number, columns: number): Promise<TerminalSession> =>
      request<TerminalSession>('/terminal-sessions', {
        method: 'POST',
        body: { hostId, rows, columns },
      }),
    output: (
      sessionId: string,
      offset: number,
      signal?: AbortSignal,
    ): Promise<TerminalOutput> =>
      request<TerminalOutput>(`/terminal-sessions/${encodeURIComponent(sessionId)}/output`, {
        query: { offset, wait: 1000 },
        signal,
        // TerminalOutput has its own `data` field and is not an API envelope.
        unwrapEnvelope: false,
      }),
    input: (sessionId: string, data: string): Promise<{ accepted: boolean }> =>
      request<{ accepted: boolean }>(`/terminal-sessions/${encodeURIComponent(sessionId)}/input`, {
        method: 'POST', body: { data },
      }),
    resize: (sessionId: string, rows: number, columns: number): Promise<{ accepted: boolean }> =>
      request<{ accepted: boolean }>(`/terminal-sessions/${encodeURIComponent(sessionId)}/resize`, {
        method: 'POST', body: { rows, columns },
      }),
    close: (sessionId: string): Promise<{ closed: boolean }> =>
      request<{ closed: boolean }>(`/terminal-sessions/${encodeURIComponent(sessionId)}/close`, {
        method: 'POST', body: {},
      }),
  },
  system: {
    resources: async (signal?: AbortSignal): Promise<SystemResourceSnapshot> =>
      normalizeSystemResources(await request<RawSystemSummary>('/system/summary', { signal })),
    processes: (query: ProcessQuery = {}, signal?: AbortSignal): Promise<ProcessSnapshot> =>
      request<ProcessSnapshot>('/system/processes', {
        query: {
          q: query.search,
          sort: query.sort,
          order: query.order,
          limit: query.limit,
        },
        signal,
      }),
    action: (body: SystemActionInput): Promise<SystemActionResult> =>
      request<SystemActionResult>('/system/actions', { method: 'POST', body }),
    hosts: (signal?: AbortSignal): Promise<HostsSnapshot> =>
      request<HostsSnapshot>('/system/hosts', { signal }),
    cron: (signal?: AbortSignal): Promise<CronSnapshot> =>
      request<CronSnapshot>('/system/cron', { signal }),
    networkInterfaces: (signal?: AbortSignal): Promise<NetworkInterfacesSnapshot> =>
      request<NetworkInterfacesSnapshot>('/system/network-interfaces', { signal }),
    firewall: (signal?: AbortSignal): Promise<FirewallSnapshot> =>
      request<FirewallSnapshot>('/system/firewall', { signal }),
	portUsage: (signal?: AbortSignal): Promise<PortUsageSnapshot> =>
		request<PortUsageSnapshot>('/system/port-usage', { signal }),
	trafficShutdown: (signal?: AbortSignal): Promise<TrafficShutdownSnapshot> =>
		request<TrafficShutdownSnapshot>('/system/traffic-shutdown', { signal }),
	trafficShutdownAction: (body: TrafficShutdownActionInput): Promise<TrafficShutdownActionResult> =>
		request<TrafficShutdownActionResult>('/system/traffic-shutdown/actions', { method: 'POST', body }),
	accounts: async (signal?: AbortSignal): Promise<AccountManagementSnapshot> =>
		normalizeAccountManagementSnapshot(await request<AccountManagementSnapshot>('/system/accounts', { signal })),
	accountAction: (body: AccountManagementActionInput): Promise<AccountManagementActionResult> =>
		request<AccountManagementActionResult>('/system/account-actions', { method: 'POST', body }),
	sshDefense: (signal?: AbortSignal): Promise<SSHDefenseSnapshot> =>
		request<SSHDefenseSnapshot>('/system/ssh-defense', { signal }),
	sshDefenseAction: (body: SSHDefenseActionInput): Promise<SSHDefenseActionResult> =>
		request<SSHDefenseActionResult>('/system/ssh-defense/actions', { method: 'POST', body }),
	systemTuning: (signal?: AbortSignal): Promise<SystemTuningSnapshot> =>
		request<SystemTuningSnapshot>('/system/system-tuning', { signal }),
	systemTuningAction: (body: SystemTuningActionInput): Promise<SystemTuningActionResult> =>
		request<SystemTuningActionResult>('/system/system-tuning/actions', { method: 'POST', body }),
    resourceAction: (body: SystemResourceActionInput): Promise<SystemResourceActionResult> =>
      request<SystemResourceActionResult>('/system/resource-actions', { method: 'POST', body }),
    maintenance: async (
      signal?: AbortSignal,
    ): Promise<SystemOverview['management']['maintenance']> => {
      const summary = await request<RawSystemSummary>('/system/summary', { signal })
      return normalizeMaintenance(summary.management?.maintenance)
    },
    publicNetwork: (signal?: AbortSignal): Promise<RawPublicNetworkSummary> =>
      request<RawPublicNetworkSummary>('/system/public-network', { signal }),
  },
  sites: {
    iconURL: (id: string): string =>
      buildUrl(`/sites/${encodeURIComponent(id)}/icon`),
    appearance: (id: string, signal?: AbortSignal): Promise<SiteAppearance> =>
      request<SiteAppearance>(`/sites/${encodeURIComponent(id)}/appearance`, { signal }),
    list: async (query?: { search?: string; cursor?: string }, signal?: AbortSignal): Promise<ApiList<Site>> => {
      const result = normalizeList(await request<ApiList<RawSite> | RawSite[]>('/sites', { query, signal }))
      return { ...result, items: result.items.map(normalizeSite) }
    },
    create: createSite,
    installations: async (signal?: AbortSignal): Promise<SiteInstallationProgress[]> => {
      const result = normalizeList(
        await request<ApiList<RawSiteInstallJob> | RawSiteInstallJob[]>('/site-installations', { signal }),
      )
      return result.items.map(normalizeSiteInstallationProgress)
    },
    installation: async (id: string, signal?: AbortSignal): Promise<SiteInstallationProgress> =>
      normalizeSiteInstallationProgress(
        await request<RawSiteInstallJob>(`/site-installations/${encodeURIComponent(id)}`, { signal }),
      ),
    terminal: (
      id: string,
      offset = 0,
      inputOpen = false,
      signal?: AbortSignal,
    ): Promise<AppTerminalChunk> =>
      request<AppTerminalChunk>(`/site-installations/${encodeURIComponent(id)}/terminal`, {
        query: { offset, wait: 1000, inputOpen },
        signal,
      }),
    terminalInput: (id: string, data: string): Promise<{ ok: boolean }> =>
      request<{ ok: boolean }>(`/site-installations/${encodeURIComponent(id)}/input`, {
        method: 'POST',
        body: { data },
      }),
    update: async (id: string, body: SiteInput): Promise<Site> =>
      normalizeSite(await request<RawSite>(`/sites/${encodeURIComponent(id)}`, { method: 'PATCH', body })),
    remove: (id: string, primaryDomain: string) =>
      request<SiteDeleteResult>(
        `/sites/${encodeURIComponent(id)}`,
        {
          method: 'DELETE',
          body: { primaryDomain },
        },
      ),
  },
  webEnvironment: {
    summary: (signal?: AbortSignal): Promise<WebEnvironmentSummary> =>
      request<WebEnvironmentSummary>('/web-environment', { signal }),
    catalog: (signal?: AbortSignal): Promise<WebEnvironmentCatalog> =>
      request<WebEnvironmentCatalog>('/web-environment/catalog', { signal }),
    backups: async (signal?: AbortSignal): Promise<ApiList<WebEnvironmentBackup>> =>
      normalizeList(
        await request<ApiList<WebEnvironmentBackup> | WebEnvironmentBackup[]>(
          '/web-environment/backups',
          { signal },
        ),
      ),
    jobs: async (signal?: AbortSignal): Promise<ApiList<WebEnvironmentJob>> =>
      normalizeList(
        await request<ApiList<WebEnvironmentJob> | WebEnvironmentJob[]>('/web-environment/jobs', {
          signal,
        }),
      ),
    job: (id: string, signal?: AbortSignal): Promise<WebEnvironmentJob> =>
      request<WebEnvironmentJob>(`/web-environment/jobs/${encodeURIComponent(id)}`, { signal }),
    terminal: (
      id: string,
      offset = 0,
      inputOpen = false,
      signal?: AbortSignal,
    ): Promise<AppTerminalChunk> =>
      request<AppTerminalChunk>(`/web-environment/jobs/${encodeURIComponent(id)}/terminal`, {
        query: { offset, wait: 1000, inputOpen },
        signal,
      }),
    terminalInput: (id: string, data: string): Promise<void> =>
      request<void>(`/web-environment/jobs/${encodeURIComponent(id)}/input`, {
        method: 'POST',
        body: { data },
      }),
    start: (body: WebEnvironmentActionInput): Promise<WebEnvironmentJob> =>
      request<WebEnvironmentJob>('/web-environment/jobs', { method: 'POST', body }),
    backupDownloadURL: (id: string): string =>
      buildUrl(`/web-environment/backups/${encodeURIComponent(id)}`),
  },
  apps: {
    inventory: (signal?: AbortSignal): Promise<AppMarketInventory> =>
      request<AppMarketInventory>('/apps', { signal }),
    installPort: (
      id: string,
      port: number,
      signal?: AbortSignal,
    ): Promise<AppInstallPortStatus> =>
      request<AppInstallPortStatus>(`/apps/${encodeURIComponent(id)}/install-port`, {
        query: { port },
        signal,
      }),
    install: (
      id: string,
      body: {
        hostPort?: number
        accessMode?: 'direct' | 'domain_only'
      },
    ): Promise<AppInstallJob> =>
      request<AppInstallJob>(`/apps/${encodeURIComponent(id)}/install`, { method: 'POST', body }),
    job: (id: string, signal?: AbortSignal): Promise<AppInstallJob> =>
      request<AppInstallJob>(`/app-jobs/${encodeURIComponent(id)}`, { signal }),
    jobs: async (signal?: AbortSignal): Promise<ApiList<AppInstallJob>> =>
      normalizeList(await request<ApiList<AppInstallJob> | AppInstallJob[]>('/app-jobs', { signal })),
    terminal: (
      id: string,
      offset: number,
      inputOpen = false,
      signal?: AbortSignal,
    ): Promise<AppTerminalChunk> =>
      request<AppTerminalChunk>(`/app-jobs/${encodeURIComponent(id)}/terminal`, {
        query: { offset, wait: 1000, inputOpen },
        signal,
      }),
    terminalInput: (id: string, data: string): Promise<{ ok: boolean }> =>
      request<{ ok: boolean }>(`/app-jobs/${encodeURIComponent(id)}/input`, {
        method: 'POST',
        body: { data },
      }),
    cancelJob: (id: string): Promise<AppInstallJob> =>
      request<AppInstallJob>(`/app-jobs/${encodeURIComponent(id)}/cancel`, {
        method: 'POST',
      }),
    action: (
      id: string,
      action: 'start' | 'stop' | 'restart' | 'update' | 'uninstall' | 'direct_access' | 'manage',
      body: { resourceVersion: string; accessMode?: 'direct' | 'domain_only' },
    ): Promise<AppMutationResult | AppInstallJob> =>
      request<AppMutationResult | AppInstallJob>(`/apps/${encodeURIComponent(id)}/${action}`, {
        method: 'POST',
        body,
      }),
    checkUpdate: (id: string, resourceVersion: string): Promise<AppImageUpdateResult> =>
      request<AppImageUpdateResult>(`/apps/${encodeURIComponent(id)}/check_update`, {
        method: 'POST',
        body: { resourceVersion },
      }),
  },
  diagnostics: {
    catalog: (signal?: AbortSignal): Promise<DiagnosticCatalog> =>
      request<DiagnosticCatalog>('/diagnostics', { signal }),
    jobs: async (signal?: AbortSignal): Promise<ApiList<DiagnosticJob>> =>
      normalizeList(
        await request<ApiList<DiagnosticJob> | DiagnosticJob[]>('/diagnostic-jobs', { signal }),
      ),
    job: (id: string, signal?: AbortSignal): Promise<DiagnosticJob> =>
      request<DiagnosticJob>(`/diagnostic-jobs/${encodeURIComponent(id)}`, { signal }),
    terminal: (
      id: string,
      offset = 0,
      inputOpen = false,
      signal?: AbortSignal,
    ): Promise<AppTerminalChunk> =>
      request<AppTerminalChunk>(`/diagnostic-jobs/${encodeURIComponent(id)}/terminal`, {
        query: { offset, wait: 1000, inputOpen },
        signal,
      }),
    terminalInput: (id: string, data: string): Promise<{ ok: boolean }> =>
      request<{ ok: boolean }>(`/diagnostic-jobs/${encodeURIComponent(id)}/input`, {
        method: 'POST',
        body: { data },
      }),
    start: (checkId: string): Promise<DiagnosticJob> =>
      request<DiagnosticJob>('/diagnostic-jobs', {
        method: 'POST',
        body: { checkId },
      }),
  },
  files: {
    entry: (path: string, signal?: AbortSignal): Promise<FileEntry> =>
      request<FileEntry>('/files/entry', { query: { path }, signal }),
    entries: (paths: readonly string[], signal?: AbortSignal): Promise<FileEntryBatchResult> =>
      request<FileEntryBatchResult>('/files/entries', {
        method: 'POST',
        body: { paths },
        signal,
      }),
    list: (
      path = '/',
      options?: { offset?: number; search?: string },
      signal?: AbortSignal,
    ): Promise<FileDirectory> =>
      request<FileDirectory>('/files', {
        query: { path, limit: 100, offset: options?.offset, search: options?.search },
        signal,
      }),
    contentUrl: (path: string, disposition: 'inline' | 'attachment' = 'inline'): string =>
      buildUrl('/files/content', { path, disposition }),
    archiveUrl: (
      entries: readonly Pick<FileEntry, 'path' | 'resourceVersion'>[],
      name: string,
    ): string => buildUrl('/files/archive', {
      selection: JSON.stringify({
        sources: entries.map((entry) => entry.path),
        expectedResourceVersions: Object.fromEntries(
          entries.map((entry) => [entry.path, entry.resourceVersion]),
        ),
      }),
      name,
    }),
    createDownloadTicket: (path: string): Promise<FileDownloadTicket> =>
      request<FileDownloadTicket>('/files/download-tickets', {
        method: 'POST',
        body: { path },
      }),
    createArchiveDownloadTicket: (
      entries: readonly Pick<FileEntry, 'path' | 'resourceVersion'>[],
      name: string,
    ): Promise<FileDownloadTicket> =>
      request<FileDownloadTicket>('/files/archive-download-tickets', {
        method: 'POST',
        body: {
          sources: entries.map((entry) => entry.path),
          expectedResourceVersions: Object.fromEntries(
            entries.map((entry) => [entry.path, entry.resourceVersion]),
          ),
          name,
        },
      }),
    share: (
      path: string,
      resourceVersion: string,
      signal?: AbortSignal,
    ): Promise<FileShareLookup> =>
      request<FileShareLookup>('/files/shares', {
        query: { path, resourceVersion },
        signal,
      }),
    shares: (signal?: AbortSignal): Promise<FileShareList> =>
      request<FileShareList>('/files/shares', { signal }),
    createShare: (body: FileShareCreateInput): Promise<FileShareAdminView> =>
      request<FileShareAdminView>('/files/shares', {
        method: 'POST',
        body,
      }),
    deleteShare: (id: string): Promise<void> =>
      request<void>(`/files/shares/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    publicShare: (token: string, signal?: AbortSignal): Promise<PublicFileShareView> =>
      request<PublicFileShareView>(`/public/file-shares/${encodeURIComponent(token)}`, { signal }),
    transferFromPanel: async (
      input: CrossPanelFileTransferInput,
      onEvent: (event: CrossPanelFileTransferEvent) => void,
      signal?: AbortSignal,
    ): Promise<FileEntry> => {
      const headers = new Headers({ Accept: 'application/x-ndjson', 'Content-Type': 'application/json' })
      if (csrfToken) headers.set('X-CSRF-Token', csrfToken)
      let response: Response
      try {
        response = await fetch(buildUrl('/files/transfers'), {
          method: 'POST', credentials: 'same-origin', cache: 'no-store', headers,
          body: JSON.stringify(input), signal,
        })
      } catch (error) {
        if (error instanceof DOMException && error.name === 'AbortError') throw error
        throw new ApiError('无法连接到面板服务，请检查服务状态后重试。', 0, 'network_error', error)
      }
      if (!response.ok) {
        const payload = await parsePayload(response)
        const problem = payload && typeof payload === 'object' ? payload as ProblemPayload : undefined
        throw new ApiError(
          problem?.detail || problem?.title || '跨面板复制失败。',
          response.status, problem?.code || 'file_transfer_failed', payload, problem?.requestId,
        )
      }
      if (!response.body) throw new ApiError('浏览器不支持流式传输状态。', 0, 'stream_unavailable')
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffered = ''
      let completed: FileEntry | undefined
      const consume = (line: string): void => {
        if (!line.trim()) return
        let event: CrossPanelFileTransferEvent
        try {
          event = JSON.parse(line) as CrossPanelFileTransferEvent
        } catch {
          throw new ApiError('面板返回了无效的传输状态。', 0, 'file_transfer_response_invalid')
        }
        onEvent(event)
        if (event.state === 'error') throw new ApiError(event.detail || '跨面板复制失败。', 0, 'file_transfer_failed')
        if (event.state === 'complete' && event.entry) completed = event.entry
      }
      try {
        while (true) {
          const { value, done } = await reader.read()
          buffered += decoder.decode(value, { stream: !done })
          const lines = buffered.split('\n')
          buffered = lines.pop() || ''
          for (const line of lines) consume(line)
          if (done) break
        }
        consume(buffered)
      } catch (error) {
        await reader.cancel().catch(() => undefined)
        throw error
      }
      if (!completed) throw new ApiError('跨面板复制未正常结束。', 0, 'file_transfer_incomplete')
      return completed
    },
    thumbnailUrl: (path: string, version: string): string =>
      buildUrl('/files/content', { path, disposition: 'inline', mode: 'thumbnail', version }),
    text: async (path: string): Promise<string> =>
      (
        await rawFileResponse('/files/content', {
          query: { path, disposition: 'inline', mode: 'text' },
        })
      ).text(),
    write: (path: string, content: string, expectedResourceVersion: string): Promise<FileWriteResult> =>
      request<FileWriteResult>('/files/content', {
        method: 'PUT',
        query: { path },
        body: { content, expectedResourceVersion },
      }),
    upload: async (
      path: string,
      file: File,
      overwrite = false,
      onProgress?: (percent: number) => void,
      signal?: AbortSignal,
    ): Promise<FileEntry> =>
      new Promise<FileEntry>((resolve, reject) => {
        const xhr = new XMLHttpRequest()
        let settled = false
        const finish = (callback: () => void): void => {
          if (settled) return
          settled = true
          signal?.removeEventListener('abort', abort)
          callback()
        }
        const abort = (): void => xhr.abort()
        xhr.open('POST', buildUrl('/files/upload', { path, name: file.name, overwrite }))
        xhr.withCredentials = true
        xhr.responseType = 'json'
        xhr.setRequestHeader('Content-Type', 'application/octet-stream')
        if (csrfToken) xhr.setRequestHeader('X-CSRF-Token', csrfToken)
        xhr.upload.onprogress = (event) => {
          if (event.lengthComputable) onProgress?.(Math.round((event.loaded / event.total) * 100))
        }
        xhr.onerror = () => finish(() => reject(new ApiError('文件上传连接中断。', 0, 'network_error')))
        xhr.onabort = () => finish(() => reject(new DOMException('文件上传已取消。', 'AbortError')))
        xhr.onload = () => {
          const payload = xhr.response
          if (xhr.status >= 200 && xhr.status < 300) {
            finish(() => resolve(payload as FileEntry))
            return
          }
          const problem = payload && typeof payload === 'object' ? (payload as ProblemPayload) : undefined
          finish(() => reject(
            new ApiError(
              problem?.detail || problem?.title || '文件上传失败。',
              xhr.status,
              problem?.code || 'file_upload_failed',
              payload,
              problem?.requestId,
            ),
          ))
        }
        if (signal?.aborted) {
          finish(() => reject(new DOMException('文件上传已取消。', 'AbortError')))
          return
        }
        signal?.addEventListener('abort', abort, { once: true })
        xhr.send(file)
      }),
    action: (input: FileActionInput, signal?: AbortSignal): Promise<FileActionResult> =>
      request<FileActionResult>('/files/actions', { method: 'POST', body: input, signal }),
    trash: (): Promise<FileTrashDirectory> => request<FileTrashDirectory>('/files/trash'),
  },
  docker: {
    environment: (signal?: AbortSignal): Promise<DockerEnvironment> =>
      request<DockerEnvironment>('/docker/environment', { signal }),
    inventory: async (
      signal?: AbortSignal,
      onUpdate?: (inventory: DockerInventory) => void,
    ): Promise<DockerInventory> => {
      const [summary, containersResult, composeProjectsResult] = await Promise.all([
        request<RawDockerSummary>('/docker/summary', { signal }),
        request<ApiList<RawContainer> | RawContainer[]>('/docker/containers', { signal }),
        request<ApiList<{ name: string }> | { name: string }[]>('/docker/compose-projects', { signal })
          .catch(() => ({ items: [], total: 0 })),
      ])
      const inventory: DockerInventory = {
        available: summary.available,
        version: summary.serverVersion,
        observedAt: summary.collectedAt,
        containers: normalizeList(containersResult).items.map(normalizeContainer),
        composeProjects: normalizeList(composeProjectsResult).items.map((item) => item.name),
        images: [],
        networks: [],
        volumes: [],
        loading: { images: true, networks: true, volumes: true },
        errors: {},
      }
      const emit = () =>
        onUpdate?.({
          ...inventory,
          images: [...inventory.images],
          networks: [...inventory.networks],
          volumes: [...inventory.volumes],
          loading: { ...inventory.loading },
          errors: { ...inventory.errors },
        })
      emit()
      const loadResource = async <K extends 'images' | 'networks' | 'volumes'>(
        key: K,
        path: string,
      ): Promise<void> => {
        try {
          if (key === 'images') {
            const result = await request<ApiList<RawDockerImage> | RawDockerImage[]>(path, { signal })
            inventory.images = normalizeList(result).items.map((item) => ({
              id: item.id,
              tags: item.repoTags || [],
              digests: item.repoDigests || [],
              sizeBytes: item.sizeBytes,
              createdAt: item.createdAt ? new Date(item.createdAt * 1_000).toISOString() : undefined,
              inUse: Number(item.containers || 0) > 0,
              resourceVersion: item.resourceVersion,
            }))
          } else if (key === 'networks') {
            const result = await request<ApiList<RawDockerNetwork> | RawDockerNetwork[]>(path, { signal })
            inventory.networks = normalizeList(result).items.map((item) => ({
              id: item.id,
              name: item.name,
              driver: item.driver,
              scope: item.scope,
              containers: item.containerCount || 0,
              resourceVersion: item.resourceVersion,
            }))
          } else {
            const result = await request<ApiList<RawDockerVolume> | RawDockerVolume[]>(path, { signal })
            const usedVolumes = new Set(
              normalizeList(containersResult).items.flatMap((container) =>
                (container.mounts || [])
                  .filter((mount) => mount.type === 'volume' && mount.name)
                  .map((mount) => String(mount.name)),
              ),
            )
            inventory.volumes = normalizeList(result).items.map((item) => ({
              name: item.name,
              driver: item.driver,
              mountpoint: item.mountpoint,
              inUse: usedVolumes.has(item.name),
              resourceVersion: item.resourceVersion,
            }))
          }
        } catch (reason) {
          if (reason instanceof DOMException && reason.name === 'AbortError') throw reason
          inventory.errors![key] = reason instanceof ApiError ? reason.message : `${key} 读取失败`
        } finally {
          inventory.loading![key] = false
          emit()
        }
      }
      await Promise.allSettled([
        loadResource('images', '/docker/images'),
        loadResource('networks', '/docker/networks'),
        loadResource('volumes', '/docker/volumes'),
      ])
      return inventory
    },
    action: (
      id: string,
      action: 'start' | 'stop' | 'restart' | 'pause' | 'unpause' | 'remove',
      resourceVersion: string,
    ) =>
      request<DockerActionResult>(`/docker/containers/${encodeURIComponent(id)}/${action}`, {
        method: 'POST',
        body: { resourceVersion },
      }),
    logs: (id: string, tail = 200, signal?: AbortSignal) =>
      request<{ lines: string[]; truncated?: boolean }>(`/docker/containers/${encodeURIComponent(id)}/logs`, {
        query: { tail },
        signal,
      }),
    stats: (id: string, signal?: AbortSignal): Promise<DockerContainerStats> =>
      request<DockerContainerStats>(`/docker/containers/${encodeURIComponent(id)}/stats`, { signal }),
    exec: (id: string, resourceVersion: string, command: string): Promise<DockerExecResult> =>
      request<DockerExecResult>(`/docker/containers/${encodeURIComponent(id)}/exec`, {
        method: 'POST',
        body: { resourceVersion, command },
      }),
    backups: async (signal?: AbortSignal): Promise<ApiList<DockerBackup>> =>
      normalizeList(await request<ApiList<DockerBackup> | DockerBackup[]>('/docker/backups', { signal })),
    composeProject: (name: string, signal?: AbortSignal): Promise<DockerComposeProject> =>
      request<DockerComposeProject>(`/docker/compose-projects/${encodeURIComponent(name)}`, { signal }),
    task: (body: DockerMaintenanceInput): Promise<DockerMaintenanceJob> =>
      request<DockerMaintenanceJob>('/docker/tasks', { method: 'POST', body }),
    job: (id: string, signal?: AbortSignal): Promise<DockerMaintenanceJob> =>
      request<DockerMaintenanceJob>(`/docker/jobs/${encodeURIComponent(id)}`, { signal }),
    jobs: async (signal?: AbortSignal): Promise<ApiList<DockerMaintenanceJob>> =>
      normalizeList(await request<ApiList<DockerMaintenanceJob> | DockerMaintenanceJob[]>('/docker/jobs', { signal })),
  },
  monitoring: {
    history: (
      range: MonitoringRange,
      query?: MonitoringHistoryQuery,
      signal?: AbortSignal,
    ): Promise<MonitoringHistory> =>
      request<MonitoringHistory>('/monitoring/history', {
        query: { range, start: query?.start, end: query?.end },
        signal,
      }),
  },
  jobs: {
    list: async (query?: { limit?: number }, signal?: AbortSignal): Promise<ApiList<Job>> => {
      const result = normalizeList(await request<ApiList<RawJob> | RawJob[]>('/jobs', { query, signal }))
      return { ...result, items: result.items.map(normalizeJob) }
    },
  },
  audit: {
    list: async (
      query?: { source?: string; outcome?: string; cursor?: string },
      signal?: AbortSignal,
    ): Promise<ApiList<AuditEvent>> => {
      const result = normalizeList(await request<ApiList<RawAuditEvent> | RawAuditEvent[]>('/audit', { query, signal }))
      return {
        ...result,
        items: result.items.map((item) => ({
          id: item.id,
          occurredAt: item.occurredAt,
          actor: item.actorId || item.actorType || 'system',
          source: (['web', 'cli', 'reconcile', 'system', 'external'].includes(item.actorType || '')
            ? item.actorType
            : 'system') as AuditEvent['source'],
          action: item.action,
          resourceType: item.targetKind,
          resourceName: item.targetId,
          outcome: (['success', 'failure', 'denied', 'observed'].includes(item.result || '')
            ? item.result
            : item.result === 'failed'
              ? 'failure'
              : 'observed') as AuditEvent['outcome'],
          requestId: item.requestId,
          remoteAddress: item.sourceIp,
        })),
      }
    },
  },
	settings: {
    get: (signal?: AbortSignal) => request<PanelSettings>('/settings', { signal }),
	securityEntrance: {
		get: () => request<SecurityEntranceSettings>('/settings/security-entry'),
		update: (input: {
			enabled: boolean
			path?: string
			regenerate?: boolean
			expectedResourceVersion: string
		}) => request<SecurityEntranceSettings>('/settings/security-entry', { method: 'PUT', body: input }),
	},
	totp: {
		status: () => request<TOTPStatus>('/settings/totp'),
		startEnrollment: (currentPassword: string) =>
			request<TOTPEnrollment>('/settings/totp/enrollment', {
				method: 'POST',
				body: { currentPassword },
			}),
		confirmEnrollment: (enrollmentId: string, code: string) =>
			request<TOTPRecoveryCodes>('/settings/totp/enrollment', {
				method: 'PUT',
				body: { enrollmentId, code },
			}),
		regenerateRecoveryCodes: (currentPassword: string, secondFactor: string) =>
			request<TOTPRecoveryCodes>('/settings/totp/recovery-codes', {
				method: 'POST',
				body: { currentPassword, secondFactor },
			}),
		disable: (currentPassword: string, secondFactor: string) =>
			request<{ ok: boolean }>('/settings/totp', {
				method: 'DELETE',
				body: { currentPassword, secondFactor },
			}),
	},
    changePassword: (currentPassword: string, newPassword: string) =>
      request<void>('/settings/password', {
        method: 'PUT',
        body: { currentPassword, newPassword },
      }),
    changeUsername: (currentPassword: string, newUsername: string) =>
      request<void>('/settings/username', {
        method: 'PUT',
        body: { currentPassword, newUsername },
      }),
  },
}

export function resetApiSecurityState(): void {
  csrfToken = ''
  previousNetworkSample = undefined
}

export function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return request<T>(path, options)
}
