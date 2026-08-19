<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ClusterView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ClusterView/zh-TW').then((module) => module.default))
import {
  ArrowUpRight,
  Check,
  Copy,
  Gauge,
  GripVertical,
  KeyRound,
  LayoutGrid,
  LayoutList,
  LoaderCircle,
  MemoryStick,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Server,
  Share2,
  ShieldCheck,
  Trash2,
} from '@lucide/vue'
import PageHeader from '@/components/common/PageHeader.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import CountryFlagIcon from '@/components/overview/CountryFlagIcon.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { ApiError, api } from '@/lib/api'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import {
  clampPercent,
  formatBytes,
  formatDateTime,
  formatDuration,
  formatPercent,
  formatRate,
  relativeTime,
} from '@/lib/format'
import { useToast } from '@/stores/toast'
import type {
  ClusterController,
  ClusterHost,
  ClusterHostList,
  ClusterLightEnrollment,
  ClusterPairingCode,
  ClusterShareSettings,
} from '@/types/api'

const toast = useToast()
const windowActive = inject(desktopWindowActiveKey, computed(() => true))
const inventory = ref<ClusterHostList>()
const loading = ref(true)
const refreshing = ref(false)
const refreshWarning = ref('')
const loadError = ref('')
const search = ref('')
const addOpen = ref(false)
const accessOpen = ref(false)
const manageOpen = ref(false)
const shareOpen = ref(false)
const adding = ref(false)
const saving = ref(false)
const deleting = ref(false)
const enablingMutualFiles = ref(false)
const generatingCode = ref(false)
const generatingLightEnrollment = ref(false)
const controllersLoading = ref(false)
const shareLoading = ref(false)
const shareSaving = ref(false)
const shareResetting = ref(false)
const pairingCode = ref<ClusterPairingCode>()
const lightEnrollment = ref<ClusterLightEnrollment>()
const controllers = ref<ClusterController[]>([])
const shareSettings = ref<ClusterShareSettings>()
const selected = ref<ClusterHost>()
const addAccessInput = ref<HTMLTextAreaElement>()
const addForm = reactive({ name: '', accessCredential: '' })
const shareForm = reactive({ enabled: false, title: '', description: '' })
const editName = ref('')
const originError = ref('')
type HostViewMode = 'list' | 'card'
const hostViewModeStorageKey = 'kpanel:cluster-host-view'
const hostOrderStorageKey = 'kpanel:cluster-host-order'
const viewMode = ref<HostViewMode>('list')
const hostOrder = ref<string[]>([])
const draggedHostId = ref('')
const dragOverHostId = ref('')
let loadInFlight = false
let loadController: AbortController | undefined
let pollTimer: number | undefined
const delayedRefreshes = new Set<number>()

type OriginSecurityMode = 'empty' | 'tls' | 'e2e_http' | 'invalid'

interface OriginSecurityAssessment {
  mode: OriginSecurityMode
  message: string
}

interface ClusterAccessCredential {
  origin: string
  pairingCode: string
}

const clusterAccessCredentialPrefix = 'KPANEL_CLUSTER_ACCESS_V1'

const parsedAccessCredential = computed(() =>
  parseClusterAccessCredential(addForm.accessCredential),
)
const originAssessment = computed<OriginSecurityAssessment>(() =>
  assessOriginSecurity(parsedAccessCredential.value?.origin || ''),
)
const panelOrigin = computed(() =>
  typeof window === 'undefined' ? '' : window.location.origin,
)
const accessCredentialText = computed(() =>
  pairingCode.value && panelOrigin.value
    ? formatClusterAccessCredential(panelOrigin.value, pairingCode.value.code)
    : '',
)
const shareURL = computed(() =>
  shareSettings.value?.sharePath && typeof window !== 'undefined'
    ? `${window.location.origin}${shareSettings.value.sharePath}`
    : '',
)

const hostOperatingSystemIdentity = (host: ClusterHost) =>
  detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)

const orderedHosts = computed(() => {
  const items = inventory.value?.items || []
  const positions = new Map(hostOrder.value.map((id, index) => [id, index]))
  return items
    .map((host, originalIndex) => ({ host, originalIndex }))
    .sort((left, right) => {
      const leftPosition = positions.get(left.host.id)
      const rightPosition = positions.get(right.host.id)
      if (leftPosition === undefined && rightPosition === undefined) {
        return left.originalIndex - right.originalIndex
      }
      if (leftPosition === undefined) return 1
      if (rightPosition === undefined) return -1
      return leftPosition - rightPosition
    })
    .map(({ host }) => host)
})

const filteredHosts = computed(() => {
  const term = search.value.trim().toLocaleLowerCase()
  if (!term) return orderedHosts.value
  return orderedHosts.value.filter((host) => {
    const telemetry = host.lastSnapshot?.telemetry
    return [
      host.name,
      host.origin,
      host.isLocal ? '本机 当前面板' : '',
      telemetry?.hostname,
      telemetry?.os,
      telemetry?.publicNetwork?.country,
      telemetry?.publicNetwork?.city,
      telemetry?.publicNetwork?.isp,
    ]
      .filter(Boolean)
      .some((value) => String(value).toLocaleLowerCase().includes(term))
  })
})

const onlineCount = computed(
  () => inventory.value?.items.filter((item) => item.state === 'online').length || 0,
)
const attentionCount = computed(
  () =>
    inventory.value?.items.filter((item) => !['online', 'unknown'].includes(item.state)).length ||
    0,
)

function friendlyError(reason: unknown, fallback: string): string {
  if (!(reason instanceof ApiError)) return fallback
  const messages: Record<string, string> = {
    cluster_origin_invalid: '请输入 HTTPS 根地址，或 http://IP:端口。不能包含路径、参数或账号信息。',
    cluster_light_https_required: '轻量节点需要可从被控机访问的 HTTPS 根地址。请先通过 k fd 为 KPanel 绑定域名后重试。',
    cluster_origin_blocked: '该地址被网络安全策略拒绝；私网地址需由部署管理员加入 CIDR 白名单。',
    cluster_pairing_failed: '授权码无效、已过期或已被使用，请在目标 KPanel 重新生成。',
    cluster_duplicate: '该 KPanel 已经添加到主机列表。',
    cluster_host_limit: '已达到 100 台主机上限。',
    cluster_remote_tls_error: '目标 KPanel 的 HTTPS 证书校验失败。',
    cluster_remote_authentication_failed: '加密响应校验失败，连接已拒绝。',
    cluster_mutual_files_unsupported: '目标 KPanel 版本不支持双向文件互传，请先升级目标面板后重试。',
    federation_incompatible: '目标 KPanel 不支持当前加密直连协议，请更新目标面板后重试。',
    federation_identity_changed: '目标主机加密身份发生变化，已停止连接；确认服务器未被替换后请重新配对。',
    cluster_remote_unreachable: '暂时无法连接目标 KPanel，请检查域名、证书和网络。',
    cluster_resource_changed: '主机信息已变化，请刷新后重试。',
    cluster_share_changed: '分享设置已在其他页面变化，请重新打开后再保存。',
  }
  return messages[reason.code] || reason.message || fallback
}

function assessOriginSecurity(raw: string): OriginSecurityAssessment {
  const value = raw.trim()
  if (!value) {
    return {
      mode: 'empty',
      message: '支持证书 HTTPS；无域名时可使用 http://IP:端口，集群数据由 KPanel 端到端加密。',
    }
  }
  if (value.length > 512 || /[\r\n\t\u0000]/.test(value)) {
    return { mode: 'invalid', message: '主机 URL 格式无效。' }
  }

  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    return { mode: 'invalid', message: '请输入完整的主机 URL。' }
  }
  const authorityAndSuffix = value.slice(value.indexOf('//') + 2)
  const hasSuffix =
    authorityAndSuffix.includes('/') ||
    parsed.search !== '' ||
    parsed.hash !== '' ||
    parsed.username !== '' ||
    parsed.password !== ''
  if (hasSuffix) {
    return { mode: 'invalid', message: '主机 URL 只能填写根地址，不能包含路径、参数或账号信息。' }
  }

  const hostname = parsed.hostname.replace(/^\[|\]$/g, '')
  if (value.startsWith('https://') && validHTTPSHost(hostname)) {
    return { mode: 'tls', message: 'HTTPS：验证目标证书并使用 TLS 加密。' }
  }
  if (
    value.startsWith('http://') &&
    parsed.port !== '' &&
    parsed.port !== '80' &&
    isLiteralIPAddress(hostname)
  ) {
    return {
      mode: 'e2e_http',
      message: 'IP 加密直连：集群数据端到端加密；浏览器打开管理页面仍是普通 HTTP。',
    }
  }
  if (value.startsWith('http://')) {
    return {
      mode: 'invalid',
      message: 'HTTP 加密直连仅支持明确的 IP 和非 80 端口，例如 http://203.0.113.10:8080。',
    }
  }
  return {
    mode: 'invalid',
    message: '请输入 HTTPS 根地址，或 http://IP:端口。',
  }
}

function validHTTPSHost(hostname: string): boolean {
  if (isLiteralIPAddress(hostname)) return true
  if (
    hostname.length < 1 ||
    hostname.length > 253 ||
    !hostname.includes('.') ||
    !/^[a-z0-9.-]+$/i.test(hostname)
  ) {
    return false
  }
  return hostname.split('.').every(
    (label) =>
      label.length > 0 &&
      label.length <= 63 &&
      !label.startsWith('-') &&
      !label.endsWith('-'),
  )
}

function isLiteralIPAddress(hostname: string): boolean {
  if (hostname.includes(':')) return /^[0-9a-f:.]+$/i.test(hostname)
  const parts = hostname.split('.')
  return (
    parts.length === 4 &&
    parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)
  )
}

async function load(silent = false): Promise<void> {
  if (loadInFlight) return
  loadInFlight = true
  if (!silent && !inventory.value) loading.value = true
  else refreshing.value = true
  loadController = new AbortController()
  try {
    inventory.value = await api.cluster.hosts(loadController.signal)
    reconcileHostOrder(inventory.value.items)
    loadError.value = ''
    refreshWarning.value = ''
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    const message = friendlyError(reason, '无法读取集群主机，请稍后重试。')
    if (inventory.value) refreshWarning.value = `${message} 当前保留上次成功数据。`
    else loadError.value = message
  } finally {
    loading.value = false
    refreshing.value = false
    loadInFlight = false
  }
}

function openAdd(): void {
  addOpen.value = true
  void nextTick(() => addAccessInput.value?.focus())
}

function closeAdd(): void {
  if (adding.value || generatingLightEnrollment.value) return
  addOpen.value = false
  addForm.name = ''
  addForm.accessCredential = ''
  originError.value = ''
  lightEnrollment.value = undefined
}

async function createLightEnrollment(): Promise<void> {
  if (generatingLightEnrollment.value) return
  generatingLightEnrollment.value = true
  try {
    lightEnrollment.value = await api.cluster.createLightEnrollment()
  } catch (reason) {
    toast.danger(
      '轻量节点命令生成失败',
      friendlyError(reason, '请确认当前 KPanel 已通过 HTTPS 域名访问；轻量节点不使用 HTTP 直连地址。'),
    )
  } finally {
    generatingLightEnrollment.value = false
  }
}

async function copyLightEnrollment(): Promise<void> {
  if (!lightEnrollment.value?.command) return
  await copyToClipboard(
    lightEnrollment.value.command,
    '轻量节点接入命令已复制',
    '请手动选择完整命令复制。',
  )
}

async function addHost(): Promise<void> {
  if (adding.value) return
  const accessCredential = parsedAccessCredential.value
  if (!accessCredential) {
    originError.value = '接入凭据格式无效，请完整粘贴目标 KPanel 生成的三行内容。'
    void nextTick(() => addAccessInput.value?.focus())
    return
  }
  if (originAssessment.value.mode === 'invalid') {
    originError.value = originAssessment.value.message
    void nextTick(() => addAccessInput.value?.focus())
    return
  }
  originError.value = ''
  adding.value = true
  try {
    const host = await api.cluster.add({
      name: addForm.name.trim() || undefined,
      origin: accessCredential.origin,
      pairingCode: accessCredential.pairingCode,
    })
    adding.value = false
    closeAdd()
    toast.success(
      '主机已加入集群',
      host.state === 'pairing'
        ? `${host.name} 的安全配对正在后台继续。`
        : host.mutualFileTransferAvailable
          ? `${host.name} 已完成配对，双向文件互传已自动启用。`
          : `${host.name} 已完成配对，当前保持单向文件读取；可在主机管理中启用，旧版 KPanel 需先升级。`,
    )
    await load(true)
  } catch (reason) {
    const message = friendlyError(reason, '请检查目标 KPanel 和授权码后重试。')
    if (
      reason instanceof ApiError &&
      ['cluster_origin_invalid', 'cluster_origin_blocked'].includes(reason.code)
    ) {
      originError.value = message
      void nextTick(() => addAccessInput.value?.focus())
    }
    toast.danger('添加主机失败', message)
  } finally {
    adding.value = false
  }
}

async function openAccess(): Promise<void> {
  accessOpen.value = true
  await loadControllers()
}

function closeAccess(): void {
  accessOpen.value = false
  pairingCode.value = undefined
}

async function loadControllers(): Promise<void> {
  controllersLoading.value = true
  try {
    controllers.value = (await api.cluster.controllers()).items
  } catch (reason) {
    toast.danger('无法读取授权列表', friendlyError(reason, '请稍后重试。'))
  } finally {
    controllersLoading.value = false
  }
}

async function createPairingCode(): Promise<void> {
  generatingCode.value = true
  try {
    pairingCode.value = await api.cluster.createPairingCode()
  } catch (reason) {
    toast.danger('授权码生成失败', friendlyError(reason, '请稍后重试。'))
  } finally {
    generatingCode.value = false
  }
}

function formatClusterAccessCredential(origin: string, code: string): string {
  return [clusterAccessCredentialPrefix, origin.trim(), code.trim()].join('\n')
}

function parseClusterAccessCredential(raw: string): ClusterAccessCredential | undefined {
  const lines = raw
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
  if (lines[0] === clusterAccessCredentialPrefix) lines.shift()
  if (lines.length !== 2) return undefined
  const [origin, pairingCode] = lines
  if (
    !origin ||
    origin.length > 512 ||
    !pairingCode ||
    pairingCode.length > 1024 ||
    /\s/.test(pairingCode)
  ) {
    return undefined
  }
  return { origin, pairingCode }
}

async function copyAccessCredential(): Promise<void> {
  if (!accessCredentialText.value) return
  await copyToClipboard(
    accessCredentialText.value,
    '接入凭据已复制',
    '请手动选择完整接入凭据复制。',
  )
}

async function openShare(): Promise<void> {
  shareOpen.value = true
  shareLoading.value = true
  try {
    const settings = await api.cluster.shareSettings()
    applyShareSettings(settings)
  } catch (reason) {
    toast.danger('分享设置读取失败', friendlyError(reason, '请稍后重试。'))
    shareOpen.value = false
  } finally {
    shareLoading.value = false
  }
}

function closeShare(): void {
  if (shareSaving.value || shareResetting.value) return
  shareOpen.value = false
}

function applyShareSettings(settings: ClusterShareSettings): void {
  shareSettings.value = settings
  shareForm.enabled = settings.enabled
  shareForm.title = settings.title
  shareForm.description = settings.description
}

async function saveShare(): Promise<void> {
  if (!shareSettings.value || shareSaving.value) return
  shareSaving.value = true
  try {
    const settings = await api.cluster.updateShare({
      enabled: shareForm.enabled,
      title: shareForm.title,
      description: shareForm.description,
      expectedResourceVersion: shareSettings.value.resourceVersion,
    })
    applyShareSettings(settings)
    toast.success(settings.enabled ? '公开分享已开启' : '公开分享已关闭')
  } catch (reason) {
    toast.danger('分享设置保存失败', friendlyError(reason, '请稍后重试。'))
  } finally {
    shareSaving.value = false
  }
}

async function resetShareLink(): Promise<void> {
  if (!shareSettings.value || shareResetting.value) return
  if (!window.confirm('重置公开链接？旧链接会立即失效。')) return
  shareResetting.value = true
  try {
    const settings = await api.cluster.resetShareToken(shareSettings.value.resourceVersion)
    applyShareSettings(settings)
    toast.success('公开链接已重置', '旧链接已经失效。')
  } catch (reason) {
    toast.danger('公开链接重置失败', friendlyError(reason, '请稍后重试。'))
  } finally {
    shareResetting.value = false
  }
}

async function copyShareLink(): Promise<void> {
  if (!shareURL.value) return
  await copyToClipboard(shareURL.value, '公开链接已复制', '请手动选择完整链接复制。')
}

function previewShare(): void {
  if (shareURL.value) window.open(shareURL.value, '_blank', 'noopener,noreferrer')
}

async function copyToClipboard(value: string, success: string, fallback: string): Promise<void> {
  if (await writeClipboardText(value)) {
    toast.success(success)
    return
  }
  toast.danger('复制失败', fallback)
}

async function writeClipboardText(value: string): Promise<boolean> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return true
    } catch {
      // HTTP IP、WebView 或浏览器拒绝剪贴板权限时，继续使用兼容复制。
    }
  }
  return legacyCopyText(value)
}

function legacyCopyText(value: string): boolean {
  if (
    typeof document === 'undefined' ||
    typeof document.createElement !== 'function' ||
    typeof document.execCommand !== 'function'
  ) {
    return false
  }

  const input = document.createElement('textarea')
  input.value = value
  input.readOnly = true
  input.setAttribute('aria-hidden', 'true')
  input.style.position = 'fixed'
  input.style.inset = '0 auto auto -9999px'
  input.style.opacity = '0'
  document.body.appendChild(input)
  try {
    input.focus()
    input.select()
    input.setSelectionRange(0, input.value.length)
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    input.remove()
  }
}

function setViewMode(mode: HostViewMode): void {
  viewMode.value = mode
  try {
    window.localStorage.setItem(hostViewModeStorageKey, mode)
  } catch {
    // 浏览器禁止持久化时仍保留本次页面选择。
  }
}

function restoreViewMode(): void {
  try {
    const stored = window.localStorage.getItem(hostViewModeStorageKey)
    if (stored === 'list' || stored === 'card') viewMode.value = stored
  } catch {
    // 隐私模式或存储被禁用时使用默认行列表。
  }
}

function persistHostOrder(): void {
  try {
    window.localStorage.setItem(hostOrderStorageKey, JSON.stringify(hostOrder.value))
  } catch {
    // 隐私模式或存储被禁用时仍保留本次页面顺序。
  }
}

function restoreHostOrder(): void {
  try {
    const stored = JSON.parse(window.localStorage.getItem(hostOrderStorageKey) || '[]')
    if (
      Array.isArray(stored) &&
      stored.length <= 101 &&
      stored.every((id) => typeof id === 'string' && id.length > 0 && id.length <= 128)
    ) {
      hostOrder.value = [...new Set(stored)]
    }
  } catch {
    hostOrder.value = []
  }
}

function reconcileHostOrder(items: ClusterHost[]): void {
  const validIDs = new Set(items.map((host) => host.id))
  const next = hostOrder.value.filter((id) => validIDs.has(id))
  for (const host of items) {
    if (!next.includes(host.id)) next.push(host.id)
  }
  if (
    next.length !== hostOrder.value.length ||
    next.some((id, index) => id !== hostOrder.value[index])
  ) {
    hostOrder.value = next
    persistHostOrder()
  }
}

function moveHost(hostID: string, offset: number): void {
  if (search.value.trim()) return
  const ids = orderedHosts.value.map((host) => host.id)
  const current = ids.indexOf(hostID)
  const target = Math.max(0, Math.min(ids.length - 1, current + offset))
  if (current < 0 || current === target) return
  ids.splice(current, 1)
  ids.splice(target, 0, hostID)
  hostOrder.value = ids
  persistHostOrder()
}

function startHostDrag(event: DragEvent, hostID: string): void {
  if (search.value.trim()) {
    event.preventDefault()
    return
  }
  draggedHostId.value = hostID
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', hostID)
  }
}

function dropHost(targetID: string): void {
  const sourceID = draggedHostId.value
  dragOverHostId.value = ''
  draggedHostId.value = ''
  if (!sourceID || sourceID === targetID || search.value.trim()) return
  const ids = orderedHosts.value.map((host) => host.id)
  const source = ids.indexOf(sourceID)
  const target = ids.indexOf(targetID)
  if (source < 0 || target < 0) return
  ids.splice(source, 1)
  ids.splice(target, 0, sourceID)
  hostOrder.value = ids
  persistHostOrder()
}

function finishHostDrag(): void {
  draggedHostId.value = ''
  dragOverHostId.value = ''
}

async function revokeController(controller: ClusterController): Promise<void> {
  if (!window.confirm(`撤销 ${controller.name || controller.fingerprint} 的访问授权？`)) return
  try {
    await api.cluster.revokeController(controller.id)
    controllers.value = controllers.value.filter((item) => item.id !== controller.id)
    toast.success('控制端授权已撤销')
  } catch (reason) {
    toast.danger('撤销失败', friendlyError(reason, '请刷新后重试。'))
  }
}

function openManage(host: ClusterHost): void {
  selected.value = host
  editName.value = host.name
  manageOpen.value = true
}

function closeManage(): void {
  if (saving.value || deleting.value || enablingMutualFiles.value) return
  manageOpen.value = false
  selected.value = undefined
}

function mutualFilesHostEligible(host: ClusterHost): boolean {
  return !host.isLocal
    && host.kind !== 'light_node'
    && host.federationProtocol === 'v2'
    && !['pairing', 'revoking'].includes(host.state)
    && host.fileTransferAvailable === true
    && host.scope.split(/\s+/).includes('cluster.files.read')
}

async function enableMutualFiles(): Promise<void> {
  const host = selected.value
  if (
    !host
    || enablingMutualFiles.value
    || !mutualFilesHostEligible(host)
  ) return
  const refreshing = host.mutualFileTransferAvailable
  enablingMutualFiles.value = true
  try {
    const updated = await api.cluster.enableMutualFiles(host.id)
    upsertHost(updated)
    if (selected.value?.id === host.id) selected.value = updated
    toast.success(
      refreshing ? '双向文件互传连接已刷新' : '双向文件互传已启用',
      refreshing
        ? `${updated.name} 已使用当前 KPanel 地址刷新互传连接。`
        : `${updated.name} 现在可以与当前 KPanel 互相复制文件。`,
    )
  } catch (reason) {
    toast.danger(
      refreshing ? '刷新双向文件互传连接失败' : '启用双向文件互传失败',
      friendlyError(reason, '请确认双方 KPanel 在线后重试。'),
    )
  } finally {
    enablingMutualFiles.value = false
  }
}

async function saveName(): Promise<void> {
  const host = selected.value
  if (!host || saving.value || enablingMutualFiles.value || !editName.value.trim()) return
  saving.value = true
  try {
    const updated = await api.cluster.rename(host.id, {
      name: editName.value.trim(),
      expectedResourceVersion: host.resourceVersion,
    })
    upsertHost(updated)
    selected.value = updated
    toast.success('主机名称已更新')
  } catch (reason) {
    toast.danger('保存失败', friendlyError(reason, '请刷新后重试。'))
  } finally {
    saving.value = false
  }
}

async function removeHost(): Promise<void> {
  const host = selected.value
  if (!host || host.isLocal || deleting.value || enablingMutualFiles.value) return
  if (!window.confirm(`从当前 KPanel 移除 ${host.name}？目标主机业务不会受到影响。`)) return
  deleting.value = true
  try {
    const result = await api.cluster.remove(host.id, host.resourceVersion)
    if (inventory.value) {
      inventory.value.items = inventory.value.items.filter((item) => item.id !== host.id)
      inventory.value.total = inventory.value.items.length
    }
    deleting.value = false
    closeManage()
    if (result.credentialRemoved === false) {
      toast.danger('主机已移除，但凭据清理失败', '请检查 KPanel 数据目录权限；服务重启时会再次清理孤立凭据。')
    } else {
      toast.success(
        '主机已移除',
        result.remoteRevoked ? '远端授权也已撤销。' : '远端不可达；可稍后在目标 KPanel 撤销残留授权。',
      )
    }
  } catch (reason) {
    toast.danger('移除失败', friendlyError(reason, '请刷新后重试。'))
    await load(true)
    const fresh = inventory.value?.items.find((item) => item.id === host.id)
    if (fresh) {
      selected.value = fresh
      editName.value = fresh.name
    } else {
      manageOpen.value = false
      selected.value = undefined
    }
  } finally {
    deleting.value = false
  }
}

async function refreshHost(host: ClusterHost): Promise<void> {
  if (host.polling) return
  try {
    const queued = await api.cluster.refresh(host.id)
    upsertHost(queued)
    const timer = window.setTimeout(async () => {
      delayedRefreshes.delete(timer)
      await load(true)
    }, 1_200)
    delayedRefreshes.add(timer)
  } catch (reason) {
    toast.danger('刷新失败', friendlyError(reason, '请稍后重试。'))
  }
}

function upsertHost(host: ClusterHost): void {
  if (!inventory.value) return
  const index = inventory.value.items.findIndex((item) => item.id === host.id)
  if (index >= 0) inventory.value.items[index] = host
  else inventory.value.items.unshift(host)
  inventory.value.total = inventory.value.items.length
}

function openPanel(host: ClusterHost): void {
  if (host.kind === 'light_node') return
  if (
    !host.isLocal &&
    host.transportSecurity === 'e2e_http' &&
    !window.confirm(
      '集群监控数据已端到端加密，但这个管理页面仍通过普通 HTTP 打开，登录密码和 Session 不受加密直连保护。建议先为目标面板配置 HTTPS。仍然打开？',
    )
  ) {
    return
  }
  window.open(panelURL(host), '_blank', 'noopener,noreferrer')
}

function displayOrigin(host: ClusterHost): string {
  if (host.kind === 'light_node') return ''
  return host.isLocal ? window.location.origin : host.origin
}

const securityEntrancePathPattern = /^[a-z0-9](?:[a-z0-9-]{4,46}[a-z0-9])$/

function panelURL(host: ClusterHost): string {
  const origin = displayOrigin(host)
  if (
    !origin ||
    host.isLocal ||
    !host.securityEntrancePath ||
    !securityEntrancePathPattern.test(host.securityEntrancePath)
  ) {
    return origin
  }
  return `${origin}/${host.securityEntrancePath}`
}

function transportSecurityLabel(host: ClusterHost): string {
  if (host.isLocal) return '本机 Agent'
  if (host.kind === 'light_node') return '轻量节点'
  return host.transportSecurity === 'e2e_http' ? '加密直连' : 'HTTPS'
}

function transportSecurityDescription(host: ClusterHost): string {
  if (host.isLocal) return '本机通过 Unix Socket 读取 Agent 摘要'
  if (host.kind === 'light_node') return '低权限 Agent 主动通过 HTTPS 上报只读主机摘要'
  if (host.transportSecurity === 'e2e_http') {
    return '集群监控数据端到端加密；普通浏览器管理页面仍是 HTTP'
  }
  return '验证目标证书并通过 TLS 加密集群连接'
}

function shortFingerprint(value?: string): string {
  if (!value || value.length <= 26) return value || ''
  return `${value.slice(0, 16)}…${value.slice(-8)}`
}

function onVisibilityChange(): void {
  if (!document.hidden && windowActive.value) void load(true)
}

onMounted(() => {
  restoreViewMode()
  restoreHostOrder()
  if (windowActive.value) {
    void load()
    pollTimer = window.setInterval(() => {
      if (!document.hidden && windowActive.value) void load(true)
    }, 15_000)
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
})

watch(windowActive, (active) => {
  if (!active) {
    loadController?.abort()
    if (pollTimer) window.clearInterval(pollTimer)
    pollTimer = undefined
    return
  }
  void load(Boolean(inventory.value))
  if (!pollTimer) {
    pollTimer = window.setInterval(() => {
      if (!document.hidden && windowActive.value) void load(true)
    }, 15_000)
  }
})

onBeforeUnmount(() => {
  loadController?.abort()
  if (pollTimer) window.clearInterval(pollTimer)
  delayedRefreshes.forEach((timer) => window.clearTimeout(timer))
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="page cluster-page">
    <PageHeader
      title="集群监控"
      description="在一个视图中查看本机与已接入节点；管理接入关系，或打开对应 KPanel 继续操作。"
    />

    <section class="cluster-hero" aria-label="集群概况与操作">
      <div class="cluster-stats">
        <div><strong>{{ inventory?.total || 0 }}</strong><span>全部节点</span></div>
        <div><strong>{{ onlineCount }}</strong><span>在线</span></div>
        <div><strong>{{ attentionCount }}</strong><span>需关注</span></div>
        <div><strong>{{ inventory?.maxHosts || 100 }}</strong><span>远程上限</span></div>
      </div>
      <div class="cluster-hero__actions">
        <button
          class="icon-button icon-button--small"
          type="button"
          :disabled="refreshing"
          title="刷新集群状态"
          aria-label="刷新集群状态"
          @click="load(true)"
        >
          <RefreshCw :size="15" :class="{ spin: refreshing }" />
        </button>
        <button class="button button--secondary button--small" type="button" @click="openShare">
          <Share2 :size="15" /> 公开分享
        </button>
        <button class="button button--secondary button--small" type="button" @click="openAccess">
          <KeyRound :size="15" /> 接入授权
        </button>
        <button class="button button--primary button--small" type="button" @click="openAdd">
          <Plus :size="15" /> 添加主机
        </button>
      </div>
    </section>

    <div v-if="refreshWarning" class="inline-alert inline-alert--warning" role="status">
      {{ refreshWarning }}
    </div>

    <div v-if="inventory?.items.length" class="cluster-toolbar">
      <label class="cluster-search">
        <Server :size="17" />
        <input
          v-model="search"
          type="search"
          aria-label="搜索集群主机"
          placeholder="搜索名称、系统、地区或运营商…"
        />
      </label>
      <div class="cluster-view-switch" role="group" aria-label="主机排列方式">
        <button
          type="button"
          :class="{ 'is-active': viewMode === 'list' }"
          :aria-pressed="viewMode === 'list'"
          title="行列表"
          @click="setViewMode('list')"
        >
          <LayoutList :size="15" /> 列表
        </button>
        <button
          type="button"
          :class="{ 'is-active': viewMode === 'card' }"
          :aria-pressed="viewMode === 'card'"
          title="卡片排列"
          @click="setViewMode('card')"
        >
          <LayoutGrid :size="15" /> 卡片
        </button>
      </div>
    </div>

    <LoadingState v-if="loading" title="正在读取集群主机…" />
    <ErrorState v-else-if="loadError && !inventory" :message="loadError" @retry="load()" />
    <EmptyState
      v-else-if="!inventory?.items.length"
      title="尚未添加主机"
      description="先在目标 KPanel 生成一次性授权码，再将它添加到当前面板。"
    >
      <button class="button button--primary" type="button" @click="openAdd">
        <Plus :size="16" /> 添加第一台主机
      </button>
    </EmptyState>
    <EmptyState
      v-else-if="!filteredHosts.length"
      title="没有匹配的主机"
      description="请清除搜索词后重试。"
    />

    <section
      v-else
      class="cluster-grid"
      :class="`is-${viewMode}`"
      :aria-busy="refreshing"
      :aria-label="viewMode === 'list' ? '集群主机行列表' : '集群主机卡片列表'"
    >
      <article
        v-for="host in filteredHosts"
        :key="host.id"
        class="cluster-card"
        :class="{ 'is-drag-over': dragOverHostId === host.id }"
        @dragenter.prevent="dragOverHostId = host.id"
        @dragover.prevent
        @drop.prevent="dropHost(host.id)"
      >
        <header class="cluster-card__header">
          <button
            class="cluster-card__drag"
            type="button"
            :draggable="!search.trim()"
            :disabled="Boolean(search.trim())"
            :title="search.trim() ? '清除搜索后可调整顺序' : '拖拽调整顺序；也可使用上下方向键'"
            :aria-label="`调整 ${host.name} 的显示顺序`"
            @dragstart="startHostDrag($event, host.id)"
            @dragend="finishHostDrag"
            @keydown.up.prevent="moveHost(host.id, -1)"
            @keydown.down.prevent="moveHost(host.id, 1)"
          >
            <GripVertical :size="15" />
          </button>
          <OperatingSystemIcon
            :distro="hostOperatingSystemIdentity(host).key"
            :label="hostOperatingSystemIdentity(host).label"
          />
          <div>
            <span>
              <strong>{{ host.name }}</strong>
            <em v-if="host.isLocal" class="cluster-card__local">本机</em>
              <em v-else-if="host.kind === 'light_node'" class="cluster-card__transport is-light_node">
                轻量节点
              </em>
              <em
                v-else
                class="cluster-card__transport"
                :class="`is-${host.transportSecurity}`"
                :title="transportSecurityDescription(host)"
              >
                {{ transportSecurityLabel(host) }}
              </em>
              <StatusBadge :status="host.state" subtle />
            </span>
            <a
              v-if="host.kind !== 'light_node' && (host.isLocal || host.transportSecurity === 'tls')"
              class="cluster-card__origin"
              :href="panelURL(host)"
              target="_blank"
              rel="noopener noreferrer"
            >
              {{ displayOrigin(host) }} <ArrowUpRight :size="12" />
            </a>
            <button
              v-else-if="host.kind !== 'light_node'"
              class="cluster-card__origin"
              type="button"
              :title="transportSecurityDescription(host)"
              @click="openPanel(host)"
            >
              {{ displayOrigin(host) }} <ArrowUpRight :size="12" />
            </button>
            <small
              v-if="!host.isLocal && host.peerFingerprint"
              class="cluster-card__fingerprint"
              :title="host.peerFingerprint"
            >
              身份指纹 {{ shortFingerprint(host.peerFingerprint) }}
            </small>
            <small v-else-if="host.kind === 'light_node'" class="cluster-card__fingerprint">
              只读摘要 · 主动 HTTPS 上报
            </small>
          </div>
          <button
            class="icon-button icon-button--small"
            type="button"
            :disabled="host.polling"
            :aria-label="`刷新 ${host.name}`"
            @click="refreshHost(host)"
          >
            <LoaderCircle v-if="host.polling" class="spin" :size="15" />
            <RefreshCw v-else :size="15" />
          </button>
        </header>

        <div v-if="host.lastSnapshot" class="cluster-card__metrics">
          <div>
            <span><Gauge :size="14" /> CPU</span>
            <strong>{{ formatPercent(host.lastSnapshot.telemetry.cpu.usagePercent) }}</strong>
            <i
              role="progressbar"
              aria-label="CPU 使用率"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-valuenow="clampPercent(host.lastSnapshot.telemetry.cpu.usagePercent)"
            >
              <b :style="{ width: `${clampPercent(host.lastSnapshot.telemetry.cpu.usagePercent)}%` }" />
            </i>
            <small>{{ host.lastSnapshot.telemetry.cpu.cores }} 核</small>
          </div>
          <div>
            <span><MemoryStick :size="14" /> 内存</span>
            <strong>{{ formatPercent(host.lastSnapshot.telemetry.memory.usagePercent) }}</strong>
            <i
              role="progressbar"
              aria-label="内存使用率"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-valuenow="clampPercent(host.lastSnapshot.telemetry.memory.usagePercent)"
            >
              <b :style="{ width: `${clampPercent(host.lastSnapshot.telemetry.memory.usagePercent)}%` }" />
            </i>
            <small>
              {{ formatBytes(host.lastSnapshot.telemetry.memory.usedBytes) }} /
              {{ formatBytes(host.lastSnapshot.telemetry.memory.totalBytes) }}
            </small>
          </div>
          <div>
            <span><Server :size="14" /> 磁盘</span>
            <strong>{{ formatPercent(host.lastSnapshot.telemetry.disk.usagePercent) }}</strong>
            <i
              role="progressbar"
              aria-label="磁盘使用率"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-valuenow="clampPercent(host.lastSnapshot.telemetry.disk.usagePercent)"
            >
              <b :style="{ width: `${clampPercent(host.lastSnapshot.telemetry.disk.usagePercent)}%` }" />
            </i>
            <small>
              {{ formatBytes(host.lastSnapshot.telemetry.disk.usedBytes) }} /
              {{ formatBytes(host.lastSnapshot.telemetry.disk.totalBytes) }}
            </small>
          </div>
        </div>

        <div v-if="host.lastSnapshot" class="cluster-card__details">
          <div>
            <span>系统</span>
            <strong>{{ host.lastSnapshot.telemetry.os }}</strong>
            <small>{{ host.lastSnapshot.telemetry.architecture }} · {{ host.lastSnapshot.telemetry.kernel }}</small>
          </div>
          <div>
            <span>地区</span>
            <strong>
              <CountryFlagIcon
                v-if="host.lastSnapshot.telemetry.publicNetwork.countryCode"
                :country-code="host.lastSnapshot.telemetry.publicNetwork.countryCode"
                :label="host.lastSnapshot.telemetry.publicNetwork.country || '地区'"
              />
              {{
                [
                  host.lastSnapshot.telemetry.publicNetwork.country,
                  host.lastSnapshot.telemetry.publicNetwork.city,
                ].filter(Boolean).join(' · ') || '未获取'
              }}
            </strong>
            <small>{{ host.lastSnapshot.telemetry.publicNetwork.isp || '运营商未知' }}</small>
          </div>
          <div>
            <span>网络</span>
            <strong>↓ {{ formatRate(host.lastSnapshot.receiveBytesPerSecond) }}</strong>
            <small>↑ {{ formatRate(host.lastSnapshot.transmitBytesPerSecond) }}</small>
          </div>
          <div>
            <span>运行时间</span>
            <strong>{{ formatDuration(host.lastSnapshot.telemetry.uptimeSeconds) }}</strong>
            <small>延迟 {{ host.lastSnapshot.latencyMilliseconds }} ms</small>
          </div>
        </div>
        <div v-else class="cluster-card__empty">
          <ShieldCheck :size="20" />
          <div>
            <strong>等待首次主机摘要</strong>
            <small>{{ host.lastError || '配对已完成，后端正在安全轮询。' }}</small>
          </div>
        </div>

        <div v-if="host.lastError" class="cluster-card__warning" role="status">
          {{ host.lastError }}
        </div>

        <footer class="cluster-card__footer">
          <span>
            最近在线 {{ relativeTime(host.lastSuccessAt) }}
            <small v-if="host.lastSnapshot">采集于 {{ formatDateTime(host.lastSnapshot.receivedAt) }}</small>
          </span>
          <div>
            <button
              class="button button--ghost button--small"
              type="button"
              @click="openManage(host)"
            >
              <Pencil :size="14" /> 管理
            </button>
            <button
              v-if="host.kind !== 'light_node'"
              class="button button--primary button--small"
              type="button"
              @click="openPanel(host)"
            >
              {{ host.isLocal ? '当前面板' : '打开面板' }} <ArrowUpRight :size="14" />
            </button>
          </div>
        </footer>
      </article>
    </section>

    <ModalDialog
      :open="shareOpen"
      title="公开分享"
      description="生成匿名只读页面，向其他人展示当前集群的机器状态。"
      size="medium"
      @close="closeShare"
    >
      <LoadingState v-if="shareLoading" title="正在读取分享设置…" />
      <form v-else id="cluster-share-form" class="cluster-share" @submit.prevent="saveShare">
        <label class="cluster-share__switch">
          <span>
            <strong>启用公开分享</strong>
            <small>默认关闭；关闭后现有链接立即返回 404。</small>
          </span>
          <input v-model="shareForm.enabled" type="checkbox" role="switch" />
        </label>

        <label class="field">
          展示标题
          <input v-model="shareForm.title" maxlength="80" placeholder="我的 KPanel 集群" />
        </label>
        <label class="field">
          一句话介绍（可选）
          <input
            v-model="shareForm.description"
            maxlength="240"
            placeholder="例如：这些是我正在运行的服务器。"
          />
        </label>

        <section class="cluster-share__privacy">
          <ShieldCheck :size="19" />
          <span>
            <strong>公开字段经过白名单过滤</strong>
            <small>仅展示名称、状态、地区、系统和资源使用情况；不公开 IP、面板地址、节点 ID、身份指纹、错误详情、版本或管理入口。</small>
          </span>
        </section>

        <section v-if="shareURL" class="cluster-share__link">
          <span>
            <strong>{{ shareSettings?.enabled ? '当前公开链接' : '已暂停的公开链接' }}</strong>
            <small>{{ shareSettings?.enabled ? '任何获得链接的人都可以查看。' : '保存并开启后，此链接恢复访问。' }}</small>
          </span>
          <pre>{{ shareURL }}</pre>
          <div>
            <button class="button button--secondary button--small" type="button" @click="copyShareLink">
              <Copy :size="14" /> 复制链接
            </button>
            <button class="button button--secondary button--small" type="button" @click="previewShare">
              <ArrowUpRight :size="14" /> 预览
            </button>
            <button
              class="button button--ghost button--small"
              type="button"
              :disabled="shareResetting"
              @click="resetShareLink"
            >
              <LoaderCircle v-if="shareResetting" class="spin" :size="14" />
              <RotateCcw v-else :size="14" /> 重置链接
            </button>
          </div>
        </section>
        <p v-else class="cluster-share__hint">首次开启并保存后生成随机公开链接。</p>
      </form>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="shareSaving" @click="closeShare">取消</button>
        <button
          class="button button--primary"
          type="submit"
          form="cluster-share-form"
          :disabled="shareLoading || shareSaving"
        >
          <LoaderCircle v-if="shareSaving" class="spin" :size="16" />
          <Share2 v-else :size="16" />
          {{ shareSaving ? '正在保存…' : '保存分享设置' }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="addOpen"
      title="添加 KPanel 主机"
      description="在目标 KPanel 的“集群 → 接入授权”复制接入凭据，然后在此整段粘贴。"
      size="small"
      @close="closeAdd"
    >
      <form
        id="cluster-add-form"
        class="form-stack"
        autocomplete="off"
        data-form-type="other"
        @submit.prevent="addHost"
      >
        <label class="field">
          主机名称（可选）
          <input
            v-model="addForm.name"
            name="cluster-host-label"
            maxlength="80"
            placeholder="例如：香港生产机"
            autocomplete="off"
            data-1p-ignore
            data-lpignore="true"
          />
          <small>留空时使用目标主机名。</small>
        </label>
        <label class="field">
          接入凭据
          <textarea
            ref="addAccessInput"
            v-model="addForm.accessCredential"
            name="cluster-access-credential"
            rows="4"
            required
            maxlength="1600"
            placeholder="在目标 KPanel 一键复制，然后完整粘贴到这里"
            autocomplete="one-time-code"
            autocapitalize="off"
            spellcheck="false"
            data-1p-ignore
            data-lpignore="true"
            data-bwignore
            aria-describedby="cluster-origin-help"
            :aria-invalid="originError || originAssessment.mode === 'invalid' ? 'true' : undefined"
            @input="originError = ''"
          />
          <small
            id="cluster-origin-help"
            class="cluster-origin-help"
            :class="{
              'is-danger': originError || originAssessment.mode === 'invalid',
              'is-secure':
                !originError && ['tls', 'e2e_http'].includes(originAssessment.mode),
            }"
            :role="originError || originAssessment.mode === 'invalid' ? 'alert' : undefined"
          >
            {{
              originError ||
              (parsedAccessCredential
                ? originAssessment.message
                : '凭据同时包含主机 URL 与一次性授权码，不会保存到浏览器或审计日志。')
            }}
          </small>
        </label>
        <section class="cluster-light-enrollment">
          <div>
            <Server :size="17" />
            <span>
              <strong>非面板 Linux 主机</strong>
              <small>无需 Docker；生成命令后，在目标机以 root 执行即可加入只读监控。</small>
            </span>
            <button
              v-if="!lightEnrollment"
              class="button button--secondary button--small"
              type="button"
              :disabled="generatingLightEnrollment"
              @click="createLightEnrollment"
            >
              <LoaderCircle v-if="generatingLightEnrollment" class="spin" :size="14" />
              <Plus v-else :size="14" /> 生成接入命令
            </button>
          </div>
          <div v-if="lightEnrollment" class="cluster-light-enrollment__command">
            <pre>{{ lightEnrollment.command }}</pre>
            <button class="button button--secondary button--small" type="button" @click="copyLightEnrollment">
              <Copy :size="14" /> 复制命令
            </button>
            <button
              class="button button--ghost button--small"
              type="button"
              :disabled="generatingLightEnrollment"
              @click="createLightEnrollment"
            >
              <LoaderCircle v-if="generatingLightEnrollment" class="spin" :size="14" />
              <RefreshCw v-else :size="14" /> 重新生成
            </button>
            <small>一次性命令，{{ formatDateTime(lightEnrollment.expiresAt) }} 前有效。</small>
          </div>
        </section>
      </form>
      <template #footer>
        <button class="button button--secondary" type="button" :disabled="adding" @click="closeAdd">取消</button>
        <button
          class="button button--primary"
          type="submit"
          form="cluster-add-form"
          :disabled="
            adding ||
            !parsedAccessCredential ||
            originAssessment.mode === 'invalid'
          "
        >
          <LoaderCircle v-if="adding" class="spin" :size="16" />
          <Plus v-else :size="16" />
          {{ adding ? '正在安全配对…' : '添加主机' }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="accessOpen"
      title="本机接入授权"
      description="其他 KPanel 可按授权范围读取摘要、打开终端和浏览文件；授权可随时撤销。"
      size="medium"
      @close="closeAccess"
    >
      <div class="cluster-access">
        <section class="cluster-access__code">
          <div>
            <KeyRound :size="20" />
            <span>
              <strong>本机接入凭据</strong>
              <small>同时包含当前主机 URL 与一次性授权码；5 分钟内只能使用一次，权限包含摘要读取、终端和文件只读访问。</small>
            </span>
          </div>
          <button
            v-if="!pairingCode"
            class="button button--primary"
            type="button"
            :disabled="generatingCode"
            @click="createPairingCode"
          >
            <LoaderCircle v-if="generatingCode" class="spin" :size="16" />
            <KeyRound v-else :size="16" /> 生成接入凭据
          </button>
          <div v-else class="cluster-access__token">
            <pre>{{ accessCredentialText }}</pre>
            <button
              class="button button--secondary button--small"
              type="button"
              aria-label="复制完整接入凭据"
              @click="copyAccessCredential"
            >
              <Copy :size="14" /> 复制接入凭据
            </button>
            <small>到期时间：{{ formatDateTime(pairingCode.expiresAt) }}</small>
          </div>
        </section>

        <section class="cluster-access__controllers">
          <header>
            <div>
              <strong>已授权控制端</strong>
              <small>这里只列出可读取本机概要的 KPanel，不包含任何远程管理权限。</small>
            </div>
            <button
              class="icon-button icon-button--small"
              type="button"
              aria-label="刷新已授权控制端"
              :disabled="controllersLoading"
              @click="loadControllers"
            >
              <RefreshCw :size="14" :class="{ spin: controllersLoading }" />
            </button>
          </header>
          <p v-if="controllersLoading && !controllers.length">正在读取授权列表…</p>
          <p v-else-if="!controllers.length">暂无已授权控制端。</p>
          <template v-else>
            <article v-for="controller in controllers" :key="controller.id">
              <span>
                <strong>{{ controller.name || '未命名 KPanel' }}</strong>
                <code>{{ controller.fingerprint }}</code>
                <small>
                  授权于 {{ formatDateTime(controller.createdAt) }} · 最近访问
                  {{ relativeTime(controller.lastSeenAt) }}
                </small>
              </span>
              <button
                class="icon-button icon-button--small icon-button--danger"
                type="button"
                :aria-label="`撤销 ${controller.name || '控制端'} 授权`"
                @click="revokeController(controller)"
              >
                <Trash2 :size="14" />
              </button>
            </article>
          </template>
        </section>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="manageOpen"
      :title="selected ? `管理 ${selected.name}` : '管理主机'"
      :description="
        selected?.isLocal
          ? '修改本机在集群列表中的显示名称；本机节点始终保留。'
          : '修改仅影响当前中心端显示；移除不会停止目标主机业务。'
      "
      size="small"
      @close="closeManage"
    >
      <div v-if="selected" class="form-stack">
        <label class="field">
          显示名称
          <input v-model="editName" maxlength="80" autocomplete="off" />
        </label>
        <div class="cluster-manage__identity">
          <template v-if="selected.kind !== 'light_node'">
            <span>目标地址</span><code>{{ displayOrigin(selected) }}</code>
          </template>
          <span>连接方式</span><code>{{ transportSecurityLabel(selected) }}</code>
          <template v-if="selected.peerFingerprint">
            <span>身份指纹</span><code>{{ selected.peerFingerprint }}</code>
          </template>
          <span>节点 ID</span><code>{{ selected.remoteNodeId }}</code>
          <span>{{ selected.kind === 'light_node' ? '节点程序' : 'Panel / Agent' }}</span>
          <code v-if="selected.kind === 'light_node'">{{ selected.panelVersion || selected.lastSnapshot?.telemetry.agentVersion || '未知' }}</code>
          <code v-else>{{ selected.panelVersion || '未知' }} / {{ selected.lastSnapshot?.telemetry.agentVersion || '未知' }}</code>
          <template v-if="mutualFilesHostEligible(selected)">
            <span>文件互传</span>
            <div
              v-if="selected.mutualFileTransferAvailable"
              class="cluster-manage__mutual-controls"
            >
              <strong class="cluster-manage__mutual-state">
                <Check :size="14" /> 双向文件互传已启用
              </strong>
              <button
                class="button button--ghost button--small cluster-manage__mutual-button"
                type="button"
                :disabled="enablingMutualFiles || saving || deleting"
                @click="enableMutualFiles"
              >
                <LoaderCircle v-if="enablingMutualFiles" class="spin" :size="14" />
                <RefreshCw v-else :size="14" />
                {{ enablingMutualFiles ? '正在刷新…' : '刷新连接' }}
              </button>
            </div>
            <button
              v-else
              class="button button--secondary button--small cluster-manage__mutual-button"
              type="button"
              :disabled="enablingMutualFiles || saving || deleting"
              @click="enableMutualFiles"
            >
              <LoaderCircle v-if="enablingMutualFiles" class="spin" :size="14" />
              <ShieldCheck v-else :size="14" />
              {{ enablingMutualFiles ? '正在启用…' : '启用双向文件互传' }}
            </button>
          </template>
        </div>
      </div>
      <template #footer>
        <button
          v-if="selected && !selected.isLocal"
          class="button button--danger"
          type="button"
          :disabled="saving || deleting || enablingMutualFiles"
          @click="removeHost"
        >
          <LoaderCircle v-if="deleting" class="spin" :size="16" />
          <Trash2 v-else :size="16" /> 移除主机
        </button>
        <button class="button button--secondary" type="button" :disabled="saving || deleting || enablingMutualFiles" @click="closeManage">关闭</button>
        <button
          class="button button--primary"
          type="button"
          :disabled="saving || deleting || enablingMutualFiles || !editName.trim()"
          @click="saveName"
        >
          <LoaderCircle v-if="saving" class="spin" :size="16" />
          <Check v-else :size="16" /> 保存名称
        </button>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.cluster-page {
  --cluster-accent: #6d5dfc;
  align-content: start;
  gap: 18px;
}

.cluster-hero {
  position: relative;
  display: grid;
  overflow: hidden;
  grid-template-columns: max-content minmax(0, 1fr);
  align-items: center;
  isolation: isolate;
  gap: 20px;
  padding: 12px 14px;
  background:
    radial-gradient(circle at 86% 0%, color-mix(in srgb, var(--cluster-accent) 10%, transparent), transparent 34%),
    linear-gradient(110deg, color-mix(in srgb, var(--cluster-accent) 6%, var(--surface)) 0%, var(--surface) 58%);
  border: 1px solid color-mix(in srgb, var(--cluster-accent) 22%, var(--border));
  border-radius: 15px;
  box-shadow: var(--shadow-sm);
}

.cluster-hero::before {
  position: absolute;
  z-index: 0;
  top: 50%;
  right: 42px;
  width: 190px;
  height: 190px;
  border: 30px solid color-mix(in srgb, var(--cluster-accent) 7%, transparent);
  border-radius: 50%;
  content: '';
  pointer-events: none;
  transform: translateY(-50%);
}

.cluster-hero > * {
  position: relative;
  z-index: 1;
}

.cluster-hero__actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: nowrap;
  justify-self: end;
  justify-content: flex-end;
  gap: 8px;
  white-space: nowrap;
}

.cluster-stats {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(4, minmax(116px, 132px));
}

.cluster-stats div {
  display: grid;
  min-height: 48px;
  padding: 5px 14px;
  place-content: center;
  text-align: center;
  border-left: 1px solid var(--border);
}

.cluster-stats div:first-child {
  border-left: 0;
}

.cluster-stats strong {
  font-size: 19px;
}

.cluster-stats span {
  margin-top: 2px;
  color: var(--muted);
  font-size: 11px;
}

.cluster-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.cluster-search {
  display: flex;
  width: min(520px, 100%);
  height: 42px;
  align-items: center;
  gap: 9px;
  padding: 0 13px;
  color: var(--muted);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.cluster-search:focus-within {
  border-color: var(--brand);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 12%, transparent);
}

.cluster-search input {
  width: 100%;
  color: var(--text);
  background: transparent;
  border: 0;
  outline: 0;
}

.cluster-view-switch {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 3px;
  padding: 3px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.cluster-view-switch button {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  color: var(--muted);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: calc(var(--radius-sm) - 3px);
  font-size: 11px;
  font-weight: 700;
}

.cluster-view-switch button:hover {
  color: var(--text);
  background: var(--surface-muted);
}

.cluster-view-switch button.is-active {
  color: var(--brand);
  background: var(--brand-soft);
}

.cluster-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.cluster-grid.is-list {
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
}

.cluster-card {
  display: grid;
  min-width: 0;
  overflow: hidden;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
}

.cluster-card.is-drag-over {
  border-color: color-mix(in srgb, var(--brand) 56%, var(--border));
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 14%, transparent);
}

.cluster-grid.is-list .cluster-card {
  grid-template-columns:
    minmax(260px, 1.4fr)
    minmax(270px, 1.1fr)
    minmax(360px, 1.5fr)
    minmax(210px, 0.8fr);
  grid-template-areas:
    "header metrics details footer"
    "warning warning warning warning";
  align-items: stretch;
}

.cluster-grid.is-list .cluster-card__header {
  grid-area: header;
  border-right: 1px solid var(--border);
  border-bottom: 0;
}

.cluster-grid.is-list .cluster-card__metrics {
  grid-area: metrics;
  border-right: 1px solid var(--border);
  border-bottom: 0;
}

.cluster-grid.is-list .cluster-card__metrics > div {
  place-content: center;
}

.cluster-grid.is-list .cluster-card__details {
  grid-area: details;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  align-content: center;
  border-right: 1px solid var(--border);
}

.cluster-grid.is-list .cluster-card__empty {
  grid-area: 1 / 2 / 2 / 4;
  min-height: 92px;
  border-right: 1px solid var(--border);
}

.cluster-grid.is-list .cluster-card__warning {
  grid-area: warning;
}

.cluster-grid.is-list .cluster-card__footer {
  grid-area: footer;
  align-items: stretch;
  justify-content: center;
  flex-direction: column;
  margin-top: 0;
  border-top: 0;
}

.cluster-grid.is-list .cluster-card__footer > div,
.cluster-grid.is-list .cluster-card__footer .button {
  width: 100%;
}

.cluster-grid.is-list .cluster-card__footer .button {
  justify-content: center;
}

.cluster-card__header {
  display: grid;
  grid-template-columns: auto auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border-bottom: 1px solid var(--border);
}

.cluster-card__drag {
  display: grid;
  width: 26px;
  height: 34px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  color: var(--text-tertiary);
  background: transparent;
  cursor: grab;
}

.cluster-card__drag:hover,
.cluster-card__drag:focus-visible {
  color: var(--brand);
  background: var(--brand-soft);
}

.cluster-card__drag:active {
  cursor: grabbing;
}

.cluster-card__drag:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.cluster-card__header > div {
  min-width: 0;
}

.cluster-card__header > div > span {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.cluster-card__header strong {
  overflow: hidden;
  font-size: 15px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-card__local,
.cluster-card__transport {
  flex: 0 0 auto;
  padding: 2px 7px;
  font-size: 10px;
  font-style: normal;
  font-weight: 800;
  border-radius: 999px;
}

.cluster-card__local,
.cluster-card__transport.is-tls {
  color: var(--brand);
  background: var(--brand-soft);
  border: 1px solid color-mix(in srgb, var(--brand) 22%, var(--border));
}

.cluster-card__transport.is-e2e_http {
  color: var(--blue);
  background: var(--blue-soft);
  border: 1px solid color-mix(in srgb, var(--blue) 22%, var(--border));
}

.cluster-card__transport.is-light_node {
  color: var(--brand);
  background: var(--brand-soft);
  border: 1px solid color-mix(in srgb, var(--brand) 22%, var(--border));
}

.cluster-card__origin {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  overflow: hidden;
  color: var(--muted);
  font-size: 11px;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-card__origin:is(button) {
  padding: 0;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.cluster-card__origin:hover {
  color: var(--brand);
}

.cluster-card__fingerprint {
  display: block;
  max-width: 100%;
  margin-top: 3px;
  overflow: hidden;
  color: var(--muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-origin-help.is-secure {
  color: var(--brand);
}

.cluster-origin-help.is-danger {
  color: var(--danger);
}

.cluster-light-enrollment {
  display: grid;
  gap: 10px;
  padding: 12px;
  color: var(--muted);
  background: color-mix(in srgb, var(--brand-soft) 42%, var(--surface));
  border: 1px solid color-mix(in srgb, var(--brand) 22%, var(--border));
  border-radius: var(--radius-md);
}

.cluster-light-enrollment > div:first-child {
  display: flex;
  align-items: center;
  gap: 9px;
}

.cluster-light-enrollment span {
  display: grid;
  flex: 1;
  min-width: 0;
  gap: 2px;
}

.cluster-light-enrollment strong {
  color: var(--text);
  font-size: 12px;
}

.cluster-light-enrollment small {
  font-size: 11px;
  line-height: 1.45;
}

.cluster-light-enrollment__command {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.cluster-light-enrollment__command pre {
  min-width: 0;
  margin: 0;
  padding: 9px 10px;
  overflow: auto;
  color: var(--brand);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 10px;
  line-height: 1.5;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  user-select: all;
}

.cluster-light-enrollment__command small {
  grid-column: 1 / -1;
}

.cluster-card__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  background: var(--border);
  border-bottom: 1px solid var(--border);
}

.cluster-card__metrics > div {
  display: grid;
  gap: 5px;
  padding: 12px;
  background: var(--surface);
}

.cluster-card__metrics span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--muted);
  font-size: 10px;
}

.cluster-card__metrics strong {
  font-size: 17px;
}

.cluster-card__metrics small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-card__metrics i {
  display: block;
  height: 3px;
  overflow: hidden;
  background: var(--surface-muted);
  border-radius: 99px;
}

.cluster-card__metrics b {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, var(--brand), #3bbfa3);
  border-radius: inherit;
}

.cluster-card__details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
  padding: 15px 16px;
}

.cluster-card__details > div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.cluster-card__details span {
  color: var(--muted);
  font-size: 10px;
}

.cluster-card__details strong {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  overflow-wrap: anywhere;
  font-size: 12px;
}

.cluster-card__details small {
  overflow: hidden;
  color: var(--muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-card__empty {
  display: flex;
  min-height: 138px;
  align-items: center;
  justify-content: center;
  gap: 11px;
  padding: 20px;
  color: var(--muted);
}

.cluster-card__empty div {
  display: grid;
  gap: 4px;
}

.cluster-card__empty small {
  max-width: 280px;
  line-height: 1.5;
}

.cluster-card__warning {
  padding: 9px 16px;
  color: var(--danger);
  background: var(--danger-soft);
  border-top: 1px solid color-mix(in srgb, var(--danger) 18%, transparent);
  font-size: 11px;
}

.cluster-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  margin-top: auto;
  border-top: 1px solid var(--border);
}

.cluster-card__footer > span {
  display: grid;
  color: var(--text-soft);
  font-size: 10px;
}

.cluster-card__footer small {
  color: var(--muted);
}

.cluster-card__footer > div {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
}

.cluster-access {
  display: grid;
  gap: 16px;
}

.cluster-access__code,
.cluster-access__controllers {
  padding: 16px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}

.cluster-access__code > div:first-child,
.cluster-access__controllers header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.cluster-access__code > div:first-child {
  justify-content: flex-start;
  margin-bottom: 14px;
}

.cluster-access__code span,
.cluster-access__controllers header div,
.cluster-access__controllers article span {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.cluster-access small {
  color: var(--muted);
  font-size: 11px;
  line-height: 1.5;
}

.cluster-access__token {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.cluster-access__token pre {
  min-width: 0;
  margin: 0;
  padding: 10px;
  overflow: auto;
  color: var(--brand);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 11px;
  line-height: 1.55;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  user-select: all;
}

.cluster-access__token small {
  grid-column: 1 / -1;
}

.cluster-access__controllers header {
  margin-bottom: 10px;
}

.cluster-access__controllers > p {
  margin: 12px 0 0;
  color: var(--muted);
  font-size: 12px;
}

.cluster-access__controllers article {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 11px 0;
  border-top: 1px solid var(--border);
}

.cluster-access__controllers code {
  overflow: hidden;
  color: var(--muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-manage__identity {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 8px 12px;
  padding: 13px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.cluster-manage__identity span {
  color: var(--muted);
  font-size: 11px;
}

.cluster-manage__identity code {
  overflow-wrap: anywhere;
  font-size: 11px;
}

.cluster-share {
  display: grid;
  gap: 15px;
}

.cluster-share__switch,
.cluster-share__privacy,
.cluster-share__link > span {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}

.cluster-share__switch {
  align-items: center;
  padding: 14px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}

.cluster-share__switch > span,
.cluster-share__privacy > span,
.cluster-share__link > span {
  display: grid;
  gap: 4px;
}

.cluster-share__switch small,
.cluster-share__privacy small,
.cluster-share__link small,
.cluster-share__hint {
  color: var(--muted);
  font-size: 11px;
  line-height: 1.5;
}

.cluster-share__switch input {
  width: 38px;
  height: 21px;
  flex: 0 0 auto;
  accent-color: var(--brand);
}

.cluster-share__privacy {
  justify-content: flex-start;
  padding: 13px;
  color: var(--success);
  background: color-mix(in srgb, var(--success) 8%, var(--surface));
  border: 1px solid color-mix(in srgb, var(--success) 18%, var(--border));
  border-radius: var(--radius-sm);
}

.cluster-share__privacy small {
  color: var(--text-soft);
}

.cluster-share__link {
  display: grid;
  gap: 10px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}

.cluster-share__link pre {
  padding: 10px;
  margin: 0;
  overflow: auto;
  color: var(--brand);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  user-select: all;
}

.cluster-share__link > div {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.cluster-share__hint {
  margin: 0;
  text-align: center;
}

.cluster-manage__mutual-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--brand);
  font-size: 12px;
}

.cluster-manage__mutual-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.cluster-manage__mutual-button {
  width: fit-content;
}

@media (max-width: 1240px) {
  .cluster-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .cluster-grid.is-list .cluster-card {
    grid-template-columns: minmax(280px, 1fr) minmax(240px, 0.8fr);
    grid-template-areas:
      "header footer"
      "metrics metrics"
      "details details"
      "warning warning";
  }

  .cluster-grid.is-list .cluster-card__header,
  .cluster-grid.is-list .cluster-card__metrics,
  .cluster-grid.is-list .cluster-card__details,
  .cluster-grid.is-list .cluster-card__empty {
    border-right: 0;
  }

  .cluster-grid.is-list .cluster-card__header {
    border-bottom: 1px solid var(--border);
  }

  .cluster-grid.is-list .cluster-card__footer {
    align-items: flex-end;
    border-bottom: 1px solid var(--border);
  }

  .cluster-grid.is-list .cluster-card__footer > div,
  .cluster-grid.is-list .cluster-card__footer .button {
    width: auto;
  }

  .cluster-grid.is-list .cluster-card__empty {
    grid-area: 2 / 1 / 4 / -1;
  }
}

@media (max-width: 680px) {
  .cluster-grid {
    grid-template-columns: 1fr;
  }

  .cluster-hero {
    grid-template-columns: minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
    border-radius: 14px;
  }

  .cluster-hero__actions {
    display: grid;
    grid-template-columns: 42px repeat(3, minmax(0, 1fr));
    width: 100%;
    gap: 6px;
  }

  .cluster-hero__actions > * {
    width: 100%;
  }

  .cluster-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .cluster-view-switch {
    align-self: stretch;
  }

  .cluster-view-switch button {
    flex: 1;
    justify-content: center;
  }

  .cluster-stats {
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .cluster-stats div:nth-child(3) {
    border-top: 1px solid var(--border);
    border-left: 0;
  }

  .cluster-stats div:nth-child(4) {
    border-top: 1px solid var(--border);
  }

  .cluster-card__footer {
    align-items: flex-start;
    flex-direction: column;
  }

  .cluster-card__footer > div,
  .cluster-card__footer .button {
    width: 100%;
  }

  .cluster-card__footer > div {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .cluster-card__header {
    grid-template-columns: 24px auto minmax(0, 1fr) 40px;
    gap: 8px;
    padding: 12px;
  }

  .cluster-card__metrics > div,
  .cluster-card__details {
    padding: 11px;
  }

  .cluster-grid.is-list .cluster-card {
    grid-template-columns: minmax(0, 1fr);
    grid-template-areas:
      "header"
      "metrics"
      "details"
      "footer"
      "warning";
  }

  .cluster-grid.is-list .cluster-card__footer {
    align-items: flex-start;
    border-top: 1px solid var(--border);
    border-bottom: 0;
  }

  .cluster-grid.is-list .cluster-card__footer > div,
  .cluster-grid.is-list .cluster-card__footer .button {
    width: 100%;
  }
}
</style>
