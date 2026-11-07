import { reactive } from 'vue'
import type { CinemaCheckpoint, CinemaPlaybackPreference, FileEntry } from '@/types/api'

export const MAX_CINEMA_CHECKPOINTS = 50
export const cinemaPlayback = reactive({ owner: '', entries: [] as CinemaCheckpoint[] })

export function cinemaCheckpointKey(item: Pick<CinemaCheckpoint, 'hostId' | 'path'>): string {
  return JSON.stringify([item.hostId, item.path])
}

export function parseCinemaPlayback(value: unknown): CinemaPlaybackPreference | undefined {
  if (!value || typeof value !== 'object') return undefined
  const entries = (value as { entries?: unknown }).entries
  if (!Array.isArray(entries) || entries.length > MAX_CINEMA_CHECKPOINTS) return undefined
  const result: CinemaCheckpoint[] = [], seen = new Set<string>()
  for (const item of entries) {
    if (!item || typeof item !== 'object') return undefined
    const { path, hostId, fileVersion, position, duration, completed } = item
    if (typeof path !== 'string' || !path.startsWith('/') || new TextEncoder().encode(path).length > 4096 || /[\x00-\x1f\x7f\\]/u.test(path) || path.slice(1).split('/').some(part => !part || part === '.' || part === '..') || typeof hostId !== 'string' || !/^[A-Za-z0-9_-]{0,128}$/.test(hostId) || typeof fileVersion !== 'string' || !fileVersion || new TextEncoder().encode(fileVersion).length > 256 || /[\x00-\x1f\x7f]/u.test(fileVersion) || typeof position !== 'number' || !Number.isFinite(position) || position < 0 || typeof duration !== 'number' || !Number.isFinite(duration) || duration <= 0 || duration > 31536000 || position > duration || typeof completed !== 'boolean') return undefined
    const key = cinemaCheckpointKey({ hostId, path })
    if (seen.has(key)) return undefined
    seen.add(key)
    result.push({ path, hostId, fileVersion, position, duration, completed })
  }
  return { entries: result }
}

/** Latest explicit edits first; each film update preserves other films. */
export function mergeCinemaPlayback(previous: CinemaPlaybackPreference | undefined, patch: CinemaPlaybackPreference | undefined): CinemaPlaybackPreference {
  const entries = new Map<string, CinemaCheckpoint>()
  for (const item of [...(patch?.entries ?? []), ...(previous?.entries ?? [])]) {
    const key = cinemaCheckpointKey(item)
    if (!entries.has(key)) entries.set(key, { ...item })
  }
  return { entries: [...entries.values()].slice(0, MAX_CINEMA_CHECKPOINTS) }
}

export function setCinemaPlaybackOwner(owner: string): void {
  cinemaPlayback.owner = owner
  cinemaPlayback.entries = []
}

export function applyCinemaPlayback(value: unknown, pending?: CinemaPlaybackPreference): void {
  cinemaPlayback.entries = mergeCinemaPlayback(parseCinemaPlayback(value), pending).entries
}

export function cinemaCheckpoint(entry: Pick<FileEntry, 'path' | 'resourceVersion'>, hostId: string): CinemaCheckpoint | undefined {
  return cinemaPlayback.entries.find(item => item.path === entry.path && item.hostId === hostId && item.fileVersion === entry.resourceVersion)
}

export function recordCinemaCheckpoint(item: CinemaCheckpoint, owner: string, flush = false): void {
  if (!owner || cinemaPlayback.owner !== owner) return
  const patch = parseCinemaPlayback({ entries: [item] })
  if (!patch) return
  cinemaPlayback.entries = mergeCinemaPlayback({ entries: cinemaPlayback.entries }, patch).entries
  window.dispatchEvent(new CustomEvent('kpanel:preferences-changed', { detail: { cinemaPlayback: patch } }))
  if (flush) window.dispatchEvent(new Event('kpanel:preferences-flush'))
}
