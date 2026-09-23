// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createSceneHost, type SceneHostMessage, type SceneHostOptions } from './sceneHost'

class FakeWorker {
  static instances: FakeWorker[] = []
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: ErrorEvent) => void) | null = null
  messages: Array<{ message: SceneHostMessage, transfer: Transferable[] }> = []
  terminated = false

  constructor(readonly url: URL, readonly options: WorkerOptions) {
    FakeWorker.instances.push(this)
  }

  postMessage(message: SceneHostMessage, transfer: Transferable[] = []): void {
    this.messages.push({ message, transfer })
  }

  terminate(): void {
    this.terminated = true
  }

  emit(data: unknown): void {
    this.onmessage?.({ data } as MessageEvent)
  }

  fail(): void {
    this.onerror?.({ preventDefault() {} } as ErrorEvent)
  }
}

function options(overrides: Partial<SceneHostOptions> = {}): SceneHostOptions {
  return {
    sceneId: 'rain',
    width: 1280,
    height: 720,
    pixelRatio: 2,
    paused: false,
    reducedMotion: false,
    preferWorker: true,
    onReady: vi.fn(),
    onQuality: vi.fn(),
    onError: vi.fn(),
    ...overrides,
  }
}

function transferableCanvas(): HTMLCanvasElement {
  const canvas = document.createElement('canvas')
  const offscreen = { width: 300, height: 150 }
  Object.defineProperty(canvas, 'transferControlToOffscreen', { value: vi.fn(() => offscreen) })
  return canvas
}

describe('desktop scene host', () => {
  beforeEach(() => {
    FakeWorker.instances = []
    vi.stubGlobal('Worker', FakeWorker)
    vi.stubGlobal('OffscreenCanvas', class {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('transfers the canvas to a module worker and relays its status', () => {
    const hostOptions = options()
    const host = createSceneHost(transferableCanvas(), hostOptions)
    const worker = FakeWorker.instances[0]!
    expect(host.mode).toBe('worker')
    expect(worker.options).toMatchObject({ type: 'module' })
    expect(worker.messages[0]!.message).toMatchObject({ type: 'init', sceneId: 'rain', width: 1280, height: 720, pixelRatio: 2 })
    expect(worker.messages[0]!.transfer).toHaveLength(1)

    worker.emit({ type: 'ready', quality: 0 })
    worker.emit({ type: 'quality', quality: 2 })
    worker.emit({ type: 'error', reason: 'load' })
    expect(hostOptions.onReady).toHaveBeenCalledTimes(1)
    expect(hostOptions.onQuality).toHaveBeenCalledWith(2)
    expect(hostOptions.onError).toHaveBeenCalledWith('load')

    host.setPaused(true)
    host.resize(800, 600, 1)
    host.dispose()
    expect(worker.messages.slice(1).map((entry) => entry.message)).toEqual([
      { type: 'paused', value: true },
      { type: 'resize', width: 800, height: 600, pixelRatio: 1 },
      { type: 'dispose' },
    ])
    expect(worker.terminated).toBe(true)
    worker.emit({ type: 'ready', quality: 0 })
    expect(hostOptions.onReady).toHaveBeenCalledTimes(1)
  })

  it('reports a worker that fails before booting so the caller can retry inline', () => {
    const hostOptions = options()
    createSceneHost(transferableCanvas(), hostOptions)
    FakeWorker.instances[0]!.fail()
    expect(hostOptions.onError).toHaveBeenCalledWith('worker')
    expect(FakeWorker.instances[0]!.terminated).toBe(true)
  })

  it('reports a crash after booting as a render failure', () => {
    const hostOptions = options()
    createSceneHost(transferableCanvas(), hostOptions)
    const worker = FakeWorker.instances[0]!
    worker.emit({ type: 'ready', quality: 0 })
    worker.fail()
    expect(hostOptions.onError).toHaveBeenCalledWith('render')
  })

  it('renders on the main thread when workers are not preferred or unavailable', () => {
    const noTransfer = document.createElement('canvas')
    Object.defineProperty(noTransfer, 'getContext', { value: () => null })
    const inline = createSceneHost(noTransfer, options())
    expect(inline.mode).toBe('inline')
    expect(FakeWorker.instances).toHaveLength(0)

    const declined = transferableCanvas()
    Object.defineProperty(declined, 'getContext', { value: () => null })
    expect(createSceneHost(declined, options({ preferWorker: false })).mode).toBe('inline')
    expect(FakeWorker.instances).toHaveLength(0)
  })
})
