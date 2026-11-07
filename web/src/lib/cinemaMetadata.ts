import type { GalleryItem } from './gallery'

export type CinemaProvider = 'tmdb' | 'tvmaze' | 'wikipedia'
export interface CinemaMetadata {
  provider: CinemaProvider
  id: number
  kind: 'movie' | 'tv'
  title: string
  originalTitle?: string
  year?: string
  overview: string
  rating?: number
  genres?: string[]
  cast?: string[]
  poster?: string
  backdrop?: string
  url: string
  fetchedAt?: number
}
export interface CinemaQuery {
  provider: CinemaProvider
  title: string
  kind: 'movie' | 'tv'
  language: 'zh-CN' | 'zh-TW' | 'en-US'
  credential?: string
  id?: number
}
export function cinemaQueryTitle(title: string) {
  const encoder = new TextEncoder()
  let value = ''; let bytes = 0
  for (const character of title.replace(/[\u0000-\u001f\u007f-\u009f]/g, ' ').trim()) {
    bytes += encoder.encode(character).length
    if (bytes > 240) break
    value += character
  }
  return value.trim()
}
export function cinemaSearchTitle(name: string) {
  const stem = name.replace(/\.[a-z0-9]{2,5}$/i, '')
  const normalizedStem = stem.replace(/[._]/g, ' ')
  const release = [...normalizedStem.matchAll(/\b((?:19|20)\d{2})\b/g)].filter(match =>
    match.index! > 0 && normalizedStem.slice(0, match.index).trim() && Number(match[1]) <= new Date().getFullYear() + 2).at(-1)
  const year = release?.[1]
  const episode = /\bS\d{1,2}E\d{1,3}\b|第.{1,3}[季集]/i.test(stem)
  const title = (release ? normalizedStem.slice(0, release.index) : normalizedStem)
    .replace(/\bS\d{1,2}(?:E\d{1,3})?\b.*$/i, '')
    .replace(/\b(?:2160p|1080p|720p|480p|WEB[ -]?DL|BluRay|BDRip|HDRip|x26[45]|HEVC|AVC)\b.*$/i, '')
    .replace(/[\[【].*?[\]】]/g, '').replace(/[()（）]/g, ' ').trim()
  return { title: cinemaQueryTitle(title), year, kind: episode ? 'tv' as const : 'movie' as const }
}
function normalized(title: string) {
  return title.replace(/\s*[（(](?:\d{4}\s*)?(?:film|TV series|电影|電影|电视剧|電視劇)[）)]$/i, '')
    .toLocaleLowerCase().replace(/[\s\p{P}\p{S}]/gu, '')
}
// Ambiguous titles and year mismatches require an explicit user choice.
export function automaticCinemaMatch(name: string, items: readonly CinemaMetadata[]) {
  const exact = cinemaExactMatches(name, items)
  return exact.length === 1 ? exact[0] : undefined
}
export function cinemaExactMatches(name: string, items: readonly CinemaMetadata[]) {
  const query = cinemaSearchTitle(name)
  return items.filter(item => (normalized(item.title) === normalized(query.title) ||
    Boolean(item.originalTitle && normalized(item.originalTitle) === normalized(query.title))) &&
    (!query.year || item.year === query.year || (query.kind === 'tv' && item.kind === 'tv' && Number(item.year) > 0 && Number(item.year) <= Number(query.year))))
}
export function cinemaMetadataKey(host: string, language: string, item: GalleryItem) {
  return JSON.stringify([host, language, item.entry.path, item.entry.resourceVersion ?? item.entry.modifiedAt])
}
export function cinemaMetadataImage(item: CinemaMetadata, backdrop = false) {
  const path = backdrop ? item.backdrop ?? item.poster : item.poster
  if (!path) return undefined
  return `/api/v1/cinema/image?${new URLSearchParams({ provider: item.provider, path, size: backdrop ? 'backdrop' : 'poster' })}`
}
export function cinemaSourceURL(item: CinemaMetadata) {
  if (!Number.isSafeInteger(item.id) || item.id <= 0) return undefined
  if (item.provider === 'tmdb') return `https://www.themoviedb.org/${item.kind === 'tv' ? 'tv' : 'movie'}/${item.id}`
  if (item.provider === 'tvmaze') return `https://www.tvmaze.com/shows/${item.id}`
  if (item.provider === 'wikipedia' && /^https:\/\/(en|zh)\.wikipedia\.org\/\?curid=\d+$/.test(item.url)) return item.url
  return undefined
}

const CACHE_KEY = 'kpanel:cinema-metadata:v1'
interface Entry { key: string; metadata: CinemaMetadata; expires: number }
const TTL = 30 * 24 * 60 * 60 * 1000
export function readCinemaMetadataCache(): Map<string, CinemaMetadata> {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw || raw.length > 2 * 1024 * 1024) return new Map()
    const entries: unknown = JSON.parse(raw)
    if (!Array.isArray(entries)) return new Map()
    return new Map(entries.slice(-200).filter((v): v is Entry =>
      v && typeof v.key === 'string' && v.key.length <= 4096 && v.expires > Date.now() && v.expires <= Date.now() + TTL &&
      v.metadata && typeof v.metadata.title === 'string' && typeof v.metadata.overview === 'string' &&
      JSON.stringify(v.metadata).length <= 20000 && Boolean(cinemaSourceURL(v.metadata)) &&
      (v.metadata.rating === undefined || typeof v.metadata.rating === 'number' && Number.isFinite(v.metadata.rating)) &&
      [v.metadata.genres, v.metadata.cast].every(list => list === undefined || Array.isArray(list) && list.length <= 8 && list.every(v => typeof v === 'string')) &&
      [v.metadata.poster, v.metadata.backdrop, v.metadata.year, v.metadata.originalTitle].every(value => value === undefined || typeof value === 'string'))
      .map(v => [v.key, v.metadata]))
  } catch { return new Map() }
}
export function saveCinemaMetadataCache(cache: ReadonlyMap<string, CinemaMetadata>) {
  const entries = [...cache].slice(-200).map(([key, metadata]) => ({ key, metadata, expires: (metadata.fetchedAt ?? Date.now()) + TTL }))
  let raw = JSON.stringify(entries)
  while (raw.length > 2 * 1024 * 1024 && entries.length) { entries.shift(); raw = JSON.stringify(entries) }
  try { localStorage.setItem(CACHE_KEY, raw); return true } catch { return false }
}

export interface CinemaMetadataSettings { enabled: boolean; provider: CinemaProvider | 'auto' }
const SETTINGS_KEY = 'kpanel:cinema-metadata-settings:v1'
const ATTEMPTS_KEY = 'kpanel:cinema-metadata-attempts:v1'
export function readCinemaMetadataSettings(): CinemaMetadataSettings {
  try {
    const value = JSON.parse(localStorage.getItem(SETTINGS_KEY) ?? '{}')
    return { enabled: value?.enabled !== false, provider: ['wikipedia', 'tvmaze'].includes(value?.provider) ? value.provider : 'auto' }
  } catch { return { enabled: true, provider: 'auto' } }
}
export function saveCinemaMetadataSettings(value: CinemaMetadataSettings) {
  try { localStorage.setItem(SETTINGS_KEY, JSON.stringify(value)); return true } catch { return false }
}
export function readCinemaMetadataAttempts(): Map<string, number> {
  try {
    const raw = localStorage.getItem(ATTEMPTS_KEY)
    if (!raw || raw.length > 256 * 1024) return new Map()
    const value: unknown = JSON.parse(raw)
    if (!Array.isArray(value)) return new Map()
    return new Map(value.slice(-1000).filter((v): v is [string, number] => Array.isArray(v) && v.length === 2 && typeof v[0] === 'string' && v[0].length <= 5000 && Number.isFinite(v[1]) && v[1] > Date.now() && v[1] <= Date.now() + TTL))
  } catch { return new Map() }
}
export function saveCinemaMetadataAttempts(attempts: ReadonlyMap<string, number>) {
  const entries = [...attempts].filter(([, until]) => until > Date.now()).slice(-1000)
  let raw = JSON.stringify(entries)
  while (raw.length > 256 * 1024 && entries.length) { entries.shift(); raw = JSON.stringify(entries) }
  try { localStorage.setItem(ATTEMPTS_KEY, raw); return true } catch { return false }
}
// Bound the live map as well as its serialized copy across directory/host changes.
export function recordCinemaMetadataAttempt(attempts: Map<string, number>, key: string, until: number) {
  const now = Date.now()
  for (const [identity, deadline] of attempts) if (deadline <= now) attempts.delete(identity)
  attempts.delete(key); attempts.set(key, until)
  while (attempts.size > 1000) attempts.delete(attempts.keys().next().value!)
  let size = 2
  for (const [identity, deadline] of [...attempts].reverse()) {
    size += JSON.stringify([identity, deadline]).length + 1
    if (size > 256 * 1024) attempts.delete(identity)
  }
}
