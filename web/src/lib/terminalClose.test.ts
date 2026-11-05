// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { closeTerminalForOwner, closeTerminalQuietly, deferTerminalClose, flushPendingTerminalLaunchAcks, queueTerminalLaunchAck, queueTerminalLaunchClose, retryPendingTerminalCloses } from './terminalClose'

const mocks = vi.hoisted(() => ({ detach: vi.fn(), close: vi.fn(), resolveShortcut: vi.fn(), acknowledgeLaunch: vi.fn(), owner: { authenticated: false, user: { id: 'owner' } } }))
vi.mock('@/lib/api', () => ({
  api: { terminals: { detach: mocks.detach, close: mocks.close, resolveShortcut: mocks.resolveShortcut, acknowledgeLaunch: mocks.acknowledgeLaunch } },
  ApiError: class extends Error { constructor(message: string, public status: number, public code: string) { super(message) } },
}))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: mocks.owner }) }))

describe('quiet terminal close', () => {
  beforeEach(() => { vi.useFakeTimers(); mocks.detach.mockReset(); mocks.close.mockReset(); mocks.acknowledgeLaunch.mockReset(); mocks.owner.authenticated = false; mocks.owner.user.id = 'owner'; window.localStorage.clear() })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })
  it('does not detach a confirmed close', async () => {
    await expect(closeTerminalQuietly('id', async () => undefined)).resolves.toBe('closed')
    expect(mocks.detach).not.toHaveBeenCalled()
  })
  it('records pending cleanup after an unconfirmed close', async () => {
    mocks.detach.mockResolvedValue({ detached: true })
    await expect(closeTerminalQuietly('id', async () => { throw new Error('offline') })).resolves.toBe('detached')
    expect(mocks.detach).toHaveBeenCalledWith('id')
  })
  it('keeps the outcome pending when the server rejects cleanup', async () => {
    mocks.detach.mockRejectedValue(new Error('unavailable'))
    await expect(closeTerminalQuietly('id', async () => { throw new Error('offline') })).resolves.toBe('pending')
  })
  it('accepts only the specific missing-session error', async () => {
    await expect(closeTerminalQuietly('id', async () => { throw new ApiError('missing', 404, 'terminal_not_found') })).resolves.toBe('missing')
    expect(mocks.detach).not.toHaveBeenCalled()
    mocks.detach.mockRejectedValue(new ApiError('missing', 404, 'route_not_found'))
    await expect(closeTerminalQuietly('id', async () => { throw new Error('offline') })).resolves.toBe('pending')
  })
  it('keeps an offline session for bounded, owner-scoped retry', async () => {
    const id = 'a'.repeat(64)
    mocks.owner.authenticated = true
    mocks.detach.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ detached: true })
    mocks.close.mockRejectedValue(new Error('offline'))
    await expect(closeTerminalQuietly(id, async () => { throw new Error('offline') })).resolves.toBe('pending')
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner:${id}`)).toBe('1')
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner:${id}`)).toBeNull()
  })
  it('does not retry a previous account’s session after an account switch', async () => {
    const id = 'b'.repeat(64)
    mocks.owner.authenticated = true
    mocks.owner.user.id = 'owner'
    mocks.detach.mockRejectedValue(new Error('offline'))
    await closeTerminalQuietly(id, async () => { throw new Error('offline') })
    mocks.owner.user.id = 'other'
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.close).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner:${id}`)).toBe('1')
  })
  it('keeps the original owner when the account changes during close', async () => {
    const id = 'c'.repeat(64)
    mocks.owner.authenticated = true
    let reject!: (reason: Error) => void
    const closing = closeTerminalQuietly(id, () => new Promise((_, fail) => { reject = fail }))
    mocks.owner.user.id = 'other'
    reject(new Error('offline'))
    await expect(closing).resolves.toBe('pending')
    expect(mocks.detach).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner:${id}`)).toBe('1')
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:other:${id}`)).toBeNull()
  })
  it('defers a late session to its original owner without using another account', async () => {
    mocks.owner.authenticated = true
    mocks.owner.user.id = 'other'
    const close = vi.fn()
    const id = 'f'.repeat(64)
    await expect(closeTerminalForOwner(id, 'owner', close)).resolves.toBe('pending')
    expect(close).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner:${id}`)).toBe('1')
  })
  it('retries a lost launch lookup only for its owner without launching again', async () => {
    mocks.owner.authenticated = true
    mocks.resolveShortcut.mockRejectedValue(new Error('offline'))
    const launch = { launchId: 'd'.repeat(64), shortcutId: 'a'.repeat(32), hostId: 'local' }
    queueTerminalLaunchClose(launch, 'owner')
    await vi.advanceTimersByTimeAsync(0)
    mocks.owner.user.id = 'other'
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.resolveShortcut).toHaveBeenCalledTimes(1)
    const key = `kpanel:pending-terminal-launch-closes:owner:${launch.launchId}`
    expect(JSON.parse(window.localStorage.getItem(key)!)).toEqual(launch)
    mocks.owner.user.id = 'owner'
    mocks.resolveShortcut.mockResolvedValue({ sessionId: 'e'.repeat(64) })
    mocks.close.mockResolvedValue({ closed: true })
    retryPendingTerminalCloses()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.close).toHaveBeenCalledWith('e'.repeat(64))
    expect(window.localStorage.getItem(key)).toBeNull()
  })
  it('persists failed acknowledgements and releases their slot after the same user reconnects', async () => {
    const sessionId = '1'.repeat(64), launchId = '2'.repeat(64)
    mocks.owner.authenticated = true
    mocks.acknowledgeLaunch.mockRejectedValue(new Error('offline'))
    queueTerminalLaunchAck(sessionId, launchId, 'owner')
    await vi.advanceTimersByTimeAsync(4000)
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(3)
    const key = `kpanel:pending-terminal-launch-acks:owner:${sessionId}:${launchId}`
    expect(window.localStorage.getItem(key)).toBe('{}')
    mocks.owner.user.id = 'other'
    retryPendingTerminalCloses()
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(3)
    mocks.owner.user.id = 'owner'
    mocks.acknowledgeLaunch.mockResolvedValueOnce({ acknowledged: true })
    retryPendingTerminalCloses()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(4)
    expect(window.localStorage.getItem(key)).toBeNull()
  })
  it('retries all unconfirmed launch acknowledgements after repeated offline reopenings', async () => {
    const sessionId = '3'.repeat(64)
    mocks.owner.authenticated = true
    mocks.acknowledgeLaunch.mockRejectedValue(new Error('offline'))
    for (let index = 0; index < 16; index++) queueTerminalLaunchAck(sessionId, index.toString(16).padStart(64, '0'), 'owner')
    await vi.advanceTimersByTimeAsync(4000)
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(48)
    mocks.acknowledgeLaunch.mockResolvedValue({ acknowledged: true })
    retryPendingTerminalCloses()
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(64)
    expect([...Array(window.localStorage.length)].map((_, index) => window.localStorage.key(index)).filter(key => key?.startsWith('kpanel:pending-terminal-launch-acks:owner:'))).toEqual([])
  })
  it('prioritizes this shortcut’s acknowledgements when many others are pending', async () => {
    mocks.owner.authenticated = true
    const sessionId = '4'.repeat(64), target = 'a'.repeat(32)
    const own: string[] = []
    for (let index = 0; index < 80; index++) {
      const id = index.toString(16).padStart(64, '0')
      const shortcutId = index >= 64 ? target : 'b'.repeat(32)
      window.localStorage.setItem(`kpanel:pending-terminal-launch-acks:owner:${sessionId}:${id}`, JSON.stringify({ shortcutId }))
      if (index >= 64) own.push(id)
    }
    mocks.acknowledgeLaunch.mockResolvedValue({ acknowledged: true })
    await flushPendingTerminalLaunchAcks('owner', { shortcutId: target })
    expect(mocks.acknowledgeLaunch).toHaveBeenCalledTimes(64)
    for (const id of own) expect(mocks.acknowledgeLaunch).toHaveBeenCalledWith(sessionId, id)
  })
  it('keeps every pending session identity when more than 32 are deferred', () => {
    mocks.owner.authenticated = true
    mocks.owner.user.id = 'other'
    for (let index = 0; index < 40; index++) deferTerminalClose(index.toString(16).padStart(64, '0'), 'bulk-owner')
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:bulk-owner:${'0'.repeat(64)}`)).toBe('1')
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:bulk-owner:${(39).toString(16).padStart(64, '0')}`)).toBe('1')
  })
  it('retries an earlier array-format terminal close without discarding it', async () => {
    const id = '7'.repeat(64)
    mocks.owner.authenticated = true
    window.localStorage.setItem('kpanel:pending-terminal-closes:owner', JSON.stringify([id]))
    mocks.close.mockResolvedValue({ closed: true })
    retryPendingTerminalCloses()
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.close).toHaveBeenCalledWith(id)
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-terminal-closes:owner')!)).toEqual([])
  })
})
