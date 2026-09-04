import { describe, expect, it } from 'vitest'
import {
  readClusterHostOrder,
  reconcileClusterHostOrder,
  sortClusterHosts,
} from './clusterHostOrder'

function host(id: string): { id: string; name: string } {
  return { id, name: id }
}

describe('cluster host ordering', () => {
  it('reads a bounded, unique order from the shared storage key', () => {
    const storage = {
      getItem: () => '["remote-2", "local", "remote-2"]',
    }

    expect(readClusterHostOrder(storage)).toEqual(['remote-2', 'local'])
  })

  it('rejects malformed or oversized persisted orders', () => {
    expect(readClusterHostOrder({ getItem: () => '{broken' })).toEqual([])
    expect(readClusterHostOrder({ getItem: () => JSON.stringify(Array.from({ length: 102 }, (_, index) => `host-${index}`)) })).toEqual([])
    expect(readClusterHostOrder({ getItem: () => '["valid", 42]' })).toEqual([])
  })

  it('keeps the cluster page order and appends hosts missing from it', () => {
    const items = [host('local'), host('remote-1'), host('remote-2')]

    expect(sortClusterHosts(items, ['remote-2', 'local'])).toEqual([
      items[2],
      items[0],
      items[1],
    ])
    expect(reconcileClusterHostOrder(items, ['stale', 'remote-2'])).toEqual([
      'remote-2',
      'local',
      'remote-1',
    ])
  })
})
