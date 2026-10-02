<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/GalleryView/en-US').then((module) => module.default)
  : import('@/i18n/pages/GalleryView/zh-TW').then((module) => module.default))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}
import {
  ArrowUpDown,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Download,
  Film,
  FolderOpen,
  FolderPlus,
  Grid2x2,
  Grid3x3,
  Images,
  LayoutGrid,
  ListChecks,
  MapPin,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import HostSwitcher from '@/components/common/HostSwitcher.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import GalleryTile from '@/components/gallery/GalleryTile.vue'
import GalleryViewer, { type GalleryViewerSources } from '@/components/gallery/GalleryViewer.vue'
import { getLocale, useI18n } from '@/i18n'
import { ApiError, api } from '@/lib/api'
import { applyClusterHostOrderPreference, readClusterHostOrder, sortClusterHosts } from '@/lib/clusterHostOrder'
import { clusterHostPanelPageURL } from '@/lib/clusterHostNavigation'
import { fileHostStatus } from '@/lib/fileHostStatus'
import type { HostSwitcherStatus } from '@/lib/hostSwitcher'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { downloadFileEntries } from '@/lib/fileDownloads'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { notifyFileDirectoriesChanged, subscribeFileDirectoryChanges } from '@/lib/fileWindowTransfer'
import { formatBytes } from '@/lib/format'
import {
  browserStorage,
  filterGalleryItems,
  formatGalleryMonth,
  galleryAlbumNameProblem,
  galleryBaseName,
  galleryBrowserCanShow,
  galleryParentPath,
  galleryPathTrail,
  galleryTileSource,
  GALLERY_DEFAULT_ROOT,
  GALLERY_ORIGINAL_FALLBACK_MAX_BYTES,
  GALLERY_UPLOAD_ACCEPT,
  groupGalleryItemsByMonth,
  isGalleryUploadCandidate,
  isWithinGalleryRoot,
  joinGalleryPath,
  nextGalleryUploadName,
  normalizeGalleryRoot,
  readGalleryPreferences,
  sortGalleryItems,
  writeGalleryPreferences,
  type GalleryDensity,
  type GalleryFilter,
  type GalleryItem,
} from '@/lib/gallery'
import { loadGalleryFolder, type GalleryAlbum, type GalleryFolderSnapshot } from '@/lib/galleryLibrary'
import { galleryPosterKey, readGalleryPoster } from '@/lib/galleryPosters'
import { useToast } from '@/stores/toast'
import type { ClusterHost, FileEntry } from '@/types/api'

type UploadPhase = 'queued' | 'running' | 'done' | 'error' | 'cancelled'

interface UploadTask {
  id: number
  file: File
  name: string
  target: string
  hostId: string
  progress: number
  phase: UploadPhase
  detail?: string
  controller?: AbortController
}

type AlbumDialog =
  | { mode: 'create'; parent: string }
  | { mode: 'rename'; album: GalleryAlbum }

type DeleteDialog =
  | { kind: 'media'; entries: FileEntry[] }
  | { kind: 'album'; album: GalleryAlbum }

const RENDER_STEP = 240
const UPLOAD_CONCURRENCY = 2
const ALBUM_PREVIEW_COUNT = 11
const RELOAD_DEBOUNCE_MS = 450

const route = useRoute()
const router = useRouter()
const toast = useToast()
const i18n = useI18n()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))

const preferences = reactive(readGalleryPreferences(browserStorage()))
watch(preferences, (value) => writeGalleryPreferences(browserStorage(), { ...value }))

const page = ref<HTMLElement>()
const sentinel = ref<HTMLElement>()
const scroller = ref<HTMLElement>()
const uploadInput = ref<HTMLInputElement>()
const windowed = ref(false)

const hostId = computed(() => typeof route.query.hostId === 'string' ? route.query.hostId : '')
const fileAPI = computed(() => fileAPIForHost(hostId.value))
const currentPath = computed(() => {
  const requested = typeof route.query.path === 'string' ? normalizeGalleryRoot(route.query.path) : undefined
  return requested || preferences.root
})
// The library folder is a path, so it names the same place on every host: opening the
// gallery from a remote file pane starts at that host's own copy of it.
const isLibraryRoot = computed(() => currentPath.value === preferences.root)
const insideLibrary = computed(() => isWithinGalleryRoot(preferences.root, currentPath.value))
const trail = computed(() => galleryPathTrail(insideLibrary.value ? preferences.root : '/', currentPath.value))

// Hosts the gallery can switch to; also gives a remote host its name instead of its opaque id.
const hosts = ref<ClusterHost[]>([])
const hostsLoading = ref(false)
const hostsError = ref(false)
const hostSwitcher = ref<InstanceType<typeof HostSwitcher>>()
const hostName = computed(() => hosts.value.find((host) => host.id === hostId.value)?.name ?? '')
const orderedHosts = computed(() => sortClusterHosts(hosts.value, readClusterHostOrder()))
const activeHostId = computed(() => hostId.value || hosts.value.find((host) => host.isLocal)?.id || '')
const activeHostLabel = computed(() => hostId.value ? hostName.value || hostId.value : phrase('本机'))
// A single-panel install has nothing to switch to, so the picker stays out of the way.
const showHostSwitcher = computed(() => hosts.value.length > 1 || Boolean(hostId.value))

const snapshot = shallowRef<GalleryFolderSnapshot>()
const loading = ref(true)
const loadError = ref('')
let loadController: AbortController | undefined
let loadSequence = 0
let reloadTimer: number | undefined
let unmounted = false

const filter = ref<GalleryFilter>('all')
const search = ref('')
const renderLimit = ref(RENDER_STEP)
const albumsExpanded = ref(false)
const albumMenu = ref<string>()
const moreMenuOpen = ref(false)

const selecting = ref(false)
const selected = ref(new Set<string>())
let selectionAnchor = ''

const viewerPath = ref<string>()
const dragging = ref(false)
let dragDepth = 0

const uploads = ref<UploadTask[]>([])
let uploadSequence = 0
let uploadWorkers = 0
const uploadsCollapsed = ref(false)
let uploadClearTimer: number | undefined

const albumDialog = ref<AlbumDialog>()
const albumName = ref('')
const albumBusy = ref(false)
const deleteDialog = ref<DeleteDialog>()
const deleteBusy = ref(false)
const locationDialogOpen = ref(false)
const locationValue = ref('')

const allItems = computed(() => snapshot.value?.items ?? [])
// Name order while albums are still being counted, newest first once they all are,
// so cards do not jump each time one album finishes.
const albums = computed(() => {
  const list = [...(snapshot.value?.albums ?? [])]
  if (snapshot.value?.scanning) return list
  return list.sort((left, right) => (
    right.latest - left.latest || left.name.localeCompare(right.name, undefined, { numeric: true })
  ))
})
const visibleAlbums = computed(() => albumsExpanded.value ? albums.value : albums.value.slice(0, ALBUM_PREVIEW_COUNT))
const imageCount = computed(() => allItems.value.filter((item) => item.kind === 'image').length)
const videoCount = computed(() => allItems.value.filter((item) => item.kind === 'video').length)
const totalBytes = computed(() => allItems.value.reduce((total, item) => total + item.entry.sizeBytes, 0))
const visibleItems = computed(() => sortGalleryItems(
  filterGalleryItems(allItems.value, filter.value, search.value),
  preferences.sort,
))
const renderedItems = computed(() => visibleItems.value.slice(0, renderLimit.value))
const groups = computed(() => groupGalleryItemsByMonth(renderedItems.value, preferences.sort))
const locale = computed(() => getLocale())
const viewerIndex = computed(() => {
  if (!viewerPath.value) return -1
  return visibleItems.value.findIndex((item) => item.entry.path === viewerPath.value)
})
const selectedEntries = computed(() => allItems.value.filter((item) => selected.value.has(item.entry.path)).map((item) => item.entry))
const title = computed(() => {
  if (isLibraryRoot.value) return phrase('图库')
  return galleryBaseName(currentPath.value)
})
// The cover spans the page, so prefer the newest photo whose original is small
// enough to load; a 320px Agent thumbnail is only the last resort.
const heroItem = computed(() => {
  let sharp: GalleryItem | undefined
  let any: GalleryItem | undefined
  for (const item of allItems.value) {
    if (item.kind !== 'image' || !galleryBrowserCanShow(item)) continue
    const source = galleryTileSource(item, true).type
    if (source === 'none') continue
    if (source === 'original' && (!sharp || item.time > sharp.time)) sharp = item
    if (!any || item.time > any.time) any = item
  }
  return sharp ?? any
})
const heroUrl = computed(() => heroItem.value ? tileUrls(heroItem.value, true).image : undefined)
const summary = computed(() => {
  const parts = [`${imageCount.value} ${phrase('张照片')}`, `${videoCount.value} ${phrase('段视频')}`]
  if (albums.value.length) parts.push(`${albums.value.length} ${phrase('个相册')}`)
  if (totalBytes.value) parts.push(formatBytes(totalBytes.value))
  return parts.join(' · ')
})
const filterOptions = computed<Array<{ value: GalleryFilter; label: string; count: number }>>(() => [
  { value: 'all', label: '全部', count: allItems.value.length },
  { value: 'image', label: '照片', count: imageCount.value },
  { value: 'video', label: '视频', count: videoCount.value },
])
const densityOptions: Array<{ value: GalleryDensity; label: string; icon: typeof Grid3x3 }> = [
  { value: 'compact', label: '紧凑', icon: Grid3x3 },
  { value: 'comfortable', label: '标准', icon: LayoutGrid },
  { value: 'spacious', label: '舒展', icon: Grid2x2 },
]
const activeUploads = computed(() => uploads.value.filter((task) => task.phase === 'queued' || task.phase === 'running'))
const finishedUploads = computed(() => uploads.value.filter((task) => task.phase === 'done').length)
const failedUploads = computed(() => uploads.value.filter((task) => task.phase === 'error').length)
const uploadProgress = computed(() => {
  const tracked = uploads.value.filter((task) => task.phase !== 'cancelled')
  const bytes = tracked.reduce((total, task) => total + task.file.size, 0)
  if (!bytes) return 0
  const done = tracked.reduce((total, task) => total + (task.phase === 'done' ? task.file.size : (task.file.size * task.progress) / 100), 0)
  return Math.round((done / bytes) * 100)
})
const albumNameProblem = computed(() => {
  const dialog = albumDialog.value
  if (!dialog) return undefined
  const siblings = (snapshot.value?.albums ?? [])
    .filter((album) => dialog.mode === 'create' || album.path !== dialog.album.path)
    .map((album) => album.name)
  return galleryAlbumNameProblem(albumName.value, siblings)
})
const albumSubmitLabel = computed(() => {
  if (albumBusy.value) return '正在保存…'
  return albumDialog.value?.mode === 'rename' ? '重命名' : '创建相册'
})
const locationProblem = computed(() => normalizeGalleryRoot(locationValue.value) ? '' : '请输入绝对路径，例如 /home/gallery；不能使用根目录 /')
const emptyLibrary = computed(() => Boolean(snapshot.value?.exists && !allItems.value.length && !albums.value.length))

function albumMeta(album: GalleryAlbum): string {
  if (album.failed) return phrase('暂时无法读取')
  if (!album.scanned) return phrase('正在统计…')
  const parts: string[] = []
  if (album.imageCount) parts.push(`${album.imageCount} ${phrase('张照片')}`)
  if (album.videoCount) parts.push(`${album.videoCount} ${phrase('段视频')}`)
  if (album.folderCount) parts.push(`${album.folderCount} ${phrase('个子相册')}`)
  return parts.length ? parts.join(' · ') : phrase('空相册')
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError || error instanceof Error) return error.message
  return phrase('操作未完成，请稍后重试。')
}

function tileUrls(item: GalleryItem, large = false): { image?: string; fallback?: string; video?: string } {
  const source = galleryTileSource(item, large)
  const api = fileAPI.value
  if (source.type === 'video') return { video: api.contentUrl(item.entry.path, 'inline') }
  if (source.type === 'thumbnail') {
    return {
      image: api.thumbnailUrl(item.entry.path, item.entry.resourceVersion),
      // Some JPEGs (CMYK, unusual sampling) defeat the Agent decoder; the original still renders.
      fallback: item.entry.sizeBytes <= GALLERY_ORIGINAL_FALLBACK_MAX_BYTES ? api.contentUrl(item.entry.path, 'inline') : undefined,
    }
  }
  if (source.type === 'original') return { image: api.contentUrl(item.entry.path, 'inline') }
  return {}
}

function viewerSources(item: GalleryItem): GalleryViewerSources {
  const api = fileAPI.value
  const tile = galleryTileSource(item)
  let preview: string | undefined
  if (tile.type === 'thumbnail') preview = api.thumbnailUrl(item.entry.path, item.entry.resourceVersion)
  else if (item.kind === 'video') preview = readGalleryPoster(galleryPosterKey(item.entry, api.contentUrl(item.entry.path, 'inline')))?.poster
  return {
    preview,
    original: api.contentUrl(item.entry.path, 'inline'),
  }
}

function galleryQuery(path: string): Record<string, string> {
  const query: Record<string, string> = {}
  if (path !== preferences.root || hostId.value) query.path = path
  if (hostId.value) query.hostId = hostId.value
  return query
}

function openFolder(path: string): void {
  void router.push({ name: 'gallery', query: galleryQuery(path) })
}

function revealInFiles(path: string): void {
  void router.push({ name: 'files', query: { path, ...(hostId.value ? { hostId: hostId.value } : {}) } })
}

async function load(options: { quiet?: boolean } = {}): Promise<void> {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  const sequence = ++loadSequence
  const path = currentPath.value
  const api = fileAPI.value
  if (!options.quiet) {
    loading.value = true
    if (snapshot.value?.path !== path) snapshot.value = undefined
  }
  loadError.value = ''
  try {
    await loadGalleryFolder({ list: api.list }, path, (next) => {
      if (sequence !== loadSequence || unmounted) return
      // A refresh keeps the current page until the whole scan is in, so album
      // media do not blink out and back while each album is re-read.
      if (options.quiet && next.scanning) return
      snapshot.value = next
      loading.value = false
    }, controller.signal)
  } catch (error) {
    if (sequence !== loadSequence || unmounted || controller.signal.aborted) return
    loadError.value = errorMessage(error)
  } finally {
    if (sequence === loadSequence) {
      loading.value = false
      if (loadController === controller) loadController = undefined
    }
  }
}

function scheduleReload(): void {
  if (unmounted) return
  window.clearTimeout(reloadTimer)
  reloadTimer = window.setTimeout(() => {
    reloadTimer = undefined
    void load({ quiet: true })
  }, RELOAD_DEBOUNCE_MS)
}

async function loadHosts(): Promise<void> {
  hostsLoading.value = true
  hostsError.value = false
  try {
    const inventory = await api.cluster.hosts()
    if (unmounted) return
    applyClusterHostOrderPreference(inventory.hostOrder)
    hosts.value = inventory.items
  } catch {
    // The id label is enough, and the picker reports the failure, when the host list cannot be read.
    hostsError.value = true
  } finally {
    hostsLoading.value = false
  }
}

/** The picker just opened: read the cluster hosts if the page has not yet. */
function onHostPickerOpen(): void {
  if (!hosts.value.length && !hostsLoading.value) void loadHosts()
}

function hostStatusOf(host: ClusterHost): HostSwitcherStatus {
  const status = fileHostStatus(host)
  return { action: status.action, tone: status.tone, label: phrase(status.label) }
}

/** A paired Panel without a file relay: open its own gallery page in a new tab. */
function openRemoteGallery(host: ClusterHost): void {
  const target = clusterHostPanelPageURL(host, 'gallery')
  if (!target) {
    void router.push({ name: 'cluster' })
    return
  }
  if (host.transportSecurity === 'e2e_http' && !window.confirm(i18n.t('cluster.confirm.openHttpPanel'))) return
  const opened = window.open(target, '_blank', 'noopener,noreferrer')
  if (!opened) toast.show(phrase('远端图库未打开'), { message: phrase('浏览器阻止了新标签页，请允许弹出窗口后重试。') })
}

/** A host's gallery starts at its own library folder: the folder path is the same on every host. */
function onHostSelected(host: ClusterHost): void {
  const status = fileHostStatus(host)
  if (status.action === 'manage') {
    hostSwitcher.value?.close()
    void router.push({ name: 'cluster' })
    return
  }
  if (status.action === 'open') {
    hostSwitcher.value?.close()
    openRemoteGallery(host)
    return
  }
  hostSwitcher.value?.close(true)
  if ((host.isLocal && !hostId.value) || host.id === hostId.value) return
  void router.push({ name: 'gallery', query: host.isLocal ? {} : { hostId: host.id } })
}

watch([currentPath, hostId], ([, nextHost], [, previousHost]) => {
  if (nextHost !== previousHost) snapshot.value = undefined
  albumDialog.value = undefined
  deleteDialog.value = undefined
  albumMenu.value = undefined
  moreMenuOpen.value = false
  viewerPath.value = undefined
  clearSelection()
  renderLimit.value = RENDER_STEP
  albumsExpanded.value = false
  search.value = ''
  void load()
})

watch([filter, search, () => preferences.sort], () => {
  renderLimit.value = RENDER_STEP
})

watch(viewerIndex, (index) => {
  // The open item left the visible set (deleted, filtered or moved elsewhere).
  if (viewerPath.value && index < 0) viewerPath.value = undefined
})

function clearSelection(): void {
  selected.value = new Set()
  selectionAnchor = ''
}

function stopSelecting(): void {
  selecting.value = false
  clearSelection()
}

function toggleItem(item: GalleryItem, range: boolean): void {
  selecting.value = true
  const next = new Set(selected.value)
  if (range && selectionAnchor) {
    const paths = visibleItems.value.map((candidate) => candidate.entry.path)
    const from = paths.indexOf(selectionAnchor)
    const to = paths.indexOf(item.entry.path)
    if (from >= 0 && to >= 0) {
      for (const path of paths.slice(Math.min(from, to), Math.max(from, to) + 1)) next.add(path)
      selected.value = next
      return
    }
  }
  if (next.has(item.entry.path)) next.delete(item.entry.path)
  else next.add(item.entry.path)
  selected.value = next
  selectionAnchor = item.entry.path
}

function selectAllVisible(): void {
  selected.value = new Set(visibleItems.value.map((item) => item.entry.path))
}

function openViewer(item: GalleryItem): void {
  viewerPath.value = item.entry.path
}

function navigateViewer(index: number): void {
  const item = visibleItems.value[index]
  if (!item) return
  viewerPath.value = item.entry.path
  // Keep the timeline rendered up to the viewed photo so closing lands on it.
  if (index >= renderLimit.value) renderLimit.value = index + RENDER_STEP
}

async function downloadEntries(entries: FileEntry[]): Promise<void> {
  if (!entries.length) return
  try {
    await downloadFileEntries(entries, `${title.value}-${entries.length}`, hostId.value || undefined)
  } catch (error) {
    toast.danger(phrase('下载未开始'), errorMessage(error))
  }
}

function folderChanges(paths: Iterable<string>): string[] {
  return [...new Set([...paths].map(galleryParentPath))]
}

// ---- Uploads -------------------------------------------------------------

function takenNames(target: string): Set<string> {
  const names = new Set<string>()
  for (const item of allItems.value) if (item.folder === target) names.add(item.entry.name)
  for (const album of snapshot.value?.albums ?? []) if (galleryParentPath(album.path) === target) names.add(album.name)
  for (const task of uploads.value) {
    if (task.target === target && task.phase !== 'cancelled' && task.phase !== 'error') names.add(task.name)
  }
  return names
}

/** Create `path` on the given host when it does not exist yet (first upload into a new library). */
async function ensureFolder(path: string, folderHostId = hostId.value): Promise<void> {
  if (folderHostId === hostId.value && snapshot.value?.path === path && snapshot.value.exists) return
  const api = fileAPIForHost(folderHostId)
  try {
    await api.entry(path)
    return
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 404) throw error
  }
  const result = await api.action({ action: 'mkdir', target: galleryParentPath(path), name: galleryBaseName(path) })
  if (result.failed.length) throw new Error(result.failed[0]!.detail)
}

function enqueueUploads(files: Iterable<File>): void {
  const candidates = [...files]
  const media = candidates.filter(isGalleryUploadCandidate)
  const skipped = candidates.length - media.length
  if (skipped) toast.show(phrase('已跳过非图片或视频文件'), { message: `${skipped} ${phrase('个文件未加入上传')}` })
  if (!media.length) return
  const target = currentPath.value
  for (const file of media) {
    const name = nextGalleryUploadName(file.name, takenNames(target))
    uploads.value.push({ id: ++uploadSequence, file, name, target, hostId: hostId.value, progress: 0, phase: 'queued' })
  }
  uploadsCollapsed.value = false
  window.clearTimeout(uploadClearTimer)
  pumpUploads()
}

function updateTask(id: number, patch: Partial<UploadTask>): void {
  uploads.value = uploads.value.map((task) => task.id === id ? { ...task, ...patch } : task)
}

function pumpUploads(): void {
  if (unmounted) return
  while (uploadWorkers < UPLOAD_CONCURRENCY) {
    const task = uploads.value.find((candidate) => candidate.phase === 'queued')
    if (!task) break
    uploadWorkers += 1
    void runUpload(task).finally(() => {
      uploadWorkers -= 1
      pumpUploads()
      if (!uploadWorkers && !uploads.value.some((candidate) => candidate.phase === 'queued')) finishUploadBatch()
    })
  }
}

async function runUpload(task: UploadTask): Promise<void> {
  const controller = new AbortController()
  updateTask(task.id, { phase: 'running', progress: 0, detail: undefined, controller })
  const api = fileAPIForHost(task.hostId)
  let name = task.name
  const tried = new Set([name])
  try {
    await ensureFolder(task.target, task.hostId)
    for (let attempt = 0; ; attempt += 1) {
      try {
        await api.upload(task.target, new File([task.file], name, { type: task.file.type }), false, (progress) => {
          updateTask(task.id, { progress })
        }, controller.signal)
        break
      } catch (error) {
        // Never overwrite: a same-named file is another photo, so pick the next free name.
        if (!(error instanceof ApiError) || error.status !== 409 || attempt >= 4) throw error
        name = nextGalleryUploadName(task.file.name, new Set([...takenNames(task.target), ...tried]))
        tried.add(name)
        updateTask(task.id, { name })
      }
    }
    updateTask(task.id, { phase: 'done', progress: 100, controller: undefined })
  } catch (error) {
    if (controller.signal.aborted) {
      updateTask(task.id, { phase: 'cancelled', controller: undefined })
      return
    }
    updateTask(task.id, { phase: 'error', detail: errorMessage(error), controller: undefined })
  }
}

function finishUploadBatch(): void {
  if (unmounted) return
  const targetsByHost = new Map<string, Set<string>>()
  for (const task of uploads.value) {
    if (task.phase !== 'done') continue
    const targets = targetsByHost.get(task.hostId) ?? new Set<string>()
    targets.add(task.target)
    targetsByHost.set(task.hostId, targets)
  }
  for (const [taskHostId, targets] of targetsByHost) notifyFileDirectoriesChanged([...targets], undefined, [], taskHostId)
  scheduleReload()
  if (!failedUploads.value) {
    window.clearTimeout(uploadClearTimer)
    uploadClearTimer = window.setTimeout(() => {
      uploads.value = uploads.value.filter((task) => task.phase === 'queued' || task.phase === 'running')
    }, 4200)
  }
}

function cancelUpload(task: UploadTask): void {
  if (task.phase === 'queued') updateTask(task.id, { phase: 'cancelled' })
  else task.controller?.abort()
}

function retryUpload(task: UploadTask): void {
  updateTask(task.id, { phase: 'queued', progress: 0, detail: undefined })
  pumpUploads()
}

function clearFinishedUploads(): void {
  uploads.value = uploads.value.filter((task) => task.phase === 'queued' || task.phase === 'running')
}

function onUploadInput(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files) enqueueUploads(input.files)
  input.value = ''
}

function hasExternalFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes('Files')
}

function onDragEnter(event: DragEvent): void {
  if (!hasExternalFiles(event) || !snapshot.value) return
  event.preventDefault()
  dragDepth += 1
  dragging.value = true
}

function onDragOver(event: DragEvent): void {
  if (!hasExternalFiles(event) || !snapshot.value) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function onDragLeave(event: DragEvent): void {
  if (!hasExternalFiles(event)) return
  dragDepth = Math.max(0, dragDepth - 1)
  if (!dragDepth) dragging.value = false
}

function onDrop(event: DragEvent): void {
  if (!hasExternalFiles(event)) return
  event.preventDefault()
  dragDepth = 0
  dragging.value = false
  if (event.dataTransfer?.files.length) enqueueUploads(event.dataTransfer.files)
}

// ---- Albums, deletion and location ----------------------------------------

function openAlbumDialog(dialog: AlbumDialog): void {
  albumMenu.value = undefined
  moreMenuOpen.value = false
  albumDialog.value = dialog
  albumName.value = dialog.mode === 'rename' ? dialog.album.name : ''
}

async function submitAlbumDialog(): Promise<void> {
  const dialog = albumDialog.value
  if (!dialog || albumNameProblem.value || albumBusy.value) return
  const name = albumName.value.trim()
  albumBusy.value = true
  try {
    if (dialog.mode === 'create') {
      await ensureFolder(dialog.parent)
      const result = await fileAPI.value.action({ action: 'mkdir', target: dialog.parent, name })
      if (result.failed.length) throw new Error(result.failed[0]!.detail)
      notifyFileDirectoriesChanged([dialog.parent], undefined, [], hostId.value)
      toast.success(phrase('相册已创建'), name)
    } else {
      const parent = galleryParentPath(dialog.album.path)
      const destination = joinGalleryPath(parent, name)
      const result = await fileAPI.value.action({
        action: 'rename',
        sources: [dialog.album.path],
        target: destination,
        expectedResourceVersion: dialog.album.entry.resourceVersion,
      })
      if (result.failed.length) throw new Error(result.failed[0]!.detail)
      notifyFileDirectoriesChanged([parent], undefined, [{ source: dialog.album.path, destination }], hostId.value)
      toast.success(phrase('相册已重命名'), name)
      if (currentPath.value === dialog.album.path) openFolder(destination)
    }
    albumDialog.value = undefined
    await load({ quiet: true })
  } catch (error) {
    toast.danger(phrase(dialog.mode === 'create' ? '相册未创建' : '相册未重命名'), errorMessage(error))
  } finally {
    albumBusy.value = false
  }
}

function requestDelete(dialog: DeleteDialog): void {
  albumMenu.value = undefined
  moreMenuOpen.value = false
  deleteDialog.value = dialog
}

const deleteSummary = computed(() => {
  const dialog = deleteDialog.value
  if (!dialog) return ''
  if (dialog.kind === 'album') {
    const count = dialog.album.imageCount + dialog.album.videoCount
    return count ? `${dialog.album.name} · ${count} ${phrase('个媒体文件')}` : dialog.album.name
  }
  return dialog.entries.length === 1 ? dialog.entries[0]!.name : `${dialog.entries.length} ${phrase('个文件')}`
})

async function confirmDelete(): Promise<void> {
  const dialog = deleteDialog.value
  if (!dialog || deleteBusy.value) return
  const entries = dialog.kind === 'album' ? [dialog.album.entry] : dialog.entries
  deleteBusy.value = true
  try {
    const result = await fileAPI.value.action({
      action: 'trash',
      sources: entries.map((entry) => entry.path),
      expectedResourceVersions: Object.fromEntries(entries.map((entry) => [entry.path, entry.resourceVersion])),
    })
    const removed = new Set(result.succeeded.map((item) => item.path))
    if (removed.size) notifyFileDirectoriesChanged(folderChanges(removed), undefined, [], hostId.value)
    if (result.failed.length) {
      toast.danger(
        phrase(removed.size ? '部分文件未移入回收站' : '未能移入回收站'),
        `${removed.size} ${phrase('项成功')}，${result.failed.length} ${phrase('项失败')}：${result.failed[0]!.detail}`,
      )
    } else {
      toast.success(phrase('已移入回收站'), phrase('可在文件管理的回收站中恢复。'))
    }
    const gone = (path: string) => [...removed].some((source) => path === source || path.startsWith(`${source}/`))
    // Keep the viewer open on the neighbouring photo instead of closing it.
    if (viewerPath.value && gone(viewerPath.value)) {
      const index = viewerIndex.value
      const after = visibleItems.value.slice(index + 1).find((item) => !gone(item.entry.path))
      const before = visibleItems.value.slice(0, Math.max(index, 0)).reverse().find((item) => !gone(item.entry.path))
      viewerPath.value = (after ?? before)?.entry.path
    }
    if (snapshot.value) {
      snapshot.value = {
        ...snapshot.value,
        items: snapshot.value.items.filter((item) => !gone(item.entry.path)),
        albums: snapshot.value.albums.filter((album) => !gone(album.path)),
      }
    }
    selected.value = new Set([...selected.value].filter((path) => !removed.has(path)))
    if (dialog.kind === 'album' && removed.has(currentPath.value)) openFolder(galleryParentPath(currentPath.value))
    deleteDialog.value = undefined
    if (!selected.value.size && dialog.kind === 'media' && dialog.entries.length > 1) stopSelecting()
    scheduleReload()
  } catch (error) {
    toast.danger(phrase('未能移入回收站'), errorMessage(error))
  } finally {
    deleteBusy.value = false
    if (viewerPath.value) void nextTick(() => page.value?.querySelector<HTMLElement>('.gallery-viewer')?.focus({ preventScroll: true }))
  }
}

function openLocationDialog(initial = preferences.root): void {
  moreMenuOpen.value = false
  locationValue.value = initial
  locationDialogOpen.value = true
}

function submitLocation(): void {
  const root = normalizeGalleryRoot(locationValue.value)
  if (!root) return
  preferences.root = root
  locationDialogOpen.value = false
  void router.push({ name: 'gallery', query: {} })
}

async function createLibraryFolder(): Promise<void> {
  try {
    await ensureFolder(currentPath.value)
    notifyFileDirectoriesChanged([galleryParentPath(currentPath.value)], undefined, [], hostId.value)
    await load()
  } catch (error) {
    toast.danger(phrase('图库文件夹未创建'), errorMessage(error))
  }
}

function onAlbumMenu(event: MouseEvent, album: GalleryAlbum): void {
  event.stopPropagation()
  moreMenuOpen.value = false
  albumMenu.value = albumMenu.value === album.path ? undefined : album.path
}

function closeMenus(event: MouseEvent): void {
  const target = event.target as HTMLElement | null
  if (target?.closest('.gallery-menu, [data-gallery-menu-trigger]')) return
  albumMenu.value = undefined
  moreMenuOpen.value = false
}

function onPageKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape' || viewerPath.value) return
  if (hostSwitcher.value?.isOpen) {
    hostSwitcher.value.close(true)
    event.preventDefault()
  } else if (albumMenu.value || moreMenuOpen.value) {
    albumMenu.value = undefined
    moreMenuOpen.value = false
    event.preventDefault()
  } else if (selecting.value) {
    stopSelecting()
    event.preventDefault()
  }
}

let sentinelObserver: IntersectionObserver | undefined
watch(sentinel, (element, previous) => {
  if (previous) sentinelObserver?.unobserve(previous)
  if (element) sentinelObserver?.observe(element)
})

let unsubscribeChanges: (() => void) | undefined

onMounted(() => {
  void loadHosts()
  windowed.value = Boolean(page.value?.closest('.desktop-window__body'))
  if (typeof IntersectionObserver !== 'undefined') {
    // Inside a desktop window the gallery scrolls its own frame, which would clip a viewport root.
    sentinelObserver = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting) && renderLimit.value < visibleItems.value.length) {
        renderLimit.value += RENDER_STEP
      }
    }, { root: windowed.value ? scroller.value : null, rootMargin: '800px 0px' })
    if (sentinel.value) sentinelObserver.observe(sentinel.value)
  }
  unsubscribeChanges = subscribeFileDirectoryChanges((directories, _origin, moves, changedHost) => {
    if ((changedHost || '') !== hostId.value || !snapshot.value) return
    const watched = new Set([currentPath.value, ...snapshot.value.albums.map((album) => album.path)])
    const touched = [...directories].some((directory) => watched.has(directory))
      || (moves ?? []).some((move) => watched.has(galleryParentPath(move.source)) || watched.has(galleryParentPath(move.destination)))
    if (touched) scheduleReload()
  })
  document.addEventListener('pointerdown', closeMenus)
  void load()
})

watch(desktopWindowActive, (active) => {
  if (!active) {
    albumMenu.value = undefined
    moreMenuOpen.value = false
    hostSwitcher.value?.close()
  }
})

onBeforeUnmount(() => {
  unmounted = true
  loadController?.abort()
  sentinelObserver?.disconnect()
  unsubscribeChanges?.()
  window.clearTimeout(reloadTimer)
  window.clearTimeout(uploadClearTimer)
  document.removeEventListener('pointerdown', closeMenus)
  for (const task of uploads.value) task.controller?.abort()
})
</script>

<template>
  <div
    ref="page"
    class="gallery-page"
    :class="[`gallery-page--${preferences.density}`, { 'gallery-page--windowed': windowed }]"
    @dragenter="onDragEnter"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
    @keydown="onPageKeydown"
  >
    <div ref="scroller" class="gallery-scroll">
      <section class="gallery-hero" :class="{ 'gallery-hero--photo': heroUrl }">
        <div v-if="heroUrl" class="gallery-hero__backdrop" aria-hidden="true">
          <img :src="heroUrl" alt="" decoding="async" />
        </div>
        <div class="gallery-hero__content">
          <nav v-if="trail.length > 1 || !insideLibrary" class="gallery-trail" aria-label="图库位置">
            <template v-for="(segment, index) in trail" :key="segment.path">
              <ChevronRight v-if="index > 0" :size="14" aria-hidden="true" />
              <button
                v-if="index < trail.length - 1"
                type="button"
                class="gallery-trail__link"
                @click="openFolder(segment.path)"
              >
                <Images v-if="index === 0 && insideLibrary" :size="15" aria-hidden="true" />
                {{ index === 0 && insideLibrary ? '图库' : segment.name }}
              </button>
              <span v-else class="gallery-trail__current" aria-current="page">
                <Images v-if="index === 0 && insideLibrary" :size="15" aria-hidden="true" />
                {{ index === 0 && insideLibrary ? '图库' : segment.name }}
              </span>
            </template>
          </nav>
          <h1 class="gallery-hero__title">{{ title }}</h1>
          <p v-if="snapshot?.exists" class="gallery-hero__meta">
            {{ summary }}<span v-if="snapshot.scanning" class="gallery-hero__scanning"> · 正在汇总相册…</span>
          </p>
          <p class="gallery-hero__path">
            <MapPin :size="14" aria-hidden="true" />
            <span>{{ currentPath }}</span>
          </p>
          <div v-if="showHostSwitcher" class="gallery-host">
            <HostSwitcher
              ref="hostSwitcher"
              :hosts="orderedHosts"
              :active-id="activeHostId"
              :label="activeHostLabel"
              :loading="hostsLoading"
              :error="hostsError"
              :status-of="hostStatusOf"
              @open="onHostPickerOpen"
              @refresh="loadHosts"
              @select="onHostSelected"
            />
          </div>
        </div>
        <div class="gallery-hero__actions">
          <button
            v-if="snapshot?.exists"
            class="button button--secondary button--small"
            type="button"
            @click="openAlbumDialog({ mode: 'create', parent: currentPath })"
          >
            <FolderPlus :size="16" /> 新建相册
          </button>
          <button class="button button--primary button--small" type="button" :disabled="!snapshot" @click="uploadInput?.click()">
            <Upload :size="16" /> 上传照片和视频
          </button>
          <div class="gallery-hero__more">
            <button
              class="button button--secondary button--small gallery-icon-button"
              type="button"
              data-gallery-menu-trigger
              aria-haspopup="menu"
              :aria-expanded="moreMenuOpen"
              title="更多操作"
              aria-label="更多操作"
              @click="moreMenuOpen = !moreMenuOpen; albumMenu = undefined"
            ><MoreHorizontal :size="17" /></button>
            <div v-if="moreMenuOpen" class="gallery-menu" role="menu">
              <button role="menuitem" type="button" @click="moreMenuOpen = false; revealInFiles(currentPath)">
                <FolderOpen :size="16" /> 在文件管理中打开
              </button>
              <button role="menuitem" type="button" @click="moreMenuOpen = false; load()">
                <RefreshCw :size="16" /> 刷新
              </button>
              <button v-if="!hostId" role="menuitem" type="button" @click="openLocationDialog()">
                <MapPin :size="16" /> 更改图库位置
              </button>
              <button
                v-if="!hostId && !insideLibrary"
                role="menuitem"
                type="button"
                @click="openLocationDialog(currentPath)"
              >
                <Images :size="16" /> 设为图库位置
              </button>
            </div>
          </div>
        </div>
        <input
          ref="uploadInput"
          class="sr-only"
          type="file"
          multiple
          :accept="GALLERY_UPLOAD_ACCEPT"
          aria-label="选择要上传的照片和视频"
          @change="onUploadInput"
        />
      </section>

      <div v-if="snapshot?.exists && !insideLibrary && !hostId" class="gallery-notice" role="note">
        <CircleAlert :size="17" aria-hidden="true" />
        <span>正在以图库方式浏览文件夹；它不在当前图库位置 {{ preferences.root }} 之内。</span>
        <button type="button" class="gallery-notice__action" @click="openLocationDialog(currentPath)">设为图库位置</button>
      </div>
      <div v-if="snapshot?.truncated" class="gallery-notice" role="note">
        <CircleAlert :size="17" aria-hidden="true" />
        <span>文件较多，仅汇总了前 48 个相册和每个文件夹的前几千项；更深的内容请打开对应相册查看。</span>
      </div>
      <div v-if="snapshot?.failedAlbums" class="gallery-notice gallery-notice--warning" role="status">
        <CircleAlert :size="17" aria-hidden="true" />
        <span>{{ snapshot.failedAlbums }} 个相册暂时无法读取，已跳过。</span>
        <button type="button" class="gallery-notice__action" @click="load()">重试</button>
      </div>
      <div v-if="loadError && snapshot" class="gallery-notice gallery-notice--warning" role="alert">
        <CircleAlert :size="17" aria-hidden="true" />
        <span>{{ phrase('图库读取失败') }}：{{ loadError }}</span>
        <button type="button" class="gallery-notice__action" @click="load()">重试</button>
      </div>

      <LoadingState v-if="loading && !snapshot" :rows="3" cards label="正在读取图库" />
      <ErrorState v-else-if="loadError && !snapshot" :message="loadError" title="图库读取失败" @retry="load()" />

      <section v-else-if="snapshot && !snapshot.exists" class="gallery-empty">
        <span class="gallery-empty__art" aria-hidden="true"><Images :size="40" :stroke-width="1.6" /></span>
        <h2>{{ isLibraryRoot ? '开始建立你的图库' : '这个文件夹不存在' }}</h2>
        <p v-if="isLibraryRoot">
          照片和视频会保存在服务器的 {{ currentPath }} 文件夹中，文件管理里也能看到它们。可以先创建文件夹，或在“更多操作”里换一个位置。
        </p>
        <p v-else>{{ currentPath }} 已被移动或删除。</p>
        <div class="gallery-empty__actions">
          <button v-if="isLibraryRoot" class="button button--primary" type="button" @click="createLibraryFolder">
            <FolderPlus :size="16" /> 创建图库文件夹
          </button>
          <button v-if="isLibraryRoot && !hostId" class="button button--secondary" type="button" @click="openLocationDialog()">
            <MapPin :size="16" /> 更改图库位置
          </button>
          <button v-else-if="!isLibraryRoot" class="button button--secondary" type="button" @click="openFolder(preferences.root)">
            <Images :size="16" /> 返回图库
          </button>
        </div>
      </section>

      <template v-else-if="snapshot">
        <section v-if="albums.length" class="gallery-albums" aria-labelledby="gallery-albums-title">
          <header class="gallery-section-header">
            <h2 id="gallery-albums-title">相册 <span>{{ albums.length }}</span></h2>
            <button
              v-if="albums.length > ALBUM_PREVIEW_COUNT"
              type="button"
              class="gallery-link"
              @click="albumsExpanded = !albumsExpanded"
            >{{ albumsExpanded ? '收起' : '查看全部' }}</button>
          </header>
          <div class="gallery-albums__grid">
            <article v-for="album in visibleAlbums" :key="album.path" class="album-card">
              <button class="album-card__open" type="button" :aria-label="`打开相册 ${album.name}`" @click="openFolder(album.path)">
                <span class="album-card__cover">
                  <img
                    v-if="album.cover && tileUrls(album.cover).image"
                    :src="tileUrls(album.cover).image"
                    alt=""
                    loading="lazy"
                    decoding="async"
                    draggable="false"
                  />
                  <video
                    v-else-if="album.cover && tileUrls(album.cover).video"
                    :src="`${tileUrls(album.cover).video}#t=0.1`"
                    muted
                    playsinline
                    preload="metadata"
                    tabindex="-1"
                    aria-hidden="true"
                  />
                  <span v-else class="album-card__placeholder"><Images :size="28" :stroke-width="1.6" /></span>
                </span>
                <span class="album-card__name">{{ album.name }}</span>
                <span class="album-card__meta">{{ albumMeta(album) }}</span>
              </button>
              <button
                class="album-card__menu-button"
                type="button"
                data-gallery-menu-trigger
                aria-haspopup="menu"
                :aria-expanded="albumMenu === album.path"
                :aria-label="`${album.name} 操作`"
                @click="onAlbumMenu($event, album)"
              ><MoreHorizontal :size="17" /></button>
              <div v-if="albumMenu === album.path" class="gallery-menu gallery-menu--album" role="menu">
                <button role="menuitem" type="button" @click="openAlbumDialog({ mode: 'rename', album })">
                  <Pencil :size="16" /> 重命名
                </button>
                <button role="menuitem" type="button" @click="albumMenu = undefined; revealInFiles(album.path)">
                  <FolderOpen :size="16" /> 在文件管理中打开
                </button>
                <button role="menuitem" type="button" class="gallery-menu__danger" @click="requestDelete({ kind: 'album', album })">
                  <Trash2 :size="16" /> 删除相册
                </button>
              </div>
            </article>
          </div>
        </section>

        <div v-if="allItems.length" class="gallery-toolbar" role="toolbar" aria-label="图库筛选与排版">
          <div class="gallery-segmented" role="group" aria-label="媒体类型">
            <button
              v-for="option in filterOptions"
              :key="option.value"
              type="button"
              :class="{ 'is-active': filter === option.value }"
              :aria-pressed="filter === option.value"
              @click="filter = option.value"
            >
              {{ option.label }} <span>{{ option.count }}</span>
            </button>
          </div>
          <label class="gallery-search">
            <Search :size="16" aria-hidden="true" />
            <input v-model="search" type="search" placeholder="搜索文件名" aria-label="搜索文件名" />
            <button v-if="search" type="button" aria-label="清除搜索" @click="search = ''"><X :size="14" /></button>
          </label>
          <span class="gallery-toolbar__spacer" />
          <label class="gallery-sort">
            <ArrowUpDown :size="15" aria-hidden="true" />
            <select v-model="preferences.sort" aria-label="排序方式">
              <option value="newest">最新优先</option>
              <option value="oldest">最早优先</option>
              <option value="name">按名称</option>
            </select>
          </label>
          <div class="gallery-density" role="group" aria-label="缩略图大小">
            <button
              v-for="option in densityOptions"
              :key="option.value"
              type="button"
              :class="{ 'is-active': preferences.density === option.value }"
              :aria-pressed="preferences.density === option.value"
              :title="option.label"
              :aria-label="option.label"
              @click="preferences.density = option.value"
            ><component :is="option.icon" :size="16" /></button>
          </div>
          <button
            class="button button--small"
            :class="selecting ? 'button--primary' : 'button--secondary'"
            type="button"
            :aria-pressed="selecting"
            @click="selecting ? stopSelecting() : (selecting = true)"
          >
            <ListChecks :size="16" /> {{ selecting ? '完成' : '选择' }}
          </button>
        </div>

        <div v-if="selecting" class="gallery-selection" role="region" aria-label="已选择的项目">
          <strong>已选择 {{ selected.size }} 项</strong>
          <button type="button" class="gallery-link" @click="selectAllVisible">全选当前 {{ visibleItems.length }} 项</button>
          <button v-if="selected.size" type="button" class="gallery-link" @click="clearSelection">清除</button>
          <span class="gallery-toolbar__spacer" />
          <button class="button button--secondary button--small" type="button" :disabled="!selected.size" @click="downloadEntries(selectedEntries)">
            <Download :size="16" /> 下载
          </button>
          <button
            class="button button--danger button--small"
            type="button"
            :disabled="!selected.size"
            @click="requestDelete({ kind: 'media', entries: selectedEntries })"
          >
            <Trash2 :size="16" /> 移入回收站
          </button>
        </div>

        <section v-if="emptyLibrary" class="gallery-empty gallery-empty--drop">
          <span class="gallery-empty__art" aria-hidden="true"><Images :size="40" :stroke-width="1.6" /></span>
          <h2>{{ isLibraryRoot ? '图库还是空的' : '这个相册还是空的' }}</h2>
          <p>把照片或视频拖到这里，或点击下方按钮上传；也可以先新建几个相册整理。</p>
          <div class="gallery-empty__actions">
            <button class="button button--primary" type="button" @click="uploadInput?.click()">
              <Upload :size="16" /> 上传照片和视频
            </button>
            <button class="button button--secondary" type="button" @click="openAlbumDialog({ mode: 'create', parent: currentPath })">
              <FolderPlus :size="16" /> 新建相册
            </button>
          </div>
        </section>

        <section v-else-if="allItems.length && !visibleItems.length" class="gallery-empty gallery-empty--compact">
          <h2>没有匹配的照片或视频</h2>
          <p>换个关键词，或切换回“全部”。</p>
          <div class="gallery-empty__actions">
            <button class="button button--secondary" type="button" @click="filter = 'all'; search = ''">清除筛选</button>
          </div>
        </section>

        <section
          v-for="group in groups"
          :key="group.key"
          class="gallery-month"
          :aria-label="group.month ? formatGalleryMonth(group.month, locale) : '全部照片和视频'"
        >
          <header v-if="group.key !== 'all'" class="gallery-month__header">
            <h3>{{ group.month ? formatGalleryMonth(group.month, locale) : '时间未知' }}</h3>
            <span>{{ group.items.length }} 项</span>
          </header>
          <div class="gallery-grid">
            <GalleryTile
              v-for="(item, index) in group.items"
              :key="item.entry.path"
              :item="item"
              :image-url="tileUrls(item, index === 0 && group.items.length >= 5 && preferences.density !== 'compact').image"
              :fallback-url="tileUrls(item, index === 0 && group.items.length >= 5 && preferences.density !== 'compact').fallback"
              :video-url="tileUrls(item).video"
              :featured="index === 0 && group.items.length >= 5 && preferences.density !== 'compact' && preferences.sort !== 'name'"
              :selecting="selecting"
              :selected="selected.has(item.entry.path)"
              @open="openViewer"
              @toggle="toggleItem"
            />
          </div>
        </section>
        <div v-if="renderLimit < visibleItems.length" ref="sentinel" class="gallery-more" role="status">
          正在加载更多…
          <button type="button" class="gallery-link" @click="renderLimit += RENDER_STEP">立即加载</button>
        </div>
      </template>
    </div>

    <div v-if="dragging" class="drop-overlay gallery-drop" aria-hidden="true">
      <div class="gallery-drop__card">
        <Upload :size="30" />
        <strong>松开即可上传</strong>
        <span>保存到 {{ title }} · 只接收图片和视频</span>
      </div>
    </div>

    <aside v-if="uploads.length" class="gallery-uploads" :class="{ 'gallery-uploads--collapsed': uploadsCollapsed }" aria-label="上传队列">
      <header class="gallery-uploads__header">
        <div>
          <strong v-if="activeUploads.length">正在上传 {{ activeUploads.length }} 项 · {{ uploadProgress }}%</strong>
          <strong v-else-if="failedUploads">{{ failedUploads }} 项上传失败</strong>
          <strong v-else>已上传 {{ finishedUploads }} 项</strong>
          <span class="gallery-uploads__bar" aria-hidden="true"><span :style="{ width: `${uploadProgress}%` }" /></span>
        </div>
        <button type="button" class="gallery-link" @click="uploadsCollapsed = !uploadsCollapsed">{{ uploadsCollapsed ? '展开' : '收起' }}</button>
        <button v-if="!activeUploads.length" type="button" class="gallery-uploads__close" aria-label="关闭上传队列" @click="clearFinishedUploads"><X :size="16" /></button>
      </header>
      <ul v-if="!uploadsCollapsed" class="gallery-uploads__list">
        <li v-for="task in uploads" :key="task.id" :class="`is-${task.phase}`">
          <span class="gallery-uploads__icon" aria-hidden="true">
            <CheckCircle2 v-if="task.phase === 'done'" :size="17" />
            <CircleAlert v-else-if="task.phase === 'error'" :size="17" />
            <Film v-else-if="task.file.type.startsWith('video/')" :size="17" />
            <Images v-else :size="17" />
          </span>
          <span class="gallery-uploads__name">
            <strong :title="task.name">{{ task.name }}</strong>
            <small v-if="task.phase === 'error'">{{ task.detail }}</small>
            <small v-else-if="task.phase === 'cancelled'">已取消</small>
            <small v-else-if="task.phase === 'queued'">等待中 · {{ formatBytes(task.file.size) }}</small>
            <small v-else-if="task.phase === 'running'">{{ task.progress }}% · {{ formatBytes(task.file.size) }}</small>
            <small v-else-if="task.name !== task.file.name">已保存，因重名改为 {{ task.name }}</small>
            <small v-else>已保存</small>
          </span>
          <button
            v-if="task.phase === 'queued' || task.phase === 'running'"
            type="button"
            class="gallery-link"
            @click="cancelUpload(task)"
          >取消</button>
          <button v-else-if="task.phase === 'error'" type="button" class="gallery-link" @click="retryUpload(task)">重试</button>
        </li>
      </ul>
    </aside>

    <GalleryViewer
      v-if="viewerPath && viewerIndex >= 0"
      :items="visibleItems"
      :index="viewerIndex"
      :sources="viewerSources"
      :contained="windowed"
      :library-root="insideLibrary ? preferences.root : undefined"
      :filmstrip="preferences.filmstrip"
      can-delete
      @close="viewerPath = undefined"
      @update:filmstrip="preferences.filmstrip = $event"
      @navigate="navigateViewer"
      @download="downloadEntries([$event.entry])"
      @reveal="viewerPath = undefined; revealInFiles($event.folder)"
      @delete="requestDelete({ kind: 'media', entries: [$event.entry] })"
    />

    <ModalDialog
      :open="Boolean(albumDialog)"
      :title="phrase(albumDialog?.mode === 'rename' ? '重命名相册' : '新建相册')"
      :description="phrase(albumDialog?.mode === 'rename' ? '相册就是服务器上的文件夹，重命名会同步修改文件夹名称。' : '相册会以文件夹形式创建在当前位置，文件管理中同样可见。')"
      size="small"
      :close-disabled="albumBusy"
      @close="albumDialog = undefined"
    >
      <form class="gallery-form" @submit.prevent="submitAlbumDialog">
        <label>
          <span>{{ phrase('相册名称') }}</span>
          <input
            v-model="albumName"
            class="text-input"
            type="text"
            maxlength="120"
            autocomplete="off"
            :placeholder="phrase('例如：2026 京都旅行')"
            :aria-invalid="Boolean(albumName && albumNameProblem)"
          />
        </label>
        <p v-if="albumName && albumNameProblem" class="gallery-form__error" role="alert">{{ phrase(albumNameProblem) }}</p>
        <div class="gallery-form__actions">
          <button class="button button--secondary" type="button" :disabled="albumBusy" @click="albumDialog = undefined">{{ phrase('取消') }}</button>
          <button class="button button--primary" type="submit" :disabled="albumBusy || Boolean(albumNameProblem)">
            {{ phrase(albumSubmitLabel) }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <ModalDialog
      :open="Boolean(deleteDialog)"
      :title="phrase(deleteDialog?.kind === 'album' ? '删除相册' : '移入回收站')"
      :description="phrase(deleteDialog?.kind === 'album' ? '整个相册文件夹及其中的照片和视频会移入回收站，可在文件管理的回收站中恢复。' : '文件会移入回收站，可在文件管理的回收站中恢复。')"
      size="small"
      :close-disabled="deleteBusy"
      @close="deleteDialog = undefined"
    >
      <div class="gallery-form">
        <p class="gallery-form__summary"><Trash2 :size="16" aria-hidden="true" /> {{ deleteSummary }}</p>
        <div class="gallery-form__actions">
          <button class="button button--secondary" type="button" :disabled="deleteBusy" @click="deleteDialog = undefined">{{ phrase('取消') }}</button>
          <button class="button button--danger" type="button" :disabled="deleteBusy" @click="confirmDelete">
            {{ phrase(deleteBusy ? '正在移入…' : '移入回收站') }}
          </button>
        </div>
      </div>
    </ModalDialog>

    <ModalDialog
      :open="locationDialogOpen"
      :title="phrase('图库位置')"
      :description="phrase('图库读取这个文件夹：其中的照片和视频直接显示，子文件夹作为相册。设置只保存在当前浏览器。')"
      size="small"
      @close="locationDialogOpen = false"
    >
      <form class="gallery-form" @submit.prevent="submitLocation">
        <label>
          <span>{{ phrase('文件夹路径') }}</span>
          <input v-model="locationValue" class="text-input" type="text" spellcheck="false" autocomplete="off" :aria-invalid="Boolean(locationProblem)" />
        </label>
        <p v-if="locationProblem" class="gallery-form__error" role="alert">{{ phrase(locationProblem) }}</p>
        <div class="gallery-form__actions">
          <button
            class="button button--secondary"
            type="button"
            :disabled="locationValue === GALLERY_DEFAULT_ROOT"
            @click="locationValue = GALLERY_DEFAULT_ROOT"
          >{{ phrase('恢复默认') }}</button>
          <button class="button button--primary" type="submit" :disabled="Boolean(locationProblem)">{{ phrase('保存') }}</button>
        </div>
      </form>
    </ModalDialog>
  </div>
</template>

<style scoped>
.gallery-page {
  --gallery-tile: 168px;
  --gallery-gap: 6px;
  --gallery-tile-radius: var(--radius-sm);

  position: relative;
  min-height: 100%;
}

.gallery-page--compact {
  --gallery-tile: 112px;
  --gallery-gap: 3px;
  --gallery-tile-radius: 4px;
}

.gallery-page--spacious {
  --gallery-tile: 236px;
  --gallery-gap: 10px;
  --gallery-tile-radius: var(--radius);
}

.gallery-page--windowed {
  display: flex;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
}

.gallery-page--windowed .gallery-scroll {
  flex: 1 1 auto;
  min-height: 0;
  padding: 16px clamp(14px, 2.4cqi, 26px) 28px;
  overflow: auto;
  overscroll-behavior: contain;
}

.gallery-scroll {
  display: grid;
  align-content: start;
  gap: 18px;
}

/* ---- Hero ---- */

/* No overflow clipping here: the actions menu drops below the cover; the backdrop clips itself. */
.gallery-hero {
  position: relative;
  z-index: 2;
  display: flex;
  min-height: 176px;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px 24px;
  padding: 22px 24px;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  background: var(--surface-raised);
  isolation: isolate;
}

.gallery-hero--photo {
  min-height: clamp(200px, 26vh, 260px);
  border-color: transparent;
  color: #fff;
  background: #0b0f15;
}

.gallery-page--windowed .gallery-hero--photo {
  min-height: clamp(168px, 22cqi, 236px);
}

.gallery-hero__backdrop {
  position: absolute;
  z-index: -1;
  inset: 0;
  overflow: hidden;
  border-radius: inherit;
}

.gallery-hero__backdrop img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  animation: gallery-hero-in .9s ease-out both;
}

.gallery-hero__backdrop::after {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgb(6 9 14 / 8%) 0%, rgb(6 9 14 / 24%) 45%, rgb(6 9 14 / 78%) 100%),
    linear-gradient(90deg, rgb(6 9 14 / 42%) 0%, transparent 62%);
  content: '';
}

.gallery-hero__content {
  display: grid;
  min-width: 0;
  flex: 1 1 320px;
  gap: 6px;
}

.gallery-trail {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 4px;
  color: var(--muted);
  font-size: 13px;
}

.gallery-hero--photo .gallery-trail {
  color: rgb(255 255 255 / 80%);
}

.gallery-trail__link,
.gallery-trail__current {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 26px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: inherit;
  font: inherit;
  font-weight: 500;
}

.gallery-trail__link {
  cursor: pointer;
}

.gallery-trail__link:hover {
  background: color-mix(in srgb, currentColor 14%, transparent);
  color: var(--text);
}

.gallery-hero--photo .gallery-trail__link:hover {
  color: #fff;
}

.gallery-trail__current {
  color: var(--text);
}

.gallery-hero--photo .gallery-trail__current {
  color: #fff;
}

.gallery-hero__title {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: clamp(24px, 3.2vw, 32px);
  font-weight: 700;
  line-height: 1.2;
}

.gallery-hero--photo .gallery-hero__title {
  text-shadow: 0 1px 18px rgb(0 0 0 / 35%);
}

.gallery-hero__meta {
  margin: 0;
  color: var(--text-soft);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.gallery-hero--photo .gallery-hero__meta {
  color: rgb(255 255 255 / 90%);
}

.gallery-hero__scanning {
  color: var(--muted);
}

.gallery-hero--photo .gallery-hero__scanning {
  color: rgb(255 255 255 / 72%);
}

.gallery-hero__path {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  margin: 2px 0 0;
  color: var(--muted);
  font-size: 13px;
  overflow-wrap: anywhere;
}

.gallery-hero--photo .gallery-hero__path {
  color: rgb(255 255 255 / 74%);
}

/* The shared host picker sits under the path line. */
.gallery-host {
  margin-top: 8px;
  width: fit-content;
  max-width: 100%;
}

.gallery-hero__actions {
  position: relative;
  display: flex;
  flex: 0 1 auto;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.gallery-hero--photo .gallery-hero__actions .button--secondary {
  border-color: rgb(255 255 255 / 30%);
  background: rgb(255 255 255 / 14%);
  color: #fff;
}

.gallery-hero--photo .gallery-hero__actions .button--secondary:hover {
  background: rgb(255 255 255 / 24%);
}

.gallery-hero--photo .gallery-hero__actions .button--primary {
  border-color: transparent;
  background: #fff;
  color: #10151d;
}

.gallery-hero--photo .gallery-hero__actions .button--primary:hover {
  background: #e9edf3;
}

.gallery-hero__more {
  position: relative;
}

.gallery-icon-button {
  width: 36px;
  padding: 0;
  justify-content: center;
}

/* ---- Menus and notices ---- */

.gallery-menu {
  position: absolute;
  z-index: 40;
  top: calc(100% + 6px);
  right: 0;
  display: grid;
  min-width: 200px;
  padding: 6px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--surface-raised);
  box-shadow: var(--shadow-md);
  color: var(--text);
}

.gallery-menu button {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: inherit;
  font-size: 14px;
  text-align: left;
  cursor: pointer;
}

.gallery-menu button:hover,
.gallery-menu button:focus-visible {
  background: var(--surface-subtle);
  outline: none;
}

.gallery-menu .gallery-menu__danger {
  color: var(--danger, #c2410c);
}

.gallery-notice {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  padding: 10px 14px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-subtle);
  color: var(--text-soft);
  font-size: 14px;
  line-height: 1.5;
}

.gallery-notice--warning {
  border-color: color-mix(in srgb, var(--warning, #d97706) 40%, var(--border));
}

.gallery-notice > span {
  flex: 1 1 260px;
}

.gallery-notice__action,
.gallery-link {
  min-height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--brand);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
}

.gallery-notice__action:hover,
.gallery-link:hover {
  background: var(--brand-soft);
}

/* ---- Albums ---- */

.gallery-section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.gallery-section-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.gallery-section-header h2 span {
  margin-left: 6px;
  color: var(--muted);
  font-size: 14px;
  font-weight: 500;
}

.gallery-albums__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(176px, 1fr));
  gap: 14px;
}

.album-card {
  position: relative;
  display: grid;
  min-width: 0;
  align-content: start;
}

.album-card__open {
  display: grid;
  min-width: 0;
  gap: 4px;
  padding: 0 0 4px;
  border: 0;
  background: none;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.album-card__cover {
  position: relative;
  display: grid;
  aspect-ratio: 4 / 3;
  place-items: center;
  overflow: hidden;
  margin-bottom: 6px;
  border-radius: var(--radius);
  background: var(--surface-subtle);
  color: var(--muted);
}

.album-card__cover img,
.album-card__cover video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform .5s cubic-bezier(.2, .7, .2, 1);
}

.album-card__open:hover .album-card__cover img,
.album-card__open:hover .album-card__cover video {
  transform: scale(1.04);
}

.album-card__open:focus-visible {
  outline: none;
}

.album-card__open:focus-visible .album-card__cover {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
}

.album-card__name {
  overflow: hidden;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.album-card__meta {
  color: var(--muted);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.album-card__menu-button {
  position: absolute;
  top: 8px;
  right: 8px;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: rgb(8 12 18 / 46%);
  color: #fff;
  cursor: pointer;
  opacity: 0;
  transition: opacity .15s ease;
}

.album-card:hover .album-card__menu-button,
.album-card__menu-button:focus-visible,
.album-card__menu-button[aria-expanded='true'] {
  opacity: 1;
}

.album-card__menu-button:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 1px;
}

.gallery-menu--album {
  top: 46px;
  right: 8px;
}

@media (hover: none) {
  .album-card__menu-button {
    opacity: 1;
  }
}

/* ---- Toolbar and selection ---- */

.gallery-toolbar,
.gallery-selection {
  position: sticky;
  z-index: 12;
  top: var(--topbar-height);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-raised);
  box-shadow: var(--shadow-sm);
}

.gallery-page--windowed .gallery-toolbar,
.gallery-page--windowed .gallery-selection {
  top: 0;
}

.gallery-selection {
  border-color: color-mix(in srgb, var(--brand) 40%, var(--border));
  background: color-mix(in srgb, var(--brand-soft) 60%, var(--surface-raised));
}

.gallery-selection strong {
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.gallery-toolbar__spacer {
  flex: 1 1 0;
}

.gallery-segmented,
.gallery-density {
  display: inline-flex;
  padding: 3px;
  border-radius: var(--radius-sm);
  background: var(--surface-subtle);
}

.gallery-segmented button,
.gallery-density button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 12px;
  border: 0;
  border-radius: calc(var(--radius-sm) - 2px);
  background: none;
  color: var(--text-soft);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
}

.gallery-density button {
  width: 34px;
  justify-content: center;
  padding: 0;
}

.gallery-segmented button span {
  color: var(--muted);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.gallery-segmented button.is-active,
.gallery-density button.is-active {
  background: var(--surface-raised);
  box-shadow: var(--shadow-sm);
  color: var(--text);
}

.gallery-segmented button:focus-visible,
.gallery-density button:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 1px;
}

.gallery-search {
  display: inline-flex;
  min-width: 0;
  flex: 0 1 240px;
  align-items: center;
  gap: 6px;
  min-height: 36px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--muted);
}

.gallery-search:focus-within {
  border-color: var(--brand);
}

.gallery-search input {
  min-width: 0;
  flex: 1 1 auto;
  border: 0;
  outline: none;
  background: none;
  color: var(--text);
  font-size: 14px;
}

.gallery-search button {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: none;
  color: var(--muted);
  cursor: pointer;
}

.gallery-sort {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted);
}

.gallery-sort select {
  min-height: 36px;
  padding: 0 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 14px;
}

/* ---- Timeline ---- */

.gallery-month {
  display: grid;
  gap: 10px;
  content-visibility: auto;
  contain-intrinsic-size: auto 640px;
}

.gallery-month__header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 6px 2px 0;
}

.gallery-month__header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.gallery-month__header span {
  color: var(--muted);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.gallery-grid {
  display: grid;
  grid-auto-flow: row dense;
  grid-template-columns: repeat(auto-fill, minmax(var(--gallery-tile), 1fr));
  gap: var(--gallery-gap);
}

.gallery-more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 18px;
  color: var(--muted);
  font-size: 14px;
}

/* ---- Empty and missing states ---- */

.gallery-empty {
  display: grid;
  justify-items: center;
  gap: 10px;
  padding: clamp(32px, 8vh, 72px) 24px;
  border: 1.5px dashed var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--surface);
  text-align: center;
}

.gallery-empty--compact {
  padding: 32px 24px;
  border-style: solid;
}

.gallery-empty__art {
  display: grid;
  place-items: center;
  width: 76px;
  height: 76px;
  border-radius: var(--radius-lg);
  background: var(--brand-soft);
  color: var(--brand);
}

.gallery-empty h2 {
  margin: 4px 0 0;
  font-size: 20px;
  font-weight: 600;
}

.gallery-empty p {
  max-width: 520px;
  margin: 0;
  color: var(--text-soft);
  font-size: 14px;
  line-height: 1.65;
  overflow-wrap: anywhere;
}

.gallery-empty__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 6px;
}

/* ---- Drop overlay ---- */

.gallery-drop {
  position: fixed;
  z-index: 85;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  background: color-mix(in srgb, var(--brand) 18%, rgb(6 9 14 / 52%));
  pointer-events: none;
}

.gallery-page--windowed .gallery-drop {
  position: absolute;
}

.gallery-drop__card {
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 28px 40px;
  border: 2px dashed var(--brand);
  border-radius: var(--radius-lg);
  background: var(--surface-raised);
  box-shadow: var(--shadow-md);
  color: var(--brand);
  text-align: center;
}

.gallery-drop__card strong {
  color: var(--text);
  font-size: 18px;
  font-weight: 600;
}

.gallery-drop__card span {
  color: var(--text-soft);
  font-size: 14px;
}

/* ---- Upload queue ---- */

.gallery-uploads {
  position: fixed;
  z-index: 60;
  right: clamp(12px, 2vw, 24px);
  bottom: clamp(12px, 2vw, 24px);
  width: min(380px, calc(100% - 24px));
  overflow: hidden;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--surface-raised);
  box-shadow: var(--shadow-md);
  animation: gallery-uploads-in .22s ease-out both;
}

.gallery-page--windowed .gallery-uploads {
  position: absolute;
  z-index: 20;
}

.gallery-uploads__header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 10px 12px 16px;
}

.gallery-uploads__header > div {
  display: grid;
  min-width: 0;
  flex: 1 1 auto;
  gap: 8px;
}

.gallery-uploads__header strong {
  font-size: 14px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.gallery-uploads__bar {
  display: block;
  height: 4px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--surface-subtle);
}

.gallery-uploads__bar span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--brand);
  transition: width .25s ease;
}

.gallery-uploads__close {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: none;
  color: var(--muted);
  cursor: pointer;
}

.gallery-uploads__list {
  display: grid;
  max-height: 280px;
  margin: 0;
  padding: 0 8px 8px;
  overflow: auto;
  list-style: none;
}

.gallery-uploads__list li {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 48px;
  padding: 6px 8px;
  border-top: 1px solid var(--border);
}

.gallery-uploads__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  color: var(--muted);
}

.gallery-uploads__list li.is-done .gallery-uploads__icon {
  color: var(--success, #15803d);
}

.gallery-uploads__list li.is-error .gallery-uploads__icon {
  color: var(--danger, #c2410c);
}

.gallery-uploads__name {
  display: grid;
  min-width: 0;
  flex: 1 1 auto;
}

.gallery-uploads__name strong {
  overflow: hidden;
  font-size: 14px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gallery-uploads__name small {
  color: var(--muted);
  font-size: 13px;
  overflow-wrap: anywhere;
}

.gallery-uploads__list li.is-error small {
  color: var(--danger, #c2410c);
}

/* ---- Dialog forms ---- */

.gallery-form {
  display: grid;
  gap: 14px;
}

.gallery-form label {
  display: grid;
  gap: 6px;
  font-size: 14px;
  font-weight: 500;
}

.gallery-form__error {
  margin: -6px 0 0;
  color: var(--danger, #c2410c);
  font-size: 13px;
}

.gallery-form__summary {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 12px 14px;
  border-radius: var(--radius-sm);
  background: var(--surface-subtle);
  font-size: 14px;
  overflow-wrap: anywhere;
}

.gallery-form__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

@keyframes gallery-hero-in {
  from { opacity: 0; transform: scale(1.04); }
  to { opacity: 1; transform: none; }
}

@keyframes gallery-uploads-in {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: none; }
}

@container desktop-window (max-width: 640px) {
  .gallery-hero {
    padding: 18px;
  }

  .gallery-albums__grid {
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  }
}

@media (max-width: 720px) {
  .gallery-page {
    --gallery-tile: 104px;
    --gallery-gap: 3px;
  }

  .gallery-page--spacious {
    --gallery-tile: 150px;
  }

  .gallery-hero {
    padding: 18px;
  }

  .gallery-hero__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .gallery-albums__grid {
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 10px;
  }

  .gallery-search {
    flex: 1 1 100%;
  }

  .gallery-toolbar__spacer {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .gallery-hero__backdrop img,
  .gallery-uploads,
  .album-card__cover img,
  .album-card__cover video {
    animation: none;
    transition: none;
  }
}
</style>
