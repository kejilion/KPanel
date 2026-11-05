import { api, ApiError } from '@/lib/api'
import { applySyncedLocale, getLocale, t } from '@/i18n'
import { applyDesktopPreferences, desktopPreferencesSnapshot, parseDesktopSessionPreferences } from '@/stores/desktopMode'
import { useToast } from '@/stores/toast'
import type { UserPreferences, UserPreferencesPatch } from '@/types/api'
import { applyMediaLocation, legacyGalleryLocation, mediaLocationKeys, parseMediaLocation } from '@/lib/mediaLocations'

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
}
let current: Sync | undefined
const pendingKey = 'kpanel:preferences-pending:v1'

function readPending(owner: string): UserPreferencesPatch {
  try {
    const raw = sessionStorage.getItem(pendingKey)
    if (!raw || raw.length > 64000) return {}
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
      sync.loaded = true
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
            if (sameSection(key, sync.pending[key], patch[key])) delete sync.pending[key]
          }
          persist(sync)
        } catch (error) {
          if (current !== sync) return
          if (!(error instanceof ApiError && error.status === 409) || ++conflicts > 3) throw error
          const server = await api.preferences.get(requestSignal(sync))
          if (current !== sync) return
          sync.server = server
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
  Object.assign(sync.pending, patch)
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

function retry(): void {
  if (!current) return
  current.retries = 0
  schedule(current, 0)
}

/** Called before the authenticated shell mounts, so cached windows cannot launch first. */
export function startPreferencesSync(owner: string): Promise<void> {
  if (current?.owner === owner) return current.loading ?? Promise.resolve()
  stopPreferencesSync()
  const sync: Sync = {
    owner, controller: new AbortController(), initial: { locale: getLocale(), desktop: desktopPreferencesSnapshot(), galleryLocation: legacyGalleryLocation() },
    pending: readPending(owner), loaded: false, retries: 0, notified: false,
    restoreDesktop: true,
  }
  current = sync
  for (const key of mediaLocationKeys) applyMediaLocation(key, sync.pending[key] ?? sync.initial[key])
  window.addEventListener('kpanel:preferences-changed', changed)
  window.addEventListener('online', retry)
  window.addEventListener('focus', retry)
  return load(sync)
}

export function stopPreferencesSync(): void {
  if (current) {
    window.clearTimeout(current.timer)
    current.controller.abort()
  }
  current = undefined
  for (const key of mediaLocationKeys) applyMediaLocation(key, undefined)
  window.removeEventListener('kpanel:preferences-changed', changed)
  window.removeEventListener('online', retry)
  window.removeEventListener('focus', retry)
}
