// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { closeTerminalQuietly } from './terminalClose'

const mocks = vi.hoisted(() => ({ detach: vi.fn(), close: vi.fn(), owner: { authenticated: false, user: { id: 'owner' } } }))
vi.mock('@/lib/api', () => ({
  api: { terminals: { detach: mocks.detach, close: mocks.close } },
  ApiError: class extends Error { constructor(message: string, public status: number, public code: string) { super(message) } },
}))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: mocks.owner }) }))

describe('quiet terminal close', () => {
  beforeEach(() => { vi.useFakeTimers(); mocks.detach.mockReset(); mocks.close.mockReset(); mocks.owner.authenticated = false; mocks.owner.user.id = 'owner'; window.localStorage.clear() })
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
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-terminal-closes:owner')!)).toEqual([id])
    await vi.advanceTimersByTimeAsync(2000)
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-terminal-closes:owner')!)).toEqual([])
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
    expect(JSON.parse(window.localStorage.getItem('kpanel:pending-terminal-closes:owner')!)).toEqual([id])
  })
})
