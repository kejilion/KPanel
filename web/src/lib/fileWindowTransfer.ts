import { readonly, ref } from 'vue'
import { ApiError } from '@/lib/api'
import { useDesktopIcons } from '@/stores/desktopIcons'
import type { FileActionResult, FileEntry } from '@/types/api'

export type FileTransferOperation = 'copy' | 'move'
export type FileTransferEntry = Pick<FileEntry, 'name' | 'path' | 'kind' | 'resourceVersion'>
export type FileTransferTargetError = 'invalid' | 'same_location' | 'inside_source'

export interface FileClipboardState {
  mode: FileTransferOperation
  entries: FileEntry[]
}

export interface FilePathMove {
  source: string
  destination: string
}

type DirectoryChangeListener = (
  directories: ReadonlySet<string>,
  origin?: symbol,
  moves?: readonly FilePathMove[],
) => void

const clipboard = ref<FileClipboardState>()
const directoryChangeListeners = new Set<DirectoryChangeListener>()

function parentPath(value: string): string {
  const index = value.lastIndexOf('/')
  return index <= 0 ? '/' : value.slice(0, index)
}

function isCanonicalAbsolutePath(value: string): boolean {
  return value === '/' || (
    value.startsWith('/')
    && value.length <= 4096
    && !value.includes('\0')
    && !value.includes('\\')
    && value.slice(1).split('/').every((part) => part && part !== '.' && part !== '..')
  )
}

function pathAtOrInside(value: string, parent: string): boolean {
  return value === parent || (parent === '/' ? value.startsWith('/') : value.startsWith(`${parent}/`))
}

export function fileTransferOperation(event: Pick<DragEvent, 'ctrlKey' | 'altKey'>): FileTransferOperation {
  return event.ctrlKey || event.altKey ? 'copy' : 'move'
}

export function fileTransferTargetError(
  entries: readonly Pick<FileTransferEntry, 'path' | 'kind'>[],
  target: string,
): FileTransferTargetError | undefined {
  if (!entries.length || !isCanonicalAbsolutePath(target)) return 'invalid'
  for (const entry of entries) {
    if (!isCanonicalAbsolutePath(entry.path)) return 'invalid'
    if (parentPath(entry.path) === target) return 'same_location'
    if (entry.kind === 'directory' && pathAtOrInside(target, entry.path)) return 'inside_source'
  }
  return undefined
}

export function changedFileDirectories(
  result: Pick<FileActionResult, 'succeeded'>,
  target?: string,
): string[] {
  const directories = new Set<string>()
  if (target && isCanonicalAbsolutePath(target)) directories.add(target)
  for (const item of result.succeeded) {
    if (isCanonicalAbsolutePath(item.path)) directories.add(parentPath(item.path))
    if (item.destination && isCanonicalAbsolutePath(item.destination)) directories.add(parentPath(item.destination))
  }
  return [...directories]
}

export function successfulFileMoves(
  result: Pick<FileActionResult, 'action' | 'succeeded'>,
): FilePathMove[] {
  if (result.action !== 'move' && result.action !== 'rename') return []
  return result.succeeded
    .filter((item): item is typeof item & { destination: string } => Boolean(item.destination))
    .map((item) => ({ source: item.path, destination: item.destination }))
    .filter((item) => isCanonicalAbsolutePath(item.source) && isCanonicalAbsolutePath(item.destination))
    .sort((left, right) => right.source.length - left.source.length)
}

export function remapMovedFilePath(value: string, mappings: readonly FilePathMove[]): string {
  for (const mapping of mappings) {
    if (value === mapping.source) return mapping.destination
    if (mapping.source !== '/' && value.startsWith(`${mapping.source}/`)) {
      return `${mapping.destination}${value.slice(mapping.source.length)}`
    }
  }
  return value
}

export async function syncMovedDesktopShortcuts(
  result: Pick<FileActionResult, 'action' | 'succeeded'>,
): Promise<number> {
  const mappings = successfulFileMoves(result)
  if (!mappings.length) return 0

  const desktopIcons = useDesktopIcons()
  let updated = 0
  const apply = () => desktopIcons.mutate((draft) => {
    updated = 0
    for (const shortcut of draft.shortcuts) {
      if ((shortcut.targetType !== 'file' && shortcut.targetType !== 'directory') || !shortcut.path) continue
      const nextPath = remapMovedFilePath(shortcut.path, mappings)
      if (nextPath === shortcut.path) continue
      shortcut.path = nextPath
      updated += 1
    }
    return updated ? undefined : false
  })

  try {
    await apply()
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 409) throw error
    await apply()
  }
  return updated
}

export function notifyFileDirectoriesChanged(
  directories: readonly string[],
  origin?: symbol,
  moves: readonly FilePathMove[] = [],
): void {
  const normalized = new Set(directories.filter(isCanonicalAbsolutePath))
  const normalizedMoves = moves
    .filter((move) => isCanonicalAbsolutePath(move.source) && isCanonicalAbsolutePath(move.destination))
    .sort((left, right) => right.source.length - left.source.length)
  if (!normalized.size && !normalizedMoves.length) return
  for (const listener of directoryChangeListeners) listener(normalized, origin, normalizedMoves)
}

export function subscribeFileDirectoryChanges(listener: DirectoryChangeListener): () => void {
  directoryChangeListeners.add(listener)
  return () => directoryChangeListeners.delete(listener)
}

export function useFileClipboard() {
  return {
    clipboard: readonly(clipboard),
    set(mode: FileTransferOperation, entries: readonly FileEntry[]) {
      clipboard.value = { mode, entries: entries.map((entry) => ({ ...entry })) }
    },
    clear() {
      clipboard.value = undefined
    },
  }
}

export function resetFileWindowTransferForTest(): void {
  clipboard.value = undefined
  directoryChangeListeners.clear()
}
