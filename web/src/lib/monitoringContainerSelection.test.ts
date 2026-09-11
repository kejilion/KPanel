import { describe, expect, it, vi } from 'vitest'
import type { MonitoringContainerSeries } from '@/types/api'
import {
  assignMonitoringContainerColorSlots,
  defaultMonitoringContainerIDs,
  monitoringContainerSelectionLimit,
  readMonitoringContainerPreference,
	readMonitoringHostContainerPreference,
  reconcileMonitoringContainerIDs,
  writeMonitoringContainerPreference,
	writeMonitoringHostContainerPreference,
} from './monitoringContainerSelection'

function container(id: string, collectedAt?: string): MonitoringContainerSeries {
  return {
    containerId: id,
    name: id,
    image: `${id}:latest`,
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

describe('monitoring container selection', () => {
  it('isolates host preferences, preserves local choices and bounds retained hosts', () => {
    const values = new Map<string, string>()
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) } }
    writeMonitoringHostContainerPreference('local', { ids: ['local-container'], slots: {} }, storage)
    for (let i = 1; i <= 101; i++) {
      writeMonitoringHostContainerPreference(i.toString(16).padStart(32, '0'), { ids: [`container-${i}`], slots: {} }, storage)
    }
    expect(readMonitoringHostContainerPreference('local', storage)?.ids).toEqual(['local-container'])
    expect(readMonitoringHostContainerPreference('1'.padStart(32, '0'), storage)).toBeUndefined()
    expect(readMonitoringHostContainerPreference('2'.padStart(32, '0'), storage)?.ids).toEqual(['container-2'])
    expect(Object.keys(JSON.parse(values.get('kejilion-panel-monitoring-host-containers-v1')!))).toHaveLength(100)
    writeMonitoringHostContainerPreference('__proto__', { ids: ['bad'], slots: {} }, storage)
    expect(values.size).toBe(2)
    expect(readMonitoringHostContainerPreference('2'.padStart(32, '0'), { getItem: () => ' '.repeat(128 * 1024 + 1) })).toBeUndefined()
  })

  it('defaults to three current containers and excludes historical rows', () => {
    const containers = [
      container('current-a', '2026-08-05T00:00:00Z'),
      container('current-b', '2026-08-05T00:00:00Z'),
      container('current-c', '2026-08-05T00:00:00Z'),
      container('current-d', '2026-08-05T00:00:00Z'),
      container('historical', '2026-08-04T00:00:00Z'),
    ]
    expect(defaultMonitoringContainerIDs(containers)).toEqual(['current-a', 'current-b', 'current-c'])
    expect(defaultMonitoringContainerIDs([containers[4]!])).toEqual(['historical'])
  })

  it('retains available preferences, caps selection and falls back when all disappeared', () => {
    const containers = Array.from({ length: 7 }, (_, index) =>
      container(`container-${index}`, '2026-08-05T00:00:00Z'))
    expect(reconcileMonitoringContainerIDs(containers, [
      'container-5', 'missing', 'container-1', 'container-5', 'container-4', 'container-3', 'container-2', 'container-0',
    ])).toEqual(['container-5', 'container-1', 'container-4', 'container-3', 'container-2'])
    expect(reconcileMonitoringContainerIDs(containers, ['missing'])).toEqual([
      'container-0', 'container-1', 'container-2',
    ])
  })

  it('keeps surviving color slots stable and fills only unused slots', () => {
    const first = assignMonitoringContainerColorSlots(['a', 'b', 'c'])
    expect(first).toEqual({ a: 0, b: 1, c: 2 })
    expect(assignMonitoringContainerColorSlots(['b', 'c', 'd'], first)).toEqual({ b: 1, c: 2, d: 0 })
    expect(Object.keys(assignMonitoringContainerColorSlots(
      Array.from({ length: monitoringContainerSelectionLimit + 2 }, (_, index) => String(index)),
    ))).toHaveLength(monitoringContainerSelectionLimit)
  })

  it('reads only bounded identifiers and repairs conflicting color slots', () => {
    const preference = readMonitoringContainerPreference({
      getItem: () => JSON.stringify({
        ids: ['a', 'a', '', 42, 'x'.repeat(65), 'b', 'c', 'd', 'e', 'f'],
        slots: { a: 2, b: 2, c: 99, d: 1 },
      }),
    })
    expect(preference).toEqual({
      ids: ['a', 'b', 'c', 'd', 'e'],
      slots: { a: 2, b: 0, c: 3, d: 1, e: 4 },
    })
    expect(readMonitoringContainerPreference({ getItem: () => '{invalid' })).toBeUndefined()
  })

  it('persists a bounded preference and tolerates unavailable storage', () => {
    const setItem = vi.fn()
    writeMonitoringContainerPreference({ ids: ['a', 'b'], slots: { a: 4, b: 4 } }, { setItem })
    expect(setItem).toHaveBeenCalledWith(
      'kejilion-panel-monitoring-containers-v1',
      JSON.stringify({ ids: ['a', 'b'], slots: { a: 4, b: 0 } }),
    )
    expect(() => writeMonitoringContainerPreference(
      { ids: ['a'], slots: { a: 0 } },
      { setItem: () => { throw new Error('blocked') } },
    )).not.toThrow()
  })
})
