import { api, ApiError } from '@/lib/api'
import { useSession } from '@/stores/session'

const pendingKey = 'kpanel:pending-terminal-closes'
const active = new Set<string>()
const currentOwner = () => {
  const state = useSession().state
  return state.authenticated ? state.user?.id || '' : ''
}
export const terminalCleanupOwner = currentOwner
function readPending(owner: string): string[] {
  if (!owner) return []
  try {
    const base = `${pendingKey}:${encodeURIComponent(owner)}`
    const ids = new Set<string>()
    try {
      const value: unknown = JSON.parse(window.localStorage.getItem(base) || '[]')
      if (Array.isArray(value)) for (const id of value) if (typeof id === 'string' && /^[a-f0-9]{64}$/.test(id)) ids.add(id)
    } catch { /* Ignore an invalid legacy value. */ }
    for (let index = 0; index < window.localStorage.length; index++) {
      const key = window.localStorage.key(index)
      if (key?.startsWith(`${base}:`)) {
        const id = key.slice(base.length + 1)
        if (/^[a-f0-9]{64}$/.test(id)) ids.add(id)
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
    const base = `${pendingKey}:${encodeURIComponent(owner)}`
    window.localStorage.removeItem(`${base}:${id}`)
    const value: unknown = JSON.parse(window.localStorage.getItem(base) || '[]')
    if (Array.isArray(value) && value.includes(id)) window.localStorage.setItem(base, JSON.stringify(value.filter(entry => entry !== id)))
  } catch { /* best effort */ }
}
function queuePending(id: string, owner: string): void {
  if (!owner || !/^[a-f0-9]{64}$/.test(id)) return
  const key = `${owner}\0${id}`
  if (active.has(key)) return
  writePending(owner, id)
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

export async function closeTerminalForOwner(id: string, owner: string, close: () => Promise<void>): Promise<'closed' | 'detached' | 'missing' | 'pending'> {
  if (owner && currentOwner() !== owner) {
    queuePending(id, owner)
    return 'pending'
  }
  return closeTerminalQuietly(id, close)
}

const scanningCloses = new Set<string>()
export function retryPendingTerminalCloses(): void {
  const owner = currentOwner()
  retryLaunchAcks(owner)
  if (!owner || scanningCloses.has(owner)) return
  const pending: Array<() => void> = [
    ...readPending(owner).map(id => () => queuePending(id, owner)),
    ...readLaunches(owner).map(launch => () => queueTerminalLaunchClose(launch, owner)),
  ]
  if (!pending.length) return
  scanningCloses.add(owner)
  let index = 0
  const batch = () => {
    if (currentOwner() !== owner) { scanningCloses.delete(owner); return }
    for (const retry of pending.slice(index, index + 64)) retry()
    index += 64
    if (index < pending.length) window.setTimeout(batch, 2000)
    else scanningCloses.delete(owner)
  }
  batch()
}

type AckTarget = { shortcutId?: string; purpose?: 'site-management' }
type LaunchAck = { sessionId: string; launchId: string; target: AckTarget }
const ackKey = 'kpanel:pending-terminal-launch-acks'
const scanningAcks = new Set<string>()
function readLaunchAcks(owner: string): LaunchAck[] {
  if (!owner) return []
  try {
    const prefix = `${ackKey}:${encodeURIComponent(owner)}:`
    const result: LaunchAck[] = []
    for (let index = 0; index < window.localStorage.length; index++) {
      const key = window.localStorage.key(index)
      if (!key?.startsWith(prefix)) continue
      const [sessionId, launchId, extra] = key.slice(prefix.length).split(':')
      if (!extra && /^[a-f0-9]{64}$/.test(sessionId || '') && /^[a-f0-9]{64}$/.test(launchId || '')) {
        let target: AckTarget = {}
        try {
          const value: unknown = JSON.parse(window.localStorage.getItem(key) || '{}')
          if (value && typeof value === 'object') {
            const saved = value as AckTarget
            if (saved.purpose === 'site-management') target = { purpose: saved.purpose }
            else if (typeof saved.shortcutId === 'string' && /^[a-f0-9]{32}$/.test(saved.shortcutId)) target = { shortcutId: saved.shortcutId }
          }
        } catch { /* An older acknowledgement has no target metadata. */ }
        result.push({ sessionId: sessionId!, launchId: launchId!, target })
      }
    }
    return result
  } catch { return [] }
}
function launchAckStorageKey(owner: string, sessionId: string, launchId: string): string {
  return `${ackKey}:${encodeURIComponent(owner)}:${sessionId}:${launchId}`
}
// Release known acknowledgement slots before opening another named session.
// Requests are idempotent; failures stay queued and do not replay a command.
export async function flushPendingTerminalLaunchAcks(owner: string, target?: AckTarget): Promise<void> {
  if (!owner || currentOwner() !== owner) return
  const pending = readLaunchAcks(owner).sort((a, b) => {
    const matches = (item: LaunchAck) => target && (target.purpose ? item.target.purpose === target.purpose : target.shortcutId && item.target.shortcutId === target.shortcutId) ? 1 : 0
    return matches(b) - matches(a)
  }).slice(0, 64)
  await Promise.all(pending.map(async ({ sessionId, launchId }) => {
    if (currentOwner() !== owner) return
    try {
      const result = await api.terminals.acknowledgeLaunch(sessionId, launchId)
      if (currentOwner() === owner && result.acknowledged) window.localStorage.removeItem(launchAckStorageKey(owner, sessionId, launchId))
    } catch (reason) {
      if (currentOwner() === owner && reason instanceof ApiError && reason.code === 'terminal_not_found') {
        try { window.localStorage.removeItem(launchAckStorageKey(owner, sessionId, launchId)) } catch { /* best effort */ }
      }
    }
  }))
}
function retryLaunchAcks(owner: string): void {
  if (!owner || scanningAcks.has(owner)) return
  const pending = readLaunchAcks(owner)
  if (!pending.length) return
  scanningAcks.add(owner)
  let index = 0
  const batch = () => {
    if (currentOwner() !== owner) { scanningAcks.delete(owner); return }
    for (const ack of pending.slice(index, index + 64)) queueTerminalLaunchAck(ack.sessionId, ack.launchId, owner, ack.target)
    index += 64
    if (index < pending.length) window.setTimeout(batch, 2000)
    else scanningAcks.delete(owner)
  }
  batch()
}

// The server keeps a launch identity until the browser confirms receipt.
// Keep failed acknowledgements so reconnecting can release that bounded slot.
export function queueTerminalLaunchAck(sessionId: string, launchId: string, owner: string, target: AckTarget = {}): void {
  if (!owner || !/^[a-f0-9]{64}$/.test(sessionId) || !/^[a-f0-9]{64}$/.test(launchId)) {
    void api.terminals.acknowledgeLaunch(sessionId, launchId).catch(() => undefined)
    return
  }
  const key = `ack\0${owner}\0${sessionId}\0${launchId}`
  const storageKey = launchAckStorageKey(owner, sessionId, launchId)
  const saved: AckTarget = target.purpose === 'site-management' ? { purpose: target.purpose }
    : typeof target.shortcutId === 'string' && /^[a-f0-9]{32}$/.test(target.shortcutId) ? { shortcutId: target.shortcutId } : {}
  try { window.localStorage.setItem(storageKey, JSON.stringify(saved)) } catch { /* best effort */ }
  if (active.has(key)) return
  active.add(key)
  const attempt = async (remaining: number): Promise<void> => {
    if (currentOwner() !== owner) { active.delete(key); return }
    try {
      const result = await api.terminals.acknowledgeLaunch(sessionId, launchId)
      if (currentOwner() !== owner) { active.delete(key); return }
      if (!result.acknowledged) throw new Error('Launch acknowledgement was not confirmed')
      try { window.localStorage.removeItem(storageKey) } catch { /* best effort */ }
      active.delete(key)
    } catch (reason) {
      if (currentOwner() !== owner) { active.delete(key); return }
      if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
        try { window.localStorage.removeItem(storageKey) } catch { /* best effort */ }
        active.delete(key)
      } else if (remaining > 1) {
        window.setTimeout(() => void attempt(remaining - 1), 2000)
      } else active.delete(key)
    }
  }
  void attempt(3)
}

type PendingLaunch = { launchId: string; hostId: string; shortcutId?: string; purpose?: 'site-management' }
const launchKey = 'kpanel:pending-terminal-launch-closes'
function validLaunch(value: unknown): value is PendingLaunch {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<PendingLaunch>
  return typeof item.launchId === 'string' && /^[a-f0-9]{64}$/.test(item.launchId) && typeof item.hostId === 'string' && item.hostId.length <= 128 && (item.purpose === 'site-management' || typeof item.shortcutId === 'string' && /^[a-f0-9]{32}$/.test(item.shortcutId))
}
function readLaunches(owner: string): PendingLaunch[] {
  if (!owner) return []
  try {
    const base = `${launchKey}:${encodeURIComponent(owner)}`
    const entries = new Map<string, PendingLaunch>()
    try {
      const value: unknown = JSON.parse(window.localStorage.getItem(base) || '[]')
      if (Array.isArray(value)) for (const item of value) if (validLaunch(item)) entries.set(item.launchId, item)
    } catch { /* Ignore an invalid legacy value. */ }
    for (let index = 0; index < window.localStorage.length; index++) {
      const key = window.localStorage.key(index)
      if (!key?.startsWith(`${base}:`)) continue
      try {
        const value: unknown = JSON.parse(window.localStorage.getItem(key) || 'null')
        if (validLaunch(value) && key === `${base}:${value.launchId}`) entries.set(value.launchId, value)
      } catch { /* Ignore an invalid entry. */ }
    }
    return [...entries.values()]
  } catch { return [] }
}
function writeLaunch(owner: string, launch: PendingLaunch): void {
  if (!owner) return
  try { window.localStorage.setItem(`${launchKey}:${encodeURIComponent(owner)}:${launch.launchId}`, JSON.stringify(launch)) } catch { /* best effort */ }
}
function forgetLaunch(owner: string, launchId: string): void {
  if (!owner) return
  try {
    const base = `${launchKey}:${encodeURIComponent(owner)}`
    window.localStorage.removeItem(`${base}:${launchId}`)
    const value: unknown = JSON.parse(window.localStorage.getItem(base) || '[]')
    if (Array.isArray(value) && value.some(entry => validLaunch(entry) && entry.launchId === launchId)) {
      window.localStorage.setItem(base, JSON.stringify(value.filter(entry => !validLaunch(entry) || entry.launchId !== launchId)))
    }
  } catch { /* best effort */ }
}

// Lookup consumes the prepared identity and never starts a command.
export function queueTerminalLaunchClose(launch: PendingLaunch, owner: string): void {
  if (!owner || !validLaunch(launch)) return
  const key = `launch\0${owner}\0${launch.launchId}`
  writeLaunch(owner, launch)
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
      forgetLaunch(owner, launch.launchId)
      active.delete(key)
    } catch (reason) {
      if (currentOwner() !== owner) { active.delete(key); return }
      if (reason instanceof ApiError && reason.code === 'terminal_shortcut_session_not_found') {
        forgetLaunch(owner, launch.launchId)
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
  // Persist before the first request: the page can disappear while that
  // request is still in flight, before a failure handler has a chance to run.
  if (owner && /^[a-f0-9]{64}$/.test(id)) writePending(owner, id)
  try {
    await close()
    forgetPending(owner, id)
    return 'closed'
  } catch (reason) {
    if (owner && currentOwner() !== owner) {
      if (queueOnFailure) queuePending(id, owner)
      return 'pending'
    }
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      forgetPending(owner, id)
      return 'missing'
    }
  }
  if (owner && currentOwner() !== owner) {
    if (queueOnFailure) queuePending(id, owner)
    return 'pending'
  }
  try {
    const result = await api.terminals.detach(id)
    if (!result.detached) throw new Error('Terminal cleanup was not recorded')
    forgetPending(owner, id)
    return 'detached'
  } catch (reason) {
    if (owner && currentOwner() !== owner) {
      if (queueOnFailure) queuePending(id, owner)
      return 'pending'
    }
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      forgetPending(owner, id)
      return 'missing'
    }
    // The server may itself be unreachable or its bounded cleanup queue full.
    // Do not claim termination: its existing session remains eligible for retry.
    if (queueOnFailure) queuePending(id, owner)
    return 'pending'
  }
}
