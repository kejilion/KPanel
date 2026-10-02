/**
 * Manual album covers. A cover is remembered in a small hidden file inside the
 * folder it belongs to, so it follows the folder across browsers, backups and
 * moves and needs no database: the file names one photo by its path relative
 * to that folder. Anything unreadable or stale simply falls back to the
 * automatic cover, so a damaged marker can never hide a photo or break a page.
 */

export const GALLERY_COVER_MARKER = '.kpanel-cover.json'
/** A marker is a few dozen bytes; anything larger is not ours and is ignored. */
export const GALLERY_COVER_MARKER_MAX_BYTES = 2048

const MAX_RELATIVE_PATH = 1024

/** A forward-slash path below the folder, with no way to climb out of it. */
export function isGalleryCoverPath(value: unknown): value is string {
  if (typeof value !== 'string' || !value || value.length > MAX_RELATIVE_PATH) return false
  if (value.startsWith('/') || /[\\\u0000-\u001f]/u.test(value)) return false
  const parts = value.split('/')
  if (parts.some((part) => !part || part === '.' || part === '..')) return false
  // Hidden files never show in the gallery, so they cannot be a cover either.
  return !parts[parts.length - 1]!.startsWith('.')
}

/** The marker's text for a cover, or for "automatic again" when `relative` is undefined. */
export function serializeGalleryCover(relative?: string): string {
  return `${JSON.stringify(relative === undefined ? { version: 1 } : { version: 1, cover: relative })}\n`
}

/** The cover path a marker names, or undefined for an automatic cover or a marker we cannot trust. */
export function parseGalleryCover(text: string): string | undefined {
  if (text.length > GALLERY_COVER_MARKER_MAX_BYTES) return undefined
  try {
    const value: unknown = JSON.parse(text)
    if (!value || typeof value !== 'object') return undefined
    const record = value as Record<string, unknown>
    return record.version === 1 && isGalleryCoverPath(record.cover) ? record.cover : undefined
  } catch {
    return undefined
  }
}

/** `path` relative to `folder`, or undefined when it is not strictly inside it. */
export function galleryRelativePath(folder: string, path: string): string | undefined {
  const prefix = folder === '/' ? '/' : `${folder}/`
  if (!path.startsWith(prefix) || path.length === prefix.length) return undefined
  const relative = path.slice(prefix.length)
  return isGalleryCoverPath(relative) ? relative : undefined
}

export function resolveGalleryCover(folder: string, relative: string): string {
  return folder === '/' ? `/${relative}` : `${folder}/${relative}`
}
