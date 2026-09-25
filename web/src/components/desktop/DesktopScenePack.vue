<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '@/lib/api'
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'
import { parseScenePackEvent, scenePackPageURL, type ScenePack, type ScenePackCommand } from '@/lib/scenePacks'
import '@/styles/desktopScenePack.css'

/**
 * Runs an installed 3D scene pack as the desktop wallpaper. The pack page lives
 * in an iframe sandboxed to scripts only (opaque origin: no panel cookies,
 * storage, DOM or top navigation) and is driven with pause/resume/camera
 * messages. A live scene starts from black so its entrance is the first thing
 * seen; the still poster is only for reduced motion or when the scene fails.
 */
const props = defineProps<{
  packId: string
  /** A maximized window or a full side-by-side split hides the wallpaper. */
  covered: boolean
}>()

const emit = defineEmits<{
  cameras: [ids: string[]]
  camera: [index: number]
  failed: []
}>()

const READY_TIMEOUT_MS = 15_000

const frame = ref<HTMLIFrameElement>()
const ready = ref(false)
const failed = ref(false)
const located = ref(false)
const documentHidden = ref(typeof document !== 'undefined' && document.visibilityState === 'hidden')
const { reducedMotion } = useSceneMotionPreference()
const posterURL = computed(() => api.desktop.scenePackPosterURL(props.packId))
// The installed page URL comes from the server's pack list, never from the wallpaper key.
const pack = ref<ScenePack>()
const pageURL = computed(() => (pack.value ? scenePackPageURL(pack.value) : undefined))
const running = computed(() => !reducedMotion.value && !failed.value && Boolean(pageURL.value))
const paused = computed(() => props.covered || documentHidden.value)
const state = computed(() => {
  if (failed.value) return 'failed'
  if (reducedMotion.value || (located.value && !pageURL.value)) return 'static'
  return ready.value ? 'running' : 'loading'
})
const showPoster = computed(() => state.value === 'static' || state.value === 'failed')
let readyTimer: number | undefined

function post(command: ScenePackCommand): void {
  // The sandbox has an opaque origin, so '*' is the only usable target; commands carry no data.
  frame.value?.contentWindow?.postMessage(command, '*')
}

function syncPause(): void {
  if (ready.value) post({ source: 'kpanel-desktop', type: paused.value ? 'pause' : 'resume' })
}

function fail(): void {
  if (failed.value) return
  failed.value = true
  ready.value = false
  emit('failed')
}

function onMessage(event: MessageEvent): void {
  if (!frame.value || event.source !== frame.value.contentWindow) return
  const message = parseScenePackEvent(event.data)
  if (!message) return
  if (message.type === 'ready') {
    ready.value = true
    if (readyTimer !== undefined) window.clearTimeout(readyTimer)
    emit('cameras', message.cameras)
    syncPause()
  } else if (message.type === 'camera') {
    emit('camera', message.index)
  } else {
    fail()
  }
}

function onVisibilityChange(): void {
  documentHidden.value = document.visibilityState === 'hidden'
}

function isReloadKey(event: KeyboardEvent): boolean {
  // Hard reloads (with Shift) keep the browser's own handling: location.reload() cannot bypass the cache.
  if (event.shiftKey || event.altKey) return false
  return event.key === 'F5' || ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'r')
}

// Reloading tears the sandboxed frame down, and a frame that is still on screen at that moment
// is painted white for one frame (hiding it from beforeunload is already too late). For the
// reload keys, hide it first, let that frame reach the screen, then reload.
function onKeyDown(event: KeyboardEvent): void {
  if (!frame.value || event.defaultPrevented || !isReloadKey(event)) return
  event.preventDefault()
  frame.value.style.visibility = 'hidden'
  window.requestAnimationFrame(() => window.requestAnimationFrame(() => window.location.reload()))
}

function armReadyTimeout(): void {
  if (readyTimer !== undefined) window.clearTimeout(readyTimer)
  readyTimer = window.setTimeout(() => { if (!ready.value) fail() }, READY_TIMEOUT_MS)
}

watch(paused, syncPause)
watch(running, (value) => {
  ready.value = false
  if (value) armReadyTimeout()
})

async function locatePack(): Promise<void> {
  try {
    const found = (await api.desktop.scenePacks()).packs.find((candidate) => candidate.id === props.packId)
    if (found?.installed && scenePackPageURL(found)) pack.value = found
    else fail()
  } catch {
    // Keep the poster when the list is unreachable; the pack page needs its file base.
  } finally {
    located.value = true
  }
}

onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('keydown', onKeyDown)
  document.addEventListener('visibilitychange', onVisibilityChange)
  void locatePack()
})

onBeforeUnmount(() => {
  window.removeEventListener('message', onMessage)
  window.removeEventListener('keydown', onKeyDown)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  if (readyTimer !== undefined) window.clearTimeout(readyTimer)
})

defineExpose({
  /** Asks the pack for its next camera; ignored until the pack is ready. */
  nextCamera(): void {
    if (ready.value) post({ source: 'kpanel-desktop', type: 'camera' })
  },
})
</script>

<template>
  <div
    class="desktop-scene-pack"
    :class="{ 'desktop-scene-pack--ready': ready }"
    :data-scene-pack="packId"
    :data-scene-pack-state="state"
    aria-hidden="true"
  >
    <img v-if="showPoster" class="desktop-scene-pack__poster" :src="posterURL" alt="" decoding="async" />
    <iframe
      v-if="running"
      ref="frame"
      class="desktop-scene-pack__frame"
      :src="pageURL"
      sandbox="allow-scripts"
      referrerpolicy="no-referrer"
      allow=""
      tabindex="-1"
      title=""
    />
  </div>
</template>
