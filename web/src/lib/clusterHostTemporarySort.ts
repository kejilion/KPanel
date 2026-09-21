import { networkTrafficCounterBytes } from '@/lib/networkTraffic'
import type { ClusterHost } from '@/types/api'

export type ClusterHostTemporarySortKey = 'custom' | 'cpu' | 'memory' | 'disk' | 'traffic'
export type ClusterHostTemporarySortDirection = 'asc' | 'desc'

function normalizedMetric(value: number | undefined): number | undefined {
  if (value === undefined || !Number.isFinite(value)) return undefined
  return Math.max(0, value)
}

function hostMetric(host: ClusterHost, key: ClusterHostTemporarySortKey): number | undefined {
  const telemetry = host.lastSnapshot?.telemetry
  if (!telemetry || key === 'custom') return undefined
  if (key === 'cpu') return normalizedMetric(telemetry.cpu.usagePercent)
  if (key === 'memory') return normalizedMetric(telemetry.memory.usagePercent)
  if (key === 'disk') return normalizedMetric(telemetry.disk.usagePercent)

  const received = networkTrafficCounterBytes(telemetry.network, 'received')
  const sent = networkTrafficCounterBytes(telemetry.network, 'sent')
  if (received === undefined || sent === undefined) return undefined
  return normalizedMetric(received + sent)
}

export function sortClusterHostsTemporarily(
  items: readonly ClusterHost[],
  key: ClusterHostTemporarySortKey,
  direction: ClusterHostTemporarySortDirection,
): ClusterHost[] {
  if (key === 'custom') return [...items]
  return items
    .map((host, customIndex) => ({ host, customIndex, metric: hostMetric(host, key) }))
    .sort((left, right) => {
      if (left.metric === undefined && right.metric === undefined) {
        return left.customIndex - right.customIndex
      }
      if (left.metric === undefined) return 1
      if (right.metric === undefined) return -1
      const metricOrder = direction === 'desc'
        ? right.metric - left.metric
        : left.metric - right.metric
      return metricOrder || left.customIndex - right.customIndex
    })
    .map(({ host }) => host)
}
