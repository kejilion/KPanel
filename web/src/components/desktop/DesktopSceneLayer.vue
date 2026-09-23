<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { DesktopSceneID } from '@/lib/desktopScenes/catalog'
import { createSceneHost, type SceneHost } from '@/lib/desktopScenes/sceneHost'

const props = defineProps<{
  sceneId: DesktopSceneID
  /** Pending layers download and paint one frame invisibly, then wait to be shown. */
  active: boolean
  /** The desktop is covered, e.g. by a maximized window. */
  occluded: boolean
}>()

const emit = defineEmits<{
  ready: [sceneId: DesktopSceneID]
  error: [sceneId: DesktopSceneID]
}>()

const canvas = ref<HTMLCanvasElement>()
const canvasKey = ref(0)
const ready = ref(false)
const quality = ref(0)
const mode = ref<SceneHost['mode']>()
const documentHidden = ref(typeof document !== 'undefined' && document.visibilityState === 'hidden')
const reducedMotion = ref(false)
const paused = computed(() => !props.active || props.occluded || documentHidden.value)

let host: SceneHost | undefined
let preferWorker = true
let resizeObserver: ResizeObserver | undefined
let resizeFrame = 0
let motionQuery: MediaQueryList | undefined

function measure(): { width: number, height: number, pixelRatio: number } {
  const element = canvas.value
  return {
    width: element?.clientWidth || window.innerWidth,
    height: element?.clientHeight || window.innerHeight,
    pixelRatio: window.devicePixelRatio || 1,
  }
}

function mountHost(): void {
  const element = canvas.value
  if (!element || host) return
  const size = measure()
  host = createSceneHost(element, {
    sceneId: props.sceneId,
    ...size,
    paused: paused.value,
    reducedMotion: reducedMotion.value,
    preferWorker,
    onReady() {
      if (ready.value) return
      ready.value = true
      emit('ready', props.sceneId)
    },
    onQuality(level) {
      quality.value = level
    },
    onError(reason) {
      host?.dispose()
      host = undefined
      if (reason === 'worker' && preferWorker && !ready.value) {
        // A transferred canvas cannot be reused; remount a fresh one on the main thread.
        preferWorker = false
        canvasKey.value++
        void nextTick(mountHost)
        return
      }
      emit('error', props.sceneId)
    },
  })
  mode.value = host.mode
}

function scheduleResize(): void {
  if (resizeFrame) return
  resizeFrame = window.requestAnimationFrame(() => {
    resizeFrame = 0
    const size = measure()
    host?.resize(size.width, size.height, size.pixelRatio)
  })
}

function onVisibilityChange(): void {
  documentHidden.value = document.visibilityState === 'hidden'
}

function onMotionChange(event: MediaQueryListEvent): void {
  reducedMotion.value = event.matches
}

watch(paused, (value) => host?.setPaused(value))
watch(reducedMotion, (value) => host?.setReducedMotion(value))
watch(canvasKey, () => {
  resizeObserver?.disconnect()
  void nextTick(() => {
    if (canvas.value) resizeObserver?.observe(canvas.value)
  })
})

onMounted(() => {
  motionQuery = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : undefined
  reducedMotion.value = Boolean(motionQuery?.matches)
  motionQuery?.addEventListener?.('change', onMotionChange)
  document.addEventListener('visibilitychange', onVisibilityChange)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(scheduleResize)
    if (canvas.value) resizeObserver.observe(canvas.value)
  } else {
    window.addEventListener('resize', scheduleResize)
  }
  mountHost()
})

onBeforeUnmount(() => {
  host?.dispose()
  host = undefined
  resizeObserver?.disconnect()
  window.removeEventListener('resize', scheduleResize)
  if (resizeFrame) window.cancelAnimationFrame(resizeFrame)
  motionQuery?.removeEventListener?.('change', onMotionChange)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div
    class="desktop-scene"
    :class="{ 'desktop-scene--active': active, 'desktop-scene--ready': ready }"
    :data-desktop-scene="sceneId"
    :data-scene-mode="mode"
    :data-scene-quality="quality"
    :data-scene-paused="paused"
    aria-hidden="true"
  >
    <canvas :key="canvasKey" ref="canvas" class="desktop-scene__canvas" />
  </div>
</template>
