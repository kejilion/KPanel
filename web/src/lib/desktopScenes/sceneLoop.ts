import { sceneModules, type DesktopSceneID } from './loaders'
import { createShaderRenderer } from './renderer'
import { SceneUnsupportedError, type SceneModule, type SceneRenderer, type SceneSurface, type ShaderScene } from './types'

// Shared by the scene worker and the main-thread fallback. Scenes are ambient, so
// the loop caps the frame rate and pixel budget, never queues more than the GPU
// keeps up with, and steps quality down when frames keep getting dropped.

export const SCENE_QUALITY_LEVELS = [
  { scale: 1, fps: 30, detail: 1 },
  { scale: 0.8, fps: 30, detail: 0.8 },
  { scale: 0.66, fps: 30, detail: 0.55 },
  { scale: 0.5, fps: 24, detail: 0.3 },
] as const

// Shaded scenes are soft, so CSS pixels are enough even on high-density screens.
const MAX_DEVICE_SCALE = 1
const MAX_PIXELS = 2_100_000
const WARMUP_MS = 1500
const WINDOW_MS = 2000
const MIN_DRAWN_RATIO = 0.8
const MIN_RATE_RATIO = 0.6
const STRIKES_BEFORE_DOWNGRADE = 2
const POINTER_EASE_PER_SECOND = 2.4
const STILL_REFRESH_MS = 60_000

export type SceneLoopStatus =
  | { type: 'ready', quality: number }
  | { type: 'quality', quality: number }
  | { type: 'error', reason: 'load' | 'render' | 'unsupported' }

export interface SceneLoopOptions {
  sceneId: DesktopSceneID
  surface: SceneSurface
  /** Schedules one animation callback and returns its cancel function. */
  schedule: (callback: (now: number) => void) => () => void
  now: () => number
  onStatus: (status: SceneLoopStatus) => void
  load?: (sceneId: DesktopSceneID) => Promise<SceneModule>
  createRenderer?: (surface: SceneSurface, scene: ShaderScene) => Promise<SceneRenderer>
  clockHour?: () => number
}

export interface SceneLoop {
  resize: (cssWidth: number, cssHeight: number, devicePixelRatio: number) => void
  setPaused: (paused: boolean) => void
  setReducedMotion: (reducedMotion: boolean) => void
  setPointer: (x: number, y: number) => void
  dispose: () => void
}

export function localClockHour(date = new Date()): number {
  return date.getHours() + date.getMinutes() / 60 + date.getSeconds() / 3600
}

export function scenePixelSize(cssWidth: number, cssHeight: number, devicePixelRatio: number, quality: number): { width: number, height: number } {
  const level = SCENE_QUALITY_LEVELS[Math.min(quality, SCENE_QUALITY_LEVELS.length - 1)]!
  let scale = Math.min(devicePixelRatio > 0 ? devicePixelRatio : 1, MAX_DEVICE_SCALE)
  const pixels = cssWidth * cssHeight * scale * scale
  if (pixels > MAX_PIXELS) scale *= Math.sqrt(MAX_PIXELS / pixels)
  scale *= level.scale
  return {
    width: Math.max(1, Math.round(cssWidth * scale)),
    height: Math.max(1, Math.round(cssHeight * scale)),
  }
}

export function createSceneLoop(options: SceneLoopOptions): SceneLoop {
  const clockHour = options.clockHour ?? (() => localClockHour())
  let renderer: SceneRenderer | undefined
  let cssWidth = 0
  let cssHeight = 0
  let pixelRatio = 1
  let quality = 0
  let paused = false
  let reducedMotion = false
  let disposed = false
  let failed = false
  let presented = false
  let cancelFrame: (() => void) | undefined
  let stillTimer: ReturnType<typeof setTimeout> | undefined
  let lastFrame: number | undefined
  let elapsed = 0
  const pointerTarget: [number, number] = [0, 0]
  const pointer: [number, number] = [0, 0]
  // Frame accounting for the quality ladder.
  let windowStart: number | undefined
  let dueFrames = 0
  let drawnFrames = 0
  let strikes = 0

  function animating(): boolean {
    return Boolean(renderer) && !paused && !reducedMotion && !disposed && !failed && cssWidth > 0 && cssHeight > 0
  }

  function applySize(): void {
    if (!renderer || cssWidth <= 0 || cssHeight <= 0) return
    const size = scenePixelSize(cssWidth, cssHeight, pixelRatio, quality)
    if (presented && options.surface.width === size.width && options.surface.height === size.height) return
    options.surface.width = size.width
    options.surface.height = size.height
    renderer.resize(size.width, size.height)
  }

  function draw(): boolean {
    if (!renderer || failed || cssWidth <= 0 || cssHeight <= 0) return false
    try {
      renderer.render({
        time: elapsed,
        hour: clockHour(),
        pointer,
        detail: SCENE_QUALITY_LEVELS[quality]!.detail,
      })
    } catch {
      failed = true
      stop()
      options.onStatus({ type: 'error', reason: 'render' })
      return false
    }
    return true
  }

  function account(now: number, drawn: boolean): void {
    if (windowStart === undefined) {
      windowStart = now + WARMUP_MS
      return
    }
    if (now < windowStart) return
    dueFrames++
    if (drawn) drawnFrames++
    const span = now - windowStart
    if (span < WINDOW_MS) return
    const fps = SCENE_QUALITY_LEVELS[quality]!.fps
    const behind = drawnFrames < dueFrames * MIN_DRAWN_RATIO || drawnFrames < (span / 1000) * fps * MIN_RATE_RATIO
    strikes = behind ? strikes + 1 : 0
    windowStart = now
    dueFrames = 0
    drawnFrames = 0
    if (strikes >= STRIKES_BEFORE_DOWNGRADE && quality < SCENE_QUALITY_LEVELS.length - 1) {
      quality++
      strikes = 0
      windowStart = undefined
      applySize()
      options.onStatus({ type: 'quality', quality })
    }
  }

  function tick(now: number): void {
    cancelFrame = undefined
    if (!animating()) return
    cancelFrame = options.schedule(tick)
    const interval = 1000 / SCENE_QUALITY_LEVELS[quality]!.fps
    if (lastFrame !== undefined && now - lastFrame < interval - 2) return
    if (renderer!.backlogged()) {
      account(now, false)
      return
    }
    const delta = lastFrame === undefined ? 0 : Math.min(0.1, (now - lastFrame) / 1000)
    lastFrame = now
    elapsed += delta
    const ease = 1 - Math.exp(-delta * POINTER_EASE_PER_SECOND)
    pointer[0] += (pointerTarget[0] - pointer[0]) * ease
    pointer[1] += (pointerTarget[1] - pointer[1]) * ease
    account(now, draw())
  }

  function stop(): void {
    cancelFrame?.()
    cancelFrame = undefined
    if (stillTimer !== undefined) clearTimeout(stillTimer)
    stillTimer = undefined
    lastFrame = undefined
    windowStart = undefined
    dueFrames = 0
    drawnFrames = 0
  }

  function refreshStill(): void {
    stillTimer = undefined
    if (!renderer || disposed || paused || !reducedMotion) return
    draw()
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
    let module: SceneModule
    try {
      module = await (options.load ?? ((id) => sceneModules.load(id)))(options.sceneId)
    } catch {
      if (!disposed) options.onStatus({ type: 'error', reason: 'load' })
      return
    }
    if (disposed) return
    let created: SceneRenderer
    try {
      created = await (options.createRenderer ?? createShaderRenderer)(options.surface, module.scene)
    } catch (error) {
      if (!disposed) options.onStatus({ type: 'error', reason: error instanceof SceneUnsupportedError ? 'unsupported' : 'render' })
      return
    }
    if (disposed) {
      created.dispose()
      return
    }
    renderer = created
    if (cssWidth > 0 && cssHeight > 0) present()
  }

  // Paint once even while paused or pending so the cross-fade always has a frame.
  function present(): void {
    applySize()
    if (!draw()) return
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
      if (!renderer || failed || cssWidth <= 0 || cssHeight <= 0) return
      if (!presented) {
        present()
        return
      }
      applySize()
      if (!animating()) draw()
    },
    setPaused(next) {
      if (paused === next) return
      paused = next
      sync()
    },
    setReducedMotion(next) {
      if (reducedMotion === next) return
      reducedMotion = next
      if (next) {
        pointerTarget[0] = pointerTarget[1] = pointer[0] = pointer[1] = 0
      }
      sync()
    },
    setPointer(x, y) {
      if (reducedMotion) return
      pointerTarget[0] = Math.max(-1, Math.min(1, x))
      pointerTarget[1] = Math.max(-1, Math.min(1, y))
    },
    dispose() {
      disposed = true
      stop()
      renderer?.dispose()
      renderer = undefined
    },
  }
}
