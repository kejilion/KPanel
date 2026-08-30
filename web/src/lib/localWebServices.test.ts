import { describe, expect, it } from 'vitest'
import type { PortUsageEntry } from '@/types/api'
import {
  discoverLocalWebServiceCandidates,
  localWebServiceAddressLabel,
  localWebServiceOrigin,
} from './localWebServices'

function entry(overrides: Partial<PortUsageEntry> = {}): PortUsageEntry {
  return {
    protocol: 'tcp',
    state: 'LISTEN',
    localAddress: '127.0.0.1',
    localPort: '3000',
    peerAddress: '0.0.0.0',
    peerPort: '*',
    raw: 'tcp LISTEN fixture',
    ...overrides,
  }
}

describe('local web service candidates', () => {
  it('keeps TCP listeners, removes invalid records, and groups one port', () => {
    const candidates = discoverLocalWebServiceCandidates([
      entry({ process: 'node', pid: 3001 }),
      entry({ localAddress: '0.0.0.0', process: 'nginx', pid: 3002 }),
      entry({ protocol: 'udp', localPort: '53' }),
      entry({ state: 'ESTABLISHED', localPort: '4000' }),
      entry({ localPort: '0' }),
      entry({ localPort: '65536' }),
    ])

    expect(candidates).toEqual([{
      port: 3000,
      addresses: ['127.0.0.1', '0.0.0.0'],
      processes: ['nginx', 'node'],
      pids: [3001, 3002],
      containers: [],
    }])
    expect(localWebServiceOrigin(candidates[0]!)).toBe('http://127.0.0.1:3000')
  })

  it('formats IPv6 origins and listen ranges without an active probe', () => {
    const candidates = discoverLocalWebServiceCandidates([
      entry({ protocol: 'tcp6', localAddress: '::1', localPort: '8080' }),
    ])

    expect(localWebServiceOrigin(candidates[0]!)).toBe('http://[::1]:8080')
    expect(localWebServiceAddressLabel('::1')).toBe('仅本机')
    expect(localWebServiceAddressLabel('::')).toBe('所有 IPv6 地址')
  })

  it('carries published Docker ownership into the selectable port', () => {
    const candidates = discoverLocalWebServiceCandidates([
      entry({
        localPort: '9000',
        process: 'docker-proxy',
        pid: 1320,
        container: {
          id: 'container-1',
          name: 'kpanel-demo',
          image: 'demo-web:latest',
          containerPort: 8080,
        },
      }),
    ])

    expect(candidates[0]?.containers).toEqual([{
      id: 'container-1',
      name: 'kpanel-demo',
      image: 'demo-web:latest',
      containerPort: 8080,
    }])
  })
})
