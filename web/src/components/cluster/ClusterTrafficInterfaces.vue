<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import { useI18n } from '@/i18n'
import { ApiError, api } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import type { ClusterHost, TrafficInterfaceReason, TrafficInterfaceStatus, TrafficInterfacesSnapshot } from '@/types/api'

// Each host keeps its own selection: this panel edits only its local host.
// Other KPanel hosts are set in their own panel, lightweight nodes on the node.
const props = defineProps<{ host: ClusterHost; disabled: boolean }>()
const { t } = useI18n()
const id = `cluster-traffic-interfaces-${useId()}`
const lightCommand = '/usr/local/lib/kejilion-node/kejilion-node interfaces'
const snapshot = ref<TrafficInterfacesSnapshot>()
const loading = ref(false)
const loadError = ref('')
const mode = ref<'auto' | 'manual'>('auto')
const chosen = ref<string[]>([])
let controller: AbortController | undefined

const rows = computed(() => snapshot.value?.interfaces.filter((entry) => entry.reason !== 'loopback') ?? [])
const savedInclude = computed(() => snapshot.value?.selection.include ?? [])
const savedExclude = computed(() => snapshot.value?.selection.exclude ?? [])
const dirty = computed(() => {
  if (!snapshot.value) return false
  if (mode.value === 'auto') return savedInclude.value.length > 0
  return !sameNames(chosen.value, savedInclude.value)
})
// Docker adds a veth per container: uncounted, unchosen virtual interfaces sit
// in a collapsed group so the uplinks stay visible. Grouping follows the saved
// state, so a row does not jump between groups while it is being ticked.
const sections = computed(() => {
  const primary: TrafficInterfaceStatus[] = []
  const others: TrafficInterfaceStatus[] = []
  for (const entry of rows.value) {
    const quiet = entry.virtual && !entry.counted && entry.reason !== 'missing' && !savedInclude.value.includes(entry.name)
    if (quiet) others.push(entry)
    else primary.push(entry)
  }
  return [
    { key: 'primary', collapsed: false, rows: primary },
    ...(others.length ? [{ key: 'others', collapsed: true, rows: others }] : []),
  ]
})
const fellBack = computed(() => savedInclude.value.length > 0 && rows.value.every(
  (entry) => !savedInclude.value.includes(entry.name) || entry.reason === 'missing',
))

function sameNames(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((name) => right.includes(name))
}

function apply(value: TrafficInterfacesSnapshot): void {
  snapshot.value = value
  const include = value.selection.include
  mode.value = include.length ? 'manual' : 'auto'
  chosen.value = include.length
    ? [...include]
    : value.interfaces.filter((entry) => entry.counted).map((entry) => entry.name)
}

async function load(): Promise<void> {
  controller?.abort()
  snapshot.value = undefined
  loadError.value = ''
  if (!props.host.isLocal) return
  const current = new AbortController()
  controller = current
  loading.value = true
  try {
    apply(await api.system.trafficInterfaces(current.signal))
  } catch (reason) {
    if (current.signal.aborted) return
    loadError.value = reason instanceof ApiError && reason.status === 404
      ? t('cluster.trafficInterfaces.unsupported')
      : t('cluster.trafficInterfaces.loadFailed')
  } finally {
    if (controller === current) loading.value = false
  }
}

function toggle(name: string, checked: boolean): void {
  chosen.value = checked
    ? [...chosen.value.filter((item) => item !== name), name]
    : chosen.value.filter((item) => item !== name)
}

function reasonLabel(reason: TrafficInterfaceReason): string {
  return t(`cluster.trafficInterfaces.reason.${reason}`)
}

/** Returns a message when the selection cannot be saved, before any save phase starts. */
function validate(): string {
  return snapshot.value && mode.value === 'manual' && chosen.value.length === 0
    ? t('cluster.trafficInterfaces.chooseOne')
    : ''
}

async function save(): Promise<void> {
  const current = snapshot.value
  if (!current || !dirty.value) return
  const manual = mode.value === 'manual'
  try {
    apply(await api.system.updateTrafficInterfaces({
      include: manual ? [...chosen.value] : [],
      // Exclusions refine the automatic choice; an explicit list replaces them.
      exclude: manual ? [] : [...current.selection.exclude],
      expectedResourceVersion: current.resourceVersion,
    }))
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'traffic_interfaces_changed') await load()
    throw reason
  }
}

watch(() => props.host.id, load, { immediate: true })
onBeforeUnmount(() => controller?.abort())
defineExpose({ dirty, validate, save })
</script>

<template>
  <div class="cluster-manage__details cluster-traffic-interfaces form-stack">
    <strong :id="`${id}-title`">{{ t('cluster.trafficInterfaces.title') }}</strong>
    <template v-if="host.isLocal">
      <p v-if="loading" class="cluster-traffic-interfaces__status" role="status">{{ t('cluster.trafficInterfaces.loading') }}</p>
      <div v-else-if="loadError" class="cluster-traffic-interfaces__status" role="alert">
        <span>{{ loadError }}</span>
        <button class="button button--ghost button--small" type="button" :disabled="disabled" @click="load">
          {{ t('cluster.trafficInterfaces.retry') }}
        </button>
      </div>
      <template v-else-if="snapshot">
        <div class="cluster-traffic-interfaces__modes" role="radiogroup" :aria-labelledby="`${id}-title`">
          <label>
            <input v-model="mode" type="radio" value="auto" :name="`${id}-mode`" :disabled="disabled" />
            {{ t('cluster.trafficInterfaces.auto') }}
          </label>
          <label>
            <input v-model="mode" type="radio" value="manual" :name="`${id}-mode`" :disabled="disabled" />
            {{ t('cluster.trafficInterfaces.manual') }}
          </label>
        </div>
        <p v-if="snapshot.selectionError" class="cluster-traffic-interfaces__warning" role="alert">
          {{ t('cluster.trafficInterfaces.selectionError') }}
        </p>
        <p v-else-if="mode === 'manual' && fellBack && !dirty" class="cluster-traffic-interfaces__warning" role="status">
          {{ t('cluster.trafficInterfaces.fallback') }}
        </p>
        <p v-if="mode === 'auto' && savedExclude.length" class="cluster-traffic-interfaces__note">
          {{ t('cluster.trafficInterfaces.excluded', { names: savedExclude.join(', ') }) }}
        </p>
        <component
          :is="section.collapsed ? 'details' : 'div'"
          v-for="section in rows.length ? sections : []"
          :key="section.key"
          class="cluster-traffic-interfaces__group"
        >
          <summary v-if="section.collapsed">{{ t('cluster.trafficInterfaces.others', { count: section.rows.length }) }}</summary>
          <ul class="cluster-traffic-interfaces__list">
            <li v-for="entry in section.rows" :key="entry.name">
              <label>
                <input
                  type="checkbox"
                  :checked="mode === 'manual' ? chosen.includes(entry.name) : entry.counted"
                  :disabled="disabled || mode === 'auto'"
                  :aria-label="t('cluster.trafficInterfaces.countInterface', { name: entry.name })"
                  @change="toggle(entry.name, ($event.target as HTMLInputElement).checked)"
                />
                <code>{{ entry.name }}</code>
              </label>
              <span class="cluster-traffic-interfaces__reason">{{ reasonLabel(entry.reason) }}</span>
              <span v-if="entry.reason !== 'missing'" class="cluster-traffic-interfaces__bytes">
                ↓ {{ formatBytes(entry.receivedBytes) }} · ↑ {{ formatBytes(entry.sentBytes) }}
              </span>
            </li>
          </ul>
        </component>
        <p v-if="!rows.length" class="cluster-traffic-interfaces__note">{{ t('cluster.trafficInterfaces.empty') }}</p>
      </template>
      <small>{{ t('cluster.trafficInterfaces.hint') }}</small>
    </template>
    <template v-else-if="host.kind === 'light_node'">
      <small>{{ t('cluster.trafficInterfaces.lightHint') }}</small>
      <code class="cluster-traffic-interfaces__command">{{ lightCommand }}</code>
      <code class="cluster-traffic-interfaces__command">{{ lightCommand }} include eth0</code>
      <small>{{ t('cluster.trafficInterfaces.lightAuto') }}</small>
    </template>
    <small v-else>{{ t('cluster.trafficInterfaces.remoteHint') }}</small>
  </div>
</template>

<style scoped>
/* The dialog's own section styles are scoped to ClusterView and reach only this root. */
.cluster-traffic-interfaces small { font-size: .8125rem; line-height: 1.5; color: var(--text-soft); }
.cluster-traffic-interfaces__modes { display: flex; flex-wrap: wrap; gap: 8px 20px; font-size: .875rem; }
.cluster-traffic-interfaces__modes label,
.cluster-traffic-interfaces__list label { display: inline-flex; align-items: center; gap: 8px; cursor: pointer; }
.cluster-traffic-interfaces__list { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
.cluster-traffic-interfaces__group summary { cursor: pointer; padding: 4px 0 8px; font-size: .875rem; color: var(--text-soft); }
.cluster-traffic-interfaces__group summary:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
.cluster-traffic-interfaces__list li {
  display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px;
  padding: 8px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); font-size: .875rem;
}
.cluster-traffic-interfaces__list code { font-size: .875rem; overflow-wrap: anywhere; }
.cluster-traffic-interfaces__reason,
.cluster-traffic-interfaces__bytes { font-size: .8125rem; color: var(--text-soft); }
.cluster-traffic-interfaces__bytes { margin-left: auto; font-variant-numeric: tabular-nums; }
.cluster-traffic-interfaces__status { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 0; font-size: .875rem; color: var(--text-soft); }
.cluster-traffic-interfaces__warning { margin: 0; font-size: .875rem; color: var(--warning); }
.cluster-traffic-interfaces__note { margin: 0; font-size: .875rem; color: var(--text-soft); }
.cluster-traffic-interfaces__command {
  display: block; padding: 6px 8px; border-radius: var(--radius-sm); background: var(--surface-muted);
  font-size: .8125rem; overflow-wrap: anywhere; user-select: all;
}
</style>
