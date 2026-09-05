<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Clock3,
  Gauge,
  Globe2,
  HardDrive,
  LayoutGrid,
  LayoutList,
  MapPin,
  MemoryStick,
  Moon,
  RefreshCw,
  Server,
  Search,
  Sun,
} from '@lucide/vue'
import LogoMark from '@/components/common/LogoMark.vue'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { usePhraseCatalog } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatNetworkTrafficCounter } from '@/lib/networkTraffic'
import {
  clampPercent,
  formatDateTime,
  formatDuration,
  formatPercent,
  formatRate,
  relativeTime,
} from '@/lib/format'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import { useTheme } from '@/stores/theme'
import type { PublicClusterShareHost, PublicClusterShareSnapshot } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ClusterShareView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ClusterShareView/zh-TW').then((module) => module.default))

const route = useRoute()
const snapshot = ref<PublicClusterShareSnapshot>()
const loading = ref(true)
const refreshing = ref(false)
const errorMessage = ref('')
type ShareViewMode = 'list' | 'card' | 'globe'
const viewMode = ref<ShareViewMode>('list')
const viewModeStorageKey = 'kpanel:cluster-share-view'
const search = ref('')
const filteredHosts = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  return (snapshot.value?.items || []).filter(host => [host.name, host.os, host.location.country, host.location.city, host.location.isp].filter(Boolean).join(' ').toLowerCase().includes(keyword))
})
const ClusterGlobe = defineAsyncComponent(() => import('@/components/cluster/ClusterGlobe.vue'))
const { resolved: resolvedTheme, setTheme } = useTheme()
let controller: AbortController | undefined
let pollTimer: number | undefined

const token = computed(() => String(route.params.token || ''))
const tokenIsValid = computed(() => /^[a-f0-9]{64}$/.test(token.value))

function operatingSystemIdentity(host: PublicClusterShareHost) {
  return detectOperatingSystemIdentity({ os: host.os })
}

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

function locationLabel(host: PublicClusterShareHost): string {
  return [host.location.country, host.location.region, host.location.city]
    .filter((value, index, items) => value && items.indexOf(value) === index)
    .join(' · ') || '地区未公开'
}

function friendlyError(reason: unknown): string {
  if (reason instanceof ApiError && reason.status === 404) {
    return '分享链接无效、已关闭或已经重置。'
  }
  return '暂时无法读取集群状态，请稍后重试。'
}

async function load(silent = false): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  if (!silent || !snapshot.value) loading.value = true
  else refreshing.value = true
  if (!tokenIsValid.value) {
    snapshot.value = undefined
    errorMessage.value = '分享链接格式无效。'
    loading.value = false
    refreshing.value = false
    return
  }
  try {
    snapshot.value = await api.cluster.publicShare(token.value, controller.signal)
    errorMessage.value = ''
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (reason instanceof ApiError && reason.status === 404) snapshot.value = undefined
    errorMessage.value = friendlyError(reason)
  } finally {
    loading.value = false
    refreshing.value = false
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
  <main class="share-page">
    <div class="share-page__glow share-page__glow--one" />
    <div class="share-page__glow share-page__glow--two" />

    <div class="share-shell">
      <header class="share-header">
        <a class="share-brand" href="https://github.com/kejilion/KPanel" target="_blank" rel="noopener noreferrer">
          <LogoMark compact class="share-brand__logo" />
          <strong>KPanel</strong>
        </a>
        <div class="share-header__actions">
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
            aria-label="刷新公开状态"
            @click="load(true)"
          >
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
            <span>{{ refreshing ? '正在刷新' : '刷新' }}</span>
          </button>
        </div>
      </header>

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
        </div>
      </section>

      <div v-if="snapshot?.items.length" class="share-toolbar"><label class="share-search"><Search :size="17" /><input v-model="search" type="search" aria-label="搜索公开主机" placeholder="搜索名称、地区或系统…" /></label><span>{{ filteredHosts.length }} / {{ snapshot.items.length }}</span></div>

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
        {{ errorMessage }} 当前保留上一次成功数据。
      </div>

      <section v-if="snapshot?.items.length && !filteredHosts.length" class="share-state"><Search :size="24" /><strong>没有匹配的主机</strong><button type="button" @click="search = ''">清除搜索</button></section>

      <ClusterGlobe v-else-if="snapshot?.items.length && viewMode === 'globe'" :hosts="filteredHosts" :searchable="false" />

      <section
        v-else-if="filteredHosts.length"
        class="share-grid"
        :class="`is-${viewMode}`"
        :aria-label="viewMode === 'list' ? '公开机器行列表' : '公开机器卡片列表'"
      >
        <article v-for="host in filteredHosts" :key="host.id" class="share-card">
          <header class="share-card__header">
            <OperatingSystemIcon
              :distro="operatingSystemIdentity(host).key"
              :label="operatingSystemIdentity(host).label"
            />
            <div class="share-card__identity">
              <h2>{{ host.name }}</h2>
              <p class="share-card__system">
                <span>{{ host.os || operatingSystemIdentity(host).label }}</span>
                <small v-if="host.architecture">{{ host.architecture }}</small>
              </p>
              <p class="share-card__location">
                <CountryFlagIcon
                  v-if="host.location.countryCode"
                  :country-code="host.location.countryCode"
                  :label="host.location.country || '地区'"
                />
                <MapPin v-else :size="12" />
                <span>{{ locationLabel(host) }}</span>
                <em>{{ host.location.isp || '网络信息未公开' }}</em>
              </p>
            </div>
            <div class="share-card__aside">
              <span class="share-status" :class="`is-${host.state}`">
                <i /> {{ stateLabel(host.state) }}
              </span>
              <small>{{ host.collectedAt ? `采集于 ${relativeTime(host.collectedAt)}` : '尚无数据' }}</small>
            </div>
          </header>

          <div v-if="host.collectedAt" class="share-metrics">
            <div>
              <span><Gauge :size="14" /> CPU</span>
              <strong>{{ formatPercent(host.cpu.usagePercent) }}</strong>
              <i><b :style="{ width: `${clampPercent(host.cpu.usagePercent)}%` }" /></i>
              <small>{{ host.cpu.cores }} 核</small>
            </div>
            <div>
              <span><MemoryStick :size="14" /> 内存</span>
              <strong>{{ formatPercent(host.memory.usagePercent) }}</strong>
              <i><b :style="{ width: `${clampPercent(host.memory.usagePercent)}%` }" /></i>
              <small>{{ host.memory.totalBytes ? `${Math.round(host.memory.totalBytes / 1073741824)} GB` : '—' }}</small>
            </div>
            <div>
              <span><HardDrive :size="14" /> 磁盘</span>
              <strong>{{ formatPercent(host.disk.usagePercent) }}</strong>
              <i><b :style="{ width: `${clampPercent(host.disk.usagePercent)}%` }" /></i>
              <small>{{ host.disk.totalBytes ? `${Math.round(host.disk.totalBytes / 1073741824)} GB` : '—' }}</small>
            </div>
          </div>
          <div v-else class="share-card__empty">等待第一份状态数据</div>

          <dl class="share-details">
            <div class="share-details__traffic">
              <dt>实时流量</dt>
              <dd>
                <span title="实时下行">
                  <ArrowDown :size="13" aria-hidden="true" />
                  <span class="sr-only">实时下行</span>
                  {{ formatRate(host.network.receiveBytesPerSecond || 0) }}
                </span>
                <span title="实时上行">
                  <ArrowUp :size="13" aria-hidden="true" />
                  <span class="sr-only">实时上行</span>
                  {{ formatRate(host.network.transmitBytesPerSecond || 0) }}
                </span>
              </dd>
            </div>
            <div class="share-details__traffic">
              <dt>累计流量</dt>
              <dd>
                <span title="累计接收">
                  <ArrowDown :size="13" aria-hidden="true" />
                  <span class="sr-only">累计接收</span>
                  {{ host.collectedAt ? formatNetworkTrafficCounter(host.network, 'received') : '—' }}
                </span>
                <span title="累计传送">
                  <ArrowUp :size="13" aria-hidden="true" />
                  <span class="sr-only">累计传送</span>
                  {{ host.collectedAt ? formatNetworkTrafficCounter(host.network, 'sent') : '—' }}
                </span>
              </dd>
            </div>
            <div>
              <dt><Clock3 :size="13" /> 运行时间</dt>
              <dd>{{ host.uptimeSeconds ? formatDuration(host.uptimeSeconds) : '—' }}</dd>
            </div>
          </dl>
        </article>
      </section>

      <section v-else-if="snapshot" class="share-state">
        <Server :size="26" />
        <strong>还没有可展示的机器</strong>
      </section>

      <footer class="share-footer">
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
    radial-gradient(circle at 16% -8%, color-mix(in srgb, var(--brand) 18%, transparent), transparent 34%),
    radial-gradient(circle at 92% 4%, color-mix(in srgb, var(--blue) 12%, transparent), transparent 30%),
    var(--bg);
  transition: color 0.2s ease, background 0.2s ease;
}

.share-page__glow {
  position: fixed;
  width: 440px;
  height: 440px;
  pointer-events: none;
  filter: blur(100px);
  opacity: 0.14;
  border-radius: 50%;
}

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
.share-card__header,
.share-status,
.share-details dt,
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
  border-radius: 10px;
  box-shadow: var(--shadow-sm);
  cursor: pointer;
}

.share-icon-button { width: 38px; justify-content: center; padding: 0; }
.share-refresh:hover,
.share-icon-button:hover { color: var(--brand-strong); border-color: var(--brand-muted); }
.share-refresh:disabled { cursor: wait; opacity: 0.58; }

.share-view-switch {
  padding: 3px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 11px;
}

.share-view-switch button {
  min-height: 30px;
  gap: 5px;
  padding: 0 10px;
  color: var(--muted);
  background: transparent;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  font-size: .875rem;
  font-weight: 600;
}

.share-view-switch button.is-active {
  color: var(--brand-strong);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
}

.share-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 20px;
  padding: clamp(20px, 2vw, 24px);
  margin-bottom: 14px;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--brand-soft) 46%, transparent), transparent 48%),
    color-mix(in srgb, var(--surface) 96%, transparent);
  border: 1px solid var(--border);
  border-radius: 24px;
  box-shadow: var(--shadow-md);
  backdrop-filter: blur(18px);
}

.share-kicker { display: flex; align-items: center; gap: 7px; color: var(--brand-strong); font-size: 11px; font-weight: 700; letter-spacing: 0.16em; }
.share-hero h1 { margin: 7px 0 4px; font-size: clamp(28px, 3vw, 38px); line-height: 1.05; }
.share-hero p { max-width: 670px; margin: 0 0 7px; color: var(--text-soft); font-size: 14px; line-height: 1.5; }
.share-hero small { color: var(--muted); }

.share-stats { display: grid; grid-template-columns: repeat(3, minmax(90px, 1fr)); }
.share-stats div { display: grid; gap: 3px; padding: 2px 16px; border-left: 1px solid var(--border); }
.share-stats strong { font-size: 25px; line-height: 1; }
.share-stats span { color: var(--muted); font-size: 11px; }
.share-stats .is-online strong { color: var(--brand); }
.share-stats .is-attention strong { color: var(--amber); }

.share-grid { display: grid; gap: 12px; }
.share-toolbar { display: flex; align-items: center; gap: 16px; margin-bottom: 16px; }
.share-toolbar > span { margin-left: auto; color: var(--text-soft); font-size: .8125rem; font-variant-numeric: tabular-nums; }
.share-search { display: flex; align-items: center; gap: 10px; flex: 1; max-width: 520px; min-width: 0; padding: 0 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-soft); background: var(--surface); }
.share-search input { min-width: 0; width: 100%; min-height: 42px; padding: 10px 0; font-size: .875rem; color: var(--text); background: transparent; border: 0; outline: none; box-shadow: none; }
.share-search:focus-within { outline: 2px solid var(--brand); outline-offset: 2px; }
.share-grid.is-card { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 15px; }
.share-grid.is-card .share-card { display: flex; flex-direction: column; }
.share-grid.is-card .share-card__header { grid-template-columns: auto minmax(0, 1fr); align-items: start; flex: 1; }
.share-grid.is-card .share-card__aside { grid-column: 1 / -1; display: flex; flex-wrap: wrap; justify-content: space-between; }
.share-card {
  min-width: 0;
  overflow: hidden;
  background: color-mix(in srgb, var(--surface) 96%, transparent);
  border: 1px solid var(--border);
  border-radius: 18px;
  box-shadow: var(--shadow-sm);
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}
.share-card:hover { border-color: var(--border-strong); box-shadow: var(--shadow-md); transform: translateY(-1px); }

.share-grid.is-list .share-card {
  display: grid;
  grid-template-columns: minmax(300px, 1fr) minmax(300px, 0.95fr) minmax(400px, 1.35fr);
  grid-template-areas: "header metrics details";
  align-items: stretch;
}

.share-grid.is-list .share-card__header { grid-area: header; border-right: 1px solid var(--border); }
.share-grid.is-list .share-metrics { grid-area: metrics; border-block: 0; border-right: 1px solid var(--border); }
.share-grid.is-list .share-details { grid-area: details; align-content: center; }

.share-card__header { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; padding: 14px; }
.share-card__header :deep(.os-identity__mark) { width: 40px; height: 40px; border-radius: 11px; }
.share-card__header :deep(.os-identity__mark svg) { width: 23px; height: 23px; }
.share-card h2 { margin: 0 0 6px; font-size: 1rem; line-height: 1.5; overflow-wrap: anywhere; }
.share-card__identity { min-width: 0; }
.share-card__system { display: flex; flex-wrap: wrap; min-width: 0; align-items: center; gap: 6px; margin: 0 0 7px; color: var(--text-soft); font-size: .8125rem; line-height: 1.5; overflow-wrap: anywhere; }
.share-card__system small { flex: 0 0 auto; padding: 1px 4px; color: var(--text-soft); background: var(--neutral-soft); border-radius: var(--radius-sm); font-size: .75rem; }
.share-card__location { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; margin: 0; color: var(--text-soft); font-size: .75rem; line-height: 1.5; overflow-wrap: anywhere; }
.share-card__location em { color: var(--text-soft); font-size: .75rem; font-style: normal; }
.share-card__location em::before { margin-right: 5px; content: "·"; }
.share-card__location :deep(.country-flag) { width: 16px; height: 16px; }
.share-card__aside { display: grid; min-width: 58px; justify-items: end; align-content: center; gap: 6px; }
.share-card__aside > small { color: var(--text-soft); font-size: .75rem; line-height: 1.5; }
.share-status { gap: 5px; color: var(--text-soft); font-size: .8125rem; white-space: nowrap; }
.share-status i { width: 7px; height: 7px; background: currentColor; border-radius: 50%; box-shadow: 0 0 10px currentColor; }
.share-status.is-online { color: var(--brand); }
.share-status.is-degraded { color: var(--amber); }
.share-status.is-offline { color: var(--danger); }

.share-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-block: 1px solid var(--border); }
.share-metrics > div { display: grid; align-content: center; gap: 5px; padding: 11px 13px; border-left: 1px solid var(--border); }
.share-metrics > div:first-child { border-left: 0; }
.share-metrics span { display: flex; align-items: center; gap: 5px; color: var(--text-soft); font-size: .8125rem; }
.share-metrics strong { font-size: 16px; }
.share-metrics > div > i { height: 4px; overflow: hidden; background: var(--neutral-soft); border-radius: 10px; }
.share-metrics b { display: block; height: 100%; background: linear-gradient(90deg, var(--blue), var(--brand)); border-radius: inherit; }
.share-metrics small { color: var(--text-soft); font-size: .75rem; }
.share-card__empty { padding: 29px; color: var(--muted); text-align: center; border-block: 1px solid var(--border); }

.share-details { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; gap: 14px; padding: 14px; margin: 0; }
.share-details div { min-width: 0; }
/* 实时和累计收发都只用箭头标示方向，左右并排后与其他格同高，不再撑高整张卡 */
.share-details__traffic dd { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.share-details__traffic dd > span { display: flex; min-width: 0; align-items: center; gap: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.share-details__traffic dd svg { flex: 0 0 auto; color: var(--muted); }
.share-details dt { gap: 4px; margin-bottom: 6px; color: var(--text-soft); font-size: .75rem; }
.share-details dd { margin: 0; font-size: .875rem; font-weight: 500; line-height: 1.5; overflow-wrap: anywhere; }

.share-state { display: grid; min-height: 280px; place-items: center; align-content: center; gap: 12px; color: var(--muted); text-align: center; }
.share-state p { margin: 0; }
.share-state--error svg { color: var(--danger); }
.share-warning { padding: 12px 15px; margin-bottom: 14px; color: var(--amber); background: var(--amber-soft); border: 1px solid color-mix(in srgb, var(--amber) 28%, var(--border)); border-radius: 12px; }
.share-footer { justify-content: space-between; gap: 20px; padding: 28px 4px 0; color: var(--muted); font-size: 10px; }
.share-footer strong { color: var(--text-soft); }

@media (max-width: 1100px) {
  .share-grid.is-list .share-card {
    grid-template-columns: minmax(300px, 0.95fr) minmax(0, 1.35fr);
    grid-template-areas:
      "header metrics"
      "details details";
  }
  .share-grid.is-list .share-metrics { border-right: 0; }
  .share-grid.is-list .share-details { border-top: 1px solid var(--border); }
}

@media (max-width: 1000px) {
  .share-grid.is-card { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .share-hero { grid-template-columns: 1fr; align-items: start; gap: 16px; }
  .share-stats div:first-child { border-left: 0; }
}

@media (max-width: 650px) {
  .share-shell { width: min(100% - 24px, 1280px); padding-top: 16px; }
  .share-header { margin-bottom: 18px; }
  .share-header__actions { gap: 6px; }
  .share-view-switch button { padding-inline: 8px; }
  .share-refresh span { display: none; }
  .share-hero { gap: 14px; padding: 18px 16px; border-radius: 18px; }
  .share-hero h1 { font-size: 30px; }
  .share-stats { width: 100%; }
  .share-stats div { padding: 2px 13px; }
  .share-stats div:first-child { border-left: 0; }
  .share-grid { grid-template-columns: minmax(0, 1fr); }
  .share-grid.is-card { grid-template-columns: minmax(0, 1fr); }
  .share-grid.is-list .share-card { display: block; }
  .share-grid.is-list .share-card__header { border-right: 0; }
  .share-grid.is-list .share-details { border-top: 0; }
  .share-grid.is-list .share-metrics { border-block: 1px solid var(--border); }
  .share-footer { align-items: flex-start; flex-direction: column; }
}

@media (max-width: 430px) {
  .share-header { align-items: center; flex-wrap: wrap; }
  .share-brand strong { display: none; }
  .share-view-switch button { justify-content: center; }
  .share-stats div { padding-inline: 10px; }
  .share-details { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (prefers-reduced-motion: reduce) {
  .spin { animation: none; }
}
</style>
