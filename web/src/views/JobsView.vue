<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { CheckCircle2, Clock3, LoaderCircle, RefreshCw, RotateCw, Search, TimerReset } from '@lucide/vue'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/JobsView/en-US').then((module) => module.default)
  : import('@/i18n/pages/JobsView/zh-TW').then((module) => module.default))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}
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
const selectedJobId = ref('')
const selectedAction = ref('')
const selectedJob = computed(() => error.value ? undefined : jobs.value.find((job) => job.id === selectedJobId.value))
const businessPath = computed(() => {
  const action = selectedAction.value
  if (action.startsWith('docker.')) return '/docker'
  if (action.startsWith('app.')) return '/apps'
  if (action.startsWith('site.') || action.startsWith('web.environment.')) return '/sites'
  return ''
})
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

function stageLabel(stage: string): string {
  const labels: Record<string, string> = {
    queued: '等待执行', running: '执行中', executing: '执行中', completed: '执行完成', failed: '执行失败',
    interrupted: '执行中断', not_started: '尚未执行', persistence_pending: '状态待保存',
    status_unavailable: '结果未确认', outcome_unknown: '结果未确认', attention_required: '需要检查',
  }
  return phrase(labels[stage] || stage)
}

function jobErrorMessage(job: Job): string {
  if (job.stages?.some((stage) => stage.name === 'persistence_pending') && job.id.startsWith('docker:')) {
    return phrase(job.startedAt
      ? 'Docker 任务执行已停止，但结果尚未保存；请恢复任务存储并刷新，系统只重试保存，不会重复执行'
      : 'Docker 任务尚未执行，无法保存任务状态；请恢复任务存储并刷新，系统只重试保存，不会自动执行')
  }
  if (job.errorCode === 'job_status_unavailable') return phrase('任务不在当前可查询记录中，请返回业务页面核对资源状态')
  if (job.errorCode === 'job_outcome_unknown') return phrase('仅有操作提交记录，执行结果未确认')
  return job.errorMessage || ''
}

async function load(options: { silent?: boolean } = {}): Promise<void> {
  if (timer) window.clearTimeout(timer)
  timer = undefined
  controller?.abort()
  const current = new AbortController()
  controller = current
  if (options.silent) refreshing.value = true
  else loading.value = true
  error.value = ''

  try {
    const result = await api.jobs.list({ limit: 50 }, current.signal)
    if (controller !== current || current.signal.aborted) return
    jobs.value = result.items
  } catch (reason) {
    if (controller !== current || current.signal.aborted) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    jobs.value = []
    if (reason instanceof ApiError && reason.status === 404) {
      error.value = '当前服务版本尚未开放任务查询接口。'
    } else if (reason instanceof ApiError && reason.code === 'job_source_unavailable') {
      error.value = '无法确认后台任务状态，请稍后刷新或返回业务页面查看'
    } else if (reason instanceof ApiError && reason.code === 'job_source_invalid') {
      error.value = '后台任务状态无效，请稍后刷新'
    } else {
      error.value = reason instanceof ApiError ? reason.message : '无法读取任务记录。'
    }
  } finally {
    if (controller !== current) return
    loading.value = false
    refreshing.value = false
    if (!current.signal.aborted && desktopWindowActive.value && (selectedJobId.value || error.value || jobs.value.some((job) => isActive(job.status) || job.stages?.some((stage) => stage.name === 'persistence_pending' || stage.name === 'status_unavailable')))) {
      timer = window.setTimeout(() => void load({ silent: true }), 4_000)
    }
  }
}

onMounted(() => {
  if (desktopWindowActive.value) void load()
})

watch(selectedJobId, (id) => {
  if (id && desktopWindowActive.value && !refreshing.value && !timer) {
    timer = window.setTimeout(() => void load({ silent: true }), 4_000)
  }
})

watch(desktopWindowActive, (active) => {
  if (active) void load({ silent: true })
  else {
    controller?.abort()
    if (timer) window.clearTimeout(timer)
    timer = undefined
  }
})

onBeforeUnmount(() => {
  controller?.abort()
  if (timer) window.clearTimeout(timer)
})
</script>

<template>
  <div class="page">
    <PageHeader title="变更记录" description="集中查看后台任务进度与操作记录。" />

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
      description="执行操作后，任务进度与操作记录会显示在这里。"
    />

    <section v-else class="job-list">
      <button v-for="job in filteredJobs" :key="job.id" class="job-item" type="button" @click="selectedJobId = job.id; selectedAction = job.action">
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
          <span v-else-if="job.errorMessage" class="job-item__error">{{ jobErrorMessage(job) }}</span>
        </span>
        <code>{{ shortId(job.id) }}</code>
      </button>
    </section>

    <ModalDialog
      :open="Boolean(selectedJobId)"
      :title="selectedJob ? phrase(actionLabel(selectedJob.action)) : phrase('任务详情')"
      :description="selectedJob ? phrase(`任务 ${selectedJob.id}`) : ''"
      size="large"
      @close="selectedJobId = ''"
    >
      <template v-if="selectedJob">
        <div class="modal-status-row">
          <StatusBadge :status="selectedJob.status" />
          <span>{{ phrase(sourceLabel(selectedJob.source)) }}</span>
          <span v-if="selectedJob.progress !== undefined">{{ phrase(`进度 ${selectedJob.progress}%`) }}</span>
        </div>
        <dl class="detail-list detail-list--grid">
          <div>
            <dt>{{ phrase('目标类型') }}</dt>
            <dd>{{ selectedJob.resourceType || phrase('系统') }}</dd>
          </div>
          <div>
            <dt>{{ phrase('目标资源') }}</dt>
            <dd>{{ selectedJob.resourceName || '—' }}</dd>
          </div>
          <div>
            <dt>{{ phrase('创建时间') }}</dt>
            <dd>{{ formatDateTime(selectedJob.createdAt) }}</dd>
          </div>
          <div>
            <dt>{{ phrase('完成时间') }}</dt>
            <dd>{{ formatDateTime(selectedJob.finishedAt) }}</dd>
          </div>
        </dl>
        <section v-if="selectedJob.stages?.length" class="detail-section">
          <h3><RotateCw :size="17" /> {{ phrase('执行阶段') }}</h3>
          <ol class="stage-list">
            <li v-for="stage in selectedJob.stages" :key="stage.name">
              <span class="stage-list__marker" />
              <div>
                <strong>{{ stageLabel(stage.name) }}</strong>
                <small>{{ stage.message || formatDateTime(stage.finishedAt || stage.startedAt) }}</small>
              </div>
              <StatusBadge :status="selectedJob.status" subtle />
            </li>
          </ol>
        </section>
        <div v-if="selectedJob.errorMessage" class="inline-alert inline-alert--danger">
          <span><strong>{{ selectedJob.errorCode || phrase('任务失败') }}</strong><br />{{ jobErrorMessage(selectedJob) }}</span>
        </div>
      </template>
      <div v-else-if="selectedJobId" class="inline-alert inline-alert--warning" role="status">
        {{ phrase(error || '该记录已不在当前查询窗口中，请返回业务页面核对状态。') }}
      </div>
      <template #footer>
        <RouterLink v-if="businessPath" class="button button--secondary" :to="businessPath">{{ phrase('返回业务页面') }}</RouterLink>
        <button class="button button--secondary" type="button" @click="selectedJobId = ''">{{ phrase('关闭') }}</button>
      </template>
    </ModalDialog>
  </div>
</template>
