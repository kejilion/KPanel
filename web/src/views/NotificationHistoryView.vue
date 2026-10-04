<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { FileText, RefreshCw, Search } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { ApiError, api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { summarizeNotification } from '@/lib/notificationSummary'
import { detectOperatingSystemIdentity, type OperatingSystemIdentity } from '@/lib/operatingSystem'
import type { NotificationEvent, NotificationHistoryPage } from '@/types/api'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/NotificationHistoryView/en-US').then((module) => module.default)
  : import('@/i18n/pages/NotificationHistoryView/zh-TW').then((module) => module.default))
function phrase(value: string): string { phraseCatalogVersion.value; return translatePhrase(value) }

const route = useRoute()
const filters = reactive({ days: '7', host: typeof route.query.host === 'string' ? route.query.host : '', rule: '', search: '' })
const items = ref<NotificationEvent[]>([])
const hostSystems = ref(new Map<string, OperatingSystemIdentity>())
const hostController = new AbortController()
const rows = computed(() => items.value.map(event => ({
  event, ...summarizeNotification(event),
  system: hostSystems.value.get(event.isLocal ? 'local' : event.hostId) || detectOperatingSystemIdentity(),
})))
const selectedEvent = ref<NotificationEvent>()
const hosts = ref<NotificationHistoryPage['hosts']>([])
const nextCursor = ref('')
const loading = ref(true)
const loadingMore = ref(false)
const error = ref('')
const retention = ref({ days: 30, events: 2000 })
let controller: AbortController | undefined
let requestID = 0
let appliedFilters: Record<string, string> = {}

const rules: Record<string, string> = {
  cpu: 'CPU 使用率', memory: '内存使用率', disk: '磁盘使用率', traffic: '网络吞吐',
  'traffic-total-received': '累计接收', 'traffic-total-sent': '累计传送', availability: '主机连接', ssh: 'SSH 登录',
  'server-expiry': '服务器到期', 'panel-login': '面板登录',
}
const kinds: Record<NotificationEvent['kind'], string> = { alert: '告警', recovery: '恢复', info: '信息' }
const deliveries: Record<NotificationEvent['delivery'], string> = { local_only: '仅本地', pending: '待发送', sent: '已发送', failed: '发送失败', cancelled: '已停止发送' }

async function load(append = false): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  const id = ++requestID
  loading.value = !append
  loadingMore.value = append
  error.value = ''
  if (!append) {
    items.value = []
    nextCursor.value = ''
    appliedFilters = { since: new Date(Date.now() - Number(filters.days) * 86400000).toISOString() }
    for (const key of ['host', 'rule', 'search'] as const) {
      const value = filters[key].trim()
      if (value) appliedFilters[key] = value
    }
  }
  try {
    const result = await api.cluster.notificationHistory({ ...appliedFilters, ...(append ? { cursor: nextCursor.value } : {}) }, controller.signal)
    if (id !== requestID) return
    items.value = append ? [...items.value, ...result.items] : result.items
    hosts.value = result.hosts
    nextCursor.value = result.nextCursor || ''
    retention.value = { days: result.retentionDays, events: result.maxEvents }
  } catch (reason) {
    if (id !== requestID || (reason instanceof DOMException && reason.name === 'AbortError')) return
    error.value = reason instanceof ApiError && reason.status === 401
      ? '登录已过期，请重新登录。'
      : reason instanceof ApiError && reason.status === 400
        ? '筛选条件无效，请缩短搜索内容或调整筛选后重试。'
        : '通知记录暂时不可用，请重试或检查 KPanel 数据目录。'
  } finally {
    if (id === requestID) { loading.value = false; loadingMore.value = false }
  }
}

watch(() => [filters.days, filters.host, filters.rule], () => void load())
watch(() => route.query.host, (host) => { filters.host = typeof host === 'string' ? host : '' })
onMounted(() => {
  void load()
  // Optional decoration: a slow or unavailable host inventory must not delay history.
  void api.cluster.hosts(hostController.signal).then(result => {
    if (hostController.signal.aborted) return
    hostSystems.value = new Map(result.items.map(host => [
      host.isLocal ? 'local' : host.id, detectOperatingSystemIdentity(host.lastSnapshot?.telemetry),
    ]))
  }).catch(() => { /* Keep the generic Linux mark when host metadata is unavailable. */ })
})
onBeforeUnmount(() => { requestID++; controller?.abort(); hostController.abort() })
</script>

<template>
  <div class="page notification-history">
    <div class="notification-history__summary">
      <p class="notification-history__intro">{{ phrase('本机与集群事件默认保存在当前 KPanel，外部推送可选。') }}</p>
      <p class="notification-history__retention">{{ phrase('保留最近') }} {{ retention.days }} {{ phrase('天，最多') }} {{ retention.events }} {{ phrase('条；达到容量上限时清理最早记录。') }}</p>
    </div>
    <form class="notification-history__filters toolbar-card" @submit.prevent="load()">
      <label class="field notification-history__search">
        <span>{{ phrase('搜索记录') }}</span>
        <div><Search :size="17" aria-hidden="true" /><input v-model="filters.search" type="search" maxlength="200" :placeholder="phrase('主机名称或通知内容')" /></div>
      </label>
      <label class="field"><span>{{ phrase('时间范围') }}</span><select v-model="filters.days">
        <option value="1">{{ phrase('最近 24 小时') }}</option><option value="7">{{ phrase('最近 7 天') }}</option><option value="30">{{ phrase('最近 30 天') }}</option>
      </select></label>
      <label class="field"><span>{{ phrase('主机') }}</span><select v-model="filters.host">
        <option value="">{{ phrase('全部主机') }}</option><option value="local">{{ phrase('仅本机') }}</option>
        <option v-if="filters.host && filters.host !== 'local' && !hosts.some(host => host.id === filters.host)" :value="filters.host">{{ filters.host }}</option>
        <option v-for="host in hosts.filter(host => !host.isLocal)" :key="host.id" :value="host.id">{{ host.name }}</option>
      </select></label>
      <label class="field"><span>{{ phrase('事件类型') }}</span><select v-model="filters.rule">
        <option value="">{{ phrase('全部类型') }}</option><option v-for="(label, key) in rules" :key="key" :value="key">{{ phrase(label) }}</option>
      </select></label>
      <button type="submit" class="button button--secondary" :disabled="loading"><RefreshCw :size="16" />{{ phrase('查询') }}</button>
    </form>
    <LoadingState v-if="loading" />
    <ErrorState v-else-if="error" :message="phrase(error)" @retry="load(items.length > 0)" />
    <EmptyState v-else-if="!items.length" :title="phrase('暂无符合条件的通知')" :description="phrase('可以调整筛选条件；首次启用后只记录新发生的事件。')" />
    <div v-if="!loading && items.length" class="notification-history__list">
      <table class="notification-history__table" :aria-label="phrase('通知记录')">
        <thead><tr><th scope="col">{{ phrase('时间') }}</th><th scope="col">{{ phrase('主机') }}</th><th scope="col">{{ phrase('事件类型') }}</th><th scope="col">{{ phrase('事件信息') }}</th><th scope="col">{{ phrase('外部投递') }}</th><th scope="col"><span class="sr-only">{{ phrase('查看原文') }}</span></th></tr></thead>
        <tbody>
          <tr v-for="{ event, fields, text, occurredAt, system } in rows" :key="event.id" class="notification-history__event">
            <td class="notification-history__time"><time :datetime="occurredAt || event.createdAt" :title="occurredAt || event.createdAt">{{ formatDateTime(occurredAt || event.createdAt) }}</time></td>
            <td class="notification-history__host"><div class="notification-history__host-label"><OperatingSystemIcon :distro="system.key" :label="system.label" :show-tooltip="true" /><strong><span class="sr-only">{{ system.label }} · </span>{{ event.hostName }}</strong></div></td>
            <td class="notification-history__type"><span>{{ phrase(rules[event.rule] || event.rule) }}</span><span class="notification-history__kind" :data-kind="event.kind">{{ phrase(kinds[event.kind]) }}</span></td>
            <td class="notification-history__content">
              <dl v-if="fields.length" class="notification-history__fields"><div v-for="field in fields" :key="field.label"><dt>{{ phrase(field.label) }}</dt><dd>{{ field.value }}</dd></div></dl>
              <span v-else>{{ text }}</span>
            </td>
            <td class="notification-history__delivery" :class="{ 'notification-history__failure': event.delivery === 'failed' }">{{ phrase(deliveries[event.delivery]) }}</td>
            <td class="notification-history__action"><button type="button" class="icon-button" :aria-label="phrase('查看原文') + ' · ' + event.hostName + ' · ' + formatDateTime(event.createdAt)" @click="selectedEvent = event"><FileText :size="16" /></button></td>
          </tr>
        </tbody>
      </table>
      <button v-if="nextCursor" class="button button--secondary" type="button" :disabled="loadingMore" @click="load(true)">{{ phrase(loadingMore ? '正在加载…' : '加载更多') }}</button>
    </div>
    <ModalDialog :open="Boolean(selectedEvent)" :title="phrase('通知原文')" @close="selectedEvent = undefined">
      <div v-if="selectedEvent" class="notification-history__original">
        <p>{{ selectedEvent.message }}</p>
        <p v-if="selectedEvent.relatedEventId">{{ phrase('关联告警编号') }}: {{ selectedEvent.relatedEventId }}</p>
        <p v-if="selectedEvent.provider">{{ phrase('渠道') }}: {{ selectedEvent.provider }} · {{ phrase('发送次数') }}: {{ selectedEvent.attempts }}</p>
        <p v-if="selectedEvent.lastAttemptAt">{{ phrase('最近尝试') }}: {{ formatDateTime(selectedEvent.lastAttemptAt) }}</p>
        <p v-if="selectedEvent.delivery === 'failed'">{{ phrase('发送失败会自动重试，最长 24 小时；本地记录已保存。') }}</p>
        <p v-if="selectedEvent.delivery === 'cancelled'">{{ phrase('外部推送已关闭、配置已变化或重试已到期，本地记录仍保留。') }}</p>
      </div>
    </ModalDialog>
  </div>
</template>

<style scoped>
.notification-history { min-width: 0; gap: 12px; container: notification-history / inline-size; }
.notification-history__summary { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 6px 24px; }
.notification-history__intro { margin: 0; color: var(--text-soft); font-size: 14px; line-height: 1.6; }
.notification-history__filters { display: flex; flex-wrap: wrap; gap: 16px; align-items: end; }
.notification-history__filters label { display: grid; gap: 6px; flex: 1 1 145px; min-width: 0; font-size: 14px; }
.notification-history__filters input, .notification-history__filters select { width: 100%; min-width: 0; min-height: 40px; font-size: 14px; }
.notification-history__filters .notification-history__search { flex-basis: 240px; }
.notification-history__search > div { display: flex; align-items: center; gap: 8px; }
.notification-history__retention { color: var(--muted); font-size: 13px; line-height: 1.5; margin: 0; }
.notification-history__list { display: grid; gap: 12px; min-width: 0; }
.notification-history__list > button { grid-column: 1 / -1; justify-self: center; }
.notification-history__table { width: 100%; border-spacing: 0; table-layout: fixed; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); font-size: 14px; line-height: 1.5; }
.notification-history__table th { text-align: start; color: var(--text-soft); font-weight: 500; }
.notification-history__table th, .notification-history__table td { padding: 10px 12px; overflow-wrap: anywhere; vertical-align: middle; }
.notification-history__table td { border-top: 1px solid var(--border); }
.notification-history__table th:nth-child(1) { width: 108px; }
.notification-history__table th:nth-child(2) { width: 180px; }
.notification-history__table th:nth-child(3) { width: 160px; }
.notification-history__table th:nth-child(5) { width: 100px; }
.notification-history__table th:nth-child(6) { width: 48px; }
.notification-history__time { color: var(--text-soft); font-size: 13px; }
.notification-history__host-label { display: flex; align-items: center; gap: 8px; min-width: 0; }
.notification-history__host-label strong { min-width: 0; }
.notification-history__host-label :deep(.os-identity__mark) { width: 24px; height: 24px; }
.notification-history__host-label :deep(.os-identity__mark svg), .notification-history__host-label :deep(.os-identity__mark img) { width: 16px; height: 16px; }
.notification-history__type > span + span { margin-inline-start: 8px; }
.notification-history__kind { font-weight: 600; }
.notification-history__kind[data-kind="alert"] { color: var(--danger); }
.notification-history__kind[data-kind="recovery"] { color: var(--success); }
.notification-history__failure { color: var(--danger); }
.notification-history__fields { display: flex; flex-wrap: wrap; gap: 4px 16px; margin: 0; }
.notification-history__fields > div { display: flex; gap: 6px; min-width: 0; }
.notification-history__fields dt { flex: none; color: var(--text-soft); }
.notification-history__fields dd { margin: 0; min-width: 0; font-variant-numeric: tabular-nums; }
.notification-history__action .icon-button { width: 28px; height: 28px; }
.notification-history__original { font-size: 14px; line-height: 1.6; overflow-wrap: anywhere; }
.notification-history__original p { white-space: pre-wrap; }
@container notification-history (max-width: 740px) {
  .notification-history__table, .notification-history__table tbody { display: block; }
  .notification-history__table thead { display: none; }
  .notification-history__event { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 12px; padding: 12px; border-top: 1px solid var(--border); }
  .notification-history__event:first-child { border-top: 0; }
  .notification-history__table td { padding: 0; border: 0; }
  .notification-history__host { grid-column: 1; grid-row: 1; }
  .notification-history__time { grid-column: 2; grid-row: 1; }
  .notification-history__type { grid-column: 1; grid-row: 2; }
  .notification-history__delivery { grid-column: 2; grid-row: 2; }
  .notification-history__content { grid-column: 1; grid-row: 3; }
  .notification-history__action { grid-column: 2; grid-row: 3; justify-self: end; }
}
</style>
