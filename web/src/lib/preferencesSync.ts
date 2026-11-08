import { applyCinemaPlayback, mergeCinemaPlayback, parseCinemaPlayback, setCinemaPlaybackOwner } from '@/lib/cinemaPlayback'
import { api, ApiError } from '@/lib/api'
import { clearMusicInfoMemory } from '@/lib/musicInfoCache'
import { applySyncedLocale, getLocale, t } from '@/i18n'
import { applyDesktopPreferences, desktopPreferencesSnapshot, parseDesktopSessionPreferences } from '@/stores/desktopMode'
import { useToast } from '@/stores/toast'
import { applyMediaLocation, legacyGalleryLocation, mediaLocationKeys, parseMediaLocation } from '@/lib/mediaLocations'
import { watch, type WatchStopHandle } from 'vue'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { isMusic, readMusicTrack, musicQueueTrack, MUSIC_LIMIT, trackKey } from '@/lib/music'
import { useMusicPlayer } from '@/stores/musicPlayer'
import type { MusicPlaybackPreference, UserPreferences, UserPreferencesPatch } from '@/types/api'

interface Sync {
  owner: string
  controller: AbortController
  initial: UserPreferencesPatch
  pending: UserPreferencesPatch
  server?: UserPreferences
  loaded: boolean
  loading?: Promise<void>
  saving?: Promise<void>
  timer?: number
  retries: number
  notified: boolean
  restoreDesktop: boolean
  musicTouched: boolean
  restoringMusic: boolean
  musicWatchers: WatchStopHandle[]
  lastMusicProgress: number
  musicSeek?: () => void
  musicRefreshing?: Promise<void>
  musicRestoreSerial: number
  lastMusicRestore?: MusicPlaybackPreference
}
let current: Sync | undefined
const pendingKey = 'kpanel:preferences-pending:v1'

function parseMusicPlayback(value: unknown): MusicPlaybackPreference | undefined {
  if (!value || typeof value !== 'object') return undefined
  const m = value as Record<string, unknown>
  if (typeof m.path !== 'string' || new TextEncoder().encode(m.path).length > 4096 || typeof m.hostId !== 'string' || !/^[A-Za-z0-9_-]{0,128}$/.test(m.hostId) || typeof m.position !== 'number' || !Number.isFinite(m.position) || m.position < 0 || m.position > 31536000 || typeof m.playing !== 'boolean' || typeof m.shuffle !== 'boolean' || !['off', 'all', 'one'].includes(String(m.repeat))) return undefined
  if (m.path === '') {
    if (m.hostId || m.position || m.playing) return undefined
  } else if (!m.path.startsWith('/') || /[\x00-\x1f\x7f\\]/u.test(m.path) || m.path.slice(1).split('/').some(p => !p || p === '.' || p === '..')) return undefined
  let queue: MusicPlaybackPreference['queue']
  if (m.queue != null) {
    if (!Array.isArray(m.queue) || m.queue.length > MUSIC_LIMIT) return undefined
    queue = []; const keys = new Set<string>(); let bytes = 0
    for (const item of m.queue) {
      if (!item || typeof item.path !== 'string' || !item.path || new TextEncoder().encode(item.path).length > 4096 || !item.path.startsWith('/') || /[\x00-\x1f\x7f\\]/u.test(item.path) || item.path.slice(1).split('/').some((p: string) => !p || p === '.' || p === '..') || typeof item.hostId !== 'string' || !/^[A-Za-z0-9_-]{0,128}$/.test(item.hostId)) return undefined
      const key = JSON.stringify([item.hostId, item.path])
      if (keys.has(key)) return undefined
      keys.add(key); bytes += new TextEncoder().encode(item.path).length + item.hostId.length
      if (bytes > 256 * 1024) return undefined
      queue.push({ path: item.path, hostId: item.hostId })
    }
  }
  return { path: m.path, hostId: m.hostId, position: m.position, playing: m.playing, repeat: m.repeat as MusicPlaybackPreference['repeat'], shuffle: m.shuffle, ...(queue ? { queue } : {}) }
}

function musicSnapshot(): MusicPlaybackPreference {
  const state = useMusicPlayer().state
  return { path: state.current?.entry.path ?? '', hostId: state.current?.hostId ?? '', position: state.current ? Math.min(31536000, Math.max(0, Number.isFinite(state.position) ? state.position : 0)) : 0, playing: !!state.current && state.playing, repeat: state.repeat, shuffle: state.shuffle, queue: state.queue.map(track => ({ path: track.entry.path, hostId: track.hostId })) }
}

function watchMusic(sync: Sync): void {
  const state = useMusicPlayer().state
  const update = () => {
    if (current !== sync || sync.restoringMusic) return
    sync.musicTouched = true
    sync.lastMusicProgress = Date.now()
    const musicPlayback = parseMusicPlayback(musicSnapshot())
    if (musicPlayback) changed(new CustomEvent('kpanel:preferences-changed', { detail: { musicPlayback } }))
    else failed(sync)
  }
  sync.musicWatchers.push(watch([() => state.current?.entry.path, () => state.current?.hostId, () => state.playing, () => state.repeat, () => state.shuffle], update, { flush: 'sync' }))
  sync.musicWatchers.push(watch(() => JSON.stringify(state.queue.map(trackKey)), update, { flush: 'sync' }))
  sync.musicWatchers.push(watch(() => state.position, () => {
    if (!state.current || sync.restoringMusic) return
    if (state.playing && Date.now() - sync.lastMusicProgress >= 30000) update()
  }, { flush: 'sync' }))
  sync.musicSeek = update
  window.addEventListener('kpanel:music-seek', update)
}

async function restoreMusic(sync: Sync, value: MusicPlaybackPreference | undefined): Promise<void> {
  const serial = ++sync.musicRestoreSerial
  const music = useMusicPlayer()
  if (sync.musicTouched || (music.state.current && (!music.state.restored || music.state.playing))) return
  if (value?.queue) {
    const queue = value.queue.map(item => musicQueueTrack(item.path, item.hostId))
    const track = value.path ? queue.find(item => item.entry.path === value.path && item.hostId === value.hostId) ?? musicQueueTrack(value.path, value.hostId) : undefined
    sync.restoringMusic = true
    try { music.restore(track, value.position, value.repeat, value.shuffle, queue); sync.lastMusicRestore = value }
    finally { sync.restoringMusic = false; sync.lastMusicProgress = Date.now() }
    return
  }
  if (!value?.path) {
    if (value && music.state.restored) {
      sync.restoringMusic = true
      try { music.stop() } finally { sync.restoringMusic = false }
    }
    sync.lastMusicRestore = value
    return
  }
  try {
    const entry = await fileAPIForHost(value.hostId).entry(value.path, requestSignal(sync))
    if (entry.path !== value.path || !isMusic(entry)) return
    const track = await readMusicTrack(entry, value.hostId, requestSignal(sync))
    if (current !== sync || serial !== sync.musicRestoreSerial || sync.controller.signal.aborted || sync.musicTouched || (music.state.current && (!music.state.restored || music.state.playing)) || entry.path !== value.path || !isMusic(entry)) return
    if (!track) return
    sync.restoringMusic = true
    try { music.restore(track, value.position, value.repeat, value.shuffle); sync.lastMusicRestore = value }
    finally { sync.restoringMusic = false; sync.lastMusicProgress = Date.now() }
  } catch { /* The saved host or file may be offline; keep the checkpoint for retry. */ }
}

function refreshMusic(sync: Sync): void {
  const music = useMusicPlayer()
  if (!sync.loaded || !sync.server || sync.musicRefreshing || sync.saving) return
  const previous = sync.server
  sync.musicRefreshing = (async () => {
    try {
      const server = await api.preferences.get(requestSignal(sync))
      if (current !== sync || sync.server !== previous || sync.saving) return
      sync.server = server
      applyCinemaPlayback(server.cinemaPlayback, sync.pending.cinemaPlayback)
      if (!sync.musicTouched && !sync.pending.musicPlayback && (!music.state.current || music.state.restored) && !sameSection('musicPlayback', sync.lastMusicRestore, server.musicPlayback)) await restoreMusic(sync, parseMusicPlayback(server.musicPlayback))
    } catch { /* A later focus or login can retry without interrupting playback. */ }
    finally { sync.musicRefreshing = undefined }
  })()
}

function readPending(owner: string): UserPreferencesPatch {
  try {
    const raw = sessionStorage.getItem(pendingKey)
    if (!raw || new TextEncoder().encode(raw).length > 1024 * 1024) return {}
    const value = JSON.parse(raw)
    if (value.owner !== owner) return {}
    const patch: UserPreferencesPatch = {}
    if (['zh-CN', 'zh-TW', 'en-US'].includes(value.patch?.locale)) patch.locale = value.patch.locale
    const desktop = parseDesktopSessionPreferences(value.patch?.desktop)
    if (desktop) patch.desktop = desktop
    for (const key of mediaLocationKeys) {
      const location = parseMediaLocation(key, value.patch?.[key])
      if (location) patch[key] = location
    }
    const cinemaPlayback = parseCinemaPlayback(value.patch?.cinemaPlayback)
    if (cinemaPlayback) patch.cinemaPlayback = cinemaPlayback
    const musicPlayback = parseMusicPlayback(value.patch?.musicPlayback)
    if (musicPlayback) patch.musicPlayback = musicPlayback
    return patch
  } catch { return {} }
}

function persist(sync: Sync): void {
  try {
    if (Object.keys(sync.pending).length) sessionStorage.setItem(pendingKey, JSON.stringify({ owner: sync.owner, patch: sync.pending }))
    else sessionStorage.removeItem(pendingKey)
  } catch { /* In-memory edits still retry when tab storage is disabled. */ }
}

function requestSignal(sync: Sync): AbortSignal {
  return AbortSignal.any([sync.controller.signal, AbortSignal.timeout(8000)])
}

function sameSection(key: keyof UserPreferencesPatch, left: unknown, right: unknown): boolean {
  if (key === 'locale') return left === right
  if (key === 'cinemaPlayback') {
    const a = parseCinemaPlayback(left), b = parseCinemaPlayback(right)
    return a !== undefined && b !== undefined && JSON.stringify(a) === JSON.stringify(b)
  }
  if (key === 'musicPlayback') {
    const a = parseMusicPlayback(left), b = parseMusicPlayback(right)
    return a !== undefined && b !== undefined && JSON.stringify(a) === JSON.stringify(b)
  }
  if (key === 'browser') return JSON.stringify(left) === JSON.stringify(right)
  if (key !== 'desktop') {
    const a = parseMediaLocation(key, left), b = parseMediaLocation(key, right)
    return a !== undefined && b !== undefined && a.path === b.path && a.hostId === b.hostId
  }
  const a = parseDesktopSessionPreferences(left)
  const b = parseDesktopSessionPreferences(right)
  return a !== undefined && b !== undefined && JSON.stringify(a) === JSON.stringify(b)
}

function failed(sync: Sync): void {
  if (current !== sync) return
  if (!sync.notified) {
    useToast().danger(t(sync.server ? 'settings.preferencesSyncFailed' : 'settings.preferencesLoadFailed'),
      t(sync.server ? 'settings.preferencesSyncRetry' : 'settings.preferencesLoadRetry'))
    sync.notified = true
  }
  if (sync.retries < 4) schedule(sync, 1000 * 2 ** sync.retries++)
}

function schedule(sync: Sync, delay = 450): void {
  window.clearTimeout(sync.timer)
  sync.timer = window.setTimeout(() => {
    if (current !== sync) return
    if (!sync.loaded) void load(sync)
    else void save(sync)
  }, delay)
}

async function load(sync: Sync): Promise<void> {
  if (sync.loading) return sync.loading
  sync.loading = (async () => {
    try {
      const server = await api.preferences.get(requestSignal(sync))
      if (current !== sync) return
      sync.server = server
      // Migrate each unconfigured section once. A competing configured value
      // cancels implicit migration; only explicit edits are retried on conflict.
      const migration: UserPreferencesPatch = {}
      if (!server.locale) migration.locale = sync.initial.locale
      if (!server.desktop) migration.desktop = sync.initial.desktop
      for (const key of mediaLocationKeys) {
        if (!server[key] && sync.initial[key]) migration[key] = sync.initial[key]
        applyMediaLocation(key, sync.pending[key] ?? server[key] ?? migration[key])
      }
      applyCinemaPlayback(server.cinemaPlayback, sync.pending.cinemaPlayback)
      sync.loaded = true
      void restoreMusic(sync, parseMusicPlayback(sync.pending.musicPlayback ?? server.musicPlayback))
      const locale = sync.pending.locale ?? sync.server?.locale ?? migration.locale
      if (locale) await applySyncedLocale(locale)
      if (current !== sync) return
      const desktop = sync.pending.desktop ?? sync.server?.desktop ?? migration.desktop
      // A failed first read may leave the shell in its cached mode. Restore on
      // a later successful read only until the user has edited that desktop.
      if (sync.restoreDesktop && desktop) applyDesktopPreferences(desktop)
      sync.restoreDesktop = false
      if (!Object.keys(migration).length && !Object.keys(sync.pending).length) {
        sync.retries = 0
        sync.notified = false
      }
      // Restore before mounting, but do not hold navigation for disk writes or
      // conflict retries. save() keeps the same bounded, versioned retry path.
      void save(sync, migration)
    } catch {
      failed(sync)
    } finally { sync.loading = undefined }
  })()
  return sync.loading
}

function save(sync: Sync, migration: UserPreferencesPatch = {}): Promise<void> {
  if (sync.saving) return sync.saving
  if (!sync.server || (!Object.keys(migration).length && !Object.keys(sync.pending).length)) return Promise.resolve()
  window.clearTimeout(sync.timer)
  sync.saving = (async () => {
    let conflicts = 0
    try {
      while (current === sync && sync.server && (Object.keys(migration).length || Object.keys(sync.pending).length)) {
        const patch = { ...migration, ...sync.pending }
        try {
          const result = await api.preferences.update({ ...patch, expectedResourceVersion: sync.server.resourceVersion }, requestSignal(sync))
          if (current !== sync) return
          sync.server = result
          migration = {}
          for (const key of Object.keys(patch) as (keyof UserPreferencesPatch)[]) {
            if (key === 'cinemaPlayback') {
              // A write may acknowledge B while a newer local edit to A is in
              // flight. Retrying the whole old batch would overwrite remote B.
              const sent = new Map((patch.cinemaPlayback?.entries ?? []).map(item => [JSON.stringify([item.hostId, item.path]), JSON.stringify(item)]))
              const remaining = (sync.pending.cinemaPlayback?.entries ?? []).filter(item => sent.get(JSON.stringify([item.hostId, item.path])) !== JSON.stringify(item))
              if (remaining.length) sync.pending.cinemaPlayback = { entries: remaining }
              else delete sync.pending.cinemaPlayback
              continue
            }
            if (sameSection(key, sync.pending[key], patch[key])) delete sync.pending[key]
          }
          applyCinemaPlayback(result.cinemaPlayback, sync.pending.cinemaPlayback)
          persist(sync)
        } catch (error) {
          if (current !== sync) return
          if (!(error instanceof ApiError && error.status === 409) || ++conflicts > 3) throw error
          const server = await api.preferences.get(requestSignal(sync))
          if (current !== sync) return
          sync.server = server
          applyCinemaPlayback(server.cinemaPlayback, sync.pending.cinemaPlayback)
          if (server.locale) delete migration.locale
          if (server.desktop) delete migration.desktop
          for (const key of mediaLocationKeys) if (server[key]) {
            if (migration[key] && !sync.pending[key]) applyMediaLocation(key, server[key])
            delete migration[key]
          }
          // Do not remount live windows on a conflict; the next login/refresh
          // restores server state. Only dirty sections are written back.
        }
      }
      sync.retries = 0
      sync.notified = false
    } catch {
      // Retry implicit migration from a fresh GET, never a stale full snapshot.
      if (Object.keys(migration).length) sync.loaded = false
      failed(sync)
    }
    finally { sync.saving = undefined }
  })()
  return sync.saving
}

function changed(event: Event): void {
  const sync = current
  if (!sync) return
  const patch = (event as CustomEvent<UserPreferencesPatch>).detail
  if (patch.desktop) sync.restoreDesktop = false
  const { cinemaPlayback, ...other } = patch
  Object.assign(sync.pending, other)
  const cinema = parseCinemaPlayback(cinemaPlayback)
  if (cinema) sync.pending.cinemaPlayback = mergeCinemaPlayback(sync.pending.cinemaPlayback, cinema)
  // Do not erase an undo while an older write is in flight: its response may
  // soon change the server away from the value the user just restored.
  if (!sync.saving && sync.server) {
    for (const key of Object.keys(sync.pending) as (keyof UserPreferencesPatch)[]) {
      if (sameSection(key, sync.pending[key], sync.server[key])) delete sync.pending[key]
    }
  }
  persist(sync)
  sync.retries = 0
  if (sync.loaded && !Object.keys(sync.pending).length) {
    sync.notified = false
    window.clearTimeout(sync.timer)
    return
  }
  schedule(sync)
}

function flush(): void {
  if (!current) return
  if (current.loaded) void save(current)
  else void load(current)
}

function retry(): void {
  if (!current) return
  current.retries = 0
  schedule(current, 0)
  refreshMusic(current)
}

/** Called before the authenticated shell mounts, so cached windows cannot launch first. */
export function startPreferencesSync(owner: string): Promise<void> {
  if (current?.owner === owner) return current.loading ?? Promise.resolve()
  stopPreferencesSync()
  const sync: Sync = {
    owner, controller: new AbortController(), initial: { locale: getLocale(), desktop: desktopPreferencesSnapshot(), galleryLocation: legacyGalleryLocation() },
    pending: readPending(owner), loaded: false, retries: 0, notified: false,
    restoreDesktop: true, musicTouched: false, restoringMusic: false, musicWatchers: [], lastMusicProgress: Date.now(), musicRestoreSerial: 0,
  }
  current = sync
  setCinemaPlaybackOwner(owner)
  for (const key of mediaLocationKeys) applyMediaLocation(key, sync.pending[key] ?? sync.initial[key])
  window.addEventListener('kpanel:preferences-changed', changed)
  window.addEventListener('kpanel:preferences-flush', flush)
  window.addEventListener('online', retry)
  window.addEventListener('focus', retry)
  watchMusic(sync)
  return load(sync)
}

export function stopPreferencesSync(): void {
  clearMusicInfoMemory()
  if (current) {
    window.clearTimeout(current.timer)
    current.controller.abort()
    for (const stop of current.musicWatchers) stop()
    if (current.musicSeek) window.removeEventListener('kpanel:music-seek', current.musicSeek)
  }
  current = undefined
  setCinemaPlaybackOwner('')
  for (const key of mediaLocationKeys) applyMediaLocation(key, undefined)
  window.removeEventListener('kpanel:preferences-changed', changed)
  window.removeEventListener('kpanel:preferences-flush', flush)
  window.removeEventListener('online', retry)
  window.removeEventListener('focus', retry)
}
