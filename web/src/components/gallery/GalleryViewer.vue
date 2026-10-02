<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ChevronLeft,
  ChevronRight,
  Download,
  FolderOpen,
  ImageOff,
  Info,
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

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

export interface GalleryViewerSources {
  /** Small image shown at once while the original loads, and in the filmstrip. */
  preview?: string
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
}>()

const emit = defineEmits<{
  close: []
  navigate: [index: number]
  download: [item: GalleryItem]
  reveal: [item: GalleryItem]
  delete: [item: GalleryItem]
}>()

const MIN_SCALE = 1
const MAX_SCALE = 6
const STRIP_RADIUS = 40

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
let opener: HTMLElement | null = null
let pointer: { id: number; x: number; y: number; startX: number; startY: number; moved: boolean } | undefined

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
const mediaTransform = computed(() => `translate3d(${offset.value.x}px, ${offset.value.y}px, 0) scale(${scale.value})`)

function resetView(): void {
  scale.value = 1
  offset.value = { x: 0, y: 0 }
}

function resetMedia(): void {
  loaded.value = false
  failed.value = false
  failureDetail.value = ''
  naturalSize.value = undefined
  durationSeconds.value = undefined
  resetView()
}

watch(() => item.value?.entry.path, () => {
  resetMedia()
  preloadNeighbours()
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
  const limitX = (bounds.width * (nextScale - 1)) / 2
  const limitY = (bounds.height * (nextScale - 1)) / 2
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
  pointer = { id: event.pointerId, x: event.clientX, y: event.clientY, startX: event.clientX, startY: event.clientY, moved: false }
  stage.value?.setPointerCapture(event.pointerId)
}

function onPointerMove(event: PointerEvent): void {
  if (!pointer || pointer.id !== event.pointerId) return
  const deltaX = event.clientX - pointer.x
  const deltaY = event.clientY - pointer.y
  pointer.x = event.clientX
  pointer.y = event.clientY
  if (Math.abs(event.clientX - pointer.startX) + Math.abs(event.clientY - pointer.startY) > 6) pointer.moved = true
  if (zoomed.value) offset.value = clampOffset(scale.value, offset.value.x + deltaX, offset.value.y + deltaY)
}

function onPointerUp(event: PointerEvent): void {
  if (!pointer || pointer.id !== event.pointerId) return
  const swipe = event.clientX - pointer.startX
  const vertical = Math.abs(event.clientY - pointer.startY)
  const wasZoomed = zoomed.value
  pointer = undefined
  // A horizontal drag on an unzoomed photo turns the page like a phone gallery.
  if (!wasZoomed && Math.abs(swipe) > 64 && vertical < 80) go(swipe < 0 ? 1 : -1)
}

function onImageLoad(event: Event): void {
  const image = event.currentTarget as HTMLImageElement
  naturalSize.value = { width: image.naturalWidth, height: image.naturalHeight }
  loaded.value = true
}

function onImageError(): void {
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

function retry(): void {
  resetMedia()
  retryKey.value += 1
}

function preloadNeighbours(): void {
  for (const step of [1, -1]) {
    const neighbour = props.items[props.index + step]
    if (!neighbour || neighbour.kind !== 'image' || !galleryBrowserCanShow(neighbour)) continue
    const image = new Image()
    image.decoding = 'async'
    image.src = props.sources(neighbour).original
  }
}

function scrollStripToCurrent(): void {
  const current = strip.value?.querySelector<HTMLElement>('[aria-current="true"]')
  if (typeof current?.scrollIntoView === 'function') current.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' })
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
    if (zoomed.value) resetView()
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
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (!props.contained) activateModal(modalId)
  document.addEventListener('fullscreenchange', syncFullscreen)
  root.value?.focus({ preventScroll: true })
  preloadNeighbours()
  void nextTick(scrollStripToCurrent)
})

onBeforeUnmount(() => {
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
    :class="{ 'gallery-viewer--contained': contained, 'gallery-viewer--info': infoOpen }"
    role="dialog"
    aria-modal="true"
    :aria-label="phrase('照片与视频查看器')"
    tabindex="-1"
    @keydown="onKeydown"
  >
    <div v-if="itemSources?.preview" class="gallery-viewer__ambient" aria-hidden="true">
      <img :src="itemSources.preview" alt="" />
    </div>

    <header class="gallery-viewer__bar">
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
        :class="{ 'is-zoomed': zoomed, 'is-image': item?.kind === 'image' }"
        @wheel="onWheel"
        @dblclick="onDoubleClick"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="pointer = undefined"
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
              :key="`${item.entry.path}:${retryKey}`"
              class="gallery-viewer__media"
              :class="{ 'is-loaded': loaded }"
              :src="itemSources?.original"
              :alt="item.entry.name"
              :style="{ transform: mediaTransform }"
              decoding="async"
              draggable="false"
              @load="onImageLoad"
              @error="onImageError"
            />
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
          :title="phrase('上一张')"
          :aria-label="phrase('上一张')"
          @pointerdown.stop
          @click="go(-1)"
        ><ChevronLeft :size="28" /></button>
        <button
          v-if="index < items.length - 1"
          class="gallery-viewer__nav gallery-viewer__nav--next"
          type="button"
          :title="phrase('下一张')"
          :aria-label="phrase('下一张')"
          @pointerdown.stop
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

    <nav v-if="items.length > 1" ref="strip" class="gallery-viewer__strip" :aria-label="phrase('缩略图')">
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
        <img v-if="sources(thumb.item).preview" :src="sources(thumb.item).preview" alt="" loading="lazy" decoding="async" draggable="false" />
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
  grid-template-rows: auto minmax(0, 1fr) auto;
  overflow: hidden;
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

.gallery-viewer__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  padding: 12px 14px 10px 20px;
  background: linear-gradient(rgb(6 8 11 / 72%), rgb(6 8 11 / 0%));
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
  min-height: 0;
}

.gallery-viewer__stage {
  position: relative;
  display: grid;
  flex: 1 1 auto;
  min-width: 0;
  place-items: center;
  overflow: hidden;
  touch-action: none;
  user-select: none;
}

.gallery-viewer__stage.is-image {
  cursor: zoom-in;
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

.gallery-viewer__media--preview {
  width: min(calc(100% - 48px), 1200px);
  height: auto;
  filter: blur(6px);
}

.gallery-viewer__media--video {
  width: auto;
  max-width: calc(100% - 120px);
  background: #000;
}

.gallery-viewer__stage.is-zoomed .gallery-viewer__media {
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
  padding: 8px 20px 20px;
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
  display: flex;
  justify-content: safe center;
  gap: 6px;
  min-width: 0;
  padding: 10px 16px 14px;
  overflow-x: auto;
  scrollbar-width: none;
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
  .gallery-viewer__bar {
    align-items: flex-start;
    padding: 10px 8px 8px 14px;
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

  .gallery-viewer__info {
    position: absolute;
    z-index: 2;
    inset: auto 0 0;
    max-height: 55%;
    border-top: 1px solid var(--viewer-line);
    border-left: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .gallery-viewer,
  .gallery-viewer__info,
  .gallery-viewer__media {
    animation: none;
    transition: none;
  }
}
</style>
