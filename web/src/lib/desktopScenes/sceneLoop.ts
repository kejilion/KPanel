import {
  coverImageRect,
  mulberry32,
  type SceneFrame,
  type ScenePainter,
  type ScenePainterFactory,
  type SceneViewport,
} from './painterKit'

/**
 * Frame-capped particle loop with a self-limiting performance governor.
 *
 * - paints at most `fps` frames per second (30 by default) and never while paused;
 * - measures its own step+draw time and the frames the browser actually delivers;
 * - lowers particle density when either budget is missed and, if the lowest
 *   density still misses it, suspends so the desktop keeps priority.
 */
export const SCENE_WORK_BUDGET_MS = 4
export const SCENE_GOVERNOR_WINDOW_MS = 2000
export const SCENE_MIN_DENSITY = 0.25

export type SceneLoopStatus = 'running' | 'paused' | 'static' | 'suspended'

export interface SceneLoopOptions {
  canvas?: HTMLCanvasElement
  painter?: ScenePainterFactory
  seed: number
  entranceSeconds: number
  density: number
  reducedMotion: boolean
  fps?: number
  maxPixelRatio?: number
  onFrame?: (frame: SceneFrame) => void
  onStatus?: (status: SceneLoopStatus) => void
  onDensity?: (density: number) => void
  now?: () => number
  requestFrame?: (callback: FrameRequestCallback) => number
  cancelFrame?: (handle: number) => void
  devicePixelRatio?: () => number
}

export interface SceneLoop {
  resize(width: number, height: number): void
  setPaused(paused: boolean): void
  setReducedMotion(reduced: boolean): void
  /** Forwards painter parameters; safe to call from `onFrame`. */
  setParams(params: Readonly<Record<string, number>>): void
  /** Repaints a still frame when the loop is not running (e.g. after new params). */
  refresh(): void
  destroy(): void
  readonly density: number
  readonly status: SceneLoopStatus
}

function createSurface(width: number, height: number): HTMLCanvasElement | OffscreenCanvas | undefined {
  if (typeof OffscreenCanvas === 'function') return new OffscreenCanvas(width, height)
  if (typeof document === 'undefined') return undefined
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}

export function createSceneLoop(options: SceneLoopOptions): SceneLoop {
  const fps = options.fps ?? 30
  const interval = 1000 / fps
  const now = options.now ?? (() => performance.now())
  const requestFrame = options.requestFrame ?? ((callback: FrameRequestCallback) => window.requestAnimationFrame(callback))
  const cancelFrame = options.cancelFrame ?? ((handle: number) => window.cancelAnimationFrame(handle))
  const pixelRatioSource = options.devicePixelRatio ?? (() => (typeof window === 'undefined' ? 1 : window.devicePixelRatio || 1))
  const maxDensity = Math.min(1, Math.max(SCENE_MIN_DENSITY, options.density))

  let context: CanvasRenderingContext2D | null = null
  let painter: ScenePainter | undefined
  if (options.canvas && options.painter) {
    try {
      context = options.canvas.getContext('2d', { alpha: true })
    } catch {
      context = null
    }
    if (context) {
      painter = options.painter({ random: mulberry32(options.seed), createSurface })
      painter.setDensity(maxDensity)
    }
  }

  let density = maxDensity
  let status: SceneLoopStatus = 'paused'
  let reducedMotion = options.reducedMotion
  let paused = true
  let destroyed = false
  let handle: number | undefined
  let lastPaint = 0
  let sceneTime = 0
  let width = 0
  let height = 0
  let pixelRatio = 1
  let windowStart = 0
  let windowFrames = 0
  let windowWork = 0
  let missedWindows = 0
  let healthyWindows = 0

  function setStatus(next: SceneLoopStatus): void {
    if (status === next) return
    status = next
    options.onStatus?.(next)
  }

  function entranceProgress(): number {
    return options.entranceSeconds <= 0 ? 1 : Math.min(1, sceneTime / options.entranceSeconds)
  }

  function paint(dt: number): void {
    const frame: SceneFrame = { dt, time: sceneTime, entrance: entranceProgress(), pixelRatio }
    options.onFrame?.(frame)
    if (!context || !painter || width <= 0 || height <= 0) return
    context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
    context.clearRect(0, 0, width, height)
    painter.frame(context, frame)
  }

  /** Static presentation for reduced motion or a suspended loop: settle, then draw once. */
  function paintStill(): void {
    if (!painter) {
      options.onFrame?.({ dt: 0, time: Math.max(sceneTime, options.entranceSeconds), entrance: 1, pixelRatio })
      return
    }
    if (sceneTime < options.entranceSeconds + 6) {
      const step = 1 / 15
      while (sceneTime < options.entranceSeconds + 6) {
        sceneTime += step
        if (context && width > 0 && height > 0) painter.frame(context, { dt: step, time: sceneTime, entrance: 1, pixelRatio })
      }
    }
    paint(0)
  }

  function evaluate(timestamp: number, work: number): void {
    windowFrames += 1
    windowWork += work
    if (!windowStart) windowStart = timestamp
    const elapsed = timestamp - windowStart
    if (elapsed < SCENE_GOVERNOR_WINDOW_MS) return
    const averageWork = windowWork / windowFrames
    const deliveredFps = (windowFrames * 1000) / elapsed
    windowStart = timestamp
    windowFrames = 0
    windowWork = 0
    const missed = averageWork > SCENE_WORK_BUDGET_MS || deliveredFps < fps * 0.6
    if (missed) {
      healthyWindows = 0
      missedWindows += 1
      // One slow window can be a tab switch or GC; act on two in a row.
      if (missedWindows < 2) return
      missedWindows = 0
      if (density > SCENE_MIN_DENSITY + 1e-6) {
        density = Math.max(SCENE_MIN_DENSITY, density * 0.6)
        painter?.setDensity(density)
        options.onDensity?.(density)
        return
      }
      stopFrames()
      paintStill()
      setStatus('suspended')
      return
    }
    missedWindows = 0
    healthyWindows += 1
    if (healthyWindows >= 5 && averageWork < SCENE_WORK_BUDGET_MS / 3 && density < maxDensity) {
      healthyWindows = 0
      density = Math.min(maxDensity, density * 1.25)
      painter?.setDensity(density)
      options.onDensity?.(density)
    }
  }

  function tick(timestamp: number): void {
    handle = requestFrame(tick)
    if (lastPaint && timestamp - lastPaint < interval - 1.5) return
    const dt = lastPaint ? Math.min((timestamp - lastPaint) / 1000, 0.1) : 1 / fps
    lastPaint = timestamp
    sceneTime += dt
    const started = now()
    paint(dt)
    evaluate(timestamp, now() - started)
  }

  function stopFrames(): void {
    if (handle !== undefined) cancelFrame(handle)
    handle = undefined
    lastPaint = 0
    windowStart = 0
    windowFrames = 0
    windowWork = 0
  }

  function sync(): void {
    if (destroyed || status === 'suspended') return
    if (reducedMotion) {
      stopFrames()
      paintStill()
      setStatus('static')
      return
    }
    if (paused) {
      stopFrames()
      setStatus('paused')
      return
    }
    if (handle === undefined) handle = requestFrame(tick)
    setStatus('running')
  }

  return {
    resize(nextWidth: number, nextHeight: number) {
      width = Math.max(0, Math.round(nextWidth))
      height = Math.max(0, Math.round(nextHeight))
      pixelRatio = Math.min(pixelRatioSource(), options.maxPixelRatio ?? 1.25)
      const viewport: SceneViewport = { width, height, image: coverImageRect(width, height) }
      if (options.canvas && context) {
        options.canvas.width = Math.max(1, Math.round(width * pixelRatio))
        options.canvas.height = Math.max(1, Math.round(height * pixelRatio))
      }
      painter?.resize(viewport)
      if (status === 'static' || status === 'suspended' || (paused && sceneTime > 0)) paint(0)
    },
    setPaused(next: boolean) {
      paused = next
      sync()
    },
    setReducedMotion(next: boolean) {
      reducedMotion = next
      sync()
    },
    setParams(params) {
      painter?.setParams?.(params)
    },
    refresh() {
      if (status !== 'running' && !destroyed) paint(0)
    },
    destroy() {
      destroyed = true
      stopFrames()
    },
    get density() {
      return density
    },
    get status() {
      return status
    },
  }
}
