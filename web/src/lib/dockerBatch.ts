import { formatBytes, shortId } from '@/lib/format'
import type {
  DockerContainer,
  DockerImage,
  DockerMaintenanceInput,
  DockerMaintenanceJob,
  DockerNetwork,
  DockerVolume,
} from '@/types/api'

// A batch is a client-side sequence of the existing single-resource calls, so
// every item keeps its own resourceVersion check, audit record and Agent-side
// validation. Nothing here adds a protocol action or widens Agent permissions.

export type DockerBatchTab = 'containers' | 'images' | 'networks' | 'volumes'
export type ContainerLifecycleAction = 'start' | 'stop' | 'restart' | 'pause' | 'unpause' | 'remove'
/** `start` also resumes paused containers: both mean "make it run". */
export type ContainerBatchAction = 'start' | 'restart' | 'pause' | 'stop' | 'remove'
export type ResourceBatchAction = 'image_pull' | 'image_remove' | 'network_remove' | 'volume_remove'
export type DockerBatchAction = ContainerBatchAction | ResourceBatchAction

export type DockerBatchOperation =
  | { kind: 'container'; id: string; action: ContainerLifecycleAction; resourceVersion: string }
  | { kind: 'task'; input: DockerMaintenanceInput }

export interface DockerBatchPlanItem {
  key: string
  label: string
  detail: string
  state?: DockerContainer['state']
  note?: string
  /** KPanel's own container: runs last because stopping it ends the batch. */
  self?: boolean
  operation: DockerBatchOperation
}

export interface DockerBatchSkip {
  key: string
  label: string
  reason: string
}

export interface DockerBatchPlan {
  tab: DockerBatchTab
  action: DockerBatchAction
  items: DockerBatchPlanItem[]
  skipped: DockerBatchSkip[]
}

export type DockerBatchItemStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'unknown'

export interface DockerBatchRunItem extends DockerBatchPlanItem {
  status: DockerBatchItemStatus
  message: string
}

export interface DockerBatchRun {
  plan: DockerBatchPlan
  items: DockerBatchRunItem[]
  phase: 'running' | 'stopping' | 'finished'
}

export interface DockerBatchSummary {
  total: number
  done: number
  succeeded: number
  failed: number
  cancelled: number
  unknown: number
}

/** The request may have reached Docker but its result never came back. */
export class DockerBatchUnconfirmedError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'DockerBatchUnconfirmedError'
  }
}

export const containerBatchActions: readonly ContainerBatchAction[] = ['start', 'restart', 'pause', 'stop', 'remove']
const predefinedNetworks: readonly string[] = ['bridge', 'host', 'none']

const actionLabels: Record<DockerBatchAction, string> = {
  start: '启动',
  restart: '重启',
  pause: '暂停',
  stop: '停止',
  remove: '删除',
  image_pull: '更新',
  image_remove: '删除',
  network_remove: '删除',
  volume_remove: '删除',
}

export function dockerBatchActionLabel(action: DockerBatchAction): string {
  return actionLabels[action]
}

// Whole sentences per action, never stitched from fragments, so each one is a
// single entry in the phrase catalogs.
export function dockerBatchTitle(action: DockerBatchAction): string {
  switch (action) {
    case 'start': return '批量启动容器'
    case 'restart': return '批量重启容器'
    case 'pause': return '批量暂停容器'
    case 'stop': return '批量停止容器'
    case 'remove': return '批量删除容器'
    case 'image_pull': return '批量更新镜像'
    case 'image_remove': return '批量删除镜像'
    case 'network_remove': return '批量删除网络'
    case 'volume_remove': return '批量删除存储卷'
  }
}

export function dockerBatchDescription(plan: DockerBatchPlan): string {
  const count = plan.items.length
  if (!count) return '所选项都不适用此操作，没有可执行的项目。'
  switch (plan.action) {
    case 'start': return `将逐个启动 ${count} 个容器。`
    case 'restart': return `将逐个重启 ${count} 个容器。`
    case 'pause': return `将逐个暂停 ${count} 个容器。`
    case 'stop': return `将逐个停止 ${count} 个容器。`
    case 'remove': return `将逐个删除 ${count} 个容器。`
    case 'image_pull': return `将逐个拉取 ${count} 个镜像的最新版本。`
    case 'image_remove': return `将逐个删除 ${count} 个镜像。`
    case 'network_remove': return `将逐个删除 ${count} 个网络。`
    case 'volume_remove': return `将逐个删除 ${count} 个存储卷。`
  }
}

export function dockerBatchIsDanger(action: DockerBatchAction): boolean {
  return action === 'stop' || action === 'remove' || action.endsWith('_remove')
}

export function dockerContainerPermits(container: DockerContainer, action: string): boolean {
  return Boolean(
    container.resourceVersion &&
    container.allowedActions?.some(
      (allowed) => allowed === action || allowed.endsWith(`.${action}`) || allowed.endsWith(`/${action}`),
    ),
  )
}

/** The container this panel runs in, when KPanel itself is deployed with Docker. */
export function isKPanelContainer(container: Pick<DockerContainer, 'name' | 'image'>): boolean {
  return container.name === 'kejilion-panel' ||
    /(?:^|\/)kjlion\/kejilion-panel(?:[:@]|$)/i.test(container.image.trim())
}

const runningStates = new Set<DockerContainer['state']>(['running', 'paused', 'restarting'])

function isActiveContainer(container: DockerContainer): boolean {
  return runningStates.has(container.state)
}

function containerLifecycleAction(container: DockerContainer, action: ContainerBatchAction): ContainerLifecycleAction | undefined {
  if (action === 'start') {
    if (dockerContainerPermits(container, 'start')) return 'start'
    if (dockerContainerPermits(container, 'unpause')) return 'unpause'
    return undefined
  }
  return dockerContainerPermits(container, action) ? action : undefined
}

function containerSkipReason(container: DockerContainer, action: ContainerBatchAction): string {
  if (!container.resourceVersion) return '缺少资源版本，请刷新后重试'
  const stopped = container.state === 'exited' || container.state === 'created' || container.state === 'dead'
  if (action === 'start' && (container.state === 'running' || container.state === 'restarting')) return '已在运行'
  if (action === 'stop' && stopped) return '已停止'
  if (action === 'pause' && container.state === 'paused') return '已处于暂停状态'
  if ((action === 'pause' || action === 'restart') && stopped) return '未在运行'
  return '当前状态不支持此操作'
}

export function planContainerBatch(containers: readonly DockerContainer[], action: ContainerBatchAction): DockerBatchPlan {
  const items: DockerBatchPlanItem[] = []
  const skipped: DockerBatchSkip[] = []
  for (const container of containers) {
    const lifecycle = containerLifecycleAction(container, action)
    if (!lifecycle || !container.resourceVersion) {
      skipped.push({ key: container.id, label: container.name, reason: containerSkipReason(container, action) })
      continue
    }
    const self = isKPanelContainer(container)
    items.push({
      key: container.id,
      label: container.name,
      detail: container.project ? `${container.project} · ${container.image}` : container.image,
      state: container.state,
      note: lifecycle === 'unpause' ? '已暂停，将继续运行' : self ? 'KPanel 自身容器' : undefined,
      self: self || undefined,
      operation: { kind: 'container', id: container.id, action: lifecycle, resourceVersion: container.resourceVersion },
    })
  }
  // A stable partition: everything else keeps the on-screen order.
  return { tab: 'containers', action, items: [...items.filter((item) => !item.self), ...items.filter((item) => item.self)], skipped }
}

/** Canonical form of an image reference, as Docker shortens it in RepoTags. */
export function normalizeImageReference(reference: string): string {
  let value = reference.trim().toLowerCase()
  if (!value) return ''
  if (/^sha256:[0-9a-f]{64}$/.test(value)) return value
  let digest = ''
  const at = value.indexOf('@')
  if (at >= 0) {
    digest = value.slice(at)
    value = value.slice(0, at)
  }
  const parts = value.split('/')
  if (parts.length > 1 && ['docker.io', 'index.docker.io', 'registry-1.docker.io'].includes(parts[0]!)) parts.shift()
  if (parts.length === 2 && parts[0] === 'library') parts.shift()
  value = parts.join('/')
  if (digest) return `${value}${digest}`
  return value.includes(':', value.lastIndexOf('/') + 1) ? value : `${value}:latest`
}

function imageReferences(image: DockerImage): Set<string> {
  return new Set([
    image.id.toLowerCase(),
    ...image.tags.filter((tag) => tag && tag !== '<none>:<none>').map(normalizeImageReference),
    ...(image.digests || []).filter((digest) => digest && !digest.startsWith('<none>')).map(normalizeImageReference),
  ])
}

function imageLabel(image: DockerImage): string {
  return image.tags.find((tag) => tag && tag !== '<none>:<none>') || `未标记镜像 ${shortId(image.id.replace(/^sha256:/, ''))}`
}

// Language-neutral so the surrounding sentence stays one translatable pattern.
function joinNames(names: readonly string[]): string {
  return names.length > 3 ? `${names.slice(0, 3).join(', ')} +${names.length - 3}` : names.join(', ')
}

export function planImageBatch(
  images: readonly DockerImage[],
  containers: readonly DockerContainer[],
  action: 'image_pull' | 'image_remove',
): DockerBatchPlan {
  const items: DockerBatchPlanItem[] = []
  const skipped: DockerBatchSkip[] = []
  const containerReferences = containers.map((container) => ({ container, reference: normalizeImageReference(container.image) }))
  for (const image of images) {
    const label = imageLabel(image)
    const otherTags = image.tags.filter((tag) => tag && tag !== '<none>:<none>' && tag !== label)
    const detail = [shortId(image.id.replace(/^sha256:/, '')), formatBytes(image.sizeBytes), otherTags.join(', ')]
      .filter(Boolean).join(' · ')
    if (action === 'image_pull') {
      const tag = image.tags.find((item) => item && item !== '<none>:<none>')
      if (!tag) {
        skipped.push({ key: image.id, label, reason: '没有标签，无法拉取更新' })
        continue
      }
      items.push({ key: image.id, label, detail, operation: { kind: 'task', input: { action: 'image_pull', image: tag } } })
      continue
    }
    if (!image.resourceVersion) {
      skipped.push({ key: image.id, label, reason: '缺少资源版本，请刷新后重试' })
      continue
    }
    // Container image references are names, not IDs, so this is advisory only:
    // Docker itself refuses to delete an image a running container uses.
    const references = imageReferences(image)
    const users = containerReferences.filter(({ reference }) => references.has(reference)).map(({ container }) => container)
    const active = users.filter(isActiveContainer).map((container) => container.name)
    const note = active.length
      ? `运行中的容器 ${joinNames(active)} 正在使用`
      : users.length ? `仍被已停止的容器 ${joinNames(users.map((container) => container.name))} 引用` : undefined
    items.push({
      key: image.id,
      label,
      detail,
      note,
      operation: { kind: 'task', input: { action: 'image_remove', target: image.id, expectedResourceVersion: image.resourceVersion } },
    })
  }
  return { tab: 'images', action, items, skipped }
}

export function planNetworkBatch(networks: readonly DockerNetwork[], containers: readonly DockerContainer[]): DockerBatchPlan {
  const items: DockerBatchPlanItem[] = []
  const skipped: DockerBatchSkip[] = []
  for (const network of networks) {
    if (predefinedNetworks.includes(network.name)) {
      skipped.push({ key: network.id, label: network.name, reason: 'Docker 预置网络，不能删除' })
      continue
    }
    if (!network.resourceVersion) {
      skipped.push({ key: network.id, label: network.name, reason: '缺少资源版本，请刷新后重试' })
      continue
    }
    const users = containers.filter((container) => container.networks.includes(network.name))
    const active = users.filter(isActiveContainer).map((container) => container.name)
    if (active.length) {
      skipped.push({ key: network.id, label: network.name, reason: `运行中的容器 ${joinNames(active)} 仍连接此网络，Docker 不允许删除` })
      continue
    }
    items.push({
      key: network.id,
      label: network.name,
      detail: [network.driver, shortId(network.id)].filter(Boolean).join(' · '),
      note: users.length ? `已停止的容器 ${joinNames(users.map((container) => container.name))} 仍配置此网络` : undefined,
      operation: { kind: 'task', input: { action: 'network_remove', target: network.id, expectedResourceVersion: network.resourceVersion } },
    })
  }
  return { tab: 'networks', action: 'network_remove', items, skipped }
}

export function planVolumeBatch(volumes: readonly DockerVolume[], containers: readonly DockerContainer[]): DockerBatchPlan {
  const items: DockerBatchPlanItem[] = []
  const skipped: DockerBatchSkip[] = []
  for (const volume of volumes) {
    if (!volume.resourceVersion) {
      skipped.push({ key: volume.name, label: volume.name, reason: '缺少资源版本，请刷新后重试' })
      continue
    }
    // Docker refuses to remove a volume any container references, stopped or not.
    const users = containers
      .filter((container) => container.mounts.some((mount) => mount.type === 'volume' && mount.name === volume.name))
      .map((container) => container.name)
    if (users.length) {
      skipped.push({ key: volume.name, label: volume.name, reason: `仍被容器 ${joinNames(users)} 挂载，Docker 不允许删除` })
      continue
    }
    items.push({
      key: volume.name,
      label: volume.name,
      detail: [volume.driver, volume.mountpoint].filter(Boolean).join(' · '),
      operation: { kind: 'task', input: { action: 'volume_remove', target: volume.name, expectedResourceVersion: volume.resourceVersion } },
    })
  }
  return { tab: 'volumes', action: 'volume_remove', items, skipped }
}

export function dockerBatchNotes(plan: DockerBatchPlan): string[] {
  const notes: string[] = []
  switch (plan.action) {
    case 'start':
      if (plan.items.some((item) => item.operation.kind === 'container' && item.operation.action === 'unpause')) {
        notes.push('已暂停的容器会继续运行，其余容器正常启动。')
      }
      break
    case 'restart':
      notes.push('每个容器最多等待 10 秒正常退出，然后重新启动。')
      break
    case 'stop':
      notes.push('每个容器最多等待 10 秒正常退出，超时后强制停止。')
      break
    case 'pause':
      notes.push('暂停会冻结容器内的全部进程，恢复前服务不可用。')
      break
    case 'remove':
      notes.push('运行中的容器会被强制停止后删除；镜像、存储卷和 Compose 配置保留。')
      break
    case 'image_pull':
      notes.push('拉取所选标签的最新镜像；正在运行的容器不会自动重建。')
      break
    case 'image_remove':
      notes.push('运行中容器正在使用的镜像，Docker 会拒绝删除并在结果中标记为失败。')
      break
    case 'network_remove':
      if (plan.items.some((item) => item.note)) notes.push('网络删除后，仍配置它的已停止容器需要先重新连接网络才能启动。')
      break
    case 'volume_remove':
      notes.push('存储卷中的数据会被永久删除，无法恢复。')
      break
  }
  notes.push('按列表顺序逐个执行，单项失败不会中断其余项。')
  return notes
}

export function dockerBatchSelfWarning(plan: DockerBatchPlan): string {
  const self = plan.items.find((item) => item.self)
  if (!self || self.operation.kind !== 'container') return ''
  const name = self.label
  switch (self.operation.action) {
    case 'stop': return `所选包含 KPanel 自身容器 ${name}，会放在最后执行；停止后面板会离线，需要通过 SSH 或 kejilion.sh 重新启动。`
    case 'restart': return `所选包含 KPanel 自身容器 ${name}，会放在最后执行；重启期间面板会短暂断开，稍后刷新页面即可。`
    case 'pause': return `所选包含 KPanel 自身容器 ${name}，会放在最后执行；暂停后面板会失去响应，需要通过 SSH 执行 docker unpause 恢复。`
    case 'remove': return `所选包含 KPanel 自身容器 ${name}，会放在最后执行；删除后面板会离线，需要重新运行安装脚本部署。`
    default: return ''
  }
}

export function createDockerBatchRun(plan: DockerBatchPlan): DockerBatchRun {
  return {
    plan,
    items: plan.items.map((item) => ({ ...item, status: 'pending', message: '' })),
    phase: 'running',
  }
}

export function summarizeDockerBatch(run: Pick<DockerBatchRun, 'items'>): DockerBatchSummary {
  const summary: DockerBatchSummary = { total: run.items.length, done: 0, succeeded: 0, failed: 0, cancelled: 0, unknown: 0 }
  for (const item of run.items) {
    if (item.status === 'succeeded') summary.succeeded += 1
    else if (item.status === 'failed') summary.failed += 1
    else if (item.status === 'cancelled') summary.cancelled += 1
    else if (item.status === 'unknown') summary.unknown += 1
    else continue
    summary.done += 1
  }
  return summary
}

function isAbort(reason: unknown): boolean {
  return reason instanceof DOMException && reason.name === 'AbortError'
}

/**
 * Runs the items one at a time; the Agent serialises lifecycle calls and
 * allows one maintenance job at a time, so parallel dispatch gains nothing.
 * `run` is mutated in place so a reactive wrapper follows each step.
 */
export async function executeDockerBatch(
  run: DockerBatchRun,
  execute: (item: DockerBatchRunItem, signal: AbortSignal) => Promise<void>,
  describeError: (reason: unknown) => string,
  signal: AbortSignal,
): Promise<void> {
  for (const item of run.items) {
    if (item.status !== 'pending') continue
    if (signal.aborted || run.phase === 'stopping') {
      item.status = 'cancelled'
      continue
    }
    item.status = 'running'
    try {
      await execute(item, signal)
      item.status = 'succeeded'
    } catch (reason) {
      if (reason instanceof DockerBatchUnconfirmedError) {
        item.status = 'unknown'
        item.message = reason.message
      } else if (isAbort(reason) || signal.aborted) {
        item.status = 'unknown'
        item.message = '已离开页面，结果未确认'
      } else {
        item.status = 'failed'
        item.message = describeError(reason)
      }
    }
  }
  run.phase = 'finished'
}

function wait(delay: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'))
      return
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, delay)
    function abort(): void {
      clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    }
    signal.addEventListener('abort', abort, { once: true })
  })
}

export interface DockerBatchTaskClient {
  submit: (input: DockerMaintenanceInput) => Promise<DockerMaintenanceJob>
  job: (id: string, signal?: AbortSignal) => Promise<DockerMaintenanceJob>
}

export interface DockerBatchTaskTiming {
  pollDelay: number
  conflictDelay: number
  conflictAttempts: number
  pollFailures: number
}

const defaultTaskTiming: DockerBatchTaskTiming = { pollDelay: 800, conflictDelay: 2_000, conflictAttempts: 15, pollFailures: 5 }

function errorStatus(reason: unknown): number | undefined {
  const status = (reason as { status?: unknown } | null)?.status
  return typeof status === 'number' ? status : undefined
}

function errorCode(reason: unknown): string {
  const code = (reason as { code?: unknown } | null)?.code
  return typeof code === 'string' ? code : ''
}

/**
 * Submits one maintenance task and waits for its terminal state. The Agent
 * runs one Docker job at a time, so a conflict from a job started elsewhere
 * is waited out briefly instead of failing the item at once.
 */
export async function runDockerBatchTask(
  input: DockerMaintenanceInput,
  client: DockerBatchTaskClient,
  signal: AbortSignal,
  timing: DockerBatchTaskTiming = defaultTaskTiming,
): Promise<void> {
  let job: DockerMaintenanceJob | undefined
  for (let attempt = 0; !job; attempt += 1) {
    try {
      job = await client.submit(input)
    } catch (reason) {
      const conflict = errorStatus(reason) === 409 && errorCode(reason) === 'docker_task_conflict'
      if (!conflict || attempt + 1 >= timing.conflictAttempts) throw reason
      await wait(timing.conflictDelay, signal)
    }
  }
  let failures = 0
  while (job.status === 'queued' || job.status === 'running') {
    await wait(timing.pollDelay, signal)
    try {
      job = await client.job(job.id, signal)
      failures = 0
    } catch (reason) {
      if (isAbort(reason)) throw reason
      failures += 1
      if (errorStatus(reason) === 404 || failures >= timing.pollFailures) {
        throw new DockerBatchUnconfirmedError('后台任务已提交，但无法读取执行结果，请刷新后核对')
      }
    }
  }
  if (job.status === 'failed') throw new Error(job.message || 'Docker 后台任务失败')
}
