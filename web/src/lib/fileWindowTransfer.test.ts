// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import type { DesktopWorkspace, DesktopWorkspaceUpdate, FileActionResult, FileEntry } from '@/types/api'
import {
  changedFileDirectories,
  fileTransferOperation,
  fileTransferTargetError,
  notifyFileDirectoriesChanged,
  remapMovedFilePath,
  resetFileWindowTransferForTest,
  subscribeFileDirectoryChanges,
  syncMovedDesktopShortcuts,
  successfulFileMoves,
  useFileClipboard,
} from './fileWindowTransfer'

function workspace(overrides: Partial<DesktopWorkspace> = {}): DesktopWorkspace {
  return {
    schemaVersion: 2,
    resourceVersion: `sha256:${'1'.repeat(64)}`,
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    labels: {},
    shortcuts: [],
    ...overrides,
  }
}

function file(path: string): FileEntry {
  return {
    name: path.split('/').at(-1)!, path, kind: 'file', mime: 'text/plain', sizeBytes: 1,
    mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-14T00:00:00Z',
    resourceVersion: `sha256:${path}`, editable: true, previewable: true,
  }
}

describe('file window transfer', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    resetDesktopIconsForTest()
    resetFileWindowTransferForTest()
  })

  it('uses move by default and copy while Ctrl or Option is held', () => {
    expect(fileTransferOperation({ ctrlKey: false, altKey: false })).toBe('move')
    expect(fileTransferOperation({ ctrlKey: true, altKey: false })).toBe('copy')
    expect(fileTransferOperation({ ctrlKey: false, altKey: true })).toBe('copy')
  })

  it('rejects the source directory, its descendants, and the existing parent', () => {
    const source = [{ path: '/home/project', kind: 'directory' as const }]
    expect(fileTransferTargetError(source, '/home')).toBe('same_location')
    expect(fileTransferTargetError(source, '/home/project')).toBe('inside_source')
    expect(fileTransferTargetError(source, '/home/project/src')).toBe('inside_source')
    expect(fileTransferTargetError([{ path: '/', kind: 'directory' }], '/home')).toBe('inside_source')
    expect(fileTransferTargetError(source, '/srv')).toBeUndefined()
  })

  it('shares the keyboard clipboard between file windows without exposing mutable entries', () => {
    const firstWindow = useFileClipboard()
    const secondWindow = useFileClipboard()
    const entry = file('/home/readme.txt')
    firstWindow.set('copy', [entry])
    entry.name = 'changed.txt'

    expect(secondWindow.clipboard.value?.mode).toBe('copy')
    expect(secondWindow.clipboard.value?.entries[0]?.name).toBe('readme.txt')
    secondWindow.clear()
    expect(firstWindow.clipboard.value).toBeUndefined()
  })

  it('updates exact and descendant desktop references after a successful move', async () => {
    const initial = workspace({
      positions: { [`shortcut:${'a'.repeat(32)}`]: { x: 0.2, y: 0.3 } },
      shortcuts: [
        {
          id: 'a'.repeat(32), name: 'project', description: '', targetType: 'directory', path: '/home/project',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
        {
          id: 'b'.repeat(32), name: 'config', description: '', targetType: 'file', path: '/home/project/config.yml',
          createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
        },
      ],
    })
    vi.spyOn(api.desktop, 'workspace').mockResolvedValue(initial)
    vi.spyOn(api.desktop, 'updateWorkspace').mockImplementation(async (input: DesktopWorkspaceUpdate) => ({
      ...initial,
      resourceVersion: `sha256:${'2'.repeat(64)}`,
      positions: input.positions,
      shortcuts: input.shortcuts.map((shortcut) => ({
        ...shortcut,
        createdAt: '2026-08-14T00:00:00Z',
        updatedAt: '2026-08-14T00:00:00Z',
      })),
    }))
    const result: Pick<FileActionResult, 'action' | 'succeeded'> = {
      action: 'move',
      succeeded: [{ path: '/home/project', destination: '/srv/project' }],
    }

    expect(await syncMovedDesktopShortcuts(result)).toBe(2)
    const update = vi.mocked(api.desktop.updateWorkspace).mock.calls[0]![0]
    expect(update.shortcuts.map((shortcut) => shortcut.path)).toEqual(['/srv/project', '/srv/project/config.yml'])
    expect(update.positions).toEqual(initial.positions)
  })

  it('reports affected source and destination directories and excludes the notifying window', () => {
    const origin = Symbol('origin')
    const first = vi.fn()
    const second = vi.fn()
    subscribeFileDirectoryChanges((directories, source) => {
      if (source !== origin) first([...directories])
    })
    subscribeFileDirectoryChanges((directories) => second([...directories]))
    const directories = changedFileDirectories({
      succeeded: [{ path: '/home/project', destination: '/srv/project' }],
    }, '/srv')

    notifyFileDirectoriesChanged(directories, origin)

    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledWith(['/srv', '/home'])
  })

  it('lets another open file window follow a relocated directory', () => {
    const moves = successfulFileMoves({
      action: 'move',
      succeeded: [{ path: '/home/project', destination: '/srv/project' }],
    })
    expect(remapMovedFilePath('/home/project', moves)).toBe('/srv/project')
    expect(remapMovedFilePath('/home/project/src', moves)).toBe('/srv/project/src')
    expect(remapMovedFilePath('/home/other', moves)).toBe('/home/other')
  })
})
