import { reactive } from 'vue'
import type { FileEntry } from '@/types/api'

/**
 * Poster frames captured from gallery videos. Chromium caps how many media
 * players a page may hold, so a tile grabs one frame into a small JPEG, keeps
 * it here and drops its <video> element; only a hover preview or the viewer
 * plays the stream again. The cache lives for the tab and is bounded.
 */

export interface GalleryPoster {
  poster?: string
  duration?: number
}

const MAX_POSTERS = 240
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
  posters.set(key, { ...value })
  while (posters.size > MAX_POSTERS) {
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
