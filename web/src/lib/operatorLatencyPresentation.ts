import type { MonitoringOperatorLatencySeries } from '@/types/api'

export const operatorLatencyColors: Record<string, string> = {
  'telecom-beijing': '#60a5fa',
  'telecom-shanghai': '#38bdf8',
  'telecom-guangzhou': '#818cf8',
  'unicom-beijing': '#34d399',
  'unicom-shanghai': '#2dd4bf',
  'unicom-guangzhou': '#22c55e',
  'mobile-beijing': '#fbbf24',
  'mobile-shanghai': '#fb923c',
  'mobile-guangzhou': '#f472b6',
}

export function mergeOperatorLatencyVisibility(
  current: Readonly<Record<string, boolean>>,
  series: MonitoringOperatorLatencySeries[],
): Record<string, boolean> {
  const next = { ...current }
  for (const item of series) {
    if (!(item.id in next)) next[item.id] = true
  }
  return next
}

// The latest point's typical latency: its median where recorded, otherwise
// the single value older rollups kept. null still means every probe failed.
export function latestOperatorLatency(series: MonitoringOperatorLatencySeries): number | null | undefined {
  const point = series.points.at(-1)
  if (!point || point.latencyMilliseconds === null) return point?.latencyMilliseconds
  return point.medianMilliseconds ?? point.latencyMilliseconds
}
