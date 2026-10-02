import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import {
  GALLERY_ALBUM_SCAN_LIMIT,
  GALLERY_FOLDER_PAGES,
  GALLERY_PAGE_SIZE,
  loadGalleryFolder,
  type GalleryFolderSnapshot,
} from './galleryLibrary'
import type { FileDirectory, FileEntry } from '@/types/api'

function file(path: string, overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name: path.slice(path.lastIndexOf('/') + 1),
    path,
    kind: 'file',
    mime: 'application/octet-stream',
    sizeBytes: 1024,
    mode: '-rw-r--r--',
    owner: 'root',
    group: 'root',
    modifiedAt: '2026-09-01T00:00:00Z',
    resourceVersion: `sha256:${'b'.repeat(64)}`,
    editable: false,
    previewable: true,
    ...overrides,
  }
}

function folder(path: string): FileEntry {
  return file(path, { kind: 'directory', mime: undefined, sizeBytes: 4096 })
}

function directory(path: string, entries: FileEntry[], nextOffset?: number): FileDirectory {
  return { path, entries, offset: 0, nextOffset, truncated: false, readAt: '2026-09-01T00:00:00Z' }
}

describe('gallery folder loader', () => {
  it('reads the folder, then each direct album, and reports progress', async () => {
    const tree: Record<string, FileEntry[]> = {
      '/home/gallery': [
        file('/home/gallery/loose.jpg'),
        file('/home/gallery/notes.txt'),
        folder('/home/gallery/Kyoto'),
        folder('/home/gallery/.trash'),
        folder('/home/gallery/Empty'),
      ],
      '/home/gallery/Kyoto': [
        file('/home/gallery/Kyoto/old.jpg', { modifiedAt: '2026-08-01T00:00:00Z' }),
        file('/home/gallery/Kyoto/new.jpg', { modifiedAt: '2026-09-10T00:00:00Z' }),
        file('/home/gallery/Kyoto/clip.mp4', { modifiedAt: '2026-09-20T00:00:00Z' }),
        folder('/home/gallery/Kyoto/Day 1'),
      ],
      '/home/gallery/Empty': [],
    }
    const list = vi.fn(async (path: string) => directory(path, tree[path] ?? []))
    const updates: GalleryFolderSnapshot[] = []

    const result = await loadGalleryFolder({ list }, '/home/gallery', (snapshot) => updates.push(snapshot))

    expect(list).not.toHaveBeenCalledWith('/home/gallery/.trash', expect.anything(), undefined)
    expect(updates[0]!.scanning).toBe(true)
    expect(updates.at(-1)!.scanning).toBe(false)
    expect(result.items.map((item) => item.entry.name).sort()).toEqual(['clip.mp4', 'loose.jpg', 'new.jpg', 'old.jpg'])
    const kyoto = result.albums.find((album) => album.name === 'Kyoto')!
    expect(kyoto).toMatchObject({ imageCount: 2, videoCount: 1, folderCount: 1, scanned: true })
    // Covers prefer the newest still image over a newer video.
    expect(kyoto.cover?.entry.name).toBe('new.jpg')
    expect(result.albums.find((album) => album.name === 'Empty')).toMatchObject({ imageCount: 0, cover: undefined })
    expect(result.truncated).toBe(false)
  })

  it('reports a missing folder instead of failing', async () => {
    const list = vi.fn(async () => {
      throw new ApiError('not found', 404, 'not_found')
    })
    await expect(loadGalleryFolder({ list }, '/home/gallery')).resolves.toMatchObject({ exists: false, items: [] })
  })

  it('keeps going when one album cannot be read', async () => {
    const list = vi.fn(async (path: string) => {
      if (path === '/home/gallery/Locked') throw new ApiError('denied', 403, 'forbidden')
      if (path === '/home/gallery') return directory(path, [folder('/home/gallery/Locked'), folder('/home/gallery/Open')])
      return directory(path, [file(`${path}/a.png`)])
    })
    const result = await loadGalleryFolder({ list }, '/home/gallery')
    expect(result.failedAlbums).toBe(1)
    expect(result.albums.find((album) => album.name === 'Locked')).toMatchObject({ failed: true })
    expect(result.items).toHaveLength(1)
  })

  it('pages with the large page size and stops at the page and album bounds', async () => {
    const list = vi.fn(async (path: string, options: { offset?: number; limit?: number }) => {
      expect(options.limit).toBe(GALLERY_PAGE_SIZE)
      if (path !== '/big') return directory(path, [])
      const offset = options.offset ?? 0
      const albums = offset === 0
        ? Array.from({ length: GALLERY_ALBUM_SCAN_LIMIT + 2 }, (_, index) => folder(`/big/album-${index}`))
        : []
      return directory(path, [file(`/big/${offset}.jpg`), ...albums], offset + GALLERY_PAGE_SIZE)
    })
    const result = await loadGalleryFolder({ list }, '/big')
    const rootCalls = list.mock.calls.filter(([path]) => path === '/big')
    expect(rootCalls).toHaveLength(GALLERY_FOLDER_PAGES)
    expect(list.mock.calls.length - rootCalls.length).toBe(GALLERY_ALBUM_SCAN_LIMIT)
    expect(result.truncated).toBe(true)
    expect(result.albums).toHaveLength(GALLERY_ALBUM_SCAN_LIMIT + 2)
  })

  it('propagates aborts so a newer navigation wins', async () => {
    const controller = new AbortController()
    const list = vi.fn(async (path: string) => {
      if (path === '/home/gallery') return directory(path, [folder('/home/gallery/A')])
      controller.abort()
      throw new DOMException('aborted', 'AbortError')
    })
    await expect(loadGalleryFolder({ list }, '/home/gallery', undefined, controller.signal)).rejects.toThrow('aborted')
  })
})
