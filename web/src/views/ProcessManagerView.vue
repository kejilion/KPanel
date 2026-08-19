<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ProcessManagerView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ProcessManagerView/zh-TW').then((module) => module.default))
import {
  Activity,
  ArrowDown,
  ArrowUp,
  CircleStop,
  Cpu,
  Gauge,
  ListTree,
  MemoryStick,
  Pause,
  Play,
  RefreshCw,
  Search,
  Skull,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { ApiError, api } from '@/lib/api'
import { formatBytes, formatDateTime, formatPercent } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type { ProcessMetric, ProcessOrder, ProcessSnapshot, ProcessSort } from '@/types/api'

const processLimit = 200
const refreshIntervalMilliseconds = 2_000
const searchDebounceMilliseconds = 250
const labels = {
  title: '进程管理器',
  paused: '已暂停',
  resume: '继续',
  pause: '暂停',
  refresh: '刷新',
  process: '进程',
  active: '活跃',
  sampling: '正在采样',
  live: '实时',
  name: '名称',
  force: '强制结束',
  forceTitle: '强制结束进程',
  sending: '正在发送…',
  confirm: '确认结束',
} as const

const snapshot = shallowRef<ProcessSnapshot>()
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const search = ref('')
const sort = ref<ProcessSort>('cpu')
const order = ref<ProcessOrder>('desc')
const paused = ref(false)
const documentVisible = ref(typeof document === 'undefined' || document.visibilityState === 'visible')
const selectedKey = ref('')
const pendingProcess = ref<ProcessMetric>()
const pendingSignal = ref<'term' | 'kill'>('term')
const actionRunning = ref(false)
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
const toast = useToast()
let controller: AbortController | undefined
let refreshTimer: number | undefined
let searchTimer: number | undefined
let requestSequence = 0

const canPoll = computed(() => !paused.value && documentVisible.value && desktopWindowActive.value)
const items = computed(() => snapshot.value?.items || [])
const selectedProcess = computed(() => items.value.find((item) => processKey(item) === selectedKey.value))
const memoryPercent = computed(() => {
  const summary = snapshot.value?.summary
  if (!summary?.memoryTotalBytes) return 0
  return summary.memoryUsedBytes / summary.memoryTotalBytes * 100
})
const resultDescription = computed(() => {
  if (!snapshot.value) return ''
  const shown = items.value.length
  const matched = snapshot.value.total
  return search.value.trim()
    ? `匹配 ${matched} 个，显示 ${shown} 个`
    : `显示 ${shown} 个，共观测 ${snapshot.value.summary.total} 个`
})

function processKey(process: ProcessMetric): string {
  return `${process.pid}:${process.startTimeTicks}`
}

function clearRefreshTimer(): void {
  if (refreshTimer !== undefined) window.clearTimeout(refreshTimer)
  refreshTimer = undefined
}

function scheduleRefresh(): void {
  clearRefreshTimer()
  if (!canPoll.value) return
  refreshTimer = window.setTimeout(() => void load(true), refreshIntervalMilliseconds)
}

function readableError(reason: unknown): string {
  if (reason instanceof ApiError) {
    const messages: Partial<Record<string, string>> = {
      process_metrics_busy: '进程采样正忙，请稍后重试。',
      process_metrics_unavailable: '宿主机进程状态暂时不可用。',
      invalid_process_query: '进程查询条件无效。',
    }
    return messages[reason.code] || reason.message
  }
  return reason instanceof Error ? reason.message : '无法读取进程状态。'
}

async function load(silent = false): Promise<void> {
  clearRefreshTimer()
  controller?.abort()
  const currentController = new AbortController()
  controller = currentController
  const sequence = ++requestSequence
  if (!silent || !snapshot.value) loading.value = true
  else refreshing.value = true
  try {
    const result = await api.system.processes({
      search: search.value.trim() || undefined,
      sort: sort.value,
      order: order.value,
      limit: processLimit,
    }, currentController.signal)
    if (sequence !== requestSequence) return
    snapshot.value = result
    error.value = ''
    if (selectedKey.value && !result.items.some((item) => processKey(item) === selectedKey.value)) {
      selectedKey.value = ''
    }
  } catch (reason) {
    if (currentController.signal.aborted || sequence !== requestSequence) return
    error.value = readableError(reason)
  } finally {
    if (sequence !== requestSequence) return
    loading.value = false
    refreshing.value = false
    controller = undefined
    scheduleRefresh()
  }
}

function setSort(next: ProcessSort): void {
  if (sort.value === next) order.value = order.value === 'desc' ? 'asc' : 'desc'
  else {
    sort.value = next
    order.value = next === 'name' || next === 'user' || next === 'state' ? 'asc' : 'desc'
  }
}

function sortLabel(next: ProcessSort): string {
  if (sort.value !== next) return ''
  return order.value === 'desc' ? '，降序' : '，升序'
}

function sortAriaLabel(label: string, next: ProcessSort): string {
  return `按${label}排序${sortLabel(next)}`
}

function stateLabel(state: string): string {
  const labels: Record<string, string> = {
    R: '运行', S: '休眠', D: '等待', Z: '僵尸', T: '停止', t: '跟踪', I: '空闲', X: '结束',
  }
  return labels[state] || state || '未知'
}

function scannedLabel(total: number): string {
  return `本次扫描 ${total} 个`
}

function activitySummaryLabel(sleeping: number, zombie: number): string {
  return `${sleeping} 个休眠 · ${zombie} 个僵尸`
}

function stateTone(state: string): string {
  if (state === 'R') return 'running'
  if (state === 'Z' || state === 'X') return 'danger'
  if (state === 'T' || state === 't') return 'warning'
  return 'idle'
}

function percentWidth(value: number): string {
  return `${Math.max(0, Math.min(100, value))}%`
}

function selectProcess(process: ProcessMetric): void {
  selectedKey.value = processKey(process)
}

function askSignal(process: ProcessMetric, signal: 'term' | 'kill'): void {
  pendingProcess.value = process
  pendingSignal.value = signal
}

async function sendSignal(): Promise<void> {
  const process = pendingProcess.value
  if (!process || actionRunning.value) return
  actionRunning.value = true
  try {
    const result = await api.system.action({
      action: 'process-signal',
      pid: process.pid,
      startTimeTicks: process.startTimeTicks,
      signal: pendingSignal.value,
    })
    toast.success('信号已发送', result.message)
    pendingProcess.value = undefined
    await load(true)
  } catch (reason) {
    toast.danger('结束进程失败', readableError(reason))
  } finally {
    actionRunning.value = false
  }
}

function togglePause(): void {
  paused.value = !paused.value
}

function onVisibilityChange(): void {
  documentVisible.value = document.visibilityState === 'visible'
}

watch([sort, order], () => void load(true))
watch(search, () => {
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => void load(true), searchDebounceMilliseconds)
})
watch(canPoll, (active) => {
  clearRefreshTimer()
  if (active) void load(true)
  else controller?.abort()
})

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibilityChange)
  void load()
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  clearRefreshTimer()
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  controller?.abort()
})
</script>

<template>
  <div class="page process-page">
    <PageHeader
      :title="labels.title"
      description="实时查看服务器进程的 CPU 与内存占用，并按实际状态管理进程。"
    />

    <LoadingState v-if="loading && !snapshot" :rows="5" cards />
    <ErrorState v-else-if="error && !snapshot" :message="error" @retry="load()" />

    <template v-else-if="snapshot">
      <div v-if="error" class="inline-alert inline-alert--warning" role="status">
        自动刷新暂时失败，正在显示上一次观测结果。
      </div>

      <section class="process-summary" aria-label="资源概要">
        <article class="process-summary__card process-summary__card--cpu">
          <span><Cpu :size="17" /> CPU</span>
          <strong>{{ formatPercent(snapshot.summary.cpuPercent) }}</strong>
          <div class="process-meter"><i :style="{ width: percentWidth(snapshot.summary.cpuPercent) }" /></div>
        </article>
        <article class="process-summary__card process-summary__card--memory">
          <span><MemoryStick :size="17" /> 内存</span>
          <strong>{{ formatPercent(memoryPercent) }}</strong>
          <small>{{ formatBytes(snapshot.summary.memoryUsedBytes) }} / {{ formatBytes(snapshot.summary.memoryTotalBytes) }}</small>
          <div class="process-meter"><i :style="{ width: percentWidth(memoryPercent) }" /></div>
        </article>
        <article class="process-summary__card">
          <span><ListTree :size="17" /> {{ labels.process }}</span>
          <strong>{{ snapshot.summary.total }}</strong>
          <small>{{ scannedLabel(snapshot.scanned) }}</small>
        </article>
        <article class="process-summary__card">
          <span><Activity :size="17" /> {{ labels.active }}</span>
          <strong>{{ snapshot.summary.running }}</strong>
          <small>{{ activitySummaryLabel(snapshot.summary.sleeping, snapshot.summary.zombie) }}</small>
        </article>
      </section>

      <section class="process-workspace">
        <header class="process-toolbar">
          <label class="process-search">
            <Search :size="17" />
            <input v-model="search" type="search" maxlength="128" placeholder="搜索名称、用户或 PID" aria-label="搜索进程" />
          </label>
          <div class="process-toolbar__controls">
            <div class="process-toolbar__status">
              <span v-if="refreshing" class="process-live"><i /> {{ labels.sampling }}</span>
              <span v-else-if="!paused" class="process-live"><i /> {{ labels.live }}</span>
              <span>{{ resultDescription }}</span>
              <span v-if="snapshot" class="process-observed-at">{{ paused ? labels.paused : formatDateTime(snapshot.collectedAt) }}</span>
            </div>
            <button class="icon-button" type="button" :title="paused ? labels.resume : labels.pause" :aria-label="paused ? labels.resume : labels.pause" @click="togglePause">
              <Play v-if="paused" :size="16" />
              <Pause v-else :size="16" />
            </button>
            <button class="icon-button" type="button" :disabled="refreshing" :title="labels.refresh" :aria-label="labels.refresh" @click="load(true)">
              <RefreshCw :size="16" :class="{ spin: refreshing }" />
            </button>
          </div>
        </header>

        <div class="process-layout" :class="{ 'process-layout--details': selectedProcess }">
          <div class="process-table-wrap">
            <table v-if="items.length" class="process-table">
              <thead>
                <tr>
                  <th><button type="button" :aria-label="sortAriaLabel(labels.name, 'name')" @click="setSort('name')">{{ labels.name }} <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'name'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('PID', 'pid')" @click="setSort('pid')">PID <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'pid'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('用户', 'user')" @click="setSort('user')">用户 <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'user'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('CPU', 'cpu')" @click="setSort('cpu')">CPU <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'cpu'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('内存', 'memory')" @click="setSort('memory')">内存 <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'memory'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('线程', 'threads')" @click="setSort('threads')">线程 <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'threads'" :size="13" /></button></th>
                  <th><button type="button" :aria-label="sortAriaLabel('状态', 'state')" @click="setSort('state')">状态 <component :is="order === 'desc' ? ArrowDown : ArrowUp" v-if="sort === 'state'" :size="13" /></button></th>
                  <th><span class="sr-only">操作</span></th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="process in items"
                  :key="processKey(process)"
                  :class="{ 'is-selected': processKey(process) === selectedKey }"
                  tabindex="0"
                  @click="selectProcess(process)"
                  @keydown.enter="selectProcess(process)"
                >
                  <td><strong>{{ process.name }}</strong><small>PPID {{ process.parentPid }}</small></td>
                  <td class="process-table__mono">{{ process.pid }}</td>
                  <td>{{ process.user || process.userId }}</td>
                  <td class="process-table__metric"><strong>{{ formatPercent(process.cpuPercent) }}</strong><i><span :style="{ width: percentWidth(process.cpuPercent) }" /></i></td>
                  <td class="process-table__metric"><strong>{{ formatBytes(process.memoryBytes) }}</strong><i><span :style="{ width: percentWidth(snapshot.summary.memoryTotalBytes ? process.memoryBytes / snapshot.summary.memoryTotalBytes * 100 : 0) }" /></i></td>
                  <td>{{ process.threads }}</td>
                  <td><span class="process-state" :class="`process-state--${stateTone(process.state)}`"><i />{{ stateLabel(process.state) }}</span></td>
                  <td><button class="icon-button icon-button--danger" type="button" title="结束进程" aria-label="结束进程" @click.stop="askSignal(process, 'term')"><CircleStop :size="16" /></button></td>
                </tr>
              </tbody>
            </table>
            <EmptyState v-else title="没有匹配的进程" message="请调整搜索条件后重试。" />
          </div>

          <aside v-if="selectedProcess" class="process-details" aria-label="进程详情">
            <header>
              <span class="process-details__icon"><Gauge :size="19" /></span>
              <div><strong>{{ selectedProcess.name }}</strong><small>PID {{ selectedProcess.pid }}</small></div>
              <button class="icon-button" type="button" aria-label="关闭详情" @click="selectedKey = ''">×</button>
            </header>
            <dl>
              <div><dt>CPU</dt><dd>{{ formatPercent(selectedProcess.cpuPercent) }}</dd></div>
              <div><dt>内存</dt><dd>{{ formatBytes(selectedProcess.memoryBytes) }}</dd></div>
              <div><dt>用户</dt><dd>{{ selectedProcess.user || selectedProcess.userId }}</dd></div>
              <div><dt>状态</dt><dd>{{ stateLabel(selectedProcess.state) }}</dd></div>
              <div><dt>父进程 PID</dt><dd>{{ selectedProcess.parentPid }}</dd></div>
              <div><dt>线程</dt><dd>{{ selectedProcess.threads }}</dd></div>
              <div><dt>Nice</dt><dd>{{ selectedProcess.nice }}</dd></div>
            </dl>
            <div class="process-details__actions">
              <button class="button button--danger" type="button" @click="askSignal(selectedProcess, 'term')">
                <CircleStop :size="16" /> 结束进程
              </button>
              <button class="button button--danger-text" type="button" @click="askSignal(selectedProcess, 'kill')">
                <Skull :size="16" /> {{ labels.force }}
              </button>
            </div>
            <p>优先使用正常结束；仅在进程无响应时强制结束。最终结果以内核返回和重新采样为准。</p>
          </aside>
        </div>
      </section>
    </template>

    <ModalDialog
      :open="Boolean(pendingProcess)"
      :title="pendingSignal === 'kill' ? labels.forceTitle : '结束进程'"
      :description="pendingProcess ? `${pendingProcess.name} · PID ${pendingProcess.pid}` : ''"
      size="small"
      @close="!actionRunning && (pendingProcess = undefined)"
    >
      <div class="process-confirm">
        <span :class="pendingSignal === 'kill' ? 'process-confirm__danger' : ''"><Skull v-if="pendingSignal === 'kill'" :size="22" /><CircleStop v-else :size="22" /></span>
        <p v-if="pendingSignal === 'kill'">SIGKILL 不允许进程清理资源，可能造成未保存数据丢失。</p>
        <p v-else>将发送 SIGTERM，让进程有机会保存状态并正常退出。</p>
      </div>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="actionRunning" @click="pendingProcess = undefined">取消</button>
        <button class="button button--danger" type="button" :disabled="actionRunning" @click="sendSignal">
          {{ actionRunning ? labels.sending : labels.confirm }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.process-page { display: grid; gap: 18px; }
.process-observed-at { color: var(--muted); font-size: 12px; font-variant-numeric: tabular-nums; }
.process-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.process-summary__card { position: relative; min-width: 0; overflow: hidden; padding: 16px 17px; border: 1px solid var(--line); border-radius: 16px; background: var(--surface); box-shadow: var(--shadow-soft); }
.process-summary__card::after { position: absolute; right: -18px; bottom: -24px; width: 78px; height: 78px; border-radius: 50%; background: color-mix(in srgb, var(--brand) 8%, transparent); content: ''; }
.process-summary__card > span { display: flex; align-items: center; gap: 7px; color: var(--muted); font-size: 12px; font-weight: 750; }
.process-summary__card > strong { display: block; margin-top: 9px; color: var(--text); font-size: 25px; line-height: 1; font-variant-numeric: tabular-nums; }
.process-summary__card > small { display: block; overflow: hidden; margin-top: 7px; color: var(--muted); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.process-meter { height: 3px; overflow: hidden; margin-top: 12px; border-radius: 999px; background: var(--line-soft); }
.process-meter i { display: block; height: 100%; border-radius: inherit; background: var(--brand); transition: width 240ms ease; }
.process-summary__card--memory .process-meter i { background: var(--blue); }
.process-workspace { overflow: hidden; border: 1px solid var(--line); border-radius: 18px; background: var(--surface); box-shadow: var(--shadow-soft); }
.process-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 13px 15px; border-bottom: 1px solid var(--line); }
.process-search { display: flex; width: min(420px, 100%); align-items: center; gap: 9px; padding: 0 12px; border: 1px solid var(--line); border-radius: 11px; color: var(--muted); background: var(--surface-muted); transition: border-color 160ms ease, box-shadow 160ms ease; }
.process-search:focus-within { border-color: color-mix(in srgb, var(--brand) 55%, var(--line)); box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 10%, transparent); }
.process-search input { width: 100%; height: 38px; border: 0; outline: 0; color: var(--text); background: transparent; font: inherit; font-size: 13px; }
.process-toolbar__status { display: flex; align-items: center; gap: 12px; color: var(--muted); font-size: 11px; white-space: nowrap; }
.process-toolbar__controls { display: flex; min-width: 0; align-items: center; gap: 7px; }
.process-live { display: inline-flex; align-items: center; gap: 6px; color: var(--success); font-weight: 750; }
.process-live i, .process-state i { width: 7px; height: 7px; border-radius: 50%; background: currentColor; box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 12%, transparent); }
.process-layout { display: grid; min-height: 420px; grid-template-columns: minmax(0, 1fr); }
.process-layout--details { grid-template-columns: minmax(0, 1fr) 250px; }
.process-table-wrap { min-width: 0; overflow: auto; }
.process-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.process-table th { position: sticky; z-index: 2; top: 0; padding: 0; border-bottom: 1px solid var(--line); background: var(--surface-muted); text-align: left; }
.process-table th button, .process-table th > span { display: inline-flex; min-height: 38px; align-items: center; gap: 4px; padding: 0 11px; border: 0; color: var(--muted); background: transparent; font: inherit; font-size: 10px; font-weight: 800; letter-spacing: .04em; white-space: nowrap; }
.process-table th button { cursor: pointer; }
.process-table th button:hover { color: var(--text); }
.process-table td { height: 49px; padding: 6px 11px; border-bottom: 1px solid var(--line-soft); color: var(--muted); white-space: nowrap; }
.process-table tbody tr { cursor: pointer; outline: none; transition: background 120ms ease; }
.process-table tbody tr:hover, .process-table tbody tr:focus-visible, .process-table tbody tr.is-selected { background: color-mix(in srgb, var(--brand) 6%, transparent); }
.process-table td:first-child { min-width: 170px; }
.process-table td:first-child strong, .process-table td:first-child small { display: block; max-width: 220px; overflow: hidden; text-overflow: ellipsis; }
.process-table td:first-child strong { color: var(--text); font-size: 12px; }
.process-table td:first-child small { margin-top: 3px; font-size: 10px; }
.process-table__mono { color: var(--text) !important; font-family: var(--font-mono); font-variant-numeric: tabular-nums; }
.process-table__metric { min-width: 92px; font-variant-numeric: tabular-nums; }
.process-table__metric strong { color: var(--text); font-size: 11px; }
.process-table__metric > i { display: block; width: 68px; height: 2px; overflow: hidden; margin-top: 5px; border-radius: 99px; background: var(--line-soft); }
.process-table__metric > i span { display: block; height: 100%; background: var(--brand); }
.process-state { display: inline-flex; align-items: center; gap: 7px; font-weight: 700; }
.process-state--running { color: var(--success); }
.process-state--danger { color: var(--danger); }
.process-state--warning { color: var(--warning); }
.process-state--idle { color: var(--muted); }
.process-details { display: flex; flex-direction: column; gap: 16px; padding: 16px; border-left: 1px solid var(--line); background: color-mix(in srgb, var(--surface-muted) 55%, var(--surface)); }
.process-details header { display: grid; align-items: center; grid-template-columns: 38px minmax(0, 1fr) auto; gap: 9px; }
.process-details header strong, .process-details header small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.process-details header small { margin-top: 3px; color: var(--muted); font-size: 10px; }
.process-details__icon { display: grid; width: 38px; height: 38px; place-items: center; border-radius: 11px; color: var(--brand); background: color-mix(in srgb, var(--brand) 10%, transparent); }
.process-details dl { display: grid; gap: 1px; margin: 0; overflow: hidden; border: 1px solid var(--line); border-radius: 12px; background: var(--line); }
.process-details dl div { display: flex; justify-content: space-between; gap: 12px; padding: 9px 10px; background: var(--surface); }
.process-details dt { color: var(--muted); font-size: 10px; }
.process-details dd { overflow: hidden; margin: 0; color: var(--text); font-size: 11px; font-weight: 750; text-overflow: ellipsis; white-space: nowrap; }
.process-details__actions { display: grid; gap: 8px; }
.process-details > p { margin: 0; color: var(--muted); font-size: 10px; line-height: 1.6; }
.process-confirm { display: flex; align-items: flex-start; gap: 12px; }
.process-confirm > span { display: grid; flex: 0 0 42px; width: 42px; height: 42px; place-items: center; border-radius: 12px; color: var(--warning); background: color-mix(in srgb, var(--warning) 12%, transparent); }
.process-confirm > span.process-confirm__danger { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
.process-confirm p { margin: 3px 0 0; color: var(--muted); font-size: 12px; line-height: 1.7; }
@media (max-width: 1050px) { .process-layout--details { grid-template-columns: minmax(0, 1fr); } .process-details { border-top: 1px solid var(--line); border-left: 0; } }
@media (max-width: 760px) {
  .process-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .process-toolbar { align-items: stretch; flex-direction: column; }
  .process-toolbar__controls { justify-content: space-between; }
  .process-toolbar__status { min-width: 0; flex: 1; justify-content: space-between; overflow-x: auto; }
  .process-table th:nth-child(3), .process-table td:nth-child(3),
  .process-table th:nth-child(6), .process-table td:nth-child(6),
  .process-table th:nth-child(7), .process-table td:nth-child(7) { display: none; }
  .process-table th button, .process-table th > span, .process-table td { padding-right: 7px; padding-left: 7px; }
  .process-table td:first-child { min-width: 116px; }
  .process-table td:first-child strong, .process-table td:first-child small { max-width: 128px; }
  .process-table__metric { min-width: 70px; }
  .process-table__metric > i { width: 48px; }
}
@media (max-width: 480px) {
  .process-page { gap: 12px; }
  .process-observed-at { display: none; }
  .process-summary { gap: 8px; }
  .process-summary__card { min-height: 112px; padding: 12px; border-radius: 14px; }
  .process-summary__card > strong { margin-top: 7px; font-size: 22px; }
  .process-summary__card > small { margin-top: 5px; }
  .process-meter { margin-top: 9px; }
  .process-workspace { border-radius: 14px; }
  .process-toolbar { gap: 9px; padding: 11px; }
  .process-toolbar__controls { flex-wrap: wrap; gap: 6px; }
  .process-toolbar__status { width: 100%; order: 2; }
  .process-layout { min-height: 360px; }
  .process-table th button, .process-table th > span { min-height: 40px; }
}
@media (prefers-reduced-motion: reduce) { .process-meter i { transition: none; } }
</style>
