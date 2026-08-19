<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from '@/i18n'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/DiagnosticsView/en-US').then((module) => module.default)
  : import('@/i18n/pages/DiagnosticsView/zh-TW').then((module) => module.default))
import {
  Activity,
  Cpu,
  ExternalLink,
  Gauge,
  Globe2,
  LoaderCircle,
  Menu,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Play,
  Timer,
  TriangleAlert,
  X,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import AppInteractiveTerminal from '@/components/apps/AppInteractiveTerminal.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { formatDateTime } from '@/lib/format'
import { containWheelScroll } from '@/lib/scroll'
import { useToast } from '@/stores/toast'
import type { DiagnosticCatalog, DiagnosticCheck, DiagnosticJob } from '@/types/api'

const catalog = ref<DiagnosticCatalog>()
const jobs = ref<DiagnosticJob[]>([])
const selectedCheck = ref<DiagnosticCheck>()
const pendingCheck = ref<DiagnosticCheck>()
const activeJob = ref<DiagnosticJob>()
const loading = ref(true)
const starting = ref(false)
const error = ref('')
const commandsCollapsed = ref(false)
const mobileCommandsOpen = ref(false)
const toast = useToast()
const i18n = useI18n()
const windowActive = inject(desktopWindowActiveKey, computed(() => true))
let controller: AbortController | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let pollFailures = 0
let pollGeneration = 0
let pollingJobID = ''
const activePollDelay = 2_000
const backgroundPollDelay = 15_000
const commandsCollapsedStorageKey = 'kpanel:diagnostics:commands-collapsed'

const categories = computed(() => catalog.value?.categories || [])
const groupedChecks = computed(() =>
  categories.value
    .map((category) => ({
      ...category,
      items: (catalog.value?.items || []).filter((item) => item.category === category.id),
    }))
    .filter((category) => category.items.length),
)
const testedCheckIDs = computed(() => new Set(
  jobs.value
    .filter((job) => job.status === 'succeeded' || job.status === 'failed')
    .map((job) => job.checkId),
))
const hasActiveJob = computed(
  () => activeJob.value?.status === 'queued' || activeJob.value?.status === 'running',
)
const activeLog = computed(() => activeJob.value?.logs.join('\n') || '等待脚本输出…')

function categoryName(id: string): string {
  if (i18n.locale.value === 'en-US') {
    const labels: Record<string, string> = {
      access: 'IP & Access',
      network: 'Network',
      hardware: 'Hardware',
      benchmark: 'Benchmarks',
    }
    return labels[id] || id
  }
  return categories.value.find((item) => item.id === id)?.name || id
}

function checkNameLabel(value: string): string {
  if (i18n.locale.value !== 'en-US') return value
  const labels: Record<string, string> = {
    'ChatGPT 解锁检测': 'ChatGPT access check',
    'IP 质量体检': 'IP quality check',
    'SuperSpeed 三网测速': 'SuperSpeed network test',
    '网络质量体检': 'Network quality check',
    'YABS 性能测试': 'YABS benchmark',
    'NodeQuality 综合测评': 'NodeQuality benchmark',
  }
  return labels[value] || value
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
  pollGeneration += 1
  pollingJobID = ''
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = undefined
  pollController?.abort()
  pollController = undefined
}

function schedulePoll(id: string, generation: number, delay: number): void {
  if (generation !== pollGeneration || pollingJobID !== id) return
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(() => {
    pollTimer = undefined
    void refreshJob(id, generation)
  }, delay)
}

async function refreshJob(id: string, generation = pollGeneration): Promise<void> {
  if (pollController) return
  const requestController = new AbortController()
  pollController = requestController
  try {
    const next = await api.diagnostics.job(id, requestController.signal)
    if (generation !== pollGeneration || pollController !== requestController) return
    pollFailures = 0
    const previous = activeJob.value?.status
    activeJob.value = next
    const index = jobs.value.findIndex((item) => item.id === next.id)
    if (index >= 0) jobs.value.splice(index, 1, next)
    else jobs.value.unshift(next)
    if (next.status === 'succeeded' || next.status === 'failed') {
      stopPolling()
      if (previous === 'queued' || previous === 'running') {
        if (next.status === 'succeeded') toast.success(`${checkNameLabel(next.checkName)}已完成`)
        else toast.danger(`${checkNameLabel(next.checkName)}执行失败`, next.message)
      }
    }
  } catch (reason) {
    if (
      generation === pollGeneration &&
      pollController === requestController &&
      !(reason instanceof DOMException && reason.name === 'AbortError')
    ) {
      pollFailures += 1
      if (pollFailures >= 3) {
        toast.danger('体检进度刷新中断', '后台任务可能仍在运行，请稍后点击刷新重新连接。')
        stopPolling()
      }
    }
  } finally {
    if (pollController === requestController) pollController = undefined
    if (
      generation === pollGeneration &&
      pollingJobID === id &&
      hasActiveJob.value
    ) {
      schedulePoll(
        id,
        generation,
        windowActive.value ? activePollDelay : backgroundPollDelay,
      )
    }
  }
}

function startPolling(job: DiagnosticJob, immediate = windowActive.value): void {
  stopPolling()
  pollFailures = 0
  activeJob.value = job
  pollingJobID = job.id
  const generation = pollGeneration
  if (immediate) void refreshJob(job.id, generation)
  else schedulePoll(job.id, generation, backgroundPollDelay)
}

async function load(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
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
    toast.success(`${checkNameLabel(check.name)}已开始`, '任务已在后台运行；第三方脚本需要确认时可直接在终端输入。')
  } catch (reason) {
    toast.danger(
      '体检任务启动失败',
      reason instanceof ApiError ? reason.message : '请检查 Agent、systemd 和 kejilion.sh 是否正常并保持版本一致。',
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
  }
  if (job.status === 'queued' || job.status === 'running') startPolling(job)
}

function selectCheck(check: DiagnosticCheck): void {
  selectedCheck.value = check
  mobileCommandsOpen.value = false
  const matchingJob = jobs.value.find((job) => job.checkId === check.id)
  if (matchingJob) openJob(matchingJob)
  else if (!hasActiveJob.value) activeJob.value = undefined
}

function requestCheck(check: DiagnosticCheck): void {
  selectCheck(check)
  pendingCheck.value = check
}

function runCheckLabel(check: DiagnosticCheck): string {
  return `运行 ${checkNameLabel(check.name)}`
}

function containLogWheel(event: WheelEvent): void {
  containWheelScroll(event, event.currentTarget as HTMLElement)
}

function toggleCommands(): void {
  commandsCollapsed.value = !commandsCollapsed.value
  try {
    window.localStorage.setItem(
      commandsCollapsedStorageKey,
      commandsCollapsed.value ? '1' : '0',
    )
  } catch {
    // Collapsing remains available when storage is blocked.
  }
}

watch(windowActive, (active) => {
  const job = activeJob.value
  if (!job || (job.status !== 'queued' && job.status !== 'running')) return
  startPolling(job, active)
})

onMounted(() => {
  try {
    commandsCollapsed.value = window.localStorage.getItem(commandsCollapsedStorageKey) === '1'
  } catch {
    commandsCollapsed.value = false
  }
  void load()
})
onBeforeUnmount(() => {
  controller?.abort()
  stopPolling()
})
</script>

<template>
  <div class="diagnostics-page">
    <PageHeader
      title="体检"
      description="调用 kejilion.sh 的第三方测试工具，查看网络线路、IP 质量和服务器性能。"
    />

    <LoadingState v-if="loading" title="正在读取体检项目" description="正在检查本机脚本能力与第三方测试来源。" />
    <ErrorState v-else-if="error" title="体检功能暂不可用" :message="error" @retry="load()" />

    <template v-else-if="catalog">
      <section
        class="diagnostic-workbench"
        :class="{
          'is-command-panel-collapsed': commandsCollapsed,
          'is-command-drawer-open': mobileCommandsOpen,
        }"
      >
        <button
          v-if="mobileCommandsOpen"
          class="diagnostic-command-overlay"
          type="button"
          aria-label="关闭体检项目选择"
          @click="mobileCommandsOpen = false"
        />
        <aside id="diagnostic-command-drawer" class="diagnostic-command-panel">
          <button
            class="diagnostic-command-panel__toggle diagnostic-command-panel__desktop-toggle"
            type="button"
            aria-controls="diagnostic-command-selector"
            :aria-expanded="!commandsCollapsed"
            :title="commandsCollapsed ? '展开体检列表' : '收起体检列表'"
            :aria-label="commandsCollapsed ? '展开体检列表' : '收起体检列表'"
            @click="toggleCommands"
          >
            <PanelLeftOpen v-if="commandsCollapsed" :size="17" />
            <PanelLeftClose v-else :size="17" />
          </button>
          <button
            class="diagnostic-command-panel__toggle diagnostic-command-panel__mobile-close"
            type="button"
            aria-label="关闭体检项目选择"
            @click="mobileCommandsOpen = false"
          >
            <X :size="17" />
          </button>
          <div id="diagnostic-command-selector" v-if="groupedChecks.length" v-show="!commandsCollapsed || mobileCommandsOpen" class="diagnostic-command-list">
            <section
              v-for="group in groupedChecks"
              :key="group.id"
              class="diagnostic-command-group"
              :class="`is-category-${group.id}`"
            >
              <header>
                <span>{{ categoryName(group.id) }}</span>
                <small>{{ group.items.length }}</small>
              </header>
              <div
                v-for="check in group.items"
                :key="check.id"
                class="diagnostic-command-row"
                :class="[`is-category-${check.category}`, { 'is-active': selectedCheck?.id === check.id }]"
              >
                <button class="diagnostic-command-select" type="button" @click="selectCheck(check)">
                  <span class="diagnostic-card__icon"><component :is="categoryIcon(check.category)" :size="17" /></span>
                  <strong>{{ checkNameLabel(check.name) }}</strong>
                  <small v-if="testedCheckIDs.has(check.id)" class="diagnostic-command-tested">已测</small>
                </button>
                <button
                  class="diagnostic-command-run"
                  type="button"
                  :disabled="hasActiveJob || starting"
                  :title="runCheckLabel(check)"
                  :aria-label="runCheckLabel(check)"
                  @click="requestCheck(check)"
                >
                  <LoaderCircle v-if="starting && pendingCheck?.id === check.id" :size="15" class="is-spinning" />
                  <Play v-else :size="15" />
                </button>
              </div>
            </section>
          </div>
          <div v-if="commandsCollapsed && !mobileCommandsOpen" class="diagnostic-command-rail" aria-label="收起的体检命令列表">
            <button
              v-for="check in catalog.items"
              :key="check.id"
              class="diagnostic-command-rail__item"
              :class="[`is-category-${check.category}`, { 'is-active': selectedCheck?.id === check.id }]"
              type="button"
              :title="checkNameLabel(check.name)"
              :aria-label="checkNameLabel(check.name)"
              @click="selectCheck(check)"
            >
              <component :is="categoryIcon(check.category)" :size="17" />
            </button>
          </div>
          <EmptyState v-if="(!commandsCollapsed || mobileCommandsOpen) && !groupedChecks.length" title="暂无体检项目" description="请刷新后重试。" />
        </aside>

        <section class="diagnostic-result">
          <div class="diagnostic-mobile-selector">
            <button
              type="button"
              aria-controls="diagnostic-command-drawer"
              :aria-expanded="mobileCommandsOpen"
              aria-label="打开体检项目选择"
              @click="mobileCommandsOpen = true"
            >
              <Menu :size="18" />
              <span>{{ selectedCheck ? checkNameLabel(selectedCheck.name) : '选择体检项目' }}</span>
            </button>
            <small>{{ catalog.items.length }} 个项目</small>
          </div>
          <div v-if="hasActiveJob" class="diagnostic-progress" aria-label="任务进度">
            <span :style="{ width: `${activeJob?.progress || 0}%` }" />
          </div>
          <div v-if="!activeJob?.interactive" class="diagnostic-terminal-bar">
            <span><i :class="{ 'is-live': hasActiveJob }" /> {{ hasActiveJob ? '实时输出' : '终端输出' }}</span>
            <StatusBadge v-if="activeJob" :status="activeJob.status" />
          </div>
          <AppInteractiveTerminal
            v-if="activeJob?.interactive && windowActive"
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

    </template>

    <ModalDialog
      :open="Boolean(pendingCheck)"
      title="确认运行第三方体检？"
      :description="pendingCheck ? `${checkNameLabel(pendingCheck.name)} · 预计 ${pendingCheck.estimatedMinutes} 分钟` : ''"
      size="small"
      @close="pendingCheck = undefined"
    >
      <div v-if="pendingCheck" class="diagnostic-confirm">
        <TriangleAlert :size="24" />
        <div>
          <p>
            此操作将以 root 权限运行 kejilion.sh 中登记的第三方命令，可能安装测试工具并占用较多网络、CPU 或磁盘资源。
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

.diagnostic-card p,
.diagnostic-result p {
  margin: 0;
}

.diagnostic-workbench {
  display: grid;
  grid-template-columns: minmax(270px, 310px) minmax(0, 1fr);
  height: var(--terminal-workspace-height);
  min-height: var(--terminal-workspace-min-height);
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--terminal-workspace-radius);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
  transition: grid-template-columns 180ms ease;
}

.diagnostic-workbench.is-command-panel-collapsed {
  grid-template-columns: 52px minmax(0, 1fr);
}

.diagnostic-command-panel {
  position: relative;
  display: grid;
  grid-template-rows: minmax(0, 1fr);
  min-width: 0;
  min-height: 0;
  border-right: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-subtle) 38%, var(--surface));
}

.diagnostic-command-panel__toggle {
  position: absolute;
  z-index: 3;
  top: 8px;
  right: 8px;
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-muted);
  background: var(--surface);
  cursor: pointer;
}

.diagnostic-command-panel__toggle:hover,
.diagnostic-command-panel__toggle:focus-visible {
  color: var(--primary);
  border-color: color-mix(in srgb, var(--primary) 50%, var(--border));
  outline: none;
}

.diagnostic-command-panel__mobile-close,
.diagnostic-command-overlay,
.diagnostic-mobile-selector {
  display: none;
}

.diagnostic-workbench.is-command-panel-collapsed .diagnostic-command-panel__toggle {
  right: 10px;
}

.diagnostic-command-list {
  display: grid;
  align-content: start;
  min-height: 0;
  overflow: auto;
  padding: 8px;
}

.diagnostic-command-group:first-child > header {
  min-height: 34px;
  padding-right: 42px;
}

.diagnostic-command-rail {
  display: grid;
  min-height: 0;
  align-content: start;
  justify-items: center;
  gap: 7px;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 48px 7px 10px;
  scrollbar-width: none;
}

.diagnostic-command-rail::-webkit-scrollbar {
  display: none;
}

.diagnostic-command-rail__item {
  --diagnostic-category: var(--primary);

  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--diagnostic-category) 26%, transparent);
  border-radius: 9px;
  color: var(--diagnostic-category);
  background: color-mix(in srgb, var(--diagnostic-category) 10%, var(--surface));
  cursor: pointer;
}

.diagnostic-command-rail__item:hover,
.diagnostic-command-rail__item:focus-visible,
.diagnostic-command-rail__item.is-active {
  border-color: var(--diagnostic-category);
  background: color-mix(in srgb, var(--diagnostic-category) 18%, var(--surface));
  outline: none;
}

.diagnostic-command-group {
  --diagnostic-category: var(--primary);
}

.diagnostic-command-group + .diagnostic-command-group {
  padding-top: 8px;
  margin-top: 8px;
  border-top: 1px dashed color-mix(in srgb, var(--diagnostic-category) 28%, var(--border));
}

.diagnostic-command-group > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 9px 5px;
  color: var(--diagnostic-category);
  font-size: 11px;
  font-weight: 700;
}

.diagnostic-command-group > header small {
  color: var(--text-tertiary);
  font-size: 10px;
}

.diagnostic-command-row {
  --diagnostic-category: var(--primary);

  display: grid;
  grid-template-columns: minmax(0, 1fr) 34px;
  gap: 4px;
  align-items: center;
  width: 100%;
  border: 1px solid transparent;
  border-radius: 10px;
  background: transparent;
}

.diagnostic-command-group.is-category-access,
.diagnostic-command-row.is-category-access,
.diagnostic-command-rail__item.is-category-access { --diagnostic-category: #087a72; }
.diagnostic-command-group.is-category-network,
.diagnostic-command-row.is-category-network,
.diagnostic-command-rail__item.is-category-network { --diagnostic-category: #2563c4; }
.diagnostic-command-group.is-category-hardware,
.diagnostic-command-row.is-category-hardware,
.diagnostic-command-rail__item.is-category-hardware { --diagnostic-category: #965900; }
.diagnostic-command-group.is-category-benchmark,
.diagnostic-command-row.is-category-benchmark,
.diagnostic-command-rail__item.is-category-benchmark { --diagnostic-category: #7546c8; }
.diagnostic-command-group.is-category-comprehensive,
.diagnostic-command-row.is-category-comprehensive,
.diagnostic-command-rail__item.is-category-comprehensive { --diagnostic-category: #7546c8; }

:global(:root[data-theme='dark'] .diagnostic-command-group.is-category-access),
:global(:root[data-theme='dark'] .diagnostic-command-row.is-category-access),
:global(:root[data-theme='dark'] .diagnostic-command-rail__item.is-category-access) { --diagnostic-category: #4ecdc4; }
:global(:root[data-theme='dark'] .diagnostic-command-group.is-category-network),
:global(:root[data-theme='dark'] .diagnostic-command-row.is-category-network),
:global(:root[data-theme='dark'] .diagnostic-command-rail__item.is-category-network) { --diagnostic-category: #6ea8fe; }
:global(:root[data-theme='dark'] .diagnostic-command-group.is-category-hardware),
:global(:root[data-theme='dark'] .diagnostic-command-row.is-category-hardware),
:global(:root[data-theme='dark'] .diagnostic-command-rail__item.is-category-hardware) { --diagnostic-category: #f5b942; }
:global(:root[data-theme='dark'] .diagnostic-command-group.is-category-benchmark),
:global(:root[data-theme='dark'] .diagnostic-command-row.is-category-benchmark),
:global(:root[data-theme='dark'] .diagnostic-command-rail__item.is-category-benchmark),
:global(:root[data-theme='dark'] .diagnostic-command-group.is-category-comprehensive),
:global(:root[data-theme='dark'] .diagnostic-command-row.is-category-comprehensive),
:global(:root[data-theme='dark'] .diagnostic-command-rail__item.is-category-comprehensive) { --diagnostic-category: #b58cff; }

.diagnostic-command-select {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr) auto;
  gap: 9px;
  align-items: center;
  min-width: 0;
  padding: 7px 6px 7px 8px;
  border: 0;
  color: var(--text);
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.diagnostic-command-row:hover {
  background: var(--surface);
}

.diagnostic-command-row.is-active {
  border-color: color-mix(in srgb, var(--diagnostic-category) 42%, var(--border));
  background: color-mix(in srgb, var(--diagnostic-category) 8%, var(--surface));
}

.diagnostic-command-select strong {
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.diagnostic-command-tested {
  border-radius: 999px;
  padding: 2px 6px;
  color: var(--diagnostic-category);
  background: color-mix(in srgb, var(--diagnostic-category) 12%, var(--surface));
  font-size: 10px;
  font-weight: 700;
  white-space: nowrap;
}

.diagnostic-card__icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 9px;
  color: var(--diagnostic-category);
  background: color-mix(in srgb, var(--diagnostic-category) 12%, var(--surface));
}

.diagnostic-command-run {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--diagnostic-category) 38%, var(--border));
  border-radius: 8px;
  color: var(--diagnostic-category);
  background: color-mix(in srgb, var(--diagnostic-category) 8%, var(--surface));
  cursor: pointer;
}

.diagnostic-command-run:hover:not(:disabled) {
  color: var(--surface);
  background: var(--diagnostic-category);
}

.diagnostic-command-run:disabled {
  cursor: not-allowed;
  opacity: .42;
}

.diagnostic-result h2 {
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

.diagnostic-result {
  overflow: hidden;
}

.diagnostic-result {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--surface);
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
  border-bottom: 1px solid color-mix(in srgb, var(--terminal-shell-border, #29383a) 78%, var(--terminal-shell-text, #d8dddc));
  background: var(--terminal-shell-panel, #111a1d);
  color: color-mix(in srgb, var(--terminal-shell-text, #d8dddc) 78%, var(--terminal-shell-muted, #8a9695));
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
  flex: 1 1 0;
  min-height: 0;
  border: 0;
  border-radius: 0;
}

.diagnostic-interactive-terminal :deep(.interactive-terminal__screen) {
  height: auto;
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
    position: relative;
    grid-template-columns: minmax(0, 1fr);
    height: auto;
    min-height: min(580px, calc(100dvh - 110px));
    border-radius: 14px;
  }

  .diagnostic-workbench.is-command-panel-collapsed {
    grid-template-columns: minmax(0, 1fr);
  }

  .diagnostic-command-panel {
    position: absolute;
    z-index: 22;
    inset: 0 auto 0 0;
    width: min(320px, calc(100% - 48px));
    border-right: 1px solid var(--border);
    border-bottom: 0;
    box-shadow: var(--shadow-md);
    transform: translateX(-105%);
    transition: transform .2s ease;
  }

  .diagnostic-workbench.is-command-drawer-open .diagnostic-command-panel {
    transform: translateX(0);
  }

  .diagnostic-command-overlay {
    position: absolute;
    z-index: 21;
    inset: 0;
    display: block;
    border: 0;
    background: rgb(5 16 13 / 42%);
  }

  .diagnostic-command-panel__desktop-toggle {
    display: none;
  }

  .diagnostic-command-panel__mobile-close {
    display: grid;
  }

  .diagnostic-command-list {
    max-height: none;
    padding-top: 44px;
  }

  .diagnostic-mobile-selector {
    display: flex;
    min-width: 0;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    min-height: 50px;
    padding: 8px 12px;
    border-bottom: 1px solid color-mix(in srgb, var(--terminal-shell-border, #29383a) 78%, var(--terminal-shell-text, #d8dddc));
    background: var(--terminal-shell-panel, #111a1d);
  }

  .diagnostic-mobile-selector button {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 8px;
    border: 0;
    padding: 7px 8px;
    color: var(--terminal-shell-text, #d8dddc);
    background: transparent;
    font-weight: 700;
    cursor: pointer;
  }

  .diagnostic-mobile-selector button span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .diagnostic-mobile-selector small {
    flex: 0 0 auto;
    color: var(--terminal-shell-muted, #8a9695);
  }

  .diagnostic-log,
  .diagnostic-interactive-terminal :deep(.interactive-terminal__screen) {
    min-height: min(400px, 48dvh);
  }

  .diagnostic-terminal-bar,
  .diagnostic-result footer {
    padding-right: 14px;
    padding-left: 14px;
  }

}

@media (prefers-reduced-motion: reduce) {
  .diagnostic-command-panel {
    transition: none;
  }
}
</style>
