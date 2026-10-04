import { describe, expect, it, vi } from 'vitest'
import type { MonitoringContainerSeries } from '@/types/api'
import {
  bandCeiling,
  isHistoricalContainer,
  monitoringRangeFromQuery,
  monitoringWindowFromQuery,
  nearestTimestamp,
  newestContainerSampleTime,
  normalizeTrendChartWidth,
  parseMonitoringTimestamp,
  sliceMonitoringHistory,
  svgClientXToViewBox,
  svgViewBoxXToClient,
  trendSelectionFromViewBox,
  trendLegendLabel,
  uniqueSeriesTimes,
} from './monitoringPresentation'

function container(id: string, collectedAt?: string): MonitoringContainerSeries {
  return {
    containerId: id,
    name: id,
    image: 'image:latest',
    points: collectedAt ? [{
      collectedAt,
      cpuPercent: 0,
      memoryBytes: 0,
      memoryLimitBytes: 0,
      memoryPercent: 0,
      networkRxBytes: 0,
      networkTxBytes: 0,
      networkRxBytesPerSecond: 0,
      networkTxBytesPerSecond: 0,
      blockReadBytes: 0,
      blockWriteBytes: 0,
      pids: 0,
    }] : [],
  }
}

describe('monitoring presentation', () => {
  it('normalizes supported monitoring route state', () => {
    expect(monitoringRangeFromQuery('12m')).toBe('12m')
    expect(monitoringRangeFromQuery(['12m'])).toBe('6h')
    expect(monitoringRangeFromQuery('invalid')).toBe('6h')
    expect(monitoringWindowFromQuery(
      '2026-08-04T00:00:00+08:00',
      '2026-08-05T00:00:00+08:00',
    )).toEqual({
      start: '2026-08-03T16:00:00.000Z',
      end: '2026-08-04T16:00:00.000Z',
    })
  })

  it('rejects partial, repeated, invalid and reversed monitoring route windows', () => {
    expect(monitoringWindowFromQuery('2026-08-04T00:00:00Z', undefined)).toBeUndefined()
    expect(monitoringWindowFromQuery(['2026-08-04T00:00:00Z'], '2026-08-05T00:00:00Z')).toBeUndefined()
    expect(monitoringWindowFromQuery('invalid', '2026-08-05T00:00:00Z')).toBeUndefined()
    expect(monitoringWindowFromQuery('2026-08-05T00:00:00Z', '2026-08-04T00:00:00Z')).toBeUndefined()
  })

  it('fills the rendered chart width and keeps a safe fallback', () => {
    expect(normalizeTrendChartWidth(1440)).toBe(1440)
    expect(normalizeTrendChartWidth(0)).toBe(720)
    expect(normalizeTrendChartWidth(180)).toBe(320)
  })

  it('shows the latest probe state instead of a stale historical value', () => {
    const formatter = (value: number) => `${value} ms`
    expect(trendLegendLabel('超时', 274, formatter)).toBe('超时')
    expect(trendLegendLabel(undefined, 274, formatter)).toBe('274 ms')
  })

  it('uses every series timestamp and reaches both edges', () => {
    const times = uniqueSeriesTimes([
      { points: [{ time: 20 }, { time: 30 }] },
      { points: [{ time: 10 }, { time: 30 }] },
    ])
    expect(times).toEqual([10, 20, 30])
    expect(nearestTimestamp(times, -100)).toBe(10)
    expect(nearestTimestamp(times, 100)).toBe(30)
    expect(nearestTimestamp(times, 24)).toBe(20)
  })

  it('parses repeated chart timestamps once across charts', () => {
    const parse = vi.spyOn(Date, 'parse')
    const timestamp = '2026-08-05T14:00:00.123Z'

    expect(parseMonitoringTimestamp(timestamp)).toBe(Date.UTC(2026, 7, 5, 14, 0, 0, 123))
    expect(parseMonitoringTimestamp(timestamp)).toBe(Date.UTC(2026, 7, 5, 14, 0, 0, 123))
    expect(parse).toHaveBeenCalledTimes(1)
    parse.mockRestore()
  })

  it('uses the SVG screen matrix before the bounding box fallback', () => {
    const matrixTransform = vi.fn(() => ({ x: 64 }))
    const svg = {
      getScreenCTM: () => ({ inverse: () => ({}) }),
      createSVGPoint: () => ({ x: 0, y: 0, matrixTransform }),
      getBoundingClientRect: () => ({ left: 100, width: 900 }),
    } as unknown as SVGSVGElement

    expect(svgClientXToViewBox(svg, 180, 40, 720)).toBe(64)
    expect(matrixTransform).toHaveBeenCalledOnce()
  })

  it('falls back to linear box mapping when no SVG matrix is available', () => {
    const svg = {
      getScreenCTM: () => null,
      getBoundingClientRect: () => ({ left: 100, width: 900 }),
    } as unknown as SVGSVGElement
    expect(svgClientXToViewBox(svg, 550, 0, 720)).toBe(360)
  })

  it('maps a view-box x coordinate through the rendered SVG matrix', () => {
    const svg = {
      getScreenCTM: () => ({ a: 0.75, e: 120 }),
      getBoundingClientRect: () => ({ left: 100, width: 900 }),
    } as unknown as SVGSVGElement

    expect(svgViewBoxXToClient(svg, 400, 720)).toBe(420)
  })

  it('falls back to the rendered SVG box when mapping a tooltip anchor', () => {
    const svg = {
      getScreenCTM: () => null,
      getBoundingClientRect: () => ({ left: 100, width: 900 }),
    } as unknown as SVGSVGElement

    expect(svgViewBoxXToClient(svg, 360, 720)).toBe(550)
  })

  it('normalizes and clamps a reverse drag selection', () => {
    expect(trendSelectionFromViewBox(720, 40, 64, 708, 1_000, 2_000)).toEqual({
      start: 1_000,
      end: 2_000,
    })
    expect(trendSelectionFromViewBox(500, 200, 100, 700, 0, 600)).toEqual({
      start: 100,
      end: 400,
    })
  })

  it('ignores accidental and invalid drag selections', () => {
    expect(trendSelectionFromViewBox(100, 108, 64, 708, 1_000, 2_000)).toBeUndefined()
    expect(trendSelectionFromViewBox(100, 200, 64, 64, 1_000, 2_000)).toBeUndefined()
  })

  it('marks only containers older than the newest sample as historical', () => {
    const current = container('current', '2026-07-31T12:10:00Z')
    const old = container('old', '2026-07-31T12:05:00Z')
    const empty = container('empty')
    const newest = newestContainerSampleTime([old, empty, current])

    expect(newest).toBe(Date.parse('2026-07-31T12:10:00Z'))
    expect(isHistoricalContainer(current, newest)).toBe(false)
    expect(isHistoricalContainer(old, newest)).toBe(true)
    expect(isHistoricalContainer(empty, newest)).toBe(true)
  })

  it('creates an immediate bounded preview without mutating the source history', () => {
    const source = {
      range: '24h' as const,
      startedAt: '2026-08-05T00:00:00Z',
      endedAt: '2026-08-05T02:00:00Z',
      bucketSeconds: 60,
      host: [
        { collectedAt: '2026-08-05T00:00:00Z' },
        { collectedAt: '2026-08-05T01:00:00Z' },
        { collectedAt: '2026-08-05T02:00:00Z' },
      ],
      containers: [], operatorLatency: [], storage: {}, scannedBytes: 0, skippedLines: 0, truncatedSeries: 0,
    } as unknown as import('@/types/api').MonitoringHistory
    const preview = sliceMonitoringHistory(source, '2026-08-05T00:30:00Z', '2026-08-05T01:30:00Z')
    expect(preview.host.map((point) => point.collectedAt)).toEqual(['2026-08-05T01:00:00Z'])
    expect(source.host).toHaveLength(3)
  })
})

describe('band ceiling', () => {
  it('keeps room for usual peaks but not for a rare spike', () => {
    const highs = Array.from({ length: 19 }, (_, index) => 20 + index)
    expect(bandCeiling([...highs, 2000])).toBe(37)
    expect(bandCeiling([5, 900])).toBe(900)
    expect(bandCeiling([42])).toBe(42)
    expect(bandCeiling([Number.NaN, 12, Number.POSITIVE_INFINITY])).toBe(12)
    expect(bandCeiling([])).toBe(Number.NEGATIVE_INFINITY)
  })
})
