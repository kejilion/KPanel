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

export function deferTerminalClose(id: string, owner: string): void {
  queuePending(id, owner)
}

export function retryPendingTerminalCloses(): void {
  const owner = currentOwner()
  for (const id of readPending(owner)) queuePending(id, owner)
  for (const launch of readLaunches(owner)) queueTerminalLaunchClose(launch, owner)
}

type PendingLaunch = { launchId: string; hostId: string; shortcutId?: string; purpose?: 'site-management' }
const launchKey = 'kpanel:pending-terminal-launch-closes'
function readLaunches(owner: string): PendingLaunch[] {
  if (!owner) return []
  try {
    const values: unknown = JSON.parse(window.localStorage.getItem(`${launchKey}:${encodeURIComponent(owner)}`) || '[]')
    if (!Array.isArray(values)) return []
    return values.filter((v): v is PendingLaunch => v && typeof v === 'object' && /^[a-f0-9]{64}$/.test(v.launchId) && typeof v.hostId === 'string' && v.hostId.length <= 128 && (v.purpose === 'site-management' || /^[a-f0-9]{32}$/.test(v.shortcutId))).slice(-32)
  } catch { return [] }
}
function writeLaunches(owner: string, launches: PendingLaunch[]): void {
  if (!owner) return
  try { window.localStorage.setItem(`${launchKey}:${encodeURIComponent(owner)}`, JSON.stringify(launches.slice(-32))) } catch { /* best effort */ }
}

// Lookup consumes the prepared identity and never starts a command.
export function queueTerminalLaunchClose(launch: PendingLaunch, owner: string): void {
  if (!owner) return
  const key = `launch\0${owner}\0${launch.launchId}`
  writeLaunches(owner, [...readLaunches(owner).filter(value => value.launchId !== launch.launchId), launch])
  if (active.has(key)) return
  active.add(key)
  const attempt = async (remaining: number): Promise<void> => {
    if (currentOwner() !== owner) { active.delete(key); return }
    try {
      const signal = AbortSignal.timeout(10_000)
      const session = launch.purpose
        ? await api.terminals.resolvePurpose(launch.purpose, launch.launchId, signal)
        : await api.terminals.resolveShortcut(launch.hostId, launch.shortcutId!, launch.launchId, signal)
      if (currentOwner() !== owner) { active.delete(key); return }
      await closeTerminalQuietly(session.sessionId, async () => {
        if (!(await api.terminals.close(session.sessionId)).closed) throw new Error('Terminal close was not confirmed')
      })
      writeLaunches(owner, readLaunches(owner).filter(value => value.launchId !== launch.launchId))
      active.delete(key)
    } catch (reason) {
      if (reason instanceof ApiError && reason.code === 'terminal_shortcut_session_not_found') {
        writeLaunches(owner, readLaunches(owner).filter(value => value.launchId !== launch.launchId))
        active.delete(key)
      } else if (remaining > 1) {
        window.setTimeout(() => void attempt(remaining - 1), 2000)
      } else active.delete(key)
    }
  }
  void attempt(3)
}

// A failed remote close is not proof of termination. Try to preserve a
// server-side cleanup identity and queue another attempt if it is unavailable.
export async function closeTerminalQuietly(id: string, close: () => Promise<void>, queueOnFailure = true): Promise<'closed' | 'detached' | 'missing' | 'pending'> {
  const owner = currentOwner()
  try {
    await close()
    return 'closed'
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') return 'missing'
  }
  if (owner && currentOwner() !== owner) {
    if (queueOnFailure) queuePending(id, owner)
    return 'pending'
  }
  try {
    const result = await api.terminals.detach(id)
    if (!result.detached) throw new Error('Terminal cleanup was not recorded')
    return 'detached'
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') return 'missing'
    // The server may itself be unreachable or its bounded cleanup queue full.
    // Do not claim termination: its existing session remains eligible for retry.
    if (queueOnFailure) queuePending(id, owner)
    return 'pending'
  }
}
