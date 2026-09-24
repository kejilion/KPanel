<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import {
  chronoPoster,
  desktopScenePoster,
  findDesktopScene,
  type DesktopSceneID,
  type DesktopSceneLayer,
} from '@/lib/desktopScenes/catalog'
import {
  CHRONO_PHASES,
  chronoWeights,
  chronoWeightsAt,
  minuteOfDay,
  type ChronoPhase,
  type ChronoWeights,
} from '@/lib/desktopScenes/chrono'
import { coverImageRect, type SceneFrame } from '@/lib/desktopScenes/painterKit'
import { createSceneLoop, type SceneLoop, type SceneLoopStatus } from '@/lib/desktopScenes/sceneLoop'
import '@/styles/desktopScenes.css'

/**
 * One live desktop scene: poster, anchored light layers and a particle canvas
 * inside a stage that pushes in on entrance and then breathes slowly. Motion
 * pauses while the desktop is covered or hidden and follows reduced motion.
 */
const props = defineProps<{
  scene: DesktopSceneID
  /** A maximized window or a full side-by-side split hides the wallpaper. */
  covered: boolean
  /** `select` plays the full entrance; `restore` is the page-load variant. */
  entrance: 'restore' | 'select'
}>()

const SELECT_HANDOFF_MS = 650
const DECODE_TIMEOUT_MS = 8000
const CHRONO_REFRESH_MS = 60_000
// Bottom-to-top paint order: the golden hour sits under day and night.
const CHRONO_STACK: readonly ChronoPhase[] = ['golden', 'day', 'night']

const definition = findDesktopScene(props.scene)
const root = ref<HTMLElement>()
const canvas = ref<HTMLCanvasElement>()
const ready = ref(false)
const failed = ref(false)
const status = ref<SceneLoopStatus>('paused')
const density = ref(1)
const reducedMotion = ref(prefersReducedMotion())
const documentHidden = ref(typeof document !== 'undefined' && document.visibilityState === 'hidden')
const timelapse = ref(props.scene === 'chrono' && props.entrance === 'select' && !reducedMotion.value)
const loadedPhases = shallowRef<ReadonlySet<ChronoPhase>>(new Set())
// Phases with any weight; the others leave the render tree instead of compositing at zero opacity.
const activePhases = shallowRef<ReadonlySet<ChronoPhase>>(new Set())
let weights: ChronoWeights = chronoWeights(new Date())
let timelapseStart = minuteOfDay(new Date())
let loop: SceneLoop | undefined
let resizeObserver: ResizeObserver | undefined
let motionQuery: MediaQueryList | undefined
let chronoTimer: number | undefined
let disposed = false

const poster = computed(() => desktopScenePoster(props.scene))
const paused = computed(() => props.covered || documentHidden.value || status.value === 'suspended')
const classes = computed(() => ({
  'desktop-scene--ready': ready.value,
  'desktop-scene--live': ready.value && !reducedMotion.value,
  'desktop-scene--static': reducedMotion.value,
  'desktop-scene--paused': paused.value,
  'desktop-scene--timelapse': timelapse.value,
  [`desktop-scene--${props.entrance}`]: true,
  ...Object.fromEntries([...activePhases.value].map((phase) => [`desktop-scene--phase-${phase}`, true])),
}))

function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function layerStyle(layer: DesktopSceneLayer): Record<string, string> {
  return {
    '--x': String(layer.x),
    '--y': String(layer.y),
    '--w': String(layer.w),
    '--h': String(layer.h),
    '--delay': `${layer.delay ?? 0}s`,
  }
}

function neededPhases(): Set<ChronoPhase> {
  if (timelapse.value) return new Set(CHRONO_PHASES)
  return new Set(CHRONO_PHASES.filter((phase) => weights[phase] > 0.001))
}

function applyWeights(next: ChronoWeights): void {
  weights = next
  const element = root.value
  if (element) for (const phase of CHRONO_PHASES) element.style.setProperty(`--scene-${phase}`, next[phase].toFixed(4))
  loop?.setParams(next)
  const phases = neededPhases()
  if (CHRONO_PHASES.some((phase) => phases.has(phase) !== activePhases.value.has(phase))) activePhases.value = phases
  if ([...phases].some((phase) => !loadedPhases.value.has(phase))) {
    loadedPhases.value = new Set([...loadedPhases.value, ...phases])
  }
}

function measure(): void {
  const element = root.value
  if (!element) return
  const width = element.clientWidth
  const height = element.clientHeight
  const rect = coverImageRect(width, height)
  element.style.setProperty('--scene-x', `${rect.x}px`)
  element.style.setProperty('--scene-y', `${rect.y}px`)
  element.style.setProperty('--scene-w', `${rect.width}px`)
  element.style.setProperty('--scene-h', `${rect.height}px`)
  loop?.resize(width, height)
}

function decode(source: string): Promise<void> {
  if (typeof Image === 'undefined') return Promise.resolve()
  const image = new Image()
  image.decoding = 'async'
  image.src = source
  return typeof image.decode === 'function'
    ? image.decode()
    : new Promise((resolve, reject) => {
      image.onload = () => resolve()
      image.onerror = () => reject(new Error('scene_image_unavailable'))
    })
}

function withTimeout(task: Promise<unknown>): Promise<unknown> {
  return Promise.race([task, new Promise((resolve) => window.setTimeout(resolve, DECODE_TIMEOUT_MS))])
}

function easeInOut(value: number): number {
  return value < 0.5 ? 4 * value ** 3 : 1 - (-2 * value + 2) ** 3 / 2
}

function onFrame(frame: SceneFrame): void {
  if (!timelapse.value) return
  // Twenty-four hours in one breath, starting and ending at the current time.
  const minute = timelapseStart + 1440 * easeInOut(frame.entrance)
  applyWeights(chronoWeightsAt(minute))
  if (frame.entrance >= 1) {
    timelapse.value = false
    applyWeights(chronoWeights(new Date()))
  }
}

function syncLoop(): void {
  loop?.setReducedMotion(reducedMotion.value)
  loop?.setPaused(!ready.value || paused.value)
}

function onVisibilityChange(): void {
  documentHidden.value = document.visibilityState === 'hidden'
}

function onMotionPreference(event: MediaQueryListEvent): void {
  reducedMotion.value = event.matches
  if (event.matches && timelapse.value) {
    timelapse.value = false
    applyWeights(chronoWeights(new Date()))
  }
}

function initialDensity(): number {
  if (typeof navigator === 'undefined') return 1
  const device = navigator as Navigator & { deviceMemory?: number, connection?: { saveData?: boolean } }
  let density = 1
  if ((device.hardwareConcurrency || 8) <= 4) density *= 0.7
  if ((device.deviceMemory ?? 8) <= 4) density *= 0.75
  if (device.connection?.saveData) density *= 0.6
  return Math.max(0.35, density)
}

function sceneSeed(): number {
  let hash = 2166136261
  for (const character of props.scene) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619)
  return hash >>> 0
}

async function start(): Promise<void> {
  // Optional art (a branch, an aurora veil) never blocks the scene; a missing poster does.
  const optional = definition.layers.flatMap((layer) => (layer.image ? [decode(layer.image).catch(() => undefined)] : []))
  let posters = [poster.value]
  if (props.scene === 'chrono') {
    applyWeights(timelapse.value ? chronoWeightsAt(timelapseStart) : chronoWeights(new Date()))
    posters = [...neededPhases()].map(chronoPoster)
  }
  const [posterReady, painterFactory] = await Promise.all([
    withTimeout(Promise.all([...posters.map(decode), ...optional])).then(() => true, () => false),
    definition.loadPainter().catch(() => undefined),
    props.entrance === 'select' ? new Promise((resolve) => window.setTimeout(resolve, SELECT_HANDOFF_MS)) : undefined,
  ])
  if (disposed) return
  if (!posterReady) {
    // The static wallpaper underneath keeps the desktop usable without the scene.
    failed.value = true
    return
  }
  loop = createSceneLoop({
    canvas: canvas.value,
    painter: painterFactory,
    seed: sceneSeed(),
    entranceSeconds: definition.entranceSeconds,
    density: initialDensity(),
    reducedMotion: reducedMotion.value,
    onFrame,
    onStatus: (next) => { status.value = next },
    onDensity: (next) => { density.value = next },
  })
  density.value = loop.density
  if (props.scene === 'chrono') loop.setParams(weights)
  measure()
  ready.value = true
  syncLoop()
}

watch([paused, reducedMotion], syncLoop)

onMounted(() => {
  if (typeof ResizeObserver === 'function' && root.value) {
    resizeObserver = new ResizeObserver(() => measure())
    resizeObserver.observe(root.value)
  } else {
    window.addEventListener('resize', measure)
  }
  measure()
  document.addEventListener('visibilitychange', onVisibilityChange)
  if (typeof window.matchMedia === 'function') {
    motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    motionQuery.addEventListener?.('change', onMotionPreference)
  }
  if (props.scene === 'chrono') {
    chronoTimer = window.setInterval(() => {
      if (timelapse.value || documentHidden.value) return
      applyWeights(chronoWeights(new Date()))
      loop?.refresh()
    }, CHRONO_REFRESH_MS)
  }
  void start()
})

onBeforeUnmount(() => {
  disposed = true
  loop?.destroy()
  resizeObserver?.disconnect()
  window.removeEventListener('resize', measure)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  motionQuery?.removeEventListener?.('change', onMotionPreference)
  if (chronoTimer !== undefined) window.clearInterval(chronoTimer)
})

defineExpose({ status, ready })
</script>

<template>
  <div
    v-if="!failed"
    ref="root"
    class="desktop-scene"
    :class="classes"
    :data-scene="scene"
    :data-scene-status="ready ? status : 'loading'"
    :data-scene-density="ready ? density.toFixed(2) : undefined"
    :style="{ '--focus-x': String(definition.focus.x), '--focus-y': String(definition.focus.y) }"
    aria-hidden="true"
  >
    <div class="desktop-scene__stage">
      <template v-if="scene === 'chrono'">
        <div
          v-for="phase in CHRONO_STACK"
          :key="phase"
          class="desktop-scene__poster"
          :data-phase="phase"
          :style="loadedPhases.has(phase) ? { backgroundImage: `url('${chronoPoster(phase)}')` } : undefined"
        />
      </template>
      <div v-else class="desktop-scene__poster" :style="{ backgroundImage: `url('${poster}')` }" />
      <div
        v-for="(layer, index) in definition.layers"
        :key="`${layer.id}-${index}`"
        class="desktop-scene__layer"
        :class="`desktop-scene__layer--${layer.id}`"
        :style="layerStyle(layer)"
      >
        <div
          class="desktop-scene__art"
          :style="layer.image ? { backgroundImage: `url('${layer.image}')` } : undefined"
        />
      </div>
      <canvas ref="canvas" class="desktop-scene__particles" />
    </div>
  </div>
</template>
