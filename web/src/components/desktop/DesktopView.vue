<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import type { Component } from 'vue'
import {
  ArrowLeft,
  CircleArrowUp,
  RefreshCw,
  Sun,
  Moon,
  Image as ImageIcon,
  Info,
  Pencil,
  SquareTerminal,
  AppWindow,
  ExternalLink,
  Globe2,
  ListTree,
  MonitorCog,
  Plus,
  Trash2,
  EyeOff,
  File,
  FolderOpen,
  HardDriveUpload,
  LoaderCircle,
  Check,
  Maximize2,
  Minimize2,
  X,
} from '@lucide/vue'
import DesktopWindow from '@/components/desktop/DesktopWindow.vue'
import DesktopEntryIcon from '@/components/desktop/DesktopEntryIcon.vue'
import DesktopClock from '@/components/desktop/DesktopClock.vue'
import DesktopMonitor from '@/components/desktop/DesktopMonitor.vue'
import DesktopIconManagerDialog from '@/components/desktop/DesktopIconManagerDialog.vue'
import DesktopShortcutDialog, {
  type DesktopShortcutDraft,
} from '@/components/desktop/DesktopShortcutDialog.vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import LogoMark from '@/components/common/LogoMark.vue'
import { DEFAULT_WINDOW_GRADIENT, desktopApps, desktopRoutePath, findDesktopApp } from '@/lib/desktopApps'
import {
  getCachedDesktopEntries,
  loadDesktopEntries,
  type DesktopEntries,
  type DesktopEntry,
} from '@/lib/desktopEntries'
import { api, ApiError, type SystemResourceSnapshot } from '@/lib/api'
import { transferCrossPanelFileBatch } from '@/lib/crossPanelFileTransfer'
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
  peekDesktopFileDragOrigin,
} from '@/lib/desktopFileShortcuts'
import {
  collectExternalDrop,
  DESKTOP_UPLOAD_DIRECTORY,
  DesktopExternalDropError,
  hasExternalFileDrop,
  uploadExternalDrop,
  type DesktopExternalTransferProgress,
} from '@/lib/desktopExternalDrop'
import { shortcutFileGradient, shortcutFileIcon } from '@/lib/fileEntryPresentation'
import {
  autoArrangeDesktopIcons,
  deriveDesktopIconLayout,
  desktopIconGrid,
  desktopIconGridSlotForPosition,
  desktopIconPositionForGridSlot,
  desktopIconPixelsToPosition,
  desktopIconPositionToPixels,
  dropDesktopIcon,
  MAX_DESKTOP_ICON_POSITIONS,
  moveDesktopIconGroup,
  moveDesktopIconByKeyboard,
  type DesktopIconBounds,
  type DesktopIconPlacement,
} from '@/lib/desktopIconLayout'
import { prefetchNavigationRoute } from '@/lib/navigation'
import {
  desktopCloseGuardCoordinator,
  desktopCloseGuardCoordinatorKey,
} from '@/lib/desktopRouteKeys'
import { useDesktopMode } from '@/stores/desktopMode'
import { useDesktopIcons } from '@/stores/desktopIcons'
import { useDocumentFullscreen } from '@/composables/useDocumentFullscreen'
import { useTheme } from '@/stores/theme'
import { useToast } from '@/stores/toast'
import { useI18n } from '@/i18n'
import type { AgentStatus, DesktopIconPosition, DesktopShortcut, FileEntry } from '@/types/api'

/**
 * Desktop overlay with Windows-style selection/open behavior, desktop-side
 * clock and resource widgets, and a persistent bottom taskbar.
 */

const props = defineProps<{
  agent?: AgentStatus
  kpanelUpdateAvailable?: boolean
  kpanelUpdateDescription?: string
}>()

const desktop = useDesktopMode()
const desktopIcons = useDesktopIcons()
const documentFullscreen = useDocumentFullscreen()
const theme = useTheme()
const toast = useToast()
const i18n = useI18n()
provide(desktopCloseGuardCoordinatorKey, desktopCloseGuardCoordinator)

const openWindows = computed(() => desktop.windows.value)
const focusedWindow = computed(() =>
  desktop.windows.value.find((windowState) => windowState.id === desktop.focusedId.value),
)
const agentStatus = computed(() => {
  const agent = props.agent
  if (!agent?.connected) return { state: 'offline', label: i18n.t('agent.offline') }
  if (!agent.compatible) return { state: 'incompatible', label: i18n.t('agent.incompatible') }
  if (agent.readOnly) return { state: 'read-only', label: i18n.t('agent.readOnly') }
  return { state: 'online', label: i18n.t('agent.online') }
})

// Dynamic entries: installed apps and configured sites surfaced as desktop
// icons that open their external URL.
const SITE_RENAMES_KEY = 'kpanel:desktop-site-names:v1'
const DESKTOP_UPLOAD_LOCATION_KEY = 'kpanel:desktop-upload-location:v1'
const DESKTOP_WALLPAPER_KEY = 'kpanel:desktop-wallpaper:v1'
const MAX_SITE_NAME_LENGTH = 48

const DESKTOP_WALLPAPERS = [
  {
    id: 'classic',
    src: '/wallpapers/kpanel-desktop.webp',
    nameKey: 'desktop.wallpaperClassic',
    descriptionKey: 'desktop.wallpaperClassicDescription',
  },
  {
    id: 'orbit',
    src: '/wallpapers/kpanel-desktop-orbit.webp',
    nameKey: 'desktop.wallpaperOrbit',
    descriptionKey: 'desktop.wallpaperOrbitDescription',
  },
  {
    id: 'horizon',
    src: '/wallpapers/kpanel-desktop-horizon.webp',
    nameKey: 'desktop.wallpaperHorizon',
    descriptionKey: 'desktop.wallpaperHorizonDescription',
  },
  {
    id: 'rift',
    src: '/wallpapers/kpanel-desktop-rift.webp',
    nameKey: 'desktop.wallpaperRift',
    descriptionKey: 'desktop.wallpaperRiftDescription',
  },
  {
    id: 'prism',
    src: '/wallpapers/kpanel-desktop-prism.webp',
    nameKey: 'desktop.wallpaperPrism',
    descriptionKey: 'desktop.wallpaperPrismDescription',
  },
] as const
type DesktopWallpaperID = typeof DESKTOP_WALLPAPERS[number]['id']

function isDesktopWallpaperID(value: string | null): value is DesktopWallpaperID {
  return DESKTOP_WALLPAPERS.some((wallpaper) => wallpaper.id === value)
}

function readDesktopWallpaperID(): DesktopWallpaperID {
  try {
    const stored = window.localStorage.getItem(DESKTOP_WALLPAPER_KEY)
    return isDesktopWallpaperID(stored) ? stored : 'classic'
  } catch {
    return 'classic'
  }
}

function normalizedHostDirectory(value: string): string | undefined {
  const candidate = value.trim()
  if (
    !candidate.startsWith('/')
    || candidate.length > 4096
    || /[\u0000-\u001f\\]/.test(candidate)
    || (candidate !== '/' && candidate.slice(1).split('/').some((part) => !part || part === '.' || part === '..'))
  ) return undefined
  return candidate
}

function readDesktopUploadDirectory(): string {
  try {
    return normalizedHostDirectory(window.localStorage.getItem(DESKTOP_UPLOAD_LOCATION_KEY) || '')
      || DESKTOP_UPLOAD_DIRECTORY
  } catch {
    return DESKTOP_UPLOAD_DIRECTORY
  }
}

function readSiteNames(): Record<string, string> {
  try {
    const raw = window.localStorage.getItem(SITE_RENAMES_KEY)
    if (!raw || raw.length > 16_000) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return Object.fromEntries(
      Object.entries(parsed)
        .filter(([id, name]) => id.length <= 128 && typeof name === 'string')
        .map(([id, name]) => [id, String(name).trim().slice(0, MAX_SITE_NAME_LENGTH)])
        .filter(([, name]) => Boolean(name)),
    )
  } catch {
    return {}
  }
}

const siteNames = ref<Record<string, string>>(readSiteNames())
const siteAppearanceNames = ref<Record<string, string>>({})

function siteDomainName(entry: DesktopEntry): string {
  return entry.site?.primaryDomain || entry.name
}

function defaultSiteName(entry: DesktopEntry): string {
  return siteAppearanceNames.value[entry.id] || siteDomainName(entry)
}

function applySiteNames(value?: DesktopEntries): DesktopEntries | undefined {
  if (!value) return undefined
  const apply = (entry: DesktopEntry): DesktopEntry => {
    if (entry.kind !== 'site') return entry
    return { ...entry, name: siteNames.value[entry.id] || defaultSiteName(entry) }
  }
  return {
    ...value,
    sites: value.sites.map(apply),
    visible: value.visible.map(apply),
  }
}

const entries = ref<DesktopEntries | undefined>(applySiteNames(getCachedDesktopEntries()))
const systemResources = ref<SystemResourceSnapshot>()
const entriesLoading = ref(!entries.value)
let entriesAbort: AbortController | undefined
let entriesSequence = 0

const workspace = computed(() => desktopIcons.workspace.value)
const hiddenEntryKeys = computed(() => new Set(workspace.value.hiddenEntryKeys))
const visibleDynamicEntries = computed(() =>
  (entries.value?.visible || []).filter((entry) => !hiddenEntryKeys.value.has(entry.key)),
)
const hiddenEntries = computed(() =>
  (entries.value?.visible || []).filter((entry) => hiddenEntryKeys.value.has(entry.key)),
)
const shortcuts = computed<DesktopShortcut[]>(() => workspace.value.shortcuts.map((shortcut) => ({
  ...shortcut,
  iconURL: shortcut.iconURL
    || (shortcut.iconVersion ? api.desktop.shortcutIconURL(shortcut.id, shortcut.iconVersion) : undefined),
})))
const shortcutEntries = computed<DesktopEntry[]>(() => shortcuts.value.map((shortcut) => ({
  key: `shortcut:${shortcut.id}`,
  kind: 'shortcut',
  id: shortcut.id,
  name: shortcut.name,
  description: shortcut.description || shortcut.path,
  launch: shortcut.targetType === 'url' ? 'external' : shortcut.targetType,
  url: shortcut.url,
  path: shortcut.path,
  iconURL: shortcut.iconURL,
  icon: shortcut.targetType === 'file' || shortcut.targetType === 'directory'
    ? shortcutFileIcon(shortcut.name, shortcut.targetType)
    : undefined,
  shortcut,
})))

function isTransferableDesktopShortcut(entry: DesktopEntry): boolean {
  return entry.kind === 'shortcut'
    && (entry.launch === 'file' || entry.launch === 'directory')
    && Boolean(entry.path)
    && entry.path !== '/'
}

const desktopShortcutPaths = computed(() => [...new Set(
  shortcutEntries.value
    .filter(isTransferableDesktopShortcut)
    .map((entry) => entry.path!),
)])
const desktopShortcutPathSignature = computed(() => desktopShortcutPaths.value.join('\0'))

const iconsElement = ref<HTMLElement>()
const desktopElement = ref<HTMLElement>()
const iconBounds = ref<DesktopIconBounds>({ width: 90, height: 96 })
const compactIconLayout = ref(window.innerWidth <= 760)
const localPositions = ref<Record<string, DesktopIconPosition>>({})
const dragPreviews = ref<Record<string, { left: number; top: number }>>({})
const draggingIcons = ref<Set<string>>(new Set())
const nativeDragHiddenIcons = ref<Set<string>>(new Set())
const iconAnnouncement = ref('')
const iconManagerOpen = ref(false)
const wallpaperDialogOpen = ref(false)
const desktopWallpaperID = ref<DesktopWallpaperID>(readDesktopWallpaperID())
const activeDesktopWallpaper = computed(() =>
  DESKTOP_WALLPAPERS.find((wallpaper) => wallpaper.id === desktopWallpaperID.value)
    || DESKTOP_WALLPAPERS[0],
)
const desktopWallpaperStyle = computed(() => ({
  '--desktop-wallpaper-image': `url("${activeDesktopWallpaper.value.src}")`,
}))
const shortcutDialogOpen = ref(false)
const editingShortcut = ref<DesktopShortcut>()
const deletingShortcut = ref<DesktopShortcut>()
const removingEntry = ref<DesktopEntry>()
const batchRemovingEntries = ref<DesktopEntry[]>([])
const shortcutSaving = ref(false)
const shortcutError = ref('')
const localClusterNodeId = ref('')
const desktopFileMetadata = ref<Record<string, FileEntry>>({})
const fileDropActive = ref(false)
const fileDropMode = ref<'shortcut' | 'upload' | 'panel-copy'>('shortcut')
type DesktopTransferPhase = 'preparing' | 'uploading' | 'complete' | 'partial' | 'error' | 'cancelled'
interface DesktopTransferState extends DesktopExternalTransferProgress {
  phase: DesktopTransferPhase
  operation?: 'upload' | 'panel-copy'
  roots: number
  failed: number
  detail: string
}
const desktopTransfer = ref<DesktopTransferState>()
const dropPulse = ref<{ id: number; left: number; top: number }>()
const desktopUploadDirectory = ref(readDesktopUploadDirectory())
const uploadLocationOpen = ref(false)
const uploadLocationDraft = ref('')
const uploadLocationSaving = ref(false)
const uploadLocationError = ref('')
const desktopTransferPercent = computed(() => {
  const transfer = desktopTransfer.value
  if (!transfer) return 0
  if (transfer.phase === 'complete') return 100
  if (transfer.operation === 'panel-copy' && transfer.totalFiles > 1) {
    const current = transfer.totalBytes > 0
      ? Math.min(1, transfer.loadedBytes / transfer.totalBytes)
      : transfer.phase === 'preparing' ? 0.1 : 0.5
    return Math.min(100, Math.round((transfer.completedFiles + current) / transfer.totalFiles * 100))
  }
  if (transfer.totalBytes > 0) return Math.min(100, Math.round(transfer.loadedBytes / transfer.totalBytes * 100))
  if (transfer.operation === 'panel-copy') return transfer.phase === 'preparing' ? 12 : 55
  if (transfer.totalFiles > 0) return Math.min(100, Math.round(transfer.completedFiles / transfer.totalFiles * 100))
  return transfer.phase === 'preparing' ? 8 : 0
})
const desktopTransferActive = computed(() => Boolean(
  desktopTransfer.value && ['preparing', 'uploading'].includes(desktopTransfer.value.phase),
))
const deletingFileShortcut = computed(() => deletingShortcut.value?.targetType === 'file' || deletingShortcut.value?.targetType === 'directory')
let pendingShortcutID = ''
let workspaceAbort: AbortController | undefined
let desktopFileMetadataAbort: AbortController | undefined
let desktopFileMetadataSequence = 0
let iconsResizeObserver: ResizeObserver | undefined
let desktopTransferController: AbortController | undefined
let desktopTransferClearTimer: number | undefined
let dropPulseTimer: number | undefined
let themeTransitionTimer: number | undefined
let themeTogglePendingAfterContextMenu = false
let pendingPositionWrites = 0
let latestPositionWrite = 0

const allIconKeys = computed(() => [
  ...desktopApps.map((app) => `nav:${app.path}`),
  ...visibleDynamicEntries.value.map((entry) => entry.key),
  ...shortcutEntries.value.map((entry) => entry.key),
])

const savedPlacements = computed<DesktopIconPlacement[]>(() =>
  Object.entries(localPositions.value).map(([key, position]) => ({ key, position })),
)

const renderedIconLayout = computed(() => deriveDesktopIconLayout(
  allIconKeys.value,
  savedPlacements.value,
  iconBounds.value,
  compactIconLayout.value,
))

const renderedPositionByKey = computed(() => new Map(
  renderedIconLayout.value.placements.map((placement) => [placement.key, placement.position]),
))
const renderedOverflowIndexByKey = computed(() => new Map(
  renderedIconLayout.value.overflowKeys.map((key, index) => [key, index]),
))
const iconOverflowStartTop = computed(() => (
  renderedIconLayout.value.contentHeight
  + (renderedIconLayout.value.overflowKeys.length ? 44 : 0)
))
const iconScrollHeight = computed(() => {
  const layout = renderedIconLayout.value
  const overflowCount = layout.overflowKeys.length
  if (!overflowCount) return Math.ceil(layout.contentHeight)
  const overflowRows = Math.ceil(overflowCount / layout.grid.columns)
  return Math.ceil(
    iconOverflowStartTop.value
    + Math.max(0, overflowRows - 1) * layout.grid.stepY
    + layout.grid.metrics.height,
  )
})

watch(() => workspace.value.positions, (positions) => {
  if (draggingIcons.value.size || pendingPositionWrites > 0) return
  localPositions.value = Object.fromEntries(
    Object.entries(positions).map(([key, position]) => [key, { ...position }]),
  )
}, { deep: true, immediate: true })

watch([() => workspace.value.labels, () => desktopIcons.loaded.value], ([labels, isLoaded]) => {
  if (!isLoaded) return
  siteNames.value = Object.fromEntries(
    Object.entries(labels)
      .filter(([key]) => key.startsWith('site:'))
      .map(([key, name]) => [key.slice('site:'.length), name]),
  )
  entries.value = applySiteNames(entries.value)
}, { deep: true })

// Context menu: `targetEntry` set when the menu is for an entry icon; cleared
// for the empty-desktop menu.
const contextMenu = ref<{ x: number; y: number; open: boolean }>({ x: 0, y: 0, open: false })
const contextMenuTarget = ref<'desktop' | 'taskbar' | 'taskbar-window'>('desktop')
const contextMenuElement = ref<HTMLElement>()
const menuEntry = ref<DesktopEntry>()
const menuNavPath = ref('')
const menuSelectionKeys = ref<string[]>([])
const menuRemovableCount = computed(() => {
  const selected = new Set(menuSelectionKeys.value)
  return [...visibleDynamicEntries.value, ...shortcutEntries.value]
    .filter((entry) => selected.has(entry.key)).length
})
const menuWindowId = ref<number>()
const detailEntry = ref<DesktopEntry>()
const externalOpenEntry = ref<DesktopEntry>()
const externalOpenImageFailed = ref(false)
const renameEntry = ref<DesktopEntry>()
const renameValue = ref('')
let contextMenuOpener: HTMLElement | undefined

interface DesktopWindowHandle {
  requestClose: () => Promise<void>
}

const desktopWindowRefs = new Map<number, DesktopWindowHandle>()

function setDesktopWindowRef(windowId: number, instance: unknown): void {
  if (!instance) {
    desktopWindowRefs.delete(windowId)
    return
  }
  const handle = instance as Partial<DesktopWindowHandle>
  if (typeof handle.requestClose === 'function') {
    desktopWindowRefs.set(windowId, handle as DesktopWindowHandle)
  }
}

/** Icons currently playing their open-bounce animation. */
const bouncingIcon = ref<string>('')
const selectedIcons = ref<Set<string>>(new Set())
const selectedIconCount = computed(() => selectedIcons.value.size)
const selectedEntries = computed(() => {
  const byKey = new Map(
    [...visibleDynamicEntries.value, ...shortcutEntries.value].map((entry) => [entry.key, entry]),
  )
  return [...selectedIcons.value]
    .map((key) => byKey.get(key))
    .filter((entry): entry is DesktopEntry => Boolean(entry))
})

async function refreshDesktopFileMetadata(paths: readonly string[], replaceAll = false): Promise<void> {
  const requested = [...new Set(paths)].filter((path) => path && path !== '/').slice(0, 64)
  desktopFileMetadataAbort?.abort()
  desktopFileMetadataAbort = undefined
  const sequence = ++desktopFileMetadataSequence
  if (!requested.length) {
    if (replaceAll) desktopFileMetadata.value = {}
    return
  }
  const controller = new AbortController()
  desktopFileMetadataAbort = controller
  try {
    const result = await api.files.entries(requested, controller.signal)
    if (controller.signal.aborted || sequence !== desktopFileMetadataSequence) return
    const next = replaceAll ? {} : { ...desktopFileMetadata.value }
    for (const path of requested) delete next[path]
    for (const entry of result.entries) {
      if ((entry.kind === 'file' || entry.kind === 'directory') && requested.includes(entry.path)) {
        next[entry.path] = entry
      }
    }
    desktopFileMetadata.value = next
  } catch (error) {
    if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) return
    if (replaceAll && sequence === desktopFileMetadataSequence) desktopFileMetadata.value = {}
  } finally {
    if (desktopFileMetadataAbort === controller) desktopFileMetadataAbort = undefined
  }
}

watch(desktopShortcutPathSignature, () => {
  void refreshDesktopFileMetadata(desktopShortcutPaths.value, true)
}, { immediate: true })

const removableSelectedCount = computed(() => selectedEntries.value.length)
watch(allIconKeys, (keys) => {
  const visible = new Set(keys)
  const next = [...selectedIcons.value].filter((key) => visible.has(key))
  if (next.length !== selectedIcons.value.size) setIconSelection(next)
})
let bounceTimer: number | undefined
let resizeFrame: number | undefined
let resizePersistTimer: number | undefined

function motionDuration(duration: number): number {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 0 : duration
}

function gradientFor(path: string): string {
  const gradient = findDesktopApp(path)?.gradient ?? DEFAULT_WINDOW_GRADIENT
  return `linear-gradient(145deg, ${gradient[0]} 0%, ${gradient[1]} 100%)`
}

const SITE_GRADIENTS: Array<[string, string]> = [
  ['#22d3ee', '#0e7490'],
  ['#34d399', '#047857'],
  ['#60a5fa', '#1d4ed8'],
  ['#a78bfa', '#6d28d9'],
  ['#f472b6', '#be185d'],
  ['#fb923c', '#c2410c'],
  ['#facc15', '#a16207'],
  ['#2dd4bf', '#0f766e'],
]

function stableSiteColorIndex(entry: DesktopEntry): number {
  const key = entry.site?.primaryDomain || entry.url || entry.id
  let hash = 2_166_136_261
  for (let index = 0; index < key.length; index += 1) {
    hash ^= key.charCodeAt(index)
    hash = Math.imul(hash, 16_777_619)
  }
  return (hash >>> 0) % SITE_GRADIENTS.length
}

function entryGradient(entry: DesktopEntry): string {
  if (entry.kind === 'site') {
    const [start, end] = SITE_GRADIENTS[stableSiteColorIndex(entry)] ?? ['#22d3ee', '#0e7490']
    return `linear-gradient(145deg, ${start} 0%, ${end} 100%)`
  }
  if (entry.kind === 'shortcut') {
    if (entry.launch === 'directory' || entry.launch === 'file') {
      return shortcutFileGradient(entry.name, entry.launch)
    }
    return 'linear-gradient(145deg, #38bdf8 0%, #0369a1 100%)'
  }
  // App-market apps keep a neutral brand tile; the market icon image sits on it.
  return `linear-gradient(145deg, #5b7a72 0%, #243b36 100%)`
}

function openApp(path: string): void {
  const app = findDesktopApp(path)
  if (!app) return
  // Open immediately so the interface feels responsive, while the icon keeps
  // a short launch animation as visual acknowledgement.
  if (bounceTimer !== undefined) window.clearTimeout(bounceTimer)
  bouncingIcon.value = path
  const existingFileWindow = app.path === '/files'
    ? openWindows.value.find((windowState) => fileWindowDirectory(windowState.path) === '/')
    : undefined
  let windowId: number
  if (existingFileWindow) {
    desktop.restoreWindow(existingFileWindow.id)
    windowId = existingFileWindow.id
  } else {
    windowId = desktop.openWindow(app.path, app.labelKey, app.path === '/files' ? true : app.allowMultiple)
  }
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), {
      message: i18n.t('desktop.windowLimitMessage'),
    })
  }
  bounceTimer = window.setTimeout(() => {
    bouncingIcon.value = ''
    bounceTimer = undefined
  }, motionDuration(460))
}

function openKPanelUpdate(): void {
  const app = findDesktopApp('/apps')
  if (!app) return
  const windowId = desktop.openWindow(
    '/apps?app=kpanel&action=update',
    app.labelKey,
    app.allowMultiple,
    true,
  )
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), {
      message: i18n.t('desktop.windowLimitMessage'),
    })
  }
}

function openEntry(entry: DesktopEntry): void {
  if (entry.kind === 'app' && entry.launch === 'script') {
    openAppScriptEntry(entry)
    return
  }
  if ((entry.launch === 'file' || entry.launch === 'directory') && entry.path) {
    openFileShortcut(entry)
    return
  }
  if (entry.url) {
    requestExternalOpen(entry)
    return
  }
}

function parentFilePath(filePath: string): string {
  const separator = filePath.lastIndexOf('/')
  return separator <= 0 ? '/' : filePath.slice(0, separator)
}

function fileShortcutRoute(entry: DesktopEntry): string | undefined {
  if (!entry.path || (entry.launch !== 'file' && entry.launch !== 'directory')) return undefined
  const query = new URLSearchParams({
    path: entry.launch === 'directory' ? entry.path : parentFilePath(entry.path),
  })
  if (entry.launch === 'file') query.set('file', entry.path)
  return `/files?${query.toString()}`
}

function fileWindowDirectory(fullPath: string): string | undefined {
  if (desktopRoutePath(fullPath) !== '/files') return undefined
  const queryIndex = fullPath.indexOf('?')
  if (queryIndex === -1) return '/'
  const hashIndex = fullPath.indexOf('#', queryIndex)
  const query = new URLSearchParams(fullPath.slice(queryIndex + 1, hashIndex === -1 ? undefined : hashIndex))
  const pathValues = query.getAll('path')
  if (pathValues.length > 1) return undefined
  const path = pathValues[0] || '/'
  if (
    !path.startsWith('/')
    || path.length > 4096
    || /[\u0000-\u001f\\]/.test(path)
    || (path !== '/' && path.slice(1).split('/').some((part) => !part || part === '.' || part === '..'))
  ) return undefined
  return path
}

function openFileShortcut(entry: DesktopEntry): void {
  const route = fileShortcutRoute(entry)
  if (!route) return
  const app = findDesktopApp('/files')
  if (!app) return
  const directory = fileWindowDirectory(route)
  const existing = openWindows.value.find(
    (windowState) => directory !== undefined && fileWindowDirectory(windowState.path) === directory,
  )
  if (existing) {
    if (entry.launch === 'file' && existing.path !== route) {
      desktop.updateWindowRoute(existing.id, route, app.labelKey)
    }
    desktop.restoreWindow(existing.id)
    return
  }
  const windowId = desktop.openWindow(route, app.labelKey, true)
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), { message: i18n.t('desktop.windowLimitMessage') })
  }
}

function requestExternalOpen(entry: DesktopEntry): void {
  if (!entry.url) return
  externalOpenImageFailed.value = false
  externalOpenEntry.value = entry
}

function closeExternalOpen(): void {
  externalOpenEntry.value = undefined
}

function confirmExternalOpen(): void {
  const entry = externalOpenEntry.value
  if (!entry?.url) return
  window.open(entry.url, '_blank', 'noopener,noreferrer')
  closeExternalOpen()
}

const externalOpenMonogram = computed(() =>
  externalOpenEntry.value?.name.trim().slice(0, 1).toLocaleUpperCase() || 'K',
)

function openAppScriptEntry(entry: DesktopEntry): void {
  const path = `/app-script/${encodeURIComponent(entry.id)}`
  const app = findDesktopApp(path)
  if (!app) return
  const windowId = desktop.openWindow(path, app.labelKey, app.allowMultiple, true)
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), {
      message: i18n.t('desktop.windowLimitMessage'),
    })
  }
}

function openAppMarketEntry(entry: DesktopEntry): void {
  const app = findDesktopApp('/apps')
  if (!app) return
  const query = new URLSearchParams({ app: entry.id })
  const windowId = desktop.openWindow(
    `/apps?${query.toString()}`,
    app.labelKey,
    app.allowMultiple,
    true,
  )
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), {
      message: i18n.t('desktop.windowLimitMessage'),
    })
  }
}

function openNavIcon(path: string): void {
  openApp(path)
}

function setIconSelection(keys: Iterable<string>): void {
  selectedIcons.value = new Set(keys)
}

function clearIconSelection(): void {
  setIconSelection([])
}

function selectIcon(key: string, event?: MouseEvent): void {
  if (!event || (!event.ctrlKey && !event.metaKey && !event.shiftKey)) {
    setIconSelection([key])
    return
  }
  const next = new Set(selectedIcons.value)
  if ((event.ctrlKey || event.metaKey) && next.has(key)) next.delete(key)
  else next.add(key)
  setIconSelection(next)
}

function selectNavIcon(path: string, event?: MouseEvent): void {
  selectIcon(`nav:${path}`, event)
}

function selectEntry(entry: DesktopEntry, event?: MouseEvent): void {
  selectIcon(entry.key, event)
}

function warmNavIcon(path: string): void {
  void prefetchNavigationRoute(path)
}

function windowIcon(path: string): Component {
  return findDesktopApp(path)?.icon ?? AppWindow
}

function scriptWindowEntry(path: string): DesktopEntry | undefined {
  const match = path.match(/^\/app-script\/([A-Za-z0-9_-]{1,128})(?:[?#]|$)/)
  if (!match) return undefined
  const appID = match[1]
  return entries.value?.apps.find((entry) => entry.id === appID)
    ?? entries.value?.visible.find((entry) => entry.kind === 'app' && entry.id === appID)
}

function windowIconURL(path: string): string | undefined {
  return scriptWindowEntry(path)?.iconURL
}

function windowTitle(titleKey: string, path?: string): string {
  const scriptEntry = path ? scriptWindowEntry(path) : undefined
  if (scriptEntry) return i18n.t('desktop.namedScriptWindowTitle', { name: scriptEntry.name })
  if (path?.startsWith('/files?')) {
    const query = new URLSearchParams(path.slice(path.indexOf('?') + 1))
    const target = query.get('file') || query.get('path')
    const name = target === '/'
      ? i18n.t('desktop.fileRootName')
      : target?.slice(target.lastIndexOf('/') + 1)
    if (name) return i18n.t('desktop.namedFileWindowTitle', { name })
  }
  return i18n.t(titleKey as Parameters<typeof i18n.t>[0])
}

async function showContextMenu(
  event: MouseEvent,
  entry?: DesktopEntry,
  target: 'desktop' | 'taskbar' | 'taskbar-window' = 'desktop',
  windowId?: number,
  navPath = '',
  selectionKeys: readonly string[] = [],
): Promise<void> {
  event.preventDefault()
  contextMenuOpener = event.currentTarget instanceof HTMLElement
    ? event.currentTarget
    : document.activeElement instanceof HTMLElement
      ? document.activeElement
      : undefined
  contextMenu.value = { x: event.clientX, y: event.clientY, open: true }
  contextMenuTarget.value = target
  menuEntry.value = entry
  menuNavPath.value = navPath
  menuSelectionKeys.value = [...selectionKeys]
  menuWindowId.value = windowId
  await nextTick()

  const menu = contextMenuElement.value
  if (!menu) return
  const rect = menu.getBoundingClientRect()
  const margin = 10
  contextMenu.value = {
    open: true,
    x: Math.min(Math.max(margin, event.clientX), Math.max(margin, window.innerWidth - rect.width - margin)),
    y: Math.min(Math.max(margin, event.clientY), Math.max(margin, window.innerHeight - rect.height - margin)),
  }
  menu.querySelector<HTMLButtonElement>('button')?.focus({ preventScroll: true })
}

function onContextMenu(event: MouseEvent): void {
  void showContextMenu(event)
}

function onEntryContext(event: MouseEvent, entry: DesktopEntry): void {
  if (!selectedIcons.value.has(entry.key)) setIconSelection([entry.key])
  const selection = selectedIcons.value.size > 1 ? [...selectedIcons.value] : []
  void showContextMenu(event, entry, 'desktop', undefined, '', selection)
}

function onNavContext(event: MouseEvent, path: string): void {
  const key = `nav:${path}`
  if (!selectedIcons.value.has(key)) setIconSelection([key])
  const selection = selectedIcons.value.size > 1 ? [...selectedIcons.value] : []
  void showContextMenu(event, undefined, 'desktop', undefined, path, selection)
}

function onTaskbarContext(event: MouseEvent): void {
  void showContextMenu(event, undefined, 'taskbar')
}

function onTaskbarItemContext(event: MouseEvent, windowId: number): void {
  void showContextMenu(event, undefined, 'taskbar-window', windowId)
}

function onEntryOpen(_event: MouseEvent | KeyboardEvent, entry: DesktopEntry): void {
  openEntry(entry)
}

function closeContextMenu(restoreFocus = true): void {
  contextMenu.value.open = false
  menuEntry.value = undefined
  menuNavPath.value = ''
  menuSelectionKeys.value = []
  menuWindowId.value = undefined
  const opener = contextMenuOpener
  contextMenuOpener = undefined
  if (restoreFocus && opener?.isConnected) {
    void nextTick(() => opener.focus({ preventScroll: true }))
  }
}

function measureIconWorkArea(): void {
  const rect = iconsElement.value?.getBoundingClientRect()
  const fallbackRight = window.innerWidth > 900 ? 342 : 16
  iconBounds.value = {
    width: Math.max(90, rect?.width || window.innerWidth - fallbackRight),
    height: Math.max(96, rect?.height || window.innerHeight - 88),
  }
  const compact = window.innerWidth <= 760
  if (compact && !compactIconLayout.value) {
    if (iconDrag) cancelIconDrag()
    if (selectionFrame) cancelSelectionFrame()
  }
  compactIconLayout.value = compact
}

function iconSlotStyle(key: string): Record<string, string> {
  const preview = dragPreviews.value[key]
  if (preview) {
    return { left: `${preview.left}px`, top: `${preview.top}px` }
  }
  const position = renderedPositionByKey.value.get(key)
  if (position) {
    const pixels = desktopIconPositionToPixels(position, iconBounds.value)
    return { left: `${pixels.left}px`, top: `${pixels.top}px` }
  }
  const overflowIndex = renderedOverflowIndexByKey.value.get(key)
  if (overflowIndex === undefined) return { display: 'none' }
  const grid = renderedIconLayout.value.grid
  return {
    left: `${(overflowIndex % grid.columns) * grid.stepX}px`,
    top: `${iconOverflowStartTop.value + Math.floor(overflowIndex / grid.columns) * grid.stepY}px`,
  }
}

function workspaceErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 409) return i18n.t('desktop.workspaceConflict')
  if (error instanceof ApiError) return error.message
  return i18n.t('desktop.workspaceSaveFailed')
}

async function persistPositions(next: Record<string, DesktopIconPosition>): Promise<void> {
  if (Object.keys(next).length > MAX_DESKTOP_ICON_POSITIONS) {
    const message = i18n.t('desktop.iconLayoutLimitMessage', { count: MAX_DESKTOP_ICON_POSITIONS })
    toast.danger(i18n.t('desktop.iconLayoutLimitTitle'), message)
    throw new Error(message)
  }
  const write = ++latestPositionWrite
  pendingPositionWrites += 1
  localPositions.value = next
  try {
    await desktopIcons.mutate((draft) => {
      draft.positions = Object.fromEntries(
        Object.entries(next).map(([key, position]) => [key, { ...position }]),
      )
    })
  } catch (error) {
    if (write === latestPositionWrite) {
      localPositions.value = Object.fromEntries(
        Object.entries(workspace.value.positions).map(([key, position]) => [key, { ...position }]),
      )
    }
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
    throw error
  } finally {
    pendingPositionWrites -= 1
  }
}

function placementsToPositions(
  placements: DesktopIconPlacement[],
  base = localPositions.value,
): Record<string, DesktopIconPosition> {
  const next = Object.fromEntries(
    Object.entries(base).map(([key, position]) => [key, { ...position }]),
  )
  for (const placement of placements) next[placement.key] = { ...placement.position }
  return next
}

interface DesktopShortcutNativeDrag {
  anchorKey: string
  keys: string[]
  paths: string[]
  origins: Record<string, { left: number; top: number }>
  grabOffset: { left: number; top: number }
  lastX: number
  lastY: number
  localPreviewActive: boolean
  localDropHandled: boolean
  outsideDesktop: boolean
}

let desktopShortcutNativeDrag: DesktopShortcutNativeDrag | undefined

function desktopShortcutDragCandidates(anchor: DesktopEntry): DesktopEntry[] {
  const candidates = selectedIcons.value.has(anchor.key) && selectedIcons.value.size > 1
    ? selectedEntries.value
    : [anchor]
  return candidates.filter(isTransferableDesktopShortcut)
}

function desktopShortcutTransferEntries(anchor: DesktopEntry): FileEntry[] {
  return desktopShortcutDragCandidates(anchor).flatMap((entry) => {
    const metadata = entry.path ? desktopFileMetadata.value[entry.path] : undefined
    if (!metadata || metadata.kind !== entry.launch) return []
    return [metadata]
  })
}

function desktopShortcutTransferReady(entry: DesktopEntry): boolean {
  if (!isTransferableDesktopShortcut(entry) || !localClusterNodeId.value) return false
  const metadata = desktopFileMetadata.value[entry.path!]
  return Boolean(metadata && metadata.kind === entry.launch)
}

function desktopShortcutTransferHint(entry: DesktopEntry): string | undefined {
  if (!isTransferableDesktopShortcut(entry)) return undefined
  return i18n.t(
    desktopShortcutTransferReady(entry)
      ? 'desktop.crossPanelDragReady'
      : 'desktop.crossPanelDragPreparing',
  )
}

async function loadLocalClusterIdentity(): Promise<void> {
  try {
    const hosts = await api.cluster.hosts()
    localClusterNodeId.value = hosts.nodeId
  } catch {
    localClusterNodeId.value = ''
  }
}

function startDesktopShortcutDrag(event: DragEvent, entry: DesktopEntry): void {
  if (!event.dataTransfer || compactIconLayout.value || !isTransferableDesktopShortcut(entry)) {
    event.preventDefault()
    return
  }
  const candidates = desktopShortcutDragCandidates(entry)
  const transferable = desktopShortcutTransferEntries(entry)
  const anchorMetadata = desktopFileMetadata.value[entry.path!]
  if (!localClusterNodeId.value || !anchorMetadata || anchorMetadata.kind !== entry.launch || !transferable.length) {
    event.preventDefault()
    void loadLocalClusterIdentity()
    void refreshDesktopFileMetadata(candidates.map((candidate) => candidate.path!))
    toast.show(i18n.t('desktop.crossPanelDragUnavailableTitle'), {
      message: i18n.t('desktop.crossPanelDragUnavailableMessage'),
    })
    return
  }
  const keys = selectedIcons.value.has(entry.key) && selectedIcons.value.size > 1
    ? [...selectedIcons.value].filter((key) => renderedPositionByKey.value.has(key))
    : [entry.key]
  if (!selectedIcons.value.has(entry.key)) setIconSelection([entry.key])
  const directFile = transferable.length === 1 && transferable[0]!.kind === 'file'
  const nativeArchiveName = directFile
    ? undefined
    : nativeArchiveDownloadName(transferable, 'KPanel Desktop')
  const nativeDownloadURL = directFile
    ? api.files.contentUrl(transferable[0]!.path, 'attachment')
    : nativeArchiveName
      ? api.files.archiveUrl(transferable, nativeArchiveName)
      : undefined
  if (!beginDesktopFileDrag(
    event,
    transferable,
    localClusterNodeId.value,
    'desktop-shortcut',
    nativeDownloadURL,
    nativeArchiveName,
  )) {
    event.preventDefault()
    return
  }
  const grid = desktopIconGrid(iconBounds.value)
  const origins = Object.fromEntries(keys.map((key) => [
    key,
    desktopIconPositionToPixels(renderedPositionByKey.value.get(key)!, iconBounds.value),
  ]))
  const anchorElement = event.currentTarget instanceof HTMLElement ? event.currentTarget : undefined
  const anchorRect = anchorElement?.getBoundingClientRect()
  const grabOffset = {
    left: Math.min(grid.metrics.width, Math.max(0, event.clientX - (anchorRect?.left ?? 0))),
    top: Math.min(grid.metrics.height, Math.max(0, event.clientY - (anchorRect?.top ?? 0))),
  }
  if (anchorElement && typeof event.dataTransfer.setDragImage === 'function') {
    try {
      event.dataTransfer.setDragImage(anchorElement, grabOffset.left, grabOffset.top)
    } catch {
      // Some embedded browsers reject a custom drag image; native dragging still works.
    }
  }
  desktopShortcutNativeDrag = {
    anchorKey: entry.key,
    keys,
    paths: transferable.map((item) => item.path),
    origins,
    grabOffset,
    lastX: event.clientX,
    lastY: event.clientY,
    localPreviewActive: false,
    localDropHandled: false,
    outsideDesktop: false,
  }
  draggingIcons.value = new Set(keys)
  nativeDragHiddenIcons.value = new Set()
  closeContextMenu(false)
  const skipped = Math.max(0, keys.length - transferable.length)
  iconAnnouncement.value = i18n.t('desktop.crossPanelDragStarted', { count: transferable.length })
  if (skipped) {
    toast.show(i18n.t('desktop.crossPanelDragSkippedTitle'), {
      message: i18n.t('desktop.crossPanelDragSkippedMessage', {
        count: transferable.length,
        skipped,
      }),
    })
  }
}

function desktopShortcutDragEndPosition(event: DragEvent): DesktopIconPosition | undefined {
  const drag = desktopShortcutNativeDrag
  if (!drag || drag.outsideDesktop || drag.localDropHandled || event.dataTransfer?.dropEffect !== 'none') return undefined
  if (!Number.isFinite(event.clientX) || !Number.isFinite(event.clientY)) return undefined
  if (event.clientX === 0 && event.clientY === 0) return undefined
  const target = document.elementFromPoint(event.clientX, event.clientY)
  const desktop = desktopElement.value
  if (!target || !desktop?.contains(target)) return undefined
  if (target.closest('.desktop-window, .desktop__widgets, .desktop__taskbar')) return undefined
  return desktopShortcutDropPosition(event)
}

function finishDesktopShortcutDrag(event: DragEvent): void {
  const drag = desktopShortcutNativeDrag
  const fallbackPosition = desktopShortcutDragEndPosition(event)
  if (fallbackPosition) void moveDesktopShortcutDrop(fallbackPosition)
  clearDesktopShortcutNativeDragPreview()
  desktopShortcutNativeDrag = undefined
  draggingIcons.value = new Set()
  nativeDragHiddenIcons.value = new Set()
  clearDesktopFileDrag()
  if (drag?.paths.length) void refreshDesktopFileMetadata(drag.paths)
}

function clearDesktopShortcutNativeDragPreview(): void {
  const drag = desktopShortcutNativeDrag
  if (!drag?.localPreviewActive) return
  drag.localPreviewActive = false
  dragPreviews.value = {}
  stopIconAutoScroll()
}

function updateDesktopShortcutNativeDragPreview(clientX: number, clientY: number): void {
  const drag = desktopShortcutNativeDrag
  const element = iconsElement.value
  const anchorOrigin = drag?.origins[drag.anchorKey]
  if (!drag || !element || !anchorOrigin) return
  const rect = element.getBoundingClientRect()
  drag.lastX = clientX
  drag.lastY = clientY
  drag.localPreviewActive = true
  const desiredLeft = clientX - rect.left - drag.grabOffset.left
  const desiredTop = clientY - rect.top + element.scrollTop - drag.grabOffset.top
  dragPreviews.value = clampedIconDragPreviews(
    drag.origins,
    desiredLeft - anchorOrigin.left,
    desiredTop - anchorOrigin.top,
  )
  scheduleIconAutoScroll()
}

function desktopShortcutDropPosition(event: DragEvent): DesktopIconPosition | undefined {
  const drag = desktopShortcutNativeDrag
  if (!drag) return undefined
  const preview = dragPreviews.value[drag.anchorKey]
  if (preview) return desktopIconPixelsToPosition(preview, iconBounds.value)
  const element = iconsElement.value
  if (!element) return undefined
  const rect = element.getBoundingClientRect()
  return desktopIconPixelsToPosition({
    left: event.clientX - rect.left - drag.grabOffset.left,
    top: event.clientY - rect.top + element.scrollTop - drag.grabOffset.top,
  }, iconBounds.value)
}

async function moveDesktopShortcutDrop(
  destination: DesktopIconPosition,
): Promise<void> {
  const drag = desktopShortcutNativeDrag
  if (!drag || drag.localDropHandled) return
  drag.localDropHandled = true
  for (const key of drag.keys) suppressActivationAfterDrag.add(key)
  const placements = drag.keys.length > 1
    ? moveDesktopIconGroup(
        renderedIconLayout.value.placements,
        drag.keys,
        drag.anchorKey,
        destination,
        iconBounds.value,
      )
    : dropDesktopIcon(
        renderedIconLayout.value.placements,
        drag.anchorKey,
        destination,
        iconBounds.value,
      )
  const next = placementsToPositions(placements)
  iconAnnouncement.value = drag.keys.length > 1
    ? i18n.t('desktop.iconsMoved', { count: drag.keys.length })
    : i18n.t('desktop.iconMoved', { name: iconLabel(drag.anchorKey) })
  draggingIcons.value = new Set()
  nativeDragHiddenIcons.value = new Set()
  clearDesktopShortcutNativeDragPreview()
  await persistPositions(next).catch(() => undefined)
}

interface IconDragState {
  key: string
  keys: string[]
  pointerId: number
  pointerType: string
  captureTarget?: HTMLElement
  pointerCaptured: boolean
  startX: number
  startY: number
  lastX: number
  lastY: number
  startScrollTop: number
  origins: Record<string, { left: number; top: number }>
  moved: boolean
}

let iconDrag: IconDragState | undefined
let iconAutoScrollFrame: number | undefined
const suppressActivationAfterDrag = new Set<string>()

function removeIconDragListeners(): void {
  window.removeEventListener('pointermove', onIconDragMove)
  window.removeEventListener('pointerup', onIconDragEnd)
  window.removeEventListener('pointercancel', onIconDragCancel)
  window.removeEventListener('blur', cancelIconDrag)
  iconsElement.value?.removeEventListener('scroll', onIconDragScroll)
}

function stopIconAutoScroll(): void {
  if (iconAutoScrollFrame === undefined) return
  window.cancelAnimationFrame(iconAutoScrollFrame)
  iconAutoScrollFrame = undefined
}

function clampedIconDragPreviews(
  originsByKey: Record<string, { left: number; top: number }>,
  requestedDeltaX: number,
  requestedDeltaY: number,
): Record<string, { left: number; top: number }> {
  const grid = desktopIconGrid(iconBounds.value)
  const origins = Object.values(originsByKey)
  const minimumLeft = Math.min(...origins.map((origin) => origin.left))
  const maximumLeft = Math.max(...origins.map((origin) => origin.left))
  const minimumTop = Math.min(...origins.map((origin) => origin.top))
  const maximumTop = Math.max(...origins.map((origin) => origin.top))
  const deltaX = Math.min(
    grid.maxLeft - maximumLeft,
    Math.max(-minimumLeft, requestedDeltaX),
  )
  const deltaY = Math.min(
    grid.maxRow * grid.stepY - maximumTop,
    Math.max(-minimumTop, requestedDeltaY),
  )
  return Object.fromEntries(
    Object.entries(originsByKey).map(([key, origin]) => [key, {
      left: origin.left + deltaX,
      top: origin.top + deltaY,
    }]),
  )
}

function updateIconDragPreview(drag: IconDragState): void {
  const scrollDelta = (iconsElement.value?.scrollTop || 0) - drag.startScrollTop
  dragPreviews.value = clampedIconDragPreviews(
    drag.origins,
    drag.lastX - drag.startX,
    drag.lastY - drag.startY + scrollDelta,
  )
}

function iconAutoScrollVelocity(clientY: number): number {
  const element = iconsElement.value
  const rect = element?.getBoundingClientRect()
  if (!element || !rect || rect.height <= 0 || element.scrollHeight <= element.clientHeight) return 0
  const edge = Math.min(56, Math.max(32, rect.height * 0.12))
  if (clientY < rect.top + edge) {
    return -Math.ceil(Math.min(1, (rect.top + edge - clientY) / edge) * 18)
  }
  if (clientY > rect.bottom - edge) {
    return Math.ceil(Math.min(1, (clientY - (rect.bottom - edge)) / edge) * 18)
  }
  return 0
}

function scheduleIconAutoScroll(): void {
  if (iconAutoScrollFrame !== undefined) return
  const pointerDrag = iconDrag?.moved ? iconDrag : undefined
  const nativeDrag = desktopShortcutNativeDrag?.localPreviewActive
    ? desktopShortcutNativeDrag
    : undefined
  const clientY = pointerDrag?.lastY ?? nativeDrag?.lastY
  if (clientY === undefined || !iconAutoScrollVelocity(clientY)) return
  iconAutoScrollFrame = window.requestAnimationFrame(() => {
    iconAutoScrollFrame = undefined
    const activePointer = iconDrag?.moved ? iconDrag : undefined
    const activeNative = desktopShortcutNativeDrag?.localPreviewActive
      ? desktopShortcutNativeDrag
      : undefined
    const element = iconsElement.value
    const activeClientY = activePointer?.lastY ?? activeNative?.lastY
    if (activeClientY === undefined || !element) return
    const velocity = iconAutoScrollVelocity(activeClientY)
    if (!velocity) return
    const before = element.scrollTop
    element.scrollTop = Math.max(
      0,
      Math.min(element.scrollHeight - element.clientHeight, before + velocity),
    )
    if (element.scrollTop === before) return
    if (activePointer) updateIconDragPreview(activePointer)
    else if (activeNative) updateDesktopShortcutNativeDragPreview(activeNative.lastX, activeNative.lastY)
    scheduleIconAutoScroll()
  })
}

function onIconDragScroll(): void {
  const drag = iconDrag
  if (drag?.moved) updateIconDragPreview(drag)
}

function releaseIconDragPointer(drag: IconDragState): void {
  const target = drag.captureTarget
  if (!target || !drag.pointerCaptured) return
  target.removeEventListener('lostpointercapture', onIconDragLostPointerCapture)
  try {
    if (
      typeof target.releasePointerCapture === 'function'
      && (typeof target.hasPointerCapture !== 'function' || target.hasPointerCapture(drag.pointerId))
    ) {
      target.releasePointerCapture(drag.pointerId)
    }
  } catch {
    // Pointer capture may already have been released by the browser.
  }
  drag.pointerCaptured = false
}

function captureIconDragPointer(drag: IconDragState): void {
  const target = drag.captureTarget
  if (drag.pointerCaptured || !target || typeof target.setPointerCapture !== 'function') return
  target.addEventListener('lostpointercapture', onIconDragLostPointerCapture)
  try {
    target.setPointerCapture(drag.pointerId)
    drag.pointerCaptured = true
  } catch {
    target.removeEventListener('lostpointercapture', onIconDragLostPointerCapture)
    // Window listeners keep dragging functional when capture is unavailable.
  }
}

function beginIconDrag(event: PointerEvent, key: string): void {
  if (event.button === 0 && event.isPrimary !== false) suppressActivationAfterDrag.delete(key)
  if (compactIconLayout.value || event.button !== 0 || event.isPrimary === false || iconDrag) return
  const pointerType = event.pointerType || 'mouse'
  if (
    pointerType === 'mouse'
    && shortcutEntries.value.some((entry) => entry.key === key && isTransferableDesktopShortcut(entry))
  ) return
  const position = renderedPositionByKey.value.get(key)
  if (!position) return
  const keys = selectedIcons.value.has(key) && selectedIcons.value.size > 1
    ? [...selectedIcons.value].filter((selectedKey) => renderedPositionByKey.value.has(selectedKey))
    : [key]
  const origins = Object.fromEntries(keys.map((selectedKey) => [
    selectedKey,
    desktopIconPositionToPixels(renderedPositionByKey.value.get(selectedKey)!, iconBounds.value),
  ]))
  iconDrag = {
    key,
    keys,
    pointerId: event.pointerId,
    pointerType,
    captureTarget: event.currentTarget instanceof HTMLElement ? event.currentTarget : undefined,
    pointerCaptured: false,
    startX: event.clientX,
    startY: event.clientY,
    lastX: event.clientX,
    lastY: event.clientY,
    startScrollTop: iconsElement.value?.scrollTop || 0,
    origins,
    moved: false,
  }
  window.addEventListener('pointermove', onIconDragMove, { passive: false })
  window.addEventListener('pointerup', onIconDragEnd)
  window.addEventListener('pointercancel', onIconDragCancel)
  window.addEventListener('blur', cancelIconDrag)
  iconsElement.value?.addEventListener('scroll', onIconDragScroll, { passive: true })
}

function onIconDragMove(event: PointerEvent): void {
  const drag = iconDrag
  if (!drag || event.pointerId !== drag.pointerId) return
  const deltaX = event.clientX - drag.startX
  const deltaY = event.clientY - drag.startY
  drag.lastX = event.clientX
  drag.lastY = event.clientY
  const threshold = drag.pointerType === 'mouse' ? 6 : 12
  if (!drag.moved && Math.hypot(deltaX, deltaY) < threshold) return
  if (!drag.moved) {
    captureIconDragPointer(drag)
    drag.moved = true
    draggingIcons.value = new Set(drag.keys)
    if (!selectedIcons.value.has(drag.key)) setIconSelection([drag.key])
    closeContextMenu(false)
    document.body.classList.add('desktop-icon-dragging')
  }
  event.preventDefault()
  updateIconDragPreview(drag)
  scheduleIconAutoScroll()
}

function finishIconDrag(): IconDragState | undefined {
  const drag = iconDrag
  iconDrag = undefined
  stopIconAutoScroll()
  removeIconDragListeners()
  if (drag) releaseIconDragPointer(drag)
  document.body.classList.remove('desktop-icon-dragging')
  draggingIcons.value = new Set()
  return drag
}

function onIconDragEnd(event: PointerEvent): void {
  const drag = iconDrag
  if (!drag || event.pointerId !== drag.pointerId) return
  const previews = dragPreviews.value
  finishIconDrag()
  dragPreviews.value = {}
  const anchorPreview = previews[drag.key]
  if (!drag.moved || !anchorPreview) return
  for (const key of drag.keys) suppressActivationAfterDrag.add(key)
  const destination = desktopIconPixelsToPosition(anchorPreview, iconBounds.value)
  const placements = drag.keys.length > 1
    ? moveDesktopIconGroup(
        renderedIconLayout.value.placements,
        drag.keys,
        drag.key,
        destination,
        iconBounds.value,
      )
    : dropDesktopIcon(
        renderedIconLayout.value.placements,
        drag.key,
        destination,
        iconBounds.value,
      )
  const next = placementsToPositions(placements)
  iconAnnouncement.value = drag.keys.length > 1
    ? i18n.t('desktop.iconsMoved', { count: drag.keys.length })
    : i18n.t('desktop.iconMoved', { name: iconLabel(drag.key) })
  void persistPositions(next).catch(() => undefined)
}

function cancelIconDrag(): void {
  const drag = finishIconDrag()
  dragPreviews.value = {}
  if (drag?.moved) for (const key of drag.keys) suppressActivationAfterDrag.add(key)
}

function onIconDragCancel(event: PointerEvent): void {
  if (iconDrag && event.pointerId === iconDrag.pointerId) cancelIconDrag()
}

function onIconDragLostPointerCapture(event: PointerEvent): void {
  if (iconDrag && event.pointerId === iconDrag.pointerId) cancelIconDrag()
}

function suppressDraggedActivation(event: Event, key: string): void {
  if (!suppressActivationAfterDrag.has(key)) return
  event.preventDefault()
  event.stopImmediatePropagation()
}

function clearDraggedActivationSuppression(key: string): void {
  suppressActivationAfterDrag.delete(key)
}

function iconLabel(key: string): string {
  if (key.startsWith('nav:')) {
    const app = desktopApps.find((candidate) => `nav:${candidate.path}` === key)
    return app ? i18n.t(app.labelKey) : key
  }
  return [...visibleDynamicEntries.value, ...shortcutEntries.value]
    .find((entry) => entry.key === key)?.name || key
}

function nudgeIcon(key: string, deltaX: number, deltaY: number): void {
  if (compactIconLayout.value) return
  if (!renderedPositionByKey.value.has(key)) {
    const message = i18n.t('desktop.iconLayoutLimitMessage', { count: MAX_DESKTOP_ICON_POSITIONS })
    iconAnnouncement.value = message
    toast.danger(i18n.t('desktop.iconLayoutLimitTitle'), message)
    return
  }
  const direction = deltaX < 0 ? 'left' : deltaX > 0 ? 'right' : deltaY < 0 ? 'up' : 'down'
  const movingKeys = selectedIcons.value.has(key) && selectedIcons.value.size > 1
    ? [...selectedIcons.value].filter((selectedKey) => renderedPositionByKey.value.has(selectedKey))
    : [key]
  const current = renderedPositionByKey.value.get(key)!
  const destination = desktopIconPositionForGridSlot((() => {
    const slot = desktopIconGridSlotForPosition(current, iconBounds.value)
    if (direction === 'left') slot.column -= 1
    else if (direction === 'right') slot.column += 1
    else if (direction === 'up') slot.row -= 1
    else slot.row += 1
    return slot
  })(), iconBounds.value)
  const placements = movingKeys.length > 1
    ? moveDesktopIconGroup(renderedIconLayout.value.placements, movingKeys, key, destination, iconBounds.value)
    : moveDesktopIconByKeyboard(renderedIconLayout.value.placements, key, direction, iconBounds.value)
  setIconSelection(movingKeys)
  iconAnnouncement.value = movingKeys.length > 1
    ? i18n.t('desktop.iconsMoved', { count: movingKeys.length })
    : i18n.t('desktop.iconMoved', { name: iconLabel(key) })
  void persistPositions(placementsToPositions(placements)).catch(() => undefined)
}

async function autoArrangeIcons(): Promise<void> {
  if (compactIconLayout.value) return
  closeContextMenu()
  const arranged = autoArrangeDesktopIcons(allIconKeys.value, iconBounds.value)
  if (arranged.overflowKeys.length) {
    const message = i18n.t('desktop.iconLayoutLimitMessage', { count: MAX_DESKTOP_ICON_POSITIONS })
    iconAnnouncement.value = message
    toast.danger(i18n.t('desktop.iconLayoutLimitTitle'), message)
    return
  }
  try {
    await persistPositions(placementsToPositions(arranged.placements))
    iconAnnouncement.value = i18n.t('desktop.iconsArranged')
    toast.success(i18n.t('desktop.iconsArranged'))
  } catch {
    // persistPositions already surfaced a specific failure.
  }
}

interface DesktopSelectionFrame {
  pointerId: number
  captureTarget?: HTMLElement
  pointerCaptured: boolean
  startX: number
  startY: number
  currentX: number
  currentY: number
  moved: boolean
  additive: boolean
  previousSelection: Set<string>
}

const selectionBox = ref<{ left: number; top: number; width: number; height: number }>()
let selectionFrame: DesktopSelectionFrame | undefined

function removeSelectionFrameListeners(): void {
  window.removeEventListener('pointermove', onSelectionFrameMove)
  window.removeEventListener('pointerup', onSelectionFrameEnd)
  window.removeEventListener('pointercancel', onSelectionFrameCancel)
  window.removeEventListener('blur', cancelSelectionFrame)
}

function releaseSelectionFramePointer(frame: DesktopSelectionFrame): void {
  const target = frame.captureTarget
  if (!target || !frame.pointerCaptured) return
  target.removeEventListener('lostpointercapture', onSelectionFrameLostPointerCapture)
  try {
    if (
      typeof target.releasePointerCapture === 'function'
      && (typeof target.hasPointerCapture !== 'function' || target.hasPointerCapture(frame.pointerId))
    ) {
      target.releasePointerCapture(frame.pointerId)
    }
  } catch {
    // Pointer capture may already have been released by the browser.
  }
  frame.pointerCaptured = false
}

function captureSelectionFramePointer(frame: DesktopSelectionFrame): void {
  const target = frame.captureTarget
  if (frame.pointerCaptured || !target || typeof target.setPointerCapture !== 'function') return
  target.addEventListener('lostpointercapture', onSelectionFrameLostPointerCapture)
  try {
    target.setPointerCapture(frame.pointerId)
    frame.pointerCaptured = true
  } catch {
    target.removeEventListener('lostpointercapture', onSelectionFrameLostPointerCapture)
  }
}

function updateSelectionFromFrame(frame: DesktopSelectionFrame): void {
  const left = Math.min(frame.startX, frame.currentX)
  const top = Math.min(frame.startY, frame.currentY)
  const right = Math.max(frame.startX, frame.currentX)
  const bottom = Math.max(frame.startY, frame.currentY)
  selectionBox.value = { left, top, width: right - left, height: bottom - top }
  const next = frame.additive ? new Set(frame.previousSelection) : new Set<string>()
  for (const slot of iconsElement.value?.querySelectorAll<HTMLElement>('[data-icon-key]') || []) {
    const rect = slot.getBoundingClientRect()
    if (rect.right <= left || rect.left >= right || rect.bottom <= top || rect.top >= bottom) continue
    const key = slot.dataset.iconKey
    if (key) next.add(key)
  }
  setIconSelection(next)
}

function finishSelectionFrame(cancelled: boolean): DesktopSelectionFrame | undefined {
  const frame = selectionFrame
  selectionFrame = undefined
  removeSelectionFrameListeners()
  if (frame) releaseSelectionFramePointer(frame)
  document.body.classList.remove('desktop-selection-active')
  selectionBox.value = undefined
  if (cancelled && frame) setIconSelection(frame.previousSelection)
  return frame
}

function onSelectionFrameMove(event: PointerEvent): void {
  const frame = selectionFrame
  if (!frame || event.pointerId !== frame.pointerId) return
  frame.currentX = event.clientX
  frame.currentY = event.clientY
  if (!frame.moved && Math.hypot(event.clientX - frame.startX, event.clientY - frame.startY) < 4) return
  if (!frame.moved) {
    frame.moved = true
    captureSelectionFramePointer(frame)
    document.body.classList.add('desktop-selection-active')
  }
  event.preventDefault()
  updateSelectionFromFrame(frame)
}

function onSelectionFrameEnd(event: PointerEvent): void {
  const frame = selectionFrame
  if (!frame || event.pointerId !== frame.pointerId) return
  finishSelectionFrame(false)
  if (frame.moved) {
    iconAnnouncement.value = i18n.t('desktop.selectedCount', { count: selectedIconCount.value })
  }
}

function cancelSelectionFrame(): void {
  finishSelectionFrame(true)
}

function onSelectionFrameCancel(event: PointerEvent): void {
  if (selectionFrame && event.pointerId === selectionFrame.pointerId) cancelSelectionFrame()
}

function onSelectionFrameLostPointerCapture(event: PointerEvent): void {
  if (selectionFrame && event.pointerId === selectionFrame.pointerId) cancelSelectionFrame()
}

function onGlobalPointerDown(event: PointerEvent): void {
  if (iconDrag && event.pointerId !== iconDrag.pointerId) cancelIconDrag()
  if (selectionFrame && event.pointerId !== selectionFrame.pointerId) cancelSelectionFrame()
  if (!contextMenu.value.open) return
  // A right-button press may be followed by one or more contextmenu events
  // while the button is held. Keep the existing menu mounted and let the
  // contextmenu handler reposition it, instead of starting close/open
  // transitions in the same pointer cycle.
  if (event.button === 2) return
  const target = event.target
  if (target instanceof Node && contextMenuElement.value?.contains(target)) return
  closeContextMenu(false)
}

function onGlobalKeyDown(event: KeyboardEvent): void {
  const target = event.target
  const editing = target instanceof HTMLElement
    && (target.matches('input, textarea, select') || target.isContentEditable)
  const desktopFocused = target instanceof Node && Boolean(desktopElement.value?.contains(target))
  if ((event.ctrlKey || event.metaKey) && event.key.toLocaleLowerCase() === 'a' && desktopFocused && !editing) {
    event.preventDefault()
    setIconSelection(allIconKeys.value)
    iconAnnouncement.value = i18n.t('desktop.selectedCount', { count: selectedIconCount.value })
    return
  }
  if ((event.key === 'Delete' || event.key === 'Backspace') && desktopFocused && !editing && selectedIconCount.value) {
    event.preventDefault()
    requestBatchRemoveSelected()
    return
  }
  if (event.key !== 'Escape') return
  if (selectionFrame) cancelSelectionFrame()
  else if (iconDrag) cancelIconDrag()
  else if (contextMenu.value.open) closeContextMenu()
  else clearIconSelection()
}

function onContextMenuKeyDown(event: KeyboardEvent): void {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const items = Array.from(contextMenuElement.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') || [])
  if (!items.length) return
  event.preventDefault()
  const current = items.indexOf(document.activeElement as HTMLButtonElement)
  const index = event.key === 'Home'
    ? 0
    : event.key === 'End'
      ? items.length - 1
      : event.key === 'ArrowDown'
        ? (current + 1 + items.length) % items.length
        : (current - 1 + items.length) % items.length
  items[index]?.focus({ preventScroll: true })
}

function onDesktopPointerDown(event: PointerEvent): void {
  const target = event.target instanceof Element ? event.target : undefined
  if (target?.closest('.desktop-window, .desktop__widgets, .desktop__taskbar, .desktop__icon, .desktop__selection-actions')) return
  const currentTarget = event.currentTarget instanceof HTMLElement ? event.currentTarget : undefined
  currentTarget?.focus({ preventScroll: true })
  if (
    compactIconLayout.value
    || (event.button !== undefined && event.button !== 0)
    || event.isPrimary === false
    || (event.pointerType && event.pointerType !== 'mouse')
    || selectionFrame
  ) return
  const previousSelection = new Set(selectedIcons.value)
  const additive = event.ctrlKey || event.metaKey || event.shiftKey
  if (!additive) clearIconSelection()
  if (contextMenu.value.open) closeContextMenu(false)
  selectionFrame = {
    pointerId: event.pointerId,
    captureTarget: currentTarget,
    pointerCaptured: false,
    startX: event.clientX,
    startY: event.clientY,
    currentX: event.clientX,
    currentY: event.clientY,
    moved: false,
    additive,
    previousSelection,
  }
  window.addEventListener('pointermove', onSelectionFrameMove, { passive: false })
  window.addEventListener('pointerup', onSelectionFrameEnd)
  window.addEventListener('pointercancel', onSelectionFrameCancel)
  window.addEventListener('blur', cancelSelectionFrame)
}

function desktopFileDropAllowed(event: DragEvent): boolean {
  if (!hasDesktopFileDrag(event) && !hasCrossPanelFileDrag(event) && !hasExternalFileDrop(event)) return false
  const target = event.target as HTMLElement | null
  return !target?.closest('.desktop-window, .desktop__widgets, .desktop__taskbar')
}

function onDesktopFileDragOver(event: DragEvent): void {
  if (!desktopFileDropAllowed(event)) {
    fileDropActive.value = false
    clearDesktopShortcutNativeDragPreview()
    return
  }
  event.preventDefault()
  const internal = hasDesktopFileDrag(event)
  const localShortcutMove = internal
    && desktopShortcutNativeDrag
    && peekDesktopFileDragOrigin(event) === 'desktop-shortcut'
  if (localShortcutMove) {
    desktopShortcutNativeDrag!.outsideDesktop = false
    nativeDragHiddenIcons.value = new Set()
    fileDropActive.value = false
    updateDesktopShortcutNativeDragPreview(event.clientX, event.clientY)
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    return
  }
  const crossPanel = !internal && hasCrossPanelFileDrag(event)
  const external = !internal && !crossPanel && hasExternalFileDrop(event)
  if ((external || crossPanel) && desktopTransfer.value && ['preparing', 'uploading'].includes(desktopTransfer.value.phase)) {
    fileDropActive.value = false
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
    return
  }
  fileDropMode.value = external ? 'upload' : crossPanel ? 'panel-copy' : 'shortcut'
  if (event.dataTransfer) event.dataTransfer.dropEffect = external || crossPanel ? 'copy' : 'link'
  fileDropActive.value = true
}

function onDesktopFileDragLeave(event: DragEvent): void {
  const related = event.relatedTarget as Node | null
  if (!related || !(event.currentTarget as HTMLElement).contains(related)) {
    if (desktopShortcutNativeDrag) {
      desktopShortcutNativeDrag.outsideDesktop = true
      nativeDragHiddenIcons.value = new Set(desktopShortcutNativeDrag.keys)
    }
    fileDropActive.value = false
    clearDesktopShortcutNativeDragPreview()
  }
}

function desktopDropPosition(event: DragEvent): DesktopIconPosition | undefined {
  const element = iconsElement.value
  if (!element) return undefined
  const rect = element.getBoundingClientRect()
  return desktopIconPixelsToPosition({
    left: event.clientX - rect.left,
    top: event.clientY - rect.top + element.scrollTop,
  }, iconBounds.value)
}

async function addDroppedFileEntries(
  sourceEntries: Parameters<typeof addFileEntriesToDesktop>[0],
  destination: DesktopIconPosition,
): Promise<Awaited<ReturnType<typeof addFileEntriesToDesktop>>> {
  return addFileEntriesToDesktop(sourceEntries, (draft, added) => {
    if (!added.length) return
    const addedKeys = added.map((shortcut) => `shortcut:${shortcut.id}`)
    const layout = deriveDesktopIconLayout(
      [...allIconKeys.value, ...addedKeys],
      Object.entries(draft.positions).map(([key, position]) => ({ key, position })),
      iconBounds.value,
      false,
    )
    const placeableKeys = addedKeys.filter((key) => layout.placements.some((placement) => placement.key === key))
    if (!placeableKeys.length) return
    const placeableKeySet = new Set(placeableKeys)
    const initial = new Map(layout.placements.map((placement) => [placement.key, placement.position]))
    const grid = desktopIconGrid(iconBounds.value)
    const start = desktopIconGridSlotForPosition(destination, iconBounds.value)
    const page = Math.floor(start.row / grid.rows)
    const startIndex = page * grid.pageCapacity
      + start.column * grid.rows
      + (start.row % grid.rows)
    let placements = layout.placements
    placeableKeys.forEach((key, index) => {
      const ordered = startIndex + index
      const targetPage = Math.floor(ordered / grid.pageCapacity)
      const withinPage = ordered % grid.pageCapacity
      const slot = {
        column: Math.floor(withinPage / grid.rows),
        row: targetPage * grid.rows + (withinPage % grid.rows),
      }
      placements = dropDesktopIcon(
        placements,
        key,
        desktopIconPositionForGridSlot(slot, iconBounds.value),
        iconBounds.value,
      )
    })
    for (const placement of placements) {
      const previous = initial.get(placement.key)
      if (placeableKeySet.has(placement.key) || !previous || previous.x !== placement.position.x || previous.y !== placement.position.y) {
        draft.positions[placement.key] = placement.position
      }
    }
  })
}

function showDropPulse(event: DragEvent): void {
  if (dropPulseTimer !== undefined) window.clearTimeout(dropPulseTimer)
  dropPulse.value = { id: Date.now(), left: event.clientX, top: event.clientY }
  dropPulseTimer = window.setTimeout(() => {
    dropPulse.value = undefined
    dropPulseTimer = undefined
  }, motionDuration(720))
}

function scheduleDesktopTransferClear(delay = 5200): void {
  if (desktopTransferClearTimer !== undefined) window.clearTimeout(desktopTransferClearTimer)
  desktopTransferClearTimer = window.setTimeout(() => {
    desktopTransfer.value = undefined
    desktopTransferClearTimer = undefined
  }, delay)
}

function cancelDesktopTransfer(): void {
  desktopTransferController?.abort()
}

function dismissDesktopTransfer(): void {
  if (desktopTransfer.value && ['preparing', 'uploading'].includes(desktopTransfer.value.phase)) return
  desktopTransfer.value = undefined
  if (desktopTransferClearTimer !== undefined) {
    window.clearTimeout(desktopTransferClearTimer)
    desktopTransferClearTimer = undefined
  }
}

function openUploadLocationDialog(): void {
  if (desktopTransferActive.value) return
  uploadLocationDraft.value = desktopUploadDirectory.value
  uploadLocationError.value = ''
  uploadLocationOpen.value = true
}

function closeUploadLocationDialog(): void {
  if (uploadLocationSaving.value) return
  uploadLocationOpen.value = false
  uploadLocationError.value = ''
}

async function saveUploadLocation(): Promise<void> {
  const path = normalizedHostDirectory(uploadLocationDraft.value)
  if (!path) {
    uploadLocationError.value = i18n.t('desktop.transferLocationInvalid')
    return
  }
  uploadLocationSaving.value = true
  uploadLocationError.value = ''
  try {
    if (path !== DESKTOP_UPLOAD_DIRECTORY) {
      const target = await api.files.entry(path)
      if (target.kind !== 'directory') {
        uploadLocationError.value = i18n.t('desktop.transferLocationNotDirectory')
        return
      }
    }
    desktopUploadDirectory.value = path
    try {
      window.localStorage.setItem(DESKTOP_UPLOAD_LOCATION_KEY, path)
    } catch {
      // The selected location still applies to this session when storage is unavailable.
    }
    uploadLocationOpen.value = false
    toast.success(i18n.t('desktop.transferLocationSaved'), path)
  } catch (error) {
    uploadLocationError.value = error instanceof Error ? error.message : i18n.t('desktop.transferLocationUnavailable')
  } finally {
    uploadLocationSaving.value = false
  }
}

function openDesktopTransferDirectory(): void {
  const app = findDesktopApp('/files')
  if (!app) return
  const route = `/files?${new URLSearchParams({ path: desktopUploadDirectory.value }).toString()}`
  const existing = openWindows.value.find(
    (windowState) => fileWindowDirectory(windowState.path) === desktopUploadDirectory.value,
  )
  if (existing) {
    desktop.restoreWindow(existing.id)
    return
  }
  const windowId = desktop.openWindow(route, app.labelKey, true)
  if (windowId === 0) {
    toast.show(i18n.t('desktop.windowLimitTitle'), { message: i18n.t('desktop.windowLimitMessage') })
  }
}

function externalDropErrorMessage(error: DesktopExternalDropError): string {
  switch (error.code) {
    case 'too_many': return i18n.t('desktop.externalDropErrorTooMany')
    case 'too_large': return i18n.t('desktop.externalDropErrorTooLarge')
    case 'too_deep': return i18n.t('desktop.externalDropErrorTooDeep')
    case 'invalid': return i18n.t('desktop.externalDropErrorInvalid')
    default: return i18n.t('desktop.externalDropErrorUnsupported')
  }
}

async function addInternalFileDrop(event: DragEvent, destination: DesktopIconPosition): Promise<void> {
  const sourceEntries = desktopFileDragEntries(event)
  clearDesktopFileDrag()
  if (!sourceEntries.length) return
  try {
    const result = await addDroppedFileEntries(sourceEntries, destination)
    if (result.added.length) {
      toast.success(
        result.added.length === 1 ? i18n.t('desktop.fileShortcutAdded') : i18n.t('desktop.fileShortcutsAdded', { count: result.added.length }),
        result.added.length === 1 ? result.added[0]!.name : i18n.t('desktop.fileShortcutsDropHint'),
      )
    } else if (result.duplicates.length) {
      toast.show(i18n.t('desktop.fileShortcutDuplicate'), { message: result.duplicates[0]!.name })
    }
  } catch (error) {
    if (error instanceof DesktopShortcutLimitError) {
      toast.danger(i18n.t('desktop.shortcutLimitTitle'), i18n.t('desktop.shortcutLimitMessage', {
        available: error.available,
        requested: error.requested,
      }))
    } else {
      toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
    }
  }
}

async function addExternalFileDrop(event: DragEvent, destination: DesktopIconPosition): Promise<void> {
  const dataTransfer = event.dataTransfer
  if (!dataTransfer) return
  if (desktopTransfer.value && ['preparing', 'uploading'].includes(desktopTransfer.value.phase)) {
    toast.show(i18n.t('desktop.transferBusyTitle'), { message: i18n.t('desktop.transferBusyMessage') })
    return
  }
  if (desktopTransferClearTimer !== undefined) window.clearTimeout(desktopTransferClearTimer)
  const controller = new AbortController()
  desktopTransferController = controller
  desktopTransfer.value = {
    phase: 'preparing', roots: 0, failed: 0, detail: '', currentName: '',
    completedFiles: 0, totalFiles: 0, loadedBytes: 0, totalBytes: 0,
  }
  showDropPulse(event)
  try {
    const manifest = await collectExternalDrop(dataTransfer, controller.signal)
    desktopTransfer.value = {
      phase: 'uploading', roots: manifest.roots.length, failed: 0, detail: '',
      currentName: manifest.roots[0]?.name || '', completedFiles: 0,
      totalFiles: manifest.files.length, loadedBytes: 0, totalBytes: manifest.totalBytes,
    }
    const result = await uploadExternalDrop(manifest, api.files, controller.signal, (progress) => {
      if (desktopTransferController !== controller || controller.signal.aborted) return
      desktopTransfer.value = {
        ...progress,
        phase: 'uploading', roots: manifest.roots.length, failed: 0, detail: '',
      }
    }, desktopUploadDirectory.value)
    if (controller.signal.aborted) return
    let addedCount = 0
    let shortcutDetail = ''
    let shortcutSaveFailed = false
    try {
      const shortcutResult = await addDroppedFileEntries(result.entries, destination)
      addedCount = shortcutResult.added.length
      if (shortcutResult.duplicates.length) shortcutDetail = i18n.t('desktop.fileShortcutDuplicate')
    } catch (error) {
      shortcutSaveFailed = true
      shortcutDetail = error instanceof DesktopShortcutLimitError
        ? i18n.t('desktop.transferShortcutLimit')
        : workspaceErrorMessage(error)
    }
    const shortcutFailed = shortcutSaveFailed && addedCount < result.entries.length
    const phase = result.failed.length || shortcutFailed ? 'partial' : 'complete'
    desktopTransfer.value = {
      phase,
      roots: manifest.roots.length,
      failed: result.failed.length + (shortcutFailed ? result.entries.length - addedCount : 0),
      detail: shortcutDetail || (result.failed[0]?.detail ?? ''),
      currentName: result.entries.at(-1)?.name || manifest.roots.at(-1)?.name || '',
      completedFiles: manifest.files.length - result.failed.length,
      totalFiles: manifest.files.length,
      loadedBytes: manifest.totalBytes,
      totalBytes: manifest.totalBytes,
    }
    if (phase === 'complete') {
      toast.success(i18n.t('desktop.transferCompleteTitle'), i18n.t('desktop.transferCompleteMessage', {
        count: addedCount,
      }))
      scheduleDesktopTransferClear()
    } else {
      toast.danger(i18n.t('desktop.transferPartialTitle'), shortcutDetail || result.failed[0]?.detail || '')
    }
  } catch (error) {
    if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) {
      desktopTransfer.value = {
        ...(desktopTransfer.value || {
          roots: 0, failed: 0, detail: '', currentName: '', completedFiles: 0,
          totalFiles: 0, loadedBytes: 0, totalBytes: 0,
        }),
        phase: 'cancelled', detail: i18n.t('desktop.transferCancelledMessage'),
      }
      scheduleDesktopTransferClear(2600)
      return
    }
    const detail = error instanceof DesktopExternalDropError
      ? externalDropErrorMessage(error)
      : error instanceof Error
        ? error.message
        : i18n.t('desktop.transferFailedMessage')
    desktopTransfer.value = {
      ...(desktopTransfer.value || {
        roots: 0, failed: 1, currentName: '', completedFiles: 0,
        totalFiles: 0, loadedBytes: 0, totalBytes: 0,
      }),
      phase: 'error', failed: Math.max(1, desktopTransfer.value?.failed || 0), detail,
    }
    toast.danger(i18n.t('desktop.transferFailedTitle'), detail)
  } finally {
    if (desktopTransferController === controller) desktopTransferController = undefined
  }
}

async function addCrossPanelFileDrop(event: DragEvent, destination: DesktopIconPosition): Promise<void> {
  const payload = crossPanelFileDragEntries(event)
  if (!payload) {
    toast.danger(i18n.t('desktop.transferFailedTitle'), i18n.t('desktop.panelCopyInvalid'))
    return
  }
  if (payload.sourceNodeId === localClusterNodeId.value) {
    toast.show(i18n.t('desktop.panelCopySameNode'))
    return
  }
  if (desktopTransferActive.value) {
    toast.show(i18n.t('desktop.transferBusyTitle'), { message: i18n.t('desktop.transferBusyMessage') })
    return
  }
  if (desktopTransferClearTimer !== undefined) window.clearTimeout(desktopTransferClearTimer)
  const controller = new AbortController()
  desktopTransferController = controller
  const total = payload.entries.length
  desktopTransfer.value = {
    phase: 'preparing', operation: 'panel-copy', roots: total, failed: 0, detail: i18n.t('desktop.panelCopyConnecting'),
    currentName: payload.entries[0]?.name || '', completedFiles: 0, totalFiles: total, loadedBytes: 0, totalBytes: 0,
  }
  showDropPulse(event)
  try {
    const result = await transferCrossPanelFileBatch(
      payload,
      desktopUploadDirectory.value,
      api.files.transferFromPanel,
      ({ source, event: progress, completed }) => {
        if (desktopTransferController !== controller || controller.signal.aborted) return
        const loadedBytes = progress.loadedBytes || 0
        const totalBytes = progress.totalBytes || 0
        desktopTransfer.value = {
          phase: progress.state === 'connecting' ? 'preparing' : 'uploading', operation: 'panel-copy',
          roots: total, failed: 0,
          detail: progress.state === 'committing'
            ? i18n.t('desktop.panelCopyCommitting')
            : progress.state === 'connecting'
              ? i18n.t('desktop.panelCopyConnecting')
              : i18n.t('desktop.panelCopyTransferring'),
          currentName: source.name, completedFiles: completed, totalFiles: total, loadedBytes, totalBytes,
        }
      }, controller.signal)
    const copiedEntries = result.succeeded.map(({ entry }) => entry)
    let shortcutCount = 0
    let shortcutDetail = ''
    if (copiedEntries.length) {
      try {
        const shortcuts = await addDroppedFileEntries(copiedEntries, destination)
        shortcutCount = shortcuts.added.length + shortcuts.duplicates.length
        if (shortcuts.duplicates.length) shortcutDetail = i18n.t('desktop.fileShortcutDuplicate')
      } catch (error) {
        shortcutDetail = error instanceof DesktopShortcutLimitError
          ? i18n.t('desktop.transferShortcutLimit')
          : workspaceErrorMessage(error)
      }
    }
    const shortcutFailures = Math.max(0, copiedEntries.length - shortcutCount)
    const failures = result.failed.length + shortcutFailures
    const attempted = result.succeeded.length + result.failed.length
    const failureDetail = result.failed[0]?.detail || shortcutDetail
    const phase: DesktopTransferPhase = result.cancelled
      ? copiedEntries.length ? 'partial' : 'cancelled'
      : copiedEntries.length === 0 ? 'error'
        : failures > 0 ? 'partial' : 'complete'
    const detail = result.cancelled
      ? i18n.t('desktop.panelCopyCancelledPartial', { count: copiedEntries.length })
      : failures > 0
        ? `${i18n.t('desktop.panelCopyBatchPartial', { success: shortcutCount, failed: failures })}${failureDetail ? ` ${failureDetail}` : ''}`
        : total > 1
          ? i18n.t('desktop.panelCopyBatchCompleteMessage', { count: total })
          : shortcutDetail
    desktopTransfer.value = {
      phase, operation: 'panel-copy', roots: total, failed: failures, detail,
      currentName: copiedEntries.at(-1)?.name || payload.entries[Math.min(attempted, total - 1)]?.name || '',
      completedFiles: attempted, totalFiles: total, loadedBytes: 0, totalBytes: 0,
    }
    if (result.cancelled) {
      toast.show(i18n.t('desktop.transferCancelledTitle'), { message: detail })
      scheduleDesktopTransferClear(2600)
    } else if (phase === 'complete') {
      toast.success(
        i18n.t('desktop.panelCopyCompleteTitle'),
        total === 1
          ? i18n.t('desktop.panelCopyCompleteMessage', { name: copiedEntries[0]!.name })
          : i18n.t('desktop.panelCopyBatchCompleteMessage', { count: total }),
      )
      scheduleDesktopTransferClear()
    } else {
      toast.danger(
        phase === 'error' ? i18n.t('desktop.transferFailedTitle') : i18n.t('desktop.transferPartialTitle'),
        detail || result.failed[0]?.detail || i18n.t('desktop.transferFailedMessage'),
      )
    }
  } catch (error) {
    if (controller.signal.aborted || (error instanceof DOMException && error.name === 'AbortError')) {
      desktopTransfer.value = {
        ...(desktopTransfer.value || {
          roots: total, failed: 0, detail: '', currentName: payload.entries[0]?.name || '',
          completedFiles: 0, totalFiles: total, loadedBytes: 0, totalBytes: 0,
        }),
        phase: 'cancelled', detail: i18n.t('desktop.transferCancelledMessage'),
      }
      scheduleDesktopTransferClear(2600)
      return
    }
    const detail = error instanceof Error ? error.message : i18n.t('desktop.transferFailedMessage')
    desktopTransfer.value = {
      ...(desktopTransfer.value || {
        roots: total, failed: 1, currentName: payload.entries[0]?.name || '',
        completedFiles: 0, totalFiles: total, loadedBytes: 0, totalBytes: 0,
      }),
      phase: 'error', failed: 1, detail,
    }
    toast.danger(i18n.t('desktop.transferFailedTitle'), detail)
  } finally {
    if (desktopTransferController === controller) desktopTransferController = undefined
  }
}

async function onDesktopFileDrop(event: DragEvent): Promise<void> {
  if (!desktopFileDropAllowed(event)) return
  event.preventDefault()
  fileDropActive.value = false
  const localShortcutMove = Boolean(
    desktopShortcutNativeDrag
    && hasDesktopFileDrag(event)
    && desktopFileDragOrigin(event) === 'desktop-shortcut',
  )
  const destination = localShortcutMove
    ? desktopShortcutDropPosition(event)
    : desktopDropPosition(event)
  if (!destination) return
  if (localShortcutMove) await moveDesktopShortcutDrop(destination)
  else if (hasDesktopFileDrag(event)) await addInternalFileDrop(event, destination)
  else if (hasCrossPanelFileDrag(event)) await addCrossPanelFileDrop(event, destination)
  else if (hasExternalFileDrop(event)) await addExternalFileDrop(event, destination)
}

function onContextMenuAction(
  action: 'refresh' | 'theme' | 'classic' | 'about' | 'processes' | 'add-shortcut'
    | 'manage-icons' | 'wallpaper' | 'fullscreen',
): void {
  closeContextMenu()
  switch (action) {
    case 'refresh':
      void refreshDesktop()
      break
    case 'theme':
      themeTogglePendingAfterContextMenu = true
      break
    case 'wallpaper':
      wallpaperDialogOpen.value = true
      break
    case 'fullscreen':
      void toggleDocumentFullscreen()
      break
    case 'classic':
      void enterClassicSafely()
      break
    case 'about':
      toast.success(i18n.t('desktop.aboutTitle'), i18n.t('desktop.aboutMessage'))
      break
    case 'add-shortcut':
      openShortcutDialog()
      break
    case 'manage-icons':
      iconManagerOpen.value = true
      break
    case 'processes': {
      const windowId = desktop.openWindow('/processes', 'route.processes', false)
      if (windowId === 0) {
        toast.show(i18n.t('desktop.windowLimitTitle'), {
          message: i18n.t('desktop.windowLimitMessage'),
        })
      }
      break
    }
  }
}

function onContextMenuAfterLeave(): void {
  if (!themeTogglePendingAfterContextMenu) return
  themeTogglePendingAfterContextMenu = false
  toggleDesktopTheme()
}

function toggleDesktopTheme(): void {
  const root = document.documentElement
  const duration = motionDuration(420)
  if (themeTransitionTimer !== undefined) window.clearTimeout(themeTransitionTimer)
  if (duration > 0) root.classList.add('desktop-theme-transitioning')
  theme.setTheme(theme.resolved.value === 'dark' ? 'light' : 'dark')
  if (duration === 0) return
  themeTransitionTimer = window.setTimeout(() => {
    root.classList.remove('desktop-theme-transitioning')
    themeTransitionTimer = undefined
  }, duration + 40)
}

function selectDesktopWallpaper(wallpaperID: DesktopWallpaperID): void {
  desktopWallpaperID.value = wallpaperID
  wallpaperDialogOpen.value = false
  try {
    window.localStorage.setItem(DESKTOP_WALLPAPER_KEY, wallpaperID)
  } catch {
    // The wallpaper still applies to this session when storage is unavailable.
  }
}

function onNavMenuOpen(): void {
  const path = menuNavPath.value
  closeContextMenu()
  if (path) openNavIcon(path)
}

async function toggleDocumentFullscreen(): Promise<void> {
  if (await documentFullscreen.toggle()) return
  toast.show(i18n.t('desktop.fullscreenUnavailableTitle'), {
    message: i18n.t('desktop.fullscreenUnavailableMessage'),
  })
}

async function enterClassicSafely(): Promise<void> {
  if (await desktopCloseGuardCoordinator.checkAll()) desktop.enterClassic()
}

function onEntryMenuOpen(): void {
  const entry = menuEntry.value
  closeContextMenu()
  if (entry) openEntry(entry)
}

function onEntryMenuDetails(): void {
  const entry = menuEntry.value
  closeContextMenu()
  if (!entry) return
  if (entry.kind === 'app') {
    openAppMarketEntry(entry)
    return
  }
  detailEntry.value = entry
}

function onDetailEntryOpen(): void {
  const entry = detailEntry.value
  detailEntry.value = undefined
  if (entry) openEntry(entry)
}

function onEntryMenuRename(): void {
  const entry = menuEntry.value
  closeContextMenu()
  if (entry?.kind !== 'site') return
  renameEntry.value = entry
  renameValue.value = entry.name
}

function closeRename(): void {
  renameEntry.value = undefined
  renameValue.value = ''
}

async function saveRename(): Promise<void> {
  const entry = renameEntry.value
  const name = renameValue.value.trim().slice(0, MAX_SITE_NAME_LENGTH)
  if (entry?.kind !== 'site' || !name) return
  const defaultName = defaultSiteName(entry)
  try {
    await desktopIcons.mutate((draft) => {
      if (name === defaultName) delete draft.labels[entry.key]
      else draft.labels[entry.key] = name
    })
    const next = { ...siteNames.value }
    if (name === defaultName) delete next[entry.id]
    else next[entry.id] = name
    siteNames.value = next
    window.localStorage.removeItem(SITE_RENAMES_KEY)
    entries.value = applySiteNames(entries.value)
    closeRename()
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

async function resetRename(): Promise<void> {
  const entry = renameEntry.value
  if (entry?.kind !== 'site') return
  try {
    await desktopIcons.mutate((draft) => {
      delete draft.labels[entry.key]
    })
    const next = { ...siteNames.value }
    delete next[entry.id]
    siteNames.value = next
    window.localStorage.removeItem(SITE_RENAMES_KEY)
    entries.value = applySiteNames(entries.value)
    closeRename()
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

function requestRemoveEntry(): void {
  const entry = menuEntry.value
  closeContextMenu()
  if (entry?.kind === 'app' || entry?.kind === 'site') removingEntry.value = entry
}

async function confirmRemoveEntry(): Promise<void> {
  const entry = removingEntry.value
  if (!entry || (entry.kind !== 'app' && entry.kind !== 'site')) return
  try {
    await desktopIcons.mutate((draft) => {
      if (!draft.hiddenEntryKeys.includes(entry.key)) draft.hiddenEntryKeys.push(entry.key)
    })
    clearIconSelection()
    removingEntry.value = undefined
    toast.success(i18n.t('desktop.removedFromDesktopTitle'), entry.name)
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

function requestBatchRemoveSelected(keys: readonly string[] = [...selectedIcons.value]): void {
  const selected = new Set(keys)
  const removable = [...visibleDynamicEntries.value, ...shortcutEntries.value]
    .filter((entry) => selected.has(entry.key))
  closeContextMenu()
  if (removable.length) {
    batchRemovingEntries.value = removable
    return
  }
  iconAnnouncement.value = i18n.t('desktop.fixedEntriesCannotRemove')
  toast.show(i18n.t('desktop.fixedEntriesCannotRemove'))
}

async function confirmBatchRemoveSelected(): Promise<void> {
  const targets = batchRemovingEntries.value
  if (!targets.length) return
  const hiddenKeys = new Set(
    targets
      .filter((entry) => entry.kind === 'app' || entry.kind === 'site')
      .map((entry) => entry.key),
  )
  const shortcutIDs = new Set(
    targets
      .filter((entry) => entry.kind === 'shortcut' && entry.shortcut)
      .map((entry) => entry.shortcut!.id),
  )
  try {
    await desktopIcons.mutate((draft) => {
      draft.hiddenEntryKeys = [...new Set([...draft.hiddenEntryKeys, ...hiddenKeys])]
      draft.shortcuts = draft.shortcuts.filter((shortcut) => !shortcutIDs.has(shortcut.id))
      for (const id of shortcutIDs) delete draft.positions[`shortcut:${id}`]
    })
    batchRemovingEntries.value = []
    clearIconSelection()
    toast.success(i18n.t('desktop.removedSelectedTitle', { count: targets.length }))
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

async function restoreEntry(entry: DesktopEntry): Promise<void> {
  try {
    await desktopIcons.mutate((draft) => {
      draft.hiddenEntryKeys = draft.hiddenEntryKeys.filter((key) => key !== entry.key)
    })
    toast.success(i18n.t('desktop.restoredToDesktopTitle'), entry.name)
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

function openShortcutDialog(shortcut?: DesktopShortcut): void {
  if (shortcut && shortcut.targetType !== 'url') return
  pendingShortcutID = ''
  editingShortcut.value = shortcut
  shortcutError.value = ''
  shortcutDialogOpen.value = true
  closeContextMenu(false)
}

function closeShortcutDialog(): void {
  if (shortcutSaving.value) return
  shortcutDialogOpen.value = false
  editingShortcut.value = undefined
  shortcutError.value = ''
  pendingShortcutID = ''
}

async function saveShortcut(
  draft: DesktopShortcutDraft,
  icon: File | undefined,
  removeIcon: boolean,
): Promise<void> {
  shortcutSaving.value = true
  shortcutError.value = ''
  const id = draft.id || pendingShortcutID || desktopIcons.generateShortcutID()
  try {
    await desktopIcons.mutate((workspaceDraft) => {
      const next = {
        id,
        name: draft.name,
        description: draft.description,
        targetType: draft.targetType,
        url: draft.url,
      }
      const index = workspaceDraft.shortcuts.findIndex((shortcut) => shortcut.id === id)
      if (index >= 0) workspaceDraft.shortcuts.splice(index, 1, next)
      else workspaceDraft.shortcuts.push(next)
    })
    if (!draft.id) pendingShortcutID = id
    if (removeIcon) await api.desktop.removeShortcutIcon(id)
    if (icon) await api.desktop.uploadShortcutIcon(id, icon)
    if (removeIcon || icon) await desktopIcons.load()
    shortcutDialogOpen.value = false
    editingShortcut.value = undefined
    pendingShortcutID = ''
    toast.success(i18n.t(draft.id ? 'desktop.shortcutUpdated' : 'desktop.shortcutCreated'), draft.name)
  } catch (error) {
    shortcutError.value = workspaceErrorMessage(error)
  } finally {
    shortcutSaving.value = false
  }
}

function requestDeleteShortcut(shortcut?: DesktopShortcut): void {
  const target = shortcut || menuEntry.value?.shortcut
  closeContextMenu()
  if (target) deletingShortcut.value = target
}

async function confirmDeleteShortcut(): Promise<void> {
  const shortcut = deletingShortcut.value
  if (!shortcut) return
  try {
    await desktopIcons.mutate((draft) => {
      draft.shortcuts = draft.shortcuts.filter((item) => item.id !== shortcut.id)
      delete draft.positions[`shortcut:${shortcut.id}`]
    })
    deletingShortcut.value = undefined
    clearIconSelection()
    toast.success(
      i18n.t(shortcut.targetType === 'url' ? 'desktop.shortcutDeleted' : 'desktop.removedFromDesktopTitle'),
      shortcut.name,
    )
  } catch (error) {
    toast.danger(i18n.t('desktop.workspaceSaveErrorTitle'), workspaceErrorMessage(error))
  }
}

function editMenuShortcut(): void {
  const shortcut = menuEntry.value?.shortcut
  closeContextMenu()
  if (shortcut?.targetType === 'url') openShortcutDialog(shortcut)
}

function onTaskbarClick(windowId: number): void {
  const target = desktop.windows.value.find((windowState) => windowState.id === windowId)
  if (!target) return
  if (target.minimized || desktop.focusedId.value !== windowId) {
    desktop.restoreWindow(windowId)
  } else {
    desktop.minimizeWindow(windowId)
  }
}

async function closeTaskbarWindow(): Promise<void> {
  const windowId = menuWindowId.value
  if (windowId === undefined) return
  const windowHandle = desktopWindowRefs.get(windowId)
  closeContextMenu()
  await windowHandle?.requestClose()
}

async function loadEntries(force = false): Promise<void> {
  entriesAbort?.abort()
  entriesAbort = new AbortController()
  const sequence = ++entriesSequence
  entriesLoading.value = true
  try {
    const nextEntries = await loadDesktopEntries(entriesAbort.signal, undefined, force)
    if (sequence === entriesSequence) {
      entries.value = applySiteNames(nextEntries)
      void loadSiteAppearanceNames(nextEntries, entriesAbort.signal, sequence)
    }
  } catch {
    if (sequence === entriesSequence) entries.value = undefined
  } finally {
    if (sequence === entriesSequence) entriesLoading.value = false
  }
}

async function loadWorkspace(): Promise<void> {
  workspaceAbort?.abort()
  workspaceAbort = new AbortController()
  const legacyNames = readSiteNames()
  try {
    const value = await desktopIcons.load(workspaceAbort.signal)
    const persistedNames = Object.fromEntries(
      Object.entries(value.labels)
        .filter(([key]) => key.startsWith('site:'))
        .map(([key, name]) => [key.slice('site:'.length), name]),
    )
    siteNames.value = { ...legacyNames, ...persistedNames }
    entries.value = applySiteNames(entries.value)
    if (!value.available) {
      toast.danger(
        i18n.t('desktop.workspaceUnavailableTitle'),
        i18n.t('desktop.workspaceUnavailableMessage'),
      )
      return
    }

    const legacyEntries = Object.entries(legacyNames)
      .filter(([id]) => !Object.hasOwn(value.labels, `site:${id}`))
    if (legacyEntries.length) {
      await desktopIcons.mutate((draft) => {
        for (const [id, name] of legacyEntries) draft.labels[`site:${id}`] = name
      })
      window.localStorage.removeItem(SITE_RENAMES_KEY)
    }
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return
    toast.danger(i18n.t('desktop.workspaceLoadErrorTitle'), workspaceErrorMessage(error))
  }
}

async function refreshDesktop(): Promise<void> {
  await Promise.allSettled([loadEntries(true), loadWorkspace()])
}

function appearanceEntries(value: DesktopEntries): DesktopEntry[] {
  const unique = new Map<string, DesktopEntry>()
  for (const entry of [...value.sites, ...value.visible]) {
    if (entry.kind === 'site') unique.set(entry.id, entry)
  }
  return [...unique.values()]
}

async function loadSiteAppearanceNames(
  value: DesktopEntries,
  signal: AbortSignal,
  sequence: number,
): Promise<void> {
  const queue = appearanceEntries(value)
  const names: Record<string, string> = {}
  let cursor = 0

  async function worker(): Promise<void> {
    while (!signal.aborted && cursor < queue.length) {
      const entry = queue[cursor]
      cursor += 1
      if (!entry) return
      try {
        const appearance = await api.sites.appearance(entry.id, signal)
        const name = appearance.name?.trim().slice(0, MAX_SITE_NAME_LENGTH)
        if (name) names[entry.id] = name
      } catch {
        // Appearance is optional. The primary domain remains the safe fallback.
      }
    }
  }

  await Promise.all(Array.from({ length: Math.min(4, queue.length) }, () => worker()))
  if (signal.aborted || sequence !== entriesSequence) return
  siteAppearanceNames.value = names
  entries.value = applySiteNames(entries.value)
}

onMounted(() => {
  document.documentElement.classList.add('desktop-mode-open')
  document.body.classList.add('desktop-mode-open')
  window.addEventListener('pointerdown', onGlobalPointerDown)
  window.addEventListener('keydown', onGlobalKeyDown)
  window.addEventListener('resize', onViewportResize)
  void loadEntries()
  void loadWorkspace()
  void loadLocalClusterIdentity()
  void nextTick(() => {
    measureIconWorkArea()
    if (typeof ResizeObserver !== 'undefined' && iconsElement.value) {
      iconsResizeObserver = new ResizeObserver(measureIconWorkArea)
      iconsResizeObserver.observe(iconsElement.value)
    }
  })
})

onBeforeUnmount(() => {
  entriesSequence += 1
  document.documentElement.classList.remove('desktop-mode-open')
  document.body.classList.remove('desktop-mode-open')
  window.removeEventListener('pointerdown', onGlobalPointerDown)
  window.removeEventListener('keydown', onGlobalKeyDown)
  window.removeEventListener('resize', onViewportResize)
  entriesAbort?.abort()
  workspaceAbort?.abort()
  desktopFileMetadataAbort?.abort()
  desktopTransferController?.abort()
  iconsResizeObserver?.disconnect()
  cancelIconDrag()
  cancelSelectionFrame()
  desktopShortcutNativeDrag = undefined
  clearDesktopFileDrag()
  fileDropActive.value = false
  suppressActivationAfterDrag.clear()
  if (bounceTimer !== undefined) window.clearTimeout(bounceTimer)
  if (desktopTransferClearTimer !== undefined) window.clearTimeout(desktopTransferClearTimer)
  if (dropPulseTimer !== undefined) window.clearTimeout(dropPulseTimer)
  if (themeTransitionTimer !== undefined) window.clearTimeout(themeTransitionTimer)
  themeTogglePendingAfterContextMenu = false
  document.documentElement.classList.remove('desktop-theme-transitioning')
  if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame)
  if (resizePersistTimer !== undefined) {
    window.clearTimeout(resizePersistTimer)
    desktop.resizeForViewport({ width: window.innerWidth, height: window.innerHeight })
  }
})

function onViewportResize(): void {
  if (resizeFrame !== undefined) window.cancelAnimationFrame(resizeFrame)
  resizeFrame = window.requestAnimationFrame(() => {
    resizeFrame = undefined
    measureIconWorkArea()
    desktop.resizeForViewport({ width: window.innerWidth, height: window.innerHeight }, false)
  })
  if (resizePersistTimer !== undefined) window.clearTimeout(resizePersistTimer)
  resizePersistTimer = window.setTimeout(() => {
    resizePersistTimer = undefined
    desktop.resizeForViewport({ width: window.innerWidth, height: window.innerHeight })
  }, 180)
}
</script>

<template>
  <div
    ref="desktopElement"
    class="desktop"
    tabindex="-1"
    @pointerdown="onDesktopPointerDown"
    @contextmenu="onContextMenu"
    @dragover="onDesktopFileDragOver"
    @dragleave="onDesktopFileDragLeave"
    @drop="onDesktopFileDrop"
  >
    <div class="desktop__wallpaper" aria-hidden="true">
      <Transition name="desktop-wallpaper-fade">
        <div
          :key="activeDesktopWallpaper.id"
          class="desktop__wallpaper-image"
          :data-wallpaper="activeDesktopWallpaper.id"
          :style="desktopWallpaperStyle"
        />
      </Transition>
      <div class="desktop__aurora desktop__aurora--one" />
      <div class="desktop__aurora desktop__aurora--two" />
      <div class="desktop__aurora desktop__aurora--three" />
    </div>

    <div
      v-if="selectionBox"
      class="desktop__selection-box"
      :style="{
        left: `${selectionBox.left}px`,
        top: `${selectionBox.top}px`,
        width: `${selectionBox.width}px`,
        height: `${selectionBox.height}px`,
      }"
      aria-hidden="true"
    />

    <div
      v-if="fileDropActive"
      class="desktop__file-drop"
      :class="{ 'desktop__file-drop--upload': fileDropMode !== 'shortcut' }"
      role="status"
      aria-live="polite"
    >
      <span>
        <HardDriveUpload v-if="fileDropMode !== 'shortcut'" :size="19" aria-hidden="true" />
        <Plus v-else :size="19" aria-hidden="true" />
      </span>
      <strong>{{ i18n.t(fileDropMode === 'upload'
        ? 'desktop.externalDropTitle'
        : fileDropMode === 'panel-copy'
          ? 'desktop.panelCopyDropTitle'
          : 'desktop.fileDropTitle') }}</strong>
      <small>{{ i18n.t(fileDropMode === 'upload'
        ? 'desktop.externalDropHint'
        : fileDropMode === 'panel-copy'
          ? 'desktop.panelCopyDropHint'
          : 'desktop.fileDropHint') }}</small>
      <code v-if="fileDropMode !== 'shortcut'">{{ desktopUploadDirectory }}</code>
    </div>

    <span
      v-if="dropPulse"
      :key="dropPulse.id"
      class="desktop__drop-pulse"
      :style="{ left: `${dropPulse.left}px`, top: `${dropPulse.top}px` }"
      aria-hidden="true"
    />

    <section
      v-if="desktopTransfer"
      class="desktop-transfer"
      :class="`desktop-transfer--${desktopTransfer.phase}`"
      role="status"
      aria-live="polite"
      :aria-label="i18n.t('desktop.transferTitle')"
    >
      <div class="desktop-transfer__glyph" aria-hidden="true">
        <LoaderCircle v-if="desktopTransferActive" :size="19" />
        <Check v-else-if="desktopTransfer.phase === 'complete'" :size="19" />
        <HardDriveUpload v-else :size="19" />
      </div>
      <div class="desktop-transfer__content">
        <header>
          <strong>
            {{ desktopTransfer.phase === 'preparing'
              ? i18n.t(desktopTransfer.operation === 'panel-copy' ? 'desktop.panelCopyConnecting' : 'desktop.transferPreparing')
              : desktopTransfer.phase === 'uploading'
                ? i18n.t(desktopTransfer.operation === 'panel-copy' ? 'desktop.panelCopyTransferring' : 'desktop.transferUploading', { count: desktopTransfer.roots })
                : desktopTransfer.phase === 'complete'
                  ? i18n.t(desktopTransfer.operation === 'panel-copy' ? 'desktop.panelCopyCompleteTitle' : 'desktop.transferCompleteTitle')
                  : desktopTransfer.phase === 'partial'
                    ? i18n.t('desktop.transferPartialTitle')
                    : desktopTransfer.phase === 'cancelled'
                      ? i18n.t('desktop.transferCancelledTitle')
                      : i18n.t('desktop.transferFailedTitle') }}
          </strong>
          <span v-if="desktopTransferActive">{{ desktopTransferPercent }}%</span>
        </header>
        <div class="desktop-transfer__destination-row">
          <button
            class="desktop-transfer__destination"
            type="button"
            :title="desktopUploadDirectory"
            @click="openDesktopTransferDirectory"
          >
            {{ desktopUploadDirectory }}
          </button>
          <button
            v-if="!desktopTransferActive"
            class="desktop-transfer__change"
            type="button"
            @click="openUploadLocationDialog"
          >
            {{ i18n.t('desktop.transferLocationChange') }}
          </button>
        </div>
        <div class="desktop-transfer__track" aria-hidden="true">
          <span :style="{ width: `${desktopTransferPercent}%` }" />
        </div>
        <small>
          {{ desktopTransfer.detail || (desktopTransfer.currentName
            ? desktopTransfer.currentName
            : i18n.t('desktop.transferReading')) }}
        </small>
      </div>
      <button
        class="desktop-transfer__action"
        type="button"
        :aria-label="i18n.t(desktopTransferActive ? 'desktop.transferCancel' : 'desktop.transferDismiss')"
        @click="desktopTransferActive ? cancelDesktopTransfer() : dismissDesktopTransfer()"
      >
        <X :size="16" aria-hidden="true" />
      </button>
    </section>

    <aside class="desktop__widgets" :aria-label="i18n.t('desktop.toolbarLabel')" @contextmenu.stop>
      <DesktopClock
        :network="entries?.publicNetwork"
        :system-timezone="systemResources?.timezone"
      />
      <DesktopMonitor @snapshot="systemResources = $event" />
    </aside>

    <nav
      ref="iconsElement"
      class="desktop__icons"
      :aria-label="i18n.t('desktop.gridLabel')"
      :aria-busy="entriesLoading"
    >
      <div
        class="desktop__icons-scroll-space"
        :style="{ height: `${iconScrollHeight}px` }"
        aria-hidden="true"
      />
      <p
        v-if="renderedIconLayout.overflowKeys.length"
        class="desktop__icons-overflow-note"
        :style="{ top: `${renderedIconLayout.contentHeight + 8}px` }"
        role="status"
      >
        {{ i18n.t('desktop.iconOverflowNotice', {
          count: renderedIconLayout.overflowKeys.length,
          limit: MAX_DESKTOP_ICON_POSITIONS,
        }) }}
      </p>
      <!-- Static navigation apps -->
      <div
        v-for="(app, index) in desktopApps"
        :key="app.path"
        class="desktop__icon-slot"
        :class="{ 'desktop__icon-slot--dragging': draggingIcons.has(`nav:${app.path}`) }"
        :style="iconSlotStyle(`nav:${app.path}`)"
        :data-icon-key="`nav:${app.path}`"
        @pointerdown="beginIconDrag($event, `nav:${app.path}`)"
        @keydown.capture="clearDraggedActivationSuppression(`nav:${app.path}`)"
        @click.capture="suppressDraggedActivation($event, `nav:${app.path}`)"
        @dblclick.capture="suppressDraggedActivation($event, `nav:${app.path}`)"
      >
        <DesktopEntryIcon
          :label="i18n.t(app.labelKey)"
          :nav-icon="app.icon"
          :gradient="gradientFor(app.path)"
          :active="bouncingIcon === app.path"
          :selected="selectedIcons.has(`nav:${app.path}`)"
          :order="index"
          :dragging="draggingIcons.has(`nav:${app.path}`)"
          @select="(event) => selectNavIcon(app.path, event)"
          @open="openNavIcon(app.path)"
          @context="(event) => onNavContext(event, app.path)"
          @warm="warmNavIcon(app.path)"
          @nudge="(x, y) => nudgeIcon(`nav:${app.path}`, x, y)"
        />
      </div>

      <!-- Dynamic entries: installed apps and sites -->
      <template v-if="entries">
        <div
          v-for="(entry, index) in visibleDynamicEntries"
          :key="entry.key"
          class="desktop__icon-slot"
          :class="{ 'desktop__icon-slot--dragging': draggingIcons.has(entry.key) }"
          :style="iconSlotStyle(entry.key)"
          :data-icon-key="entry.key"
          @pointerdown="beginIconDrag($event, entry.key)"
          @keydown.capture="clearDraggedActivationSuppression(entry.key)"
          @click.capture="suppressDraggedActivation($event, entry.key)"
          @dblclick.capture="suppressDraggedActivation($event, entry.key)"
        >
          <DesktopEntryIcon
            :label="entry.name"
            :entry="entry"
            :gradient="entryGradient(entry)"
            :selected="selectedIcons.has(entry.key)"
            :order="desktopApps.length + index"
            :dragging="draggingIcons.has(entry.key)"
            @select="(event) => selectEntry(entry, event)"
            @open="(event) => onEntryOpen(event, entry)"
            @context="(event) => onEntryContext(event, entry)"
            @nudge="(x, y) => nudgeIcon(entry.key, x, y)"
          />
        </div>
      </template>
      <div
        v-for="(entry, index) in shortcutEntries"
        :key="entry.key"
        class="desktop__icon-slot"
        :class="{
          'desktop__icon-slot--dragging': draggingIcons.has(entry.key),
          'desktop__icon-slot--native-drag-hidden': nativeDragHiddenIcons.has(entry.key),
          'desktop__icon-slot--transferable': isTransferableDesktopShortcut(entry),
          'desktop__icon-slot--transfer-ready': desktopShortcutTransferReady(entry),
        }"
        :style="iconSlotStyle(entry.key)"
        :data-icon-key="entry.key"
        :draggable="!compactIconLayout && isTransferableDesktopShortcut(entry)"
        @pointerdown="beginIconDrag($event, entry.key)"
        @dragstart.stop="startDesktopShortcutDrag($event, entry)"
        @dragend.stop="finishDesktopShortcutDrag($event)"
        @keydown.capture="clearDraggedActivationSuppression(entry.key)"
        @click.capture="suppressDraggedActivation($event, entry.key)"
        @dblclick.capture="suppressDraggedActivation($event, entry.key)"
      >
        <DesktopEntryIcon
          :label="entry.name"
          :entry="entry"
          :gradient="entryGradient(entry)"
          :selected="selectedIcons.has(entry.key)"
          :order="desktopApps.length + visibleDynamicEntries.length + index"
          :dragging="draggingIcons.has(entry.key)"
          :transfer-hint="desktopShortcutTransferHint(entry)"
          :transfer-ready="desktopShortcutTransferReady(entry)"
          @select="(event) => selectEntry(entry, event)"
          @open="(event) => onEntryOpen(event, entry)"
          @context="(event) => onEntryContext(event, entry)"
          @nudge="(x, y) => nudgeIcon(entry.key, x, y)"
        />
      </div>
      <span v-if="entriesLoading" class="desktop__sr-only" aria-live="polite">
        {{ i18n.t('desktop.entriesLoading') }}
      </span>
      <span class="desktop__sr-only" aria-live="polite">{{ iconAnnouncement }}</span>
    </nav>

    <DesktopWindow
      v-for="windowState in openWindows"
      :key="windowState.id"
      :ref="(instance) => setDesktopWindowRef(windowState.id, instance)"
      :window-state="windowState"
      :icon="windowIcon(windowState.path)"
      :icon-url="windowIconURL(windowState.path)"
      :title="windowTitle(windowState.titleKey, windowState.path)"
    />

    <Transition name="desktop-menu" @after-leave="onContextMenuAfterLeave">
      <div
        v-if="contextMenu.open"
        ref="contextMenuElement"
        class="desktop__context-menu"
        :class="{ 'desktop__context-menu--entry': menuEntry || menuNavPath || menuSelectionKeys.length > 1 }"
        :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
        role="menu"
        @contextmenu.prevent.stop
        @pointerdown.stop
        @keydown="onContextMenuKeyDown"
      >
        <template v-if="menuSelectionKeys.length > 1">
          <button
            type="button"
            role="menuitem"
            class="desktop__context-danger"
            :disabled="!menuRemovableCount || !workspace.available || desktopIcons.saving.value"
            @click="requestBatchRemoveSelected(menuSelectionKeys)"
          >
            <EyeOff :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.removeFromDesktop') }}
          </button>
          <button type="button" role="menuitem" @click="closeContextMenu(); clearIconSelection()">
            <X :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.clearSelection') }}
          </button>
        </template>
        <template v-else-if="menuEntry">
          <button type="button" role="menuitem" @click="onEntryMenuOpen">
            <SquareTerminal v-if="menuEntry.launch === 'script'" :size="15" aria-hidden="true" />
            <FolderOpen v-else-if="menuEntry.launch === 'directory'" :size="15" aria-hidden="true" />
            <File v-else-if="menuEntry.launch === 'file'" :size="15" aria-hidden="true" />
            <AppWindow v-else-if="menuEntry.url" :size="15" aria-hidden="true" />
            <ExternalLink v-else :size="15" aria-hidden="true" />
            {{ menuEntry.launch === 'script'
              ? i18n.t('desktop.entryScriptManage')
              : menuEntry.url
                ? i18n.t('desktop.systemBrowserOpen')
                : i18n.t('desktop.entryOpen') }}
          </button>
          <button type="button" role="menuitem" @click="onEntryMenuDetails">
            <Info :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.entryDetails') }}
          </button>
          <button
            v-if="menuEntry.kind === 'site'"
            type="button"
            role="menuitem"
            @click="onEntryMenuRename"
          >
            <Pencil :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.entryRename') }}
          </button>
          <button
            v-if="menuEntry.kind === 'shortcut' && menuEntry.shortcut?.targetType === 'url'"
            type="button"
            role="menuitem"
            @click="editMenuShortcut"
          >
            <Pencil :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.shortcutEdit') }}
          </button>
          <div class="desktop__context-separator" role="separator" />
          <button
            v-if="menuEntry.kind === 'app' || menuEntry.kind === 'site'"
            type="button"
            role="menuitem"
            class="desktop__context-danger"
            :disabled="!workspace.available || desktopIcons.saving.value"
            @click="requestRemoveEntry"
          >
            <EyeOff :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.removeFromDesktop') }}
          </button>
          <button
            v-else
            type="button"
            role="menuitem"
            class="desktop__context-danger"
            :disabled="!workspace.available || desktopIcons.saving.value"
            @click="requestDeleteShortcut()"
          >
            <Trash2 :size="15" aria-hidden="true" />
            {{ menuEntry.shortcut?.targetType === 'file' || menuEntry.shortcut?.targetType === 'directory'
              ? i18n.t('desktop.removeFromDesktop')
              : i18n.t('desktop.shortcutDelete') }}
          </button>
        </template>
        <template v-else-if="menuNavPath">
          <button type="button" role="menuitem" @click="onNavMenuOpen">
            <AppWindow :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.entryOpen') }}
          </button>
        </template>
        <template v-else-if="contextMenuTarget === 'taskbar'">
          <button
            type="button"
            role="menuitem"
            data-context-action="processes"
            @click="onContextMenuAction('processes')"
          >
            <ListTree :size="15" aria-hidden="true" />
            {{ i18n.t('route.processes') }}
          </button>
        </template>
        <template v-else-if="contextMenuTarget === 'taskbar-window'">
          <button
            type="button"
            role="menuitem"
            data-context-action="close-window"
            @click="closeTaskbarWindow"
          >
            <X :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.closeWindow') }}
          </button>
        </template>
        <template v-else>
          <button type="button" role="menuitem" @click="onContextMenuAction('refresh')">
            <RefreshCw :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.menuRefresh') }}
          </button>
          <button
            type="button"
            role="menuitem"
            :disabled="!workspace.available || desktopIcons.saving.value"
            @click="onContextMenuAction('add-shortcut')"
          >
            <Plus :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.shortcutAdd') }}
          </button>
          <button
            type="button"
            role="menuitem"
            :disabled="!workspace.available"
            @click="onContextMenuAction('manage-icons')"
          >
            <MonitorCog :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.iconManagerTitle') }}
          </button>
          <div class="desktop__context-separator" role="separator" />
          <button type="button" role="menuitem" data-context-action="theme" @click="onContextMenuAction('theme')">
            <Sun v-if="theme.resolved.value === 'dark'" :size="15" aria-hidden="true" />
            <Moon v-else :size="15" aria-hidden="true" />
            {{ theme.resolved.value === 'dark' ? i18n.t('desktop.menuLight') : i18n.t('desktop.menuDark') }}
          </button>
          <button
            type="button"
            role="menuitem"
            data-context-action="wallpaper"
            @click="onContextMenuAction('wallpaper')"
          >
            <ImageIcon :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.changeWallpaper') }}
          </button>
          <button
            type="button"
            role="menuitem"
            data-context-action="fullscreen"
            @click="onContextMenuAction('fullscreen')"
          >
            <Minimize2 v-if="documentFullscreen.active.value" :size="15" aria-hidden="true" />
            <Maximize2 v-else :size="15" aria-hidden="true" />
            {{ documentFullscreen.active.value
              ? i18n.t('desktop.exitFullscreen')
              : i18n.t('desktop.enterFullscreen') }}
          </button>
          <button type="button" role="menuitem" @click="onContextMenuAction('about')">
            <Info :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.menuAbout') }}
          </button>
          <div class="desktop__context-separator" role="separator" />
          <button type="button" role="menuitem" @click="onContextMenuAction('classic')">
            <ArrowLeft :size="15" aria-hidden="true" />
            {{ i18n.t('desktop.switchClassic') }}
          </button>
        </template>
      </div>
    </Transition>

    <Transition name="desktop-menu">
      <div
        v-if="selectedIconCount > 1"
        class="desktop__selection-actions"
        role="toolbar"
        :aria-label="i18n.t('desktop.selectionActionsLabel')"
        @pointerdown.stop
        @contextmenu.prevent.stop
      >
        <strong>{{ i18n.t('desktop.selectedCount', { count: selectedIconCount }) }}</strong>
        <button
          type="button"
          :disabled="!removableSelectedCount || !workspace.available || desktopIcons.saving.value"
          @click="requestBatchRemoveSelected()"
        >
          <EyeOff :size="15" aria-hidden="true" />
          {{ i18n.t('desktop.removeFromDesktop') }}
        </button>
        <button type="button" @click="clearIconSelection">
          <X :size="15" aria-hidden="true" />
          {{ i18n.t('desktop.clearSelection') }}
        </button>
      </div>
    </Transition>

    <footer
      class="desktop__taskbar"
      role="toolbar"
      :aria-label="i18n.t('desktop.taskbarLabel')"
      @contextmenu.prevent.stop="onTaskbarContext"
    >
      <div class="desktop__taskbar-brand" aria-label="KPanel">
        <LogoMark compact />
        <span>KPanel</span>
        <div v-if="props.agent" class="desktop__taskbar-agent">
          <span class="desktop__taskbar-agent-status" :class="`desktop__taskbar-agent-status--${agentStatus.state}`">
            <i aria-hidden="true" />
            <span>{{ agentStatus.label }}</span>
          </span>
          <button
            v-if="props.kpanelUpdateAvailable"
            class="desktop__taskbar-agent-update"
            type="button"
            :aria-label="props.kpanelUpdateDescription"
            :title="props.kpanelUpdateDescription"
            @click="openKPanelUpdate"
          >
            <CircleArrowUp :size="13" aria-hidden="true" />
            <span>{{ i18n.t('nav.updateAvailable') }}</span>
          </button>
          <small v-else-if="props.agent.version">v{{ props.agent.version }}</small>
        </div>
      </div>
      <div class="desktop__taskbar-apps">
        <button
          v-for="windowState in openWindows"
          :key="windowState.id"
          class="desktop__taskbar-item"
          :class="{
            'desktop__taskbar-item--active': windowState.id === focusedWindow?.id,
            'desktop__taskbar-item--minimized': windowState.minimized,
          }"
          type="button"
          :data-window-id="windowState.id"
          :aria-label="windowTitle(windowState.titleKey, windowState.path)"
          :title="windowTitle(windowState.titleKey, windowState.path)"
          :aria-pressed="windowState.id === focusedWindow?.id"
          @click="onTaskbarClick(windowState.id)"
          @contextmenu.stop="onTaskbarItemContext($event, windowState.id)"
        >
          <span
            class="desktop__taskbar-glyph"
            :style="{ background: gradientFor(windowState.path) }"
          >
            <component
              :is="findDesktopApp(windowState.path)?.icon || AppWindow"
              :size="19"
              :stroke-width="1.9"
              aria-hidden="true"
            />
          </span>
          <span class="desktop__taskbar-label">{{ windowTitle(windowState.titleKey, windowState.path) }}</span>
          <i aria-hidden="true" />
        </button>
      </div>
      <div class="desktop__system-tray">
        <button
          class="desktop__tray-button"
          type="button"
          :title="theme.resolved.value === 'dark' ? i18n.t('desktop.menuLight') : i18n.t('desktop.menuDark')"
          :aria-label="theme.resolved.value === 'dark' ? i18n.t('desktop.menuLight') : i18n.t('desktop.menuDark')"
          @click="toggleDesktopTheme"
        >
          <Sun v-if="theme.resolved.value === 'dark'" :size="16" aria-hidden="true" />
          <Moon v-else :size="16" aria-hidden="true" />
        </button>
        <button
          class="desktop__classic-button"
          type="button"
          :title="i18n.t('desktop.switchClassic')"
          :aria-label="i18n.t('desktop.switchClassic')"
          @click="enterClassicSafely"
        >
          <ArrowLeft :size="15" aria-hidden="true" />
          <span>{{ i18n.t('desktop.switchClassic') }}</span>
        </button>
      </div>
    </footer>

    <ModalDialog
      :open="wallpaperDialogOpen"
      :title="i18n.t('desktop.wallpaperTitle')"
      :description="i18n.t('desktop.wallpaperDescription')"
      size="wide"
      @close="wallpaperDialogOpen = false"
    >
      <div
        class="desktop-wallpaper-picker"
        role="radiogroup"
        :aria-label="i18n.t('desktop.wallpaperTitle')"
      >
        <button
          v-for="wallpaper in DESKTOP_WALLPAPERS"
          :key="wallpaper.id"
          class="desktop-wallpaper-picker__option"
          :class="{ 'desktop-wallpaper-picker__option--selected': wallpaper.id === desktopWallpaperID }"
          type="button"
          role="radio"
          :aria-checked="wallpaper.id === desktopWallpaperID"
          :data-wallpaper-option="wallpaper.id"
          @click="selectDesktopWallpaper(wallpaper.id)"
        >
          <span
            class="desktop-wallpaper-picker__preview"
            :style="{ backgroundImage: `url('${wallpaper.src}')` }"
            aria-hidden="true"
          />
          <span class="desktop-wallpaper-picker__copy">
            <strong>{{ i18n.t(wallpaper.nameKey) }}</strong>
            <small>{{ i18n.t(wallpaper.descriptionKey) }}</small>
          </span>
          <Check
            v-if="wallpaper.id === desktopWallpaperID"
            class="desktop-wallpaper-picker__check"
            :size="17"
            aria-hidden="true"
          />
        </button>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(externalOpenEntry)"
      :title="i18n.t('desktop.externalOpenConfirmTitle')"
      size="compact"
      @close="closeExternalOpen"
    >
      <div v-if="externalOpenEntry" class="desktop__external-confirm">
        <div class="desktop__external-confirm-entry">
          <span
            class="desktop__external-confirm-icon"
            :style="{ background: entryGradient(externalOpenEntry) }"
            aria-hidden="true"
          >
            <img
              v-if="externalOpenEntry.iconURL && !externalOpenImageFailed"
              class="desktop__external-confirm-icon-image"
              :src="externalOpenEntry.iconURL"
              alt=""
              decoding="async"
              referrerpolicy="no-referrer"
              width="64"
              height="64"
              @error="externalOpenImageFailed = true"
            />
            <span v-else-if="externalOpenEntry.kind === 'site'" class="desktop__site-fallback">
              <span class="desktop__site-fallback-letter">{{ externalOpenMonogram }}</span>
              <span class="desktop__site-fallback-badge">
                <Globe2 :size="11" :stroke-width="2.2" />
              </span>
            </span>
            <span v-else class="desktop__icon-monogram">{{ externalOpenMonogram }}</span>
          </span>
          <div class="desktop__external-confirm-identity">
            <strong>{{ externalOpenEntry.name }}</strong>
            <code>{{ externalOpenEntry.url }}</code>
          </div>
        </div>
      </div>
      <template #footer>
        <button class="button button--ghost" type="button" @click="closeExternalOpen">
          {{ i18n.t('common.cancel') }}
        </button>
        <button class="button button--primary" type="button" @click="confirmExternalOpen">
          <ExternalLink :size="15" aria-hidden="true" />
          {{ i18n.t('desktop.systemBrowserOpen') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(detailEntry)"
      :title="detailEntry?.name || ''"
      size="small"
      @close="detailEntry = undefined"
    >
      <dl v-if="detailEntry" class="desktop__detail">
        <template v-if="detailEntry.kind === 'app'">
          <dt>{{ i18n.t('desktop.detailType') }}</dt>
          <dd>{{ i18n.t('desktop.detailApp') }}</dd>
          <dt>{{ i18n.t('desktop.detailStatus') }}</dt>
          <dd>{{ detailEntry.app?.runtime.state || i18n.t('desktop.detailUnknown') }}</dd>
          <dt>{{ i18n.t('desktop.detailURL') }}</dt>
          <dd class="desktop__detail-url">{{ detailEntry.url }}</dd>
        </template>
        <template v-else-if="detailEntry.kind === 'site'">
          <dt>{{ i18n.t('desktop.detailType') }}</dt>
          <dd>{{ i18n.t('desktop.detailSite') }}</dd>
          <dt>{{ i18n.t('desktop.detailDomain') }}</dt>
          <dd>{{ detailEntry.site?.primaryDomain }}</dd>
          <dt>{{ i18n.t('desktop.detailType2') }}</dt>
          <dd>{{ detailEntry.site?.type }}</dd>
          <dt>{{ i18n.t('desktop.detailURL') }}</dt>
          <dd class="desktop__detail-url">{{ detailEntry.url }}</dd>
        </template>
        <template v-else>
          <dt>{{ i18n.t('desktop.detailType') }}</dt>
          <dd>{{ detailEntry.launch === 'directory'
            ? i18n.t('desktop.detailDirectoryShortcut')
            : detailEntry.launch === 'file'
              ? i18n.t('desktop.detailFileShortcut')
              : i18n.t('desktop.detailShortcut') }}</dd>
          <dt>{{ i18n.t('desktop.detailDescription') }}</dt>
          <dd>{{ detailEntry.description || i18n.t('desktop.detailNoDescription') }}</dd>
          <dt>{{ detailEntry.path ? i18n.t('desktop.detailPath') : i18n.t('desktop.detailURL') }}</dt>
          <dd class="desktop__detail-url">{{ detailEntry.path || detailEntry.url }}</dd>
        </template>
      </dl>
      <template #footer>
        <button class="button button--primary" type="button" @click="onDetailEntryOpen">
          <FolderOpen v-if="detailEntry?.launch === 'directory'" :size="15" aria-hidden="true" />
          <File v-else-if="detailEntry?.launch === 'file'" :size="15" aria-hidden="true" />
          <ExternalLink v-else :size="15" aria-hidden="true" />
          {{ i18n.t('desktop.entryOpen') }}
        </button>
        <button class="button button--ghost" type="button" @click="detailEntry = undefined">
          {{ i18n.t('common.closeDialog') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(renameEntry)"
      :title="i18n.t('desktop.renameTitle')"
      size="small"
      @close="closeRename"
    >
      <form class="desktop__rename-form" @submit.prevent="saveRename">
        <label>
          <span>{{ i18n.t('desktop.renameLabel') }}</span>
          <input
            v-model="renameValue"
            :maxlength="MAX_SITE_NAME_LENGTH"
            :placeholder="renameEntry ? defaultSiteName(renameEntry) : ''"
            autocomplete="off"
          />
        </label>
        <small>{{ renameEntry?.site?.primaryDomain }}</small>
      </form>
      <template #footer>
        <button
          v-if="renameEntry && siteNames[renameEntry.id]"
          class="button button--ghost"
          type="button"
          @click="resetRename"
        >
          {{ i18n.t('desktop.renameReset') }}
        </button>
        <button class="button button--ghost" type="button" @click="closeRename">
          {{ i18n.t('common.cancel') }}
        </button>
        <button class="button button--primary" type="button" :disabled="!renameValue.trim()" @click="saveRename">
          {{ i18n.t('desktop.renameSave') }}
        </button>
      </template>
    </ModalDialog>

    <DesktopIconManagerDialog
      :open="iconManagerOpen"
      :hidden-entries="hiddenEntries"
      :shortcuts="shortcuts"
      :busy="desktopIcons.saving.value"
      :can-auto-arrange="!compactIconLayout && workspace.available"
      @close="iconManagerOpen = false"
      @add="openShortcutDialog()"
      @auto-arrange="autoArrangeIcons"
      @edit="openShortcutDialog"
      @remove="requestDeleteShortcut"
      @restore="restoreEntry"
    />

    <DesktopShortcutDialog
      :open="shortcutDialogOpen"
      :shortcut="editingShortcut"
      :saving="shortcutSaving"
      :error-message="shortcutError"
      @close="closeShortcutDialog"
      @save="saveShortcut"
    />

    <ModalDialog
      :open="uploadLocationOpen"
      :title="i18n.t('desktop.transferLocationTitle')"
      size="small"
      @close="closeUploadLocationDialog"
    >
      <form class="desktop__upload-location-form" @submit.prevent="saveUploadLocation">
        <label>
          <span>{{ i18n.t('desktop.transferLocationLabel') }}</span>
          <input
            v-model="uploadLocationDraft"
            type="text"
            maxlength="4096"
            autocomplete="off"
            spellcheck="false"
            :placeholder="DESKTOP_UPLOAD_DIRECTORY"
            :aria-invalid="Boolean(uploadLocationError)"
          />
        </label>
        <p>{{ i18n.t('desktop.transferLocationHint') }}</p>
        <small v-if="uploadLocationError" role="alert">{{ uploadLocationError }}</small>
      </form>
      <template #footer>
        <button class="button button--ghost" type="button" :disabled="uploadLocationSaving" @click="closeUploadLocationDialog">
          {{ i18n.t('common.cancel') }}
        </button>
        <button class="button button--primary" type="button" :disabled="uploadLocationSaving" @click="saveUploadLocation">
          {{ uploadLocationSaving ? i18n.t('common.saving') : i18n.t('desktop.transferLocationSave') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(removingEntry)"
      :title="i18n.t('desktop.removeFromDesktopTitle')"
      size="compact"
      @close="removingEntry = undefined"
    >
      <div v-if="removingEntry" class="desktop__confirm-copy">
        <strong>{{ removingEntry.name }}</strong>
        <p>{{ removingEntry.kind === 'app'
          ? i18n.t('desktop.removeAppFromDesktopMessage')
          : i18n.t('desktop.removeSiteFromDesktopMessage') }}</p>
      </div>
      <template #footer>
        <button class="button button--ghost" type="button" @click="removingEntry = undefined">
          {{ i18n.t('common.cancel') }}
        </button>
        <button
          class="button button--primary"
          type="button"
          :disabled="desktopIcons.saving.value"
          @click="confirmRemoveEntry"
        >
          {{ i18n.t('desktop.removeFromDesktop') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(deletingShortcut)"
      :title="i18n.t(deletingFileShortcut ? 'desktop.fileShortcutRemoveTitle' : 'desktop.shortcutDeleteTitle')"
      size="compact"
      @close="deletingShortcut = undefined"
    >
      <div v-if="deletingShortcut" class="desktop__confirm-copy">
        <strong>{{ deletingShortcut.name }}</strong>
        <p>{{ i18n.t(deletingFileShortcut ? 'desktop.fileShortcutRemoveMessage' : 'desktop.shortcutDeleteMessage') }}</p>
      </div>
      <template #footer>
        <button class="button button--ghost" type="button" @click="deletingShortcut = undefined">
          {{ i18n.t('common.cancel') }}
        </button>
        <button
          :class="deletingFileShortcut ? 'button button--primary' : 'button button--danger'"
          type="button"
          :disabled="desktopIcons.saving.value"
          @click="confirmDeleteShortcut"
        >
          {{ i18n.t(deletingFileShortcut ? 'desktop.removeFromDesktop' : 'desktop.shortcutDelete') }}
        </button>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="batchRemovingEntries.length > 0"
      :title="i18n.t('desktop.removeSelectedTitle', { count: batchRemovingEntries.length })"
      size="compact"
      @close="batchRemovingEntries = []"
    >
      <div class="desktop__confirm-copy">
        <strong>{{ i18n.t('desktop.selectedCount', { count: batchRemovingEntries.length }) }}</strong>
        <p>{{ i18n.t('desktop.removeSelectedMessage') }}</p>
      </div>
      <template #footer>
        <button class="button button--ghost" type="button" @click="batchRemovingEntries = []">
          {{ i18n.t('common.cancel') }}
        </button>
        <button
          class="button button--primary"
          type="button"
          :disabled="desktopIcons.saving.value"
          @click="confirmBatchRemoveSelected"
        >
          {{ i18n.t('desktop.removeSelected', { count: batchRemovingEntries.length }) }}
        </button>
      </template>
    </ModalDialog>
  </div>
</template>
