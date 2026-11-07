import { afterEach, describe, expect, it, vi } from 'vitest'
import { parseLyrics, tracksFromEntries, trackKey, readMusicFolder, readMusicTrack, readMusicLyrics } from './music'
import { parseMusicTags } from './musicTags'
import type { FileEntry } from '@/types/api'
const list = vi.hoisted(() => vi.fn())
const entries = vi.hoisted(() => vi.fn())
vi.mock('./fileHostContext', () => ({ fileAPIForHost: (host: string) => ({ list: (...args: unknown[]) => list(host, ...args), entries: (...args: unknown[]) => entries(host, ...args), contentUrl: (path: string) => 'https://fixture.test/' + host + path }) }))
afterEach(() => vi.unstubAllGlobals())
function file(name: string, sizeBytes = 100): FileEntry {
  return { name, path: `/music/${name}`, kind: 'file', sizeBytes, mime: '', mode: '', owner: '', group: '', modifiedAt: '', resourceVersion: 'v1', editable: false, previewable: true }
}
describe('music library', () => {
  it('reads UTF-8 lyrics from the original host media endpoint without requiring editor support', async () => {
    const fetcher = vi.fn(async () => new Response('[00:01]保存的歌词'))
    vi.stubGlobal('fetch', fetcher)
    const track = { entry: file('Song.mp3'), hostId: 'edge', title: 'Song', artist: '', lyrics: '/music/Song.lrc' }
    expect(await readMusicLyrics(track, new AbortController().signal)).toBe('[00:01]保存的歌词')
    expect(fetcher).toHaveBeenCalledWith('https://fixture.test/edge/music/Song.lrc', expect.objectContaining({ headers: { Range: 'bytes=0-262144' } }))
  })
  it('bounds raw lyric reads, rejects binary or invalid UTF-8, and cancels the body on failure', async () => {
    const track = { entry: file('Song.mp3'), hostId: '', title: 'Song', artist: '', lyrics: '/music/Song.lrc' }
    const cancelled = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new ReadableStream({ start(controller) { controller.enqueue(new Uint8Array(262145)) }, cancel: cancelled }))))
    await expect(readMusicLyrics(track, new AbortController().signal)).rejects.toThrow('limit'); expect(cancelled).toHaveBeenCalledOnce()
    for (const bytes of [new Uint8Array([0]), new Uint8Array([255])]) {
      vi.stubGlobal('fetch', vi.fn(async () => new Response(bytes)))
      await expect(readMusicLyrics(track, new AbortController().signal)).rejects.toThrow()
    }
    const controller = new AbortController(); controller.abort()
    await expect(readMusicLyrics(track, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
  })
  it('restores same-directory sidecars with one bounded host-bound lookup and existing size limits', async () => {
    entries.mockResolvedValue({ entries: [file('Song.lrc'), file('Song.png'), file('cover.jpg', 9_000_000), { ...file('Song.jpg'), path: '/other/Song.jpg' }] })
    const signal = new AbortController().signal
    expect(await readMusicTrack(file('Song.mp3'), 'edge', signal)).toMatchObject({ hostId: 'edge', lyrics: '/music/Song.lrc', cover: '/music/Song.png' })
    expect(entries).toHaveBeenLastCalledWith('edge', expect.arrayContaining(['/music/Song.lrc', '/music/Song.png']), signal)
    expect(entries.mock.calls.at(-1)?.[1]).toHaveLength(17)
    entries.mockResolvedValue({ entries: [file('Song.lrc', 300_000), file('Song.png', 9_000_000)] })
    expect(await readMusicTrack(file('Song.mp3'), 'edge', signal)).toMatchObject({ lyrics: undefined, cover: undefined })
  })
  it('keeps audio when optional sidecar lookup fails and propagates cancellation', async () => {
    entries.mockRejectedValue(Error('offline'))
    expect((await readMusicTrack(file('Song.mp3'), '', new AbortController().signal))?.entry.path).toBe('/music/Song.mp3')
    const controller = new AbortController(); controller.abort()
    await expect(readMusicTrack(file('Song.mp3'), '', controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
  })
  it('keeps identical remote paths distinct and resolves bounded sidecars', () => {
    const entries = [file('Artist - Song.FLAC'), file('Artist - Song.lrc'), file('cover.jpg'), file('ignore.txt')]
    const [local] = tracksFromEntries(entries, '')
    const [remote] = tracksFromEntries(entries, 'edge')
    expect(local).toMatchObject({ title: 'Song', artist: 'Artist', cover: '/music/cover.jpg', lyrics: '/music/Artist - Song.lrc' })
    expect(trackKey(local!)).not.toBe(trackKey(remote!))
    expect(tracksFromEntries([file('song.wav'), file('cover.jpg', 9000000)], '')[0]?.cover).toBeUndefined()
  })
  it('bounds paginated scans and binds each request to the selected host', async () => {
    list.mockImplementation(async (_host, _path, options) => ({ entries: Array.from({ length: 500 }, (_, i) => file(`${i}.mp3`)), nextOffset: options.offset + 500 }))
    const result = await readMusicFolder('/music', 'edge', new AbortController().signal)
    expect(result.truncated).toBe(true); expect(result.entries).toHaveLength(2000)
    expect(list.mock.calls.every(call => call[0] === 'edge')).toBe(true)
    expect(list).toHaveBeenCalledTimes(4)
  })
  it('prefers a song-specific cover over a generic folder cover regardless of entry order', () => {
    const entries = [file('cover.jpg'), file('folder.png'), file('Song.webp'), file('Song.flac')]
    expect(tracksFromEntries(entries, '')[0]?.cover).toBe('/music/Song.webp')
    expect(tracksFromEntries(entries.reverse(), '')[0]?.cover).toBe('/music/Song.webp')
  })
  it('parses repeated timestamps, decimal precision and sorting without HTML', () => {
    expect(parseLyrics('[00:02.20][00:01.005]<b>Hi</b>\n[99:99]invalid')).toEqual([{ time: 1.005, text: '<b>Hi</b>' }, { time: 2.2, text: '<b>Hi</b>' }])
    expect(parseLyrics('ordinary lyrics')).toEqual([])
  })
})
describe('bounded embedded metadata', () => {
  function id3(id: string, content: number[], version = 3): Uint8Array {
    const length = content.length
    const bytes = new Uint8Array(20 + length)
    bytes.set([73, 68, 51, version, 0, 0, 0, 0, 0, 10 + length])
    bytes.set([...new TextEncoder().encode(id), 0, 0, 0, length, 0, 0], 10)
    bytes.set(content, 20)
    return bytes
  }
  it('reads common GBK and mislabeled UTF-8 Chinese tags while keeping Western text', () => {
    const gbk = [0xb7, 0xe7, 0xb8, 0xe8, 0xc7, 0xe0, 0xcc, 0xec]
    expect(parseMusicTags(id3('TIT2', [0, ...gbk])).title).toBe('风歌青天')
    expect(parseMusicTags(id3('TPE1', [0, 0xd6, 0xdc, 0xbd, 0xdc, 0xc2, 0xd7], 4)).artist).toBe('周杰伦')
    expect(parseMusicTags(id3('TIT2', [0, ...new TextEncoder().encode('风歌青天')])).title).toBe('风歌青天')
    expect(parseMusicTags(id3('TPE1', [0, ...[0x42, 0x6a, 0xf6, 0x72, 0x6b]])).artist).toBe('Björk')
    expect(parseMusicTags(id3('TIT2', [0, 0x4e, 0x69, 0xf1, 0x6f, 0x20, 0x50, 0x65, 0xf1, 0x6f])).title).toBe('Niño Peño')
    expect(parseMusicTags(id3('TIT2', [3, ...new TextEncoder().encode('你好')])).title).toBe('你好')
    expect(parseMusicTags(id3('TIT2', [1, 0xff, 0xfe, 0x60, 0x4f, 0x7d, 0x59])).title).toBe('你好')
  })
  it('reads ID3 text and ignores truncated frames', () => {
    const text = new TextEncoder().encode('\x03Song')
    const bytes = new Uint8Array(20 + text.length)
    bytes.set([73, 68, 51, 3, 0, 0, 0, 0, 0, 10 + text.length]); bytes.set([84, 73, 84, 50, 0, 0, 0, text.length, 0, 0], 10); bytes.set(text, 20)
    expect(parseMusicTags(bytes).title).toBe('Song')
    expect(parseMusicTags(bytes.subarray(0, 22)).title).toBeUndefined()
  })
  it('does not throw for short or malformed prefixes', () => {
    for (const value of ['', 'ID3', 'fLaC', 'fLaC\x86\xff\xff\xff', 'ID3\x03\0\0\x7f\x7f\x7f\x7f']) expect(() => parseMusicTags(new TextEncoder().encode(value))).not.toThrow()
  })
})
