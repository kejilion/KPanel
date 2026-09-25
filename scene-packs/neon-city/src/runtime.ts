import * as THREE from 'three'
import { deliverCommand, hostCommand, postToHost, sendEventsVia, type HostCommand, type PackEvent } from './bridge'

/**
 * Where the scene runs. The scene frame is sandboxed but on the desktop's own site, so the browser
 * runs it on the desktop's main thread: whatever the scene does there (decoding pictures, sending
 * them to the graphics card, compiling shaders, building geometry, drawing each frame) holds the
 * desktop up while it runs. So where the browser allows, the scene runs in a worker and draws
 * through an OffscreenCanvas; the page only hands its canvas over and passes messages and size
 * changes along. Where it does not, or the worker cannot start, the scene runs on the page.
 * ?worker=off keeps it on the page, to compare.
 */
export interface View {
  canvas: HTMLCanvasElement | OffscreenCanvas
  /** The page's size in CSS pixels, and its device pixel ratio; kept current before onResize runs. */
  width: number
  height: number
  pixelRatio: number
  params: URLSearchParams
  onResize(handler: () => void): void
  requestFrame(callback: (now: number) => void): number
  cancelFrame(handle: number): void
  /** No WebGL here: from a worker the page tries itself; on the page it is the end. */
  unavailable(): void
}

type ToWorker =
  | { kind: 'start', canvas: OffscreenCanvas, width: number, height: number, pixelRatio: number, search: string, base: string }
  | { kind: 'resize', width: number, height: number, pixelRatio: number }
  | { kind: 'command', command: HostCommand }
type FromWorker = { kind: 'event', event: PackEvent } | { kind: 'fallback' }

interface WorkerScope {
  postMessage(message: FromWorker): void
  addEventListener(type: 'message', listener: (event: MessageEvent<ToWorker>) => void): void
  requestAnimationFrame?: (callback: (now: number) => void) => number
  cancelAnimationFrame?: (handle: number) => void
}

let base = typeof location === 'undefined' ? '' : location.href

/** A file of the pack, by its path in the pack: fetched relative to the pack's page wherever the scene runs. */
export function packURL(path: string): string {
  return new URL(path, base).href
}

export function run(start: (view: View) => Promise<void>): void {
  if (typeof document === 'undefined') runInWorker(start)
  else runOnPage(start)
}

function begin(start: (view: View) => Promise<void>, view: View, from: string): void {
  base = from
  // Three's loaders resolve every file (and the files a model refers to) through this.
  THREE.DefaultLoadingManager.setURLModifier((url) => new URL(url, base).href)
  start(view).catch(() => postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'assets_unavailable' }))
}

function runInWorker(start: (view: View) => Promise<void>): void {
  const scope = self as unknown as WorkerScope
  const resized: (() => void)[] = []
  let view: View | undefined
  sendEventsVia((event) => scope.postMessage({ kind: 'event', event }))
  scope.addEventListener('message', ({ data }) => {
    if (data.kind === 'start' && !view) {
      const frame = scope.requestAnimationFrame?.bind(scope)
      const cancel = scope.cancelAnimationFrame?.bind(scope)
      view = {
        canvas: data.canvas,
        width: data.width,
        height: data.height,
        pixelRatio: data.pixelRatio,
        params: new URLSearchParams(data.search),
        onResize: (handler) => resized.push(handler),
        requestFrame: (callback) => (frame ? frame(callback) : setTimeout(() => callback(performance.now()), 16) as unknown as number),
        cancelFrame: (handle) => (cancel ? cancel(handle) : clearTimeout(handle)),
        unavailable: () => scope.postMessage({ kind: 'fallback' }),
      }
      begin(start, view, data.base)
    } else if (data.kind === 'resize' && view) {
      view.width = data.width
      view.height = data.height
      view.pixelRatio = data.pixelRatio
      for (const handler of resized) handler()
    } else if (data.kind === 'command') {
      deliverCommand(data.command)
    }
  })
}

function runOnPage(start: (view: View) => Promise<void>): void {
  const script = document.currentScript as HTMLScriptElement | null
  const size = () => ({ width: window.innerWidth, height: window.innerHeight, pixelRatio: window.devicePixelRatio || 1 })
  const addCanvas = () => document.body.appendChild(document.createElement('canvas'))
  let canvas = addCanvas()
  let worker: Worker | undefined
  let ready = false

  addEventListener('message', (event) => {
    const command = hostCommand(event)
    if (!command) return
    if (worker) worker.postMessage({ kind: 'command', command } satisfies ToWorker)
    else deliverCommand(command)
  })

  const onPage = () => {
    worker?.terminate()
    worker = undefined
    // A canvas handed to a worker cannot be drawn to here: start on a fresh one.
    canvas.remove()
    canvas = addCanvas()
    const resized: (() => void)[] = []
    const view: View = {
      canvas,
      ...size(),
      params: new URLSearchParams(location.search),
      onResize: (handler) => resized.push(handler),
      requestFrame: (callback) => requestAnimationFrame(callback),
      cancelFrame: (handle) => cancelAnimationFrame(handle),
      unavailable: () => postToHost({ source: 'kpanel-scene-pack', type: 'error', reason: 'webgl_unavailable' }),
    }
    addEventListener('resize', () => {
      Object.assign(view, size())
      for (const handler of resized) handler()
    })
    begin(start, view, location.href)
  }

  const canHandOver = typeof Worker !== 'undefined' && 'transferControlToOffscreen' in canvas && script?.src
  if (!canHandOver || new URLSearchParams(location.search).get('worker') === 'off') {
    onPage()
    return
  }
  try {
    // The frame's origin is opaque, so a worker can only be started from a blob: of its own; that
    // one loads the scene's script (the same file as this one, from the cache).
    const source = URL.createObjectURL(new Blob([`importScripts(${JSON.stringify(script.src)})`], { type: 'text/javascript' }))
    const offscreen = canvas.transferControlToOffscreen()
    worker = new Worker(source)
    worker.addEventListener('message', ({ data }: MessageEvent<FromWorker>) => {
      if (data.kind === 'fallback') {
        onPage()
      } else if (data.kind === 'event') {
        if (data.event.type === 'ready') ready = true
        if (window.parent !== window) window.parent.postMessage(data.event, '*')
      }
    })
    // The worker could not load or run the scene before it was up: the page takes over.
    worker.addEventListener('error', () => {
      if (!ready) onPage()
    })
    worker.postMessage({ kind: 'start', canvas: offscreen, ...size(), search: location.search, base: location.href } satisfies ToWorker, [offscreen])
    addEventListener('resize', () => worker?.postMessage({ kind: 'resize', ...size() } satisfies ToWorker))
  } catch {
    onPage()
  }
}
