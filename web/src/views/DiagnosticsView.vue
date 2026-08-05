<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from '@/i18n'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog(() => import('@/i18n/pages/DiagnosticsView/en-US').then((module) => module.default))
import {
  Activity,
  Cpu,
  ExternalLink,
  Gauge,
  Globe2,
  History,
  LoaderCircle,
  Maximize2,
  Minimize2,
  Network,
  Play,
  RefreshCw,
  ShieldCheck,
  Timer,
  TriangleAlert,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import AppInteractiveTerminal from '@/components/apps/AppInteractiveTerminal.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { containWheelScroll } from '@/lib/scroll'
import { useToast } from '@/stores/toast'
import type { DiagnosticCatalog, DiagnosticCheck, DiagnosticJob } from '@/types/api'

const catalog = ref<DiagnosticCatalog>()
const jobs = ref<DiagnosticJob[]>([])
const selectedCategory = ref('all')
const selectedCheck = ref<DiagnosticCheck>()
const pendingCheck = ref<DiagnosticCheck>()
const activeJob = ref<DiagnosticJob>()
const loading = ref(true)
const refreshing = ref(false)
const starting = ref(false)
const error = ref('')
const fullscreen = ref(false)
const toast = useToast()
const { t } = useI18n()
let controller: AbortController | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let pollInFlight = false
let pollFailures = 0

const categories = computed(() => catalog.value?.categories || [])
const visibleChecks = computed(() =>
  (catalog.value?.items || []).filter(
    (item) => selectedCategory.value === 'all' || item.category === selectedCategory.value,
  ),
)
const recentJobs = computed(() => jobs.value.slice(0, 10))
const hasActiveJob = computed(
  () => activeJob.value?.status === 'queued' || activeJob.value?.status === 'running',
)
const activeLog = computed(() => activeJob.value?.logs.join('\n') || '等待脚本输出…')

function categoryName(id: string): string {
  return categories.value.find((item) => item.id === id)?.name || id
}

function categoryIcon(id: string) {
  if (id === 'access') return Globe2
  if (id === 'network') return Network
  if (id === 'hardware') return Cpu
  return Gauge
}

function impactLabel(impact: DiagnosticCheck['impact']): string {
  if (impact === 'light') return '轻量检测'
  if (impact === 'network') return '消耗网络流量'
  return '高负载跑分'
}

function impactClass(impact: DiagnosticCheck['impact']): string {
  return `is-${impact}`
}

function sourceHost(value: string): string {
  try {
    return new URL(value).hostname
  } catch {
    return value
  }
}

function stopPolling(): void {
  if (pollTimer) window.clearInterval(pollTimer)
  pollTimer = undefined
  pollController?.abort()
  pollController = undefined
}

async function refreshJob(id: string): Promise<void> {
  if (pollInFlight) return
  pollInFlight = true
  const requestController = new AbortController()
  pollController = requestController
  try {
    const next = await api.diagnostics.job(id, requestController.signal)
    pollFailures = 0
    const previous = activeJob.value?.status
    activeJob.value = next
    const index = jobs.value.findIndex((item) => item.id === next.id)
    if (index >= 0) jobs.value.splice(index, 1, next)
    else jobs.value.unshift(next)
    if (next.status === 'succeeded' || next.status === 'failed') {
      stopPolling()
      if (previous === 'queued' || previous === 'running') {
        if (next.status === 'succeeded') toast.success(`${next.checkName}已完成`)
        else toast.danger(`${next.checkName}执行失败`, next.message)
      }
    }
  } catch (reason) {
    if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
      pollFailures += 1
      if (pollFailures >= 3) {
        toast.danger('体检进度刷新中断', '后台任务可能仍在运行，请稍后点击刷新重新连接。')
        stopPolling()
      }
    }
  } finally {
    if (pollController === requestController) pollController = undefined
    pollInFlight = false
  }
}

function startPolling(job: DiagnosticJob): void {
  stopPolling()
  pollFailures = 0
  activeJob.value = job
  void refreshJob(job.id)
  pollTimer = window.setInterval(() => void refreshJob(job.id), 2_000)
}

async function load(silent = false): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    const [nextCatalog, history] = await Promise.all([
      api.diagnostics.catalog(controller.signal),
      api.diagnostics.jobs(controller.signal),
    ])
    catalog.value = nextCatalog
    jobs.value = history.items
    if (!selectedCheck.value || !nextCatalog.items.some((item) => item.id === selectedCheck.value?.id)) {
      selectedCheck.value = nextCatalog.items[0]
    }
    const current = history.items.find((item) => item.id === activeJob.value?.id)
    if (current) activeJob.value = current
    const active = history.items.find((item) => item.status === 'queued' || item.status === 'running')
    if (active) startPolling(active)
    else stopPolling()
  } catch (reason) {
    if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
      error.value = reason instanceof ApiError ? reason.message : '无法读取体检项目，请检查 Agent 与 kejilion.sh 版本。'
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function confirmStart(): Promise<void> {
  const check = pendingCheck.value
  if (!check || starting.value || hasActiveJob.value) return
  starting.value = true
  try {
    const job = await api.diagnostics.start(check.id)
    jobs.value.unshift(job)
    pendingCheck.value = undefined
    startPolling(job)
    toast.success(`${check.name}已开始`, '任务已在后台运行；第三方脚本需要确认时可直接在终端输入。')
  } catch (reason) {
    toast.danger(
      '体检任务启动失败',
      reason instanceof ApiError ? reason.message : '请检查 Agent、systemd 与 kejilion.sh 体检协议。',
    )
  } finally {
    starting.value = false
  }
}

function openJob(job: DiagnosticJob): void {
  activeJob.value = job
  const check = catalog.value?.items.find((item) => item.id === job.checkId)
  if (check) {
    selectedCheck.value = check
    selectedCategory.value = check.category
  }
  if (job.status === 'queued' || job.status === 'running') startPolling(job)
}

function selectCheck(check: DiagnosticCheck): void {
  selectedCheck.value = check
  const matchingJob = jobs.value.find((job) => job.checkId === check.id)
  if (matchingJob) openJob(matchingJob)
  else if (!hasActiveJob.value) activeJob.value = undefined
}

function containLogWheel(event: WheelEvent): void {
  containWheelScroll(event, event.currentTarget as HTMLElement)
}

function setFullscreen(enabled: boolean): void {
  fullscreen.value = enabled
  document.body.classList.toggle('diagnostic-fullscreen-open', enabled)
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape' && fullscreen.value) setFullscreen(false)
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  void load()
})
onBeforeUnmount(() => {
  controller?.abort()
  stopPolling()
  window.removeEventListener('keydown', handleKeydown)
  document.body.classList.remove('diagnostic-fullscreen-open')
})
</script>

<template>
  <div class="diagnostics-page">
    <PageHeader
      title="体检"
      description="直接调用 kejilion.sh 的第三方测试合集，实时查看线路、IP 质量与性能跑分结果。"
    >
      <template #actions>
        <button class="button button--secondary" type="button" :disabled="refreshing" @click="load(true)">
          <RefreshCw :size="17" :class="{ 'is-spinning': refreshing }" />
          刷新
        </button>
      </template>
    </PageHeader>

    <LoadingState v-if="loading" title="正在读取体检项目" description="正在校验本机脚本协议与第三方来源。" />
    <ErrorState v-else-if="error" title="体检功能暂不可用" :message="error" @retry="load()" />

    <template v-else-if="catalog">
      <section class="diagnostic-workbench" :class="{ 'is-fullscreen': fullscreen }">
        <aside class="diagnostic-command-panel">
          <header class="diagnostic-command-panel__header">
            <span><ShieldCheck :size="18" /></span>
            <div>
              <strong>体检命令</strong>
              <small>固定命令由本机 kejilion.sh 提供，不接受自定义 Shell</small>
            </div>
          </header>
          <nav class="diagnostic-tabs" aria-label="体检分类">
            <button type="button" :class="{ 'is-active': selectedCategory === 'all' }" @click="selectedCategory = 'all'">
              全部 <span>{{ catalog.items.length }}</span>
            </button>
            <button
              v-for="item in categories"
              :key="item.id"
              type="button"
              :class="{ 'is-active': selectedCategory === item.id }"
              @click="selectedCategory = item.id"
            >
              {{ item.name }}
              <span>{{ catalog.items.filter((check) => check.category === item.id).length }}</span>
            </button>
          </nav>
          <div v-if="visibleChecks.length" class="diagnostic-command-list">
            <button
              v-for="check in visibleChecks"
              :key="check.id"
              type="button"
              :class="{ 'is-active': selectedCheck?.id === check.id }"
              @click="selectCheck(check)"
            >
              <span class="diagnostic-card__icon"><component :is="categoryIcon(check.category)" :size="19" /></span>
              <span>
                <strong>{{ check.name }}</strong>
                <small>{{ categoryName(check.category) }} · 约 {{ check.estimatedMinutes }} 分钟</small>
              </span>
            </button>
          </div>
          <EmptyState v-else title="当前分类没有项目" description="请切换其他分类。" />
        </aside>

        <section class="diagnostic-result">
          <header class="diagnostic-result__header">
            <div v-if="selectedCheck">
              <span class="eyebrow">{{ categoryName(selectedCheck.category) }}</span>
              <h2>{{ selectedCheck.name }}</h2>
              <p>{{ selectedCheck.description }}</p>
            </div>
            <div class="diagnostic-result__actions">
              <button
                v-if="selectedCheck"
                class="button button--primary"
                type="button"
                :disabled="hasActiveJob || starting"
                @click="pendingCheck = selectedCheck"
              >
                <Play :size="16" /> {{ hasActiveJob ? '任务运行中' : '开始体检' }}
              </button>
              <button
                class="diagnostic-fullscreen-toggle"
                type="button"
                :title="fullscreen ? t('common.exitFullscreen') : t('common.enterFullscreen')"
                :aria-label="fullscreen ? t('common.exitFullscreen') : t('common.enterFullscreen')"
                @click="setFullscreen(!fullscreen)"
              >
                <Minimize2 v-if="fullscreen" :size="17" />
                <Maximize2 v-else :size="17" />
              </button>
            </div>
          </header>
          <div v-if="hasActiveJob" class="diagnostic-progress" aria-label="任务进度">
            <span :style="{ width: `${activeJob?.progress || 0}%` }" />
          </div>
          <div v-if="!activeJob?.interactive" class="diagnostic-terminal-bar">
            <span><i :class="{ 'is-live': hasActiveJob }" /> {{ hasActiveJob ? '实时输出' : '终端输出' }}</span>
            <StatusBadge v-if="activeJob" :status="activeJob.status" />
          </div>
          <AppInteractiveTerminal
            v-if="activeJob?.interactive"
            class="diagnostic-interactive-terminal"
            :job-id="activeJob.id"
            :input-open="activeJob.inputOpen"
            kind="diagnostic"
          />
          <p v-else-if="!activeJob" class="diagnostic-log diagnostic-log-empty" @wheel="containLogWheel">选择左侧体检命令，点击“开始体检”后在这里查看实时输出。</p>
          <pre v-else class="diagnostic-log" aria-live="polite" data-i18n-ignore @wheel="containLogWheel">{{ activeLog }}</pre>
          <footer v-if="activeJob">
            <span><Activity :size="14" /> {{ activeJob.message }}</span>
            <span><Timer :size="14" /> {{ formatDateTime(activeJob.startedAt || activeJob.createdAt) }}</span>
            <a :href="activeJob.sourceUrl" target="_blank" rel="noopener noreferrer">
              查看来源 <ExternalLink :size="13" />
            </a>
          </footer>
          <footer v-else-if="selectedCheck">
            <span><Timer :size="14" /> 约 {{ selectedCheck.estimatedMinutes }} 分钟</span>
            <span class="impact-pill" :class="impactClass(selectedCheck.impact)">{{ impactLabel(selectedCheck.impact) }}</span>
            <a :href="selectedCheck.sourceUrl" target="_blank" rel="noopener noreferrer">
              {{ sourceHost(selectedCheck.sourceUrl) }} <ExternalLink :size="13" />
            </a>
          </footer>
        </section>
      </section>

      <section class="diagnostic-history">
        <header>
          <div>
            <span class="eyebrow">历史记录</span>
            <h2><History :size="19" /> 最近体检</h2>
          </div>
        </header>
        <div v-if="recentJobs.length" class="diagnostic-history__list">
          <button v-for="job in recentJobs" :key="job.id" type="button" @click="openJob(job)">
            <span>
              <strong>{{ job.checkName }}</strong>
              <small>{{ formatDateTime(job.createdAt) }} · {{ categoryName(job.category) }}</small>
            </span>
            <StatusBadge :status="job.status" subtle />
          </button>
        </div>
        <EmptyState v-else title="还没有体检记录" description="选择上方项目开始第一次服务器体检。" />
      </section>
    </template>

    <ModalDialog
      :open="Boolean(pendingCheck)"
      title="确认运行第三方体检？"
      :description="pendingCheck ? `${pendingCheck.name} · 预计 ${pendingCheck.estimatedMinutes} 分钟` : ''"
      size="small"
      @close="pendingCheck = undefined"
    >
      <div v-if="pendingCheck" class="diagnostic-confirm">
        <TriangleAlert :size="24" />
        <div>
          <p>
            此操作将以 root 权限运行 kejilion.sh 中登记的第三方命令，可能安装检测依赖并产生较高资源占用。
          </p>
          <a :href="pendingCheck.sourceUrl" target="_blank" rel="noopener noreferrer">
            {{ pendingCheck.sourceUrl }} <ExternalLink :size="13" />
          </a>
        </div>
      </div>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="starting" @click="pendingCheck = undefined">
          取消
        </button>
        <button class="button button--primary" type="button" :disabled="starting" @click="confirmStart">
          <LoaderCircle v-if="starting" :size="16" class="is-spinning" />
          <Play v-else :size="16" />
          {{ starting ? '正在启动' : '确认开始' }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.diagnostics-page {
  display: grid;
  gap: 16px;
}

.diagnostic-history {
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
}

.diagnostic-card p,
.diagnostic-result p {
  margin: 0;
}

.diagnostic-command-panel__header {
  display: flex;
  gap: 11px;
  align-items: center;
  padding: 15px 16px 12px;
}

.diagnostic-command-panel__header > span {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: 0 0 auto;
  border-radius: 10px;
  color: var(--success);
  background: color-mix(in srgb, var(--success) 11%, var(--surface));
}

.diagnostic-command-panel__header > div {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.diagnostic-command-panel__header small {
  color: var(--text-tertiary);
  font-size: 11px;
  line-height: 1.4;
}

.diagnostic-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 0 14px 13px;
  border-bottom: 1px solid var(--border);
}

.diagnostic-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  flex: 0 0 auto;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-secondary);
  padding: 7px 10px;
  font-size: 12px;
  cursor: pointer;
}

.diagnostic-tabs button span {
  color: var(--text-tertiary);
  font-size: 12px;
}

.diagnostic-tabs button.is-active {
  border-color: color-mix(in srgb, var(--primary) 42%, var(--border));
  background: color-mix(in srgb, var(--primary) 10%, var(--surface));
  color: var(--primary);
}

.diagnostic-workbench {
  display: grid;
  grid-template-columns: minmax(270px, 310px) minmax(0, 1fr);
  height: clamp(680px, calc(100vh - 190px), 860px);
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
}

.diagnostic-workbench.is-fullscreen {
  position: fixed;
  z-index: 5000;
  inset: 0;
  width: 100vw;
  height: 100dvh;
  min-height: 0;
  grid-template-columns: minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr);
  border: 0;
  border-radius: 0;
}

.diagnostic-workbench.is-fullscreen .diagnostic-command-panel {
  display: none;
}

.diagnostic-workbench.is-fullscreen .diagnostic-log,
.diagnostic-workbench.is-fullscreen .diagnostic-interactive-terminal :deep(.interactive-terminal__screen) {
  min-height: 0;
}

:global(body.diagnostic-fullscreen-open) {
  overflow: hidden;
}

.diagnostic-command-panel {
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  min-width: 0;
  min-height: 0;
  border-right: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-muted) 38%, var(--surface));
}

.diagnostic-command-list {
  display: grid;
  align-content: start;
  min-height: 0;
  overflow: auto;
  padding: 8px;
}

.diagnostic-command-list > button {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr);
  gap: 11px;
  align-items: center;
  width: 100%;
  padding: 12px;
  border: 1px solid transparent;
  border-radius: 11px;
  background: transparent;
  color: var(--text);
  text-align: left;
  cursor: pointer;
}

.diagnostic-command-list > button:hover {
  background: var(--surface);
}

.diagnostic-command-list > button.is-active {
  border-color: color-mix(in srgb, var(--primary) 35%, var(--border));
  background: color-mix(in srgb, var(--primary) 9%, var(--surface));
}

.diagnostic-command-list strong,
.diagnostic-command-list small {
  display: block;
}

.diagnostic-command-list small {
  margin-top: 4px;
  color: var(--text-tertiary);
  font-size: 12px;
}

.diagnostic-card__icon {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  border-radius: 13px;
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 11%, var(--surface));
}

.eyebrow {
  color: var(--text-tertiary);
  font-size: 12px;
}

.diagnostic-result h2,
.diagnostic-history h2 {
  margin: 2px 0 0;
  font-size: 17px;
}

.diagnostic-result footer span,
.diagnostic-result footer a,
.diagnostic-source {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.impact-pill {
  padding: 4px 7px;
  border-radius: 999px;
  background: var(--surface-muted);
}

.impact-pill.is-network {
  color: var(--warning);
}

.impact-pill.is-intensive {
  color: var(--danger);
}

.diagnostic-result,
.diagnostic-history {
  overflow: hidden;
}

.diagnostic-result {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--surface);
}

.diagnostic-result__header,
.diagnostic-history > header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding: 18px 20px;
  border-bottom: 1px solid var(--border);
}

.diagnostic-result__header p {
  margin-top: 5px;
  color: var(--text-secondary);
}

.diagnostic-result__actions {
  display: flex;
  flex: 0 0 auto;
  align-self: center;
  align-items: center;
  gap: 8px;
}

.diagnostic-fullscreen-toggle {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid var(--terminal-shell-border, #29383a);
  border-radius: 8px;
  color: var(--terminal-shell-muted, #8a9695);
  background: var(--terminal-shell-panel, #111a1d);
  cursor: pointer;
}

.diagnostic-fullscreen-toggle:hover {
  color: var(--terminal-shell-text, #d8dddc);
  border-color: var(--brand);
}

.diagnostic-progress {
  height: 4px;
  background: var(--surface-muted);
}

.diagnostic-progress span {
  display: block;
  height: 100%;
  min-width: 3%;
  border-radius: 999px;
  background: var(--primary);
  transition: width 220ms ease;
}

.diagnostic-terminal-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 42px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--terminal-shell-border, #29383a);
  background: var(--terminal-shell-panel, #111a1d);
  color: var(--terminal-shell-muted, #8a9695);
  font-size: 12px;
}

.diagnostic-terminal-bar > span:first-child {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.diagnostic-terminal-bar i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #667085;
}

.diagnostic-terminal-bar i.is-live {
  background: #36d399;
  box-shadow: 0 0 0 4px rgb(54 211 153 / 12%);
}

.diagnostic-log {
  flex: 1 1 auto;
  min-height: 0;
  max-height: none;
  overflow: auto;
  overscroll-behavior: contain;
  margin: 0;
  padding: 18px 20px;
  background: var(--terminal-shell-background, #0b1214);
  color: var(--terminal-shell-text, #d8dddc);
  font: 12.5px/1.65 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.diagnostic-interactive-terminal {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  flex: 1 1 auto;
  min-height: 0;
  border: 0;
  border-radius: 0;
}

.diagnostic-interactive-terminal :deep(.interactive-terminal__screen) {
  height: 100%;
  min-height: 0;
}

.diagnostic-result footer {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 18px;
  padding: 12px 20px;
  color: var(--text-tertiary);
  font-size: 12px;
}

.diagnostic-result footer a {
  color: var(--primary);
}

.diagnostic-history h2 {
  display: flex;
  align-items: center;
  gap: 7px;
}

.diagnostic-history__list {
  display: grid;
}

.diagnostic-history__list button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
  padding: 14px 20px;
  border: 0;
  border-bottom: 1px solid var(--border);
  background: transparent;
  color: var(--text);
  text-align: left;
  cursor: pointer;
}

.diagnostic-history__list button:last-child {
  border-bottom: 0;
}

.diagnostic-history__list button:hover {
  background: var(--surface-muted);
}

.diagnostic-history__list strong,
.diagnostic-history__list small {
  display: block;
}

.diagnostic-history__list small {
  margin-top: 4px;
  color: var(--text-tertiary);
}

.diagnostic-confirm {
  display: flex;
  gap: 14px;
  color: var(--warning);
}

.diagnostic-confirm > svg {
  flex: 0 0 auto;
}

.diagnostic-confirm p {
  margin: 0 0 10px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.diagnostic-confirm a {
  color: var(--primary);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.is-spinning {
  animation: diagnostic-spin 900ms linear infinite;
}

@keyframes diagnostic-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 680px) {
  .diagnostic-workbench {
    grid-template-columns: 1fr;
    height: auto;
  }

  .diagnostic-command-panel {
    border-right: 0;
    border-bottom: 1px solid var(--border);
  }

  .diagnostic-command-list {
    max-height: 300px;
  }

  .diagnostic-log,
  .diagnostic-interactive-terminal :deep(.interactive-terminal__screen) {
    min-height: 420px;
  }

  .diagnostic-result__header {
    flex-direction: column;
  }

  .diagnostic-result__actions,
  .diagnostic-result__actions > .button {
    width: 100%;
  }

  .diagnostic-result__actions {
    align-self: stretch;
  }
}
</style>
