import type { ClusterHost } from '@/types/api'
import type { MonitoringMetric } from './monitoringNavigation'

export function clusterHostMonitoringRoute(host: Pick<ClusterHost, 'id' | 'isLocal'>, metric: MonitoringMetric) {
  return { path: '/monitoring', query: host.isLocal ? { metric } : { hostId: host.id, metric } }
}

/** Terminal resolves every host, including the local one, by id. */
export function clusterHostTerminalRoute(host: Pick<ClusterHost, 'id'>) {
  return { path: '/terminal', query: { hostId: host.id } }
}

/** Files treat an omitted `hostId` as the local host. */
export function clusterHostFilesRoute(host: Pick<ClusterHost, 'id' | 'isLocal'>) {
  return { path: '/files', query: host.isLocal ? {} : { hostId: host.id } }
}

export const clusterSecurityEntrancePathPattern = /^[a-z0-9](?:[a-z0-9-]{4,46}[a-z0-9])$/

function displayOrigin(host: ClusterHost): string {
  if (host.kind === 'light_node') return ''
  if (host.isLocal) return typeof window === 'undefined' ? '' : window.location.origin
  return host.origin
}

/** Build the same trusted Panel entry used by the cluster management page. */
export function clusterHostPanelURL(host: ClusterHost): string {
  const origin = displayOrigin(host)
  if (
    !origin
    || host.isLocal
    || !host.securityEntrancePath
    || !clusterSecurityEntrancePathPattern.test(host.securityEntrancePath)
  ) {
    return origin
  }
  return `${origin}/${host.securityEntrancePath}`
}
