<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Filter, LoaderCircle, RefreshCw, Search, ShieldCheck } from '@lucide/vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/AuditView/en-US').then((module) => module.default)
  : import('@/i18n/pages/AuditView/zh-TW').then((module) => module.default))
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api } from '@/lib/api'
import { formatDateTime, shortId } from '@/lib/format'
import type { AuditEvent } from '@/types/api'

const events = ref<AuditEvent[]>([])
const loading = ref(true)
const refreshing = ref(false)
const loadingMore = ref(false)
const error = ref('')
const nextCursor = ref('')
const search = ref('')
const outcome = ref('')
const source = ref('')
const selectedEvent = ref<AuditEvent>()
let controller: AbortController | undefined

const filteredEvents = computed(() => {
  const query = search.value.trim().toLowerCase()
  return events.value.filter(
    (event) =>
      (!outcome.value || event.outcome === outcome.value) &&
      (!source.value || event.source === source.value) &&
      (!query ||
        event.action.toLowerCase().includes(query) ||
        event.actor.toLowerCase().includes(query) ||
        event.resourceName?.toLowerCase().includes(query) ||
        event.requestId?.toLowerCase().includes(query)),
  )
})

function sourceLabel(value: AuditEvent['source']): string {
  return {
    web: 'Web',
    cli: 'CLI',
    reconcile: '自动核对',
    system: '系统',
    external: '外部变更',
  }[value]
}

async function load(options: { silent?: boolean; append?: boolean } = {}): Promise<void> {
  if (!options.append) {
    controller?.abort()
    controller = new AbortController()
  }
  if (options.append) loadingMore.value = true
  else if (options.silent) refreshing.value = true
  else loading.value = true
  error.value = ''

  try {
    const result = await api.audit.list(
      { cursor: options.append ? nextCursor.value : undefined },
      controller?.signal,
    )
    events.value = options.append ? [...events.value, ...result.items] : result.items
    nextCursor.value = result.nextCursor || ''
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取审计记录。'
  } finally {
    loading.value = false
    refreshing.value = false
    loadingMore.value = false
  }
}

onMounted(() => void load())
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <div class="page">
    <PageHeader title="审计记录" description="记录登录、Web、CLI 与外部变更，敏感信息在保存前完成脱敏。" />

    <div class="audit-assurance">
      <span><ShieldCheck :size="20" /></span>
      <div>
        <strong>审计链路已启用</strong>
        <p>Cookie、Token、私钥、数据库密码及 Docker 环境变量不会写入审计详情。</p>
      </div>
    </div>

    <section class="toolbar-card toolbar-card--audit">
      <div class="search-field">
        <Search :size="17" />
        <input v-model="search" type="search" placeholder="搜索动作、操作者、资源或请求 ID" aria-label="搜索审计记录" />
      </div>
      <label class="select-field">
        <Filter :size="15" />
        <select v-model="source" aria-label="按来源筛选">
          <option value="">全部来源</option>
          <option value="web">Web</option>
          <option value="cli">CLI</option>
          <option value="reconcile">自动核对</option>
          <option value="external">外部变更</option>
          <option value="system">系统</option>
        </select>
      </label>
      <label class="select-field">
        <select v-model="outcome" aria-label="按结果筛选">
          <option value="">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
          <option value="denied">拒绝</option>
          <option value="observed">观测</option>
        </select>
      </label>
      <button class="icon-button" type="button" :disabled="refreshing" title="刷新审计记录" aria-label="刷新审计记录" @click="load({ silent: true })">
        <RefreshCw :size="17" :class="{ spin: refreshing }" />
      </button>
    </section>

    <LoadingState v-if="loading" :rows="6" />
    <ErrorState v-else-if="error && !events.length" :message="error" @retry="load()" />
    <EmptyState
      v-else-if="!filteredEvents.length"
      :title="events.length ? '没有符合条件的记录' : '暂无审计记录'"
      description="完成登录、自动核对或管理操作后，审计事件会显示在这里。"
    />

    <section v-else class="table-card">
      <div class="table-scroll">
        <table class="data-table">
          <thead><tr><th>时间</th><th>操作者</th><th>动作</th><th>资源</th><th>结果</th><th>请求 ID</th><th></th></tr></thead>
          <tbody>
            <tr v-for="event in filteredEvents" :key="event.id">
              <td><span class="table-time">{{ formatDateTime(event.occurredAt) }}</span></td>
              <td>
                <div class="table-stack">
                  <strong>{{ event.actor }}</strong>
                  <small>{{ sourceLabel(event.source) }}</small>
                </div>
              </td>
              <td><code class="action-code">{{ event.action }}</code></td>
              <td>
                <div class="table-stack">
                  <span>{{ event.resourceName || '系统' }}</span>
                  <small>{{ event.resourceType || '—' }}</small>
                </div>
              </td>
              <td><StatusBadge :status="event.outcome" /></td>
              <td><code>{{ shortId(event.requestId) }}</code></td>
              <td class="table-actions">
                <button class="button button--ghost button--small" type="button" @click="selectedEvent = event">详情</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer class="table-card__footer">
        <span>当前显示 {{ filteredEvents.length }} 条记录</span>
        <button v-if="nextCursor" class="button button--secondary button--small" type="button" :disabled="loadingMore" @click="load({ append: true })">
          <LoaderCircle v-if="loadingMore" class="spin" :size="15" />
          加载更多
        </button>
      </footer>
    </section>

    <ModalDialog
      :open="Boolean(selectedEvent)"
      title="审计事件详情"
      :description="selectedEvent ? `事件 ${selectedEvent.id}` : ''"
      @close="selectedEvent = undefined"
    >
      <template v-if="selectedEvent">
        <div class="modal-status-row">
          <StatusBadge :status="selectedEvent.outcome" />
          <span>{{ sourceLabel(selectedEvent.source) }}</span>
        </div>
        <dl class="detail-list">
          <div><dt>时间</dt><dd>{{ formatDateTime(selectedEvent.occurredAt) }}</dd></div>
          <div><dt>操作者</dt><dd>{{ selectedEvent.actor }}</dd></div>
          <div><dt>来源地址</dt><dd>{{ selectedEvent.remoteAddress || '未记录' }}</dd></div>
          <div><dt>动作</dt><dd><code>{{ selectedEvent.action }}</code></dd></div>
          <div><dt>目标</dt><dd>{{ selectedEvent.resourceType || '系统' }} / {{ selectedEvent.resourceName || '—' }}</dd></div>
          <div><dt>请求 ID</dt><dd><code>{{ selectedEvent.requestId || '—' }}</code></dd></div>
          <div v-if="selectedEvent.summary"><dt>摘要</dt><dd>{{ selectedEvent.summary }}</dd></div>
        </dl>
      </template>
      <template #footer>
        <button class="button button--secondary" type="button" @click="selectedEvent = undefined">关闭</button>
      </template>
    </ModalDialog>
  </div>
</template>
