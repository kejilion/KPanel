import { describe, expect, it } from 'vitest'
import type { GalleryAlbum, GalleryFolderSnapshot } from '@/lib/galleryLibrary'
import type { GalleryItem } from '@/lib/gallery'
import {
  entriesToMove,
  movedPaths,
  parseGalleryDrag,
  relocateGalleryItems,
  serializeGalleryDrag,
} from './galleryMove'
import type { FileEntry } from '@/types/api'

function entry(path: string, modifiedAt = '2026-09-10T00:00:00Z'): FileEntry {
  return {
    name: path.slice(path.lastIndexOf('/') + 1), path, kind: 'file', mime: 'image/jpeg', sizeBytes: 2048,
    mode: '-rw-r--r--', owner: 'root', group: 'root', modifiedAt, resourceVersion: `sha256:${'a'.repeat(64)}`,
    editable: false, previewable: true,
  }
}

function item(path: string, modifiedAt?: string): GalleryItem {
  const value = entry(path, modifiedAt)
  return { entry: value, kind: 'image', folder: path.slice(0, path.lastIndexOf('/')), time: Date.parse(value.modifiedAt) }
}

function album(path: string, overrides: Partial<GalleryAlbum> = {}): GalleryAlbum {
  return {
    entry: { ...entry(path), kind: 'directory', mime: undefined }, path, name: path.slice(path.lastIndexOf('/') + 1),
    imageCount: 0, videoCount: 0, folderCount: 0, latest: 0, coverPinned: false, scanned: true, truncated: false, failed: false,
    ...overrides,
  }
}

const ROOT = '/home/gallery'

function snapshot(): GalleryFolderSnapshot {
  const items = [
    item(`${ROOT}/loose.jpg`, '2026-09-20T00:00:00Z'),
    item(`${ROOT}/Kyoto/temple.jpg`, '2026-09-12T00:00:00Z'),
    item(`${ROOT}/Kyoto/old.jpg`, '2026-07-01T00:00:00Z'),
    item(`${ROOT}/Osaka/castle.jpg`, '2026-08-01T00:00:00Z'),
  ]
  return {
    path: ROOT, exists: true, items, scanning: false, truncated: false, failedAlbums: 0,
    coverPath: `${ROOT}/Kyoto/temple.jpg`,
    albums: [
      album(`${ROOT}/Kyoto`, { imageCount: 2, cover: items[1], coverPinned: true }),
      album(`${ROOT}/Osaka`, { imageCount: 1, cover: items[3] }),
      album(`${ROOT}/Failed`, { failed: true }),
    ],
  }
}

describe('gallery drag payload', () => {
  it('round-trips and rejects anything that is not ours', () => {
    const payload = { hostId: 'edge-1', paths: [`${ROOT}/a.jpg`, `${ROOT}/b.jpg`] }
    expect(parseGalleryDrag(serializeGalleryDrag(payload))).toEqual(payload)
    for (const text of ['', 'nope', '[]', '{"hostId":1,"paths":["/a"]}', '{"hostId":"","paths":[]}', '{"hostId":"","paths":["relative.jpg"]}', '{"hostId":"","paths":[7]}']) {
      expect(parseGalleryDrag(text), text).toBeUndefined()
    }
    expect(parseGalleryDrag(JSON.stringify({ hostId: '', paths: Array.from({ length: 501 }, (_, i) => `/a${i}.jpg`) }))).toBeUndefined()
  })
})

describe('what to move', () => {
  it('skips files already in the destination', () => {
    const entries = [entry(`${ROOT}/a.jpg`), entry(`${ROOT}/Kyoto/b.jpg`)]
    expect(entriesToMove(entries, `${ROOT}/Kyoto`).map((value) => value.name)).toEqual(['a.jpg'])
    expect(entriesToMove(entries, ROOT).map((value) => value.name)).toEqual(['b.jpg'])
    expect(entriesToMove(entries, '/elsewhere')).toHaveLength(2)
  })

  it('maps old paths to the paths the host reports, or builds them from the destination', () => {
    const moved = movedPaths(
      { succeeded: [{ path: `${ROOT}/a.jpg`, destination: `${ROOT}/Kyoto/a.jpg` }, { path: `${ROOT}/b.jpg` }], failed: [] },
      `${ROOT}/Kyoto`,
    )
    expect([...moved]).toEqual([[`${ROOT}/a.jpg`, `${ROOT}/Kyoto/a.jpg`], [`${ROOT}/b.jpg`, `${ROOT}/Kyoto/b.jpg`]])
    expect(movedPaths({ succeeded: [{ path: '/x/a.jpg' }], failed: [] }, '/').get('/x/a.jpg')).toBe('/a.jpg')
  })
})

describe('the page after a move', () => {
  it('keeps a file that stays in the page, under its new path, and recounts both albums', () => {
    const next = relocateGalleryItems(snapshot(), new Map([[`${ROOT}/loose.jpg`, `${ROOT}/Osaka/loose.jpg`]]))
    expect(next.items.map((value) => value.entry.path).sort()).toEqual([
      `${ROOT}/Kyoto/old.jpg`, `${ROOT}/Kyoto/temple.jpg`, `${ROOT}/Osaka/castle.jpg`, `${ROOT}/Osaka/loose.jpg`,
    ])
    const moved = next.items.find((value) => value.entry.name === 'loose.jpg')!
    expect(moved).toMatchObject({ folder: `${ROOT}/Osaka` })
    const osaka = next.albums.find((value) => value.name === 'Osaka')!
    expect(osaka.imageCount).toBe(2)
    // The newest photo is now the card cover, since this card was on automatic.
    expect(osaka.cover?.entry.name).toBe('loose.jpg')
    expect(next.albums.find((value) => value.name === 'Kyoto')!.imageCount).toBe(2)
  })

  it('drops a file that left for a folder the page does not show', () => {
    const next = relocateGalleryItems(snapshot(), new Map([[`${ROOT}/Osaka/castle.jpg`, '/srv/other/castle.jpg']]))
    expect(next.items).toHaveLength(3)
    const osaka = next.albums.find((value) => value.name === 'Osaka')!
    expect(osaka).toMatchObject({ imageCount: 0, cover: undefined })
  })

  it('lets a moved-away cover go: the page cover clears and the album falls back to its newest photo', () => {
    const next = relocateGalleryItems(snapshot(), new Map([[`${ROOT}/Kyoto/temple.jpg`, `${ROOT}/Osaka/temple.jpg`]]))
    // The page cover followed the photo? No: the marker names the old path, so it no longer matches anything.
    expect(next.coverPath).toBeUndefined()
    const kyoto = next.albums.find((value) => value.name === 'Kyoto')!
    expect(kyoto.coverPinned).toBe(false)
    expect(kyoto.cover?.entry.name).toBe('old.jpg')
  })

  it('leaves albums that were never read, and the snapshot itself, alone when nothing moved', () => {
    const base = snapshot()
    expect(relocateGalleryItems(base, new Map())).toBe(base)
    const next = relocateGalleryItems(base, new Map([[`${ROOT}/loose.jpg`, `${ROOT}/Kyoto/loose.jpg`]]))
    expect(next.albums.find((value) => value.name === 'Failed')).toBe(base.albums[2])
  })
})
