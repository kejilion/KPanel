import { describe, expect, it } from 'vitest'
import { sortClusterHostsTemporarily } from './clusterHostTemporarySort'
import type { ClusterHost } from '@/types/api'

function host(
  id: string,
  cpu: number,
  memory: number,
  disk: number,
  received: number,
  sent: number,
): ClusterHost {
  return {
    id,
    lastSnapshot: {
      telemetry: {
        cpu: { usagePercent: cpu },
        memory: { usagePercent: memory },
        disk: { usagePercent: disk },
        network: { receivedBytes: received, sentBytes: sent },
      },
    },
  } as ClusterHost
}

describe('temporary cluster host sorting', () => {
  const customOrder = [
    host('custom-first', 20, 80, 30, 100, 200),
    host('custom-second', 70, 10, 50, 900, 100),
    host('custom-third', 20, 40, 90, 400, 400),
  ]

  it.each([
    ['cpu', ['custom-second', 'custom-first', 'custom-third']],
    ['memory', ['custom-first', 'custom-third', 'custom-second']],
    ['disk', ['custom-third', 'custom-second', 'custom-first']],
    ['traffic', ['custom-second', 'custom-third', 'custom-first']],
  ] as const)('sorts %s descending and uses custom order for equal values', (key, expected) => {
    expect(sortClusterHostsTemporarily(customOrder, key, 'desc').map((item) => item.id))
      .toEqual(expected)
  })

  it('supports ascending order without mutating the custom order', () => {
    expect(sortClusterHostsTemporarily(customOrder, 'cpu', 'asc').map((item) => item.id))
      .toEqual(['custom-first', 'custom-third', 'custom-second'])
    expect(customOrder.map((item) => item.id))
      .toEqual(['custom-first', 'custom-second', 'custom-third'])
  })

  it('keeps missing and invalid metrics last in either direction', () => {
    const missing = { ...customOrder[0]!, id: 'missing', lastSnapshot: undefined }
    const invalid = host('invalid', Number.NaN, 0, 0, 0, 0)
    const values: ClusterHost[] = [missing, customOrder[1]!, invalid]

    expect(sortClusterHostsTemporarily(values, 'cpu', 'desc').map((item) => item.id))
      .toEqual(['custom-second', 'missing', 'invalid'])
    expect(sortClusterHostsTemporarily(values, 'cpu', 'asc').map((item) => item.id))
      .toEqual(['custom-second', 'missing', 'invalid'])
  })

  it('returns a separate array in the unchanged custom order', () => {
    const result = sortClusterHostsTemporarily(customOrder, 'custom', 'desc')

    expect(result).toEqual(customOrder)
    expect(result).not.toBe(customOrder)
  })
})
