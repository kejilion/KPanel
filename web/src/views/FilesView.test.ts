import { readFileSync } from 'node:fs'
import { createSSRApp, nextTick, reactive, ssrContextKey } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { resetDesktopIconsForTest } from '@/stores/desktopIcons'
import { resetFileWindowTransferForTest } from '@/lib/fileWindowTransfer'
import { resetLocaleForTest, setLocale } from '@/i18n'
import {
  beginDesktopFileDrag,
  clearDesktopFileDrag,
  DESKTOP_FILE_DRAG_TYPE,
  NATIVE_FILE_DOWNLOAD_DRAG_TYPE,
} from '@/lib/desktopFileShortcuts'
import type { DesktopWorkspaceUpdate, FileRemoteDownloadJob } from '@/types/api'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  entry: vi.fn(),
  action: vi.fn(),
  transferFromPanel: vi.fn(),
  remoteDownload: vi.fn(),
  createRemoteDownloadJob: vi.fn(),
  remoteDownloadJobs: vi.fn(),
  remoteDownloadJob: vi.fn(),
  cancelRemoteDownloadJob: vi.fn(),
  deleteRemoteDownloadJob: vi.fn(),
  trash: vi.fn(),
  write: vi.fn(),
  upload: vi.fn(),
  contentUrl: vi.fn(),
  archiveUrl: vi.fn(),
  createArchiveDownloadTicket: vi.fn(),
  createDownloadTicket: vi.fn(),
  thumbnailUrl: vi.fn(),
  desktopWorkspace: vi.fn(),
  desktopUpdate: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
  show: vi.fn(),
  route: { query: {} as Record<string, unknown> },
  push: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/lib/api', () => ({
  isRemoteFileHostSelected: vi.fn(() => false),
  ApiError: class MockApiError extends Error {
    readonly status: number
    readonly code: string

    constructor(message: string, status = 0, code = 'request_failed') {
      super(message)
      this.status = status
      this.code = code
    }
  },
  api: {
    files: {
      entry: mocks.entry,
      list: mocks.list,
      action: mocks.action,
      transferFromPanel: mocks.transferFromPanel,
      remoteDownload: mocks.remoteDownload,
      createRemoteDownloadJob: mocks.createRemoteDownloadJob,
      remoteDownloadJobs: mocks.remoteDownloadJobs,
      remoteDownloadJob: mocks.remoteDownloadJob,
      cancelRemoteDownloadJob: mocks.cancelRemoteDownloadJob,
      deleteRemoteDownloadJob: mocks.deleteRemoteDownloadJob,
      trash: mocks.trash,
      contentUrl: mocks.contentUrl,
      archiveUrl: mocks.archiveUrl,
      createArchiveDownloadTicket: mocks.createArchiveDownloadTicket,
      createDownloadTicket: mocks.createDownloadTicket,
      thumbnailUrl: mocks.thumbnailUrl,
      text: vi.fn(),
      write: mocks.write,
      upload: mocks.upload,
    },
    desktop: {
      workspace: mocks.desktopWorkspace,
      updateWorkspace: mocks.desktopUpdate,
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({
    success: mocks.success,
    danger: mocks.danger,
    show: mocks.show,
  }),
}))

interface FileDirectoryResult {
  path: string
  entries: TestFileEntry[]
  offset: number
  total: number
  truncated: boolean
  readAt: string
}

function testDirectory(path: string): FileDirectoryResult {
  return {
    path,
    entries: [],
    offset: 0,
    total: 0,
    truncated: false,
    readAt: '2026-07-30T00:00:00Z',
  }
}

interface FileBindings {
  requestedFilePath: (value: unknown) => string | undefined
  openRequestedFile: (value: unknown) => Promise<void>
  loadDirectory: (path?: string, append?: boolean) => Promise<string | undefined>
  navigateDirectory: (path: string) => Promise<void>
  savePreview: (content?: string) => Promise<void>
  download: (entry: TestFileEntry) => Promise<void>
  downloadSelected: (entry?: TestFileEntry) => Promise<void>
  submitDialog: () => Promise<void>
  cancelArchive: () => void
  openTrash: () => Promise<void>
  toggleAllTrash: () => void
  runTrashAction: (action: 'trash_restore' | 'trash_delete' | 'trash_empty') => Promise<void>
  pasteClipboard: (target?: string) => Promise<void>
  uploadFiles: (files: FileList | File[]) => Promise<void>
  retryUploadTask: (id: string) => Promise<void>
  dismissUploadTask: (id: string) => void
  transferInternalFileDrop: (event: DragEvent, target: string) => Promise<void>
  transferCrossPanelFileDrop: (event: DragEvent, target: string) => Promise<void>
  onDrop: (event: DragEvent) => Promise<void>
  cancelFileTransfer: () => void
  dismissFileTransfer: () => void
  openRemoteDownloadDialog: () => void
  closeRemoteDownloadDialog: () => void
  validRemoteDownloadForm: () => boolean
  submitRemoteDownload: () => Promise<void>
  loadRemoteDownloadJobs: (silent?: boolean) => Promise<void>
  cancelRemoteDownloadJob: (job: FileRemoteDownloadJob) => Promise<void>
  deleteRemoteDownloadJob: (job: FileRemoteDownloadJob) => Promise<void>
  isRemoteDownloadJobActive: (job: FileRemoteDownloadJob) => boolean
  remoteDownloadJobDetail: (job: FileRemoteDownloadJob) => string
  addEntriesToDesktop: (entry?: TestFileEntry, currentDirectory?: boolean) => Promise<void>
  startEntryDrag: (event: DragEvent, entry: TestFileEntry) => void
  setClipboard: (mode: 'copy' | 'move', entry?: TestFileEntry) => void
  showContext: (event: MouseEvent, entry: TestFileEntry) => void
  showDirectoryContext: (event: MouseEvent) => void
  openFileShare: (entry?: TestFileEntry) => void
  closeFileShare: () => void
  openShareManager: () => void
  closeShareManager: () => void
  handleEntryClick: (event: MouseEvent, entry: TestFileEntry) => void
  selectEntry: (event: MouseEvent, path: string) => void
  invertSelection: () => void
  preventNativeSelection: (event: Event) => void
  handleFileShortcut: (event: KeyboardEvent) => void
  filesPage: { value?: HTMLElement }
  openDialog: (action: 'mkdir' | 'rename' | 'chmod' | 'compress' | 'extract' | 'trash', entry?: TestFileEntry) => void
  setViewMode: (mode: 'list' | 'grid') => void
  restoreViewMode: () => void
  canShowThumbnail: (entry: TestFileEntry) => boolean
  thumbnailURL: (entry: TestFileEntry) => string
  markThumbnailFailed: (path: string) => void
  entryIconKind: (entry: TestFileEntry) => string
  currentPath: { value: string }
  search: { value: string }
  entries: { value: TestFileEntry[] }
  entrySearchCatalog: {
    value: Array<{ entry: TestFileEntry; searchName: string }>
  }
  directory: {
    value?: {
      path: string
      entries: TestFileEntry[]
    }
  }
  selected: { value: Set<string> }
  clipboard: {
    value?: {
      mode: 'copy' | 'move'
      entries: TestFileEntry[]
    }
  }
  fileTransferState: {
    value?: {
      mode: 'copy' | 'move'
      target: string
      count: number
      phase: 'running' | 'success' | 'partial' | 'cancelled' | 'error'
      detail?: string
    }
  }
  uploadTasks: {
    value: Array<{
      id: string
      name: string
      target: string
      progress: number
      phase: 'running' | 'success' | 'error'
      detail?: string
    }>
  }
  remoteDownloadDialogOpen: { value: boolean }
  remoteDownloadURL: { value: string }
  remoteDownloadName: { value: string }
  remoteDownloadTarget: { value: string }
  remoteDownloadHTTPWarningID: string
  remoteDownloadFormErrorID: string
  remoteDownloadUsesPlainHTTP: { value: boolean }
  remoteDownloadURLDescription: { value?: string }
  remoteDownloadFormError: { value: string }
  remoteDownloadJobs: { value: FileRemoteDownloadJob[] }
  remoteDownloadJobsLoading: { value: boolean }
  remoteDownloadTasksVisible: { value: boolean }
  remoteDownloadSubmitting: { value: boolean }
  remoteDownloadPendingActions: { value: Set<string> }
  remoteDownloadJobsErrorMessage: { value: string }
  activeRemoteDownloadCount: { value: number }
  contextMenu: { value?: { entry?: TestFileEntry; x: number; y: number } }
  shareEntry: { value?: TestFileEntry }
  shareManagerOpen: { value: boolean }
  dialogEntries: { value: TestFileEntry[] }
  dialogAction: { value?: 'mkdir' | 'rename' | 'chmod' | 'compress' | 'extract' | 'trash' }
  dialogValue: { value: string }
  dialogFormat: { value: 'tar.gz' | 'zip' | 'tar' }
  viewMode: { value: 'list' | 'grid' }
  trashEntries: { value: Array<{ id: string; resourceVersion: string; restorable: boolean }> }
  selectedTrash: { value: Set<string> }
  previewEntry: { value?: TestFileEntry }
  previewContent: { value: string }
  previewDirty: { value: boolean }
  mediaLoading: { value: boolean }
  mediaReady: { value: boolean }
  mediaError: { value: boolean }
  mediaErrorMessage: { value: string }
  mediaErrorDetail: { value: string }
  mediaRetryable: { value: boolean }
  handleMediaLoadStart: () => void
  handleMediaMetadata: () => void
  handleVideoFrameReady: (event: Event) => void
  codeEditorRef: {
    value?: {
      getValue: () => string
      markClean: () => void
      openSearch: () => void
      focus: () => void
    }
  }
}

interface TestFileEntry {
  name: string
  path: string
  kind: 'file' | 'directory' | 'symlink' | 'special'
  mime: string
  sizeBytes: number
  mode: string
  owner: string
  group: string
  modifiedAt: string
  resourceVersion: string
  editable: boolean
  previewable: boolean
}

function testEntry(name: string): TestFileEntry {
  return {
    name,
    path: `/${name}`,
    kind: 'file',
    mime: 'text/plain',
    sizeBytes: 4,
    mode: '-rw-r--r--',
    owner: 'root',
    group: 'root',
    modifiedAt: '2026-07-30T00:00:00Z',
    resourceVersion: `sha256:${name}`,
    editable: true,
    previewable: true,
  }
}

function testRemoteDownloadJob(
  overrides: Partial<FileRemoteDownloadJob> = {},
): FileRemoteDownloadJob {
  return {
    id: 'a'.repeat(32),
    state: 'queued',
    source: 'https://downloads.example.com',
    targetDirectory: '/home/releases',
    createdAt: '2026-08-23T00:00:00Z',
    updatedAt: '2026-08-23T00:00:00Z',
    ...overrides,
  }
}

function internalDrag(entries: TestFileEntry[], modifiers: { ctrlKey?: boolean; altKey?: boolean } = {}): DragEvent {
  const values = new Map<string, string>()
  const types: string[] = []
  const event = {
    ctrlKey: Boolean(modifiers.ctrlKey),
    altKey: Boolean(modifiers.altKey),
    dataTransfer: {
      types,
      effectAllowed: 'none',
      dropEffect: 'none',
      setData(type: string, value: string) {
        if (!types.includes(type)) types.push(type)
        values.set(type, value)
      },
      getData(type: string) {
        return values.get(type) || ''
      },
    },
  } as unknown as DragEvent
  beginDesktopFileDrag(event, entries)
  return event
}

function crossPanelDrag(entries: TestFileEntry[]): DragEvent {
  const event = internalDrag(entries)
  beginDesktopFileDrag(event, entries, 'a'.repeat(32))
  clearDesktopFileDrag()
  return event
}

function externalFileEntry(name: string, content: string) {
  return {
    name,
    isFile: true,
    isDirectory: false,
    file(success: (value: File) => void) {
      success(new File([content], name, { type: 'text/plain' }))
    },
  }
}

function externalDirectoryEntry(name: string, children: unknown[]) {
  return {
    name,
    isFile: false,
    isDirectory: true,
    createReader() {
      let read = false
      return {
        readEntries(success: (values: unknown[]) => void) {
          if (read) success([])
          else {
            read = true
            success(children)
          }
        },
      }
    },
  }
}

function externalDirectoryDrop(entry: ReturnType<typeof externalDirectoryEntry>): DragEvent {
  return {
    dataTransfer: {
      types: ['Files'],
      items: [{ kind: 'file', webkitGetAsEntry: () => entry }],
      // Chromium exposes a top-level directory placeholder here. Treating this
      // value as a readable File reproduces the interrupted-upload regression.
      files: [new File([], entry.name)],
    },
  } as unknown as DragEvent
}

function externalFileDrop(entry: ReturnType<typeof externalFileEntry>): DragEvent {
  return {
    dataTransfer: {
      types: ['Files'],
      items: [{ kind: 'file', webkitGetAsEntry: () => entry }],
      files: [new File(['fallback'], entry.name, { type: 'text/plain' })],
    },
  } as unknown as DragEvent
}

function setupView(): FileBindings {
  const component = FilesView as unknown as {
    setup: (props: Record<string, never>, context: { expose: () => void }) => FileBindings
  }
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  try {
    return app.runWithContext(() => component.setup({}, { expose: () => undefined }))
  } finally {
    warn.mockRestore()
  }
}

beforeEach(() => {
  resetLocaleForTest()
  vi.clearAllMocks()
  resetDesktopIconsForTest()
  resetFileWindowTransferForTest()
  clearDesktopFileDrag()
  mocks.route = reactive({ query: {} as Record<string, unknown> })
  mocks.push.mockImplementation(async (location: { query?: Record<string, unknown> }) => {
    mocks.route.query = location.query || {}
  })
  vi.stubGlobal('window', {
    innerWidth: 1280,
    innerHeight: 720,
    confirm: vi.fn(() => true),
    setTimeout: vi.fn(() => 1),
    clearTimeout: vi.fn(),
    localStorage: {
      getItem: vi.fn(),
      setItem: vi.fn(),
    },
  })
  vi.stubGlobal('location', { href: 'https://panel.example/files' })
  vi.stubGlobal('document', { activeElement: null, documentElement: { lang: '', dir: '' } })
  mocks.list.mockResolvedValue(testDirectory('/web'))
  mocks.action.mockResolvedValue({ action: 'trash', succeeded: [], failed: [] })
  mocks.transferFromPanel.mockImplementation(async (input: { path: string; targetDirectory: string }, onEvent: (event: unknown) => void) => {
    onEvent({ state: 'complete' })
    const source = input.path.slice(input.path.lastIndexOf('/') + 1)
    return { ...testEntry(source), path: `${input.targetDirectory}/${source}` }
  })
  mocks.remoteDownload.mockImplementation(async (
    input: { name?: string; targetDirectory: string },
    onEvent: (event: unknown) => void,
  ) => {
    const name = input.name || 'download'
    const entry = { ...testEntry(name), path: `${input.targetDirectory}/${name}`, sizeBytes: 7 }
    onEvent({ state: 'connecting' })
    onEvent({ state: 'transferring', loadedBytes: 7, totalBytes: 7, name })
    onEvent({ state: 'confirming', loadedBytes: 7, totalBytes: 7, name })
    onEvent({ state: 'complete', loadedBytes: 7, totalBytes: 7, name, entry })
    return entry
  })
  const remoteDownloadJob = testRemoteDownloadJob()
  mocks.createRemoteDownloadJob.mockResolvedValue(remoteDownloadJob)
  mocks.remoteDownloadJobs.mockResolvedValue({ items: [] })
  mocks.remoteDownloadJob.mockResolvedValue(remoteDownloadJob)
  mocks.cancelRemoteDownloadJob.mockResolvedValue(remoteDownloadJob)
  mocks.deleteRemoteDownloadJob.mockResolvedValue(undefined)
  mocks.trash.mockResolvedValue({ entries: [], total: 0, readAt: '2026-07-30T00:00:00Z' })
  mocks.write.mockImplementation(async (_path: string, _content: string, _version: string) => ({
    entry: testEntry('saved.txt'),
  }))
  mocks.createDownloadTicket.mockResolvedValue({
    downloadUrl: '/api/v1/files/download/test-ticket',
    expiresAt: '2026-07-30T00:05:00Z',
  })
  mocks.createArchiveDownloadTicket.mockResolvedValue({
    downloadUrl: '/api/v1/files/archive-download/test-ticket',
    expiresAt: '2026-07-30T00:05:00Z',
  })
  mocks.contentUrl.mockImplementation((path: string, disposition: string) => (
    `/api/v1/files/content?path=${encodeURIComponent(path)}&disposition=${disposition}`
  ))
  mocks.archiveUrl.mockImplementation((_entries: TestFileEntry[], name: string) => (
    `/api/v1/files/archive?selection=test&name=${encodeURIComponent(name)}`
  ))
  mocks.thumbnailUrl.mockImplementation((path: string, version: string) => `/thumb?path=${path}&version=${version}`)
  const desktopWorkspace = {
    schemaVersion: 3 as const,
    resourceVersion: `sha256:${'1'.repeat(64)}`,
    available: true,
    hiddenEntryKeys: [],
    positions: {},
    widgetPositions: {},
    labels: {},
    shortcuts: [],
  }
  mocks.desktopWorkspace.mockResolvedValue(desktopWorkspace)
  mocks.desktopUpdate.mockImplementation(async (input: DesktopWorkspaceUpdate) => ({
    ...desktopWorkspace,
    resourceVersion: `sha256:${'2'.repeat(64)}`,
    shortcuts: input.shortcuts.map((shortcut: Record<string, unknown>) => ({
      ...shortcut,
      createdAt: '2026-08-14T00:00:00Z',
      updatedAt: '2026-08-14T00:00:00Z',
    })),
  }))
})

describe('FilesView external upload', () => {
  it('keeps an ordinary operating-system file drop on the existing direct upload path', async () => {
    const view = setupView()
    view.currentPath.value = '/srv'
    mocks.list.mockResolvedValue(testDirectory('/srv'))
    mocks.upload.mockImplementation(async (path: string, file: File) => ({
      ...testEntry(file.name),
      path: `${path}/${file.name}`,
    }))

    await view.onDrop(externalFileDrop(externalFileEntry('notes.txt', 'hello')))

    expect(mocks.upload).toHaveBeenCalledWith('/srv', expect.objectContaining({ name: 'notes.txt' }), false, expect.any(Function))
    expect(mocks.entry).not.toHaveBeenCalled()
    expect(mocks.action).not.toHaveBeenCalled()
    expect(mocks.danger).not.toHaveBeenCalled()
  })

  it('uses the recursive desktop upload path for an operating-system directory drop', async () => {
    const view = setupView()
    const directories = new Set(['/srv'])
    view.currentPath.value = '/srv'
    mocks.list.mockResolvedValue(testDirectory('/srv'))
    mocks.entry.mockImplementation(async (path: string) => {
      if (directories.has(path)) {
        return { ...testEntry(path.split('/').at(-1) || '/'), path, kind: 'directory' }
      }
      throw Object.assign(new Error('not found'), { status: 404 })
    })
    mocks.action.mockImplementation(async (input: { action: string; target?: string; name?: string }) => {
      const path = `${input.target === '/' ? '' : input.target}/${input.name}`
      directories.add(path)
      return { action: input.action, succeeded: [{ path }], failed: [] }
    })
    mocks.upload.mockImplementation(async (path: string, file: File, _overwrite: boolean, onProgress?: (value: number) => void) => {
      onProgress?.(100)
      return { ...testEntry(file.name), path: `${path}/${file.name}` }
    })
    const event = externalDirectoryDrop(externalDirectoryEntry('project', [
      externalFileEntry('README.md', 'hello'),
      externalDirectoryEntry('empty', []),
      externalDirectoryEntry('src', [externalFileEntry('main.ts', 'export {}')]),
    ]))

    await view.onDrop(event)

    expect(mocks.action.mock.calls.map(([input]) => input)).toEqual([
      { action: 'mkdir', target: '/srv', name: 'project' },
      { action: 'mkdir', target: '/srv/project', name: 'empty' },
      { action: 'mkdir', target: '/srv/project', name: 'src' },
    ])
    expect(mocks.upload.mock.calls.map(([path, file]) => ({ path, name: file.name }))).toEqual(expect.arrayContaining([
      { path: '/srv/project', name: 'README.md' },
      { path: '/srv/project/src', name: 'main.ts' },
    ]))
    expect(mocks.upload).not.toHaveBeenCalledWith('/srv', expect.objectContaining({ name: 'project' }), expect.anything(), expect.anything())
    expect(mocks.danger).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenCalledWith('/srv', expect.anything(), expect.anything())
  })

  it('retains a failed direct upload for inline retry without a duplicate toast', async () => {
    const view = setupView()
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    view.currentPath.value = '/srv'
    mocks.list.mockResolvedValue(testDirectory('/srv'))
    mocks.upload
      .mockRejectedValueOnce(new Error('connection reset'))
      .mockImplementationOnce(async (path: string, uploaded: File, _overwrite: boolean, onProgress?: (value: number) => void) => {
        onProgress?.(100)
        return { ...testEntry(uploaded.name), path: `${path}/${uploaded.name}` }
      })

    await view.uploadFiles([file])

    expect(view.uploadTasks.value).toHaveLength(1)
    expect(view.uploadTasks.value[0]).toMatchObject({
      name: 'notes.txt', target: '/srv', progress: 0, phase: 'error', detail: 'connection reset',
    })
    expect(mocks.danger).not.toHaveBeenCalled()

    await view.retryUploadTask(view.uploadTasks.value[0]!.id)

    expect(mocks.upload).toHaveBeenCalledTimes(2)
    expect(view.uploadTasks.value[0]).toMatchObject({ progress: 100, phase: 'success' })
    expect(mocks.danger).not.toHaveBeenCalled()
    view.dismissUploadTask(view.uploadTasks.value[0]!.id)
    expect(view.uploadTasks.value).toEqual([])
  })
})

describe('FilesView remote download', () => {
  it('keeps an empty initial task refresh hidden until there is useful status to show', () => {
    const view = setupView()

    expect(view.remoteDownloadTasksVisible.value).toBe(false)
    view.remoteDownloadJobsLoading.value = true
    expect(view.remoteDownloadTasksVisible.value).toBe(false)

    view.remoteDownloadJobs.value = [testRemoteDownloadJob()]
    expect(view.remoteDownloadTasksVisible.value).toBe(true)
  })

  it('presents the feature as a regular remote download in every locale', () => {
    const remoteDownloadCopy = (locale: string) => locale
      .split('\n')
      .filter((line) => line.includes('files.remoteDownload.'))
      .join('\n')
    const zhCN = remoteDownloadCopy(readFileSync(new URL('../i18n/messages/zh-CN.ts', import.meta.url), 'utf8'))
    const zhTW = remoteDownloadCopy(readFileSync(new URL('../i18n/messages/zh-TW.ts', import.meta.url), 'utf8'))
    const enUS = remoteDownloadCopy(readFileSync(new URL('../i18n/messages/en-US.ts', import.meta.url), 'utf8'))

    expect(zhCN).toContain("'files.remoteDownload.label': '远程下载'")
    expect(zhCN).toContain("'files.remoteDownload.tooltip': '从链接下载，关闭页面后仍会继续'")
    expect(zhCN).toContain("'files.remoteDownload.dialogDescription': '从链接下载到 {target}，关闭页面后仍会继续。'")
    expect(zhCN).toContain("'files.remoteDownload.note': '支持公开 HTTP/HTTPS 地址，单个文件最多 512 MiB。'")
    expect(zhCN).toContain("'files.remoteDownload.tasksDescription': '关闭页面后仍会继续。'")
    expect(zhCN).not.toMatch(/离线下载|后台任务|后台下载/)
    expect(zhTW).toContain('"files.remoteDownload.label": "遠端下載"')
    expect(zhTW).toContain('"files.remoteDownload.tooltip": "從連結下載，關閉頁面後仍會繼續"')
    expect(zhTW).toContain('"files.remoteDownload.dialogDescription": "從連結下載到 {target}，關閉頁面後仍會繼續。"')
    expect(zhTW).toContain('"files.remoteDownload.note": "支援公開 HTTP/HTTPS 網址，單一檔案最多 512 MiB。"')
    expect(zhTW).toContain('"files.remoteDownload.tasksDescription": "關閉頁面後仍會繼續。"')
    expect(zhTW).not.toMatch(/離線下載|背景任務|背景下載/)
    expect(enUS).toContain("'files.remoteDownload.label': 'Remote download'")
    expect(enUS).toContain("'files.remoteDownload.tooltip': 'Download from a link; it continues after you close this page'")
    expect(enUS).toContain("'files.remoteDownload.dialogDescription': 'Download from a link to {target}; it continues after you close this page.'")
    expect(enUS).toContain("'files.remoteDownload.note': 'Supports public HTTP/HTTPS URLs up to 512 MiB per file.'")
    expect(enUS).toContain("'files.remoteDownload.tasksDescription': 'Downloads continue after you close this page.'")
    expect(enUS).not.toMatch(/offline download|background task|background download/i)
  })

  it('creates a background job for the snapshotted target and clears the signed URL from view state', async () => {
    const view = setupView()
    mocks.createRemoteDownloadJob.mockResolvedValueOnce(testRemoteDownloadJob({
      name: 'release.zip',
      targetDirectory: '/home/releases',
    }))
    view.currentPath.value = '/home/releases'
    view.openRemoteDownloadDialog()
    expect(view.remoteDownloadTarget.value).toBe('/home/releases')
    view.remoteDownloadURL.value = 'https://downloads.example.com/release.zip?token=secret'
    view.remoteDownloadName.value = 'release.zip'
    view.currentPath.value = '/etc'

    await view.submitRemoteDownload()

    expect(mocks.createRemoteDownloadJob).toHaveBeenCalledWith({
      url: 'https://downloads.example.com/release.zip?token=secret',
      targetDirectory: '/home/releases',
      name: 'release.zip',
    })
    expect(mocks.remoteDownload).not.toHaveBeenCalled()
    expect(view.remoteDownloadDialogOpen.value).toBe(false)
    expect(view.remoteDownloadURL.value).toBe('')
    expect(view.remoteDownloadJobs.value[0]).toMatchObject({
      state: 'queued', targetDirectory: '/home/releases', name: 'release.zip',
    })
    expect(JSON.stringify(view.remoteDownloadJobs.value)).not.toContain('secret')
    expect(view.remoteDownloadSubmitting.value).toBe(false)
  })

  it('reconciles a newly created matching job after the create response is lost', async () => {
    const view = setupView()
    const known = testRemoteDownloadJob({ name: 'release.zip' })
    const recovered = testRemoteDownloadJob({
      id: 'b'.repeat(32),
      name: 'release.zip',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    })
    view.remoteDownloadJobs.value = [known]
    mocks.createRemoteDownloadJob.mockRejectedValueOnce(new Error('connection reset'))
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [recovered, known] })
    view.currentPath.value = '/home/releases'
    view.openRemoteDownloadDialog()
    view.remoteDownloadURL.value = 'https://downloads.example.com/release.zip?token=secret'
    view.remoteDownloadName.value = 'release.zip'

    await view.submitRemoteDownload()

    expect(mocks.remoteDownloadJobs).toHaveBeenCalledOnce()
    expect(mocks.createRemoteDownloadJob.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.remoteDownloadJobs.mock.invocationCallOrder[0]!,
    )
    expect(view.remoteDownloadJobs.value.map((job) => job.id)).toContain(recovered.id)
    expect(view.remoteDownloadDialogOpen.value).toBe(false)
    expect(view.remoteDownloadURL.value).toBe('')
    expect(view.remoteDownloadJobsErrorMessage.value).toBe('')
    expect(mocks.danger).not.toHaveBeenCalled()
  })

  it('keeps the original create error and form when no matching submitted job is found', async () => {
    const view = setupView()
    const unrelated = testRemoteDownloadJob({
      id: 'b'.repeat(32),
      name: 'other.zip',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    })
    mocks.createRemoteDownloadJob.mockRejectedValueOnce(new Error('connection reset'))
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [unrelated] })
    view.currentPath.value = '/home/releases'
    view.openRemoteDownloadDialog()
    view.remoteDownloadURL.value = 'https://downloads.example.com/release.zip?token=secret'
    view.remoteDownloadName.value = 'release.zip'

    await view.submitRemoteDownload()

    expect(view.remoteDownloadDialogOpen.value).toBe(true)
    expect(view.remoteDownloadURL.value).toContain('token=secret')
    expect(view.remoteDownloadJobsErrorMessage.value).toBe('connection reset')
    expect(mocks.danger).toHaveBeenCalledOnce()
  })

  it('rejects an unsupported URL before calling the API', async () => {
    const view = setupView()
    view.openRemoteDownloadDialog()
    view.remoteDownloadURL.value = 'file:///etc/passwd'

    await view.submitRemoteDownload()

    expect(mocks.createRemoteDownloadJob).not.toHaveBeenCalled()
    expect(view.remoteDownloadDialogOpen.value).toBe(true)
    expect(view.remoteDownloadFormError.value).toContain('HTTP 或 HTTPS')
    await setLocale('en-US', false)
    expect(view.remoteDownloadFormError.value).toBe('Enter a complete HTTP or HTTPS download URL.')
  })

  it('keeps the plaintext HTTP warning and URL error descriptions synchronized', () => {
    const view = setupView()
    expect(view.remoteDownloadURLDescription.value).toBeUndefined()

    view.remoteDownloadURL.value = 'http://downloads.example.com/file.bin'
    expect(view.remoteDownloadUsesPlainHTTP.value).toBe(true)
    expect(view.remoteDownloadURLDescription.value).toBe(view.remoteDownloadHTTPWarningID)

    view.remoteDownloadURL.value = 'http://'
    expect(view.validRemoteDownloadForm()).toBe(false)
    expect(view.remoteDownloadURLDescription.value).toBe(
      `${view.remoteDownloadHTTPWarningID} ${view.remoteDownloadFormErrorID}`,
    )

    view.remoteDownloadURL.value = 'https://downloads.example.com/file.bin'
    expect(view.remoteDownloadUsesPlainHTTP.value).toBe(false)
    expect(view.remoteDownloadURLDescription.value).toBe(view.remoteDownloadFormErrorID)
    expect(view.validRemoteDownloadForm()).toBe(true)
    expect(view.remoteDownloadURLDescription.value).toBeUndefined()

    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
    expect(source).toContain(':aria-describedby="remoteDownloadURLDescription"')
    expect(source).toContain(':id="remoteDownloadHTTPWarningID"')
    expect(source).toMatch(/remote-download-warning[\s\S]*?role="status"[\s\S]*?aria-live="polite"/)
  })

  it('requests cancellation, then keeps polling until the server reports a terminal state', async () => {
    const view = setupView()
    const active = testRemoteDownloadJob({ state: 'transferring', loadedBytes: 1024 })
    const cancelled = testRemoteDownloadJob({
      state: 'cancelled', loadedBytes: 1024, finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [active]
    mocks.cancelRemoteDownloadJob.mockResolvedValueOnce(active)
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [cancelled] })

    await view.cancelRemoteDownloadJob(active)

    expect(mocks.cancelRemoteDownloadJob).toHaveBeenCalledWith(active.id)
    expect(mocks.remoteDownloadJobs).toHaveBeenCalledOnce()
    expect(view.remoteDownloadJobs.value[0]?.state).toBe('cancelled')
    expect(view.remoteDownloadJobDetail(cancelled)).toContain('原子提交窗口')
    await setLocale('en-US', false)
    expect(view.remoteDownloadJobDetail(cancelled)).toContain('atomic commit window')
    expect(mocks.success).not.toHaveBeenCalled()
  })

  it('keeps remote download in the file command bar with a labelled task region and semantic progress', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
    const commandBar = source.slice(
      source.indexOf('<div class="file-command-bar__actions">'),
      source.indexOf('</div>', source.indexOf('<div class="file-command-bar__actions">')),
    )
    expect(commandBar).toContain("files.remoteDownload.label")
    expect(source).toContain('aria-labelledby="remote-download-tasks-title"')
    expect(source).toContain('aria-live="polite"')
    const phaseMarkup = source.slice(
      source.indexOf('class="remote-download-task__phase"'),
      source.indexOf('</span>', source.indexOf('class="remote-download-task__phase"')),
    )
    expect(phaseMarkup).toContain('role="status"')
    expect(phaseMarkup).toContain('aria-live="polite"')
    expect(phaseMarkup).toContain('aria-atomic="true"')
    expect(phaseMarkup).not.toContain('files.remoteDownload.received')
    expect(source).toContain('class="remote-download-task__bytes"')
    expect(source).not.toMatch(/class="remote-download-task__bytes"[^>]*(?:role|aria-live)=/)
    expect(source).toContain('<progress')
    expect(source).toContain('role="alert"')
    expect(source).toContain("files.remoteDownload.note")
  })

  it('uses one bounded readable activity stack for uploads and file transfers', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
    const narrowStyles = source.slice(
      source.indexOf('@media (max-width: 720px)'),
      source.indexOf('@media (max-width: 480px)'),
    )

    expect(source).toContain('class="remote-download-task__summary"')
    expect(source).not.toContain('class="remote-download-tasks__privacy"')
    expect(source).toMatch(/\.remote-download-tasks\s*\{[^}]*background:\s*var\(--surface\);/)
    expect(source).toMatch(/\.remote-download-task\s*\{[^}]*background:\s*var\(--surface-subtle\);/)
    expect(source).toContain('class="file-status-stack"')
    expect(source).toContain('class="file-activity-stack"')
    expect(source).toContain('file-activity-row upload-task')
    expect(source).toContain('@click="retryUploadTask(task.id)"')
    expect(source).toContain('@click="dismissUploadTask(task.id)"')
    expect(source).toContain('role="progressbar"')
    expect(source).toContain(':aria-valuenow="task.progress"')
    expect(source).toContain("files.upload.progressLabel")
    expect(source).toMatch(/\.file-status-stack\s*\{[^}]*max-height:\s*min\(420px, 46dvh\);[^}]*overflow-y:\s*auto;/)
    expect(source).toMatch(/\.file-activity-row\s*\{[^}]*grid-template-columns:\s*auto minmax\(0, 1fr\) auto;/)
    expect(source).toMatch(/\.file-activity-row strong\s*\{[^}]*font-size:\s*14px;/)
    expect(source).toMatch(/\.file-activity-row small\s*\{[^}]*font-size:\s*13px;/)
    expect(source).toMatch(/\.file-activity-row__actions button\s*\{[^}]*min-height:\s*40px;/)
    expect(source).toMatch(/\.file-internal-drop-hint strong\s*\{[^}]*font-size:\s*14px;/)
    expect(source).toMatch(/\.file-internal-drop-hint small\s*\{[^}]*font-size:\s*13px;/)
    expect(narrowStyles).toMatch(/\.remote-download-task\s*\{[^}]*grid-template-columns:\s*auto minmax\(0, 1fr\) auto;/)
    expect(narrowStyles).toMatch(/\.remote-download-tasks__refresh\s*\{[^}]*width:\s*44px;[^}]*min-width:\s*44px;/)
  })

  it('keeps the file command bar compact with an accessible icon refresh and directory wording', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
    const refreshStart = source.indexOf('file-command-bar__refresh')
    const refreshButton = source.slice(refreshStart, source.indexOf('</button>', refreshStart))

    expect(refreshButton).toContain('title="刷新目录"')
    expect(refreshButton).toContain('aria-label="刷新目录"')
    expect(refreshButton).toContain('<RefreshCw')
    expect(refreshButton).not.toMatch(/>\s*刷新\s*</)
    expect(source).toMatch(/\.file-command-bar__actions \.file-command-bar__refresh\s*{[\s\S]*?width:\s*40px;[\s\S]*?padding:\s*0;/)
    expect(source).toContain('title="新建目录" aria-label="新建目录"')
    expect(source).not.toContain('新建文件夹')
  })

  it('does not let a completion refresh abort an active directory navigation', async () => {
    const view = setupView()
    const active = testRemoteDownloadJob({ targetDirectory: '/home/releases' })
    const complete = testRemoteDownloadJob({
      state: 'complete', targetDirectory: '/home/releases',
      finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [active]
    view.currentPath.value = '/home/releases'
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [complete] })
    let resolveNavigation!: (directory: FileDirectoryResult) => void
    let navigationSignal: AbortSignal | undefined
    mocks.list.mockImplementationOnce((_path: string, _options: unknown, signal?: AbortSignal) => {
      navigationSignal = signal
      return new Promise<FileDirectoryResult>((resolve) => {
        resolveNavigation = resolve
      })
    })

    const navigation = view.navigateDirectory('/etc')
    await view.loadRemoteDownloadJobs()

    expect(navigationSignal?.aborted).toBe(false)
    expect(mocks.list).toHaveBeenCalledTimes(1)
    resolveNavigation(testDirectory('/etc'))
    await navigation
    expect(view.currentPath.value).toBe('/etc')
    expect(mocks.list).toHaveBeenCalledTimes(1)
  })

  it('refreshes a target that completes while navigation into that target is still reading', async () => {
    const view = setupView()
    const active = testRemoteDownloadJob({ targetDirectory: '/etc' })
    const complete = testRemoteDownloadJob({
      state: 'complete', targetDirectory: '/etc',
      finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [active]
    view.currentPath.value = '/home'
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [complete] })
    let resolveNavigation!: (directory: FileDirectoryResult) => void
    mocks.list
      .mockImplementationOnce(() => new Promise<FileDirectoryResult>((resolve) => {
        resolveNavigation = resolve
      }))
      .mockResolvedValueOnce(testDirectory('/etc'))

    const navigation = view.navigateDirectory('/etc')
    await view.loadRemoteDownloadJobs()
    expect(mocks.list).toHaveBeenCalledTimes(1)
    resolveNavigation(testDirectory('/etc'))
    await navigation

    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2))
    expect(mocks.list.mock.calls[1]?.[0]).toBe('/etc')
    expect(view.currentPath.value).toBe('/etc')
  })

  it('keeps a requested-target refresh when an unrelated target completes concurrently', async () => {
    const view = setupView()
    const targetActive = testRemoteDownloadJob({ id: 'a'.repeat(32), targetDirectory: '/etc' })
    const unrelatedActive = testRemoteDownloadJob({ id: 'b'.repeat(32), targetDirectory: '/var' })
    const targetComplete = { ...targetActive, state: 'complete' as const, finishedAt: '2026-08-23T00:00:10Z' }
    const unrelatedComplete = {
      ...unrelatedActive, state: 'complete' as const, finishedAt: '2026-08-23T00:00:10Z',
    }
    view.remoteDownloadJobs.value = [targetActive, unrelatedActive]
    view.currentPath.value = '/home'
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [targetComplete, unrelatedComplete] })
    let resolveNavigation!: (directory: FileDirectoryResult) => void
    mocks.list
      .mockImplementationOnce(() => new Promise<FileDirectoryResult>((resolve) => {
        resolveNavigation = resolve
      }))
      .mockResolvedValueOnce(testDirectory('/etc'))

    const navigation = view.navigateDirectory('/etc')
    await view.loadRemoteDownloadJobs()
    resolveNavigation(testDirectory('/etc'))
    await navigation

    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2))
    expect(mocks.list.mock.calls.map((call) => call[0])).toEqual(['/etc', '/etc'])
    expect(mocks.list.mock.calls.some((call) => call[0] === '/var')).toBe(false)
  })

  it('refreshes the original target when a navigation fails after a job completes', async () => {
    const view = setupView()
    const active = testRemoteDownloadJob({ targetDirectory: '/home/releases' })
    const complete = testRemoteDownloadJob({
      state: 'complete', targetDirectory: '/home/releases',
      finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [active]
    view.currentPath.value = '/home/releases'
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [complete] })
    let rejectNavigation!: (error: Error) => void
    mocks.list
      .mockImplementationOnce(() => new Promise<FileDirectoryResult>((_resolve, reject) => {
        rejectNavigation = reject
      }))
      .mockResolvedValueOnce(testDirectory('/home/releases'))

    const navigation = view.navigateDirectory('/etc')
    await view.loadRemoteDownloadJobs()
    rejectNavigation(new Error('navigation failed'))
    await navigation

    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2))
    expect(mocks.list.mock.calls[1]?.[0]).toBe('/home/releases')
    expect(view.currentPath.value).toBe('/home/releases')
  })

  it('queues a final refresh behind an active read of the same target directory', async () => {
    const view = setupView()
    const active = testRemoteDownloadJob({ targetDirectory: '/home/releases' })
    const complete = testRemoteDownloadJob({
      state: 'complete', targetDirectory: '/home/releases',
      finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [active]
    view.currentPath.value = '/home/releases'
    mocks.remoteDownloadJobs.mockResolvedValueOnce({ items: [complete] })
    let resolveCurrentRead!: (directory: FileDirectoryResult) => void
    let currentReadSignal: AbortSignal | undefined
    mocks.list
      .mockImplementationOnce((_path: string, _options: unknown, signal?: AbortSignal) => {
        currentReadSignal = signal
        return new Promise<FileDirectoryResult>((resolve) => {
          resolveCurrentRead = resolve
        })
      })
      .mockResolvedValueOnce(testDirectory('/home/releases'))

    const currentRead = view.loadDirectory('/home/releases')
    await view.loadRemoteDownloadJobs()

    expect(currentReadSignal?.aborted).toBe(false)
    expect(mocks.list).toHaveBeenCalledTimes(1)
    resolveCurrentRead(testDirectory('/home/releases'))
    await currentRead
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2))
    expect(mocks.list.mock.calls[1]?.[0]).toBe('/home/releases')
  })

  it('refreshes a directory when the first restored terminal job is newer than its snapshot', async () => {
    const view = setupView()
    view.currentPath.value = '/home/releases'
    view.directory.value = testDirectory('/home/releases')
    mocks.remoteDownloadJobs.mockResolvedValueOnce({
      items: [testRemoteDownloadJob({
        state: 'complete',
        finishedAt: '2026-08-23T00:00:10Z',
        updatedAt: '2026-08-23T00:00:10Z',
      })],
    })
    mocks.list.mockResolvedValueOnce({
      ...testDirectory('/home/releases'), readAt: '2026-08-23T00:00:11Z',
    })

    await view.loadRemoteDownloadJobs()

    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledOnce())
    expect(mocks.list.mock.calls[0]?.[0]).toBe('/home/releases')
  })

  it('aborts a stale task-list request before clearing a terminal job so it cannot reappear', async () => {
    const view = setupView()
    const complete = testRemoteDownloadJob({
      state: 'complete', finishedAt: '2026-08-23T00:00:10Z',
    })
    view.remoteDownloadJobs.value = [complete]
    let staleSignal: AbortSignal | undefined
    mocks.remoteDownloadJobs
      .mockImplementationOnce((signal?: AbortSignal) => new Promise((_resolve, reject) => {
        staleSignal = signal
        signal?.addEventListener(
          'abort',
          () => reject(new DOMException('Aborted', 'AbortError')),
          { once: true },
        )
      }))
      .mockResolvedValueOnce({ items: [] })

    const staleLoad = view.loadRemoteDownloadJobs(true)
    await vi.waitFor(() => expect(staleSignal).toBeDefined())
    await view.deleteRemoteDownloadJob(complete)
    await staleLoad

    expect(staleSignal?.aborted).toBe(true)
    expect(mocks.deleteRemoteDownloadJob).toHaveBeenCalledWith(complete.id)
    expect(mocks.remoteDownloadJobs).toHaveBeenCalledTimes(2)
    expect(view.remoteDownloadJobs.value).toEqual([])
  })
})

describe('FilesView desktop shortcuts', () => {
  it('adds native file and ZIP DownloadURL payloads without replacing internal drags', () => {
    const view = setupView()
    const first = testEntry('one.txt')
    const second = testEntry('two.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    const single = internalDrag([])

    view.startEntryDrag(single, first)

    expect(single.dataTransfer?.types).toContain(DESKTOP_FILE_DRAG_TYPE)
    expect(single.dataTransfer?.types).toContain(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)
    expect(mocks.contentUrl).toHaveBeenCalledWith('/one.txt', 'attachment')
    expect(single.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toContain(
      '/api/v1/files/content?path=%2Fone.txt&disposition=attachment',
    )
    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()

    view.selected.value = new Set([first.path, second.path])
    const selection = internalDrag([])
    view.startEntryDrag(selection, first)
    expect(selection.dataTransfer?.types).toContain(DESKTOP_FILE_DRAG_TYPE)
    expect(selection.dataTransfer?.types).toContain(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)
    expect(mocks.archiveUrl).toHaveBeenCalledWith([first, second], 'home.zip')
    expect(selection.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toContain(
      'application/zip:home.zip:https://panel.example/api/v1/files/archive',
    )

    const folder = { ...testEntry('photos'), kind: 'directory' as const }
    const directory = internalDrag([])
    view.startEntryDrag(directory, folder)
    expect(directory.dataTransfer?.types).toContain(DESKTOP_FILE_DRAG_TYPE)
    expect(directory.dataTransfer?.types).toContain(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)
    expect(mocks.archiveUrl).toHaveBeenCalledWith([folder], 'photos.zip')
    expect(directory.dataTransfer?.getData(NATIVE_FILE_DOWNLOAD_DRAG_TYPE)).toContain(
      'application/zip:photos.zip:https://panel.example/api/v1/files/archive',
    )
    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.show).not.toHaveBeenCalled()
  })

  it('adds the current multi-selection in one desktop workspace update', async () => {
    const view = setupView()
    const first = testEntry('nginx.conf')
    const second = testEntry('site.log')
    view.directory.value = { path: '/etc', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])

    await view.addEntriesToDesktop(first)

    expect(mocks.desktopUpdate).toHaveBeenCalledTimes(1)
    const input = mocks.desktopUpdate.mock.calls[0]![0]
    expect(input.shortcuts.map((shortcut: Record<string, unknown>) => ({
      name: shortcut.name,
      targetType: shortcut.targetType,
      path: shortcut.path,
    }))).toEqual([
      { name: 'nginx.conf', targetType: 'file', path: '/nginx.conf' },
      { name: 'site.log', targetType: 'file', path: '/site.log' },
    ])
    expect(mocks.success).toHaveBeenCalledWith('已添加 2 项到桌面', '图标已按桌面空位自动排列。')
  })

  it('resolves a desktop file target and opens it in the file preview', async () => {
    const view = setupView()
    const entry = { ...testEntry('nginx.conf'), path: '/etc/nginx/nginx.conf', editable: false }
    mocks.entry.mockResolvedValueOnce(entry)

    await view.openRequestedFile(entry.path)

    expect(mocks.entry).toHaveBeenCalledWith(entry.path)
    expect(view.selected.value).toEqual(new Set([entry.path]))
    expect(view.previewEntry.value).toEqual(entry)
  })
})

describe('FilesView downloads', () => {
  it('uses a short-lived download URL and reports ticket failures', async () => {
    const anchor = {
      href: '',
      download: '',
      rel: '',
      click: vi.fn(),
      remove: vi.fn(),
    }
    const appendChild = vi.fn()
    vi.stubGlobal('document', {
      activeElement: null,
      body: { appendChild },
      createElement: vi.fn(() => anchor),
    })
    const view = setupView()
    const entry = testEntry('hello.txt')

    await view.download(entry)

    expect(mocks.createDownloadTicket).toHaveBeenCalledWith('/hello.txt')
    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchor.href).toBe('/api/v1/files/download/test-ticket')
    expect(anchor.download).toBe('hello.txt')
    expect(appendChild).toHaveBeenCalledWith(anchor)
    expect(anchor.click).toHaveBeenCalledOnce()
    expect(anchor.remove).toHaveBeenCalledOnce()

    mocks.createDownloadTicket.mockRejectedValueOnce(new Error('ticket failed'))
    await view.download(entry)
    expect(mocks.danger).toHaveBeenCalledWith('下载失败', 'ticket failed')
  })
})

describe('FilesView route path', () => {
  it('accepts bounded absolute paths and rejects traversal or relative paths', () => {
    const view = setupView()
    expect(view.requestedFilePath('/home/web/html/example.com')).toBe('/home/web/html/example.com')
    expect(view.requestedFilePath(['/root/project'])).toBe('/root/project')
    expect(view.requestedFilePath('../etc')).toBeUndefined()
    expect(view.requestedFilePath('/home/../etc')).toBeUndefined()
    expect(view.requestedFilePath('/home//example')).toBeUndefined()
    expect(view.requestedFilePath('/home/./example')).toBeUndefined()
    expect(view.requestedFilePath('/home\\example')).toBeUndefined()
    expect(view.requestedFilePath(`/home/${'x'.repeat(4096)}`)).toBeUndefined()
  })

  it('records successful directory navigation once using the Agent-confirmed path', async () => {
    mocks.route.query = { path: '/home' }
    mocks.list.mockResolvedValueOnce(testDirectory('/home/project'))
    const view = setupView()

    await view.navigateDirectory('/home/project/')
    await nextTick()

    expect(mocks.list).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledOnce()
    expect(mocks.push).toHaveBeenCalledWith({
      name: 'files',
      query: { path: '/home/project' },
    })
  })

  it('loads browser history changes including the bare files route', async () => {
    mocks.route.query = { path: '/home/project' }
    mocks.list.mockImplementation(async (path: string) => testDirectory(path))
    const view = setupView()
    view.currentPath.value = '/home/project'

    mocks.route.query = { path: '/home' }
    await vi.waitFor(() => expect(view.currentPath.value).toBe('/home'))
    mocks.route.query = {}
    await vi.waitFor(() => expect(view.currentPath.value).toBe('/'))

    expect(mocks.list.mock.calls.map(([path]) => path)).toEqual(['/home', '/'])
    expect(mocks.push).not.toHaveBeenCalled()
  })

  it('does not record failed or superseded directory navigation', async () => {
    let resolveSlow: ((value: FileDirectoryResult) => void) | undefined
    mocks.list.mockImplementation((path: string) => {
      if (path === '/slow') {
        return new Promise<FileDirectoryResult>((resolve) => {
          resolveSlow = resolve
        })
      }
      return Promise.resolve(testDirectory(path))
    })
    const view = setupView()

    const slow = view.navigateDirectory('/slow')
    await view.navigateDirectory('/fast')
    resolveSlow?.(testDirectory('/slow'))
    await slow

    expect(view.currentPath.value).toBe('/fast')
    expect(mocks.push).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledWith({ name: 'files', query: { path: '/fast' } })

    mocks.push.mockClear()
    mocks.list.mockRejectedValueOnce(new Error('missing'))
    await view.navigateDirectory('/missing')
    expect(mocks.push).not.toHaveBeenCalled()
  })
})

describe('FilesView large icon layout', () => {
  it('uses metadata-first streaming and a stable responsive video stage', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(
      /<video[\s\S]*preload="metadata"[\s\S]*playsinline[\s\S]*@loadedmetadata="handleMediaMetadata"[\s\S]*@loadeddata="handleVideoFrameReady"/,
    )
    expect(source).toContain('<source :src="previewURL" :type="previewEntry.mime || undefined" />')
    expect(source).toContain('视频流响应超时，请检查网络或服务器。')
    expect(source).toContain('浏览器只能播放音轨，无法解码视频画面。')
    expect(source).toMatch(/\.media-player\s*\{[^}]*aspect-ratio:\s*16 \/ 9;/)
    expect(source).toMatch(/\.media-player video\s*\{[^}]*width:\s*100%;[^}]*height:\s*100%;/)
    expect(source).toContain('支持边缓冲边播放')
  })

  it('uses one theme-derived palette across text, media, and metadata previews', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(/\.code-viewer\s*\{[^}]*border:\s*1px solid var\(--file-preview-border\);[^}]*background:\s*var\(--file-preview-background\);/)
    expect(source).toMatch(/\.media-viewer\s*\{[\s\S]*?var\(--file-preview-glow\)[\s\S]*?var\(--file-preview-panel\)[\s\S]*?var\(--file-preview-background\)/)
    expect(source).toContain('color: var(--file-preview-muted);')
    expect(source).toContain('color: var(--file-preview-text);')
    expect(source).not.toContain('rgb(53 203 166 / 15%)')
    expect(source).not.toContain('linear-gradient(180deg, #111c1d')
  })

  it('requires decoded video dimensions instead of treating container metadata as visual readiness', () => {
    const view = setupView()

    view.handleMediaLoadStart()
    view.handleMediaMetadata()
    expect(view.mediaReady.value).toBe(false)
    expect(view.mediaLoading.value).toBe(false)

    const pause = vi.fn()
    view.handleVideoFrameReady({
      currentTarget: { videoWidth: 0, videoHeight: 0, pause },
    } as unknown as Event)
    expect(pause).toHaveBeenCalledOnce()
    expect(view.mediaError.value).toBe(true)
    expect(view.mediaRetryable.value).toBe(false)
    expect(view.mediaErrorMessage.value).toBe('浏览器只能播放音轨，无法解码视频画面。')
    expect(view.mediaErrorDetail.value).toContain('H.264 + AAC')

    view.handleVideoFrameReady({
      currentTarget: { videoWidth: 1920, videoHeight: 1080, pause: vi.fn() },
    } as unknown as Event)
    expect(view.mediaReady.value).toBe(true)
    expect(view.mediaError.value).toBe(false)
  })

  it('keeps the desktop shortcut action behind permissions without wrapping batch labels', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')
    const batchToolbar = source.match(/aria-label="批量文件操作"[\s\S]*?<\/Transition>/)?.[0] || ''
    const contextMenu = source.match(/class="file-context-menu k-context-menu"[\s\S]*?<ModalDialog/)?.[0] || ''

    expect(batchToolbar.indexOf("openDialog('chmod')")).toBeGreaterThan(-1)
    expect(batchToolbar).toContain('v-if="selectedEntriesDownloadable"')
    expect(batchToolbar).toContain('@click="downloadSelected()"')
    expect(batchToolbar.indexOf('addEntriesToDesktop()')).toBeGreaterThan(batchToolbar.indexOf("openDialog('chmod')"))
    expect(batchToolbar.indexOf('invertSelection')).toBeGreaterThan(batchToolbar.indexOf('addEntriesToDesktop()'))
    expect(contextMenu.indexOf("openDialog('chmod', contextMenu.entry)")).toBeGreaterThan(-1)
    expect(contextMenu).toContain('contextBatchDownloadable')
    expect(contextMenu).toContain('@click="downloadSelected(contextMenu.entry)"')
    expect(contextMenu).toContain('v-if="contextShareEntry"')
    expect(contextMenu.indexOf('openFileShare(contextMenu.entry)')).toBeGreaterThan(
      contextMenu.indexOf('downloadSelected(contextMenu.entry)'),
    )
    expect(contextMenu.indexOf('openFileShare(contextMenu.entry)')).toBeLessThan(
      contextMenu.indexOf("openDialog('compress', contextMenu.entry)"),
    )
    expect(contextMenu).toContain('role="menuitem"')
    expect(contextMenu).toContain('@keydown.stop="handleContextMenuKeydown"')
    expect(source).toContain('<Teleport to="body">')
    expect(source).toContain('placeContextMenu(menu, { x: current.x, y: current.y }, contextMenuOpener)')
    expect(source).toContain("document.addEventListener('scroll', closeContextMenuOnScroll, true)")
    expect(source).toContain("window.visualViewport?.addEventListener?.('resize', closeContextMenuOnViewportChange)")
    expect(source).toContain('aria-haspopup="menu"')
    expect(source).toContain(':aria-expanded="contextMenu?.entry?.path === entry.path"')
    expect(source).toContain('focusFirstContextMenuItem(menu, focusOrigin)')
    expect(batchToolbar).not.toContain('openFileShare')
    expect(source).toContain('<FileShareDialog v-if="shareEntry" :entry="shareEntry" @close="closeFileShare" />')
    expect(source).toContain('<FileShareManagerDialog v-if="shareManagerOpen" @close="closeShareManager" />')
    expect(source).toContain('<Share2 :size="15" /> 分享管理')
    expect(contextMenu.indexOf('addEntriesToDesktop(contextMenu.entry)')).toBeGreaterThan(
      contextMenu.indexOf("openDialog('chmod', contextMenu.entry)"),
    )
    expect(contextMenu.indexOf("openDialog('trash', contextMenu.entry)")).toBeGreaterThan(
      contextMenu.indexOf('addEntriesToDesktop(contextMenu.entry)'),
    )
    expect(source).toMatch(/\.batch-bar button\s*\{[^}]*flex:\s*0 0 auto;[^}]*white-space:\s*nowrap;/)
    expect(source).toMatch(/:global\(\.desktop-window \.batch-bar\)\s*\{[^}]*width:\s*min\(760px, calc\(100% - 28px\)\);/)
    expect(source).not.toMatch(/:global\(\.desktop-window\)\s+\.batch-bar/)
  })

  it('lets the code editor consume the remaining fullscreen height', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(/:global\(\.modal-panel--fullscreen \.code-viewer\)\s*\{[^}]*display:\s*flex;[^}]*height:\s*100%;[^}]*flex-direction:\s*column;/)
    expect(source).toMatch(/:global\(\.modal-panel--fullscreen \.code-editor\)\s*\{[^}]*height:\s*auto;[^}]*min-height:\s*0;[^}]*flex:\s*1 1 auto;/)
    expect(source).not.toMatch(/:global\(\.modal-panel--fullscreen\)\s+\.code-/)
  })

  it('skips layout and paint work for offscreen directory entries', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toMatch(/\.file-row--entry\s*\{[^}]*content-visibility:\s*auto;/)
    expect(source).toMatch(/\.file-grid-card\s*\{[^}]*content-visibility:\s*auto;/)
  })

  it('defaults to the list and persists the selected layout', () => {
    const view = setupView()

    expect(view.viewMode.value).toBe('list')
    view.setViewMode('grid')

    expect(view.viewMode.value).toBe('grid')
    expect(window.localStorage.setItem).toHaveBeenCalledWith('kpanel:files:view:v1', 'grid')
  })

  it('restores a valid saved layout preference', () => {
    vi.mocked(window.localStorage.getItem).mockReturnValue('grid')
    const view = setupView()

    view.restoreViewMode()

    expect(view.viewMode.value).toBe('grid')
  })

  it('only requests bounded safe raster thumbnails and falls back after an error', () => {
    const view = setupView()
    const image = { ...testEntry('photo.png'), mime: 'image/png', sizeBytes: 1024 }
    const svg = { ...testEntry('active.svg'), mime: 'image/svg+xml', sizeBytes: 1024 }
    const oversized = { ...testEntry('large.jpg'), mime: 'image/jpeg', sizeBytes: 13 * 1024 * 1024 }

    expect(view.canShowThumbnail(image)).toBe(true)
    expect(view.thumbnailURL(image)).toContain('/thumb?path=/photo.png')
    expect(view.canShowThumbnail(svg)).toBe(false)
    expect(view.canShowThumbnail(oversized)).toBe(false)

    view.markThumbnailFailed(image.path)
    expect(view.canShowThumbnail(image)).toBe(false)
  })

  it('lazy-loads thumbnails without making the original image draggable', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toContain('loading="lazy"')
    expect(source).toContain('decoding="async"')
    expect(source).toContain('draggable="false"')
    expect(source).toContain('markThumbnailFailed(entry.path)')
  })

  it('uses distinct icons for common file families', () => {
    const view = setupView()

    expect(view.entryIconKind({ ...testEntry('backup.tar.gz'), mime: 'application/gzip' })).toBe('archive')
    expect(view.entryIconKind({ ...testEntry('data.xlsx'), mime: 'application/octet-stream' })).toBe('spreadsheet')
    expect(view.entryIconKind({ ...testEntry('site.sql'), mime: 'text/plain' })).toBe('database')
    expect(view.entryIconKind({ ...testEntry('.env'), mime: 'text/plain' })).toBe('secret')
    expect(view.entryIconKind({ ...testEntry('main.go'), mime: 'text/plain' })).toBe('code')
  })
})

describe('FilesView directory loading', () => {
  it('reuses the sorted directory catalog while the search query changes', () => {
    const view = setupView()
    view.directory.value = {
      path: '/',
      entries: [testEntry('zeta.log'), testEntry('alpha.txt'), testEntry('beta.txt')],
    }

    view.search.value = 'txt'
    const catalog = view.entrySearchCatalog.value
    view.search.value = ''
    expect(view.entries.value.map((entry) => entry.name)).toEqual([
      'alpha.txt',
      'beta.txt',
      'zeta.log',
    ])

    view.search.value = 'txt'
    expect(view.entries.value.map((entry) => entry.name)).toEqual(['alpha.txt', 'beta.txt'])
    expect(view.entrySearchCatalog.value).toBe(catalog)
  })

  it('keeps the collapsed sidebar offset scoped through a custom property', () => {
    const source = readFileSync(new URL('./FilesView.vue', import.meta.url), 'utf8')

    expect(source).toContain('var(--app-shell-inline-offset)')
    expect(source).not.toContain(':global(.app-shell__main--sidebar-collapsed)')
  })

  it('uses the Agent-confirmed path and clears stale errors', async () => {
    const view = setupView()
    await view.loadDirectory('/web')

    expect(mocks.list).toHaveBeenCalledWith(
      '/web',
      { offset: 0, search: undefined },
      expect.any(AbortSignal),
    )
    expect(view.currentPath.value).toBe('/web')
    expect(view.directory.value?.entries).toEqual([])
    expect(mocks.danger).not.toHaveBeenCalled()
  })

  it('appends a subsequent directory page without duplicating entries', async () => {
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    mocks.list
      .mockResolvedValueOnce({
        path: '/web',
        entries: [first],
        offset: 0,
        nextOffset: 1,
        total: 2,
        truncated: true,
        readAt: '2026-07-30T00:00:00Z',
      })
      .mockResolvedValueOnce({
        path: '/web',
        entries: [first, second],
        offset: 1,
        total: 2,
        truncated: false,
        readAt: '2026-07-30T00:00:01Z',
      })
    const view = setupView()

    await view.loadDirectory('/web')
    await view.loadDirectory('/web', true)

    expect(mocks.list).toHaveBeenLastCalledWith(
      '/web',
      { offset: 1, search: undefined },
      expect.any(AbortSignal),
    )
    expect(view.directory.value?.entries.map((entry) => entry.name)).toEqual([
      'first.txt',
      'second.txt',
    ])
  })

  it('keeps the current directory when refresh fails', async () => {
    mocks.list.mockRejectedValueOnce(new Error('offline'))
    const view = setupView()
    view.currentPath.value = '/docker'

    await view.loadDirectory('/web')

    expect(view.currentPath.value).toBe('/docker')
    expect(mocks.danger).toHaveBeenCalledWith('目录读取失败', 'offline')
  })

  it('reports partial batch results and refreshes the real directory state', async () => {
    const view = setupView()
    const entry = testEntry('keep.txt')
    view.directory.value = {
      path: '/',
      entries: [entry],
    }
    view.selected.value = new Set(['/keep.txt'])
    view.openDialog('trash')
    mocks.action.mockResolvedValueOnce({
      action: 'trash',
      succeeded: [],
      failed: [{ path: '/keep.txt', detail: '文件状态已变化' }],
    })

    await view.submitDialog()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'trash',
      sources: ['/keep.txt'],
      expectedResourceVersions: { '/keep.txt': 'sha256:keep.txt' },
    })
    expect(mocks.danger).toHaveBeenCalledWith(
      '文件操作未完成',
      '0 项成功，1 项失败：文件状态已变化',
    )
    expect(mocks.list).toHaveBeenCalled()
    expect(view.dialogAction.value).toBeUndefined()
  })

  it('compresses selected entries with the chosen fixed archive format', async () => {
    const view = setupView()
    const first = testEntry('website.txt')
    const second = testEntry('assets.txt')
    view.currentPath.value = '/web'
    view.directory.value = { path: '/web', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])
    view.openDialog('compress')
    view.dialogValue.value = 'release'
    view.dialogFormat.value = 'zip'
    mocks.action.mockResolvedValueOnce({
      action: 'compress',
      succeeded: [{ path: '/web/release.zip' }],
      failed: [],
    })

    await view.submitDialog()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'compress',
      sources: [first.path, second.path],
      target: '/web',
      name: 'release.zip',
      format: 'zip',
      expectedResourceVersions: {
        [first.path]: first.resourceVersion,
        [second.path]: second.resourceVersion,
      },
    }, expect.any(AbortSignal))
    expect(mocks.success).toHaveBeenCalledWith('压缩完成', '1 项已处理')
  })

  it('extracts a supported archive into a new non-overwriting directory', async () => {
    const view = setupView()
    const entry = testEntry('backup.tar.gz')
    view.currentPath.value = '/backups'
    view.directory.value = { path: '/backups', entries: [entry] }
    mocks.action.mockResolvedValueOnce({
      action: 'extract',
      succeeded: [{ path: entry.path, destination: '/backups/backup' }],
      failed: [],
    })

    view.openDialog('extract', entry)
    await view.submitDialog()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'extract',
      sources: [entry.path],
      target: '/backups',
      name: 'backup',
      format: 'tar.gz',
      expectedResourceVersion: entry.resourceVersion,
    }, expect.any(AbortSignal))
    expect(mocks.success).toHaveBeenCalledWith('解压完成', '1 项已处理')
  })

  it('aborts an active archive request and reports cleanup without a false failure', async () => {
    const view = setupView()
    const entry = testEntry('large.log')
    view.currentPath.value = '/logs'
    view.directory.value = { path: '/logs', entries: [entry] }
    view.selected.value = new Set([entry.path])
    view.openDialog('compress')
    mocks.action.mockImplementationOnce((_input: unknown, signal?: AbortSignal) =>
      new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
      }),
    )

    const pending = view.submitDialog()
    view.cancelArchive()
    await pending

    expect(mocks.success).toHaveBeenCalledWith('操作已停止', '未完成的临时文件已清理。')
    expect(mocks.danger).not.toHaveBeenCalledWith('文件操作失败', expect.any(String))
    expect(view.dialogAction.value).toBeUndefined()
  })

  it('restores a recycle-bin entry with resource-version protection', async () => {
    const view = setupView()
    mocks.trash.mockResolvedValueOnce({
      entries: [{
        id: 'trash-id',
        name: 'config.json',
        originalPath: '/etc/config.json',
        kind: 'file',
        sizeBytes: 4,
        mode: '-rw-r--r--',
        owner: 'root',
        group: 'root',
        deletedAt: '2026-07-30T00:00:00Z',
        resourceVersion: 'sha256:trash',
        restorable: true,
      }],
      total: 1,
      readAt: '2026-07-30T00:00:00Z',
    })
    mocks.action.mockResolvedValueOnce({
      action: 'trash_restore',
      succeeded: [{ path: 'trash-id', destination: '/etc/config.json' }],
      failed: [],
    })

    await view.openTrash()
    view.selectedTrash.value = new Set(['trash-id'])
    await view.runTrashAction('trash_restore')

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'trash_restore',
      trashIds: ['trash-id'],
      expectedResourceVersions: { 'trash-id': 'sha256:trash' },
    })
    expect(mocks.success).toHaveBeenCalledWith('恢复完成', '1 项已处理')
  })

  it('selects and permanently deletes every visible recycle-bin entry', async () => {
    const view = setupView()
    const entries = [
      { id: 'trash-first', resourceVersion: 'sha256:first', restorable: true },
      { id: 'trash-second', resourceVersion: 'sha256:second', restorable: true },
    ].map((entry, index) => ({
      ...entry,
      name: `${index}.txt`,
      originalPath: `/${index}.txt`,
      kind: 'file' as const,
      sizeBytes: 4,
      mode: '-rw-r--r--',
      owner: 'root',
      group: 'root',
      deletedAt: '2026-07-30T00:00:00Z',
    }))
    mocks.trash.mockResolvedValueOnce({
      entries,
      total: entries.length,
      readAt: '2026-07-30T00:00:00Z',
    })
    mocks.action.mockResolvedValueOnce({
      action: 'trash_delete',
      succeeded: entries.map((entry) => ({ path: entry.id })),
      failed: [],
    })

    await view.openTrash()
    view.toggleAllTrash()
    await view.runTrashAction('trash_delete')

    expect(window.confirm).toHaveBeenCalledWith('彻底删除选中的 2 项？此操作不可恢复。')
    expect(mocks.action).toHaveBeenCalledWith({
      action: 'trash_delete',
      trashIds: ['trash-first', 'trash-second'],
      expectedResourceVersions: {
        'trash-first': 'sha256:first',
        'trash-second': 'sha256:second',
      },
    })
  })

  it('saves the live editor value without copying the document on every keystroke', async () => {
    const view = setupView()
    const entry = testEntry('config.json')
    const markClean = vi.fn()
    view.previewEntry.value = entry
    view.previewContent.value = 'stale content'
    view.previewDirty.value = true
    view.codeEditorRef.value = {
      getValue: () => 'latest editor content',
      markClean,
      openSearch: vi.fn(),
      focus: vi.fn(),
    }
    mocks.write.mockResolvedValueOnce({ entry: { ...entry, resourceVersion: 'sha256:saved' } })

    await view.savePreview()

    expect(mocks.write).toHaveBeenCalledWith(
      entry.path,
      'latest editor content',
      entry.resourceVersion,
    )
    expect(view.previewContent.value).toBe('latest editor content')
    expect(view.previewDirty.value).toBe(false)
    expect(markClean).toHaveBeenCalledOnce()
  })

  it('selects an unchecked entry when opening its context menu', () => {
    const view = setupView()
    const checked = testEntry('checked.txt')
    const clicked = testEntry('clicked.txt')
    view.directory.value = { path: '/', entries: [checked, clicked] }
    view.selected.value = new Set([checked.path])

    view.showContext(
      {
        preventDefault: vi.fn(),
        clientX: 400,
        clientY: 300,
      } as unknown as MouseEvent,
      clicked,
    )

    expect([...view.selected.value]).toEqual([clicked.path])
    expect(view.contextMenu.value?.entry?.path).toBe(clicked.path)
  })

  it('preserves a multi-selection when opening a selected entry context menu', () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])

    view.showContext(
      {
        preventDefault: vi.fn(),
        clientX: 400,
        clientY: 300,
      } as unknown as MouseEvent,
      second,
    )

    expect([...view.selected.value]).toEqual([first.path, second.path])
    expect(view.contextMenu.value?.entry?.path).toBe(second.path)
  })

  it('opens sharing for exactly one regular file and rejects directories or a multi-selection', () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    const directory = { ...testEntry('folder'), kind: 'directory' as const }
    view.directory.value = { path: '/', entries: [first, second, directory] }

    view.selected.value = new Set([first.path])
    view.openFileShare(first)
    expect(view.shareEntry.value).toEqual(first)
    expect(view.contextMenu.value).toBeUndefined()

    view.closeFileShare()
    view.selected.value = new Set([first.path, second.path])
    view.openFileShare(second)
    expect(view.shareEntry.value).toBeUndefined()

    view.selected.value = new Set([directory.path])
    view.openFileShare(directory)
    expect(view.shareEntry.value).toBeUndefined()
  })

  it('opens the lightweight share manager independently from file selection', () => {
    const view = setupView()
    view.selected.value = new Set(['/missing/old-image.png'])

    view.openShareManager()
    expect(view.shareManagerOpen.value).toBe(true)

    view.closeShareManager()
    expect(view.shareManagerOpen.value).toBe(false)
  })

  it('keeps the full selection for every batch-capable context action', () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])

    view.openDialog('compress', second)
    expect(view.dialogEntries.value.map((entry) => entry.path)).toEqual([first.path, second.path])

    view.openDialog('chmod', second)
    expect(view.dialogEntries.value.map((entry) => entry.path)).toEqual([first.path, second.path])

    view.setClipboard('copy', second)
    expect(view.clipboard.value?.entries.map((entry) => entry.path)).toEqual([first.path, second.path])

    view.selected.value = new Set([first.path, second.path])
    view.setClipboard('move', second)
    expect(view.clipboard.value?.mode).toBe('move')
    expect(view.clipboard.value?.entries.map((entry) => entry.path)).toEqual([first.path, second.path])
  })

  it('keeps rename and extract scoped to the context-menu entry', () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const archive = { ...testEntry('second.zip'), mime: 'application/zip' }
    view.directory.value = { path: '/', entries: [first, archive] }
    view.selected.value = new Set([first.path, archive.path])

    view.openDialog('rename', archive)
    expect(view.dialogEntries.value.map((entry) => entry.path)).toEqual([archive.path])

    view.openDialog('extract', archive)
    expect(view.dialogEntries.value.map((entry) => entry.path)).toEqual([archive.path])
  })

  it('trashes every selected entry from a selected entry context menu', async () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])
    mocks.action.mockResolvedValueOnce({
      action: 'trash',
      succeeded: [{ path: first.path }, { path: second.path }],
      failed: [],
    })

    view.openDialog('trash', second)
    await view.submitDialog()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'trash',
      sources: [first.path, second.path],
      expectedResourceVersions: {
        [first.path]: first.resourceVersion,
        [second.path]: second.resourceVersion,
      },
    })
  })

  it('submits every selected entry and resource version for batch chmod', async () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])
    mocks.action.mockResolvedValueOnce({
      action: 'chmod',
      succeeded: [{ path: first.path }, { path: second.path }],
      failed: [],
    })

    view.openDialog('chmod', second)
    view.dialogValue.value = '640'
    await view.submitDialog()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'chmod',
      sources: [first.path, second.path],
      mode: '640',
      expectedResourceVersions: {
        [first.path]: first.resourceVersion,
        [second.path]: second.resourceVersion,
      },
    })
  })

  it('downloads a selected batch as one ZIP from a selected entry context menu', async () => {
    const anchors: Array<{ click: ReturnType<typeof vi.fn>; remove: ReturnType<typeof vi.fn> }> = []
    vi.stubGlobal('document', {
      activeElement: null,
      body: { appendChild: vi.fn() },
      createElement: vi.fn(() => {
        const anchor = { href: '', download: '', rel: '', click: vi.fn(), remove: vi.fn() }
        anchors.push(anchor)
        return anchor
      }),
    })
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    view.selected.value = new Set([first.path, second.path])

    const downloadPromise = view.downloadSelected(second)

    expect(mocks.createDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.createArchiveDownloadTicket).toHaveBeenCalledWith([first, second], 'home.zip')
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchors).toHaveLength(0)
    await downloadPromise
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/archive-download/test-ticket',
      download: 'home.zip',
      rel: 'noopener',
    })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
  })

  it('does not silently download only the supported part of a mixed selection', async () => {
    const view = setupView()
    const file = testEntry('first.txt')
    const special = { ...testEntry('device'), kind: 'special' as const }
    view.directory.value = { path: '/', entries: [file, special] }
    view.selected.value = new Set([file.path, special.path])

    await view.downloadSelected(special)

    expect(mocks.createDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(mocks.danger).toHaveBeenCalledWith('下载失败', '只能下载普通文件或文件夹')
  })

  it('uses Windows-style click, control-click, and shift-click selection', () => {
    const view = setupView()
    const first = testEntry('a.txt')
    const second = testEntry('b.txt')
    const third = testEntry('c.txt')
    view.directory.value = { path: '/', entries: [first, second, third] }

    view.selectEntry({} as MouseEvent, first.path)
    expect([...view.selected.value]).toEqual([first.path])

    view.selectEntry({ ctrlKey: true } as MouseEvent, third.path)
    expect([...view.selected.value]).toEqual([first.path, third.path])

    view.selectEntry({ shiftKey: true } as MouseEvent, second.path)
    expect([...view.selected.value]).toEqual([second.path, third.path])
  })

  it('opens entries with one tap on phone widths without changing desktop selection behavior', () => {
    const view = setupView()
    const entry = testEntry('mobile.txt')
    view.directory.value = { path: '/', entries: [entry] }
    const mobileEvent = {
      preventDefault: vi.fn(),
      stopPropagation: vi.fn(),
    } as unknown as MouseEvent

    window.innerWidth = 390
    view.handleEntryClick(mobileEvent, entry)
    expect(view.previewEntry.value?.path).toBe(entry.path)
    expect(view.selected.value.size).toBe(0)
    expect(mobileEvent.preventDefault).toHaveBeenCalledOnce()

    window.innerWidth = 1280
    view.previewEntry.value = undefined
    view.handleEntryClick({ shiftKey: false, ctrlKey: false, metaKey: false } as MouseEvent, entry)
    expect(view.previewEntry.value).toBeUndefined()
    expect(view.selected.value).toEqual(new Set([entry.path]))
  })

  it('prevents browser text selection inside the file list', () => {
    const view = setupView()
    const preventDefault = vi.fn()

    view.preventNativeSelection({ preventDefault } as unknown as Event)

    expect(preventDefault).toHaveBeenCalledOnce()
  })

  it('clears a single selection when the selected row is clicked again', () => {
    const view = setupView()
    const entry = testEntry('toggle.txt')
    view.directory.value = { path: '/', entries: [entry] }

    view.selectEntry({} as MouseEvent, entry.path)
    view.selectEntry({} as MouseEvent, entry.path)

    expect([...view.selected.value]).toEqual([])
  })

  it('inverts the selection across the visible entries', () => {
    const view = setupView()
    const first = testEntry('first.txt')
    const second = testEntry('second.txt')
    const third = testEntry('third.txt')
    view.directory.value = { path: '/', entries: [first, second, third] }
    view.selected.value = new Set([first.path, third.path])

    view.invertSelection()

    expect([...view.selected.value]).toEqual([second.path])
  })

  it('selects every visible entry with control-a', () => {
    const view = setupView()
    const first = testEntry('a.txt')
    const second = testEntry('b.txt')
    view.directory.value = { path: '/', entries: [first, second] }
    const preventDefault = vi.fn()
    const target = { matches: vi.fn(() => false) } as unknown as HTMLElement
    view.filesPage.value = {
      contains: (node: Node | null) => node === target,
    } as HTMLElement

    view.handleFileShortcut({
      key: 'a',
      ctrlKey: true,
      preventDefault,
      target,
    } as unknown as KeyboardEvent)

    expect(preventDefault).toHaveBeenCalled()
    expect([...view.selected.value]).toEqual([first.path, second.path])
  })

  it('ignores destructive shortcuts when focus is outside the files page', () => {
    const view = setupView()
    const entry = testEntry('outside.txt')
    view.directory.value = { path: '/', entries: [entry] }
    view.selected.value = new Set([entry.path])
    view.filesPage.value = { contains: () => false } as unknown as HTMLElement
    const preventDefault = vi.fn()

    view.handleFileShortcut({
      key: 'Delete',
      preventDefault,
      target: { matches: vi.fn(() => false) },
    } as unknown as KeyboardEvent)

    expect(preventDefault).not.toHaveBeenCalled()
    expect(view.dialogAction.value).toBeUndefined()
  })

  it('ignores file shortcuts while a toolbar or menu button has focus', () => {
    const view = setupView()
    const entry = testEntry('button-focus.txt')
    const target = { matches: vi.fn(() => true) } as unknown as HTMLElement
    view.directory.value = { path: '/', entries: [entry] }
    view.selected.value = new Set([entry.path])
    view.filesPage.value = { contains: (node: Node | null) => node === target } as HTMLElement
    const preventDefault = vi.fn()

    view.handleFileShortcut({ key: 'Delete', preventDefault, target } as unknown as KeyboardEvent)

    expect(preventDefault).not.toHaveBeenCalled()
    expect(view.dialogAction.value).toBeUndefined()
  })

  it('copies to the page clipboard, clears selection, and does not execute a file action', () => {
    const view = setupView()
    const checked = testEntry('checked.txt')
    const clicked = testEntry('clicked.txt')
    view.directory.value = { path: '/', entries: [checked, clicked] }
    view.selected.value = new Set([checked.path])

    view.setClipboard('copy', clicked)

    expect(view.clipboard.value?.mode).toBe('copy')
    expect(view.clipboard.value?.entries.map((entry) => entry.path)).toEqual([clicked.path])
    expect([...view.selected.value]).toEqual([])
    expect(mocks.action).not.toHaveBeenCalled()
  })

  it('opens a current-directory context menu from blank space', () => {
    const view = setupView()
    const preventDefault = vi.fn()

    view.showDirectoryContext({
      preventDefault,
      clientX: 400,
      clientY: 300,
      target: { closest: vi.fn(() => null) },
    } as unknown as MouseEvent)

    expect(preventDefault).toHaveBeenCalled()
    expect(view.contextMenu.value).toEqual({ x: 400, y: 300 })
  })

  it('pastes copied entries into the current directory and keeps the clipboard', async () => {
    const view = setupView()
    const entry = testEntry('source.txt')
    view.currentPath.value = '/target'
    view.directory.value = { path: '/', entries: [entry] }
    view.setClipboard('copy', entry)
    mocks.action.mockResolvedValueOnce({
      action: 'copy',
      succeeded: [{ path: entry.path, destination: '/target/source.txt' }],
      failed: [],
    })

    await view.pasteClipboard()

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'copy',
      sources: [entry.path],
      target: '/target',
      expectedResourceVersions: { [entry.path]: entry.resourceVersion },
    })
    expect(view.clipboard.value?.entries).toEqual([entry])
    expect(mocks.list).toHaveBeenCalled()
    expect(mocks.success).not.toHaveBeenCalled()
  })

  it('keeps only failed entries after a partial cut paste', async () => {
    const view = setupView()
    const moved = testEntry('moved.txt')
    const failed = testEntry('failed.txt')
    view.directory.value = { path: '/', entries: [moved, failed] }
    view.selected.value = new Set([moved.path, failed.path])
    view.setClipboard('move', moved)
    mocks.action.mockResolvedValueOnce({
      action: 'move',
      succeeded: [{ path: moved.path, destination: `/target/${moved.name}` }],
      failed: [{ path: failed.path, detail: '目标已存在' }],
    })

    await view.pasteClipboard('/target')

    expect(view.clipboard.value?.mode).toBe('move')
    expect(view.clipboard.value?.entries.map((entry) => entry.path)).toEqual([failed.path])
    expect(view.fileTransferState.value).toMatchObject({
      mode: 'move', target: '/target', count: 1, phase: 'partial',
      detail: '1 项成功，1 项失败：目标已存在',
    })
    expect(mocks.danger).not.toHaveBeenCalled()
  })

  it('moves a native file-window drag with version protection and completion feedback', async () => {
    const view = setupView()
    const entry = { ...testEntry('project.txt'), path: '/source/project.txt' }
    const event = internalDrag([entry])
    view.currentPath.value = '/target'
    mocks.action.mockResolvedValueOnce({
      action: 'move',
      succeeded: [{ path: entry.path, destination: '/target/project.txt' }],
      failed: [],
    })

    await view.transferInternalFileDrop(event, '/target')

    expect(mocks.action).toHaveBeenCalledWith({
      action: 'move',
      sources: [entry.path],
      target: '/target',
      expectedResourceVersions: { [entry.path]: entry.resourceVersion },
    }, undefined)
    expect(view.fileTransferState.value).toMatchObject({
      mode: 'move', target: '/target', count: 1, phase: 'success',
    })
    expect(mocks.success).not.toHaveBeenCalled()
    expect(window.setTimeout).toHaveBeenCalledWith(expect.any(Function), 2200)
  })

  it('allows a copy to be cancelled without claiming that completed copies were removed', async () => {
    const view = setupView()
    const entry = { ...testEntry('project.txt'), path: '/source/project.txt' }
    const event = internalDrag([entry], { ctrlKey: true })
    mocks.action.mockImplementationOnce((_input: unknown, signal?: AbortSignal) => new Promise((_resolve, reject) => {
      signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))

    const transfer = view.transferInternalFileDrop(event, '/target')
    expect(view.fileTransferState.value?.phase).toBe('running')
    view.cancelFileTransfer()
    await transfer

    expect(view.fileTransferState.value).toMatchObject({ mode: 'copy', phase: 'cancelled' })
    expect(view.fileTransferState.value?.detail).toBe('已经复制完成的项目会保留在目标目录。')
    expect(mocks.show).not.toHaveBeenCalled()
  })

  it('copies a multi-selection from another KPanel into the dropped directory', async () => {
    const view = setupView()
    const first = { ...testEntry('one.txt'), path: '/source/one.txt' }
    const second = { ...testEntry('two.txt'), path: '/source/two.txt' }
    const event = crossPanelDrag([first, second])
    mocks.transferFromPanel
      .mockResolvedValueOnce({ ...first, path: '/target/one.txt' })
      .mockRejectedValueOnce(new Error('source changed'))

    await view.transferCrossPanelFileDrop(event, '/target')

    expect(mocks.transferFromPanel).toHaveBeenCalledTimes(2)
    expect(mocks.transferFromPanel.mock.calls[0]?.[0]).toEqual({
      sourceNodeId: 'a'.repeat(32), path: '/source/one.txt',
      resourceVersion: first.resourceVersion, targetDirectory: '/target',
    })
    expect(view.fileTransferState.value).toMatchObject({
      mode: 'copy', target: '/target', count: 2, completed: 2, phase: 'partial', remote: true,
    })
    expect(view.fileTransferState.value?.detail).toBe('1 项成功，1 项失败：source changed')
    expect(mocks.danger).not.toHaveBeenCalled()
    view.dismissFileTransfer()
    expect(view.fileTransferState.value).toBeUndefined()
    expect(mocks.list).toHaveBeenCalled()
  })
})
