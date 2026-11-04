import type { FileEntry } from '@/types/api'
import { fileAPIForHost } from './fileHostContext'

export const MUSIC_ROOT = '/home/docker/navidrome/music'
export const MUSIC_LIMIT = 2000
const extensions = new Set(['mp3', 'flac', 'wav', 'm4a', 'aac', 'ogg', 'opus', 'aiff', 'aif', 'ape', 'alac', 'wma', 'dsf', 'dff'])
export function musicExtension(name: string): string { return name.split('.').pop()?.toLowerCase() ?? '' }
export function isMusic(entry: FileEntry): boolean { return entry.kind === 'file' && extensions.has(musicExtension(entry.name)) }
export interface Track { entry: FileEntry; hostId: string; title: string; artist: string; cover?: string; lyrics?: string }
export function tracksFromEntries(entries: FileEntry[], hostId: string): Track[] {
  return entries.filter(isMusic).map(entry => {
    const stem = entry.name.replace(/\.[^.]+$/, '')
    const split = stem.indexOf(' - ')
    const coverNames = [stem.toLowerCase(), 'cover', 'folder', 'front']
    const cover = entries.filter(e => e.kind === 'file' && e.sizeBytes <= 8 * 1024 * 1024 &&
      ['jpg', 'jpeg', 'png', 'webp'].includes(musicExtension(e.name)) &&
      coverNames.includes(e.name.replace(/\.[^.]+$/, '').toLowerCase()))
      .sort((a, b) => coverNames.indexOf(a.name.replace(/\.[^.]+$/, '').toLowerCase()) - coverNames.indexOf(b.name.replace(/\.[^.]+$/, '').toLowerCase()))[0]
    const lyrics = entries.find(e => e.kind === 'file' && e.sizeBytes <= 256 * 1024 && e.name.toLowerCase() === `${stem}.lrc`.toLowerCase())
    return { entry, hostId, title: split > 0 ? stem.slice(split + 3) : stem, artist: split > 0 ? stem.slice(0, split) : '', cover: cover?.path, lyrics: lyrics?.path }
  })
}
export async function readMusicFolder(path: string, hostId: string, signal: AbortSignal) {
  const entries: FileEntry[] = []
  let offset = 0
  let truncated = false
  while (entries.length < MUSIC_LIMIT) {
    const page = await fileAPIForHost(hostId).list(path, { offset, limit: 500 }, signal)
    signal.throwIfAborted()
    entries.push(...page.entries)
    truncated ||= Boolean(page.scanTruncated)
    if (page.nextOffset === undefined || page.nextOffset <= offset) return { entries, truncated }
    offset = page.nextOffset
  }
  return { entries, truncated: true }
}
export interface LyricLine { time: number; text: string }
export function parseLyrics(source: string): LyricLine[] {
  const lines: LyricLine[] = []
  const offset = Number(source.match(/\[offset:([+-]?\d+)\]/i)?.[1] ?? 0) / 1000
  for (const line of source.slice(0, 256 * 1024).split(/\r?\n/)) {
    const text = line.replace(/\[[^\]]*\]/g, '').trim()
    for (const match of line.matchAll(/\[(\d{1,3}):([0-5]\d)(?:[.:](\d{1,3}))?\]/g)) {
      lines.push({ time: Math.max(0, Number(match[1]) * 60 + Number(match[2]) + Number(`0.${match[3] ?? 0}`) + offset), text })
    }
  }
  return lines.sort((a, b) => a.time - b.time).slice(0, 5000)
}
export function musicTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00'
  return `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`
}
export function trackKey(track: Track): string { return JSON.stringify([track.hostId, track.entry.path]) }
