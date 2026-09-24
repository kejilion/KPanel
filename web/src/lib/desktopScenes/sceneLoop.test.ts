import { describe, expect, it, vi } from 'vitest'
import type { ScenePainter, ScenePainterFactory, SceneFrame } from './painterKit'
import { createSceneLoop, SCENE_GOVERNOR_WINDOW_MS, SCENE_MIN_DENSITY, SCENE_RETRY_MS } from './sceneLoop'

interface Harness {
  frames: SceneFrame[]
  densities: number[]
  statuses: string[]
  pending: () => number
  /** Timers the loop scheduled (suspension retries), in order. */
  timers: { callback: () => void, delay: number }[]
  /** Advances the fake display by `count` refreshes of `interval` ms, spending `work` ms per paint. */
  run: (count: number, interval?: number, work?: number) => void
  loop: ReturnType<typeof createSceneLoop>
}

function harness(options: { reducedMotion?: boolean, density?: number } = {}): Harness {
  const frames: SceneFrame[] = []
  const densities: number[] = []
  const statuses: string[] = []
  let queue: FrameRequestCallback[] = []
  const timers: { callback: () => void, delay: number }[] = []
  let clock = 0
  let workPerPaint = 0.4
  let painting = false
  const painter: ScenePainter = {
    resize: vi.fn(),
    setDensity: vi.fn(),
    frame: (_context, frame) => {
      frames.push(frame)
      if (painting) clock += workPerPaint
    },
  }
  const context = { setTransform: vi.fn(), clearRect: vi.fn(), globalAlpha: 1, globalCompositeOperation: 'source-over' }
  const canvas = { width: 0, height: 0, getContext: () => context } as unknown as HTMLCanvasElement
  const factory: ScenePainterFactory = () => painter
  const loop = createSceneLoop({
    canvas,
    painter: factory,
    seed: 1,
    entranceSeconds: 2,
    density: options.density ?? 1,
    reducedMotion: options.reducedMotion ?? false,
    now: () => clock,
    requestFrame: (callback) => queue.push(callback),
    cancelFrame: () => { queue = [] },
    schedule: (callback, delay) => timers.push({ callback, delay }),
    unschedule: vi.fn(),
    devicePixelRatio: () => 2,
    onStatus: (status) => statuses.push(status),
    onDensity: (density) => densities.push(density),
  })
  loop.resize(1280, 720)
  return {
    frames,
    densities,
    statuses,
    timers,
    pending: () => queue.length,
    loop,
    run(count, interval = 1000 / 60, work = 0.4) {
      workPerPaint = work
      for (let index = 0; index < count; index++) {
        clock += interval
        const callbacks = queue
        queue = []
        painting = true
        for (const callback of callbacks) callback(clock)
        painting = false
      }
    },
  }
}

describe('scene loop', () => {
  it('stays idle until unpaused and paints at most thirty frames per second', () => {
    const test = harness()
    expect(test.pending()).toBe(0)
    test.loop.setPaused(false)
    expect(test.loop.status).toBe('running')
    test.run(120)
    expect(test.frames.length).toBeGreaterThanOrEqual(58)
    expect(test.frames.length).toBeLessThanOrEqual(61)
    expect(test.frames.at(-1)!.entrance).toBe(1)
  })

  it('stops requesting frames while paused and never jumps time after resuming', () => {
    const test = harness()
    test.loop.setPaused(false)
    test.run(30)
    test.loop.setPaused(true)
    expect(test.loop.status).toBe('paused')
    expect(test.pending()).toBe(0)
    const painted = test.frames.length
    test.run(300)
    expect(test.frames.length).toBe(painted)
    test.loop.setPaused(false)
    test.run(4)
    expect(Math.max(...test.frames.slice(painted).map((frame) => frame.dt))).toBeLessThanOrEqual(0.1)
    expect(test.statuses).toEqual(['running', 'paused', 'running'])
  })

  it('paints one settled still for reduced motion without scheduling frames', () => {
    const test = harness({ reducedMotion: true })
    test.loop.setPaused(false)
    expect(test.loop.status).toBe('static')
    expect(test.pending()).toBe(0)
    expect(test.frames.at(-1)).toMatchObject({ dt: 0, entrance: 1 })
  })

  it('caps the backing store at 1.25 device pixels', () => {
    const test = harness()
    test.loop.setPaused(false)
    test.run(2)
    expect(test.frames.at(-1)!.pixelRatio).toBe(1.25)
  })

  it('thins particles when paints exceed the work budget, then suspends at the floor', () => {
    const test = harness()
    test.loop.setPaused(false)
    const windowFrames = Math.ceil(SCENE_GOVERNOR_WINDOW_MS / (1000 / 60)) + 2
    // Each paint costs 9 ms: every window misses, and the loop acts on the second miss.
    for (let index = 0; index < 20 && test.loop.status === 'running'; index++) test.run(windowFrames, 1000 / 60, 9)
    expect(test.densities.filter((value) => value < 1)).toEqual([0.6, 0.36, SCENE_MIN_DENSITY])
    expect(test.loop.status).toBe('suspended')
    expect(test.pending()).toBe(0)
    test.loop.setPaused(false)
    expect(test.loop.status).toBe('suspended')
  })

  it('retries a suspended scene after a doubling back-off', () => {
    const test = harness()
    test.loop.setPaused(false)
    const windowFrames = Math.ceil(SCENE_GOVERNOR_WINDOW_MS / (1000 / 60)) + 2
    const suspend = () => {
      for (let index = 0; index < 20 && test.loop.status === 'running'; index++) test.run(windowFrames, 1000 / 60, 9)
    }
    suspend()
    expect(test.loop.status).toBe('suspended')
    expect(test.timers.map((timer) => timer.delay)).toEqual([SCENE_RETRY_MS])
    test.timers[0]!.callback()
    expect(test.loop.status).toBe('running')
    expect(test.pending()).toBe(1)
    suspend()
    expect(test.timers.map((timer) => timer.delay)).toEqual([SCENE_RETRY_MS, SCENE_RETRY_MS * 2])
    test.loop.destroy()
    test.timers[1]!.callback()
    expect(test.loop.status).toBe('suspended')
    expect(test.pending()).toBe(0)
  })

  it('keeps timing the scene when a painter cannot be built', () => {
    const onFrame = vi.fn()
    let queued: FrameRequestCallback | undefined
    const loop = createSceneLoop({
      canvas: { getContext: () => ({ setTransform: vi.fn(), clearRect: vi.fn() }) } as unknown as HTMLCanvasElement,
      painter: () => { throw new Error('sprite surface unavailable') },
      seed: 1,
      entranceSeconds: 1,
      density: 1,
      reducedMotion: false,
      onFrame,
      requestFrame: (callback) => { queued = callback; return 1 },
      cancelFrame: () => { queued = undefined },
    })
    loop.resize(800, 600)
    loop.setPaused(false)
    queued?.(16)
    expect(onFrame).toHaveBeenCalledTimes(1)
    expect(loop.status).toBe('running')
  })

  it('treats one slow window as noise and restores density after sustained headroom', () => {
    const test = harness()
    test.loop.setPaused(false)
    const windowFrames = Math.ceil(SCENE_GOVERNOR_WINDOW_MS / (1000 / 60)) + 2
    test.run(windowFrames, 1000 / 60, 9)
    test.run(windowFrames, 1000 / 60, 0.2)
    expect(test.loop.density).toBe(1)
    test.run(windowFrames * 2, 1000 / 60, 9)
    expect(test.loop.density).toBeCloseTo(0.6)
    test.run(windowFrames * 6, 1000 / 60, 0.2)
    expect(test.loop.density).toBeCloseTo(0.75)
  })

  it('cancels its frame on destroy', () => {
    const test = harness()
    test.loop.setPaused(false)
    test.loop.destroy()
    expect(test.pending()).toBe(0)
    test.loop.setPaused(false)
    expect(test.pending()).toBe(0)
  })

  it('still drives scene timing when no canvas context is available', () => {
    const onFrame = vi.fn()
    let queued: FrameRequestCallback | undefined
    const loop = createSceneLoop({
      canvas: { getContext: () => null } as unknown as HTMLCanvasElement,
      painter: () => { throw new Error('painter must not be created without a context') },
      seed: 1,
      entranceSeconds: 1,
      density: 1,
      reducedMotion: false,
      onFrame,
      requestFrame: (callback) => { queued = callback; return 1 },
      cancelFrame: () => { queued = undefined },
    })
    loop.resize(800, 600)
    loop.setPaused(false)
    queued?.(16)
    expect(onFrame).toHaveBeenCalledTimes(1)
  })
})
