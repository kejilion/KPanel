<script setup lang="ts">
import ClusterShareTheme from '@/components/cluster/ClusterShareTheme.vue'
import ClusterHostDetails from '@/components/cluster/ClusterHostDetails.vue'
import ClusterHostRegionInfo from '@/components/cluster/ClusterHostRegionInfo.vue'
import ClusterHostSystemInfo from '@/components/cluster/ClusterHostSystemInfo.vue'
import ClusterRemainingValue from '@/components/cluster/ClusterRemainingValue.vue'
import ClusterTemporarySortMenu from '@/components/cluster/ClusterTemporarySortMenu.vue'
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  Activity,
  ArrowDown,
  ArrowDownUp,
  ArrowUp,
  Clock3,
  Gauge,
  Globe2,
  HardDrive,
  LayoutGrid,
  LayoutList,
  MemoryStick,
  Moon,
  RefreshCw,
  Server,
  Search,
  Sun,
} from '@lucide/vue'
import LogoMark from '@/components/common/LogoMark.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { usePhraseCatalog } from '@/i18n/phrase'
import { useI18n } from '@/i18n'
import { sortPublicClusterHostsTemporarily, type ClusterHostTemporarySortKey, type ClusterHostTemporarySortDirection } from '@/lib/clusterHostTemporarySort'
import { ApiError, api } from '@/lib/api'
import { clusterTrafficCounters, formatNetworkTrafficCounter } from '@/lib/networkTraffic'
import { formatCapacityPair, usageTone } from '@/lib/clusterHostIdentity'
import ClusterTrafficHeading from '@/components/cluster/ClusterTrafficHeading.vue'
import {
  clampPercent,
  formatDateTime,
  formatDuration,
  formatPercent,
  formatRate,
  relativeTime,
} from '@/lib/format'
import { useTheme } from '@/stores/theme'
import type { PublicClusterShareHost, PublicClusterShareSnapshot } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ClusterShareView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ClusterShareView/zh-TW').then((module) => module.default))

const route = useRoute()
const snapshot = ref<PublicClusterShareSnapshot>()
const publicDetails = computed(() => Object.fromEntries((snapshot.value?.items || []).map(host => [host.id, {
  price: host.price, expiresOn: host.expiresOn,
}])))
const loading = ref(true)
const refreshing = ref(false)
const immersive = ref(false)
const errorMessage = ref('')
type ShareViewMode = 'list' | 'card' | 'globe'
const viewMode = ref<ShareViewMode>('list')
const viewModeStorageKey = 'kpanel:cluster-share-view'
const search = ref('')
const { t } = useI18n()
const sortKey = ref<ClusterHostTemporarySortKey>('custom')
const sortDirection = ref<ClusterHostTemporarySortDirection>('asc')
const sortOptions = computed(() => [
  { value: 'custom' as const, label: t('cluster.details.defaultOrder') },
  { value: 'cpu' as const, label: t('cluster.details.sortCPU') },
  { value: 'memory' as const, label: t('cluster.details.sortMemory') },
  { value: 'disk' as const, label: t('cluster.details.sortDisk') },
  { value: 'traffic' as const, label: t('cluster.details.sortTraffic') },
  { value: 'expiresOn' as const, label: t('cluster.details.expiresOn') },
  { value: 'price' as const, label: t('cluster.details.price') },
])
const sortDirectionLabel = computed(() => sortKey.value === 'expiresOn'
  ? t(sortDirection.value === 'asc' ? 'cluster.details.sortEarlier' : 'cluster.details.sortLater')
  : t(sortDirection.value === 'asc' ? 'cluster.details.sortAsc' : 'cluster.details.sortDesc'))
function changeSort(key: ClusterHostTemporarySortKey): void {
  sortKey.value = key
  sortDirection.value = key === 'custom' || key === 'expiresOn' || key === 'price' ? 'asc' : 'desc'
}
const filteredHosts = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  const hosts = (snapshot.value?.items || []).filter(host => [host.name, host.os, host.location.country, host.location.city, host.location.isp].filter(Boolean).join(' ').toLowerCase().includes(keyword))
  return sortPublicClusterHostsTemporarily(hosts, sortKey.value, sortDirection.value)
})
const ClusterGlobe = defineAsyncComponent(() => import('@/components/cluster/ClusterGlobe.vue'))
const { resolved: resolvedTheme, setTheme } = useTheme()
let controller: AbortController | undefined
let pollTimer: number | undefined

const token = computed(() => String(route.params.token || ''))
const tokenIsValid = computed(() => /^[a-f0-9]{64}$/.test(token.value))

function setViewMode(mode: ShareViewMode): void {
  viewMode.value = mode
  try { window.localStorage.setItem(viewModeStorageKey, mode) } catch { /* Keep the current page choice. */ }
}

function restoreViewMode(): void {
  try {
    const stored = window.localStorage.getItem(viewModeStorageKey)
    if (stored === 'list' || stored === 'card' || stored === 'globe') viewMode.value = stored
  } catch { /* Default to the list when storage is unavailable. */ }
}

function toggleTheme(): void {
  setTheme(resolvedTheme.value === 'dark' ? 'light' : 'dark')
}

function stateLabel(state: PublicClusterShareHost['state']): string {
  return {
    online: '在线',
    degraded: '需关注',
    offline: '离线',
    pending: '等待数据',
  }[state]
}

function friendlyError(reason: unknown): string {
  if (reason instanceof ApiError && reason.status === 404) {
    return '分享链接无效、已关闭或已经重置。'
  }
  return '暂时无法读取集群状态，请稍后重试。'
}

async function load(silent = false): Promise<void> {
  // A background poll must not interrupt a request or animate the page controls.
  if (silent && controller) return
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  if (!snapshot.value) loading.value = true
  else if (!silent) refreshing.value = true
  if (!tokenIsValid.value) {
    snapshot.value = undefined
    errorMessage.value = '分享链接格式无效。'
    loading.value = false
    refreshing.value = false
    controller = undefined
    return
  }
  try {
    const nextSnapshot = await api.cluster.publicShare(token.value, requestController.signal)
    if (requestController.signal.aborted || controller !== requestController) return
    snapshot.value = nextSnapshot
    errorMessage.value = ''
  } catch (reason) {
    if (requestController.signal.aborted || controller !== requestController) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (reason instanceof ApiError && reason.status === 404) snapshot.value = undefined
    errorMessage.value = friendlyError(reason)
  } finally {
    if (controller === requestController) {
      controller = undefined
      loading.value = false
      refreshing.value = false
    }
  }
}

function onVisibilityChange(): void {
  if (!document.hidden) void load(true)
}

watch(token, () => { snapshot.value = undefined; void load() })

onMounted(() => {
  restoreViewMode()
  void load()
  pollTimer = window.setInterval(() => {
    if (!document.hidden) void load(true)
  }, 15_000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  controller?.abort()
  if (pollTimer) window.clearInterval(pollTimer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <main class="share-page" :class="{ 'share-page--immersive': immersive }">
    <template v-if="!immersive">
      <div class="share-page__glow share-page__glow--one" />
      <div class="share-page__glow share-page__glow--two" />
    </template>

    <div class="share-shell">
      <header v-if="!immersive" class="share-header">
        <a class="share-brand" href="https://github.com/kejilion/KPanel" target="_blank" rel="noopener noreferrer">
          <LogoMark compact class="share-brand__logo" />
          <strong>KPanel</strong>
        </a>
        <div class="share-header__actions">
          <button
            class="share-icon-button"
            type="button"
            :title="resolvedTheme === 'dark' ? '切换浅色模式' : '切换深色模式'"
            :aria-label="resolvedTheme === 'dark' ? '切换浅色模式' : '切换深色模式'"
            @click="toggleTheme"
          >
            <Sun v-if="resolvedTheme === 'dark'" :size="16" />
            <Moon v-else :size="16" />
          </button>
          <button
            class="share-refresh"
            type="button"
            :disabled="loading || refreshing"
            :aria-label="refreshing ? '正在刷新' : '刷新公开状态'"
            :aria-busy="loading || refreshing"
            @click="load()"
          >
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
            <span>刷新</span>
          </button>
        </div>
      </header>

      <ClusterShareTheme :snapshot="snapshot" :error-message="errorMessage" @immersive="immersive = $event" @refresh="load()">
      <section v-if="snapshot" class="share-hero">
        <div class="share-hero__copy">
          <span class="share-kicker"><Globe2 :size="14" /> PUBLIC FLEET</span>
          <h1>{{ snapshot.title }}</h1>
          <p>{{ snapshot.description || '这些是我正在运行的服务器。' }}</p>
          <small>数据生成于 {{ formatDateTime(snapshot.generatedAt) }} · {{ relativeTime(snapshot.generatedAt) }}</small>
        </div>
        <div class="share-stats" aria-label="集群状态概览">
          <div><strong>{{ snapshot.total }}</strong><span>全部机器</span></div>
          <div class="is-online"><strong>{{ snapshot.online }}</strong><span>在线</span></div>
          <div class="is-attention"><strong>{{ snapshot.attention }}</strong><span>需关注</span></div>
          <ClusterRemainingValue :hosts="snapshot.items" :details="publicDetails" read-only />
        </div>
      </section>

      <div v-if="snapshot?.items.length" class="share-toolbar">
        <label class="share-search"><Search :size="17" /><input v-model="search" type="search" aria-label="搜索公开主机" placeholder="搜索名称、地区或系统…" /></label>
        <div class="share-sort">
          <ClusterTemporarySortMenu :model-value="sortKey" :options="sortOptions" :label="t('cluster.details.sortLabel')" :prefix="t('cluster.details.sortPrefix')" :title="sortKey === 'price' ? t('cluster.details.priceSortHint') : undefined" @update:model-value="changeSort" />
          <button type="button" class="share-sort__direction" :disabled="sortKey === 'custom'" :title="sortDirectionLabel" :aria-label="sortDirectionLabel" @click="sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'">
            <ArrowUp v-if="sortDirection === 'asc'" :size="15" aria-hidden="true" /><ArrowDown v-else :size="15" aria-hidden="true" />
          </button>
        </div>
        <div class="share-view-switch" role="group" aria-label="机器排列方式">
          <button
            type="button"
            :class="{ 'is-active': viewMode === 'list' }"
            :aria-pressed="viewMode === 'list'"
            title="列表排列"
            @click="setViewMode('list')"
          >
            <LayoutList :size="15" /> <span>列表</span>
          </button>
          <button
            type="button"
            :class="{ 'is-active': viewMode === 'card' }"
            :aria-pressed="viewMode === 'card'"
            title="卡片排列"
            @click="setViewMode('card')"
          >
            <LayoutGrid :size="15" /> <span>卡片</span>
          </button>
          <button
            type="button"
            :class="{ 'is-active': viewMode === 'globe' }"
            :aria-pressed="viewMode === 'globe'"
            title="地球展示"
            @click="setViewMode('globe')"
          >
            <Globe2 :size="15" /> <span>地球</span>
          </button>
        </div>
      </div>

      <section v-if="loading && !snapshot" class="share-state" aria-live="polite">
        <RefreshCw class="spin" :size="24" />
        <strong>正在读取机器状态…</strong>
      </section>

      <section v-else-if="errorMessage && !snapshot" class="share-state share-state--error" role="alert">
        <Activity :size="25" />
        <strong>无法打开分享页</strong>
        <p>{{ errorMessage }}</p>
        <button type="button" @click="load()">重试</button>
      </section>

      <div v-else-if="errorMessage" class="share-warning" role="status">
        <span>{{ errorMessage }}</span> <span>当前保留上一次成功数据。</span>
      </div>

      <section v-if="snapshot?.items.length && !filteredHosts.length" class="share-state"><Search :size="24" /><strong>没有匹配的主机</strong><button type="button" @click="search = ''">清除搜索</button></section>

      <ClusterGlobe v-else-if="snapshot?.items.length && viewMode === 'globe'" :hosts="filteredHosts" :searchable="false" />

      <section
        v-else-if="filteredHosts.length"
        class="share-grid"
        :class="`is-${viewMode}`"
        :aria-label="viewMode === 'list' ? '公开机器行列表' : '公开机器卡片列表'"
      >
        <article v-for="host in filteredHosts" :key="host.id" class="share-card" :class="`is-state-${host.state}`">
          <header class="share-card__header">
            <ClusterHostSystemInfo class="share-card__system" :system="host" />
            <div class="share-card__identity">
              <span class="share-card__title">
                <ClusterHostRegionInfo class="share-card__region" :location="host.location" shared />
                <h2>{{ host.name }}</h2>
                <StatusBadge :status="host.state" :label="stateLabel(host.state)" subtle />
              </span>
              <small class="share-card__collected">{{ host.collectedAt ? `采集于 ${relativeTime(host.collectedAt)}` : '尚无数据' }}</small>
              <ClusterHostDetails :details="host" />
            </div>
          </header>

          <div v-if="host.collectedAt" class="share-metrics">
            <div>
              <span><Gauge :size="14" /> CPU</span>
              <strong>{{ formatPercent(host.cpu.usagePercent) }}</strong>
              <i :class="`is-${usageTone(host.cpu.usagePercent)}`"><b :style="{ width: `${clampPercent(host.cpu.usagePercent)}%` }" /></i>
              <small>{{ host.cpu.cores }} 核</small>
            </div>
            <div>
              <span><MemoryStick :size="14" /> 内存</span>
              <strong>{{ formatPercent(host.memory.usagePercent) }}</strong>
              <i :class="`is-${usageTone(host.memory.usagePercent)}`"><b :style="{ width: `${clampPercent(host.memory.usagePercent)}%` }" /></i>
              <small>{{ formatCapacityPair(host.memory.usedBytes, host.memory.totalBytes) }}</small>
            </div>
            <div>
              <span><HardDrive :size="14" /> 磁盘</span>
              <strong>{{ formatPercent(host.disk.usagePercent) }}</strong>
              <i :class="`is-${usageTone(host.disk.usagePercent)}`"><b :style="{ width: `${clampPercent(host.disk.usagePercent)}%` }" /></i>
              <small>{{ formatCapacityPair(host.disk.usedBytes, host.disk.totalBytes) }}</small>
            </div>
          </div>
          <div v-else class="share-card__empty">等待第一份状态数据</div>

          <dl class="share-details">
            <div class="share-details__rate">
              <dt><Activity :size="14" /> 实时网速</dt>
              <dd>
                <span class="share-flow is-down" title="实时下行">
                  <ArrowDown :size="13" aria-hidden="true" />
                  <span class="sr-only">实时下行</span>
                  {{ formatRate(host.network.receiveBytesPerSecond || 0) }}
                </span>
                <span class="share-flow is-up" title="实时上行">
                  <ArrowUp :size="13" aria-hidden="true" />
                  <span class="sr-only">实时上行</span>
                  {{ formatRate(host.network.transmitBytesPerSecond || 0) }}
                </span>
              </dd>
            </div>
            <div class="share-details__traffic">
              <dt><ArrowDownUp :size="14" /> <ClusterTrafficHeading :period="host.trafficPeriod" :details="host" /></dt>
              <dd>
                <span class="share-flow is-down" title="累计接收">
                  <ArrowDown :size="13" aria-hidden="true" />
                  <span class="sr-only">累计接收</span>
                  {{ host.collectedAt ? formatNetworkTrafficCounter(clusterTrafficCounters(host), 'received') : '—' }}
                </span>
                <span class="share-flow is-up" title="累计传送">
                  <ArrowUp :size="13" aria-hidden="true" />
                  <span class="sr-only">累计传送</span>
                  {{ host.collectedAt ? formatNetworkTrafficCounter(clusterTrafficCounters(host), 'sent') : '—' }}
                </span>
              </dd>
            </div>
            <div class="share-details__uptime">
              <dt><Clock3 :size="14" /> 运行时间</dt>
              <dd>{{ host.uptimeSeconds ? formatDuration(host.uptimeSeconds) : '—' }}</dd>
            </div>
          </dl>
        </article>
      </section>

      <section v-else-if="snapshot" class="share-state">
        <Server :size="26" />
        <strong>还没有可展示的机器</strong>
      </section>

      </ClusterShareTheme>

      <footer v-if="!immersive" class="share-footer">
        <span>Powered by <strong>KPanel</strong></span>
        <span>公开页不包含 IP、管理入口或访问凭据</span>
      </footer>
    </div>
  </main>
</template>

<style scoped>
.share-page {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  color: var(--text);
  background:
    radial-gradient(circle at 16% -8%, color-mix(in srgb, var(--brand) 14%, transparent), transparent 34%),
    radial-gradient(circle at 92% 4%, color-mix(in srgb, var(--blue) 9%, transparent), transparent 30%),
    var(--bg);
  transition: color 0.2s ease, background 0.2s ease;
}

.share-page__glow {
  position: fixed;
  width: 440px;
  height: 440px;
  pointer-events: none;
  filter: blur(100px);
  opacity: 0.1;
  border-radius: 50%;
}

.share-page--immersive { min-height: 0; background: none; }
.share-page--immersive .share-shell { width: 0; height: 0; padding: 0; }

.share-page__glow--one { top: 10%; right: -180px; background: var(--brand); }
.share-page__glow--two { bottom: -240px; left: -160px; background: var(--blue); }

.share-shell {
  position: relative;
  z-index: 1;
  width: min(1280px, calc(100% - 40px));
  margin: 0 auto;
  padding: 18px 0 26px;
}

.share-header,
.share-header__actions,
.share-brand,
.share-refresh,
.share-icon-button,
.share-view-switch,
.share-view-switch button,
.share-footer {
  display: flex;
  align-items: center;
}

.share-header { justify-content: space-between; gap: 18px; margin-bottom: 18px; }
.share-header__actions { justify-content: flex-end; flex-wrap: wrap; gap: 9px; }
.share-brand { gap: 10px; color: inherit; text-decoration: none; letter-spacing: 0.02em; }
.share-brand__logo {
  flex: 0 0 auto;
  gap: 0;
}

.share-brand__logo :deep(.brand__mark) {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  box-shadow: var(--shadow-button);
}

.share-refresh,
.share-icon-button,
.share-state button {
  min-height: 38px;
  gap: 7px;
  padding: 9px 13px;
  color: var(--text-soft);
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-sm);
  cursor: pointer;
}

.share-icon-button { width: 38px; justify-content: center; padding: 0; }
.share-refresh:hover,
.share-icon-button:hover { color: var(--brand-strong); border-color: var(--brand-muted); }
.share-refresh:disabled { cursor: wait; opacity: 0.58; }

/* The hero follows the cluster summary band: one tinted surface, a quiet ring, clear dividers. */
.share-hero {
  position: relative;
  display: grid;
  overflow: hidden;
  isolation: isolate;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 20px;
  padding: clamp(20px, 2vw, 26px);
  margin-bottom: 16px;
  background:
    radial-gradient(circle at 86% 0%, color-mix(in srgb, var(--brand) 10%, transparent), transparent 34%),
    linear-gradient(110deg, color-mix(in srgb, var(--brand) 6%, var(--surface)) 0%, var(--surface) 58%);
  border: 1px solid color-mix(in srgb, var(--brand) 22%, var(--border));
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
}

.share-hero::before {
  position: absolute;
  z-index: 0;
  top: 50%;
  right: 42px;
  width: 200px;
  height: 200px;
  border: 30px solid color-mix(in srgb, var(--brand) 6%, transparent);
  border-radius: 50%;
  content: '';
  pointer-events: none;
  transform: translateY(-50%);
}

.share-hero > * { position: relative; z-index: 1; }

.share-kicker { display: flex; align-items: center; gap: 7px; color: var(--brand-strong); font-size: 12px; font-weight: 700; letter-spacing: 0.16em; }
.share-hero h1 { margin: 8px 0 6px; font-size: clamp(28px, 3vw, 36px); line-height: 1.15; }
.share-hero p { max-width: 670px; margin: 0 0 8px; color: var(--text-soft); font-size: 14px; line-height: 1.6; }
.share-hero small { color: var(--muted); font-size: .8125rem; }

.share-stats { display: grid; grid-template-columns: repeat(3, minmax(80px, 1fr)); align-items: center; }
.share-stats:has(> .cluster-value) { grid-template-columns: repeat(3, minmax(80px, 1fr)) minmax(150px, 1.6fr); }
.share-stats > div { display: grid; gap: 4px; padding: 4px 18px; text-align: center; border-left: 1px solid var(--border); }
.share-stats > div:first-child { border-left: 0; }
.share-stats strong { font-size: 25px; line-height: 1.1; font-variant-numeric: tabular-nums; }
.share-stats span { color: var(--muted); font-size: 12px; }
.share-stats .is-online strong { color: var(--success); }
.share-stats .is-attention strong { color: var(--warning); }

.share-toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 12px; margin-bottom: 16px; }
.share-search { display: flex; align-items: center; gap: 10px; max-width: 520px; min-width: 0; padding: 0 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-soft); background: var(--surface); }
.share-search input { min-width: 0; width: 100%; min-height: 40px; padding: 9px 0; font-size: .875rem; color: var(--text); background: transparent; border: 0; outline: none; box-shadow: none; }
.share-search:focus-within { border-color: var(--brand); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 16%, transparent); }
.share-sort { display: flex; min-width: 0; min-height: 40px; align-items: stretch; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); }
.share-sort:focus-within { border-color: var(--brand); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 16%, transparent); }
.share-sort__direction { display: grid; place-items: center; min-width: 40px; padding: 0; color: var(--muted); background: transparent; border: 0; border-left: 1px solid var(--border); cursor: pointer; }
.share-sort__direction:hover:not(:disabled) { color: var(--brand); background: var(--brand-soft); }
.share-sort__direction:disabled { opacity: .4; cursor: not-allowed; }
.share-sort__direction:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }

.share-view-switch {
  gap: 3px;
  padding: 3px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.share-view-switch button {
  min-height: 32px;
  gap: 6px;
  padding: 0 10px;
  color: var(--muted);
  background: transparent;
  border: 0;
  border-radius: calc(var(--radius-sm) - 3px);
  cursor: pointer;
  font-size: .875rem;
  font-weight: 600;
}

.share-view-switch button:hover { color: var(--text); background: var(--interaction-hover); }
.share-view-switch button.is-active { color: var(--brand); background: var(--brand-soft); }

@media (max-width: 900px) {
  .share-toolbar { grid-template-columns: minmax(0, 1fr) auto; }
  .share-search { grid-column: 1 / -1; max-width: none; }
}
@media (max-width: 560px) {
  .share-toolbar { grid-template-columns: minmax(0, 1fr); }
  .share-view-switch button { flex: 1; justify-content: center; }
}

/* Host cards mirror the cluster page: identity, metric tiles, then live and cumulative traffic. */
.share-grid {
  display: grid;
  container: share-layout / inline-size;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 22.5rem), 1fr));
  gap: 16px;
}

.share-grid.is-list { grid-template-columns: minmax(0, 1fr); gap: 10px; }

.share-card {
  position: relative;
  display: flex;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  background: color-mix(in srgb, var(--surface) 96%, transparent);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  transition: border-color 160ms ease, box-shadow 160ms ease;
}

/* The state accent mirrors the status badge tone; the badge text stays the primary signal. */
.share-card::before {
  position: absolute;
  z-index: 1;
  inset: 0 auto 0 0;
  width: 3px;
  background: var(--share-card-accent, transparent);
  content: '';
  pointer-events: none;
}

.share-card.is-state-degraded { --share-card-accent: var(--warning); }
.share-card.is-state-offline { --share-card-accent: var(--danger); }
.share-card:hover { border-color: color-mix(in srgb, var(--brand) 24%, var(--border)); box-shadow: var(--shadow-md); }

/* Cards in one row share section tracks, so a wrapped name never shifts the meters. */
@supports (grid-template-rows: subgrid) {
  .share-grid.is-card { row-gap: 0; }
  .share-grid.is-card .share-card { display: grid; grid-row: span 3; grid-template-rows: subgrid; row-gap: 0; margin-bottom: 16px; }
}

.share-card__header { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 12px; padding: 14px 14px 12px; }
.share-card__header :deep(.os-identity__mark) { width: 40px; height: 40px; border-radius: var(--radius); }
.share-card__header :deep(.os-identity__mark svg),
.share-card__header :deep(.os-identity__mark img) { width: 22px; height: 22px; }
.share-card__identity { display: grid; min-width: 0; gap: 3px; }
.share-card__title { display: flex; flex-wrap: wrap; min-width: 0; align-items: center; gap: 6px 8px; }
.share-card__title :deep(.country-flag) { width: 20px; height: 20px; }
.share-card h2 { min-width: 0; margin: 0; font-size: 1rem; font-weight: 600; line-height: 1.4; overflow-wrap: anywhere; }
.share-card__collected { color: var(--muted); font-size: .8125rem; line-height: 1.45; font-variant-numeric: tabular-nums; }

.share-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; padding: 0 14px 10px; }
.share-metrics > div { display: grid; min-width: 0; align-content: start; gap: 6px; padding: 10px 11px; background: var(--surface-subtle); border: 1px solid color-mix(in srgb, var(--border) 70%, transparent); border-radius: var(--radius); }
.share-metrics span { display: inline-flex; min-width: 0; align-items: center; gap: 5px; color: var(--text-soft); font-size: .8125rem; line-height: 1.4; }
.share-metrics strong { font-size: 1.125rem; font-weight: 600; line-height: 1.25; font-variant-numeric: tabular-nums; }
.share-metrics small { color: var(--text-soft); font-size: .75rem; line-height: 1.45; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.share-metrics > div > i { display: block; height: 4px; overflow: hidden; background: color-mix(in srgb, var(--text-soft) 14%, transparent); border-radius: 999px; }
.share-metrics b { display: block; height: 100%; background: linear-gradient(90deg, var(--brand), #3bbfa3); border-radius: inherit; transition: width 300ms ease; }
.share-metrics i.is-warning b { background: var(--warning); }
.share-metrics i.is-danger b { background: var(--danger); }
.share-card__empty { display: grid; min-height: 120px; place-items: center; padding: 16px 20px; color: var(--muted); text-align: center; }

.share-details { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); align-items: start; gap: 8px; padding: 0 14px 14px; margin: 0; font-variant-numeric: tabular-nums; }
.share-details > div { display: grid; min-width: 0; align-content: start; gap: 3px; padding: 6px 11px; }
.share-details dt { display: inline-flex; min-width: 0; align-items: center; gap: 5px; margin: 0; color: var(--text-soft); font-size: .8125rem; line-height: 1.4; }
.share-details dd { display: grid; gap: 3px; margin: 0; font-size: .875rem; font-weight: 500; line-height: 1.45; overflow-wrap: anywhere; }
.share-details__rate dd { font-weight: 600; }
.share-flow { display: flex; min-width: 0; align-items: center; gap: 4px; }
.share-flow > svg { flex: 0 0 auto; }
.share-flow.is-down > svg { color: var(--blue); }
.share-flow.is-up > svg { color: var(--success); }

/* Rows: identity above the data while mid-width, one line once every group fits. */
@container share-layout (min-width: 42.5rem) {
  .share-grid.is-list .share-card {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.15fr);
    grid-template-areas:
      "header header"
      "metrics details";
    align-items: stretch;
  }

  .share-grid.is-list .share-card__header { grid-area: header; padding: 12px 14px; }
  .share-grid.is-list .share-metrics { grid-area: metrics; gap: 0; padding: 0; border-top: 1px solid var(--border); border-right: 1px solid var(--border); }
  .share-grid.is-list .share-metrics > div { align-content: center; padding: 12px 14px; background: transparent; border: 0; border-radius: 0; }
  .share-grid.is-list .share-details { grid-area: details; align-content: stretch; gap: 0; padding: 0; border-top: 1px solid var(--border); }
  .share-grid.is-list .share-details > div { align-content: center; padding: 12px 14px; }
  .share-grid.is-list .share-card__empty { grid-column: 1 / -1; grid-row: 2; min-height: 92px; border-top: 1px solid var(--border); }
}

@container share-layout (min-width: 62rem) {
  .share-grid.is-list .share-card {
    grid-template-columns:
      minmax(16rem, 1.1fr)
      minmax(18rem, 1.15fr)
      minmax(21rem, 1.35fr);
    grid-template-areas: "header metrics details";
  }

  .share-grid.is-list .share-card__header { border-right: 1px solid var(--border); }
  .share-grid.is-list .share-metrics,
  .share-grid.is-list .share-details { border-top: 0; }
  .share-grid.is-list .share-metrics > div,
  .share-grid.is-list .share-details > div { padding: 12px 8px; }
  .share-grid.is-list .share-metrics > div:first-child,
  .share-grid.is-list .share-details > div:first-child { padding-left: 14px; }
  .share-grid.is-list .share-card__empty { grid-area: 1 / 2 / 2 / 4; border-top: 0; }
}

@container share-layout (max-width: 30rem) {
  .share-card__header { padding: 12px 12px 10px; }
  .share-metrics,
  .share-details { padding-inline: 10px; gap: 6px; }
}

.share-state { display: grid; min-height: 280px; place-items: center; align-content: center; gap: 12px; color: var(--muted); text-align: center; }
.share-state p { margin: 0; }
.share-state--error svg { color: var(--danger); }
.share-warning { padding: 12px 15px; margin-bottom: 14px; color: var(--amber); background: var(--amber-soft); border: 1px solid color-mix(in srgb, var(--amber) 28%, var(--border)); border-radius: var(--radius); }
.share-footer { justify-content: space-between; gap: 20px; padding: 28px 4px 0; color: var(--muted); font-size: 12px; }
.share-footer strong { color: var(--text-soft); }

@media (max-width: 1000px) {
  .share-hero { grid-template-columns: 1fr; align-items: start; gap: 16px; }
  .share-hero::before { display: none; }
}

@media (max-width: 650px) {
  .share-shell { width: min(100% - 24px, 1280px); padding-top: 16px; }
  .share-header { margin-bottom: 18px; }
  .share-header__actions { gap: 6px; }
  .share-view-switch button { padding-inline: 8px; }
  .share-refresh span { display: none; }
  .share-hero { gap: 14px; padding: 18px 16px; }
  .share-hero h1 { font-size: 28px; }
  .share-stats, .share-stats:has(> .cluster-value) { width: 100%; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .share-stats > .cluster-value { grid-column: 1 / -1; margin-top: .75rem; padding-top: .75rem; border-left: 0; border-top: 1px solid var(--border); }
  .share-stats > div { padding: 2px 12px; }
  .share-footer { align-items: flex-start; flex-direction: column; }
}

@media (max-width: 430px) {
  .share-header { align-items: center; flex-wrap: wrap; }
  .share-brand strong { display: none; }
  .share-stats > div { padding-inline: 8px; }
}

@media (prefers-reduced-motion: reduce) {
  .spin { animation: none; }
  .share-card,
  .share-metrics b { transition: none; }
}
</style>
