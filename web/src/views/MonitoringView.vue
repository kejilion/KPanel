<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/MonitoringView/en-US').then((module) => module.default)
  : import('@/i18n/pages/MonitoringView/zh-TW').then((module) => module.default))
import { ArrowLeft, Box, Cpu, Database, HardDrive, MemoryStick, Network, RadioTower, RefreshCw, RotateCcw, Search } from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import TrendChart, { type TrendSeries } from '@/components/monitoring/TrendChart.vue'
import { ApiError, api } from '@/lib/api'
import { formatBytes, formatDateTime, formatPercent, formatRate } from '@/lib/format'
import {
  monitoringTargetId,
  normalizeMonitoringMetric,
  type MonitoringMetric,
} from '@/lib/monitoringNavigation'
import {
  isHistoricalContainer,
  monitoringRangeFromQuery,
  monitoringWindowFromQuery,
  newestContainerSampleTime,
  sliceMonitoringHistory,
} from '@/lib/monitoringPresentation'
import {
  assignMonitoringContainerColorSlots,
  monitoringContainerColors,
  monitoringContainerSelectionLimit,
  readMonitoringContainerPreference,
  reconcileMonitoringContainerIDs,
  writeMonitoringContainerPreference,
} from '@/lib/monitoringContainerSelection'
import {
  latestOperatorLatency,
  mergeOperatorLatencyVisibility,
  operatorLatencyColors,
} from '@/lib/operatorLatencyPresentation'
import type {
  MonitoringContainerSeries,
  MonitoringHistory,
  MonitoringHistoryQuery,
  MonitoringHostPoint,
  MonitoringOperatorLatencySeries,
  MonitoringRange,
} from '@/types/api'

const ranges: Array<{ value: MonitoringRange; label: string }> = [
  { value: '1h', label: '1 小时' },
  { value: '6h', label: '6 小时' },
  { value: '24h', label: '24 小时' },
  { value: '7d', label: '7 天' },
  { value: '30d', label: '30 天' },
  { value: '3m', label: '3 个月' },
  { value: '6m', label: '6 个月' },
  { value: '12m', label: '12 个月' },
]
const operatorLabels: Record<MonitoringOperatorLatencySeries['operator'], string> = {
  telecom: '电信', unicom: '联通', mobile: '移动',
}
const regionLabels: Record<MonitoringOperatorLatencySeries['region'], string> = {
  beijing: '北京', shanghai: '上海', guangzhou: '广州',
}

const history = shallowRef<MonitoringHistory>()
const route = useRoute()
const router = useRouter()
const selectedRange = ref<MonitoringRange>('6h')
const restoredContainerPreference = readMonitoringContainerPreference()
const selectedContainerIds = ref<string[]>(restoredContainerPreference?.ids || [])
const selectedContainerColorSlots = ref<Record<string, number>>(restoredContainerPreference?.slots || {})
const containerSearch = ref('')
const containerSelectionError = ref('')
const highlightedContainerId = ref('')
const containerCPUMode = ref<'peak' | 'average'>('peak')
const containerMemoryMode = ref<'bytes' | 'percent'>('bytes')
const containerNetworkMode = ref<'total' | 'direction'>('total')
const containerBlockMode = ref<'total' | 'direction'>('total')
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const diskChartMode = ref<'capacity' | 'io'>('capacity')
const networkChartMode = ref<'traffic' | 'connections'>('traffic')
const operatorLatencyVisibility = ref<Record<string, boolean>>({})
const activeWindow = ref<MonitoringHistoryQuery>()
const rootHistory = shallowRef<MonitoringHistory>()
const updating = ref(false)
const interactionError = ref('')
let controller: AbortController | undefined

const latestHost = computed<MonitoringHostPoint | undefined>(() => history.value?.host.at(-1))
const historyStorageBytes = computed(() =>
  (history.value?.storage.storageBytes || 0) + (history.value?.storage.rollupStorageBytes || 0),
)
const historyStorageLimit = computed(() =>
  (history.value?.storage.maxStorageBytes || 0) + (history.value?.storage.maxRollupStorageBytes || 0),
)
const newestContainerSample = computed(() => newestContainerSampleTime(history.value?.containers || []))
const containerCatalog = computed<MonitoringContainerSeries[]>(() => {
  const current = history.value?.containers || []
  const root = rootHistory.value?.range === selectedRange.value ? rootHistory.value.containers : []
  const result = current.map((container) => container)
  const known = new Set(result.map((container) => container.containerId))
  for (const container of root) {
    if (!known.has(container.containerId)) result.push({ ...container, points: [] })
  }
  return result
})
const selectedContainers = computed(() => {
  const byID = new Map(containerCatalog.value.map((container) => [container.containerId, container]))
  return selectedContainerIds.value.flatMap((id) => {
    const container = byID.get(id)
    return container ? [container] : []
  })
})
const filteredContainers = computed(() => {
  const query = containerSearch.value.trim().toLocaleLowerCase()
  if (!query) return containerCatalog.value
  return containerCatalog.value.filter((container) =>
    container.name.toLocaleLowerCase().includes(query) ||
    container.image.toLocaleLowerCase().includes(query) ||
    container.containerId.toLocaleLowerCase().includes(query))
})
const duplicateContainerNames = computed(() => {
  const counts = new Map<string, number>()
  for (const container of containerCatalog.value) counts.set(container.name, (counts.get(container.name) || 0) + 1)
  return new Set(Array.from(counts).filter(([, count]) => count > 1).map(([name]) => name))
})
const containerHasAverage = computed(() => selectedContainers.value.some((container) =>
  container.points.some((point) => (point.cpuSampleCount || 0) > 0)))
const containerIntervalMilliseconds = computed(() => Math.max(
  history.value?.bucketSeconds || 1,
  history.value?.storage.containerIntervalSeconds || 1,
) * 1_000)
const selectedMetric = computed<MonitoringMetric | undefined>(() => normalizeMonitoringMetric(route.query.metric))
const zoomWindowLabel = computed(() => activeWindow.value
  ? `${formatWindowTime(activeWindow.value.start)} – ${formatWindowTime(activeWindow.value.end)}`
  : '')

const hostCPU = computed<TrendSeries[]>(() => {
  const points = history.value?.host || []
  const hasAverage = points.some((point) => (point.cpuSampleCount || 0) > 0)
  const series: TrendSeries[] = [{
    label: hasAverage ? 'CPU 峰值' : 'CPU',
    color: 'var(--brand)',
    points: points.map((point) => ({ at: point.collectedAt, value: point.cpuPercent })),
  }]
  if (hasAverage) {
    series.push({
      label: 'CPU 平均',
      color: 'var(--blue)',
      points: points.flatMap((point) => (point.cpuSampleCount || 0) > 0
        ? [{ at: point.collectedAt, value: point.cpuAveragePercent || 0 }]
        : []),
    })
  }
  series.push({
    label: '1 分钟负载占核',
    color: 'var(--violet)',
    points: points.map((point) => ({
      at: point.collectedAt,
      value: point.cpuCores > 0 ? (point.loadOne / point.cpuCores) * 100 : 0,
    })),
  })
  return series
})
const hostMemory = computed<TrendSeries[]>(() => {
  const series: TrendSeries[] = [{
    label: '内存',
    color: 'var(--blue)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: percent(point.memoryUsedBytes, point.memoryTotalBytes),
    })),
  }]
  if ((history.value?.host || []).some((point) => point.swapTotalBytes > 0)) {
    series.push({
      label: 'Swap',
      color: 'var(--violet)',
      points: (history.value?.host || []).map((point) => ({
        at: point.collectedAt,
        value: percent(point.swapUsedBytes, point.swapTotalBytes),
      })),
    })
  }
  return series
})
const hostDiskCapacity = computed<TrendSeries[]>(() => [{
  label: '系统盘使用率',
  color: 'var(--amber)',
  points: (history.value?.host || []).map((point) => ({ at: point.collectedAt, value: point.diskPercent })),
}])
const hostDiskIO = computed<TrendSeries[]>(() => [
  {
    label: '读取',
    color: 'var(--blue)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.diskReadBytesPerSecond || 0,
    })),
  },
  {
    label: '写入',
    color: 'var(--amber)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.diskWriteBytesPerSecond || 0,
    })),
  },
])
const activeHostDisk = computed(() => diskChartMode.value === 'io' ? hostDiskIO.value : hostDiskCapacity.value)
const hostNetworkTraffic = computed<TrendSeries[]>(() => [
  {
    label: '下载',
    color: 'var(--brand)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.networkRxBytesPerSecond,
    })),
  },
  {
    label: '上传',
    color: 'var(--blue)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.networkTxBytesPerSecond,
    })),
  },
])
const hostNetworkConnections = computed<TrendSeries[]>(() => [
  {
    label: 'TCP',
    color: 'var(--brand)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.tcpConnections,
    })),
  },
  {
    label: 'UDP',
    color: 'var(--violet)',
    points: (history.value?.host || []).map((point) => ({
      at: point.collectedAt,
      value: point.udpConnections,
    })),
  },
])
const activeHostNetwork = computed(() => networkChartMode.value === 'traffic'
  ? hostNetworkTraffic.value
  : hostNetworkConnections.value)
const operatorLatencyRoutes = computed<MonitoringOperatorLatencySeries[]>(() => history.value?.operatorLatency || [])
const operatorLatencyChart = computed<TrendSeries[]>(() => operatorLatencyRoutes.value
  .filter((series) => operatorLatencyVisibility.value[series.id])
  .map((series) => ({
    label: operatorLatencyLabel(series),
    color: operatorLatencyColors[series.id] || 'var(--brand)',
    points: series.points.flatMap((point) => point.latencyMilliseconds === null
      ? []
      : [{ at: point.collectedAt, value: point.latencyMilliseconds }]),
  }))
  .filter((series) => series.points.length > 0))
const operatorLatencyVisibleCount = computed(() => operatorLatencyRoutes.value
  .filter((series) => operatorLatencyVisibility.value[series.id]).length)
const containerCPU = computed<TrendSeries[]>(() => selectedContainers.value.flatMap((container) => {
  const average = containerCPUMode.value === 'average'
  const points = average
    ? container.points.flatMap((point) => (point.cpuSampleCount || 0) > 0
      ? [{ at: point.collectedAt, value: point.cpuAveragePercent || 0 }]
      : [])
    : container.points.map((point) => ({ at: point.collectedAt, value: point.cpuPercent }))
  return points.length ? [containerTrendSeries(container, `cpu-${containerCPUMode.value}`, average ? 'CPU 平均' : 'CPU 峰值', points)] : []
}))
const containerMemory = computed<TrendSeries[]>(() => selectedContainers.value.flatMap((container) => {
  const percentage = containerMemoryMode.value === 'percent'
  const points = container.points.map((point) => ({
    at: point.collectedAt,
    value: percentage ? point.memoryPercent : point.memoryBytes,
  }))
  return points.length ? [containerTrendSeries(container, `memory-${containerMemoryMode.value}`, percentage ? '限额占比' : '实际用量', points)] : []
}))
const containerNetwork = computed<TrendSeries[]>(() => selectedContainers.value.flatMap((container) => {
  if (containerNetworkMode.value === 'total') {
    const points = container.points.map((point) => ({
      at: point.collectedAt,
      value: point.networkRxBytesPerSecond + point.networkTxBytesPerSecond,
    }))
    return points.length ? [containerTrendSeries(container, 'network-total', '总吞吐', points)] : []
  }
  return [
    containerTrendSeries(container, 'network-rx', '接收', container.points.map((point) => ({
      at: point.collectedAt, value: point.networkRxBytesPerSecond,
    }))),
    containerTrendSeries(container, 'network-tx', '发送', container.points.map((point) => ({
      at: point.collectedAt, value: point.networkTxBytesPerSecond,
    })), 'dashed'),
  ].filter((series) => series.points.length)
}))
const containerBlock = computed<TrendSeries[]>(() => selectedContainers.value.flatMap((container) => {
  if (containerBlockMode.value === 'total') {
    const points = container.points.map((point) => ({
      at: point.collectedAt,
      value: (point.blockReadBytesPerSecond || 0) + (point.blockWriteBytesPerSecond || 0),
    }))
    return points.length ? [containerTrendSeries(container, 'block-total', '总 I/O', points)] : []
  }
  return [
    containerTrendSeries(container, 'block-read', '读取', container.points.map((point) => ({
      at: point.collectedAt, value: point.blockReadBytesPerSecond || 0,
    }))),
    containerTrendSeries(container, 'block-write', '写入', container.points.map((point) => ({
      at: point.collectedAt, value: point.blockWriteBytesPerSecond || 0,
    })), 'dashed'),
  ].filter((series) => series.points.length)
}))

function containerColor(containerId: string): string {
  const slot = selectedContainerColorSlots.value[containerId] ?? 0
  return monitoringContainerColors[slot] || monitoringContainerColors[0]
}

function containerSeriesName(container: MonitoringContainerSeries): string {
  return duplicateContainerNames.value.has(container.name)
    ? `${container.name} · ${container.containerId.slice(0, 8)}`
    : container.name
}

function containerTrendSeries(
  container: MonitoringContainerSeries,
  metric: string,
  metricLabel: string,
  points: Array<{ at: string; value: number }>,
  dash: 'solid' | 'dashed' = 'solid',
): TrendSeries {
  return {
    id: `${container.containerId}:${metric}`,
    group: container.containerId,
    label: `${containerSeriesName(container)} · ${metricLabel}`,
    color: containerColor(container.containerId),
    dash,
    maxGapMilliseconds: containerIntervalMilliseconds.value * 1.75,
    maxPointDistanceMilliseconds: containerIntervalMilliseconds.value * 0.75,
    points,
  }
}

function percent(used?: number, total?: number): number {
  if (!used || !total) return 0
  return Math.min(100, Math.max(0, (used / total) * 100))
}

function persistContainerSelection(ids: string[]): void {
  selectedContainerColorSlots.value = assignMonitoringContainerColorSlots(
    ids,
    selectedContainerColorSlots.value,
  )
  selectedContainerIds.value = ids
  writeMonitoringContainerPreference({ ids, slots: selectedContainerColorSlots.value })
}

function reconcileContainerSelection(containers: MonitoringContainerSeries[]): void {
  const ids = reconcileMonitoringContainerIDs(containers, selectedContainerIds.value)
  if (ids.length) persistContainerSelection(ids)
}

function restoreContainerCheckbox(event: Event | undefined, checked: boolean): void {
  if (event?.target instanceof HTMLInputElement) event.target.checked = checked
}

function toggleContainer(container: MonitoringContainerSeries, event?: Event): void {
  containerSelectionError.value = ''
  const selected = selectedContainerIds.value.includes(container.containerId)
  if (selected) {
    if (selectedContainerIds.value.length <= 1) {
      containerSelectionError.value = '至少保留一个容器用于对比。'
      restoreContainerCheckbox(event, true)
      return
    }
    persistContainerSelection(selectedContainerIds.value.filter((id) => id !== container.containerId))
    return
  }
  if (selectedContainerIds.value.length >= monitoringContainerSelectionLimit) {
    containerSelectionError.value = `最多同时比较 ${monitoringContainerSelectionLimit} 个容器，请先取消一个。`
    restoreContainerCheckbox(event, false)
    return
  }
  persistContainerSelection([...selectedContainerIds.value, container.containerId])
}

function containerSelected(containerId: string): boolean {
  return selectedContainerIds.value.includes(containerId)
}

function latestContainerPoint(container: MonitoringContainerSeries) {
  return container.points.at(-1)
}

function containerIsHistorical(container: MonitoringContainerSeries): boolean {
  return isHistoricalContainer(container, newestContainerSample.value)
}

function containerImageLabel(container: MonitoringContainerSeries): string {
  return container.image || '镜像信息未保留'
}

function toggleOperatorLatency(id: string): void {
  operatorLatencyVisibility.value = {
    ...operatorLatencyVisibility.value,
    [id]: !operatorLatencyVisibility.value[id],
  }
}

function showAllOperatorLatency(visible: boolean): void {
  operatorLatencyVisibility.value = Object.fromEntries(
    operatorLatencyRoutes.value.map((series) => [series.id, visible]),
  )
}

function formatLatency(value: number): string {
  return `${value >= 100 ? value.toFixed(0) : value.toFixed(1)} ms`
}

function operatorLatencyLabel(series: MonitoringOperatorLatencySeries): string {
  return `${operatorLabels[series.operator]} · ${regionLabels[series.region]}`
}

function latestLatencyLabel(series: MonitoringOperatorLatencySeries): string {
  const latency = latestOperatorLatency(series)
  if (latency === undefined) return '等待采样'
  if (latency === null) return '超时'
  return formatLatency(latency)
}

type HistoryLoadMode = 'initial' | 'refresh' | 'zoom'

async function load(mode: HistoryLoadMode = 'initial', query = activeWindow.value): Promise<void> {
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  const range = selectedRange.value
  if (mode === 'initial') loading.value = true
  if (mode === 'refresh') refreshing.value = true
  if (mode === 'zoom') updating.value = true
  if (mode === 'initial') error.value = ''
  interactionError.value = ''
  try {
    const result = await api.monitoring.history(range, query, requestController.signal)
    if (controller !== requestController) return
    history.value = result
    activeWindow.value = query ? { ...query } : undefined
    if (!query) rootHistory.value = result
    operatorLatencyVisibility.value = mergeOperatorLatencyVisibility(
      operatorLatencyVisibility.value,
      result.operatorLatency || [],
    )
    reconcileContainerSelection(containerCatalog.value)
  } catch (reason) {
    if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
      const message = reason instanceof ApiError
        ? reason.message
        : '无法读取历史监控数据，请检查 Agent 状态后重试。'
      if (mode === 'initial' && !history.value) error.value = message
      else interactionError.value = message
    }
  } finally {
    if (controller === requestController) {
      loading.value = false
      refreshing.value = false
      updating.value = false
    }
  }
}

function changeRange(range: MonitoringRange): void {
  if (range === selectedRange.value) {
    if (activeWindow.value) resetZoom()
    return
  }
  void router.push({
    query: monitoringRouteQuery(range),
    state: { monitoringZoomDepth: 0 },
  })
}

function zoomToRange(selection: MonitoringHistoryQuery): void {
  if (!history.value || updating.value) return
  const start = Date.parse(selection.start)
  const end = Date.parse(selection.end)
  const minimumDuration = Math.max(1, history.value.bucketSeconds) * 2_000
  if (!Number.isFinite(start) || !Number.isFinite(end) || end - start < minimumDuration) {
    interactionError.value = '框选区间太短，请选择至少两个数据桶。'
    return
  }
  const next = { start: new Date(start).toISOString(), end: new Date(end).toISOString() }
  activeWindow.value = next
  history.value = sliceMonitoringHistory(history.value, next.start, next.end)
  const depth = currentZoomDepth()
  const location = {
    query: monitoringRouteQuery(selectedRange.value, next),
    state: { monitoringZoomDepth: Math.min(32, depth + 1) },
  }
  if (depth >= 32) void router.replace(location)
  else void router.push(location)
}

function backZoom(): void {
  if (!activeWindow.value) return
  if (currentZoomDepth() > 0) router.back()
  else resetZoom()
}

function resetZoom(): void {
  controller?.abort()
  interactionError.value = ''
  updating.value = false
  refreshing.value = false
  const depth = currentZoomDepth()
  if (depth > 0) {
    router.go(-depth)
    return
  }
  void router.replace({
    query: monitoringRouteQuery(selectedRange.value),
    state: { monitoringZoomDepth: 0 },
  })
}

function monitoringRouteQuery(range: MonitoringRange, query?: MonitoringHistoryQuery) {
  const next: LocationQueryRaw = { ...route.query, range }
  delete next.start
  delete next.end
  if (query) {
    next.start = query.start
    next.end = query.end
  }
  return next
}

function currentZoomDepth(): number {
  const value = router.options.history.state.monitoringZoomDepth
  return typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 32 ? value : 0
}

function applyRouteState(): void {
  const nextRange = monitoringRangeFromQuery(route.query.range)
  const nextWindow = monitoringWindowFromQuery(route.query.start, route.query.end)
  const rangeChanged = nextRange !== selectedRange.value
  selectedRange.value = nextRange
  activeWindow.value = nextWindow
  interactionError.value = ''

  if (rangeChanged) {
    rootHistory.value = undefined
    history.value = undefined
  }
  if (!nextWindow && rootHistory.value?.range === nextRange) {
    controller?.abort()
    history.value = rootHistory.value
    loading.value = false
    refreshing.value = false
    updating.value = false
    return
  }
  void load(history.value ? 'zoom' : 'initial', nextWindow)
}

function formatWindowTime(value: string): string {
  const time = Date.parse(value)
  if (!Number.isFinite(time)) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(new Date(time))
}

function chartIsSelected(...metrics: MonitoringMetric[]): boolean {
  return selectedMetric.value !== undefined && metrics.includes(selectedMetric.value)
}

async function focusSelectedMetric(): Promise<void> {
  const metric = selectedMetric.value
  if (!metric || !history.value?.host.length) return
  await nextTick()
  document.getElementById(monitoringTargetId(metric))?.scrollIntoView({
    behavior: 'smooth',
    block: 'center',
  })
}

watch([selectedMetric, () => history.value?.host.length], () => void focusSelectedMetric(), { flush: 'post' })
watch(containerHasAverage, (available) => {
  if (!available && containerCPUMode.value === 'average') containerCPUMode.value = 'peak'
})
watch(selectedContainerIds, (ids) => {
  if (highlightedContainerId.value && !ids.includes(highlightedContainerId.value)) {
    highlightedContainerId.value = ''
  }
})
watch(
  [() => route.query.range, () => route.query.start, () => route.query.end],
  applyRouteState,
  { immediate: true },
)

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="monitoring-page" :class="{ 'is-updating': updating }">
    <PageHeader title="历史监控" description="查看主机与容器的资源趋势；历史数据只保存在当前服务器。" />

    <div class="monitoring-toolbar" aria-label="监控时间范围">
      <button
        v-for="range in ranges"
        :key="range.value"
        class="range-button"
        :class="{ 'range-button--active': selectedRange === range.value }"
        type="button"
        @click="changeRange(range.value)"
      >
        {{ range.label }}
      </button>
      <span v-if="history?.storage.lastSampleAt" class="monitoring-toolbar__meta">
        最近采样 {{ formatDateTime(history.storage.lastSampleAt) }}
      </span>
      <button class="icon-button" type="button" :disabled="refreshing || updating" title="刷新监控数据" aria-label="刷新监控数据" @click="load('refresh')">
        <RefreshCw :size="16" :class="{ 'is-spinning': refreshing }" />
      </button>
    </div>

    <div class="monitoring-zoom-strip" :class="{ 'is-loading': updating }">
      <span v-if="activeWindow"><strong>已放大</strong> {{ zoomWindowLabel }}</span>
      <span v-else>在任意趋势图上横向拖动，即可同步框选放大全部图表。</span>
      <span v-if="updating" class="monitoring-zoom-strip__loading" title="正在读取更精细的数据">
        <RefreshCw :size="14" class="is-spinning" /> 正在读取更精细的数据
      </span>
      <div v-if="activeWindow" class="monitoring-zoom-strip__actions">
        <button type="button" aria-label="返回" :disabled="updating" @click="backZoom">
          <ArrowLeft :size="14" /> 返回
        </button>
        <button type="button" aria-label="重置" @click="resetZoom">
          <RotateCcw :size="14" /> 重置
        </button>
      </div>
    </div>

    <div v-if="interactionError" class="monitoring-warning">{{ interactionError }}</div>

    <LoadingState v-if="loading" :rows="4" cards label="正在读取历史监控数据" />
    <ErrorState v-else-if="error" title="历史监控读取失败" :message="error" @retry="load()" />
    <template v-else-if="history">
      <div class="summary-grid">
        <article class="summary-card">
          <span class="summary-card__icon"><Cpu :size="19" /></span>
          <div><span>CPU</span><strong>{{ formatPercent(latestHost?.cpuPercent) }}</strong></div>
          <small>{{ latestHost?.cpuCores || 0 }} 核 · 负载 {{ latestHost?.loadOne.toFixed(2) || '0.00' }}</small>
        </article>
        <article class="summary-card">
          <span class="summary-card__icon is-blue"><MemoryStick :size="19" /></span>
          <div><span>内存</span><strong>{{ formatPercent(percent(latestHost?.memoryUsedBytes, latestHost?.memoryTotalBytes)) }}</strong></div>
          <small>{{ formatBytes(latestHost?.memoryUsedBytes) }} / {{ formatBytes(latestHost?.memoryTotalBytes) }}</small>
        </article>
        <article class="summary-card">
          <span class="summary-card__icon is-amber"><HardDrive :size="19" /></span>
          <div><span>系统盘</span><strong>{{ formatPercent(latestHost?.diskPercent) }}</strong></div>
          <small>{{ formatBytes(latestHost?.diskUsedBytes) }} / {{ formatBytes(latestHost?.diskTotalBytes) }}</small>
        </article>
        <article class="summary-card">
          <span class="summary-card__icon is-violet"><Database :size="19" /></span>
          <div><span>历史数据</span><strong>{{ formatBytes(historyStorageBytes) }}</strong></div>
          <small>
            原始 {{ history.storage.retentionDays }} 天 · 趋势 {{ history.storage.rollupRetentionDays || 0 }} 天 ·
            上限 {{ formatBytes(historyStorageLimit) }}
          </small>
        </article>
      </div>

      <div
        v-if="history.storage.lastError || history.storage.storageLimitReached || history.storage.rollupStorageLimitReached"
        class="monitoring-warning"
      >
        {{ history.storage.lastError || '历史数据已达到固定存储上限，系统将优先保留最新数据。' }}
      </div>

      <div v-if="history.host.length" class="chart-grid">
        <article
          id="host-cpu-load-history"
          class="chart-card"
          :class="{ 'chart-card--selected': chartIsSelected('cpu', 'load') }"
        >
          <header><div><Cpu :size="18" /><strong>CPU 与负载</strong></div><span>{{ history.host.length }} 个点</span></header>
          <TrendChart :series="hostCPU" :formatter="formatPercent" :max-value="100" :selectable="!updating" @select-range="zoomToRange" />
        </article>
        <article
          id="host-memory-history"
          class="chart-card"
          :class="{ 'chart-card--selected': chartIsSelected('memory') }"
        >
          <header><div><MemoryStick :size="18" /><strong>内存</strong></div><span>内存 / Swap</span></header>
          <TrendChart :series="hostMemory" :formatter="formatPercent" :max-value="100" :selectable="!updating" @select-range="zoomToRange" />
        </article>
        <article
          id="host-disk-history"
          class="chart-card"
          :class="{ 'chart-card--selected': chartIsSelected('disk') }"
        >
          <header>
            <div><HardDrive :size="18" /><strong>磁盘</strong></div>
            <div class="chart-switch" aria-label="磁盘指标">
              <button type="button" :class="{ 'is-active': diskChartMode === 'capacity' }" @click="diskChartMode = 'capacity'">容量</button>
              <button type="button" :class="{ 'is-active': diskChartMode === 'io' }" @click="diskChartMode = 'io'">读写 I/O</button>
            </div>
          </header>
          <TrendChart
            :series="activeHostDisk"
            :formatter="diskChartMode === 'io' ? formatRate : formatPercent"
            :max-value="diskChartMode === 'capacity' ? 100 : undefined"
            :selectable="!updating"
            @select-range="zoomToRange"
          />
        </article>
        <article
          id="host-network-history"
          class="chart-card"
          :class="{ 'chart-card--selected': chartIsSelected('network') }"
        >
          <header>
            <div><Network :size="18" /><strong>网络与连接</strong></div>
            <div class="chart-switch" aria-label="网络指标">
              <button type="button" :class="{ 'is-active': networkChartMode === 'traffic' }" @click="networkChartMode = 'traffic'">流量</button>
              <button type="button" :class="{ 'is-active': networkChartMode === 'connections' }" @click="networkChartMode = 'connections'">连接数</button>
            </div>
          </header>
          <TrendChart :series="activeHostNetwork" :formatter="networkChartMode === 'traffic' ? formatRate : (value) => value.toFixed(0)" :selectable="!updating" @select-range="zoomToRange" />
        </article>
      </div>
      <EmptyState v-else title="正在积累历史数据" description="功能启用后约 1 分钟生成首个主机采样点，刷新页面即可查看。" />

      <section class="container-section">
        <header class="section-heading">
          <div>
            <span class="section-heading__icon"><Box :size="18" /></span>
            <div><h2>容器监控</h2><p>运行中容器每 5 分钟采样一次，最多记录 32 个。</p></div>
          </div>
          <span>{{ containerCatalog.length }} 个容器有历史数据 · 已选 {{ selectedContainerIds.length }}/{{ monitoringContainerSelectionLimit }}</span>
        </header>

        <div v-if="containerCatalog.length" class="container-layout">
          <div class="container-list-shell">
            <label class="container-search">
              <Search :size="15" />
              <input v-model="containerSearch" type="search" aria-label="搜索容器名称、镜像或 ID" placeholder="搜索容器名称、镜像或 ID" />
            </label>
            <p class="container-selection-note">默认选择当前资源占用靠前的 3 个容器，最多同时比较 5 个。</p>
            <p v-if="containerSelectionError" class="container-selection-error" role="status">{{ containerSelectionError }}</p>
            <div class="container-list">
              <label
                v-for="container in filteredContainers"
                :key="container.containerId"
                class="container-row"
                :class="{
                  'container-row--active': containerSelected(container.containerId),
                  'container-row--historical': containerIsHistorical(container),
                }"
                @mouseenter="containerSelected(container.containerId) && (highlightedContainerId = container.containerId)"
                @mouseleave="highlightedContainerId = ''"
                @focusin="containerSelected(container.containerId) && (highlightedContainerId = container.containerId)"
                @focusout="highlightedContainerId = ''"
              >
                <input
                  type="checkbox"
                  :checked="containerSelected(container.containerId)"
                  :aria-label="container.name"
                  @change="toggleContainer(container, $event)"
                />
                <i
                  class="container-row__color"
                  :class="{ 'is-visible': containerSelected(container.containerId) }"
                  :style="containerSelected(container.containerId) ? { background: containerColor(container.containerId) } : undefined"
                />
                <span class="container-row__identity">
                  <span class="container-row__title">
                    <strong :title="container.name">{{ container.name }}</strong>
                    <em v-if="containerIsHistorical(container)">历史</em>
                  </span>
                  <small :title="containerImageLabel(container)">{{ containerImageLabel(container) }}</small>
                </span>
                <span class="container-row__metrics">
                  <strong>{{ latestContainerPoint(container) ? formatPercent(latestContainerPoint(container)?.cpuPercent) : '—' }}</strong>
                  <small v-if="latestContainerPoint(container)">{{ formatBytes(latestContainerPoint(container)?.memoryBytes) }}</small>
                  <small v-else>当前窗口无数据</small>
                </span>
              </label>
              <p v-if="!filteredContainers.length" class="container-list-empty">没有匹配的容器</p>
            </div>
          </div>
          <div class="container-compare">
            <header>
              <div>
                <h3>容器对比</h3>
                <p>颜色代表容器；详细模式下实线和虚线代表不同方向。</p>
              </div>
            </header>
            <div class="selected-container-strip" role="list" aria-label="已选择的容器">
              <div
                v-for="container in selectedContainers"
                :key="container.containerId"
                class="selected-container-card"
                :class="{ 'is-highlighted': highlightedContainerId === container.containerId }"
                role="listitem"
                tabindex="0"
                :aria-label="`聚焦 ${containerSeriesName(container)} 曲线`"
                @mouseenter="highlightedContainerId = container.containerId"
                @mouseleave="highlightedContainerId = ''"
                @focus="highlightedContainerId = container.containerId"
                @blur="highlightedContainerId = ''"
              >
                <i :style="{ background: containerColor(container.containerId) }" />
                <strong :title="containerSeriesName(container)">{{ containerSeriesName(container) }}</strong>
                <small>{{ latestContainerPoint(container) ? formatPercent(latestContainerPoint(container)?.cpuPercent) : '—' }}</small>
              </div>
            </div>
            <div class="container-charts">
              <article>
                <header><strong>CPU</strong><div class="chart-switch"><button :class="{ 'is-active': containerCPUMode === 'peak' }" type="button" @click="containerCPUMode = 'peak'">峰值</button><button :class="{ 'is-active': containerCPUMode === 'average' }" type="button" :disabled="!containerHasAverage" @click="containerCPUMode = 'average'">平均</button></div></header>
                <TrendChart :series="containerCPU" :formatter="formatPercent" :show-legend="false" :highlight-group="highlightedContainerId" :selectable="!updating" @select-range="zoomToRange" />
              </article>
              <article>
                <header><strong>内存</strong><div class="chart-switch"><button :class="{ 'is-active': containerMemoryMode === 'bytes' }" type="button" @click="containerMemoryMode = 'bytes'">实际用量</button><button :class="{ 'is-active': containerMemoryMode === 'percent' }" type="button" @click="containerMemoryMode = 'percent'">限额占比</button></div></header>
                <TrendChart :series="containerMemory" :formatter="containerMemoryMode === 'bytes' ? formatBytes : formatPercent" :max-value="containerMemoryMode === 'percent' ? 100 : undefined" :show-legend="false" :highlight-group="highlightedContainerId" :selectable="!updating" @select-range="zoomToRange" />
              </article>
              <article>
                <header><strong>磁盘 I/O</strong><div class="chart-switch"><button :class="{ 'is-active': containerBlockMode === 'total' }" type="button" @click="containerBlockMode = 'total'">合计</button><button :class="{ 'is-active': containerBlockMode === 'direction' }" type="button" @click="containerBlockMode = 'direction'">读 / 写</button></div></header>
                <div v-if="containerBlockMode === 'direction'" class="line-style-key"><span><i />读取</span><span><i class="is-dashed" />写入</span></div>
                <TrendChart :series="containerBlock" :formatter="formatRate" :show-legend="false" :highlight-group="highlightedContainerId" :selectable="!updating" @select-range="zoomToRange" />
              </article>
              <article>
                <header><strong>网络</strong><div class="chart-switch"><button :class="{ 'is-active': containerNetworkMode === 'total' }" type="button" @click="containerNetworkMode = 'total'">总吞吐</button><button :class="{ 'is-active': containerNetworkMode === 'direction' }" type="button" @click="containerNetworkMode = 'direction'">接收 / 发送</button></div></header>
                <div v-if="containerNetworkMode === 'direction'" class="line-style-key"><span><i />接收</span><span><i class="is-dashed" />发送</span></div>
                <TrendChart :series="containerNetwork" :formatter="formatRate" :show-legend="false" :highlight-group="highlightedContainerId" :selectable="!updating" @select-range="zoomToRange" />
              </article>
            </div>
          </div>
        </div>
        <EmptyState v-else title="暂无容器历史数据" description="没有运行中的 Docker 容器，或首轮容器采样尚未完成。" />
      </section>

      <article v-if="operatorLatencyRoutes.length" class="chart-card chart-card--wide operator-latency-card">
        <header class="operator-latency-heading">
          <div>
            <span class="operator-latency-icon"><RadioTower :size="18" /></span>
            <span><strong>三网延迟</strong><small>电信、联通、移动在北京、上海、广州的固定节点</small></span>
          </div>
          <span v-if="history.storage.lastOperatorLatencyAt">
            最近一轮成功 {{ history.storage.lastOperatorLatencySuccessful || 0 }}/9 · 每
            {{ Math.max(1, Math.round((history.storage.operatorLatencyIntervalSeconds || 300) / 60)) }} 分钟
          </span>
          <span v-else>等待首次三网延迟采样</span>
        </header>
        <div class="operator-latency-controls">
          <div class="operator-latency-routes" aria-label="线路显示选择">
            <button
              v-for="series in operatorLatencyRoutes"
              :key="series.id"
              class="operator-route"
              :class="{ 'operator-route--active': operatorLatencyVisibility[series.id] }"
              type="button"
              :aria-pressed="Boolean(operatorLatencyVisibility[series.id])"
              :title="series.address"
              @click="toggleOperatorLatency(series.id)"
            >
              <i :style="{ background: operatorLatencyColors[series.id] }" />
              <span>{{ operatorLatencyLabel(series) }}</span>
              <small>{{ latestLatencyLabel(series) }}</small>
            </button>
          </div>
          <div class="operator-latency-actions">
            <button type="button" @click="showAllOperatorLatency(true)">全显示</button>
            <button type="button" @click="showAllOperatorLatency(false)">全隐藏</button>
          </div>
        </div>
        <p class="operator-latency-note">每 5 分钟采样固定节点；超时记为缺测，不记作 0 ms。</p>
        <TrendChart
          v-if="operatorLatencyChart.length"
          :series="operatorLatencyChart"
          :formatter="formatLatency"
          :selectable="!updating"
          :show-legend="false"
          @select-range="zoomToRange"
        />
        <div v-else-if="operatorLatencyVisibleCount === 0" class="operator-latency-empty">
          已隐藏全部线路，选择上方线路即可显示。
        </div>
        <div v-else class="operator-latency-empty">等待首次三网延迟采样。</div>
      </article>

      <footer class="monitoring-footnote">
        采样间隔：主机 {{ history.storage.hostIntervalSeconds }} 秒，容器
        {{ history.storage.containerIntervalSeconds }} 秒。查询读取
        {{ formatBytes(history.scannedBytes) }}，跳过 {{ history.skippedLines }} 条异常记录。
      </footer>
    </template>
  </section>
</template>

<style scoped>
.monitoring-page { display: grid; gap: 18px; }
.monitoring-page :deep(.trend-chart__line) { transition: opacity .14s ease; }
.monitoring-page.is-updating :deep(.trend-chart__line) { opacity: .72; }
.monitoring-toolbar {
  display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 6px; border: 1px solid var(--border);
  border-radius: 12px; background: var(--surface); box-shadow: var(--shadow-sm);
}
.range-button {
  min-height: 34px; padding: 0 14px; border: 0; border-radius: 8px;
  color: var(--text-soft); background: transparent; cursor: pointer;
}
.range-button:hover, .range-button--active { color: var(--brand-strong); background: var(--brand-soft); }
.monitoring-toolbar__meta { margin-left: auto; padding-right: 8px; color: var(--muted); font-size: .78rem; }
.monitoring-zoom-strip {
  display: flex; min-height: 38px; align-items: center; gap: 12px; padding: 7px 11px;
  border: 1px solid var(--border); border-radius: 10px; color: var(--muted);
  background: var(--surface-subtle); font-size: .76rem;
}
.monitoring-zoom-strip.is-loading { border-color: color-mix(in srgb, var(--brand) 28%, var(--border)); }
.monitoring-zoom-strip strong { color: var(--text); }
.monitoring-zoom-strip__loading { display: inline-flex; align-items: center; gap: 6px; color: var(--brand-strong); }
.monitoring-zoom-strip__actions { display: inline-flex; gap: 4px; margin-left: auto; }
.monitoring-zoom-strip__actions button {
  display: inline-flex; min-height: 28px; align-items: center; gap: 5px; padding: 0 9px;
  border: 1px solid var(--border); border-radius: 7px; color: var(--text-soft);
  background: var(--surface); font-size: .72rem; cursor: pointer;
}
.monitoring-zoom-strip__actions button:hover:not(:disabled) { color: var(--brand-strong); border-color: var(--brand); }
.monitoring-zoom-strip__actions button:disabled { cursor: wait; opacity: .55; }
.summary-grid, .chart-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.summary-card, .chart-card, .container-section {
  border: 1px solid var(--border); border-radius: 14px; background: var(--surface); box-shadow: var(--shadow-sm);
}
.summary-card { display: grid; grid-template-columns: auto 1fr; gap: 10px 12px; padding: 16px; }
.summary-card__icon, .section-heading__icon {
  display: grid; width: 38px; height: 38px; place-items: center; border-radius: 10px;
  background: var(--brand-soft); color: var(--brand);
}
.summary-card__icon.is-blue { color: var(--blue); background: var(--blue-soft); }
.summary-card__icon.is-violet { color: var(--violet); background: var(--violet-soft); }
.summary-card__icon.is-amber { color: var(--amber); background: var(--amber-soft); }
.summary-card div { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.summary-card span, .summary-card small { color: var(--muted); font-size: .78rem; }
.summary-card strong { color: var(--text); font-size: 1.25rem; }
.summary-card small { grid-column: 1 / -1; }
.monitoring-warning {
  padding: 11px 14px; border: 1px solid color-mix(in srgb, var(--amber) 35%, var(--border));
  border-radius: 10px; color: var(--amber); background: var(--amber-soft); font-size: .82rem;
}
.chart-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.chart-card { min-width: 0; padding: 16px; }
.chart-card--selected {
  border-color: color-mix(in srgb, var(--brand) 62%, var(--border));
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 14%, transparent), var(--shadow-sm);
}
.chart-card--wide { grid-column: 1 / -1; }
.chart-card > header, .section-heading, .container-detail > header {
  display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px;
}
.chart-card > header div, .section-heading > div { display: flex; align-items: center; gap: 8px; }
.chart-card > header span, .section-heading > span { color: var(--muted); font-size: .76rem; }
.chart-switch { display: inline-flex !important; gap: 2px !important; padding: 3px; border: 1px solid var(--border); border-radius: 9px; background: var(--surface-subtle); }
.chart-switch button { min-height: 26px; padding: 0 9px; border: 0; border-radius: 6px; color: var(--muted); background: transparent; font-size: .72rem; cursor: pointer; }
.chart-switch button:hover, .chart-switch button.is-active { color: var(--brand-strong); background: var(--brand-soft); }
.operator-latency-card { padding-bottom: 12px; }
.operator-latency-icon {
  display: grid; width: 34px; height: 34px; flex: 0 0 auto; place-items: center;
  border-radius: 9px; color: var(--brand); background: var(--brand-soft);
}
.operator-latency-heading > div > span { display: grid; gap: 2px; }
.operator-latency-heading small { color: var(--muted); font-size: .72rem; font-weight: 400; }
.operator-latency-controls { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; margin-bottom: 7px; }
.operator-latency-routes { display: flex; min-width: 0; flex: 1; flex-wrap: wrap; gap: 6px; }
.operator-route {
  display: inline-flex; min-height: 30px; align-items: center; gap: 6px; padding: 0 9px;
  border: 1px solid var(--border); border-radius: 999px; color: var(--muted);
  background: var(--surface-subtle); font-size: .72rem; cursor: pointer;
}
.operator-route:hover { border-color: color-mix(in srgb, var(--brand) 42%, var(--border)); color: var(--text); }
.operator-route--active { border-color: color-mix(in srgb, var(--brand) 38%, var(--border)); color: var(--text); background: var(--brand-soft); }
.operator-route i { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; opacity: .38; }
.operator-route--active i { opacity: 1; }
.operator-route small { color: inherit; font-size: .66rem; opacity: .72; }
.operator-latency-actions { display: inline-flex; flex: 0 0 auto; gap: 3px; padding: 3px; border: 1px solid var(--border); border-radius: 9px; background: var(--surface-subtle); }
.operator-latency-actions button { min-height: 26px; padding: 0 8px; border: 0; border-radius: 6px; color: var(--muted); background: transparent; font-size: .7rem; cursor: pointer; }
.operator-latency-actions button:hover { color: var(--brand-strong); background: var(--brand-soft); }
.operator-latency-note { margin: 0 0 2px; color: var(--muted); font-size: .7rem; }
.operator-latency-empty { display: grid; min-height: 210px; place-items: center; color: var(--muted); font-size: .8rem; }
.container-section { padding: 18px; }
.section-heading h2, .container-compare h3 { margin: 0; font-size: 1rem; }
.section-heading p, .container-compare > header p { margin: 3px 0 0; color: var(--muted); font-size: .76rem; }
.container-layout { display: grid; grid-template-columns: minmax(220px, 300px) minmax(0, 1fr); gap: 14px; }
.container-list-shell { display: flex; min-width: 0; min-height: 0; flex-direction: column; }
.container-search {
  display: flex; min-height: 36px; align-items: center; gap: 8px; padding: 0 10px;
  border: 1px solid var(--border); border-radius: 9px; color: var(--muted); background: var(--surface);
}
.container-search:focus-within { border-color: var(--brand); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 12%, transparent); }
.container-search input { min-width: 0; width: 100%; border: 0; outline: 0; color: var(--text); background: transparent; font: inherit; font-size: .76rem; }
.container-selection-note { margin: 8px 2px 4px; color: var(--muted); font-size: .68rem; line-height: 1.45; }
.container-selection-error { margin: 4px 2px; color: var(--amber); font-size: .7rem; }
.container-list { min-height: 360px; max-height: 680px; overflow: auto; padding-right: 4px; }
.container-row {
  display: grid; width: 100%; grid-template-columns: 16px 6px minmax(0, 1fr) minmax(64px, auto);
  align-items: center; gap: 10px; padding: 10px; border: 1px solid transparent; border-radius: 10px;
  color: var(--text); background: transparent; text-align: left; cursor: pointer;
}
.container-row input { width: 15px; height: 15px; flex: 0 0 auto; margin: 0; accent-color: var(--brand); }
.container-row__color { width: 6px; height: 28px; border-radius: 999px; opacity: 0; }
.container-row__color.is-visible { opacity: 1; }
.container-row + .container-row { margin-top: 4px; }
.container-row:hover, .container-row--active {
  border-color: color-mix(in srgb, var(--brand) 40%, var(--border)); background: var(--brand-soft);
}
.container-row--historical { color: var(--muted); background: var(--surface-subtle); opacity: .66; }
.container-row--historical:hover, .container-row--historical.container-row--active { opacity: 1; }
.container-row__identity { min-width: 0; }
.container-row__metrics { min-width: 64px; text-align: right; font-variant-numeric: tabular-nums; }
.container-row strong, .container-row small { display: block; }
.container-row__title { display: flex; min-width: 0; align-items: center; gap: 7px; }
.container-row__title strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.container-row__title em {
  flex: 0 0 auto; padding: 1px 5px; border-radius: 999px; color: var(--muted);
  background: color-mix(in srgb, var(--muted) 12%, transparent); font-size: .62rem; font-style: normal;
}
.container-row small {
  max-width: 100%; margin-top: 3px; overflow: hidden; color: var(--muted);
  font-size: .7rem; text-overflow: ellipsis; white-space: nowrap;
}
.container-row__metrics strong { font-size: .78rem; }
.container-list-empty { padding: 28px 8px; color: var(--muted); font-size: .76rem; text-align: center; }
.container-compare {
  min-width: 0; padding: 14px; border: 1px solid var(--border);
  border-radius: 12px; background: var(--surface-subtle);
}
.selected-container-strip { display: flex; gap: 6px; margin: 10px 0; overflow-x: auto; padding-bottom: 2px; }
.selected-container-card {
  display: flex; min-width: 120px; min-height: 34px; flex: 1 1 0; align-items: center; gap: 7px; padding: 0 9px;
  border: 1px solid var(--border); border-radius: 8px; background: var(--surface); outline: 0;
  transition: border-color .14s ease, box-shadow .14s ease;
}
.selected-container-card.is-highlighted, .selected-container-card:focus-visible { border-color: var(--brand); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 11%, transparent); }
.selected-container-card > i { width: 6px; height: 18px; flex: 0 0 auto; border-radius: 999px; }
.selected-container-card > strong { min-width: 0; flex: 1; overflow: hidden; color: var(--text); font-size: .72rem; text-overflow: ellipsis; white-space: nowrap; }
.selected-container-card > small { flex: 0 0 auto; color: var(--muted); font-size: .66rem; font-variant-numeric: tabular-nums; }
.container-charts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.container-charts > article { min-width: 0; padding: 12px; border: 1px solid var(--border); border-radius: 11px; background: var(--surface); }
.container-charts > article > header { display: flex; min-height: 28px; align-items: center; justify-content: space-between; gap: 8px; }
.container-charts > article > header > strong { font-size: .78rem; }
.container-charts .chart-switch button:disabled { cursor: not-allowed; opacity: .45; }
.line-style-key { display: flex; min-height: 22px; align-items: center; justify-content: flex-end; gap: 12px; color: var(--muted); font-size: .66rem; }
.line-style-key span { display: inline-flex; align-items: center; gap: 5px; }
.line-style-key i { width: 18px; height: 0; border-top: 2px solid currentColor; }
.line-style-key i.is-dashed { border-top-style: dashed; }
.monitoring-footnote { color: var(--muted); font-size: .74rem; text-align: center; }
@media (max-width: 1180px) {
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .container-charts { grid-template-columns: 1fr; }
}
@media (max-width: 780px) {
  .monitoring-toolbar { flex-wrap: wrap; }
  .range-button { flex: 1 1 calc(33.333% - 6px); justify-content: center; padding: 0 8px; }
  .monitoring-toolbar__meta { width: 100%; margin-left: 0; padding: 4px 8px; }
  .monitoring-zoom-strip { flex-wrap: wrap; }
  .monitoring-zoom-strip__actions { width: 100%; margin-left: 0; }
  .monitoring-zoom-strip__actions button { flex: 1; justify-content: center; }
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .chart-grid, .container-layout { grid-template-columns: 1fr; }
  .container-list { min-height: 0; max-height: 360px; }
  .chart-card--wide { grid-column: auto; }
  .container-list-shell { min-height: 0; }
  .container-list { position: static; max-height: 240px; }
  .container-detail > header { align-items: flex-start; flex-direction: column; }
  .operator-latency-heading, .operator-latency-controls { align-items: flex-start; flex-direction: column; }
  .operator-latency-actions { align-self: stretch; }
  .operator-latency-actions button { flex: 1; }
  .operator-route { flex: 1 1 calc(50% - 4px); justify-content: flex-start; }
}

@media (max-width: 480px) {
  .monitoring-page { gap: 12px; }
  .monitoring-toolbar { gap: 5px; padding: 5px; }
  .monitoring-toolbar__meta { padding: 4px 6px; font-size: .7rem; }
  .monitoring-zoom-strip { gap: 8px; padding: 8px; }
  .summary-grid { gap: 8px; }
  .summary-card { gap: 7px; padding: 12px; }
  .summary-card__icon { width: 32px; height: 32px; }
  .summary-card strong { font-size: 1.05rem; }
  .summary-card small { font-size: .7rem; }
  .chart-card, .container-section { padding: 12px; border-radius: 12px; }
  .chart-card > header, .section-heading, .container-detail > header { align-items: flex-start; flex-direction: column; }
  .container-compare { padding: 10px; }
  .operator-route { flex-basis: 100%; }
}
</style>
