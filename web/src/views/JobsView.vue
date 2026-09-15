<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from '@/i18n'
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
import ProblemReportButton from '@/components/problem-report/ProblemReportButton.vue'
import type { ReportError } from '@/components/problem-report/report'
import { ApiError, api } from '@/lib/api'
import { formatDateTime, relativeTime, shortId } from '@/lib/format'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import type { Job, JobOwner, JobSourceStatus, JobStatus } from '@/types/api'

type JobFilter = 'all' | 'active' | 'succeeded' | 'failed'

const i18n = useI18n()
const jobs = ref<Job[]>([])
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const reportError = ref<ReportError>()
const search = ref('')
const filter = ref<JobFilter>('all')
const route = useRoute()
const router = useRouter()
const selectedJobId = ref(typeof route.query.job === 'string' ? route.query.job.slice(0, 160) : '')
const selectedAction = ref('')
const detail = ref<Job>()
const detailError = ref('')
const detailReportError = ref<ReportError>()
const detailLoading = ref(false)
const sources = ref<JobSourceStatus[]>([])
const partial = ref(false)
const unavailableSources = computed(() => sources.value.filter((source) => source.state !== 'available'))
const selectedOwner = computed(() => /^(docker|app|webenv|file-archive|backup):([a-f0-9]{32})$/.exec(selectedJobId.value))
const selectedJob = computed(() => selectedOwner.value ? detail.value : error.value ? undefined : jobs.value.find((job) => job.id === selectedJobId.value))
const businessPath = computed(() => {
  const action = selectedJob.value?.action || selectedAction.value
  const owner = selectedOwner.value?.[1]
  if (owner === 'docker') return '/docker'
  if (owner === 'app') return '/apps'
  if (owner === 'webenv') return '/sites'
  if (owner === 'file-archive') return '/files'
  if (owner === 'backup') return '/settings'
  if (action.startsWith('docker.')) return '/docker'
  if (action.startsWith('app.')) return '/apps'
  if (action.startsWith('site.') || action.startsWith('web.environment.')) return '/sites'
  return ''
})
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
let controller: AbortController | undefined
let timer: number | undefined
let detailController: AbortController | undefined
let detailTimer: number | undefined

function ownerLabel(source: JobSourceStatus['source']): string {
  if (source === 'file-archive') return i18n.t('files.archive.jobs')
  if (source === 'backup') return '备份与恢复'
  return phrase({ docker: 'Docker', app: '应用', webenv: '网站环境', audit: '操作记录' }[source])
}

function selectJob(id: string, action = ''): void {
  selectedAction.value = action
  selectedJobId.value = id
  void router.replace({ query: { ...route.query, job: id || undefined } })
}

async function loadDetail(): Promise<void> {
  if (detailTimer) window.clearTimeout(detailTimer)
  detailTimer = undefined
  detailController?.abort()
  const owner = selectedOwner.value
  if (!owner || !desktopWindowActive.value) return
  const identity = selectedJobId.value
  const current = new AbortController()
  detailController = current
  detailLoading.value = true
  detailError.value = ''
  detailReportError.value = undefined
  try {
    const job = await api.jobs.detail(owner[1] as JobOwner, owner[2]!, current.signal)
    if (detailController !== current || current.signal.aborted || selectedJobId.value !== identity) return
    if (job.id !== identity) throw new Error('identity mismatch')
    detail.value = job
  } catch (reason) {
    if (detailController !== current || current.signal.aborted || selectedJobId.value !== identity) return
    detail.value = undefined
    detailReportError.value = reason instanceof ApiError
      ? { code: reason.code, status: reason.status, requestId: reason.requestId } : undefined
    detailError.value = reason instanceof ApiError && reason.status === 404
      ? '任务不存在或已超出来源保留期'
      : '无法确认任务详情，请稍后刷新或返回业务页面查看'
  } finally {
    if (detailController !== current) return
    detailLoading.value = false
    if (!current.signal.aborted && desktopWindowActive.value && selectedJobId.value === identity) {
      detailTimer = window.setTimeout(() => void loadDetail(), 4_000)
    }
  }
}

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
  if (action === 'file.compress') return i18n.t('files.archive.createTitle')
  if (action === 'file.extract') return i18n.t('files.archive.extractTitle')
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
  const archiveLabels: Record<string, string> = {
    cancelling: i18n.t('files.archive.state.cancelling'),
    complete: i18n.t('files.archive.state.complete'),
    partial: i18n.t('files.archive.state.partial'),
    error: i18n.t('files.archive.state.error'),
    cancelled: i18n.t('files.archive.state.cancelled'),
  }
  if (archiveLabels[stage]) return archiveLabels[stage]
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
  reportError.value = undefined

  try {
    const result = await api.jobs.list({ limit: 50 }, current.signal)
    if (controller !== current || current.signal.aborted) return
    jobs.value = result.items
    sources.value = result.sources || []
    partial.value = Boolean(result.partial)
  } catch (reason) {
    if (controller !== current || current.signal.aborted) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    jobs.value = []
    sources.value = []
    partial.value = false
    reportError.value = reason instanceof ApiError
      ? { code: reason.code, status: reason.status, requestId: reason.requestId } : undefined
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
    if (!current.signal.aborted && desktopWindowActive.value && (partial.value || selectedJobId.value || error.value || jobs.value.some((job) => isActive(job.status) || job.stages?.some((stage) => stage.name === 'persistence_pending' || stage.name === 'status_unavailable')))) {
      timer = window.setTimeout(() => void load({ silent: true }), 4_000)
    }
  }
}

onMounted(() => {
  if (desktopWindowActive.value) void load()
  void loadDetail()
})

watch(() => route.query.job, (id) => {
  selectedJobId.value = typeof id === 'string' ? id.slice(0, 160) : ''
})

watch(selectedJobId, () => {
  detail.value = undefined
  detailError.value = ''
  detailReportError.value = undefined
  void loadDetail()
  if (selectedJobId.value && desktopWindowActive.value && !refreshing.value && !timer) {
    timer = window.setTimeout(() => void load({ silent: true }), 4_000)
  }
})

watch(desktopWindowActive, (active) => {
  if (active) { void load({ silent: true }); void loadDetail() }
  else {
    controller?.abort()
    if (timer) window.clearTimeout(timer)
    timer = undefined
    detailController?.abort()
    detail.value = undefined
    if (detailTimer) window.clearTimeout(detailTimer)
    detailTimer = undefined
  }
})

onBeforeUnmount(() => {
  controller?.abort()
  detailController?.abort()
  if (detailTimer) window.clearTimeout(detailTimer)
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
      <button class="icon-button" type="button" :disabled="refreshing" title="刷新变更记录" aria-label="刷新变更记录" @click="load({ silent: true }); loadDetail()">
        <RefreshCw :size="17" :class="{ spin: refreshing }" />
      </button>
    </section>

    <div v-if="partial" class="inline-alert inline-alert--warning" role="status">
      <span>{{ phrase('记录不完整，以下来源暂不可用；其他记录已更新。') }}
        {{ unavailableSources.map((source) => ownerLabel(source.source)).join('、') }}
      </span>
    </div>
    <!-- Keep the teleported report mounted while background reads change the list state. -->
    <ErrorState v-show="!loading && error && !jobs.length" :message="error" :report-error="reportError" report-feature="jobs" @retry="load()" />
    <LoadingState v-if="loading" :rows="5" />
    <EmptyState
      v-else-if="!error && !filteredJobs.length"
      :title="jobs.length ? '没有符合条件的记录' : '暂无变更记录'"
      description="执行操作后，任务进度与操作记录会显示在这里。"
    />

    <section v-else-if="!error" class="job-list">
      <button v-for="job in filteredJobs" :key="job.id" class="job-item" type="button" @click="selectJob(job.id, job.action)">
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
      @close="selectJob('')"
    >
      <LoadingState v-if="selectedOwner && detailLoading && !selectedJob" :rows="2" />
      <template v-else-if="selectedJob">
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
        {{ phrase(detailError || error || '该记录已不在当前查询窗口中，请返回业务页面核对状态。') }}
      </div>
      <template #footer>
        <ProblemReportButton
          source="job" feature="jobs" :job="selectedJob"
          :error="selectedOwner ? detailReportError : reportError"
          :record-state="(selectedOwner ? detailLoading : refreshing || loading) ? 'refreshing' : selectedJob ? 'available' : 'unavailable'"
        />
        <RouterLink v-if="businessPath" class="button button--secondary" :to="businessPath">{{ phrase('返回业务页面') }}</RouterLink>
        <button v-if="selectedOwner" class="button button--secondary" type="button" :disabled="detailLoading" @click="loadDetail()">{{ phrase('刷新任务详情') }}</button>
        <button class="button button--secondary" type="button" @click="selectJob('')">{{ phrase('关闭') }}</button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.inline-alert {
  font-size: 0.875rem;
  line-height: 1.5;
}

.job-item small {
  font-size: 0.8125rem;
}
</style>
