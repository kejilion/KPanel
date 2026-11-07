// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { CinemaCheckpoint, UserPreferences } from '@/types/api'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), danger: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { preferences: { get: mocks.get, update: mocks.update } }, ApiError: class extends Error { constructor(message: string, public status: number) { super(message) } } }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ danger: mocks.danger }) }))
vi.mock('@/stores/musicPlayer', () => ({ useMusicPlayer: () => ({ state: { queue: [], playing: false, restored: false }, stop: vi.fn() }) }))
import { ApiError } from './api'
import { startPreferencesSync, stopPreferencesSync } from './preferencesSync'
import { cinemaPlayback, mergeCinemaPlayback, recordCinemaCheckpoint } from './cinemaPlayback'

const point = (path = '/films/a.mp4', position = 42): CinemaCheckpoint => ({ path, hostId: 'edge', fileVersion: 'v1', position, duration: 120, completed: false })
let server: UserPreferences
beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks(); localStorage.clear(); sessionStorage.clear()
  server = { resourceVersion: 'v1', locale: 'zh-CN', desktop: { mode: 'classic', windows: [], windowSizes: [], sideSplitRatio: .5, focusedId: 0 }, cinemaPlayback: { entries: [point()] } }
  mocks.get.mockImplementation(async () => structuredClone(server))
  mocks.update.mockImplementation(async patch => {
    server = { ...server, ...patch, cinemaPlayback: mergeCinemaPlayback(server.cinemaPlayback, patch.cinemaPlayback), resourceVersion: 'v2' }
    return structuredClone(server)
  })
})
afterEach(() => { stopPreferencesSync(); vi.useRealTimers() })

it('restores remote progress without starting a player and refreshes on focus', async () => {
  await startPreferencesSync('admin')
  expect(cinemaPlayback.entries[0]?.position).toBe(42)
  expect(mocks.update).not.toHaveBeenCalled()
  server.cinemaPlayback = { entries: [point('/films/a.mp4', 80)] }
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(cinemaPlayback.entries[0]?.position).toBe(80)
})
it('coalesces different films, flushes on exit, and preserves unrelated music settings', async () => {
  await startPreferencesSync('admin')
  server.musicPlayback = { path: '/music/song.mp3', hostId: '', position: 11, playing: false, repeat: 'off', shuffle: false }
  recordCinemaCheckpoint(point('/films/a.mp4', 50), 'admin')
  recordCinemaCheckpoint(point('/films/b.mp4', 15), 'admin', true)
  await vi.advanceTimersByTimeAsync(1)
  expect(server.cinemaPlayback?.entries.map(item => item.path)).toEqual(['/films/b.mp4', '/films/a.mp4'])
  expect(server.musicPlayback?.position).toBe(11)
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
})
it('retries a conflict with only edited films so another device film survives', async () => {
  await startPreferencesSync('admin')
  server = { ...server, resourceVersion: 'remote-v2', cinemaPlayback: { entries: [point('/films/remote.mp4', 70), point()] } }
  mocks.update.mockRejectedValueOnce(new ApiError('conflict', 409))
  recordCinemaCheckpoint(point('/films/a.mp4', 12), 'admin', true)
  await vi.advanceTimersByTimeAsync(1)
  expect(mocks.update.mock.calls.at(-1)?.[0].cinemaPlayback.entries).toHaveLength(1)
  expect(server.cinemaPlayback?.entries.map(item => item.position)).toEqual([12, 70])
})
it('keeps failed writes through reload and does not leak them to another owner', async () => {
  await startPreferencesSync('admin')
  mocks.update.mockRejectedValue(new Error('offline'))
  recordCinemaCheckpoint(point('/films/a.mp4', 88), 'admin', true)
  await vi.advanceTimersByTimeAsync(1)
  stopPreferencesSync()
  expect(cinemaPlayback.entries).toHaveLength(0)
  await startPreferencesSync('other')
  expect(cinemaPlayback.entries[0]?.position).toBe(42)
  stopPreferencesSync()
  mocks.update.mockImplementation(async patch => ({ ...server, cinemaPlayback: mergeCinemaPlayback(server.cinemaPlayback, patch.cinemaPlayback), resourceVersion: 'v3' }))
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  expect(cinemaPlayback.entries[0]?.position).toBe(88)
})

it('acknowledges each film separately when another local edit arrives during a batch write', async () => {
  await startPreferencesSync('admin')
  let finish!: (value: UserPreferences) => void
  const normal = mocks.update.getMockImplementation()!
  mocks.update.mockImplementationOnce(() => new Promise<UserPreferences>(resolve => { finish = resolve }))
    .mockImplementationOnce(async () => {
      server = { ...server, resourceVersion: 'remote-v3', cinemaPlayback: { entries: [point('/films/b.mp4', 90), point('/films/a.mp4', 10)] } }
      throw new ApiError('conflict', 409)
    }).mockImplementation(normal)
  recordCinemaCheckpoint(point('/films/a.mp4', 10), 'admin')
  recordCinemaCheckpoint(point('/films/b.mp4', 10), 'admin', true)
  recordCinemaCheckpoint(point('/films/a.mp4', 20), 'admin')
  finish({ ...server, resourceVersion: 'saved-v2', cinemaPlayback: { entries: [point('/films/b.mp4', 10), point('/films/a.mp4', 10)] } })
  await vi.advanceTimersByTimeAsync(1)
  expect(mocks.update.mock.calls.at(-1)?.[0].cinemaPlayback.entries).toEqual([point('/films/a.mp4', 20)])
  expect(server.cinemaPlayback?.entries).toContainEqual(point('/films/b.mp4', 90))
  expect(server.cinemaPlayback?.entries).toContainEqual(point('/films/a.mp4', 20))
})
