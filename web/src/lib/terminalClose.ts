import { api, ApiError } from '@/lib/api'
import { useSession } from '@/stores/session'

const pendingKey = 'kpanel:pending-terminal-closes'
const active = new Set<string>()
const currentOwner = () => {
  const state = useSession().state
  return state.authenticated ? state.user?.id || '' : ''
}
function readPending(owner: string): string[] {
  if (!owner) return []
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(`${pendingKey}:${encodeURIComponent(owner)}`) || '[]')
    return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string' && /^[a-f0-9]{64}$/.test(id)).slice(-32) : []
  } catch { return [] }
}
function writePending(owner: string, ids: string[]): void {
  if (!owner) return
  try { window.localStorage.setItem(`${pendingKey}:${encodeURIComponent(owner)}`, JSON.stringify(ids.slice(-32))) } catch { /* best effort */ }
}
function forgetPending(owner: string, id: string): void {
  writePending(owner, readPending(owner).filter(value => value !== id))
}
function queuePending(id: string, owner: string): void {
  if (!owner || !/^[a-f0-9]{64}$/.test(id)) return
  const key = `${owner}\0${id}`
  if (active.has(key)) return
  writePending(owner, [...readPending(owner).filter(value => value !== id), id])
  active.add(key)
  const attempt = async (remaining: number): Promise<void> => {
    if (currentOwner() !== owner) { active.delete(key); return }
    const result = await closeTerminalQuietly(id, async () => {
      if (!(await api.terminals.close(id)).closed) throw new Error('Terminal close was not confirmed')
    }, false)
    if (result !== 'pending') {
      forgetPending(owner, id)
      active.delete(key)
    } else if (remaining > 1) {
      window.setTimeout(() => void attempt(remaining - 1), 2000)
    } else {
      active.delete(key)
    }
  }
  window.setTimeout(() => void attempt(3), 2000)
}

export function retryPendingTerminalCloses(): void {
  const owner = currentOwner()
  for (const id of readPending(owner)) queuePending(id, owner)
}

// A failed remote close is not proof of termination. Try to preserve a
// server-side cleanup identity and queue another attempt if it is unavailable.
export async function closeTerminalQuietly(id: string, close: () => Promise<void>, queueOnFailure = true): Promise<'closed' | 'detached' | 'missing' | 'pending'> {
  try {
    await close()
    return 'closed'
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') return 'missing'
  }
  try {
    const result = await api.terminals.detach(id)
    if (!result.detached) throw new Error('Terminal cleanup was not recorded')
    return 'detached'
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') return 'missing'
    // The server may itself be unreachable or its bounded cleanup queue full.
    // Do not claim termination: its existing session remains eligible for retry.
    if (queueOnFailure) queuePending(id, currentOwner())
    return 'pending'
  }
}
