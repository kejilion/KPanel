import { api, ApiError } from '@/lib/api'

const pendingKey = 'kpanel:pending-app-job-stops'
const maxPending = 32
const active = new Set<string>()

function readPending(owner: string): string[] {
  if (!owner) return []
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(`${pendingKey}:${encodeURIComponent(owner)}`) || '[]')
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string' && /^[a-zA-Z0-9_-]{1,128}$/.test(id)).slice(-maxPending) : []
  } catch { return [] }
}
function writePending(owner: string, ids: string[]): void {
  if (!owner) return
  try { window.localStorage.setItem(`${pendingKey}:${encodeURIComponent(owner)}`, JSON.stringify(ids.slice(-maxPending))) } catch { /* best effort */ }
}

// Cancel is idempotent. A successful response means the Agent has persisted
// the cancellation intent; on network failure keep the identity for retry.
export function requestQuietAppJobStop(id: string, owner: string, currentOwner: () => string): void {
  const key = `${owner}\0${id}`
  if (!/^[a-zA-Z0-9_-]{1,128}$/.test(id) || active.has(key)) return
  writePending(owner, [...readPending(owner).filter(value => value !== id), id])
  active.add(key)
  const attempt = async (remaining: number): Promise<void> => {
    if (owner && currentOwner() !== owner) { active.delete(key); return }
    try {
      await api.apps.cancelJob(id, AbortSignal.timeout(10_000))
      writePending(owner, readPending(owner).filter(value => value !== id))
      active.delete(key)
    } catch (reason) {
      if (reason instanceof ApiError && reason.code === 'app_job_not_active') {
        writePending(owner, readPending(owner).filter(value => value !== id))
        active.delete(key)
      } else if (remaining > 1 && owner) {
        window.setTimeout(() => void attempt(remaining - 1), 2000)
      } else {
        active.delete(key)
      }
    }
  }
  void attempt(3)
}

export function retryPendingAppJobStops(owner: string, currentOwner: () => string): void {
  for (const id of readPending(owner)) requestQuietAppJobStop(id, owner, currentOwner)
}
