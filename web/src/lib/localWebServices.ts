import type { PortUsageContainer, PortUsageEntry } from '@/types/api'

export const LOCAL_WEB_SERVICE_MAX_CANDIDATES = 128

export interface LocalWebServiceCandidate {
  port: number
  addresses: string[]
  processes: string[]
  pids: number[]
  containers: PortUsageContainer[]
}

interface CandidateAccumulator {
  port: number
  addresses: Set<string>
  processes: Set<string>
  pids: Set<number>
  containers: Map<string, PortUsageContainer>
}

const IPV4_WILDCARDS = new Set(['0.0.0.0', '*'])
const IPV6_WILDCARDS = new Set(['::', '[::]'])
const LOOPBACK_ADDRESSES = new Set(['127.0.0.1', '::1'])

function parsePort(value: string): number | undefined {
  const normalized = value.trim()
  if (!/^\d+$/.test(normalized)) return undefined
  const port = Number(normalized)
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : undefined
}

function isIPv4Address(value: string): boolean {
  const parts = value.split('.')
  return parts.length === 4 && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255)
}

function isIPv6Address(value: string): boolean {
  if (!value.includes(':')) return false
  try {
    return new URL(`http://[${value}]:1`).hostname !== ''
  } catch {
    return false
  }
}

function normalizeAddress(value: string): string {
  const address = value.trim().replace(/^\[(.*)\]$/, '$1')
  if (IPV4_WILDCARDS.has(address) || IPV6_WILDCARDS.has(address)) return address
  if (isIPv4Address(address) || isIPv6Address(address)) return address
  return ''
}

function processName(value?: string): string {
  const source = value?.trim() || ''
  if (!source) return ''
  const name = /users:\(\("([^"]+)"/.exec(source)?.[1] || source
  return name.slice(0, 128)
}

function containerKey(container: PortUsageContainer): string {
  return container.id.trim() || `name:${container.name.trim()}`
}

function addressRank(address: string): number {
  if (LOOPBACK_ADDRESSES.has(address)) return 0
  if (!IPV4_WILDCARDS.has(address) && !IPV6_WILDCARDS.has(address)) return 1
  if (IPV4_WILDCARDS.has(address)) return 2
  return 3
}

function compareAddresses(left: string, right: string): number {
  return addressRank(left) - addressRank(right) || left.localeCompare(right)
}

/**
 * Turns the port-usage socket snapshot into bounded, selectable TCP candidates.
 * It intentionally does not claim that a port is HTTP-ready: no network probe is
 * performed, so users can still correct the generated origin manually.
 */
export function discoverLocalWebServiceCandidates(
  entries: readonly PortUsageEntry[],
): LocalWebServiceCandidate[] {
  const groups = new Map<number, CandidateAccumulator>()

  for (const entry of entries) {
    const protocol = entry.protocol.trim().toLowerCase()
    if (protocol !== 'tcp' && protocol !== 'tcp4' && protocol !== 'tcp6') continue
    if (entry.state.trim().toUpperCase() !== 'LISTEN') continue

    const port = parsePort(entry.localPort)
    if (port === undefined) continue

    const group = groups.get(port) || {
      port,
      addresses: new Set<string>(),
      processes: new Set<string>(),
      pids: new Set<number>(),
      containers: new Map<string, PortUsageContainer>(),
    }
    const address = normalizeAddress(entry.localAddress)
    if (address) group.addresses.add(address)
    const name = processName(entry.process)
    if (name) group.processes.add(name)
    const pid = Number(entry.pid)
    if (Number.isInteger(pid) && pid > 0) group.pids.add(pid)
    if (entry.container?.name?.trim()) {
      group.containers.set(containerKey(entry.container), entry.container)
    }
    groups.set(port, group)
  }

  return [...groups.values()]
    .sort((left, right) => left.port - right.port)
    .slice(0, LOCAL_WEB_SERVICE_MAX_CANDIDATES)
    .map((group) => ({
      port: group.port,
      addresses: [...group.addresses].sort(compareAddresses),
      processes: [...group.processes].sort((left, right) => left.localeCompare(right)),
      pids: [...group.pids].sort((left, right) => left - right),
      containers: [...group.containers.values()].sort((left, right) =>
        left.name.localeCompare(right.name) || left.id.localeCompare(right.id),
      ),
    }))
}

function preferredHost(addresses: readonly string[]): string {
  const loopback = addresses.find((address) => LOOPBACK_ADDRESSES.has(address))
  if (loopback) return loopback

  const specific = addresses.find(
    (address) => !IPV4_WILDCARDS.has(address) && !IPV6_WILDCARDS.has(address),
  )
  if (specific) return specific
  if (addresses.some((address) => IPV4_WILDCARDS.has(address))) return '127.0.0.1'
  if (addresses.some((address) => IPV6_WILDCARDS.has(address))) return '::1'
  return '127.0.0.1'
}

export function localWebServiceOrigin(candidate: LocalWebServiceCandidate): string {
  const host = preferredHost(candidate.addresses)
  const formattedHost = host.includes(':') ? `[${host}]` : host
  return `http://${formattedHost}:${candidate.port}`
}

export function localWebServiceAddressLabel(address: string): string {
  if (IPV4_WILDCARDS.has(address)) return '所有 IPv4 地址'
  if (IPV6_WILDCARDS.has(address)) return '所有 IPv6 地址'
  if (LOOPBACK_ADDRESSES.has(address)) return '仅本机'
  return address
}
