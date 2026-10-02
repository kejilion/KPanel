<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, type Component } from 'vue'
import {
  Activity,
  Box,
  Boxes,
  CheckCircle2,
  CircleAlert,
  Globe2,
  LoaderCircle,
  Network,
  RefreshCw,
} from '@lucide/vue'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { ClusterHostList, SystemOverview } from '@/types/api'

const props = defineProps<{ onOpen?: (path: string) => void }>()
type MetricState = 'healthy' | 'attention' | 'unknown'
type OverallState = 'syncing' | MetricState

interface ServiceMetric {
  key: 'websites' | 'containers' | 'apps' | 'cluster'
  label: string
  value: string
  detail: string
  progress: number
  state: MetricState
  tone: 'brand' | 'blue' | 'amber' | 'violet'
  path: string
  icon: Component
}

const i18n = useI18n()
const overview = ref<SystemOverview>()
const cluster = ref<ClusterHostList>()
const loading = ref(true)
const refreshing = ref(false)
const loadFailed = ref(false)
let refreshTimer: number | undefined
let controller: AbortController | undefined
let refreshActive = false
let refreshSequence = 0

function makeMetric(
  key: ServiceMetric['key'],
  label: string,
  active: number | undefined,
  total: number | undefined,
  ready: boolean,
  detail: string,
  path: string,
  icon: Component,
  tone: ServiceMetric['tone'],
  displayValue?: string,
  stateOverride?: MetricState,
): ServiceMetric {
  const state: MetricState = stateOverride ?? (!ready || active === undefined || total === undefined
    ? 'unknown'
    : total === 0 || active >= total ? 'healthy' : 'attention')
  return {
    key,
    label,
    value: displayValue ?? (active === undefined || total === undefined ? '—' : String(active) + '/' + String(total)),
    detail,
    progress: active === undefined || total === undefined || total <= 0
      ? 0
      : Math.min(100, Math.round(active / total * 100)),
    state,
    tone,
    path,
    icon,
  }
}

const metrics = computed<ServiceMetric[]>(() => {
  const sites = overview.value?.sites
  const containers = overview.value?.containers
  const apps = overview.value?.apps
  const clusterItems = Array.isArray(cluster.value?.items) ? cluster.value.items : []
  const clusterTotal = cluster.value
    ? cluster.value.total ?? clusterItems.length
    : undefined
  const clusterOnline = cluster.value
    ? clusterItems.filter((host) => host.state === 'online').length
    : undefined
  const ready = Boolean(overview.value) && !loading.value
  return [
    makeMetric(
      'websites',
      i18n.t('desktop.serviceStatusWebsites'),
      sites ? sites.total - sites.drifted : undefined,
      sites?.total,
      sites !== undefined,
      sites?.total === 0
        ? i18n.t('desktop.serviceStatusNoItems')
        : sites
          ? i18n.t('desktop.serviceStatusDriftCount', { value: sites.drifted })
          : i18n.t('desktop.serviceStatusUnavailable'),
      '/sites',
      Globe2,
      'brand',
      sites ? String(sites.total) : undefined,
    ),
    makeMetric(
      'containers',
      i18n.t('desktop.serviceStatusContainers'),
      containers?.running,
      containers?.total,
      containers !== undefined,
      containers?.total === 0
        ? i18n.t('desktop.serviceStatusNoItems')
        : containers
          ? i18n.t('desktop.serviceStatusRunningCount', { value: containers.running })
          : i18n.t('desktop.serviceStatusUnavailable'),
      '/docker',
      Box,
      'blue',
      containers ? String(containers.total) : undefined,
    ),
    makeMetric(
      'apps',
      i18n.t('desktop.serviceStatusApps'),
      apps?.running,
      apps?.installed,
      ready && apps !== undefined,
      apps?.installed === 0 && ready
        ? i18n.t('desktop.serviceStatusNoItems')
        : apps
          ? i18n.t('desktop.serviceStatusRunningCount', { value: apps.running })
          : i18n.t('desktop.serviceStatusUnavailable'),
      '/apps',
      Boxes,
      'amber',
      apps ? String(apps.installed) : undefined,
      apps !== undefined && ready ? 'healthy' : 'unknown',
    ),
    makeMetric(
      'cluster',
      i18n.t('desktop.serviceStatusCluster'),
      clusterOnline,
      clusterTotal,
      cluster.value !== undefined,
      clusterTotal === 0
        ? i18n.t('desktop.serviceStatusNotConnected')
        : cluster.value
          ? i18n.t('desktop.serviceStatusOnlineState')
          : i18n.t('desktop.serviceStatusUnavailable'),
      '/cluster',
      Network,
      'violet',
    ),
  ]
})

const attentionCount = computed(() => metrics.value.filter((metric) => metric.state === 'attention').length
  + (overview.value && !overview.value.agent.connected ? 1 : 0))
const unknownCount = computed(() => metrics.value.filter((metric) => metric.state === 'unknown').length)
const overallState = computed<OverallState>(() => {
  if (loading.value) return 'syncing'
  if (attentionCount.value > 0) return 'attention'
  if (unknownCount.value > 0 || loadFailed.value) return 'unknown'
  return 'healthy'
})
const overallTitle = computed(() => {
  if (overallState.value === 'syncing') return i18n.t('desktop.serviceStatusSyncing')
  if (overallState.value === 'attention') return i18n.t('desktop.serviceStatusAttention')
  if (overallState.value === 'unknown') return i18n.t('desktop.serviceStatusUnknown')
  return i18n.t('desktop.serviceStatusHealthy')
})
const overallDetail = computed(() => {
  if (overallState.value === 'syncing') return i18n.t('desktop.serviceStatusSyncingDetail')
  if (overallState.value === 'attention') return i18n.t('desktop.serviceStatusAttentionDetail', { count: attentionCount.value })
  if (overallState.value === 'unknown') return i18n.t('desktop.serviceStatusUnknownDetail')
  return i18n.t('desktop.serviceStatusHealthyDetail')
})
function open(path: string): void {
  props.onOpen?.(path)
}

async function refresh(): Promise<void> {
  if (refreshActive) return
  refreshActive = true
  loading.value = !overview.value
  refreshing.value = true
  loadFailed.value = false
  const requestController = new AbortController()
  controller?.abort()
  controller = requestController
  const sequence = ++refreshSequence
  const overviewRequest = api.overview.get(requestController.signal, (value) => {
    if (sequence === refreshSequence) overview.value = value
  })
  const clusterRequest = api.cluster.hosts(requestController.signal).then((value) => {
    if (sequence === refreshSequence) cluster.value = value
  })
  const [overviewResult, clusterResult] = await Promise.allSettled([overviewRequest, clusterRequest])
  if (sequence === refreshSequence) {
    loadFailed.value = overviewResult.status === 'rejected' && clusterResult.status === 'rejected'
    loading.value = false
    refreshing.value = false
  }
  refreshActive = false
}

function stopPolling(): void {
  if (refreshTimer !== undefined) {
    window.clearInterval(refreshTimer)
    refreshTimer = undefined
  }
}

function onVisibilityChange(): void {
  if (document.hidden) {
    stopPolling()
    controller?.abort()
    return
  }
  void refresh()
  stopPolling()
  refreshTimer = window.setInterval(() => void refresh(), 45_000)
}

onMounted(() => {
  if (!document.hidden) {
    void refresh()
    refreshTimer = window.setInterval(() => void refresh(), 45_000)
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  stopPolling()
  controller?.abort()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <section class="desktop-service-status" :aria-label="i18n.t('desktop.serviceStatusLabel')">
    <header class="desktop-service-status__header desktop-widget__drag-handle">
      <Activity :size="15" aria-hidden="true" />
      <span>{{ i18n.t('desktop.serviceStatusTitle') }}</span>
      <i
        class="desktop-service-status__dot"
        :class="{
          'desktop-service-status__dot--syncing': overallState === 'syncing',
          'desktop-service-status__dot--healthy': overallState === 'healthy',
          'desktop-service-status__dot--attention': overallState === 'attention',
          'desktop-service-status__dot--unknown': overallState === 'unknown',
        }"
        aria-hidden="true"
      />
    </header>

    <div class="desktop-service-status__hero">
      <span
        class="desktop-service-status__hero-icon"
        :class="{
          'desktop-service-status__hero-icon--syncing': overallState === 'syncing',
          'desktop-service-status__hero-icon--healthy': overallState === 'healthy',
          'desktop-service-status__hero-icon--attention': overallState === 'attention',
          'desktop-service-status__hero-icon--unknown': overallState === 'unknown',
        }"
        aria-hidden="true"
      >
        <LoaderCircle v-if="overallState === 'syncing'" class="desktop-service-status__spin" :size="18" />
        <CircleAlert v-else-if="overallState === 'attention'" :size="18" />
        <CheckCircle2 v-else-if="overallState === 'healthy'" :size="18" />
        <Activity v-else :size="18" />
      </span>
      <span class="desktop-service-status__hero-copy">
        <strong>{{ overallTitle }}</strong>
        <small :title="overallDetail">{{ overallDetail }}</small>
      </span>
      <button
        type="button"
        class="desktop-service-status__refresh"
        data-widget-interactive
        :aria-label="i18n.t('desktop.serviceStatusRefresh')"
        :title="i18n.t('desktop.serviceStatusRefresh')"
        :disabled="refreshing"
        @click="refresh()"
      >
        <RefreshCw :class="{ 'desktop-service-status__spin': refreshing }" :size="14" aria-hidden="true" />
      </button>
    </div>

    <div class="desktop-service-status__metrics">
      <button
        v-for="metric in metrics"
        :key="metric.key"
        type="button"
        class="desktop-service-status__metric"
        :class="{
          'desktop-service-status__metric--healthy': metric.state === 'healthy',
          'desktop-service-status__metric--attention': metric.state === 'attention',
          'desktop-service-status__metric--unknown': metric.state === 'unknown',
        }"
        data-widget-interactive
        @click="open(metric.path)"
      >
        <span
          class="desktop-service-status__metric-icon"
          :class="{
            'desktop-service-status__metric-icon--brand': metric.tone === 'brand',
            'desktop-service-status__metric-icon--blue': metric.tone === 'blue',
            'desktop-service-status__metric-icon--amber': metric.tone === 'amber',
            'desktop-service-status__metric-icon--violet': metric.tone === 'violet',
          }"
          aria-hidden="true"
        >
          <component :is="metric.icon" :size="15" />
        </span>
        <span class="desktop-service-status__metric-copy">
          <span class="desktop-service-status__metric-label">{{ metric.label }}</span>
          <span class="desktop-service-status__metric-summary">
            <strong>{{ metric.value }}</strong>
            <small :title="metric.detail">{{ metric.detail }}</small>
          </span>
          <span class="desktop-service-status__metric-track" aria-hidden="true">
            <i :style="{ width: String(metric.progress) + '%' }" />
          </span>
        </span>
        <span class="desktop-service-status__metric-arrow" aria-hidden="true">›</span>
      </button>
    </div>

  </section>
</template>
