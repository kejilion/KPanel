import { beforeEach, expect, it, vi } from 'vitest'
import { saveMusicSidecars } from './musicSidecars'
import type { Track } from './music'
const harness = vi.hoisted(() => ({ entry: vi.fn(), upload: vi.fn(), files: new Map<string, File>() }))
vi.mock('./fileHostContext', () => ({ fileAPIForHost: (host: string) => ({
  entry: (path: string, signal: AbortSignal) => harness.entry(host, path, signal),
  upload: (directory: string, file: File, overwrite: boolean, progress: unknown, signal: AbortSignal) => harness.upload(host, directory, file, overwrite, progress, signal),
}) }))
const track: Track = { hostId: 'node-a', title: 'Parsed title', artist: 'Artist', entry: { name: '原名.mp3', path: '/music/原名.mp3', kind: 'file', sizeBytes: 1000, resourceVersion: 'v1', mode: '', owner: '', group: '', modifiedAt: '', editable: false, previewable: true } }
const png = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='
const fail = (status: number) => Object.assign(new Error('fixture failure'), { status })
const signal = () => new AbortController().signal
beforeEach(() => {
  harness.files.clear()
  harness.entry.mockReset().mockImplementation(async (host: string, path: string) => {
    const file = harness.files.get(host + path)
    if (!file) throw fail(404)
    return { kind: 'file', path, sizeBytes: file.size }
  })
  harness.upload.mockReset().mockImplementation(async (host: string, directory: string, file: File, overwrite: boolean) => {
    expect(overwrite).toBe(false)
    const path = `${directory === '/' ? '' : directory}/${file.name}`
    if (harness.files.has(host + path)) throw fail(409)
    harness.files.set(host + path, file)
    return { kind: 'file', path, sizeBytes: file.size }
  })
})
it('saves UTF-8 lyrics and real PNG bytes beside the original file on the captured host', async () => {
  const result = await saveMusicSidecars(track, { lyrics: '[00:01]中文歌词', cover: png }, signal())
  expect(result).toEqual({ lyrics: '/music/原名.lrc', cover: '/music/原名.png', saved: 2, kept: 0, failed: 0 })
  expect(await harness.files.get('node-a/music/原名.lrc')!.text()).toBe('[00:01]中文歌词')
  expect(new Uint8Array(await harness.files.get('node-a/music/原名.png')!.arrayBuffer()).slice(0, 8)).toEqual(new Uint8Array([137,80,78,71,13,10,26,10]))
  expect(harness.upload.mock.calls.every(call => call[0] === 'node-a' && call[1] === '/music')).toBe(true)
})
it('preserves existing lyrics and a cover with a different extension', async () => {
  harness.files.set('node-a/music/原名.lrc', new File(['manual lyrics'], '原名.lrc'))
  harness.files.set('node-a/music/原名.jpg', new File(['manual cover'], '原名.jpg'))
  const result = await saveMusicSidecars(track, { lyrics: 'online', cover: png }, signal())
  expect(result).toMatchObject({ saved: 0, kept: 2, failed: 0, cover: '/music/原名.jpg' })
  expect(harness.upload).not.toHaveBeenCalled()
  expect(await harness.files.get('node-a/music/原名.lrc')!.text()).toBe('manual lyrics')
})
it('preserves oversized existing files without reattaching them as readable sidecars', async () => {
  harness.entry.mockImplementation(async (_host: string, path: string) => ({ kind: 'file', path, sizeBytes: 10 * 1024 ** 3 }))
  expect(await saveMusicSidecars(track, { lyrics: 'online', cover: png }, signal())).toEqual({ saved: 0, kept: 2, failed: 0 })
  expect(harness.upload).not.toHaveBeenCalled()
})
it('handles a concurrent creator without overwriting and does not hide unrelated conflicts', async () => {
  harness.upload.mockImplementationOnce(async (host, _directory, file) => {
    harness.files.set(host + '/music/' + file.name, new File(['other tab'], file.name))
    throw fail(409)
  })
  expect(await saveMusicSidecars(track, { lyrics: 'online' }, signal())).toMatchObject({ kept: 1, failed: 0 })
  harness.files.clear(); harness.upload.mockRejectedValueOnce(fail(409))
  expect(await saveMusicSidecars(track, { lyrics: 'online' }, signal())).toMatchObject({ saved: 0, kept: 0, failed: 1 })
})
it('retains successful work when the other upload fails and retries only missing files', async () => {
  harness.upload.mockRejectedValueOnce(fail(403))
  expect(await saveMusicSidecars(track, { lyrics: 'online', cover: png }, signal())).toMatchObject({ saved: 1, failed: 1, cover: '/music/原名.png' })
  expect(await saveMusicSidecars(track, { lyrics: 'online', cover: png }, signal())).toMatchObject({ saved: 1, kept: 1, failed: 0 })
})
it('rejects traversal, oversized text, non-image data, directories and unverified upload results', async () => {
  for (const path of ['/music/../x.mp3', 'relative.mp3', '/music\\x.mp3', '/music/\u0000x.mp3']) {
    await expect(saveMusicSidecars({ ...track, entry: { ...track.entry, path } }, { lyrics: 'x' }, signal())).rejects.toThrow()
  }
  expect(await saveMusicSidecars(track, { lyrics: '中'.repeat(50000), cover: 'data:text/html;base64,QQ==' }, signal())).toMatchObject({ failed: 2 })
  expect(harness.upload).not.toHaveBeenCalled()
  harness.entry.mockResolvedValueOnce({ path: '/music/原名.lrc', kind: 'directory' })
  expect(await saveMusicSidecars(track, { lyrics: 'x' }, signal())).toMatchObject({ failed: 1 })
  harness.upload.mockResolvedValueOnce({ kind: 'file', path: '/another/file', sizeBytes: 1 })
  expect(await saveMusicSidecars(track, { lyrics: 'x' }, signal())).toMatchObject({ failed: 1 })
})
it('cancels before writing and supports root-directory songs', async () => {
  const controller = new AbortController(); controller.abort()
  await expect(saveMusicSidecars(track, { lyrics: 'x' }, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
  expect(harness.upload).not.toHaveBeenCalled()
  const root = { ...track, hostId: '', entry: { ...track.entry, path: '/song.flac' } }
  expect(await saveMusicSidecars(root, { lyrics: 'x' }, signal())).toMatchObject({ lyrics: '/song.lrc', saved: 1 })
})
