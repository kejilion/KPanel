import { ApiError } from '@/lib/api'
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

export interface GalleryListAPI {
  list: (path: string, options: { offset?: number; limit?: number }, signal?: AbortSignal) => Promise<FileDirectory>
}

export interface GalleryAlbum {
  entry: FileEntry
  path: string
  name: string
  imageCount: number
  videoCount: number
  folderCount: number
  cover?: GalleryItem
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

function albumCover(items: readonly GalleryItem[]): GalleryItem | undefined {
  let image: GalleryItem | undefined
  let video: GalleryItem | undefined
  for (const item of items) {
    if (!galleryBrowserCanShow(item)) continue
    if (item.kind === 'image' && (!image || item.time > image.time)) image = item
    if (item.kind === 'video' && (!video || item.time > video.time)) video = item
  }
  return image ?? video
}

function summarizeAlbum(album: GalleryAlbum, listing: FolderListing): { album: GalleryAlbum; items: GalleryItem[] } {
  const items = galleryItemsFromEntries(listing.entries, album.path)
  return {
    items,
    album: {
      ...album,
      imageCount: items.filter((item) => item.kind === 'image').length,
      videoCount: items.filter((item) => item.kind === 'video').length,
      folderCount: galleryAlbumEntries(listing.entries).length,
      cover: albumCover(items),
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
    scanned: false, truncated: false, failed: false,
  }))
  const ownItems = galleryItemsFromEntries(listing.entries, path)
  const albumItems = new Map<string, GalleryItem[]>()
  let failedAlbums = 0
  const snapshot = (scanning: boolean): GalleryFolderSnapshot => ({
    path,
    exists: true,
    items: [...ownItems, ...[...albumItems.values()].flat()],
    albums: [...albums],
    truncated: listing.truncated || folders.length > GALLERY_ALBUM_SCAN_LIMIT || albums.some((album) => album.truncated),
    scanning,
    failedAlbums,
  })
  const scanTargets = albums.slice(0, GALLERY_ALBUM_SCAN_LIMIT)
  onUpdate?.(snapshot(scanTargets.length > 0))

  let cursor = 0
  let completed = 0
  await Promise.all(Array.from({ length: Math.min(ALBUM_SCAN_CONCURRENCY, scanTargets.length) }, async () => {
    while (cursor < scanTargets.length) {
      const index = cursor++
      const album = scanTargets[index]!
      try {
        const summary = summarizeAlbum(album, await listFolder(api, album.path, GALLERY_ALBUM_PAGES, signal))
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
