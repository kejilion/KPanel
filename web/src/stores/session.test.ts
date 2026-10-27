// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AuthStatus } from '@/types/api'

const mocks = vi.hoisted(() => ({
  status: vi.fn(), login: vi.fn(), logout: vi.fn(), reset: vi.fn(), stop: vi.fn(), apply: vi.fn(), cancel: vi.fn(),
  startPreferences: vi.fn(), stopPreferences: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { auth: { status: mocks.status, login: mocks.login, logout: mocks.logout } }, resetApiSecurityState: mocks.reset }))
vi.mock('@/lib/appearanceSync', () => ({ stopAppearanceSync: mocks.stop }))
vi.mock('@/lib/loginAppearance', () => ({ applyLoginAppearance: mocks.apply, cancelLoginWallpaper: mocks.cancel }))
vi.mock('@/lib/preferencesSync', () => ({ startPreferencesSync: mocks.startPreferences, stopPreferencesSync: mocks.stopPreferences }))
import { useSession } from './session'

it('passes a finite timeout to bootstrap status so a stalled connection can recover', async () => {
  const timeout = vi.spyOn(AbortSignal, 'timeout')
  mocks.status.mockResolvedValue({ setupRequired: false, authenticated: false })
  await useSession().refresh(true)
  expect(timeout).toHaveBeenCalledWith(10_000)
  expect(mocks.status).toHaveBeenLastCalledWith(expect.any(AbortSignal))
  timeout.mockRestore()
})

const appearance = { configured: true, resourceVersion: 'sha256:current', theme: 'dark' as const, colors: null, wallpaper: 'prism', classicLevel: 'off' as const }
const guest: AuthStatus = {
  setupRequired: false, authenticated: false,
  loginAppearance: { theme: 'dark', colors: null, wallpaper: { url: '/wallpapers/kpanel-desktop-prism.webp', focusX: 500, focusY: 500, bright: false } },
}
const authenticated: AuthStatus = { setupRequired: false, authenticated: true, user: { id: 'admin', username: 'admin' }, appearance }

it('rechecks authentication after waiting for preferences so logout cannot enter the shell', async () => {
  let resolve!: () => void
  mocks.startPreferences.mockImplementationOnce(() => new Promise<void>(done => { resolve = done }))
  const session = useSession()
  mocks.login.mockResolvedValue(authenticated)
  await session.login({ username: 'admin', password: 'fixture-only' })
  const restore = session.restorePreferences()
  mocks.status.mockResolvedValue(guest)
  await session.refresh(true)
  resolve()
  expect(await restore).toBe(false)
  expect(mocks.stopPreferences).toHaveBeenCalled()
})

it('rejects a hydration handoff belonging to the previous account', async () => {
  let resolve!: () => void
  mocks.startPreferences.mockImplementationOnce(() => new Promise<void>(done => { resolve = done }))
  const session = useSession()
  mocks.login.mockResolvedValueOnce(authenticated).mockResolvedValueOnce({ ...authenticated, user: { id: 'new', username: 'new' } })
  await session.login({ username: 'admin', password: 'fixture-only' })
  const restore = session.restorePreferences()
  await session.login({ username: 'new', password: 'fixture-only' })
  resolve()
  expect(await restore).toBe(false)
})

describe('session appearance handoff', () => {
  beforeEach(() => vi.resetAllMocks())
  it('hands off a login snapshot once so reentering the shell reads current server state', async () => {
    mocks.login.mockResolvedValue(authenticated)
    const session = useSession()
    await session.login({ username: 'admin', password: 'fixture-only' })
    expect(session.takeAppearanceSnapshot()).toEqual(appearance)
    expect(session.takeAppearanceSnapshot()).toBeUndefined()
    mocks.status.mockResolvedValue({ ...authenticated, appearance: { ...appearance, wallpaper: 'rift' } })
    await session.refresh(true)
    expect(session.takeAppearanceSnapshot()?.wallpaper).toBe('rift')
  })

  it('keeps appearance synchronization and the session alive if logout fails', async () => {
    mocks.login.mockResolvedValue(authenticated)
    mocks.logout.mockRejectedValue(new Error('offline'))
    const session = useSession()
    await session.login({ username: 'admin', password: 'fixture-only' })
    await expect(session.logout()).rejects.toThrow('offline')
    expect(session.state.authenticated).toBe(true)
    expect(session.state.loading).toBe(false)
    expect(mocks.stop).not.toHaveBeenCalled()
    expect(mocks.reset).not.toHaveBeenCalled()
    expect(mocks.status).not.toHaveBeenCalled()
  })

  it('does not let a delayed logout clear a newer login in memory', async () => {
    let finish!: () => void
    mocks.logout.mockReturnValue(new Promise<void>(resolve => { finish = resolve }))
    mocks.login.mockResolvedValue(authenticated)
    const session = useSession()
    const old = session.logout()
    await session.login({ username: 'admin', password: 'fixture-only' })
    finish()
    await old
    expect(session.state.authenticated).toBe(true)
    expect(mocks.stop).not.toHaveBeenCalled()
    expect(mocks.status).not.toHaveBeenCalled()
  })

  it('carries authenticated appearance to the shell and reloads guest branding on logout', async () => {
    mocks.login.mockResolvedValue(authenticated)
    mocks.logout.mockResolvedValue(undefined)
    mocks.status.mockResolvedValue(guest)
    const session = useSession()
    await session.login({ username: 'admin', password: 'fixture-only' })
    expect(session.state.appearance).toEqual(appearance)
    await session.logout()
    expect(mocks.stop).toHaveBeenCalledTimes(1)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    expect(mocks.apply).toHaveBeenCalledWith(guest.loginAppearance)
    expect(session.state.authenticated).toBe(false)
    expect(session.state.appearance).toBeUndefined()
  })

  it('does not restore an old authenticated state after logout refresh', async () => {
    let resolve!: (value: AuthStatus) => void
    mocks.status.mockReturnValueOnce(new Promise(done => { resolve = done })).mockResolvedValueOnce(guest)
    mocks.logout.mockResolvedValue(undefined)
    const session = useSession()
    const old = session.refresh(true)
    await session.logout()
    resolve(authenticated)
    await old
    expect(session.state.authenticated).toBe(false)
    expect(mocks.apply).toHaveBeenLastCalledWith(guest.loginAppearance)
  })

  it('deduplicates session reads and prevents a slow guest response from replacing a login', async () => {
    let resolve!: (value: AuthStatus) => void
    mocks.status.mockReturnValueOnce(new Promise(done => { resolve = done }))
    mocks.login.mockResolvedValue(authenticated)
    const session = useSession()
    const old = session.refresh(true)
    const same = session.refresh()
    expect(mocks.status).toHaveBeenCalledTimes(1)
    await session.login({ username: 'admin', password: 'fixture-only' })
    resolve(guest)
    await Promise.all([old, same])
    expect(session.state.authenticated).toBe(true)
    expect(session.state.appearance).toEqual(appearance)
    expect(mocks.apply).not.toHaveBeenCalled()
  })
})
