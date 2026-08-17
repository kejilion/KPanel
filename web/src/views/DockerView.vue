<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/DockerView/en-US').then((module) => module.default)
  : import('@/i18n/pages/DockerView/zh-TW').then((module) => module.default))
import {
  Box,
  Boxes,
  BrushCleaning,
  ChevronRight,
  CircleStop,
  Container,
  Copy,
  Download,
  EllipsisVertical,
  FileText,
  HardDrive,
  LoaderCircle,
  Network,
  Pause,
  Play,
  Plus,
  RefreshCw,
  RotateCw,
  Search,
  ShieldCheck,
  Trash2,
  Waypoints,
  Wrench,
} from '@lucide/vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import DockerDeploymentEditor from '@/components/docker/DockerDeploymentEditor.vue'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { analyzeDockerDeployment, composeEnvironmentVariables } from '@/lib/dockerDeployment'
import { dockerComposeGroupAccent, groupDockerContainers, type DockerContainerGroup } from '@/lib/dockerComposeGroups'
import {
  sortDockerContainers,
  sortDockerImages,
  sortDockerNetworks,
  sortDockerVolumes,
  type ResourceSort,
} from '@/lib/dockerSorting'
import { formatBytes, formatDateTime, relativeTime, shortId } from '@/lib/format'
import { usePanelState } from '@/stores/panel'
import { useToast } from '@/stores/toast'
import type {
  DockerBackup,
  DockerContainer,
  DockerComposeProject,
  DockerContainerCreateEnvironment,
  DockerContainerCreateMount,
  DockerContainerCreatePort,
  DockerContainerStats,
  DockerEnvironment,
  DockerInventory,
  DockerMaintenanceInput,
  DockerMaintenanceJob,
} from '@/types/api'

type DockerTab = 'environment' | 'containers' | 'images' | 'networks' | 'volumes'
type ContainerAction = 'start' | 'stop' | 'restart' | 'pause' | 'unpause' | 'remove'
type DockerContextMenu =
  | { kind: 'container'; item: DockerContainer; x: number; y: number }
  | { kind: 'image'; item: DockerInventory['images'][number]; x: number; y: number }
  | { kind: 'network'; item: DockerInventory['networks'][number]; x: number; y: number }
  | { kind: 'volume'; item: DockerInventory['volumes'][number]; x: number; y: number }

interface CreatePortRow {
  publicPort: string
  privatePort: string
  protocol: 'tcp' | 'udp'
  hostIp: string
}

interface CreateMountRow {
  type: 'volume' | 'bind'
  source: string
  target: string
  readOnly: boolean
}

interface CreateEnvironmentRow {
  name: string
  value: string
}

interface CreateComposeEnvironmentRow extends CreateEnvironmentRow {
  defaultValue?: string
  detected: boolean
  required: boolean
}

const panel = usePanelState()
const toast = useToast()
const windowActive = inject(desktopWindowActiveKey, computed(() => true))
const data = ref<DockerInventory>()
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const search = ref('')
const activeTab = ref<DockerTab>('containers')
const resourceSort = ref<ResourceSort>('smart')
const taskRunning = ref(false)
const activeJob = ref<DockerMaintenanceJob>()
const pendingMaintenance = ref<{
  title: string
  description: string
  input: DockerMaintenanceInput
  danger?: boolean
}>()
const selectedContainer = ref<DockerContainer>()
const pendingAction = ref<ContainerAction>()
const actionRunning = ref(false)
const contextMenu = ref<DockerContextMenu>()
const collapsedContainerGroups = ref(new Set<string>())

const backups = ref<DockerBackup[]>([])
const environment = ref<DockerEnvironment>()
const backupsLoading = ref(false)
const publicIPv4 = ref('')
const mirrorPreset = ref<'cn' | 'official'>('cn')
const ipv6Enabled = ref(false)
const ipv6CIDR = ref('fd42:6b50:616e:656c::/64')
const systemUpdatePending = ref(false)
const systemUpdating = ref(false)
const uninstallNoticeOpen = ref(false)

const imageReference = ref('')
const networkName = ref('')
const networkDriver = ref('bridge')
const volumeName = ref('')
const volumeDriver = ref('local')
const membershipContainerID = ref('')
const membershipNetworkID = ref('')

const createOpen = ref(false)
const createSource = ref('')
const createAdvanced = ref(false)
const createManualMode = ref(false)
const createComposeProject = ref('')
const createName = ref('')
const createImage = ref('')
const createNetwork = ref('bridge')
const createRestartPolicy = ref<'no' | 'always' | 'unless-stopped' | 'on-failure'>('unless-stopped')
const createCommand = ref('')
const createPorts = ref<CreatePortRow[]>([
  { publicPort: '', privatePort: '', protocol: 'tcp', hostIp: '0.0.0.0' },
])
const createMounts = ref<CreateMountRow[]>([])
const createEnvironment = ref<CreateEnvironmentRow[]>([])
const createComposeEnvironment = ref<CreateComposeEnvironmentRow[]>([])
const createComposeEnvironmentOpen = ref(false)
const createComposeEnvironmentRevealed = ref(false)

const composeOpen = ref(false)
const composeLoading = ref(false)
const composeError = ref('')
const composeProject = ref<DockerComposeProject>()
const composeFilePath = ref('')
const composeSource = ref('')
const composeEnvironmentSource = ref('')
const composeEnvironmentOpen = ref(false)
const composeEnvironmentRevealed = ref(false)

const logsOpen = ref(false)
const logsLoading = ref(false)
const logLines = ref<string[]>([])
const logError = ref('')
const statsOpen = ref(false)
const statsLoading = ref(false)
const stats = ref<DockerContainerStats>()
const statsError = ref('')
const consoleOpen = ref(false)
const consoleCommand = ref('')
const consoleOutput = ref('')
const consoleExitCode = ref<number>()
const consoleRunning = ref(false)
const accessOpen = ref(false)
const accessAllowedIP = ref('')
const migrationOpen = ref(false)
const migrationBackup = ref<DockerBackup>()
const migrationHost = ref('')
const migrationUser = ref('root')
const migrationPort = ref('22')

let controller: AbortController | undefined
let composeController: AbortController | undefined
let logController: AbortController | undefined
let statsController: AbortController | undefined
let statsTimer: number | undefined
let statsPollGeneration = 0
let jobTimer: number | undefined
let jobController: AbortController | undefined
let jobPollGeneration = 0
let pollingJobID = ''
const activeDockerJobKey = 'kpanel.active-docker-job'
const activeJobPollDelay = 1_500
const backgroundJobPollDelay = 15_000
const statsPollDelay = 3_000

const tabs = computed(() => [
  { id: 'containers' as const, label: '容器', icon: Container, count: String(data.value?.containers.length || 0) },
  { id: 'images' as const, label: '镜像', icon: Box, count: String(data.value?.images.length || 0) },
  { id: 'networks' as const, label: '网络', icon: Network, count: String(data.value?.networks.length || 0) },
  { id: 'volumes' as const, label: '存储卷', icon: HardDrive, count: String(data.value?.volumes.length || 0) },
  { id: 'environment' as const, label: '环境设置', icon: Wrench, count: data.value?.available ? '正常' : '异常' },
])
const dockerJobActive = computed(() =>
  activeJob.value?.status === 'queued' || activeJob.value?.status === 'running',
)
const contextContainer = computed(() =>
  contextMenu.value?.kind === 'container' ? contextMenu.value.item : undefined,
)
const contextImage = computed(() =>
  contextMenu.value?.kind === 'image' ? contextMenu.value.item : undefined,
)
const contextNetwork = computed(() =>
  contextMenu.value?.kind === 'network' ? contextMenu.value.item : undefined,
)
const contextVolume = computed(() =>
  contextMenu.value?.kind === 'volume' ? contextMenu.value.item : undefined,
)

const runningCount = computed(() => data.value?.containers.filter((item) => item.state === 'running').length || 0)
const manageableCount = computed(() => data.value?.containers.filter((item) => (item.allowedActions?.length || 0) > 0).length || 0)
const membershipContainers = computed(() =>
  (data.value?.containers || []).filter(
    (item) => item.resourceVersion,
  ),
)
const membershipNetworks = computed(() =>
  (data.value?.networks || []).filter(
    (item) => item.resourceVersion,
  ),
)
const availableImageTags = computed(() =>
  (data.value?.images || []).flatMap((item) => item.tags).filter((item) => item && item !== '<none>:<none>'),
)
const createNetworks = computed(() => data.value?.networks || [])
const createAnalysis = computed(() => analyzeDockerDeployment(createSource.value))
const detectedComposeEnvironment = computed(() => createAnalysis.value.kind === 'compose'
  ? composeEnvironmentVariables(createAnalysis.value.compose)
  : [])
const createDiagnostics = computed(() => createAnalysis.value.kind === 'invalid' ? createAnalysis.value.diagnostics : [])
const createComposeEnvironmentMissing = computed(() => createComposeEnvironment.value.filter((item) => item.required && !item.value).length)
const createComposeEnvironmentValid = computed(() => {
  const names = new Set<string>()
  return createComposeEnvironmentMissing.value === 0 && createComposeEnvironment.value.every((item) => {
    const valid = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(item.name) && !names.has(item.name) &&
      item.value.length <= 2048 && !/[\r\n\u0000]/.test(item.value)
    names.add(item.name)
    return valid
  })
})
const createModeLabel = computed(() => {
  if (createManualMode.value) return '手动配置'
  if (createAnalysis.value.kind === 'docker-run') return 'Docker Run'
  if (createAnalysis.value.kind === 'compose') return 'Docker Compose'
  return ''
})
const createSummary = computed(() => {
  const analysis = createAnalysis.value
  if (createManualMode.value) return createImage.value.trim() || '填写镜像后即可部署'
  if (analysis.kind === 'docker-run') {
    const input = analysis.input
    return [input.name || '自动命名', input.image, `${input.ports?.length || 0} 个端口`, `${input.mounts?.length || 0} 个挂载`].join(' · ')
  }
  if (analysis.kind === 'compose') return `${analysis.projectName} · ${analysis.services.length} 个服务`
  return ''
})
const createCanSubmit = computed(() => {
  if (createManualMode.value) return Boolean(createImage.value.trim())
  if (createAnalysis.value.kind === 'docker-run') {
    return createAdvanced.value ? Boolean(createImage.value.trim()) : true
  }
  return createAnalysis.value.kind === 'compose' && Boolean(createComposeProject.value.trim()) &&
    createComposeEnvironmentValid.value
})

const sortedContainers = computed(() =>
  sortDockerContainers(data.value?.containers || [], resourceSort.value),
)
const containerCatalog = computed(() => sortedContainers.value.map((item) => ({
  item,
  searchText: [item.name, item.image, item.project].filter(Boolean).join('\u0000').toLowerCase(),
})))
const sortedImages = computed(() =>
  sortDockerImages(data.value?.images || [], resourceSort.value),
)
const imageCatalog = computed(() => sortedImages.value.map((item) => ({
  item,
  searchText: [item.id, ...item.tags].join('\u0000').toLowerCase(),
})))
const sortedNetworks = computed(() =>
  sortDockerNetworks(data.value?.networks || [], resourceSort.value),
)
const networkCatalog = computed(() => sortedNetworks.value.map((item) => ({
  item,
  searchText: `${item.name}\u0000${item.driver}`.toLowerCase(),
})))
const sortedVolumes = computed(() =>
  sortDockerVolumes(data.value?.volumes || [], resourceSort.value),
)
const volumeCatalog = computed(() => sortedVolumes.value.map((item) => ({
  item,
  searchText: `${item.name}\u0000${item.driver}`.toLowerCase(),
})))

const filteredContainers = computed(() => {
  const query = search.value.trim().toLowerCase()
  return query
    ? containerCatalog.value.filter(({ searchText }) => searchText.includes(query)).map(({ item }) => item)
    : sortedContainers.value
})
const containerGroups = computed(() => groupDockerContainers(filteredContainers.value))

function isContainerGroupCollapsed(key: string): boolean {
  return collapsedContainerGroups.value.has(key)
}

function toggleContainerGroup(key: string): void {
  const next = new Set(collapsedContainerGroups.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsedContainerGroups.value = next
}

function containerGroupStyle(group: DockerContainerGroup): Record<string, string> {
  return {
    '--docker-group-accent': group.kind === 'compose' ? dockerComposeGroupAccent(group.name) : 'var(--muted)',
  }
}

function visibleContainerGroupRows(group: DockerContainerGroup): DockerContainer[] {
  return isContainerGroupCollapsed(group.key) ? [] : group.containers
}
const composeAnalysis = computed(() => analyzeDockerDeployment(composeSource.value))
const composeDiagnostics = computed(() => composeAnalysis.value.kind === 'invalid' ? composeAnalysis.value.diagnostics : [])
const selectedComposeFile = computed(() => composeProject.value?.configFiles.find((file) => file.path === composeFilePath.value))
const composeEnvironmentCount = computed(() => composeEnvironmentSource.value.split(/\r?\n/)
  .filter((line) => line.trim() && !line.trim().startsWith('#')).length)
const composeEnvironmentValid = computed(() => new TextEncoder().encode(composeEnvironmentSource.value).length <= 24 * 1024 &&
  !composeEnvironmentSource.value.includes('\u0000'))
const composeCanRedeploy = computed(() => composeAnalysis.value.kind === 'compose' && Boolean(selectedComposeFile.value) &&
  composeEnvironmentValid.value)
const filteredImages = computed(() => {
  const query = search.value.trim().toLowerCase()
  return query
    ? imageCatalog.value.filter(({ searchText }) => searchText.includes(query)).map(({ item }) => item)
    : sortedImages.value
})
const filteredNetworks = computed(() => {
  const query = search.value.trim().toLowerCase()
  return query
    ? networkCatalog.value.filter(({ searchText }) => searchText.includes(query)).map(({ item }) => item)
    : sortedNetworks.value
})
const filteredVolumes = computed(() => {
  const query = search.value.trim().toLowerCase()
  return query
    ? volumeCatalog.value.filter(({ searchText }) => searchText.includes(query)).map(({ item }) => item)
    : sortedVolumes.value
})

const visibleResourceCount = computed(() => {
  if (activeTab.value === 'containers') return filteredContainers.value.length
  if (activeTab.value === 'images') return filteredImages.value.length
  if (activeTab.value === 'networks') return filteredNetworks.value.length
  if (activeTab.value === 'volumes') return filteredVolumes.value.length
  return 0
})

function formatPorts(container: DockerContainer): string {
  if (!container.ports.length) return '无公开端口'
  return container.ports
    .map((port) => `${port.publicPort ? `${port.ip || '0.0.0.0'}:${port.publicPort} → ` : ''}${port.privatePort}/${port.protocol}`)
    .join('，')
}

function permits(container: DockerContainer, action: string): boolean {
  return Boolean(
    container.resourceVersion &&
    container.allowedActions?.some(
      (allowed) => allowed === action || allowed.endsWith(`.${action}`) || allowed.endsWith(`/${action}`),
    ),
  )
}

function contextPosition(event: MouseEvent): { x: number; y: number } {
  const target = event.currentTarget as HTMLElement | null
  const bounds = target?.getBoundingClientRect()
  const x = event.clientX || bounds?.right || 16
  const y = event.clientY || bounds?.bottom || 16
  return {
    x: Math.max(8, Math.min(x, window.innerWidth - 226)),
    y: Math.max(8, Math.min(y, window.innerHeight - 430)),
  }
}

function showContainerContext(event: MouseEvent, item: DockerContainer): void {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { kind: 'container', item, ...contextPosition(event) }
}

function showImageContext(event: MouseEvent, item: DockerInventory['images'][number]): void {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { kind: 'image', item, ...contextPosition(event) }
}

function showNetworkContext(event: MouseEvent, item: DockerInventory['networks'][number]): void {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { kind: 'network', item, ...contextPosition(event) }
}

function showVolumeContext(event: MouseEvent, item: DockerInventory['volumes'][number]): void {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { kind: 'volume', item, ...contextPosition(event) }
}

async function copyResourceValue(value: string, label: string): Promise<void> {
  contextMenu.value = undefined
  let copied = false
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      copied = true
    } catch {
      // HTTP IP access and restricted browsing contexts may deny the modern clipboard API.
    }
  }
  if (!copied && typeof document.execCommand === 'function') {
    const input = document.createElement('textarea')
    input.value = value
    input.readOnly = true
    input.style.position = 'fixed'
    input.style.left = '-9999px'
    document.body.appendChild(input)
    try {
      input.select()
      copied = document.execCommand('copy')
    } finally {
      input.remove()
    }
  }
  if (copied) toast.success(`${label}已复制`)
  else toast.danger('复制失败', `请手动复制：${value}`)
}

function manageNetworkMembership(network: DockerInventory['networks'][number]): void {
  membershipNetworkID.value = network.id
  contextMenu.value = undefined
  toast.success('已选择网络', '请在上方选择容器后执行加入或退出。')
}

function closeContextMenuOnOutsideClick(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (!target.closest('.docker-context-menu') && !target.closest('.docker-context-trigger')) {
    contextMenu.value = undefined
  }
}

function closeContextMenu(): void {
  contextMenu.value = undefined
}

async function load(silent = false): Promise<void> {
  controller?.abort()
  composeController?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    data.value = await api.docker.inventory(controller.signal, (partial) => {
      data.value = partial
      loading.value = false
    })
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取 Docker 资源。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function loadBackups(): Promise<void> {
  backupsLoading.value = true
  try {
    backups.value = (await api.docker.backups()).items
  } catch (reason) {
    toast.danger('Docker 备份列表读取失败', reason instanceof ApiError ? reason.message : '请稍后重试。')
  } finally {
    backupsLoading.value = false
  }
}

async function loadEnvironment(): Promise<void> {
  try {
    const observed = await api.docker.environment()
    environment.value = observed
    if (observed.mirrorPreset === 'cn' || observed.mirrorPreset === 'official') {
      mirrorPreset.value = observed.mirrorPreset
    }
    ipv6Enabled.value = observed.ipv6Enabled
    if (observed.ipv6Cidr) ipv6CIDR.value = observed.ipv6Cidr
  } catch {
    // Inventory remains independently usable when environment metadata fails.
  }
}

async function loadPublicIPv4(): Promise<void> {
  try {
    publicIPv4.value = (await api.system.publicNetwork()).ipv4 || ''
  } catch {
    // Access control remains usable with an explicitly entered allow IP.
  }
}

function stopJobPolling(): void {
  jobPollGeneration += 1
  pollingJobID = ''
  if (jobTimer) window.clearTimeout(jobTimer)
  jobTimer = undefined
  jobController?.abort()
  jobController = undefined
}

function scheduleJobPoll(id: string, generation: number, delay: number): void {
  if (generation !== jobPollGeneration || pollingJobID !== id) return
  if (jobTimer) window.clearTimeout(jobTimer)
  jobTimer = window.setTimeout(() => {
    jobTimer = undefined
    void refreshJob(id, generation)
  }, delay)
}

async function refreshJob(id: string, generation = jobPollGeneration): Promise<void> {
  if (jobController) return
  const requestController = new AbortController()
  jobController = requestController
  try {
    const job = await api.docker.job(id, requestController.signal)
    if (generation !== jobPollGeneration || jobController !== requestController) return
    activeJob.value = job
    if (job.status === 'queued' || job.status === 'running') return
    stopJobPolling()
    window.localStorage.removeItem(activeDockerJobKey)
    if (job.status === 'succeeded') {
      toast.success(
        'Docker 后台任务完成',
        job.resultPath ? `结果：${job.resultPath}` : job.message || job.target || '资源状态已更新。',
      )
      await Promise.all([load(true), loadBackups(), loadEnvironment()])
    } else {
      toast.danger('Docker 后台任务失败', job.message || '请刷新资源后重试。')
    }
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (generation !== jobPollGeneration || jobController !== requestController) return
    if (reason instanceof ApiError && reason.status === 404) {
      stopJobPolling()
      activeJob.value = undefined
      window.localStorage.removeItem(activeDockerJobKey)
    }
  } finally {
    if (jobController === requestController) jobController = undefined
    if (
      generation === jobPollGeneration &&
      pollingJobID === id
    ) {
      scheduleJobPoll(
        id,
        generation,
        windowActive.value ? activeJobPollDelay : backgroundJobPollDelay,
      )
    }
  }
}

function beginJobPolling(id: string, immediate = windowActive.value): void {
  stopJobPolling()
  window.localStorage.setItem(activeDockerJobKey, id)
  pollingJobID = id
  const generation = jobPollGeneration
  if (immediate) void refreshJob(id, generation)
  else scheduleJobPoll(id, generation, backgroundJobPollDelay)
}

function startJobPolling(job: DockerMaintenanceJob, immediate = windowActive.value): void {
  activeJob.value = job
  beginJobPolling(job.id, immediate)
}

async function restoreBackgroundJob(): Promise<void> {
  const saved = window.localStorage.getItem(activeDockerJobKey)
  if (saved) {
    try {
      const job = await api.docker.job(saved)
      activeJob.value = job
      if (job.status === 'queued' || job.status === 'running') startJobPolling(job)
      else window.localStorage.removeItem(activeDockerJobKey)
      return
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 404) {
        window.localStorage.removeItem(activeDockerJobKey)
      } else {
        beginJobPolling(saved)
        return
      }
    }
  }
  try {
    const jobs = await api.docker.jobs()
    const running = jobs.items.find((job) => job.status === 'queued' || job.status === 'running')
    if (running) startJobPolling(running)
  } catch {
    // Older Agents remain usable without background task endpoints.
  }
}

async function submitTask(input: DockerMaintenanceInput): Promise<void> {
  if (taskRunning.value || panel.isReadOnly.value) return
  taskRunning.value = true
  try {
    const job = await api.docker.task(input)
    pendingMaintenance.value = undefined
    createOpen.value = false
    if (input.action === 'container_create' || input.action === 'compose_deploy') resetCreateForm()
    accessOpen.value = false
    migrationOpen.value = false
    startJobPolling(job)
    toast.success('已转入后台执行', '可以离开 Docker 页面，任务会继续运行。')
  } catch (reason) {
    toast.danger('Docker 任务提交失败', reason instanceof ApiError ? reason.message : 'Agent 拒绝了本次操作。')
  } finally {
    taskRunning.value = false
  }
}

function askTask(title: string, description: string, input: DockerMaintenanceInput, danger = false): void {
  pendingMaintenance.value = { title, description, input, danger }
}

function askPrune(action: 'prune' | 'container_prune' | 'image_prune' | 'network_prune' | 'volume_prune', label: string): void {
  askTask(`确认${label}`, '直接执行 Docker Engine 对应的 prune；范围与 kejilion.sh 一致。', {
    action,
  }, true)
}

async function submitSystemUpdate(): Promise<void> {
  if (dockerJobActive.value) {
    toast.danger('请等待 Docker 后台任务完成', '环境更新期间 Docker Engine 可能重启，不能与资源任务并行。')
    return
  }
  systemUpdating.value = true
  try {
    const result = await api.system.action({ action: 'update', maintenancePolicy: 'full' })
    systemUpdatePending.value = false
    toast.success('Docker 环境更新已进入系统后台任务', result.message)
  } catch (reason) {
    toast.danger('Docker 环境更新提交失败', reason instanceof ApiError ? reason.message : '请稍后重试。')
  } finally {
    systemUpdating.value = false
  }
}

function pullImage(): void {
  const image = imageReference.value.trim()
  if (!image) return
  void submitTask({ action: 'image_pull', image })
  imageReference.value = ''
}

function updateImage(image: DockerInventory['images'][number]): void {
  contextMenu.value = undefined
  const tag = image.tags.find((item) => item && item !== '<none>:<none>')
  if (!tag) return
  askTask('确认拉取镜像最新版本', tag, { action: 'image_pull', image: tag })
}

function askImageRemoval(image: DockerInventory['images'][number]): void {
  contextMenu.value = undefined
  if (!image.resourceVersion) return
  askTask('确认删除镜像', image.tags.join(', ') || shortId(image.id), {
    action: 'image_remove',
    target: image.id,
    expectedResourceVersion: image.resourceVersion,
  }, true)
}

function createDockerNetwork(): void {
  const name = networkName.value.trim()
  const driver = networkDriver.value.trim()
  if (!name || !driver) return
  void submitTask({ action: 'network_create', name, driver })
  networkName.value = ''
}

function askNetworkRemoval(network: DockerInventory['networks'][number]): void {
  contextMenu.value = undefined
  if (!network.resourceVersion) return
  askTask('确认删除网络', network.name, {
    action: 'network_remove',
    target: network.id,
    expectedResourceVersion: network.resourceVersion,
  }, true)
}

function updateNetworkMembership(action: 'network_connect' | 'network_disconnect'): void {
  const container = membershipContainers.value.find((item) => item.id === membershipContainerID.value)
  const network = membershipNetworks.value.find((item) => item.id === membershipNetworkID.value)
  if (!container?.resourceVersion || !network?.resourceVersion) return
  askTask(
    action === 'network_connect' ? '确认将容器加入网络' : '确认让容器退出网络',
    `${container.name} · ${network.name}`,
    {
      action,
      target: network.id,
      expectedResourceVersion: network.resourceVersion,
      containerId: container.id,
      containerResourceVersion: container.resourceVersion,
    },
  )
}

function createDockerVolume(): void {
  const name = volumeName.value.trim()
  const driver = volumeDriver.value.trim()
  if (!name || !driver) return
  void submitTask({ action: 'volume_create', name, driver })
  volumeName.value = ''
}

function askVolumeRemoval(volume: DockerInventory['volumes'][number]): void {
  contextMenu.value = undefined
  if (!volume.resourceVersion) return
  askTask('确认删除存储卷', volume.name, {
    action: 'volume_remove',
    target: volume.name,
    expectedResourceVersion: volume.resourceVersion,
  }, true)
}

function addCreatePort(): void {
  if (createPorts.value.length >= 16) return
  createPorts.value.push({ publicPort: '', privatePort: '', protocol: 'tcp', hostIp: '0.0.0.0' })
}

function addCreateMount(): void {
  if (createMounts.value.length >= 16) return
  createMounts.value.push({ type: 'volume', source: '', target: '', readOnly: false })
}

function addCreateEnvironment(): void {
  if (createEnvironment.value.length >= 64) return
  createEnvironment.value.push({ name: '', value: '' })
}

function resetStructuredCreateForm(): void {
  createName.value = ''
  createImage.value = ''
  createNetwork.value = 'bridge'
  createRestartPolicy.value = 'unless-stopped'
  createCommand.value = ''
  createPorts.value = [{ publicPort: '', privatePort: '', protocol: 'tcp', hostIp: '0.0.0.0' }]
  createMounts.value = []
  createEnvironment.value = []
}

function syncCreateComposeEnvironment(): void {
  if (createAnalysis.value.kind !== 'compose') {
    createComposeEnvironment.value = []
    createComposeEnvironmentOpen.value = false
    return
  }
  const existing = new Map(createComposeEnvironment.value.map((item) => [item.name, item]))
  const detectedNames = new Set(detectedComposeEnvironment.value.map((item) => item.name))
  const detected = detectedComposeEnvironment.value.map((item) => ({
    name: item.name,
    value: existing.get(item.name)?.value || '',
    defaultValue: item.defaultValue,
    detected: true,
    required: item.required,
  }))
  const manual = createComposeEnvironment.value.filter((item) => !item.detected && !detectedNames.has(item.name))
  createComposeEnvironment.value = [...detected, ...manual]
  if (createComposeEnvironmentMissing.value > 0) createComposeEnvironmentOpen.value = true
}

function addCreateComposeEnvironment(): void {
  createComposeEnvironment.value.push({ name: '', value: '', detected: false, required: false })
  createComposeEnvironmentOpen.value = true
}

function encodeComposeEnvironmentValue(value: string): string {
  if (!value) return ''
  return `'${value.replace(/'/g, "\\'")}'`
}

function createComposeEnvironmentSource(): string {
  return createComposeEnvironment.value
    .filter((item) => item.name && (item.value || item.required))
    .map((item) => `${item.name}=${encodeComposeEnvironmentValue(item.value)}`)
    .join('\n')
}

function resetCreateForm(): void {
  createSource.value = ''
  createAdvanced.value = false
  createManualMode.value = false
  createComposeProject.value = ''
  createComposeEnvironment.value = []
  createComposeEnvironmentOpen.value = false
  createComposeEnvironmentRevealed.value = false
  resetStructuredCreateForm()
}

function applyDockerRunInput(input: DockerMaintenanceInput): void {
  createName.value = input.name || ''
  createImage.value = input.image || ''
  createNetwork.value = input.network || 'bridge'
  createRestartPolicy.value = input.restartPolicy || 'unless-stopped'
  createCommand.value = (input.command || []).join('\n')
  createPorts.value = (input.ports || []).map((item) => ({
    publicPort: String(item.publicPort),
    privatePort: String(item.privatePort),
    protocol: item.protocol || 'tcp',
    hostIp: item.hostIp || '0.0.0.0',
  }))
  createMounts.value = (input.mounts || []).map((item) => ({
    type: item.type || 'volume', source: item.source, target: item.target, readOnly: Boolean(item.readOnly),
  }))
  createEnvironment.value = (input.environment || []).map((item) => ({ ...item }))
}

function showCreateAdvanced(): void {
  createAdvanced.value = !createAdvanced.value
  if (createAdvanced.value && createAnalysis.value.kind === 'docker-run') {
    applyDockerRunInput(createAnalysis.value.input)
  }
}

function startManualCreate(): void {
  createSource.value = ''
  createManualMode.value = true
  createAdvanced.value = true
  resetStructuredCreateForm()
}

function buildStructuredContainerInput(): DockerMaintenanceInput | undefined {
  const name = createName.value.trim()
  const image = createImage.value.trim()
  if (!image) return undefined
  const ports: DockerContainerCreatePort[] = createPorts.value
    .filter((item) => item.publicPort.trim() || item.privatePort.trim())
    .map((item) => ({
      publicPort: Number(item.publicPort),
      privatePort: Number(item.privatePort),
      protocol: item.protocol,
      hostIp: item.hostIp,
    }))
  if (ports.some((item) => !Number.isInteger(item.publicPort) || !Number.isInteger(item.privatePort) ||
    item.publicPort < 1 || item.publicPort > 65535 || item.privatePort < 1 || item.privatePort > 65535)) {
    toast.danger('端口格式无效', '主机端口和容器端口必须是 1-65535 的整数。')
    return
  }
  const mounts: DockerContainerCreateMount[] = createMounts.value
    .filter((item) => item.source.trim() || item.target.trim())
    .map((item) => ({
      type: item.type,
      source: item.source.trim(),
      target: item.target.trim(),
      readOnly: item.readOnly,
    }))
  if (mounts.some((item) => !item.source || !item.target.startsWith('/') ||
    (item.type === 'bind' && !item.source.startsWith('/')))) {
    toast.danger('存储挂载无效', '命名卷需填写卷名；宿主机目录和容器路径需使用绝对路径。')
    return
  }
  const environment: DockerContainerCreateEnvironment[] = createEnvironment.value
    .filter((item) => item.name.trim() || item.value)
    .map((item) => ({ name: item.name.trim(), value: item.value }))
  const environmentName = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/
  const seenEnvironment = new Set<string>()
  if (environment.some((item) => {
    const invalid = !environmentName.test(item.name) || seenEnvironment.has(item.name) ||
      item.value.length > 2048 || /[\r\n\u0000]/.test(item.value)
    seenEnvironment.add(item.name)
    return invalid
  })) {
    toast.danger('环境变量无效', '变量名必须规范且不能重复；变量值不能换行，单项最多 2048 字符。')
    return
  }
  return {
    action: 'container_create',
    name,
    image,
    network: createNetwork.value,
    restartPolicy: createRestartPolicy.value,
    command: createCommand.value.split('\n').map((item) => item.trim()).filter(Boolean),
    ports,
    mounts,
    environment,
  }
}

function submitContainerCreate(): void {
  if (createManualMode.value || createAnalysis.value.kind === 'docker-run') {
    const input = createAdvanced.value || createManualMode.value
      ? buildStructuredContainerInput()
      : createAnalysis.value.kind === 'docker-run' ? createAnalysis.value.input : undefined
    if (!input) return
    askTask('确认部署 Docker 容器', createSummary.value, input)
    return
  }
  const analysis = createAnalysis.value
  if (analysis.kind === 'compose') {
    const projectName = createComposeProject.value.trim()
    if (!/^[a-z0-9][a-z0-9_-]{0,62}$/.test(projectName)) {
      toast.danger('项目名称无效', '仅支持小写字母、数字、连字符和下划线，最长 63 个字符。')
      return
    }
    askTask('确认部署 Compose 项目', `${projectName} · ${analysis.services.length} 个服务`, {
      action: 'compose_deploy', name: projectName, compose: analysis.compose,
      composeEnvironment: createComposeEnvironmentSource(),
    })
    return
  }
  if (analysis.kind === 'invalid') toast.danger('无法识别部署内容', analysis.message)
}

watch(createSource, () => {
  if (!createSource.value.trim()) return
  createManualMode.value = false
  const analysis = createAnalysis.value
  if (analysis.kind === 'docker-run') {
    applyDockerRunInput(analysis.input)
  } else if (analysis.kind === 'compose') {
    createComposeProject.value = analysis.projectName
  }
  syncCreateComposeEnvironment()
})

async function openComposeProject(group: DockerContainerGroup): Promise<void> {
  if (group.kind !== 'compose') return
  composeController?.abort()
  composeController = new AbortController()
  composeOpen.value = true
  composeLoading.value = true
  composeError.value = ''
  composeProject.value = undefined
  composeFilePath.value = ''
  composeSource.value = ''
  try {
    const project = await api.docker.composeProject(group.name, composeController.signal)
    composeProject.value = project
    composeEnvironmentSource.value = project.environmentFile?.source || ''
    composeEnvironmentOpen.value = false
    composeEnvironmentRevealed.value = false
    const firstFile = project.configFiles[0]
    if (firstFile) {
      composeFilePath.value = firstFile.path
      composeSource.value = firstFile.source
    }
  } catch (reason) {
    if ((reason as Error).name === 'AbortError') return
    composeError.value = reason instanceof ApiError ? reason.message : '无法读取 Compose 项目配置。'
  } finally {
    composeLoading.value = false
  }
}

function selectComposeFile(): void {
  const file = selectedComposeFile.value
  composeSource.value = file?.source || ''
}

function closeComposeProject(): void {
  composeController?.abort()
  composeController = undefined
  composeOpen.value = false
  composeProject.value = undefined
  composeError.value = ''
  composeFilePath.value = ''
  composeSource.value = ''
  composeEnvironmentSource.value = ''
  composeEnvironmentOpen.value = false
  composeEnvironmentRevealed.value = false
}

function askComposeLifecycle(action: 'compose_start' | 'compose_stop' | 'compose_restart'): void {
  const project = composeProject.value
  if (!project) return
  const labels = {
    compose_start: ['确认启动 Compose 项目', '启动项目中已有的全部服务'],
    compose_stop: ['确认停止 Compose 项目', '停止项目服务，容器和配置继续保留'],
    compose_restart: ['确认重启 Compose 项目', '按当前配置重启项目中的全部服务'],
  } as const
  closeComposeProject()
  askTask(labels[action][0], `${project.name} · ${labels[action][1]}`, {
    action, name: project.name, expectedResourceVersion: project.resourceVersion,
  }, action === 'compose_stop')
}

function submitComposeRedeploy(): void {
  const project = composeProject.value
  const file = selectedComposeFile.value
  if (!project || !file) return
  const analysis = composeAnalysis.value
  if (analysis.kind !== 'compose') {
    if (analysis.kind === 'invalid') toast.danger('Compose 配置存在语法问题', analysis.message)
    return
  }
  if (!composeEnvironmentValid.value) {
    toast.danger('项目变量文件过大', '.env 不能超过 24 KiB，且不能包含 NUL 字符。')
    return
  }
  const environmentSource = composeEnvironmentSource.value
  closeComposeProject()
  askTask('确认更新并重新部署 Compose 项目', `${project.name} · ${file.name} · ${analysis.services.length} 个服务`, {
    action: 'compose_redeploy',
    name: project.name,
    composeFile: file.path,
    compose: analysis.compose,
    composeEnvironment: environmentSource,
    expectedResourceVersion: project.resourceVersion,
  })
}

function askAction(container: DockerContainer, action: ContainerAction): void {
  contextMenu.value = undefined
  selectedContainer.value = container
  pendingAction.value = action
}

async function runAction(): Promise<void> {
  if (!selectedContainer.value || !pendingAction.value || !selectedContainer.value.resourceVersion) return
  actionRunning.value = true
  try {
    const action = pendingAction.value
    const result = await api.docker.action(selectedContainer.value.id, action, selectedContainer.value.resourceVersion)
    toast.success(
      '容器操作已完成',
      action === 'remove'
        ? `${selectedContainer.value.name} 已删除，镜像和存储卷保留`
        : result.status || selectedContainer.value.name,
    )
    pendingAction.value = undefined
    selectedContainer.value = undefined
    await load(true)
  } catch (reason) {
    toast.danger('容器操作失败', reason instanceof ApiError ? reason.message : '请稍后重试。')
  } finally {
    actionRunning.value = false
  }
}

async function showLogs(container: DockerContainer): Promise<void> {
  contextMenu.value = undefined
  selectedContainer.value = container
  logsOpen.value = true
  logsLoading.value = true
  logLines.value = []
  logError.value = ''
  logController?.abort()
  logController = new AbortController()
  try {
    const result = await api.docker.logs(container.id, 300, logController.signal)
    logLines.value = result.lines || []
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    logError.value = reason instanceof ApiError ? reason.message : '无法读取容器日志。'
  } finally {
    logsLoading.value = false
  }
}

function closeLogs(): void {
  logController?.abort()
  logsOpen.value = false
  selectedContainer.value = undefined
}

function stopStatsPolling(): void {
  statsPollGeneration += 1
  if (statsTimer) window.clearTimeout(statsTimer)
  statsTimer = undefined
  statsController?.abort()
  statsController = undefined
  statsLoading.value = false
}

function scheduleStatsRefresh(containerID: string, generation: number): void {
  if (
    generation !== statsPollGeneration ||
    !windowActive.value ||
    !statsOpen.value ||
    selectedContainer.value?.id !== containerID
  ) {
    return
  }
  if (statsTimer) window.clearTimeout(statsTimer)
  statsTimer = window.setTimeout(() => {
    statsTimer = undefined
    void refreshStats(generation)
  }, statsPollDelay)
}

async function refreshStats(generation = statsPollGeneration): Promise<void> {
  const container = selectedContainer.value
  if (!container || !statsOpen.value || !windowActive.value || statsController) return
  const requestController = new AbortController()
  statsController = requestController
  statsLoading.value = !stats.value
  statsError.value = ''
  try {
    const next = await api.docker.stats(container.id, requestController.signal)
    if (
      generation !== statsPollGeneration ||
      statsController !== requestController ||
      !windowActive.value ||
      !statsOpen.value ||
      selectedContainer.value?.id !== container.id
    ) {
      return
    }
    stats.value = next
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (generation !== statsPollGeneration || statsController !== requestController) return
    statsError.value = reason instanceof ApiError ? reason.message : '无法读取容器性能数据。'
  } finally {
    if (statsController === requestController) {
      statsController = undefined
      statsLoading.value = false
    }
    scheduleStatsRefresh(container.id, generation)
  }
}

function showStats(container: DockerContainer): void {
  stopStatsPolling()
  contextMenu.value = undefined
  selectedContainer.value = container
  stats.value = undefined
  statsOpen.value = true
  if (windowActive.value) void refreshStats()
}

function closeStats(): void {
  stopStatsPolling()
  statsOpen.value = false
  stats.value = undefined
  selectedContainer.value = undefined
}

function openConsole(container: DockerContainer): void {
  contextMenu.value = undefined
  selectedContainer.value = container
  consoleCommand.value = ''
  consoleOutput.value = ''
  consoleExitCode.value = undefined
  consoleOpen.value = true
}

async function runConsoleCommand(): Promise<void> {
  const container = selectedContainer.value
  const command = consoleCommand.value.trim()
  if (!container?.resourceVersion || !command || consoleRunning.value) return
  consoleRunning.value = true
  try {
    const result = await api.docker.exec(container.id, container.resourceVersion, command)
    consoleOutput.value = result.output || '命令执行完成，没有输出。'
    consoleExitCode.value = result.exitCode
  } catch (reason) {
    consoleOutput.value = reason instanceof ApiError ? reason.message : '容器控制台请求失败。'
    consoleExitCode.value = -1
  } finally {
    consoleRunning.value = false
  }
}

function closeConsole(): void {
  consoleOpen.value = false
  consoleCommand.value = ''
  consoleOutput.value = ''
  consoleExitCode.value = undefined
  selectedContainer.value = undefined
}

function openAccess(container: DockerContainer): void {
  contextMenu.value = undefined
  selectedContainer.value = container
  accessAllowedIP.value = publicIPv4.value
  accessOpen.value = true
}

function askAccess(allowExternal: boolean): void {
  const container = selectedContainer.value
  if (!container?.resourceVersion) return
  askTask(
    allowExternal ? '确认允许容器外部访问' : '确认阻止容器外部访问',
    allowExternal
      ? `${container.name} 将移除与 kejilion.sh 相同的 DOCKER-USER 限制规则`
      : `${container.name} 仅保留已建立连接、本机和指定来源 IP`,
    {
      action: 'container_access',
      target: container.id,
      expectedResourceVersion: container.resourceVersion,
      enabled: allowExternal,
      allowedIp: accessAllowedIP.value.trim() || undefined,
    },
    !allowExternal,
  )
}

function askRestore(backup: DockerBackup): void {
  askTask('确认还原 Docker 备份', `${backup.id} · ${formatBytes(backup.sizeBytes)}`, {
    action: 'backup_restore',
    backupId: backup.id,
  }, true)
}

function openMigration(backup: DockerBackup): void {
  migrationBackup.value = backup
  migrationHost.value = ''
  migrationUser.value = 'root'
  migrationPort.value = '22'
  migrationOpen.value = true
}

function askMigration(): void {
  if (!migrationBackup.value || !migrationHost.value.trim()) return
  const port = Number(migrationPort.value)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    toast.danger('SSH 端口无效', '端口必须是 1-65535 的整数。')
    return
  }
  askTask('确认迁移 Docker 备份', `${migrationUser.value}@${migrationHost.value}:${port}`, {
    action: 'backup_migrate',
    backupId: migrationBackup.value.id,
    migrationHost: migrationHost.value.trim(),
    migrationUser: migrationUser.value.trim(),
    migrationPort: port,
  })
}

onMounted(() => {
  window.addEventListener('click', closeContextMenuOnOutsideClick)
  window.addEventListener('resize', closeContextMenu)
  window.addEventListener('scroll', closeContextMenu, true)
  void load()
  void loadBackups()
  void loadEnvironment()
  void loadPublicIPv4()
  void restoreBackgroundJob()
})

watch(windowActive, (active) => {
  const job = activeJob.value
  const jobID = job && (job.status === 'queued' || job.status === 'running')
    ? job.id
    : pollingJobID
  if (jobID) {
    beginJobPolling(jobID, active)
  }

  if (!active) {
    stopStatsPolling()
    return
  }
  if (statsOpen.value && selectedContainer.value) {
    stopStatsPolling()
    void refreshStats()
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeContextMenuOnOutsideClick)
  window.removeEventListener('resize', closeContextMenu)
  window.removeEventListener('scroll', closeContextMenu, true)
  controller?.abort()
  logController?.abort()
  stopStatsPolling()
  stopJobPolling()
})
</script>

<template>
  <div class="page docker-page">
    <PageHeader
      title="Docker 管理"
      description="直接管理服务器上的容器、镜像、网络与存储；与 kejilion.sh 共用同一 Docker 实际状态。"
    />

    <div
      v-if="activeJob && (activeJob.status === 'queued' || activeJob.status === 'running')"
      class="inline-alert inline-alert--info docker-job"
      role="status"
    >
      <LoaderCircle class="spin" :size="18" />
      <span>
        <strong>{{ activeJob.message || 'Docker 后台任务正在执行' }}</strong>
        <small>{{ activeJob.target || activeJob.action }} · {{ activeJob.progress }}%</small>
      </span>
      <progress :value="activeJob.progress" max="100">{{ activeJob.progress }}%</progress>
    </div>

    <LoadingState v-if="loading" :rows="4" cards />
    <ErrorState v-else-if="error && !data" :message="error" @retry="load()" />

    <template v-else-if="data">
      <div v-if="!data.available" class="inline-alert inline-alert--warning" role="status">
        Docker Engine 当前不可用。资源写入已停止，页面可能只显示最后一次成功观测结果。
      </div>

      <section class="docker-command-center">
        <header class="docker-command-center__header">
          <div>
            <span class="workspace-card__icon"><Boxes :size="20" /></span>
            <span><strong>Docker Engine</strong><small>{{ data.version || '版本待检测' }} · 观测于 {{ formatDateTime(data.observedAt) }}</small></span>
          </div>
          <div class="docker-command-center__actions">
            <StatusBadge :status="data.available ? 'running' : 'critical'" :label="data.available ? '运行正常' : '连接异常'" />
            <button class="icon-button" type="button" :disabled="refreshing" title="刷新 Docker 状态" aria-label="刷新 Docker 状态" @click="load(true)">
              <RefreshCw :size="16" :class="{ spin: refreshing }" />
            </button>
          </div>
        </header>

        <section class="docker-summary" aria-label="Docker 摘要">
          <div><span class="summary-strip__icon"><Container :size="19" /></span><span><strong>{{ data.containers.length }}</strong><small>全部容器</small></span></div>
          <div><span class="summary-strip__icon summary-strip__icon--success"><Play :size="19" /></span><span><strong>{{ runningCount }}</strong><small>运行中</small></span></div>
          <div><span class="summary-strip__icon summary-strip__icon--blue"><ShieldCheck :size="19" /></span><span><strong>{{ manageableCount }}</strong><small>可管理</small></span></div>
          <div><span class="summary-strip__icon summary-strip__icon--violet"><Boxes :size="19" /></span><span><strong>{{ data.images.length }}</strong><small>本地镜像</small></span></div>
        </section>

        <nav class="docker-nav" aria-label="Docker 功能分区">
          <button
            v-for="tab in tabs"
            :key="tab.id"
            type="button"
            :class="{ 'is-active': activeTab === tab.id }"
            :aria-current="activeTab === tab.id ? 'page' : undefined"
            @click="activeTab = tab.id; search = ''; resourceSort = 'smart'"
          >
            <component :is="tab.icon" :size="17" />
            <strong>{{ tab.label }}</strong>
            <small>{{ tab.count }}</small>
          </button>
        </nav>

        <div v-if="activeTab !== 'environment'" class="docker-toolbar">
          <span class="docker-toolbar__count">显示 {{ visibleResourceCount }} 项</span>
          <div class="search-field search-field--small">
            <Search :size="16" />
            <input v-model="search" type="search" placeholder="搜索当前资源" aria-label="搜索 Docker 资源" />
          </div>
          <select v-model="resourceSort" class="select-input docker-sort" aria-label="Docker 资源排序">
            <option value="smart">智能排序</option>
            <option value="name-asc">名称 A–Z</option>
            <option value="name-desc">名称 Z–A</option>
          </select>
        </div>
      </section>

      <template v-if="activeTab === 'environment'">
        <div class="workspace-grid workspace-grid--environment">
          <section class="workspace-card workspace-card--wide">
            <header>
              <span class="workspace-card__icon"><Wrench :size="20" /></span>
              <div><strong>环境生命周期</strong><small>Docker Engine 与 Compose 由宿主机软件包管理器维护</small></div>
            </header>
            <div class="action-grid">
              <article class="action-card">
                <div>
                  <strong>Docker 环境</strong>
                  <small>
                    {{ environment?.storageDriver || '存储驱动待检测' }}
                    <template v-if="environment?.dataRoot"> · {{ environment.dataRoot }}</template>
                  </small>
                </div>
                <StatusBadge :status="data.available ? 'running' : 'critical'" />
              </article>
              <article class="action-card">
                <div><strong>安装 / 更新</strong><small>按当前发行版执行后台系统更新，与系统管理共用任务状态</small></div>
                <button class="button button--primary button--small" type="button" :disabled="panel.isReadOnly.value || dockerJobActive" @click="systemUpdatePending = true">
                  <RefreshCw :size="15" /> 更新环境
                </button>
              </article>
              <article class="action-card">
                <div><strong>卸载 Docker</strong><small>卸载会终止 KPanel；当前 Agent 尚缺少离线完成与结果回写适配器</small></div>
                <button class="button button--danger button--small" type="button" @click="uninstallNoticeOpen = true">
                  <Trash2 :size="15" /> 查看实现状态
                </button>
              </article>
            </div>
          </section>

          <section class="workspace-card">
            <header>
              <span class="workspace-card__icon"><Download :size="20" /></span>
              <div><strong>备份、还原与迁移</strong><small>后台归档当前适配器支持的 /home/docker 应用产物</small></div>
            </header>
            <div class="card-actions">
              <button class="button button--primary button--small" type="button" :disabled="taskRunning || panel.isReadOnly.value" @click="submitTask({ action: 'backup_create' })">
                <Download :size="15" /> 创建备份
              </button>
              <button class="button button--secondary button--small" type="button" :disabled="backupsLoading" @click="loadBackups">
                <RefreshCw :size="15" :class="{ spin: backupsLoading }" /> 刷新
              </button>
            </div>
            <LoadingState v-if="backupsLoading && !backups.length" :rows="2" />
            <div v-else-if="backups.length" class="backup-list">
              <article v-for="backup in backups" :key="backup.id">
                <div>
                  <strong>{{ backup.id }}</strong>
                  <small>{{ formatBytes(backup.sizeBytes) }} · {{ relativeTime(backup.createdAt) }}</small>
                </div>
                <div>
                  <button class="button button--ghost button--small" type="button" @click="openMigration(backup)">迁移</button>
                  <button class="button button--secondary button--small" type="button" @click="askRestore(backup)">还原</button>
                </div>
              </article>
            </div>
            <EmptyState v-else title="还没有 Docker 备份" description="创建后可在这里执行还原或 SSH 密钥迁移。" />
            <p class="card-note">还原任务直接按备份产物恢复；路径校验、原子替换与失败回滚属于实现保障，不限制管理员选择。</p>
          </section>

          <section class="workspace-card">
            <header>
              <span class="workspace-card__icon"><Network :size="20" /></span>
              <div><strong>镜像源与 IPv6</strong><small>保留 daemon.json 其他键，Docker 重启失败自动回滚</small></div>
            </header>
            <label class="field">
              <span>Docker 镜像源</span>
              <select v-model="mirrorPreset" class="select-input">
                <option value="cn">中国大陆加速源</option>
                <option value="official">Docker 官方默认</option>
              </select>
            </label>
            <div v-if="environment?.mirrorPreset === 'custom'" class="inline-alert inline-alert--warning">
              当前是脚本或人工配置的自定义镜像源。选择预设后才会替换 `registry-mirrors`，其他 daemon.json 键保持不变。
            </div>
            <div v-if="environment?.daemonWarning" class="inline-alert inline-alert--warning">{{ environment.daemonWarning }}</div>
            <button class="button button--secondary button--small" type="button" @click="askTask('确认切换 Docker 镜像源', mirrorPreset === 'cn' ? '启用与 kejilion.sh 一致的大陆镜像列表' : '移除 registry-mirrors，恢复官方默认线路', { action: 'daemon_mirror', preset: mirrorPreset })">
              应用镜像源
            </button>
            <div class="divider" />
            <label class="check-row">
              <input v-model="ipv6Enabled" type="checkbox" />
              <span><strong>Docker IPv6</strong><small>需要真实可路由或 ULA 的 /64 网段</small></span>
            </label>
            <input v-if="ipv6Enabled" v-model="ipv6CIDR" class="text-input" type="text" placeholder="fd42:6b50:616e:656c::/64" />
            <button class="button button--secondary button--small" type="button" @click="askTask(`确认${ipv6Enabled ? '开启' : '关闭'} Docker IPv6`, ipv6Enabled ? ipv6CIDR : '移除 fixed-cidr-v6', { action: 'daemon_ipv6', enabled: ipv6Enabled, ipv6Cidr: ipv6Enabled ? ipv6CIDR.trim() : undefined })">
              应用 IPv6 设置
            </button>
          </section>

          <section class="workspace-card workspace-card--danger">
            <header>
              <span class="workspace-card__icon"><BrushCleaning :size="20" /></span>
              <div><strong>环境清理</strong><small>对应 kejilion.sh 的 docker system prune -af --volumes</small></div>
            </header>
            <p>清理停止容器、未使用镜像、网络、卷和构建缓存，与 Docker Engine 的实际判定一致。</p>
            <button class="button button--danger button--small" type="button" @click="askPrune('prune', '完整清理未使用资源')">
              <Trash2 :size="15" /> 执行完整清理
            </button>
          </section>
        </div>
      </template>

      <template v-else-if="activeTab === 'containers'">
        <section class="workspace-card workspace-card--wide resource-section">
          <header class="resource-section__header">
            <div>
              <span class="workspace-card__icon"><Container :size="20" /></span>
              <div><strong>容器日常管理</strong><small>创建、生命周期、日志、性能、控制台和外部访问；右键可打开完整操作菜单</small></div>
            </div>
            <div class="card-actions">
              <button class="button button--secondary button--small" type="button" @click="askPrune('container_prune', '清理已停止容器')">
                <BrushCleaning :size="15" /> 清理停止容器
              </button>
              <button class="button button--primary button--small" type="button" :disabled="panel.isReadOnly.value" @click="resetCreateForm(); createOpen = true">
                <Plus :size="15" /> 新建容器
              </button>
            </div>
          </header>
          <EmptyState v-if="!filteredContainers.length" title="没有符合条件的容器" description="Docker Engine 未返回容器，或搜索条件没有匹配项。" />
          <div v-else class="table-scroll">
            <table class="data-table docker-table">
              <colgroup>
                <col class="docker-table__name" />
                <col class="docker-table__status" />
                <col class="docker-table__ports" />
                <col class="docker-table__network" />
                <col class="docker-table__owner" />
                <col class="docker-table__actions" />
              </colgroup>
              <thead><tr><th>容器</th><th>状态</th><th>端口</th><th>网络</th><th>归属</th><th>操作</th></tr></thead>
              <tbody v-for="group in containerGroups" :key="group.key" class="docker-group" :style="containerGroupStyle(group)">
                <tr class="docker-group__row">
                  <td colspan="6">
                    <div class="docker-group__summary">
                      <button
                        class="docker-group__toggle"
                        type="button"
                        :aria-expanded="!isContainerGroupCollapsed(group.key)"
                        :aria-label="`${isContainerGroupCollapsed(group.key) ? '展开' : '收起'} ${group.name}`"
                        @click="toggleContainerGroup(group.key)"
                      >
                        <ChevronRight :size="15" :class="{ 'is-expanded': !isContainerGroupCollapsed(group.key) }" />
                        <span class="docker-group__icon"><Boxes v-if="group.kind === 'compose'" :size="17" /><Container v-else :size="17" /></span>
                        <span class="docker-group__copy">
                          <strong>{{ group.name }}</strong>
                          <small v-if="group.kind === 'compose'">Compose 项目 · {{ group.running }}/{{ group.containers.length }} 运行中 · {{ group.services.length || group.containers.length }} 个服务</small>
                          <small v-else>{{ group.running }}/{{ group.containers.length }} 运行中 · 不属于 Compose 项目</small>
                        </span>
                      </button>
                      <button
                        v-if="group.kind === 'compose'"
                        class="button button--secondary button--small"
                        type="button"
                        :disabled="panel.isReadOnly.value || dockerJobActive"
                        @click="openComposeProject(group)"
                      ><Wrench :size="14" /> 管理 Compose</button>
                    </div>
                  </td>
                </tr>
                <TransitionGroup name="docker-group-row">
                  <tr
                    v-for="container in visibleContainerGroupRows(group)"
                    :key="container.id"
                    :class="`docker-row docker-row--${container.state}`"
                    @contextmenu="showContainerContext($event, container)"
                  >
                  <td>
                    <div class="resource-name">
                      <span class="resource-name__icon resource-name__icon--docker"><Container :size="18" /></span>
                      <span><strong>{{ container.name }}</strong><small :title="container.image">{{ container.image }}</small></span>
                    </div>
                  </td>
                  <td><div class="table-stack"><StatusBadge :status="container.state" /><small>{{ container.statusText || '—' }}</small></div></td>
                  <td><span class="table-code" :title="formatPorts(container)">{{ formatPorts(container) }}</span></td>
                  <td><span class="table-code">{{ container.networks.join(', ') || '—' }}</span></td>
                  <td><div class="table-stack"><StatusBadge :status="container.access" subtle /><small>{{ container.project || '独立容器' }}</small></div></td>
                  <td>
                    <div class="docker-row-actions">
                      <div class="docker-row-actions__group" aria-label="查看与管理">
                        <button v-if="permits(container, 'stats')" class="icon-button" type="button" title="性能占用" aria-label="性能占用" @click="showStats(container)"><Waypoints :size="16" /></button>
                        <button v-if="permits(container, 'logs')" class="icon-button" type="button" title="查看日志" aria-label="查看日志" @click="showLogs(container)"><FileText :size="16" /></button>
                        <button v-if="permits(container, 'exec')" class="icon-button" type="button" title="进入控制台" aria-label="进入控制台" @click="openConsole(container)"><Wrench :size="16" /></button>
                        <button v-if="permits(container, 'access')" class="icon-button" type="button" title="外部访问" aria-label="外部访问" @click="openAccess(container)"><ShieldCheck :size="16" /></button>
                      </div>
                      <div class="docker-row-actions__group" aria-label="生命周期">
                        <button v-if="permits(container, 'start')" class="icon-button icon-button--success" type="button" title="启动" aria-label="启动" @click="askAction(container, 'start')"><Play :size="16" /></button>
                        <button v-if="permits(container, 'unpause')" class="icon-button icon-button--success" type="button" title="继续运行" aria-label="继续运行" @click="askAction(container, 'unpause')"><Play :size="16" /></button>
                        <button v-if="permits(container, 'restart')" class="icon-button" type="button" title="重启" aria-label="重启" @click="askAction(container, 'restart')"><RotateCw :size="16" /></button>
                        <button v-if="permits(container, 'pause')" class="icon-button" type="button" title="暂停" aria-label="暂停" @click="askAction(container, 'pause')"><Pause :size="16" /></button>
                        <button v-if="permits(container, 'stop')" class="icon-button icon-button--danger" type="button" title="停止" aria-label="停止" @click="askAction(container, 'stop')"><CircleStop :size="16" /></button>
                        <button v-if="permits(container, 'remove')" class="icon-button icon-button--danger" type="button" title="删除" aria-label="删除" @click="askAction(container, 'remove')"><Trash2 :size="16" /></button>
                      </div>
                      <button class="icon-button docker-context-trigger" type="button" title="更多操作" aria-label="更多操作" @click="showContainerContext($event, container)"><EllipsisVertical :size="16" /></button>
                      <span v-if="!container.allowedActions?.length" class="action-unavailable-label">状态暂不可操作</span>
                    </div>
                  </td>
                  </tr>
                </TransitionGroup>
              </tbody>
            </table>
          </div>
        </section>
      </template>

      <template v-else-if="activeTab === 'images'">
        <section class="workspace-card workspace-card--wide resource-section">
          <header class="resource-section__header">
            <div><span class="workspace-card__icon"><Box :size="20" /></span><div><strong>镜像日常管理</strong><small>拉取即更新；右键可复制引用、更新或删除</small></div></div>
            <div class="card-actions">
              <input v-model="imageReference" class="text-input compact-input" type="text" placeholder="nginx:alpine" @keyup.enter="pullImage" />
              <button class="button button--primary button--small" type="button" :disabled="!imageReference.trim()" @click="pullImage"><Download :size="15" /> 拉取镜像</button>
              <button class="button button--secondary button--small" type="button" @click="askPrune('image_prune', '清理未使用镜像')"><BrushCleaning :size="15" /> 清理</button>
            </div>
          </header>
          <EmptyState v-if="!filteredImages.length" title="没有本地镜像" description="可输入完整镜像引用拉取，任务会在后台继续。" />
          <div v-else class="table-scroll">
            <table class="data-table"><thead><tr><th>镜像</th><th>摘要</th><th>大小</th><th>创建时间</th><th>状态</th><th>操作</th></tr></thead>
              <tbody><tr v-for="image in filteredImages" :key="image.id" @contextmenu="showImageContext($event, image)">
                <td><strong>{{ image.tags.join(', ') || '未标记镜像' }}</strong></td>
                <td><code>{{ shortId(image.id) }}</code></td>
                <td>{{ formatBytes(image.sizeBytes) }}</td>
                <td>{{ image.createdAt ? relativeTime(image.createdAt) : '未知' }}</td>
                <td><StatusBadge :status="image.inUse ? 'running' : 'stopped'" :label="image.inUse ? '使用中' : '未使用'" subtle /></td>
                <td><div class="row-actions">
                  <button v-if="image.tags.length" class="button button--ghost button--small" type="button" @click="updateImage(image)"><RefreshCw :size="14" /> 更新</button>
                  <button class="icon-button icon-button--danger" type="button" title="删除镜像" :disabled="!image.resourceVersion" @click="askImageRemoval(image)"><Trash2 :size="16" /></button>
                  <button class="icon-button docker-context-trigger" type="button" title="更多操作" aria-label="更多操作" @click="showImageContext($event, image)"><EllipsisVertical :size="16" /></button>
                </div></td>
              </tr></tbody>
            </table>
          </div>
        </section>
      </template>

      <template v-else-if="activeTab === 'networks'">
        <div class="workspace-grid">
          <section class="workspace-card workspace-card--wide resource-section">
            <header class="resource-section__header">
              <div><span class="workspace-card__icon"><Network :size="20" /></span><div><strong>网络日常管理</strong><small>网络创建、删除和成员关系均直接写入 Docker Engine；支持右键管理</small></div></div>
              <div class="card-actions">
                <input v-model="networkName" class="text-input compact-input" type="text" placeholder="新网络名称" @keyup.enter="createDockerNetwork" />
                <input v-model="networkDriver" class="text-input compact-input compact-input--driver" type="text" placeholder="驱动，例如 bridge" @keyup.enter="createDockerNetwork" />
                <button class="button button--primary button--small" type="button" :disabled="!networkName.trim() || !networkDriver.trim()" @click="createDockerNetwork"><Plus :size="15" /> 创建网络</button>
                <button class="button button--secondary button--small" type="button" @click="askPrune('network_prune', '清理未使用网络')"><BrushCleaning :size="15" /> 清理</button>
              </div>
            </header>
            <div class="network-membership">
              <select v-model="membershipContainerID" class="select-input"><option value="">选择容器</option><option v-for="item in membershipContainers" :key="item.id" :value="item.id">{{ item.name }}</option></select>
              <select v-model="membershipNetworkID" class="select-input"><option value="">选择网络</option><option v-for="item in membershipNetworks" :key="item.id" :value="item.id">{{ item.name }}</option></select>
              <button class="button button--secondary button--small" type="button" :disabled="!membershipContainerID || !membershipNetworkID" @click="updateNetworkMembership('network_connect')">加入</button>
              <button class="button button--ghost button--small" type="button" :disabled="!membershipContainerID || !membershipNetworkID" @click="updateNetworkMembership('network_disconnect')">退出</button>
            </div>
            <EmptyState v-if="!filteredNetworks.length" title="没有 Docker 网络" description="Docker Engine 未返回网络资源。" />
            <div v-else class="table-scroll">
              <table class="data-table"><thead><tr><th>网络</th><th>驱动</th><th>范围</th><th>容器数</th><th>操作</th></tr></thead>
                <tbody><tr v-for="network in filteredNetworks" :key="network.id" @contextmenu="showNetworkContext($event, network)">
                  <td><strong>{{ network.name }}</strong><small class="table-sub">{{ shortId(network.id) }}</small></td>
                  <td>{{ network.driver }}</td><td>{{ network.scope || 'local' }}</td><td>{{ network.containers || 0 }}</td>
                  <td><div class="row-actions">
                    <button class="icon-button icon-button--danger" type="button" title="删除网络" :disabled="!network.resourceVersion" @click="askNetworkRemoval(network)"><Trash2 :size="16" /></button>
                    <button class="icon-button docker-context-trigger" type="button" title="更多操作" aria-label="更多操作" @click="showNetworkContext($event, network)"><EllipsisVertical :size="16" /></button>
                  </div></td>
                </tr></tbody>
              </table>
            </div>
          </section>
        </div>
      </template>

      <template v-else>
        <section class="workspace-card workspace-card--wide resource-section">
          <header class="resource-section__header">
            <div><span class="workspace-card__icon"><HardDrive :size="20" /></span><div><strong>存储卷日常管理</strong><small>所有 Docker 卷均可直接管理；右键可复制名称、挂载点或删除</small></div></div>
            <div class="card-actions">
              <input v-model="volumeName" class="text-input compact-input" type="text" placeholder="新存储卷名称" @keyup.enter="createDockerVolume" />
              <input v-model="volumeDriver" class="text-input compact-input compact-input--driver" type="text" placeholder="驱动，例如 local" @keyup.enter="createDockerVolume" />
              <button class="button button--primary button--small" type="button" :disabled="!volumeName.trim() || !volumeDriver.trim()" @click="createDockerVolume"><Plus :size="15" /> 创建卷</button>
              <button class="button button--secondary button--small" type="button" @click="askPrune('volume_prune', '清理未使用存储卷')"><BrushCleaning :size="15" /> 清理</button>
            </div>
          </header>
          <EmptyState v-if="!filteredVolumes.length" title="没有 Docker 存储卷" description="可创建 local 卷，并在新建容器时选择挂载。" />
          <div v-else class="table-scroll">
            <table class="data-table"><thead><tr><th>存储卷</th><th>驱动</th><th>挂载点</th><th>状态</th><th>操作</th></tr></thead>
              <tbody><tr v-for="volume in filteredVolumes" :key="volume.name" @contextmenu="showVolumeContext($event, volume)">
                <td><strong>{{ volume.name }}</strong></td><td>{{ volume.driver }}</td>
                <td><span class="table-code" :title="volume.mountpoint">{{ volume.mountpoint || '—' }}</span></td>
                <td><StatusBadge :status="volume.inUse ? 'running' : 'stopped'" :label="volume.inUse ? '使用中' : '未使用'" subtle /></td>
                <td><div class="row-actions">
                  <button class="icon-button icon-button--danger" type="button" title="删除存储卷" :disabled="!volume.resourceVersion" @click="askVolumeRemoval(volume)"><Trash2 :size="16" /></button>
                  <button class="icon-button docker-context-trigger" type="button" title="更多操作" aria-label="更多操作" @click="showVolumeContext($event, volume)"><EllipsisVertical :size="16" /></button>
                </div></td>
              </tr></tbody>
            </table>
          </div>
        </section>
      </template>
    </template>

    <div
      v-if="contextMenu"
      class="docker-context-menu"
      :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
      role="menu"
      @click.stop
      @contextmenu.prevent
    >
      <template v-if="contextContainer">
        <strong class="docker-context-menu__title">{{ contextContainer.name }}</strong>
        <button v-if="permits(contextContainer, 'logs')" type="button" @click="showLogs(contextContainer)">
          <FileText :size="15" />查看日志
        </button>
        <button v-if="permits(contextContainer, 'stats')" type="button" @click="showStats(contextContainer)">
          <Waypoints :size="15" />性能占用
        </button>
        <button v-if="permits(contextContainer, 'exec')" type="button" @click="openConsole(contextContainer)">
          <Wrench :size="15" />容器控制台
        </button>
        <button v-if="permits(contextContainer, 'access')" type="button" @click="openAccess(contextContainer)">
          <ShieldCheck :size="15" />外部访问
        </button>
        <hr />
        <button v-if="permits(contextContainer, 'start')" type="button" @click="askAction(contextContainer, 'start')">
          <Play :size="15" />启动
        </button>
        <button v-if="permits(contextContainer, 'unpause')" type="button" @click="askAction(contextContainer, 'unpause')">
          <Play :size="15" />继续运行
        </button>
        <button v-if="permits(contextContainer, 'restart')" type="button" @click="askAction(contextContainer, 'restart')">
          <RotateCw :size="15" />重启
        </button>
        <button v-if="permits(contextContainer, 'pause')" type="button" @click="askAction(contextContainer, 'pause')">
          <Pause :size="15" />暂停
        </button>
        <button v-if="permits(contextContainer, 'stop')" type="button" @click="askAction(contextContainer, 'stop')">
          <CircleStop :size="15" />停止
        </button>
        <hr />
        <button type="button" @click="copyResourceValue(contextContainer.id, '容器 ID')">
          <Copy :size="15" />复制容器 ID
        </button>
        <button type="button" @click="copyResourceValue(contextContainer.image, '镜像名称')">
          <Copy :size="15" />复制镜像名称
        </button>
        <button v-if="permits(contextContainer, 'remove')" class="danger-link" type="button" @click="askAction(contextContainer, 'remove')">
          <Trash2 :size="15" />删除容器
        </button>
      </template>

      <template v-else-if="contextImage">
        <strong class="docker-context-menu__title">{{ contextImage.tags[0] || shortId(contextImage.id) }}</strong>
        <button v-if="contextImage.tags.length" type="button" @click="updateImage(contextImage)">
          <RefreshCw :size="15" />拉取最新版本
        </button>
        <button type="button" @click="copyResourceValue(contextImage.tags[0] || contextImage.id, '镜像引用')">
          <Copy :size="15" />复制镜像引用
        </button>
        <button type="button" @click="copyResourceValue(contextImage.id, '镜像 ID')">
          <Copy :size="15" />复制镜像 ID
        </button>
        <hr />
        <button class="danger-link" type="button" :disabled="!contextImage.resourceVersion" @click="askImageRemoval(contextImage)">
          <Trash2 :size="15" />删除镜像
        </button>
      </template>

      <template v-else-if="contextNetwork">
        <strong class="docker-context-menu__title">{{ contextNetwork.name }}</strong>
        <button type="button" @click="manageNetworkMembership(contextNetwork)">
          <Network :size="15" />管理容器成员
        </button>
        <button type="button" @click="copyResourceValue(contextNetwork.name, '网络名称')">
          <Copy :size="15" />复制网络名称
        </button>
        <button type="button" @click="copyResourceValue(contextNetwork.id, '网络 ID')">
          <Copy :size="15" />复制网络 ID
        </button>
        <hr />
        <button class="danger-link" type="button" :disabled="!contextNetwork.resourceVersion" @click="askNetworkRemoval(contextNetwork)">
          <Trash2 :size="15" />删除网络
        </button>
      </template>

      <template v-else-if="contextVolume">
        <strong class="docker-context-menu__title">{{ contextVolume.name }}</strong>
        <button type="button" @click="copyResourceValue(contextVolume.name, '存储卷名称')">
          <Copy :size="15" />复制存储卷名称
        </button>
        <button v-if="contextVolume.mountpoint" type="button" @click="copyResourceValue(contextVolume.mountpoint, '挂载点')">
          <Copy :size="15" />复制挂载点
        </button>
        <hr />
        <button class="danger-link" type="button" :disabled="!contextVolume.resourceVersion" @click="askVolumeRemoval(contextVolume)">
          <Trash2 :size="15" />删除存储卷
        </button>
      </template>
    </div>

    <ModalDialog :open="createOpen" title="部署 Docker 应用" description="粘贴现成内容即可，KPanel 会自动识别 Docker Run 或 Docker Compose。" size="large" @close="createOpen = false">
      <section v-if="!createManualMode" class="deployment-input-card">
        <label class="field">
          <span>粘贴部署内容</span>
          <DockerDeploymentEditor
            v-model="createSource"
            :diagnostics="createDiagnostics"
            placeholder="docker run -d --name my-app -p 8080:80 nginx:alpine&#10;&#10;也可以直接粘贴 compose.yaml 内容"
          />
        </label>
        <div v-if="createAnalysis.kind !== 'invalid' || !createDiagnostics.length" class="deployment-detection" :class="{ 'is-invalid': createAnalysis.kind === 'invalid', 'is-ready': createAnalysis.kind === 'docker-run' || createAnalysis.kind === 'compose' }">
          <template v-if="createAnalysis.kind === 'empty'">
            <span class="deployment-kind"><FileText :size="16" />等待粘贴</span>
            <small>支持常见 docker run 参数和完整 Compose YAML，无需先选择部署类型。</small>
          </template>
          <template v-else-if="createAnalysis.kind === 'invalid'">
            <span class="deployment-kind"><CircleStop :size="16" />暂时无法识别</span>
            <small>{{ createAnalysis.message }}</small>
          </template>
          <template v-else>
            <span class="deployment-kind"><ShieldCheck :size="16" />{{ createModeLabel }}</span>
            <small>{{ createSummary }}</small>
          </template>
        </div>
      </section>

      <div class="deployment-options">
        <button v-if="createManualMode" class="button button--ghost button--small" type="button" @click="resetCreateForm"><FileText :size="14" /> 返回粘贴部署内容</button>
        <button v-else class="button button--ghost button--small" type="button" @click="startManualCreate"><Wrench :size="14" /> 没有现成内容？手动配置</button>
        <button v-if="!createManualMode && (createAnalysis.kind === 'docker-run' || createAnalysis.kind === 'compose')" class="button button--ghost button--small" type="button" @click="showCreateAdvanced"><Wrench :size="14" /> {{ createAdvanced ? '收起高级设置' : '高级设置' }}</button>
      </div>

      <section v-if="createAdvanced && createAnalysis.kind === 'compose' && !createManualMode" class="deployment-advanced">
        <label class="field"><span>Compose 项目名称</span><input v-model="createComposeProject" class="text-input" type="text" maxlength="63" autocomplete="off" /><small>已自动从服务名生成；项目文件会保存到 /home/docker 下。</small><code data-i18n-ignore>/home/docker/{{ createComposeProject || 'project' }}/docker-compose.yml</code></label>
      </section>

      <section v-if="createAnalysis.kind === 'compose' && (createComposeEnvironment.length || createAdvanced)" class="deployment-advanced compose-environment-card">
        <header class="compose-environment-card__header">
          <div>
            <strong>项目变量 <code data-i18n-ignore>.env</code></strong>
            <small v-if="createComposeEnvironmentMissing" class="compose-environment-card__missing">{{ createComposeEnvironmentMissing }} 项待填写</small>
            <small v-else>{{ createComposeEnvironment.length }} 个变量 · 部署时自动加载</small>
          </div>
          <div class="compose-environment-card__actions">
            <button v-if="createComposeEnvironmentOpen" class="button button--ghost button--small" type="button" @click="createComposeEnvironmentRevealed = !createComposeEnvironmentRevealed">{{ createComposeEnvironmentRevealed ? '隐藏值' : '显示值' }}</button>
            <button class="button button--ghost button--small" type="button" @click="createComposeEnvironmentOpen = !createComposeEnvironmentOpen">{{ createComposeEnvironmentOpen ? '收起' : '填写变量' }}</button>
          </div>
        </header>
        <div v-if="createComposeEnvironmentOpen" class="compose-environment-card__body">
          <div v-for="(variable, index) in createComposeEnvironment" :key="`${variable.name}:${index}`" class="repeat-row repeat-row--compose-environment">
            <input v-model="variable.name" class="text-input" type="text" maxlength="128" placeholder="变量名" :readonly="variable.detected" autocomplete="off" />
            <input v-model="variable.value" class="text-input" :type="createComposeEnvironmentRevealed ? 'text' : 'password'" maxlength="2048" :placeholder="variable.defaultValue !== undefined ? `默认：${variable.defaultValue || '空值'}` : variable.required ? '必填' : '变量值'" autocomplete="new-password" />
            <span v-if="variable.required" class="compose-environment-card__required">必填</span>
            <span v-else-if="variable.defaultValue !== undefined" class="compose-environment-card__default">有默认值</span>
            <button v-if="!variable.detected" class="icon-button icon-button--danger" type="button" title="移除" @click="createComposeEnvironment.splice(index, 1)"><Trash2 :size="15" /></button>
          </div>
          <button class="button button--ghost button--small compose-environment-card__add" type="button" @click="addCreateComposeEnvironment"><Plus :size="14" /> 添加变量</button>
          <small>用于 Compose 中的 <code data-i18n-ignore>${VAR}</code> 插值；变量值不会保留在已完成的任务记录中。</small>
        </div>
      </section>

      <section v-if="createAdvanced && (createManualMode || createAnalysis.kind === 'docker-run')" class="deployment-advanced">
        <div class="form-grid form-grid--two">
          <label class="field"><span>容器名称（可选）</span><input v-model="createName" class="text-input" type="text" placeholder="留空由 Docker 自动命名" /></label>
          <label class="field"><span>镜像</span><input v-model="createImage" class="text-input" type="text" list="docker-image-tags" placeholder="nginx:alpine" /><small>本机没有时自动拉取。</small><datalist id="docker-image-tags"><option v-for="tag in availableImageTags" :key="tag" :value="tag" /></datalist></label>
          <label class="field"><span>网络</span><select v-model="createNetwork" class="select-input"><option v-for="item in createNetworks" :key="item.id" :value="item.name">{{ item.name }}</option></select></label>
          <label class="field"><span>重启策略</span><select v-model="createRestartPolicy" class="select-input"><option value="unless-stopped">unless-stopped</option><option value="always">always</option><option value="on-failure">on-failure</option><option value="no">no</option></select></label>
        </div>
        <div class="form-section">
          <header><div><strong>端口映射</strong><small>0.0.0.0 为公开，127.0.0.1 仅供本机反代</small></div><button class="button button--ghost button--small" type="button" @click="addCreatePort"><Plus :size="14" /> 添加</button></header>
          <div v-for="(port, index) in createPorts" :key="index" class="repeat-row repeat-row--ports">
            <input v-model="port.publicPort" class="text-input" inputmode="numeric" placeholder="主机端口" /><span>→</span><input v-model="port.privatePort" class="text-input" inputmode="numeric" placeholder="容器端口" />
            <select v-model="port.protocol" class="select-input"><option value="tcp">TCP</option><option value="udp">UDP</option></select><input v-model="port.hostIp" class="text-input" type="text" list="docker-host-ip-presets" placeholder="0.0.0.0" /><button class="icon-button icon-button--danger" type="button" title="移除" @click="createPorts.splice(index, 1)"><Trash2 :size="15" /></button>
          </div>
        </div>
        <div class="form-section">
          <header><div><strong>存储挂载</strong><small>支持命名卷与宿主机绝对目录</small></div><button class="button button--ghost button--small" type="button" @click="addCreateMount"><Plus :size="14" /> 添加</button></header>
          <div v-for="(mount, index) in createMounts" :key="index" class="repeat-row repeat-row--mounts">
            <select v-model="mount.type" class="select-input"><option value="volume">命名卷</option><option value="bind">宿主机目录</option></select><input v-model="mount.source" class="text-input" type="text" :list="mount.type === 'volume' ? 'docker-volume-presets' : undefined" :placeholder="mount.type === 'volume' ? '卷名' : '/home/docker/my-app'" /><input v-model="mount.target" class="text-input" type="text" placeholder="/data" /><label class="inline-check"><input v-model="mount.readOnly" type="checkbox" /> 只读</label><button class="icon-button icon-button--danger" type="button" title="移除" @click="createMounts.splice(index, 1)"><Trash2 :size="15" /></button>
          </div>
        </div>
        <datalist id="docker-host-ip-presets"><option value="0.0.0.0" /><option value="127.0.0.1" /><option value="::" /><option value="::1" /></datalist>
        <datalist id="docker-volume-presets"><option v-for="volume in data?.volumes || []" :key="volume.name" :value="volume.name" /></datalist>
        <div class="form-section">
          <header><div><strong>环境变量</strong><small>任务完成后不保留在 KPanel 任务记录中</small></div><button class="button button--ghost button--small" type="button" @click="addCreateEnvironment"><Plus :size="14" /> 添加</button></header>
          <div v-for="(variable, index) in createEnvironment" :key="index" class="repeat-row repeat-row--environment"><input v-model="variable.name" class="text-input" type="text" maxlength="128" placeholder="变量名，例如 TZ" autocomplete="off" /><input v-model="variable.value" class="text-input" type="text" maxlength="2048" placeholder="变量值" autocomplete="off" /><button class="icon-button icon-button--danger" type="button" title="移除" @click="createEnvironment.splice(index, 1)"><Trash2 :size="15" /></button></div>
        </div>
        <label class="field"><span>启动参数（可选，每行一个参数）</span><textarea v-model="createCommand" class="text-area" rows="3" placeholder="--config&#10;/data/config.yml" /></label>
      </section>
      <div class="inline-alert inline-alert--info">Docker Run 会转换为结构化 Docker API；Compose 会先校验配置，再保存到 `/home/docker` 并后台启动。启动失败会自动尝试回滚，并保留明确的处理状态。</div>
      <template #footer><span class="modal-footer-note">{{ createModeLabel || '自动识别部署方式' }}</span><button class="button button--secondary" type="button" @click="createOpen = false">取消</button><button class="button button--primary" type="button" :disabled="!createCanSubmit" @click="submitContainerCreate">部署</button></template>
    </ModalDialog>

    <ModalDialog
      :open="composeOpen"
      :title="composeProject ? `管理 Compose · ${composeProject.name}` : '管理 Compose 项目'"
      description="配置来自 Docker Compose 的实际工作目录；修改前校验版本，失败时自动恢复原配置。"
      size="large"
      @close="closeComposeProject"
    >
      <LoadingState v-if="composeLoading" :rows="4" />
      <ErrorState v-else-if="composeError" :message="composeError" />
      <div v-else-if="composeProject" class="compose-manager">
        <div class="compose-manager__meta">
          <span><small>项目目录</small><code data-i18n-ignore>{{ composeProject.workingDirectory }}</code></span>
          <span><small>服务</small><strong>{{ composeProject.services.join(' · ') || '由 Compose 实际配置决定' }}</strong></span>
        </div>
        <label v-if="composeProject.configFiles.length > 1" class="field">
          <span>配置文件</span>
          <select v-model="composeFilePath" class="select-input" @change="selectComposeFile">
            <option v-for="file in composeProject.configFiles" :key="file.path" :value="file.path">{{ file.name }}</option>
          </select>
        </label>
        <label class="field">
          <span>{{ selectedComposeFile?.name || 'Compose 配置' }}</span>
          <DockerDeploymentEditor
            v-model="composeSource"
            :diagnostics="composeDiagnostics"
            :aria-label="`${composeProject.name} Compose 配置`"
          />
        </label>
        <section class="compose-environment-card compose-environment-card--manager">
          <header class="compose-environment-card__header">
            <div>
              <strong>项目变量 <code data-i18n-ignore>.env</code></strong>
              <small>{{ composeEnvironmentCount ? `${composeEnvironmentCount} 个变量` : '当前为空' }} · 与配置一起校验和回滚</small>
            </div>
            <button class="button button--ghost button--small" type="button" @click="composeEnvironmentOpen = !composeEnvironmentOpen">{{ composeEnvironmentOpen ? '收起' : '管理变量' }}</button>
          </header>
          <div v-if="composeEnvironmentOpen" class="compose-environment-card__body">
            <div v-if="!composeEnvironmentRevealed" class="compose-environment-card__concealed">
              <span>变量值默认隐藏，显示后才能编辑。</span>
              <button class="button button--secondary button--small" type="button" @click="composeEnvironmentRevealed = true">显示并编辑</button>
            </div>
            <label v-else class="field">
              <span>每行一个 <code data-i18n-ignore>KEY=VALUE</code></span>
              <textarea v-model="composeEnvironmentSource" class="text-area compose-environment-card__editor" rows="6" spellcheck="false" autocomplete="off" placeholder="DB_PASSWORD=change-me" />
              <small>保存后写入项目目录中的 <code data-i18n-ignore>.env</code>，权限为 0600；敏感值不会保留在已完成的任务记录中。</small>
            </label>
          </div>
        </section>
        <div v-if="composeAnalysis.kind !== 'invalid' || !composeDiagnostics.length" class="deployment-detection" :class="{ 'is-invalid': composeAnalysis.kind === 'invalid', 'is-ready': composeAnalysis.kind === 'compose' }">
          <span v-if="composeAnalysis.kind === 'compose'" class="deployment-kind"><ShieldCheck :size="16" />语法检查通过</span>
          <span v-else class="deployment-kind"><CircleStop :size="16" />配置暂不可部署</span>
          <small v-if="composeAnalysis.kind === 'compose'">识别到 {{ composeAnalysis.services.length }} 个服务；Agent 提交前还会执行 docker compose config。</small>
          <small v-else-if="composeAnalysis.kind === 'invalid'">{{ composeAnalysis.message }}</small>
        </div>
      </div>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="!composeProject" @click="askComposeLifecycle('compose_start')"><Play :size="15" /> 启动项目</button>
        <button class="button button--secondary" type="button" :disabled="!composeProject" @click="askComposeLifecycle('compose_restart')"><RotateCw :size="15" /> 重启项目</button>
        <button class="button button--secondary" type="button" :disabled="!composeProject" @click="askComposeLifecycle('compose_stop')"><CircleStop :size="15" /> 停止项目</button>
        <button class="button button--primary" type="button" :disabled="!composeCanRedeploy" @click="submitComposeRedeploy"><RefreshCw :size="15" /> 保存并重新部署</button>
      </template>
    </ModalDialog>

    <ModalDialog :open="logsOpen" :title="`${selectedContainer?.name || '容器'} 日志`" description="显示最近 300 行，输出经过敏感字段脱敏和大小限制。" size="large" @close="closeLogs">
      <LoadingState v-if="logsLoading" :rows="3" />
      <ErrorState v-else-if="logError" :message="logError" retry-label="重新读取" @retry="selectedContainer && showLogs(selectedContainer)" />
      <p v-else-if="!logLines.length" class="log-viewer log-viewer-empty">当前没有日志输出。</p>
      <pre v-else class="log-viewer" data-i18n-ignore>{{ logLines.join('\n') }}</pre>
      <template #footer><span class="modal-footer-note">{{ logLines.length }} 行</span><button class="button button--secondary" type="button" @click="closeLogs">关闭</button></template>
    </ModalDialog>

    <ModalDialog :open="statsOpen" :title="`${selectedContainer?.name || '容器'} 性能占用`" description="Docker 单次采样，每 3 秒刷新；关闭弹窗即停止采样。" size="large" @close="closeStats">
      <LoadingState v-if="statsLoading && !stats" :rows="3" cards />
      <ErrorState v-else-if="statsError && !stats" :message="statsError" retry-label="重新读取" @retry="refreshStats" />
      <div v-else-if="stats" class="stats-grid">
        <article><small>CPU</small><strong>{{ stats.cpuPercent.toFixed(2) }}%</strong></article>
        <article><small>内存</small><strong>{{ stats.memoryPercent.toFixed(2) }}%</strong><span>{{ formatBytes(stats.memoryBytes) }} / {{ formatBytes(stats.memoryLimitBytes) }}</span></article>
        <article><small>网络接收</small><strong>{{ formatBytes(stats.networkRxBytes) }}</strong></article>
        <article><small>网络发送</small><strong>{{ formatBytes(stats.networkTxBytes) }}</strong></article>
        <article><small>磁盘读取</small><strong>{{ formatBytes(stats.blockReadBytes) }}</strong></article>
        <article><small>磁盘写入</small><strong>{{ formatBytes(stats.blockWriteBytes) }}</strong></article>
        <article><small>进程数</small><strong>{{ stats.pids }}</strong></article>
        <article><small>采样时间</small><strong class="stats-time">{{ formatDateTime(stats.collectedAt) }}</strong></article>
      </div>
      <template #footer><button class="button button--secondary" type="button" @click="refreshStats()"><RefreshCw :size="15" /> 刷新</button><button class="button button--secondary" type="button" @click="closeStats">关闭</button></template>
    </ModalDialog>

    <ModalDialog :open="consoleOpen" :title="`${selectedContainer?.name || '容器'} 控制台`" description="单次命令通过容器内 /bin/sh 执行，最长 20 秒；命令本身不写入审计或任务日志。" size="large" @close="closeConsole">
      <label class="field"><span>命令</span><div class="console-command"><span>$</span><input v-model="consoleCommand" class="text-input" type="text" maxlength="2048" placeholder="ls -la /app" @keyup.enter="runConsoleCommand" /><button class="button button--primary" type="button" :disabled="!consoleCommand.trim() || consoleRunning" @click="runConsoleCommand"><LoaderCircle v-if="consoleRunning" class="spin" :size="15" /><Play v-else :size="15" /> 执行</button></div></label>
      <p v-if="!consoleOutput" class="log-viewer log-viewer-empty">输入命令后查看输出。</p>
      <pre v-else class="log-viewer console-output" data-i18n-ignore>{{ consoleOutput }}</pre>
      <div v-if="consoleExitCode !== undefined" class="inline-alert" :class="consoleExitCode === 0 ? 'inline-alert--success' : 'inline-alert--warning'">退出码：{{ consoleExitCode }}{{ consoleExitCode === 0 ? '，执行成功' : '，请检查输出' }}</div>
      <template #footer><button class="button button--secondary" type="button" @click="closeConsole">关闭</button></template>
    </ModalDialog>

    <ModalDialog :open="accessOpen" :title="`${selectedContainer?.name || '容器'} 外部访问`" description="规则与 kejilion.sh 的 DOCKER-USER 方案互通，按容器 Docker IPv4 生效。" size="small" @close="accessOpen = false; selectedContainer = undefined">
      <label class="field"><span>阻止外部访问时额外允许的来源 IPv4</span><input v-model="accessAllowedIP" class="text-input" type="text" placeholder="留空则仅保留本机和已建立连接" /><small>默认使用当前服务器公网 IPv4，便于和脚本端清除规则保持一致。</small></label>
      <div class="inline-alert inline-alert--info">多网络容器会对每个 Docker IPv4 同步应用规则，与脚本端 DOCKER-USER 产物互通。</div>
      <template #footer><button class="button button--secondary" type="button" @click="askAccess(true)">允许外部访问</button><button class="button button--danger" type="button" @click="askAccess(false)">阻止外部访问</button></template>
    </ModalDialog>

    <ModalDialog :open="migrationOpen" title="迁移 Docker 备份" :description="migrationBackup?.id" size="small" @close="migrationOpen = false">
      <div class="form-grid">
        <label class="field"><span>目标服务器</span><input v-model="migrationHost" class="text-input" type="text" placeholder="server.example.com 或 IP" /></label>
        <label class="field"><span>SSH 用户</span><input v-model="migrationUser" class="text-input" type="text" placeholder="root" /></label>
        <label class="field"><span>SSH 端口</span><input v-model="migrationPort" class="text-input" inputmode="numeric" placeholder="22" /></label>
      </div>
      <div class="inline-alert inline-alert--info">只使用宿主机已配置的 SSH 密钥与 known_hosts，不接收或保存密码；备份传到目标服务器 `/tmp`。</div>
      <template #footer><button class="button button--secondary" type="button" @click="migrationOpen = false">取消</button><button class="button button--primary" type="button" :disabled="!migrationHost.trim()" @click="askMigration">检查并迁移</button></template>
    </ModalDialog>

    <ModalDialog :open="Boolean(pendingMaintenance)" :title="pendingMaintenance?.title || '确认 Docker 操作'" :description="pendingMaintenance?.description" size="small" @close="pendingMaintenance = undefined">
      <div class="confirm-content">
        <span class="confirm-content__icon" :class="{ 'is-danger': pendingMaintenance?.danger }"><Trash2 v-if="pendingMaintenance?.danger" :size="23" /><ShieldCheck v-else :size="23" /></span>
        <p>Agent 会在执行前重新校验输入和 Docker 实际状态；任务进入后台后可以离开当前页面。</p>
      </div>
      <template #footer><button class="button button--secondary" type="button" @click="pendingMaintenance = undefined">取消</button><button class="button" :class="pendingMaintenance?.danger ? 'button--danger' : 'button--primary'" type="button" :disabled="taskRunning || !pendingMaintenance" @click="pendingMaintenance && submitTask(pendingMaintenance.input)"><LoaderCircle v-if="taskRunning" class="spin" :size="16" />{{ taskRunning ? '正在提交…' : '确认执行' }}</button></template>
    </ModalDialog>

    <ModalDialog :open="Boolean(pendingAction)" :title="pendingAction === 'stop' ? '确认停止容器' : pendingAction === 'restart' ? '确认重启容器' : pendingAction === 'pause' ? '确认暂停容器' : pendingAction === 'unpause' ? '确认继续运行容器' : pendingAction === 'remove' ? '确认删除容器' : '确认启动容器'" :description="selectedContainer ? `${selectedContainer.name} · ${selectedContainer.image}` : ''" size="small" @close="pendingAction = undefined; selectedContainer = undefined">
      <div class="confirm-content"><span class="confirm-content__icon" :class="{ 'is-danger': pendingAction === 'stop' || pendingAction === 'remove' }"><Trash2 v-if="pendingAction === 'remove'" :size="23" /><CircleStop v-else-if="pendingAction === 'stop'" :size="23" /><Pause v-else-if="pendingAction === 'pause'" :size="23" /><RotateCw v-else-if="pendingAction === 'restart'" :size="23" /><Play v-else :size="23" /></span><p>{{ pendingAction === 'remove' ? '将强制删除所选容器；镜像和存储卷保留。' : pendingAction === 'pause' ? '暂停后保留容器当前进程状态，可随时继续运行。' : 'Agent 会再次验证容器资源版本和实时状态。' }}</p></div>
      <template #footer><button class="button button--secondary" type="button" @click="pendingAction = undefined; selectedContainer = undefined">取消</button><button class="button" :class="pendingAction === 'stop' || pendingAction === 'remove' ? 'button--danger' : 'button--primary'" type="button" :disabled="actionRunning" @click="runAction"><LoaderCircle v-if="actionRunning" class="spin" :size="16" />{{ actionRunning ? '正在提交…' : '确认执行' }}</button></template>
    </ModalDialog>

    <ModalDialog :open="systemUpdatePending" title="确认更新 Docker 环境" description="将调用系统管理的发行版原生后台更新任务，Docker 软件包随系统软件包一起更新。" size="small" @close="systemUpdatePending = false">
      <div class="inline-alert inline-alert--warning">Docker Engine 可能短暂重启，KPanel 页面会暂时断开并在容器恢复后重新可用。</div>
      <template #footer><button class="button button--secondary" type="button" @click="systemUpdatePending = false">取消</button><button class="button button--primary" type="button" :disabled="systemUpdating" @click="submitSystemUpdate"><LoaderCircle v-if="systemUpdating" class="spin" :size="16" />提交后台更新</button></template>
    </ModalDialog>

    <ModalDialog :open="uninstallNoticeOpen" title="Docker 卸载尚未支持" description="卸载 Docker 会同时终止 KPanel。当前版本还不能在面板离线后继续执行并回传结果。" size="small" @close="uninstallNoticeOpen = false">
      <div class="inline-alert inline-alert--warning">如需卸载，请通过 SSH 运行 `k docker`。这是尚未完成的离线任务能力，不是权限限制。</div>
      <p class="modal-copy">后续版本将直接复用 kejilion.sh 的卸载流程，并在 KPanel 停止后继续记录执行结果。</p>
      <template #footer><button class="button button--secondary" type="button" @click="uninstallNoticeOpen = false">我知道了</button></template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.docker-page { gap: 18px; }
.docker-job { display: grid; grid-template-columns: auto minmax(0, 1fr) minmax(160px, 28%); align-items: center; gap: 12px; }
.docker-job span { display: grid; gap: 3px; }
.docker-job small { color: var(--muted); }
.docker-job progress { width: 100%; }
.docker-command-center { overflow: hidden; border: 1px solid var(--border); border-radius: 16px; background: var(--surface); box-shadow: var(--shadow-sm); }
.docker-command-center__header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border-bottom: 1px solid var(--border); }
.docker-command-center__header > div { display: flex; align-items: center; gap: 11px; }
.docker-command-center__header > div > span:last-child { display: grid; gap: 3px; }
.docker-command-center__header small { color: var(--muted); }
.docker-command-center__actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.docker-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); border-bottom: 1px solid var(--border); }
.docker-summary > div { display: flex; align-items: center; gap: 11px; min-width: 0; padding: 13px 16px; border-right: 1px solid var(--border); }
.docker-summary > div:last-child { border-right: 0; }
.docker-summary > div > span:last-child { display: grid; gap: 2px; }
.docker-summary strong { font-size: 1.12rem; }
.docker-summary small { color: var(--muted); font-size: .73rem; }
.docker-nav { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 4px; padding: 8px; border-bottom: 1px solid var(--border); background: color-mix(in srgb, var(--surface-raised) 62%, transparent); }
.docker-nav button { min-width: 0; min-height: 40px; border: 1px solid transparent; border-radius: 9px; background: transparent; color: var(--text); padding: 7px 11px; display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 8px; text-align: left; cursor: pointer; transition: background-color .16s ease, border-color .16s ease; }
.docker-nav button:hover { border-color: color-mix(in srgb, var(--brand) 38%, var(--border)); background: var(--surface); }
.docker-nav button.is-active { border-color: color-mix(in srgb, var(--brand) 34%, var(--border)); color: var(--brand); background: var(--surface); box-shadow: var(--shadow-sm); }
.docker-nav button > svg { color: var(--brand); }
.docker-nav small { color: var(--muted); }
.docker-toolbar { display: grid; grid-template-columns: 1fr minmax(220px, 320px) 130px; gap: 10px; align-items: center; padding: 10px 12px; }
.docker-toolbar__count { padding-left: 4px; color: var(--muted); font-size: .78rem; }
.docker-sort { width: 100%; height: 39px; min-height: 39px; border-radius: 10px; }
.workspace-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.workspace-grid--environment { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.workspace-card { min-width: 0; border: 1px solid var(--border); border-radius: 16px; background: var(--surface); padding: 18px; display: grid; align-content: start; gap: 16px; }
.workspace-card--wide { grid-column: 1 / -1; }
.workspace-card--danger { border-color: color-mix(in srgb, var(--danger) 35%, var(--border)); }
.workspace-card > header, .resource-section__header, .form-section > header { display: flex; justify-content: space-between; align-items: flex-start; gap: 14px; }
.workspace-card > header > div, .resource-section__header > div:first-child, .form-section > header > div { display: grid; gap: 3px; }
.workspace-card > header:not(.resource-section__header) { grid-template-columns: auto 1fr; justify-content: start; }
.workspace-card > header small, .resource-section__header small, .form-section small, .card-note { color: var(--muted); }
.workspace-card__icon { width: 38px; height: 38px; border-radius: 11px; display: grid; place-items: center; color: var(--brand); background: color-mix(in srgb, var(--brand) 10%, transparent); flex: 0 0 auto; }
.action-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.action-card { border: 1px solid var(--border); border-radius: 13px; padding: 14px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.action-card > div { display: grid; gap: 4px; }
.action-card small { color: var(--muted); line-height: 1.45; }
.card-actions, .row-actions { display: flex; align-items: center; gap: 8px; }
.workspace-card > header > .card-actions,
.resource-section__header > .card-actions { display: flex; flex: 0 0 auto; flex-wrap: nowrap; }
.card-actions .button { flex: 0 0 auto; white-space: nowrap; }
.row-actions--wrap { flex-wrap: wrap; }
.backup-list { display: grid; gap: 8px; }
.backup-list article { display: flex; justify-content: space-between; gap: 12px; align-items: center; border: 1px solid var(--border); border-radius: 12px; padding: 12px; }
.backup-list article > div { display: grid; gap: 3px; }
.backup-list article > div:last-child { display: flex; grid-auto-flow: column; gap: 7px; }
.backup-list small { color: var(--muted); }
.card-note { margin: 0; font-size: .82rem; line-height: 1.55; }
.divider { height: 1px; background: var(--border); }
.check-row { display: flex; gap: 10px; align-items: flex-start; }
.check-row span { display: grid; gap: 3px; }
.resource-section { padding: 0; overflow: hidden; }
.resource-section__header { min-height: 76px; padding: 18px; border-bottom: 1px solid var(--border); align-items: center; }
.resource-section__header > div:first-child { grid-template-columns: auto minmax(0, 1fr); align-items: center; flex: 1 1 auto; min-width: 0; }
.resource-section__header > div:first-child > div { display: grid; min-width: 0; gap: 3px; }
.resource-section__header > .card-actions { margin-left: auto; justify-content: flex-end; }
.resource-section .table-scroll, .resource-section > .empty-state { margin: 0; }
.docker-table { min-width: 1240px; }
.docker-table__name { width: 20%; }
.docker-table__status { width: 11%; }
.docker-table__ports { width: 23%; }
.docker-table__network { width: 13%; }
.docker-table__owner { width: 11%; }
.docker-table__actions { width: 372px; }
.docker-table > thead th:last-child,
.docker-table .docker-row > td:last-child {
  width: 372px;
  min-width: 372px;
  max-width: 372px;
  padding-right: 8px;
  padding-left: 8px;
}
.docker-row-actions { display: flex; align-items: center; gap: 7px; white-space: nowrap; }
.docker-row-actions__group { display: inline-flex; align-items: center; gap: 4px; padding: 3px; border: 1px solid var(--border); border-radius: 10px; background: color-mix(in srgb, var(--surface-raised) 70%, transparent); }
.docker-row-actions__group .icon-button { width: 32px; height: 32px; border: 0; border-radius: 7px; background: transparent; }
.docker-row-actions__group .icon-button:hover { background: var(--surface); }
.docker-row { transition: background-color .14s ease; }
.docker-row:hover { background: color-mix(in srgb, var(--brand) 4%, var(--surface)); }
.docker-row--running > td:first-child { box-shadow: inset 3px 0 0 color-mix(in srgb, var(--docker-group-accent, var(--brand)) 75%, transparent); }
.docker-row--restarting > td:first-child, .docker-row--paused > td:first-child { box-shadow: inset 3px 0 0 color-mix(in srgb, var(--amber) 75%, transparent); }
.docker-group + .docker-group .docker-group__row td { border-top: 8px solid var(--surface-subtle); }
.docker-group__row td { padding: 0; background: color-mix(in srgb, var(--docker-group-accent, var(--brand)) 6%, var(--surface-raised)); box-shadow: inset 3px 0 0 color-mix(in srgb, var(--docker-group-accent, var(--brand)) 72%, transparent); }
.docker-group__summary { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 54px; padding: 8px 12px; }
.docker-group__toggle { display: grid; min-width: 0; align-items: center; grid-template-columns: auto auto minmax(0, 1fr); gap: 9px; padding: 0; border: 0; outline: 0; color: inherit; background: transparent; font: inherit; text-align: left; cursor: pointer; }
.docker-group__toggle > svg { color: color-mix(in srgb, var(--docker-group-accent, var(--brand)) 72%, var(--muted)); transition: transform .12s ease-out, color .12s ease-out; }
.docker-group__toggle > svg.is-expanded { transform: rotate(90deg); }
.docker-group__toggle:hover > svg { color: var(--docker-group-accent, var(--brand)); }
.docker-group__toggle:focus-visible { border-radius: 10px; box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 14%, transparent); }
.docker-group__copy { display: grid; min-width: 0; gap: 2px; }
.docker-group__summary small { overflow: hidden; color: var(--muted); font-size: .72rem; text-overflow: ellipsis; white-space: nowrap; }
.docker-group__icon { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 10px; color: var(--docker-group-accent, var(--brand)); background: color-mix(in srgb, var(--docker-group-accent, var(--brand)) 12%, transparent); }
.docker-group-row-enter-active,
.docker-group-row-leave-active { will-change: opacity, transform; transition: opacity .12s linear, transform .12s cubic-bezier(.2, .8, .2, 1); }
.docker-group-row-enter-from,
.docker-group-row-leave-to { opacity: 0; transform: translate3d(0, -3px, 0); }
.docker-context-menu {
  position: fixed;
  z-index: 110;
  display: grid;
  width: 218px;
  max-height: calc(100vh - 16px);
  overflow-y: auto;
  padding: 6px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  box-shadow: var(--shadow-md);
}
.docker-context-menu__title {
  overflow: hidden;
  padding: 8px 10px 7px;
  color: var(--text-muted);
  font-size: .78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.docker-context-menu button {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 8px;
  color: var(--text);
  text-align: left;
  background: transparent;
  cursor: pointer;
}
.docker-context-menu button:hover { background: var(--surface-subtle); }
.docker-context-menu button:disabled { opacity: .42; cursor: not-allowed; }
.docker-context-menu button.danger-link { color: var(--danger); }
.docker-context-menu hr {
  width: 100%;
  margin: 4px 0;
  border: 0;
  border-top: 1px solid var(--border);
}
.network-membership { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(180px, 1fr) auto auto; gap: 8px; padding: 14px 18px; border-bottom: 1px solid var(--border); background: color-mix(in srgb, var(--surface-raised) 70%, transparent); }
.text-input,
.select-input { width: 100%; min-width: 0; height: 42px; border: 1px solid var(--border-strong); border-radius: 11px; background-color: var(--surface-subtle); color: var(--text); padding: 0 12px; box-shadow: inset 0 1px 0 color-mix(in srgb, var(--surface) 72%, transparent); font: inherit; transition: border-color .16s ease, background-color .16s ease, box-shadow .16s ease; }
.text-input::placeholder { color: color-mix(in srgb, var(--muted) 72%, transparent); }
.text-input:hover:not(:disabled),
.select-input:hover:not(:disabled) { border-color: color-mix(in srgb, var(--brand) 38%, var(--border-strong)); background-color: var(--surface); }
.text-input:focus,
.select-input:focus { outline: none; border-color: var(--brand); background-color: var(--surface); box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 13%, transparent), inset 0 1px 0 color-mix(in srgb, var(--surface) 78%, transparent); }
.text-input:disabled,
.select-input:disabled { opacity: .55; cursor: not-allowed; }
.select-input { appearance: none; padding-right: 38px; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%23677c76' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m7 10 5 5 5-5'/%3E%3C/svg%3E"); background-repeat: no-repeat; background-position: right 12px center; background-size: 16px; cursor: pointer; }
.select-input option { background: var(--surface); color: var(--text); }
.text-input[list]::-webkit-calendar-picker-indicator { width: 15px; height: 15px; margin: 0; opacity: .42; cursor: pointer; transition: opacity .16s ease; }
.text-input[list]:hover::-webkit-calendar-picker-indicator { opacity: .68; }
.inline-check input[type='checkbox'],
.check-row input[type='checkbox'] { width: 18px; height: 18px; flex: 0 0 auto; margin: 0; border: 1px solid var(--border-strong); border-radius: 5px; accent-color: var(--brand); }
.compact-input { width: min(240px, 34vw); }
.compact-input--driver { width: min(170px, 24vw); }
.table-sub { display: block; color: var(--muted); margin-top: 3px; }
.action-unavailable-label { font-size: .78rem; color: var(--muted); }
.form-grid { display: grid; gap: 14px; }
.form-grid--two { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.field { display: grid; gap: 7px; }
.field > span { font-weight: 650; }
.field > small { color: var(--muted); line-height: 1.45; }
.text-area { width: 100%; resize: vertical; min-height: 84px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface); color: var(--text); padding: 10px 12px; font: inherit; }
.text-area:focus { outline: 2px solid color-mix(in srgb, var(--brand) 25%, transparent); border-color: var(--brand); }
.deployment-input-card { display: grid; gap: 11px; padding: 15px; border: 1px solid var(--border-strong); border-radius: 15px; background: color-mix(in srgb, var(--surface-raised) 78%, transparent); }
.deployment-detection { display: flex; min-width: 0; align-items: center; gap: 10px; color: var(--muted); }
.deployment-detection small { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.deployment-detection.is-ready { color: var(--text); }
.deployment-detection.is-invalid { color: var(--danger); }
.deployment-kind { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 6px; font-size: .82rem; font-weight: 750; }
.deployment-options { display: flex; align-items: center; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.deployment-advanced { margin-top: 14px; padding: 15px; border: 1px solid var(--border); border-radius: 14px; background: color-mix(in srgb, var(--surface-raised) 62%, transparent); }
.deployment-advanced > .form-section:first-child { margin-top: 0; }
.compose-environment-card { display: grid; gap: 12px; }
.compose-environment-card--manager { padding: 13px 14px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface-subtle); }
.compose-environment-card__header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.compose-environment-card__header > div:first-child { display: grid; min-width: 0; gap: 3px; }
.compose-environment-card__header small { color: var(--muted); }
.compose-environment-card__header code { color: inherit; }
.compose-environment-card__actions { display: flex; flex: 0 0 auto; gap: 6px; }
.compose-environment-card__body { display: grid; gap: 9px; padding-top: 11px; border-top: 1px solid var(--border); }
.compose-environment-card__body > small { color: var(--muted); }
.compose-environment-card__missing, .compose-environment-card__required { color: var(--danger) !important; }
.compose-environment-card__required, .compose-environment-card__default { align-self: center; font-size: .72rem; white-space: nowrap; }
.compose-environment-card__default { color: var(--muted); }
.compose-environment-card__add { justify-self: start; }
.compose-environment-card__concealed { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--muted); font-size: .8rem; }
.compose-environment-card__editor { min-height: 128px; font-family: var(--font-mono); font-size: .78rem; line-height: 1.55; }
.compose-manager { display: grid; gap: 15px; }
.compose-manager__meta { display: grid; grid-template-columns: minmax(0, 1.35fr) minmax(0, 1fr); gap: 10px; }
.compose-manager__meta > span { display: grid; min-width: 0; gap: 5px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 11px; background: var(--surface-subtle); }
.compose-manager__meta small { color: var(--muted); }
.compose-manager__meta code, .compose-manager__meta strong { overflow: hidden; font-size: .78rem; text-overflow: ellipsis; white-space: nowrap; }
.form-section { display: grid; gap: 10px; margin-top: 18px; }
.repeat-row { display: grid; gap: 8px; align-items: center; }
.repeat-row--ports { grid-template-columns: minmax(100px, 1fr) auto minmax(100px, 1fr) 100px 110px auto; }
.repeat-row--mounts { grid-template-columns: 120px minmax(180px, 1fr) minmax(150px, 1fr) auto auto; }
.repeat-row--environment { grid-template-columns: minmax(150px, .7fr) minmax(180px, 1.3fr) auto; }
.repeat-row--compose-environment { grid-template-columns: minmax(150px, .7fr) minmax(180px, 1.3fr) auto auto; }
.inline-check { display: flex; align-items: center; gap: 6px; white-space: nowrap; }
.log-viewer { margin: 0; min-height: 280px; max-height: 58vh; overflow: auto; border: 1px solid var(--terminal-shell-border, #29383a); border-radius: var(--terminal-shell-radius, 12px); background: var(--terminal-shell-background, #0b1214); color: var(--terminal-shell-text, #d8dddc); box-shadow: var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%)); padding: 15px; font: 12.5px/1.65 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.stats-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.stats-grid article { border: 1px solid var(--border); border-radius: 13px; padding: 14px; display: grid; gap: 5px; }
.stats-grid small, .stats-grid span { color: var(--muted); }
.stats-grid strong { font-size: 1.3rem; }
.stats-grid .stats-time { font-size: .9rem; line-height: 1.4; }
.console-command { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; }
.console-command > span { font-family: ui-monospace, monospace; color: var(--brand); font-weight: 700; }
.console-output { min-height: 240px; margin-top: 14px; }
.modal-copy { color: var(--muted); line-height: 1.65; }
@media (max-width: 1000px) {
  .docker-nav { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .docker-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .docker-summary > div:nth-child(2) { border-right: 0; }
  .docker-summary > div:nth-child(-n+2) { border-bottom: 1px solid var(--border); }
  .action-grid { grid-template-columns: 1fr; }
  .stats-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .resource-section__header { align-items: stretch; flex-direction: column; }
  .resource-section__header > .card-actions { width: 100%; margin-left: 0; justify-content: flex-start; flex-wrap: wrap; }
}
@media (max-width: 720px) {
  .docker-job { grid-template-columns: auto 1fr; }
  .docker-job progress { grid-column: 1 / -1; }
  .docker-nav { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .docker-nav button:last-child:nth-child(odd) { grid-column: 1 / -1; }
  .docker-command-center__header { align-items: stretch; flex-direction: column; padding: 12px; }
  .docker-command-center__actions { width: 100%; justify-content: space-between; }
  .docker-summary > div { gap: 8px; padding: 10px; }
  .docker-toolbar { grid-template-columns: 1fr; }
  .docker-toolbar__count { display: none; }
  .workspace-grid, .workspace-grid--environment, .form-grid--two { grid-template-columns: 1fr; }
  .docker-toolbar, .resource-section__header, .backup-list article { align-items: stretch; flex-direction: column; }
  .workspace-card > header > .card-actions,
  .resource-section__header > .card-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); width: 100%; }
  .workspace-card { gap: 13px; padding: 14px; border-radius: 14px; }
  .workspace-card > header:not(.resource-section__header) { display: grid; align-items: stretch; grid-template-columns: 1fr; }
  .workspace-card > header:not(.resource-section__header) > .card-actions { grid-column: 1; }
  .resource-section__header { min-height: 0; padding: 14px; }
  .backup-list article { gap: 9px; padding: 11px; }
  .compact-input { width: 100%; }
  .network-membership { grid-template-columns: 1fr; }
  .docker-group__summary { grid-template-columns: minmax(0, 1fr); }
  .docker-group__summary .button { grid-column: 1 / -1; width: 100%; }
  .compose-manager__meta { grid-template-columns: 1fr; }
  .repeat-row--ports, .repeat-row--mounts, .repeat-row--environment, .repeat-row--compose-environment { grid-template-columns: 1fr; }
  .compose-environment-card__header, .compose-environment-card__concealed { align-items: stretch; flex-direction: column; }
  .compose-environment-card__actions { flex-wrap: wrap; }
  .repeat-row--ports > span { display: none; }
  .deployment-input-card, .deployment-advanced { padding: 12px; }
  .deployment-detection { align-items: flex-start; flex-direction: column; gap: 4px; }
  .deployment-detection small { white-space: normal; }
  .deployment-options { justify-content: stretch; flex-direction: column; }
  .deployment-options .button { width: 100%; }
}
@media (prefers-reduced-motion: reduce) {
  .docker-group__toggle > svg,
  .docker-group-row-enter-active,
  .docker-group-row-leave-active { transition: none; }
}
</style>
