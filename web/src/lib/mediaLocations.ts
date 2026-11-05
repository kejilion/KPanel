import { reactive } from 'vue'
import { browserStorage, GALLERY_PREFERENCES_KEY, normalizeGalleryRoot } from '@/lib/gallery'
import type { MediaLocation } from '@/types/api'

export const mediaLocationKeys = ['galleryLocation', 'musicLocation', 'cinemaLocation'] as const
export type MediaLocationKey = typeof mediaLocationKeys[number]
export const mediaLocations = reactive<Partial<Record<MediaLocationKey, MediaLocation>>>({})

export function parseMediaLocation(key: MediaLocationKey, value: unknown): MediaLocation | undefined {
  if (!value || typeof value !== 'object') return undefined
  const item = value as Record<string, unknown>
  if (typeof item.path !== 'string' || typeof item.hostId !== 'string' || !/^[A-Za-z0-9_-]{0,128}$/.test(item.hostId)) return undefined
  if (key === 'galleryLocation' && item.hostId) return undefined
  const path = item.path === '/' && key !== 'galleryLocation' ? '/' : normalizeGalleryRoot(item.path)
  if (!path || path !== item.path || /\u007f/u.test(path)) return undefined
  return { path, hostId: item.hostId }
}

export function legacyGalleryLocation(): MediaLocation | undefined {
  try {
    const raw = browserStorage()?.getItem(GALLERY_PREFERENCES_KEY)
    if (!raw || raw.length > 4096) return undefined
    const value = JSON.parse(raw)
    return parseMediaLocation('galleryLocation', { path: value?.root, hostId: '' })
  } catch { return undefined }
}

export function applyMediaLocation(key: MediaLocationKey, value: MediaLocation | undefined): void {
  if (value) mediaLocations[key] = { ...value }
  else delete mediaLocations[key]
}

export function setMediaLocation(key: MediaLocationKey, path: string, hostId = ''): void {
  const value = parseMediaLocation(key, { path, hostId })
  if (!value) return
  applyMediaLocation(key, value)
  window.dispatchEvent(new CustomEvent('kpanel:preferences-changed', { detail: { [key]: value } }))
}
