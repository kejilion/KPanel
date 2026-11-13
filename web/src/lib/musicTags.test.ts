import { afterEach, expect, it, vi } from 'vitest'
import { MusicVersionMismatchError, readMusicTags } from './musicTags'

afterEach(() => vi.unstubAllGlobals())

it('binds tag reads to the listed file version and rejects a replacement', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(new Uint8Array(), {
    status: 206, headers: { 'Content-Range': 'bytes 0-0/1', ETag: '"version-2"' },
  }))
  vi.stubGlobal('fetch', fetcher)
  await expect(readMusicTags('/music/song.mp3', new AbortController().signal, 'version-1')).rejects.toBeInstanceOf(MusicVersionMismatchError)
  expect(fetcher.mock.calls[0]?.[1].headers.get('Range')).toBe('bytes=0-' + (2 * 1024 * 1024 - 1))
  expect(fetcher.mock.calls[0]?.[1].cache).toBe('no-store')
})

it('keeps ordinary HTTP failures distinct from a file version mismatch', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 500 })))
  await expect(readMusicTags('/music/song.mp3', new AbortController().signal, 'version-1')).rejects.toThrow('Music tags unavailable')
  await expect(readMusicTags('/music/song.mp3', new AbortController().signal, 'version-1')).rejects.not.toBeInstanceOf(MusicVersionMismatchError)
})
