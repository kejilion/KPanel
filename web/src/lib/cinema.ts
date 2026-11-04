import { fileAPIForHost } from './fileHostContext'
import { galleryFileExtension, galleryMediaKind, galleryParentPath, joinGalleryPath, type GalleryItem } from './gallery'
import type { FileEntry } from '@/types/api'

export const CINEMA_ROOT = '/home/videos'
const DIRECTORY_LIMIT = 40
const ENTRY_LIMIT = 4000
const SETTINGS_KEY = 'kpanel:cinema:v1'
const IMAGE_EXTENSIONS = new Set(['jpg', 'jpeg', 'png', 'webp', 'avif'])

export function cinemaTitle(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot > 0 ? name.slice(0, dot) : name
}

export function cinemaRenameTarget(entry: FileEntry, title: string): string | undefined {
  const name = title.trim()
  if (!name || name.startsWith('.') || /[\/\\\u0000-\u001f\u007f]/u.test(name)) return undefined
  const fileName = name + entry.name.slice(cinemaTitle(entry.name).length)
  if (new TextEncoder().encode(fileName).length > 255) return undefined
  return joinGalleryPath(galleryParentPath(entry.path), fileName)
}

export function cinemaImage(entry: Pick<FileEntry, 'name' | 'kind'>): boolean {
  return entry.kind === 'file' && !entry.name.startsWith('.') && IMAGE_EXTENSIONS.has(galleryFileExtension(entry.name))
}
export function automaticCinemaCover(item: GalleryItem, images: readonly FileEntry[], solo: boolean): FileEntry | undefined {
  const stem = cinemaTitle(item.entry.name).toLowerCase()
  const candidates = [`${stem}-poster`, stem, ...(solo ? ['poster', 'folder', 'cover'] : [])]
  for (const candidate of candidates) {
    const image = images.find(entry => cinemaTitle(entry.name).toLowerCase() === candidate)
    if (image) return image
  }
  return undefined
}

export interface CinemaFolder {
  films: GalleryItem[]
  images: Map<string, FileEntry[]>
  folders: FileEntry[]
  unreadable: number
  truncated: boolean
}

export async function readCinemaFolder(root: string, host: string, signal: AbortSignal): Promise<CinemaFolder> {
  const files = fileAPIForHost(host)
  const result: CinemaFolder = { films: [], images: new Map(), folders: [], unreadable: 0, truncated: false }
  const pending = [root]
  let count = 0
  for (let index = 0; index < pending.length && count < ENTRY_LIMIT; index++) {
    const folder = pending[index]!
    let offset = 0
    try {
      while (count < ENTRY_LIMIT) {
        const page = await files.list(folder, { offset, limit: Math.min(250, ENTRY_LIMIT - count) }, signal)
        signal.throwIfAborted()
        count += page.entries.length
        result.truncated ||= Boolean(page.scanTruncated)
        for (const entry of page.entries) {
          if (entry.name.startsWith('.') || galleryParentPath(entry.path) !== folder) continue
          if (entry.kind === 'directory') {
            if (folder === root) result.folders.push(entry)
            if (root === '/') continue
            if (pending.length < DIRECTORY_LIMIT) pending.push(entry.path)
            else result.truncated = true
          } else if (cinemaImage(entry)) {
            const images = result.images.get(folder) ?? []
            images.push(entry)
            result.images.set(folder, images)
          } else if (galleryMediaKind(entry) === 'video') {
            result.films.push({ entry, folder, kind: 'video', time: Date.parse(entry.modifiedAt) || 0 })
          }
        }
        if (page.nextOffset === undefined || page.nextOffset <= offset) break
        offset = page.nextOffset
      }
    } catch (error) {
      if (signal.aborted || folder === root) throw error
      result.unreadable++
    }
  }
  result.truncated ||= count >= ENTRY_LIMIT
  return result
}

export interface CinemaPreferences {
  layout: 'poster' | 'wide'
  featured: string
  covers: [string, string][]
}

function settingsScope(host: string, path: string): string {
  return JSON.stringify([host, path])
}

function readSettings(): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}')
    if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  } catch { /* Browser storage may be disabled. */ }
  return {}
}

export function readCinemaPreferences(host: string, path: string): CinemaPreferences {
  const value = readSettings()[settingsScope(host, path)] as Partial<CinemaPreferences> | undefined
  return {
    layout: value?.layout === 'wide' ? 'wide' : 'poster',
    featured: typeof value?.featured === 'string' ? value.featured : '',
    covers: Array.isArray(value?.covers) ? value.covers.slice(0, 100).filter(
      pair => Array.isArray(pair) && pair.length === 2 && pair.every(part => typeof part === 'string' && part.length <= 1300),
    ) : [],
  }
}

export function writeCinemaPreferences(host: string, path: string, value: CinemaPreferences): void {
  const key = settingsScope(host, path)
  try {
    const other = Object.entries(readSettings()).filter(([saved]) => saved !== key).slice(-23)
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(Object.fromEntries([...other, [key, value]])))
  } catch { /* Keep the current view usable when storage is full or disabled. */ }
}
