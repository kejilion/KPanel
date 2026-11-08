import { shallowReactive } from 'vue'
import { api } from './api'
import { fileAPIForHost } from './fileHostContext'
import { isMusic, type Track } from './music'
import { musicQualityKey, readMusicQuality, type MusicQuality } from './musicQuality'
import { readMusicTags, type MusicTags } from './musicTags'

export interface MusicRef { hostId: string; path: string; resourceVersion: string; sizeBytes: number }
export interface MusicInfo { tags?: Pick<MusicTags, 'title' | 'artist' | 'album'>; quality?: MusicQuality; embeddedAssets?: boolean }
export interface MusicRecord { ref: MusicRef; info: MusicInfo; schema: number; source: string; detectedAt: string }
export interface MusicCacheInput { refs?: MusicRef[]; observation?: { ref: MusicRef; info: MusicInfo } }
const memory = shallowReactive(new Map<string, MusicInfo>())
let generation = 0
export function clearMusicInfoMemory() { generation++; memory.clear() }
export function peekMusicInfo(track: Track): MusicInfo | undefined { return memory.get(musicQualityKey(track)) }
function reference(track: Track): MusicRef { return { hostId: track.hostId, path: track.entry.path, resourceVersion: track.entry.resourceVersion, sizeBytes: track.entry.sizeBytes } }
function validRef(track: Track) { return Boolean(track.entry.resourceVersion) && track.entry.path.startsWith('/') }
function remember(track: Track, info: MusicInfo) {
  const key = musicQualityKey(track)
  if (memory.size >= 256 && !memory.has(key)) memory.delete(memory.keys().next().value!)
  memory.set(key, { ...memory.get(key), ...info })
}
/** Batch only a bounded initial window; remaining rows are resolved when visible. */
export async function lookupMusicInfo(tracks: Track[], signal: AbortSignal) {
  const epoch = generation
  const selected = tracks.filter(t => validRef(t) && !peekMusicInfo(t)).slice(0, 64)
  for (let at = 0; at < selected.length && !signal.aborted; at += 32) {
    const batch = selected.slice(at, at + 32)
    try {
      const result = await api.music.cache({ refs: batch.map(reference) }, signal)
      if (signal.aborted || epoch !== generation) return
      for (const record of result.items) {
        const track = batch.find(t => JSON.stringify(reference(t)) === JSON.stringify(record.ref))
        if (track && record.schema === 1 && record.source === 'file-header') remember(track, record.info)
      }
    } catch { /* Optional disposable cache: file reads and playback still work. */ }
  }
}
export async function saveMusicInfo(track: Track, info: MusicInfo, signal: AbortSignal) {
  if (signal.aborted || !validRef(track)) return
  remember(track, info)
  try { await api.music.cache({ observation: { ref: reference(track), info } }, signal) }
  catch { /* Retry naturally on a later visit; never replace real file metadata with an error. */ }
}
export async function loadMusicInfo(original: Track, signal: AbortSignal): Promise<MusicInfo> {
  const epoch = generation
  // Restored queue references deliberately lack a version. Resolve before reusing any cache.
  if (!validRef(original)) {
    const entry = await fileAPIForHost(original.hostId).entry(original.entry.path, signal)
    if (signal.aborted || epoch !== generation || !isMusic(entry)) return {}
    original.entry = entry
  }
  const track = { ...original, entry: { ...original.entry } }
  await lookupMusicInfo([track], signal)
  if (signal.aborted || epoch !== generation) return {}
  const cached = peekMusicInfo(track) ?? {}
  if (cached.tags && cached.quality) return cached
  const [tags, quality] = await Promise.allSettled([
    cached.tags ? Promise.resolve(undefined) : readMusicTags(fileAPIForHost(track.hostId).contentUrl(track.entry.path), signal),
    cached.quality ? Promise.resolve(undefined) : readMusicQuality(track, signal),
  ])
  if (signal.aborted || epoch !== generation) return {}
  const info: MusicInfo = {}
  if (tags.status === 'fulfilled' && tags.value) {
    const { title, artist, album, picture, lyrics } = tags.value
    info.tags = { title, artist, album }; info.embeddedAssets = Boolean(picture || lyrics)
  }
  if (quality.status === 'fulfilled' && quality.value) info.quality = quality.value
  if (info.tags || info.quality) await saveMusicInfo(track, info, signal)
  return peekMusicInfo(track) ?? {}
}
