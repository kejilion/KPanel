<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CheckCircle2, Clock3, LoaderCircle, RefreshCw, RotateCw, Search, TimerReset } from '@lucide/vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/JobsView/en-US').then((module) => module.default)
  : import('@/i18n/pages/JobsView/zh-TW').then((module) => module.default))
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import { ApiError, api } from '@/lib/api'
import { formatDateTime, relativeTime, shortId } from '@/lib/format'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { Job, JobStatus } from '@/types/api'

type JobFilter = 'all' | 'active' | 'succeeded' | 'failed'

const jobs = ref<Job[]>([])
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const search = ref('')
const filter = ref<JobFilter>('all')
const selectedJob = ref<Job>()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
let controller: AbortController | undefined
let timer: number | undefined

const isActive = (status: JobStatus) => status === 'queued' || status === 'running'
const isFailure = (status: JobStatus) =>
  ['failed', 'failed_rolled_back', 'failed_needs_attention', 'interrupted'].includes(status)

const filteredJobs = computed(() => {
  const query = search.value.trim().toLowerCase()
  return jobs.value.filter((job) => {
    const matchesQuery =
      !query ||
      job.action.toLowerCase().includes(query) ||
      job.id.toLowerCase().includes(query) ||
      job.resourceName?.toLowerCase().includes(query)
    if (!matchesQuery) return false
    if (filter.value === 'active') return isActive(job.status)
    if (filter.value === 'succeeded') return job.status === 'succeeded'
    if (filter.value === 'failed') return isFailure(job.status)
    return true
  })
})

const counts = computed(() => ({
  all: jobs.value.length,
  active: jobs.value.filter((job) => isActive(job.status)).length,
  succeeded: jobs.value.filter((job) => job.status === 'succeeded').length,
  failed: jobs.value.filter((job) => isFailure(job.status)).length,
}))

function actionLabel(action: string): string {
  const labels: Record<string, string> = {
    'site.create': '创建网站',
    'site.update': '更新网站',
    'docker.start': '启动容器',
    'docker.stop': '停止容器',
    'docker.restart': '重启容器',
    'app.install': '安装应用',
  }
  return labels[action] || action
}

function sourceLabel(source?: Job['source']): string {
  return { web: 'Web', cli: 'CLI', reconcile: '自动核对', system: '系统' }[source || 'system']
}

async function load(options: { silent?: boolean } = {}): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  if (options.silent) refreshing.value = true
  else loading.value = true
  error.value = ''

  try {
    const result = await api.jobs.list({ limit: 50 }, controller.signal)
    jobs.value = result.items
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (reason instanceof ApiError && reason.status === 404) {
      error.value = '当前服务版本尚未开放任务查询接口。'
    } else {
      error.value = reason instanceof ApiError ? reason.message : '无法读取任务记录。'
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

onMounted(() => {
  if (desktopWindowActive.value) void load()
  timer = window.setInterval(() => {
    if (desktopWindowActive.value && jobs.value.some((job) => isActive(job.status))) void load({ silent: true })
  }, 4_000)
})

watch(desktopWindowActive, (active) => {
  if (active) void load({ silent: true })
  else controller?.abort()
})

onBeforeUnmount(() => {
  controller?.abort()
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <div class="page">
    <PageHeader title="变更记录" description="集中查看网站与容器变更的执行进度和结果。" />

    <section class="toolbar-card toolbar-card--search-tabs">
      <div class="search-field">
        <Search :size="17" />
        <input v-model="search" type="search" placeholder="搜索变更、资源或记录 ID" aria-label="搜索变更记录" />
      </div>
      <div class="filter-tabs" role="tablist" aria-label="变更记录筛选">
        <button
          v-for="item in [
            { key: 'all', label: '全部' },
            { key: 'active', label: '进行中' },
            { key: 'succeeded', label: '已完成' },
            { key: 'failed', label: '异常' },
          ]"
          :key="item.key"
          type="button"
          role="tab"
          :aria-selected="filter === item.key"
          :class="{ 'is-active': filter === item.key }"
          @click="filter = item.key as JobFilter"
        >
          {{ item.label }} <span>{{ counts[item.key as JobFilter] }}</span>
        </button>
      </div>
      <button class="icon-button" type="button" :disabled="refreshing" title="刷新变更记录" aria-label="刷新变更记录" @click="load({ silent: true })">
        <RefreshCw :size="17" :class="{ spin: refreshing }" />
      </button>
    </section>

    <LoadingState v-if="loading" :rows="5" />
    <ErrorState v-else-if="error && !jobs.length" :message="error" @retry="load()" />
    <EmptyState
      v-else-if="!filteredJobs.length"
      :title="jobs.length ? '没有符合条件的记录' : '暂无变更记录'"
      description="通过 KPanel 执行网站或容器变更后，进度与结果会显示在这里。"
    />

    <section v-else class="job-list">
      <button v-for="job in filteredJobs" :key="job.id" class="job-item" type="button" @click="selectedJob = job">
        <span class="job-item__status" :class="`is-${job.status}`">
          <LoaderCircle v-if="job.status === 'running'" class="spin" :size="19" />
          <Clock3 v-else-if="job.status === 'queued'" :size="19" />
          <CheckCircle2 v-else-if="job.status === 'succeeded'" :size="19" />
          <TimerReset v-else :size="19" />
        </span>
        <span class="job-item__main">
          <span>
            <strong>{{ actionLabel(job.action) }}</strong>
            <StatusBadge :status="job.status" subtle />
          </span>
          <small>
            {{ job.resourceName || job.resourceType || '系统任务' }} · {{ sourceLabel(job.source) }} ·
            {{ relativeTime(job.createdAt) }}
          </small>
          <span v-if="isActive(job.status)" class="job-progress">
            <i><b :style="{ width: `${job.progress || 0}%` }" /></i>
            <em>{{ job.progress || 0 }}%</em>
          </span>
          <span v-else-if="job.errorMessage" class="job-item__error">{{ job.errorMessage }}</span>
        </span>
        <code>{{ shortId(job.id) }}</code>
      </button>
    </section>

    <ModalDialog
      :open="Boolean(selectedJob)"
      :title="selectedJob ? actionLabel(selectedJob.action) : '任务详情'"
      :description="selectedJob ? `任务 ${selectedJob.id}` : ''"
      size="large"
      @close="selectedJob = undefined"
    >
      <template v-if="selectedJob">
        <div class="modal-status-row">
          <StatusBadge :status="selectedJob.status" />
          <span>{{ sourceLabel(selectedJob.source) }}</span>
          <span v-if="selectedJob.progress !== undefined">进度 {{ selectedJob.progress }}%</span>
        </div>
        <dl class="detail-list detail-list--grid">
          <div>
            <dt>目标类型</dt>
            <dd>{{ selectedJob.resourceType || '系统' }}</dd>
          </div>
          <div>
            <dt>目标资源</dt>
            <dd>{{ selectedJob.resourceName || '—' }}</dd>
          </div>
          <div>
            <dt>创建时间</dt>
            <dd>{{ formatDateTime(selectedJob.createdAt) }}</dd>
          </div>
          <div>
            <dt>完成时间</dt>
            <dd>{{ formatDateTime(selectedJob.finishedAt) }}</dd>
          </div>
        </dl>
        <section v-if="selectedJob.stages?.length" class="detail-section">
          <h3><RotateCw :size="17" /> 执行阶段</h3>
          <ol class="stage-list">
            <li v-for="stage in selectedJob.stages" :key="stage.name">
              <span class="stage-list__marker" />
              <div>
                <strong>{{ stage.name }}</strong>
                <small>{{ stage.message || formatDateTime(stage.finishedAt || stage.startedAt) }}</small>
              </div>
              <StatusBadge :status="stage.status" subtle />
            </li>
          </ol>
        </section>
        <div v-if="selectedJob.errorMessage" class="inline-alert inline-alert--danger">
          <span><strong>{{ selectedJob.errorCode || '任务失败' }}</strong><br />{{ selectedJob.errorMessage }}</span>
        </div>
      </template>
      <template #footer>
        <button class="button button--secondary" type="button" @click="selectedJob = undefined">关闭</button>
      </template>
    </ModalDialog>
  </div>
</template>
