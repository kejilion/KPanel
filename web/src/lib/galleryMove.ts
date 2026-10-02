import { albumCover, type GalleryAlbum, type GalleryFolderSnapshot } from '@/lib/galleryLibrary'
import { galleryBaseName, galleryParentPath, type GalleryItem } from '@/lib/gallery'
import type { FileEntry } from '@/types/api'

/**
 * Moving photos between albums. The move itself is the ordinary file "move"
 * action (same host, never overwrites, one result per file); this module only
 * decides what to ask for and how the page changes afterwards, so the page can
 * update at once instead of waiting for a re-read.
 */

export const GALLERY_DRAG_TYPE = 'application/x-kpanel-gallery-items'
const GALLERY_DRAG_MAX_PATHS = 500
const GALLERY_DRAG_MAX_PATH_LENGTH = 4096

/** What a drag from the timeline carries: the host it came from and the files being moved. */
export interface GalleryDragPayload {
  hostId: string
  paths: string[]
}

export function serializeGalleryDrag(payload: GalleryDragPayload): string {
  return JSON.stringify(payload)
}

/** The drag payload, or undefined for anything that is not one of ours. */
export function parseGalleryDrag(text: string): GalleryDragPayload | undefined {
  try {
    const value: unknown = JSON.parse(text)
    if (!value || typeof value !== 'object') return undefined
    const { hostId, paths } = value as Record<string, unknown>
    if (typeof hostId !== 'string' || !Array.isArray(paths) || !paths.length || paths.length > GALLERY_DRAG_MAX_PATHS) return undefined
    return paths.every((path) => typeof path === 'string' && path.startsWith('/') && path.length <= GALLERY_DRAG_MAX_PATH_LENGTH)
      ? { hostId, paths: paths as string[] }
      : undefined
  } catch {
    return undefined
  }
}

/** Entries that actually need moving: the ones not already in `destination`. */
export function entriesToMove(entries: readonly FileEntry[], destination: string): FileEntry[] {
  return entries.filter((entry) => galleryParentPath(entry.path) !== destination)
}

export interface GalleryMoveResult {
  succeeded: ReadonlyArray<{ path: string; destination?: string }>
  failed: ReadonlyArray<{ path: string; detail: string }>
}

/** Old path to new path for every file the host reports moved. */
export function movedPaths(result: GalleryMoveResult, destinationFolder: string): Map<string, string> {
  const moved = new Map<string, string>()
  for (const item of result.succeeded) {
    moved.set(item.path, item.destination ?? `${destinationFolder === '/' ? '' : destinationFolder}/${galleryBaseName(item.path)}`)
  }
  return moved
}

function recountAlbum(album: GalleryAlbum, items: readonly GalleryItem[]): GalleryAlbum {
  const own = items.filter((item) => item.folder === album.path)
  const pinned = album.coverPinned && album.cover ? own.find((item) => item.entry.path === album.cover!.entry.path) : undefined
  return {
    ...album,
    imageCount: own.filter((item) => item.kind === 'image').length,
    videoCount: own.filter((item) => item.kind === 'video').length,
    cover: pinned ?? albumCover(own),
    coverPinned: Boolean(pinned),
    latest: own.reduce((latest, item) => Math.max(latest, item.time), 0),
  }
}

/**
 * The page after `moved` happened. A file that landed in the page's folder or
 * in one of its scanned albums stays in the timeline under its new path; one
 * that left for anywhere else disappears. Album counts and covers follow, and a
 * hand-picked cover that moved away quietly stops being one (the marker is
 * stale; the next read treats it as absent).
 */
export function relocateGalleryItems(
  snapshot: GalleryFolderSnapshot,
  moved: ReadonlyMap<string, string>,
): GalleryFolderSnapshot {
  if (!moved.size) return snapshot
  const folders = new Set([snapshot.path, ...snapshot.albums.map((album) => album.path)])
  const items: GalleryItem[] = []
  for (const item of snapshot.items) {
    const destination = moved.get(item.entry.path)
    if (destination === undefined) {
      items.push(item)
      continue
    }
    const folder = galleryParentPath(destination)
    if (!folders.has(folder)) continue
    items.push({ ...item, folder, entry: { ...item.entry, path: destination, name: galleryBaseName(destination) } })
  }
  const coverPath = snapshot.coverPath && items.some((item) => item.entry.path === snapshot.coverPath) ? snapshot.coverPath : undefined
  return {
    ...snapshot,
    items,
    coverPath,
    albums: snapshot.albums.map((album) => (album.scanned && !album.failed ? recountAlbum(album, items) : album)),
  }
}
