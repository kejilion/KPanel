// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { FileEntry, MusicPlaybackPreference, UserPreferences } from '@/types/api'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), entry: vi.fn(), entries: vi.fn(), text: vi.fn(), danger: vi.fn(), play: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { preferences: { get: mocks.get, update: mocks.update } },
  ApiError: class extends Error { constructor(message: string, public status: number) { super(message) } },
}))
vi.mock('@/lib/fileHostContext', () => ({ fileAPIForHost: () => ({ entry: mocks.entry, entries: mocks.entries, contentUrl: (path: string) => path, text: mocks.text }) }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ danger: mocks.danger }) }))
import { startPreferencesSync, stopPreferencesSync } from './preferencesSync'
import { mediaLocations, setMediaLocation } from './mediaLocations'
import { useMusicPlayer } from '@/stores/musicPlayer'
import { musicQueueTrack } from './music'

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
  vi.stubGlobal('fetch', vi.fn(async (url: string) => url.endsWith('.lrc') ? new Response(await mocks.text()) : { ok: false }))
  mocks.get.mockResolvedValue(remote())
  mocks.update.mockImplementation(async (patch) => ({ ...remote(), ...patch, resourceVersion: 'v2' }))
  mocks.entry.mockResolvedValue({ ...entry })
  mocks.entries.mockResolvedValue({ entries: [], unavailable: [] }); mocks.text.mockResolvedValue('')
})

it('restores saved local lyrics and artwork before attempting online metadata', async () => {
  mocks.entries.mockResolvedValue({ entries: ['Song.lrc', 'Song.png'].map(name => ({ ...entry, name, path: '/music/' + name })), unavailable: [] })
  mocks.text.mockResolvedValue('[00:00]Saved local lyrics')
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const state = useMusicPlayer().state
  expect(state.current).toMatchObject({ hostId: 'edge', lyrics: '/music/Song.lrc', cover: '/music/Song.png' })
  expect(state.lyrics).toBe('[00:00]Saved local lyrics'); expect(state.onlineStatus).toBe('')
  expect(state.restored).toBe(true); expect(mocks.play).not.toHaveBeenCalled()
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

it('saves music location and playback changes together without losing either section', async () => {
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, playing: false }, musicLocation: { path: '/old', hostId: '' } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  expect(useMusicPlayer().state.current?.entry.path).toBe(entry.path)
  setMediaLocation('musicLocation', '/music', 'edge')
  useMusicPlayer().toggle()
  await vi.advanceTimersByTimeAsync(500)
  expect(mediaLocations.musicLocation).toEqual({ path: '/music', hostId: 'edge' })
  expect(mocks.update).toHaveBeenCalledWith(expect.objectContaining({
    musicLocation: { path: '/music', hostId: 'edge' },
    musicPlayback: expect.objectContaining({ path: entry.path, position: 42, playing: true }),
    expectedResourceVersion: 'v1',
  }), expect.any(AbortSignal))
  expect(sessionStorage.getItem('kpanel:preferences-pending:v1')).toBeNull()
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
  mocks.entry.mockResolvedValue({ ...entry })
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current?.entry.path).toBe(entry.path)
  expect(mocks.play).not.toHaveBeenCalled()
})

it('saves an explicit seek without waiting for the progress interval', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const music = useMusicPlayer()
  music.state.duration = 100
  music.seek(20)
  await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback.position).toBe(20)
})

it('retries a newer checkpoint after its file was temporarily unavailable', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(0)
  const newer = { ...entry, path: '/music/newer.mp3', name: 'newer.mp3' }
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'v2', musicPlayback: { ...saved, path: newer.path, position: 80 } })
  mocks.entry.mockRejectedValueOnce(new Error('offline'))
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current?.entry.path).toBe(entry.path)
  mocks.entry.mockResolvedValue(newer)
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current?.entry.path).toBe(newer.path)
})

it('preserves a queue edited while a remote checkpoint is loading', async () => {
  let finish!: (value: FileEntry) => void
  mocks.entry.mockImplementationOnce(() => new Promise<FileEntry>(resolve => { finish = resolve }))
  await startPreferencesSync('admin')
  const queued = { entry: { ...entry, path: '/music/queued.mp3', name: 'queued.mp3' }, hostId: '', title: 'queued', artist: '' }
  useMusicPlayer().enqueue(queued)
  finish(entry); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.queue.map(t => t.entry.path)).toEqual([queued.entry.path])
  expect(useMusicPlayer().state.current).toBeUndefined()
})

it('restores an ordered cross-host queue without eagerly probing offline songs or autoplaying', async () => {
  const queue = [{ path: '/music/offline.mp3', hostId: 'offline' }, { path: entry.path, hostId: 'edge' }, { path: entry.path, hostId: '' }]
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, queue } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  const state = useMusicPlayer().state
  expect(state.queue.map(track => ({ path: track.entry.path, hostId: track.hostId }))).toEqual(queue)
  expect(state.current?.hostId).toBe('edge'); expect(state.position).toBe(42)
  expect(mocks.entry).toHaveBeenCalledTimes(1)
  expect(mocks.play).not.toHaveBeenCalled(); expect(mocks.update).not.toHaveBeenCalled()
})

it('saves queue-only edits, reordering and clearing while keeping the current song independent', async () => {
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  const music = useMusicPlayer(), extra = musicQueueTrack('/music/extra.mp3', 'other')
  music.enqueue(extra); await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback.queue).toEqual([{ path: entry.path, hostId: 'edge' }, { path: extra.entry.path, hostId: 'other' }])
  music.move(extra, -1); await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback.queue[0].path).toBe(extra.entry.path)
  music.clearQueue(); music.remove(music.state.current!); await vi.advanceTimersByTimeAsync(500)
  expect(mocks.update.mock.calls.at(-1)?.[0].musicPlayback).toMatchObject({ path: entry.path, queue: [] })
})

it('restores an empty queue independently of a current song, and a queue with no selected song', async () => {
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, queue: [] } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.queue).toEqual([]); expect(useMusicPlayer().state.current?.entry.path).toBe(entry.path)
  stopPreferencesSync(); useMusicPlayer().stop()
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, path: '', hostId: '', playing: false, position: 0, queue: [{ path: entry.path, hostId: 'edge' }] } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current).toBeUndefined(); expect(useMusicPlayer().state.queue).toHaveLength(1)
  expect(mocks.play).not.toHaveBeenCalled()
})

it('preserves unavailable queue references and refreshes a passive remote reorder on focus', async () => {
  const queue = [{ path: entry.path, hostId: 'edge' }, { path: '/music/later.mp3', hostId: 'offline' }]
  mocks.entry.mockRejectedValue(new Error('offline'))
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, queue } })
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.queue).toHaveLength(2); expect(mocks.update).not.toHaveBeenCalled()
  mocks.get.mockResolvedValue({ ...remote(), resourceVersion: 'v2', musicPlayback: { ...saved, queue: [...queue].reverse() } })
  window.dispatchEvent(new Event('focus')); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.queue[0]?.hostId).toBe('offline'); expect(mocks.play).not.toHaveBeenCalled()
})

it('keeps a pending queue larger than the old64KB storage limit and rejects invalid remote references', async () => {
  const queue = Array.from({ length: 800 }, (_, i) => ({ path: '/music/' + 'long-name-'.repeat(9) + i + '.mp3', hostId: 'edge' }))
  sessionStorage.setItem('kpanel:preferences-pending:v1', JSON.stringify({ owner: 'admin', patch: { musicPlayback: { ...saved, queue } } }))
  await startPreferencesSync('admin'); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.queue).toHaveLength(800)
  stopPreferencesSync(); useMusicPlayer().stop()
  mocks.get.mockResolvedValue({ ...remote(), musicPlayback: { ...saved, queue: [{ path: '/../secret', hostId: '' }] } })
  await startPreferencesSync('other'); await vi.advanceTimersByTimeAsync(1)
  expect(useMusicPlayer().state.current).toBeUndefined(); expect(useMusicPlayer().state.queue).toEqual([])
})
