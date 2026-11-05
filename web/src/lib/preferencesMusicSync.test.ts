// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { FileEntry, MusicPlaybackPreference, UserPreferences } from '@/types/api'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), entry: vi.fn(), danger: vi.fn(), play: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { preferences: { get: mocks.get, update: mocks.update } },
  ApiError: class extends Error { constructor(message: string, public status: number) { super(message) } },
}))
vi.mock('@/lib/fileHostContext', () => ({ fileAPIForHost: () => ({ entry: mocks.entry, contentUrl: (path: string) => path, text: vi.fn() }) }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ danger: mocks.danger }) }))
import { startPreferencesSync, stopPreferencesSync } from './preferencesSync'
import { useMusicPlayer } from '@/stores/musicPlayer'

const entry: FileEntry = { name: 'Song.mp3', path: '/music/Song.mp3', kind: 'file', sizeBytes: 100, mode: '', owner: '', group: '', modifiedAt: '', resourceVersion: 'v1', editable: false, previewable: true }
const saved: MusicPlaybackPreference = { path: entry.path, hostId: 'edge', position: 42, playing: true, repeat: 'all', shuffle: true }
const remote = (): UserPreferences => ({ resourceVersion: 'v1', locale: 'zh-CN', desktop: { mode: 'classic', windows: [], windowSizes: [], sideSplitRatio: .5, focusedId: 0 }, musicPlayback: { ...saved } })
class FakeAudio {
  src = ''; preload = ''; volume = 1; muted = false; currentTime = 0; duration = 100; error = null
  onplaying: (() => void) | null = null; onpause: (() => void) | null = null; onwaiting = null; oncanplay = null
  ontimeupdate = null; onloadedmetadata = null; onended = null; onerror = null
  play() { mocks.play(); this.onplaying?.(); return Promise.resolve() }
  pause() { this.onpause?.() }
  load() {} removeAttribute() { this.src = '' }
}

beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks(); localStorage.clear(); sessionStorage.clear()
  vi.stubGlobal('Audio', FakeAudio)
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false })))
  mocks.get.mockResolvedValue(remote())
  mocks.update.mockImplementation(async (patch) => ({ ...remote(), ...patch, resourceVersion: 'v2' }))
  mocks.entry.mockResolvedValue({ ...entry })
})
afterEach(() => { stopPreferencesSync(); useMusicPlayer().stop(); vi.useRealTimers(); vi.unstubAllGlobals() })

it('restores a saved track and progress without starting audio or overwriting the source state', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const state = useMusicPlayer().state
  expect(mocks.entry).toHaveBeenCalledWith(entry.path, expect.any(AbortSignal))
  expect(state.current?.entry.path).toBe(entry.path)
  expect(state.position).toBe(42)
  expect(state.restored).toBe(true)
  expect(state.playing).toBe(false)
  expect(mocks.play).not.toHaveBeenCalled()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('does not replace a song chosen while the saved file is loading', async () => {
  let finish!: (value: FileEntry) => void
  mocks.entry.mockImplementationOnce(() => new Promise<FileEntry>(resolve => { finish = resolve }))
  await startPreferencesSync('admin')
  useMusicPlayer().play({ entry: { ...entry, path: '/music/other.mp3', name: 'other.mp3' }, hostId: '', title: 'other', artist: '' })
  finish(entry); await vi.advanceTimersByTimeAsync(0)
  expect(useMusicPlayer().state.current?.entry.path).toBe('/music/other.mp3')
})

it('saves explicit changes and throttles ordinary progress updates', async () => {
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, playing: false } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const music = useMusicPlayer()
  music.toggle(); await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback).toMatchObject({ path: entry.path, playing: true, position: 42 })
  const count = mocks.update.mock.calls.length
  music.state.position = 43; await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update).toHaveBeenCalledTimes(count)
  await vi.advanceTimersByTimeAsync(30000)
  music.state.position = 44; await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback.position).toBe(44)
})

it('refreshes a passive checkpoint on focus without starting playback', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  expect(useMusicPlayer().state.restored).toBe(true)
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'v2', musicPlayback: { ...saved, position: 80 } })
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(mocks.get).toHaveBeenCalledTimes(2)
  expect(mocks.entry).toHaveBeenCalledTimes(2)
  expect(useMusicPlayer().state.position).toBe(80)
  expect(mocks.play).not.toHaveBeenCalled()
})

it('keeps the newest remote checkpoint when an earlier file lookup finishes later', async () => {
  let finish!: (value: FileEntry) => void
  mocks.entry.mockImplementationOnce(() => new Promise<FileEntry>(resolve => { finish = resolve }))
  await startPreferencesSync('admin')
  const newer = { ...entry, path: '/music/newer.mp3', name: 'newer.mp3' }
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'v2', musicPlayback: { ...saved, path: newer.path, position: 80 } })
  mocks.entry.mockResolvedValue(newer)
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  finish(entry); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current?.entry.path).toBe(newer.path)
  expect(useMusicPlayer().state.position).toBe(80)
  expect(mocks.update).not.toHaveBeenCalled()
})

it('ignores unavailable files and does not erase their saved checkpoint', async () => {
  mocks.entry.mockRejectedValue(new Error('host offline'))
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1000)
  expect(useMusicPlayer().state.current).toBeUndefined()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('saves an explicit seek without waiting for the progress interval', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const music = useMusicPlayer()
  music.state.duration = 100
  music.seek(20)
  await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback.position).toBe(20)
})
