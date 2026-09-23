import type { DesktopSceneID } from './loaders'
import { createSceneLoop, type SceneLoop, type SceneLoopStatus } from './sceneLoop'
import type { SceneSurface } from './types'

export type SceneHostMessage =
  | {
    type: 'init'
    canvas: OffscreenCanvas
    sceneId: DesktopSceneID
    width: number
    height: number
    pixelRatio: number
    paused: boolean
    reducedMotion: boolean
  }
  | { type: 'resize', width: number, height: number, pixelRatio: number }
  | { type: 'paused', value: boolean }
  | { type: 'reducedMotion', value: boolean }
  | { type: 'dispose' }

export type SceneHostError = 'load' | 'render' | 'worker'

export interface SceneHostOptions {
  sceneId: DesktopSceneID
  width: number
  height: number
  pixelRatio: number
  paused: boolean
  reducedMotion: boolean
  preferWorker: boolean
  onReady: () => void
  onQuality: (quality: number) => void
  /** `worker` means the worker could not boot; the canvas is spent and callers may retry inline. */
  onError: (reason: SceneHostError) => void
}

export interface SceneHost {
  readonly mode: 'worker' | 'inline'
  resize: (width: number, height: number, pixelRatio: number) => void
  setPaused: (paused: boolean) => void
  setReducedMotion: (reducedMotion: boolean) => void
  dispose: () => void
}

export function supportsSceneWorker(canvas: HTMLCanvasElement): boolean {
  return typeof Worker !== 'undefined'
    && typeof OffscreenCanvas !== 'undefined'
    && typeof canvas.transferControlToOffscreen === 'function'
}

function forwardStatus(status: SceneLoopStatus, options: SceneHostOptions): void {
  if (status.type === 'ready') options.onReady()
  else if (status.type === 'quality') options.onQuality(status.quality)
  else options.onError(status.reason)
}

function createWorkerHost(canvas: HTMLCanvasElement, options: SceneHostOptions): SceneHost {
  const offscreen = canvas.transferControlToOffscreen()
  const worker = new Worker(new URL('./sceneWorker.ts', import.meta.url), { type: 'module', name: 'kpanel-desktop-scene' })
  let disposed = false
  let booted = false
  worker.onmessage = (event: MessageEvent<SceneLoopStatus>) => {
    if (disposed) return
    booted = true
    forwardStatus(event.data, options)
  }
  worker.onerror = (event) => {
    event.preventDefault()
    if (disposed) return
    disposed = true
    worker.terminate()
    options.onError(booted ? 'render' : 'worker')
  }
  const post = (message: SceneHostMessage, transfer: Transferable[] = []) => {
    if (!disposed) worker.postMessage(message, transfer)
  }
  post({
    type: 'init',
    canvas: offscreen,
    sceneId: options.sceneId,
    width: options.width,
    height: options.height,
    pixelRatio: options.pixelRatio,
    paused: options.paused,
    reducedMotion: options.reducedMotion,
  }, [offscreen])
  return {
    mode: 'worker',
    resize: (width, height, pixelRatio) => post({ type: 'resize', width, height, pixelRatio }),
    setPaused: (value) => post({ type: 'paused', value }),
    setReducedMotion: (value) => post({ type: 'reducedMotion', value }),
    dispose() {
      post({ type: 'dispose' })
      disposed = true
      worker.terminate()
    },
  }
}

function createInlineSurface(width: number, height: number): SceneSurface {
  if (typeof OffscreenCanvas !== 'undefined') return new OffscreenCanvas(width, height)
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}

function createInlineHost(canvas: HTMLCanvasElement, options: SceneHostOptions): SceneHost {
  const loop: SceneLoop = createSceneLoop({
    sceneId: options.sceneId,
    surface: canvas,
    createSurface: createInlineSurface,
    schedule(callback) {
      const handle = window.requestAnimationFrame(callback)
      return () => window.cancelAnimationFrame(handle)
    },
    now: () => performance.now(),
    onStatus: (status) => forwardStatus(status, options),
  })
  loop.setPaused(options.paused)
  loop.setReducedMotion(options.reducedMotion)
  loop.resize(options.width, options.height, options.pixelRatio)
  return {
    mode: 'inline',
    resize: loop.resize,
    setPaused: loop.setPaused,
    setReducedMotion: loop.setReducedMotion,
    dispose: loop.dispose,
  }
}

export function createSceneHost(canvas: HTMLCanvasElement, options: SceneHostOptions): SceneHost {
  if (options.preferWorker && supportsSceneWorker(canvas)) {
    try {
      return createWorkerHost(canvas, options)
    } catch {
      // Worker construction failures surface as a spent canvas; let the caller remount inline.
      queueMicrotask(() => options.onError('worker'))
      return { mode: 'worker', resize() {}, setPaused() {}, setReducedMotion() {}, dispose() {} }
    }
  }
  return createInlineHost(canvas, options)
}
