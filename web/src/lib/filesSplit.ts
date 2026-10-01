import type { InjectionKey, Ref } from 'vue'

/**
 * Classic file-manager split view.
 *
 * Each pane is a complete FilesView. The primary pane keeps the application
 * route (URL, Back, deep links); the secondary pane gets its own in-memory
 * router, the same isolation desktop windows use, so two panes behave like two
 * file-manager windows side by side.
 */

export type FilesSplitRole = 'primary' | 'secondary'

export interface FilesSplitControl {
  role: FilesSplitRole
  /** The workspace is wide enough to offer a second pane. */
  available: Readonly<Ref<boolean>>
  /** The second pane is mounted. */
  open: Readonly<Ref<boolean>>
  toggle: () => void
}

export const filesSplitControlKey = Symbol('files-split-control') as InjectionKey<FilesSplitControl>

/** Workspace content width needed before the split toggle is offered. */
export const FILES_SPLIT_MIN_WIDTH = 1200
/** Below this workspace width an open split stacks its panes vertically. */
export const FILES_SPLIT_STACK_WIDTH = 880
export const FILES_SPLIT_GAP = 16
/** Below this pane width search and view controls move under the path. */
export const FILES_PANE_TOOLBAR_STACK_WIDTH = 760

export type FilesPaneDensity = 'regular' | 'compact' | 'narrow'

/** Mirrors the desktop-window breakpoints for the file list at pane width. */
export function filesPaneDensity(width: number): FilesPaneDensity {
  if (width > 0 && width < 600) return 'narrow'
  if (width > 0 && width < 840) return 'compact'
  return 'regular'
}

const storageKey = 'kpanel:files:split:v1'

export interface FilesSplitPreference {
  open: boolean
  secondaryPath?: string
}

/**
 * Keep only a `/files` location with its directory and host. A previewed file
 * is not restored, so reopening the page never reopens an editor by itself.
 */
export function normalizeFilesSplitPath(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.startsWith('/files') || value.length > 4096) return undefined
  let parsed: URL
  try {
    parsed = new URL(value, 'http://kpanel.invalid')
  } catch {
    return undefined
  }
  if (parsed.origin !== 'http://kpanel.invalid' || parsed.pathname !== '/files') return undefined
  const query = new URLSearchParams()
  const path = parsed.searchParams.get('path')
  const hostId = parsed.searchParams.get('hostId')
  if (path?.startsWith('/')) query.set('path', path)
  if (hostId) query.set('hostId', hostId)
  const search = query.toString()
  return search ? `/files?${search}` : '/files'
}

export function readFilesSplitPreference(): FilesSplitPreference {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return { open: false }
    const value = JSON.parse(raw) as Partial<FilesSplitPreference> | null
    const secondaryPath = normalizeFilesSplitPath(value?.secondaryPath)
    return { open: value?.open === true, ...(secondaryPath ? { secondaryPath } : {}) }
  } catch {
    return { open: false }
  }
}

export function writeFilesSplitPreference(value: FilesSplitPreference): void {
  try {
    const secondaryPath = normalizeFilesSplitPath(value.secondaryPath)
    window.localStorage.setItem(storageKey, JSON.stringify({
      open: value.open,
      ...(secondaryPath ? { secondaryPath } : {}),
    }))
  } catch {
    // Storage is a convenience; the split still works for this page view.
  }
}
