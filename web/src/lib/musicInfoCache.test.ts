import { afterEach, expect, it, vi } from 'vitest'
import { api } from './api'
import { musicQueueTrack, type Track } from './music'
import { readMusicTags } from './musicTags'
import { readMusicQuality } from './musicQuality'
import { clearMusicInfoMemory, invalidateMusicInfo, loadMusicInfo, lookupMusicInfo, peekMusicInfo, saveMusicInfo, type MusicRecord } from './musicInfoCache'

vi.mock('./api', () => ({ api: { music: { cache: vi.fn() }, files: { entry: vi.fn(), contentUrl: (path: string) => path } } }))
vi.mock('./musicTags', () => ({ readMusicTags: vi.fn() }))
vi.mock('./musicQuality', async original => ({ ...await original<typeof import('./musicQuality')>(), readMusicQuality: vi.fn() }))
const persisted = new Map<string, MusicRecord>()
const track = (version = 'v1', hostId = ''): Track => ({ ...musicQueueTrack('/music/filename.mp3', hostId), entry: { ...musicQueueTrack('/music/filename.mp3', hostId).entry, resourceVersion: version, sizeBytes: 1000 } })
const signal = () => new AbortController().signal
function setup() {
  vi.mocked(api.music.cache).mockImplementation(async input => {
    if (input.observation) {
      const { ref, info } = input.observation, key = JSON.stringify(ref)
      persisted.set(key, { ref, info: { ...persisted.get(key)?.info, ...info }, schema: 1, source: 'file-header', detectedAt: new Date().toISOString() })
    }
    return { items: (input.refs ?? []).flatMap(ref => persisted.has(JSON.stringify(ref)) ? [persisted.get(JSON.stringify(ref))!] : []) }
  })
  vi.mocked(readMusicTags).mockResolvedValue({ title: '标签中的歌名', artist: '标签歌手', album: '标签专辑' })
  vi.mocked(readMusicQuality).mockResolvedValue({ bitrate: 256, bitrateKind: 'average', vbr: true })
}
afterEach(() => { clearMusicInfoMemory(); persisted.clear(); vi.resetAllMocks() })
it('reuses persisted tags and quality after a new client session; changed file/host invalidates', async () => {
  setup(); const first = await loadMusicInfo(track(), signal())
  expect(first.tags?.title).toBe('标签中的歌名'); expect(first.quality?.bitrate).toBe(256)
  expect(first.embeddedAssets).toBe(false)
  clearMusicInfoMemory()
  expect(await loadMusicInfo(track(), signal())).toEqual(first)
  expect(readMusicTags).toHaveBeenCalledOnce(); expect(readMusicQuality).toHaveBeenCalledOnce()
  await loadMusicInfo(track('v2'), signal()); await loadMusicInfo(track('v1', 'node-b'), signal())
  expect(readMusicTags).toHaveBeenCalledTimes(3); expect(readMusicQuality).toHaveBeenCalledTimes(3)
})
it('does not turn a transient read failure into a persistent negative result; merges partial success', async () => {
  setup(); vi.mocked(readMusicTags).mockRejectedValueOnce(new Error('offline'))
  const partial = await loadMusicInfo(track(), signal())
  expect(partial.tags).toBeUndefined(); expect(partial.quality?.bitrate).toBe(256)
  clearMusicInfoMemory(); await loadMusicInfo(track(), signal())
  expect(readMusicTags).toHaveBeenCalledTimes(2); expect(readMusicQuality).toHaveBeenCalledOnce()
  expect(peekMusicInfo(track())?.tags?.artist).toBe('标签歌手')
})
it('keeps embedded media out of JSON and resolves the real version of restored references', async () => {
  setup(); const restored = musicQueueTrack('/music/filename.mp3', 'node-b')
  vi.mocked(api.files.entry).mockResolvedValue(track('real').entry)
  vi.mocked(readMusicTags).mockResolvedValue({ title: '内嵌', lyrics: 'lyrics', picture: new Blob(['picture']) })
  const info = await loadMusicInfo(restored, signal())
  expect(api.files.entry).toHaveBeenCalledOnce(); expect(restored.entry.resourceVersion).toBe('real')
  expect(info).toMatchObject({ tags: { title: '内嵌' }, embeddedAssets: true })
  const serialized = JSON.stringify([...persisted.values()]); expect(serialized).not.toContain('picture'); expect(serialized).not.toContain('lyrics')
})
it('falls back when cache storage is unavailable and discards in-flight results after logout', async () => {
  setup(); vi.mocked(api.music.cache).mockRejectedValue(new Error('cache unavailable'))
  expect((await loadMusicInfo(track(), signal())).quality?.bitrate).toBe(256)
  clearMusicInfoMemory()
  let resolve!: (value: {}) => void
  vi.mocked(readMusicTags).mockImplementation(() => new Promise(r => { resolve = r }))
  const loading = loadMusicInfo(track('v3'), signal())
  await vi.waitFor(() => expect(resolve).toBeTypeOf('function'))
  clearMusicInfoMemory(); resolve({ title: 'stale' }); await loading
  expect(peekMusicInfo(track('v3'))).toBeUndefined()
})
it('does not reload an invalidated file version from persistent cache during the current session', async () => {
  setup(); const old = track('v2')
  await loadMusicInfo(old, signal())
  invalidateMusicInfo(old); vi.mocked(api.music.cache).mockClear()
  await saveMusicInfo(old, { tags: { title: 'stale write' } }, signal())
  await lookupMusicInfo([old], signal())
  expect(api.music.cache).not.toHaveBeenCalled()
  expect(peekMusicInfo(old)).toBeUndefined()
})

it('repairs legacy empty quality without rereading cached tags', async () => {
  setup(); const item = track()
  const ref = { hostId: item.hostId, path: item.entry.path, resourceVersion: item.entry.resourceVersion, sizeBytes: item.entry.sizeBytes }
  persisted.set(JSON.stringify(ref), { ref, info: { tags: { title: 'old' }, quality: {} }, schema: 1, source: 'file-header', detectedAt: '' })
  expect((await loadMusicInfo(item, signal())).quality?.bitrate).toBe(256)
  expect(readMusicQuality).toHaveBeenCalledOnce(); expect(readMusicTags).not.toHaveBeenCalled()
})

it('does not persist an empty detection as a completed quality result', async () => {
  setup(); vi.mocked(readMusicQuality).mockResolvedValueOnce({}).mockResolvedValueOnce({ lossless: true })
  expect((await loadMusicInfo(track(), signal())).quality).toBeUndefined()
  expect([...persisted.values()][0]!.info.quality).toBeUndefined()
  clearMusicInfoMemory()
  expect((await loadMusicInfo(track(), signal())).quality?.lossless).toBe(true)
  expect(readMusicQuality).toHaveBeenCalledTimes(2)
})

it('publishes verified quality before a slow tag read finishes and keeps it if that read times out', async () => {
  setup(); let finish!: (value: {}) => void
  vi.mocked(readMusicTags).mockImplementation(() => new Promise(resolve => { finish = resolve }))
  const controller = new AbortController(), loading = loadMusicInfo(track(), controller.signal)
  await vi.waitFor(() => expect(peekMusicInfo(track())?.quality?.bitrate).toBe(256))
  controller.abort(); finish({}); await loading
  expect(peekMusicInfo(track())?.quality?.bitrate).toBe(256)
})

it('does not publish a late quality observation for an invalidated file version', async () => {
  setup(); let finish!: (value: {}) => void
  vi.mocked(readMusicQuality).mockImplementation(() => new Promise(resolve => { finish = resolve }))
  const item = track(), loading = loadMusicInfo(item, signal())
  await vi.waitFor(() => expect(finish).toBeTypeOf('function'))
  invalidateMusicInfo(item); finish({ lossless: true }); await loading
  expect(peekMusicInfo(item)).toBeUndefined(); expect(persisted.size).toBe(0)
})
