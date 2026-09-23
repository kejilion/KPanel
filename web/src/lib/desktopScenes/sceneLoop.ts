import { sceneModules, type DesktopSceneID } from './loaders'
import type { SceneContext2D, SceneInstance, SceneModule, SceneSurface } from './types'

// Shared by the scene worker and the main-thread fallback. Scenes are ambient, so
// the loop caps the frame rate, bounds the pixel budget and steps quality down
// when frames get expensive instead of competing with terminals and windows.

export const SCENE_QUALITY_LEVELS = [
  { scale: 1, fps: 30 },
  { scale: 0.8, fps: 30 },
  { scale: 0.65, fps: 24 },
  { scale: 0.5, fps: 20 },
] as const

const MAX_DEVICE_SCALE = 1.5
const MAX_PIXELS = 2_400_000
const FRAME_BUDGET_MS = 7
const SLOW_FRAMES_BEFORE_DOWNGRADE = 40
const STILL_REFRESH_MS = 60_000

export type SceneLoopStatus =
  | { type: 'ready', quality: number }
  | { type: 'quality', quality: number }
  | { type: 'error', reason: 'load' | 'render' }

export interface SceneLoopOptions {
  sceneId: DesktopSceneID
  surface: SceneSurface
  createSurface: (width: number, height: number) => SceneSurface
  /** Schedules one animation callback and returns its cancel function. */
  schedule: (callback: (now: number) => void) => () => void
  now: () => number
  onStatus: (status: SceneLoopStatus) => void
  load?: (sceneId: DesktopSceneID) => Promise<SceneModule>
  clockHour?: () => number
}

export interface SceneLoop {
  resize: (cssWidth: number, cssHeight: number, devicePixelRatio: number) => void
  setPaused: (paused: boolean) => void
  setReducedMotion: (reducedMotion: boolean) => void
  dispose: () => void
}

export function localClockHour(date = new Date()): number {
  return date.getHours() + date.getMinutes() / 60 + date.getSeconds() / 3600
}

export function scenePixelSize(cssWidth: number, cssHeight: number, devicePixelRatio: number, quality: number): { width: number, height: number } {
  const level = SCENE_QUALITY_LEVELS[Math.min(quality, SCENE_QUALITY_LEVELS.length - 1)]!
  let scale = Math.min(Math.max(devicePixelRatio || 1, 1), MAX_DEVICE_SCALE) * level.scale
  const pixels = cssWidth * cssHeight * scale * scale
  if (pixels > MAX_PIXELS) scale *= Math.sqrt(MAX_PIXELS / pixels)
  return {
    width: Math.max(1, Math.round(cssWidth * scale)),
    height: Math.max(1, Math.round(cssHeight * scale)),
  }
}

export function createSceneLoop(options: SceneLoopOptions): SceneLoop {
  const clockHour = options.clockHour ?? (() => localClockHour())
  let scene: SceneInstance | undefined
  let cssWidth = 0
  let cssHeight = 0
  let pixelRatio = 1
  let quality = 0
  let paused = false
  let reducedMotion = false
  let disposed = false
  let failed = false
  let sized = false
  let presented = false
  let cancelFrame: (() => void) | undefined
  let stillTimer: ReturnType<typeof setTimeout> | undefined
  let lastFrame: number | undefined
  let elapsed = 0
  let averageCost = 0
  let slowFrames = 0

  const context = options.surface.getContext('2d', { alpha: false }) as SceneContext2D | null

  function animating(): boolean {
    return Boolean(scene) && !paused && !reducedMotion && !disposed && !failed && cssWidth > 0 && cssHeight > 0
  }

  function applySize(): void {
    if (!scene || cssWidth <= 0 || cssHeight <= 0) return
    const size = scenePixelSize(cssWidth, cssHeight, pixelRatio, quality)
    if (sized && options.surface.width === size.width && options.surface.height === size.height) return
    options.surface.width = size.width
    options.surface.height = size.height
    scene.resize(size.width, size.height)
    sized = true
  }

  function draw(delta: number): boolean {
    if (!scene || failed || cssWidth <= 0 || cssHeight <= 0) return false
    const started = options.now()
    try {
      scene.render({ time: elapsed, delta, hour: clockHour() })
    } catch {
      failed = true
      stop()
      options.onStatus({ type: 'error', reason: 'render' })
      return false
    }
    const cost = options.now() - started
    averageCost = averageCost === 0 ? cost : averageCost * 0.9 + cost * 0.1
    slowFrames = averageCost > FRAME_BUDGET_MS ? slowFrames + 1 : 0
    if (slowFrames >= SLOW_FRAMES_BEFORE_DOWNGRADE && quality < SCENE_QUALITY_LEVELS.length - 1) {
      quality++
      slowFrames = 0
      averageCost = 0
      applySize()
      options.onStatus({ type: 'quality', quality })
    }
    return true
  }

  function tick(now: number): void {
    cancelFrame = undefined
    if (!animating()) return
    cancelFrame = options.schedule(tick)
    const interval = 1000 / SCENE_QUALITY_LEVELS[quality]!.fps
    if (lastFrame !== undefined && now - lastFrame < interval - 2) return
    const delta = lastFrame === undefined ? 0 : Math.min(0.1, (now - lastFrame) / 1000)
    lastFrame = now
    elapsed += delta
    draw(delta)
  }

  function stop(): void {
    cancelFrame?.()
    cancelFrame = undefined
    if (stillTimer !== undefined) clearTimeout(stillTimer)
    stillTimer = undefined
    lastFrame = undefined
  }

  function refreshStill(): void {
    stillTimer = undefined
    if (!scene || disposed || paused || !reducedMotion) return
    draw(0)
    stillTimer = setTimeout(refreshStill, STILL_REFRESH_MS)
  }

  function sync(): void {
    stop()
    if (animating()) {
      cancelFrame = options.schedule(tick)
      return
    }
    if (reducedMotion && !paused) refreshStill()
  }

  async function start(): Promise<void> {
    if (!context) {
      options.onStatus({ type: 'error', reason: 'render' })
      return
    }
    let module: SceneModule
    try {
      module = await (options.load ?? ((id) => sceneModules.load(id)))(options.sceneId)
    } catch {
      if (!disposed) options.onStatus({ type: 'error', reason: 'load' })
      return
    }
    if (disposed) return
    try {
      scene = module.createScene(context, { createSurface: options.createSurface })
    } catch {
      options.onStatus({ type: 'error', reason: 'render' })
      return
    }
    if (cssWidth > 0 && cssHeight > 0) present()
  }

  // Paint once even while paused or pending so the cross-fade always has a frame.
  function present(): void {
    applySize()
    if (!draw(0)) return
    presented = true
    options.onStatus({ type: 'ready', quality })
    sync()
  }

  void start()

  return {
    resize(nextWidth, nextHeight, nextRatio) {
      cssWidth = Math.max(0, nextWidth)
      cssHeight = Math.max(0, nextHeight)
      pixelRatio = nextRatio
      if (!scene || failed || cssWidth <= 0 || cssHeight <= 0) return
      if (!presented) {
        present()
        return
      }
      applySize()
      if (!animating()) draw(0)
    },
    setPaused(next) {
      if (paused === next) return
      paused = next
      sync()
    },
    setReducedMotion(next) {
      if (reducedMotion === next) return
      reducedMotion = next
      sync()
    },
    dispose() {
      disposed = true
      stop()
      scene = undefined
    },
  }
}
