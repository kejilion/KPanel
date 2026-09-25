<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '@/lib/api'
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'
import { parseScenePackEvent, rememberedFileBase, rememberFileBase, scenePackPageURL, type ScenePack, type ScenePackCommand } from '@/lib/scenePacks'
import '@/styles/desktopScenePack.css'

/**
 * Runs an installed 3D scene pack as the desktop wallpaper. The pack page lives
 * in an iframe sandboxed to scripts only (opaque origin: no panel cookies,
 * storage, DOM or top navigation) and is driven with pause/resume/camera
 * messages. A live scene starts from black so its entrance is the first thing
 * seen; the still poster is only for reduced motion or when the scene fails.
 *
 * Loading: the frame starts at once from the file base the pack was last found
 * at, while the pack list is fetched to confirm it (or to move to a new one after
 * an update). If the pack takes a while, a thin progress line shows over the
 * black, fed by the pack's progress messages. The watchdog counts from the
 * pack's last sign of life, so a large scene on a slow link is not given up on
 * while it is still loading.
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

/** Given up on after this long without a message from the pack. */
const READY_TIMEOUT_MS = 20_000
/** The progress line only shows when loading takes longer than this. */
const PROGRESS_DELAY_MS = 1200

const frame = ref<HTMLIFrameElement>()
const ready = ref(false)
const failed = ref(false)
const located = ref(false)
const documentHidden = ref(typeof document !== 'undefined' && document.visibilityState === 'hidden')
const { reducedMotion } = useSceneMotionPreference()
const posterURL = computed(() => api.desktop.scenePackPosterURL(props.packId))
// The page URL comes from the server's pack list (or where the list last put it), never from the wallpaper key.
const pack = ref<ScenePack>()
const rememberedBase = ref(rememberedFileBase(props.packId))
const pageURL = computed(() => {
  if (pack.value) return scenePackPageURL(pack.value)
  return rememberedBase.value ? scenePackPageURL({ fileBase: rememberedBase.value }) : undefined
})
const progress = ref(0)
const slow = ref(false)
let slowTimer: number | undefined
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
  if (message.type === 'progress') {
    if (!ready.value) {
      progress.value = Math.max(progress.value, message.value)
      armReadyTimeout()
    }
  } else if (message.type === 'ready') {
    ready.value = true
    progress.value = 1
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
// A new page (another pack, or the confirmed base after an update) starts loading afresh.
watch([running, pageURL], ([value]) => {
  ready.value = false
  progress.value = 0
  slow.value = false
  if (slowTimer !== undefined) window.clearTimeout(slowTimer)
  if (!value) return
  armReadyTimeout()
  slowTimer = window.setTimeout(() => { slow.value = !ready.value }, PROGRESS_DELAY_MS)
}, { immediate: true })

async function locatePack(): Promise<void> {
  try {
    const found = (await api.desktop.scenePacks()).packs.find((candidate) => candidate.id === props.packId)
    if (found?.installed && scenePackPageURL(found)) {
      pack.value = found
      rememberFileBase(props.packId, found.fileBase)
    } else {
      rememberFileBase(props.packId, null)
      rememberedBase.value = undefined
      fail()
    }
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
  if (slowTimer !== undefined) window.clearTimeout(slowTimer)
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
    <Transition name="desktop-scene-pack-progress">
      <div
        v-if="running && !ready && slow"
        class="desktop-scene-pack__progress"
        :style="{ '--scene-pack-progress': progress.toFixed(3) }"
      />
    </Transition>
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
