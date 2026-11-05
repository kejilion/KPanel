import { describe, expect, it } from 'vitest'
import { fileHostLabel, fileHostStatus } from './fileHostStatus'
import type { ClusterHost } from '@/types/api'

it('labels local sources consistently and retains remote identity when inventory is unavailable', () => {
  const hosts = [{ id: 'local-id', name: 'Australia', isLocal: true }, { id: 'remote-id', name: 'Tokyo', isLocal: false }]
  expect(fileHostLabel('', hosts, '本机')).toBe('本机')
  expect(fileHostLabel('local-id', hosts, 'Local')).toBe('Local')
  expect(fileHostLabel('remote-id', hosts, '本机')).toBe('Tokyo')
  expect(fileHostLabel('remote-id', [], '本机')).toBe('remote-id')
})

describe('file transport availability', () => {
  const host = { id: 'remote', kind: 'panel', fileManagementAvailable: true } as ClusterHost
  it('lets an authorized file relay report its own result after a summary outage', () => {
    for (const state of ['online', 'degraded', 'stale', 'offline'] as const) {
      expect(fileHostStatus({ ...host, state }).action).toBe('select')
    }
  })
  it('preserves authentication, certificate, protocol and pairing restrictions', () => {
    for (const state of ['auth_failed', 'tls_error', 'incompatible', 'pairing', 'revoking'] as const) {
      expect(fileHostStatus({ ...host, state }).action).toBe('manage')
    }
    expect(fileHostStatus({ ...host, state: 'offline', fileManagementAvailable: false }).action).not.toBe('select')
  })
  it('does not present an offline summary as a verified file connection', () => {
    expect(fileHostStatus({ ...host, state: 'offline' })).toEqual({
      action: 'select', tone: 'warning', label: '监控离线 · 可尝试连接',
    })
  })
  it('continues to use the independent light file broker capability', () => {
    expect(fileHostStatus({ ...host, kind: 'light_node', state: 'offline' }).action).toBe('select')
    expect(fileHostStatus({ ...host, kind: 'light_node', state: 'online', fileManagementAvailable: false }).action).toBe('manage')
  })
})
