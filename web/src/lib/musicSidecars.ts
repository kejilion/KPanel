import { fileAPIForHost } from './fileHostContext'
import type { Track } from './music'
import type { FileEntry } from '@/types/api'

export interface MusicSidecars { lyrics?: string; cover?: string }
export interface SidecarSaveResult { lyrics?: string; cover?: string; saved: number; kept: number; failed: number }
const lyricLimit = 128 * 1024, coverLimit = 256 * 1024

function status(error: unknown): number | undefined {
  return error && typeof error === 'object' && 'status' in error ? Number(error.status) : undefined
}

export async function saveMusicSidecars(track: Track, assets: MusicSidecars, signal: AbortSignal): Promise<SidecarSaveResult> {
  const result: SidecarSaveResult = { saved: 0, kept: 0, failed: 0 }
  const path = track.entry.path, slash = path.lastIndexOf('/')
  const stem = path.slice(slash + 1).replace(/\.[^.]+$/, '')
  if (!path.startsWith('/') || /[\u0000-\u001f\\]/.test(path) || path.split('/').some(part => part === '.' || part === '..') || !stem || stem === '.' || stem === '..') {
    throw new Error('Invalid music file path')
  }
  const directory = path.slice(0, slash) || '/', prefix = directory === '/' ? '' : directory
  const files = fileAPIForHost(track.hostId) // Capture the song's host; never use the current directory selection.

  async function existing(target: string): Promise<FileEntry | undefined> {
    signal.throwIfAborted()
    try {
      const entry = await files.entry(target, signal)
      if (entry.path !== target || entry.kind !== 'file') throw new Error('Sidecar target is not a regular file')
      return entry
    } catch (error) {
      if (status(error) === 404) return undefined
      throw error
    }
  }
  function preserved(entry: FileEntry, limit: number) {
    return { ...(entry.sizeBytes >= 0 && entry.sizeBytes <= limit ? { path: entry.path } : {}), kept: true }
  }
  async function create(file: File, readLimit: number): Promise<{ path?: string; kept: boolean }> {
    const target = `${prefix}/${file.name}`
    const present = await existing(target)
    if (present) return preserved(present, readLimit)
    signal.throwIfAborted()
    try {
      const entry = await files.upload(directory, file, false, undefined, signal)
      if (entry.path !== target || entry.kind !== 'file' || entry.sizeBytes !== file.size) throw new Error('Invalid sidecar upload result')
      return { path: entry.path, kept: false }
    } catch (error) {
      // Another tab/process may have created it after the probe. The receiver's
      // no-overwrite commit is authoritative; only a verified file counts as kept.
      if (status(error) === 409) {
        const raced = await existing(target)
        if (raced) return preserved(raced, readLimit)
      }
      throw error
    }
  }
  function remember(kind: 'lyrics' | 'cover', outcome: { path?: string; kept: boolean }) {
    if (outcome.path) result[kind] = outcome.path
    if (outcome.kept) result.kept++
    else result.saved++
  }

  if (assets.lyrics) {
    try {
      const file = new File([assets.lyrics], `${stem}.lrc`, { type: 'text/plain;charset=utf-8' })
      if (file.size > lyricLimit) throw new Error('Lyrics exceed the save limit')
      remember('lyrics', await create(file, 256 * 1024))
    } catch (error) { if (signal.aborted) throw error; result.failed++ }
  }
  if (assets.cover) {
    try {
      let kept: FileEntry | undefined
      for (const extension of ['jpg', 'jpeg', 'png', 'webp']) {
        const target = `${prefix}/${stem}.${extension}`
        const present = await existing(target)
        if (present) { kept = present; break }
      }
      if (kept) remember('cover', preserved(kept, 8 * 1024 * 1024))
      else {
        const match = /^data:image\/(jpeg|png);base64,([A-Za-z0-9+/]+={0,2})$/.exec(assets.cover)
        if (!match || match[2]!.length > Math.ceil(coverLimit / 3) * 4) throw new Error('Invalid cover data')
        const raw = atob(match[2]!), bytes = Uint8Array.from(raw, character => character.charCodeAt(0))
        const png = match[1] === 'png'
        if (!bytes.length || bytes.length > coverLimit || (png
          ? ![137, 80, 78, 71, 13, 10, 26, 10].every((value, i) => bytes[i] === value)
          : bytes[0] !== 255 || bytes[1] !== 216 || bytes[2] !== 255)) throw new Error('Invalid cover image')
        remember('cover', await create(new File([bytes], `${stem}.${png ? 'png' : 'jpg'}`, { type: `image/${match[1]}` }), 8 * 1024 * 1024))
      }
    } catch (error) { if (signal.aborted) throw error; result.failed++ }
  }
  return result
}
