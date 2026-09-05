<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ArrowLeft, ArrowUpRight, ChevronLeft, ChevronRight, Globe2, MapPin, Minus, Pause, Pencil, Play, Plus, RotateCcw, Search, X } from '@lucide/vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { clampPercent, formatBytes, formatDateTime, formatDuration, formatPercent, formatRate, relativeTime } from '@/lib/format'
import { formatNetworkTrafficCounter } from '@/lib/networkTraffic'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { ClusterHost } from '@/types/api'
import { GlobeRenderer, globeHostLocation, groupGlobeHosts, isPublicGlobeHost, type GlobeHost, type GlobePalette } from './globeRenderer'

const props = withDefaults(defineProps<{ hosts: GlobeHost[]; searchable?: boolean }>(), { searchable: true })
const emit = defineEmits<{ manage: [host: ClusterHost]; openPanel: [host: ClusterHost] }>()
const active = inject(desktopWindowActiveKey, computed(() => true))
const root = ref<HTMLElement>()
const canvas = ref<HTMLCanvasElement>()
const selectedId = ref('')
const rotating = ref(true)
const unavailable = ref(false)
const query = ref('')
const statusFilter = ref<'all' | 'online' | 'attention'>('all')
const regionFilter = ref('')
const showDetails = ref(false)
const zoom = ref(1)
const allRegions = computed(() => groupGlobeHosts(props.hosts))
const knownIds = computed(() => new Set(allRegions.value.flatMap(region => region.hosts.map(host => host.id))))
const needsAttention = (host: GlobeHost) => !['online', 'unknown', 'pending'].includes(host.state)
const filteredHosts = computed(() => props.hosts.filter(host => {
  const location = globeHostLocation(host)
  if (statusFilter.value === 'online' && host.state !== 'online') return false
  if (statusFilter.value === 'attention' && !needsAttention(host)) return false
  if (regionFilter.value === 'unknown' && knownIds.value.has(host.id)) return false
  if (regionFilter.value && regionFilter.value !== 'unknown' && location?.countryCode?.trim().toUpperCase() !== regionFilter.value) return false
  const text = [host.name, location?.country, location?.city, location?.isp, hostSample(host)?.os].filter(Boolean).join(' ').toLowerCase()
  return text.includes(query.value.trim().toLowerCase())
}))
const online = computed(() => props.hosts.filter(host => host.state === 'online').length)
const attention = computed(() => props.hosts.filter(needsAttention).length)
const hasFilters = computed(() => Boolean(query.value || regionFilter.value || statusFilter.value !== 'all'))
const regions = computed(() => groupGlobeHosts(filteredHosts.value))
const located = computed(() => regions.value.reduce((sum, region) => sum + region.hosts.length, 0))
const selected = computed(() => filteredHosts.value.find(host => host.id === selectedId.value) || filteredHosts.value[0])
const selectedIndex = computed(() => filteredHosts.value.findIndex(host => host.id === selected.value?.id))
const managementHost = computed(() => selected.value && !isPublicGlobeHost(selected.value) ? selected.value : undefined)
const publicHost = computed(() => selected.value && isPublicGlobeHost(selected.value) ? selected.value : undefined)
const telemetry = computed(() => managementHost.value?.lastSnapshot?.telemetry)
const publicSample = computed(() => publicHost.value?.collectedAt ? publicHost.value : undefined)
const sample = computed(() => publicSample.value || telemetry.value)
const location = computed(() => selected.value ? globeHostLocation(selected.value) : undefined)
const collectedAt = computed(() => publicHost.value?.collectedAt || managementHost.value?.lastSnapshot?.receivedAt)
const receiveRate = computed(() => publicSample.value?.network.receiveBytesPerSecond ?? managementHost.value?.lastSnapshot?.receiveBytesPerSecond)
const transmitRate = computed(() => publicSample.value?.network.transmitBytesPerSecond ?? managementHost.value?.lastSnapshot?.transmitBytesPerSecond)
const latency = computed(() => {
  const value = managementHost.value?.lastSnapshot?.latencyMilliseconds
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? `${value} ms` : '--'
})
const selectedLocated = computed(() => regions.value.some(region => region.hosts.some(host => host.id === selected.value?.id)))
const metrics = computed(() => [
  { label: 'CPU', value: sample.value?.cpu.usagePercent, details: [sample.value?.cpu.cores ? `${sample.value.cpu.cores} ${phrase('核')}` : '—'] },
  { label: '内存', value: sample.value?.memory.usagePercent, details: [formatBytes(sample.value?.memory.usedBytes), formatBytes(sample.value?.memory.totalBytes)] },
  { label: '磁盘', value: sample.value?.disk.usagePercent, details: [formatBytes(sample.value?.disk.usedBytes), formatBytes(sample.value?.disk.totalBytes)] },
])
let renderer: GlobeRenderer | undefined
let resizeObserver: ResizeObserver | undefined
let intersectionObserver: IntersectionObserver | undefined
let themeObserver: MutationObserver | undefined
let motion: MediaQueryList | undefined
let frame = 0
let lastTime = 0
let visible = true
let dirty = true
let disposed = false
let drag: { id: number; x: number; y: number; distance: number } | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function palette(): GlobePalette {
  const style = getComputedStyle(root.value!)
  return {
    brand: style.getPropertyValue('--brand').trim(),
    land: style.getPropertyValue('--brand-strong').trim(),
    light: document.documentElement.dataset.theme !== 'dark',
    text: style.getPropertyValue('--text').trim(),
    surface: style.getPropertyValue('--surface').trim(),
    border: style.getPropertyValue('--border').trim(),
    success: style.getPropertyValue('--success').trim(),
    warning: style.getPropertyValue('--warning').trim(),
  }
}

function canRender(): boolean {
  return Boolean(renderer && !disposed && active.value && visible && !document.hidden)
}

function schedule(): void {
  dirty = true
  if (!frame && canRender()) frame = requestAnimationFrame(tick)
}

function tick(time: number): void {
  frame = 0
  if (!canRender()) return
  // Draw at most 30 fps; no reactive updates or Vue renders in the animation loop.
  if (dirty || time - lastTime >= 1000 / 30) {
    if (rotating.value && !drag) renderer!.move(Math.min(time - (lastTime || time), 80) * .003, 0)
    renderer!.draw()
    dirty = false
    lastTime = time
  }
  if (rotating.value) frame = requestAnimationFrame(tick)
}

function syncActivity(): void {
  if (frame) cancelAnimationFrame(frame)
  frame = 0
  lastTime = 0
  schedule()
}

function select(host: GlobeHost): void {
  selectedId.value = host.id
  showDetails.value = true
  rotating.value = false
  const region = regions.value.find(item => item.hosts.some(node => node.id === host.id))
  if (region) renderer?.focus(region)
  schedule()
}

function hostSample(host: GlobeHost) {
  return isPublicGlobeHost(host) ? (host.collectedAt ? host : undefined) : host.lastSnapshot?.telemetry
}

function nodeMetric(host: GlobeHost, key: 'cpu' | 'memory' | 'disk'): string {
  const value = hostSample(host)?.[key].usagePercent
  return typeof value === 'number' && Number.isFinite(value) ? formatPercent(value) : '—'
}

function clearFilters(): void {
  query.value = ''; statusFilter.value = 'all'; regionFilter.value = ''
}

function stepNode(delta: number): void {
  const host = filteredHosts.value[selectedIndex.value + delta]
  if (host) select(host)
}

function setZoom(value: number): void {
  zoom.value = Math.max(1, Math.min(2, Math.round(value * 100) / 100))
  renderer?.setZoom(zoom.value)
  schedule()
}

function stateLabel(host: GlobeHost): string | undefined {
  if (!isPublicGlobeHost(host)) return undefined
  return phrase({ online: '在线', degraded: '需关注', offline: '离线', pending: '等待数据' }[host.state])
}

function cacheFlag(code: string, event: Event): void {
  const image = event.target
  if (!(image instanceof HTMLImageElement) || !image.naturalWidth) return
  // Reuse the node list's already loaded image; no second request or per-frame decoding.
  renderer?.setFlag(code, image)
  schedule()
}

function reset(): void {
  if (renderer) { renderer.latitude = 18; renderer.longitude = 105 }
  setZoom(1)
  schedule()
}

function startDrag(event: PointerEvent): void {
  if (event.button !== 0 || drag || !renderer) return
  rotating.value = false
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, distance: 0 }
  canvas.value?.setPointerCapture(event.pointerId)
}

function moveDrag(event: PointerEvent): void {
  if (!drag || drag.id !== event.pointerId) return
  const dx = event.clientX - drag.x, dy = event.clientY - drag.y
  drag.distance += Math.abs(dx) + Math.abs(dy)
  renderer?.move(-dx * .35, dy * .35)
  drag.x = event.clientX; drag.y = event.clientY
  schedule()
}

function finishDrag(event: PointerEvent): void {
  if (!drag || drag.id !== event.pointerId) return
  const clicked = drag.distance < 6 && event.type === 'pointerup'
  drag = undefined
  if (canvas.value?.hasPointerCapture(event.pointerId)) canvas.value.releasePointerCapture(event.pointerId)
  if (clicked && canvas.value) {
    const rect = canvas.value.getBoundingClientRect()
    const group = renderer?.hitTest(event.clientX - rect.left, event.clientY - rect.top)
    if (group) {
      if (group.hosts.length === 1) select(group.hosts[0]!)
      else {
        regionFilter.value = group.code
        showDetails.value = false
        renderer?.focus(group)
      }
    }
  }
  schedule()
}

function keyboard(event: KeyboardEvent): void {
  const directions: Record<string, [number, number]> = {
    ArrowLeft: [-12, 0], ArrowRight: [12, 0], ArrowUp: [0, 10], ArrowDown: [0, -10],
  }
  const delta = directions[event.key]
  if (!delta && event.key !== 'Home') return
  event.preventDefault()
  rotating.value = false
  if (delta) renderer?.move(...delta)
  else reset()
  schedule()
}

function syncMotion(): void {
  if (motion?.matches) rotating.value = false
}

watch([regions, selected], () => {
  renderer?.setHosts(regions.value, selected.value?.id || '')
  schedule()
})
watch(active, syncActivity)
watch(rotating, syncActivity)
watch([query, statusFilter, regionFilter], () => { showDetails.value = false })
watch(showDetails, async details => {
  await nextTick()
  if (disposed) return
  const target = root.value?.querySelector<HTMLElement>(details ? '.cluster-globe__back' : '.cluster-globe__node[aria-pressed="true"], .cluster-globe__empty button')
  target?.focus({ preventScroll: true })
  target?.scrollIntoView?.({ block: 'nearest' })
})
watch(allRegions, () => {
  if (regionFilter.value && regionFilter.value !== 'unknown' && !allRegions.value.some(region => region.code === regionFilter.value)) regionFilter.value = ''
})

onMounted(() => {
  let context: CanvasRenderingContext2D | null = null
  try { context = canvas.value?.getContext('2d') || null } catch { /* Keep the host list usable. */ }
  if (!context) { unavailable.value = true; rotating.value = false; return }
  renderer = new GlobeRenderer(context, palette())
  renderer.setHosts(regions.value, selected.value?.id || '')
  motion = window.matchMedia('(prefers-reduced-motion: reduce)')
  syncMotion()
  motion.addEventListener('change', syncMotion)
  resizeObserver = new ResizeObserver(entries => {
    const rect = entries[0]?.contentRect
    if (!rect || rect.width < 1 || rect.height < 1) return
    renderer?.resize(rect.width, rect.height, window.devicePixelRatio)
    schedule()
  })
  resizeObserver.observe(canvas.value!)
  intersectionObserver = new IntersectionObserver(entries => {
    visible = entries.some(entry => entry.isIntersecting)
    syncActivity()
  })
  intersectionObserver.observe(canvas.value!)
  themeObserver = new MutationObserver(() => { renderer?.setPalette(palette()); schedule() })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'style', 'class'] })
  document.addEventListener('visibilitychange', syncActivity)
  schedule()
})

onBeforeUnmount(() => {
  disposed = true
  if (frame) cancelAnimationFrame(frame)
  resizeObserver?.disconnect()
  intersectionObserver?.disconnect()
  themeObserver?.disconnect()
  motion?.removeEventListener('change', syncMotion)
  document.removeEventListener('visibilitychange', syncActivity)
  renderer = undefined
})
</script>

<template>
  <section ref="root" class="cluster-globe" aria-label="集群地球视图">
    <div class="cluster-globe__layout">
    <div class="cluster-globe__world">
      <header class="cluster-globe__heading">
        <div><span class="cluster-globe__eyebrow"><Globe2 :size="15" /> 全球节点</span><h2>全球主机，一眼尽览</h2></div>
        <div class="cluster-globe__coverage"><strong>{{ regions.length }}</strong><span>覆盖地区</span></div>
      </header>
      <div class="cluster-globe__stage">
        <canvas
          ref="canvas" class="cluster-globe__canvas" tabindex="0" role="img"
          :aria-label="phrase('节点地球：拖动或使用方向键旋转，Home 复位；也可从节点列表选择主机。')"
          @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="finishDrag"
          @pointercancel="finishDrag" @lostpointercapture="finishDrag" @keydown="keyboard"
        />
        <p v-if="unavailable" class="cluster-globe__fallback" role="status">地球绘制不可用，仍可从节点列表查看主机。</p>
      </div>
      <footer class="cluster-globe__legend">
        <div class="cluster-globe__legend-items"><span><i class="is-online" /> 在线</span><span><i class="is-attention" /> 其他状态</span><span>{{ located }} / {{ filteredHosts.length }} {{ phrase('已定位') }}</span></div>
        <div class="cluster-globe__controls">
          <div class="cluster-globe__zoom" role="group" aria-label="地球缩放">
            <button class="icon-button icon-button--small" type="button" :disabled="unavailable || zoom <= 1" title="缩小地球" aria-label="缩小地球" @click="setZoom(zoom - .25)"><Minus :size="15" /></button>
            <span>{{ Math.round(zoom * 100) }}%</span>
            <button class="icon-button icon-button--small" type="button" :disabled="unavailable || zoom >= 2" title="放大地球" aria-label="放大地球" @click="setZoom(zoom + .25)"><Plus :size="15" /></button>
          </div>
          <button class="button button--secondary button--small" type="button" :disabled="unavailable" :aria-pressed="rotating" @click="rotating = !rotating">
            <Pause v-if="rotating" :size="14" /><Play v-else :size="14" />{{ rotating ? phrase('暂停旋转') : phrase('自动旋转') }}
          </button>
          <button class="icon-button icon-button--small" type="button" :disabled="unavailable" title="复位地球" aria-label="复位地球" @click="reset"><RotateCcw :size="15" /></button>
        </div>
        <p>按国家/地区近似定位。同地区节点合并显示；密集标记自动避让，可放大或筛选查看。</p>
      </footer>
    </div>
    <aside class="cluster-globe__sidebar">
      <div v-if="!showDetails || !selected" class="cluster-globe__explorer">
        <div class="cluster-globe__nodes-heading"><strong>节点列表</strong><span>{{ filteredHosts.length }} / {{ hosts.length }}</span></div>
        <div class="cluster-globe__filters">
          <label v-if="searchable" class="cluster-globe__search"><Search :size="16" /><input v-model="query" type="search" aria-label="搜索地球节点" placeholder="搜索名称、地区或系统…" /></label>
          <div class="cluster-globe__status-filters" role="group" aria-label="筛选节点状态">
            <button type="button" :aria-pressed="statusFilter === 'all'" @click="statusFilter = 'all'">全部 <span>{{ hosts.length }}</span></button>
            <button type="button" :aria-pressed="statusFilter === 'online'" @click="statusFilter = 'online'">在线 <span>{{ online }}</span></button>
            <button type="button" :aria-pressed="statusFilter === 'attention'" @click="statusFilter = 'attention'">需关注 <span>{{ attention }}</span></button>
          </div>
          <div class="cluster-globe__region-filter"><Globe2 :size="16" /><select v-model="regionFilter" aria-label="筛选节点地区"><option value="">全部地区</option><option v-for="region in allRegions" :key="region.code" :value="region.code" data-i18n-ignore>{{ globeHostLocation(region.hosts[0]!)?.country || region.code }} · {{ region.hosts.length }}</option><option v-if="knownIds.size < hosts.length" value="unknown">位置未知</option></select><button v-if="hasFilters" type="button" class="icon-button icon-button--small" title="清除筛选" aria-label="清除筛选" @click="clearFilters"><X :size="14" /></button></div>
        </div>
        <div v-if="filteredHosts.length" class="cluster-globe__nodes" role="group" aria-label="选择集群节点">
          <button v-for="host in filteredHosts" :key="host.id" type="button" class="cluster-globe__node" :class="{ 'is-selected': selected?.id === host.id }" :aria-pressed="selected?.id === host.id" @click="select(host)">
            <CountryFlagIcon
              v-if="globeHostLocation(host)?.countryCode"
              :country-code="globeHostLocation(host)!.countryCode!"
              :label="globeHostLocation(host)!.country || globeHostLocation(host)!.countryCode!"
              @load="cacheFlag(globeHostLocation(host)!.countryCode!, $event)"
            />
            <Globe2 v-else :size="19" />
            <span class="cluster-globe__node-body"><strong class="cluster-globe__node-name" data-i18n-ignore>{{ host.name }}</strong><span class="cluster-globe__node-meta"><StatusBadge :status="host.state" :label="stateLabel(host)" subtle /><span data-i18n-ignore>{{ globeHostLocation(host)?.city || phrase('位置未知') }}</span></span></span>
            <span class="cluster-globe__node-metrics"><span>CPU <b>{{ nodeMetric(host, 'cpu') }}</b></span><span>{{ phrase('内存') }} <b>{{ nodeMetric(host, 'memory') }}</b></span><span>{{ phrase('磁盘') }} <b>{{ nodeMetric(host, 'disk') }}</b></span></span>
          </button>
        </div>
        <div v-else class="cluster-globe__empty" role="status"><Search :size="24" /><strong>没有匹配的节点</strong><button class="button button--secondary button--small" type="button" @click="clearFilters">清除筛选</button></div>
      </div>
      <div v-else class="cluster-globe__detail">
        <nav class="cluster-globe__detail-nav" aria-label="浏览节点详情"><button type="button" class="cluster-globe__back" @click="showDetails = false"><ArrowLeft :size="16" /> 节点列表</button><span>{{ selectedIndex + 1 }} / {{ filteredHosts.length }}</span><button class="icon-button icon-button--small" type="button" :disabled="selectedIndex <= 0" title="上一个节点" aria-label="上一个节点" @click="stepNode(-1)"><ChevronLeft :size="16" /></button><button class="icon-button icon-button--small" type="button" :disabled="selectedIndex >= filteredHosts.length - 1" title="下一个节点" aria-label="下一个节点" @click="stepNode(1)"><ChevronRight :size="16" /></button></nav>
        <div class="cluster-globe__detail-heading"><span>当前节点</span><StatusBadge :status="selected.state" :label="stateLabel(selected)" subtle /></div>
        <h3 data-i18n-ignore>{{ selected.name }}</h3>
        <p class="cluster-globe__location"><MapPin :size="14" /><span data-i18n-ignore>{{ [location?.country, location?.region, location?.city].filter((value, index, items) => value && items.indexOf(value) === index).join(' · ') || phrase(publicHost ? '地区未公开' : '位置未知') }}</span></p>
        <p v-if="!selectedLocated" class="cluster-globe__unknown">地理信息不足，暂未在地球上标点。</p>
        <div class="cluster-globe__metrics">
          <div v-for="metric in metrics" :key="metric.label">
            <span>{{ phrase(metric.label) }}</span><strong>{{ typeof metric.value === 'number' && Number.isFinite(metric.value) ? formatPercent(metric.value) : '--' }}</strong>
            <div class="cluster-globe__meter" aria-hidden="true"><i :style="{ width: `${clampPercent(metric.value ?? 0)}%` }" /></div>
            <small v-if="sample"><span v-for="(detail, index) in metric.details" :key="index">{{ index ? ' / ' : '' }}{{ detail }}</span></small>
          </div>
        </div>
        <dl v-if="sample" class="cluster-globe__host-details">
          <div class="cluster-globe__system"><dt>系统</dt><dd><span data-i18n-ignore>{{ sample.os || '—' }}</span><small v-if="sample.architecture || telemetry?.kernel" data-i18n-ignore>{{ [sample.architecture, telemetry?.kernel].filter(Boolean).join(' · ') }}</small></dd></div>
          <div class="cluster-globe__isp"><dt>运营商</dt><dd data-i18n-ignore>{{ location?.isp || phrase(publicHost ? '网络信息未公开' : '运营商未知') }}</dd></div>
          <div><dt>实时下行</dt><dd>{{ formatRate(receiveRate) }}</dd></div>
          <div><dt>实时上行</dt><dd>{{ formatRate(transmitRate) }}</dd></div>
          <div><dt>累计接收</dt><dd>{{ formatNetworkTrafficCounter(sample.network, 'received') }}</dd></div>
          <div><dt>累计传送</dt><dd>{{ formatNetworkTrafficCounter(sample.network, 'sent') }}</dd></div>
          <div><dt>运行时间</dt><dd>{{ formatDuration(sample.uptimeSeconds) }}</dd></div>
          <div v-if="managementHost"><dt>延迟</dt><dd>{{ latency }}</dd></div>
        </dl>
        <p class="cluster-globe__seen"><template v-if="managementHost">{{ phrase('最近在线') }} {{ relativeTime(managementHost.lastSuccessAt) }}<br /></template><span v-if="collectedAt">{{ phrase('采集于') }} {{ formatDateTime(collectedAt) }}</span></p>
        <p v-if="managementHost?.lastError" class="inline-alert inline-alert--warning" role="status" data-i18n-ignore>{{ managementHost.lastError }}</p>
        <p v-else-if="!sample" class="cluster-globe__unknown">等待首次主机摘要</p>
        <div v-if="managementHost" class="cluster-globe__actions">
          <button class="button button--secondary button--small" type="button" @click="emit('manage', managementHost)"><Pencil :size="14" /> {{ phrase('管理') }}</button>
          <button v-if="managementHost.kind !== 'light_node'" class="button button--primary button--small" type="button" @click="emit('openPanel', managementHost)">{{ managementHost.isLocal ? phrase('当前面板') : phrase('打开面板') }}<ArrowUpRight :size="14" /></button>
        </div>
      </div>
    </aside>
    </div>
  </section>
</template>

<style scoped>
.cluster-globe {
  overflow: hidden;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  container-type: inline-size;
  color: var(--text);
}
.cluster-globe__layout { display: grid; grid-template-columns: minmax(0, 1fr) minmax(320px, 35%); }
.cluster-globe__world { min-width: 0; display: flex; flex-direction: column; background: radial-gradient(ellipse at 45% 42%, color-mix(in srgb, var(--brand) 7%, transparent), transparent 68%), var(--surface-subtle); }
.cluster-globe__heading { display: flex; justify-content: space-between; gap: 16px; padding: 24px 24px 0; }
.cluster-globe__eyebrow { display: inline-flex; align-items: center; gap: 8px; color: var(--brand-strong); font-size: .875rem; font-weight: 600; }
.cluster-globe__heading h2 { margin: 8px 0 0; font-size: clamp(1.125rem, 2cqi, 1.5rem); line-height: 1.4; font-weight: 600; }
.cluster-globe__coverage { display: flex; flex-direction: column; align-items: flex-end; flex-shrink: 0; }
.cluster-globe__coverage strong { font-size: 1.75rem; line-height: 1.2; font-weight: 600; }
.cluster-globe__coverage span { color: var(--text-soft); font-size: .8125rem; }
.cluster-globe__stage { position: relative; flex: 1; min-height: 280px; }
.cluster-globe__canvas { display: block; width: 100%; height: clamp(320px, 45cqi, 540px); cursor: grab; touch-action: pan-y; }
.cluster-globe__canvas:active { cursor: grabbing; }
.cluster-globe__canvas:focus-visible { outline: 2px solid var(--brand); outline-offset: -5px; border-radius: var(--radius); }
.cluster-globe__fallback { position: absolute; inset: 30% 15% auto; font-size: .875rem; text-align: center; }
.cluster-globe__legend { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding: 0 24px 20px; }
.cluster-globe__legend-items { display: flex; flex-wrap: wrap; gap: 16px; color: var(--text-soft); font-size: .8125rem; }
.cluster-globe__legend-items span { display: inline-flex; align-items: center; gap: 6px; }
.cluster-globe__legend-items i { width: 7px; height: 7px; border-radius: 50%; }
.cluster-globe__legend-items .is-online { background: var(--success); }
.cluster-globe__legend-items .is-attention { background: var(--warning); }
.cluster-globe__controls { display: flex; gap: 8px; align-items: center; }
.cluster-globe__zoom { display: flex; align-items: center; gap: 2px; }
.cluster-globe__zoom span { min-width: 42px; text-align: center; font-size: .8125rem; font-variant-numeric: tabular-nums; color: var(--text-soft); }
.cluster-globe__legend p { flex-basis: 100%; margin: 0; color: var(--text-soft); font-size: .8125rem; line-height: 1.6; }
.cluster-globe__sidebar { display: flex; flex-direction: column; min-width: 0; border-left: 1px solid var(--border); }
.cluster-globe__detail { padding: 20px; }
.cluster-globe__detail-heading { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: .8125rem; color: var(--text-soft); }
.cluster-globe__detail h3 { margin: 12px 0 8px; font-size: 1.25rem; line-height: 1.4; overflow-wrap: anywhere; }
.cluster-globe__location { display: flex; align-items: flex-start; gap: 6px; margin: 0; font-size: .8125rem; color: var(--text-soft); overflow-wrap: anywhere; }
.cluster-globe__location svg { margin-top: 2px; flex-shrink: 0; }
.cluster-globe__metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 4.5rem), 1fr)); gap: 14px; margin: 20px 0 14px; }
.cluster-globe__metrics span { display: block; font-size: .8125rem; color: var(--text-soft); }
.cluster-globe__metrics strong { display: block; margin: 4px 0 8px; font-size: 1.125rem; font-variant-numeric: tabular-nums; }
.cluster-globe__metrics small { display: block; margin-top: 8px; color: var(--text-soft); font-size: .8125rem; line-height: 1.5; overflow-wrap: anywhere; }
.cluster-globe__metrics small span { display: inline-block; font-size: inherit; white-space: nowrap; }
.cluster-globe__meter { height: 3px; border-radius: var(--radius-sm); background: var(--surface-subtle); overflow: hidden; }
.cluster-globe__meter i { display: block; height: 100%; background: var(--brand); }
.cluster-globe__host-details { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 7rem), 1fr)); gap: 14px 16px; margin: 20px 0; padding-block: 16px; border-block: 1px solid var(--border); }
.cluster-globe__host-details > div { min-width: 0; }
.cluster-globe__system, .cluster-globe__isp { grid-column: 1 / -1; }
.cluster-globe__host-details dt { margin-bottom: 4px; color: var(--text-soft); font-size: .8125rem; }
.cluster-globe__host-details dd { margin: 0; font-size: .875rem; line-height: 1.5; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
.cluster-globe__host-details small { display: block; margin-top: 4px; font-size: .8125rem; color: var(--text-soft); }
.cluster-globe__seen, .cluster-globe__unknown { margin: 10px 0 0; color: var(--text-soft); font-size: .8125rem; line-height: 1.6; }
.cluster-globe__actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
.cluster-globe .button { font-size: .875rem; }
.cluster-globe__nodes-heading { display: flex; justify-content: space-between; padding: 20px 20px 14px; font-size: .875rem; }
.cluster-globe__nodes-heading span { color: var(--text-soft); }
.cluster-globe__explorer { display: flex; flex: 1; flex-direction: column; min-width: 0; }
.cluster-globe__filters { display: grid; gap: 12px; padding: 0 16px 16px; border-bottom: 1px solid var(--border); }
.cluster-globe__search { display: flex; align-items: center; gap: 8px; padding: 0 10px; min-height: 40px; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--muted); background: var(--surface-subtle); }
.cluster-globe__search input { min-width: 0; width: 100%; padding: 8px 0; border: 0; border-radius: 0; box-shadow: none; outline: none; background: transparent; color: var(--text); font-size: .875rem; }
.cluster-globe__search:focus-within { outline: 2px solid var(--brand); outline-offset: 2px; }
.cluster-globe__status-filters { display: flex; flex-wrap: wrap; gap: 6px; }
.cluster-globe__status-filters button { display: flex; align-items: center; justify-content: center; flex: 1; gap: 6px; min-height: 36px; padding: 6px 8px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text-soft); cursor: pointer; font-size: .875rem; white-space: nowrap; }
.cluster-globe__status-filters button[aria-pressed="true"] { border-color: var(--brand-muted); background: var(--brand-soft); color: var(--brand-strong); }
.cluster-globe__status-filters span { font-size: .75rem; font-variant-numeric: tabular-nums; }
.cluster-globe__region-filter { display: flex; align-items: center; gap: 8px; color: var(--text-soft); }
.cluster-globe__region-filter select { flex: 1; min-width: 0; width: 100%; min-height: 36px; padding: 6px 8px; font-size: .875rem; color: var(--text); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); }
.cluster-globe__nodes { overflow: auto; max-height: clamp(320px, 43cqi, 520px); min-height: 220px; padding: 8px; flex: 1; scrollbar-gutter: stable; overscroll-behavior: contain; }
.cluster-globe__node { display: grid; grid-template-columns: 22px minmax(0, 1fr) auto; align-items: center; gap: 10px; width: 100%; border: 1px solid transparent; border-bottom-color: var(--border); border-radius: var(--radius-sm); background: transparent; color: var(--text); padding: 12px 8px; min-height: 76px; text-align: left; font-size: .875rem; cursor: pointer; }
.cluster-globe__node:hover { background: var(--surface-subtle); }
.cluster-globe__node.is-selected { background: var(--brand-soft); border-color: color-mix(in srgb, var(--brand) 30%, var(--border)); }
.cluster-globe__node:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
.cluster-globe__node-body { display: grid; min-width: 0; gap: 7px; }
.cluster-globe__node-name { overflow-wrap: anywhere; font-weight: 600; line-height: 1.5; }
.cluster-globe__node-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; color: var(--text-soft); font-size: .75rem; overflow-wrap: anywhere; }
.cluster-globe__node-metrics { display: grid; gap: 8px; color: var(--text-soft); font-size: .75rem; }
.cluster-globe__node-metrics > span { display: flex; justify-content: space-between; gap: 8px; }
.cluster-globe__node-metrics b { color: var(--text); font-size: .8125rem; font-weight: 500; font-variant-numeric: tabular-nums; }
.cluster-globe__detail-nav { display: flex; align-items: center; gap: 6px; padding-bottom: 16px; margin-bottom: 16px; border-bottom: 1px solid var(--border); }
.cluster-globe__detail-nav > span { margin-left: auto; color: var(--text-soft); font-size: .8125rem; }
.cluster-globe__back { display: inline-flex; gap: 6px; align-items: center; min-height: 36px; padding: 0; color: var(--brand-strong); background: transparent; border: 0; font-size: .875rem; cursor: pointer; }
.cluster-globe__empty { display: grid; place-items: center; align-content: center; gap: 16px; min-height: 300px; color: var(--text-soft); font-size: .875rem; }
.cluster-globe__node > svg, .cluster-globe__node :deep(.country-flag) { flex-shrink: 0; }
@container (max-width: 760px) {
  .cluster-globe__heading { padding: 20px 16px 0; }
  .cluster-globe__legend { padding: 0 16px 16px; }
}
@container (max-width: 820px) {
  .cluster-globe__layout { grid-template-columns: minmax(0, 1fr); }
  .cluster-globe__sidebar { border-left: 0; border-top: 1px solid var(--border); }
  .cluster-globe__nodes { max-height: 400px; }
  .cluster-globe__detail { padding: 16px; }
  .cluster-globe__controls { flex-wrap: wrap; }
}
@container (max-width: 30rem) {
  .cluster-globe__node { grid-template-columns: 22px minmax(0, 1fr); }
  .cluster-globe__node-metrics { grid-column: 2; display: flex; flex-wrap: wrap; gap: 8px 16px; }
}
</style>
