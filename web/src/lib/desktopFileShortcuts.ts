import type { DesktopWorkspaceDraft } from '@/stores/desktopIcons'
import { useDesktopIcons } from '@/stores/desktopIcons'
import type { DesktopShortcut, DesktopWorkspaceUpdate, FileEntry } from '@/types/api'

export const DESKTOP_FILE_DRAG_TYPE = 'application/x-kpanel-desktop-file-shortcut'
export const CROSS_PANEL_FILE_DRAG_TYPE = 'application/x-kpanel-cross-panel-file-v1'
export const CROSS_PANEL_FILES_DRAG_TYPE = 'application/x-kpanel-cross-panel-files-v2'
export const CROSS_PANEL_FILES_TEXT_PREFIX = 'KPanel cross-panel files v2\n'
export const NATIVE_FILE_DOWNLOAD_DRAG_TYPE = 'DownloadURL'
export const MAX_DESKTOP_SHORTCUTS = 64
export const MAX_CROSS_PANEL_DRAG_ENTRIES = 64

export type DesktopFileEntry = Pick<FileEntry, 'name' | 'path' | 'kind'>
  & Partial<Pick<FileEntry, 'mime' | 'resourceVersion'>>

export interface CrossPanelFileDragEntry extends DesktopFileEntry {
  kind: 'file' | 'directory'
  resourceVersion: string
  sourceNodeId: string
  version: 1
}

export interface CrossPanelFileDragItem extends DesktopFileEntry {
  kind: 'file' | 'directory'
  resourceVersion: string
}

export interface CrossPanelFileDragPayload {
  sourceNodeId: string
  entries: CrossPanelFileDragItem[]
}
export type DesktopFileShortcutInput = DesktopWorkspaceUpdate['shortcuts'][number]

export interface DesktopFileShortcutAddResult {
  added: DesktopFileShortcutInput[]
  duplicates: DesktopShortcut[]
  ignored: DesktopFileEntry[]
}

export class DesktopShortcutLimitError extends Error {
  constructor(readonly available: number, readonly requested: number) {
    super('desktop_shortcut_limit')
  }
}

interface ActiveDesktopFileDrag {
  token: string
  entries: DesktopFileEntry[]
  origin: DesktopFileDragOrigin
}

export type DesktopFileDragOrigin = 'file-manager' | 'desktop-shortcut'

let activeDrag: ActiveDesktopFileDrag | undefined

function randomToken(): string {
  const bytes = new Uint8Array(16)
  globalThis.crypto?.getRandomValues?.(bytes)
  if (bytes.some(Boolean)) return [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`
}

function supportedEntry(entry: DesktopFileEntry): entry is DesktopFileEntry & { kind: 'file' | 'directory' } {
  return entry.kind === 'file' || entry.kind === 'directory'
}

function nativeDownloadName(name: string): string {
  const cleaned = name.replace(/[\u0000-\u001f<>:"/\\|?*]/g, '_').replace(/[ .]+$/u, '').trim()
  const characters = [...(cleaned || 'download')]
  const bounded = characters.length <= 180 ? characters.join('') : characters.slice(0, 180).join('')
  const stem = bounded.replace(/\.[^.]*$/u, '').toUpperCase()
  return /^(?:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(stem) ? `_${bounded}` : bounded
}

function nativeDownloadMime(mime?: string): string {
  const value = mime?.trim() || ''
  return /^[A-Za-z0-9!#$&^_.+-]+\/[A-Za-z0-9!#$&^_.+-]+$/.test(value)
    ? value
    : 'application/octet-stream'
}

function nativeDownloadTarget(
  downloadURL: string,
  pageURL = globalThis.location?.href,
): URL | undefined {
  if (!downloadURL || !pageURL) return undefined
  try {
    const page = new URL(pageURL)
    const target = new URL(downloadURL, page)
    if (!['http:', 'https:'].includes(target.protocol) || target.origin !== page.origin) return undefined
    if (target.username || target.password) return undefined
    return target
  } catch {
    return undefined
  }
}

export function nativeArchiveDownloadName(
  entries: readonly DesktopFileEntry[],
  batchName: string,
): string | undefined {
  const supported = entries.filter(supportedEntry)
  if (!supported.length || supported.length !== entries.length) return undefined
  const sourceName = supported.length === 1 && supported[0]!.kind === 'directory'
    ? supported[0]!.name
    : batchName
  const cleaned = nativeDownloadName(sourceName).replace(/\.zip$/iu, '') || 'download'
  return `${cleaned}.zip`
}

function nativeDownloadDragDescriptor(
  name: string,
  mime: string | undefined,
  downloadURL: string,
  pageURL = globalThis.location?.href,
): string | undefined {
  const target = nativeDownloadTarget(downloadURL, pageURL)
  return target ? `${nativeDownloadMime(mime)}:${nativeDownloadName(name)}:${target.href}` : undefined
}

export function nativeFileDownloadDragDescriptor(
  entry: DesktopFileEntry,
  downloadURL: string,
  pageURL = globalThis.location?.href,
): string | undefined {
  if (entry.kind !== 'file' || !downloadURL || !pageURL) return undefined
  return nativeDownloadDragDescriptor(entry.name, entry.mime, downloadURL, pageURL)
}

export function nativeArchiveDownloadDragDescriptor(
  entries: readonly DesktopFileEntry[],
  downloadURL: string,
  archiveName: string,
  pageURL = globalThis.location?.href,
): string | undefined {
  if (!entries.length || entries.some((entry) => !supportedEntry(entry)) || !archiveName.endsWith('.zip')) {
    return undefined
  }
  return nativeDownloadDragDescriptor(archiveName, 'application/zip', downloadURL, pageURL)
}

function addNativeDownloadDrag(
  dataTransfer: DataTransfer,
  entries: readonly DesktopFileEntry[],
  downloadURL?: string,
  archiveName?: string,
): void {
  if (!downloadURL) return
  const target = nativeDownloadTarget(downloadURL)
  if (!target) return
  const descriptor = archiveName
    ? nativeArchiveDownloadDragDescriptor(entries, downloadURL, archiveName)
    : entries.length === 1
      ? nativeFileDownloadDragDescriptor(entries[0]!, downloadURL)
      : undefined
  if (!descriptor) return
  try {
    // Chromium turns this private drag type into a promised file for Windows
    // Explorer and macOS Finder. Other browsers ignore the extra type while
    // KPanel's existing same-page and cross-panel payloads remain available.
    dataTransfer.setData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE, descriptor)
  } catch {
    // A browser rejecting the private type must not disable internal dragging.
  }
  try {
    // Keep the standard URI fallback for Windows Explorer and browsers that do
    // not consume Chromium's DownloadURL type.
    dataTransfer.setData('text/uri-list', `${target.href}\r\n`)
  } catch {
    // A browser rejecting the fallback must not disable internal dragging.
  }
}

function cleanShortcutName(entry: DesktopFileEntry): string {
  const cleaned = entry.name.replace(/[\s\p{Cc}]+/gu, ' ').trim()
  const fallback = entry.kind === 'directory' ? '文件夹' : '文件'
  const characters = [...(cleaned || fallback)]
  return characters.length <= 48 ? characters.join('') : `${characters.slice(0, 47).join('')}…`
}

function targetKey(targetType: DesktopFileShortcutInput['targetType'], path?: string): string {
  return `${targetType}\0${path || ''}`
}

export async function addFileEntriesToDesktop(
  entries: readonly DesktopFileEntry[],
  place?: (
    draft: DesktopWorkspaceDraft,
    added: readonly DesktopFileShortcutInput[],
  ) => void,
): Promise<DesktopFileShortcutAddResult> {
  const desktopIcons = useDesktopIcons()
  const candidates = entries.filter(supportedEntry)
  const ignored = entries.filter((entry) => !supportedEntry(entry))
  const result: DesktopFileShortcutAddResult = { added: [], duplicates: [], ignored }
  if (!candidates.length) return result

  await desktopIcons.mutate((draft) => {
    const existingByTarget = new Map<string, DesktopShortcut>()
    for (const shortcut of desktopIcons.workspace.value.shortcuts) {
      if ((shortcut.targetType === 'file' || shortcut.targetType === 'directory') && shortcut.path) {
        existingByTarget.set(targetKey(shortcut.targetType, shortcut.path), shortcut)
      }
    }
    const seen = new Set(existingByTarget.keys())
    const additions: DesktopFileShortcutInput[] = []
    const duplicates: DesktopShortcut[] = []
    for (const entry of candidates) {
      const targetType = entry.kind as 'file' | 'directory'
      const key = targetKey(targetType, entry.path)
      const duplicate = existingByTarget.get(key)
      if (duplicate) {
        duplicates.push(duplicate)
        continue
      }
      if (seen.has(key)) continue
      seen.add(key)
      additions.push({
        id: desktopIcons.generateShortcutID(),
        name: cleanShortcutName(entry),
        description: '',
        targetType,
        path: entry.path,
      })
    }
    const available = Math.max(0, MAX_DESKTOP_SHORTCUTS - draft.shortcuts.length)
    if (additions.length > available) throw new DesktopShortcutLimitError(available, additions.length)
    result.duplicates = duplicates
    if (!additions.length) return false
    draft.shortcuts.push(...additions)
    place?.(draft, additions)
    result.added = additions
  })
  return result
}

export function beginDesktopFileDrag(
  event: DragEvent,
  entries: readonly DesktopFileEntry[],
  sourceNodeId?: string,
  origin: DesktopFileDragOrigin = 'file-manager',
  nativeDownloadURL?: string,
  nativeArchiveName?: string,
): boolean {
  const dataTransfer = event.dataTransfer
  const supported = entries.filter(supportedEntry)
  if (!dataTransfer || !supported.length) return false
  const token = randomToken()
  activeDrag = { token, entries: supported.map((entry) => ({ ...entry })), origin }
  dataTransfer.effectAllowed = 'all'
  dataTransfer.setData(DESKTOP_FILE_DRAG_TYPE, token)
  const crossPanelEntries = supported.filter(
    (entry): entry is CrossPanelFileDragItem => Boolean(entry.resourceVersion),
  )
  if (
    crossPanelEntries.length === supported.length
    && sourceNodeId
    && /^[a-f0-9]{32}$/.test(sourceNodeId)
  ) {
    let batchDescriptor: string
    if (crossPanelEntries.length > MAX_CROSS_PANEL_DRAG_ENTRIES) {
      batchDescriptor = JSON.stringify({
        version: 2, sourceNodeId, entries: [], rejectedCount: crossPanelEntries.length,
      })
    } else {
      batchDescriptor = JSON.stringify({
        version: 2,
        sourceNodeId,
        entries: crossPanelEntries.map((entry) => ({
          name: entry.name,
          path: entry.path,
          kind: entry.kind,
          resourceVersion: entry.resourceVersion,
        })),
      })
    }
    dataTransfer.setData(CROSS_PANEL_FILES_DRAG_TYPE, batchDescriptor)
    // WebKit exposes arbitrary drag MIME types only to same-origin pages.
    // The descriptor is intentionally non-secret and still requires the
    // destination Panel to authenticate to the paired source node.
    dataTransfer.setData('text/plain', CROSS_PANEL_FILES_TEXT_PREFIX + batchDescriptor)
    if (crossPanelEntries.length === 1) {
      const crossPanelEntry = crossPanelEntries[0]!
      dataTransfer.setData(CROSS_PANEL_FILE_DRAG_TYPE, JSON.stringify({
        version: 1,
        sourceNodeId,
        name: crossPanelEntry.name,
        path: crossPanelEntry.path,
        kind: crossPanelEntry.kind,
        resourceVersion: crossPanelEntry.resourceVersion,
      } satisfies CrossPanelFileDragEntry))
    }
  } else {
    dataTransfer.setData('text/plain', supported.length === 1 ? supported[0]!.name : `${supported.length} 个项目`)
  }
  addNativeDownloadDrag(dataTransfer, supported, nativeDownloadURL, nativeArchiveName)
  return true
}

export function hasCrossPanelFileDrag(event: DragEvent): boolean {
  const types = Array.from(event.dataTransfer?.types || [])
  return types.includes(CROSS_PANEL_FILES_DRAG_TYPE)
    || types.includes(CROSS_PANEL_FILE_DRAG_TYPE)
    || (!types.includes('Files') && types.includes('text/plain'))
}

function validCrossPanelDragItem(value: unknown): value is CrossPanelFileDragItem {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const entry = value as Partial<CrossPanelFileDragItem>
  return (entry.kind === 'file' || entry.kind === 'directory')
    && Boolean(entry.name)
    && entry.name!.length <= 255
    && !/[\u0000-\u001f/\\]/.test(entry.name!)
    && Boolean(entry.path)
    && entry.path!.startsWith('/')
    && entry.path!.length <= 4096
    && !/[\u0000-\u001f\\]/.test(entry.path!)
    && !entry.path!.split('/').some((part, index) => index > 0 && (!part || part === '.' || part === '..'))
    && Boolean(entry.resourceVersion)
    && entry.resourceVersion!.length <= 256
}

export function crossPanelFileDragEntries(event: DragEvent): CrossPanelFileDragPayload | undefined {
  const customBatch = event.dataTransfer?.getData(CROSS_PANEL_FILES_DRAG_TYPE)
  const textFallback = event.dataTransfer?.getData('text/plain') || ''
  const rawBatch = customBatch || (
    textFallback.startsWith(CROSS_PANEL_FILES_TEXT_PREFIX)
      ? textFallback.slice(CROSS_PANEL_FILES_TEXT_PREFIX.length)
      : ''
  )
  if (rawBatch && rawBatch.length <= 512 * 1024) {
    try {
      const value: unknown = JSON.parse(rawBatch)
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        const payload = value as { version?: unknown; sourceNodeId?: unknown; entries?: unknown }
        if (
          payload.version === 2
          && typeof payload.sourceNodeId === 'string'
          && /^[a-f0-9]{32}$/.test(payload.sourceNodeId)
          && Array.isArray(payload.entries)
          && payload.entries.length > 0
          && payload.entries.length <= MAX_CROSS_PANEL_DRAG_ENTRIES
          && payload.entries.every(validCrossPanelDragItem)
        ) {
          return {
            sourceNodeId: payload.sourceNodeId,
            entries: payload.entries.map((entry) => ({ ...(entry as CrossPanelFileDragItem) })),
          }
        }
      }
    } catch {
      // A single-item source also publishes the v1 descriptor, so fall back
      // instead of turning a damaged v2 value into a false negative.
    }
  }

  const raw = event.dataTransfer?.getData(CROSS_PANEL_FILE_DRAG_TYPE)
  if (!raw || raw.length > 8_192) return undefined
  try {
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
    const entry = value as Partial<CrossPanelFileDragEntry>
    const sourceNodeId = entry.sourceNodeId
    if (
      entry.version !== 1
      || !sourceNodeId
      || !/^[a-f0-9]{32}$/.test(sourceNodeId)
      || !validCrossPanelDragItem(entry)
    ) return undefined
    return {
      sourceNodeId,
      entries: [{
        name: entry.name!, path: entry.path!, kind: entry.kind!,
        resourceVersion: entry.resourceVersion!,
      }],
    }
  } catch {
    return undefined
  }
}

export function crossPanelFileDragEntry(event: DragEvent): CrossPanelFileDragEntry | undefined {
  const payload = crossPanelFileDragEntries(event)
  if (!payload || payload.entries.length !== 1) return undefined
  return { version: 1, sourceNodeId: payload.sourceNodeId, ...payload.entries[0]! }
}

export function hasDesktopFileDrag(event: DragEvent): boolean {
  return Boolean(activeDrag && Array.from(event.dataTransfer?.types || []).includes(DESKTOP_FILE_DRAG_TYPE))
}

export function desktopFileDragEntries(event: DragEvent): DesktopFileEntry[] {
  const token = event.dataTransfer?.getData(DESKTOP_FILE_DRAG_TYPE)
  if (!activeDrag || !token || token !== activeDrag.token) return []
  return activeDrag.entries.map((entry) => ({ ...entry }))
}

export function desktopFileDragOrigin(event: DragEvent): DesktopFileDragOrigin | undefined {
  const token = event.dataTransfer?.getData(DESKTOP_FILE_DRAG_TYPE)
  if (!activeDrag || !token || token !== activeDrag.token) return undefined
  return activeDrag.origin
}

/** Drag data is protected before drop in some browsers, so hover uses only the active in-memory payload. */
export function peekDesktopFileDragEntries(event: DragEvent): DesktopFileEntry[] {
  if (!hasDesktopFileDrag(event) || !activeDrag) return []
  return activeDrag.entries.map((entry) => ({ ...entry }))
}

/** Keep same-page hover behavior stable while the browser protects drag data. */
export function peekDesktopFileDragOrigin(event: DragEvent): DesktopFileDragOrigin | undefined {
  if (!hasDesktopFileDrag(event) || !activeDrag) return undefined
  return activeDrag.origin
}

export function clearDesktopFileDrag(): void {
  activeDrag = undefined
}

export function resetDesktopFileDragForTest(): void {
  activeDrag = undefined
}
