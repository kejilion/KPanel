// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), danger: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { preferences: { get: mocks.get, update: mocks.update } },
  ApiError: class extends Error { constructor(message: string, public status: number) { super(message) } },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ danger: mocks.danger }) }))
import { ApiError } from '@/lib/api'
import { getLocale, resetLocaleForTest, setLocale } from '@/i18n'
import { applyDesktopPreferences, desktopPreferencesSnapshot, initializeDesktopMode, resetDesktopModeForTest, useDesktopMode } from '@/stores/desktopMode'
import type { DesktopSessionPreferences, UserPreferences } from '@/types/api'
import { startPreferencesSync, stopPreferencesSync } from './preferencesSync'
import { mediaLocations, setMediaLocation } from './mediaLocations'

const desktop: DesktopSessionPreferences = { mode: 'desktop', windows: [], windowSizes: [], sideSplitRatio: .6, focusedId: 0 }
const remote = (): UserPreferences => ({ locale: 'zh-TW', desktop: structuredClone(desktop), resourceVersion: 'v1' })
const flush = () => vi.advanceTimersByTimeAsync(500)

it('restores all media locations with the existing single login request', async () => {
  localStorage.setItem('kpanel:gallery:v1', JSON.stringify({ root: '/old/photos' }))
  mocks.get.mockResolvedValue({ ...remote(), galleryLocation: { path: '/new/photos', hostId: '' },
    musicLocation: { path: '/music', hostId: 'edge-1' }, cinemaLocation: { path: '/films', hostId: '' } })
  await startPreferencesSync('admin')
  expect(mocks.get).toHaveBeenCalledTimes(1)
  expect(mocks.update).not.toHaveBeenCalled()
  expect(mediaLocations.galleryLocation?.path).toBe('/new/photos')
  expect(mediaLocations.musicLocation).toEqual({ path: '/music', hostId: 'edge-1' })
  expect(mediaLocations.cinemaLocation?.path).toBe('/films')
})

it('migrates a legacy gallery only when the server has no gallery location', async () => {
  localStorage.setItem('kpanel:gallery:v1', JSON.stringify({ root: '/old/photos' }))
  await startPreferencesSync('admin')
  expect(mocks.update.mock.calls[0]![0]).toEqual({ galleryLocation: { path: '/old/photos', hostId: '' }, expectedResourceVersion: 'v1' })
  expect(mediaLocations.galleryLocation?.path).toBe('/old/photos')
})

it('drops a competing gallery migration and restores the winning server path', async () => {
  localStorage.setItem('kpanel:gallery:v1', JSON.stringify({ root: '/old/photos' }))
  mocks.update.mockRejectedValueOnce(new ApiError('conflict', 409))
  mocks.get.mockResolvedValueOnce(remote()).mockResolvedValueOnce({ ...remote(), resourceVersion: 'other', galleryLocation: { path: '/other/photos', hostId: '' } })
  await startPreferencesSync('admin'); await flush()
  expect(mediaLocations.galleryLocation?.path).toBe('/other/photos')
  expect(mocks.update).toHaveBeenCalledTimes(1)
})

it('retries only the explicitly edited media section on conflict and retains pending edits', async () => {
  mocks.get.mockResolvedValue({ ...remote(), musicLocation: { path: '/old', hostId: '' } })
  await startPreferencesSync('admin')
  mocks.update.mockRejectedValueOnce(new ApiError('conflict', 409))
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'other', cinemaLocation: { path: '/other', hostId: '' } })
  setMediaLocation('musicLocation', '/new', 'edge-1')
  await flush()
  expect(mocks.update.mock.calls[1]![0]).toEqual({ musicLocation: { path: '/new', hostId: 'edge-1' }, expectedResourceVersion: 'other' })
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
})

it('restores a pending location after interrupted saving and rejects malformed pending paths', async () => {
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: {
    musicLocation: { path: '/pending', hostId: 'edge-1' }, cinemaLocation: { path: '/../../etc', hostId: '' },
  } }))
  await startPreferencesSync('admin')
  expect(mediaLocations.musicLocation?.path).toBe('/pending')
  expect(mocks.update.mock.calls[0]![0]).toEqual({ musicLocation: { path: '/pending', hostId: 'edge-1' }, expectedResourceVersion: 'v1' })
})

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.get.mockReset(); mocks.update.mockReset()
  localStorage.clear(); sessionStorage.clear()
  resetDesktopModeForTest(); resetLocaleForTest()
  Object.defineProperty(window, 'innerWidth', { value: 1440, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: 900, configurable: true })
  mocks.get.mockResolvedValue(remote())
  mocks.update.mockImplementation(async (patch) => ({ ...remote(), ...patch, resourceVersion: 'v2' }))
})
afterEach(() => { stopPreferencesSync(); vi.useRealTimers() })

it('server state wins over another browser cache, including an empty taskbar', async () => {
  useDesktopMode().openWindow('/files', 'route.files', true)
  await setLocale('en-US')
  await startPreferencesSync('admin')
  expect(getLocale()).toBe('zh-TW')
  expect(useDesktopMode().mode.value).toBe('desktop')
  expect(useDesktopMode().windows.value).toHaveLength(0)
  expect(mocks.update).not.toHaveBeenCalled()
})

it('migrates an unconfigured server once; a clean browser then restores it', async () => {
  mocks.get.mockResolvedValue({ resourceVersion: 'empty' })
  useDesktopMode().enterDesktop()
  useDesktopMode().openWindow('/files?path=%2Fhome', 'route.files', true)
  await setLocale('en-US')
  await startPreferencesSync('admin')
  const saved = mocks.update.mock.calls[0]![0]
  expect(saved.locale).toBe('en-US')
  expect(saved.desktop.windows[0].path).toBe('/files?path=%2Fhome')
  stopPreferencesSync(); localStorage.clear(); sessionStorage.clear(); resetDesktopModeForTest()
  mocks.get.mockResolvedValue({ ...saved, resourceVersion: 'v2' })
  await startPreferencesSync('admin')
  expect(useDesktopMode().windows.value).toHaveLength(1)
  expect(useDesktopMode().consumeWindowLaunch(useDesktopMode().windows.value[0]!.id)).toBe(false)
})

it('debounces window and language changes into one bounded update', async () => {
  await startPreferencesSync('admin')
  const store = useDesktopMode()
  const id = store.openWindow('/files', 'route.files', true)
  store.minimizeWindow(id)
  await setLocale('en-US')
  await flush()
  expect(mocks.update).toHaveBeenCalledTimes(1)
  expect(mocks.update.mock.calls[0]![0]).toMatchObject({ locale: 'en-US', desktop: { windows: [{ id, minimized: true }] } })
})

it('retries only the edited section after another browser updates the server', async () => {
  await startPreferencesSync('admin')
  mocks.update.mockRejectedValueOnce(new ApiError('conflict', 409))
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'other-browser' })
  await setLocale('en-US')
  await flush()
  expect(mocks.update.mock.calls[1]![0]).toEqual({ locale: 'en-US', expectedResourceVersion: 'other-browser' })
  expect(useDesktopMode().windows.value).toHaveLength(0)
})

it('drops conflicting legacy migration without remounting live windows; next refresh restores the server', async () => {
  mocks.get.mockResolvedValueOnce({ resourceVersion: 'empty' }).mockResolvedValueOnce(remote())
  mocks.update.mockRejectedValueOnce(new ApiError('conflict', 409))
  await startPreferencesSync('admin')
  await flush()
  expect(mocks.update).toHaveBeenCalledTimes(1)
  expect(getLocale()).toBe('zh-CN')
  expect(useDesktopMode().mode.value).toBe('classic')
  stopPreferencesSync()
  await startPreferencesSync('admin')
  expect(getLocale()).toBe('zh-TW')
  expect(useDesktopMode().mode.value).toBe('desktop')
})

it('restores initial preferences without waiting for a slow migration write', async () => {
  mocks.get.mockResolvedValue({ resourceVersion: 'empty' })
  let finish!: (value: UserPreferences) => void
  mocks.update.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
  useDesktopMode().enterDesktop()
  await startPreferencesSync('admin')
  expect(useDesktopMode().mode.value).toBe('desktop')
  expect(mocks.update).toHaveBeenCalledTimes(1)
  finish({ ...remote(), locale: 'zh-CN', resourceVersion: 'saved' })
  await flush()
  expect(mocks.update).toHaveBeenCalledTimes(1)
})

it('restores pending explicit preferences before their background save finishes', async () => {
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: { locale: 'en-US' } }))
  let finish!: (value: UserPreferences) => void
  mocks.update.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
  await startPreferencesSync('admin')
  expect(getLocale()).toBe('en-US')
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toContain('en-US')
  finish({ ...remote(), locale: 'en-US', resourceVersion: 'saved' })
  await flush()
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
})

it('ignores a completed background write from the account that logged out', async () => {
  let finish!: (value: UserPreferences) => void
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'old', patch: { locale: 'en-US' } }))
  mocks.update.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
  await startPreferencesSync('old')
  stopPreferencesSync()
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'new', patch: { locale: 'zh-CN' } }))
  let finishNew!: (value: UserPreferences) => void
  mocks.update.mockImplementationOnce(() => new Promise((resolve) => { finishNew = resolve }))
  await startPreferencesSync('new')
  finish({ ...remote(), locale: 'en-US', resourceVersion: 'old' })
  await flush()
  expect(getLocale()).toBe('zh-CN')
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toContain('"owner":"new"')
  finishNew({ ...remote(), locale: 'zh-CN', resourceVersion: 'new' })
  await flush()
})

it('keeps edits made during an in-flight save and serializes the next update', async () => {
  await startPreferencesSync('admin')
  let resolve!: (value: UserPreferences) => void
  mocks.update.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
  await setLocale('en-US')
  await flush()
  await setLocale('zh-CN')
  resolve({ ...remote(), locale: 'en-US', resourceVersion: 'v2' })
  await flush()
  expect(mocks.update).toHaveBeenCalledTimes(2)
  expect(mocks.update.mock.calls[1]![0]).toEqual({ locale: 'zh-CN', expectedResourceVersion: 'v2' })
})

it('failed initial loading never writes cached state; retry does not remount live windows', async () => {
  mocks.get.mockRejectedValueOnce(new Error('offline'))
  await startPreferencesSync('admin')
  expect(mocks.update).not.toHaveBeenCalled()
  expect(mocks.danger).toHaveBeenCalledTimes(1)
  const id = useDesktopMode().openWindow('/files', 'route.files', true)
  window.dispatchEvent(new Event('online'))
  await flush()
  expect(useDesktopMode().consumeWindowLaunch(id)).toBe(true)
  expect(mocks.update.mock.calls[0]![0].desktop.windows[0].id).toBe(id)
})

it('restores the untouched saved desktop when the first preferences request fails', async () => {
  const saved = structuredClone(desktop)
  saved.windows = [{ id: 7, path: '/files', geometry: { left: 100, top: 100, width: 900, height: 600 }, minimized: false, maximized: false, snap: '', z: 100 }]
  saved.focusedId = 7
  mocks.get.mockRejectedValueOnce(new Error('initial request interrupted'))
    .mockResolvedValueOnce({ ...remote(), desktop: saved })
  await startPreferencesSync('admin')
  expect(useDesktopMode().mode.value).toBe('classic')
  window.dispatchEvent(new Event('online'))
  await flush()
  expect(useDesktopMode().mode.value).toBe('desktop')
  expect(useDesktopMode().windows.value.map((window) => window.path)).toEqual(['/files'])
  expect(useDesktopMode().consumeWindowLaunch(7)).toBe(false)
  expect(mocks.update).not.toHaveBeenCalled()
})

it('does not remount windows edited while the initial preferences request is pending', async () => {
  let finish!: (value: UserPreferences) => void
  mocks.get.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
  const loading = startPreferencesSync('admin')
  const id = useDesktopMode().openWindow('/files', 'route.files', true)
  finish(remote())
  await loading
  expect(useDesktopMode().windows.value.map((window) => window.id)).toEqual([id])
  expect(useDesktopMode().consumeWindowLaunch(id)).toBe(true)
})

it('retains failed explicit edits across refresh and isolates them from other accounts', async () => {
  await startPreferencesSync('admin')
  mocks.update.mockRejectedValueOnce(new Error('offline'))
  await setLocale('en-US'); await flush()
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toContain('en-US')
  stopPreferencesSync()
  await startPreferencesSync('other-admin')
  expect(getLocale()).toBe('zh-TW')
  stopPreferencesSync()
  await startPreferencesSync('admin')
  expect(getLocale()).toBe('en-US')
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
})

it('ignores responses from an old session after logout', async () => {
  let resolve!: (value: UserPreferences) => void
  mocks.get.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
  const pending = startPreferencesSync('old')
  stopPreferencesSync()
  await startPreferencesSync('new')
  resolve({ ...remote(), locale: 'en-US' })
  await pending
  expect(getLocale()).toBe('zh-TW')
})

it('fits saved windows to the viewport without saving a passive resize or replaying commands', async () => {
  const value = structuredClone(desktop)
  value.windows = [{ id: 1, path: '/command-shortcut/' + 'a'.repeat(32), geometry: { left: 1400, top: 800, width: 1200, height: 800 }, minimized: false, maximized: false, snap: 'left', z: 100 }]
  value.focusedId = 1
  mocks.get.mockResolvedValue({ ...remote(), desktop: value })
  await startPreferencesSync('admin')
  expect(useDesktopMode().consumeWindowLaunch(1)).toBe(false)
  useDesktopMode().resizeForViewport({ width: 390, height: 844 })
  await flush()
  expect(mocks.update).not.toHaveBeenCalled()
  expect(useDesktopMode().windows.value[0]!.geometry.width).toBeLessThanOrEqual(390)
  expect(desktopPreferencesSnapshot().windows[0]!.geometry.width).toBe(1200)
})

it('clears cached windows when the shared layout is intentionally empty', () => {
  useDesktopMode().openWindow('/files', 'route.files', true)
  applyDesktopPreferences(desktop)
  initializeDesktopMode()
  expect(useDesktopMode().windows.value).toHaveLength(0)
})

it('bounds migration retries even when GET succeeds but every PUT fails', async () => {
  mocks.get.mockResolvedValue({ resourceVersion: 'empty' })
  mocks.update.mockRejectedValue(new Error('read-only disk'))
  await startPreferencesSync('admin')
  await vi.advanceTimersByTimeAsync(60000)
  expect(mocks.update).toHaveBeenCalledTimes(5)
  expect(mocks.get).toHaveBeenCalledTimes(5)
  expect(mocks.danger).toHaveBeenCalledTimes(1)
})

it('discards malformed window pending data without blocking a valid language edit', async () => {
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: {
    locale: 'en-US', desktop: { ...desktop, windows: [{ id: 1, path: '/files', geometry: null }] },
  } }))
  await startPreferencesSync('admin')
  expect(mocks.update.mock.calls[0]![0]).toEqual({ locale: 'en-US', expectedResourceVersion: 'v1' })
  expect(getLocale()).toBe('en-US')
})

it('strips UI-only fields from recovered pending windows before sending them', async () => {
  const value = { ...desktop, windows: [{ id: 1, path: '/files', geometry: { left: 20, top: 20, width: 800, height: 600 }, minimized: false, maximized: false, snap: '', z: 100, launchPending: true }] }
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: { desktop: value } }))
  await startPreferencesSync('admin')
  expect(mocks.update.mock.calls[0]![0].desktop.windows[0]).not.toHaveProperty('launchPending')
})

it('does not write an unchanged language choice', async () => {
  await startPreferencesSync('admin')
  await setLocale('zh-TW'); await flush()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('still saves an undo while the prior language write is in flight', async () => {
  await startPreferencesSync('admin')
  let resolve!: (value: UserPreferences) => void
  mocks.update.mockImplementationOnce(() => new Promise(done => { resolve = done }))
  await setLocale('en-US'); await flush()
  await setLocale('zh-TW')
  resolve({ ...remote(), locale: 'en-US', resourceVersion: 'v2' })
  await flush()
  expect(mocks.update.mock.calls[1]![0]).toEqual({ locale: 'zh-TW', expectedResourceVersion: 'v2' })
})

it('updates the browser language cache when server preferences are restored', async () => {
  localStorage.setItem('kejilion-panel-locale', 'zh-CN')
  await startPreferencesSync('admin')
  expect(localStorage.getItem('kejilion-panel-locale')).toBe('zh-TW')
  expect(mocks.update).not.toHaveBeenCalled()
})

it('allocates unique safe IDs after restoring sparse and maximum-sized window IDs', () => {
  const windowValue = { path: '/files', geometry: { left: 20, top: 20, width: 800, height: 600 }, minimized: false, maximized: false, snap: '' as const, z: 100 }
  applyDesktopPreferences({ ...desktop, windows: [1, 3, Number.MAX_SAFE_INTEGER].map(id => ({ ...windowValue, id })) })
  const store = useDesktopMode()
  expect(store.openWindow('/files', 'route.files', true)).toBe(2)
  expect(store.openWindow('/files', 'route.files', true)).toBe(4)
  expect(new Set(store.windows.value.map(w => w.id)).size).toBe(5)
})

it('recovers command-window size preferences and pending edits without re-running the command', async () => {
  const value: DesktopSessionPreferences = { ...desktop, windows: [{ id: 1, path: '/command-shortcut/' + 'a'.repeat(32),
    geometry: { left: 20, top: 20, width: 950, height: 650 }, minimized: false, maximized: false, snap: '', z: 100 }],
    windowSizes: [{ path: '/command-shortcut', width: 950, height: 650 }] }
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: { desktop: value } }))
  await startPreferencesSync('admin')
  expect(mocks.update.mock.calls[0]![0].desktop.windowSizes).toEqual(value.windowSizes)
  expect(desktopPreferencesSnapshot().windowSizes).toEqual(value.windowSizes)
  expect(useDesktopMode().consumeWindowLaunch(1)).toBe(false)
})

it('does not resave an identical desktop DTO just because its property order differs', async () => {
  const value: DesktopSessionPreferences = { ...desktop, windows: [{ id: 1, path: '/files',
    geometry: { left: 20, top: 20, width: 800, height: 600 }, minimized: false, maximized: false, snap: '', z: 100 }] }
  mocks.get.mockResolvedValue({ ...remote(), desktop: value })
  await startPreferencesSync('admin')
  // Recovered tab-storage DTOs use canonical field order, unlike Go's wire DTO.
  const current = desktopPreferencesSnapshot()
  window.dispatchEvent(new CustomEvent('kpanel:preferences-changed', { detail: { desktop: {
    focusedId: current.focusedId, windowSizes: current.windowSizes, sideSplitRatio: current.sideSplitRatio,
    windows: current.windows, mode: current.mode,
  } } }))
  await flush()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('cancels retry when a failed edit is undone to the server value', async () => {
  await startPreferencesSync('admin')
  mocks.update.mockRejectedValue(new Error('offline'))
  await setLocale('en-US'); await flush()
  await setLocale('zh-TW')
  await vi.advanceTimersByTimeAsync(20000)
  expect(mocks.update).toHaveBeenCalledTimes(1)
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
  await setLocale('en-US'); await flush()
  expect(mocks.danger).toHaveBeenCalledTimes(2)
})

it('phone window actions save desktop geometry, snap, sizes and split ratio unchanged', async () => {
  const geometry = { left: 250, top: 150, width: 1200, height: 750 }
  const value: DesktopSessionPreferences = { ...desktop, sideSplitRatio: .62,
    windows: [{ id: 1, path: '/files', geometry, minimized: false, maximized: false, snap: 'left', z: 100 }],
    windowSizes: [{ path: '/files', width: 1200, height: 750 }], focusedId: 1 }
  Object.defineProperty(window, 'innerWidth', { value: 390, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: 844, configurable: true })
  mocks.get.mockResolvedValue({ ...remote(), desktop: value })
  await startPreferencesSync('admin')
  const store = useDesktopMode()
  expect(store.windows.value[0]!.geometry.width).toBeLessThanOrEqual(390)
  store.minimizeWindow(1)
  await flush()
  const sent = mocks.update.mock.calls[0]![0].desktop
  expect(sent.windows[0]).toMatchObject({ geometry, snap: 'left', minimized: true })
  expect(sent.windowSizes).toEqual(value.windowSizes)
  expect(sent.sideSplitRatio).toBe(.62)
  // A subsequent computer session must get the original desktop layout.
  stopPreferencesSync(); resetDesktopModeForTest(); sessionStorage.clear()
  Object.defineProperty(window, 'innerWidth', { value: 1600, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: 1000, configurable: true })
  mocks.get.mockResolvedValue({ ...remote(), desktop: sent })
  await startPreferencesSync('admin')
  expect(store.windows.value[0]!.geometry).toEqual(geometry)
})

it('restores preferred geometry when the same browser grows from phone to desktop', () => {
  const geometry = { left: 200, top: 80, width: 1100, height: 700 }
  applyDesktopPreferences({ ...desktop, windows: [{ id: 1, path: '/files', geometry, minimized: false, maximized: false, snap: 'right', z: 100 }] })
  const store = useDesktopMode()
  store.resizeForViewport({ width: 390, height: 844 })
  expect(store.windows.value[0]!.snap).toBeNull()
  store.resizeForViewport({ width: 1600, height: 1000 })
  expect(store.windows.value[0]!.geometry).toEqual(geometry)
  expect(store.windows.value[0]!.snap).toBe('right')
})

it('phone geometry commits and new windows cannot establish phone-sized desktop preferences', () => {
  Object.defineProperty(window, 'innerWidth', { value: 390, configurable: true })
  Object.defineProperty(window, 'innerHeight', { value: 844, configurable: true })
  applyDesktopPreferences(desktop)
  const store = useDesktopMode()
  const id = store.openWindow('/files', 'route.files', true)
  store.updateGeometry(id, { left: 0, top: 0, width: 300, height: 400 })
  store.commitGeometry(id)
  const sent = desktopPreferencesSnapshot()
  expect(sent.windows[0]!.geometry.width).toBeGreaterThan(760)
  expect(sent.windowSizes).toEqual([])
  // Local caches also preserve the preferred layout across refresh.
  resetDesktopModeForTest()
  initializeDesktopMode()
  expect(desktopPreferencesSnapshot().windows[0]!.geometry.width).toBeGreaterThan(760)
})

it('desktop resizing still updates the shared window and app size preference', () => {
  applyDesktopPreferences({ ...desktop, windows: [{ id: 1, path: '/files', geometry: { left: 100, top: 100, width: 900, height: 600 }, minimized: false, maximized: false, snap: '', z: 100 }] })
  useDesktopMode().updateGeometry(1, { left: 120, top: 110, width: 1000, height: 650 })
  const sent = desktopPreferencesSnapshot()
  expect(sent.windows[0]!.geometry).toEqual({ left: 120, top: 110, width: 1000, height: 650 })
  expect(sent.windowSizes).toEqual([{ path: '/files', width: 1000, height: 650 }])
})

it('drag activation followed immediately by commit saves the detached layout', () => {
  applyDesktopPreferences({ ...desktop, windows: [{ id: 1, path: '/files', geometry: { left: 100, top: 100, width: 900, height: 600 }, minimized: false, maximized: false, snap: 'left', z: 100 }] })
  const store = useDesktopMode()
  store.restoreWindowForDrag(1, { left: 200, top: 90, width: 900, height: 600 })
  store.commitGeometry(1)
  expect(desktopPreferencesSnapshot().windows[0]).toMatchObject({ snap: '', geometry: { left: 200, top: 90, width: 900, height: 600 } })
})

it('cancelled divider motion cannot replace the shared ratio with a viewport-clamped value', () => {
  Object.defineProperty(window, 'innerWidth', { value: 800, configurable: true })
  applyDesktopPreferences({ ...desktop, sideSplitRatio: .8 })
  const store = useDesktopMode()
  const fittedStartRatio = store.sideSplitRatio.value
  expect(fittedStartRatio).not.toBe(.8)
  store.setSideSplitRatio(.5, false)
  store.setSideSplitRatio(fittedStartRatio, false)
  expect(desktopPreferencesSnapshot().sideSplitRatio).toBe(.8)
  store.setSideSplitRatio(.5, false)
  store.commitSideSplitRatio()
  expect(desktopPreferencesSnapshot().sideSplitRatio).toBe(.5)
})
