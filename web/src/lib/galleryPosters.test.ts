// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  captureGalleryVideoFrame,
  galleryPosterKey,
  readGalleryPoster,
  resetGalleryPostersForTest,
  storeGalleryPoster,
  loadGalleryPoster,
} from './galleryPosters'

afterEach(() => { resetGalleryPostersForTest(); vi.restoreAllMocks() })

describe('gallery poster cache', () => {
  it('waits for an explicit seek and releases the decoder after capturing', async () => {
    const video = document.createElement('video')
    Object.defineProperties(video, {
      duration: { value: 12 }, videoWidth: { value: 640 }, videoHeight: { value: 360 },
    })
    vi.spyOn(video, 'load').mockImplementation(() => undefined)
    const pause = vi.spyOn(video, 'pause').mockImplementation(() => undefined)
    const original = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation(((name: string) => name === 'video' ? video : original(name)) as typeof document.createElement)
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage: vi.fn() } as unknown as CanvasRenderingContext2D)
    vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/jpeg;base64,frame')
    const pending = loadGalleryPoster('video', '/video.mp4', new AbortController().signal)
    await Promise.resolve()
    video.dispatchEvent(new Event('loadeddata'))
    expect(readGalleryPoster('video')).toBeUndefined()
    video.dispatchEvent(new Event('loadedmetadata'))
    expect(video.currentTime).toBe(1)
    expect(readGalleryPoster('video')).toBeUndefined()
    video.dispatchEvent(new Event('seeked'))
    await pending
    expect(readGalleryPoster('video')).toEqual({ poster: 'data:image/jpeg;base64,frame', duration: 12, width: 640, height: 360 })
    expect(video.getAttribute('src')).toBeNull()
    expect(pause).toHaveBeenCalled()
  })

  it('reads dimensions without seeking or capturing when artwork already exists and reuses metadata', async () => {
    const video = document.createElement('video')
    Object.defineProperties(video, { duration: { value: 12 }, videoWidth: { value: 3840 }, videoHeight: { value: 1608 } })
    vi.spyOn(video, 'load').mockImplementation(() => undefined)
    const pause = vi.spyOn(video, 'pause').mockImplementation(() => undefined)
    const original = document.createElement.bind(document)
    const create = vi.spyOn(document, 'createElement').mockImplementation(((name: string) => name === 'video' ? video : original(name)) as typeof document.createElement)
    const pending = loadGalleryPoster('artwork', '/film.mp4', new AbortController().signal, true)
    await Promise.resolve()
    expect(video.preload).toBe('metadata')
    video.dispatchEvent(new Event('loadedmetadata'))
    await pending
    expect(readGalleryPoster('artwork')).toEqual({ duration: 12, width: 3840, height: 1608 })
    expect(video.currentTime).toBe(0)
    expect(create).not.toHaveBeenCalledWith('canvas')
    expect(video.getAttribute('src')).toBeNull()
    expect(pause).toHaveBeenCalledOnce()
    await loadGalleryPoster('artwork', '/film.mp4', new AbortController().signal, true)
    expect(create).toHaveBeenCalledTimes(1)
  })

  it('cancels the decoder when navigation abandons a pending poster', async () => {
    const video = document.createElement('video')
    vi.spyOn(video, 'load').mockImplementation(() => undefined)
    vi.spyOn(video, 'pause').mockImplementation(() => undefined)
    const original = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation(((name: string) => name === 'video' ? video : original(name)) as typeof document.createElement)
    const controller = new AbortController()
    const pending = loadGalleryPoster('cancelled', '/video.mp4', controller.signal)
    const rejected = expect(pending).rejects.toBeDefined()
    await Promise.resolve()
    controller.abort()
    await rejected
    expect(video.getAttribute('src')).toBeNull()
    expect(readGalleryPoster('cancelled')).toBeUndefined()
  })

  it('evicts posters when their encoded memory exceeds the byte budget', () => {
    storeGalleryPoster('large-a', { poster: 'x'.repeat(4 * 1024 * 1024) })
    storeGalleryPoster('large-b', { poster: 'x'.repeat(4 * 1024 * 1024) })
    expect(readGalleryPoster('large-a')).toBeUndefined()
    expect(readGalleryPoster('large-b')?.poster).toBeDefined()
  })

  it('isolates the same path and version on two hosts', () => {
    const entry = { path: '/home/gallery/a.mp4', resourceVersion: 'sha256:1' }
    const local = galleryPosterKey(entry, '/api/v1/files/content?path=a.mp4')
    const remote = galleryPosterKey(entry, '/api/v1/files/content?path=a.mp4&hostId=edge-1')
    storeGalleryPoster(local, { poster: 'local-frame', duration: 7 })
    expect(readGalleryPoster(remote)).toBeUndefined()
    storeGalleryPoster(remote, { poster: 'remote-frame', duration: 9 })
    expect(readGalleryPoster(local)).toEqual({ poster: 'local-frame', duration: 7 })
  })

  it('keys by version so a replaced file captures a new frame', () => {
    const first = galleryPosterKey({ path: '/home/gallery/a.mp4', resourceVersion: 'sha256:1' })
    const second = galleryPosterKey({ path: '/home/gallery/a.mp4', resourceVersion: 'sha256:2' })
    storeGalleryPoster(first, { poster: 'data:image/jpeg;base64,AA==', duration: 7 })
    expect(readGalleryPoster(first)).toEqual({ poster: 'data:image/jpeg;base64,AA==', duration: 7 })
    expect(readGalleryPoster(second)).toBeUndefined()
  })

  it('evicts the oldest posters beyond its bound', () => {
    for (let index = 0; index < 250; index += 1) storeGalleryPoster(`key-${index}`, { duration: index })
    expect(readGalleryPoster('key-0')).toBeUndefined()
    expect(readGalleryPoster('key-9')).toBeUndefined()
    expect(readGalleryPoster('key-10')).toEqual({ duration: 10 })
    expect(readGalleryPoster('key-249')).toEqual({ duration: 249 })
  })

  it('does not capture a frame from a stream without video', () => {
    const video = document.createElement('video')
    expect(captureGalleryVideoFrame(video)).toBeUndefined()
  })
})
