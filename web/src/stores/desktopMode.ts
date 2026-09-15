import { computed, reactive, readonly } from 'vue'
import {
  cascadePosition,
  clampToViewport,
  DEFAULT_SIDE_SPLIT_RATIO,
  normalizeGeometry,
  normalizeSideSplitRatio,
  supportsSideWindowSnap,
  type ViewportSize,
  type WindowGeometry,
  type WindowSnap,
  type WindowSnapTarget,
} from '@/lib/desktopWindowGeometry'
import { canonicalDesktopAppPath, desktopRoutePath, findDesktopApp } from '@/lib/desktopApps'

/**
 * Desktop (Windows-style) mode state.
 *
 * The desktop shell reuses the existing lazy-loaded route views as window
 * contents. This store owns only mode/open-window state and browser-local
 * window geometry; icon layout and shortcuts live in `desktopIcons.ts` and the
 * authenticated Panel workspace API.
 */

export type DesktopMode = 'classic' | 'desktop'

export interface DesktopWindowState {
  id: number
  /** Navigation route path rendered inside this window. */
  path: string
  titleKey: string
  geometry: WindowGeometry
  minimized: boolean
  maximized: boolean
  snap: WindowSnap | null
  z: number
  /** Focused window id (0 = none). */
  focusedId: number
}

interface DesktopState {
  mode: DesktopMode
  windows: DesktopWindowState[]
  focusedId: number
  sideSplitRatio: number
}

export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

const MODE_KEY = 'kejilion-panel-desktop-mode'
const WINDOWS_KEY = 'kejilion-panel-desktop-windows'
const WINDOW_PREFERENCES_KEY = 'kpanel:desktop-window-sizes:v1'
const SIDE_SPLIT_RATIO_KEY = 'kpanel:desktop-side-split:v1'
const MAX_WINDOWS = 8
const MAX_WINDOW_PREFERENCES = 32
const MAX_WINDOW_PREFERENCES_BYTES = 8_192
const MAX_WINDOW_PREFERENCE_DIMENSION = 16_384
const BASE_WINDOW_Z = 100
const MAX_WINDOW_Z = 900

interface WindowSizePreference {
  path: string
  width: number
  height: number
}

const state = reactive<DesktopState>({
  mode: 'classic',
  windows: [],
  focusedId: 0,
  sideSplitRatio: DEFAULT_SIDE_SPLIT_RATIO,
})

let nextWindowId = 1
let nextZ = BASE_WINDOW_Z
let windowPreferencesLoaded = false
let windowPreferencesStorage: StorageLike | undefined
const windowPreferences = new Map<string, WindowSizePreference>()
const sizeDirtyWindowIds = new Set<number>()

function isDesktopMode(value: unknown): value is DesktopMode {
  return value === 'classic' || value === 'desktop'
}

function isWindowRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isWindowSnap(value: unknown): value is WindowSnap {
  return value === 'left' || value === 'right'
}

function titleKeyForWindowPath(path: string): string | undefined {
  path = desktopRoutePath(path)
  if (path === '/monitoring') return 'route.monitoring'
  if (path === '/processes') return 'route.processes'
  if (path === '/system') return 'route.systemCenter'
  if (path === '/sites/environment') return 'route.environment'
  if (path.startsWith('/ai/s/') && !/^\/ai\/s\/[A-Za-z0-9_-]{1,128}$/.test(path)) return undefined
  return findDesktopApp(path)?.labelKey
}

function isSafeWindowFullPath(fullPath: string): boolean {
  return (
    fullPath.startsWith('/')
    && !fullPath.startsWith('//')
    && fullPath.length <= 768
    && !/[\u0000-\u001f\\]/.test(fullPath)
    && Boolean(titleKeyForWindowPath(fullPath))
  )
}

function windowPreferencePath(path: string): string | undefined {
  const routePath = desktopRoutePath(path)
  if (routePath === '/processes' || routePath === '/system' || routePath === '/app-script') return routePath
  return findDesktopApp(path)?.path
}

function loadWindowPreferences(storage: StorageLike | undefined): void {
  windowPreferences.clear()
  windowPreferencesLoaded = true
  windowPreferencesStorage = storage
  try {
    const raw = storage?.getItem(WINDOW_PREFERENCES_KEY) ?? null
    if (!raw || raw.length > MAX_WINDOW_PREFERENCES_BYTES) return
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return
    for (const value of parsed.filter(isWindowRecord).slice(-MAX_WINDOW_PREFERENCES)) {
      const path = typeof value.path === 'string' ? windowPreferencePath(value.path) : undefined
      if (
        !path
        || path !== value.path
        || !Number.isFinite(value.width)
        || !Number.isFinite(value.height)
        || Number(value.width) <= 0
        || Number(value.height) <= 0
        || Number(value.width) > MAX_WINDOW_PREFERENCE_DIMENSION
        || Number(value.height) > MAX_WINDOW_PREFERENCE_DIMENSION
      ) continue
      windowPreferences.delete(path)
      windowPreferences.set(path, {
        path,
        width: Math.round(Number(value.width)),
        height: Math.round(Number(value.height)),
      })
    }
  } catch {
    // Corrupt or unavailable browser storage falls back to application defaults.
  }
}

function ensureWindowPreferences(): void {
  if (!windowPreferencesLoaded) loadWindowPreferences(defaultStorage())
}

function persistWindowPreferences(): void {
  try {
    windowPreferencesStorage?.setItem(WINDOW_PREFERENCES_KEY, JSON.stringify([...windowPreferences.values()]))
  } catch {
    // Preference persistence is optional; window behaviour remains available.
  }
}

function rememberWindowPreference(path: string, geometry: WindowGeometry, viewport = defaultViewport()): void {
  ensureWindowPreferences()
  const preferencePath = windowPreferencePath(path)
  if (!preferencePath) return
  const normalized = normalizeGeometry(geometry, viewport)
  const next = {
    path: preferencePath,
    width: Math.round(normalized.width),
    height: Math.round(normalized.height),
  }
  const previous = windowPreferences.get(preferencePath)
  if (previous?.width === next.width && previous.height === next.height) return
  windowPreferences.delete(preferencePath)
  windowPreferences.set(preferencePath, next)
  while (windowPreferences.size > MAX_WINDOW_PREFERENCES) {
    const oldest = windowPreferences.keys().next().value as string | undefined
    if (!oldest) break
    windowPreferences.delete(oldest)
  }
  persistWindowPreferences()
}

function commitWindowPreference(windowState: DesktopWindowState): void {
  if (windowState.maximized || !sizeDirtyWindowIds.delete(windowState.id)) return
  rememberWindowPreference(windowState.path, windowState.geometry)
}

function readPersistedMode(storage: StorageLike | undefined): DesktopMode {
  try {
    const raw = storage?.getItem(MODE_KEY) ?? null
    return isDesktopMode(raw) ? raw : 'classic'
  } catch {
    return 'classic'
  }
}

function readPersistedWindows(
  storage: StorageLike | undefined,
  viewport: ViewportSize,
): DesktopWindowState[] {
  try {
    const raw = storage?.getItem(WINDOWS_KEY) ?? null
    if (!raw) return []
    if (raw.length > 64_000) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const restored: DesktopWindowState[] = []
    const usedIds = new Set<number>()
    const allocateId = (): number => {
      while (usedIds.has(nextWindowId)) nextWindowId += 1
      const id = nextWindowId
      nextWindowId += 1
      return id
    }
    for (const value of parsed.filter(isWindowRecord).slice(0, MAX_WINDOWS)) {
      const path = typeof value.path === 'string' ? value.path : ''
      const titleKey = titleKeyForWindowPath(path)
      if (!titleKey || !isSafeWindowFullPath(path)) continue

      const persistedId = value.id
      const canReuseId = Number.isSafeInteger(persistedId) && Number(persistedId) > 0 && !usedIds.has(Number(persistedId))
      const id = canReuseId ? Number(persistedId) : allocateId()
      usedIds.add(id)
      const maximized = Boolean(value.maximized)
      restored.push({
        id,
        path,
        titleKey,
        geometry: normalizeGeometry(
          isWindowRecord(value.geometry) ? (value.geometry as Partial<WindowGeometry>) : undefined,
          viewport,
        ),
        minimized: Boolean(value.minimized),
        maximized,
        snap: !maximized && isWindowSnap(value.snap) && supportsSideWindowSnap(viewport) ? value.snap : null,
        z: BASE_WINDOW_Z,
        focusedId: id,
      })
    }
    return restored
  } catch {
    return []
  }
}

function persistWindows(storage: StorageLike | undefined): void {
  try {
    const snapshot = state.windows
      .slice(0, MAX_WINDOWS)
      .map((windowState) => ({
        id: windowState.id,
        path: windowState.path,
        titleKey: windowState.titleKey,
        geometry: windowState.geometry,
        minimized: windowState.minimized,
        maximized: windowState.maximized,
        snap: windowState.snap,
      }))
    storage?.setItem(WINDOWS_KEY, JSON.stringify(snapshot))
  } catch {
    // Storage full/unavailable: desktop keeps working for the session.
  }
}

function persistMode(storage: StorageLike | undefined): void {
  try {
    storage?.setItem(MODE_KEY, state.mode)
  } catch {
    // Same as above: non-fatal.
  }
}

function readPersistedSideSplitRatio(
  storage: StorageLike | undefined,
  viewport: ViewportSize,
): number {
  try {
    const raw = storage?.getItem(SIDE_SPLIT_RATIO_KEY) ?? null
    if (!raw || raw.length > 32) return DEFAULT_SIDE_SPLIT_RATIO
    const ratio = Number(raw)
    if (!Number.isFinite(ratio) || ratio <= 0 || ratio >= 1) return DEFAULT_SIDE_SPLIT_RATIO
    return normalizeSideSplitRatio(ratio, viewport)
  } catch {
    return DEFAULT_SIDE_SPLIT_RATIO
  }
}

function persistSideSplitRatio(storage: StorageLike | undefined): void {
  try {
    storage?.setItem(SIDE_SPLIT_RATIO_KEY, String(state.sideSplitRatio))
  } catch {
    // The divider remains adjustable when browser storage is unavailable.
  }
}

function bringToFront(id: number): void {
  const target = state.windows.find((windowState) => windowState.id === id)
  if (!target) return
  if (nextZ >= MAX_WINDOW_Z) normalizeWindowStack()
  nextZ += 1
  target.z = nextZ
  state.focusedId = id
}

function normalizeWindowStack(): void {
  const ordered = [...state.windows].sort((a, b) => a.z - b.z)
  let z = BASE_WINDOW_Z
  for (const windowState of ordered) {
    windowState.z = z
    z += 1
  }
  nextZ = z
}

function resizeForViewport(viewport: ViewportSize, persist = true): void {
  state.sideSplitRatio = normalizeSideSplitRatio(state.sideSplitRatio, viewport)
  for (const windowState of state.windows) {
    windowState.geometry = clampToViewport(windowState.geometry, viewport)
    if (windowState.snap && !supportsSideWindowSnap(viewport)) windowState.snap = null
  }
  if (persist) {
    persistWindows(defaultStorage())
    persistSideSplitRatio(defaultStorage())
  }
}

function defaultViewport(): ViewportSize {
  return { width: window.innerWidth, height: window.innerHeight }
}

function defaultStorage(): StorageLike | undefined {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

/** Initialize from persistence. Called once on app boot. */
export function initializeDesktopMode(
  storage: StorageLike | undefined = defaultStorage(),
  viewport: ViewportSize = defaultViewport(),
): void {
  state.mode = readPersistedMode(storage)
  state.sideSplitRatio = readPersistedSideSplitRatio(storage, viewport)
  loadWindowPreferences(storage)
  const restored = readPersistedWindows(storage, viewport)
  if (restored.length) {
    state.windows.splice(0, state.windows.length, ...restored)
    // Reassign z values in insertion order so the last restored window is on top.
    let z = BASE_WINDOW_Z
    for (const windowState of state.windows) {
      windowState.z = z
      windowState.focusedId = windowState.id
      z += 1
    }
    nextZ = z
    nextWindowId = Math.max(nextWindowId, restored.reduce((max, w) => Math.max(max, w.id), 0) + 1)
    state.focusedId = [...state.windows]
      .filter((windowState) => !windowState.minimized)
      .sort((a, b) => b.z - a.z)[0]?.id ?? 0
    persistWindows(storage)
  }
}

export function useDesktopMode() {
  const windows = computed(() => state.windows)

  function enterDesktop(): void {
    if (state.mode === 'desktop') return
    state.mode = 'desktop'
    resizeForViewport(defaultViewport())
    persistMode(defaultStorage())
  }

  function enterClassic(): void {
    if (state.mode === 'classic') return
    state.mode = 'classic'
    persistMode(defaultStorage())
  }

  function toggleMode(): void {
    if (state.mode === 'desktop') enterClassic()
    else enterDesktop()
  }

  /**
   * Open a window for a route path. `allowMultiple` governs whether the same
   * path may spawn more than one window; single-instance paths (terminal) focus
   * the existing window instead. Returns 0 when the bounded window limit is
   * reached; callers surface that state rather than silently closing user work.
   */
  function openWindow(path: string, titleKey: string, allowMultiple: boolean, navigateExisting = false): number {
    if (!allowMultiple) {
      const appPath = canonicalDesktopAppPath(path)
      const existing = state.windows.find(
        (windowState) => canonicalDesktopAppPath(windowState.path) === appPath,
      )
      if (existing) {
        const safeTitleKey = titleKeyForWindowPath(path)
        if (navigateExisting && safeTitleKey && isSafeWindowFullPath(path)) {
          existing.path = path
          existing.titleKey = safeTitleKey
        }
        existing.minimized = false
        bringToFront(existing.id)
        persistWindows(defaultStorage())
        return existing.id
      }
    }
    if (state.windows.length >= MAX_WINDOWS) return 0
    const id = nextWindowId++
    nextZ += 1
    const geometry = cascadeGeometry(id, path)
    state.windows.push({
      id,
      path,
      titleKey,
      geometry,
      minimized: false,
      maximized: false,
      snap: null,
      z: nextZ,
      focusedId: id,
    })
    state.focusedId = id
    persistWindows(defaultStorage())
    return id
  }

  function cascadeGeometry(id: number, path: string): WindowGeometry {
    const index = state.windows.filter((windowState) => windowState.id !== id).length
    const viewport = defaultViewport()
    ensureWindowPreferences()
    const preferencePath = windowPreferencePath(path)
    const preference = preferencePath ? windowPreferences.get(preferencePath) : undefined
    return cascadePosition(index, viewport, preference?.width, preference?.height)
  }

  function closeWindow(id: number): void {
    const index = state.windows.findIndex((windowState) => windowState.id === id)
    if (index === -1) return
    commitWindowPreference(state.windows[index]!)
    sizeDirtyWindowIds.delete(id)
    state.windows.splice(index, 1)
    if (state.focusedId === id) {
      const top = [...state.windows]
        .filter((windowState) => !windowState.minimized)
        .sort((a, b) => b.z - a.z)[0]
      state.focusedId = top?.id ?? 0
    }
    persistWindows(defaultStorage())
  }

  function minimizeWindow(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    target.minimized = true
    if (state.focusedId === id) {
      const top = [...state.windows]
        .filter((windowState) => windowState.id !== id && !windowState.minimized)
        .sort((a, b) => b.z - a.z)[0]
      state.focusedId = top?.id ?? 0
    }
    persistWindows(defaultStorage())
  }

  function restoreWindow(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    target.minimized = false
    bringToFront(id)
    persistWindows(defaultStorage())
  }

  function toggleMinimized(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    if (target.minimized) restoreWindow(id)
    else minimizeWindow(id)
  }

  function toggleMaximize(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    target.maximized = !target.maximized
    target.snap = null
    bringToFront(id)
    persistWindows(defaultStorage())
  }

  function focusWindow(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target || target.minimized) return
    if (state.focusedId === id) return
    bringToFront(id)
  }

  function updateGeometry(id: number, geometry: WindowGeometry, persist = true): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    const previousWidth = target.geometry.width
    const previousHeight = target.geometry.height
    target.geometry = clampToViewport(geometry, defaultViewport())
    if (target.geometry.width !== previousWidth || target.geometry.height !== previousHeight) {
      sizeDirtyWindowIds.add(id)
    }
    if (persist) {
      commitWindowPreference(target)
      persistWindows(defaultStorage())
    }
  }

  function commitGeometry(id: number): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    commitWindowPreference(target)
    persistWindows(defaultStorage())
  }

  function setSideSplitRatio(
    ratio: number,
    persist = true,
    viewport: ViewportSize = defaultViewport(),
  ): void {
    state.sideSplitRatio = normalizeSideSplitRatio(ratio, viewport)
    if (persist) persistSideSplitRatio(defaultStorage())
  }

  function commitSideSplitRatio(): void {
    persistSideSplitRatio(defaultStorage())
  }

  function snapWindow(id: number, snap: WindowSnapTarget): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return
    if (snap === 'maximize') {
      target.maximized = true
      target.snap = null
    } else {
      if (!supportsSideWindowSnap(defaultViewport())) return
      target.maximized = false
      target.snap = snap
    }
    bringToFront(id)
    persistWindows(defaultStorage())
  }

  function restoreWindowForDrag(id: number, geometry: WindowGeometry): WindowGeometry | undefined {
    const target = state.windows.find((windowState) => windowState.id === id)
    if (!target) return undefined
    target.maximized = false
    target.snap = null
    target.geometry = clampToViewport(geometry, defaultViewport())
    bringToFront(id)
    return target.geometry
  }

  function updateWindowRoute(id: number, path: string, titleKey: string): void {
    const target = state.windows.find((windowState) => windowState.id === id)
    const safeTitleKey = titleKeyForWindowPath(path)
    if (!target || !isSafeWindowFullPath(path) || !safeTitleKey || safeTitleKey !== titleKey) return
    if (target.path === path && target.titleKey === safeTitleKey) return
    target.path = path
    target.titleKey = safeTitleKey
    persistWindows(defaultStorage())
  }

  return {
    state: readonly(state),
    windows,
    focusedId: computed(() => state.focusedId),
    sideSplitRatio: computed(() => state.sideSplitRatio),
    mode: computed(() => state.mode),
    enterDesktop,
    enterClassic,
    toggleMode,
    openWindow,
    closeWindow,
    minimizeWindow,
    restoreWindow,
    toggleMinimized,
    toggleMaximize,
    focusWindow,
    updateGeometry,
    commitGeometry,
    setSideSplitRatio,
    commitSideSplitRatio,
    snapWindow,
    restoreWindowForDrag,
    updateWindowRoute,
    resizeForViewport,
  }
}

export function resetDesktopModeForTest(): void {
  state.mode = 'classic'
  state.windows.splice(0, state.windows.length)
  state.focusedId = 0
  state.sideSplitRatio = DEFAULT_SIDE_SPLIT_RATIO
  nextWindowId = 1
  nextZ = BASE_WINDOW_Z
  windowPreferences.clear()
  sizeDirtyWindowIds.clear()
  windowPreferencesLoaded = false
  windowPreferencesStorage = undefined
}
