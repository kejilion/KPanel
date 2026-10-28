<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { Activity, ArrowDownToLine, ArrowUpRight, BarChart3, CircleHelp, Compass, Database, FileText, Globe2, Layers3, Link2, LoaderCircle, MonitorSmartphone, Pause, Play, RefreshCw, ShieldCheck, ShieldAlert, ShieldX, Timer, Trash2, TriangleAlert } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { localizeError } from '@/i18n/errors'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import { api } from '@/lib/api'
import { formatBytes, formatDateTime } from '@/lib/format'
import type { SiteTrafficSnapshot } from '@/types/siteTraffic'
import type { WebEnvironmentSummary } from '@/types/api'
import WorldTrafficMap from '@/components/sites/WorldTrafficMap.vue'
import SitesSectionTabs from '@/components/sites/SitesSectionTabs.vue'
import SiteFavicon from '@/components/sites/SiteFavicon.vue'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
usePhraseCatalog(locale => locale === 'en-US'
 ? import('@/i18n/pages/SiteMonitoringView/en-US').then(m => m.default)
 : import('@/i18n/pages/SiteMonitoringView/zh-TW').then(m => m.default))
const i18n = useI18n()
function phrase(value: string): string { phraseCatalogVersion.value; return translatePhrase(value) }
const data = ref<SiteTrafficSnapshot>()
const protectionStatus = ref<WebEnvironmentSummary>()
const loading = ref(true), refreshing = ref(false), error = ref(''), notice = ref('')
const protectionStatusLoading = ref(false), protectionStatusError = ref('')
const period = ref('24h'), host = ref(''), view = ref<'traffic' | 'security'>('traffic'), selectedCountry = ref('')
const busy = ref(false), guideOpen = ref(false), clearOpen = ref(false)
const siteIDs = ref(new Map<string, string>())
const iconController = new AbortController()
const normalizeHost = (value: string) => value.toLowerCase().replace(/\.$/, '')
const siteID = (value: string) => siteIDs.value.get(normalizeHost(value))
async function loadSiteIcons(): Promise<void> {
 try {
  const sites = await api.sites.list(undefined, iconController.signal)
  if (disposed || iconController.signal.aborted) return
  const ids = new Map<string, string>()
  for (const site of sites.items) for (const domain of [site.primaryDomain, ...site.domains]) ids.set(normalizeHost(domain), site.id)
  siteIDs.value = ids
 } catch { /* Site metadata is optional; traffic remains usable with local globe icons. */ }
}
let controller: AbortController | undefined
let protectionController: AbortController | undefined
let timer: ReturnType<typeof setTimeout> | undefined
let sequence = 0, protectionSequence = 0, protectionLastRequested = 0, disposed = false
const number = (value: number) => new Intl.NumberFormat(i18n.locale.value, { maximumFractionDigits: 1 }).format(value)
const countries = computed(() => view.value === 'traffic' ? data.value?.countries || [] : data.value?.protection.countries || [])
const countryCounts = computed(() => new Map(countries.value.map(item => [item.code, item.requests])))
const visibleCountries = computed(() => countries.value.filter(item => item.code !== 'ZZ').slice(0, 8))
const maxCountry = computed(() => Math.max(1, ...countries.value.map(item => item.requests)))
const series = computed(() => view.value === 'traffic' ? data.value?.trend || [] : data.value?.protection.trend || [])
const stateLabels: Record<string, string> = { disabled: '尚未启用', collecting: '开始采集', active: '持续采集', waiting: '等待日志', source_unavailable: '采集暂不可用' }
const status = computed(() => data.value?.catchingUp ? phrase('正在处理积压') : phrase(stateLabels[data.value?.state || 'disabled'] || '状态待核验'))
const healthy = computed(() => data.value?.enabled && data.value.state === 'active' && !data.value.catchingUp)
const errorRate = computed(() => data.value?.requests ? number(data.value.errors / data.value.requests * 100) + '%' : '—')
const totalLocated = computed(() => countries.value.filter(c => c.code !== 'ZZ').reduce((sum, c) => sum + c.requests, 0))
const totalMap = computed(() => countries.value.reduce((sum, c) => sum + c.requests, 0))
const protection = computed(() => protectionStatus.value?.protection)
const siteJails = computed(() => protection.value?.siteJails || [])
const fail2banLogEntries = computed(() => protection.value?.fail2banLogs?.entries || [])
const defenseWafMode = computed(() => protection.value?.wafMode || (protection.value?.waf ? 'legacy' : 'unknown'))
const defenseWafLabel = computed(() => phrase(({
 on: '拦截模式', off: '已关闭', detect: '检测模式', mixed: '按站点配置', legacy: '旧配置已启用', unknown: '状态待核验',
} as Record<string, string>)[defenseWafMode.value] || '状态待核验'))
const fail2banInstalled = computed(() => Boolean(protection.value?.fail2banInstalled || protection.value?.fail2ban))
const fail2banLabel = computed(() => protection.value?.fail2ban ? phrase('运行中')
 : fail2banInstalled.value ? phrase('已安装，未运行') : phrase('未安装'))
const fail2banBadge = computed(() => protection.value?.fail2ban ? 'healthy' : fail2banInstalled.value ? 'warning' : 'stopped')
const cloudflareLabel = computed(() => {
 const status = protection.value?.cloudflareStatus
 if (!status) return phrase(protection.value?.cloudflare ? '已配置' : '未配置')
 if (status.cleanupPending) return phrase('关闭清理待重试')
 if (status.mode === 'local') return phrase('源站封禁')
 if (status.mode === 'cloudflare') {
  if (status.pending) return phrase('等待同步')
  if (!status.healthy) return phrase('同步待核验')
  return phrase(status.running ? '已同步' : '已配置，未运行')
 }
 return phrase('状态待核验')
})
const cloudflareBadge = computed(() => {
 const status = protection.value?.cloudflareStatus
 if (!status) return protection.value?.cloudflare ? 'healthy' : 'unknown'
 if (status.cleanupPending || status.pending || (status.mode === 'cloudflare' && (!status.healthy || !status.running))) return 'warning'
 if (status.mode === 'cloudflare' && status.running && status.healthy) return 'healthy'
 if (status.mode === 'local') return 'stopped'
 return 'unknown'
})
const ddosLabel = computed(() => {
 const state = protection.value
 if (!state?.ddosRulesAvailable) return phrase('状态待核验')
 const expected = state.ddosRuleExpected || 8
 const count = state.ddosRuleCount || 0
 return phrase(count === expected ? '规则完整' : count ? '规则未完整' : '已关闭') + ` (${count}/${expected})`
})
const ddosBadge = computed(() => {
 const state = protection.value
 if (!state?.ddosRulesAvailable) return 'unknown'
 const expected = state.ddosRuleExpected || 8
 const count = state.ddosRuleCount || 0
 return count === expected ? 'healthy' : count ? 'warning' : 'stopped'
})
const cloudflareShieldLabel = computed(() => {
 const shield = protection.value?.cloudflareShield
 if (!shield?.available) return phrase('状态待核验')
 if (shield.scheduled && shield.scriptPresent) return phrase('已安排')
 if (shield.scheduled || shield.scriptPresent) return phrase('配置不完整')
 return phrase('未安排')
})
const cloudflareShieldBadge = computed(() => {
 const shield = protection.value?.cloudflareShield
 if (!shield?.available) return 'unknown'
 if (shield.scheduled && shield.scriptPresent) return 'healthy'
 if (shield.scheduled || shield.scriptPresent) return 'warning'
 return 'stopped'
})
const share = (value: number, total: number) => total > 0 ? number(value / total * 100) + '%' : '—'
const rankingWidth = (value: number, rows: { requests: number }[]) => Math.min(100, Math.max(0, value / Math.max(1, ...rows.map(row => row.requests)) * 100)) + '%'
const geoCoverage = computed(() => totalMap.value ? number(totalLocated.value / totalMap.value * 100) + '%' : '—')
const sourceWarnings: Record<string, string> = {
 initial_tail_only: '首次采集从最近日志开始，早期日志不计入历史。',
 log_truncated: '日志发生截断，部分历史请求可能缺失。',
 source_limit: '日志来源超过采集上限，统计覆盖不完整。',
 log_directory_limit: '日志目录条目较多，统计覆盖不完整。',
 unsafe_or_unreadable_log: '部分日志无法读取，其他来源仍在采集。',
 unparsed_lines: '部分日志格式无法识别，已跳过。',
 history_capacity: '统计库已达容量阈值，最旧统计已清理。',
 shared_log_unattributed: '共享访问日志没有站点字段，无法归属的请求仅计入全站总览。',
 source_unavailable: '采集暂时不可用，已保存的历史数据仍可查看。',
}
const warnings = computed(() => [...new Set(data.value?.warnings || [])].map(code => phrase(sourceWarnings[code] || '部分数据暂不可用')))
const displayValue = (value: number | undefined) => value === undefined || !data.value?.collectedAt || (view.value === 'traffic' ? !data.value.accessSource && !data.value.requests : !data.value.protection.sourceAvailable && !data.value.protection.events) ? '—' : number(value)
function countryName(code: string): string {
 if (code === 'ZZ') return phrase('未知地区')
 try { return new Intl.DisplayNames([i18n.locale.value], { type: 'region' }).of(code) || code } catch { return code }
}
function selectCountry(code: string): void { if (countryCounts.value.has(code)) selectedCountry.value = selectedCountry.value === code ? '' : code }
const chart = computed(() => {
 const points = series.value
 if (!points.length || !data.value) return { line: '', area: '', max: 0, first: '', last: '' }
 const width = period.value === '30d' ? 86400000 : period.value === '7d' ? 21600000 : 3600000
 const buckets = new Map<number, number>()
 for (const point of points) { const at = Math.floor(Date.parse(point.at) / width) * width; buckets.set(at, (buckets.get(at) || 0) + point.requests) }
 const start = Math.floor(Math.max(Date.parse(data.value.from), Date.parse(data.value.startedAt || data.value.from)) / width) * width
 const end = Math.floor(Date.parse(data.value.to) / width) * width
 const values: Array<{ at: number; value: number }> = []
 for (let at = start; at <= end && values.length < 720; at += width) values.push({ at, value: buckets.get(at) || 0 })
 for (const [at, value] of buckets) if (at < start) values.push({ at, value })
 values.sort((a, b) => a.at - b.at)
 const max = Math.max(1, ...values.map(item => item.value))
 const first = values[0]?.at || start, last = Math.max(first + width, values.at(-1)?.at || end)
 const coords = values.map(item => `${((item.at - first) / (last - first) * 1000).toFixed(1)},${(150 - item.value / max * 135).toFixed(1)}`)
 return { line: coords.join(' '), area: coords.length ? `M${coords[0]?.split(',')[0]},150 L${coords.join(' L')} L${coords.at(-1)?.split(',')[0]},150 Z` : '', max, first: new Date(first).toISOString(), last: new Date(last).toISOString() }
})
function shortTime(value: string): string { return value ? new Intl.DateTimeFormat(i18n.locale.value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '' }
function actionName(action: string): string { return phrase(({ blocked: '确认拦截', limited: '限流拒绝', matched: '规则命中', detected: '检测记录', delayed: '延迟处理' } as Record<string, string>)[action] || '待核验') }
function ruleName(rule: string): string { return rule === 'nginx.limit_req' ? phrase('请求频率限制') : rule === 'nginx.limit_conn' ? phrase('并发连接限制') : rule }
async function load(): Promise<void> {
 if (disposed) return
 const current = ++sequence
 controller?.abort(); const requestController = new AbortController(); controller = requestController
 if (data.value) refreshing.value = true
 try {
  const snapshot = await api.siteTraffic.summary({ period: period.value, host: host.value || undefined }, requestController.signal)
  if (disposed || current !== sequence) return
  data.value = snapshot; error.value = ''
 } catch (reason) {
  if (disposed || current !== sequence || requestController.signal.aborted) return
  error.value = reason instanceof Error ? localizeError(reason) : phrase('监控数据读取失败')
 } finally { if (current === sequence) { loading.value = false; refreshing.value = false } }
}
async function loadProtectionStatus(): Promise<void> {
 if (disposed) return
 const current = ++protectionSequence
 protectionController?.abort()
 const requestController = new AbortController()
 protectionController = requestController
 protectionLastRequested = Date.now()
 protectionStatusLoading.value = true
 try {
  const summary = await api.webEnvironment.summary(requestController.signal)
  if (disposed || current !== protectionSequence) return
  protectionStatus.value = summary
  protectionStatusError.value = ''
 } catch (reason) {
  if (disposed || current !== protectionSequence || requestController.signal.aborted) return
  protectionStatusError.value = reason instanceof Error ? localizeError(reason) : phrase('防护状态读取失败')
 } finally {
  if (current === protectionSequence) protectionStatusLoading.value = false
 }
}
async function refreshCurrent(): Promise<void> {
 await Promise.all([load(), view.value === 'security' ? loadProtectionStatus() : Promise.resolve()])
}
async function perform(action: 'enable' | 'pause' | 'clear'): Promise<void> {
 if (busy.value) return
 busy.value = true; notice.value = ''
 try {
  await api.siteTraffic.action(action); clearOpen.value = false
  notice.value = phrase(action === 'enable' ? '监控已开启，正在采集网站日志。' : action === 'pause' ? '采集已暂停，历史统计保留。' : '历史统计已清空，原始日志保持不变。')
  await load()
 } catch (reason) { error.value = reason instanceof Error ? localizeError(reason) : phrase('监控操作失败') }
 finally { busy.value = false }
}
function schedule(): void { timer = setTimeout(async () => { if (!disposed && document.visibilityState !== 'hidden' && data.value?.enabled && !busy.value) await load(); if (!disposed && document.visibilityState !== 'hidden' && view.value === 'security' && Date.now() - protectionLastRequested >= 30000) await loadProtectionStatus(); if (!disposed) schedule() }, 10000) }
watch([period, host], () => { selectedCountry.value = ''; data.value = undefined; loading.value = true; void load() })
watch(view, value => {
 selectedCountry.value = ''
 if (value === 'security') void loadProtectionStatus()
 else { protectionController?.abort(); protectionStatusLoading.value = false }
})
onMounted(() => { void load(); void loadSiteIcons(); schedule() })
onBeforeUnmount(() => { disposed = true; sequence++; protectionSequence++; controller?.abort(); protectionController?.abort(); iconController.abort(); if (timer) clearTimeout(timer) })
</script>

<template>
 <section class="page monitor-page" :class="{ 'is-security': view === 'security' }">
  <SitesSectionTabs />
  <LoadingState v-if="loading" :rows="4" cards />
  <ErrorState v-else-if="error && !data" title="网站监控暂不可用" :message="error" @retry="load" />
  <template v-else-if="data">
   <div class="monitor-toolbar">
    <div class="monitor-view-switch" role="group" aria-label="监控视图"><button :class="{ active: view === 'traffic' }" :aria-pressed="view === 'traffic'" @click="view = 'traffic'"><BarChart3 :size="17" />流量分析</button><button :class="{ active: view === 'security' }" :aria-pressed="view === 'security'" @click="view = 'security'"><ShieldCheck :size="17" />WAF 防御</button></div>
    <div class="monitor-filters"><label><span class="sr-only">站点筛选</span><select v-model="host" aria-label="站点筛选"><option value="">全部站点</option><option v-for="name in data.hosts" :key="name" :value="name">{{ name }}</option></select></label><label><span class="sr-only">统计时间</span><select v-model="period" aria-label="统计时间"><option value="24h">近 24 小时</option><option value="7d">近 7 天</option><option value="30d">近 30 天</option></select></label><button class="icon-button" aria-label="统计口径与设置" title="统计口径与设置" @click="guideOpen = true"><CircleHelp :size="18" /></button></div>
    <div class="monitor-actions"><span class="collection-status" :class="{ 'is-live': healthy }"><span aria-hidden="true" />{{ status }}</span><button class="button button--secondary" :disabled="refreshing || loading || (view === 'security' && protectionStatusLoading)" @click="refreshCurrent"><RefreshCw :size="16" :class="{ spinning: refreshing || protectionStatusLoading }" /><span>刷新</span></button></div>
   </div>
   <div class="monitor-scope"><span class="scope-icon" aria-hidden="true"><SiteFavicon v-if="host && siteID(host)" :site-id="siteID(host)!" :domain="host" :refresh-key="0" /><Globe2 v-else-if="host" :size="22" /><Layers3 v-else :size="22" /></span><div><strong>{{ host || phrase('全部站点') }}</strong><span>{{ phrase('源站日志统计') }}</span></div><span class="scope-summary"><Globe2 :size="15" aria-hidden="true" />{{ number(visibleCountries.length ? countries.filter(item => item.code !== 'ZZ' && item.requests > 0).length : 0) }} {{ phrase('个地区') }}</span></div>
   <div v-if="!data.enabled" class="monitor-callout"><Database :size="21" /><div><strong>{{ data.startedAt ? phrase('采集已暂停') : phrase('开启您的网站监控') }}</strong><p>增量采集访问与防御日志，保留最长 30 天的聚合统计。</p></div><button class="button button--primary" :disabled="busy" @click="perform('enable')"><Play :size="15" />开始监控</button></div>
   <p v-if="notice" class="monitor-notice" role="status">{{ notice }}</p><p v-if="error && data" class="monitor-warning" role="alert">{{ error }}</p><p v-if="data.state === 'waiting'" class="monitor-warning" role="status">尚未发现可采集的网站日志。请检查站点日志是否已启用。</p>
   <div v-if="view === 'traffic'" class="metric-grid">
    <article class="metric-card metric-card--brand"><span class="metric-heading"><span>请求数</span><span class="metric-icon" aria-hidden="true"><Activity :size="20" /></span></span><strong>{{ displayValue(data.requests) }}</strong><small>源站收到的访问请求</small></article>
    <article class="metric-card metric-card--success"><span class="metric-heading"><span>独立 IP</span><span class="metric-icon" aria-hidden="true"><MonitorSmartphone :size="20" /></span></span><strong>{{ displayValue(data.uniqueIPs) }}</strong><small>所选时间内按 IP 去重</small></article>
    <article class="metric-card metric-card--brand"><span class="metric-heading"><span>响应流量</span><span class="metric-icon" aria-hidden="true"><ArrowDownToLine :size="20" /></span></span><strong>{{ data.collectedAt && (data.accessSource || data.requests) ? formatBytes(data.bytes) : '—' }}</strong><small>HTTP 响应正文大小</small></article>
    <article class="metric-card metric-card--warning"><span class="metric-heading"><span>HTTP 错误率</span><span class="metric-icon" aria-hidden="true"><TriangleAlert :size="20" /></span></span><strong>{{ errorRate }}</strong><small>4xx 与 5xx 请求占比</small></article>
   </div>
   <div v-else class="metric-grid">
    <article class="metric-card metric-card--brand"><span class="metric-heading"><span>安全事件</span><span class="metric-icon" aria-hidden="true"><ShieldAlert :size="20" /></span></span><strong>{{ displayValue(data.protection.events) }}</strong><small>同一事务合并记录</small></article>
    <article class="metric-card metric-card--danger"><span class="metric-heading"><span>确认拦截</span><span class="metric-icon" aria-hidden="true"><ShieldX :size="20" /></span></span><strong class="danger-number">{{ displayValue(data.protection.blocked) }}</strong><small>来自明确的拦截记录</small></article>
    <article class="metric-card metric-card--warning"><span class="metric-heading"><span>限流拒绝</span><span class="metric-icon" aria-hidden="true"><Timer :size="20" /></span></span><strong>{{ displayValue(data.protection.limited) }}</strong><small>请求频率或连接数限制</small></article>
    <article class="metric-card metric-card--success"><span class="metric-heading"><span>其他防御事件</span><span class="metric-icon" aria-hidden="true"><ShieldCheck :size="20" /></span></span><strong>{{ displayValue(data.protection.matched) }}</strong><small>命中、检测与延迟处理</small></article>
   </div>
   <article v-if="view === 'security'" class="monitor-card defense-status-card">
    <header>
     <div><h3><ShieldCheck :size="18" aria-hidden="true" />{{ phrase('防护链路状态') }}</h3><p>{{ phrase('从环境管理读取最新状态；此处仅监控，不修改防护配置。') }}</p></div>
     <div class="defense-header-actions">
      <button class="text-button" :disabled="protectionStatusLoading" @click="loadProtectionStatus"><RefreshCw :size="14" :class="{ spinning: protectionStatusLoading }" />{{ phrase('刷新防护状态') }}</button>
      <RouterLink class="text-button" to="/sites/environment">{{ phrase('打开防护设置') }}<ArrowUpRight :size="14" /></RouterLink>
     </div>
    </header>
    <LoadingState v-if="protectionStatusLoading && !protectionStatus" :rows="2" />
    <div v-else-if="protectionStatusError && !protectionStatus" class="defense-status-error" role="alert">
     <ShieldAlert :size="19" /><span>{{ protectionStatusError }}</span><button class="text-button" @click="loadProtectionStatus">{{ phrase('重试') }}</button>
    </div>
    <template v-else-if="protectionStatus">
     <p v-if="protectionStatusError" class="monitor-warning defense-inline-warning" role="status">{{ protectionStatusError }}</p>
     <div class="defense-flow" :aria-label="phrase('WAF 到 Fail2Ban 网站 Jail，再到 Cloudflare 的状态链路')">
      <div class="defense-flow-step"><span>WAF / Nginx</span><StatusBadge :status="defenseWafMode === 'on' || defenseWafMode === 'legacy' ? 'healthy' : defenseWafMode === 'detect' || defenseWafMode === 'mixed' ? 'warning' : defenseWafMode === 'off' ? 'stopped' : 'unknown'" :label="defenseWafLabel" /></div>
      <span class="defense-flow-arrow" aria-hidden="true">→</span>
      <div class="defense-flow-step"><span>{{ phrase('Fail2Ban 网站 Jail') }}</span><StatusBadge :status="fail2banBadge" :label="fail2banLabel" /><small v-if="siteJails.length">{{ siteJails.filter(jail => jail.available).length }}/{{ siteJails.length }} {{ phrase('可用 Jail') }}</small></div>
      <span class="defense-flow-arrow" aria-hidden="true">→</span>
      <div class="defense-flow-step"><span>Cloudflare</span><StatusBadge :status="cloudflareBadge" :label="cloudflareLabel" /><small v-if="protection?.cloudflareStatus">{{ phrase('活动封禁') }}：{{ number(protection?.cloudflareStatus?.activeBans || 0) }}</small></div>
     </div>
     <div class="defense-status-grid">
      <div class="defense-status-item"><span>{{ phrase('DDoS 规则') }}</span><StatusBadge :status="ddosBadge" :label="ddosLabel" /></div>
      <div class="defense-status-item"><span>{{ phrase('高负载五秒盾') }}</span><StatusBadge :status="cloudflareShieldBadge" :label="cloudflareShieldLabel" /></div>
     </div>
     <section class="defense-detail-section">
      <header><h4>{{ phrase('站点 Jail 封禁计数') }}</h4><span>{{ phrase('仅展示网站防护 Jail') }}</span></header>
      <div v-if="siteJails.length" class="defense-jails-table-wrap"><table class="defense-jails-table"><thead><tr><th>{{ phrase('Jail') }}</th><th>{{ phrase('当前封禁') }}</th><th>{{ phrase('累计封禁') }}</th><th>{{ phrase('状态') }}</th></tr></thead><tbody><tr v-for="jail in siteJails" :key="jail.name"><td><code>{{ jail.name }}</code></td><td>{{ jail.available ? number(jail.currentlyBanned) : '—' }}</td><td>{{ jail.available ? number(jail.totalBanned) : '—' }}</td><td><StatusBadge :status="jail.available ? 'healthy' : 'unknown'" :label="phrase(jail.available ? '运行中' : '不可用')" /></td></tr></tbody></table></div>
      <p v-else class="inline-empty">{{ phrase('站点 Jail 状态暂不可用') }}</p>
     </section>
     <section class="defense-detail-section defense-log-section">
      <header><h4>{{ phrase('最近 Fail2Ban 网站日志') }}</h4><span>{{ phrase('最近 20 条') }}</span></header>
      <ul v-if="protection?.fail2banLogs?.available && fail2banLogEntries.length"><li v-for="(entry, index) in fail2banLogEntries" :key="`${index}-${entry}`"><code>{{ entry }}</code></li></ul>
      <p v-else class="inline-empty">{{ phrase(protection?.fail2banLogs?.available ? '暂无网站 Jail 日志' : '日志源不可用') }}</p>
     </section>
    </template>
   </article>
   <article class="monitor-card trend-card">
    <header><div><h3>{{ view === 'traffic' ? phrase('访问趋势') : phrase('防御事件趋势') }}</h3><p>{{ view === 'traffic' ? phrase('请求变化与访问高峰') : phrase('规则命中与防御活动') }}</p></div><span class="chart-legend"><i />{{ view === 'traffic' ? phrase('请求数') : phrase('安全事件') }}</span></header>
    <div v-if="chart.line" class="trend-chart" role="img" :aria-label="view === 'traffic' ? phrase('访问请求趋势图') : phrase('防御事件趋势图')"><span class="chart-maximum">{{ number(chart.max) }}</span><svg viewBox="0 0 1000 160" preserveAspectRatio="none" aria-hidden="true"><path v-for="y in [15, 60, 105, 150]" :key="y" :d="`M0,${y} H1000`" class="chart-grid" /><path :d="chart.area" class="chart-area" /><polyline :points="chart.line" class="chart-line" /></svg><div class="chart-ticks"><span>{{ shortTime(chart.first) }}</span><span>{{ shortTime(chart.last) }}</span></div></div>
    <div v-else class="chart-empty"><Activity :size="26" /><strong>{{ view === 'security' ? phrase('暂无防御事件') : phrase('等待第一批访问数据') }}</strong><p>{{ view === 'security' ? phrase('事件到达后将在这里显示趋势。') : phrase('开启监控后，收到的网站请求会显示在这里。') }}</p></div>
   </article>
   <div class="geography-grid">
    <article class="monitor-card world-card">
     <header><div><h3>{{ view === 'traffic' ? phrase('世界访问来源') : phrase('安全事件来源') }}</h3><p>按 IP 归属地区统计</p></div><span class="muted-label">{{ geoCoverage }}<span> 已识别</span></span></header>
     <WorldTrafficMap v-model:selected-country="selectedCountry" :countries="countries" :mode="view" />
     <div v-if="data.geoStatus !== 'ready'" class="geo-setup"><Globe2 :size="18" /><p>{{ data.geoStatus === 'invalid' ? phrase('地区数据库无法读取，请更换有效的国家数据库。') : phrase('内置地区库将在采集日志时自动启用。') }}</p><button class="text-button" @click="guideOpen = true">地区识别说明<ArrowUpRight :size="14" /></button></div>
    </article>
    <article class="monitor-card countries-card"><header><div><h3><Globe2 :size="18" aria-hidden="true" />国家与地区</h3><p>{{ view === 'traffic' ? phrase('按请求数排序') : phrase('按事件数排序') }}</p></div></header><div v-if="visibleCountries.length" class="ranking-list"><button v-for="country in visibleCountries" :key="country.code" class="country-row" :class="{ active: selectedCountry === country.code }" :aria-pressed="selectedCountry === country.code" @click="selectCountry(country.code)"><span class="country-symbol" aria-hidden="true"><CountryFlagIcon :country-code="country.code" :label="countryName(country.code)" /><span class="country-code">{{ country.code }}</span></span><span class="country-label">{{ countryName(country.code) }}<span class="ranking-track"><i :style="{ width: country.requests / maxCountry * 100 + '%' }" /></span></span><span class="ranking-value"><strong>{{ number(country.requests) }}</strong><small>{{ share(country.requests, totalMap) }}</small></span></button></div><div v-else class="ranking-empty"><Globe2 :size="24" /><p>暂无可识别的地区数据</p></div><div class="unknown-country"><span><Globe2 :size="15" aria-hidden="true" />未知地区</span><strong>{{ number(countryCounts.get('ZZ') || 0) }}</strong></div></article>
   </div>

   <div v-if="view === 'traffic'" class="rankings-grid">
    <article class="monitor-card"><header><div><h3><FileText :size="18" aria-hidden="true" />热门路径</h3><p>已移除 URL 查询参数</p></div></header><div v-if="data.paths.length" class="ranking-list"><div v-for="row in data.paths.slice(0, 8)" :key="row.name" class="plain-ranking rich-ranking"><span class="ranking-symbol" aria-hidden="true"><FileText :size="17" /></span><span class="ranking-content"><span :title="row.name">{{ row.name || '/' }}</span><span class="ranking-track"><i :style="{ width: rankingWidth(row.requests, data.paths) }" /></span></span><strong>{{ number(row.requests) }}</strong></div></div><p v-else class="inline-empty">暂无路径统计</p></article>
    <article class="monitor-card"><header><div><h3><Compass :size="18" aria-hidden="true" />访问来源</h3><p>请求中记录的来源域名</p></div></header><div v-if="data.referrers.length" class="ranking-list"><div v-for="row in data.referrers.slice(0, 8)" :key="row.name" class="plain-ranking rich-ranking"><span class="ranking-symbol" aria-hidden="true"><SiteFavicon v-if="row.name && siteID(row.name)" :site-id="siteID(row.name)!" :domain="row.name" :refresh-key="0" /><Link2 v-else-if="row.name" :size="17" /><Compass v-else :size="17" /></span><span class="ranking-content"><span>{{ row.name || phrase('直接访问或无来源') }}</span><span class="ranking-track"><i :style="{ width: rankingWidth(row.requests, data.referrers) }" /></span></span><strong>{{ number(row.requests) }}</strong></div></div><p v-else class="inline-empty">暂无来源统计</p></article>
   </div>
   <template v-else>
    <div v-if="!data.protection.sourceAvailable" class="monitor-callout"><ShieldAlert :size="21" /><div><strong>尚未发现 WAF 事件日志</strong><p>启用兼容的 WAF 日志后，防御事件会自动汇入。</p></div><RouterLink class="button button--secondary" to="/sites/environment">管理 WAF<ArrowUpRight :size="14" /></RouterLink></div>
    <article class="monitor-card"><header><div><h3><ShieldCheck :size="18" aria-hidden="true" />命中规则排行</h3><p>一次请求可能命中多条规则</p></div></header><div v-if="data.protection.rules.length" class="rules-grid"><div v-for="rule in data.protection.rules.slice(0, 8)" :key="rule.name" class="plain-ranking"><code>{{ ruleName(rule.name) }}</code><strong>{{ number(rule.requests) }}</strong></div></div><p v-else class="inline-empty">暂无规则命中记录</p></article>
    <article class="monitor-card events-card"><header><div><h3><ShieldAlert :size="18" aria-hidden="true" />最近安全事件</h3><p>最近 50 条，保留最长 7 天</p></div><RouterLink class="text-button" to="/files?path=/home/web/log/nginx">查看原始日志<ArrowUpRight :size="14" /></RouterLink></header><div v-if="data.protection.recent.length" class="events-table-wrap"><table><thead><tr><th>时间</th><th>站点与路径</th><th>地区</th><th>处理结果</th><th>命中规则</th></tr></thead><tbody><tr v-for="event in data.protection.recent" :key="event.id"><td>{{ formatDateTime(event.at) }}</td><td><div class="event-site"><span class="event-site-icon" aria-hidden="true"><SiteFavicon v-if="siteID(event.host)" :site-id="siteID(event.host)!" :domain="event.host" :refresh-key="0" /><Globe2 v-else :size="19" /></span><div><strong>{{ event.host || phrase('共享日志') }}</strong><span>{{ event.path || '—' }}</span></div></div></td><td><div class="event-country"><CountryFlagIcon v-if="event.country !== 'ZZ'" :country-code="event.country" :label="countryName(event.country)" /><Globe2 v-else :size="17" aria-hidden="true" />{{ countryName(event.country) }}</div></td><td><span class="event-action" :class="{ blocked: event.action === 'blocked' || event.action === 'limited' }">{{ actionName(event.action) }}</span></td><td><code>{{ event.rules.join(', ') || '—' }}</code></td></tr></tbody></table></div><p v-else class="inline-empty">暂无安全事件记录</p></article>
   </template>
   <details v-if="warnings.length" class="coverage-details"><summary>数据覆盖说明 <span>{{ warnings.length }}</span></summary><ul><li v-for="warning in warnings" :key="warning">{{ warning }}</li></ul></details>
   <footer class="monitor-footer"><span>{{ phrase('内置地区数据') }} · <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer" data-i18n-ignore>DB-IP</a></span><span><Database :size="14" />历史统计<span v-if="data.collectedAt"> · {{ formatDateTime(data.collectedAt) }}</span></span><div><button v-if="data.enabled" class="text-button" :disabled="busy" @click="perform('pause')"><Pause :size="14" />暂停采集</button><button class="text-button" :disabled="busy" @click="clearOpen = true"><Trash2 :size="14" />清空统计</button></div></footer>
  </template>
  <ModalDialog :open="guideOpen" :title="phrase('统计口径与设置')" size="medium" @close="guideOpen = false"><div class="monitor-guide"><h3>{{ phrase('采集与存储') }}</h3><p>{{ phrase('每 10 秒增量读取可用日志，统计与读取进度一并提交。聚合历史最长保留 30 天，安全事件最长保留 7 天或 5000 条。统计库上限 128 MiB，接近容量时清理最旧统计。') }}</p><h3>{{ phrase('流量统计口径') }}</h3><p>{{ phrase('请求数包含页面、静态资源与机器人请求；独立 IP 不等同于独立访客。响应流量为 HTTP 正文大小，不等同于服务器账单流量。CDN 直接命中的访问不会到达源站。') }}</p><h3>{{ phrase('本地地区识别') }}</h3><p>{{ phrase('已内置离线国家数据库，开启监控即可识别新请求的地区。也可上传兼容 MMDB 覆盖内置库；已有未知地区统计不会自动补全。') }}</p><code class="geo-path">/home/web/config/GeoLite2-Country.mmdb</code><div class="guide-actions"><a class="button button--secondary" href="https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/" target="_blank" rel="noopener noreferrer">{{ phrase('获取国家数据库') }}<ArrowUpRight :size="14" /></a><RouterLink class="button button--secondary" to="/files?path=/home/web/config" @click="guideOpen = false">{{ phrase('打开文件管理') }}</RouterLink></div><p>{{ phrase('地区为 IP 归属推测。CDN 或代理部署须先正确恢复真实访客 IP；统计不会直接信任请求中的转发头。') }}</p><h3>{{ phrase('防御事件口径') }}</h3><p>{{ phrase('审计日志记录规则命中，明确的拦截日志才计为确认拦截；普通 403 不会直接计为 WAF 拦截。原始请求头、请求体及规则匹配内容不写入统计库。') }}</p></div></ModalDialog>
  <ModalDialog :open="clearOpen" :title="phrase('清空网站监控统计')" size="small" @close="clearOpen = false"><div class="monitor-guide"><p>{{ phrase('将清除已保存的流量与防御历史。原始 Nginx 日志和 WAF 配置保持不变，采集进度会保留。') }}</p><div class="guide-actions"><button class="button button--secondary" @click="clearOpen = false">{{ i18n.t('common.cancel') }}</button><button class="button button--danger" :disabled="busy" @click="perform('clear')"><LoaderCircle v-if="busy" :size="15" class="spinning" />{{ phrase('确认清空') }}</button></div></div></ModalDialog>
 </section>
</template>
<style scoped src="@/styles/siteMonitoring.css"></style>
