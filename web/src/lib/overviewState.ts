import type { SystemOverview } from '@/types/api'

// Keep the last explicitly observed state during background refresh. This
// prevents cards/forms from collapsing every polling interval; its old time
// and refreshing marker remain visible until this group's response arrives.
export function mergeOverviewUpdate(previous: SystemOverview, incoming: SystemOverview, preserveResources = false): SystemOverview {
  if (!previous.reads || !incoming.reads) return incoming
  const reads = { ...incoming.reads }
  let management = incoming.management
  for (const group of ['config', 'ssh-defense', 'bbrv3', 'capabilities'] as const) {
    if (incoming.reads[group]?.state !== 'loading' || previous.reads[group]?.state !== 'ready') continue
    reads[group] = { ...previous.reads[group], state: 'ready', refreshing: true }
    if (group === 'config') {
      management = { ...previous.management,
        ssh: { ...previous.management.ssh, defense: management.ssh.defense },
        swap: { ...previous.management.swap, totalBytes: management.swap.totalBytes, usedBytes: management.swap.usedBytes },
        bbrv3: management.bbrv3, capabilities: management.capabilities }
    } else if (group === 'ssh-defense') {
      management = { ...management, ssh: { ...management.ssh, defense: previous.management.ssh.defense } }
    } else if (group === 'bbrv3') {
      management = { ...management, bbrv3: previous.management.bbrv3 }
    } else {
      management = { ...management, capabilities: previous.management.capabilities }
    }
  }
  // Optional resources have no per-group completion marker. During a refresh,
  // replace them atomically at request completion, not with empty partials.
  // Runtime and independently observed management groups still update immediately.
  const resources = preserveResources ? {
    publicNetwork: previous.publicNetwork, services: previous.services,
    sites: previous.sites, containers: previous.containers, apps: previous.apps,
  } : {}
  return { ...incoming, ...resources, reads, management }
}

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
