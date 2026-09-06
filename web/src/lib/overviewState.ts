import type { SystemOverview } from '@/types/api'

// Structural defaults support rendering the static tool catalog. They are not
// observations: consumers must gate all status values and actions on reads.
export function pendingOverview(): SystemOverview {
  return {
    reads: Object.fromEntries(['runtime', 'config', 'ssh-defense', 'bbrv3', 'capabilities']
      .map((group) => [group, { state: 'loading' as const }])),
    hostname: '', os: '', osLike: [], uptimeSeconds: 0, observedAt: '',
    cpu: { value: 0, cores: 0 }, memory: { value: 0 }, disk: { value: 0 },
    load: { value: 0, one: 0, five: 0, fifteen: 0 },
    network: { receiveBytesPerSecond: 0, transmitBytesPerSecond: 0, rateAvailable: false,
      tcpConnections: 0, udpConnections: 0 },
    publicNetwork: {}, services: [],
    agent: { connected: false, compatible: false, readOnly: true },
    management: {
      ssh: { ports: [], source: 'unknown', defense: { available: false, installed: false,
        running: false, enabled: false, autostart: false, banned: 0 } },
      dns: { servers: [], manager: 'unknown' },
      swap: { totalBytes: 0, usedBytes: 0, activeDevices: 0, path: '/swapfile',
        fileExists: false, fileActive: false, fileSizeBytes: 0, fileUsedBytes: 0,
        legacyExists: false, legacyActive: false, legacySizeBytes: 0,
        otherActiveDevices: 0, otherSwapTotalBytes: 0, otherSwapUsedBytes: 0 },
      packageSources: [], maintenance: { state: 'idle', progress: 0, rebootRequired: false },
      ipPreference: 'unknown', kernelOptimization: { enabled: false },
      bbr: { supported: false, enabled: false, available: [] },
      bbrv3: { available: false, supported: false, installed: false, active: false, rebootRequired: false },
      capabilities: {},
    },
  }
}
