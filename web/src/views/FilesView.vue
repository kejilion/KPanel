<script setup lang="ts">
import { computed, defineAsyncComponent, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/FilesView/en-US').then((module) => module.default)
  : import('@/i18n/pages/FilesView/zh-TW').then((module) => module.default))
import {
  Archive,
  Check,
  ChevronDown,
  ChevronRight,
  ClipboardPaste,
  Code2,
  Copy,
  CircleAlert,
  Download,
  ExternalLink,
  Eye,
  File,
  Folder,
  FolderOpen,
  HardDrive,
  LayoutGrid,
  List,
  ListRestart,
  MoreHorizontal,
  Pencil,
  Pin,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Scissors,
  Search,
  Server,
  Share2,
  ShieldCheck,
  Trash2,
  Upload,
  WrapText,
  X,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import FileShareDialog from '@/components/files/FileShareDialog.vue'
import FileShareManagerDialog from '@/components/files/FileShareManagerDialog.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { ApiError, api } from '@/lib/api'
import {
  readClusterHostOrder,
  sortClusterHosts,
  subscribeClusterHostOrder,
} from '@/lib/clusterHostOrder'
import { clusterHostPanelURL } from '@/lib/clusterHostNavigation'
import {
  contextMenuFocusOrigin,
  type ContextMenuFocusOrigin,
  focusFirstContextMenuItem,
  moveContextMenuFocus,
  placeContextMenu,
  showContextMenuKeyboardFocus,
  showContextMenuPointerFocus,
} from '@/lib/contextMenu'
import {
  desktopCloseGuardCoordinator,
  desktopWindowActiveKey,
  desktopWindowCloseGuardKey,
} from '@/lib/desktopRouteKeys'
import type { CodeLanguage } from '@/lib/code-editor-language'
import { transferCrossPanelFileBatch } from '@/lib/crossPanelFileTransfer'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import {
  collectExternalDrop,
  DesktopExternalDropError,
  uploadExternalDrop,
  type ExternalDropManifest,
} from '@/lib/desktopExternalDrop'
import { fileEntryIcon as entryIcon, fileEntryIconKind as entryIconKind } from '@/lib/fileEntryPresentation'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { downloadFileEntries } from '@/lib/fileDownloads'
import {
  addFileEntriesToDesktop,
  beginDesktopFileDrag,
  clearDesktopFileDrag,
  crossPanelFileDragEntries,
  desktopFileDragOrigin,
  desktopFileDragEntries,
  DesktopShortcutLimitError,
  hasCrossPanelFileDrag,
  hasDesktopFileDrag,
  nativeArchiveDownloadName,
  peekDesktopFileDragEntries,
  peekDesktopFileDragSourceNodeId,
} from '@/lib/desktopFileShortcuts'
import {
  changedFileDirectories,
  fileTransferOperation,
  fileTransferTargetError,
  notifyFileDirectoriesChanged,
  remapMovedFilePath,
  subscribeFileDirectoryChanges,
  successfulFileMoves,
  syncMovedDesktopShortcuts,
  useFileClipboard,
  type FileTransferOperation,
} from '@/lib/fileWindowTransfer'
import { useToast } from '@/stores/toast'
import type {
  ClusterHost,
  ClusterHostList,
  FileActionInput,
  FileActionResult,
  FileDirectory,
  FileEntry,
  FileRemoteDownloadJob,
  FileRemoteDownloadJobState,
  FileTrashEntry,
} from '@/types/api'

const CodeEditor = defineAsyncComponent(() => import('@/components/files/CodeEditor.vue'))
const route = useRoute()
const router = useRouter()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
const desktopWindowCloseGuards = inject(desktopWindowCloseGuardKey, undefined)
const filesPage = ref<HTMLElement>()
const localClusterNodeId = ref('')
const fileHostPickerButton = ref<HTMLButtonElement>()
const fileHostPickerOpen = ref(false)
const fileHostInventory = ref<ClusterHostList>()
const fileHostInventoryLoading = ref(false)
const fileHostInventoryError = ref(false)
const activeFileHostId = ref('')
const fileHostId = ref(typeof route.query.hostId === 'string' ? route.query.hostId : '')
const fileAPI = computed(() => fileAPIForHost(fileHostId.value))
const clusterHostOrderRevision = ref(0)
let unregisterWindowCloseGuard: (() => void) | undefined
let unsubscribeClusterHostOrder: (() => void) | undefined

type DialogAction = 'mkdir' | 'rename' | 'chmod' | 'compress' | 'extract' | 'trash'

function requestedFilePath(value: unknown): string | undefined {
  const candidate = Array.isArray(value) ? value[0] : value
  if (
    typeof candidate !== 'string'
    || !candidate.startsWith('/')
    || candidate.length > 4096
    || candidate.includes('\0')
    || candidate.includes('\\')
  ) {
    return undefined
  }
  if (candidate !== '/' && candidate.slice(1).split('/').some((part) => !part || part === '.' || part === '..')) {
    return undefined
  }
  return candidate
}
type PreviewMode = 'text' | 'image' | 'audio' | 'video' | 'pdf' | 'metadata'
type ArchiveFormat = 'tar.gz' | 'zip' | 'tar'
type FileViewMode = 'list' | 'grid'

interface CodeEditorHandle {
  getValue: () => string
  markClean: () => void
  openSearch: () => void
  focus: () => void
}

interface CodeEditorStatus {
  line: number
  column: number
  lines: number
}

type UploadTaskPhase = 'running' | 'success' | 'error'

interface UploadTask {
  id: string
  name: string
  target: string
  progress: number
  phase: UploadTaskPhase
  detail?: string
}

type UploadTaskSource =
  | { kind: 'file'; file: File; target: string; hostId: string }
  | { kind: 'directory'; manifest: ExternalDropManifest; target: string; hostId: string }

const toast = useToast()
const i18n = useI18n()

type FileHostAction = 'select' | 'open' | 'manage'

interface FileHostStatus {
  action: FileHostAction
  label: string
}

const fileHosts = computed(() => {
  clusterHostOrderRevision.value
  return sortClusterHosts(fileHostInventory.value?.items || [], readClusterHostOrder())
})

const hostOperatingSystemIdentity = (host: ClusterHost) =>
  detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)

const activeFileHost = computed(() =>
  fileHosts.value.find((host) => host.id === activeFileHostId.value)
    || (!fileHostId.value ? fileHosts.value.find((host) => host.isLocal) : undefined),
)
const isRemoteFileHost = computed(() => Boolean(fileHostId.value))
const activeFileHostNodeId = computed(() => {
  const host = activeFileHost.value
  return fileHostId.value ? (host?.remoteNodeId || host?.id || '') : localClusterNodeId.value
})

const activeFileHostLabel = computed(() => {
  const host = activeFileHost.value
  return fileHostId.value ? host?.name || fileHostId.value : phrase('本机')
})

function fileHostStatus(host: ClusterHost): FileHostStatus {
  if (host.isLocal) return { action: 'select', label: phrase('当前面板') }
  if (host.kind === 'light_node') {
    return host.fileManagementAvailable === true
      ? { action: 'select', label: phrase('文件管理已就绪') }
      : { action: 'manage', label: phrase('文件代理未就绪') }
  }
  if (['offline', 'auth_failed', 'tls_error', 'incompatible'].includes(host.state)) {
    return { action: 'manage', label: phrase('主机连接异常') }
  }
  if (['pairing', 'revoking'].includes(host.state)) {
    return { action: 'manage', label: phrase('主机状态处理中') }
  }
  if (host.kind === 'panel' && host.fileManagementAvailable === true) {
    return { action: 'select', label: phrase('文件管理已就绪') }
  }
  if (host.mutualFileTransferAvailable) {
    return { action: 'open', label: phrase('已配对 · 文件互传') }
  }
  if (host.fileTransferAvailable === true) {
    return { action: 'open', label: phrase('已配对 · 仅支持接收') }
  }
  return { action: 'open', label: phrase('打开远端文件管理') }
}

function closeFileHostPicker(restoreFocus = false): void {
  fileHostPickerOpen.value = false
  if (restoreFocus) void nextTick(() => fileHostPickerButton.value?.focus())
}

async function loadFileHosts(): Promise<void> {
  fileHostController?.abort()
  const controller = new AbortController()
  fileHostController = controller
  fileHostInventoryLoading.value = true
  fileHostInventoryError.value = false
  try {
    const inventory = await api.cluster.hosts(controller.signal)
    if (controller.signal.aborted || unmounted) return
    const previousActiveHostId = activeFileHostId.value
    fileHostInventory.value = inventory
    localClusterNodeId.value = inventory.nodeId
    const localHost = inventory.items.find((host) => host.isLocal)
    const previousHost = inventory.items.find((host) =>
      host.id === previousActiveHostId && fileHostStatus(host).action === 'select',
    )
    // A missing/revoked remote host must retain its target and fail closed.
    activeFileHostId.value = fileHostId.value || previousHost?.id || localHost?.id || ''
  } catch (error) {
    if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) return
    fileHostInventoryError.value = true
  } finally {
    if (fileHostController === controller) {
      fileHostController = undefined
      fileHostInventoryLoading.value = false
    }
  }
}

function toggleFileHostPicker(): void {
  fileHostPickerOpen.value = !fileHostPickerOpen.value
  if (fileHostPickerOpen.value && !fileHostInventory.value && !fileHostInventoryLoading.value) {
    void loadFileHosts()
  }
}

function openRemoteFileManager(host: ClusterHost): void {
  closeFileHostPicker()
  let target = ''
  try {
    const panelURL = clusterHostPanelURL(host)
    if (!panelURL) throw new Error('missing panel URL')
    const url = new URL(panelURL)
    if (!['http:', 'https:'].includes(url.protocol)) throw new Error('unsupported protocol')
    url.pathname = `${url.pathname.replace(/\/+$/, '')}/files`
    url.search = ''
    url.hash = ''
    target = url.toString()
  } catch {
    void router.push({ name: 'cluster' })
    return
  }
  if (
    host.transportSecurity === 'e2e_http'
    && !window.confirm(i18n.t('cluster.confirm.openHttpPanel'))
  ) return
  const opened = typeof window !== 'undefined'
    ? window.open(target, '_blank', 'noopener,noreferrer')
    : null
  if (!opened) {
    toast.show(phrase('远端文件页未打开'), { message: phrase('浏览器阻止了新标签页，请允许弹出窗口后重试。') })
  }
}

function openClusterHostManager(): void {
  closeFileHostPicker()
  void router.push({ name: 'cluster' })
}

function resetFileHostContext(hostId: string): boolean {
  if (previewDirty.value && !window.confirm('文件尚未保存，确认切换主机吗？')) return false
  previewDirty.value = false
  directoryController?.abort()
  closePreview()
  contextMenu.value = undefined
  shareEntry.value = undefined
  shareManagerOpen.value = false
  remoteDownloadDialogOpen.value = false
  dismissFileTransfer()
  clearInternalDropTarget()
  fileHostId.value = hostId
  activeFileHostId.value = hostId || fileHosts.value.find((host) => host.isLocal)?.id || ''
  directory.value = undefined
  dialogAction.value = undefined
  trashOpen.value = false
  trashEntries.value = []
  selectedTrash.value = new Set()
  uploadTasks.value.forEach((task) => dismissUploadTask(task.id))
  search.value = ''
  clearSelection()
  openedRouteFile = ''
  return true
}

function handleFileHostSelection(host: ClusterHost): void {
  const status = fileHostStatus(host)
  if (status.action === 'select') {
    if (host.id === activeFileHostId.value) {
      closeFileHostPicker(true)
      return
    }
    if (
      fileTransferState.value?.phase === 'running'
      || pasteBusy.value
      || dialogBusy.value
      || trashBusy.value
      || externalUploadController
      || previewSaving.value
      || desktopAdding.value
      || remoteDownloadSubmitting.value
      || uploadTasks.value.some((task) => task.phase === 'running')
    ) {
      toast.show('当前主机有文件操作进行中', { message: '操作完成后再切换主机，避免文件落到错误的位置。' })
      return
    }
    if (previewDirty.value) {
      if (!window.confirm('文件尚未保存，确认切换主机吗？')) return
      previewDirty.value = false
    }
    resetFileHostContext(host.isLocal ? '' : host.id)
    void router.push({ name: 'files', query: { path: currentPath.value, ...(fileHostId.value ? { hostId: fileHostId.value } : {}) } })
    if (!host.isLocal) {
      stopRemoteDownloadPolling()
      remoteDownloadJobs.value = []
      remoteDownloadJobsError.value = undefined
    } else {
      void loadRemoteDownloadJobs(true)
    }
    closeFileHostPicker(true)
    void navigateDirectory(host.isLocal ? '/home' : '/')
  } else if (status.action === 'open') {
    openRemoteFileManager(host)
  } else {
    openClusterHostManager()
  }
}

const directory = ref<FileDirectory>()
const currentPath = ref('/home')
const search = ref('')
const sortKey = ref<'name' | 'size' | 'modified'>('name')
const sortDescending = ref(false)
const viewMode = ref<FileViewMode>('list')
const loading = ref(false)
const dragging = ref(false)
const selected = ref(new Set<string>())
const selectionAnchor = ref<string>()
const uploadInput = ref<HTMLInputElement>()
const uploadTasks = ref<UploadTask[]>([])
const remoteDownloadURLInput = ref<HTMLInputElement>()
const remoteDownloadDialogOpen = ref(false)
const remoteDownloadURL = ref('')
const remoteDownloadName = ref('')
const remoteDownloadTarget = ref('/home')
const remoteDownloadFormErrorCode = ref<'url' | 'name'>()
const remoteDownloadHTTPWarningID = 'remote-download-http-warning'
const remoteDownloadFormErrorID = 'remote-download-form-error'
const remoteDownloadUsesPlainHTTP = computed(() =>
  remoteDownloadURL.value.trim().toLowerCase().startsWith('http://'),
)
const remoteDownloadURLDescription = computed(() => {
  const descriptions = []
  if (remoteDownloadUsesPlainHTTP.value) descriptions.push(remoteDownloadHTTPWarningID)
  if (remoteDownloadFormErrorCode.value === 'url') descriptions.push(remoteDownloadFormErrorID)
  return descriptions.join(' ') || undefined
})
const remoteDownloadFormError = computed(() => {
  if (remoteDownloadFormErrorCode.value === 'url') return i18n.t('files.remoteDownload.urlInvalid')
  if (remoteDownloadFormErrorCode.value === 'name') return i18n.t('files.remoteDownload.nameInvalid')
  return ''
})
const remoteDownloadJobs = ref<FileRemoteDownloadJob[]>([])
const remoteDownloadJobsLoading = ref(false)
const remoteDownloadSubmitting = ref(false)
const remoteDownloadPendingActions = ref(new Set<string>())
const remoteDownloadJobsError = ref<{ code?: string; detail?: string }>()
const remoteDownloadJobsErrorMessage = computed(() => {
  const error = remoteDownloadJobsError.value
  return error ? remoteDownloadErrorDetail(error.code, error.detail) : ''
})
const remoteDownloadTasksVisible = computed(() => (
  !isRemoteFileHost.value && (remoteDownloadJobs.value.length > 0 || Boolean(remoteDownloadJobsErrorMessage.value))
))
const dialogAction = ref<DialogAction>()
const dialogValue = ref('')
const dialogFormat = ref<ArchiveFormat>('tar.gz')
const dialogBusy = ref(false)
const dialogEntries = ref<FileEntry[]>([])
const contextMenu = ref<{ entry?: FileEntry; x: number; y: number }>()
const contextMenuElement = ref<HTMLElement>()
const shareEntry = ref<FileEntry>()
const shareManagerOpen = ref(false)
const fileClipboard = useFileClipboard()
const clipboard = computed(() => fileClipboard.clipboard.value?.hostId === fileHostId.value
  ? fileClipboard.clipboard.value : undefined)
const pasteBusy = ref(false)
const internalDropTarget = ref('')
const internalDropMode = ref<FileTransferOperation>('move')
const internalDropCount = ref(0)
const crossPanelDropActive = ref(false)
const fileTransferState = ref<{
  mode: FileTransferOperation
  target: string
  count: number
  phase: 'running' | 'success' | 'partial' | 'cancelled' | 'error'
  remote?: boolean
  completed?: number
  currentName?: string
  detail?: string
}>()
const fileStatusStackVisible = computed(() => Boolean(
  clipboard.value?.entries.length
  || remoteDownloadTasksVisible.value
  || fileTransferState.value
  || uploadTasks.value.length,
))
const desktopAdding = ref(false)
const previewEntry = ref<FileEntry>()
const previewContent = ref('')
const previewLoading = ref(false)
let previewRequestId = 0
const previewSaving = ref(false)
const previewDirty = ref(false)
const mediaLoading = ref(false)
const mediaReady = ref(false)
const mediaError = ref(false)
const mediaErrorMessage = ref('')
const mediaErrorDetail = ref('')
const mediaRetryable = ref(false)
const mediaReloadKey = ref(0)
const editorInfo = ref<Pick<CodeLanguage, 'label' | 'highlighted' | 'reason'> & { loadMs: number }>()
const codeEditorRef = ref<CodeEditorHandle>()
const editorStatus = ref<CodeEditorStatus>()
const editorLineWrap = ref(false)
const trashOpen = ref(false)
const trashLoading = ref(false)
const trashBusy = ref(false)
const trashEntries = ref<FileTrashEntry[]>([])
const trashTotal = ref(0)
const trashTruncated = ref(false)
const selectedTrash = ref(new Set<string>())
const thumbnailFailures = ref(new Set<string>())
let directoryController: AbortController | undefined
let fileHostController: AbortController | undefined
let queuedRemoteDownloadRefreshes = new Set<string>()
let archiveController: AbortController | undefined
let externalUploadController: AbortController | undefined
let fileTransferController: AbortController | undefined
let fileTransferClearTimer: number | undefined
const uploadTaskSources = new Map<string, UploadTaskSource>()
const uploadTaskClearTimers = new Map<string, number>()
let uploadTaskSequence = 0
let remoteDownloadJobsController: AbortController | undefined
let remoteDownloadPollTimer: number | undefined
let remoteDownloadJobsInitialized = false
let unsubscribeFileDirectoryChanges: (() => void) | undefined
let searchTimer: number | undefined
let mediaLoadTimer: number | undefined
let unmounted = false
let openedRouteFile = ''
let contextMenuOpener: HTMLElement | null = null
const fileWindowChangeOrigin = Symbol('file-window')

const fileViewStorageKey = 'kpanel:files:view:v1'
const thumbnailSourceMaxBytes = 12 * 1024 * 1024
const mediaLoadTimeoutMs = 20_000
const remoteDownloadPollDelay = 2_500
const remoteDownloadPollRetryDelay = 10_000
const remoteDownloadSubmissionClockSkew = 5_000
const activeRemoteDownloadStates = new Set<FileRemoteDownloadJobState>([
  'queued', 'connecting', 'transferring', 'confirming',
])

const activeRemoteDownloadCount = computed(() => remoteDownloadJobs.value.filter(isRemoteDownloadJobActive).length)

function isRemoteDownloadJobActive(job: FileRemoteDownloadJob): boolean {
  return activeRemoteDownloadStates.has(job.state)
}

function remoteDownloadJobProgress(job: FileRemoteDownloadJob): number | undefined {
  if (!job.totalBytes || job.totalBytes <= 0) return undefined
  return Math.min(100, Math.round(((job.loadedBytes || 0) / job.totalBytes) * 100))
}

function remoteDownloadJobProgressLabel(job: FileRemoteDownloadJob): string {
  const progress = remoteDownloadJobProgress(job)
  return progress === undefined
    ? remoteDownloadJobStateLabel(job)
    : i18n.t('files.remoteDownload.progressAria', { progress })
}

function remoteDownloadJobStateLabel(job: FileRemoteDownloadJob): string {
  switch (job.state) {
    case 'queued':
      return i18n.t('files.remoteDownload.phaseQueued')
    case 'connecting':
      return i18n.t('files.remoteDownload.phaseConnecting')
    case 'transferring':
      return i18n.t('files.remoteDownload.phaseTransferring')
    case 'confirming':
      return i18n.t('files.remoteDownload.phaseConfirming')
    case 'complete':
      return i18n.t('files.remoteDownload.phaseSuccess')
    case 'cancelled':
      return i18n.t('files.remoteDownload.phaseCancelled')
    case 'interrupted':
      return i18n.t('files.remoteDownload.phaseInterrupted')
    default:
      return i18n.t('files.remoteDownload.phaseError')
  }
}

function remoteDownloadJobDetail(job: FileRemoteDownloadJob): string {
  if (job.state === 'interrupted') return i18n.t('files.remoteDownload.interruptedDetail')
  if (job.state === 'cancelled') return i18n.t('files.remoteDownload.cancelledDetail')
  return job.state === 'error' ? remoteDownloadErrorDetail(job.code) : ''
}

const fileTransferTitle = computed(() => {
  const state = fileTransferState.value
  if (!state) return ''
  if (state.remote) {
    if (state.phase === 'running') return `正在从另一台主机复制 ${state.completed || 0}/${state.count} 项`
    if (state.phase === 'success') return `跨主机复制完成（${state.count} 项）`
    if (state.phase === 'cancelled') return '跨主机复制已取消'
    if (state.phase === 'error') return '跨主机复制失败'
    return `跨主机复制部分完成（${state.count} 项）`
  }
  if (state.mode === 'copy') {
    if (state.phase === 'running') return `正在复制 ${state.count} 项`
    if (state.phase === 'success') return `已复制 ${state.count} 项`
    if (state.phase === 'cancelled') return '复制已取消'
    if (state.phase === 'error') return '复制失败'
    return '部分项目未复制'
  }
  if (state.phase === 'running') return `正在移动 ${state.count} 项`
  if (state.phase === 'success') return `已移动 ${state.count} 项`
  if (state.phase === 'cancelled') return '移动已取消'
  if (state.phase === 'error') return '移动失败'
  return '部分项目未移动'
})

const internalDropTitle = computed(() => crossPanelDropActive.value
  ? `从另一台主机复制到 ${internalDropTarget.value}`
  : internalDropMode.value === 'copy'
  ? `复制 ${internalDropCount.value} 项到 ${internalDropTarget.value}`
  : `移动 ${internalDropCount.value} 项到 ${internalDropTarget.value}`)

const sortedEntries = computed(() => {
  const values = [...(directory.value?.entries || [])]
  values.sort((left, right) => {
    if (left.kind === 'directory' && right.kind !== 'directory') return -1
    if (left.kind !== 'directory' && right.kind === 'directory') return 1
    let result = 0
    if (sortKey.value === 'size') result = left.sizeBytes - right.sizeBytes
    else if (sortKey.value === 'modified')
      result = new Date(left.modifiedAt).getTime() - new Date(right.modifiedAt).getTime()
    else result = left.name.localeCompare(right.name, 'zh-CN', { numeric: true, sensitivity: 'base' })
    return sortDescending.value ? -result : result
  })
  return values
})
const entrySearchCatalog = computed(() =>
  sortedEntries.value.map((entry) => ({ entry, searchName: entry.name.toLocaleLowerCase() })),
)

const entries = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return query
    ? entrySearchCatalog.value
      .filter(({ searchName }) => searchName.includes(query))
      .map(({ entry }) => entry)
    : sortedEntries.value
})

const breadcrumbs = computed(() => {
  const parts = currentPath.value.split('/').filter(Boolean)
  return [
    { name: '根目录', path: '/' },
    ...parts.map((name, index) => ({
      name,
      path: `/${parts.slice(0, index + 1).join('/')}`,
    })),
  ]
})

const selectedEntries = computed(() =>
  (directory.value?.entries || []).filter((entry) => selected.value.has(entry.path)),
)
const contextBatchEntries = computed(() =>
  contextMenu.value?.entry ? entriesForBatch(contextMenu.value.entry) : [],
)
const contextHasMultipleEntries = computed(() => contextBatchEntries.value.length > 1)
const contextShareEntry = computed(() => {
  if (isRemoteFileHost.value) return undefined
  const targets = contextBatchEntries.value
  return targets.length === 1 && targets[0]?.kind === 'file' ? targets[0] : undefined
})
const selectedEntriesDownloadable = computed(() =>
  selectedEntries.value.length > 0 && selectedEntries.value.every(canAddToDesktop),
)
const contextBatchDownloadable = computed(() =>
  contextBatchEntries.value.length > 0 && contextBatchEntries.value.every(canAddToDesktop),
)
const allVisibleSelected = computed(
  () => entries.value.length > 0 && entries.value.every((entry) => selected.value.has(entry.path)),
)
const allTrashSelected = computed(
  () => trashEntries.value.length > 0 && trashEntries.value.every((entry) => selectedTrash.value.has(entry.id)),
)
const previewMode = computed<PreviewMode>(() => {
  const entry = previewEntry.value
  if (!entry) return 'metadata'
  if (entry.editable) return 'text'
  if (entry.mime?.startsWith('image/')) return 'image'
  if (entry.mime?.startsWith('audio/')) return 'audio'
  if (entry.mime?.startsWith('video/')) return 'video'
  if (entry.mime === 'application/pdf') return 'pdf'
  return 'metadata'
})
const mediaStatusLabel = computed(() => {
  if (mediaError.value) return mediaErrorMessage.value || '无法读取媒体文件'
  if (mediaLoading.value && previewMode.value === 'video') return '正在缓冲视频…'
  if (previewMode.value === 'video') return '按需加载 · 支持边缓冲边播放'
  if (previewMode.value === 'audio') return '音频流'
  if (previewMode.value === 'image') return '图片预览'
  if (previewMode.value === 'pdf') return 'PDF 文档'
  return ''
})
const previewURL = computed(() =>
  previewEntry.value ? fileAPI.value.contentUrl(previewEntry.value.path, 'inline') : '',
)
const dialogTitle = computed(() => {
  const titles: Record<DialogAction, string> = {
    mkdir: '新建目录',
    rename: '重命名',
    chmod: dialogEntries.value.length > 1 ? `修改 ${dialogEntries.value.length} 项权限` : '修改权限',
    compress: dialogEntries.value.length > 1 ? `压缩 ${dialogEntries.value.length} 项` : '压缩文件',
    extract: '解压文件',
    trash: dialogEntries.value.length > 1 ? `移入回收站（${dialogEntries.value.length} 项）` : '移入回收站',
  }
  return dialogAction.value ? titles[dialogAction.value] : '文件操作'
})

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error) return error.message
  return '操作未完成，请稍后重试。'
}

function remoteDownloadErrorDetail(code?: string, fallback = ''): string {
  switch (code) {
    case 'remote_download_invalid':
      return i18n.t('files.remoteDownload.error.invalid')
    case 'remote_download_busy':
      return i18n.t('files.remoteDownload.error.busy')
    case 'remote_download_queue_full':
      return i18n.t('files.remoteDownload.error.queueFull')
    case 'remote_download_jobs_unavailable':
      return i18n.t('files.remoteDownload.error.jobsUnavailable')
    case 'remote_download_job_not_found':
      return i18n.t('files.remoteDownload.error.jobNotFound')
    case 'remote_download_job_finished':
      return i18n.t('files.remoteDownload.error.jobFinished')
    case 'remote_download_job_active':
      return i18n.t('files.remoteDownload.error.jobActive')
    case 'remote_download_address_blocked':
      return i18n.t('files.remoteDownload.error.addressBlocked')
    case 'remote_download_redirect_rejected':
      return i18n.t('files.remoteDownload.error.redirectRejected')
    case 'remote_download_tls_failed':
      return i18n.t('files.remoteDownload.error.tlsFailed')
    case 'remote_download_encoding_unsupported':
    case 'remote_download_partial_unsupported':
      return i18n.t('files.remoteDownload.error.responseUnsupported')
    case 'remote_download_timeout':
    case 'remote_download_idle_timeout':
      return i18n.t('files.remoteDownload.error.timeout')
    case 'remote_download_too_large':
      return i18n.t('files.remoteDownload.error.tooLarge')
    case 'target_unavailable':
    case 'target_name_unavailable':
    case 'target_permission_denied':
      return i18n.t('files.remoteDownload.error.targetUnavailable')
    case 'target_conflict':
      return i18n.t('files.remoteDownload.error.targetConflict')
    case 'target_storage_full':
      return i18n.t('files.remoteDownload.error.storageFull')
    case 'agent_write_busy':
      return i18n.t('files.remoteDownload.error.agentBusy')
    case 'remote_download_upstream_status':
      return i18n.t('files.remoteDownload.error.upstreamRejected')
    case 'remote_download_cancelled':
      return i18n.t('files.remoteDownload.cancelledDetail')
    case 'remote_download_interrupted':
      return i18n.t('files.remoteDownload.interruptedDetail')
    case 'agent_write_interrupted':
    case 'agent_write_failed':
    case 'agent_result_invalid':
    case 'remote_download_response_invalid':
    case 'remote_download_incomplete':
      return i18n.t('files.remoteDownload.error.resultUnknown')
    case 'remote_download_unavailable':
    case 'agent_stream_unavailable':
    case 'remote_download_unreachable':
    case 'network_error':
    case 'stream_unavailable':
      return i18n.t('files.remoteDownload.error.unavailable')
    default:
      return fallback || i18n.t('files.remoteDownload.error.generic')
  }
}

function setRemoteDownloadJobsError(error: unknown): void {
  remoteDownloadJobsError.value = error instanceof ApiError
    ? { code: error.code, detail: error.message }
    : { detail: error instanceof Error ? error.message : i18n.t('files.remoteDownload.error.generic') }
}

function archiveFormat(entry: FileEntry): ArchiveFormat | undefined {
  if (entry.kind !== 'file') return undefined
  const name = entry.name.toLocaleLowerCase()
  if (name.endsWith('.tar.gz') || name.endsWith('.tgz')) return 'tar.gz'
  if (name.endsWith('.zip')) return 'zip'
  if (name.endsWith('.tar')) return 'tar'
  return undefined
}

function archiveSuffix(format: ArchiveFormat): string {
  return format === 'tar.gz' ? '.tar.gz' : `.${format}`
}

function withoutArchiveSuffix(name: string): string {
  return name.replace(/\.(?:tar\.gz|tgz|zip|tar)$/i, '') || 'archive'
}

function normalizedArchiveName(name: string, format: ArchiveFormat): string {
  return `${withoutArchiveSuffix(name.trim())}${archiveSuffix(format)}`
}

async function loadDirectory(path = currentPath.value, append = false): Promise<string | undefined> {
  if (append && !directory.value?.nextOffset) return undefined
  directoryController?.abort()
  const controller = new AbortController()
  directoryController = controller
  loading.value = true
  contextMenu.value = undefined
  try {
    const result = await fileAPI.value.list(
      path,
      {
        offset: append ? directory.value?.nextOffset : 0,
        search: search.value.trim() || undefined,
      },
      controller.signal,
    )
    if (controller.signal.aborted || directoryController !== controller) return undefined
    if (append && directory.value?.path === result.path) {
      const known = new Set(directory.value.entries.map((entry) => entry.path))
      directory.value = {
        ...result,
        entries: [
          ...directory.value.entries,
          ...result.entries.filter((entry) => !known.has(entry.path)),
        ],
      }
    } else {
      directory.value = result
    }
    currentPath.value = result.path
    if (!append) {
      selected.value = new Set()
      selectionAnchor.value = undefined
      thumbnailFailures.value = new Set()
    }
    return result.path
  } catch (error) {
    if (controller.signal.aborted) return undefined
    toast.danger('目录读取失败', errorMessage(error))
    return undefined
  } finally {
    if (directoryController === controller) {
      loading.value = false
      directoryController = undefined
      const refreshTargets = queuedRemoteDownloadRefreshes
      queuedRemoteDownloadRefreshes = new Set<string>()
      if (refreshTargets.has(currentPath.value) && !unmounted) {
        void loadDirectory(currentPath.value)
      }
    }
  }
}

async function navigateDirectory(path: string): Promise<void> {
  const resolvedPath = await loadDirectory(path)
  const routePath = requestedFilePath(route.query.path) || '/'
  if (!resolvedPath || (resolvedPath === routePath && (route.query.hostId || '') === fileHostId.value)) return
  await router.push({ name: 'files', query: { path: resolvedPath, ...(fileHostId.value ? { hostId: fileHostId.value } : {}) } })
}

async function openRequestedFile(value: unknown): Promise<void> {
  const hostId = fileHostId.value
  const filePath = requestedFilePath(value)
  if (!filePath || filePath === '/' || filePath === openedRouteFile) return
  openedRouteFile = filePath
  try {
    const entry = await fileAPI.value.entry(filePath)
    if (unmounted || hostId !== fileHostId.value) return
    if (entry.kind !== 'file') {
      toast.show('目标类型已变化', { message: '该路径现在不是普通文件，请从文件管理重新添加。' })
      return
    }
    selected.value = new Set([entry.path])
    selectionAnchor.value = entry.path
    await openPreview(entry)
  } catch (error) {
    toast.danger('桌面目标无法打开', errorMessage(error))
  }
}

async function loadRequestedRoute(): Promise<void> {
  const hostId = fileHostId.value
  await loadDirectory(requestedFilePath(route.query.path) || '/')
  if (unmounted || hostId !== fileHostId.value) return
  await openRequestedFile(route.query.file)
}

function setViewMode(mode: FileViewMode): void {
  viewMode.value = mode
  try {
    window.localStorage?.setItem(fileViewStorageKey, mode)
  } catch {
    // Browser privacy modes may reject preference storage; the current view still works.
  }
}

function restoreViewMode(): void {
  try {
    const stored = window.localStorage?.getItem(fileViewStorageKey)
    if (stored === 'list' || stored === 'grid') viewMode.value = stored
  } catch {
    // Keep the lightweight list default when browser storage is unavailable.
  }
}

function openEntry(entry: FileEntry): void {
  contextMenu.value = undefined
  if (entry.kind === 'directory') {
    void navigateDirectory(entry.path)
    return
  }
  if (entry.kind === 'file') void openPreview(entry)
}

function clearMediaLoadTimer(): void {
  if (mediaLoadTimer !== undefined) {
    window.clearTimeout(mediaLoadTimer)
    mediaLoadTimer = undefined
  }
}

function resetMediaState(entry?: FileEntry): void {
  clearMediaLoadTimer()
  const isVideo = Boolean(entry?.mime?.startsWith('video/'))
  mediaLoading.value = isVideo
  mediaReady.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
  mediaErrorDetail.value = ''
  mediaRetryable.value = false
  if (!isVideo) return
  mediaLoadTimer = window.setTimeout(() => {
    if (!mediaReady.value && !mediaError.value && previewEntry.value?.path === entry?.path) {
      mediaLoading.value = false
      mediaError.value = true
      mediaErrorMessage.value = '视频流响应超时，请检查网络或服务器。'
      mediaErrorDetail.value = '请检查网络或服务器后重试。'
      mediaRetryable.value = true
    }
    mediaLoadTimer = undefined
  }, mediaLoadTimeoutMs)
}

async function openPreview(entry: FileEntry): Promise<void> {
  const hostId = fileHostId.value
  const requestId = ++previewRequestId
  previewEntry.value = entry
  previewContent.value = ''
  previewDirty.value = false
  mediaReloadKey.value += 1
  resetMediaState(entry)
  editorInfo.value = undefined
  editorStatus.value = undefined
  editorLineWrap.value = false
  if (!entry.editable) return
  previewLoading.value = true
  try {
    const content = await fileAPI.value.text(entry.path)
    if (unmounted || hostId !== fileHostId.value || requestId !== previewRequestId) return
    previewContent.value = content
  } catch (error) {
    if (unmounted || hostId !== fileHostId.value || requestId !== previewRequestId) return
    toast.danger('文件打开失败', errorMessage(error))
    previewEntry.value = undefined
  } finally {
    if (requestId === previewRequestId) previewLoading.value = false
  }
}

function handleMediaLoadStart(): void {
  mediaLoading.value = true
  mediaReady.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
  mediaErrorDetail.value = ''
  mediaRetryable.value = false
}

function handleMediaMetadata(): void {
  mediaLoading.value = false
  clearMediaLoadTimer()
}

function handleMediaReady(): void {
  mediaReady.value = true
  mediaLoading.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
  mediaErrorDetail.value = ''
  mediaRetryable.value = false
  clearMediaLoadTimer()
}

function handleVideoFrameReady(event: Event): void {
  const video = event.currentTarget as HTMLVideoElement | null
  // A playable audio track can make loadeddata/canplay succeed even when the
  // browser cannot decode any video frame, so media events alone are not enough.
  if (!video || video.videoWidth <= 0 || video.videoHeight <= 0) {
    video?.pause()
    mediaLoading.value = false
    mediaReady.value = false
    mediaError.value = true
    mediaErrorMessage.value = '浏览器只能播放音轨，无法解码视频画面。'
    mediaErrorDetail.value = '请下载原文件，或转换为 H.264 + AAC 的 MP4 后重试。'
    mediaRetryable.value = false
    clearMediaLoadTimer()
    return
  }
  handleMediaReady()
}

function handleMediaWaiting(): void {
  if (mediaError.value) return
  mediaLoading.value = true
}

function handleMediaError(event: Event): void {
  const video = event.currentTarget as HTMLVideoElement | null
  mediaLoading.value = false
  mediaReady.value = false
  mediaError.value = true
  mediaErrorMessage.value = video?.error?.code === 4
    ? '浏览器不支持该视频编码或格式。'
    : ''
  mediaErrorDetail.value = video?.error?.code === 4
    ? '请下载原文件，或转换为 H.264 + AAC 的 MP4 后重试。'
    : '请检查文件编码、网络或服务器后重试。'
  mediaRetryable.value = video?.error?.code !== 4
  clearMediaLoadTimer()
}

function retryMedia(): void {
  const entry = previewEntry.value
  if (!entry || !entry.mime?.startsWith('video/')) return
  mediaReloadKey.value += 1
  resetMediaState(entry)
}

function closePreview(): void {
  if (previewDirty.value && !window.confirm('文件尚未保存，确认关闭吗？')) return
  previewRequestId += 1
  previewLoading.value = false
  previewEntry.value = undefined
  previewContent.value = ''
  previewDirty.value = false
  clearMediaLoadTimer()
  mediaLoading.value = false
  mediaReady.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
  mediaErrorDetail.value = ''
  mediaRetryable.value = false
  editorInfo.value = undefined
  editorStatus.value = undefined
  editorLineWrap.value = false
}

async function savePreview(content?: string): Promise<void> {
  const hostId = fileHostId.value
  const requestId = previewRequestId
  const entry = previewEntry.value
  if (!entry || !entry.editable || !previewDirty.value) return
  const nextContent = content ?? codeEditorRef.value?.getValue() ?? previewContent.value
  previewContent.value = nextContent
  previewSaving.value = true
  try {
    const result = await fileAPI.value.write(entry.path, nextContent, entry.resourceVersion)
    if (unmounted || hostId !== fileHostId.value || requestId !== previewRequestId) return
    previewEntry.value = result.entry
    if ((codeEditorRef.value?.getValue() ?? nextContent) === nextContent) {
      previewDirty.value = false
      codeEditorRef.value?.markClean()
    }
    toast.success('已保存', entry.name)
    if (!unmounted) await loadDirectory()
  } catch (error) {
    toast.danger('保存失败', errorMessage(error))
  } finally {
    previewSaving.value = false
  }
}

function handleEditorReady(info: CodeLanguage & { loadMs: number }): void {
  editorInfo.value = {
    label: info.label,
    highlighted: info.highlighted,
    reason: info.reason,
    loadMs: info.loadMs,
  }
}

function handleEditorStatus(status: CodeEditorStatus): void {
  editorStatus.value = status
}

function toggleEntry(path: string): void {
  const next = new Set(selected.value)
  if (next.has(path)) next.delete(path)
  else next.add(path)
  selected.value = next
  selectionAnchor.value = path
}

function selectEntry(event: MouseEvent, path: string): void {
  if (event.shiftKey && selectionAnchor.value) {
    const visiblePaths = entries.value.map((entry) => entry.path)
    const anchorIndex = visiblePaths.indexOf(selectionAnchor.value)
    const currentIndex = visiblePaths.indexOf(path)
    if (anchorIndex >= 0 && currentIndex >= 0) {
      const start = Math.min(anchorIndex, currentIndex)
      const end = Math.max(anchorIndex, currentIndex)
      const range = visiblePaths.slice(start, end + 1)
      selected.value =
        event.ctrlKey || event.metaKey
          ? new Set([...selected.value, ...range])
          : new Set(range)
      return
    }
  }
  if (event.ctrlKey || event.metaKey) {
    toggleEntry(path)
    return
  }
  if (selected.value.size === 1 && selected.value.has(path)) {
    clearSelection()
    return
  }
  selected.value = new Set([path])
  selectionAnchor.value = path
}

function handleEntryClick(event: MouseEvent, entry: FileEntry): void {
  if (typeof window !== 'undefined' && window.innerWidth <= 720) {
    event.preventDefault()
    event.stopPropagation()
    clearSelection()
    openEntry(entry)
    return
  }
  selectEntry(event, entry.path)
}

function toggleAll(): void {
  const clearVisible = allVisibleSelected.value
  const next = new Set(selected.value)
  if (clearVisible) entries.value.forEach((entry) => next.delete(entry.path))
  else entries.value.forEach((entry) => next.add(entry.path))
  selected.value = next
  selectionAnchor.value = clearVisible ? undefined : entries.value[0]?.path
}

function invertSelection(): void {
  const next = new Set(selected.value)
  for (const entry of entries.value) {
    if (next.has(entry.path)) next.delete(entry.path)
    else next.add(entry.path)
  }
  selected.value = next
  selectionAnchor.value = entries.value.find((entry) => next.has(entry.path))?.path
}

function clearSelection(): void {
  selected.value = new Set()
  selectionAnchor.value = undefined
}

function preventNativeSelection(event: Event): void {
  event.preventDefault()
}

function selectForContext(entry: FileEntry): void {
  if (selected.value.has(entry.path)) return
  selected.value = new Set([entry.path])
  selectionAnchor.value = entry.path
}

function entriesForBatch(entry?: FileEntry): FileEntry[] {
  if (entry && !selected.value.has(entry.path)) return [entry]
  return [...selectedEntries.value]
}

function canAddToDesktop(entry: FileEntry): boolean {
  return entry.kind === 'file' || entry.kind === 'directory'
}

function currentDirectoryEntry() {
  const name = currentPath.value === '/'
    ? '根目录'
    : currentPath.value.slice(currentPath.value.lastIndexOf('/') + 1)
  return { name, path: currentPath.value, kind: 'directory' as const }
}

async function addEntriesToDesktop(entry?: FileEntry, currentDirectory = false): Promise<void> {
  if (desktopAdding.value) return
  if (isRemoteFileHost.value) {
    toast.show('远端主机暂不支持桌面快捷方式', { message: '快捷方式属于当前 KPanel 桌面，请在本机文件中添加。' })
    return
  }
  contextMenu.value = undefined
  const targets = currentDirectory
    ? [currentDirectoryEntry()]
    : entry
      ? entriesForBatch(entry).filter(canAddToDesktop)
      : selectedEntries.value.filter(canAddToDesktop)
  if (!targets.length) {
    toast.show('无法添加到桌面', { message: '请选择普通文件或文件夹。' })
    return
  }
  desktopAdding.value = true
  try {
    const result = await addFileEntriesToDesktop(targets)
    if (result.added.length) {
      toast.success(
        result.added.length === 1 ? '已添加到桌面' : `已添加 ${result.added.length} 项到桌面`,
        result.added.length === 1 ? result.added[0]!.name : '图标已按桌面空位自动排列。',
      )
    } else if (result.duplicates.length) {
      toast.show('已经在桌面', { message: result.duplicates[0]!.name })
    }
  } catch (error) {
    if (error instanceof DesktopShortcutLimitError) {
      toast.danger('桌面快捷方式已满', `还可添加 ${error.available} 项，本次选择了 ${error.requested} 项。`)
    } else {
      toast.danger('添加到桌面失败', errorMessage(error))
    }
  } finally {
    desktopAdding.value = false
  }
}

function startEntryDrag(event: DragEvent, entry: FileEntry): void {
  const targets = (selected.value.has(entry.path) ? entriesForBatch(entry) : [entry]).filter(canAddToDesktop)
  const directFile = targets.length === 1 && targets[0]!.kind === 'file'
  const nativeArchiveName = directFile
    ? undefined
    : nativeArchiveDownloadName(targets, currentDirectoryEntry().name)
  const nativeDownloadURL = directFile
    ? fileAPI.value.contentUrl(targets[0]!.path, 'attachment')
    : nativeArchiveName
      ? fileAPI.value.archiveUrl(targets, nativeArchiveName)
      : undefined
  const sourceNodeId = activeFileHostNodeId.value
  if (!beginDesktopFileDrag(
    event,
    targets,
    sourceNodeId,
    'file-manager',
    nativeDownloadURL,
    nativeArchiveName,
    fileWindowChangeOrigin,
  )) event.preventDefault()
}

function finishEntryDrag(): void {
  clearDesktopFileDrag(fileWindowChangeOrigin)
}

function clearInternalDropTarget(): void {
  internalDropTarget.value = ''
  internalDropCount.value = 0
  crossPanelDropActive.value = false
}

function isOtherFileHostDrag(event: DragEvent): boolean {
  const sourceNodeId = peekDesktopFileDragSourceNodeId(event)
  return Boolean(sourceNodeId && sourceNodeId !== activeFileHostNodeId.value)
}

function updateInternalDropTarget(event: DragEvent, target: string): boolean {
  if (fileTransferState.value?.phase === 'running') return false
  if ((!hasDesktopFileDrag(event) || isOtherFileHostDrag(event)) && hasCrossPanelFileDrag(event)) {
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    internalDropMode.value = 'copy'
    internalDropCount.value = 0
    internalDropTarget.value = target
    crossPanelDropActive.value = true
    return true
  }
  if (!hasDesktopFileDrag(event)) return false
  if (desktopFileDragOrigin(event) === 'desktop-shortcut') {
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
    clearInternalDropTarget()
    return false
  }
  const entries = peekDesktopFileDragEntries(event)
  const operation = fileTransferOperation(event)
  const invalid = fileTransferTargetError(entries, target)
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = invalid ? 'none' : operation
  internalDropMode.value = operation
  internalDropCount.value = entries.length
  internalDropTarget.value = invalid ? '' : target
  crossPanelDropActive.value = false
  return !invalid
}

function onFileBrowserDragOver(event: DragEvent): void {
  if (hasDesktopFileDrag(event) || hasCrossPanelFileDrag(event)) {
    updateInternalDropTarget(event, currentPath.value)
    return
  }
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function onFileBrowserDragLeave(event: DragEvent): void {
  const related = event.relatedTarget as Node | null
  if (related && (event.currentTarget as HTMLElement).contains(related)) return
  dragging.value = false
  clearInternalDropTarget()
}

function onEntryDragOver(event: DragEvent, entry: FileEntry): void {
  if (entry.kind !== 'directory' || (!hasDesktopFileDrag(event) && !hasCrossPanelFileDrag(event))) return
  event.stopPropagation()
  updateInternalDropTarget(event, entry.path)
}

function onEntryDragLeave(event: DragEvent, entry: FileEntry): void {
  if (internalDropTarget.value !== entry.path) return
  const related = event.relatedTarget as Node | null
  if (related && (event.currentTarget as HTMLElement).contains(related)) return
  clearInternalDropTarget()
}

function scheduleFileTransferClear(): void {
  if (fileTransferClearTimer !== undefined) {
    window.clearTimeout(fileTransferClearTimer)
    fileTransferClearTimer = undefined
  }
  if (!['success', 'cancelled'].includes(fileTransferState.value?.phase || '')) return
  fileTransferClearTimer = window.setTimeout(() => {
    fileTransferState.value = undefined
    fileTransferClearTimer = undefined
  }, 2200)
}

function dismissFileTransfer(): void {
  if (fileTransferClearTimer !== undefined) {
    window.clearTimeout(fileTransferClearTimer)
    fileTransferClearTimer = undefined
  }
  fileTransferState.value = undefined
}

function cancelFileTransfer(): void {
  fileTransferController?.abort()
}

async function transferInternalFileDrop(event: DragEvent, target: string): Promise<void> {
  const hostId = fileHostId.value
  if (isOtherFileHostDrag(event)) {
    await transferCrossPanelFileDrop(event, target)
    clearDesktopFileDrag()
    return
  }
  const entries = desktopFileDragEntries(event)
  const operation = fileTransferOperation(event)
  clearInternalDropTarget()
  clearDesktopFileDrag()
  if (!entries.length || fileTransferTargetError(entries, target)) return
  if (fileTransferState.value?.phase === 'running') {
    toast.show('已有文件操作正在进行')
    return
  }

  if (fileTransferClearTimer !== undefined) {
    window.clearTimeout(fileTransferClearTimer)
    fileTransferClearTimer = undefined
  }
  // Copies may be cancelled safely because they never change desktop shortcut
  // targets. A move must run to a server result so successful path mappings can
  // be applied to desktop shortcuts without leaving stale references behind.
  const controller = operation === 'copy' ? new AbortController() : undefined
  fileTransferController = controller
  fileTransferState.value = { mode: operation, target, count: entries.length, phase: 'running' }
  try {
    const result = await fileAPI.value.action({
      action: operation,
      sources: entries.map((entry) => entry.path),
      target,
      expectedResourceVersions: Object.fromEntries(
        entries.flatMap((entry) => entry.resourceVersion ? [[entry.path, entry.resourceVersion]] : []),
      ),
    }, controller?.signal)
    const shortcutSyncFailed = await applySuccessfulFileChanges(result, target, hostId)
    const partial = Boolean(result.failed.length || shortcutSyncFailed)
    fileTransferState.value = {
      mode: operation,
      target,
      count: result.succeeded.length,
      phase: partial ? 'partial' : 'success',
      detail: partial
        ? shortcutSyncFailed
          ? '真实文件已移动，但桌面快捷方式路径同步失败，请刷新后重试。'
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`
        : undefined,
    }
  } catch (error) {
    if (controller?.signal.aborted) {
      fileTransferState.value = {
        mode: operation,
        target,
        count: entries.length,
        phase: 'cancelled',
        detail: '已经复制完成的项目会保留在目标目录。',
      }
    } else {
      fileTransferState.value = {
        mode: operation,
        target,
        count: entries.length,
        phase: 'error',
        detail: errorMessage(error),
      }
    }
    if (!unmounted) await loadDirectory()
  } finally {
    if (fileTransferController === controller) fileTransferController = undefined
    if (!unmounted) scheduleFileTransferClear()
  }
}

async function transferCrossPanelFileDrop(event: DragEvent, target: string): Promise<void> {
  const hostId = fileHostId.value
  const payload = crossPanelFileDragEntries(event)
  clearInternalDropTarget()
  if (!payload) {
    toast.danger('跨主机复制失败', '拖拽数据无效或超过 64 项，请从来源主机重新拖动。')
    return
  }
  if (payload.sourceNodeId === activeFileHostNodeId.value) {
    toast.show('来源和目标是同一台主机', { message: '请在文件管理器中使用复制或移动。' })
    return
  }
  if (fileTransferState.value?.phase === 'running') {
    toast.show('已有文件操作正在进行')
    return
  }
  if (fileTransferClearTimer !== undefined) {
    window.clearTimeout(fileTransferClearTimer)
    fileTransferClearTimer = undefined
  }
  const controller = new AbortController()
  fileTransferController = controller
  const total = payload.entries.length
  fileTransferState.value = {
    mode: 'copy', target, count: total, phase: 'running', remote: true,
    completed: 0, currentName: payload.entries[0]?.name,
  }
  try {
    const result = await transferCrossPanelFileBatch(
      payload,
      target,
      fileAPI.value.transferFromPanel,
      ({ source, completed }) => {
        if (fileTransferController !== controller || controller.signal.aborted) return
        fileTransferState.value = {
          mode: 'copy', target, count: total, phase: 'running', remote: true,
          completed, currentName: source.name,
        }
      },
      controller.signal,
    )
    const completed = result.succeeded.length + result.failed.length
    const phase = result.cancelled
      ? result.succeeded.length ? 'partial' : 'cancelled'
      : result.succeeded.length === 0 ? 'error'
        : result.failed.length ? 'partial' : 'success'
    fileTransferState.value = {
      mode: 'copy', target, count: total, phase, remote: true,
      completed, currentName: result.succeeded.at(-1)?.entry.name,
      detail: result.cancelled
        ? `已完成的 ${result.succeeded.length} 项会保留在目标目录。`
        : phase === 'success'
          ? undefined
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败${result.failed[0]?.detail ? `：${result.failed[0].detail}` : ''}`,
    }
    if (result.succeeded.length) {
      if (!unmounted) await loadDirectory()
      notifyFileDirectoriesChanged([target], fileWindowChangeOrigin, [], hostId)
    }
  } finally {
    if (fileTransferController === controller) fileTransferController = undefined
    if (!unmounted) scheduleFileTransferClear()
  }
}

function onEntryDrop(event: DragEvent, entry: FileEntry): void {
  if (entry.kind !== 'directory' || (!hasDesktopFileDrag(event) && !hasCrossPanelFileDrag(event))) return
  event.preventDefault()
  event.stopPropagation()
  if (hasDesktopFileDrag(event) && desktopFileDragOrigin(event) === 'desktop-shortcut') {
    clearInternalDropTarget()
    return
  }
  if (hasDesktopFileDrag(event)) void transferInternalFileDrop(event, entry.path)
  else void transferCrossPanelFileDrop(event, entry.path)
}

async function settleContextMenu(focusOrigin: ContextMenuFocusOrigin): Promise<void> {
  await nextTick()
  const menu = contextMenuElement.value
  const current = contextMenu.value
  if (!menu || !current) return
  const placed = placeContextMenu(menu, { x: current.x, y: current.y }, contextMenuOpener)
  contextMenu.value = {
    ...current,
    x: placed.x,
    y: placed.y,
  }
  await nextTick()
  focusFirstContextMenuItem(menu, focusOrigin)
}

function contextMenuPoint(event: MouseEvent): { x: number; y: number } {
  if (event.clientX || event.clientY) return { x: event.clientX, y: event.clientY }
  const target = event.currentTarget
  if (typeof HTMLElement === 'undefined' || !(target instanceof HTMLElement)) return { x: 8, y: 8 }
  const bounds = target.getBoundingClientRect()
  return { x: bounds.right, y: bounds.bottom }
}

function showContext(event: MouseEvent, entry: FileEntry): void {
  event.preventDefault()
  selectForContext(entry)
  contextMenuOpener = typeof HTMLElement !== 'undefined' && event.currentTarget instanceof HTMLElement
    ? event.currentTarget
    : null
  const point = contextMenuPoint(event)
  contextMenu.value = {
    entry,
    x: point.x,
    y: point.y,
  }
  void settleContextMenu(contextMenuFocusOrigin(event))
}

function showDirectoryContext(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (
    target.closest(
      '.file-row--entry, .file-grid-card, .file-toolbar, .file-status-stack, .file-limit, .drop-overlay',
    )
  ) {
    return
  }
  event.preventDefault()
  contextMenuOpener = typeof HTMLElement !== 'undefined' && event.currentTarget instanceof HTMLElement
    ? event.currentTarget
    : null
  const point = contextMenuPoint(event)
  contextMenu.value = {
    x: point.x,
    y: point.y,
  }
  void settleContextMenu(contextMenuFocusOrigin(event))
}

function handleContextMenuKeydown(event: KeyboardEvent): void {
  const menu = contextMenuElement.value
  if (!menu) return
  showContextMenuKeyboardFocus(menu)
  if (moveContextMenuFocus(menu, event)) return
  if (event.key === 'Escape') {
    event.preventDefault()
    contextMenu.value = undefined
    contextMenuOpener?.focus({ preventScroll: true })
  }
}

function openDialog(action: DialogAction, entry?: FileEntry): void {
  contextMenu.value = undefined
  const isBatchAction = action === 'chmod' || action === 'compress' || action === 'trash'
  dialogEntries.value = isBatchAction ? entriesForBatch(entry) : entry ? [entry] : [...selectedEntries.value]
  if ((action === 'compress' || action === 'extract') && !dialogEntries.value.length) return
  dialogAction.value = action
  if (action === 'mkdir') dialogValue.value = ''
  else if (action === 'rename') dialogValue.value = dialogEntries.value[0]?.name || ''
  else if (action === 'chmod') dialogValue.value = '644'
  else if (action === 'compress') {
    dialogFormat.value = 'tar.gz'
    dialogValue.value = dialogEntries.value.length === 1
      ? `${dialogEntries.value[0]?.name || 'archive'}${archiveSuffix(dialogFormat.value)}`
      : `archive${archiveSuffix(dialogFormat.value)}`
  } else if (action === 'extract') {
    const source = dialogEntries.value[0]
    const format = source ? archiveFormat(source) : undefined
    if (!source || !format) {
      dialogAction.value = undefined
      toast.danger('无法解压', '仅支持 .tar.gz、.tgz、.zip 和 .tar 文件。')
      return
    }
    dialogFormat.value = format
    dialogValue.value = withoutArchiveSuffix(source.name)
  }
}

function openFileShare(entry?: FileEntry): void {
  if (isRemoteFileHost.value) {
    toast.show('远端主机暂不支持文件分享', { message: '文件管理已支持直接操作，分享链接仍需在 KPanel 本机创建。' })
    return
  }
  const targets = entry ? entriesForBatch(entry) : []
  if (targets.length !== 1 || targets[0]?.kind !== 'file') return
  contextMenu.value = undefined
  shareEntry.value = targets[0]
}

function closeFileShare(): void {
  shareEntry.value = undefined
  void nextTick(() => contextMenuOpener?.focus({ preventScroll: true }))
}

function openShareManager(): void {
  if (isRemoteFileHost.value) {
    toast.show('远端主机暂不支持分享管理', { message: '分享链接属于当前 KPanel 的面板级能力。' })
    return
  }
  shareManagerOpen.value = true
}

function closeShareManager(): void {
  shareManagerOpen.value = false
}

function closeDialog(): void {
  if (dialogBusy.value) return
  dialogAction.value = undefined
  dialogValue.value = ''
  dialogEntries.value = []
}

function cancelArchive(): void {
  archiveController?.abort()
}

function setClipboard(mode: FileTransferOperation, entry?: FileEntry): void {
  contextMenu.value = undefined
  const entriesToStore = entriesForBatch(entry)
  if (!entriesToStore.length) return
  fileClipboard.set(mode, entriesToStore, fileHostId.value)
  clearSelection()
}

function clearClipboard(): void {
  fileClipboard.clear()
}

async function applySuccessfulFileChanges(
  result: FileActionResult,
  target: string | undefined,
  hostId: string,
): Promise<boolean> {
  let shortcutSyncFailed = false
  if (!hostId && result.succeeded.length && (result.action === 'move' || result.action === 'rename')) {
    try {
      await syncMovedDesktopShortcuts(result)
    } catch {
      shortcutSyncFailed = true
    }
  }
  const changedDirectories = changedFileDirectories(result, target)
  if (!unmounted && fileHostId.value === hostId) await loadDirectory()
  notifyFileDirectoriesChanged(changedDirectories, fileWindowChangeOrigin, successfulFileMoves(result), hostId)
  return shortcutSyncFailed
}

async function pasteClipboard(target = currentPath.value): Promise<void> {
  const hostId = fileHostId.value
  const stored = clipboard.value
  if (!stored?.entries.length || pasteBusy.value) return
  if (fileTransferState.value?.phase === 'running') {
    toast.show('已有文件操作正在进行')
    return
  }
  contextMenu.value = undefined
  dismissFileTransfer()
  pasteBusy.value = true
  fileTransferState.value = {
    mode: stored.mode,
    target,
    count: stored.entries.length,
    phase: 'running',
  }
  try {
    const result = await fileAPI.value.action({
      action: stored.mode,
      sources: stored.entries.map((entry) => entry.path),
      target,
      expectedResourceVersions: Object.fromEntries(
        stored.entries.map((entry) => [entry.path, entry.resourceVersion]),
      ),
    })
    if (stored.mode === 'move' && fileClipboard.clipboard.value === stored) {
      const failed = new Set(result.failed.map((item) => item.path))
      const remaining = stored.entries.filter((entry) => failed.has(entry.path))
      if (remaining.length) fileClipboard.set('move', remaining, stored.hostId)
      else fileClipboard.clear()
    }
    const shortcutSyncFailed = await applySuccessfulFileChanges(result, target, hostId)
    const partial = Boolean(result.failed.length || shortcutSyncFailed)
    fileTransferState.value = {
      mode: stored.mode,
      target,
      count: result.succeeded.length,
      phase: partial ? 'partial' : 'success',
      detail: partial
        ? shortcutSyncFailed
          ? '文件已移动，但桌面快捷方式路径同步失败，请刷新后重试。'
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`
        : undefined,
    }
  } catch (error) {
    fileTransferState.value = {
      mode: stored.mode,
      target,
      count: stored.entries.length,
      phase: 'error',
      detail: errorMessage(error),
    }
    await loadDirectory()
  } finally {
    pasteBusy.value = false
    if (!unmounted) scheduleFileTransferClear()
  }
}

async function submitDialog(): Promise<void> {
  const hostId = fileHostId.value
  const action = dialogAction.value
  if (!action) return
  const controller = action === 'compress' || action === 'extract'
    ? new AbortController()
    : undefined
  archiveController = controller
  dialogBusy.value = true
  try {
    let input: FileActionInput
    if (action === 'mkdir') {
      input = { action, target: currentPath.value, name: dialogValue.value.trim() }
    } else if (action === 'rename') {
      const entry = dialogEntries.value[0]
      if (!entry) throw new Error('请选择需要重命名的文件。')
      const parent = entry.path.slice(0, Math.max(entry.path.lastIndexOf('/'), 1))
      input = {
        action,
        sources: [entry.path],
        target: `${parent === '/' ? '' : parent}/${dialogValue.value.trim()}`,
        expectedResourceVersion: entry.resourceVersion,
      }
    } else if (action === 'trash') {
      input = {
        action,
        sources: dialogEntries.value.map((entry) => entry.path),
        expectedResourceVersions: Object.fromEntries(
          dialogEntries.value.map((entry) => [entry.path, entry.resourceVersion]),
        ),
      }
    } else if (action === 'chmod') {
      input = {
        action,
        sources: dialogEntries.value.map((entry) => entry.path),
        mode: dialogValue.value.trim(),
        expectedResourceVersions: Object.fromEntries(
          dialogEntries.value.map((entry) => [entry.path, entry.resourceVersion]),
        ),
      }
    } else if (action === 'compress') {
      input = {
        action,
        sources: dialogEntries.value.map((entry) => entry.path),
        target: currentPath.value,
        name: normalizedArchiveName(dialogValue.value, dialogFormat.value),
        format: dialogFormat.value,
        expectedResourceVersions: Object.fromEntries(
          dialogEntries.value.map((entry) => [entry.path, entry.resourceVersion]),
        ),
      }
    } else if (action === 'extract') {
      const entry = dialogEntries.value[0]
      if (!entry) throw new Error('请选择需要解压的文件。')
      input = {
        action,
        sources: [entry.path],
        target: currentPath.value,
        name: dialogValue.value.trim(),
        format: dialogFormat.value,
        expectedResourceVersion: entry.resourceVersion,
      }
    } else {
      const unsupportedAction: never = action
      throw new Error(`不支持的文件操作：${unsupportedAction}`)
    }
    const result = controller
      ? await fileAPI.value.action(input, controller.signal)
      : await fileAPI.value.action(input)
    const shortcutSyncFailed = await applySuccessfulFileChanges(result, undefined, hostId)
    if (result.failed.length || shortcutSyncFailed) {
      toast.danger(
        shortcutSyncFailed ? '文件已处理，快捷方式未同步' : result.succeeded.length ? '部分文件未处理' : '文件操作未完成',
        shortcutSyncFailed
          ? '真实文件操作已完成，请刷新桌面后重试路径同步。'
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`,
      )
    } else {
      toast.success(
        action === 'trash'
          ? '已移入回收站'
          : action === 'compress'
            ? '压缩完成'
            : action === 'extract'
              ? '解压完成'
              : '文件操作完成',
        `${Math.max(result.succeeded.length, 1)} 项已处理`,
      )
    }
    dialogAction.value = undefined
    dialogValue.value = ''
    dialogEntries.value = []
  } catch (error) {
    if (controller?.signal.aborted) {
      if (!unmounted) toast.success('操作已停止', '未完成的临时文件已清理。')
      dialogAction.value = undefined
      dialogValue.value = ''
      dialogEntries.value = []
    } else {
      toast.danger('文件操作失败', errorMessage(error))
    }
    if (!unmounted) await loadDirectory()
  } finally {
    if (archiveController === controller) archiveController = undefined
    dialogBusy.value = false
  }
}

async function openTrash(): Promise<void> {
  trashOpen.value = true
  selectedTrash.value = new Set()
  await loadTrash()
}

async function loadTrash(): Promise<void> {
  const hostId = fileHostId.value
  trashLoading.value = true
  try {
    const result = await fileAPI.value.trash()
    if (unmounted || hostId !== fileHostId.value) return
    trashEntries.value = result.entries
    trashTotal.value = result.total
    trashTruncated.value = result.truncated
    selectedTrash.value = new Set(
      [...selectedTrash.value].filter((id) => result.entries.some((entry) => entry.id === id)),
    )
  } catch (error) {
    toast.danger('回收站读取失败', errorMessage(error))
  } finally {
    trashLoading.value = false
  }
}

function toggleTrash(id: string): void {
  const next = new Set(selectedTrash.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedTrash.value = next
}

function toggleAllTrash(): void {
  selectedTrash.value = allTrashSelected.value
    ? new Set()
    : new Set(trashEntries.value.map((entry) => entry.id))
}

async function runTrashAction(action: 'trash_restore' | 'trash_delete' | 'trash_empty'): Promise<void> {
  const chosen = trashEntries.value.filter((entry) => selectedTrash.value.has(entry.id))
  if (action !== 'trash_empty' && !chosen.length) return
  if (action === 'trash_delete' && !window.confirm(`彻底删除选中的 ${chosen.length} 项？此操作不可恢复。`)) return
  if (action === 'trash_empty' && !window.confirm(`清空回收站中的 ${trashTotal.value} 项？此操作不可恢复。`)) return
  trashBusy.value = true
  try {
    const input: FileActionInput = {
      action,
      trashIds: action === 'trash_empty' ? undefined : chosen.map((entry) => entry.id),
      expectedResourceVersions:
        action === 'trash_empty'
          ? undefined
          : Object.fromEntries(chosen.map((entry) => [entry.id, entry.resourceVersion])),
    }
    const result = await fileAPI.value.action(input)
    if (result.failed.length) {
      toast.danger(
        result.succeeded.length ? '部分回收站项目未处理' : '回收站操作失败',
        `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`,
      )
    } else {
      const title = action === 'trash_restore' ? '恢复完成' : action === 'trash_empty' ? '回收站已清空' : '已彻底删除'
      toast.success(title, `${result.succeeded.length} 项已处理`)
    }
    selectedTrash.value = new Set()
    await Promise.all([loadTrash(), loadDirectory()])
  } catch (error) {
    toast.danger('回收站操作失败', errorMessage(error))
  } finally {
    trashBusy.value = false
  }
}

async function download(entry: FileEntry): Promise<void> {
  contextMenu.value = undefined
  try {
    await downloadFileEntries([entry], entry.name, fileHostId.value)
  } catch (error) {
    toast.danger('下载失败', errorMessage(error))
  }
}

async function downloadSelected(entry?: FileEntry): Promise<void> {
  contextMenu.value = undefined
  const targets = entriesForBatch(entry)
  if (!targets.length || targets.some((target) => !canAddToDesktop(target))) {
    toast.danger('下载失败', '只能下载普通文件或文件夹')
    return
  }
  try {
    await downloadFileEntries(
      targets,
      currentDirectoryEntry().name,
      fileHostId.value,
    )
  } catch (error) {
    toast.danger('下载失败', errorMessage(error))
  }
}

function setSort(key: 'name' | 'size' | 'modified'): void {
  if (sortKey.value === key) sortDescending.value = !sortDescending.value
  else {
    sortKey.value = key
    sortDescending.value = false
  }
}

function openRemoteDownloadDialog(): void {
  if (isRemoteFileHost.value) {
    toast.show('远端主机暂不支持远程下载', { message: '远程下载任务属于当前 KPanel 的面板级能力。' })
    return
  }
  if (remoteDownloadSubmitting.value) return
  remoteDownloadTarget.value = currentPath.value
  remoteDownloadURL.value = ''
  remoteDownloadName.value = ''
  remoteDownloadFormErrorCode.value = undefined
  remoteDownloadDialogOpen.value = true
  void nextTick().then(() => nextTick()).then(() => remoteDownloadURLInput.value?.focus({ preventScroll: true }))
}

function closeRemoteDownloadDialog(): void {
  remoteDownloadDialogOpen.value = false
  remoteDownloadURL.value = ''
  remoteDownloadName.value = ''
  remoteDownloadFormErrorCode.value = undefined
}

function validRemoteDownloadForm(): boolean {
  const rawURL = remoteDownloadURL.value.trim()
  try {
    const parsed = new URL(rawURL)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') throw new Error('scheme')
  } catch {
    remoteDownloadFormErrorCode.value = 'url'
    return false
  }
  const name = remoteDownloadName.value.trim()
  if (
    name && (
      name === '.' || name === '..' || /[/\\\u0000-\u001f\u007f]/.test(name)
      || new TextEncoder().encode(name).length > 255
    )
  ) {
    remoteDownloadFormErrorCode.value = 'name'
    return false
  }
  remoteDownloadFormErrorCode.value = undefined
  return true
}

function setRemoteDownloadJobPending(id: string, pending: boolean): void {
  const next = new Set(remoteDownloadPendingActions.value)
  if (pending) next.add(id)
  else next.delete(id)
  remoteDownloadPendingActions.value = next
}

function stopRemoteDownloadPolling(): void {
  if (remoteDownloadPollTimer !== undefined) window.clearTimeout(remoteDownloadPollTimer)
  remoteDownloadPollTimer = undefined
  remoteDownloadJobsController?.abort()
  remoteDownloadJobsController = undefined
  remoteDownloadJobsLoading.value = false
}

function scheduleRemoteDownloadPoll(delay = remoteDownloadPollDelay): void {
  if (remoteDownloadPollTimer !== undefined) window.clearTimeout(remoteDownloadPollTimer)
  remoteDownloadPollTimer = undefined
  if (unmounted) return
  remoteDownloadPollTimer = window.setTimeout(() => {
    remoteDownloadPollTimer = undefined
    void loadRemoteDownloadJobs(true)
  }, delay)
}

function replaceRemoteDownloadJobs(jobs: FileRemoteDownloadJob[]): void {
  const previous = new Map(remoteDownloadJobs.value.map((job) => [job.id, job]))
  const initialLoad = !remoteDownloadJobsInitialized
  remoteDownloadJobsInitialized = true
  remoteDownloadJobs.value = jobs
  const targetsToReconcile = new Set<string>()
  for (const job of jobs) {
    const earlier = previous.get(job.id)
    if (earlier && isRemoteDownloadJobActive(earlier) && !isRemoteDownloadJobActive(job)) {
      targetsToReconcile.add(job.targetDirectory)
      continue
    }
    if (!earlier && initialLoad && !isRemoteDownloadJobActive(job)) {
      const directoryReadAt = Date.parse(directory.value?.readAt || '')
      const jobFinishedAt = Date.parse(job.finishedAt || job.updatedAt)
      if (
        job.targetDirectory === currentPath.value
        && Number.isFinite(directoryReadAt)
        && Number.isFinite(jobFinishedAt)
        && jobFinishedAt > directoryReadAt
      ) targetsToReconcile.add(job.targetDirectory)
    }
  }
  for (const target of targetsToReconcile) reconcileRemoteDownloadTarget(target)
}

function upsertRemoteDownloadJob(job: FileRemoteDownloadJob): void {
  remoteDownloadJobs.value = [job, ...remoteDownloadJobs.value.filter((item) => item.id !== job.id)]
}

async function loadRemoteDownloadJobs(silent = false): Promise<void> {
  if (isRemoteFileHost.value) {
    stopRemoteDownloadPolling()
    remoteDownloadJobs.value = []
    remoteDownloadJobsError.value = undefined
    return
  }
  if (remoteDownloadPollTimer !== undefined) window.clearTimeout(remoteDownloadPollTimer)
  remoteDownloadPollTimer = undefined
  remoteDownloadJobsController?.abort()
  const controller = new AbortController()
  remoteDownloadJobsController = controller
  if (!silent) remoteDownloadJobsLoading.value = true
  try {
    const result = await api.files.remoteDownloadJobs(controller.signal)
    if (unmounted || controller.signal.aborted || remoteDownloadJobsController !== controller) return
    replaceRemoteDownloadJobs(result.items)
    remoteDownloadJobsError.value = undefined
  } catch (error) {
    if (unmounted || controller.signal.aborted || remoteDownloadJobsController !== controller) return
    setRemoteDownloadJobsError(error)
  } finally {
    if (remoteDownloadJobsController === controller) {
      remoteDownloadJobsController = undefined
      remoteDownloadJobsLoading.value = false
      if (remoteDownloadJobsError.value) scheduleRemoteDownloadPoll(remoteDownloadPollRetryDelay)
      else if (activeRemoteDownloadCount.value > 0) scheduleRemoteDownloadPoll()
    }
  }
}

function reconcileRemoteDownloadTarget(target: string): void {
  notifyFileDirectoriesChanged([target], fileWindowChangeOrigin)
  if (unmounted) return
  if (directoryController) {
    queuedRemoteDownloadRefreshes.add(target)
    return
  }
  if (currentPath.value !== target) return
  void loadDirectory(target)
}

function normalizedRemoteDownloadOrigin(value: string): string {
  try {
    return new URL(value).origin
  } catch {
    return ''
  }
}

async function reconcileFailedRemoteDownloadSubmission(
  knownJobIDs: Set<string>,
  sourceOrigin: string,
  target: string,
  requestedName: string,
  submittedAt: number,
): Promise<FileRemoteDownloadJob | undefined> {
  stopRemoteDownloadPolling()
  const controller = new AbortController()
  remoteDownloadJobsController = controller
  try {
    const result = await api.files.remoteDownloadJobs(controller.signal)
    if (unmounted || controller.signal.aborted || remoteDownloadJobsController !== controller) return undefined
    const reconciledAt = Date.now()
    const recovered = result.items.find((job) => {
      const createdAt = Date.parse(job.createdAt)
      return !knownJobIDs.has(job.id)
        && normalizedRemoteDownloadOrigin(job.source) === sourceOrigin
        && job.targetDirectory === target
        && (!requestedName || job.name === requestedName)
        && Number.isFinite(createdAt)
        && createdAt >= submittedAt - remoteDownloadSubmissionClockSkew
        && createdAt <= reconciledAt + remoteDownloadSubmissionClockSkew
    })
    replaceRemoteDownloadJobs(result.items)
    return recovered
  } catch {
    return undefined
  } finally {
    if (remoteDownloadJobsController === controller) remoteDownloadJobsController = undefined
  }
}

async function submitRemoteDownload(): Promise<void> {
  if (remoteDownloadSubmitting.value || !validRemoteDownloadForm()) return
  const sourceURL = remoteDownloadURL.value.trim()
  const requestedName = remoteDownloadName.value.trim()
  const target = remoteDownloadTarget.value
  const sourceOrigin = normalizedRemoteDownloadOrigin(sourceURL)
  const knownJobIDs = new Set(remoteDownloadJobs.value.map((job) => job.id))
  const submittedAt = Date.now()
  remoteDownloadSubmitting.value = true
  remoteDownloadJobsError.value = undefined
  stopRemoteDownloadPolling()
  try {
    const job = await api.files.createRemoteDownloadJob({
      url: sourceURL, targetDirectory: target, ...(requestedName ? { name: requestedName } : {}),
    })
    if (unmounted) return
    upsertRemoteDownloadJob(job)
    closeRemoteDownloadDialog()
    scheduleRemoteDownloadPoll(800)
  } catch (error) {
    if (unmounted) return
    const recovered = await reconcileFailedRemoteDownloadSubmission(
      knownJobIDs, sourceOrigin, target, requestedName, submittedAt,
    )
    if (unmounted) return
    if (recovered) {
      remoteDownloadJobsError.value = undefined
      closeRemoteDownloadDialog()
      if (isRemoteDownloadJobActive(recovered)) scheduleRemoteDownloadPoll(800)
      else reconcileRemoteDownloadTarget(recovered.targetDirectory)
      return
    }
    setRemoteDownloadJobsError(error)
    toast.danger(i18n.t('files.remoteDownload.createFailed'), remoteDownloadJobsErrorMessage.value)
  } finally {
    if (!unmounted) {
      remoteDownloadSubmitting.value = false
      if (activeRemoteDownloadCount.value > 0 && remoteDownloadPollTimer === undefined) {
        scheduleRemoteDownloadPoll()
      }
    }
  }
}

async function cancelRemoteDownloadJob(job: FileRemoteDownloadJob): Promise<void> {
  if (!isRemoteDownloadJobActive(job) || remoteDownloadPendingActions.value.has(job.id)) return
  setRemoteDownloadJobPending(job.id, true)
  remoteDownloadJobsError.value = undefined
  stopRemoteDownloadPolling()
  try {
    await api.files.cancelRemoteDownloadJob(job.id)
  } catch (error) {
    if (!unmounted) setRemoteDownloadJobsError(error)
  } finally {
    if (!unmounted) {
      setRemoteDownloadJobPending(job.id, false)
      await loadRemoteDownloadJobs(true)
    }
  }
}

async function deleteRemoteDownloadJob(job: FileRemoteDownloadJob): Promise<void> {
  if (isRemoteDownloadJobActive(job) || remoteDownloadPendingActions.value.has(job.id)) return
  setRemoteDownloadJobPending(job.id, true)
  remoteDownloadJobsError.value = undefined
  stopRemoteDownloadPolling()
  let deleted = false
  try {
    await api.files.deleteRemoteDownloadJob(job.id)
    deleted = true
    if (!unmounted) remoteDownloadJobs.value = remoteDownloadJobs.value.filter((item) => item.id !== job.id)
  } catch (error) {
    if (!unmounted) setRemoteDownloadJobsError(error)
  } finally {
    if (!unmounted) {
      setRemoteDownloadJobPending(job.id, false)
      if (deleted) await loadRemoteDownloadJobs(true)
      else scheduleRemoteDownloadPoll(
        activeRemoteDownloadCount.value > 0 ? remoteDownloadPollDelay : remoteDownloadPollRetryDelay,
      )
    }
  }
}

function uploadTaskName(source: UploadTaskSource): string {
  if (source.kind === 'file') return source.file.name
  return source.manifest.roots.length === 1
    ? source.manifest.roots[0]!.name
    : `${source.manifest.roots.length} 项`
}

function createUploadTask(source: UploadTaskSource): UploadTask {
  uploadTaskSequence += 1
  const task: UploadTask = {
    id: `upload-${uploadTaskSequence}`,
    name: uploadTaskName(source),
    target: source.target,
    progress: 0,
    phase: 'running',
  }
  uploadTaskSources.set(task.id, source)
  uploadTasks.value = [...uploadTasks.value, task]
  return task
}

function updateUploadTask(id: string, update: Partial<Omit<UploadTask, 'id'>>): void {
  uploadTasks.value = uploadTasks.value.map((task) => task.id === id ? { ...task, ...update } : task)
}

function dismissUploadTask(id: string): void {
  const timer = uploadTaskClearTimers.get(id)
  if (timer !== undefined) window.clearTimeout(timer)
  uploadTaskClearTimers.delete(id)
  uploadTaskSources.delete(id)
  uploadTasks.value = uploadTasks.value.filter((task) => task.id !== id)
}

function scheduleUploadTaskClear(id: string): void {
  const previous = uploadTaskClearTimers.get(id)
  if (previous !== undefined) window.clearTimeout(previous)
  uploadTaskClearTimers.set(id, window.setTimeout(() => dismissUploadTask(id), 1800))
}

function uploadTaskStatus(task: UploadTask): string {
  if (task.phase === 'running') return '上传中'
  if (task.phase === 'success') return '上传完成'
  return '上传失败'
}

async function runFileUploadTask(id: string, source: Extract<UploadTaskSource, { kind: 'file' }>): Promise<void> {
  updateUploadTask(id, { phase: 'running', progress: 0, detail: undefined })
  const onProgress = (progress: number): void => updateUploadTask(id, {
    progress: Math.max(0, Math.min(100, progress)),
  })
  try {
    await fileAPIForHost(source.hostId).upload(source.target, source.file, false, onProgress)
  } catch (error) {
    if (
      error instanceof ApiError
      && error.status === 409
      && window.confirm(`${source.file.name} 已存在，是否覆盖？`)
    ) {
      try {
        await fileAPIForHost(source.hostId).upload(source.target, source.file, true, onProgress)
      } catch (overwriteError) {
        updateUploadTask(id, { phase: 'error', detail: errorMessage(overwriteError) })
        return
      }
    } else {
      updateUploadTask(id, { phase: 'error', detail: errorMessage(error) })
      return
    }
  }
  updateUploadTask(id, { phase: 'success', progress: 100, detail: undefined })
  scheduleUploadTaskClear(id)
}

async function uploadFiles(
  files: FileList | File[],
  target = currentPath.value,
  hostId = fileHostId.value,
): Promise<void> {
  const values = Array.from(files)
  if (!values.length) return
  for (const file of values) {
    const source = { kind: 'file', file, target, hostId } as const
    const task = createUploadTask(source)
    await runFileUploadTask(task.id, source)
  }
  if (uploadInput.value) uploadInput.value.value = ''
  if (!unmounted && fileHostId.value === hostId && currentPath.value === target) await loadDirectory(target)
}

function externalUploadErrorMessage(error: unknown): string {
  if (!(error instanceof DesktopExternalDropError)) return errorMessage(error)
  switch (error.code) {
    case 'too_many': return i18n.t('desktop.externalDropErrorTooMany')
    case 'too_large': return i18n.t('desktop.externalDropErrorTooLarge')
    case 'too_deep': return i18n.t('desktop.externalDropErrorTooDeep')
    case 'invalid': return i18n.t('desktop.externalDropErrorInvalid')
    default: return i18n.t('desktop.externalDropErrorUnsupported')
  }
}

async function runDirectoryUploadTask(
  id: string,
  manifest: ExternalDropManifest,
  target: string,
  signal: AbortSignal,
  hostId: string,
): Promise<void> {
  updateUploadTask(id, { phase: 'running', progress: 0, detail: undefined })
  try {
    const result = await uploadExternalDrop(manifest, fileAPIForHost(hostId), signal, (progress) => {
      const percent = progress.totalBytes > 0
        ? Math.round(progress.loadedBytes / progress.totalBytes * 100)
        : progress.totalFiles > 0
          ? Math.round(progress.completedFiles / progress.totalFiles * 100)
          : 100
      updateUploadTask(id, { progress: Math.max(0, Math.min(100, percent)) })
    }, target)
    if (signal.aborted || unmounted) return
    if (result.failed.length) {
      updateUploadTask(id, { phase: 'error', detail: result.failed[0]!.detail })
    } else {
      updateUploadTask(id, { phase: 'success', progress: 100, detail: undefined })
      scheduleUploadTaskClear(id)
    }
  } catch (error) {
    if (signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) return
    updateUploadTask(id, { phase: 'error', detail: externalUploadErrorMessage(error) })
  } finally {
    if (!unmounted) {
      if (fileHostId.value === hostId && currentPath.value === target) await loadDirectory(target)
      notifyFileDirectoriesChanged([target], fileWindowChangeOrigin, [], hostId)
    }
  }
}

async function uploadDirectoryManifest(
  manifest: ExternalDropManifest,
  target: string,
  signal: AbortSignal,
  hostId = fileHostId.value,
): Promise<void> {
  const source = { kind: 'directory', manifest, target, hostId } as const
  const task = createUploadTask(source)
  await runDirectoryUploadTask(task.id, manifest, target, signal, source.hostId)
}

async function retryUploadTask(id: string): Promise<void> {
  const task = uploadTasks.value.find((item) => item.id === id)
  const source = uploadTaskSources.get(id)
  if (!task || task.phase !== 'error' || !source) return
  // Retrying after navigation must never target a different host.
  if (source.hostId !== fileHostId.value) return
  if (source.kind === 'file') {
    await runFileUploadTask(id, source)
    if (!unmounted && fileHostId.value === source.hostId && currentPath.value === source.target) await loadDirectory(source.target)
    return
  }
  if (externalUploadController) {
    toast.show('已有文件操作正在进行')
    return
  }
  const controller = new AbortController()
  externalUploadController = controller
  try {
    await runDirectoryUploadTask(id, source.manifest, source.target, controller.signal, source.hostId)
  } finally {
    if (externalUploadController === controller) externalUploadController = undefined
  }
}

async function onDrop(event: DragEvent): Promise<void> {
  const target = currentPath.value
  const hostId = fileHostId.value
  dragging.value = false
  if (hasDesktopFileDrag(event)) {
    if (desktopFileDragOrigin(event) === 'desktop-shortcut') {
      clearInternalDropTarget()
      return
    }
    void transferInternalFileDrop(event, currentPath.value)
    return
  }
  if (hasCrossPanelFileDrag(event)) {
    void transferCrossPanelFileDrop(event, currentPath.value)
    return
  }
  clearInternalDropTarget()
  const dataTransfer = event.dataTransfer
  if (!dataTransfer) return
  if (externalUploadController) {
    toast.show('已有文件操作正在进行')
    return
  }
  const controller = new AbortController()
  externalUploadController = controller
  try {
    const manifest = await collectExternalDrop(dataTransfer, controller.signal)
    if (manifest.roots.every((root) => root.kind === 'file')) {
      externalUploadController = undefined
      await uploadFiles(manifest.files.map((item) => item.file), target, hostId)
      return
    }
    await uploadDirectoryManifest(manifest, target, controller.signal, hostId)
  } catch (error) {
    if (!controller.signal.aborted && !(error instanceof DOMException && error.name === 'AbortError')) {
      toast.danger('文件上传失败', externalUploadErrorMessage(error))
    }
  } finally {
    if (externalUploadController === controller) externalUploadController = undefined
  }
}

function onUploadDragEnter(event: DragEvent): void {
  if (hasDesktopFileDrag(event) || hasCrossPanelFileDrag(event)) return
  if (event.dataTransfer?.files?.length || Array.from(event.dataTransfer?.types || []).includes('Files')) {
    dragging.value = true
  }
}

function canShowThumbnail(entry: FileEntry): boolean {
  return entry.kind === 'file' &&
    entry.sizeBytes > 0 &&
    entry.sizeBytes <= thumbnailSourceMaxBytes &&
    ['image/jpeg', 'image/png', 'image/gif'].includes(entry.mime || '') &&
    !thumbnailFailures.value.has(entry.path)
}

function thumbnailURL(entry: FileEntry): string {
  return fileAPI.value.thumbnailUrl(entry.path, entry.resourceVersion)
}

function markThumbnailFailed(path: string): void {
  const next = new Set(thumbnailFailures.value)
  next.add(path)
  thumbnailFailures.value = next
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes)) return '—'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes / 1024
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${units[index]}`
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '—'
    : new Intl.DateTimeFormat('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      }).format(date)
}

function handleWindowClick(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (!target.closest('.file-context-menu')) contextMenu.value = undefined
  if (!target.closest('.file-host-switcher')) closeFileHostPicker()
}

function closeContextMenuOnViewportChange(): void {
  contextMenu.value = undefined
}

function closeContextMenuOnScroll(event: Event): void {
  if (contextMenuElement.value?.contains(event.target as Node)) return
  contextMenu.value = undefined
}

function handleFileShortcut(event: KeyboardEvent): void {
  if (!desktopWindowActive.value) return
  if (event.key === 'Escape' && fileHostPickerOpen.value) {
    event.preventDefault()
    closeFileHostPicker(true)
    return
  }
  const target = event.target as HTMLElement | null
  const focusInside = filesPage.value?.contains(document.activeElement)
  if (!filesPage.value || (!filesPage.value.contains(target) && !focusInside)) return
  if (
    previewEntry.value ||
    target?.matches('input, textarea, select, button, a, [role="menuitem"], [contenteditable="true"]')
  ) {
    return
  }
  const key = event.key.toLocaleLowerCase()
  if ((event.ctrlKey || event.metaKey) && key === 'a' && entries.value.length) {
    event.preventDefault()
    selected.value = new Set(entries.value.map((entry) => entry.path))
    selectionAnchor.value = entries.value[0]?.path
  } else if ((event.ctrlKey || event.metaKey) && key === 'c' && selectedEntries.value.length) {
    event.preventDefault()
    setClipboard('copy')
  } else if ((event.ctrlKey || event.metaKey) && key === 'x' && selectedEntries.value.length) {
    event.preventDefault()
    setClipboard('move')
  } else if ((event.ctrlKey || event.metaKey) && key === 'v' && clipboard.value?.entries.length) {
    event.preventDefault()
    void pasteClipboard()
  } else if (event.key === 'Delete' && selectedEntries.value.length) {
    event.preventDefault()
    openDialog('trash')
  } else if (event.key === 'Escape') {
    contextMenu.value = undefined
  }
}

function focusFilesPage(event: PointerEvent): void {
  const target = event.target as HTMLElement
  if (target.closest('button, a, input, textarea, select, [contenteditable="true"]')) return
  filesPage.value?.focus({ preventScroll: true })
}

watch(desktopWindowActive, (active) => {
  if (!active) contextMenu.value = undefined
  if (!active) closeFileHostPicker()
})

onMounted(() => {
  unsubscribeClusterHostOrder = subscribeClusterHostOrder(() => {
    clusterHostOrderRevision.value += 1
  })
  const guard = () => !previewDirty.value || window.confirm('文件尚未保存，确认关闭窗口吗？')
  unregisterWindowCloseGuard = desktopWindowCloseGuards
    ? desktopWindowCloseGuards.register(guard)
    : desktopCloseGuardCoordinator.register('classic-files', guard)
  window.addEventListener('click', handleWindowClick)
  window.addEventListener('keydown', handleFileShortcut)
  window.addEventListener('resize', closeContextMenuOnViewportChange)
  document.addEventListener('scroll', closeContextMenuOnScroll, true)
  window.visualViewport?.addEventListener?.('resize', closeContextMenuOnViewportChange)
  window.visualViewport?.addEventListener?.('scroll', closeContextMenuOnViewportChange)
  unsubscribeFileDirectoryChanges = subscribeFileDirectoryChanges((directories, origin, moves = [], hostId = '') => {
    if (hostId !== fileHostId.value || origin === fileWindowChangeOrigin) return
    const relocatedPath = remapMovedFilePath(currentPath.value, moves)
    if (relocatedPath !== currentPath.value) {
      void navigateDirectory(relocatedPath)
      return
    }
    if (!directories.has(currentPath.value)) return
    void loadDirectory()
  })
  restoreViewMode()
  void loadFileHosts()
  void loadRequestedRoute().finally(() => {
    if (!unmounted) void loadRemoteDownloadJobs()
  })
})

watch(
  () => [route.query.path, route.query.file, route.query.hostId] as const,
  ([pathValue, fileValue, hostValue], previous) => {
    const hostId = typeof hostValue === 'string' ? hostValue : ''
    const hostChanged = hostId !== fileHostId.value
    if (hostChanged) {
      if (!resetFileHostContext(hostId)) {
        void router.push({ name: 'files', query: { path: currentPath.value, ...(fileHostId.value ? { hostId: fileHostId.value } : {}) } })
        return
      }
      stopRemoteDownloadPolling()
      remoteDownloadJobs.value = []
    }
    if (!hostChanged && pathValue === previous?.[0] && fileValue === previous?.[1]) return
    const directoryPath = requestedFilePath(pathValue) || '/'
    void (async () => {
      if (hostChanged || directoryPath !== currentPath.value) await loadDirectory(directoryPath)
      if (unmounted || hostId !== fileHostId.value) return
      if (hostChanged || fileValue !== previous?.[1]) {
        openedRouteFile = ''
        await openRequestedFile(fileValue)
      }
    })()
  },
)

watch(search, () => {
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    void loadDirectory(currentPath.value)
  }, 250)
})

onBeforeUnmount(() => {
  unregisterWindowCloseGuard?.()
  unsubscribeClusterHostOrder?.()
  unsubscribeFileDirectoryChanges?.()
  clearDesktopFileDrag(fileWindowChangeOrigin)
  fileHostController?.abort()
  unmounted = true
  queuedRemoteDownloadRefreshes.clear()
  stopRemoteDownloadPolling()
  directoryController?.abort()
  archiveController?.abort()
  externalUploadController?.abort()
  fileTransferController?.abort()
  if (fileTransferClearTimer !== undefined) window.clearTimeout(fileTransferClearTimer)
  uploadTaskClearTimers.forEach((timer) => window.clearTimeout(timer))
  uploadTaskClearTimers.clear()
  uploadTaskSources.clear()
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  clearMediaLoadTimer()
  window.removeEventListener('click', handleWindowClick)
  window.removeEventListener('keydown', handleFileShortcut)
  window.removeEventListener('resize', closeContextMenuOnViewportChange)
  document.removeEventListener('scroll', closeContextMenuOnScroll, true)
  window.visualViewport?.removeEventListener?.('resize', closeContextMenuOnViewportChange)
  window.visualViewport?.removeEventListener?.('scroll', closeContextMenuOnViewportChange)
})
</script>

<template>
  <section ref="filesPage" class="files-page" tabindex="-1" @pointerdown="focusFilesPage">
    <PageHeader title="文件管理" description="浏览、编辑和传输服务器文件；KPanel 凭据与状态目录保持隔离。" />

    <div class="file-command-bar">
      <nav class="file-shortcuts" aria-label="常用目录">
        <button v-for="item in ['/', '/home', '/root', '/etc', '/var']" :key="item" type="button" @click="navigateDirectory(item)">
          {{ item === '/' ? '根目录 /' : item }}
        </button>
      </nav>
      <div class="file-command-bar__actions">
        <button class="button button--secondary button--small file-command-bar__refresh" type="button" :disabled="loading" title="刷新目录" aria-label="刷新目录" @click="loadDirectory()">
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
        <button class="button button--secondary button--small" type="button" title="打开回收站" aria-label="打开回收站" @click="openTrash">
          <Trash2 :size="15" /> 回收站
        </button>
        <button
          class="button button--secondary button--small"
          type="button"
          :disabled="isRemoteFileHost"
          :title="isRemoteFileHost ? '远端主机暂不支持分享管理' : '分享管理'"
          aria-label="分享管理"
          @click="openShareManager"
        >
          <Share2 :size="15" /> 分享管理
        </button>
        <button class="button button--secondary button--small" type="button" title="新建目录" aria-label="新建目录" @click="openDialog('mkdir')">
          <Plus :size="15" /> 新建目录
        </button>
        <button
          class="button button--secondary button--small"
          type="button"
          :title="i18n.t('files.remoteDownload.tooltip')"
          :aria-label="i18n.t('files.remoteDownload.tooltip')"
          :disabled="remoteDownloadSubmitting || isRemoteFileHost"
          @click="openRemoteDownloadDialog"
        >
          <Download :size="15" /> {{ i18n.t('files.remoteDownload.label') }}
        </button>
        <button class="button button--primary button--small" type="button" @click="uploadInput?.click()">
          <Upload :size="15" /> 上传文件
        </button>
        <input
          ref="uploadInput"
          class="sr-only"
          type="file"
          aria-label="选择上传文件"
          multiple
          @change="($event.target as HTMLInputElement).files && uploadFiles(($event.target as HTMLInputElement).files!)"
        />
      </div>
    </div>

    <section
      class="file-browser"
      :class="{
        'file-browser--dragging': dragging,
        'file-browser--internal-drop': internalDropTarget === currentPath,
      }"
      @dragenter.prevent="onUploadDragEnter"
      @dragover="onFileBrowserDragOver"
      @dragleave="onFileBrowserDragLeave"
      @drop.prevent="onDrop"
      @contextmenu="showDirectoryContext"
    >
      <header class="file-toolbar">
        <div class="file-toolbar__path">
          <div class="file-host-switcher" data-file-host-switcher>
            <button
              ref="fileHostPickerButton"
              class="file-host-switcher__trigger"
              type="button"
              aria-haspopup="menu"
              aria-controls="file-host-switcher-menu"
              :aria-expanded="fileHostPickerOpen"
              :title="phrase('切换主机')"
              @click.stop="toggleFileHostPicker"
            >
              <Server :size="15" aria-hidden="true" />
              <span>{{ phrase('当前主机') }}</span>
              <strong>{{ activeFileHostLabel }}</strong>
              <ChevronDown :size="15" aria-hidden="true" />
            </button>
            <div
              v-if="fileHostPickerOpen"
              id="file-host-switcher-menu"
              class="file-host-switcher__menu"
              role="menu"
              :aria-label="phrase('切换主机')"
              @click.stop
            >
              <strong class="file-host-switcher__heading">{{ phrase('切换主机') }}</strong>
              <div v-if="fileHostInventoryLoading" class="file-host-switcher__message" role="status" aria-live="polite">
                <RefreshCw :size="15" class="spinning" aria-hidden="true" />
                <span>{{ phrase('正在读取主机列表…') }}</span>
              </div>
              <div v-else-if="fileHostInventoryError && !fileHosts.length" class="file-host-switcher__message file-host-switcher__message--error" role="alert">
                <CircleAlert :size="15" aria-hidden="true" />
                <span>{{ phrase('无法读取集群主机') }}</span>
              </div>
              <template v-else-if="fileHosts.length">
                <button
                  v-for="host in fileHosts"
                  :key="host.id"
                  class="file-host-switcher__item"
                  :class="{
                    'is-active': host.id === activeFileHostId,
                    'is-manage': fileHostStatus(host).action === 'manage',
                  }"
                  type="button"
                  role="menuitem"
                  :aria-current="host.id === activeFileHostId ? 'true' : undefined"
                  :title="fileHostStatus(host).action === 'open' ? phrase('打开远端文件管理') : undefined"
                  :data-file-host-id="host.id"
                  @click="handleFileHostSelection(host)"
                >
                  <OperatingSystemIcon
                    class="file-host-switcher__os"
                    :distro="hostOperatingSystemIdentity(host).key"
                    :label="hostOperatingSystemIdentity(host).label"
                  />
                  <span>
                    <strong>{{ host.isLocal ? phrase('本机') : host.name }}</strong>
                    <small>{{ fileHostStatus(host).label }}</small>
                  </span>
                  <Check v-if="host.id === activeFileHostId" :size="16" aria-hidden="true" />
                  <ExternalLink v-else-if="fileHostStatus(host).action === 'open'" :size="15" aria-hidden="true" />
                  <ChevronRight v-else :size="15" aria-hidden="true" />
                </button>
              </template>
              <div v-else class="file-host-switcher__message">
                <span>{{ phrase('暂未发现集群主机') }}</span>
              </div>
              <button
                v-if="fileHostInventoryError && fileHosts.length"
                class="file-host-switcher__retry"
                type="button"
                @click="loadFileHosts"
              >
                <CircleAlert :size="14" aria-hidden="true" />
                {{ phrase('主机列表刷新失败，点击重试') }}
              </button>
            </div>
          </div>
          <nav class="breadcrumbs" aria-label="文件路径">
            <button
              v-for="(item, index) in breadcrumbs"
              :key="item.path"
              type="button"
              :disabled="item.path === currentPath"
              @click="navigateDirectory(item.path)"
            >
              <HardDrive v-if="index === 0" :size="15" />
              <span>{{ item.name }}</span>
              <ChevronRight v-if="index < breadcrumbs.length - 1" :size="14" />
            </button>
          </nav>
        </div>
        <div class="file-toolbar__controls">
          <label class="file-search">
            <Search :size="16" />
            <input v-model="search" type="search" aria-label="搜索当前目录" placeholder="搜索当前目录" />
            <button v-if="search" type="button" aria-label="清除搜索" @click="search = ''">
              <X :size="14" />
            </button>
          </label>
          <div v-if="viewMode === 'grid'" class="file-grid-sort">
            <select v-model="sortKey" aria-label="大图标排序方式">
              <option value="name">名称</option>
              <option value="modified">修改时间</option>
              <option value="size">大小</option>
            </select>
            <button
              type="button"
              :aria-label="sortDescending ? '切换为升序' : '切换为降序'"
              :title="sortDescending ? '当前降序' : '当前升序'"
              @click="sortDescending = !sortDescending"
            >{{ sortDescending ? '↓' : '↑' }}</button>
          </div>
          <div class="file-view-switch" role="group" aria-label="文件排版方式">
            <button
              type="button"
              :class="{ 'is-active': viewMode === 'list' }"
              :aria-pressed="viewMode === 'list'"
              aria-label="列表排版"
              title="列表排版"
              @click="setViewMode('list')"
            ><List :size="17" /></button>
            <button
              type="button"
              :class="{ 'is-active': viewMode === 'grid' }"
              :aria-pressed="viewMode === 'grid'"
              aria-label="大图标排版"
              title="大图标排版"
              @click="setViewMode('grid')"
            ><LayoutGrid :size="17" /></button>
          </div>
        </div>
      </header>

      <div v-if="fileStatusStackVisible" class="file-status-stack">
      <Transition name="slide">
        <div v-if="clipboard?.entries.length" class="clipboard-bar">
          <span class="clipboard-bar__icon">
            <Copy v-if="clipboard.mode === 'copy'" :size="17" />
            <Scissors v-else :size="17" />
          </span>
          <span>
            <strong>{{ clipboard.mode === 'copy' ? '已复制' : '已剪切' }} {{ clipboard.entries.length }} 项</strong>
            <small>
              {{ clipboard.entries[0]?.name }}
              <template v-if="clipboard.entries.length > 1"> 等 {{ clipboard.entries.length }} 项</template>
            </small>
          </span>
          <button type="button" :disabled="pasteBusy" @click="pasteClipboard()">
            <ClipboardPaste :size="15" />{{ pasteBusy ? '粘贴中…' : `粘贴到 ${currentPath}` }}
          </button>
          <button type="button" :disabled="pasteBusy" @click="clearClipboard">取消</button>
        </div>
      </Transition>

      <Transition name="slide">
        <section
          v-if="remoteDownloadTasksVisible"
          class="remote-download-tasks"
          aria-labelledby="remote-download-tasks-title"
        >
          <header class="remote-download-tasks__header">
            <div>
              <strong id="remote-download-tasks-title">{{ i18n.t('files.remoteDownload.tasksTitle') }}</strong>
              <span>{{ i18n.t('files.remoteDownload.tasksDescription') }}</span>
            </div>
            <button
              type="button"
              class="remote-download-tasks__refresh"
              :disabled="remoteDownloadJobsLoading"
              :title="i18n.t('files.remoteDownload.refreshTasks')"
              :aria-label="i18n.t('files.remoteDownload.refreshTasks')"
              @click="loadRemoteDownloadJobs()"
            >
              <RefreshCw :size="15" :class="{ spinning: remoteDownloadJobsLoading }" />
            </button>
          </header>
          <p v-if="remoteDownloadJobsErrorMessage" class="remote-download-tasks__error" role="alert">
            {{ remoteDownloadJobsErrorMessage }}
          </p>
          <p
            v-if="remoteDownloadJobsLoading && !remoteDownloadJobs.length"
            class="remote-download-tasks__loading"
            role="status"
            aria-live="polite"
          >
            {{ i18n.t('files.remoteDownload.loadingTasks') }}
          </p>
          <ul v-if="remoteDownloadJobs.length" class="remote-download-task-list">
            <li
              v-for="job in remoteDownloadJobs"
              :key="job.id"
              class="remote-download-task"
              :class="`remote-download-task--${job.state}`"
            >
              <span class="remote-download-task__icon" aria-hidden="true">
                <RefreshCw v-if="isRemoteDownloadJobActive(job)" :size="17" class="spinning" />
                <Download v-else :size="17" />
              </span>
              <div class="remote-download-task__body">
                <div class="remote-download-task__summary">
                  <strong :title="job.name || i18n.t('files.remoteDownload.unnamedTask')">
                    {{ job.name || i18n.t('files.remoteDownload.unnamedTask') }}
                  </strong>
                  <small :title="`${job.source} · ${job.targetDirectory}`">
                    {{ job.source }} · {{ job.targetDirectory }}
                  </small>
                </div>
                <div class="remote-download-task__status-line">
                  <span
                    class="remote-download-task__phase"
                    role="status"
                    aria-live="polite"
                    aria-atomic="true"
                  >{{ remoteDownloadJobStateLabel(job) }}</span>
                  <span v-if="job.loadedBytes" class="remote-download-task__bytes">
                    · {{ i18n.t('files.remoteDownload.received', { bytes: formatBytes(job.loadedBytes) }) }}<template v-if="job.totalBytes">
                      / {{ formatBytes(job.totalBytes) }}</template>
                  </span>
                </div>
                <span v-if="remoteDownloadJobDetail(job)" class="remote-download-task__detail">
                  {{ remoteDownloadJobDetail(job) }}
                </span>
                <progress
                  v-if="isRemoteDownloadJobActive(job)"
                  :max="job.totalBytes || 1"
                  :value="job.totalBytes ? job.loadedBytes || 0 : undefined"
                  :aria-label="remoteDownloadJobProgressLabel(job)"
                />
              </div>
              <div class="remote-download-task__actions">
                <button
                  v-if="isRemoteDownloadJobActive(job)"
                  type="button"
                  :disabled="remoteDownloadPendingActions.has(job.id)"
                  @click="cancelRemoteDownloadJob(job)"
                >
                  {{ remoteDownloadPendingActions.has(job.id)
                    ? i18n.t('files.remoteDownload.stopping')
                    : i18n.t('files.remoteDownload.stop') }}
                </button>
                <button
                  v-else
                  type="button"
                  :disabled="remoteDownloadPendingActions.has(job.id)"
                  @click="deleteRemoteDownloadJob(job)"
                >
                  {{ remoteDownloadPendingActions.has(job.id)
                    ? i18n.t('files.remoteDownload.clearing')
                    : i18n.t('files.remoteDownload.clearTask') }}
                </button>
              </div>
            </li>
          </ul>
        </section>
      </Transition>

      <Transition name="slide">
        <section
          v-if="fileTransferState || uploadTasks.length"
          class="file-activity-stack"
          aria-label="文件操作状态"
        >
          <div
            v-if="fileTransferState"
            class="file-activity-row file-transfer-status"
            :class="`file-activity-row--${fileTransferState.phase}`"
            :role="['partial', 'error'].includes(fileTransferState.phase) ? 'alert' : 'status'"
            aria-live="polite"
          >
            <span class="file-activity-row__icon" aria-hidden="true">
              <RefreshCw v-if="fileTransferState.phase === 'running'" :size="17" class="spinning" />
              <Copy v-else-if="fileTransferState.mode === 'copy'" :size="17" />
              <Scissors v-else :size="17" />
            </span>
            <span class="file-activity-row__body">
              <strong>{{ fileTransferTitle }}</strong>
              <small :title="fileTransferState.currentName
                ? `${fileTransferState.currentName} → ${fileTransferState.target}`
                : fileTransferState.target">
                {{ fileTransferState.currentName
                  ? `${fileTransferState.currentName} → ${fileTransferState.target}`
                  : fileTransferState.target }}
              </small>
              <small v-if="fileTransferState.detail" class="file-activity-row__detail" :title="fileTransferState.detail">
                {{ fileTransferState.detail }}
              </small>
            </span>
            <span class="file-activity-row__actions">
              <button
                v-if="fileTransferState.phase === 'running' && fileTransferState.mode === 'copy'"
                type="button"
                @click="cancelFileTransfer"
              >取消</button>
              <button
                v-else-if="!['running', 'success'].includes(fileTransferState.phase)"
                type="button"
                @click="dismissFileTransfer"
              >关闭</button>
            </span>
          </div>

          <div
            v-for="task in uploadTasks"
            :key="task.id"
            class="file-activity-row upload-task"
            :class="`file-activity-row--${task.phase}`"
            :role="task.phase === 'error' ? 'alert' : 'status'"
            aria-live="polite"
          >
            <span class="file-activity-row__icon" aria-hidden="true">
              <RefreshCw v-if="task.phase === 'running'" :size="17" class="spinning" />
              <Upload v-else :size="17" />
            </span>
            <span class="file-activity-row__body">
              <strong :title="task.name">{{ task.name }}</strong>
              <small>{{ uploadTaskStatus(task) }} · {{ task.target }}</small>
              <small v-if="task.detail" class="file-activity-row__detail" :title="task.detail">{{ task.detail }}</small>
              <span
                class="file-activity-row__track"
                role="progressbar"
                :aria-label="i18n.t('files.upload.progressLabel', { name: task.name, progress: task.progress })"
                aria-valuemin="0"
                aria-valuemax="100"
                :aria-valuenow="task.progress"
              ><i :style="{ width: `${task.progress}%` }" /></span>
            </span>
            <span class="file-activity-row__actions">
              <strong v-if="task.phase !== 'error'" aria-hidden="true">{{ task.progress }}%</strong>
              <template v-else>
                <button type="button" @click="retryUploadTask(task.id)">重试</button>
                <button type="button" @click="dismissUploadTask(task.id)">关闭</button>
              </template>
            </span>
          </div>
        </section>
      </Transition>
      </div>

      <div
        v-if="viewMode === 'list'"
        class="file-table"
        role="table"
        aria-label="文件列表"
        @selectstart="preventNativeSelection"
      >
        <div class="file-row file-row--header" role="row">
          <span>
            <input type="checkbox" :checked="allVisibleSelected" aria-label="选择全部" @change="toggleAll" />
          </span>
          <button type="button" @click="setSort('name')">名称 {{ sortKey === 'name' ? (sortDescending ? '↓' : '↑') : '' }}</button>
          <button type="button" @click="setSort('size')">大小 {{ sortKey === 'size' ? (sortDescending ? '↓' : '↑') : '' }}</button>
          <span>权限</span>
          <span>所有者</span>
          <button type="button" @click="setSort('modified')">修改时间 {{ sortKey === 'modified' ? (sortDescending ? '↓' : '↑') : '' }}</button>
          <span />
        </div>

        <div
          v-for="entry in entries"
          :key="entry.path"
          class="file-row file-row--entry"
          :class="{
            'file-row--selected': selected.has(entry.path),
            'file-row--drop-target': internalDropTarget === entry.path,
          }"
          role="row"
          tabindex="0"
          :draggable="canAddToDesktop(entry)"
          @click="handleEntryClick($event, entry)"
          @dblclick="openEntry(entry)"
          @keydown.enter="openEntry(entry)"
          @contextmenu.stop="showContext($event, entry)"
          @dragstart="startEntryDrag($event, entry)"
          @dragend="finishEntryDrag"
          @dragover="onEntryDragOver($event, entry)"
          @dragleave="onEntryDragLeave($event, entry)"
          @drop="onEntryDrop($event, entry)"
        >
          <span @click.stop="toggleEntry(entry.path)">
            <input
              type="checkbox"
              :checked="selected.has(entry.path)"
              :aria-label="`选择 ${entry.name}`"
              @change="toggleEntry(entry.path)"
              @click.stop
            />
          </span>
          <span class="file-name">
            <span class="file-icon" :class="`file-icon--${entryIconKind(entry)}`">
              <component :is="entryIcon(entry)" :size="19" />
            </span>
            <span>
              <strong>{{ entry.name }}</strong>
              <small class="file-name__desktop-meta">{{ entry.kind === 'directory' ? '文件夹' : entry.mime || '文件' }}</small>
              <small class="file-name__mobile-meta">
                {{ entry.kind === 'directory' ? '文件夹' : formatBytes(entry.sizeBytes) }} · {{ formatTime(entry.modifiedAt) }}
              </small>
            </span>
          </span>
          <span>{{ entry.kind === 'directory' ? '—' : formatBytes(entry.sizeBytes) }}</span>
          <span class="mono">{{ entry.mode }}</span>
          <span>{{ entry.owner }}<small v-if="entry.group">:{{ entry.group }}</small></span>
          <span>{{ formatTime(entry.modifiedAt) }}</span>
          <span>
            <button
              class="row-menu"
              type="button"
              :aria-label="`${entry.name} 操作`"
              aria-haspopup="menu"
              aria-controls="file-context-menu"
              :aria-expanded="contextMenu?.entry?.path === entry.path"
              @click.stop="showContext($event, entry)"
            >
              <MoreHorizontal :size="18" />
            </button>
          </span>
        </div>
      </div>

      <div
        v-else
        class="file-grid"
        role="list"
        aria-label="文件大图标列表"
        @selectstart="preventNativeSelection"
      >
        <div
          v-for="entry in entries"
          :key="entry.path"
          class="file-grid-card"
          :class="{
            'file-grid-card--selected': selected.has(entry.path),
            'file-grid-card--drop-target': internalDropTarget === entry.path,
          }"
          role="listitem"
          tabindex="0"
          :draggable="canAddToDesktop(entry)"
          @click="handleEntryClick($event, entry)"
          @dblclick="openEntry(entry)"
          @keydown.enter="openEntry(entry)"
          @contextmenu.stop="showContext($event, entry)"
          @dragstart="startEntryDrag($event, entry)"
          @dragend="finishEntryDrag"
          @dragover="onEntryDragOver($event, entry)"
          @dragleave="onEntryDragLeave($event, entry)"
          @drop="onEntryDrop($event, entry)"
        >
          <input
            class="file-grid-card__check"
            type="checkbox"
            :checked="selected.has(entry.path)"
            :aria-label="`选择 ${entry.name}`"
            @change="toggleEntry(entry.path)"
            @click.stop
          />
          <button
            class="row-menu file-grid-card__menu"
            type="button"
            :aria-label="`${entry.name} 操作`"
            aria-haspopup="menu"
            aria-controls="file-context-menu"
            :aria-expanded="contextMenu?.entry?.path === entry.path"
            @click.stop="showContext($event, entry)"
          ><MoreHorizontal :size="18" /></button>
          <div class="file-grid-card__visual">
            <img
              v-if="canShowThumbnail(entry)"
              :src="thumbnailURL(entry)"
              :alt="entry.name"
              loading="lazy"
              decoding="async"
              draggable="false"
              @error="markThumbnailFailed(entry.path)"
            />
            <span
              v-else
              class="file-grid-card__icon"
              :class="`file-grid-card__icon--${entryIconKind(entry)}`"
            ><component :is="entryIcon(entry)" :size="48" /></span>
          </div>
          <strong :title="entry.name">{{ entry.name }}</strong>
          <small>
            {{ entry.kind === 'directory' ? '文件夹' : formatBytes(entry.sizeBytes) }}
            <span aria-hidden="true">·</span>
            {{ formatTime(entry.modifiedAt) }}
          </small>
        </div>
      </div>

      <div v-if="!loading && !entries.length" class="file-empty">
        <FolderOpen :size="34" />
        <strong>{{ search ? '没有匹配的文件' : '这个文件夹是空的' }}</strong>
        <span>{{ search ? '换一个关键词试试。' : '可直接拖入文件，或在右上角新建目录。' }}</span>
      </div>
      <div v-if="loading" class="file-loading"><RefreshCw :size="22" class="spinning" />正在读取目录…</div>
      <footer v-if="directory?.truncated" class="file-limit">
        <span v-if="directory.scanTruncated">目录超过 20,000 项，搜索和分页仅覆盖已扫描范围。</span>
        <span v-else-if="directory.totalKnown">
          已显示 {{ directory.entries.length }} / {{ directory.total }} 项。
        </span>
        <span v-else>已显示 {{ directory.entries.length }} 项，可继续加载。</span>
        <button
          v-if="directory.nextOffset"
          class="button button--secondary"
          type="button"
          :disabled="loading"
          @click="loadDirectory(currentPath, true)"
        >
          加载更多
        </button>
      </footer>

      <div v-if="dragging" class="drop-overlay">
        <Upload :size="34" />
        <strong>松开以上传到 {{ currentPath }}</strong>
      </div>
      <div v-if="internalDropTarget" class="file-internal-drop-hint" aria-hidden="true">
        <Copy v-if="internalDropMode === 'copy'" :size="17" />
        <Scissors v-else :size="17" />
        <span>
          <strong>{{ internalDropTitle }}</strong>
          <small>{{ crossPanelDropActive || internalDropMode === 'copy' ? '松开以复制' : '按住 Ctrl/Option 可复制' }}</small>
        </span>
      </div>
    </section>

    <Transition name="batch-dock">
      <div
        v-if="selected.size"
        class="batch-bar"
        role="toolbar"
        aria-label="批量文件操作"
      >
        <strong>已选 {{ selected.size }} 项</strong>
        <button
          v-if="selectedEntriesDownloadable"
          type="button"
          @click="downloadSelected()"
        ><Download :size="15" />{{ selectedEntries.length === 1 && selectedEntries[0]?.kind === 'file' ? '下载' : '下载 ZIP' }}</button>
        <button type="button" @click="openDialog('compress')"><Archive :size="15" />压缩</button>
        <button type="button" @click="setClipboard('copy')"><Copy :size="15" />复制</button>
        <button type="button" @click="setClipboard('move')"><Scissors :size="15" />剪切</button>
        <button type="button" @click="openDialog('chmod')"><ShieldCheck :size="15" />权限</button>
        <button
          v-if="!isRemoteFileHost && selectedEntries.some(canAddToDesktop)"
          type="button"
          :disabled="desktopAdding"
          @click="addEntriesToDesktop()"
        ><Pin :size="15" />{{ desktopAdding ? '添加中…' : '添加到桌面' }}</button>
        <button type="button" @click="invertSelection"><ListRestart :size="15" />反选</button>
        <button class="danger-link" type="button" @click="openDialog('trash')">
          <Trash2 :size="15" />回收站
        </button>
        <button type="button" aria-label="取消选择" title="取消选择" @click="clearSelection">
          <X :size="15" />取消
        </button>
      </div>
    </Transition>

    <Teleport to="body">
      <div
        v-if="contextMenu"
        id="file-context-menu"
        ref="contextMenuElement"
        class="file-context-menu k-context-menu"
        :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
        role="menu"
        :aria-label="phrase('文件操作')"
        @pointermove="showContextMenuPointerFocus"
        @keydown.stop="handleContextMenuKeydown"
      >
      <button v-if="contextMenu.entry" role="menuitem" type="button" @click="openEntry(contextMenu.entry)">
        <Eye :size="15" />{{ phrase(contextMenu.entry.kind === 'directory' ? '打开' : '查看') }}
      </button>
      <button v-if="!contextMenu.entry && !isRemoteFileHost" role="menuitem" type="button" :disabled="desktopAdding" @click="addEntriesToDesktop(undefined, true)">
        <Pin :size="15" />{{ phrase('将当前文件夹添加到桌面') }}
      </button>
      <button
        v-if="contextMenu.entry && contextBatchDownloadable"
        role="menuitem"
        type="button"
        @click="downloadSelected(contextMenu.entry)"
      >
        <Download :size="15" />{{ phrase(contextBatchEntries.length === 1 && contextBatchEntries[0]?.kind === 'file' ? '下载' : '下载 ZIP') }}
      </button>
      <button v-if="contextShareEntry" role="menuitem" type="button" @click="openFileShare(contextMenu.entry)">
        <Share2 :size="15" />{{ phrase('分享') }}
      </button>
      <button
        v-if="contextMenu.entry && !contextHasMultipleEntries && archiveFormat(contextMenu.entry)"
        role="menuitem"
        type="button"
        @click="openDialog('extract', contextMenu.entry)"
      >
        <FolderOpen :size="15" />{{ phrase('解压到文件夹') }}
      </button>
      <button v-if="contextMenu.entry" role="menuitem" type="button" @click="openDialog('compress', contextMenu.entry)">
        <Archive :size="15" />{{ phrase('压缩') }}
      </button>
      <hr v-if="contextMenu.entry" role="separator" />
      <button v-if="contextMenu.entry && !contextHasMultipleEntries" role="menuitem" type="button" @click="openDialog('rename', contextMenu.entry)">
        <Pencil :size="15" />{{ phrase('重命名') }}
      </button>
      <button v-if="contextMenu.entry" role="menuitem" type="button" @click="setClipboard('copy', contextMenu.entry)"><Copy :size="15" />{{ phrase('复制') }}</button>
      <button v-if="contextMenu.entry" role="menuitem" type="button" @click="setClipboard('move', contextMenu.entry)"><Scissors :size="15" />{{ phrase('剪切') }}</button>
      <button
        v-if="clipboard?.entries.length && contextMenu.entry?.kind === 'directory'"
        role="menuitem"
        type="button"
        :disabled="pasteBusy"
        @click="pasteClipboard(contextMenu.entry.path)"
      ><ClipboardPaste :size="15" />{{ phrase('粘贴到此文件夹') }}</button>
      <button
        v-if="clipboard?.entries.length"
        role="menuitem"
        type="button"
        :disabled="pasteBusy"
        @click="pasteClipboard()"
      ><ClipboardPaste :size="15" />{{ phrase('粘贴到当前目录') }}</button>
      <button v-if="contextMenu.entry" role="menuitem" type="button" @click="openDialog('chmod', contextMenu.entry)">
        <ShieldCheck :size="15" />{{ phrase('修改权限') }}
      </button>
      <button
        v-if="contextMenu.entry && !isRemoteFileHost && contextBatchEntries.some(canAddToDesktop)"
        role="menuitem"
        type="button"
        :disabled="desktopAdding"
        @click="addEntriesToDesktop(contextMenu.entry)"
      >
        <Pin :size="15" />{{ phrase(contextHasMultipleEntries ? `添加 ${contextBatchEntries.filter(canAddToDesktop).length} 项到桌面` : '添加到桌面') }}
      </button>
      <button v-if="!contextMenu.entry" role="menuitem" type="button" @click="openDialog('mkdir')">
        <Plus :size="15" />{{ phrase('新建目录') }}
      </button>
      <hr v-if="contextMenu.entry" role="separator" />
      <button v-if="contextMenu.entry" class="danger-link k-context-menu__item--danger" role="menuitem" type="button" @click="openDialog('trash', contextMenu.entry)">
        <Trash2 :size="15" />{{ phrase('移入回收站') }}
      </button>
      </div>
    </Teleport>

    <FileShareDialog v-if="shareEntry" :entry="shareEntry" @close="closeFileShare" />
    <FileShareManagerDialog v-if="shareManagerOpen" @close="closeShareManager" />

    <ModalDialog
      :open="remoteDownloadDialogOpen"
      :title="i18n.t('files.remoteDownload.label')"
      :description="i18n.t('files.remoteDownload.dialogDescription', { target: remoteDownloadTarget })"
      size="small"
      @close="closeRemoteDownloadDialog"
    >
      <form class="operation-form remote-download-form" @submit.prevent="submitRemoteDownload">
        <label>
          <span>{{ i18n.t('files.remoteDownload.urlLabel') }}</span>
          <input
            ref="remoteDownloadURLInput"
            v-model="remoteDownloadURL"
            type="url"
            inputmode="url"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            placeholder="https://example.com/archive.tar.gz"
            :aria-invalid="remoteDownloadFormErrorCode === 'url'"
            :aria-describedby="remoteDownloadURLDescription"
            required
          />
        </label>
        <p
          v-if="remoteDownloadUsesPlainHTTP"
          :id="remoteDownloadHTTPWarningID"
          class="remote-download-warning"
          role="status"
          aria-live="polite"
          aria-atomic="true"
        >
          {{ i18n.t('files.remoteDownload.httpWarning') }}
        </p>
        <label>
          <span>{{ i18n.t('files.remoteDownload.nameLabel') }} <small>{{ i18n.t('files.remoteDownload.optional') }}</small></span>
          <input
            v-model="remoteDownloadName"
            autocomplete="off"
            :placeholder="i18n.t('files.remoteDownload.namePlaceholder')"
            :aria-invalid="remoteDownloadFormErrorCode === 'name'"
            :aria-describedby="remoteDownloadFormErrorCode === 'name' ? remoteDownloadFormErrorID : undefined"
          />
        </label>
        <div class="remote-download-note">
          <ShieldCheck :size="18" aria-hidden="true" />
          <span>{{ i18n.t('files.remoteDownload.note') }}</span>
        </div>
        <p v-if="remoteDownloadFormError" :id="remoteDownloadFormErrorID" class="remote-download-error" role="alert">
          {{ remoteDownloadFormError }}
        </p>
        <div class="dialog-actions">
          <button class="button button--secondary" type="button" @click="closeRemoteDownloadDialog">{{ i18n.t('common.cancel') }}</button>
          <button
            class="button button--primary"
            type="submit"
            :disabled="remoteDownloadSubmitting || !remoteDownloadURL.trim()"
          >
            {{ remoteDownloadSubmitting
              ? i18n.t('files.remoteDownload.starting')
              : i18n.t('files.remoteDownload.start') }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(dialogAction)"
      :title="phrase(dialogTitle)"
      :description="phrase(
        dialogAction === 'trash'
          ? '文件将移动到 KPanel 隔离回收区，可在回收站中恢复。'
          : dialogAction === 'compress'
            ? '默认使用适合 Linux 服务器的 tar.gz；也可选择 ZIP 或 TAR。'
            : dialogAction === 'extract'
              ? '内容将解压到全新的文件夹，不覆盖已有文件。'
              : ''
      )"
      size="small"
      @close="closeDialog"
    >
      <div v-if="dialogAction !== 'trash'" class="operation-form">
        <label>
          <span>
            {{
              phrase(dialogAction === 'mkdir'
                ? '文件夹名称'
                : dialogAction === 'rename'
                  ? '新名称'
                  : dialogAction === 'chmod'
                    ? '权限（八进制）'
                    : dialogAction === 'compress'
                      ? '压缩包名称'
                      : '目标文件夹名称')
            }}
          </span>
          <input
            v-model="dialogValue"
            :placeholder="
              phrase(dialogAction === 'chmod'
                ? '例如 644 或 755'
                : dialogAction === 'compress'
                  ? '例如 website.tar.gz'
                  : dialogAction === 'extract'
                    ? '例如 website'
                    : '输入名称')
            "
            autocomplete="off"
            @keydown.enter="submitDialog"
          />
        </label>
        <label v-if="dialogAction === 'compress'">
          <span>{{ phrase('压缩格式') }}</span>
          <select v-model="dialogFormat">
            <option value="tar.gz">{{ phrase('TAR.GZ（推荐）') }}</option>
            <option value="zip">{{ phrase('ZIP（跨平台）') }}</option>
            <option value="tar">{{ phrase('TAR（不压缩）') }}</option>
          </select>
        </label>
        <small v-if="dialogAction === 'compress' || dialogAction === 'extract'" class="archive-hint">
          {{ phrase('单次最多 100 项、10,000 个条目或解压后 10 GiB；不支持符号链接、硬链接和设备文件。') }}
        </small>
      </div>
      <div v-else class="trash-summary">
        <Trash2 :size="24" />
        <strong>{{ phrase(`确认移动 ${dialogEntries.length} 项？`) }}</strong>
        <span>{{ phrase('稍后可从文件管理右上角的回收站恢复或彻底删除。') }}</span>
      </div>
      <div class="dialog-actions">
        <button
          class="button button--secondary"
          type="button"
          :disabled="dialogBusy && dialogAction !== 'compress' && dialogAction !== 'extract'"
          @click="dialogBusy ? cancelArchive() : closeDialog()"
        >
          {{ phrase(dialogBusy && (dialogAction === 'compress' || dialogAction === 'extract') ? '停止' : '取消') }}
        </button>
        <button
          class="button"
          :class="dialogAction === 'trash' ? 'button--danger' : 'button--primary'"
          type="button"
          :disabled="dialogBusy || (dialogAction !== 'trash' && !dialogValue.trim())"
          @click="submitDialog"
        >
          {{
            phrase(dialogBusy
              ? dialogAction === 'compress'
                ? '压缩中…'
                : dialogAction === 'extract'
                  ? '解压中…'
                  : '处理中…'
              : dialogAction === 'trash'
                ? '移入回收站'
                : dialogAction === 'compress'
                  ? '开始压缩'
                  : dialogAction === 'extract'
                    ? '开始解压'
                    : '确认')
          }}
        </button>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="trashOpen"
      :title="phrase('回收站')"
      :description="phrase('删除的文件保存在 Agent 隔离目录中；恢复时不会覆盖同名文件。')"
      size="wide"
      @close="!trashBusy && (trashOpen = false)"
    >
      <div class="trash-manager">
        <header>
          <span>{{ phrase(`${trashTotal} 项`) }}<span v-if="trashTruncated">{{ phrase('（显示最近 500 项）') }}</span></span>
          <div>
            <button
              class="button button--secondary"
              type="button"
              :disabled="trashLoading || trashBusy || !trashEntries.length"
              @click="toggleAllTrash"
            >{{ phrase(allTrashSelected ? '取消选择' : '选择当前列表') }}</button>
            <button class="button button--secondary" type="button" :disabled="trashLoading || trashBusy" @click="loadTrash">
              <RefreshCw :size="15" :class="{ spinning: trashLoading }" />{{ phrase('刷新') }}
            </button>
            <button
              class="button button--secondary"
              type="button"
              :disabled="trashBusy || !selectedTrash.size || trashEntries.filter((entry) => selectedTrash.has(entry.id)).some((entry) => !entry.restorable)"
              @click="runTrashAction('trash_restore')"
            ><RotateCcw :size="15" />{{ phrase('恢复') }}</button>
            <button class="button button--danger" type="button" :disabled="trashBusy || !selectedTrash.size" @click="runTrashAction('trash_delete')">
              <Trash2 :size="15" />{{ phrase('彻底删除') }}
            </button>
            <button class="button button--danger" type="button" :disabled="trashBusy || !trashTotal" @click="runTrashAction('trash_empty')">
              {{ phrase('清空回收站') }}
            </button>
          </div>
        </header>
        <div v-if="trashLoading" class="file-loading"><RefreshCw :size="22" class="spinning" />{{ phrase('正在读取回收站…') }}</div>
        <div v-else-if="trashEntries.length" class="trash-list">
          <label v-for="entry in trashEntries" :key="entry.id" class="trash-item">
            <input type="checkbox" :checked="selectedTrash.has(entry.id)" @change="toggleTrash(entry.id)" />
            <span class="file-icon" :class="{ 'file-icon--folder': entry.kind === 'directory' }">
              <Folder v-if="entry.kind === 'directory'" :size="19" />
              <File v-else :size="19" />
            </span>
            <span>
              <strong>{{ entry.name }}</strong>
              <small>{{ entry.originalPath || phrase('旧版回收站项目（仅支持彻底删除）') }}</small>
            </span>
            <span>{{ entry.kind === 'directory' ? phrase('文件夹') : formatBytes(entry.sizeBytes) }}</span>
            <span>{{ formatTime(entry.deletedAt) }}</span>
          </label>
        </div>
        <div v-else class="file-empty">
          <Trash2 :size="34" />
          <strong>{{ phrase('回收站是空的') }}</strong>
          <span>{{ phrase('移入回收站的文件会显示在这里。') }}</span>
        </div>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(previewEntry)"
      :title="previewEntry?.name || phrase('文件查看器')"
      :description="previewEntry ? `${previewEntry.path} · ${formatBytes(previewEntry.sizeBytes)}` : ''"
      size="wide"
      allow-fullscreen
      @close="closePreview"
    >
      <div v-if="previewLoading" class="preview-loading"><RefreshCw :size="22" class="spinning" />{{ phrase('正在打开文件…') }}</div>
      <div v-else-if="previewEntry && previewMode === 'text'" class="code-viewer">
        <header>
          <span><Code2 :size="15" />{{ previewEntry.mime || phrase('UTF-8 文本') }}</span>
          <span class="code-viewer__header-right">
            <span>{{ previewEntry.mode }} · {{ previewEntry.owner }}:{{ previewEntry.group }}</span>
            <span class="code-editor-tools">
              <button
                class="code-editor-tool"
                type="button"
                :title="phrase('查找或替换（Ctrl+F）')"
                :aria-label="phrase('查找或替换')"
                @click="codeEditorRef?.openSearch()"
              >
                <Search :size="15" />
              </button>
              <button
                class="code-editor-tool"
                :class="{ 'is-active': editorLineWrap }"
                type="button"
                :title="phrase('切换自动换行')"
                :aria-label="phrase('切换自动换行')"
                :aria-pressed="editorLineWrap"
                @click="editorLineWrap = !editorLineWrap"
              >
                <WrapText :size="15" />
              </button>
            </span>
          </span>
        </header>
        <div class="code-editor">
          <CodeEditor
            ref="codeEditorRef"
            v-model="previewContent"
            :file-name="previewEntry.name"
            :mime="previewEntry.mime"
            :size-bytes="previewEntry.sizeBytes"
            :editable="previewEntry.editable"
            :line-wrap="editorLineWrap"
            @dirty="previewDirty = true"
            @save="savePreview"
            @status="handleEditorStatus"
            @ready="handleEditorReady"
          />
        </div>
        <footer>
          <span>
            {{ editorStatus?.lines || 1 }} {{ phrase('行') }}
            <template v-if="editorStatus"> · {{ phrase('行') }} {{ editorStatus.line }}，{{ phrase('列') }} {{ editorStatus.column }}</template>
            · UTF-8
            <template v-if="editorInfo">
              · {{ editorInfo.label }}
              {{ phrase(editorInfo.highlighted ? '语法着色' : editorInfo.reason === 'large-file' ? '大文件纯文本' : '纯文本') }}
            </template>
          </span>
          <span v-if="previewDirty">{{ phrase('有未保存修改') }}</span>
          <span class="code-editor-actions">
            <button class="button button--primary button--small" type="button" :disabled="previewSaving || !previewDirty" @click="savePreview()">
              <Save :size="15" />{{ phrase(previewSaving ? '保存中…' : '保存 Ctrl+S') }}
            </button>
          </span>
        </footer>
      </div>
      <div v-else-if="previewEntry" class="media-viewer" :class="`media-viewer--${previewMode}`">
        <div v-if="previewMode === 'video'" class="media-player">
          <video
            :key="mediaReloadKey"
            :aria-label="previewEntry.name"
            controls
            preload="metadata"
            playsinline
            @loadstart="handleMediaLoadStart"
            @loadedmetadata="handleMediaMetadata"
            @loadeddata="handleVideoFrameReady"
            @canplay="handleVideoFrameReady"
            @playing="handleVideoFrameReady"
            @waiting="handleMediaWaiting"
            @error="handleMediaError"
          >
            <source :src="previewURL" :type="previewEntry.mime || undefined" />
          </video>
          <div v-if="mediaLoading && !mediaError" class="media-player__loading" role="status" aria-live="polite">
            <RefreshCw :size="20" class="spinning" />
            <span>{{ phrase('正在连接视频流…') }}</span>
          </div>
          <div v-else-if="mediaError" class="media-player__error" role="alert">
            <strong>{{ phrase(mediaErrorMessage || '视频暂时无法播放') }}</strong>
            <span>{{ phrase(mediaErrorDetail || '请检查文件编码或服务器是否支持该格式。') }}</span>
            <button
              v-if="mediaRetryable"
              class="button button--secondary button--small"
              type="button"
              @click.stop="retryMedia"
            >
              {{ phrase('重试播放') }}
            </button>
          </div>
        </div>
        <img v-else-if="previewMode === 'image'" :src="previewURL" :alt="previewEntry.name" decoding="async" />
        <audio v-else-if="previewMode === 'audio'" :src="previewURL" controls preload="metadata" />
        <iframe v-else-if="previewMode === 'pdf'" :src="previewURL" :title="previewEntry.name" loading="lazy" />
        <div v-else class="metadata-viewer">
          <component :is="entryIcon(previewEntry)" :size="44" />
          <strong>{{ phrase('此格式暂不在浏览器内解析') }}</strong>
          <span>{{ previewEntry.mime || phrase('未知格式') }} · {{ formatBytes(previewEntry.sizeBytes) }}</span>
          <button class="button button--primary" type="button" @click="download(previewEntry)">
            <Download :size="16" />{{ phrase('下载文件') }}
          </button>
        </div>
        <footer v-if="previewMode !== 'metadata'" class="media-viewer__footer">
          <span class="media-viewer__status" :class="{ 'is-loading': mediaLoading, 'is-error': mediaError }">
            <i aria-hidden="true" />{{ phrase(mediaStatusLabel) }}
          </span>
          <button class="button button--secondary button--small" type="button" @click="download(previewEntry)">
            <Download :size="15" />{{ phrase('下载原文件') }}
          </button>
        </footer>
      </div>
    </ModalDialog>
  </section>
</template>

<style scoped>
.files-page {
  display: grid;
  gap: 18px;
}

.file-command-bar {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-shortcuts {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.file-shortcuts button {
  padding: 7px 11px;
  border: 1px solid var(--border);
  border-radius: 999px;
  color: var(--muted);
  background: var(--surface);
  cursor: pointer;
}

.file-shortcuts button:hover {
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border));
  color: var(--brand);
}

.file-command-bar__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
}

.file-command-bar__actions .button {
  min-height: 40px;
  font-size: 14px;
}

.file-command-bar__actions .file-command-bar__refresh {
  width: 40px;
  min-width: 40px;
  flex: 0 0 40px;
  padding: 0;
}

.file-browser {
  position: relative;
  min-height: 540px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: var(--surface);
  box-shadow: var(--shadow-sm);
}

.file-browser--dragging {
  border-color: var(--brand);
}

.file-browser--internal-drop {
  border-color: color-mix(in srgb, var(--brand) 76%, var(--border));
  box-shadow:
    inset 0 0 0 2px color-mix(in srgb, var(--brand) 32%, transparent),
    var(--shadow-sm);
}

.file-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 13px 15px;
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-subtle) 45%, var(--surface));
}

.file-toolbar__path {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 10px;
}

.file-host-switcher {
  position: relative;
  flex: 0 0 auto;
}

.file-host-switcher__trigger {
  display: inline-flex;
  max-width: 220px;
  min-height: 38px;
  align-items: center;
  gap: 7px;
  padding: 7px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: border-color .16s ease, color .16s ease, background-color .16s ease;
}

.file-host-switcher__trigger:hover,
.file-host-switcher__trigger:focus-visible,
.file-host-switcher__trigger[aria-expanded='true'] {
  border-color: color-mix(in srgb, var(--brand) 55%, var(--border));
  color: var(--brand);
  outline: none;
}

.file-host-switcher__trigger > span {
  color: var(--muted);
  font-size: 13px;
  white-space: nowrap;
}

.file-host-switcher__trigger > strong {
  min-width: 0;
  overflow: hidden;
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__menu {
  position: absolute;
  z-index: 8;
  top: calc(100% + 6px);
  left: 0;
  width: min(300px, calc(100vw - 32px));
  overflow: hidden;
  border: 1px solid var(--border-strong, var(--border));
  border-radius: var(--radius);
  background: var(--surface-raised, var(--surface));
  box-shadow: var(--shadow-md, var(--shadow-sm));
}

.file-host-switcher__heading {
  display: block;
  padding: 11px 12px 9px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
}

.file-host-switcher__item {
  display: grid;
  width: 100%;
  min-height: 54px;
  grid-template-columns: 28px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 9px;
  padding: 9px 12px;
  border: 0;
  color: var(--text);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: background-color .16s ease, color .16s ease;
}

.file-host-switcher__item :deep(.file-host-switcher__os) {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-sm);
  box-shadow: none;
}

.file-host-switcher__item :deep(.file-host-switcher__os svg) {
  width: 17px;
  height: 17px;
}

.file-host-switcher__item:hover,
.file-host-switcher__item:focus-visible {
  background: var(--interaction-hover);
  outline: none;
}

.file-host-switcher__item.is-active {
  color: var(--brand-strong, var(--brand));
  background: color-mix(in srgb, var(--brand) 9%, var(--surface-raised, var(--surface)));
}

.file-host-switcher__item.is-manage {
  color: var(--muted);
}

.file-host-switcher__item > span {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.file-host-switcher__item strong {
  overflow: hidden;
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__item small {
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__item > svg:last-child {
  justify-self: end;
}

.file-host-switcher__message,
.file-host-switcher__retry {
  display: flex;
  min-height: 48px;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.file-host-switcher__message--error {
  color: var(--danger);
}

.file-host-switcher__retry {
  width: 100%;
  border: 0;
  border-top: 1px solid var(--border);
  color: var(--danger);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.file-host-switcher__retry:hover,
.file-host-switcher__retry:focus-visible {
  background: var(--interaction-hover);
  outline: none;
}

.breadcrumbs {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  overflow-x: auto;
}

.breadcrumbs button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: max-content;
  padding: 7px 4px;
  border: 0;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
}

.breadcrumbs button:last-child {
  color: var(--text);
  font-weight: 600;
}

.breadcrumbs button:disabled {
  cursor: default;
}

.file-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(280px, 34vw);
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
}

.file-search input {
  width: 100%;
  height: 36px;
  border: 0;
  outline: 0;
  color: var(--text);
  background: transparent;
}

.file-search button,
.row-menu {
  display: grid;
  place-items: center;
  padding: 5px;
  border: 0;
  border-radius: 7px;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
}

.file-search button:hover,
.row-menu:hover {
  color: var(--text);
  background: var(--interaction-hover);
}

.file-toolbar__controls,
.file-grid-sort,
.file-view-switch {
  display: flex;
  align-items: center;
  gap: 7px;
}

.file-toolbar__controls {
  flex: 0 1 auto;
  justify-content: flex-end;
}

.file-grid-sort,
.file-view-switch {
  min-height: 38px;
  padding: 3px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
}

.file-grid-sort select {
  height: 30px;
  padding: 0 6px;
  border: 0;
  color: var(--muted);
  background: transparent;
  outline: none;
}

.file-grid-sort button,
.file-view-switch button {
  display: grid;
  width: 31px;
  height: 30px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 7px;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
}

.file-grid-sort button:hover,
.file-view-switch button:hover {
  color: var(--text);
  background: var(--interaction-hover);
}

.file-view-switch button.is-active {
  color: var(--brand);
  background: var(--brand-soft);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--brand) 24%, transparent);
}

.batch-bar {
  position: fixed;
  z-index: 45;
  bottom: max(16px, env(safe-area-inset-bottom));
  left: calc(var(--app-shell-inline-offset) + (100vw - var(--app-shell-inline-offset)) / 2);
  width: min(760px, calc(100vw - var(--app-shell-inline-offset) - 32px));
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow-x: auto;
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--brand) 30%, var(--border));
  border-radius: 14px;
  background: color-mix(in srgb, var(--brand) 8%, var(--surface));
  box-shadow: 0 12px 34px rgb(0 0 0 / 20%);
  transform: translateX(-50%);
}

.batch-bar strong {
  flex: 0 0 auto;
  margin-right: 8px;
  white-space: nowrap;
}

.batch-bar button {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 5px;
  padding: 6px 9px;
  border: 0;
  border-radius: 8px;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
}

.batch-bar button:hover {
  color: var(--text);
  background: var(--interaction-hover-surface);
}

:global(.desktop-window .batch-bar) {
  left: 50%;
  width: min(760px, calc(100% - 28px));
  gap: 4px;
  padding: 8px 10px;
  scrollbar-width: none;
}

:global(.desktop-window .batch-bar::-webkit-scrollbar) {
  display: none;
}

:global(.desktop-window .batch-bar strong) {
  margin-right: 4px;
}

:global(.desktop-window .batch-bar button) {
  gap: 4px;
  padding-inline: 7px;
}

.file-status-stack {
  display: grid;
  max-height: min(420px, 46dvh);
  overflow-y: auto;
  overscroll-behavior-y: contain;
  scrollbar-gutter: stable;
}

.clipboard-bar {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  padding: 9px 15px;
  border-bottom: 1px solid color-mix(in srgb, var(--brand) 24%, var(--border));
  background: color-mix(in srgb, var(--brand) 7%, var(--surface));
}

.clipboard-bar__icon {
  width: 30px;
  height: 30px;
  display: grid;
  place-items: center;
  border-radius: 9px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 12%, transparent);
}

.clipboard-bar > span:nth-child(2) {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.clipboard-bar small {
  overflow: hidden;
  color: var(--muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.clipboard-bar button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 6px 10px;
  border: 0;
  border-radius: 8px;
  color: var(--muted);
  background: transparent;
  cursor: pointer;
}

.clipboard-bar button:first-of-type {
  color: var(--on-brand);
  background: var(--brand-action);
}

.clipboard-bar button:hover:not(:disabled) {
  color: var(--text);
  background: var(--interaction-hover-surface);
}

.clipboard-bar button:first-of-type:hover:not(:disabled) {
  color: var(--on-brand);
  background: var(--brand-strong);
}

.clipboard-bar button:disabled {
  opacity: .6;
  cursor: wait;
}

.file-activity-stack {
  display: grid;
  gap: 6px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.file-activity-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  min-height: 50px;
  padding: 7px 9px;
  border: 1px solid var(--border);
  border-radius: 9px;
  background: var(--surface-subtle);
}

.file-activity-row--partial,
.file-activity-row--cancelled {
  border-color: color-mix(in srgb, var(--amber) 32%, var(--border));
}

.file-activity-row--error {
  border-color: color-mix(in srgb, var(--danger) 30%, var(--border));
}

.file-activity-row__icon {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  border-radius: 8px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
}

.file-activity-row--partial .file-activity-row__icon,
.file-activity-row--cancelled .file-activity-row__icon {
  color: var(--amber);
  background: color-mix(in srgb, var(--amber) 13%, var(--surface));
}

.file-activity-row--error .file-activity-row__icon {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 12%, var(--surface));
}

.file-activity-row__body {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.file-activity-row strong,
.file-activity-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-activity-row strong {
  color: var(--text);
  font-size: 14px;
}

.file-activity-row small {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.4;
}

.file-activity-row .file-activity-row__detail {
  display: -webkit-box;
  overflow: hidden;
  color: var(--amber);
  white-space: normal;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.file-activity-row--error .file-activity-row__detail {
  color: var(--danger);
}

.file-activity-row__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.file-activity-row__actions > strong {
  min-width: 38px;
  color: var(--muted);
  font-size: 13px;
  text-align: right;
}

.file-activity-row__actions button {
  min-height: 40px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
  font-size: 14px;
}

.file-activity-row__track {
  height: 6px;
  overflow: hidden;
  border-radius: 99px;
  background: color-mix(in srgb, var(--border) 72%, var(--surface));
}

.file-activity-row__track i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--brand);
}

.remote-download-tasks {
  display: grid;
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.remote-download-tasks__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px 6px;
}

.remote-download-tasks__header > div {
  display: flex;
  min-width: 0;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 3px 10px;
}

.remote-download-tasks__header strong {
  color: var(--text);
  font-size: 14px;
}

.remote-download-tasks__header span,
.remote-download-tasks__loading {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.5;
}

.remote-download-tasks__header button,
.remote-download-task__actions button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 7px 12px;
  border: 1px solid var(--border);
  border-radius: 9px;
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
  font-size: 14px;
}

.remote-download-tasks__refresh {
  width: 36px;
  min-width: 36px;
  padding: 0;
}

.remote-download-tasks__header button:disabled,
.remote-download-task__actions button:disabled {
  cursor: not-allowed;
  opacity: 0.58;
}

.remote-download-tasks__error,
.remote-download-tasks__loading {
  margin: 0;
  padding: 0 12px 8px;
}

.remote-download-tasks__error {
  color: var(--danger);
  font-size: 14px;
  line-height: 1.5;
}

.remote-download-task-list {
  display: grid;
  max-height: 264px;
  gap: 6px;
  overflow-y: auto;
  margin: 0;
  padding: 0 12px 8px;
  list-style: none;
}

.remote-download-task {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 9px;
  background: var(--surface-subtle);
}

.remote-download-task--error {
  border-color: color-mix(in srgb, var(--danger) 30%, var(--border));
}

.remote-download-task--cancelled,
.remote-download-task--interrupted {
  border-color: color-mix(in srgb, var(--amber) 32%, var(--border));
}

.remote-download-task__icon {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  border-radius: 8px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 14%, var(--surface));
}

.remote-download-task--error .remote-download-task__icon {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 12%, var(--surface));
}

.remote-download-task--cancelled .remote-download-task__icon,
.remote-download-task--interrupted .remote-download-task__icon {
  color: var(--amber);
  background: color-mix(in srgb, var(--amber) 14%, var(--surface));
}

.remote-download-task__body {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.remote-download-task__summary {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: 8px;
}

.remote-download-task__summary strong {
  flex: 0 1 auto;
  overflow: hidden;
  color: var(--text);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.remote-download-task__summary small {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.remote-download-task__status-line {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px;
}

.remote-download-task__phase {
  color: var(--text);
  font-size: 14px;
  line-height: 1.45;
}

.remote-download-task__bytes {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.remote-download-task__detail {
  color: var(--amber);
  font-size: 13px;
  line-height: 1.45;
}

.remote-download-task--error .remote-download-task__detail {
  color: var(--danger);
}

.remote-download-task progress {
  width: 100%;
  height: 6px;
  overflow: hidden;
  border: 0;
  border-radius: 99px;
  accent-color: var(--brand);
}

.danger-link {
  color: var(--danger) !important;
}

.file-table {
  display: grid;
}

.file-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(166px, 1fr));
  align-content: start;
  gap: 12px;
  min-height: 430px;
  padding: 16px;
  -webkit-user-select: none;
  user-select: none;
}

.file-grid-card {
  position: relative;
  display: grid;
  min-width: 0;
  content-visibility: auto;
  contain-intrinsic-size: auto 190px;
  gap: 7px;
  padding: 10px;
  border: 1px solid transparent;
  border-radius: 13px;
  color: var(--text);
  background: transparent;
  cursor: default;
  outline: none;
  transition: border-color 0.14s ease, background-color 0.14s ease, box-shadow 0.14s ease;
}

.file-grid-card:hover,
.file-grid-card:focus-visible {
  border-color: color-mix(in srgb, var(--brand) 24%, var(--border));
  background: var(--interaction-hover);
}

.file-grid-card--selected {
  border-color: color-mix(in srgb, var(--brand) 48%, var(--border));
  background: color-mix(in srgb, var(--brand) 8%, var(--surface));
  box-shadow: 0 7px 20px color-mix(in srgb, var(--brand) 8%, transparent);
}

.file-grid-card--drop-target {
  border-color: var(--brand);
  background: color-mix(in srgb, var(--brand) 13%, var(--surface));
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--brand) 62%, transparent),
    0 10px 24px color-mix(in srgb, var(--brand) 14%, transparent);
  transform: translateY(-2px);
}

.file-grid-card__check,
.file-grid-card__menu {
  position: absolute;
  z-index: 2;
  top: 16px;
  opacity: 0;
  transition: opacity 0.12s ease;
}

.file-grid-card__check {
  left: 16px;
  width: 16px;
  height: 16px;
  accent-color: var(--brand);
}

.file-grid-card__menu {
  right: 16px;
  color: var(--text);
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  box-shadow: 0 2px 8px rgb(0 0 0 / 10%);
}

.file-grid-card:hover .file-grid-card__check,
.file-grid-card:hover .file-grid-card__menu,
.file-grid-card:focus-within .file-grid-card__check,
.file-grid-card:focus-within .file-grid-card__menu,
.file-grid-card--selected .file-grid-card__check,
.file-grid-card--selected .file-grid-card__menu {
  opacity: 1;
}

.file-grid-card__visual {
  display: grid;
  width: 100%;
  aspect-ratio: 4 / 3;
  place-items: center;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--border) 72%, transparent);
  border-radius: 10px;
  background:
    linear-gradient(45deg, color-mix(in srgb, var(--surface-subtle) 75%, transparent) 25%, transparent 25%) 0 0 / 16px 16px,
    linear-gradient(-45deg, color-mix(in srgb, var(--surface-subtle) 75%, transparent) 25%, transparent 25%) 0 0 / 16px 16px,
    var(--surface);
}

.file-grid-card__visual img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.file-grid-card__icon {
  display: grid;
  width: 76px;
  height: 76px;
  place-items: center;
  border-radius: 20px;
  color: var(--file-icon-color, var(--blue));
  background: color-mix(in srgb, var(--file-icon-color, var(--blue)) 11%, var(--surface));
}

.file-grid-card > strong,
.file-grid-card > small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-grid-card > strong {
  padding: 0 2px;
  font-size: 13px;
}

.file-grid-card > small {
  display: flex;
  gap: 5px;
  padding: 0 2px 2px;
  color: var(--muted);
  font-size: 11px;
}

.file-row {
  display: grid;
  grid-template-columns: 42px minmax(220px, 2fr) minmax(85px, 0.6fr) minmax(100px, 0.8fr) minmax(110px, 0.8fr) minmax(125px, 0.8fr) 46px;
  align-items: center;
  width: 100%;
  min-height: 58px;
  padding: 0 12px;
  border: 0;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  text-align: left;
  background: var(--surface);
  -webkit-user-select: none;
  user-select: none;
}

.file-row--header {
  min-height: 40px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 600;
  background: var(--surface-subtle);
}

.file-row--header > button {
  padding: 8px;
  border: 0;
  color: var(--muted);
  font: inherit;
  text-align: left;
  background: transparent;
  cursor: pointer;
}

.file-row--header > button:hover {
  color: var(--text);
}

.file-row--entry {
  content-visibility: auto;
  contain-intrinsic-size: auto 58px;
  font: inherit;
  cursor: default;
  outline: none;
}

.file-row--entry:hover,
.file-row--selected {
  background: color-mix(in srgb, var(--brand) 6%, var(--surface));
}

.file-row--entry:focus-visible {
  box-shadow: inset 3px 0 0 var(--brand);
}

.file-row--drop-target {
  background: color-mix(in srgb, var(--brand) 13%, var(--surface));
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--brand) 68%, transparent);
}

.file-row > span {
  min-width: 0;
  padding: 8px;
  color: var(--muted);
  font-size: 13px;
}

.file-row input {
  accent-color: var(--brand);
}

.file-name {
  display: flex;
  align-items: center;
  gap: 11px;
  color: var(--text) !important;
  cursor: pointer;
}

.file-name > span:last-child {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.file-name strong,
.file-name small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-name small,
.file-row > span > small {
  color: var(--muted);
  font-size: 11px;
}

.file-name__mobile-meta {
  display: none;
}

.file-icon {
  display: grid;
  flex: 0 0 34px;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 9px;
  color: var(--file-icon-color, var(--blue));
  background: color-mix(in srgb, var(--file-icon-color, var(--blue)) 11%, var(--surface));
}

.file-icon--folder,
.file-grid-card__icon--folder,
.file-icon--spreadsheet,
.file-grid-card__icon--spreadsheet {
  --file-icon-color: var(--brand);
}

.file-icon--image,
.file-grid-card__icon--image,
.file-icon--code,
.file-grid-card__icon--code {
  --file-icon-color: var(--blue);
}

.file-icon--media,
.file-grid-card__icon--media {
  --file-icon-color: #9567dc;
}

.file-icon--archive,
.file-grid-card__icon--archive,
.file-icon--package,
.file-grid-card__icon--package {
  --file-icon-color: var(--amber);
}

.file-icon--database,
.file-grid-card__icon--database {
  --file-icon-color: #168e9c;
}

.file-icon--presentation,
.file-grid-card__icon--presentation {
  --file-icon-color: #d96b54;
}

.file-icon--secret,
.file-grid-card__icon--secret {
  --file-icon-color: var(--danger);
}

.file-icon--document,
.file-grid-card__icon--document,
.file-icon--generic,
.file-grid-card__icon--generic {
  --file-icon-color: var(--muted);
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 12px !important;
}

.file-empty,
.file-loading {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 8px;
  min-height: 360px;
  color: var(--muted);
}

.file-empty strong {
  color: var(--text);
}

.file-limit {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 10px 15px;
  color: var(--amber);
  font-size: 12px;
  text-align: center;
}

.file-limit .button {
  min-height: 32px;
  padding: 6px 12px;
}

.drop-overlay {
  position: absolute;
  z-index: 4;
  inset: 10px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 12px;
  border: 2px dashed var(--brand);
  border-radius: 14px;
  color: var(--brand);
  background: color-mix(in srgb, var(--surface) 90%, transparent);
  backdrop-filter: blur(5px);
  pointer-events: none;
}

.file-internal-drop-hint {
  position: absolute;
  z-index: 5;
  bottom: 14px;
  left: 50%;
  display: flex;
  max-width: calc(100% - 28px);
  align-items: center;
  gap: 9px;
  padding: 9px 13px;
  border: 1px solid color-mix(in srgb, var(--brand) 42%, var(--border));
  border-radius: 12px;
  color: var(--brand);
  background: color-mix(in srgb, var(--surface) 91%, transparent);
  box-shadow: 0 12px 28px rgb(0 0 0 / 20%);
  transform: translateX(-50%);
  backdrop-filter: blur(14px) saturate(135%);
  pointer-events: none;
}

.file-internal-drop-hint span {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.file-internal-drop-hint strong,
.file-internal-drop-hint small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-internal-drop-hint strong {
  font-size: 14px;
}

.file-internal-drop-hint small {
  color: var(--muted);
  font-size: 13px;
}

.file-context-menu {
  position: fixed;
  z-index: 90;
  display: grid;
  box-sizing: border-box;
  width: 196px;
  max-width: var(--context-menu-max-width, calc(100vw - 16px));
  max-height: var(--context-menu-max-height, calc(100dvh - 16px));
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 6px;
  border: 1px solid var(--border);
  border-radius: 11px;
  background: var(--surface);
  box-shadow: var(--shadow-md);
}

.file-context-menu button {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 10px;
  border: 0;
  border-radius: 7px;
  color: var(--text);
  text-align: left;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 1.3;
}

.file-context-menu hr {
  width: 100%;
  margin: 4px 0;
  border: 0;
  border-top: 1px solid var(--border);
}

.operation-form {
  display: grid;
  gap: 14px;
}

.operation-form label {
  display: grid;
  gap: 8px;
}

.operation-form label > span {
  font-weight: 600;
}

.operation-form input,
.operation-form select {
  width: 100%;
  height: 42px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  color: var(--text);
  background: var(--surface);
  outline: none;
}

.operation-form input:focus,
.operation-form select:focus {
  border-color: var(--brand);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 16%, transparent);
}

.remote-download-form label > span small {
  margin-left: 4px;
  color: var(--muted);
  font-size: 14px;
  font-weight: 500;
}

.remote-download-note {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: start;
  gap: 9px;
  padding: 11px 12px;
  border: 1px solid color-mix(in srgb, var(--brand) 20%, var(--border));
  border-radius: 10px;
  color: var(--muted);
  background: color-mix(in srgb, var(--brand) 5%, var(--surface));
  font-size: 14px;
  line-height: 1.55;
}

.remote-download-note svg {
  margin-top: 2px;
  color: var(--brand);
}

.remote-download-warning,
.remote-download-error {
  margin: -5px 0 0;
  font-size: 14px;
  line-height: 1.5;
}

.remote-download-warning {
  color: var(--amber);
}

.remote-download-error {
  color: var(--danger);
}

.archive-hint {
  color: var(--muted);
  line-height: 1.6;
}

.trash-summary {
  display: grid;
  place-items: center;
  gap: 8px;
  padding: 16px 0 8px;
  text-align: center;
}

.trash-summary svg {
  color: var(--danger);
}

.trash-summary span {
  max-width: 380px;
  color: var(--muted);
  font-size: 13px;
}

.trash-manager {
  display: grid;
  gap: 12px;
}

.trash-manager > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.trash-manager > header > div {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.trash-list {
  max-height: min(56vh, 560px);
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 12px;
}

.trash-item {
  display: grid;
  grid-template-columns: 28px 38px minmax(180px, 1fr) 100px 128px;
  align-items: center;
  gap: 8px;
  min-height: 62px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  cursor: pointer;
}

.trash-item:last-child {
  border-bottom: 0;
}

.trash-item:hover {
  background: var(--interaction-hover);
}

.trash-item > span:nth-child(3) {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.trash-item strong,
.trash-item small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trash-item small,
.trash-item > span:nth-last-child(-n + 2) {
  color: var(--muted);
  font-size: 12px;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 9px;
  padding-top: 20px;
}

.preview-loading {
  display: flex;
  min-height: 420px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--muted);
}

.code-viewer {
  overflow: hidden;
  border: 1px solid var(--file-preview-border);
  border-radius: var(--radius, 12px);
  background: var(--file-preview-background);
  box-shadow: var(--file-preview-shadow);
}

.code-viewer > header,
.code-viewer > footer {
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 7px 12px;
  color: var(--file-preview-muted);
  font-size: 12px;
  background: var(--file-preview-panel);
}

.code-viewer > header > span:first-child {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--file-preview-text);
}

.code-viewer__header-right,
.code-editor-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}

.code-editor-tools {
  gap: 3px;
}

.code-editor-tool {
  display: inline-grid;
  width: 28px;
  height: 28px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: var(--file-preview-muted);
  background: transparent;
  cursor: pointer;
}

.code-editor-tool:hover,
.code-editor-tool.is-active {
  color: var(--file-preview-text);
  background: var(--file-preview-panel-raised);
}

.code-editor-tool.is-active {
  color: var(--file-preview-accent);
}

.code-viewer > footer {
  justify-content: flex-start;
  flex-wrap: wrap;
}

.code-editor-actions {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-left: auto;
}

.code-editor-actions .button {
  min-height: 30px;
  padding: 5px 9px;
}

.code-editor {
  position: relative;
  height: min(60vh, 620px);
  overflow: hidden;
}

.code-editor > * {
  height: 100%;
}

:global(.modal-panel--fullscreen .code-viewer) {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

:global(.modal-panel--fullscreen .code-editor) {
  height: auto;
  min-height: 0;
  flex: 1 1 auto;
}

.media-viewer {
  position: relative;
  display: flex;
  min-height: 0;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 14px;
  overflow: hidden;
  border: 1px solid var(--file-preview-border);
  border-radius: 16px;
  background:
    radial-gradient(circle at 50% -12%, var(--file-preview-glow), transparent 42%),
    linear-gradient(180deg, var(--file-preview-panel) 0%, var(--file-preview-background) 100%);
  box-shadow: var(--file-preview-shadow);
}

.media-viewer--video,
.media-viewer--image,
.media-viewer--metadata {
  min-height: min(58vh, 600px);
}

.media-player {
  position: relative;
  display: grid;
  width: min(100%, 1120px);
  min-width: 0;
  aspect-ratio: 16 / 9;
  place-items: center;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--file-preview-text) 10%, transparent);
  border-radius: 14px;
  background: #000;
  box-shadow: 0 18px 46px rgb(0 0 0 / 30%);
}

.media-player video {
  display: block;
  width: 100%;
  height: 100%;
  max-width: none;
  max-height: none;
  object-fit: contain;
  background: #000;
}

.media-player__loading,
.media-player__error {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  padding: 20px;
  color: var(--file-preview-text);
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--file-preview-background) 8%, transparent),
    color-mix(in srgb, var(--file-preview-background) 66%, transparent)
  );
  text-align: center;
}

.media-player__loading {
  pointer-events: none;
}

.media-player__error {
  flex-direction: column;
  gap: 6px;
  color: var(--danger);
  pointer-events: auto;
}

.media-player__error span {
  color: color-mix(in srgb, var(--danger) 72%, var(--file-preview-text));
  font-size: 13px;
}

.media-player__error .button {
  margin-top: 4px;
}

.media-viewer img {
  display: block;
  width: auto;
  max-width: 100%;
  max-height: min(68vh, 640px);
  border-radius: 10px;
  object-fit: contain;
  box-shadow: 0 18px 46px rgb(0 0 0 / 24%);
}

.media-viewer audio {
  width: min(720px, 100%);
}

.media-viewer iframe {
  width: 100%;
  min-height: min(68vh, 680px);
  border: 0;
  border-radius: 10px;
  background: var(--file-preview-panel);
}

.media-viewer--pdf {
  align-items: stretch;
  padding: 0;
}

.media-viewer--pdf iframe {
  min-height: min(68vh, 680px);
  border-radius: 14px;
}

.media-viewer__footer {
  display: flex;
  width: min(100%, 1120px);
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: var(--file-preview-muted);
  font-size: 13px;
}

.media-viewer__status {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
}

.media-viewer__status i {
  display: inline-block;
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--file-preview-accent);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--file-preview-accent) 13%, transparent);
}

.media-viewer__status.is-loading i {
  background: var(--amber);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--amber) 13%, transparent);
}

.media-viewer__status.is-error i {
  background: var(--danger);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--danger) 13%, transparent);
}

:global(.modal-panel--wide:not(.modal-panel--fullscreen):has(.media-viewer)) {
  width: min(1080px, calc(100vw - 32px));
}

:global(.modal-panel--wide:has(.media-viewer) .modal-panel__body) {
  padding: 10px;
  background: var(--surface-subtle);
}

:global(.modal-panel--fullscreen .media-viewer) {
  height: 100%;
  min-height: 0;
}

:global(.modal-panel--fullscreen .media-player) {
  max-height: calc(100% - 42px);
}

.metadata-viewer {
  display: grid;
  place-items: center;
  gap: 11px;
  padding: 40px;
  color: var(--file-preview-muted);
  text-align: center;
}

.metadata-viewer strong {
  color: var(--file-preview-text);
}

.spinning {
  animation: spin 0.9s linear infinite;
}

.slide-enter-active,
.slide-leave-active {
  transition: 0.16s ease;
}

.slide-enter-from,
.slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.batch-dock-enter-active,
.batch-dock-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.batch-dock-enter-from,
.batch-dock-leave-to {
  opacity: 0;
  transform: translate(-50%, 10px);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1040px) {
  .file-row {
    grid-template-columns: 42px minmax(220px, 2fr) 90px 118px 46px;
  }

  .file-row > span:nth-child(4),
  .file-row > span:nth-child(5) {
    display: none;
  }
}

@media (max-width: 1100px) {
  .file-command-bar {
    flex-wrap: wrap;
  }

  .file-command-bar__actions {
    width: 100%;
    min-width: 0;
    margin-left: 0;
    flex: 1 1 100%;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .batch-bar {
    left: 50%;
    width: calc(100vw - 32px);
  }
}

@media (max-width: 720px) {
  .files-page {
    gap: 12px;
  }

  .file-command-bar {
    align-items: stretch;
    flex-direction: column;
    flex-wrap: nowrap;
  }

  .file-shortcuts {
    flex-wrap: nowrap;
    gap: 6px;
    overflow-x: auto;
    padding-bottom: 2px;
    scrollbar-width: none;
  }

  .file-shortcuts::-webkit-scrollbar {
    display: none;
  }

  .file-shortcuts button {
    min-height: 38px;
    flex: 0 0 auto;
  }

  .file-command-bar__actions {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .file-command-bar__actions .file-command-bar__refresh {
    width: 100%;
    min-width: 0;
  }

  .file-toolbar {
    align-items: stretch;
    flex-direction: column;
    gap: 9px;
    padding: 10px;
  }

  .file-toolbar__path {
    align-items: stretch;
    flex-direction: column;
    gap: 7px;
  }

  .file-host-switcher__trigger {
    max-width: min(100%, 260px);
  }

  .breadcrumbs {
    margin: -2px 0;
  }

  .breadcrumbs button {
    min-height: 38px;
    padding: 5px 3px;
  }

  .file-search {
    width: 100%;
  }

  .file-toolbar__controls {
    width: 100%;
    flex-wrap: wrap;
  }

  .file-toolbar__controls .file-search {
    flex: 1 1 100%;
  }

  .file-grid-sort {
    margin-right: auto;
  }

  .file-grid-sort,
  .file-view-switch {
    min-height: 42px;
  }

  .file-grid-sort button,
  .file-view-switch button {
    width: 36px;
    height: 34px;
  }

  .file-grid {
    grid-template-columns: repeat(auto-fill, minmax(138px, 1fr));
    gap: 8px;
    padding: 10px;
  }

  .file-grid-card {
    padding: 8px;
    cursor: pointer;
    touch-action: manipulation;
  }

  .file-grid-card__check,
  .file-grid-card__menu {
    top: 13px;
    opacity: 1;
  }

  .file-grid-card__check {
    left: 13px;
  }

  .file-grid-card__menu {
    right: 13px;
  }

  .batch-bar {
    bottom: max(10px, env(safe-area-inset-bottom));
    width: calc(100vw - 20px);
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 4px;
    overflow: visible;
    padding: 8px;
    border-radius: 12px;
  }

  .batch-bar strong {
    grid-column: 1 / -1;
    margin: 0;
    padding: 2px 6px 5px;
  }

  .batch-bar button {
    min-height: 40px;
    justify-content: center;
    gap: 3px;
    padding: 5px 2px;
    font-size: 11px;
    white-space: nowrap;
  }

  .clipboard-bar {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .clipboard-bar button:first-of-type {
    grid-column: 1 / -1;
    justify-content: center;
    grid-row: 2;
  }

  .remote-download-task {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .remote-download-task__actions {
    grid-column: auto;
  }

  .remote-download-task__actions button,
  .remote-download-tasks__header button {
    min-height: 44px;
  }

  .remote-download-tasks__refresh {
    width: 44px;
    min-width: 44px;
  }

  .file-row {
    grid-template-columns: 38px minmax(0, 1fr) 46px;
    min-height: 62px;
    padding: 0 6px;
  }

  .file-row--entry {
    cursor: pointer;
    touch-action: manipulation;
  }

  .file-row > span {
    padding: 7px 5px;
  }

  .file-name__desktop-meta {
    display: none;
  }

  .file-name__mobile-meta {
    display: block;
  }

  .row-menu {
    width: 38px;
    height: 38px;
  }

  .file-row > :nth-child(3),
  .file-row > :nth-child(4),
  .file-row > :nth-child(5),
  .file-row > :nth-child(6) {
    display: none;
  }

  .file-context-menu {
    top: auto !important;
    right: max(10px, var(--context-menu-safe-right, 10px), env(safe-area-inset-right));
    bottom: max(10px, var(--context-menu-safe-bottom, 10px), env(safe-area-inset-bottom));
    left: max(10px, var(--context-menu-safe-left, 10px), env(safe-area-inset-left)) !important;
    width: auto;
    max-height: min(66dvh, 520px, var(--context-menu-max-height, 66dvh));
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 3px;
    overflow-y: auto;
    padding: 9px;
    border-radius: 16px;
    box-shadow: 0 18px 52px rgb(0 0 0 / 28%);
  }

  .file-context-menu button {
    min-height: 44px;
    padding: 9px 10px;
  }

  .file-context-menu hr {
    grid-column: 1 / -1;
  }

  .code-editor {
    grid-template-columns: 44px 1fr;
  }

  .code-editor-actions {
    margin-left: auto;
  }

  :global(.modal-panel--wide:has(.media-viewer)) {
    width: calc(100vw - 20px);
    max-height: calc(100dvh - 20px);
  }

  :global(.modal-panel--wide:has(.media-viewer) .modal-panel__header) {
    padding: 12px;
  }

  :global(.modal-panel--wide:has(.media-viewer) .modal-panel__header p) {
    max-width: calc(100vw - 128px);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  :global(.modal-panel--wide:has(.media-viewer) .modal-panel__body) {
    padding: 8px;
  }

  .media-viewer {
    gap: 9px;
    padding: 8px;
    border-radius: 12px;
  }

  .media-viewer--video,
  .media-viewer--image,
  .media-viewer--metadata {
    min-height: 0;
  }

  .media-player,
  .media-player video {
    border-radius: 10px;
  }

  .media-viewer__footer {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .media-viewer iframe,
  .media-viewer--pdf iframe {
    min-height: 60dvh;
  }

  .code-viewer__header-right > span:first-child {
    display: none;
  }

  .trash-manager > header {
    align-items: stretch;
    flex-direction: column;
  }

  .trash-manager > header > div {
    justify-content: flex-start;
  }

  .trash-item {
    grid-template-columns: 26px 34px minmax(0, 1fr);
  }

  .trash-item > span:nth-last-child(-n + 2) {
    display: none;
  }
}

@media (max-width: 480px) {
  .file-command-bar__actions,
  .batch-bar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .file-toolbar {
    padding: 9px;
    border-radius: 13px;
  }

  .file-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .file-grid-card {
    min-height: 112px;
  }

  .clipboard-bar {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .clipboard-bar > span:first-child {
    display: none;
  }

  .file-context-menu {
    right: max(8px, var(--context-menu-safe-right, 8px), env(safe-area-inset-right));
    bottom: max(8px, var(--context-menu-safe-bottom, 8px), env(safe-area-inset-bottom));
    left: max(8px, var(--context-menu-safe-left, 8px), env(safe-area-inset-left)) !important;
  }
}
</style>
