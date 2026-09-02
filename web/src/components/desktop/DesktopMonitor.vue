<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, Clock, Cpu, Database, HardDrive, Network } from '@lucide/vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { api, type SystemResourceSnapshot } from '@/lib/api'
import { clampPercent, formatBytes, formatDuration, formatPercent, formatRate } from '@/lib/format'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import { formatNetworkTrafficCounter } from '@/lib/networkTraffic'
import { useI18n } from '@/i18n'

/**
 * Lightweight server monitor rendered as a desktop-side widget. It polls only
 * the lightweight system summary;
 * the full overview endpoint fans out to several unrelated APIs.
 */

const i18n = useI18n()
const emit = defineEmits<{
  snapshot: [value: SystemResourceSnapshot]
}>()
const overview = ref<SystemResourceSnapshot>()
const loading = ref(true)
let refreshTimer: number | undefined
let controller: AbortController | undefined
let refreshActive = false
let compactMedia: MediaQueryList | undefined

const cpuPercent = computed(() => overview.value?.cpu.percent)
const cpuCores = computed(() => overview.value?.cpu.cores)

const memoryUsed = computed(() => overview.value?.memory.value)
const memoryTotal = computed(() => overview.value?.memory.total)
const memoryPercent = computed(() => overview.value?.memory.percent)

const diskUsed = computed(() => overview.value?.disk.value)
const diskTotal = computed(() => overview.value?.disk.total)
const diskPercent = computed(() => overview.value?.disk.percent)

const load = computed(() => overview.value?.load)
const net = computed(() => overview.value?.network)
const uptime = computed(() => overview.value?.uptimeSeconds)
const osIdentity = computed(() => detectOperatingSystemIdentity(overview.value))

const hostSystemLabel = computed(() => overview.value?.os || osIdentity.value.label)
const hostSystemMeta = computed(() => {
  const details = [overview.value?.hostname, overview.value?.architecture].filter(Boolean)
  return details.join(' · ') || '—'
})

function cpuLabel(): string {
  const cores = cpuCores.value
  if (cores === undefined) return formatPercent(cpuPercent.value)
  return i18n.t('desktop.monitorCPUValue', {
    percent: formatPercent(cpuPercent.value),
    cores: String(cores),
  })
}

function memoryLabel(): string {
  if (memoryUsed.value === undefined || memoryTotal.value === undefined) return '—'
  return `${formatBytes(memoryUsed.value)} / ${formatBytes(memoryTotal.value)}`
}

function diskLabel(): string {
  if (diskUsed.value === undefined || diskTotal.value === undefined) return '—'
  return `${formatBytes(diskUsed.value)} / ${formatBytes(diskTotal.value)}`
}

async function refresh(silent = false): Promise<void> {
  if (refreshActive) return
  refreshActive = true
  if (!silent) loading.value = true
  controller?.abort()
  controller = new AbortController()
  try {
    const snapshot = await api.system.resources(controller.signal)
    overview.value = snapshot
    emit('snapshot', snapshot)
  } catch {
    // Transient failures keep the last known values; the next tick retries.
  } finally {
    refreshActive = false
    loading.value = false
  }
}

function stopPolling(): void {
  if (refreshTimer !== undefined) {
    window.clearInterval(refreshTimer)
    refreshTimer = undefined
  }
}

function startPolling(): void {
  stopPolling()
  refreshTimer = window.setInterval(() => void refresh(true), 20_000)
}

function onVisibilityChange(): void {
  if (document.hidden || compactMedia?.matches) {
    stopPolling()
    controller?.abort()
    return
  }
  void refresh(Boolean(overview.value))
  startPolling()
}

function onCompactChange(): void {
  onVisibilityChange()
}

onMounted(() => {
  compactMedia = window.matchMedia?.('(max-width: 900px)')
  compactMedia?.addEventListener('change', onCompactChange)
  if (!document.hidden && !compactMedia?.matches) {
    void refresh()
    startPolling()
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  stopPolling()
  controller?.abort()
  compactMedia?.removeEventListener('change', onCompactChange)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <section class="desktop-monitor" :aria-label="i18n.t('desktop.monitorLabel')">
    <header class="desktop-monitor__header desktop-widget__drag-handle">
      <Activity :size="15" aria-hidden="true" />
      <span>{{ i18n.t('desktop.monitorTitle') }}</span>
      <i aria-hidden="true" />
    </header>

    <div v-if="loading && !overview" class="desktop-monitor__loading" role="status">
      {{ i18n.t('desktop.entriesLoading') }}
    </div>

    <template v-else>
      <section v-if="overview" class="desktop-monitor__host" :aria-label="i18n.t('desktop.hostIdentity')">
        <div class="desktop-monitor__host-block" :title="i18n.t('desktop.hostSystem')">
          <OperatingSystemIcon :distro="osIdentity.key" :label="osIdentity.label" />
          <span>
            <strong>{{ hostSystemLabel }}</strong>
            <small>{{ hostSystemMeta }}</small>
          </span>
        </div>
      </section>

      <dl class="desktop-monitor__list">
      <div class="desktop-monitor__metric">
        <div class="desktop-monitor__row">
          <dt><Cpu :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorCPU') }}</span></dt>
          <dd>{{ cpuLabel() }}</dd>
        </div>
        <div class="desktop-monitor__track" :aria-label="`${i18n.t('desktop.monitorCPU')} ${cpuLabel()}`">
          <span :style="{ width: `${clampPercent(cpuPercent)}%` }" />
        </div>
      </div>

      <div class="desktop-monitor__metric">
        <div class="desktop-monitor__row">
          <dt><Database :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorMemory') }}</span></dt>
          <dd>{{ memoryLabel() }}</dd>
        </div>
        <div class="desktop-monitor__track" :aria-label="`${i18n.t('desktop.monitorMemory')} ${memoryLabel()}`">
          <span :style="{ width: `${clampPercent(memoryPercent)}%` }" />
        </div>
      </div>

      <div class="desktop-monitor__metric">
        <div class="desktop-monitor__row">
          <dt><HardDrive :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorDisk') }}</span></dt>
          <dd>{{ diskLabel() }}</dd>
        </div>
        <div class="desktop-monitor__track" :aria-label="`${i18n.t('desktop.monitorDisk')} ${diskLabel()}`">
          <span :style="{ width: `${clampPercent(diskPercent)}%` }" />
        </div>
      </div>

      <div class="desktop-monitor__metric desktop-monitor__metric--network">
        <div class="desktop-monitor__row">
          <dt><Network :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorNetwork') }}</span></dt>
          <dd class="desktop-monitor__network">
            <span class="desktop-monitor__network-line">
              <span>↓ {{ formatRate(net?.receiveBytesPerSecond) }}</span>
              <span>↑ {{ formatRate(net?.transmitBytesPerSecond) }}</span>
            </span>
            <span class="desktop-monitor__network-line desktop-monitor__network-total">
              <small>{{ i18n.t('desktop.monitorTrafficTotal') }}</small>
              <span :title="i18n.t('desktop.monitorTrafficReceived')">↓ {{ formatNetworkTrafficCounter(net, 'received') }}</span>
              <span :title="i18n.t('desktop.monitorTrafficSent')">↑ {{ formatNetworkTrafficCounter(net, 'sent') }}</span>
            </span>
          </dd>
        </div>
      </div>

      <div class="desktop-monitor__metric desktop-monitor__metric--secondary">
        <div class="desktop-monitor__row">
          <dt><Activity :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorLoad') }}</span></dt>
          <dd>
            {{ load?.one?.toFixed(2) ?? '—' }}
            {{ load?.five?.toFixed(2) ?? '' }}
            {{ load?.fifteen?.toFixed(2) ?? '' }}
          </dd>
        </div>
      </div>

      <div class="desktop-monitor__metric desktop-monitor__metric--secondary">
        <div class="desktop-monitor__row">
          <dt><Clock :size="14" aria-hidden="true" /><span>{{ i18n.t('desktop.monitorUptime') }}</span></dt>
          <dd>{{ formatDuration(uptime) }}</dd>
        </div>
      </div>
      </dl>
    </template>
  </section>
</template>
