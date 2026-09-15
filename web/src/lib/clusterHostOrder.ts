import type { ClusterHostOrderPreference } from '@/types/api'

const maxClusterHostOrderLength = 101
const maxClusterHostIDBytes = 128

export const clusterHostOrderStorageKey = 'kpanel:cluster-host-order'
export const clusterHostOrderChangedEvent = 'kpanel:cluster-host-order-changed'

interface HostWithID {
  id: string
}

interface StorageReader {
  getItem: (key: string) => string | null
}

interface StorageWriter extends StorageReader {
  setItem: (key: string, value: string) => void
}

function browserStorage(): StorageWriter | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

function normalizeClusterHostOrder(value: unknown): string[] | undefined {
  if (!Array.isArray(value) || value.length > maxClusterHostOrderLength) return undefined
  const order = [...new Set(value)]
  if (!order.every((id) => {
    if (typeof id !== 'string' || id.length === 0 || id !== id.trim()) return false
    if (new TextEncoder().encode(id).byteLength > maxClusterHostIDBytes) return false
    return !Array.from(id).some((character) => {
      const codePoint = character.codePointAt(0) || 0
      return codePoint <= 0x1f || (codePoint >= 0x7f && codePoint <= 0x9f)
    })
  })) return undefined
  return order as string[]
}

export function readClusterHostOrder(
  storage: StorageReader | undefined = browserStorage(),
): string[] {
  if (!storage) return []
  try {
    const stored: unknown = JSON.parse(storage.getItem(clusterHostOrderStorageKey) || '[]')
    return normalizeClusterHostOrder(stored) || []
  } catch {
    return []
  }
}

export function cacheClusterHostOrder(
  order: readonly string[],
  storage: StorageWriter | undefined = browserStorage(),
): void {
  const normalized = normalizeClusterHostOrder(order)
  if (!normalized) return
  const current = readClusterHostOrder(storage)
  if (current.length === normalized.length && current.every((id, index) => id === normalized[index])) return
  try {
    storage?.setItem(clusterHostOrderStorageKey, JSON.stringify(normalized))
  } catch {
    // The current view still uses the server value when browser storage is unavailable.
  }
  notifyClusterHostOrderChanged()
}

export function applyClusterHostOrderPreference(
  preference: ClusterHostOrderPreference | undefined,
  storage: StorageWriter | undefined = browserStorage(),
): string[] {
  if (!preference?.configured) return readClusterHostOrder(storage)
  const order = normalizeClusterHostOrder(preference.ids)
  if (!order) return readClusterHostOrder(storage)
  cacheClusterHostOrder(order, storage)
  return order
}

export function sortClusterHosts<T extends HostWithID>(
  items: readonly T[],
  order: readonly string[],
): T[] {
  const positions = new Map(order.map((id, index) => [id, index]))
  return items
    .map((host, originalIndex) => ({ host, originalIndex }))
    .sort((left, right) => {
      const leftPosition = positions.get(left.host.id)
      const rightPosition = positions.get(right.host.id)
      if (leftPosition === undefined && rightPosition === undefined) {
        return left.originalIndex - right.originalIndex
      }
      if (leftPosition === undefined) return 1
      if (rightPosition === undefined) return -1
      return leftPosition - rightPosition
    })
    .map(({ host }) => host)
}

export function reconcileClusterHostOrder<T extends HostWithID>(
  items: readonly T[],
  order: readonly string[],
): string[] {
  const validIDs = new Set(items.map((host) => host.id))
  const next = order.filter((id) => validIDs.has(id))
  for (const host of items) {
    if (!next.includes(host.id)) next.push(host.id)
  }
  return next
}

export function notifyClusterHostOrderChanged(): void {
  if (
    typeof window === 'undefined'
    || typeof window.dispatchEvent !== 'function'
    || typeof Event === 'undefined'
  ) return
  window.dispatchEvent(new Event(clusterHostOrderChangedEvent))
}

export function subscribeClusterHostOrder(onChange: () => void): () => void {
  if (typeof window === 'undefined') return () => undefined
  const handleStorage = (event: StorageEvent) => {
    if (event.key === clusterHostOrderStorageKey) onChange()
  }
  window.addEventListener(clusterHostOrderChangedEvent, onChange)
  window.addEventListener('storage', handleStorage)
  return () => {
    window.removeEventListener(clusterHostOrderChangedEvent, onChange)
    window.removeEventListener('storage', handleStorage)
  }
}
