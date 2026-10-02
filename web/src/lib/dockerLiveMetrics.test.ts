import { effectScope, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  dockerLiveMetricsConcurrency,
  dockerLiveMetricsInterval,
  dockerLiveMetricsLimit,
  summarizeDockerLive,
  toDockerLiveSample,
  useDockerLiveMetrics,
} from './dockerLiveMetrics'
import type { DockerContainerStats } from '@/types/api'

function stats(id: string, overrides: Partial<DockerContainerStats> = {}): DockerContainerStats {
  return {
    containerId: id,
    cpuPercent: 12.5,
    memoryBytes: 100,
    memoryLimitBytes: 1000,
    memoryPercent: 10,
    networkRxBytes: 1000,
    networkTxBytes: 500,
    blockReadBytes: 4000,
    blockWriteBytes: 2000,
    pids: 4,
    collectedAt: '2026-10-02T10:00:00Z',
    ...overrides,
  }
}

describe('toDockerLiveSample', () => {
  it('has no rates until a second sample exists', () => {
    const sample = toDockerLiveSample(stats('a'))
    expect(sample.networkRxRate).toBeUndefined()
    expect(sample.blockWriteRate).toBeUndefined()
    expect(sample.cpuPercent).toBe(12.5)
  })

  it('derives per-second rates from the counter deltas and the server timestamps', () => {
    const first = toDockerLiveSample(stats('a'))
    const second = toDockerLiveSample(stats('a', {
      collectedAt: '2026-10-02T10:00:04Z',
      networkRxBytes: 5000,
      networkTxBytes: 500,
      blockReadBytes: 4000,
      blockWriteBytes: 2800,
    }), first)
    expect(second.networkRxRate).toBe(1000)
    expect(second.networkTxRate).toBe(0)
    expect(second.blockReadRate).toBe(0)
    expect(second.blockWriteRate).toBe(200)
  })

  it('drops a rate when the counter went backwards (container restarted)', () => {
    const first = toDockerLiveSample(stats('a', { networkRxBytes: 9000 }))
    const second = toDockerLiveSample(stats('a', { collectedAt: '2026-10-02T10:00:03Z', networkRxBytes: 10 }), first)
    expect(second.networkRxRate).toBeUndefined()
    expect(second.networkTxRate).toBe(0)
  })

  it('ignores sample pairs that are too close together or share a timestamp', () => {
    const first = toDockerLiveSample(stats('a'))
    const same = toDockerLiveSample(stats('a', { networkRxBytes: 9000 }), first)
    expect(same.networkRxRate).toBeUndefined()
  })

  it('sanitises non-finite or negative numbers from the response', () => {
    const sample = toDockerLiveSample(stats('a', { cpuPercent: Number.NaN, memoryBytes: -5 }))
    expect(sample.cpuPercent).toBe(0)
    expect(sample.memoryBytes).toBe(0)
  })
})

describe('summarizeDockerLive', () => {
  it('adds up only containers that have a sample', () => {
    const entries = {
      a: { sample: toDockerLiveSample(stats('a', { cpuPercent: 10, memoryBytes: 100 })), failed: false },
      b: { sample: toDockerLiveSample(stats('b', { cpuPercent: 5, memoryBytes: 50 })), failed: true },
      c: { failed: true },
    }
    expect(summarizeDockerLive([{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }], entries))
      .toEqual({ sampled: 2, cpuPercent: 15, memoryBytes: 150 })
  })
})

describe('useDockerLiveMetrics', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  function setup(
    ids: string[],
    request: (id: string, signal: AbortSignal) => Promise<DockerContainerStats>,
    activeInitially = true,
  ) {
    const targets = ref(ids.map((id) => ({ id })))
    const active = ref(activeInitially)
    const scope = effectScope()
    const metrics = scope.run(() => useDockerLiveMetrics(targets, active, request))!
    return { targets, active, scope, metrics }
  }

  it('does not request anything while inactive', async () => {
    const request = vi.fn(async (id: string) => stats(id))
    const { scope } = setup(['a'], request, false)
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 3)
    expect(request).not.toHaveBeenCalled()
    scope.stop()
  })

  it('samples every target, keeps passes going, and computes rates on the second pass', async () => {
    let tick = 0
    const request = vi.fn(async (id: string) => {
      tick += 1
      return stats(id, {
        collectedAt: new Date(Date.UTC(2026, 9, 2, 10, 0, tick * 2)).toISOString(),
        networkRxBytes: tick * 2000,
      })
    })
    const { metrics, scope } = setup(['a'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(request).toHaveBeenCalledTimes(1)
    expect(metrics.entries.value.a?.sample?.networkRxRate).toBeUndefined()
    expect(metrics.sampled.value).toBe(true)

    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval)
    expect(request).toHaveBeenCalledTimes(2)
    expect(metrics.entries.value.a?.sample?.networkRxRate).toBe(1000)
    scope.stop()
  })

  it('never has more than the concurrency limit in flight and honours the container limit', async () => {
    let inFlight = 0
    let peak = 0
    const releases: Array<() => void> = []
    const request = vi.fn((id: string) => new Promise<DockerContainerStats>((resolve) => {
      inFlight += 1
      peak = Math.max(peak, inFlight)
      releases.push(() => { inFlight -= 1; resolve(stats(id)) })
    }))
    const ids = Array.from({ length: dockerLiveMetricsLimit + 5 }, (_, index) => `c${index}`)
    const { metrics, scope } = setup(ids, request)
    await vi.advanceTimersByTimeAsync(0)
    expect(request).toHaveBeenCalledTimes(dockerLiveMetricsConcurrency)

    // Drain the whole first pass.
    for (let guard = 0; guard < 100 && releases.length; guard += 1) {
      releases.shift()!()
      await vi.advanceTimersByTimeAsync(0)
    }
    expect(peak).toBe(dockerLiveMetricsConcurrency)
    expect(request).toHaveBeenCalledTimes(dockerLiveMetricsLimit)
    expect(Object.keys(metrics.entries.value)).toHaveLength(dockerLiveMetricsLimit)
    expect(metrics.truncated.value).toBe(5)
    scope.stop()
  })

  it('keeps the last good sample but flags a failed refresh', async () => {
    let fail = false
    const request = vi.fn(async (id: string) => {
      if (fail) throw new Error('boom')
      return stats(id)
    })
    const { metrics, scope } = setup(['a'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(metrics.entries.value.a).toMatchObject({ failed: false })

    fail = true
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval)
    expect(metrics.entries.value.a?.failed).toBe(true)
    expect(metrics.entries.value.a?.sample?.cpuPercent).toBe(12.5)
    expect(metrics.failing.value).toBe(true)
    scope.stop()
  })

  it('records a failure with no sample when a container never answers', async () => {
    const request = vi.fn(async () => { throw new Error('nope') })
    const { metrics, scope } = setup(['a'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(metrics.entries.value.a).toEqual({ sample: undefined, failed: true })
    expect(metrics.sampled.value).toBe(false)
    scope.stop()
  })

  it('aborts in-flight requests and forgets samples when it goes inactive', async () => {
    const signals: AbortSignal[] = []
    const request = vi.fn((_id: string, signal: AbortSignal) => {
      signals.push(signal)
      return new Promise<DockerContainerStats>((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
      })
    })
    const { metrics, active, scope } = setup(['a', 'b'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(signals).toHaveLength(2)

    active.value = false
    await nextTick()
    expect(signals.every((signal) => signal.aborted)).toBe(true)
    expect(metrics.entries.value).toEqual({})
    expect(metrics.sampling.value).toBe(false)

    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 3)
    expect(request).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('discards samples for containers that left the target set', async () => {
    const request = vi.fn(async (id: string) => stats(id))
    const { metrics, targets, scope } = setup(['a', 'b'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(Object.keys(metrics.entries.value).sort()).toEqual(['a', 'b'])

    targets.value = [{ id: 'b' }]
    await nextTick()
    expect(Object.keys(metrics.entries.value)).toEqual(['b'])
    scope.stop()
  })

  it('stops requesting once its scope is disposed', async () => {
    const request = vi.fn(async (id: string) => stats(id))
    const { scope } = setup(['a'], request)
    await vi.advanceTimersByTimeAsync(0)
    expect(request).toHaveBeenCalledTimes(1)
    scope.stop()
    await vi.advanceTimersByTimeAsync(dockerLiveMetricsInterval * 5)
    expect(request).toHaveBeenCalledTimes(1)
  })
})
