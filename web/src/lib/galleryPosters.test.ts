// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import {
  captureGalleryVideoFrame,
  galleryPosterKey,
  readGalleryPoster,
  resetGalleryPostersForTest,
  storeGalleryPoster,
} from './galleryPosters'

afterEach(() => resetGalleryPostersForTest())

describe('gallery poster cache', () => {
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
