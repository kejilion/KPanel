import type { FileEntry } from '@/types/api'

/**
 * Media gallery model. The gallery is a presentation of ordinary host folders:
 * the library root holds loose media, each direct subfolder is an album, and
 * every file the gallery shows or uploads stays a normal file that the file
 * manager can browse, move and back up. Nothing here stores a second index.
 */

export type GalleryMediaKind = 'image' | 'video'
export type GalleryFilter = 'all' | GalleryMediaKind
export type GallerySort = 'newest' | 'oldest' | 'name'
export type GalleryDensity = 'compact' | 'comfortable' | 'spacious'

export interface GalleryPreferences {
  root: string
  sort: GallerySort
  density: GalleryDensity
}

export interface GalleryItem {
  entry: FileEntry
  kind: GalleryMediaKind
  /** Folder that directly contains the file. */
  folder: string
  /** Modification time in milliseconds; 0 when the server time is unreadable. */
  time: number
}

export interface GalleryMonthGroup {
  key: string
  /** First day of the month in local time, or undefined for the undated group. */
  month?: Date
  items: GalleryItem[]
}

export type GalleryTileSource =
  | { type: 'thumbnail' }
  | { type: 'original' }
  | { type: 'video' }
  | { type: 'none' }

interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export const GALLERY_DEFAULT_ROOT = '/home/gallery'
export const GALLERY_PREFERENCES_KEY = 'kpanel:gallery:v1'
/** Matches the Agent thumbnail source limit (internal/agent/files.go). */
export const GALLERY_THUMBNAIL_SOURCE_MAX_BYTES = 12 * 1024 * 1024
/**
 * Originals at or below this size stand in for the cover and featured tiles,
 * which are far larger than the Agent's 320x210 thumbnail box.
 */
export const GALLERY_ORIGINAL_TILE_MAX_BYTES = 12 * 1024 * 1024
/** Formats without a server thumbnail load the original only up to this size. */
export const GALLERY_ORIGINAL_FALLBACK_MAX_BYTES = 16 * 1024 * 1024
export const GALLERY_ALBUM_NAME_MAX_BYTES = 120

const defaultPreferences: GalleryPreferences = {
  root: GALLERY_DEFAULT_ROOT,
  sort: 'newest',
  density: 'comfortable',
}

const imageExtensions = new Set([
  'jpg', 'jpeg', 'jpe', 'jfif', 'png', 'gif', 'webp', 'avif', 'bmp', 'heic', 'heif', 'tif', 'tiff',
])
const videoExtensions = new Set(['mp4', 'm4v', 'mov', 'webm', 'mkv', 'ogv', 'avi', '3gp'])
/** Image formats that every supported browser decodes in an <img>. */
const browserImageExtensions = new Set(['jpg', 'jpeg', 'jpe', 'jfif', 'png', 'gif', 'webp', 'avif', 'bmp'])
/** Image formats the Agent can scale down (it sniffs JPEG, PNG and GIF content). */
const thumbnailExtensions = new Set(['jpg', 'jpeg', 'jpe', 'jfif', 'png', 'gif'])
/** Containers that usually carry browser-playable H.264/VP9/AV1 streams. */
const browserVideoExtensions = new Set(['mp4', 'm4v', 'mov', 'webm', 'ogv'])

export function galleryFileExtension(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot > 0 && dot < name.length - 1 ? name.slice(dot + 1).toLowerCase() : ''
}

/**
 * Classify a directory entry. Host MIME tables differ (a minimal container may
 * report `.mp4` as application/octet-stream), so the extension decides first
 * and the MIME type only fills in unknown extensions. SVG stays out because
 * the file service serves it as inert text rather than an image.
 */
export function galleryMediaKind(entry: Pick<FileEntry, 'name' | 'kind' | 'mime'>): GalleryMediaKind | undefined {
  if (entry.kind !== 'file' || entry.name.startsWith('.')) return undefined
  const extension = galleryFileExtension(entry.name)
  if (imageExtensions.has(extension)) return 'image'
  if (videoExtensions.has(extension)) return 'video'
  if (extension === 'svg' || entry.mime === 'image/svg+xml') return undefined
  if (entry.mime?.startsWith('image/')) return 'image'
  if (entry.mime?.startsWith('video/')) return 'video'
  return undefined
}

export function galleryBrowserCanShow(item: Pick<GalleryItem, 'entry' | 'kind'>): boolean {
  const extension = galleryFileExtension(item.entry.name)
  if (item.kind === 'video') return browserVideoExtensions.has(extension) || (!extension && Boolean(item.entry.mime?.startsWith('video/')))
  return browserImageExtensions.has(extension)
}

/** Pick the lightest source that still looks right at the requested tile size. */
export function galleryTileSource(item: Pick<GalleryItem, 'entry' | 'kind'>, large = false): GalleryTileSource {
  if (item.kind === 'video') return galleryBrowserCanShow(item) ? { type: 'video' } : { type: 'none' }
  const { sizeBytes } = item.entry
  if (!galleryBrowserCanShow(item) || sizeBytes <= 0) return { type: 'none' }
  const thumbnailable = thumbnailExtensions.has(galleryFileExtension(item.entry.name))
    && sizeBytes <= GALLERY_THUMBNAIL_SOURCE_MAX_BYTES
  if (large && sizeBytes <= GALLERY_ORIGINAL_TILE_MAX_BYTES) return { type: 'original' }
  if (thumbnailable) return { type: 'thumbnail' }
  return sizeBytes <= GALLERY_ORIGINAL_FALLBACK_MAX_BYTES ? { type: 'original' } : { type: 'none' }
}

export function galleryItemsFromEntries(entries: readonly FileEntry[], folder: string): GalleryItem[] {
  const items: GalleryItem[] = []
  for (const entry of entries) {
    const kind = galleryMediaKind(entry)
    if (!kind) continue
    const time = Date.parse(entry.modifiedAt)
    items.push({ entry, kind, folder, time: Number.isFinite(time) ? time : 0 })
  }
  return items
}

/** Album folders: visible directories only; dot folders hold tool state such as trash. */
export function galleryAlbumEntries(entries: readonly FileEntry[]): FileEntry[] {
  return entries.filter((entry) => entry.kind === 'directory' && !entry.name.startsWith('.'))
}

const nameCollator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function sortGalleryItems(items: readonly GalleryItem[], sort: GallerySort): GalleryItem[] {
  const sorted = [...items]
  if (sort === 'name') {
    sorted.sort((left, right) => nameCollator.compare(left.entry.name, right.entry.name)
      || left.entry.path.localeCompare(right.entry.path))
    return sorted
  }
  const direction = sort === 'newest' ? -1 : 1
  sorted.sort((left, right) => (left.time - right.time) * direction
    || nameCollator.compare(left.entry.name, right.entry.name)
    || left.entry.path.localeCompare(right.entry.path))
  return sorted
}

export function filterGalleryItems(
  items: readonly GalleryItem[],
  filter: GalleryFilter,
  query = '',
): GalleryItem[] {
  const needle = query.trim().toLocaleLowerCase()
  return items.filter((item) => (filter === 'all' || item.kind === filter)
    && (!needle || item.entry.name.toLocaleLowerCase().includes(needle)))
}

/**
 * Group already-sorted items by local calendar month. A name-sorted view keeps
 * a single group because month headings would interrupt the alphabetic order.
 */
export function groupGalleryItemsByMonth(items: readonly GalleryItem[], sort: GallerySort): GalleryMonthGroup[] {
  if (!items.length) return []
  if (sort === 'name') return [{ key: 'all', items: [...items] }]
  const groups: GalleryMonthGroup[] = []
  let current: GalleryMonthGroup | undefined
  for (const item of items) {
    const date = item.time > 0 ? new Date(item.time) : undefined
    const key = date ? `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}` : 'undated'
    if (!current || current.key !== key) {
      current = { key, month: date ? new Date(date.getFullYear(), date.getMonth(), 1) : undefined, items: [] }
      groups.push(current)
    }
    current.items.push(item)
  }
  return groups
}

export function formatGalleryMonth(month: Date | undefined, locale: string): string {
  if (!month) return ''
  return new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long' }).format(month)
}

export function formatGalleryDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return ''
  const total = Math.round(seconds)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const rest = String(total % 60).padStart(2, '0')
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${rest}` : `${minutes}:${rest}`
}

/** Canonical absolute folder path, or undefined. The filesystem root is refused on purpose. */
export function normalizeGalleryRoot(value: string): string | undefined {
  const trimmed = value.trim().replace(/\/+$/u, '')
  if (!trimmed.startsWith('/') || trimmed.length > 1024 || /[\u0000-\u001f\\]/u.test(trimmed)) return undefined
  const parts = trimmed.slice(1).split('/')
  if (!parts.length || parts.some((part) => !part || part === '.' || part === '..')) return undefined
  return `/${parts.join('/')}`
}

export function joinGalleryPath(parent: string, name: string): string {
  return parent === '/' ? `/${name}` : `${parent}/${name}`
}

export function galleryParentPath(path: string): string {
  const index = path.lastIndexOf('/')
  return index <= 0 ? '/' : path.slice(0, index)
}

export function galleryBaseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1) || '/'
}

export function isWithinGalleryRoot(root: string, path: string): boolean {
  return path === root || path.startsWith(`${root}/`)
}

/** Breadcrumb trail from the library root to the folder; folders elsewhere start at /. */
export function galleryPathTrail(root: string, path: string): Array<{ name: string; path: string }> {
  const base = isWithinGalleryRoot(root, path) ? root : '/'
  const trail = [{ name: base === '/' ? '/' : '', path: base }]
  const rest = path === base ? '' : path.slice(base === '/' ? 1 : base.length + 1)
  let cursor = base
  for (const name of rest.split('/').filter(Boolean)) {
    cursor = joinGalleryPath(cursor, name)
    trail.push({ name, path: cursor })
  }
  return trail
}

/** Returns a Chinese phrase key describing why an album name is unusable. */
export function galleryAlbumNameProblem(name: string, existing: Iterable<string> = []): string | undefined {
  const value = name.trim()
  if (!value) return '请输入相册名称'
  if (value === '.' || value === '..' || value.startsWith('.')) return '相册名称不能以点开头'
  if (/[/\\\u0000-\u001f]/u.test(value)) return '相册名称不能包含斜杠或控制字符'
  if (new TextEncoder().encode(value).length > GALLERY_ALBUM_NAME_MAX_BYTES) return '相册名称过长'
  const lowered = value.toLocaleLowerCase()
  for (const item of existing) {
    if (item.toLocaleLowerCase() === lowered) return '已有同名相册或文件夹'
  }
  return undefined
}

/**
 * Next free name in the "photo (2).jpg" style. Uploads never overwrite: the
 * gallery treats an existing file of the same name as a different photo.
 */
export function nextGalleryUploadName(name: string, taken: ReadonlySet<string>): string {
  const lowered = new Set([...taken].map((value) => value.toLocaleLowerCase()))
  if (!lowered.has(name.toLocaleLowerCase())) return name
  const dot = name.lastIndexOf('.')
  const stem = dot > 0 ? name.slice(0, dot) : name
  const extension = dot > 0 ? name.slice(dot) : ''
  for (let index = 2; index < 10_000; index += 1) {
    const candidate = `${stem} (${index})${extension}`
    if (!lowered.has(candidate.toLocaleLowerCase())) return candidate
  }
  return `${stem} (${Date.now()})${extension}`
}

/** Whether a dropped or picked file belongs in the gallery. */
export function isGalleryUploadCandidate(file: Pick<File, 'name' | 'type'>): boolean {
  return Boolean(galleryMediaKind({ name: file.name, kind: 'file', mime: file.type || undefined }))
}

export const GALLERY_UPLOAD_ACCEPT = [
  'image/*', 'video/*',
  ...[...imageExtensions, ...videoExtensions].map((extension) => `.${extension}`),
].join(',')

function isSort(value: unknown): value is GallerySort {
  return value === 'newest' || value === 'oldest' || value === 'name'
}

function isDensity(value: unknown): value is GalleryDensity {
  return value === 'compact' || value === 'comfortable' || value === 'spacious'
}

export function readGalleryPreferences(storage: StorageLike | undefined): GalleryPreferences {
  try {
    const raw = storage?.getItem(GALLERY_PREFERENCES_KEY)
    if (!raw || raw.length > 4096) return { ...defaultPreferences }
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return { ...defaultPreferences }
    const value = parsed as Record<string, unknown>
    return {
      root: (typeof value.root === 'string' && normalizeGalleryRoot(value.root)) || defaultPreferences.root,
      sort: isSort(value.sort) ? value.sort : defaultPreferences.sort,
      density: isDensity(value.density) ? value.density : defaultPreferences.density,
    }
  } catch {
    return { ...defaultPreferences }
  }
}

export function writeGalleryPreferences(storage: StorageLike | undefined, preferences: GalleryPreferences): void {
  try {
    storage?.setItem(GALLERY_PREFERENCES_KEY, JSON.stringify(preferences))
  } catch {
    // Private browsing can refuse storage; the current session keeps working.
  }
}

export function browserStorage(): StorageLike | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage
  } catch {
    return undefined
  }
}
