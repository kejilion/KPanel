import { describe, expect, it } from 'vitest'
import {
  GALLERY_COVER_MARKER,
  galleryRelativePath,
  isGalleryCoverPath,
  parseGalleryCover,
  resolveGalleryCover,
  serializeGalleryCover,
} from './galleryCover'

describe('gallery cover marker', () => {
  it('is a hidden json file so it never shows as media and always reads as text', () => {
    expect(GALLERY_COVER_MARKER.startsWith('.')).toBe(true)
    expect(GALLERY_COVER_MARKER.endsWith('.json')).toBe(true)
  })

  it('round-trips a cover and an automatic marker', () => {
    expect(parseGalleryCover(serializeGalleryCover('Kyoto/temple.jpg'))).toBe('Kyoto/temple.jpg')
    expect(parseGalleryCover(serializeGalleryCover('sunset.jpg'))).toBe('sunset.jpg')
    expect(parseGalleryCover(serializeGalleryCover())).toBeUndefined()
  })

  it('accepts only paths that stay below the folder', () => {
    for (const good of ['a.jpg', '山湖/日落 1.jpg', 'a/b/c.png']) expect(isGalleryCoverPath(good)).toBe(true)
    for (const bad of ['', '/etc/passwd', '../a.jpg', 'a/../b.jpg', 'a//b.jpg', './a.jpg', 'a\\b.jpg', '.hidden.jpg', 'dir/.hidden.jpg', 'a\u0000b', 'x'.repeat(1025)]) {
      expect(isGalleryCoverPath(bad), JSON.stringify(bad)).toBe(false)
    }
    expect(isGalleryCoverPath(42)).toBe(false)
  })

  it('ignores damaged, foreign or oversized markers instead of failing', () => {
    for (const text of ['', 'not json', '[]', 'null', '{"version":2,"cover":"a.jpg"}', '{"cover":"a.jpg"}', '{"version":1,"cover":"../a.jpg"}', '{"version":1,"cover":7}']) {
      expect(parseGalleryCover(text), text).toBeUndefined()
    }
    expect(parseGalleryCover(`{"version":1,"cover":"${'a'.repeat(3000)}.jpg"}`)).toBeUndefined()
  })

  it('maps between absolute paths and paths below a folder', () => {
    expect(galleryRelativePath('/home/gallery', '/home/gallery/Kyoto/temple.jpg')).toBe('Kyoto/temple.jpg')
    expect(galleryRelativePath('/home/gallery', '/home/gallery/a.jpg')).toBe('a.jpg')
    expect(galleryRelativePath('/home/gallery', '/home/gallery')).toBeUndefined()
    expect(galleryRelativePath('/home/gallery', '/home/gallery-old/a.jpg')).toBeUndefined()
    expect(galleryRelativePath('/home/gallery', '/home/other/a.jpg')).toBeUndefined()
    expect(galleryRelativePath('/', '/srv/a.jpg')).toBe('srv/a.jpg')
    expect(resolveGalleryCover('/home/gallery', 'Kyoto/temple.jpg')).toBe('/home/gallery/Kyoto/temple.jpg')
    expect(resolveGalleryCover('/', 'a.jpg')).toBe('/a.jpg')
  })
})
