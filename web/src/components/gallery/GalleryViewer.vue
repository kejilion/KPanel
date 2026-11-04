<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  FolderOpen,
  GalleryHorizontalEnd,
  ImageOff,
  ImageUp,
  Info,
  FolderInput,
  Maximize,
  Minimize,
  RotateCcw,
  Trash2,
  X,
  ZoomIn,
  ZoomOut,
} from '@lucide/vue'
import { activateModal, deactivateModal } from '@/components/common/modalStack'
import { formatBytes, formatDateTime } from '@/lib/format'
import {
  formatGalleryDuration,
  galleryBaseName,
  galleryBrowserCanShow,
  galleryFileExtension,
  type GalleryItem,
} from '@/lib/gallery'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { fetchGalleryImage, galleryAllowsPrefetch } from '@/lib/galleryMedia'
import { provideGalleryRenderPolicy } from '@/lib/galleryRenderPolicy'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { GalleryImageCache, type GalleryImageLease } from '@/lib/galleryImageCache'
import GalleryImage from './GalleryImage.vue'

const windowActive = inject(desktopWindowActiveKey, computed(() => true))
const foreground = ref(document.visibilityState !== 'hidden')
function syncVisibility(): void { foreground.value = document.visibilityState !== 'hidden' }
provideGalleryRenderPolicy(computed(() => windowActive.value && foreground.value))

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

/** One place the open photo can be made the cover of, already labelled for the reader. */
export interface GalleryCoverOption {
  id: string
  label: string
  /** The photo already is that cover; choosing it puts the folder back on its automatic cover. */
  active: boolean
}

export interface GalleryViewerSources {
  /** Small image shown at once while the original loads, and in the filmstrip. */
  preview?: string
  /** Versioned medium image for fit-to-screen viewing; original is fetched on zoom. */
  screen?: string
  original: string
}

const props = defineProps<{
  items: GalleryItem[]
  index: number
  sources: (item: GalleryItem) => GalleryViewerSources
  /** Inside a desktop window the viewer covers the window, otherwise the viewport. */
  contained?: boolean
  /** Folder shown as the gallery itself rather than by its name. */
  libraryRoot?: string
  canDelete?: boolean
  /** Shows the "move to album" button; the page owns the folder chooser. */
  canMove?: boolean
  /** Whether the thumbnail strip is shown; the page remembers the choice. */
  filmstrip?: boolean
  navigationLabels?: { previous: string; next: string }
  /** Covers the open photo can be made; empty hides the button (videos, formats a browser cannot draw). */
  coverOptions?: GalleryCoverOption[]
}>()

const emit = defineEmits<{
  close: []
  navigate: [index: number]
  download: [item: GalleryItem]
  reveal: [item: GalleryItem]
  delete: [item: GalleryItem]
  move: [item: GalleryItem]
  'update:filmstrip': [visible: boolean]
  cover: [payload: { item: GalleryItem; option: string }]
}>()

const MIN_SCALE = 1
const MAX_SCALE = 6
const STRIP_RADIUS = 40
/** Controls fade after this long without pointer, focus or key activity. */
const CHROME_IDLE_MS = 2800

const root = ref<HTMLElement>()
const stage = ref<HTMLElement>()
const strip = ref<HTMLElement>()
const infoOpen = ref(false)
const fullscreen = ref(false)
const loaded = ref(false)
const failed = ref(false)
const failureDetail = ref('')
const naturalSize = ref<{ width: number; height: number }>()
const durationSeconds = ref<number>()
const scale = ref(1)
const offset = ref({ x: 0, y: 0 })
const retryKey = ref(0)
const modalId = Symbol('gallery-viewer')
const chromeVisible = ref(true)
const coverMenuOpen = ref(false)
let chromeHeld = false
let chromeTimer: number | undefined
let opener: HTMLElement | null = null
let pointer: { id: number; x: number; y: number; startX: number; startY: number; moved: boolean; pinched: boolean; started: number } | undefined
const contacts = new Map<number, { x: number; y: number }>()
let pinch: { distance: number; x: number; y: number } | undefined
const interacting = ref(false)
const swipeOffset = ref(0)
const dismissOffset = ref(0)
// Separate from the page thumbnail cache, and disposed as soon as the viewer closes.
const originalCache = new GalleryImageCache({ count: 3, bytes: 8 * 1024 * 1024, decoded: 32 * 1024 * 1024, itemBytes: 4 * 1024 * 1024, ttl: 60_000 })
const originalUrl = ref<string>()
const screenUrl = ref<string>()
const originalError = ref(false)
const screenCache = new GalleryImageCache({ count: 3, bytes: 6 * 1024 * 1024, decoded: 24 * 1024 * 1024, itemBytes: 2 * 1024 * 1024, ttl: 60_000, retainUrls: true })
let screenLease: GalleryImageLease | undefined
let screenEpoch = 0
const pendingScreens = new Map<string, { controller: AbortController; promise: Promise<Blob> }>()
function screenKey(source: string, value: GalleryItem): string { return `${source}\0${value.entry.resourceVersion}` }
function requestScreen(source: string, key: string, priority = false): Promise<Blob> {
  const pending = pendingScreens.get(key)
  if (pending) return pending.promise
  const controller = new AbortController()
  const promise = fetchGalleryImage(source, controller.signal, priority).finally(() => {
    if (pendingScreens.get(key)?.controller === controller) pendingScreens.delete(key)
  })
  pendingScreens.set(key, { controller, promise })
  return promise
}
async function loadCurrent(): Promise<void> {
  const epoch = ++screenEpoch
  releaseOriginal()
  screenLease?.release(); screenLease = undefined; screenUrl.value = undefined
  originalError.value = false
  const value = item.value, source = itemSources.value?.screen
  const relevant = new Set(props.items.slice(Math.max(0, props.index - 1), props.index + 2).flatMap(value => {
    const source = props.sources(value).screen
    return source ? [screenKey(source, value)] : []
  }))
  for (const [key, pending] of pendingScreens) if (!relevant.has(key)) { pending.controller.abort(); pendingScreens.delete(key) }
  if (!value || value.kind !== 'image' || !canShow.value) return
  if (!source) { await loadOriginal(); return }
  const key = screenKey(source, value)
  screenLease = screenCache.acquire(key)
  if (screenLease) { screenUrl.value = screenLease.url; return }
  try {
    const blob = await requestScreen(source, key, true)
    if (epoch !== screenEpoch) return
    screenLease = screenCache.store(key, blob)
    screenUrl.value = screenLease.url
  } catch {
    // Old Agents, unsupported/oversized images and generation failures remain viewable.
    if (epoch === screenEpoch) await loadOriginal()
  }
}
let originalLease: GalleryImageLease | undefined
let originalController: AbortController | undefined

function releaseOriginal(): void {
  originalController?.abort()
  originalController = undefined
  originalLease?.release()
  originalLease = undefined
  originalUrl.value = undefined
}

async function loadOriginal(): Promise<void> {
  releaseOriginal()
  const currentItem = item.value
  const source = itemSources.value?.original
  originalError.value = false
  if (!currentItem || currentItem.kind !== 'image' || !canShow.value || !source) return
  const { resourceVersion, sizeBytes } = currentItem.entry
  // Preserve native loading for unknown sizes and large originals, including progressive decoding.
  if (!resourceVersion || sizeBytes <= 0 || sizeBytes > 12 * 1024 * 1024 || (!screenUrl.value && sizeBytes > 4 * 1024 * 1024)) {
    originalUrl.value = source
    return
  }
  const key = `${source}\0${resourceVersion}`
  const cached = originalCache.acquire(key)
  if (cached) { originalLease = cached; originalUrl.value = cached.url; return }
  const controller = new AbortController()
  originalController = controller
  try {
    // The visible original must not queue behind offscreen/filmstrip thumbnails.
    const blob = await fetchGalleryImage(source, controller.signal, true)
    if (controller.signal.aborted || originalController !== controller) return
    originalLease = originalCache.store(key, blob)
    originalUrl.value = originalLease.url
  } catch {
    if (controller.signal.aborted || originalController !== controller) return
    if (screenUrl.value) { originalError.value = true; resetView(); return }
    // Keep native loading available when no screen preview exists.
    originalUrl.value = source
  } finally {
    if (originalController === controller) originalController = undefined
  }
}

const item = computed(() => props.items[props.index])
const itemSources = computed(() => item.value ? props.sources(item.value) : undefined)
const canShow = computed(() => Boolean(item.value && galleryBrowserCanShow(item.value)))
const zoomed = computed(() => scale.value > 1.01)
const position = computed(() => `${props.index + 1} / ${props.items.length}`)
const formatLabel = computed(() => item.value ? (galleryFileExtension(item.value.entry.name).toUpperCase() || item.value.entry.mime || '') : '')
const stripItems = computed(() => {
  const start = Math.max(0, props.index - STRIP_RADIUS)
  return props.items.slice(start, props.index + STRIP_RADIUS + 1).map((entry, offsetIndex) => ({
    item: entry,
    index: start + offsetIndex,
  }))
})
const mediaTransform = computed(() => `translate3d(${offset.value.x + swipeOffset.value}px, ${offset.value.y + dismissOffset.value}px, 0) scale(${scale.value})`)

function cancelGesture(): void {
  for (const id of contacts.keys()) {
    if (stage.value?.hasPointerCapture?.(id)) stage.value.releasePointerCapture(id)
  }
  contacts.clear()
  pointer = undefined
  pinch = undefined
  interacting.value = false
  swipeOffset.value = 0
  dismissOffset.value = 0
}

function pinchPoints() {
  if (contacts.size !== 2) return undefined
  const [a, b] = [...contacts.values()]
  if (!a || !b) return undefined
  return { distance: Math.max(1, Math.hypot(b.x - a.x, b.y - a.y)), x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }
}

function resetView(): void {
  scale.value = 1
  offset.value = { x: 0, y: 0 }
}

/** Keyboard focus inside the controls keeps them up; a button merely clicked with the mouse does not. */
function chromeFocused(): boolean {
  const active = document.activeElement
  if (!(active instanceof HTMLElement) || !active.closest('.gallery-viewer__bar, .gallery-viewer__strip, .gallery-viewer__nav')) return false
  try {
    return active.matches(':focus-visible')
  } catch {
    // Browsers without :focus-visible fall back to treating focus as keyboard focus.
    return true
  }
}

/**
 * The bar, arrows and filmstrip float over the photo and fade after a short
 * idle spell so the whole picture shows; any pointer movement, key or tap on
 * a touch screen brings them back. They stay while hovered or focused.
 */
function showChrome(): void {
  chromeVisible.value = true
  window.clearTimeout(chromeTimer)
  chromeTimer = window.setTimeout(() => {
    if (!chromeHeld && !chromeFocused() && !coverMenuOpen.value) chromeVisible.value = false
  }, CHROME_IDLE_MS)
}

function chooseCoverOption(option: GalleryCoverOption): void {
  coverMenuOpen.value = false
  if (item.value) emit('cover', { item: item.value, option: option.id })
}

/** A press anywhere but the cover button and its menu closes the menu. */
function onViewerPointerDown(event: PointerEvent): void {
  if (coverMenuOpen.value && !(event.target as HTMLElement | null)?.closest('.gallery-viewer__cover')) coverMenuOpen.value = false
}

function holdChrome(held: boolean): void {
  chromeHeld = held
  if (!held) showChrome()
}

function onViewerPointerMove(event: PointerEvent): void {
  if (event.pointerType !== 'touch') showChrome()
}

function onViewerFocusIn(event: FocusEvent): void {
  if (event.target instanceof HTMLElement && event.target.closest('.gallery-viewer__bar, .gallery-viewer__strip, .gallery-viewer__nav')) showChrome()
}

function resetMedia(): void {
  cancelGesture()
  loaded.value = false
  failed.value = false
  failureDetail.value = ''
  naturalSize.value = undefined
  durationSeconds.value = undefined
  resetView()
}

watch(() => props.filmstrip, (visible) => {
  if (visible) void nextTick(scrollStripToCurrent)
})

watch([() => itemSources.value?.original, () => item.value?.entry.resourceVersion, () => item.value?.kind], () => {
  coverMenuOpen.value = false
  resetMedia()
  neighbourController?.abort()
  void nextTick(scrollStripToCurrent)
})

function go(step: number): void {
  const next = props.index + step
  if (next < 0 || next >= props.items.length) return
  emit('navigate', next)
}

function clampOffset(nextScale: number, x: number, y: number): { x: number; y: number } {
  const bounds = stage.value?.getBoundingClientRect()
  if (!bounds || nextScale <= 1) return { x: 0, y: 0 }
  const image = stage.value?.querySelector<HTMLImageElement>('img.gallery-viewer__media:not(.gallery-viewer__media--preview)')
  const width = image?.clientWidth || bounds.width
  const height = image?.clientHeight || bounds.height
  const limitX = Math.max(0, (width * nextScale - bounds.width) / 2)
  const limitY = Math.max(0, (height * nextScale - bounds.height) / 2)
  return {
    x: Math.max(-limitX, Math.min(limitX, x)),
    y: Math.max(-limitY, Math.min(limitY, y)),
  }
}

/** Zoom while keeping the stage point under the cursor fixed. */
function zoomTo(nextScale: number, clientX?: number, clientY?: number): void {
  if (item.value?.kind !== 'image' || !loaded.value) return
  const target = Math.max(MIN_SCALE, Math.min(MAX_SCALE, nextScale))
  const bounds = stage.value?.getBoundingClientRect()
  if (!bounds) return
  const pointX = (clientX ?? bounds.left + bounds.width / 2) - (bounds.left + bounds.width / 2)
  const pointY = (clientY ?? bounds.top + bounds.height / 2) - (bounds.top + bounds.height / 2)
  const ratio = target / scale.value
  const x = pointX - (pointX - offset.value.x) * ratio
  const y = pointY - (pointY - offset.value.y) * ratio
  scale.value = target
  offset.value = clampOffset(target, x, y)
}

function onWheel(event: WheelEvent): void {
  if (item.value?.kind !== 'image') return
  event.preventDefault()
  zoomTo(scale.value * Math.exp(-event.deltaY * 0.0016), event.clientX, event.clientY)
}

function onDoubleClick(event: MouseEvent): void {
  if (item.value?.kind !== 'image') return
  if (zoomed.value) resetView()
  else zoomTo(2.5, event.clientX, event.clientY)
}

function onPointerDown(event: PointerEvent): void {
  if (event.button !== 0 || item.value?.kind !== 'image') return
  if ((event.target as Element | null)?.closest('button, a, input')) return
  contacts.set(event.pointerId, { x: event.clientX, y: event.clientY })
  interacting.value = true
  stage.value?.setPointerCapture?.(event.pointerId)
  if (contacts.size > 1) {
    pointer = undefined
    swipeOffset.value = 0
    dismissOffset.value = 0
    pinch = pinchPoints()
    return
  }
  pointer = { id: event.pointerId, x: event.clientX, y: event.clientY, startX: event.clientX, startY: event.clientY, moved: false, pinched: false, started: performance.now() }
}

function onPointerMove(event: PointerEvent): void {
  if (!contacts.has(event.pointerId)) return
  contacts.set(event.pointerId, { x: event.clientX, y: event.clientY })
  if (contacts.size > 1) {
    const next = pinchPoints()
    if (pinch && next) {
      zoomTo(scale.value * next.distance / pinch.distance, pinch.x, pinch.y)
      offset.value = clampOffset(scale.value, offset.value.x + next.x - pinch.x, offset.value.y + next.y - pinch.y)
    }
    pinch = next
    return
  }
  if (!pointer || pointer.id !== event.pointerId) return
  const deltaX = event.clientX - pointer.x
  const deltaY = event.clientY - pointer.y
  pointer.x = event.clientX
  pointer.y = event.clientY
  if (Math.abs(event.clientX - pointer.startX) + Math.abs(event.clientY - pointer.startY) > 6) pointer.moved = true
  if (zoomed.value) offset.value = clampOffset(scale.value, offset.value.x + deltaX, offset.value.y + deltaY)
  else if (!pointer.pinched) {
    const x = event.clientX - pointer.startX
    const y = event.clientY - pointer.startY
    const edge = (x > 0 && props.index === 0) || (x < 0 && props.index === props.items.length - 1)
    const limit = (stage.value?.clientWidth || 360) * 0.4
    swipeOffset.value = Math.abs(x) > Math.abs(y) * 1.5 ? Math.max(-limit, Math.min(limit, x * (edge ? 0.25 : 1))) : 0
    dismissOffset.value = event.pointerType === 'touch' && y < 0 && Math.abs(y) > Math.abs(x) * 1.5
      ? Math.max(-240, y) : 0
  }
}

function onPointerUp(event: PointerEvent): void {
  if (!contacts.delete(event.pointerId)) return
  if (stage.value?.hasPointerCapture?.(event.pointerId)) stage.value.releasePointerCapture(event.pointerId)
  if (pinch || contacts.size) {
    pinch = undefined
    const remaining = [...contacts.entries()][0]
    pointer = remaining ? { id: remaining[0], ...remaining[1], startX: remaining[1].x, startY: remaining[1].y, moved: true, pinched: true, started: performance.now() } : undefined
    interacting.value = contacts.size > 0
    return
  }
  interacting.value = false
  swipeOffset.value = 0
  dismissOffset.value = 0
  if (!pointer || pointer.id !== event.pointerId) return
  const swipe = event.clientX - pointer.startX
  const vertical = Math.abs(event.clientY - pointer.startY)
  const upward = pointer.startY - event.clientY
  const wasZoomed = zoomed.value
  const tapped = !pointer.moved
  const pinched = pointer.pinched
  const elapsed = Math.max(1, performance.now() - pointer.started)
  pointer = undefined
  // A horizontal drag on an unzoomed photo turns the page like a phone gallery.
  const threshold = Math.max(32, Math.min(64, (stage.value?.clientWidth || 360) * 0.15))
  if (!pinched && !wasZoomed && event.pointerType === 'touch' && upward > Math.abs(swipe) * 1.5
    && (upward > 80 || (upward > 32 && upward / elapsed > 0.5))) emit('close')
  else if (!pinched && !wasZoomed && Math.abs(swipe) > vertical * 1.5
    && (Math.abs(swipe) > threshold || (Math.abs(swipe) > 24 && Math.abs(swipe) / elapsed > 0.5))) go(swipe < 0 ? 1 : -1)
  // Touch screens have no hover, so a tap toggles the controls instead.
  else if (tapped && event.pointerType === 'touch') {
    if (chromeVisible.value) chromeVisible.value = false
    else showChrome()
  }
}

function isCurrentImage(event: Event): boolean {
  const image = event.currentTarget as HTMLImageElement
  const source = originalUrl.value || screenUrl.value
  return Boolean(source && stage.value?.contains(image) && image.getAttribute('src') === source)
}

function onImageLoad(event: Event): void {
  if (!isCurrentImage(event)) return
  const image = event.currentTarget as HTMLImageElement
  const lease = originalUrl.value ? originalLease : screenLease
  lease?.decoded(image.naturalWidth, image.naturalHeight)
  naturalSize.value = { width: image.naturalWidth, height: image.naturalHeight }
  loaded.value = true
  preloadNeighbours()
}

function onImageError(event: Event): void {
  if (!isCurrentImage(event)) return
  // A Blob decode failure need not mean the authenticated image URL is unusable.
  // Try native loading once; a native failure continues through the usual fallback.
  if (!originalUrl.value && screenUrl.value?.startsWith('blob:')) {
    screenLease?.invalidate(); screenLease?.release(); screenLease = undefined
    screenUrl.value = itemSources.value?.screen
    return
  }
  if (!originalUrl.value && screenUrl.value) {
    screenLease?.invalidate(); screenLease?.release(); screenLease = undefined; screenUrl.value = undefined
    void loadOriginal(); return
  }
  if (originalUrl.value?.startsWith('blob:')) {
    originalLease?.invalidate(); releaseOriginal()
    originalUrl.value = itemSources.value?.original
    return
  }
  if (originalUrl.value && screenUrl.value) {
    originalLease?.invalidate(); releaseOriginal(); resetView(); originalError.value = true; return
  }
  originalLease?.invalidate()
  releaseOriginal()
  failed.value = true
  failureDetail.value = '原图读取失败，请检查网络或文件是否仍然存在。'
}

function onVideoMetadata(event: Event): void {
  const video = event.currentTarget as HTMLVideoElement
  durationSeconds.value = Number.isFinite(video.duration) ? video.duration : undefined
  if (video.videoWidth > 0) naturalSize.value = { width: video.videoWidth, height: video.videoHeight }
}

function onVideoReady(event: Event): void {
  const video = event.currentTarget as HTMLVideoElement
  if (video.videoWidth <= 0) {
    video.pause()
    failed.value = true
    failureDetail.value = '浏览器只能播放音轨，无法解码视频画面。请下载原文件，或转换为 H.264 + AAC 的 MP4。'
    return
  }
  loaded.value = true
}

function onVideoError(event: Event): void {
  const video = event.currentTarget as HTMLVideoElement
  failed.value = true
  failureDetail.value = video.error?.code === 4
    ? '浏览器不支持该视频编码或格式。请下载原文件，或转换为 H.264 + AAC 的 MP4。'
    : '视频读取失败，请检查网络或文件是否仍然存在。'
}

function retryOriginal(): void {
  void loadOriginal()
  void nextTick(() => root.value?.focus({ preventScroll: true }))
}

function retry(): void {
  resetMedia()
  originalLease?.invalidate()
  retryKey.value += 1
  // The retry button disappears; keep keyboard paging and Escape inside the modal.
  void nextTick(() => root.value?.focus({ preventScroll: true }))
}

let neighbourController: AbortController | undefined
watch([() => itemSources.value?.original, () => item.value?.entry.resourceVersion, () => item.value?.kind, retryKey], () => { void loadCurrent() }, { immediate: true })
watch(zoomed, value => { if (value && screenUrl.value && !originalUrl.value && !originalController) void loadOriginal() })
async function preloadScreens(): Promise<void> {
  const epoch = screenEpoch
  if (!galleryAllowsPrefetch(256 * 1024)) return
  // Serialize neighbours. A visible request can join pending work by versioned URL.
  for (const step of [1, -1]) {
    if (epoch !== screenEpoch) return
    const value = props.items[props.index + step]
    if (!value || value.kind !== 'image') continue
    const source = props.sources(value).screen
    if (!source) continue
    const key = screenKey(source, value)
    const cached = screenCache.acquire(key)
    if (cached) { cached.release(); continue }
    try {
      const blob = await requestScreen(source, key)
      if (epoch !== screenEpoch) return
      const lease = screenCache.store(key, blob); lease.release()
    } catch { /* Optional preloading must not interfere with the visible image. */ }
  }
}
function preloadNeighbours(): void {
  if (itemSources.value?.screen) { void preloadScreens(); return }
  neighbourController?.abort()
  neighbourController = new AbortController()
  for (const step of [1, -1]) {
    const neighbour = props.items[props.index + step]
    if (!neighbour || neighbour.kind !== 'image' || !galleryBrowserCanShow(neighbour) || !galleryAllowsPrefetch(neighbour.entry.sizeBytes)) continue
    // Warm the versioned thumbnail cache, never download neighbouring originals.
    const preview = props.sources(neighbour).preview
    if (preview) void fetchGalleryImage(preview, neighbourController.signal).catch(() => undefined)
  }
}

/**
 * Centre the current thumbnail by scrolling the strip alone. scrollIntoView
 * also scrolls every ancestor, and an overflow-hidden ancestor still accepts
 * that, which dragged the whole viewer sideways.
 */
function scrollStripToCurrent(): void {
  const list = strip.value
  const current = list?.querySelector<HTMLElement>('[aria-current="true"]')
  if (!list || !current) return
  const left = current.offsetLeft - (list.clientWidth - current.offsetWidth) / 2
  if (typeof list.scrollTo === 'function') list.scrollTo({ left: Math.max(0, left), behavior: 'smooth' })
  else list.scrollLeft = Math.max(0, left)
}

async function toggleFullscreen(): Promise<void> {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await root.value?.requestFullscreen()
  } catch {
    // Fullscreen can be refused by policy; the viewer already fills its area.
  }
}

function syncFullscreen(): void {
  fullscreen.value = Boolean(root.value && document.fullscreenElement === root.value)
}

function focusableElements(): HTMLElement[] {
  return Array.from(root.value?.querySelectorAll<HTMLElement>('button:not(:disabled), video[controls], [tabindex="0"]') ?? [])
    .filter((element) => element.offsetParent !== null || element === document.activeElement)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.defaultPrevented) return
  const target = event.target as HTMLElement | null
  const inVideo = target?.tagName === 'VIDEO'
  // Paging with the arrows keeps an immersive slideshow; other keys reveal the controls.
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') showChrome()
  if (event.key === 'Tab') {
    const focusable = focusableElements()
    if (!focusable.length) return
    const first = focusable[0]!
    const last = focusable[focusable.length - 1]!
    if (event.shiftKey && (document.activeElement === first || document.activeElement === root.value)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    if (coverMenuOpen.value) coverMenuOpen.value = false
    else if (zoomed.value) resetView()
    else if (!document.fullscreenElement) emit('close')
    return
  }
  if (inVideo && ['ArrowLeft', 'ArrowRight', ' '].includes(event.key)) return
  if (event.key === 'ArrowLeft') {
    event.preventDefault()
    go(-1)
  } else if (event.key === 'ArrowRight') {
    event.preventDefault()
    go(1)
  } else if (event.key === '+' || event.key === '=') {
    zoomTo(scale.value * 1.4)
  } else if (event.key === '-') {
    zoomTo(scale.value / 1.4)
  } else if (event.key === '0') {
    resetView()
  } else if (event.key === 'i' || event.key === 'I') {
    infoOpen.value = !infoOpen.value
  } else if (event.key === 'Delete' && props.canDelete && item.value) {
    event.preventDefault()
    emit('delete', item.value)
  }
}

onMounted(() => {
  document.addEventListener('visibilitychange', syncVisibility)
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (!props.contained) activateModal(modalId)
  document.addEventListener('fullscreenchange', syncFullscreen)
  root.value?.focus({ preventScroll: true })
  showChrome()
  void nextTick(scrollStripToCurrent)
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', syncVisibility)
  releaseOriginal()
  originalCache.dispose()
  screenEpoch += 1
  screenLease?.release(); screenCache.dispose()
  for (const pending of pendingScreens.values()) pending.controller.abort()
  pendingScreens.clear()
  cancelGesture()
  neighbourController?.abort()
  window.clearTimeout(chromeTimer)
  if (!props.contained) deactivateModal(modalId)
  document.removeEventListener('fullscreenchange', syncFullscreen)
  if (document.fullscreenElement && document.fullscreenElement === root.value) void document.exitFullscreen().catch(() => undefined)
  if (opener?.isConnected) opener.focus({ preventScroll: true })
})

</script>

<template>
  <div
    ref="root"
    class="gallery-viewer"
    :class="{
      'gallery-viewer--contained': contained,
      'gallery-viewer--info': infoOpen,
      'gallery-viewer--idle': !chromeVisible,
    }"
    role="dialog"
    aria-modal="true"
    :aria-label="phrase('照片与视频查看器')"
    tabindex="-1"
    @keydown="onKeydown"
    @pointerdown="onViewerPointerDown"
    @pointermove="onViewerPointerMove"
    @focusin="onViewerFocusIn"
  >
    <div v-if="itemSources?.preview" class="gallery-viewer__ambient" aria-hidden="true">
      <img :src="itemSources.preview" alt="" />
    </div>

    <header class="gallery-viewer__bar" @pointerenter="holdChrome(true)" @pointerleave="holdChrome(false)">
      <div class="gallery-viewer__title">
        <strong :title="item?.entry.name">{{ item?.entry.name }}</strong>
        <span>{{ item ? formatDateTime(item.entry.modifiedAt) : '' }} · {{ position }}</span>
      </div>
      <div class="gallery-viewer__actions">
        <template v-if="item?.kind === 'image' && canShow && !failed">
          <button type="button" :title="phrase('缩小')" :aria-label="phrase('缩小')" :disabled="!zoomed" @click="zoomTo(scale / 1.4)"><ZoomOut :size="18" /></button>
          <button type="button" :title="phrase('放大')" :aria-label="phrase('放大')" :disabled="!loaded || scale >= 6" @click="zoomTo(scale * 1.4)"><ZoomIn :size="18" /></button>
          <button type="button" :title="phrase('适应窗口')" :aria-label="phrase('适应窗口')" :disabled="!zoomed" @click="resetView"><RotateCcw :size="17" /></button>
          <span class="gallery-viewer__divider" aria-hidden="true" />
        </template>
        <button
          v-if="items.length > 1"
          type="button"
          :class="{ 'is-active': filmstrip }"
          :title="phrase(filmstrip ? '隐藏缩略图条' : '显示缩略图条')"
          :aria-label="phrase(filmstrip ? '隐藏缩略图条' : '显示缩略图条')"
          :aria-pressed="filmstrip"
          @click="emit('update:filmstrip', !filmstrip)"
        ><GalleryHorizontalEnd :size="18" /></button>
        <div v-if="coverOptions?.length" class="gallery-viewer__cover">
          <button
            type="button"
            :class="{ 'is-active': coverMenuOpen || coverOptions.some((option) => option.active) }"
            :title="phrase('设为封面')"
            :aria-label="phrase('设为封面')"
            aria-haspopup="menu"
            :aria-expanded="coverMenuOpen"
            @click="coverMenuOpen = !coverMenuOpen"
          ><ImageUp :size="18" /></button>
          <div v-if="coverMenuOpen" class="gallery-viewer__cover-menu" role="menu" :aria-label="phrase('设为封面')">
            <button
              v-for="option in coverOptions"
              :key="option.id"
              type="button"
              role="menuitemcheckbox"
              :aria-checked="option.active"
              @click="chooseCoverOption(option)"
            >
              <Check v-if="option.active" :size="16" aria-hidden="true" />
              <span v-else class="gallery-viewer__cover-spacer" aria-hidden="true" />
              <span class="gallery-viewer__cover-label">
                <strong>{{ option.label }}</strong>
                <small v-if="option.active">{{ phrase('当前封面，再点一次恢复自动') }}</small>
              </span>
            </button>
          </div>
        </div>
        <button
          type="button"
          :class="{ 'is-active': infoOpen }"
          :title="phrase('详细信息')"
          :aria-label="phrase('详细信息')"
          :aria-pressed="infoOpen"
          @click="infoOpen = !infoOpen"
        ><Info :size="18" /></button>
        <button type="button" :title="phrase('下载原文件')" :aria-label="phrase('下载原文件')" @click="item && emit('download', item)"><Download :size="18" /></button>
        <button type="button" :title="phrase('在文件管理中显示')" :aria-label="phrase('在文件管理中显示')" @click="item && emit('reveal', item)"><FolderOpen :size="18" /></button>
        <button
          v-if="canMove"
          type="button"
          :title="phrase('移动到相册')"
          :aria-label="phrase('移动到相册')"
          @click="item && emit('move', item)"
        ><FolderInput :size="18" /></button>
        <button
          v-if="canDelete"
          type="button"
          class="gallery-viewer__danger"
          :title="phrase('移入回收站')"
          :aria-label="phrase('移入回收站')"
          @click="item && emit('delete', item)"
        ><Trash2 :size="18" /></button>
        <button
          type="button"
          :title="phrase(fullscreen ? '退出全屏' : '全屏')"
          :aria-label="phrase(fullscreen ? '退出全屏' : '全屏')"
          @click="toggleFullscreen"
        >
          <Minimize v-if="fullscreen" :size="18" />
          <Maximize v-else :size="18" />
        </button>
        <button type="button" class="gallery-viewer__close" :title="phrase('关闭')" :aria-label="phrase('关闭')" @click="emit('close')"><X :size="20" /></button>
      </div>
    </header>

    <div class="gallery-viewer__body">
      <div
        ref="stage"
        class="gallery-viewer__stage"
        :class="{ 'is-zoomed': zoomed, 'is-image': item?.kind === 'image', 'is-interacting': interacting }"
        @wheel="onWheel"
        @dblclick="onDoubleClick"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="cancelGesture"
        @lostpointercapture="contacts.has($event.pointerId) && cancelGesture()"
      >
        <template v-if="item && canShow && !failed">
          <template v-if="item.kind === 'image'">
            <img
              v-if="itemSources?.preview && !loaded"
              class="gallery-viewer__media gallery-viewer__media--preview"
              :src="itemSources.preview"
              alt=""
              aria-hidden="true"
              draggable="false"
            />
            <img
              v-if="originalUrl || screenUrl"
              :key="`${originalUrl || screenUrl}:${retryKey}`"
              class="gallery-viewer__media"
              :class="{ 'is-loaded': loaded }"
              :src="originalUrl || screenUrl"
              :alt="item.entry.name"
              :style="{ transform: mediaTransform }"
              decoding="async"
              draggable="false"
              @load="onImageLoad"
              @error="onImageError"
            />
            <div v-if="originalError" class="gallery-viewer__retry gallery-viewer__notice-actions" :class="{ 'gallery-viewer__retry--filmstrip': filmstrip }" role="alert">
              <span>{{ phrase('原图读取失败，请检查网络或文件是否仍然存在。') }}</span>
              <button class="btn btn-secondary" @click="retryOriginal">{{ phrase('重试') }}</button>
            </div>
            <span v-if="!loaded" class="gallery-viewer__spinner" role="status" :aria-label="phrase('正在加载原图')" />
          </template>
          <video
            v-else
            :key="`${item.entry.path}:${retryKey}`"
            class="gallery-viewer__media gallery-viewer__media--video is-loaded"
            :src="itemSources?.original"
            :poster="itemSources?.preview"
            controls
            autoplay
            playsinline
            preload="metadata"
            @loadedmetadata="onVideoMetadata"
            @loadeddata="onVideoReady"
            @error="onVideoError"
          />
        </template>
        <div v-else-if="item" class="gallery-viewer__notice" role="status">
          <ImageOff :size="38" aria-hidden="true" />
          <strong>{{ phrase(failed ? '无法显示这个文件' : '浏览器无法直接显示该格式') }}</strong>
          <p>{{ phrase(failed ? failureDetail : '可以下载原文件，在本地应用中查看。') }}</p>
          <span class="gallery-viewer__format">{{ formatLabel }} · {{ formatBytes(item.entry.sizeBytes) }}</span>
          <div class="gallery-viewer__notice-actions">
            <button v-if="failed" type="button" class="gallery-viewer__pill" @click="retry">{{ phrase('重试') }}</button>
            <button type="button" class="gallery-viewer__pill gallery-viewer__pill--primary" @click="emit('download', item)">
              <Download :size="16" /> {{ phrase('下载原文件') }}
            </button>
          </div>
        </div>

        <button
          v-if="index > 0"
          class="gallery-viewer__nav gallery-viewer__nav--prev"
          type="button"
          :title="navigationLabels?.previous ?? phrase('上一张')"
          :aria-label="navigationLabels?.previous ?? phrase('上一张')"
          @pointerdown.stop
          @pointerenter="holdChrome(true)"
          @pointerleave="holdChrome(false)"
          @click="go(-1)"
        ><ChevronLeft :size="28" /></button>
        <button
          v-if="index < items.length - 1"
          class="gallery-viewer__nav gallery-viewer__nav--next"
          type="button"
          :title="navigationLabels?.next ?? phrase('下一张')"
          :aria-label="navigationLabels?.next ?? phrase('下一张')"
          @pointerdown.stop
          @pointerenter="holdChrome(true)"
          @pointerleave="holdChrome(false)"
          @click="go(1)"
        ><ChevronRight :size="28" /></button>
      </div>

      <aside v-if="infoOpen && item" class="gallery-viewer__info" :aria-label="phrase('详细信息')">
        <h2>{{ phrase('详细信息') }}</h2>
        <dl>
          <div><dt>{{ phrase('文件名') }}</dt><dd>{{ item.entry.name }}</dd></div>
          <div><dt>{{ phrase('所在相册') }}</dt><dd>{{ item.folder === libraryRoot ? phrase('图库') : galleryBaseName(item.folder) }}<small>{{ item.folder }}</small></dd></div>
          <div><dt>{{ phrase('类型') }}</dt><dd>{{ formatLabel }}</dd></div>
          <div><dt>{{ phrase('大小') }}</dt><dd>{{ formatBytes(item.entry.sizeBytes) }}</dd></div>
          <div v-if="naturalSize"><dt>{{ phrase('尺寸') }}</dt><dd>{{ naturalSize.width }} × {{ naturalSize.height }}</dd></div>
          <div v-if="durationSeconds !== undefined"><dt>{{ phrase('时长') }}</dt><dd>{{ formatGalleryDuration(durationSeconds) }}</dd></div>
          <div><dt>{{ phrase('修改时间') }}</dt><dd>{{ formatDateTime(item.entry.modifiedAt) }}</dd></div>
          <div><dt>{{ phrase('所有者') }}</dt><dd>{{ item.entry.owner }}:{{ item.entry.group }}</dd></div>
        </dl>
      </aside>
    </div>

    <nav
      v-if="items.length > 1 && filmstrip"
      ref="strip"
      class="gallery-viewer__strip"
      :aria-label="phrase('缩略图')"
      @pointerenter="holdChrome(true)"
      @pointerleave="holdChrome(false)"
    >
      <button
        v-for="thumb in stripItems"
        :key="thumb.item.entry.path"
        type="button"
        class="gallery-viewer__thumb"
        :class="{ 'gallery-viewer__thumb--video': thumb.item.kind === 'video' }"
        :aria-current="thumb.index === index ? 'true' : undefined"
        :aria-label="thumb.item.entry.name"
        @click="emit('navigate', thumb.index)"
      >
        <GalleryImage v-if="sources(thumb.item).preview" :src="sources(thumb.item).preview!" :version="thumb.item.entry.resourceVersion" alt="" draggable="false" />
        <span v-else>{{ galleryFileExtension(thumb.item.entry.name).toUpperCase() }}</span>
      </button>
    </nav>
  </div>
</template>

<style scoped>
.gallery-viewer {
  --viewer-bg: #06080b;
  --viewer-text: #f4f5f7;
  --viewer-muted: #b3bac5;
  --viewer-control: rgb(255 255 255 / 9%);
  --viewer-control-hover: rgb(255 255 255 / 17%);
  --viewer-line: rgb(255 255 255 / 12%);

  position: fixed;
  z-index: 90;
  inset: 0;
  display: grid;
  grid-template-rows: minmax(0, 1fr);
  overflow: hidden;
  /* clip, unlike hidden, cannot be scrolled by scrollIntoView or focus. */
  overflow: clip;
  color: var(--viewer-text);
  background: var(--viewer-bg);
  isolation: isolate;
  outline: none;
  animation: gallery-viewer-in .2s ease-out both;
}

.gallery-viewer--contained {
  position: absolute;
  z-index: 30;
}

.gallery-viewer__ambient {
  position: absolute;
  z-index: -1;
  inset: -12%;
  opacity: .34;
  pointer-events: none;
}

.gallery-viewer__ambient img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(64px) saturate(1.25);
}

.gallery-viewer__ambient::after {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, transparent 0%, var(--viewer-bg) 78%);
  content: '';
}

/* The bar and the filmstrip float over the photo so the stage keeps the whole area. */
.gallery-viewer__bar {
  position: absolute;
  z-index: 3;
  inset: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  padding: 12px 14px 18px 20px;
  background: linear-gradient(rgb(6 8 11 / 72%), rgb(6 8 11 / 0%));
  transition: opacity .25s ease;
}

.gallery-viewer--idle .gallery-viewer__bar,
.gallery-viewer--idle .gallery-viewer__strip,
.gallery-viewer--idle .gallery-viewer__nav {
  opacity: 0;
  pointer-events: none;
}

.gallery-viewer--idle .gallery-viewer__stage {
  cursor: none;
}

.gallery-viewer__title {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.gallery-viewer__title strong {
  overflow: hidden;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gallery-viewer__title span {
  color: var(--viewer-muted);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.gallery-viewer__actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
}

.gallery-viewer__actions button,
.gallery-viewer__nav {
  display: inline-grid;
  place-items: center;
  width: 40px;
  height: 40px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--viewer-text);
  cursor: pointer;
  transition: background-color .15s ease, opacity .15s ease;
}

.gallery-viewer__actions button:hover:not(:disabled),
.gallery-viewer__actions button.is-active {
  background: var(--viewer-control-hover);
}

.gallery-viewer__actions button:disabled {
  cursor: default;
  opacity: .38;
}

.gallery-viewer__actions .gallery-viewer__danger:hover {
  background: rgb(239 68 68 / 26%);
}

.gallery-viewer__close {
  margin-left: 4px;
  background: var(--viewer-control) !important;
}

.gallery-viewer__cover {
  position: relative;
}

.gallery-viewer__cover-menu {
  position: absolute;
  z-index: 5;
  top: calc(100% + 8px);
  right: 0;
  display: grid;
  gap: 2px;
  width: max-content;
  min-width: 200px;
  max-width: min(340px, calc(100vw - 24px));
  padding: 6px;
  border: 1px solid var(--viewer-line);
  border-radius: var(--radius);
  background: rgb(18 22 28 / 96%);
  box-shadow: var(--shadow-md);
}

.gallery-viewer__actions .gallery-viewer__cover-menu button {
  display: flex;
  width: 100%;
  height: auto;
  min-height: 44px;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  text-align: left;
}

.gallery-viewer__cover-spacer {
  width: 16px;
  flex: 0 0 16px;
}

.gallery-viewer__cover-label {
  display: grid;
  min-width: 0;
  line-height: 1.4;
}

.gallery-viewer__cover-label strong {
  font-size: 14px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.gallery-viewer__cover-label small {
  color: var(--viewer-muted);
  font-size: 13px;
}

.gallery-viewer__divider {
  width: 1px;
  height: 22px;
  margin: 0 6px;
  background: var(--viewer-line);
}

.gallery-viewer button:focus-visible {
  outline: 2px solid #9cc3ff;
  outline-offset: 2px;
}

.gallery-viewer__body {
  display: flex;
  min-width: 0;
  min-height: 0;
}

/* One definite cell, so the media's percentage max-height resolves against the
   stage; with an auto row a wide photo was sized by width alone and cropped. */
.gallery-viewer__stage {
  position: relative;
  display: grid;
  flex: 1 1 auto;
  min-width: 0;
  grid-template: minmax(0, 1fr) / minmax(0, 1fr);
  place-items: center;
  overflow: hidden;
  touch-action: none;
  user-select: none;
}

.gallery-viewer__stage.is-image {
  cursor: zoom-in;
}

.gallery-viewer__stage:not(.is-image) {
  touch-action: auto;
}

.gallery-viewer__stage.is-zoomed {
  cursor: grab;
}

.gallery-viewer__stage.is-zoomed:active {
  cursor: grabbing;
}

.gallery-viewer__media {
  grid-area: 1 / 1;
  max-width: calc(100% - 48px);
  max-height: calc(100% - 24px);
  object-fit: contain;
  opacity: 0;
  border-radius: var(--radius-sm);
  transition: opacity .28s ease, transform .18s cubic-bezier(.2, .7, .2, 1);
  will-change: transform;
}

.gallery-viewer__media.is-loaded,
.gallery-viewer__media--preview {
  opacity: 1;
}

/* The small preview fills the same box the original will occupy. */
.gallery-viewer__media--preview {
  width: calc(100% - 48px);
  height: calc(100% - 24px);
  filter: blur(6px);
}

/* Videos stay clear of both overlays so their native controls remain reachable. */
.gallery-viewer__media--video {
  max-width: calc(100% - 120px);
  max-height: calc(100% - 168px);
  background: #000;
}

.gallery-viewer__stage.is-zoomed .gallery-viewer__media,
.gallery-viewer__stage.is-interacting .gallery-viewer__media {
  transition: opacity .28s ease;
}

.gallery-viewer__spinner {
  grid-area: 1 / 1;
  width: 34px;
  height: 34px;
  border: 3px solid rgb(255 255 255 / 22%);
  border-top-color: var(--viewer-text);
  border-radius: 50%;
  animation: gallery-viewer-spin .8s linear infinite;
}

.gallery-viewer__nav {
  position: absolute;
  top: 50%;
  width: 52px;
  height: 52px;
  background: var(--viewer-control);
  transform: translateY(-50%);
  opacity: .82;
}

.gallery-viewer__nav:hover {
  background: var(--viewer-control-hover);
  opacity: 1;
}

.gallery-viewer__nav--prev {
  left: 18px;
}

.gallery-viewer__nav--next {
  right: 18px;
}

.gallery-viewer__notice {
  display: grid;
  max-width: 420px;
  justify-items: center;
  gap: 10px;
  padding: 24px;
  color: var(--viewer-muted);
  text-align: center;
}

.gallery-viewer__notice strong {
  color: var(--viewer-text);
  font-size: 17px;
  font-weight: 600;
}

.gallery-viewer__notice p {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
}

.gallery-viewer__format {
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.gallery-viewer__retry {
  position: absolute;
  bottom: 1rem;
  z-index: 2;
  inset-inline: 1rem;
  width: fit-content;
  max-width: calc(100% - 2rem);
  margin-inline: auto;
  padding: 0.75rem;
  color: var(--viewer-text);
  background: var(--viewer-bg);
  border: 1px solid var(--viewer-line);
  border-radius: var(--radius-md);
  font-size: 14px;
  line-height: 1.5;
}
/* The strip overlays the stage; keep the recovery action above its 90px box. */
.gallery-viewer__retry--filmstrip { bottom: 104px; }
.gallery-viewer__notice-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 6px;
}

.gallery-viewer__pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 40px;
  padding: 0 18px;
  border: 1px solid var(--viewer-line);
  border-radius: 999px;
  background: var(--viewer-control);
  color: var(--viewer-text);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
}

.gallery-viewer__pill--primary {
  border-color: transparent;
  background: var(--viewer-text);
  color: var(--viewer-bg);
}

.gallery-viewer__info {
  flex: 0 0 300px;
  overflow: auto;
  padding: 76px 20px 20px;
  border-left: 1px solid var(--viewer-line);
  background: rgb(14 17 22 / 86%);
  animation: gallery-viewer-info-in .2s ease-out both;
}

.gallery-viewer__info h2 {
  margin: 4px 0 14px;
  font-size: 16px;
  font-weight: 600;
}

.gallery-viewer__info dl {
  display: grid;
  gap: 14px;
  margin: 0;
}

.gallery-viewer__info dt {
  margin-bottom: 2px;
  color: var(--viewer-muted);
  font-size: 13px;
}

.gallery-viewer__info dd {
  display: grid;
  margin: 0;
  font-size: 14px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.gallery-viewer__info small {
  color: var(--viewer-muted);
  font-size: 12px;
}

.gallery-viewer__strip {
  position: absolute;
  z-index: 3;
  inset: auto 0 0;
  display: flex;
  justify-content: safe center;
  gap: 6px;
  min-width: 0;
  padding: 22px 16px 14px;
  overflow-x: auto;
  background: linear-gradient(rgb(6 8 11 / 0%), rgb(6 8 11 / 72%));
  scrollbar-width: none;
  transition: opacity .25s ease;
}

.gallery-viewer__thumb {
  position: relative;
  flex: 0 0 auto;
  width: 54px;
  height: 54px;
  padding: 0;
  overflow: hidden;
  border: 0;
  border-radius: var(--radius-sm);
  background: var(--viewer-control);
  color: var(--viewer-muted);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  opacity: .5;
  transition: opacity .15s ease, transform .15s ease;
}

.gallery-viewer__thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.gallery-viewer__thumb:hover {
  opacity: .86;
}

.gallery-viewer__thumb[aria-current='true'] {
  opacity: 1;
  outline: 2px solid var(--viewer-text);
  outline-offset: 2px;
  transform: scale(1.06);
}

.gallery-viewer__thumb--video::after {
  position: absolute;
  right: 4px;
  bottom: 4px;
  width: 0;
  height: 0;
  border-top: 5px solid transparent;
  border-bottom: 5px solid transparent;
  border-left: 8px solid #fff;
  content: '';
}

@keyframes gallery-viewer-in {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes gallery-viewer-info-in {
  from { opacity: 0; transform: translateX(12px); }
  to { opacity: 1; transform: none; }
}

@keyframes gallery-viewer-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 720px) {
  .gallery-viewer {
    grid-template-rows: auto minmax(0, 1fr) auto;
  }

  .gallery-viewer__bar {
    position: relative;
    align-items: flex-start;
    padding: 10px 8px 8px 14px;
  }

  /* Touch users must always be able to leave, including during native video playback. */
  .gallery-viewer--idle .gallery-viewer__bar {
    opacity: 1;
    pointer-events: auto;
  }

  .gallery-viewer .gallery-viewer__strip {
    position: relative;
    opacity: 1;
    pointer-events: auto;
  }

  .gallery-viewer__actions {
    max-width: 60%;
  }

  .gallery-viewer__nav {
    display: none;
  }

  .gallery-viewer__media,
  .gallery-viewer__media--video {
    max-width: 100%;
  }

  .gallery-viewer__media--video {
    width: 100%;
    height: 100%;
    max-height: 100%;
  }

  .gallery-viewer__info {
    position: absolute;
    z-index: 4;
    inset: auto 0 0;
    max-height: 55%;
    padding-top: 12px;
    border-top: 1px solid var(--viewer-line);
    border-left: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .gallery-viewer,
  .gallery-viewer__bar,
  .gallery-viewer__strip,
  .gallery-viewer__nav,
  .gallery-viewer__info,
  .gallery-viewer__media {
    animation: none;
    transition: none;
  }
}
</style>
