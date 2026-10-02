import { ApiError } from '@/lib/api'
import {
  GALLERY_COVER_MARKER,
  GALLERY_COVER_MARKER_MAX_BYTES,
  parseGalleryCover,
  resolveGalleryCover,
} from '@/lib/galleryCover'
import {
  galleryAlbumEntries,
  galleryBrowserCanShow,
  galleryItemsFromEntries,
  type GalleryItem,
} from '@/lib/gallery'
import type { FileDirectory, FileEntry } from '@/types/api'

/**
 * Reads one gallery folder and its direct subfolders through the ordinary
 * file API. Every bound below keeps a huge photo dump from turning one page
 * view into thousands of Agent requests; the snapshot says when it stopped.
 */

export const GALLERY_PAGE_SIZE = 500
export const GALLERY_FOLDER_PAGES = 10
export const GALLERY_ALBUM_PAGES = 4
export const GALLERY_ALBUM_SCAN_LIMIT = 48
const ALBUM_SCAN_CONCURRENCY = 4
/** A cover is a nicety: a marker that takes longer than this to read is treated as absent so it cannot hold the page back. */
export const GALLERY_COVER_READ_TIMEOUT_MS = 3000

export interface GalleryListAPI {
  list: (path: string, options: { offset?: number; limit?: number }, signal?: AbortSignal) => Promise<FileDirectory>
  /** Reads a small text file; only needed to honour cover markers. */
  text?: (path: string, signal?: AbortSignal) => Promise<string>
}

export interface GalleryAlbum {
  entry: FileEntry
  path: string
  name: string
  imageCount: number
  videoCount: number
  folderCount: number
  cover?: GalleryItem
  /** The cover was chosen by hand (a marker in the album names it) rather than picked automatically. */
  coverPinned: boolean
  /** Newest media time in the album, for ordering; 0 when empty. */
  latest: number
  scanned: boolean
  truncated: boolean
  failed: boolean
}

export interface GalleryFolderSnapshot {
  path: string
  exists: boolean
  /** Media directly in the folder plus media in scanned direct subfolders. */
  items: GalleryItem[]
  /** Path of the cover chosen by hand for this folder, when its marker names a photo that is still here. */
  coverPath?: string
  albums: GalleryAlbum[]
  /** Some listing stopped at a page, scan or album bound. */
  truncated: boolean
  /** True while direct subfolders are still being read. */
  scanning: boolean
  /** Direct subfolders that could not be read. */
  failedAlbums: number
}

interface FolderListing {
  entries: FileEntry[]
  truncated: boolean
}

async function listFolder(
  api: GalleryListAPI,
  path: string,
  pages: number,
  signal?: AbortSignal,
): Promise<FolderListing> {
  const entries: FileEntry[] = []
  let offset: number | undefined = 0
  let truncated = false
  for (let page = 0; offset !== undefined; page += 1) {
    if (page >= pages) {
      truncated = true
      break
    }
    const directory: FileDirectory = await api.list(path, { offset, limit: GALLERY_PAGE_SIZE }, signal)
    entries.push(...directory.entries)
    if (directory.scanTruncated) truncated = true
    offset = directory.nextOffset !== undefined && directory.nextOffset > offset ? directory.nextOffset : undefined
  }
  return { entries, truncated }
}

export function albumCover(items: readonly GalleryItem[]): GalleryItem | undefined {
  let image: GalleryItem | undefined
  let video: GalleryItem | undefined
  for (const item of items) {
    if (!galleryBrowserCanShow(item)) continue
    if (item.kind === 'image' && (!image || item.time > image.time)) image = item
    if (item.kind === 'video' && (!video || item.time > video.time)) video = item
  }
  return image ?? video
}

/**
 * The cover a folder's marker names, read only when the listing shows a marker
 * of plausible size. Any failure means "no hand-picked cover": a cover is a
 * nicety and must never make a page fail to load.
 */
async function readCoverMarker(api: GalleryListAPI, folder: string, entries: readonly FileEntry[], signal?: AbortSignal): Promise<string | undefined> {
  const marker = entries.find((entry) => entry.kind === 'file' && entry.name === GALLERY_COVER_MARKER)
  if (!marker || !api.text || marker.sizeBytes > GALLERY_COVER_MARKER_MAX_BYTES) return undefined
  const controller = new AbortController()
  const cancel = () => controller.abort()
  signal?.addEventListener('abort', cancel, { once: true })
  if (signal?.aborted) cancel()
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    const text = await Promise.race([
      api.text(resolveGalleryCover(folder, GALLERY_COVER_MARKER), controller.signal),
      new Promise<string>((_, reject) => { timer = setTimeout(() => {
        cancel()
        reject(new Error('cover read timed out'))
      }, GALLERY_COVER_READ_TIMEOUT_MS) }),
    ])
    const relative = parseGalleryCover(text)
    return relative === undefined ? undefined : resolveGalleryCover(folder, relative)
  } catch {
    if (signal?.aborted) throw signal.reason
    return undefined
  } finally {
    clearTimeout(timer)
    signal?.removeEventListener('abort', cancel)
    cancel()
  }
}

function summarizeAlbum(
  album: GalleryAlbum,
  listing: FolderListing,
  pinnedPath?: string,
): { album: GalleryAlbum; items: GalleryItem[] } {
  const items = galleryItemsFromEntries(listing.entries, album.path)
  const pinned = pinnedPath ? items.find((item) => item.entry.path === pinnedPath && galleryBrowserCanShow(item)) : undefined
  return {
    items,
    album: {
      ...album,
      imageCount: items.filter((item) => item.kind === 'image').length,
      videoCount: items.filter((item) => item.kind === 'video').length,
      folderCount: galleryAlbumEntries(listing.entries).length,
      cover: pinned ?? albumCover(items),
      coverPinned: Boolean(pinned),
      latest: items.reduce((latest, item) => Math.max(latest, item.time), 0),
      scanned: true,
      truncated: listing.truncated,
    },
  }
}

export function isMissingFolderError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

/**
 * Load a folder snapshot. `onUpdate` receives the folder's own media first and
 * then each completed album, so the page fills in without waiting for the
 * slowest subfolder. The returned promise resolves with the final snapshot.
 */
export async function loadGalleryFolder(
  api: GalleryListAPI,
  path: string,
  onUpdate?: (snapshot: GalleryFolderSnapshot) => void,
  signal?: AbortSignal,
): Promise<GalleryFolderSnapshot> {
  let listing: FolderListing
  try {
    listing = await listFolder(api, path, GALLERY_FOLDER_PAGES, signal)
  } catch (error) {
    if (!isMissingFolderError(error)) throw error
    const missing: GalleryFolderSnapshot = {
      path, exists: false, items: [], albums: [], truncated: false, scanning: false, failedAlbums: 0,
    }
    onUpdate?.(missing)
    return missing
  }

  const folders = galleryAlbumEntries(listing.entries)
    .sort((left, right) => left.name.localeCompare(right.name, undefined, { numeric: true }))
  const albums: GalleryAlbum[] = folders.map((entry) => ({
    entry, path: entry.path, name: entry.name,
    imageCount: 0, videoCount: 0, folderCount: 0, latest: 0,
    coverPinned: false, scanned: false, truncated: false, failed: false,
  }))
  const ownItems = galleryItemsFromEntries(listing.entries, path)
  // Read before the first paint only when a marker exists, so ordinary folders pay nothing.
  const pinnedPath = await readCoverMarker(api, path, listing.entries, signal)
  const albumItems = new Map<string, GalleryItem[]>()
  let failedAlbums = 0
  const snapshot = (scanning: boolean): GalleryFolderSnapshot => {
    const items = [...ownItems, ...[...albumItems.values()].flat()]
    return {
      path,
      exists: true,
      items,
      coverPath: pinnedPath && items.some((item) => item.entry.path === pinnedPath) ? pinnedPath : undefined,
      albums: [...albums],
      truncated: listing.truncated || folders.length > GALLERY_ALBUM_SCAN_LIMIT || albums.some((album) => album.truncated),
      scanning,
      failedAlbums,
    }
  }
  const scanTargets = albums.slice(0, GALLERY_ALBUM_SCAN_LIMIT)
  onUpdate?.(snapshot(scanTargets.length > 0))

  let cursor = 0
  let completed = 0
  await Promise.all(Array.from({ length: Math.min(ALBUM_SCAN_CONCURRENCY, scanTargets.length) }, async () => {
    while (cursor < scanTargets.length) {
      const index = cursor++
      const album = scanTargets[index]!
      try {
        const albumListing = await listFolder(api, album.path, GALLERY_ALBUM_PAGES, signal)
        const summary = summarizeAlbum(album, albumListing, await readCoverMarker(api, album.path, albumListing.entries, signal))
        albums[index] = summary.album
        albumItems.set(album.path, summary.items)
      } catch (error) {
        if (signal?.aborted) throw error
        failedAlbums += 1
        albums[index] = { ...album, scanned: true, failed: true }
      }
      completed += 1
      if (completed < scanTargets.length) onUpdate?.(snapshot(true))
    }
  }))
  const final = snapshot(false)
  onUpdate?.(final)
  return final
}
