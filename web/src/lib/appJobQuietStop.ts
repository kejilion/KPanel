import { api, ApiError } from '@/lib/api'

const pendingKey = 'kpanel:pending-app-job-stops'
const active = new Set<string>()
const scanning = new Set<string>()

function readPending(owner: string): string[] {
  if (!owner) return []
  try {
    const prefix = `${pendingKey}:${encodeURIComponent(owner)}:`
    const ids = new Set<string>()
    try {
      const legacy: unknown = JSON.parse(window.localStorage.getItem(`${pendingKey}:${encodeURIComponent(owner)}`) || '[]')
      if (Array.isArray(legacy)) for (const id of legacy) if (typeof id === 'string' && /^[a-zA-Z0-9_-]{1,128}$/.test(id)) ids.add(id)
    } catch { /* Ignore an invalid legacy value. */ }
    for (let index = 0; index < window.localStorage.length; index++) {
      const key = window.localStorage.key(index)
      if (key?.startsWith(prefix)) {
        const id = key.slice(prefix.length)
        if (/^[a-zA-Z0-9_-]{1,128}$/.test(id)) ids.add(id)
      }
    }
    return [...ids]
  } catch { return [] }
}
function writePending(owner: string, id: string): void {
  if (!owner) return
  try { window.localStorage.setItem(`${pendingKey}:${encodeURIComponent(owner)}:${id}`, '1') } catch { /* best effort */ }
}
function forgetPending(owner: string, id: string): void {
  if (!owner) return
  try {
    window.localStorage.removeItem(`${pendingKey}:${encodeURIComponent(owner)}:${id}`)
    const legacyKey = `${pendingKey}:${encodeURIComponent(owner)}`
    const legacy: unknown = JSON.parse(window.localStorage.getItem(legacyKey) || '[]')
    if (Array.isArray(legacy) && legacy.includes(id)) window.localStorage.setItem(legacyKey, JSON.stringify(legacy.filter(value => value !== id)))
  } catch { /* best effort */ }
}

// Cancel is idempotent. A successful response means the Agent has persisted
// the cancellation intent; on network failure keep the identity for retry.
export function requestQuietAppJobStop(id: string, owner: string, currentOwner: () => string): void {
  const key = `${owner}\0${id}`
  if (!/^[a-zA-Z0-9_-]{1,128}$/.test(id) || active.has(key)) return
  writePending(owner, id)
  active.add(key)
  const attempt = async (remaining: number, missingRemaining: number): Promise<void> => {
    if (owner && currentOwner() !== owner) { active.delete(key); return }
    try {
      await api.apps.cancelJob(id, AbortSignal.timeout(10_000))
      if (owner && currentOwner() !== owner) { active.delete(key); return }
      forgetPending(owner, id)
      active.delete(key)
    } catch (reason) {
      if (owner && currentOwner() !== owner) { active.delete(key); return }
      if (reason instanceof ApiError && reason.code === 'app_job_not_active') {
        forgetPending(owner, id)
        active.delete(key)
      } else if (reason instanceof ApiError && reason.code === 'app_job_not_found' && missingRemaining > 1 && owner) {
        // A submitted launch may still be reaching the Agent. Its fixed ID
        // lets us retry without stopping a later, unrelated task.
        window.setTimeout(() => void attempt(remaining, missingRemaining - 1), 5000)
      } else if (remaining > 1 && owner) {
        window.setTimeout(() => void attempt(remaining - 1, missingRemaining), 2000)
      } else {
        active.delete(key)
      }
    }
  }
  void attempt(3, 36)
}

export function retryPendingAppJobStops(owner: string, currentOwner: () => string): void {
  if (!owner || scanning.has(owner)) return
  const pending = readPending(owner)
  if (!pending.length) return
  scanning.add(owner)
  let index = 0
  const batch = () => {
    if (currentOwner() !== owner) { scanning.delete(owner); return }
    for (const id of pending.slice(index, index + 64)) requestQuietAppJobStop(id, owner, currentOwner)
    index += 64
    if (index < pending.length) window.setTimeout(batch, 2000)
    else scanning.delete(owner)
  }
  batch()
}
