import { describe, expect, it } from 'vitest'
import { monitoringStatusCellCount, monitoringStatusCells } from './monitoringStatusMatrix'
import type { MonitoringOperatorLatencySeries } from '@/types/api'

describe('monitoring status matrix', () => {
  it('aggregates success, partial failure, failure, and missing cells on one timeline', () => {
    const cells = monitoringStatusCells([
      { collectedAt: '2026-09-11T00:10:00Z', latencyMilliseconds: 12, successCount: 1, failureCount: 0 },
      { collectedAt: '2026-09-11T00:20:00Z', latencyMilliseconds: null, successCount: 0, failureCount: 1 },
      { collectedAt: '2026-09-11T01:10:00Z', latencyMilliseconds: null, successCount: 0, failureCount: 2 },
      { collectedAt: '2026-09-11T02:10:00Z', latencyMilliseconds: 21, successCount: 3, failureCount: 0 },
    ], '2026-09-11T00:00:00Z', '2026-09-11T04:00:00Z', 4)

    expect(cells.map((cell) => cell.state)).toEqual(['partial', 'failure', 'success', 'missing'])
    expect(cells[0]).toMatchObject({ successCount: 1, failureCount: 1 })
  })

  it('infers legacy samples from latency when counters are absent', () => {
    const cells = monitoringStatusCells([
      { collectedAt: '2026-09-11T00:15:00Z', latencyMilliseconds: 14 },
      { collectedAt: '2026-09-11T01:15:00Z', latencyMilliseconds: null },
    ], '2026-09-11T00:00:00Z', '2026-09-11T02:00:00Z', 2)

    expect(cells.map((cell) => cell.state)).toEqual(['success', 'failure'])
  })

  it('keeps every row between the shared density bounds', () => {
    const series = (pointCount: number): MonitoringOperatorLatencySeries => ({
      id: String(pointCount), address: '127.0.0.1', points: Array.from({ length: pointCount }, (_, index) => ({
        collectedAt: new Date(index * 60_000).toISOString(), latencyMilliseconds: 1,
      })),
    })

    expect(monitoringStatusCellCount([series(3)])).toBe(12)
    expect(monitoringStatusCellCount([series(24)])).toBe(24)
    expect(monitoringStatusCellCount([series(80)])).toBe(36)
  })
})
