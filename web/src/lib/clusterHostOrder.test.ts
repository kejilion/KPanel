import { describe, expect, it } from 'vitest'
import {
  applyClusterHostOrderPreference,
  cacheClusterHostOrder,
  clusterHostOrderStorageKey,
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
    expect(readClusterHostOrder({ getItem: () => '[" leading"]' })).toEqual([])
    expect(readClusterHostOrder({ getItem: () => '["line\\nbreak"]' })).toEqual([])
  })

  it('uses a configured panel preference as the browser cache authority', () => {
    let value = JSON.stringify(['local', 'remote'])
    const storage = {
      getItem: (key: string) => key === clusterHostOrderStorageKey ? value : null,
      setItem: (_key: string, next: string) => { value = next },
    }

    expect(applyClusterHostOrderPreference({
      ids: ['remote', 'local'],
      configured: true,
      resourceVersion: 'sha256:server-order',
    }, storage)).toEqual(['remote', 'local'])
    expect(JSON.parse(value)).toEqual(['remote', 'local'])
  })

  it('retains a legacy browser order only while the panel is unconfigured', () => {
    let writes = 0
    const storage = {
      getItem: () => JSON.stringify(['remote', 'local']),
      setItem: () => { writes += 1 },
    }

    expect(applyClusterHostOrderPreference({
      ids: [], configured: false, resourceVersion: 'sha256:unconfigured',
    }, storage)).toEqual([
      'remote',
      'local',
    ])
    expect(writes).toBe(0)
  })

  it('does not rewrite an unchanged cached server order', () => {
    let writes = 0
    const storage = {
      getItem: () => JSON.stringify(['remote', 'local']),
      setItem: () => { writes += 1 },
    }

    cacheClusterHostOrder(['remote', 'local'], storage)

    expect(writes).toBe(0)
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
