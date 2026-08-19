// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import type { DesktopWorkspace, DesktopWorkspaceUpdate } from '@/types/api'
import {
  addFileEntriesToDesktop,
  beginDesktopFileDrag,
  clearDesktopFileDrag,
  crossPanelFileDragEntries,
  crossPanelFileDragEntry,
  desktopFileDragOrigin,
  desktopFileDragEntries,
  DesktopShortcutLimitError,
  hasCrossPanelFileDrag,
  hasDesktopFileDrag,
  NATIVE_FILE_DOWNLOAD_DRAG_TYPE,
  nativeArchiveDownloadDragDescriptor,
  nativeArchiveDownloadName,
  nativeFileDownloadDragDescriptor,
  peekDesktopFileDragEntries,
  peekDesktopFileDragOrigin,
} from './desktopFileShortcuts'

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

function installWorkspace(initial: DesktopWorkspace): void {
  vi.spyOn(api.desktop, 'workspace').mockResolvedValue(initial)
  vi.spyOn(api.desktop, 'updateWorkspace').mockImplementation(async (input: DesktopWorkspaceUpdate) => workspace({
    resourceVersion: `sha256:${'2'.repeat(64)}`,
    hiddenEntryKeys: input.hiddenEntryKeys,
    positions: input.positions,
    labels: input.labels,
    shortcuts: input.shortcuts.map((shortcut) => ({
      ...shortcut,
      createdAt: '2026-08-14T00:00:00Z',
      updatedAt: '2026-08-14T00:00:00Z',
    })),
  }))
}

function dragEvent() {
  const values = new Map<string, string>()
  const types: string[] = []
  const dataTransfer = {
    types,
    effectAllowed: 'none',
    setData(type: string, value: string) {
      if (!types.includes(type)) types.push(type)
      values.set(type, value)
    },
    getData(type: string) {
      return values.get(type) || ''
    },
  }
  return { dataTransfer } as unknown as DragEvent
}

describe('desktop file shortcuts', () => {
  beforeEach(() => {
    resetDesktopIconsForTest()
    clearDesktopFileDrag()
    vi.restoreAllMocks()
  })

  it('adds files and directories in one workspace write and ignores unsupported entries', async () => {
    installWorkspace(workspace())
    const result = await addFileEntriesToDesktop([
      { name: 'nginx.conf', path: '/etc/nginx/nginx.conf', kind: 'file' },
      { name: '网站目录', path: '/home/web', kind: 'directory' },
      { name: 'current', path: '/proc/self/fd/1', kind: 'symlink' },
    ])

    expect(result.added.map(({ name, targetType, path }) => ({ name, targetType, path }))).toEqual([
      { name: 'nginx.conf', targetType: 'file', path: '/etc/nginx/nginx.conf' },
      { name: '网站目录', targetType: 'directory', path: '/home/web' },
    ])
    expect(result.ignored).toHaveLength(1)
    expect(api.desktop.updateWorkspace).toHaveBeenCalledTimes(1)
  })

  it('deduplicates an existing target and enforces the bounded shortcut limit', async () => {
    const existing = workspace({
      shortcuts: [{
        id: 'a'.repeat(32), name: 'nginx.conf', description: '', targetType: 'file', path: '/etc/nginx.conf',
        createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
      }],
    })
    installWorkspace(existing)
    const duplicate = await addFileEntriesToDesktop([{ name: 'nginx.conf', path: '/etc/nginx.conf', kind: 'file' }])
    expect(duplicate.added).toHaveLength(0)
    expect(duplicate.duplicates).toHaveLength(1)

    resetDesktopIconsForTest()
    const full = workspace({
      shortcuts: Array.from({ length: 64 }, (_, index) => ({
        id: index.toString(16).padStart(32, '0'), name: `Item ${index}`, description: '',
        targetType: 'file' as const, path: `/tmp/${index}`,
        createdAt: '2026-08-14T00:00:00Z', updatedAt: '2026-08-14T00:00:00Z',
      })),
    })
    vi.mocked(api.desktop.workspace).mockResolvedValue(full)
    await expect(addFileEntriesToDesktop([{ name: 'extra', path: '/tmp/extra', kind: 'file' }]))
      .rejects.toBeInstanceOf(DesktopShortcutLimitError)
  })

  it('accepts only the active in-memory drag token', () => {
    const event = dragEvent()
    expect(beginDesktopFileDrag(event, [{
      name: 'etc', path: '/etc', kind: 'directory', resourceVersion: 'sha256:etc',
    }])).toBe(true)
    expect(event.dataTransfer?.effectAllowed).toBe('all')
    expect(hasDesktopFileDrag(event)).toBe(true)
    expect(peekDesktopFileDragEntries(event)).toEqual([{
      name: 'etc', path: '/etc', kind: 'directory', resourceVersion: 'sha256:etc',
    }])
    expect(desktopFileDragEntries(event)).toEqual([{
      name: 'etc', path: '/etc', kind: 'directory', resourceVersion: 'sha256:etc',
    }])
    expect(desktopFileDragOrigin(event)).toBe('file-manager')
    clearDesktopFileDrag()
    expect(desktopFileDragEntries(event)).toEqual([])
  })

  it('distinguishes a native desktop shortcut drag from a Files drag', () => {
    const event = dragEvent()
    expect(beginDesktopFileDrag(event, [{
      name: 'app', path: '/app', kind: 'directory', resourceVersion: 'sha256:app',
    }], 'd'.repeat(32), 'desktop-shortcut')).toBe(true)
    expect(desktopFileDragOrigin(event)).toBe('desktop-shortcut')
    const protectedHover = {
      dataTransfer: {
        types: event.dataTransfer!.types,
        getData: () => '',
      },
    } as unknown as DragEvent
    expect(desktopFileDragOrigin(protectedHover)).toBeUndefined()
    expect(peekDesktopFileDragOrigin(protectedHover)).toBe('desktop-shortcut')
  })

  it('adds one same-origin file as a native Chromium download without changing internal drag data', () => {
    const event = dragEvent()
    expect(beginDesktopFileDrag(event, [{
      name: 'report:Q?.txt', path: '/reports/report:Q?.txt', kind: 'file', mime: 'text/plain',
      resourceVersion: 'sha256:report',
    }], undefined, 'file-manager', '/api/v1/files/content?path=%2Freports%2Freport%3AQ%3F.txt&disposition=attachment')).toBe(true)

    const descriptor = event.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE) || ''
    expect(descriptor).toMatch(/^text\/plain:report_Q_\.txt:https?:\/\//)
    expect(descriptor).toContain('/api/v1/files/content?path=%2Freports%2Freport%3AQ%3F.txt&disposition=attachment')
    expect(event.dataTransfer?.getData('text/uri-list')).toBe(
      'http://localhost:3000/api/v1/files/content?path=%2Freports%2Freport%3AQ%3F.txt&disposition=attachment\r\n',
    )
    expect(desktopFileDragEntries(event)).toEqual([{
      name: 'report:Q?.txt', path: '/reports/report:Q?.txt', kind: 'file', mime: 'text/plain',
      resourceVersion: 'sha256:report',
    }])
  })

  it('advertises a folder or multi-selection as one same-origin ZIP download', () => {
    const folder = { name: 'photos', path: '/photos', kind: 'directory' as const, resourceVersion: 'sha256:photos' }
    expect(nativeArchiveDownloadName([folder], 'KPanel Desktop')).toBe('photos.zip')
    const event = dragEvent()
    expect(beginDesktopFileDrag(
      event,
      [folder],
      undefined,
      'desktop-shortcut',
      '/api/v1/files/archive?selection=photos&name=photos.zip',
      'photos.zip',
    )).toBe(true)
    expect(event.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toContain(
      'application/zip:photos.zip:',
    )
    expect(event.dataTransfer?.getData('text/uri-list')).toBe(
      'http://localhost:3000/api/v1/files/archive?selection=photos&name=photos.zip\r\n',
    )
    expect(desktopFileDragEntries(event)).toEqual([folder])

    expect(nativeArchiveDownloadName([
      { name: 'a.txt', path: '/a.txt', kind: 'file' },
      { name: 'logs', path: '/logs', kind: 'directory' },
    ], 'KPanel Desktop')).toBe('KPanel Desktop.zip')
    expect(nativeArchiveDownloadDragDescriptor(
      [folder],
      'https://downloads.example/photos.zip',
      'photos.zip',
      'https://panel.example/files',
    )).toBeUndefined()
  })

  it('does not advertise directories, selections, or cross-origin URLs as native files', () => {
    const directory = dragEvent()
    beginDesktopFileDrag(directory, [{ name: 'reports', path: '/reports', kind: 'directory' }],
      undefined, 'file-manager', '/api/v1/files/content?path=%2Freports&disposition=attachment')
    expect(directory.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toBe('')

    const selection = dragEvent()
    beginDesktopFileDrag(selection, [
      { name: 'a.txt', path: '/a.txt', kind: 'file' },
      { name: 'b.txt', path: '/b.txt', kind: 'file' },
    ], undefined, 'file-manager', '/api/v1/files/content?path=%2Fa.txt&disposition=attachment')
    expect(selection.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toBe('')

    expect(nativeFileDownloadDragDescriptor(
      { name: 'secret.txt', path: '/secret.txt', kind: 'file' },
      'https://downloads.example/secret.txt',
      'https://panel.example/files',
    )).toBeUndefined()
    expect(selection.dataTransfer?.getData('text/uri-list')).toBe('')
  })

  it('serializes one versioned cross-panel descriptor without an authorization secret', () => {
    const event = dragEvent()
    expect(beginDesktopFileDrag(event, [{
      name: 'app', path: '/app', kind: 'directory', resourceVersion: 'sha256:app-version',
    }], 'a'.repeat(32))).toBe(true)
    clearDesktopFileDrag()

    expect(hasDesktopFileDrag(event)).toBe(false)
    expect(hasCrossPanelFileDrag(event)).toBe(true)
    expect(crossPanelFileDragEntry(event)).toEqual({
      version: 1,
      sourceNodeId: 'a'.repeat(32),
      name: 'app',
      path: '/app',
      kind: 'directory',
      resourceVersion: 'sha256:app-version',
    })
    expect(event.dataTransfer?.getData('application/x-kpanel-cross-panel-file-v1')).not.toContain('token')
  })

  it('serializes a bounded multi-item cross-panel descriptor without partial selection', () => {
    const event = dragEvent()
    beginDesktopFileDrag(event, [
      { name: 'a', path: '/a', kind: 'file', resourceVersion: 'sha256:a' },
      { name: 'b', path: '/b', kind: 'file', resourceVersion: 'sha256:b' },
    ], 'b'.repeat(32))
    clearDesktopFileDrag()
    expect(hasCrossPanelFileDrag(event)).toBe(true)
    expect(crossPanelFileDragEntry(event)).toBeUndefined()
    expect(crossPanelFileDragEntries(event)).toEqual({
      sourceNodeId: 'b'.repeat(32),
      entries: [
        { name: 'a', path: '/a', kind: 'file', resourceVersion: 'sha256:a' },
        { name: 'b', path: '/b', kind: 'file', resourceVersion: 'sha256:b' },
      ],
    })
    expect(event.dataTransfer?.getData('application/x-kpanel-cross-panel-file-v1')).toBe('')
  })

  it('reads the non-secret text fallback when a cross-origin browser strips custom MIME types', () => {
    const source = dragEvent()
    beginDesktopFileDrag(source, [{
      name: 'app', path: '/app', kind: 'directory', resourceVersion: 'sha256:app',
    }], 'e'.repeat(32), 'desktop-shortcut')
    const textPayload = source.dataTransfer!.getData('text/plain')
    const target = {
      dataTransfer: {
        types: ['text/plain'],
        getData: (type: string) => type === 'text/plain' ? textPayload : '',
      },
    } as unknown as DragEvent

    expect(hasCrossPanelFileDrag(target)).toBe(true)
    expect(crossPanelFileDragEntries(target)).toEqual({
      sourceNodeId: 'e'.repeat(32),
      entries: [{ name: 'app', path: '/app', kind: 'directory', resourceVersion: 'sha256:app' }],
    })
  })

  it('advertises an over-limit drag so the target can reject it explicitly', () => {
    const event = dragEvent()
    beginDesktopFileDrag(event, Array.from({ length: 65 }, (_, index) => ({
      name: `${index}.txt`, path: `/${index}.txt`, kind: 'file' as const,
      resourceVersion: `sha256:${index}`,
    })), 'c'.repeat(32))
    clearDesktopFileDrag()

    expect(hasCrossPanelFileDrag(event)).toBe(true)
    expect(crossPanelFileDragEntries(event)).toBeUndefined()
  })
})
