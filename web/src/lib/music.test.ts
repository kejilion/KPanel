import { describe, expect, it, vi } from 'vitest'
import { parseLyrics, tracksFromEntries, trackKey, readMusicFolder } from './music'
import { parseMusicTags } from './musicTags'
import type { FileEntry } from '@/types/api'
const list = vi.hoisted(() => vi.fn())
vi.mock('./fileHostContext', () => ({ fileAPIForHost: (host: string) => ({ list: (...args: unknown[]) => list(host, ...args) }) }))
function file(name: string, sizeBytes = 100): FileEntry {
  return { name, path: `/music/${name}`, kind: 'file', sizeBytes, mime: '', mode: '', owner: '', group: '', modifiedAt: '', resourceVersion: 'v1', editable: false, previewable: true }
}
describe('music library', () => {
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
  it('parses repeated timestamps, decimal precision and sorting without HTML', () => {
    expect(parseLyrics('[00:02.20][00:01.005]<b>Hi</b>\n[99:99]invalid')).toEqual([{ time: 1.005, text: '<b>Hi</b>' }, { time: 2.2, text: '<b>Hi</b>' }])
    expect(parseLyrics('ordinary lyrics')).toEqual([])
  })
})
describe('bounded embedded metadata', () => {
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
