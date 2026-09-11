// Synthetic, loopback-only preview data. Never imported by production code.
export function mockMonitoringHistory(url, remote = false) {
  const range = url.searchParams.get('range') || '6h'
  const durations = { '1h': 1, '6h': 6, '24h': 24, '7d': 168, '30d': 720, '3m': 2160, '6m': 4320, '12m': 8760 }
  const hostId = url.searchParams.get('hostId') || 'local'
  const end = Date.parse(url.searchParams.get('end') || '') || Date.now()
  const start = Date.parse(url.searchParams.get('start') || '') || end - (durations[range] || 6) * 3600000
  const interval = 60
  const bucket = Math.max(interval, Math.ceil((end - start) / 360000 / interval) * interval)
  const host = []
  const empty = hostId === 'd'.repeat(32)
  for (let at = start; at <= end && host.length < 720; at += bucket * 1000) {
    if (empty) continue
    if (hostId === 'c'.repeat(32) && (at > Date.now() - 90 * 60000 || (at > end - 4 * 3600000 && at < end - 3 * 3600000))) continue
    const phase = at / 600000
    const cpu = (remote ? 48 : 17) + 8 * Math.sin(phase) + 4 * Math.cos(phase * 2.3)
    host.push({ collectedAt: new Date(at).toISOString(), cpuPercent: cpu, cpuAveragePercent: cpu * 0.87, cpuSampleCount: 1, cpuCores: 4,
      loadOne: cpu / 28, loadFive: 1.2, loadFifteen: 0.9,
      memoryUsedBytes: (1.9 + 0.25 * Math.sin(phase / 7)) * 1024 ** 3, memoryTotalBytes: 4 * 1024 ** 3,
      swapUsedBytes: 0, swapTotalBytes: 0, diskUsedBytes: 16 * 1024 ** 3, diskTotalBytes: 40 * 1024 ** 3,
      diskPercent: 40 + 0.5 * Math.cos(phase / 10), diskIoAvailable: true,
      diskReadBytes: 4 * 1024 ** 3, diskWriteBytes: 2 * 1024 ** 3,
      diskReadBytesPerSecond: 450000 + 180000 * Math.sin(phase / 2), diskWriteBytesPerSecond: 120000 + 60000 * Math.cos(phase / 3),
      networkRxBytes: 8 * 1024 ** 3, networkTxBytes: 3 * 1024 ** 3,
      networkRxBytesPerSecond: 350000 + 120000 * Math.sin(phase / 3), networkTxBytesPerSecond: 170000 + 80000 * Math.cos(phase / 4),
      tcpConnections: Math.round(45 + 12 * Math.sin(phase)), udpConnections: 8 })
  }
  const sampled = host.filter((_point, index) => index % Math.max(1, Math.ceil(300 / bucket)) === 0)
  const containers = empty ? [] : ['nginx', 'postgres', 'redis'].map((name, index) => ({
    containerId: hostId.charCodeAt(0).toString(16).padStart(2, '0').repeat(31) + String(index + 1).padStart(2, '0'),
    name, image: `${name}:alpine`,
    points: sampled.map((point, i) => ({ collectedAt: point.collectedAt,
      cpuPercent: 8 + index * 14 + 6 * Math.sin(i / 5 + index), cpuAveragePercent: 6 + index * 12 + 4 * Math.sin(i / 5 + index), cpuSampleCount: 1,
      memoryBytes: (64 + index * 120 + 12 * Math.sin(i / 8)) * 1024 ** 2, memoryLimitBytes: 1024 ** 3, memoryPercent: 6 + index * 12,
      networkRxBytes: 1024 ** 3 + i * 60000, networkTxBytes: 2 * 1024 ** 3 + i * 40000,
      networkRxBytesPerSecond: 40000 + index * 10000 + 12000 * Math.sin(i / 4), networkTxBytesPerSecond: 24000 + index * 8000,
      blockReadBytes: 1024 ** 3 + i * 20000, blockWriteBytes: 1024 ** 3 + i * 10000,
      blockReadBytesPerSecond: 100000 + index * 70000 + 20000 * Math.cos(i / 6), blockWriteBytesPerSecond: 80000 + index * 40000, pids: 8 + index * 5,
    })),
  }))
  const operatorLatency = ['telecom', 'unicom', 'mobile'].flatMap((operator, i) =>
    ['beijing', 'shanghai', 'guangzhou'].map((region, j) => ({ id: `${operator}-${region}`, operator, region, address: `192.0.2.${i * 3 + j + 1}`,
      points: sampled.map((point, index) => ({ collectedAt: point.collectedAt,
        latencyMilliseconds: i === 2 && j === 1 && index % 23 === 0 ? null : 24 + i * 15 + j * 7 + 7 * Math.sin(index / 4 + j),
        successCount: i === 2 && j === 1 && index % 23 === 0 ? 0 : 1, failureCount: i === 2 && j === 1 && index % 23 === 0 ? 1 : 0,
      })),
    })))
  return { range, startedAt: new Date(start).toISOString(), endedAt: new Date(end).toISOString(), bucketSeconds: bucket,
    host, containers, operatorLatency, scannedBytes: 140000, skippedLines: 0, truncatedSeries: 0,
    storage: { enabled: true, retentionDays: 30, hostIntervalSeconds: interval,
      containerIntervalSeconds: 300, operatorLatencyIntervalSeconds: 300,
      maxContainers: 32, storageBytes: empty ? 0 : 238400, maxStorageBytes: 128 * 1024 ** 2,
      lastSampleAt: host.at(-1)?.collectedAt, lastContainerTotal: containers.length, lastContainerRecorded: containers.length, lastContainerFailed: 0,
      lastContainerTruncated: 0, lastDockerAvailable: !empty, operatorLatencyAvailable: true, lastOperatorLatencyAt: host.at(-1)?.collectedAt,
      lastOperatorLatencySuccessful: 9, lastOperatorLatencyFailed: 0, storageLimitReached: false,
      rollupRetentionDays: 365, rollupStorageBytes: empty ? 0 : 481200, maxRollupStorageBytes: 128 * 1024 ** 2,
      rollupStorageLimitReached: false } }
}
