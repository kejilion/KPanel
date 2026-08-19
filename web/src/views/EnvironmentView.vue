<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/EnvironmentView/en-US').then((module) => module.default)
  : import('@/i18n/pages/EnvironmentView/zh-TW').then((module) => module.default))
import {
  Archive,
  Box,
  CheckCircle2,
  Database,
  Download,
  Gauge,
  HardDrive,
  LoaderCircle,
  Play,
  RefreshCw,
  RotateCcw,
  Server,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  Zap,
} from '@lucide/vue'
import AppInteractiveTerminal from '@/components/apps/AppInteractiveTerminal.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import SitesSectionTabs from '@/components/sites/SitesSectionTabs.vue'
import { ApiError, api } from '@/lib/api'
import { formatBytes, formatDateTime } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type {
  WebEnvironmentActionInput,
  WebEnvironmentBackup,
  WebEnvironmentCatalog,
  WebEnvironmentJob,
  WebEnvironmentSummary,
} from '@/types/api'

type Section = 'overview' | 'protection' | 'optimization' | 'update' | 'backup' | 'uninstall'

const sections: Array<{ id: Section; label: string }> = [
  { id: 'overview', label: '总览' },
  { id: 'protection', label: '防护' },
  { id: 'optimization', label: '优化' },
  { id: 'update', label: '更新' },
  { id: 'backup', label: '备份还原' },
  { id: 'uninstall', label: '卸载' },
]

const section = ref<Section>('overview')
const summary = ref<WebEnvironmentSummary>()
const catalog = ref<WebEnvironmentCatalog>()
const backups = ref<WebEnvironmentBackup[]>([])
const jobs = ref<WebEnvironmentJob[]>([])
const loading = ref(true)
const refreshing = ref(false)
const catalogLoading = ref(false)
const backupsLoading = ref(false)
const error = ref('')
const auxiliaryErrors = reactive({ catalog: '', backups: '', jobs: '' })
const submitting = ref(false)
const terminalOpen = ref(false)
const terminalJob = ref<WebEnvironmentJob>()
const backupBeforeUninstall = ref(true)
const updateVersions = reactive<Record<string, string>>({})
const cloudflare = reactive({ account: '', token: '', zoneId: '' })
const toast = useToast()
let summaryController: AbortController | undefined
let catalogController: AbortController | undefined
let backupsController: AbortController | undefined
let jobsController: AbortController | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let pollGeneration = 0
let catalogLoaded = false
let backupsLoaded = false
let disposed = false

const auxiliaryWarning = computed(() =>
  Object.values(auxiliaryErrors).filter(Boolean).join('；'),
)

const activeJob = computed(() =>
  jobs.value.find((job) => ['queued', 'running', 'waiting_input'].includes(job.status)),
)
const visibleJob = computed(() => activeJob.value || jobs.value[0])
const environmentInstalled = computed(() => summary.value?.state !== 'absent')
const environmentLabel = computed(() => {
  if (!summary.value) return '未知'
  if (activeJob.value) return '正在维护'
  if (summary.value.state === 'absent') return '未安装'
  if (summary.value.state === 'partial') return '部分安装'
  return summary.value.health === 'healthy' ? '运行正常' : '运行异常'
})
const profileLabel = computed(() => {
  const labels: Record<string, string> = {
    full: '完整 LDNMP',
    nginx: '仅 Nginx',
    custom: '自定义 / 配置漂移',
    none: '未安装',
  }
  return labels[summary.value?.profile || 'none'] || summary.value?.profile || '未知'
})
const jobActive = computed(() => Boolean(activeJob.value) || submitting.value)

function jobStatusLabel(job: WebEnvironmentJob): string {
  const labels: Record<string, string> = {
    queued: '排队中',
    running: '后台运行中',
    waiting_input: '等待输入',
    succeeded: '已完成',
    failed: '执行失败',
    needs_attention: '需要人工处理',
  }
  return labels[job.status] || job.status
}

function jobMessageLabel(message: string): string {
  if (message === '冷备已完成并通过 SHA-256 校验') return message
  return message
}

function componentLabel(name: string): string {
  return { nginx: 'Nginx', mysql: 'MySQL', php: 'PHP', php74: 'PHP 7.4', redis: 'Redis' }[name] || name
}

function componentIcon(name: string) {
  if (name === 'mysql') return Database
  if (name === 'nginx') return Server
  if (name.startsWith('php')) return Box
  return Zap
}

function isAbort(reason: unknown): boolean {
  return reason instanceof Error && reason.name === 'AbortError'
}

async function loadJobs(): Promise<void> {
  jobsController?.abort()
  const requestController = new AbortController()
  jobsController = requestController
  try {
    const result = await api.webEnvironment.jobs(requestController.signal)
    if (jobsController !== requestController) return
    jobs.value = result.items
    auxiliaryErrors.jobs = ''
    schedulePoll()
  } catch (reason) {
    if (jobsController === requestController && !isAbort(reason)) {
      auxiliaryErrors.jobs = '任务记录暂时无法读取'
    }
  }
}

async function loadCatalog(force = false): Promise<void> {
  if (catalogLoaded && !force) return
  catalogController?.abort()
  const requestController = new AbortController()
  catalogController = requestController
  catalogLoading.value = true
  try {
    const result = await api.webEnvironment.catalog(requestController.signal)
    if (catalogController !== requestController) return
    catalog.value = result
    catalogLoaded = true
    auxiliaryErrors.catalog = ''
    for (const item of result.updateComponents) {
      updateVersions[item.id] ||= item.versions[0] || 'latest'
    }
  } catch (reason) {
    if (catalogController === requestController && !isAbort(reason)) {
      auxiliaryErrors.catalog = '版本目录暂时无法读取'
    }
  } finally {
    if (catalogController === requestController) catalogLoading.value = false
  }
}

async function loadBackups(force = false): Promise<void> {
  if (backupsLoaded && !force) return
  backupsController?.abort()
  const requestController = new AbortController()
  backupsController = requestController
  backupsLoading.value = true
  try {
    const result = await api.webEnvironment.backups(requestController.signal)
    if (backupsController !== requestController) return
    backups.value = result.items
    backupsLoaded = true
    auxiliaryErrors.backups = ''
  } catch (reason) {
    if (backupsController === requestController && !isAbort(reason)) {
      auxiliaryErrors.backups = '备份列表暂时无法读取'
    }
  } finally {
    if (backupsController === requestController) backupsLoading.value = false
  }
}

function loadSectionData(force = false): void {
  if (section.value === 'update') void loadCatalog(force)
  if (section.value === 'backup') void loadBackups(force)
}

async function load(silent = false): Promise<void> {
  summaryController?.abort()
  const requestController = new AbortController()
  summaryController = requestController
  if (silent) refreshing.value = true
  else loading.value = true
  try {
    const result = await api.webEnvironment.summary(requestController.signal)
    if (summaryController !== requestController) return
    summary.value = result
    error.value = ''
    void loadJobs()
    loadSectionData(silent)
  } catch (reason) {
    if (summaryController !== requestController || isAbort(reason)) return
    error.value = reason instanceof ApiError ? reason.message : '无法读取 LDNMP 环境状态。'
  } finally {
    if (summaryController === requestController) {
      loading.value = false
      refreshing.value = false
    }
  }
}

function stopJobPolling(): void {
  pollGeneration += 1
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = undefined
  pollController?.abort()
  pollController = undefined
}

function schedulePoll(delay = 1200): void {
  stopJobPolling()
  if (disposed || !activeJob.value) return
  const generation = pollGeneration
  pollTimer = window.setTimeout(() => {
    pollTimer = undefined
    if (!disposed && generation === pollGeneration) void pollActiveJob(generation)
  }, delay)
}

async function pollActiveJob(generation: number): Promise<void> {
  if (disposed || generation !== pollGeneration) return
  const current = activeJob.value
  if (!current) return
  const requestController = new AbortController()
  pollController?.abort()
  pollController = requestController
  try {
    const updated = await api.webEnvironment.job(current.id, requestController.signal)
    if (disposed || generation !== pollGeneration || requestController.signal.aborted) return
    jobs.value = jobs.value.map((item) => (item.id === updated.id ? updated : item))
    if (['queued', 'running', 'waiting_input'].includes(updated.status)) schedulePoll()
    else await load(true)
  } catch (reason) {
    if (disposed || generation !== pollGeneration || requestController.signal.aborted || isAbort(reason)) return
    schedulePoll(2200)
  } finally {
    if (pollController === requestController) pollController = undefined
  }
}

async function start(input: Omit<WebEnvironmentActionInput, 'expectedResourceVersion'>): Promise<void> {
  if (!summary.value || jobActive.value) return
  submitting.value = true
  try {
    const job = await api.webEnvironment.start({
      ...input,
      expectedResourceVersion: summary.value.resourceVersion,
    } as WebEnvironmentActionInput)
    jobs.value = [job, ...jobs.value.filter((item) => item.id !== job.id)]
    terminalJob.value = job
    terminalOpen.value = true
    toast.success('任务已转入后台', '关闭终端或刷新页面都不会中断执行。')
    schedulePoll()
  } catch (reason) {
    toast.danger('任务启动失败', reason instanceof ApiError ? reason.message : '请刷新状态后重试。')
  } finally {
    submitting.value = false
  }
}

function install(profile: 'full' | 'nginx'): void {
  const impact =
    profile === 'full'
      ? '将检查并可能调整软件源、DNS、Swap、Docker、Certbot，并占用 80/443 端口。'
      : '将安装 Nginx、Certbot 并可能占用 80/443 端口。'
  if (window.confirm(`${impact}\n\n确认继续？`)) void start({ action: 'install', profile })
}

function protection(operation: string): void {
  void start({ action: 'protection.configure', operation })
}

function configureCloudflare(operation: 'cloudflare-fail2ban' | 'cloudflare-shield'): void {
  if (!cloudflare.account || !cloudflare.token || (operation === 'cloudflare-shield' && !cloudflare.zoneId)) {
    toast.danger('Cloudflare 参数不完整', '请填写账号、API Key/Token；自动五秒盾还需要 Zone ID。')
    return
  }
  void start({
    action: 'protection.configure',
    operation,
    cloudflareAccount: cloudflare.account,
    cloudflareToken: cloudflare.token,
    ...(operation === 'cloudflare-shield' ? { cloudflareZoneId: cloudflare.zoneId } : {}),
  })
  cloudflare.token = ''
}

function optimize(operation: string): void {
  const warning =
    operation === 'high'
      ? '高性能模式会修改 Nginx、PHP、MySQL 和宿主机内核参数。确认继续？'
      : '将备份并调整对应环境配置。确认继续？'
  if (window.confirm(warning)) void start({ action: 'optimization.apply', operation })
}

function update(component: string): void {
  const version = updateVersions[component] || 'latest'
  const action = component === 'all' ? 'update.all' : 'update.component'
  const backupBeforeChange = component === 'mysql' || component === 'all'
  if (window.confirm(`将更新 ${component === 'all' ? '完整环境' : component} 到 ${version}。确认继续？`)) {
    void start({ action, component, version, backupBeforeChange } as Omit<
      WebEnvironmentActionInput,
      'expectedResourceVersion'
    >)
  }
}

function createBackup(): void {
  void start({ action: 'backup.create' })
}

function restore(backup: WebEnvironmentBackup): void {
  const verification = backup.verified ? '该备份带 SHA-256 校验。' : '这是传统备份，执行前会先做完整安全扫描。'
  if (window.confirm(`${verification}\n还原会短暂停止环境并替换 /home/web，失败时自动回滚。确认继续？`)) {
    void start({ action: 'restore', backupId: backup.id })
  }
}

function deleteBackup(backup: WebEnvironmentBackup): void {
  if (window.confirm(`永久删除备份 ${backup.id} 及其校验文件？`)) {
    void start({ action: 'backup.delete', backupId: backup.id })
  }
}

function uninstall(): void {
  if (
    window.confirm(
      '将删除 LDNMP/PhpMyAdmin 容器、镜像及 /home/web 内的网站和数据库；KPanel、Docker 和现有备份归档会保留。确认继续？',
    )
  ) {
    void start({ action: 'uninstall', backupBeforeChange: backupBeforeUninstall.value })
  }
}

function openTerminal(job?: WebEnvironmentJob): void {
  if (!job) return
  terminalJob.value = job
  terminalOpen.value = true
}

onMounted(() => void load())
watch(section, () => loadSectionData())
onBeforeUnmount(() => {
  disposed = true
  summaryController?.abort()
  catalogController?.abort()
  backupsController?.abort()
  jobsController?.abort()
  stopJobPolling()
})
</script>

<template>
  <div class="page environment-page">
    <PageHeader title="LDNMP 环境管理" description="与 kejilion.sh 共用 /home/web、Docker 和现有网站环境，直接管理服务器真实状态。" />

    <div class="page-command-bar">
      <SitesSectionTabs />
      <button class="button button--secondary" type="button" :disabled="refreshing" @click="load(true)">
        <RefreshCw :class="{ spin: refreshing }" :size="16" /> 刷新状态
      </button>
    </div>

    <LoadingState v-if="loading" />
    <ErrorState v-else-if="error" title="环境状态读取失败" :message="error" @retry="load()" />
    <template v-else-if="summary">
      <div v-if="auxiliaryWarning" class="inline-alert inline-alert--warning" role="status">
        {{ auxiliaryWarning }}，环境状态与可用管理功能不受影响。
      </div>
      <section
        v-if="visibleJob"
        class="environment-job-banner"
        :class="`is-${visibleJob.status}`"
        aria-live="polite"
      >
        <LoaderCircle v-if="activeJob" class="spin" :size="20" />
        <CheckCircle2 v-else-if="visibleJob.status === 'succeeded'" :size="20" />
        <TriangleAlert v-else :size="20" />
        <div>
          <strong>{{ visibleJob.target || 'LDNMP 环境任务' }}</strong>
          <small>{{ jobMessageLabel(visibleJob.message) }}</small>
          <i><b :style="{ width: `${visibleJob.progress}%` }" /></i>
        </div>
        <StatusBadge
          :status="activeJob ? 'running_job' : visibleJob.status"
          :label="jobStatusLabel(visibleJob)"
        />
        <strong>{{ visibleJob.progress }}%</strong>
        <button class="button button--secondary" type="button" @click="openTerminal(visibleJob)">
          查看终端
        </button>
      </section>

      <nav class="environment-tabs" aria-label="环境管理功能">
        <button
          v-for="item in sections"
          :key="item.id"
          type="button"
          :class="{ 'is-active': section === item.id }"
          @click="section = item.id"
        >
          {{ item.label }}
        </button>
      </nav>

      <template v-if="section === 'overview'">
        <section class="environment-hero">
          <div>
            <span>当前环境</span>
            <h2>{{ environmentLabel }}</h2>
            <p>{{ profileLabel }} · {{ summary.webRoot }}</p>
          </div>
          <StatusBadge
            :status="summary.health === 'healthy' ? 'healthy' : summary.state === 'absent' ? 'unknown' : 'degraded'"
            :label="environmentLabel"
          />
          <dl>
            <div><dt>占用</dt><dd>{{ formatBytes(summary.diskBytes) }}</dd></div>
            <div><dt>站点</dt><dd>{{ summary.siteCount }}</dd></div>
            <div><dt>数据库</dt><dd>{{ summary.databaseCount }}</dd></div>
            <div><dt>证书</dt><dd>{{ summary.certificateCount }}</dd></div>
          </dl>
        </section>

        <section class="environment-checks">
          <div><ShieldCheck :size="18" /><span>Compose 配置<strong>{{ summary.composeValid ? '有效' : '不可用' }}</strong></span></div>
          <div><Server :size="18" /><span>Nginx 配置<strong>{{ summary.nginxValid ? '有效' : '不可用' }}</strong></span></div>
          <div><Archive :size="18" /><span>最近备份<strong>{{ summary.latestBackup || '暂无' }}</strong></span></div>
          <div><RefreshCw :size="18" /><span>脚本 / 协议<strong>{{ summary.scriptVersion || '未知' }} / v{{ summary.protocolVersion }}</strong></span></div>
        </section>

        <section class="environment-components">
          <article v-for="component in summary.components" :key="component.name">
            <component :is="componentIcon(component.name)" :size="21" />
            <div>
              <strong>{{ componentLabel(component.name) }}</strong>
              <small>{{ component.image || '未发现镜像' }}</small>
              <code>{{ component.version || '—' }}</code>
            </div>
            <StatusBadge
              :status="component.running ? 'running' : component.exists ? component.state : 'missing'"
              :label="component.running ? '运行中' : component.exists ? component.state : '未安装'"
            />
          </article>
        </section>

        <section v-if="summary.state !== 'installed'" class="environment-install-card">
          <div>
            <HardDrive :size="24" />
            <span>
              <strong>{{ summary.state === 'partial' ? '检测到部分环境产物' : '尚未安装 LDNMP 环境' }}</strong>
              <small>安装前会展示并检查软件源、DNS、Swap、Docker、Certbot、80/443 端口及目录影响。</small>
            </span>
          </div>
          <div>
            <button class="button button--primary" type="button" :disabled="jobActive" @click="install('full')">
              <Play :size="16" /> {{ summary.state === 'partial' ? '继续安装 / 修复环境' : '安装完整 LDNMP' }}
            </button>
            <button class="button button--secondary" type="button" :disabled="jobActive" @click="install('nginx')">
              仅安装 Nginx
            </button>
          </div>
        </section>
        <div v-if="summary.state !== 'installed' && summary.portConflicts.length" class="inline-alert inline-alert--warning">
          <TriangleAlert :size="17" />
          <span>
            <strong>检测到 80/443 端口占用</strong><br />
            <code v-for="listener in summary.portConflicts" :key="listener">{{ listener }}</code>
          </span>
        </div>
      </template>

      <section v-else-if="section === 'protection'" class="environment-panel">
        <header><ShieldCheck :size="20" /><div><h2>环境防护</h2><p>配置后重新读取 Fail2Ban、Nginx 与 iptables 的真实状态。</p></div></header>
        <div class="environment-action-grid">
          <article>
            <div><strong>Fail2Ban</strong><StatusBadge :status="summary.protection.fail2ban ? 'healthy' : 'stopped'" /></div>
            <p>管理 SSH 与网站 Jail、封禁状态和日志。</p>
            <div>
              <button class="button button--secondary" :disabled="jobActive" @click="protection(summary.protection.fail2ban ? 'fail2ban-uninstall' : 'fail2ban-install')">
                {{ summary.protection.fail2ban ? '卸载' : '安装' }}
              </button>
              <button class="button button--secondary" :disabled="jobActive || !summary.protection.fail2ban" @click="protection('unban-all')">全部解封</button>
            </div>
          </article>
          <article>
            <div><strong>WAF</strong><StatusBadge :status="summary.protection.waf ? 'healthy' : 'stopped'" /></div>
            <p>沿用 kejilion.sh 的 Nginx WAF 配置。</p>
            <button class="button button--secondary" :disabled="jobActive" @click="protection(summary.protection.waf ? 'waf-off' : 'waf-on')">
              {{ summary.protection.waf ? '关闭' : '开启' }}
            </button>
          </article>
          <article>
            <div><strong>DDoS 规则</strong><StatusBadge :status="summary.protection.ddos ? 'healthy' : 'stopped'" /></div>
            <p>读取并管理宿主机 iptables 防护规则。</p>
            <button class="button button--secondary" :disabled="jobActive" @click="protection(summary.protection.ddos ? 'ddos-off' : 'ddos-on')">
              {{ summary.protection.ddos ? '关闭' : '开启' }}
            </button>
          </article>
          <article>
            <div><strong>Cloudflare</strong><StatusBadge :status="summary.protection.cloudflare ? 'healthy' : 'unknown'" :label="summary.protection.cloudflare ? '已配置' : '未配置'" /></div>
            <p>凭据只通过任务专属 0600 文件传递，不写入任务、终端、审计或面板数据库。</p>
            <label><span>账号</span><input v-model.trim="cloudflare.account" autocomplete="username" placeholder="name@example.com" /></label>
            <label><span>API Key / Token</span><input v-model="cloudflare.token" type="password" autocomplete="new-password" placeholder="••••••••" /></label>
            <label><span>Zone ID（五秒盾需要）</span><input v-model.trim="cloudflare.zoneId" autocomplete="off" /></label>
            <div>
              <button class="button button--secondary" :disabled="jobActive" @click="configureCloudflare('cloudflare-fail2ban')">Fail2Ban 模式</button>
              <button class="button button--secondary" :disabled="jobActive" @click="configureCloudflare('cloudflare-shield')">高负载五秒盾</button>
            </div>
          </article>
        </div>
      </section>

      <section v-else-if="section === 'optimization'" class="environment-panel">
        <header><Gauge :size="20" /><div><h2>性能优化</h2><p>当前检测：{{ summary.optimization.mode }}；修改后执行配置与容器健康校验。</p></div></header>
        <div class="environment-mode-actions">
          <button class="environment-mode-card" :class="{ 'is-active': summary.optimization.mode === 'standard' }" :disabled="jobActive" @click="optimize('standard')">
            <strong>标准模式</strong><small>平衡 Nginx、PHP、MySQL 与宿主机参数</small>
          </button>
          <button class="environment-mode-card" :class="{ 'is-active': summary.optimization.mode === 'high' }" :disabled="jobActive" @click="optimize('high')">
            <strong>高性能模式</strong><small>建议 2 核 4 GB 以上，参数更激进</small>
          </button>
        </div>
        <div class="environment-switches">
          <button v-for="item in [
            { id: 'gzip', label: 'gzip', enabled: summary.optimization.gzip },
            { id: 'brotli', label: 'Brotli', enabled: summary.optimization.brotli },
            { id: 'zstd', label: 'Zstd', enabled: summary.optimization.zstd },
          ]" :key="item.id" type="button" :disabled="jobActive" @click="optimize(`${item.id}-${item.enabled ? 'off' : 'on'}`)">
            <span><strong>{{ item.label }}</strong><small>独立压缩开关</small></span>
            <StatusBadge :status="item.enabled ? 'healthy' : 'stopped'" :label="item.enabled ? '已开启' : '已关闭'" />
          </button>
        </div>
      </section>

      <section v-else-if="section === 'update'" class="environment-panel">
        <header><RefreshCw :size="20" /><div><h2>组件更新</h2><p>版本目录由 kejilion.sh 返回，KPanel 不保存第二份版本列表。</p></div></header>
        <LoadingState v-if="catalogLoading" />
        <div v-else-if="catalog?.updateComponents.length" class="environment-update-list">
          <article v-for="item in catalog?.updateComponents || []" :key="item.id">
            <div>
              <strong>{{ item.id === 'all' ? '完整环境' : componentLabel(item.id) }}</strong>
              <small>
                当前：{{ summary.components.find((component) => component.name === item.id)?.version || '由脚本检测' }}
                ·
                {{
                  summary.components.find((component) => component.name === item.id)?.updateStatus === 'available'
                    ? '有更新'
                    : summary.components.find((component) => component.name === item.id)?.updateStatus === 'current'
                      ? '已是最新'
                      : summary.components.find((component) => component.name === item.id)?.updateReason || '更新时实时确认'
                }}
              </small>
            </div>
            <select v-model="updateVersions[item.id]" :aria-label="`${item.id} 目标版本`">
              <option v-for="version in item.versions" :key="version" :value="version">{{ version }}</option>
            </select>
            <button class="button button--primary" type="button" :disabled="jobActive || !environmentInstalled" @click="update(item.id)">更新</button>
          </article>
        </div>
        <p v-else class="environment-empty">版本目录暂时不可用，请刷新后重试。</p>
      </section>

      <section v-else-if="section === 'backup'" class="environment-panel">
        <header>
          <Archive :size="20" />
          <div><h2>备份与还原</h2><p>冷备短暂停止 LDNMP，归档保持与原脚本兼容并附带 SHA-256 sidecar。</p></div>
          <button class="button button--primary" type="button" :disabled="jobActive || !environmentInstalled" @click="createBackup">创建冷备</button>
        </header>
        <LoadingState v-if="backupsLoading" />
        <div v-else-if="backups.length" class="environment-backups">
          <article v-for="backup in backups" :key="backup.id">
            <Archive :size="19" />
            <div><strong>{{ backup.id }}</strong><small>{{ formatBytes(backup.sizeBytes) }} · {{ formatDateTime(backup.createdAt) }}</small></div>
            <StatusBadge :status="backup.verified ? 'healthy' : 'warning'" :label="backup.verified ? '已校验' : '传统备份'" />
            <a class="icon-button" :href="api.webEnvironment.backupDownloadURL(backup.id)" :download="backup.id" title="下载"><Download :size="16" /></a>
            <button class="icon-button" type="button" :disabled="jobActive" title="还原" @click="restore(backup)"><RotateCcw :size="16" /></button>
            <button class="icon-button is-danger" type="button" :disabled="jobActive" title="删除" @click="deleteBackup(backup)"><Trash2 :size="16" /></button>
          </article>
        </div>
        <p v-else class="environment-empty">暂无本地 LDNMP 备份。</p>
      </section>

      <section v-else class="environment-panel environment-danger">
        <header><Trash2 :size="20" /><div><h2>卸载 LDNMP 环境</h2><p>执行前请确认网站和数据库已有独立备份。</p></div></header>
        <ul>
          <li>删除 LDNMP / PhpMyAdmin 相关容器、镜像及 <code>/home/web</code></li>
          <li>保留 KPanel、Docker、<code>/home/web_*.tar.gz</code> 备份及其他系统服务</li>
          <li>当前 {{ summary.siteCount }} 个站点、{{ summary.databaseCount }} 个数据库；最近备份 {{ summary.latestBackup || '暂无' }}</li>
        </ul>
        <label><input v-model="backupBeforeUninstall" type="checkbox" /> 卸载前创建冷备（推荐）</label>
        <button class="button button--danger" type="button" :disabled="jobActive || !environmentInstalled" @click="uninstall">
          <Trash2 :size="16" /> 确认卸载环境
        </button>
      </section>
    </template>

    <ModalDialog
      :open="terminalOpen"
      title="LDNMP 后台任务"
      description="关闭窗口不会停止任务；可在环境页持续查看状态。"
      size="large"
      @close="terminalOpen = false"
    >
      <AppInteractiveTerminal v-if="terminalJob" :job-id="terminalJob.id" kind="environment" />
    </ModalDialog>
  </div>
</template>

<style scoped>
.environment-page { gap: 16px; }
.sites-section-tabs,
.environment-tabs { display: flex; gap: 6px; overflow-x: auto; padding: 5px; border: 1px solid var(--border); border-radius: 14px; background: var(--surface); }
.environment-tabs button { min-width: 92px; padding: 9px 14px; border: 0; border-radius: 10px; color: var(--muted); background: transparent; font-weight: 700; cursor: pointer; }
.environment-tabs button.is-active { color: var(--text); background: var(--surface-raised); box-shadow: var(--shadow-sm); }
.environment-job-banner { display: grid; grid-template-columns: auto minmax(0, 1fr) auto auto auto; align-items: center; gap: 14px; padding: 15px 17px; border: 1px solid var(--border); border-radius: 15px; background: var(--surface); }
.environment-job-banner > div { display: grid; gap: 5px; min-width: 0; }
.environment-job-banner small { overflow: hidden; color: var(--muted); text-overflow: ellipsis; white-space: nowrap; }
.environment-job-banner i { height: 4px; overflow: hidden; border-radius: 99px; background: var(--surface-muted); }
.environment-job-banner i b { display: block; height: 100%; border-radius: inherit; background: var(--primary); transition: width .25s ease; }
.environment-hero { display: grid; grid-template-columns: minmax(220px, 1fr) auto minmax(420px, 1.4fr); align-items: center; gap: 22px; padding: 24px; border: 1px solid var(--border); border-radius: 18px; background: linear-gradient(135deg, color-mix(in srgb, var(--primary) 9%, var(--surface)), var(--surface)); }
.environment-hero span, .environment-hero p, dt { color: var(--muted); }
.environment-hero h2 { margin: 4px 0; font-size: 28px; }
.environment-hero dl { display: grid; grid-template-columns: repeat(4, 1fr); margin: 0; }
.environment-hero dl div { padding: 4px 16px; border-left: 1px solid var(--border); }
.environment-hero dd { margin: 5px 0 0; font-size: 19px; font-weight: 800; }
.environment-checks, .environment-components, .environment-action-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
.environment-checks > div, .environment-components article, .environment-action-grid article { display: flex; align-items: center; gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: 14px; background: var(--surface); }
.environment-checks span, .environment-components article > div { display: grid; min-width: 0; gap: 3px; }
.environment-checks strong, .environment-components small, .environment-components code { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.environment-checks strong, .environment-components small { color: var(--muted); font-size: 12px; }
.environment-components article { align-items: flex-start; }
.environment-components article > div { flex: 1; }
.environment-components code { color: var(--muted); font-size: 11px; }
.environment-install-card, .environment-panel { padding: 20px; border: 1px solid var(--border); border-radius: 16px; background: var(--surface); }
.environment-install-card, .environment-install-card > div, .environment-panel > header { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.environment-install-card > div:first-child span { display: grid; gap: 4px; }
.environment-install-card small, .environment-panel p, .environment-panel header p { color: var(--muted); }
.environment-panel > header { justify-content: flex-start; margin-bottom: 18px; }
.environment-panel header div { flex: 1; }
.environment-panel h2, .environment-panel p { margin: 0; }
.environment-action-grid article { align-items: stretch; flex-direction: column; }
.environment-action-grid { grid-template-columns: repeat(3, 1fr); }
.environment-action-grid article:last-child {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.environment-action-grid article:last-child > div:first-child,
.environment-action-grid article:last-child > p,
.environment-action-grid article:last-child > div:last-child { grid-column: 1 / -1; }
.environment-action-grid article:last-child > div:last-child { justify-content: flex-end; }
.environment-action-grid article > div, .environment-action-grid article > div:last-child { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.environment-action-grid article p { flex: 1; }
.environment-action-grid label { display: grid; gap: 4px; color: var(--muted); font-size: 12px; }
.environment-action-grid input { min-height: 38px; padding: 0 10px; border: 1px solid var(--border); border-radius: 9px; color: var(--text); background: var(--surface-raised); }
.environment-mode-actions { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
.environment-mode-card { display: grid; gap: 6px; padding: 18px; text-align: left; border: 1px solid var(--border); border-radius: 14px; color: var(--text); background: var(--surface-raised); cursor: pointer; }
.environment-mode-card.is-active { border-color: var(--primary); box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary) 18%, transparent); }
.environment-mode-card small { color: var(--muted); }
.environment-switches { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-top: 12px; }
.environment-switches button { display: flex; align-items: center; justify-content: space-between; padding: 15px; border: 1px solid var(--border); border-radius: 13px; color: var(--text); background: var(--surface-raised); cursor: pointer; }
.environment-switches span { display: grid; text-align: left; }
.environment-switches small { color: var(--muted); }
.environment-update-list, .environment-backups { display: grid; gap: 8px; }
.environment-update-list article { display: grid; grid-template-columns: minmax(0, 1fr) 160px auto; align-items: center; gap: 12px; padding: 13px; border: 1px solid var(--border); border-radius: 12px; }
.environment-update-list article > div { display: grid; }
.environment-update-list small { color: var(--muted); }
.environment-update-list select { min-height: 40px; padding: 0 10px; border: 1px solid var(--border); border-radius: 9px; color: var(--text); background: var(--surface); }
.environment-backups article { display: grid; grid-template-columns: auto minmax(0, 1fr) auto auto auto auto; align-items: center; gap: 10px; padding: 13px; border: 1px solid var(--border); border-radius: 12px; }
.environment-backups article > div { display: grid; }
.environment-backups small { color: var(--muted); }
.environment-empty { padding: 34px; text-align: center; border: 1px dashed var(--border); border-radius: 12px; }
.environment-danger { border-color: color-mix(in srgb, var(--danger) 45%, var(--border)); }
.environment-danger li { margin: 8px 0; }
.environment-danger label { display: flex; gap: 8px; margin: 18px 0; }
.is-danger { color: var(--danger); }
@media (max-width: 1100px) {
  .environment-hero { grid-template-columns: 1fr auto; }
  .environment-hero dl { grid-column: 1 / -1; }
  .environment-checks, .environment-components, .environment-action-grid { grid-template-columns: repeat(2, 1fr); }
  .environment-action-grid article:last-child { grid-column: 1 / -1; }
}
@media (max-width: 720px) {
  .environment-job-banner { grid-template-columns: auto minmax(0, 1fr) auto; }
  .environment-job-banner > strong, .environment-job-banner .button { grid-column: 2 / -1; }
  .environment-hero, .environment-checks, .environment-components, .environment-action-grid, .environment-mode-actions, .environment-switches { grid-template-columns: 1fr; }
  .environment-hero { gap: 14px; padding: 16px; border-radius: 14px; }
  .environment-hero h2 { font-size: 23px; }
  .environment-hero dl div { padding: 8px 10px; }
  .environment-checks, .environment-components, .environment-action-grid, .environment-switches { gap: 9px; }
  .environment-checks > div, .environment-components article, .environment-action-grid article { padding: 13px; }
  .environment-action-grid article:last-child { grid-template-columns: 1fr; }
  .environment-action-grid article:last-child > * { grid-column: 1; }
  .environment-hero dl { grid-template-columns: repeat(2, 1fr); }
  .environment-install-card, .environment-install-card > div { align-items: stretch; flex-direction: column; }
  .environment-update-list article { grid-template-columns: 1fr; }
  .environment-backups article { grid-template-columns: auto minmax(0, 1fr) repeat(3, 40px); gap: 7px; }
  .environment-backups article > .status-badge { grid-column: 1 / 3; grid-row: 2; }
  .environment-backups article > .icon-button { width: 40px; height: 40px; }
}
</style>
