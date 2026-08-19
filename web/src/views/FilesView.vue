<script setup lang="ts">
import { computed, defineAsyncComponent, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/FilesView/en-US').then((module) => module.default)
  : import('@/i18n/pages/FilesView/zh-TW').then((module) => module.default))
import {
  Archive,
  ChevronRight,
  ClipboardPaste,
  Code2,
  Copy,
  Download,
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
  ShieldCheck,
  Trash2,
  Upload,
  WrapText,
  X,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { ApiError, api } from '@/lib/api'
import {
  desktopCloseGuardCoordinator,
  desktopWindowActiveKey,
  desktopWindowCloseGuardKey,
} from '@/lib/desktopRouteKeys'
import type { CodeLanguage } from '@/lib/code-editor-language'
import { transferCrossPanelFileBatch } from '@/lib/crossPanelFileTransfer'
import { fileEntryIcon as entryIcon, fileEntryIconKind as entryIconKind } from '@/lib/fileEntryPresentation'
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
import type { FileActionInput, FileActionResult, FileDirectory, FileEntry, FileTrashEntry } from '@/types/api'

const CodeEditor = defineAsyncComponent(() => import('@/components/files/CodeEditor.vue'))
const route = useRoute()
const router = useRouter()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))
const desktopWindowCloseGuards = inject(desktopWindowCloseGuardKey, undefined)
const filesPage = ref<HTMLElement>()
const localClusterNodeId = ref('')
let unregisterWindowCloseGuard: (() => void) | undefined

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

const toast = useToast()
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
const uploadProgress = ref<Record<string, number>>({})
const dialogAction = ref<DialogAction>()
const dialogValue = ref('')
const dialogFormat = ref<ArchiveFormat>('tar.gz')
const dialogBusy = ref(false)
const dialogEntries = ref<FileEntry[]>([])
const contextMenu = ref<{ entry?: FileEntry; x: number; y: number }>()
const fileClipboard = useFileClipboard()
const clipboard = fileClipboard.clipboard
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
}>()
const desktopAdding = ref(false)
const previewEntry = ref<FileEntry>()
const previewContent = ref('')
const previewLoading = ref(false)
const previewSaving = ref(false)
const previewDirty = ref(false)
const mediaLoading = ref(false)
const mediaReady = ref(false)
const mediaError = ref(false)
const mediaErrorMessage = ref('')
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
let archiveController: AbortController | undefined
let fileTransferController: AbortController | undefined
let fileTransferClearTimer: number | undefined
let unsubscribeFileDirectoryChanges: (() => void) | undefined
let searchTimer: number | undefined
let mediaLoadTimer: number | undefined
let unmounted = false
let openedRouteFile = ''
const fileWindowChangeOrigin = Symbol('file-window')

const fileViewStorageKey = 'kpanel:files:view:v1'
const thumbnailSourceMaxBytes = 12 * 1024 * 1024
const mediaLoadTimeoutMs = 20_000

const fileTransferTitle = computed(() => {
  const state = fileTransferState.value
  if (!state) return ''
  if (state.remote) {
    if (state.phase === 'running') return `正在从另一台 KPanel 复制 ${state.completed || 0}/${state.count} 项`
    if (state.phase === 'success') return `跨面板复制完成（${state.count} 项）`
    if (state.phase === 'cancelled') return '跨面板复制已取消'
    if (state.phase === 'error') return '跨面板复制失败'
    return `跨面板复制部分完成（${state.count} 项）`
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
  ? `从另一台 KPanel 复制到 ${internalDropTarget.value}`
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
  previewEntry.value ? api.files.contentUrl(previewEntry.value.path, 'inline') : '',
)
const dialogTitle = computed(() => {
  const titles: Record<DialogAction, string> = {
    mkdir: '新建文件夹',
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
    const result = await api.files.list(
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
    }
  }
}

async function navigateDirectory(path: string): Promise<void> {
  const resolvedPath = await loadDirectory(path)
  const routePath = requestedFilePath(route.query.path) || '/'
  if (!resolvedPath || resolvedPath === routePath) return
  await router.push({ name: 'files', query: { path: resolvedPath } })
}

async function openRequestedFile(value: unknown): Promise<void> {
  const filePath = requestedFilePath(value)
  if (!filePath || filePath === '/' || filePath === openedRouteFile) return
  openedRouteFile = filePath
  try {
    const entry = await api.files.entry(filePath)
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
  await loadDirectory(requestedFilePath(route.query.path) || '/')
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
  if (!isVideo) return
  mediaLoadTimer = window.setTimeout(() => {
    if (!mediaReady.value && !mediaError.value && previewEntry.value?.path === entry?.path) {
      mediaLoading.value = false
      mediaError.value = true
      mediaErrorMessage.value = '视频流响应超时，请检查网络或服务器。'
    }
    mediaLoadTimer = undefined
  }, mediaLoadTimeoutMs)
}

async function openPreview(entry: FileEntry): Promise<void> {
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
    previewContent.value = await api.files.text(entry.path)
  } catch (error) {
    toast.danger('文件打开失败', errorMessage(error))
    previewEntry.value = undefined
  } finally {
    previewLoading.value = false
  }
}

function handleMediaLoadStart(): void {
  mediaLoading.value = true
  mediaReady.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
}

function handleMediaReady(): void {
  mediaReady.value = true
  mediaLoading.value = false
  mediaError.value = false
  clearMediaLoadTimer()
}

function handleMediaCanPlay(): void {
  handleMediaReady()
}

function handleMediaWaiting(): void {
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
  previewEntry.value = undefined
  previewContent.value = ''
  previewDirty.value = false
  clearMediaLoadTimer()
  mediaLoading.value = false
  mediaReady.value = false
  mediaError.value = false
  mediaErrorMessage.value = ''
  editorInfo.value = undefined
  editorStatus.value = undefined
  editorLineWrap.value = false
}

async function savePreview(content?: string): Promise<void> {
  const entry = previewEntry.value
  if (!entry || !entry.editable || !previewDirty.value) return
  const nextContent = content ?? codeEditorRef.value?.getValue() ?? previewContent.value
  previewContent.value = nextContent
  previewSaving.value = true
  try {
    const result = await api.files.write(entry.path, nextContent, entry.resourceVersion)
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
    ? api.files.contentUrl(targets[0]!.path, 'attachment')
    : nativeArchiveName
      ? api.files.archiveUrl(targets, nativeArchiveName)
      : undefined
  if (!beginDesktopFileDrag(
    event,
    targets,
    localClusterNodeId.value,
    'file-manager',
    nativeDownloadURL,
    nativeArchiveName,
  )) event.preventDefault()
}

function finishEntryDrag(): void {
  clearDesktopFileDrag()
}

function clearInternalDropTarget(): void {
  internalDropTarget.value = ''
  internalDropCount.value = 0
  crossPanelDropActive.value = false
}

function updateInternalDropTarget(event: DragEvent, target: string): boolean {
  if (fileTransferState.value?.phase === 'running') return false
  if (!hasDesktopFileDrag(event) && hasCrossPanelFileDrag(event)) {
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
  if (fileTransferClearTimer !== undefined) window.clearTimeout(fileTransferClearTimer)
  fileTransferClearTimer = window.setTimeout(() => {
    fileTransferState.value = undefined
    fileTransferClearTimer = undefined
  }, 2200)
}

function cancelFileTransfer(): void {
  fileTransferController?.abort()
}

async function transferInternalFileDrop(event: DragEvent, target: string): Promise<void> {
  const entries = desktopFileDragEntries(event)
  const operation = fileTransferOperation(event)
  clearInternalDropTarget()
  clearDesktopFileDrag()
  if (!entries.length || fileTransferTargetError(entries, target)) return
  if (fileTransferState.value?.phase === 'running') {
    toast.show('已有文件操作正在进行')
    return
  }

  if (fileTransferClearTimer !== undefined) window.clearTimeout(fileTransferClearTimer)
  // Copies may be cancelled safely because they never change desktop shortcut
  // targets. A move must run to a server result so successful path mappings can
  // be applied to desktop shortcuts without leaving stale references behind.
  const controller = operation === 'copy' ? new AbortController() : undefined
  fileTransferController = controller
  fileTransferState.value = { mode: operation, target, count: entries.length, phase: 'running' }
  try {
    const result = await api.files.action({
      action: operation,
      sources: entries.map((entry) => entry.path),
      target,
      expectedResourceVersions: Object.fromEntries(
        entries.flatMap((entry) => entry.resourceVersion ? [[entry.path, entry.resourceVersion]] : []),
      ),
    }, controller?.signal)
    const shortcutSyncFailed = await applySuccessfulFileChanges(result, target)
    const partial = Boolean(result.failed.length || shortcutSyncFailed)
    fileTransferState.value = {
      mode: operation,
      target,
      count: result.succeeded.length,
      phase: partial ? 'partial' : 'success',
    }
    if (partial) {
      toast.danger(
        operation === 'copy' ? '部分文件未复制' : '部分文件未移动',
        shortcutSyncFailed
          ? '真实文件已移动，但桌面快捷方式路径同步失败，请刷新后重试。'
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`,
      )
    } else {
      toast.success(operation === 'copy' ? '复制完成' : '移动完成', `${result.succeeded.length} 项已传输到 ${target}`)
    }
  } catch (error) {
    if (controller?.signal.aborted) {
      fileTransferState.value = { mode: operation, target, count: entries.length, phase: 'cancelled' }
      toast.show('复制已取消', { message: '已经复制完成的项目会保留在目标目录。' })
    } else {
      fileTransferState.value = { mode: operation, target, count: entries.length, phase: 'error' }
      toast.danger(operation === 'copy' ? '复制失败' : '移动失败', errorMessage(error))
    }
    if (!unmounted) await loadDirectory()
  } finally {
    if (fileTransferController === controller) fileTransferController = undefined
    if (!unmounted) scheduleFileTransferClear()
  }
}

async function transferCrossPanelFileDrop(event: DragEvent, target: string): Promise<void> {
  const payload = crossPanelFileDragEntries(event)
  clearInternalDropTarget()
  if (!payload) {
    toast.danger('跨面板复制失败', '拖拽数据无效或超过 64 项，请从来源 KPanel 重新拖动。')
    return
  }
  if (payload.sourceNodeId === localClusterNodeId.value) {
    toast.show('来源和目标是同一个 KPanel', { message: '请在文件管理器中使用复制或移动。' })
    return
  }
  if (fileTransferState.value?.phase === 'running') {
    toast.show('已有文件操作正在进行')
    return
  }
  if (fileTransferClearTimer !== undefined) window.clearTimeout(fileTransferClearTimer)
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
      api.files.transferFromPanel,
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
    }
    if (result.succeeded.length) {
      if (!unmounted) await loadDirectory()
      notifyFileDirectoriesChanged([target, currentPath.value], fileWindowChangeOrigin)
    }
    if (result.cancelled) {
      toast.show('跨面板复制已取消', {
        message: `已完成的 ${result.succeeded.length} 项会保留在目标目录。`,
      })
    } else if (phase === 'success') {
      toast.success('跨面板复制完成', `${result.succeeded.length} 项已复制到 ${target}`)
    } else {
      toast.danger(
        phase === 'error' ? '跨面板复制失败' : '跨面板复制部分完成',
        `${result.succeeded.length} 项成功，${result.failed.length} 项失败${result.failed[0]?.detail ? `：${result.failed[0].detail}` : ''}`,
      )
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

function showContext(event: MouseEvent, entry: FileEntry): void {
  event.preventDefault()
  selectForContext(entry)
  contextMenu.value = {
    entry,
    x: Math.max(8, Math.min(event.clientX, window.innerWidth - 210)),
    y: Math.max(8, Math.min(event.clientY, window.innerHeight - 430)),
  }
}

function showDirectoryContext(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (
    target.closest(
      '.file-row--entry, .file-grid-card, .file-toolbar, .clipboard-bar, .upload-strip, .file-limit, .drop-overlay',
    )
  ) {
    return
  }
  event.preventDefault()
  contextMenu.value = {
    x: Math.max(8, Math.min(event.clientX, window.innerWidth - 230)),
    y: Math.max(8, Math.min(event.clientY, window.innerHeight - 220)),
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
  fileClipboard.set(mode, entriesToStore)
  clearSelection()
  toast.success(
    mode === 'copy' ? '已复制到文件剪贴板' : '已剪切到文件剪贴板',
    `${entriesToStore.length} 项，进入目标文件夹后点击“粘贴”`,
  )
}

function clearClipboard(): void {
  fileClipboard.clear()
}

async function applySuccessfulFileChanges(
  result: FileActionResult,
  target?: string,
): Promise<boolean> {
  let shortcutSyncFailed = false
  if (result.succeeded.length && (result.action === 'move' || result.action === 'rename')) {
    try {
      await syncMovedDesktopShortcuts(result)
    } catch {
      shortcutSyncFailed = true
    }
  }
  const changedDirectories = changedFileDirectories(result, target)
  if (!unmounted) await loadDirectory()
  notifyFileDirectoriesChanged(changedDirectories, fileWindowChangeOrigin, successfulFileMoves(result))
  return shortcutSyncFailed
}

async function pasteClipboard(target = currentPath.value): Promise<void> {
  const stored = clipboard.value
  if (!stored?.entries.length || pasteBusy.value) return
  contextMenu.value = undefined
  pasteBusy.value = true
  try {
    const result = await api.files.action({
      action: stored.mode,
      sources: stored.entries.map((entry) => entry.path),
      target,
      expectedResourceVersions: Object.fromEntries(
        stored.entries.map((entry) => [entry.path, entry.resourceVersion]),
      ),
    })
    if (stored.mode === 'move') {
      const failed = new Set(result.failed.map((item) => item.path))
      const remaining = stored.entries.filter((entry) => failed.has(entry.path))
      if (remaining.length) fileClipboard.set('move', remaining)
      else fileClipboard.clear()
    }
    const shortcutSyncFailed = await applySuccessfulFileChanges(result, target)
    if (result.failed.length || shortcutSyncFailed) {
      toast.danger(
        result.succeeded.length ? '部分文件未粘贴' : '粘贴未完成',
        shortcutSyncFailed
          ? '文件已移动，但桌面快捷方式路径同步失败，请刷新后重试。'
          : `${result.succeeded.length} 项成功，${result.failed.length} 项失败：${result.failed[0]?.detail || '请刷新后重试'}`,
      )
    } else {
      toast.success(
        stored.mode === 'copy' ? '复制完成' : '移动完成',
        `${result.succeeded.length} 项已粘贴到 ${target}`,
      )
    }
  } catch (error) {
    toast.danger('粘贴失败', errorMessage(error))
    await loadDirectory()
  } finally {
    pasteBusy.value = false
  }
}

async function submitDialog(): Promise<void> {
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
      ? await api.files.action(input, controller.signal)
      : await api.files.action(input)
    const shortcutSyncFailed = await applySuccessfulFileChanges(result)
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
  trashLoading.value = true
  try {
    const result = await api.files.trash()
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
    const result = await api.files.action(input)
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
    const ticket = await api.files.createDownloadTicket(entry.path)
    const anchor = document.createElement('a')
    anchor.href = ticket.downloadUrl
    anchor.download = entry.name
    anchor.rel = 'noopener'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } catch (error) {
    toast.danger('下载失败', errorMessage(error))
  }
}

async function downloadSelected(entry?: FileEntry): Promise<void> {
  contextMenu.value = undefined
  for (const selectedEntry of entriesForBatch(entry)) {
    if (selectedEntry.kind === 'file') await download(selectedEntry)
  }
}

function setSort(key: 'name' | 'size' | 'modified'): void {
  if (sortKey.value === key) sortDescending.value = !sortDescending.value
  else {
    sortKey.value = key
    sortDescending.value = false
  }
}

async function uploadFiles(files: FileList | File[]): Promise<void> {
  const values = Array.from(files)
  if (!values.length) return
  for (const file of values) {
    uploadProgress.value = { ...uploadProgress.value, [file.name]: 0 }
    try {
      await api.files.upload(currentPath.value, file, false, (progress) => {
        uploadProgress.value = { ...uploadProgress.value, [file.name]: progress }
      })
      uploadProgress.value = { ...uploadProgress.value, [file.name]: 100 }
    } catch (error) {
      if (
        error instanceof ApiError &&
        error.status === 409 &&
        window.confirm(`${file.name} 已存在，是否覆盖？`)
      ) {
        try {
          await api.files.upload(currentPath.value, file, true, (progress) => {
            uploadProgress.value = { ...uploadProgress.value, [file.name]: progress }
          })
          uploadProgress.value = { ...uploadProgress.value, [file.name]: 100 }
          continue
        } catch (overwriteError) {
          toast.danger(`${file.name} 覆盖失败`, errorMessage(overwriteError))
        }
      } else {
        toast.danger(`${file.name} 上传失败`, errorMessage(error))
      }
      const next = { ...uploadProgress.value }
      delete next[file.name]
      uploadProgress.value = next
    }
  }
  if (uploadInput.value) uploadInput.value.value = ''
  await loadDirectory()
  window.setTimeout(() => {
    uploadProgress.value = {}
  }, 1800)
}

function onDrop(event: DragEvent): void {
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
  if (event.dataTransfer?.files?.length) void uploadFiles(event.dataTransfer.files)
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
  return api.files.thumbnailUrl(entry.path, entry.resourceVersion)
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
}

function handleFileShortcut(event: KeyboardEvent): void {
  if (!desktopWindowActive.value) return
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

onMounted(() => {
  const guard = () => !previewDirty.value || window.confirm('文件尚未保存，确认关闭窗口吗？')
  unregisterWindowCloseGuard = desktopWindowCloseGuards
    ? desktopWindowCloseGuards.register(guard)
    : desktopCloseGuardCoordinator.register('classic-files', guard)
  window.addEventListener('click', handleWindowClick)
  window.addEventListener('keydown', handleFileShortcut)
  unsubscribeFileDirectoryChanges = subscribeFileDirectoryChanges((directories, origin, moves = []) => {
    if (origin === fileWindowChangeOrigin) return
    const relocatedPath = remapMovedFilePath(currentPath.value, moves)
    if (relocatedPath !== currentPath.value) {
      void navigateDirectory(relocatedPath)
      return
    }
    if (!directories.has(currentPath.value)) return
    void loadDirectory()
  })
  restoreViewMode()
  void api.cluster.hosts()
    .then((hosts) => { localClusterNodeId.value = hosts.nodeId })
    .catch(() => { localClusterNodeId.value = '' })
  void loadRequestedRoute()
})

watch(
  () => [route.query.path, route.query.file] as const,
  ([pathValue, fileValue], previous) => {
    if (pathValue === previous?.[0] && fileValue === previous?.[1]) return
    const directoryPath = requestedFilePath(pathValue) || '/'
    void (async () => {
      if (directoryPath !== currentPath.value) await loadDirectory(directoryPath)
      if (fileValue !== previous?.[1]) {
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
  unsubscribeFileDirectoryChanges?.()
  clearDesktopFileDrag()
  unmounted = true
  directoryController?.abort()
  archiveController?.abort()
  fileTransferController?.abort()
  if (fileTransferClearTimer !== undefined) window.clearTimeout(fileTransferClearTimer)
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  clearMediaLoadTimer()
  window.removeEventListener('click', handleWindowClick)
  window.removeEventListener('keydown', handleFileShortcut)
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
        <button class="button button--secondary button--small" type="button" :disabled="loading" title="刷新目录" aria-label="刷新目录" @click="loadDirectory()">
          <RefreshCw :size="15" :class="{ spinning: loading }" /> 刷新
        </button>
        <button class="button button--secondary button--small" type="button" title="打开回收站" aria-label="打开回收站" @click="openTrash">
          <Trash2 :size="15" /> 回收站
        </button>
        <button class="button button--secondary button--small" type="button" title="新建文件夹" aria-label="新建文件夹" @click="openDialog('mkdir')">
          <Plus :size="15" /> 新建文件夹
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
        <div
          v-if="fileTransferState"
          class="file-transfer-status"
          :class="`file-transfer-status--${fileTransferState.phase}`"
          role="status"
          aria-live="polite"
        >
          <span class="file-transfer-status__icon">
            <RefreshCw v-if="fileTransferState.phase === 'running'" :size="17" class="spinning" />
            <Copy v-else-if="fileTransferState.mode === 'copy'" :size="17" />
            <Scissors v-else :size="17" />
          </span>
          <span>
            <strong>{{ fileTransferTitle }}</strong>
            <small>{{ fileTransferState.currentName
              ? `${fileTransferState.currentName} → ${fileTransferState.target}`
              : fileTransferState.target }}</small>
          </span>
          <button
            v-if="fileTransferState.phase === 'running' && fileTransferState.mode === 'copy'"
            type="button"
            @click="cancelFileTransfer"
          >取消</button>
        </div>
      </Transition>

      <div v-if="Object.keys(uploadProgress).length" class="upload-strip">
        <div v-for="(progress, name) in uploadProgress" :key="name">
          <span>{{ name }}</span>
          <div><i :style="{ width: `${progress}%` }" /></div>
          <strong>{{ progress }}%</strong>
        </div>
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
        <span>{{ search ? '换一个关键词试试。' : '可直接拖入文件，或在右上角新建文件夹。' }}</span>
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
          v-if="selectedEntries.some((entry) => entry.kind === 'file')"
          type="button"
          @click="downloadSelected()"
        ><Download :size="15" />下载</button>
        <button type="button" @click="openDialog('compress')"><Archive :size="15" />压缩</button>
        <button type="button" @click="setClipboard('copy')"><Copy :size="15" />复制</button>
        <button type="button" @click="setClipboard('move')"><Scissors :size="15" />剪切</button>
        <button type="button" @click="openDialog('chmod')"><ShieldCheck :size="15" />权限</button>
        <button
          v-if="selectedEntries.some(canAddToDesktop)"
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

    <div
      v-if="contextMenu"
      class="file-context-menu"
      :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
      role="menu"
    >
      <button v-if="contextMenu.entry" type="button" @click="openEntry(contextMenu.entry)">
        <Eye :size="15" />{{ contextMenu.entry.kind === 'directory' ? '打开' : '查看' }}
      </button>
      <button v-if="!contextMenu.entry" type="button" :disabled="desktopAdding" @click="addEntriesToDesktop(undefined, true)">
        <Pin :size="15" />将当前文件夹添加到桌面
      </button>
      <button
        v-if="contextMenu.entry && contextBatchEntries.some((entry) => entry.kind === 'file')"
        type="button"
        @click="downloadSelected(contextMenu.entry)"
      >
        <Download :size="15" />下载
      </button>
      <button
        v-if="contextMenu.entry && !contextHasMultipleEntries && archiveFormat(contextMenu.entry)"
        type="button"
        @click="openDialog('extract', contextMenu.entry)"
      >
        <FolderOpen :size="15" />解压到文件夹
      </button>
      <button v-if="contextMenu.entry" type="button" @click="openDialog('compress', contextMenu.entry)">
        <Archive :size="15" />压缩
      </button>
      <hr v-if="contextMenu.entry" />
      <button v-if="contextMenu.entry && !contextHasMultipleEntries" type="button" @click="openDialog('rename', contextMenu.entry)">
        <Pencil :size="15" />重命名
      </button>
      <button v-if="contextMenu.entry" type="button" @click="setClipboard('copy', contextMenu.entry)"><Copy :size="15" />复制</button>
      <button v-if="contextMenu.entry" type="button" @click="setClipboard('move', contextMenu.entry)"><Scissors :size="15" />剪切</button>
      <button
        v-if="clipboard?.entries.length && contextMenu.entry?.kind === 'directory'"
        type="button"
        :disabled="pasteBusy"
        @click="pasteClipboard(contextMenu.entry.path)"
      ><ClipboardPaste :size="15" />粘贴到此文件夹</button>
      <button
        v-if="clipboard?.entries.length"
        type="button"
        :disabled="pasteBusy"
        @click="pasteClipboard()"
      ><ClipboardPaste :size="15" />粘贴到当前目录</button>
      <button v-if="contextMenu.entry" type="button" @click="openDialog('chmod', contextMenu.entry)">
        <ShieldCheck :size="15" />修改权限
      </button>
      <button
        v-if="contextMenu.entry && contextBatchEntries.some(canAddToDesktop)"
        type="button"
        :disabled="desktopAdding"
        @click="addEntriesToDesktop(contextMenu.entry)"
      >
        <Pin :size="15" />{{ contextHasMultipleEntries ? `添加 ${contextBatchEntries.filter(canAddToDesktop).length} 项到桌面` : '添加到桌面' }}
      </button>
      <button v-if="!contextMenu.entry" type="button" @click="openDialog('mkdir')">
        <Plus :size="15" />新建文件夹
      </button>
      <hr v-if="contextMenu.entry" />
      <button v-if="contextMenu.entry" class="danger-link" type="button" @click="openDialog('trash', contextMenu.entry)">
        <Trash2 :size="15" />移入回收站
      </button>
    </div>

    <ModalDialog
      :open="Boolean(dialogAction)"
      :title="dialogTitle"
      :description="
        dialogAction === 'trash'
          ? '文件将移动到 KPanel 隔离回收区，可在回收站中恢复。'
          : dialogAction === 'compress'
            ? '默认使用适合 Linux 服务器的 tar.gz；也可选择 ZIP 或 TAR。'
            : dialogAction === 'extract'
              ? '内容将解压到全新的文件夹，不覆盖已有文件。'
              : ''
      "
      size="small"
      @close="closeDialog"
    >
      <div v-if="dialogAction !== 'trash'" class="operation-form">
        <label>
          <span>
            {{
              dialogAction === 'mkdir'
                ? '文件夹名称'
                : dialogAction === 'rename'
                  ? '新名称'
                  : dialogAction === 'chmod'
                    ? '权限（八进制）'
                    : dialogAction === 'compress'
                      ? '压缩包名称'
                      : '目标文件夹名称'
            }}
          </span>
          <input
            v-model="dialogValue"
            :placeholder="
              dialogAction === 'chmod'
                ? '例如 644 或 755'
                : dialogAction === 'compress'
                  ? '例如 website.tar.gz'
                  : dialogAction === 'extract'
                    ? '例如 website'
                    : '输入名称'
            "
            autocomplete="off"
            @keydown.enter="submitDialog"
          />
        </label>
        <label v-if="dialogAction === 'compress'">
          <span>压缩格式</span>
          <select v-model="dialogFormat">
            <option value="tar.gz">TAR.GZ（推荐）</option>
            <option value="zip">ZIP（跨平台）</option>
            <option value="tar">TAR（不压缩）</option>
          </select>
        </label>
        <small v-if="dialogAction === 'compress' || dialogAction === 'extract'" class="archive-hint">
          单次最多 100 项、10,000 个条目或解压后 10 GiB；不支持符号链接、硬链接和设备文件。
        </small>
      </div>
      <div v-else class="trash-summary">
        <Trash2 :size="24" />
        <strong>确认移动 {{ dialogEntries.length }} 项？</strong>
        <span>稍后可从文件管理右上角的回收站恢复或彻底删除。</span>
      </div>
      <div class="dialog-actions">
        <button
          class="button button--secondary"
          type="button"
          :disabled="dialogBusy && dialogAction !== 'compress' && dialogAction !== 'extract'"
          @click="dialogBusy ? cancelArchive() : closeDialog()"
        >
          {{ dialogBusy && (dialogAction === 'compress' || dialogAction === 'extract') ? '停止' : '取消' }}
        </button>
        <button
          class="button"
          :class="dialogAction === 'trash' ? 'button--danger' : 'button--primary'"
          type="button"
          :disabled="dialogBusy || (dialogAction !== 'trash' && !dialogValue.trim())"
          @click="submitDialog"
        >
          {{
            dialogBusy
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
                    : '确认'
          }}
        </button>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="trashOpen"
      title="回收站"
      description="删除的文件保存在 Agent 隔离目录中；恢复时不会覆盖同名文件。"
      size="wide"
      @close="!trashBusy && (trashOpen = false)"
    >
      <div class="trash-manager">
        <header>
          <span>共 {{ trashTotal }} 项<span v-if="trashTruncated">（显示最近 500 项）</span></span>
          <div>
            <button
              class="button button--secondary"
              type="button"
              :disabled="trashLoading || trashBusy || !trashEntries.length"
              @click="toggleAllTrash"
            >{{ allTrashSelected ? '取消选择' : '选择当前列表' }}</button>
            <button class="button button--secondary" type="button" :disabled="trashLoading || trashBusy" @click="loadTrash">
              <RefreshCw :size="15" :class="{ spinning: trashLoading }" />刷新
            </button>
            <button
              class="button button--secondary"
              type="button"
              :disabled="trashBusy || !selectedTrash.size || trashEntries.filter((entry) => selectedTrash.has(entry.id)).some((entry) => !entry.restorable)"
              @click="runTrashAction('trash_restore')"
            ><RotateCcw :size="15" />恢复</button>
            <button class="button button--danger" type="button" :disabled="trashBusy || !selectedTrash.size" @click="runTrashAction('trash_delete')">
              <Trash2 :size="15" />彻底删除
            </button>
            <button class="button button--danger" type="button" :disabled="trashBusy || !trashTotal" @click="runTrashAction('trash_empty')">
              清空回收站
            </button>
          </div>
        </header>
        <div v-if="trashLoading" class="file-loading"><RefreshCw :size="22" class="spinning" />正在读取回收站…</div>
        <div v-else-if="trashEntries.length" class="trash-list">
          <label v-for="entry in trashEntries" :key="entry.id" class="trash-item">
            <input type="checkbox" :checked="selectedTrash.has(entry.id)" @change="toggleTrash(entry.id)" />
            <span class="file-icon" :class="{ 'file-icon--folder': entry.kind === 'directory' }">
              <Folder v-if="entry.kind === 'directory'" :size="19" />
              <File v-else :size="19" />
            </span>
            <span>
              <strong>{{ entry.name }}</strong>
              <small>{{ entry.originalPath || '旧版回收站项目（仅支持彻底删除）' }}</small>
            </span>
            <span>{{ entry.kind === 'directory' ? '文件夹' : formatBytes(entry.sizeBytes) }}</span>
            <span>{{ formatTime(entry.deletedAt) }}</span>
          </label>
        </div>
        <div v-else class="file-empty">
          <Trash2 :size="34" />
          <strong>回收站是空的</strong>
          <span>移入回收站的文件会显示在这里。</span>
        </div>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(previewEntry)"
      :title="previewEntry?.name || '文件查看器'"
      :description="previewEntry ? `${previewEntry.path} · ${formatBytes(previewEntry.sizeBytes)}` : ''"
      size="wide"
      allow-fullscreen
      @close="closePreview"
    >
      <div v-if="previewLoading" class="preview-loading"><RefreshCw :size="22" class="spinning" />正在打开文件…</div>
      <div v-else-if="previewEntry && previewMode === 'text'" class="code-viewer">
        <header>
          <span><Code2 :size="15" />{{ previewEntry.mime || 'UTF-8 文本' }}</span>
          <span class="code-viewer__header-right">
            <span>{{ previewEntry.mode }} · {{ previewEntry.owner }}:{{ previewEntry.group }}</span>
            <span class="code-editor-tools">
              <button
                class="code-editor-tool"
                type="button"
                title="查找或替换（Ctrl+F）"
                aria-label="查找或替换"
                @click="codeEditorRef?.openSearch()"
              >
                <Search :size="15" />
              </button>
              <button
                class="code-editor-tool"
                :class="{ 'is-active': editorLineWrap }"
                type="button"
                title="切换自动换行"
                aria-label="切换自动换行"
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
            {{ editorStatus?.lines || 1 }} 行
            <template v-if="editorStatus"> · 行 {{ editorStatus.line }}，列 {{ editorStatus.column }}</template>
            · UTF-8
            <template v-if="editorInfo">
              · {{ editorInfo.label }}
              {{ editorInfo.highlighted ? '语法着色' : editorInfo.reason === 'large-file' ? '大文件纯文本' : '纯文本' }}
            </template>
          </span>
          <span v-if="previewDirty">有未保存修改</span>
          <span class="code-editor-actions">
            <button class="button button--primary button--small" type="button" :disabled="previewSaving || !previewDirty" @click="savePreview()">
              <Save :size="15" />{{ previewSaving ? '保存中…' : '保存 Ctrl+S' }}
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
            @loadedmetadata="handleMediaReady"
            @canplay="handleMediaCanPlay"
            @playing="handleMediaCanPlay"
            @waiting="handleMediaWaiting"
            @error="handleMediaError"
          >
            <source :src="previewURL" :type="previewEntry.mime || undefined" />
          </video>
          <div v-if="mediaLoading && !mediaError" class="media-player__loading" role="status" aria-live="polite">
            <RefreshCw :size="20" class="spinning" />
            <span>正在连接视频流…</span>
          </div>
          <div v-else-if="mediaError" class="media-player__error" role="alert">
            <strong>{{ mediaErrorMessage || '视频暂时无法播放' }}</strong>
            <span>请检查文件编码或服务器是否支持该格式。</span>
            <button class="button button--secondary button--small" type="button" @click.stop="retryMedia">重试播放</button>
          </div>
        </div>
        <img v-else-if="previewMode === 'image'" :src="previewURL" :alt="previewEntry.name" decoding="async" />
        <audio v-else-if="previewMode === 'audio'" :src="previewURL" controls preload="metadata" />
        <iframe v-else-if="previewMode === 'pdf'" :src="previewURL" :title="previewEntry.name" loading="lazy" />
        <div v-else class="metadata-viewer">
          <component :is="entryIcon(previewEntry)" :size="44" />
          <strong>此格式暂不在浏览器内解析</strong>
          <span>{{ previewEntry.mime || '未知格式' }} · {{ formatBytes(previewEntry.sizeBytes) }}</span>
          <button class="button button--primary" type="button" @click="download(previewEntry)">
            <Download :size="16" />下载文件
          </button>
        </div>
        <footer v-if="previewMode !== 'metadata'" class="media-viewer__footer">
          <span class="media-viewer__status" :class="{ 'is-loading': mediaLoading, 'is-error': mediaError }">
            <i aria-hidden="true" />{{ mediaStatusLabel }}
          </span>
          <button class="button button--secondary button--small" type="button" @click="download(previewEntry)">
            <Download :size="15" />下载原文件
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

.breadcrumbs {
  display: flex;
  min-width: 0;
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
  font-weight: 700;
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
  background: var(--surface-subtle);
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
.file-view-switch button:hover,
.file-view-switch button.is-active {
  color: var(--text);
  background: var(--surface-subtle);
}

.file-view-switch button.is-active {
  color: var(--brand);
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
  background: var(--surface);
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
  color: #fff;
  background: var(--brand);
}

.clipboard-bar button:hover:not(:disabled) {
  color: var(--text);
  background: var(--surface);
}

.clipboard-bar button:first-of-type:hover:not(:disabled) {
  color: #fff;
  background: var(--brand-strong);
}

.clipboard-bar button:disabled {
  opacity: .6;
  cursor: wait;
}

.file-transfer-status {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-height: 50px;
  padding: 8px 15px;
  border-bottom: 1px solid color-mix(in srgb, var(--brand) 24%, var(--border));
  background: color-mix(in srgb, var(--brand) 7%, var(--surface));
}

.file-transfer-status--partial,
.file-transfer-status--error,
.file-transfer-status--cancelled {
  border-bottom-color: color-mix(in srgb, var(--amber) 30%, var(--border));
  background: color-mix(in srgb, var(--amber) 7%, var(--surface));
}

.file-transfer-status__icon {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  border-radius: 10px;
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 14%, var(--surface));
}

.file-transfer-status--partial .file-transfer-status__icon,
.file-transfer-status--error .file-transfer-status__icon,
.file-transfer-status--cancelled .file-transfer-status__icon {
  color: var(--amber);
  background: color-mix(in srgb, var(--amber) 13%, var(--surface));
}

.file-transfer-status > span:nth-child(2) {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.file-transfer-status strong,
.file-transfer-status small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-transfer-status strong {
  color: var(--text);
  font-size: 12px;
}

.file-transfer-status small {
  color: var(--muted);
  font-size: 10px;
}

.file-transfer-status button {
  min-height: 30px;
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
}

.danger-link {
  color: var(--danger) !important;
}

.upload-strip {
  display: grid;
  gap: 7px;
  padding: 10px 15px;
  border-bottom: 1px solid var(--border);
}

.upload-strip > div {
  display: grid;
  grid-template-columns: minmax(120px, 220px) 1fr 42px;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}

.upload-strip > div > div {
  height: 4px;
  overflow: hidden;
  border-radius: 99px;
  background: var(--surface-subtle);
}

.upload-strip i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--brand);
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
  border-color: var(--border);
  background: var(--surface-subtle);
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
  font-weight: 700;
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
}

.file-row--entry:hover,
.file-row--selected {
  background: color-mix(in srgb, var(--brand) 6%, var(--surface));
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
  font-size: 11px;
}

.file-internal-drop-hint small {
  color: var(--muted);
  font-size: 9px;
}

.file-context-menu {
  position: fixed;
  z-index: 90;
  display: grid;
  width: 196px;
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
}

.file-context-menu button:hover {
  background: var(--surface-subtle);
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
  font-weight: 700;
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
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--brand) 13%, transparent);
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
  background: var(--surface-subtle);
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
  border: 1px solid var(--terminal-shell-border, #29383a);
  border-radius: var(--terminal-shell-radius, 12px);
  background: var(--terminal-shell-background, #0b1214);
  box-shadow: var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%));
}

.code-viewer > header,
.code-viewer > footer {
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 7px 12px;
  color: var(--terminal-shell-muted, #8a9695);
  font-size: 12px;
  background: var(--terminal-shell-panel, #111a1d);
}

.code-viewer > header > span:first-child {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--terminal-shell-text, #d8dddc);
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
  color: var(--terminal-shell-muted, #8a9695);
  background: transparent;
  cursor: pointer;
}

.code-editor-tool:hover,
.code-editor-tool.is-active {
  color: var(--terminal-shell-text, #d8dddc);
  background: var(--terminal-shell-panel-raised, #182326);
}

.code-editor-tool.is-active {
  color: var(--brand, #35cba6);
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
  border: 1px solid var(--terminal-shell-border, #29383a);
  border-radius: 16px;
  background:
    radial-gradient(circle at 50% -12%, rgb(53 203 166 / 15%), transparent 42%),
    linear-gradient(180deg, #111c1d 0%, var(--terminal-shell-background, #0b1214) 100%);
  box-shadow: var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%));
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
  border: 1px solid rgb(255 255 255 / 10%);
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
  color: #eef8f5;
  background: linear-gradient(180deg, rgb(2 10 9 / 8%), rgb(2 10 9 / 66%));
  text-align: center;
}

.media-player__loading {
  pointer-events: none;
}

.media-player__error {
  flex-direction: column;
  gap: 6px;
  color: #ffe7e7;
  pointer-events: auto;
}

.media-player__error span {
  color: rgb(255 231 231 / 72%);
  font-size: 12px;
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
  background: #fff;
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
  color: var(--terminal-shell-muted, #8a9695);
  font-size: 12px;
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
  background: var(--brand, #35cba6);
  box-shadow: 0 0 0 4px rgb(53 203 166 / 13%);
}

.media-viewer__status.is-loading i {
  background: var(--amber, #d5ae62);
  box-shadow: 0 0 0 4px rgb(213 174 98 / 13%);
}

.media-viewer__status.is-error i {
  background: var(--danger, #d86f74);
  box-shadow: 0 0 0 4px rgb(216 111 116 / 13%);
}

:global(.modal-panel--wide:has(.media-viewer)) {
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
  color: var(--muted);
  text-align: center;
}

.metadata-viewer strong {
  color: var(--text);
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

@media (max-width: 920px) {
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

  .file-toolbar {
    align-items: stretch;
    flex-direction: column;
    gap: 9px;
    padding: 10px;
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
    right: 10px;
    bottom: max(10px, env(safe-area-inset-bottom));
    left: 10px !important;
    width: auto;
    max-height: min(66dvh, 520px);
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
    right: 8px;
    bottom: max(8px, env(safe-area-inset-bottom));
    left: 8px !important;
  }
}
</style>
