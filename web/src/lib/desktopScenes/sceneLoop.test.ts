import { describe, expect, it, vi } from 'vitest'
import { SCENE_QUALITY_LEVELS, createSceneLoop, localClockHour, scenePixelSize, type SceneLoopStatus } from './sceneLoop'
import type { SceneInstance, SceneModule, SceneSurface } from './types'

function harness(options: { renderCost?: number, load?: () => Promise<SceneModule>, render?: SceneInstance['render'] } = {}) {
  const surface = { width: 300, height: 150, getContext: () => ({}) } as unknown as SceneSurface
  const scene = { resize: vi.fn(), render: vi.fn(options.render ?? (() => {})) }
  const frames: Array<(now: number) => void> = []
  const statuses: SceneLoopStatus[] = []
  let clock = 0
  const renderCost = options.renderCost ?? 1
  scene.render.mockImplementation((frame) => {
    clock += renderCost
    options.render?.(frame)
  })
  const loop = createSceneLoop({
    sceneId: 'aurora',
    surface,
    createSurface: () => surface,
    schedule: (callback) => {
      frames.push(callback)
      return () => {
        const index = frames.indexOf(callback)
        if (index >= 0) frames.splice(index, 1)
      }
    },
    now: () => clock,
    onStatus: (status) => statuses.push(status),
    load: options.load ?? (async () => ({ createScene: () => scene })),
    clockHour: () => 9.5,
  })
  const flush = async () => {
    await Promise.resolve()
    await Promise.resolve()
  }
  const step = (now: number) => {
    clock = now
    const callback = frames.shift()
    callback?.(now)
  }
  return { loop, scene, surface, frames, statuses, flush, step }
}

describe('desktop scene loop', () => {
  it('reads the local wall clock as a fractional hour', () => {
    expect(localClockHour(new Date(2026, 8, 23, 18, 30, 36))).toBeCloseTo(18.51)
  })

  it('caps the device scale and the total pixel budget', () => {
    expect(scenePixelSize(1280, 800, 1, 0)).toEqual({ width: 1280, height: 800 })
    expect(scenePixelSize(1280, 800, 3, 0)).toEqual({ width: 1920, height: 1200 })
    const huge = scenePixelSize(3840, 2160, 2, 0)
    expect(huge.width * huge.height).toBeLessThanOrEqual(2_400_000 + 4000)
    expect(scenePixelSize(1000, 500, 1, 3)).toEqual({ width: 500, height: 250 })
  })

  it('paints one frame before reporting ready and then animates at the capped frame rate', async () => {
    const { loop, scene, surface, statuses, flush, step } = harness()
    loop.resize(1280, 800, 1)
    await flush()
    expect(surface.width).toBe(1280)
    expect(scene.resize).toHaveBeenCalledWith(1280, 800)
    expect(scene.render).toHaveBeenCalledTimes(1)
    expect(scene.render.mock.calls[0]![0]).toMatchObject({ time: 0, hour: 9.5 })
    expect(statuses).toEqual([{ type: 'ready', quality: 0 }])

    step(1000)
    step(1016)
    step(1033)
    step(1050)
    // 60 Hz callbacks against a 30 fps target: every other callback paints.
    expect(scene.render).toHaveBeenCalledTimes(3)
    expect(scene.render.mock.calls[2]![0].time).toBeCloseTo(0.033)
  })

  it('reports ready once the size arrives when the chunk loads first', async () => {
    const { loop, scene, statuses, flush } = harness()
    await flush()
    expect(statuses).toEqual([])
    loop.resize(800, 600, 1)
    expect(scene.render).toHaveBeenCalledTimes(1)
    expect(statuses).toEqual([{ type: 'ready', quality: 0 }])
  })

  it('stops scheduling while paused and keeps a single still frame for reduced motion', async () => {
    vi.useFakeTimers()
    const { loop, scene, frames, flush } = harness()
    loop.setPaused(true)
    loop.resize(640, 480, 1)
    await flush()
    expect(scene.render).toHaveBeenCalledTimes(1)
    expect(frames).toHaveLength(0)

    loop.setPaused(false)
    expect(frames).toHaveLength(1)
    loop.setReducedMotion(true)
    expect(frames).toHaveLength(0)
    expect(scene.render).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(scene.render).toHaveBeenCalledTimes(3)
    loop.dispose()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(scene.render).toHaveBeenCalledTimes(3)
    vi.useRealTimers()
  })

  it('steps quality down when frames stay over budget', async () => {
    const { loop, surface, statuses, flush, step } = harness({ renderCost: 12 })
    loop.resize(1000, 500, 1)
    await flush()
    let now = 0
    for (let frame = 0; frame < 45; frame++) step(now += 40)
    expect(statuses).toContainEqual({ type: 'quality', quality: 1 })
    expect(surface.width).toBe(Math.round(1000 * SCENE_QUALITY_LEVELS[1].scale))
  })

  it('reports download and render failures without throwing', async () => {
    const failedLoad = harness({ load: () => Promise.reject(new Error('chunk')) })
    failedLoad.loop.resize(800, 600, 1)
    await failedLoad.flush()
    expect(failedLoad.statuses).toEqual([{ type: 'error', reason: 'load' }])

    const failedRender = harness({ render: () => { throw new Error('boom') } })
    failedRender.loop.resize(800, 600, 1)
    await failedRender.flush()
    expect(failedRender.statuses).toEqual([{ type: 'error', reason: 'render' }])
    expect(failedRender.frames).toHaveLength(0)
  })
})
