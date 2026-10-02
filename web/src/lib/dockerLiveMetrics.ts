import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { DockerContainer, DockerContainerStats } from '@/types/api'

export type DockerLiveMetricsTarget = Pick<DockerContainer, 'id'>

export interface DockerLiveSample {
  cpuPercent: number
  memoryBytes: number
  memoryLimitBytes: number
  memoryPercent: number
  pids: number
  networkRxBytes: number
  networkTxBytes: number
  blockReadBytes: number
  blockWriteBytes: number
  /** Bytes per second between the last two samples; undefined until a second sample arrives. */
  networkRxRate?: number
  networkTxRate?: number
  blockReadRate?: number
  blockWriteRate?: number
  collectedAt: string
}

export interface DockerLiveEntry {
  /** Last successful sample. Kept while later attempts fail so the row does not blank out. */
  sample?: DockerLiveSample
  /** The most recent attempt failed. */
  failed: boolean
}

export interface DockerLiveSummary {
  sampled: number
  cpuPercent: number
  memoryBytes: number
}

// Each Docker stats call blocks for about a second while the engine takes two
// readings, so concurrency is what bounds both the pass duration and the load
// on a small host. Keep it deliberately low.
export const dockerLiveMetricsConcurrency = 3
export const dockerLiveMetricsLimit = 32
export const dockerLiveMetricsInterval = 2_000
export const dockerLiveMetricsTimeout = 12_000

const minimumRateWindowSeconds = 0.5

function finiteNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : 0
}

function counterRate(current: number, previous: number | undefined, seconds: number | undefined): number | undefined {
  if (previous === undefined || seconds === undefined || current < previous) return undefined
  return (current - previous) / seconds
}

export function toDockerLiveSample(
  stats: DockerContainerStats,
  previous?: DockerLiveSample,
): DockerLiveSample {
  const currentTime = Date.parse(stats.collectedAt)
  const previousTime = previous ? Date.parse(previous.collectedAt) : Number.NaN
  const seconds = Number.isFinite(currentTime) && Number.isFinite(previousTime)
    ? (currentTime - previousTime) / 1000
    : undefined
  const window = seconds !== undefined && seconds >= minimumRateWindowSeconds ? seconds : undefined
  const sample: DockerLiveSample = {
    cpuPercent: finiteNumber(stats.cpuPercent),
    memoryBytes: finiteNumber(stats.memoryBytes),
    memoryLimitBytes: finiteNumber(stats.memoryLimitBytes),
    memoryPercent: finiteNumber(stats.memoryPercent),
    pids: finiteNumber(stats.pids),
    networkRxBytes: finiteNumber(stats.networkRxBytes),
    networkTxBytes: finiteNumber(stats.networkTxBytes),
    blockReadBytes: finiteNumber(stats.blockReadBytes),
    blockWriteBytes: finiteNumber(stats.blockWriteBytes),
    collectedAt: stats.collectedAt,
  }
  sample.networkRxRate = counterRate(sample.networkRxBytes, previous?.networkRxBytes, window)
  sample.networkTxRate = counterRate(sample.networkTxBytes, previous?.networkTxBytes, window)
  sample.blockReadRate = counterRate(sample.blockReadBytes, previous?.blockReadBytes, window)
  sample.blockWriteRate = counterRate(sample.blockWriteBytes, previous?.blockWriteBytes, window)
  return sample
}

export function summarizeDockerLive(
  containers: readonly DockerLiveMetricsTarget[],
  entries: Readonly<Record<string, DockerLiveEntry | undefined>>,
): DockerLiveSummary {
  const summary: DockerLiveSummary = { sampled: 0, cpuPercent: 0, memoryBytes: 0 }
  for (const container of containers) {
    const sample = entries[container.id]?.sample
    if (!sample) continue
    summary.sampled += 1
    summary.cpuPercent += sample.cpuPercent
    summary.memoryBytes += sample.memoryBytes
  }
  return summary
}

/**
 * Polls the single-container stats endpoint for the given running containers
 * while `active` is true. Passes run back to back with a short pause, at most
 * `dockerLiveMetricsConcurrency` requests in flight, and never overlap. Turning
 * `active` off aborts in-flight requests and forgets every sample so that a
 * later activation never shows stale numbers as live.
 */
export function useDockerLiveMetrics(
  targets: Readonly<Ref<readonly DockerLiveMetricsTarget[]>>,
  active: Readonly<Ref<boolean>>,
  request: (id: string, signal: AbortSignal) => Promise<DockerContainerStats>,
) {
  const entries = ref<Record<string, DockerLiveEntry>>({})
  const sampling = ref(false)
  const controllers = new Set<AbortController>()
  let timer: ReturnType<typeof setTimeout> | undefined
  let generation = 0
  let disposed = false

  const sampled = computed(() => Object.values(entries.value).some((entry) => entry.sample))
  const failing = computed(() => Object.values(entries.value).some((entry) => entry.failed))
  const lastCollectedAt = computed(() => {
    let latest: string | undefined
    let latestTime = Number.NEGATIVE_INFINITY
    for (const entry of Object.values(entries.value)) {
      const time = entry.sample ? Date.parse(entry.sample.collectedAt) : Number.NaN
      if (Number.isFinite(time) && time > latestTime) {
        latestTime = time
        latest = entry.sample?.collectedAt
      }
    }
    return latest
  })
  const truncated = computed(() => Math.max(0, targets.value.length - dockerLiveMetricsLimit))

  function currentIDs(): Set<string> {
    return new Set(targets.value.slice(0, dockerLiveMetricsLimit).map((item) => item.id))
  }

  function prune(): void {
    const keep = currentIDs()
    for (const id of Object.keys(entries.value)) {
      if (!keep.has(id)) delete entries.value[id]
    }
  }

  function stop(): void {
    generation += 1
    clearTimeout(timer)
    timer = undefined
    for (const controller of controllers) controller.abort()
    controllers.clear()
    sampling.value = false
    entries.value = {}
  }

  function schedule(delay: number): void {
    clearTimeout(timer)
    if (disposed || !active.value) return
    const scheduledGeneration = generation
    timer = setTimeout(() => {
      timer = undefined
      if (scheduledGeneration === generation) void pass(scheduledGeneration)
    }, delay)
  }

  async function sample(id: string, passGeneration: number): Promise<void> {
    const controller = new AbortController()
    controllers.add(controller)
    const timeout = setTimeout(() => controller.abort(), dockerLiveMetricsTimeout)
    try {
      const stats = await request(id, controller.signal)
      if (passGeneration !== generation || !currentIDs().has(id)) return
      if (!stats || typeof stats.collectedAt !== 'string') throw new Error('Unconfirmed container stats response')
      entries.value[id] = {
        sample: toDockerLiveSample(stats, entries.value[id]?.sample),
        failed: false,
      }
    } catch {
      if (passGeneration !== generation || !currentIDs().has(id)) return
      entries.value[id] = { sample: entries.value[id]?.sample, failed: true }
    } finally {
      clearTimeout(timeout)
      controllers.delete(controller)
    }
  }

  async function pass(passGeneration: number): Promise<void> {
    if (disposed || !active.value || passGeneration !== generation) return
    const queue = targets.value.slice(0, dockerLiveMetricsLimit).map((item) => item.id)
    if (!queue.length) {
      schedule(dockerLiveMetricsInterval)
      return
    }
    sampling.value = true
    let index = 0
    const workers = Array.from({ length: Math.min(dockerLiveMetricsConcurrency, queue.length) }, async () => {
      while (index < queue.length && passGeneration === generation && active.value) {
        const id = queue[index]
        index += 1
        if (id) await sample(id, passGeneration)
      }
    })
    await Promise.all(workers)
    if (passGeneration !== generation) return
    sampling.value = false
    schedule(dockerLiveMetricsInterval)
  }

  watch(active, (enabled) => {
    stop()
    if (enabled && !disposed) void pass(generation)
  }, { immediate: true, flush: 'sync' })

  // Drop rows that left the target set (stopped, filtered out) right away; the
  // running pass keeps its own snapshot and discards results for removed ids.
  watch(targets, prune, { flush: 'sync' })

  onScopeDispose(() => {
    disposed = true
    stop()
  })

  return { entries, sampling, sampled, failing, lastCollectedAt, truncated }
}
