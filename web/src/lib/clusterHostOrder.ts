const maxClusterHostOrderLength = 101

export const clusterHostOrderStorageKey = 'kpanel:cluster-host-order'
export const clusterHostOrderChangedEvent = 'kpanel:cluster-host-order-changed'

interface HostWithID {
  id: string
}

interface StorageReader {
  getItem: (key: string) => string | null
}

function browserStorage(): StorageReader | undefined {
  if (typeof window === 'undefined') return undefined
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

export function readClusterHostOrder(storage = browserStorage()): string[] {
  if (!storage) return []
  try {
    const stored: unknown = JSON.parse(storage.getItem(clusterHostOrderStorageKey) || '[]')
    if (
      !Array.isArray(stored)
      || stored.length > maxClusterHostOrderLength
      || !stored.every((id) => typeof id === 'string' && id.length > 0 && id.length <= 128)
    ) {
      return []
    }
    return [...new Set(stored)]
  } catch {
    return []
  }
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
