import type { ClusterHost } from '@/types/api'

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
