import { describe, expect, it } from 'vitest'
import {
  filterGalleryItems,
  formatGalleryDuration,
  galleryAlbumNameProblem,
  galleryItemsFromEntries,
  galleryMediaKind,
  galleryPathTrail,
  galleryTileSource,
  GALLERY_DEFAULT_ROOT,
  GALLERY_PREFERENCES_KEY,
  groupGalleryItemsByMonth,
  isGalleryUploadCandidate,
  nextGalleryUploadName,
  normalizeGalleryRoot,
  readGalleryPreferences,
  sortGalleryItems,
  writeGalleryPreferences,
} from './gallery'
import type { FileEntry } from '@/types/api'

function entry(name: string, overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name,
    path: `/home/gallery/${name}`,
    kind: 'file',
    mime: 'application/octet-stream',
    sizeBytes: 2 * 1024 * 1024,
    mode: '-rw-r--r--',
    owner: 'root',
    group: 'root',
    modifiedAt: '2026-09-12T08:00:00Z',
    resourceVersion: `sha256:${'a'.repeat(64)}`,
    editable: false,
    previewable: true,
    ...overrides,
  }
}

describe('gallery media classification', () => {
  it('trusts extensions over host MIME tables and ignores hidden, svg and non-files', () => {
    expect(galleryMediaKind(entry('clip.mp4'))).toBe('video')
    expect(galleryMediaKind(entry('IMG_0001.HEIC'))).toBe('image')
    expect(galleryMediaKind(entry('scan', { mime: 'image/png' }))).toBe('image')
    expect(galleryMediaKind(entry('logo.svg', { mime: 'image/svg+xml' }))).toBeUndefined()
    expect(galleryMediaKind(entry('.cover.jpg'))).toBeUndefined()
    expect(galleryMediaKind(entry('notes.txt', { mime: 'text/plain' }))).toBeUndefined()
    expect(galleryMediaKind(entry('Trip', { kind: 'directory' }))).toBeUndefined()
  })

  it('accepts uploads by name or browser type', () => {
    expect(isGalleryUploadCandidate({ name: 'a.jpeg', type: '' })).toBe(true)
    expect(isGalleryUploadCandidate({ name: 'screen-recording', type: 'video/webm' })).toBe(true)
    expect(isGalleryUploadCandidate({ name: 'report.pdf', type: 'application/pdf' })).toBe(false)
  })

  it('picks the lightest tile source that still renders', () => {
    const jpeg = { entry: entry('a.jpg'), kind: 'image' as const }
    expect(galleryTileSource(jpeg)).toEqual({ type: 'thumbnail' })
    // Large tiles prefer a sharp original while it stays small.
    expect(galleryTileSource(jpeg, true)).toEqual({ type: 'original' })
    const heavy = { entry: entry('b.jpg', { sizeBytes: 11 * 1024 * 1024 }), kind: 'image' as const }
    expect(galleryTileSource(heavy)).toEqual({ type: 'thumbnail' })
    expect(galleryTileSource(heavy, true)).toEqual({ type: 'original' })
    const over = { entry: entry('o.jpg', { sizeBytes: 13 * 1024 * 1024 }), kind: 'image' as const }
    // Past the Agent thumbnail limit a large tile still loads the original up to the fallback cap.
    expect(galleryTileSource(over)).toEqual({ type: 'original' })
    expect(galleryTileSource({ entry: entry('c.webp'), kind: 'image' })).toEqual({ type: 'original' })
    expect(galleryTileSource({ entry: entry('d.webp', { sizeBytes: 40 * 1024 * 1024 }), kind: 'image' })).toEqual({ type: 'none' })
    expect(galleryTileSource({ entry: entry('e.heic'), kind: 'image' })).toEqual({ type: 'none' })
    expect(galleryTileSource({ entry: entry('f.mov'), kind: 'video' })).toEqual({ type: 'video' })
    expect(galleryTileSource({ entry: entry('g.avi'), kind: 'video' })).toEqual({ type: 'none' })
  })
})

describe('gallery timeline', () => {
  const items = galleryItemsFromEntries([
    entry('b.jpg', { modifiedAt: '2026-09-02T10:00:00Z' }),
    entry('a.jpg', { modifiedAt: '2026-08-20T10:00:00Z' }),
    entry('clip.mp4', { modifiedAt: '2026-09-20T10:00:00Z' }),
    entry('readme.md'),
    entry('Album', { kind: 'directory' }),
    entry('broken.png', { modifiedAt: 'not a date' }),
  ], '/home/gallery')

  it('keeps media only and tolerates unreadable times', () => {
    expect(items.map((item) => item.entry.name)).toEqual(['b.jpg', 'a.jpg', 'clip.mp4', 'broken.png'])
    expect(items.find((item) => item.entry.name === 'broken.png')?.time).toBe(0)
  })

  it('sorts, filters and groups by local month', () => {
    const newest = sortGalleryItems(items, 'newest')
    expect(newest.map((item) => item.entry.name)).toEqual(['clip.mp4', 'b.jpg', 'a.jpg', 'broken.png'])
    expect(sortGalleryItems(items, 'oldest')[0]!.entry.name).toBe('broken.png')
    expect(filterGalleryItems(newest, 'video').map((item) => item.entry.name)).toEqual(['clip.mp4'])
    expect(filterGalleryItems(newest, 'all', 'B.J').map((item) => item.entry.name)).toEqual(['b.jpg'])

    const groups = groupGalleryItemsByMonth(newest, 'newest')
    expect(groups.map((group) => [group.key, group.items.length])).toEqual([
      ['2026-09', 2], ['2026-08', 1], ['undated', 1],
    ])
    expect(groupGalleryItemsByMonth(sortGalleryItems(items, 'name'), 'name')).toHaveLength(1)
  })

  it('formats durations for tiles', () => {
    expect(formatGalleryDuration(42.4)).toBe('0:42')
    expect(formatGalleryDuration(3723)).toBe('1:02:03')
    expect(formatGalleryDuration(Number.NaN)).toBe('')
  })
})

describe('gallery folders and names', () => {
  it('accepts only canonical absolute folders other than the root', () => {
    expect(normalizeGalleryRoot(' /srv/photos/ ')).toBe('/srv/photos')
    expect(normalizeGalleryRoot('/')).toBeUndefined()
    expect(normalizeGalleryRoot('relative/path')).toBeUndefined()
    expect(normalizeGalleryRoot('/home/../etc')).toBeUndefined()
    expect(normalizeGalleryRoot('/home//gallery')).toBeUndefined()
    expect(normalizeGalleryRoot('/home\\gallery')).toBeUndefined()
  })

  it('builds a breadcrumb from the library root, or from / outside it', () => {
    expect(galleryPathTrail('/home/gallery', '/home/gallery/Trips/Kyoto')).toEqual([
      { name: '', path: '/home/gallery' },
      { name: 'Trips', path: '/home/gallery/Trips' },
      { name: 'Kyoto', path: '/home/gallery/Trips/Kyoto' },
    ])
    expect(galleryPathTrail('/', '/srv/media')).toEqual([
      { name: '/', path: '/' },
      { name: 'srv', path: '/srv' },
      { name: 'media', path: '/srv/media' },
    ])
  })

  it('validates album names against siblings case-insensitively', () => {
    expect(galleryAlbumNameProblem('  ')).toBe('请输入相册名称')
    expect(galleryAlbumNameProblem('.thumbs')).toBe('相册名称不能以点开头')
    expect(galleryAlbumNameProblem('a/b')).toBe('相册名称不能包含斜杠或控制字符')
    expect(galleryAlbumNameProblem('旅'.repeat(41))).toBe('相册名称过长')
    expect(galleryAlbumNameProblem('kyoto', ['Kyoto'])).toBe('已有同名相册或文件夹')
    expect(galleryAlbumNameProblem('2026 京都旅行', ['Kyoto'])).toBeUndefined()
  })

  it('never reuses an existing upload name', () => {
    expect(nextGalleryUploadName('IMG_1.jpg', new Set())).toBe('IMG_1.jpg')
    expect(nextGalleryUploadName('IMG_1.jpg', new Set(['img_1.JPG']))).toBe('IMG_1 (2).jpg')
    expect(nextGalleryUploadName('IMG_1.jpg', new Set(['IMG_1.jpg', 'IMG_1 (2).jpg']))).toBe('IMG_1 (3).jpg')
    expect(nextGalleryUploadName('README', new Set(['README']))).toBe('README (2)')
  })
})

describe('gallery preferences', () => {
  function memoryStorage(initial?: string) {
    const values = new Map<string, string>(initial === undefined ? [] : [[GALLERY_PREFERENCES_KEY, initial]])
    return {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => void values.set(key, value),
      values,
    }
  }

  it('falls back field by field and round-trips valid values', () => {
    expect(readGalleryPreferences(memoryStorage())).toEqual({ root: GALLERY_DEFAULT_ROOT, sort: 'newest', density: 'comfortable' })
    expect(readGalleryPreferences(memoryStorage('{"root":"/","sort":"oldest","density":"huge"}')))
      .toEqual({ root: GALLERY_DEFAULT_ROOT, sort: 'oldest', density: 'comfortable' })
    expect(readGalleryPreferences(memoryStorage('not json')).root).toBe(GALLERY_DEFAULT_ROOT)

    const storage = memoryStorage()
    writeGalleryPreferences(storage, { root: '/srv/photos', sort: 'name', density: 'spacious' })
    expect(readGalleryPreferences(storage)).toEqual({ root: '/srv/photos', sort: 'name', density: 'spacious' })
  })

  it('keeps working when storage throws', () => {
    const broken = {
      getItem: () => { throw new Error('denied') },
      setItem: () => { throw new Error('denied') },
    }
    expect(readGalleryPreferences(broken).root).toBe(GALLERY_DEFAULT_ROOT)
    expect(() => writeGalleryPreferences(broken, readGalleryPreferences(broken))).not.toThrow()
  })
})
