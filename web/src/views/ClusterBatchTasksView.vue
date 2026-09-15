<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  CheckCircle2,
  Clock3,
  Eraser,
  FileClock,
  History,
  ListChecks,
  LoaderCircle,
  PackageCheck,
  Power,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  XCircle,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { formatDateTime, relativeTime, shortId } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type {
  ClusterBatchAction,
  ClusterBatchActionDefinition,
  ClusterBatchTask,
  ClusterBatchTaskCatalog,
  ClusterBatchTaskState,
  ClusterBatchTargetState,
  ClusterHost,
  ClusterHostList,
} from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ClusterBatchTasksView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ClusterBatchTasksView/zh-TW').then((module) => module.default))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const actionPresentation: Record<ClusterBatchAction, { label: string; description: string; icon: Component }> = {
  refresh: { label: '刷新主机状态', description: '重新采集所选面板的监控摘要，不修改系统。', icon: RefreshCw },
  'system-update': { label: '系统更新', description: '执行现有完整系统更新维护任务。', icon: PackageCheck },
  'cleanup-cache': { label: '清理软件缓存', description: '仅清理软件包缓存，影响范围较小。', icon: Eraser },
  'cleanup-standard': { label: '标准系统清理', description: '执行现有标准清理策略并跟踪结果。', icon: ListChecks },
  'logs-retain-7d': { label: '日志保留 7 天', description: '按固定策略清理超过 7 天的系统日志。', icon: FileClock },
  'logs-retain-3d': { label: '日志保留 3 天', description: '按固定策略清理超过 3 天的系统日志。', icon: FileClock },
  'logs-max-500m': { label: '日志上限 500 MiB', description: '按固定策略将系统日志压缩到 500 MiB。', icon: FileClock },
  reboot: { label: '延迟重启', description: '向所选面板提交现有延迟重启动作。', icon: Power },
}

const route = useRoute()
const router = useRouter()
const toast = useToast()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
const inventory = ref<ClusterHostList>()
const catalog = ref<ClusterBatchTaskCatalog>()
const tasks = ref<ClusterBatchTask[]>([])
const loadingComposer = ref(true)
const loadingTasks = ref(true)
const refreshingTasks = ref(false)
const composerError = ref('')
const tasksError = ref('')
const selectedAction = ref<ClusterBatchAction>('refresh')
const selectedHostIDs = ref<Set<string>>(new Set())
const hostSearch = ref('')
const concurrency = ref(2)
const timeoutSeconds = ref(3600)
const confirmOpen = ref(false)
const rebootAcknowledged = ref(false)
const creating = ref(false)
const selectedTaskID = ref(validTaskID(route.query.task) ? String(route.query.task) : '')
const taskDetail = ref<ClusterBatchTask>()
const detailLoading = ref(false)
const detailError = ref('')
const mutatingTaskID = ref('')
let composerController: AbortController | undefined
let tasksController: AbortController | undefined
let detailController: AbortController | undefined
let pollTimer: number | undefined

const fallbackCatalog: ClusterBatchTaskCatalog = {
  actions: [],
  limits: {
    maxTasks: 100,
    maxActiveTasks: 4,
    maxTargets: 50,
    maxConcurrency: 4,
    minTimeoutSeconds: 60,
    maxTimeoutSeconds: 3600,
    defaultTimeoutSeconds: 3600,
  },
}

const limits = computed(() => catalog.value?.limits || fallbackCatalog.limits)
const actionDefinitions = computed(() => catalog.value?.actions || [])
const actionDefinition = computed(() => actionDefinitions.value.find((item) => item.id === selectedAction.value))
const selectedPresentation = computed(() => actionPresentation[selectedAction.value])
const panelHosts = computed(() => inventory.value?.items || [])
const filteredHosts = computed(() => {
  const query = hostSearch.value.trim().toLowerCase()
  if (!query) return panelHosts.value
  return panelHosts.value.filter((host) => [host.name, host.id, host.origin, host.panelVersion]
    .some((value) => value?.toLowerCase().includes(query)))
})
const eligibleHosts = computed(() => panelHosts.value.filter((host) => hostEligibility(host).eligible))
const selectedHosts = computed(() => panelHosts.value.filter((host) => selectedHostIDs.value.has(host.id)))
const offlineSelected = computed(() => selectedHosts.value.filter((host) => ['offline', 'auth_failed', 'tls_error', 'incompatible'].includes(host.state)).length)
const selectionAtLimit = computed(() => selectedHostIDs.value.size >= limits.value.maxTargets)
const allVisibleEligibleSelected = computed(() => {
  const visible = filteredHosts.value.filter((host) => hostEligibility(host).eligible)
  return visible.length > 0 && visible.every((host) => selectedHostIDs.value.has(host.id))
})
const canSubmit = computed(() => !creating.value && selectedHostIDs.value.size > 0 && Boolean(actionDefinition.value))
const activeTasks = computed(() => tasks.value.filter((task) => taskActive(task.state)))
const selectedTask = computed(() => taskDetail.value?.id === selectedTaskID.value
  ? taskDetail.value
  : tasks.value.find((task) => task.id === selectedTaskID.value))
const selectedTaskNeedsPolling = computed(() => Boolean(
  selectedTaskID.value
  && !detailError.value
  && (!selectedTask.value || taskActive(selectedTask.value.state)),
))
const timeoutOptions = computed(() => [
  limits.value.minTimeoutSeconds,
  300,
  900,
  1800,
  limits.value.defaultTimeoutSeconds,
  limits.value.maxTimeoutSeconds,
]
  .filter((value) => value >= limits.value.minTimeoutSeconds && value <= limits.value.maxTimeoutSeconds)
  .filter((value, index, items) => items.indexOf(value) === index)
  .sort((left, right) => left - right))

function validTaskID(value: unknown): boolean {
  return typeof value === 'string' && /^[a-f0-9]{32}$/.test(value)
}

function taskActive(state: ClusterBatchTaskState): boolean {
  return state === 'queued' || state === 'running' || state === 'cancelling'
}

function actionInfo(action: ClusterBatchAction) {
  return actionPresentation[action]
}

function actionRiskLabel(definition: ClusterBatchActionDefinition): string {
  return phrase({ read: '只读', write: '系统写入', disruptive: '中断服务' }[definition.risk])
}

function taskStateLabel(state: ClusterBatchTaskState): string {
  return phrase({
    queued: '等待执行', running: '执行中', cancelling: '正在取消', succeeded: '全部完成',
    partial: '部分完成', failed: '执行失败', cancelled: '已取消', needs_attention: '需要人工核对',
  }[state])
}

function targetStateLabel(state: ClusterBatchTargetState): string {
  return phrase({
    queued: '等待执行', submitting: '正在提交', running: '执行中', succeeded: '已完成',
    failed: '执行失败', cancelled: '已取消', unsupported: '不支持', needs_attention: '需要人工核对',
  }[state])
}

function hostStateLabel(host: ClusterHost): string {
  return phrase({
    online: '在线', degraded: '异常', stale: '数据过期', offline: '离线', pairing: '配对中',
    revoking: '撤销中', auth_failed: '授权失效', tls_error: '证书异常', incompatible: '版本不兼容', unknown: '未知',
  }[host.state])
}

function hostEligibility(host: ClusterHost): { eligible: boolean; reason: string } {
  if (host.kind === 'light_node') return { eligible: false, reason: phrase('轻量节点不提供系统维护动作') }
  if (host.state === 'pairing' || host.state === 'revoking') return { eligible: false, reason: phrase('连接尚未就绪') }
  if (selectedAction.value === 'refresh') return { eligible: true, reason: '' }
  if (host.batchTaskAvailable) return { eligible: true, reason: '' }
  if (host.isLocal) return { eligible: false, reason: phrase('当前面板版本未装载批量维护适配器') }
  if (host.federationProtocol !== 'v2') return { eligible: false, reason: phrase('旧版连接不具备批量维护权限') }
  return { eligible: false, reason: phrase('当前连接未授权批量维护，请在目标 KPanel 重新配对') }
}

function taskProgress(task: ClusterBatchTask): number {
  if (!task.total) return 0
  return Math.round((task.completed / task.total) * 100)
}

function setAction(action: ClusterBatchAction): void {
  selectedAction.value = action
  const next = new Set<string>()
  for (const id of selectedHostIDs.value) {
    const host = panelHosts.value.find((item) => item.id === id)
    if (host && hostEligibility(host).eligible) next.add(id)
  }
  selectedHostIDs.value = next
  rebootAcknowledged.value = false
}

function toggleHost(host: ClusterHost): void {
  if (!hostEligibility(host).eligible) return
  const next = new Set(selectedHostIDs.value)
  if (next.has(host.id)) next.delete(host.id)
  else if (next.size < limits.value.maxTargets) next.add(host.id)
  selectedHostIDs.value = next
}

function toggleVisibleHosts(): void {
  const visible = filteredHosts.value.filter((host) => hostEligibility(host).eligible)
  const next = new Set(selectedHostIDs.value)
  if (allVisibleEligibleSelected.value) {
    visible.forEach((host) => next.delete(host.id))
  } else {
    for (const host of visible) {
      if (next.size >= limits.value.maxTargets) break
      next.add(host.id)
    }
  }
  selectedHostIDs.value = next
}

function friendlyError(reason: unknown, fallback: string): string {
  if (!(reason instanceof ApiError)) return phrase(fallback)
  const messages: Record<string, string> = {
    cluster_batch_task_busy: '已有任务占用所选主机，或活动任务已达到上限。',
    cluster_batch_task_invalid: '任务参数无效，请检查动作、主机、并发和超时。',
    cluster_batch_task_unsupported: '所选主机均不支持这个动作。',
    cluster_batch_task_state: '任务当前状态不允许这个操作。',
    cluster_not_found: '任务或主机已不存在，请刷新后重试。',
  }
  return phrase(messages[reason.code] || reason.message || fallback)
}

async function loadComposer(): Promise<void> {
  composerController?.abort()
  const current = new AbortController()
  composerController = current
  loadingComposer.value = true
  composerError.value = ''
  try {
    const [hosts, actions] = await Promise.all([
      api.cluster.hosts(current.signal),
      api.cluster.batchActions(current.signal),
    ])
    if (composerController !== current || current.signal.aborted) return
    inventory.value = hosts
    catalog.value = actions
    if (!actions.actions.some((item) => item.id === selectedAction.value)) {
      selectedAction.value = actions.actions[0]?.id || 'refresh'
    }
    concurrency.value = Math.min(Math.max(concurrency.value, 1), actions.limits.maxConcurrency)
    if (timeoutSeconds.value < actions.limits.minTimeoutSeconds || timeoutSeconds.value > actions.limits.maxTimeoutSeconds) {
      timeoutSeconds.value = actions.limits.defaultTimeoutSeconds
    }
  } catch (reason) {
    if (composerController !== current || current.signal.aborted) return
    composerError.value = friendlyError(reason, '无法读取批量任务能力与主机列表。')
  } finally {
    if (composerController === current) loadingComposer.value = false
  }
}

function upsertTask(task: ClusterBatchTask): void {
  const summary = { ...task, targets: undefined }
  const next = tasks.value.filter((item) => item.id !== task.id)
  next.unshift(summary)
  tasks.value = next.sort((left, right) => Date.parse(right.createdAt) - Date.parse(left.createdAt))
}

async function loadTasks(options: { silent?: boolean } = {}): Promise<void> {
  tasksController?.abort()
  const current = new AbortController()
  tasksController = current
  if (options.silent) refreshingTasks.value = true
  else loadingTasks.value = true
  tasksError.value = ''
  try {
    const result = await api.cluster.batchTasks(current.signal)
    if (tasksController !== current || current.signal.aborted) return
    tasks.value = result.items
  } catch (reason) {
    if (tasksController !== current || current.signal.aborted) return
    tasksError.value = friendlyError(reason, '无法读取批量任务记录。')
  } finally {
    if (tasksController === current) {
      loadingTasks.value = false
      refreshingTasks.value = false
      schedulePoll()
    }
  }
}

async function loadTaskDetail(options: { silent?: boolean } = {}): Promise<void> {
  detailController?.abort()
  if (!selectedTaskID.value) return
  const identity = selectedTaskID.value
  const current = new AbortController()
  detailController = current
  if (!options.silent) detailLoading.value = true
  detailError.value = ''
  try {
    const result = await api.cluster.batchTask(identity, current.signal)
    if (detailController !== current || current.signal.aborted || selectedTaskID.value !== identity) return
    taskDetail.value = result
    upsertTask(result)
  } catch (reason) {
    if (detailController !== current || current.signal.aborted || selectedTaskID.value !== identity) return
    taskDetail.value = undefined
    detailError.value = friendlyError(reason, '无法读取任务详情。')
  } finally {
    if (detailController === current) {
      detailLoading.value = false
      schedulePoll()
    }
  }
}

function pageVisible(): boolean {
  return desktopWindowActive.value && (typeof document === 'undefined' || document.visibilityState !== 'hidden')
}

function schedulePoll(): void {
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = undefined
  if (!pageVisible() || (!activeTasks.value.length && !selectedTaskNeedsPolling.value)) return
  pollTimer = window.setTimeout(async () => {
    await Promise.all([
      loadTasks({ silent: true }),
      selectedTaskNeedsPolling.value ? loadTaskDetail({ silent: true }) : Promise.resolve(),
    ])
  }, 3_000)
}

function openConfirmation(): void {
  if (!canSubmit.value) return
  rebootAcknowledged.value = false
  confirmOpen.value = true
}

function closeConfirmation(): void {
  if (creating.value) return
  confirmOpen.value = false
}

async function createTask(): Promise<void> {
  if (!canSubmit.value || (selectedAction.value === 'reboot' && !rebootAcknowledged.value)) return
  creating.value = true
  try {
    const task = await api.cluster.createBatchTask({
      action: selectedAction.value,
      hostIds: [...selectedHostIDs.value],
      concurrency: concurrency.value,
      timeoutSeconds: timeoutSeconds.value,
      confirmDisruptive: selectedAction.value === 'reboot',
    })
    upsertTask(task)
    confirmOpen.value = false
    selectedHostIDs.value = new Set()
    toast.success('批量任务已创建', `${task.total} 台主机已进入任务队列。`)
    await selectTask(task.id)
  } catch (reason) {
    toast.danger('批量任务创建失败', friendlyError(reason, '请检查所选主机后重试。'))
  } finally {
    creating.value = false
  }
}

async function selectTask(id: string): Promise<void> {
  selectedTaskID.value = id
  taskDetail.value = undefined
  detailError.value = ''
  await router.replace({ query: { ...route.query, task: id || undefined } })
  if (id) await loadTaskDetail()
}

async function cancelTask(task: ClusterBatchTask): Promise<void> {
  if (!window.confirm(phrase('只会停止尚未提交的主机；已提交的维护动作会继续运行。确定取消？'))) return
  mutatingTaskID.value = task.id
  try {
    const updated = await api.cluster.cancelBatchTask(task.id)
    taskDetail.value = updated
    upsertTask(updated)
    toast.success('已请求取消', '请在任务详情中核对已提交主机的最终状态。')
  } catch (reason) {
    toast.danger('取消失败', friendlyError(reason, '请刷新任务状态后重试。'))
  } finally {
    mutatingTaskID.value = ''
  }
}

async function retryTask(task: ClusterBatchTask): Promise<void> {
  const warning = task.action === 'reboot'
    ? task.needsAttention > 0
      ? '部分主机的上次重启结果不确定。请先核对；继续会再次安排重启并中断服务。继续？'
      : '将为未成功主机再次安排重启并中断服务。继续？'
    : task.needsAttention > 0
    ? '部分主机结果不确定。请先在目标主机核对，确认不会重复执行后再重试。继续？'
    : '将为未成功的主机创建一条新任务，已成功主机不会重复执行。继续？'
  if (!window.confirm(phrase(warning))) return
  mutatingTaskID.value = task.id
  try {
    const retried = await api.cluster.retryBatchTask(task.id, {
      confirmDisruptive: task.action === 'reboot',
    })
    upsertTask(retried)
    toast.success('重试任务已创建')
    await selectTask(retried.id)
  } catch (reason) {
    toast.danger('重试失败', friendlyError(reason, '请刷新任务状态后重试。'))
  } finally {
    mutatingTaskID.value = ''
  }
}

async function deleteTask(task: ClusterBatchTask): Promise<void> {
  if (!window.confirm(phrase('只删除中心端的任务记录，不会撤销或回滚目标主机上的动作。确定删除？'))) return
  mutatingTaskID.value = task.id
  try {
    await api.cluster.deleteBatchTask(task.id)
    tasks.value = tasks.value.filter((item) => item.id !== task.id)
    await selectTask('')
    toast.success('任务记录已删除')
  } catch (reason) {
    toast.danger('删除失败', friendlyError(reason, '只有已结束的任务可以删除。'))
  } finally {
    mutatingTaskID.value = ''
  }
}

function onVisibilityChange(): void {
  if (pageVisible()) {
    void Promise.all([loadTasks({ silent: true }), selectedTaskID.value ? loadTaskDetail({ silent: true }) : Promise.resolve()])
  } else {
    tasksController?.abort()
    detailController?.abort()
    if (pollTimer) window.clearTimeout(pollTimer)
    pollTimer = undefined
  }
}

watch(() => route.query.task, (value) => {
  const next = validTaskID(value) ? String(value) : ''
  if (next === selectedTaskID.value) return
  selectedTaskID.value = next
  taskDetail.value = undefined
  if (next && pageVisible()) void loadTaskDetail()
})

watch(desktopWindowActive, onVisibilityChange)
watch(activeTasks, schedulePoll)
watch(selectedTaskNeedsPolling, schedulePoll)

onMounted(() => {
  void loadComposer()
  void loadTasks()
  if (selectedTaskID.value) void loadTaskDetail()
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  composerController?.abort()
  tasksController?.abort()
  detailController?.abort()
  if (pollTimer) window.clearTimeout(pollTimer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="page batch-page">
    <PageHeader title="批量任务" description="在多个已授权 KPanel 上编排固定维护动作，并逐台跟踪真实结果。" />

    <section class="batch-hero" aria-labelledby="batch-hero-title">
      <div class="batch-hero__copy">
        <RouterLink class="batch-back" to="/cluster"><ArrowLeft :size="16" /> 返回集群</RouterLink>
        <p class="batch-eyebrow"><ShieldCheck :size="16" /> 固定动作编排</p>
        <h2 id="batch-hero-title">批量任务中心</h2>
        <p>选择动作和主机后由中心排队执行。每台主机都有独立状态，页面关闭或服务重启后记录仍会保留。</p>
      </div>
      <dl class="batch-hero__stats">
        <div><dt>活动任务</dt><dd>{{ activeTasks.length }} / {{ limits.maxActiveTasks }}</dd></div>
        <div><dt>可选主机</dt><dd>{{ eligibleHosts.length }}</dd></div>
        <div><dt>单任务上限</dt><dd>{{ limits.maxTargets }}</dd></div>
      </dl>
    </section>

    <div class="batch-boundary" role="note">
      <ShieldCheck :size="19" />
      <span><strong>能力边界：</strong>这里只能调用 KPanel 已定义的状态刷新、系统更新、清理、日志清理和延迟重启；不接受命令、脚本、路径或自定义参数。</span>
    </div>

    <div class="batch-layout">
      <section class="batch-panel batch-composer" aria-labelledby="batch-compose-title">
        <header class="batch-panel__header">
          <div><span>新任务</span><h3 id="batch-compose-title">选择执行范围</h3></div>
          <button class="icon-button" type="button" :disabled="loadingComposer" title="刷新能力与主机" aria-label="刷新能力与主机" @click="loadComposer">
            <RefreshCw :size="17" :class="{ spin: loadingComposer }" />
          </button>
        </header>

        <LoadingState v-if="loadingComposer" title="正在读取可用动作与主机…" :rows="4" />
        <ErrorState v-else-if="composerError" :message="composerError" @retry="loadComposer" />
        <form v-else class="batch-form" @submit.prevent="openConfirmation">
          <fieldset class="batch-fieldset">
            <legend><span>1</span> 固定动作</legend>
            <div class="batch-actions" role="radiogroup" aria-label="选择固定动作">
              <button
                v-for="action in actionDefinitions"
                :key="action.id"
                type="button"
                role="radio"
                class="batch-action"
                :class="{ 'is-selected': selectedAction === action.id, 'is-disruptive': action.risk === 'disruptive' }"
                :aria-checked="selectedAction === action.id"
                @click="setAction(action.id)"
              >
                <span class="batch-action__icon"><component :is="actionInfo(action.id).icon" :size="19" /></span>
                <span><strong>{{ phrase(actionInfo(action.id).label) }}</strong><small>{{ phrase(actionInfo(action.id).description) }}</small></span>
                <em>{{ actionRiskLabel(action) }}</em>
              </button>
            </div>
          </fieldset>

          <fieldset class="batch-fieldset">
            <legend><span>2</span> 目标主机</legend>
            <div class="batch-host-toolbar">
              <label class="batch-search"><Search :size="17" /><input v-model="hostSearch" type="search" placeholder="搜索名称、地址或版本" aria-label="搜索批量任务主机" /></label>
              <button class="button button--secondary button--small" type="button" @click="toggleVisibleHosts">
                {{ allVisibleEligibleSelected ? phrase('取消可选主机') : phrase('选择全部可用主机') }}
              </button>
            </div>
            <p class="batch-selection-summary">已选择 <strong>{{ selectedHostIDs.size }}</strong> / {{ limits.maxTargets }} 台；当前动作共有 {{ eligibleHosts.length }} 台可用。</p>
            <div v-if="filteredHosts.length" class="batch-hosts">
              <label
                v-for="host in filteredHosts"
                :key="host.id"
                class="batch-host"
                :class="{ 'is-selected': selectedHostIDs.has(host.id), 'is-disabled': !hostEligibility(host).eligible }"
              >
                <input
                  type="checkbox"
                  :checked="selectedHostIDs.has(host.id)"
                  :disabled="!hostEligibility(host).eligible || (!selectedHostIDs.has(host.id) && selectionAtLimit)"
                  @change="toggleHost(host)"
                />
                <span class="batch-host__icon"><Server :size="18" /></span>
                <span class="batch-host__body">
                  <strong data-i18n-ignore>{{ host.name }}</strong>
                  <small><span>{{ host.isLocal ? phrase('本机') : host.kind === 'light_node' ? phrase('轻量节点') : 'KPanel' }}</span> · <span data-i18n-ignore>{{ host.panelVersion || host.origin || shortId(host.id) }}</span></small>
                  <em v-if="!hostEligibility(host).eligible">{{ hostEligibility(host).reason }}</em>
                </span>
                <StatusBadge :status="host.state" :label="hostStateLabel(host)" subtle />
              </label>
            </div>
            <EmptyState v-else title="没有匹配的主机" description="请清除搜索词，或先返回集群添加主机。" />
          </fieldset>

          <fieldset class="batch-fieldset batch-settings">
            <legend><span>3</span> 调度设置</legend>
            <label class="field">
              <span>并发主机数</span>
              <select v-model.number="concurrency">
                <option v-for="value in limits.maxConcurrency" :key="value" :value="value">{{ value }}</option>
              </select>
              <small>中心全局最多同时处理 {{ limits.maxConcurrency }} 台；同一主机不会进入两个活动任务。</small>
            </label>
            <label class="field">
              <span>单台跟踪时限</span>
              <select v-model.number="timeoutSeconds">
                <option v-for="value in timeoutOptions" :key="value" :value="value">{{ value / 60 }} 分钟</option>
              </select>
              <small>达到时限只会停止跟踪，不会声称已停止目标主机上的维护动作。</small>
            </label>
          </fieldset>

          <div v-if="offlineSelected" class="inline-alert inline-alert--warning" role="status">
            已选择 {{ offlineSelected }} 台当前不可达或授权异常的主机；提交结果可能需要人工核对。
          </div>
          <div class="batch-submit">
            <div><strong>{{ phrase(selectedPresentation.label) }}</strong><small>{{ selectedHostIDs.size ? `${selectedHostIDs.size} 台主机` : phrase('尚未选择主机') }}</small></div>
            <button class="button button--primary" type="submit" :disabled="!canSubmit">
              <ListChecks :size="17" /> 检查并创建任务
            </button>
          </div>
        </form>
      </section>

      <section class="batch-panel batch-history" aria-labelledby="batch-history-title">
        <header class="batch-panel__header">
          <div><span>任务记录</span><h3 id="batch-history-title">最近执行</h3></div>
          <button class="icon-button" type="button" :disabled="refreshingTasks" title="刷新任务记录" aria-label="刷新任务记录" @click="loadTasks({ silent: true })">
            <RefreshCw :size="17" :class="{ spin: refreshingTasks }" />
          </button>
        </header>
        <LoadingState v-if="loadingTasks" title="正在读取批量任务…" :rows="5" />
        <ErrorState v-else-if="tasksError && !tasks.length" :message="tasksError" @retry="loadTasks" />
        <div v-else-if="tasksError" class="inline-alert inline-alert--warning" role="status">{{ tasksError }} 当前保留的是上次成功读取的记录。</div>
        <EmptyState v-else-if="!tasks.length" title="暂无批量任务" description="创建任务后，整体进度与逐台结果会持续保留在这里。" />
        <div v-else class="batch-task-list">
          <button v-for="task in tasks" :key="task.id" class="batch-task" type="button" @click="selectTask(task.id)">
            <span class="batch-task__icon" :class="`is-${task.state}`">
              <LoaderCircle v-if="taskActive(task.state)" :size="19" :class="{ spin: task.state === 'running' }" />
              <CheckCircle2 v-else-if="task.state === 'succeeded'" :size="19" />
              <XCircle v-else-if="task.state === 'failed'" :size="19" />
              <TriangleAlert v-else :size="19" />
            </span>
            <span class="batch-task__body">
              <span><strong>{{ phrase(actionInfo(task.action).label) }}</strong><StatusBadge :status="task.state" :label="taskStateLabel(task.state)" subtle /></span>
              <small>{{ task.total }} 台主机 · {{ relativeTime(task.createdAt) }} · <code>{{ shortId(task.id) }}</code></small>
              <span class="batch-progress"><i><b :style="{ width: `${taskProgress(task)}%` }" /></i><em>{{ task.completed }} / {{ task.total }}</em></span>
              <span v-if="task.failed || task.needsAttention || task.unsupported" class="batch-task__issues">
                失败 {{ task.failed }} · 待核对 {{ task.needsAttention }} · 不支持 {{ task.unsupported }}
              </span>
            </span>
          </button>
        </div>
      </section>
    </div>

    <ModalDialog
      :open="confirmOpen"
      :title="`创建任务：${phrase(selectedPresentation.label)}`"
      :description="`将作用于 ${selectedHostIDs.size} 台主机，并发 ${concurrency}，最长跟踪 ${timeoutSeconds / 60} 分钟。`"
      size="medium"
      :close-disabled="creating"
      @close="closeConfirmation"
    >
      <div class="batch-confirm">
        <span class="batch-confirm__icon" :class="{ 'is-danger': selectedAction === 'reboot' }"><component :is="selectedPresentation.icon" :size="25" /></span>
        <div><strong>{{ phrase(selectedPresentation.label) }}</strong><p>{{ phrase(selectedPresentation.description) }}</p></div>
      </div>
      <div class="inline-alert inline-alert--info">
        每台主机使用独立操作 ID。网络中断时不会自动重复提交结果不确定的写入动作。
      </div>
      <label v-if="selectedAction === 'reboot'" class="batch-reboot-check">
        <input v-model="rebootAcknowledged" type="checkbox" />
        <span><strong>我确认重启会中断所选主机上的服务</strong><small>任务只确认延迟重启已被目标接受，不承诺主机已经恢复在线。</small></span>
      </label>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="creating" @click="closeConfirmation">返回检查</button>
        <button class="button" :class="selectedAction === 'reboot' ? 'button--danger' : 'button--primary'" type="button" :disabled="creating || (selectedAction === 'reboot' && !rebootAcknowledged)" @click="createTask">
          <LoaderCircle v-if="creating" class="spin" :size="17" />
          <ListChecks v-else :size="17" />
          {{ creating ? phrase('正在创建…') : phrase('确认创建任务') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(selectedTaskID)"
      :title="selectedTask ? phrase(actionInfo(selectedTask.action).label) : phrase('批量任务详情')"
      :description="selectedTask ? `任务 ${selectedTask.id}` : ''"
      size="wide"
      @close="selectTask('')"
    >
      <LoadingState v-if="detailLoading && !taskDetail" title="正在读取逐台结果…" :rows="4" />
      <ErrorState v-else-if="detailError && !taskDetail" :message="detailError" @retry="loadTaskDetail" />
      <template v-else-if="taskDetail">
        <div class="batch-detail-summary">
          <div><StatusBadge :status="taskDetail.state" :label="taskStateLabel(taskDetail.state)" /><span>{{ taskProgress(taskDetail) }}%</span></div>
          <dl>
            <div><dt>成功</dt><dd>{{ taskDetail.succeeded }}</dd></div>
            <div><dt>失败</dt><dd>{{ taskDetail.failed }}</dd></div>
            <div><dt>待核对</dt><dd>{{ taskDetail.needsAttention }}</dd></div>
            <div><dt>取消</dt><dd>{{ taskDetail.cancelled }}</dd></div>
            <div><dt>不支持</dt><dd>{{ taskDetail.unsupported }}</dd></div>
          </dl>
          <p><Clock3 :size="15" /> 创建于 {{ formatDateTime(taskDetail.createdAt) }}<template v-if="taskDetail.finishedAt"> · 完成于 {{ formatDateTime(taskDetail.finishedAt) }}</template></p>
        </div>
        <div v-if="taskDetail.needsAttention" class="inline-alert inline-alert--warning" role="alert">
          有 {{ taskDetail.needsAttention }} 台主机的结果无法确认。请先登录目标主机核对真实状态，再决定是否重试。
        </div>
        <section class="batch-target-results" aria-label="逐台执行结果">
          <article v-for="target in taskDetail.targets || []" :key="target.hostId" class="batch-target">
            <header>
              <span class="batch-host__icon"><Server :size="18" /></span>
              <div><strong data-i18n-ignore>{{ target.hostName }}</strong><small><code>{{ shortId(target.hostId) }}</code><template v-if="target.executionId"> · 执行 ID <code>{{ target.executionId }}</code></template></small></div>
              <StatusBadge :status="target.state" :label="targetStateLabel(target.state)" />
            </header>
            <div class="batch-target__progress"><i><b :style="{ width: `${target.progress}%` }" /></i><span>{{ target.progress }}%</span></div>
            <p>{{ phrase(target.message || target.stage || targetStateLabel(target.state)) }}</p>
            <footer><span>阶段：{{ phrase(target.stage || '—') }}</span><span v-if="target.errorCode">错误码：<code>{{ target.errorCode }}</code></span></footer>
          </article>
        </section>
      </template>
      <template #footer>
        <RouterLink class="button button--secondary" :to="{ path: '/activity', query: { tab: 'jobs', job: `cluster-batch:${selectedTaskID}` } }"><History :size="16" /> 任务中心</RouterLink>
        <button v-if="taskDetail && taskActive(taskDetail.state)" class="button button--danger-text" type="button" :disabled="mutatingTaskID === taskDetail.id" @click="cancelTask(taskDetail)">取消未提交主机</button>
        <button v-if="taskDetail && !taskActive(taskDetail.state) && (taskDetail.failed || taskDetail.cancelled || taskDetail.needsAttention || taskDetail.unsupported)" class="button button--secondary" type="button" :disabled="mutatingTaskID === taskDetail.id" @click="retryTask(taskDetail)"><RotateCcw :size="16" /> 重试未成功主机</button>
        <button v-if="taskDetail && !taskActive(taskDetail.state)" class="button button--danger-text" type="button" :disabled="mutatingTaskID === taskDetail.id" @click="deleteTask(taskDetail)"><Trash2 :size="16" /> 删除记录</button>
        <button class="button button--secondary" type="button" @click="selectTask('')">关闭</button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.batch-page { container-type: inline-size; }
.batch-hero { position: relative; display: flex; align-items: end; justify-content: space-between; gap: 24px; padding: 28px; overflow: hidden; color: var(--text); background: linear-gradient(125deg, color-mix(in srgb, var(--brand-soft) 74%, var(--surface)), var(--surface) 62%); border: 1px solid color-mix(in srgb, var(--brand) 18%, var(--border)); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); }
.batch-hero::after { position: absolute; right: -48px; bottom: -76px; width: 240px; height: 240px; background: color-mix(in srgb, var(--brand) 7%, transparent); border-radius: 50%; content: ''; }
.batch-hero > * { position: relative; z-index: 1; }
.batch-hero__copy { max-width: 720px; }
.batch-back { display: inline-flex; min-height: 32px; align-items: center; gap: 6px; margin-bottom: 18px; color: var(--text-soft); font-size: 14px; font-weight: 600; }
.batch-back:hover { color: var(--brand-strong); }
.batch-eyebrow { display: flex; align-items: center; gap: 7px; margin-bottom: 8px; color: var(--brand-strong); font-size: 14px; font-weight: 700; }
.batch-hero h2 { margin: 0; font-size: clamp(1.5rem, 3cqi, 2rem); line-height: 1.25; }
.batch-hero__copy > p:last-child { max-width: 660px; margin: 10px 0 0; color: var(--text-soft); font-size: 15px; line-height: 1.65; }
.batch-hero__stats { display: grid; min-width: 300px; grid-template-columns: repeat(3, minmax(0, 1fr)); margin: 0; padding: 16px; background: color-mix(in srgb, var(--surface) 88%, transparent); border: 1px solid var(--border); border-radius: var(--radius); }
.batch-hero__stats div { min-width: 0; padding: 0 13px; text-align: center; border-right: 1px solid var(--border); }
.batch-hero__stats div:last-child { border-right: 0; }
.batch-hero__stats dt { color: var(--text-soft); font-size: 13px; }
.batch-hero__stats dd { margin: 7px 0 0; font-size: 20px; font-weight: 700; font-variant-numeric: tabular-nums; }
.batch-boundary { display: flex; align-items: flex-start; gap: 10px; padding: 14px 16px; color: var(--text-soft); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); font-size: 14px; line-height: 1.6; }
.batch-boundary svg { flex: 0 0 auto; margin-top: 2px; color: var(--brand-strong); }
.batch-layout { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(320px, .75fr); align-items: start; gap: 20px; }
.batch-panel { min-width: 0; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); }
.batch-panel__header { display: flex; min-height: 78px; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border); }
.batch-panel__header div { display: grid; gap: 3px; }
.batch-panel__header span { color: var(--brand-strong); font-size: 13px; font-weight: 700; }
.batch-panel__header h3 { margin: 0; font-size: 18px; }
.batch-form { display: grid; gap: 0; }
.batch-fieldset { min-width: 0; margin: 0; padding: 22px 20px; border: 0; border-bottom: 1px solid var(--border); }
.batch-fieldset legend { display: flex; align-items: center; gap: 8px; padding: 0; font-size: 16px; font-weight: 700; }
.batch-fieldset legend span { display: inline-grid; width: 24px; height: 24px; place-items: center; color: var(--on-brand); background: var(--brand-action); border-radius: 50%; font-size: 13px; }
.batch-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; margin-top: 16px; }
.batch-action { position: relative; display: grid; min-width: 0; grid-template-columns: auto minmax(0, 1fr); align-items: start; gap: 10px; padding: 14px; color: var(--text); text-align: left; background: var(--surface-subtle); border: 1px solid var(--border); border-radius: var(--radius); cursor: pointer; }
.batch-action:hover { border-color: color-mix(in srgb, var(--brand) 38%, var(--border)); background: var(--interaction-hover-surface); }
.batch-action.is-selected { border-color: var(--brand); background: color-mix(in srgb, var(--brand-soft) 54%, var(--surface)); box-shadow: inset 0 0 0 1px var(--brand); }
.batch-action.is-disruptive.is-selected { border-color: var(--danger); background: color-mix(in srgb, var(--danger) 7%, var(--surface)); box-shadow: inset 0 0 0 1px var(--danger); }
.batch-action__icon, .batch-host__icon { display: inline-grid; width: 36px; height: 36px; flex: 0 0 auto; place-items: center; color: var(--brand-strong); background: var(--brand-soft); border-radius: var(--radius-sm); }
.batch-action span:nth-child(2) { display: grid; min-width: 0; gap: 5px; padding-right: 70px; }
.batch-action strong { font-size: 14px; }
.batch-action small { color: var(--text-soft); font-size: 13px; line-height: 1.5; }
.batch-action em { position: absolute; top: 12px; right: 12px; padding: 3px 6px; color: var(--text-soft); background: var(--surface); border: 1px solid var(--border); border-radius: 999px; font-size: 12px; font-style: normal; }
.batch-host-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 16px; }
.batch-search { display: flex; min-width: 0; flex: 1; align-items: center; gap: 8px; padding: 0 11px; background: var(--surface-subtle); border: 1px solid var(--border); border-radius: var(--radius-sm); }
.batch-search svg { color: var(--muted); }
.batch-search input { width: 100%; min-height: 40px; padding: 0; color: var(--text); background: transparent; border: 0; outline: 0; font-size: 14px; }
.batch-selection-summary { margin: 12px 0 10px; color: var(--text-soft); font-size: 13px; line-height: 1.5; }
.batch-hosts { display: grid; gap: 7px; max-height: 420px; padding-right: 3px; overflow-y: auto; }
.batch-host { display: flex; min-width: 0; align-items: center; gap: 10px; padding: 11px 12px; background: var(--surface-subtle); border: 1px solid var(--border); border-radius: var(--radius); cursor: pointer; }
.batch-host:hover:not(.is-disabled) { border-color: color-mix(in srgb, var(--brand) 35%, var(--border)); }
.batch-host.is-selected { background: color-mix(in srgb, var(--brand-soft) 48%, var(--surface)); border-color: var(--brand-muted); }
.batch-host.is-disabled { cursor: not-allowed; opacity: .72; }
.batch-host input { width: 17px; height: 17px; flex: 0 0 auto; accent-color: var(--brand); }
.batch-host__body { display: grid; min-width: 0; flex: 1; gap: 3px; }
.batch-host__body strong { overflow-wrap: anywhere; font-size: 14px; }
.batch-host__body small { color: var(--text-soft); font-size: 13px; line-height: 1.45; overflow-wrap: anywhere; }
.batch-host__body em { color: var(--warning); font-size: 13px; font-style: normal; line-height: 1.45; }
.batch-settings { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.batch-settings legend { grid-column: 1 / -1; }
.batch-settings .field { margin-top: 12px; }
.batch-submit { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 20px; }
.batch-submit > div { display: grid; gap: 4px; }
.batch-submit strong { font-size: 15px; }
.batch-submit small { color: var(--text-soft); font-size: 13px; }
.batch-history { position: sticky; top: 16px; overflow: hidden; }
.batch-task-list { display: grid; max-height: min(720px, 76vh); overflow-y: auto; }
.batch-task { display: flex; width: 100%; min-width: 0; align-items: flex-start; gap: 11px; padding: 15px 16px; color: var(--text); text-align: left; background: var(--surface); border: 0; border-bottom: 1px solid var(--border); cursor: pointer; }
.batch-task:last-child { border-bottom: 0; }
.batch-task:hover { background: var(--interaction-hover-surface); }
.batch-task__icon { display: grid; width: 34px; height: 34px; flex: 0 0 auto; place-items: center; color: var(--text-soft); background: var(--surface-subtle); border-radius: 50%; }
.batch-task__icon.is-succeeded { color: var(--success); background: color-mix(in srgb, var(--success) 10%, var(--surface)); }
.batch-task__icon:is(.is-failed, .is-needs_attention) { color: var(--danger); background: color-mix(in srgb, var(--danger) 9%, var(--surface)); }
.batch-task__icon.is-partial { color: var(--warning); background: color-mix(in srgb, var(--warning) 10%, var(--surface)); }
.batch-task__body { display: grid; min-width: 0; flex: 1; gap: 7px; }
.batch-task__body > span:first-child { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.batch-task__body strong { overflow-wrap: anywhere; font-size: 14px; }
.batch-task__body small, .batch-task__issues { color: var(--text-soft); font-size: 13px; line-height: 1.45; }
.batch-task__issues { color: var(--warning); }
.batch-progress, .batch-target__progress { display: flex; align-items: center; gap: 9px; }
.batch-progress i, .batch-target__progress i { height: 5px; flex: 1; overflow: hidden; background: var(--surface-subtle); border-radius: 999px; }
.batch-progress b, .batch-target__progress b { display: block; height: 100%; background: var(--brand); border-radius: inherit; transition: width 180ms ease; }
.batch-progress em { color: var(--text-soft); font-size: 12px; font-style: normal; font-variant-numeric: tabular-nums; }
.batch-confirm { display: flex; align-items: flex-start; gap: 14px; }
.batch-confirm__icon { display: grid; width: 48px; height: 48px; flex: 0 0 auto; place-items: center; color: var(--brand-strong); background: var(--brand-soft); border-radius: var(--radius); }
.batch-confirm__icon.is-danger { color: var(--danger); background: color-mix(in srgb, var(--danger) 9%, var(--surface)); }
.batch-confirm strong { font-size: 16px; }
.batch-confirm p { margin: 6px 0 0; color: var(--text-soft); font-size: 14px; line-height: 1.6; }
.batch-reboot-check { display: flex; align-items: flex-start; gap: 11px; padding: 14px; background: color-mix(in srgb, var(--danger) 6%, var(--surface)); border: 1px solid color-mix(in srgb, var(--danger) 30%, var(--border)); border-radius: var(--radius); cursor: pointer; }
.batch-reboot-check input { width: 18px; height: 18px; margin-top: 2px; accent-color: var(--danger); }
.batch-reboot-check span { display: grid; gap: 4px; }
.batch-reboot-check strong { font-size: 14px; }
.batch-reboot-check small { color: var(--text-soft); font-size: 13px; line-height: 1.5; }
.batch-detail-summary { display: grid; gap: 14px; }
.batch-detail-summary > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.batch-detail-summary > div > span:last-child { font-size: 22px; font-weight: 700; }
.batch-detail-summary dl { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); margin: 0; background: var(--surface-subtle); border: 1px solid var(--border); border-radius: var(--radius); }
.batch-detail-summary dl div { padding: 12px; text-align: center; border-right: 1px solid var(--border); }
.batch-detail-summary dl div:last-child { border-right: 0; }
.batch-detail-summary dt { color: var(--text-soft); font-size: 13px; }
.batch-detail-summary dd { margin: 5px 0 0; font-size: 18px; font-weight: 700; }
.batch-detail-summary p { display: flex; align-items: center; gap: 6px; margin: 0; color: var(--text-soft); font-size: 13px; }
.batch-target-results { display: grid; gap: 10px; margin-top: 16px; }
.batch-target { padding: 15px; background: var(--surface-subtle); border: 1px solid var(--border); border-radius: var(--radius); }
.batch-target header { display: flex; align-items: center; gap: 10px; }
.batch-target header > div { display: grid; min-width: 0; flex: 1; gap: 3px; }
.batch-target header strong { overflow-wrap: anywhere; font-size: 14px; }
.batch-target header small { color: var(--text-soft); font-size: 13px; overflow-wrap: anywhere; }
.batch-target__progress { margin-top: 13px; }
.batch-target__progress span { width: 42px; color: var(--text-soft); font-size: 13px; text-align: right; }
.batch-target p { margin: 10px 0 0; font-size: 14px; line-height: 1.55; overflow-wrap: anywhere; }
.batch-target footer { display: flex; flex-wrap: wrap; gap: 8px 18px; margin-top: 8px; color: var(--text-soft); font-size: 13px; }

@container (max-width: 900px) {
  .batch-hero { align-items: stretch; flex-direction: column; }
  .batch-hero__stats { min-width: 0; }
  .batch-layout { grid-template-columns: minmax(0, 1fr); }
  .batch-history { position: static; }
  .batch-task-list { max-height: none; }
}

@container (max-width: 620px) {
  .batch-hero { padding: 20px; }
  .batch-hero__stats { grid-template-columns: 1fr; }
  .batch-hero__stats div { display: flex; align-items: center; justify-content: space-between; padding: 9px 0; text-align: left; border-right: 0; border-bottom: 1px solid var(--border); }
  .batch-hero__stats div:last-child { border-bottom: 0; }
  .batch-hero__stats dd { margin: 0; font-size: 17px; }
  .batch-actions, .batch-settings { grid-template-columns: minmax(0, 1fr); }
  .batch-host-toolbar, .batch-submit { align-items: stretch; flex-direction: column; }
  .batch-host-toolbar .button, .batch-submit .button { width: 100%; }
  .batch-host { align-items: flex-start; flex-wrap: wrap; }
  .batch-host .status-badge { margin-left: 65px; }
  .batch-detail-summary dl { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .batch-detail-summary dl div { border-bottom: 1px solid var(--border); }
  .batch-detail-summary dl div:nth-child(2n) { border-right: 0; }
  .batch-detail-summary dl div:last-child { grid-column: 1 / -1; border-bottom: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .batch-progress b, .batch-target__progress b { transition: none; }
}
</style>
