import { formatBytes } from '@/lib/format'

/**
 * The panel receives the same monotonic byte counters through two API shapes:
 * the host summary uses receivedBytes/sentBytes, while the desktop monitor's
 * normalized snapshot calls them totalReceivedBytes/totalTransmittedBytes.
 * Keep the mapping and directional formatting in one place so every view uses
 * the same binary units and rounding without inventing an aggregate metric.
 */
export interface NetworkTrafficCounters {
  receivedBytes?: number
  sentBytes?: number
  totalReceivedBytes?: number
  totalTransmittedBytes?: number
}

export type NetworkTrafficDirection = 'received' | 'sent'

function finiteCounter(value?: number): number | undefined {
  if (value === undefined || !Number.isFinite(value) || value < 0) return undefined
  return value
}

export function networkTrafficCounterBytes(
  value: NetworkTrafficCounters | undefined,
  direction: NetworkTrafficDirection,
): number | undefined {
  if (!value) return undefined
  if (direction === 'received') {
    return finiteCounter(value.receivedBytes) ?? finiteCounter(value.totalReceivedBytes)
  }
  return finiteCounter(value.sentBytes) ?? finiteCounter(value.totalTransmittedBytes)
}

export function formatNetworkTrafficCounter(
  value: NetworkTrafficCounters | undefined,
  direction: NetworkTrafficDirection,
): string {
  return formatBytes(networkTrafficCounterBytes(value, direction))
}
