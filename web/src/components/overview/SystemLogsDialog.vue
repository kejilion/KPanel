<script setup lang="ts">
import {
  computed,
  inject,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'
import {
  CircleAlert,
  Clock3,
  HardDrive,
  Pause,
  Play,
  RefreshCw,
  Search,
  Trash2,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { formatBytes, formatDateTime } from '@/lib/format'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type {
  SystemLogEntries,
  SystemLogEntry,
  SystemLogLimit,
  SystemLogPriority,
  SystemLogQuery,
  SystemLogSource,
  SystemLogsSummary,
} from '@/types/api'

const props = withDefaults(
  defineProps<{
    open: boolean
    readable: boolean
    writable: boolean
    unavailableReason?: string
  }>(),
  { unavailableReason: '' },
)
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const windowActive = inject(desktopWindowActiveKey, computed(() => true))

const sourceOptions: ReadonlyArray<{ id: SystemLogSource; label: string; detail: string }> = [
  { id: 'system', label: '系统', detail: '全部 journal' },
  { id: 'service', label: '服务', detail: '全部服务日志' },
  { id: 'security', label: '安全', detail: '认证事件' },
  { id: 'login', label: '登录', detail: '最近登录记录' },
]
const sourceLogLabels: Readonly<Record<SystemLogSource, string>> = {
  system: '系统日志',
  service: '服务日志',
  security: '安全日志',
  login: '登录日志',
}
const priorityOptions: ReadonlyArray<{ id: SystemLogPriority; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'warning', label: '警告及以上' },
  { id: 'error', label: '错误及以上' },
]
const limitOptions: SystemLogLimit[] = [50, 100, 200]
const cleanupPolicies = [
  { id: 'retain-7d', label: '保留最近 7 天' },
  { id: 'retain-3d', label: '保留最近 3 天' },
  { id: 'max-500m', label: '归档 journal 最大 500 MiB' },
] as const
type CleanupPolicy = (typeof cleanupPolicies)[number]['id']
interface CleanupTaskIdentity {
  taskId: string
  policy: CleanupPolicy
}
interface HighlightedLogPart {
  text: string
  highlighted: boolean
}
interface DisplayLogEntry {
  key: string
  timestamp: HighlightedLogPart[]
  priority: HighlightedLogPart[]
  priorityTone: 'danger' | 'warning' | 'neutral'
  identity: HighlightedLogPart[]
  message: HighlightedLogPart[]
  hasPrefix: boolean
}

// These fixed messages arrive through JSON, so keep them visible to the phrase-catalog gate.
const systemLogBackendPhrases = [
  '系统日志仅支持 Linux',
  '系统日志清理仅支持 Linux',
  'Agent 必须以受限 root 服务运行',
  'journalctl、last 与系统日志文件均不可用',
  'systemd 后台任务执行器不可用',
  'journalctl 不可用',
  'Agent 后台执行程序不可用，请更新或重新安装 KPanel',
  'last 命令不可用',
  'journal 与固定认证日志文件均不可用',
  'du 命令不可用',
  '无法统计 /var/log 用量',
  'du 未返回日志用量',
  'du 返回的日志用量无效',
  '无法读取 journal 用量',
  'journalctl 未返回可识别的用量',
  '系统日志查询参数无效',
  '另一项系统日志查询正在执行',
  '系统日志查询超时',
  '系统日志暂时不可用',
  '系统维护任务已提交，页面将自动刷新进度',
  '正在轮转 systemd journal',
  '正在保留最近 7 天 journal',
  '正在保留最近 3 天 journal',
  '正在限制 journal 最大 500 MiB',
  'journal 已轮转并仅保留最近 7 天归档',
  'journal 已轮转并仅保留最近 3 天归档',
  'journal 已轮转并限制归档最大 500 MiB',
] as const
void systemLogBackendPhrases

const summary = ref<SystemLogsSummary>()
const entries = ref<SystemLogEntries>()
const selectedSource = ref<SystemLogSource>('system')
const selectedPriority = ref<SystemLogPriority>('all')
const selectedLimit = ref<SystemLogLimit>(100)
const keyword = ref('')
const realtime = ref(false)
const pageVisible = ref(typeof document === 'undefined' || document.visibilityState !== 'hidden')
const loadingSummary = ref(false)
const refreshingSummary = ref(false)
const loadingEntries = ref(false)
const refreshingEntries = ref(false)
const entriesPending = ref(false)
const summaryError = ref('')
const entriesError = ref('')
const unseenEntries = ref(false)
const cleanupPolicy = ref<CleanupPolicy>('retain-7d')
const cleanupSubmitting = ref(false)
const cleanupRunning = ref(false)
const cleanupNotice = ref('')
const cleanupError = ref('')
const cleanupTaskIdentity = ref<CleanupTaskIdentity>()
const logView = ref<HTMLElement>()
const followLatest = ref(true)

let summaryController: AbortController | undefined
let entriesController: AbortController | undefined
let realtimeEntriesController: AbortController | undefined
let maintenanceController: AbortController | undefined
let realtimeTimer: number | undefined
let maintenanceTimer: number | undefined
let cleanupPolling = false
let systemLogReadQueue: Promise<void> = Promise.resolve()

function queueSystemLogRead<T>(read: () => Promise<T>): Promise<T> {
  const result = systemLogReadQueue.then(read, read)
  systemLogReadQueue = result.then(() => undefined, () => undefined)
  return result
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const supportsPriority = computed(() => selectedSource.value === 'system' || selectedSource.value === 'service')
const effectivePriority = computed<SystemLogPriority>(() => supportsPriority.value ? selectedPriority.value : 'all')
const entriesQueryKey = computed(() => JSON.stringify({
  source: selectedSource.value,
  limit: selectedLimit.value,
  priority: effectivePriority.value,
}))

function currentEntriesQuery(): SystemLogQuery {
  if (selectedSource.value === 'system') {
    return { source: 'system', limit: selectedLimit.value, priority: selectedPriority.value }
  }
  if (selectedSource.value === 'service') {
    return {
      source: 'service',
      limit: selectedLimit.value,
      priority: selectedPriority.value,
    }
  }
  return { source: selectedSource.value, limit: selectedLimit.value }
}
const maintenanceBusy = computed(() => {
  const maintenance = summary.value?.maintenance
  return maintenance?.state === 'running' && maintenance.action !== 'log-cleanup'
})
const maintenanceProgress = computed(() => summary.value?.maintenance.progress || 0)

function sourceAvailability(source: SystemLogSource): { available: boolean; reason: string } {
  if (!summary.value) return { available: false, reason: '正在读取日志能力。' }
  if (source === 'login') {
    return {
      available: summary.value.sources.login.available,
      reason: summary.value.sources.login.reason || '当前主机没有可用的登录记录。',
    }
  }
  if (source === 'security') {
    return {
      available: summary.value.sources.security.available,
      reason: summary.value.sources.security.reason || '当前主机没有可用的安全日志。',
    }
  }
  return {
    available: summary.value.sources.journal.available,
    reason: summary.value.sources.journal.reason || '当前主机没有可用的 systemd journal。',
  }
}

const selectedAvailability = computed(() => sourceAvailability(selectedSource.value))
const canReadEntries = computed(() =>
  props.open && props.readable && selectedAvailability.value.available,
)
const canRealtime = computed(() =>
  realtime.value && canReadEntries.value && pageVisible.value && windowActive.value,
)

const filteredEntries = computed(() => {
  const query = keyword.value.trim().toLocaleLowerCase()
  const ordered = entries.value?.entries || []
  if (!query) return ordered
  return ordered.filter((entry) => [
    entry.timestamp,
    entry.priority,
    entry.unit,
    entry.identifier,
    entry.pid,
    entry.message,
  ].some((value) => String(value ?? '').toLocaleLowerCase().includes(query)))
})
const displayLogEntries = computed<DisplayLogEntry[]>(() => filteredEntries.value.map((entry, index) => {
  const identity = logIdentity(entry)
  return {
    key: entry.cursor || `${entry.timestamp}-${index}`,
    timestamp: highlightLogText(entry.timestamp),
    priority: highlightLogText(entry.priority?.toUpperCase()),
    priorityTone: priorityTone(entry.priority),
    identity: highlightLogText(identity),
    message: highlightLogText(entry.message),
    hasPrefix: Boolean(entry.timestamp || entry.priority || identity),
  }
}))

const footerStatus = computed(() => {
  if (!entries.value) return phrase('尚未读取日志')
  const visible = filteredEntries.value.length
  const total = entries.value.entries.length
  const sourceLabel = phrase(sourceLogLabels[selectedSource.value])
  const filtered = phrase(keyword.value.trim() ? ` · 匹配 ${visible}/${total} 条` : ` · ${total} 条`)
  const truncated = entries.value.truncated ? phrase(' · 已截断') : ''
  return `${sourceLabel}${filtered}${truncated}`
})

function logIdentity(entry: SystemLogEntry): string {
  const identity = entry.unit || entry.identifier || ''
  return identity && entry.pid ? `${identity}[${entry.pid}]` : identity
}

function priorityTone(priority?: string): 'danger' | 'warning' | 'neutral' {
  if (priority && ['emergency', 'alert', 'critical', 'error'].includes(priority)) return 'danger'
  if (priority === 'warning') return 'warning'
  return 'neutral'
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function highlightLogText(value: string | number | undefined): HighlightedLogPart[] {
  const text = String(value ?? '')
  if (!text) return []
  const query = keyword.value.trim()
  if (!query) return [{ text, highlighted: false }]

  const matcher = new RegExp(escapeRegExp(query), 'gi')
  const parts: HighlightedLogPart[] = []
  let offset = 0
  for (const match of text.matchAll(matcher)) {
    const index = match.index ?? 0
    if (index > offset) parts.push({ text: text.slice(offset, index), highlighted: false })
    parts.push({ text: match[0], highlighted: true })
    offset = index + match[0].length
  }
  if (!parts.length) return [{ text, highlighted: false }]
  if (offset < text.length) parts.push({ text: text.slice(offset), highlighted: false })
  return parts
}

function entrySignature(value?: SystemLogEntries): string {
  const latest = value?.entries.at(-1)
  return [value?.entries.length, latest?.cursor, latest?.timestamp, latest?.message].join('|')
}

function isAbortError(reason: unknown): boolean {
  return reason instanceof DOMException && reason.name === 'AbortError'
}

function reasonMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

function setSource(source: SystemLogSource): void {
  if (source === selectedSource.value) return
  if (source !== 'system' && source !== 'service') selectedPriority.value = 'all'
  selectedSource.value = source
  keyword.value = ''
  unseenEntries.value = false
}

function atLogBottom(): boolean {
  const element = logView.value
  if (!element) return true
  return element.scrollHeight - element.scrollTop - element.clientHeight <= 32
}

function scrollToLatest(): void {
  const element = logView.value
  if (element) element.scrollTop = element.scrollHeight
  followLatest.value = true
  unseenEntries.value = false
}

function onLogScroll(): void {
  followLatest.value = atLogBottom()
  if (followLatest.value) unseenEntries.value = false
}

async function loadSummary(silent = false): Promise<boolean> {
  if (!props.open || !props.readable) return false
  summaryController?.abort()
  const controller = new AbortController()
  summaryController = controller
  if (silent && summary.value) refreshingSummary.value = true
  else loadingSummary.value = true
  summaryError.value = ''
  try {
    const result = await queueSystemLogRead(async () => {
      if (
        controller.signal.aborted ||
        summaryController !== controller ||
        !props.open ||
        !props.readable
      ) return undefined
      return api.system.logsSummary(controller.signal)
    })
    if (!result) return false
    if (summaryController !== controller || !props.open) return false
    summary.value = result
    syncCleanupMaintenance(result.maintenance)
    return true
  } catch (reason) {
    if (isAbortError(reason)) return false
    summaryError.value = reasonMessage(reason, '无法读取系统日志概览。')
    return false
  } finally {
    if (summaryController === controller) {
      loadingSummary.value = false
      refreshingSummary.value = false
    }
  }
}

async function loadEntries(
  silent = false,
  options: { skipIfPending?: boolean; forceBottom?: boolean; realtime?: boolean } = {},
): Promise<void> {
  if (!canReadEntries.value) {
    entriesController?.abort()
    entries.value = undefined
    entriesError.value = ''
    unseenEntries.value = false
    return
  }
  if (entriesPending.value && options.skipIfPending) return
  entriesController?.abort()
  const controller = new AbortController()
  entriesController = controller
  if (options.realtime) realtimeEntriesController = controller
  const queryKey = entriesQueryKey.value
  const query = currentEntriesQuery()
  const previousSignature = entrySignature(entries.value)
  entriesPending.value = true
  if (silent && entries.value) refreshingEntries.value = true
  else loadingEntries.value = true
  entriesError.value = ''
  try {
    const result = await queueSystemLogRead(async () => {
      if (
        controller.signal.aborted ||
        entriesController !== controller ||
        queryKey !== entriesQueryKey.value ||
        !canReadEntries.value
      ) return undefined
      return api.system.logs(query, controller.signal)
    })
    if (!result) return
    if (entriesController !== controller || queryKey !== entriesQueryKey.value || !props.open) return
    entries.value = result
    await nextTick()
    if (options.forceBottom || followLatest.value) scrollToLatest()
    else if (previousSignature && previousSignature !== entrySignature(result)) unseenEntries.value = true
  } catch (reason) {
    if (isAbortError(reason)) return
    entriesError.value = reasonMessage(reason, '无法读取系统日志。')
  } finally {
    if (entriesController === controller) {
      entriesPending.value = false
      loadingEntries.value = false
      refreshingEntries.value = false
    }
    if (realtimeEntriesController === controller) realtimeEntriesController = undefined
  }
}

async function loadAll(silent = false): Promise<void> {
  const loaded = await loadSummary(silent)
  if ((loaded || summary.value) && props.open) {
    await loadEntries(silent, { forceBottom: !silent })
  }
}

function clearRealtimeTimer(): void {
  if (realtimeTimer !== undefined && typeof window !== 'undefined') window.clearTimeout(realtimeTimer)
  realtimeTimer = undefined
}

function scheduleRealtime(): void {
  clearRealtimeTimer()
  if (!canRealtime.value || typeof window === 'undefined') {
    realtimeEntriesController?.abort()
    realtimeEntriesController = undefined
    return
  }
  realtimeTimer = window.setTimeout(async () => {
    realtimeTimer = undefined
    if (!canRealtime.value) return
    await loadEntries(true, { skipIfPending: true, realtime: true })
    scheduleRealtime()
  }, 3_000)
}

function toggleRealtime(): void {
  realtime.value = !realtime.value
  scheduleRealtime()
}

function clearMaintenanceTimer(): void {
  if (maintenanceTimer !== undefined && typeof window !== 'undefined') window.clearTimeout(maintenanceTimer)
  maintenanceTimer = undefined
}

function stopMaintenancePolling(): void {
  cleanupPolling = false
  clearMaintenanceTimer()
  maintenanceController?.abort()
  maintenanceController = undefined
}

function matchesCleanupTask(
  maintenance: SystemLogsSummary['maintenance'],
  identity = cleanupTaskIdentity.value,
): identity is CleanupTaskIdentity {
  return Boolean(
    identity &&
    maintenance.id === identity.taskId &&
    maintenance.action === 'log-cleanup' &&
    maintenance.policy === identity.policy,
  )
}

function markCleanupIdentityChanged(maintenance: SystemLogsSummary['maintenance']): void {
  stopMaintenancePolling()
  cleanupTaskIdentity.value = undefined
  cleanupRunning.value = maintenance.action === 'log-cleanup' && maintenance.state === 'running'
  cleanupNotice.value = ''
  cleanupError.value = '日志清理任务身份已变化，请刷新确认真实状态。'
}

function syncCleanupMaintenance(maintenance: SystemLogsSummary['maintenance']): void {
  const identity = cleanupTaskIdentity.value
  const isCleanup = maintenance.action === 'log-cleanup'

  if (identity) {
    if (!matchesCleanupTask(maintenance, identity)) {
      markCleanupIdentityChanged(maintenance)
      return
    }
    cleanupRunning.value = maintenance.state === 'running'
    if (maintenance.state === 'running') {
      cleanupNotice.value = maintenance.message || '正在清理旧 journal，关闭窗口不会中断任务。'
      cleanupError.value = ''
      startMaintenancePolling()
    } else if (maintenance.state === 'failed') {
      cleanupNotice.value = ''
      cleanupError.value = maintenance.message || '日志清理任务执行失败。'
      stopMaintenancePolling()
    } else if (maintenance.state === 'succeeded') {
      cleanupNotice.value = maintenance.message || '日志清理已完成，已重新读取真实占用。'
      cleanupError.value = ''
      stopMaintenancePolling()
    } else {
      cleanupNotice.value = ''
      cleanupError.value = '无法确认日志清理结果：任务没有返回完成状态。'
      stopMaintenancePolling()
    }
    return
  }

  stopMaintenancePolling()
  cleanupRunning.value = isCleanup && maintenance.state === 'running'
  if (!isCleanup) {
    cleanupNotice.value = ''
    cleanupError.value = ''
  } else if (maintenance.state === 'running') {
    cleanupNotice.value = maintenance.message || '检测到后台日志清理任务正在运行。'
    cleanupError.value = ''
  } else if (maintenance.state === 'failed') {
    cleanupNotice.value = ''
    cleanupError.value = maintenance.message || '最近一次日志清理任务执行失败。'
  } else if (maintenance.state === 'succeeded') {
    cleanupNotice.value = '最近一次日志清理任务已完成。'
    cleanupError.value = ''
  } else {
    cleanupNotice.value = ''
    cleanupError.value = ''
  }
}

function scheduleMaintenancePoll(delay = 2_000): void {
  clearMaintenanceTimer()
  if (!cleanupPolling || !props.open || typeof window === 'undefined') return
  maintenanceTimer = window.setTimeout(() => void pollMaintenance(), delay)
}

function startMaintenancePolling(): void {
  if (!cleanupTaskIdentity.value) return
  cleanupPolling = true
  scheduleMaintenancePoll(800)
}

async function pollMaintenance(): Promise<void> {
  if (!cleanupPolling || !props.open) return
  const identity = cleanupTaskIdentity.value
  if (!identity) {
    stopMaintenancePolling()
    cleanupRunning.value = false
    cleanupNotice.value = ''
    cleanupError.value = '日志清理任务身份已变化，请刷新确认真实状态。'
    return
  }
  maintenanceController?.abort()
  const controller = new AbortController()
  maintenanceController = controller
  try {
    const maintenance = await api.system.maintenance(controller.signal)
    if (maintenanceController !== controller || !props.open) return
    cleanupError.value = ''
    if (summary.value) summary.value = { ...summary.value, maintenance }
    if (!matchesCleanupTask(maintenance, identity)) {
      markCleanupIdentityChanged(maintenance)
      return
    }
    cleanupRunning.value = maintenance.state === 'running'
    if (cleanupRunning.value) {
      cleanupNotice.value = maintenance.message || '正在清理旧 journal，关闭窗口不会中断任务。'
      scheduleMaintenancePoll()
      return
    }
    stopMaintenancePolling()
    if (maintenance.state === 'failed') {
      cleanupNotice.value = ''
      const message = maintenance.message || '日志清理任务执行失败。'
      cleanupError.value = message
      toast.danger(phrase('日志清理失败'), phrase(message))
      await loadSummary(true)
      return
    }
    if (maintenance.state !== 'succeeded') {
      cleanupNotice.value = ''
      cleanupError.value = '无法确认日志清理结果：任务没有返回完成状态。'
      return
    }
    const message = maintenance.message || '日志清理已完成，已重新读取真实占用。'
    toast.success(phrase('日志清理已完成'), phrase(message))
    await loadAll(true)
    if (cleanupTaskIdentity.value?.taskId === identity.taskId) {
      cleanupNotice.value = message
      cleanupError.value = ''
    }
  } catch (reason) {
    if (isAbortError(reason)) return
    cleanupError.value = reasonMessage(reason, '暂时无法读取日志清理进度，正在继续重试。')
    scheduleMaintenancePoll()
  }
}

async function submitCleanup(): Promise<void> {
  if (!props.writable || cleanupSubmitting.value || cleanupRunning.value || maintenanceBusy.value) return
  const policy = cleanupPolicies.find((item) => item.id === cleanupPolicy.value)!
  const confirmed = typeof window === 'undefined' || window.confirm(phrase(
    `确认清理旧 journal 日志并${phrase(policy.label)}吗？此操作不会删除 /var/log 中的其他日志文件。`,
  ))
  if (!confirmed) return
  realtime.value = false
  scheduleRealtime()
  stopMaintenancePolling()
  cleanupTaskIdentity.value = undefined
  cleanupSubmitting.value = true
  cleanupError.value = ''
  cleanupNotice.value = ''
  try {
    const submittedPolicy = cleanupPolicy.value
    const result = await api.system.action({
      action: 'log-cleanup',
      maintenancePolicy: submittedPolicy,
    })
    const taskId = result.taskId?.trim()
    const responseMismatch = result.action !== 'log-cleanup' || result.status !== 'accepted'
    const policyMismatch = result.maintenancePolicy !== undefined && result.maintenancePolicy !== submittedPolicy
    if (!taskId || responseMismatch || policyMismatch) {
      cleanupRunning.value = true
      cleanupNotice.value = ''
      cleanupError.value = !taskId
        ? 'Agent 未返回日志清理任务身份；任务可能已提交，请刷新确认真实状态。'
        : policyMismatch
          ? 'Agent 返回的日志清理策略与提交内容不一致；任务可能已提交，请刷新确认真实状态。'
          : '日志清理任务身份已变化，请刷新确认真实状态。'
      toast.danger(
        phrase(taskId ? '日志清理任务身份异常' : '日志清理任务身份缺失'),
        phrase(cleanupError.value),
      )
      return
    }
    cleanupTaskIdentity.value = { taskId, policy: submittedPolicy }
    cleanupRunning.value = true
    cleanupNotice.value = result.message || '日志清理任务已提交，关闭窗口不会中断。'
    toast.success(phrase('日志清理任务已提交'), phrase(cleanupNotice.value))
    startMaintenancePolling()
  } catch (reason) {
    cleanupError.value = reasonMessage(reason, 'Agent 未能提交日志清理任务。')
    toast.danger(phrase('提交日志清理失败'), phrase(cleanupError.value))
  } finally {
    cleanupSubmitting.value = false
  }
}

function closeDialog(): void {
  realtime.value = false
  scheduleRealtime()
  emit('close')
}

function stopOpenWork(): void {
  realtime.value = false
  clearRealtimeTimer()
  stopMaintenancePolling()
  cleanupNotice.value = ''
  cleanupError.value = ''
  cleanupRunning.value = false
  entries.value = undefined
  entriesError.value = ''
  unseenEntries.value = false
  realtimeEntriesController?.abort()
  realtimeEntriesController = undefined
  summaryController?.abort()
  entriesController?.abort()
}

function onVisibilityChange(): void {
  pageVisible.value = document.visibilityState !== 'hidden'
}

watch(
  () => [props.open, props.readable] as const,
  ([open, readable]) => {
    if (open && readable) void loadAll()
    else stopOpenWork()
  },
  { immediate: true },
)

watch(entriesQueryKey, () => {
  unseenEntries.value = false
  entriesError.value = ''
  entries.value = undefined
  if (props.open && props.readable && summary.value) {
    void loadEntries(false, { forceBottom: true })
  }
  scheduleRealtime()
})

watch([canRealtime, windowActive], scheduleRealtime)

onMounted(() => document.addEventListener('visibilitychange', onVisibilityChange))
onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  stopOpenWork()
})
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('系统日志管理')"
    :description="phrase('按需读取真实系统日志；不复制日志到 Panel，也不建立常驻采集进程。')"
    size="wide"
    @close="closeDialog"
  >
    <div class="system-logs-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning" role="status">
        <CircleAlert :size="18" />
        <span>{{ phrase(unavailableReason || '当前 Agent 的系统日志适配器未就绪。') }}</span>
      </div>

      <LoadingState v-else-if="loadingSummary && !summary" :rows="2" cards />
      <ErrorState v-else-if="summaryError && !summary" :message="phrase(summaryError)" @retry="loadAll()" />

      <template v-else-if="summary">
        <div v-if="summaryError" class="inline-alert inline-alert--warning" role="status">
          <CircleAlert :size="18" />
          <span><span>{{ phrase(summaryError) }}</span> <span>{{ phrase('正在保留上一次日志概览。') }}</span></span>
        </div>

        <section class="system-logs-overview" :aria-label="phrase('系统日志占用概览')">
          <article>
            <span class="system-logs-overview__icon"><HardDrive :size="20" /></span>
            <div>
              <span>{{ phrase('/var/log 总占用') }}</span>
              <strong>{{ summary.varLog.available ? formatBytes(summary.varLog.bytes) : phrase('不可用') }}</strong>
              <small>{{ phrase(summary.varLog.available ? '通常已经包含 journal 文件占用' : summary.varLog.reason || '不可用') }}</small>
            </div>
          </article>
          <article>
            <span class="system-logs-overview__icon is-journal"><Clock3 :size="20" /></span>
            <div>
              <span>{{ phrase('journal 占用') }}</span>
              <strong>{{ summary.journal.available ? formatBytes(summary.journal.bytes) : phrase('不可用') }}</strong>
              <small>{{ phrase(summary.journal.available ? '通常是 /var/log 的子集，两项不能相加' : summary.journal.reason || '不可用') }}</small>
            </div>
          </article>
        </section>

        <div class="system-logs-observed">
          <span>{{ phrase(`采样时间 ${formatDateTime(summary.observedAt)}`) }}</span>
          <button
            class="icon-button"
            type="button"
            :disabled="loadingSummary || loadingEntries || refreshingSummary || refreshingEntries || cleanupSubmitting"
            :title="phrase('刷新日志概览和当前日志')"
            :aria-label="phrase('刷新日志概览和当前日志')"
            @click="loadAll(true)"
          >
            <RefreshCw :size="17" :class="{ spin: refreshingSummary || refreshingEntries }" />
          </button>
        </div>

        <div
          v-if="summary.maintenance.state === 'running'"
          class="inline-alert"
          :class="summary.maintenance.action === 'log-cleanup' ? 'inline-alert--info' : 'inline-alert--warning'"
          role="status"
        >
          <RefreshCw :size="17" class="spin" />
          <span>
            {{ phrase(summary.maintenance.message || '系统维护任务正在后台执行。') }}
            <strong>{{ maintenanceProgress }}%</strong>
          </span>
        </div>

        <div class="system-log-source-switch" role="group" :aria-label="phrase('日志范围')">
          <button
            v-for="option in sourceOptions"
            :key="option.id"
            type="button"
            :class="{ 'is-active': selectedSource === option.id, 'is-unavailable': !sourceAvailability(option.id).available }"
            :aria-pressed="selectedSource === option.id"
            @click="setSource(option.id)"
          >
            <strong>{{ phrase(option.label) }}</strong>
            <small>{{ phrase(option.detail) }}</small>
          </button>
        </div>

        <div v-if="!selectedAvailability.available" class="inline-alert inline-alert--warning" role="status">
          <CircleAlert :size="18" />
          <span>{{ phrase(selectedAvailability.reason) }}</span>
        </div>

        <template v-else>
          <div class="system-log-query-bar">
            <label class="system-log-control">
              <span>{{ phrase('读取行数') }}</span>
              <select v-model.number="selectedLimit">
                <option v-for="limit in limitOptions" :key="limit" :value="limit">{{ phrase(`最近 ${limit} 行`) }}</option>
              </select>
            </label>

            <label class="system-log-control system-log-control--search">
              <span>{{ phrase(selectedSource === 'service' ? '搜索服务日志' : '搜索日志') }}</span>
              <span class="system-log-search-input">
                <Search :size="17" />
                <input
                  v-model="keyword"
                  type="search"
                  autocomplete="off"
                  :placeholder="phrase(selectedSource === 'service' ? '输入服务名或日志关键字' : '关键词、服务、PID 或消息')"
                />
              </span>
            </label>

            <button
              class="button button--secondary system-log-realtime"
              type="button"
              :aria-pressed="realtime"
              :disabled="loadingEntries"
              @click="toggleRealtime"
            >
              <Pause v-if="realtime" :size="17" />
              <Play v-else :size="17" />
              {{ phrase(realtime ? '暂停实时刷新' : '开启实时刷新') }}
            </button>
          </div>

          <div v-if="supportsPriority" class="system-log-priority" role="group" :aria-label="phrase('日志级别')">
            <span>{{ phrase('日志级别') }}</span>
            <button
              v-for="option in priorityOptions"
              :key="option.id"
              type="button"
              :class="{ 'is-active': selectedPriority === option.id }"
              :aria-pressed="selectedPriority === option.id"
              @click="selectedPriority = option.id"
            >
              {{ phrase(option.label) }}
            </button>
          </div>

          <div v-if="entriesError && entries" class="inline-alert inline-alert--warning" role="status">
            <CircleAlert :size="18" />
            <span><span>{{ phrase(entriesError) }}</span> <span>{{ phrase('正在保留上一次日志结果。') }}</span></span>
          </div>

          <LoadingState v-if="loadingEntries && !entries" :rows="3" />
          <ErrorState v-else-if="entriesError && !entries" :message="phrase(entriesError)" @retry="loadEntries()" />
          <EmptyState
            v-else-if="entries && !filteredEntries.length"
            :title="phrase(keyword.trim() ? '没有匹配的日志' : '当前范围暂无日志')"
            :description="phrase(keyword.trim() ? '调整搜索关键字或增加读取行数后重试。' : '当前主机在所选范围内没有返回日志记录。')"
          />
          <section v-else-if="entries" class="system-log-output-shell" :aria-label="phrase('日志输出')">
            <header>
              <span>{{ phrase(sourceLogLabels[selectedSource]) }}</span>
              <small>
                {{ formatDateTime(entries.observedAt) }}
                <template v-if="entries.authSource"> · {{ entries.authSource }}</template>
                <template v-if="entries.truncated"> · {{ phrase('输出已截断') }}</template>
              </small>
              <span v-if="refreshingEntries" class="system-log-refreshing" role="status">
                <RefreshCw :size="14" class="spin" /> {{ phrase('正在刷新') }}
              </span>
              <span v-else-if="realtime && !canRealtime" class="system-log-refreshing is-paused" role="status">
                {{ phrase('实时刷新已暂停') }}
              </span>
            </header>
            <pre
              ref="logView"
              class="log-view system-log-output"
              data-i18n-ignore
              tabindex="0"
              @scroll="onLogScroll"
            ><span
              v-for="entry in displayLogEntries"
              :key="entry.key"
              class="system-log-line"
            ><span class="system-log-time"><span v-for="(part, partIndex) in entry.timestamp" :key="partIndex" :class="{ 'system-log-highlight': part.highlighted }">{{ part.text }}</span></span><template v-if="entry.priority.length"> <span class="system-log-level" :class="`is-${entry.priorityTone}`"><span v-for="(part, partIndex) in entry.priority" :key="partIndex" :class="{ 'system-log-highlight': part.highlighted }">{{ part.text }}</span></span></template><template v-if="entry.identity.length"> <span class="system-log-identity"><span v-for="(part, partIndex) in entry.identity" :key="partIndex" :class="{ 'system-log-highlight': part.highlighted }">{{ part.text }}</span></span></template><template v-if="entry.hasPrefix">: </template><span class="system-log-message"><span v-for="(part, partIndex) in entry.message" :key="partIndex" :class="{ 'system-log-highlight': part.highlighted }">{{ part.text }}</span></span></span></pre>
            <button v-if="unseenEntries" class="button button--secondary system-log-latest" type="button" @click="scrollToLatest">
              {{ phrase('查看最新日志') }}
            </button>
          </section>

          <details class="system-log-cleanup">
            <summary>
              <span><Trash2 :size="18" /> {{ phrase('清理旧 journal') }}</span>
              <small>{{ phrase('固定安全策略，不删除其他日志文件') }}</small>
            </summary>
            <div class="system-log-cleanup__body">
              <div v-if="!writable" class="inline-alert inline-alert--warning" role="status">
                {{ phrase('当前 Agent 仅支持查看日志，清理适配器未就绪。') }}
              </div>
              <div v-else-if="maintenanceBusy" class="inline-alert inline-alert--warning" role="status">
                {{ phrase('已有系统维护任务正在后台执行，请等待完成后再清理日志。') }}
              </div>
              <div v-if="cleanupNotice" class="inline-alert inline-alert--info" role="status">{{ phrase(cleanupNotice) }}</div>
              <div v-if="cleanupError" class="inline-alert inline-alert--warning" role="alert">{{ phrase(cleanupError) }}</div>
              <label class="system-log-control">
                <span>{{ phrase('清理策略') }}</span>
                <select v-model="cleanupPolicy" :disabled="!writable || cleanupSubmitting || cleanupRunning || maintenanceBusy">
                  <option v-for="policy in cleanupPolicies" :key="policy.id" :value="policy.id">{{ phrase(policy.label) }}</option>
                </select>
              </label>
              <p>{{ phrase('执行时先轮转 journal，再应用所选保留策略；任务在后台运行，关闭窗口不会中断。') }}</p>
              <button
                class="button button--danger-text"
                type="button"
                :disabled="!writable || cleanupSubmitting || cleanupRunning || maintenanceBusy"
                @click="submitCleanup"
              >
                <RefreshCw v-if="cleanupSubmitting || cleanupRunning" :size="17" class="spin" />
                <Trash2 v-else :size="17" />
                {{ phrase(cleanupSubmitting ? '正在提交…' : cleanupRunning ? '清理任务进行中' : '确认清理旧 journal') }}
              </button>
            </div>
          </details>
        </template>
      </template>
    </div>

    <template #footer>
      <span class="system-log-footer-status" role="status">{{ footerStatus }}</span>
      <button class="button button--secondary" type="button" @click="closeDialog">{{ phrase('关闭') }}</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.system-logs-dialog {
  display: grid;
  gap: 16px;
}

.system-logs-overview {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.system-logs-overview article {
  display: grid;
  min-width: 0;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 12px;
  padding: 14px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.system-logs-overview__icon {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: var(--radius-sm);
}

.system-logs-overview__icon.is-journal {
  color: var(--blue);
  background: var(--blue-soft);
}

.system-logs-overview article > div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.system-logs-overview span,
.system-logs-overview small {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.5;
}

.system-logs-overview strong {
  font-size: 18px;
  line-height: 1.35;
}

.system-logs-observed {
  display: flex;
  min-height: 40px;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 13px;
}

.system-logs-observed .icon-button {
  margin-left: auto;
}

.system-log-source-switch {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.system-log-source-switch button {
  display: grid;
  min-height: 52px;
  gap: 3px;
  padding: 8px 10px;
  color: var(--text-soft);
  text-align: left;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.system-log-source-switch button:hover,
.system-log-source-switch button:focus-visible {
  border-color: var(--brand);
}

.system-log-source-switch button:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--brand) 20%, transparent);
  outline-offset: 2px;
}

.system-log-source-switch button.is-active {
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-color: var(--brand);
}

.system-log-source-switch button.is-unavailable:not(.is-active) {
  color: var(--muted);
  background: var(--surface-subtle);
}

.system-log-source-switch strong {
  font-size: 14px;
}

.system-log-source-switch small {
  color: inherit;
  font-size: 13px;
  line-height: 1.35;
}

.system-log-query-bar {
  display: flex;
  align-items: end;
  gap: 10px;
  flex-wrap: wrap;
}

.system-log-control {
  display: grid;
  min-width: 130px;
  gap: 6px;
  color: var(--text-soft);
  font-size: 14px;
  font-weight: 650;
}

.system-log-control--search {
  min-width: 220px;
  flex: 1 1 260px;
}

.system-log-control select,
.system-log-control input {
  width: 100%;
  min-height: 42px;
  padding: 0 11px;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font: inherit;
  font-weight: 450;
}

.system-log-control select:focus,
.system-log-control input:focus {
  border-color: var(--brand);
  outline: 2px solid color-mix(in srgb, var(--brand) 18%, transparent);
  outline-offset: 1px;
}

.system-log-search-input {
  position: relative;
  display: flex;
  align-items: center;
}

.system-log-search-input svg {
  position: absolute;
  left: 11px;
  z-index: 1;
  color: var(--muted);
  pointer-events: none;
}

.system-log-search-input input {
  padding-left: 36px;
}

.system-log-realtime {
  min-width: 148px;
}

.system-log-priority {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-wrap: wrap;
  color: var(--text-soft);
  font-size: 14px;
  font-weight: 650;
}

.system-log-priority button {
  min-height: 38px;
  padding: 0 11px;
  color: var(--text-soft);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: 14px;
}

.system-log-priority button:hover,
.system-log-priority button:focus-visible,
.system-log-priority button.is-active {
  color: var(--brand-strong);
  border-color: var(--brand);
}

.system-log-priority button:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--brand) 20%, transparent);
  outline-offset: 2px;
}

.system-log-priority button.is-active {
  background: var(--brand-soft);
}

.system-log-output-shell {
  position: relative;
  display: grid;
  gap: 8px;
}

.system-log-output-shell > header {
  display: flex;
  align-items: center;
  gap: 9px;
  color: var(--text-soft);
  font-size: 14px;
}

.system-log-output-shell > header small {
  color: var(--muted);
  font-size: 13px;
}

.system-log-refreshing {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-left: auto;
  color: var(--brand-strong);
  font-size: 13px;
}

.system-log-refreshing.is-paused {
  color: var(--muted);
}

.system-log-output {
  min-height: clamp(280px, 45dvh, 520px);
  max-height: min(56dvh, 620px);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.system-log-line {
  display: block;
}

.system-log-time {
  color: var(--terminal-shell-muted, #8a9695);
}

.system-log-level {
  font-weight: 750;
}

.system-log-level.is-danger {
  color: color-mix(in srgb, var(--danger) 58%, #fff);
}

.system-log-level.is-warning {
  color: color-mix(in srgb, var(--warning) 64%, #fff);
}

.system-log-level.is-neutral {
  color: var(--terminal-shell-muted, #8a9695);
}

.system-log-identity {
  color: color-mix(
    in srgb,
    var(--terminal-shell-text, #d8dddc) 72%,
    var(--terminal-shell-muted, #8a9695)
  );
}

.system-log-message {
  color: var(--terminal-shell-text, #d8dddc);
}

.system-log-highlight {
  padding: 0 2px;
  border-radius: 3px;
  color: var(--terminal-shell-text, #d8dddc);
  background: color-mix(in srgb, var(--brand) 38%, transparent);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--brand) 42%, transparent);
}

.system-log-latest {
  position: absolute;
  right: 14px;
  bottom: 14px;
}

.system-log-cleanup {
  overflow: hidden;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.system-log-cleanup > summary {
  display: flex;
  min-height: 48px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 13px;
  cursor: pointer;
  list-style: none;
}

.system-log-cleanup > summary::-webkit-details-marker {
  display: none;
}

.system-log-cleanup > summary:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--brand) 20%, transparent);
  outline-offset: -3px;
}

.system-log-cleanup > summary > span {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text);
  font-size: 14px;
  font-weight: 700;
}

.system-log-cleanup > summary small {
  color: var(--muted);
  font-size: 13px;
}

.system-log-cleanup__body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 12px;
  padding: 13px;
  border-top: 1px solid var(--border);
}

.system-log-cleanup__body .inline-alert {
  grid-column: 1 / -1;
}

.system-log-cleanup__body p {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.55;
}

.system-log-footer-status {
  min-width: 0;
  margin-right: auto;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

@media (max-width: 768px) {
  .system-log-source-switch {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .system-log-query-bar {
    align-items: stretch;
  }

  .system-log-control,
  .system-log-control--service,
  .system-log-control--search,
  .system-log-realtime {
    min-width: 0;
    flex: 1 1 calc(50% - 5px);
  }
}

@media (max-width: 520px) {
  .system-logs-overview {
    grid-template-columns: 1fr;
  }

  .system-logs-observed {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .system-logs-observed .icon-button {
    margin-left: auto;
  }

  .system-log-control,
  .system-log-control--service,
  .system-log-control--search,
  .system-log-realtime {
    flex-basis: 100%;
  }

  .system-log-output-shell > header {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .system-log-refreshing {
    width: 100%;
    margin-left: 0;
  }

  .system-log-output {
    min-height: 260px;
    max-height: 44dvh;
  }

  .system-log-cleanup > summary {
    align-items: flex-start;
    flex-direction: column;
  }

  .system-log-cleanup__body {
    grid-template-columns: 1fr;
  }

  .system-log-cleanup__body .button {
    width: 100%;
  }
}
</style>
