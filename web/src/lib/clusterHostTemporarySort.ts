import { parseClusterHostPrice } from '@/lib/clusterHostPrice'
import { clusterTrafficCounters, networkTrafficCounterBytes, type NetworkTrafficCounters } from '@/lib/networkTraffic'
import type { ClusterHost, ClusterHostDetails, PublicClusterShareHost } from '@/types/api'

export type ClusterHostDetailsSortKey = 'custom' | 'expiresOn' | 'price'
export type ClusterHostTemporarySortKey = ClusterHostDetailsSortKey | 'cpu' | 'memory' | 'disk' | 'traffic'
export type ClusterHostTemporarySortDirection = 'asc' | 'desc'

interface SortMetric {
  value: number
  group?: string
  fraction?: { numerator: bigint; denominator: bigint }
}

function priceMetric(price: string | undefined): SortMetric | undefined {
  const parsed = parseClusterHostPrice(price)
  if (!parsed) return undefined
  return { value: parsed.monthlyAmount, group: `${parsed.currency}:${parsed.cycleMonths ? 'monthly' : 'unspecified'}`, fraction: parsed.monthlyFraction }
}

function sortByMetric<T>(items: readonly T[], direction: ClusterHostTemporarySortDirection, metric: (host: T) => SortMetric | undefined): T[] {
  return items.map((host, customIndex) => ({ host, customIndex, metric: metric(host) }))
    .sort((left, right) => {
      if (!left.metric && !right.metric) return left.customIndex - right.customIndex
      if (!left.metric) return 1
      if (!right.metric) return -1
      const leftGroup = left.metric.group || ''
      const rightGroup = right.metric.group || ''
      if (leftGroup !== rightGroup) return leftGroup < rightGroup ? -1 : 1
      let order = left.metric.value - right.metric.value
      if (left.metric.fraction && right.metric.fraction) {
        // Preserve equal decimal prices across billing cycles without floating-point drift.
        const difference = left.metric.fraction.numerator * right.metric.fraction.denominator
          - right.metric.fraction.numerator * left.metric.fraction.denominator
        order = difference < 0n ? -1 : Number(difference > 0n)
      }
      return (direction === 'desc' ? -order : order) || left.customIndex - right.customIndex
    }).map(({ host }) => host)
}

export function sortClusterHostsByDetails<T>(
  items: readonly T[], key: ClusterHostDetailsSortKey, direction: ClusterHostTemporarySortDirection,
  details: (host: T) => ClusterHostDetails | undefined,
): T[] {
  if (key === 'custom') return [...items]
  return sortByMetric(items, direction, host => {
    const value = details(host)
    if (key === 'price') return priceMetric(value?.price)
    const date = value?.expiresOn
    if (!date || !/^\d{4}-\d{2}-\d{2}$/.test(date)) return undefined
    const timestamp = Date.parse(`${date}T00:00:00Z`)
    return Number.isFinite(timestamp) ? { value: timestamp } : undefined
  })
}

function normalizedMetric(value: number | undefined): number | undefined {
  if (value === undefined || !Number.isFinite(value)) return undefined
  return Math.max(0, value)
}

function telemetryMetric(
  telemetry: {
    cpu: { usagePercent: number }
    memory: { usagePercent: number }
    disk: { usagePercent: number }
    network: NetworkTrafficCounters
  } | undefined,
  key: ClusterHostTemporarySortKey,
  traffic?: NetworkTrafficCounters,
): number | undefined {
  if (!telemetry || key === 'custom') return undefined
  if (key === 'cpu') return normalizedMetric(telemetry.cpu.usagePercent)
  if (key === 'memory') return normalizedMetric(telemetry.memory.usagePercent)
  if (key === 'disk') return normalizedMetric(telemetry.disk.usagePercent)

  const received = networkTrafficCounterBytes(traffic, 'received')
  const sent = networkTrafficCounterBytes(traffic, 'sent')
  if (received === undefined || sent === undefined) return undefined
  return normalizedMetric(received + sent)
}

export function sortClusterHostsTemporarily(
  items: readonly ClusterHost[],
  key: ClusterHostTemporarySortKey,
  direction: ClusterHostTemporarySortDirection,
  details: Readonly<Record<string, ClusterHostDetails>> = {},
): ClusterHost[] {
  if (key === 'custom') return [...items]
  if (key === 'expiresOn' || key === 'price') return sortClusterHostsByDetails(items, key, direction, host => details[host.id])
  return sortByMetric(items, direction, host => {
    const value = telemetryMetric(host.lastSnapshot?.telemetry, key, clusterTrafficCounters(host))
    return value === undefined ? undefined : { value }
  })
}

export function sortPublicClusterHostsTemporarily(
  items: readonly PublicClusterShareHost[],
  key: ClusterHostTemporarySortKey,
  direction: ClusterHostTemporarySortDirection,
): PublicClusterShareHost[] {
  if (key === 'custom' || key === 'expiresOn' || key === 'price') {
    return sortClusterHostsByDetails(items, key, direction, host => host)
  }
  return sortByMetric(items, direction, host => {
    const value = telemetryMetric(host.collectedAt ? host : undefined, key, clusterTrafficCounters(host))
    return value === undefined ? undefined : { value }
  })
}
