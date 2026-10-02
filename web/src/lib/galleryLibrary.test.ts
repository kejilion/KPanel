import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import {
  GALLERY_ALBUM_SCAN_LIMIT,
  GALLERY_COVER_READ_TIMEOUT_MS,
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

describe('gallery cover markers', () => {
  const marker = (folderPath: string, size = 40) => file(`${folderPath}/.kpanel-cover.json`, { sizeBytes: size })

  it('uses the photo a folder marker names for the page and for each album card', async () => {
    const tree: Record<string, FileEntry[]> = {
      '/home/gallery': [
        file('/home/gallery/newest.jpg', { modifiedAt: '2026-09-30T00:00:00Z' }),
        marker('/home/gallery'),
        folder('/home/gallery/Kyoto'),
        folder('/home/gallery/Plain'),
      ],
      '/home/gallery/Kyoto': [
        file('/home/gallery/Kyoto/old.jpg', { modifiedAt: '2026-08-01T00:00:00Z' }),
        file('/home/gallery/Kyoto/new.jpg', { modifiedAt: '2026-09-10T00:00:00Z' }),
        marker('/home/gallery/Kyoto'),
      ],
      '/home/gallery/Plain': [file('/home/gallery/Plain/only.jpg')],
    }
    const text = vi.fn(async (path: string) => ({
      '/home/gallery/.kpanel-cover.json': '{"version":1,"cover":"Kyoto/old.jpg"}',
      '/home/gallery/Kyoto/.kpanel-cover.json': '{"version":1,"cover":"old.jpg"}',
    })[path] ?? '')
    const result = await loadGalleryFolder({ list: async (path) => directory(path, tree[path] ?? []), text }, '/home/gallery')

    // The page cover may name a photo one album down; the album card names its own folder's photo.
    expect(result.coverPath).toBe('/home/gallery/Kyoto/old.jpg')
    const kyoto = result.albums.find((album) => album.name === 'Kyoto')!
    expect(kyoto).toMatchObject({ coverPinned: true })
    expect(kyoto.cover?.entry.name).toBe('old.jpg')
    // Albums without a marker keep the automatic cover and cost no marker read.
    expect(result.albums.find((album) => album.name === 'Plain')).toMatchObject({ coverPinned: false })
    expect(text.mock.calls.map(([path]) => path).sort()).toEqual(['/home/gallery/.kpanel-cover.json', '/home/gallery/Kyoto/.kpanel-cover.json'])
  })

  it('falls back to the automatic cover when the marker is stale, damaged, oversized or unreadable', async () => {
    const entries = [file('/home/gallery/a.jpg'), marker('/home/gallery')]
    for (const reply of [
      async () => '{"version":1,"cover":"gone.jpg"}',
      async () => '{"version":1,"cover":"../../etc/passwd"}',
      async () => 'not json at all',
      async () => { throw new ApiError('denied', 403, 'forbidden') },
    ]) {
      const result = await loadGalleryFolder({ list: async (path) => directory(path, entries), text: reply }, '/home/gallery')
      expect(result.exists).toBe(true)
      expect(result.coverPath).toBeUndefined()
      expect(result.items).toHaveLength(1)
    }
    const text = vi.fn(async () => '{"version":1,"cover":"a.jpg"}')
    const oversized = [file('/home/gallery/a.jpg'), marker('/home/gallery', 100_000)]
    expect((await loadGalleryFolder({ list: async (path) => directory(path, oversized), text }, '/home/gallery')).coverPath).toBeUndefined()
    expect(text).not.toHaveBeenCalled()
  })

  it('never pins a hidden or unsupported file and never reads markers it cannot use', async () => {
    const entries = [file('/home/gallery/shot.heic'), marker('/home/gallery'), folder('/home/gallery/A')]
    const albumEntries = [file('/home/gallery/A/x.heic'), file('/home/gallery/A/y.jpg'), marker('/home/gallery/A')]
    const text = vi.fn(async (path: string) => path.startsWith('/home/gallery/A') ? '{"version":1,"cover":"x.heic"}' : '{"version":1,"cover":"shot.heic"}')
    const result = await loadGalleryFolder({ list: async (path) => directory(path, path === '/home/gallery' ? entries : albumEntries), text }, '/home/gallery')
    // HEIC is media but a browser cannot draw it: the album falls back to a photo it can show.
    expect(result.albums[0]).toMatchObject({ coverPinned: false })
    expect(result.albums[0]!.cover?.entry.name).toBe('y.jpg')
    // The page cover still resolves to a known item; the page itself refuses to draw it (see GalleryView).
    expect(result.coverPath).toBe('/home/gallery/shot.heic')
  })

  it('does not let a marker that never answers hold the page back', async () => {
    vi.useFakeTimers()
    try {
      const entries = [file('/home/gallery/a.jpg'), marker('/home/gallery')]
      let readSignal: AbortSignal | undefined
      const pending = loadGalleryFolder({ list: async (path) => directory(path, entries), text: (_path, signal) => {
        readSignal = signal
        return new Promise<string>(() => undefined)
      } }, '/home/gallery')
      await vi.advanceTimersByTimeAsync(GALLERY_COVER_READ_TIMEOUT_MS + 50)
      const result = await pending
      expect(result.exists).toBe(true)
      expect(result.items).toHaveLength(1)
      expect(result.coverPath).toBeUndefined()
      expect(readSignal?.aborted).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  it('cancels the cover request when the owning folder load is cancelled', async () => {
    const controller = new AbortController()
    let readSignal: AbortSignal | undefined
    const text = vi.fn((_path: string, signal?: AbortSignal) => {
      readSignal = signal
      return new Promise<string>((_resolve, reject) => signal?.addEventListener('abort', () => reject(signal.reason), { once: true }))
    })
    const pending = loadGalleryFolder({ list: async (path) => directory(path, [marker(path)]), text }, '/home/gallery', undefined, controller.signal)
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    await vi.waitFor(() => expect(text).toHaveBeenCalled())
    controller.abort()
    expect(readSignal?.aborted).toBe(true)
    await rejected
  })

  it('does not read markers when the caller gave no way to read text', async () => {
    const entries = [file('/home/gallery/a.jpg'), marker('/home/gallery')]
    const result = await loadGalleryFolder({ list: async (path) => directory(path, entries) }, '/home/gallery')
    expect(result.coverPath).toBeUndefined()
  })
})
