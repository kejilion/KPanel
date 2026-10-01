import { describe, expect, it } from 'vitest'
import { clusterHostFilesRoute, clusterHostMonitoringRoute, clusterHostTerminalRoute } from './clusterHostNavigation'

describe('cluster host tool routes', () => {
  const local = { id: 'local', isLocal: true }
  const remote = { id: 'host-1', isLocal: false }

  it('addresses a remote host by id and keeps local monitoring and files implicit', () => {
    expect(clusterHostMonitoringRoute(remote, 'cpu')).toEqual({ path: '/monitoring', query: { hostId: 'host-1', metric: 'cpu' } })
    expect(clusterHostMonitoringRoute(local, 'cpu')).toEqual({ path: '/monitoring', query: { metric: 'cpu' } })
    expect(clusterHostFilesRoute(remote)).toEqual({ path: '/files', query: { hostId: 'host-1' } })
    expect(clusterHostFilesRoute(local)).toEqual({ path: '/files', query: {} })
  })

  it('always names the terminal host so a reused window can switch to it', () => {
    expect(clusterHostTerminalRoute(remote)).toEqual({ path: '/terminal', query: { hostId: 'host-1' } })
    expect(clusterHostTerminalRoute(local)).toEqual({ path: '/terminal', query: { hostId: 'local' } })
  })
})
