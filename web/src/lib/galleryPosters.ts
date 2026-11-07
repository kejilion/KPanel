import { reactive } from 'vue'
import type { FileEntry } from '@/types/api'
import { queueGalleryVideo } from '@/lib/galleryMedia'

/**
 * Poster frames captured from gallery videos. Chromium caps how many media
 * players a page may hold, so a tile grabs one frame into a small JPEG, keeps
 * it here and drops its <video> element; only a hover preview or the viewer
 * plays the stream again. The cache lives for the tab and is bounded.
 */

export interface GalleryPoster {
  poster?: string
  duration?: number
  width?: number
  height?: number
}

const MAX_POSTERS = 240
const MAX_POSTER_BYTES = 12 * 1024 * 1024
const POSTER_EDGE = 640
const posters = reactive(new Map<string, GalleryPoster>())

export function galleryPosterKey(entry: Pick<FileEntry, 'path' | 'resourceVersion'>, sourceURL = ''): string {
  // The content URL carries the file relay host; path/version can match on two hosts.
  return `${sourceURL}\u0000${entry.resourceVersion}\u0000${entry.path}`
}

export function readGalleryPoster(key: string): GalleryPoster | undefined {
  return posters.get(key)
}

export function storeGalleryPoster(key: string, value: GalleryPoster): void {
  posters.delete(key)
  if ((value.poster?.length ?? 0) * 2 > MAX_POSTER_BYTES) return
  posters.set(key, { ...value })
  while (posters.size > MAX_POSTERS || [...posters.values()].reduce((bytes, value) => bytes + (value.poster?.length ?? 0) * 2, 0) > MAX_POSTER_BYTES) {
    const oldest = posters.keys().next().value
    if (oldest === undefined) break
    posters.delete(oldest)
  }
}

/** Draw the current frame into a JPEG data URL; undefined when the frame is unavailable. */
export function captureGalleryVideoFrame(video: HTMLVideoElement): string | undefined {
  const width = video.videoWidth
  const height = video.videoHeight
  if (!width || !height || typeof document === 'undefined') return undefined
  const scale = Math.min(1, POSTER_EDGE / Math.max(width, height))
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(width * scale))
  canvas.height = Math.max(1, Math.round(height * scale))
  try {
    const context = canvas.getContext('2d')
    if (!context) return undefined
    context.drawImage(video, 0, 0, canvas.width, canvas.height)
    return canvas.toDataURL('image/jpeg', 0.8)
  } catch {
    // A cross-origin or undecodable frame cannot be read back; the tile keeps its icon.
    return undefined
  }
}

export function resetGalleryPostersForTest(): void {
  posters.clear()
}

/** Seek explicitly; loadeddata alone can still represent the initial black frame. */
export async function loadGalleryPoster(key: string, url: string, signal: AbortSignal, metadataOnly = false): Promise<void> {
  await queueGalleryVideo(async () => {
    const cached = readGalleryPoster(key)
    if (metadataOnly ? cached?.width && cached.height : cached?.poster) return
    const video = document.createElement('video')
    video.muted = true
    video.playsInline = true
    video.preload = metadataOnly ? 'metadata' : 'auto'
    let frameCallback: number | undefined
    let fallbackTimer: ReturnType<typeof setTimeout> | undefined
    try {
      await new Promise<void>((resolve, reject) => {
        const cleanup = () => {
          clearTimeout(timer)
          signal.removeEventListener('abort', abort)
          video.onloadedmetadata = null
          video.onseeked = null
          video.onerror = null
        }
        const abort = () => { cleanup(); reject(signal.reason ?? new Error('Poster cancelled')) }
        const fail = () => { cleanup(); reject(new Error('Video poster unavailable')) }
        const finish = () => {
          if (signal.aborted) { abort(); return }
          const poster = captureGalleryVideoFrame(video)
          if (!poster) { fail(); return }
          storeGalleryPoster(key, { poster, duration: Number.isFinite(video.duration) ? video.duration : undefined, width: video.videoWidth, height: video.videoHeight })
          cleanup()
          resolve()
        }
        const timer = setTimeout(fail, 12_000)
        signal.addEventListener('abort', abort, { once: true })
        video.onerror = fail
        video.onloadedmetadata = () => {
          if (video.videoWidth <= 0 || video.videoHeight <= 0 || !Number.isFinite(video.duration) || video.duration <= 0) { fail(); return }
          if (metadataOnly) {
            storeGalleryPoster(key, { ...readGalleryPoster(key), duration: video.duration, width: video.videoWidth, height: video.videoHeight })
            cleanup(); resolve(); return
          }
          // Do not use a fragment: explicitly wait for the target seek to finish.
          video.currentTime = Math.min(1, video.duration / 3)
        }
        video.onseeked = () => {
          if (typeof video.requestVideoFrameCallback === 'function') {
            frameCallback = video.requestVideoFrameCallback(finish)
            // Some paused browser players do not dispatch a frame callback after seeked.
            fallbackTimer = setTimeout(finish, 150)
          } else finish()
        }
        if (signal.aborted) { abort(); return }
        video.src = url
        video.load()
      })
    } finally {
      clearTimeout(fallbackTimer)
      if (frameCallback !== undefined) video.cancelVideoFrameCallback(frameCallback)
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, signal)
}
