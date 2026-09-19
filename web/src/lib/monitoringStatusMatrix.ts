import type { MonitoringOperatorLatencyPoint, MonitoringOperatorLatencySeries } from '@/types/api'

export type MonitoringStatusCellState = 'success' | 'partial' | 'failure' | 'missing'

export interface MonitoringStatusCell {
  start: string
  end: string
  state: MonitoringStatusCellState
  successCount: number
  failureCount: number
}

export function monitoringStatusCellCount(
  series: MonitoringOperatorLatencySeries[],
  minimum = 12,
  maximum = 36,
): number {
  const largestSampleCount = series.reduce((largest, item) => Math.max(largest, item.points.length), 0)
  return Math.min(maximum, Math.max(minimum, largestSampleCount))
}

export function monitoringStatusCells(
  points: MonitoringOperatorLatencyPoint[],
  start: string,
  end: string,
  count: number,
): MonitoringStatusCell[] {
  const startMilliseconds = Date.parse(start)
  const endMilliseconds = Date.parse(end)
  if (!Number.isFinite(startMilliseconds) || !Number.isFinite(endMilliseconds) || endMilliseconds <= startMilliseconds) return []

  const cellCount = Math.max(1, Math.floor(count))
  const duration = endMilliseconds - startMilliseconds
  const samples = Array.from({ length: cellCount }, () => ({ successCount: 0, failureCount: 0 }))

  for (const point of points) {
    const collectedAt = Date.parse(point.collectedAt)
    if (!Number.isFinite(collectedAt) || collectedAt < startMilliseconds || collectedAt > endMilliseconds) continue
    const index = Math.min(cellCount - 1, Math.max(0, Math.floor(((collectedAt - startMilliseconds) / duration) * cellCount)))
    const sampleCount = Math.max(0, point.successCount || 0) + Math.max(0, point.failureCount || 0)
    if (sampleCount > 0) {
      samples[index]!.successCount += Math.max(0, point.successCount || 0)
      samples[index]!.failureCount += Math.max(0, point.failureCount || 0)
    } else if (point.latencyMilliseconds === null) {
      samples[index]!.failureCount += 1
    } else {
      samples[index]!.successCount += 1
    }
  }

  return samples.map((sample, index) => {
    const cellStart = startMilliseconds + (duration * index) / cellCount
    const cellEnd = startMilliseconds + (duration * (index + 1)) / cellCount
    let state: MonitoringStatusCellState = 'missing'
    if (sample.successCount > 0 && sample.failureCount > 0) state = 'partial'
    else if (sample.successCount > 0) state = 'success'
    else if (sample.failureCount > 0) state = 'failure'
    return {
      start: new Date(cellStart).toISOString(),
      end: new Date(cellEnd).toISOString(),
      state,
      successCount: sample.successCount,
      failureCount: sample.failureCount,
    }
  })
}
