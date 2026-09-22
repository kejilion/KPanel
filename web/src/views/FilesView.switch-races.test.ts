// @vitest-environment jsdom
import { createSSRApp, effectScope, nextTick, reactive, ssrContextKey } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { ApiError } from '@/lib/api'

const m = vi.hoisted(() => ({
  route: undefined as any, router: undefined as any,
  push: vi.fn(), replace: vi.fn(), show: vi.fn(), danger: vi.fn(), success: vi.fn(),
  files: { list: vi.fn(), entry: vi.fn(), text: vi.fn(), trash: vi.fn(), action: vi.fn(), upload: vi.fn(), contentUrl: vi.fn(), archiveUrl: vi.fn(), thumbnailUrl: vi.fn(), remoteDownloadJobs: vi.fn() },
}))
vi.mock('vue-router', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => m.route, useRouter: () => m.router || ({ push: m.push, replace: m.replace }) }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error { constructor(message: string, public status = 0) { super(message) } }, api: { files: m.files, desktop: {} } }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ show: m.show, danger: m.danger, success: m.success }) }))

const scopes: ReturnType<typeof effectScope>[] = []
function view() {
  const app = createSSRApp({ render: () => null })
  app.provide(ssrContextKey, { modules: new Set<string>() })
  const scope = effectScope(); scopes.push(scope)
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
  try { return scope.run(() => app.runWithContext(() => (FilesView as any).setup({}, { expose() {} }))) as any }
  finally { warn.mockRestore() }
}
function pending<T = any>() {
  let resolve!: (value: T) => void; let reject!: (error: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
function directory(path: string) { return { path, entries: [], offset: 0, total: 0, truncated: false, readAt: '2026-09-13T00:00:00Z' } }
function entry(name: string) { return { name, path: '/' + name, kind: 'file', mime: 'image/png', editable: false, previewable: true, sizeBytes: 4, resourceVersion: name, modifiedAt: '', mode: '', owner: '', group: '' } }
function host(id: string) { return { id, name: id, kind: 'panel', isLocal: false, state: 'online', federationProtocol: 'v2', fileManagementAvailable: true } }
function ready(v: any, path = '/only-on-A') { v.currentPath.value = path; v.directory.value = directory(path); v.activeFileHostId.value = 'A' }
beforeEach(() => {
  vi.resetAllMocks()
  m.router = undefined
  m.route = reactive({ query: { hostId: 'A', path: '/only-on-A' } as Record<string, unknown> })
  m.push.mockImplementation(async (location: any) => { m.route.query = location.query || {} })
  m.replace.mockImplementation(async (location: any) => { m.route.query = location.query || {} })
  m.files.list.mockImplementation(async (path: string) => directory(path))
  m.files.upload.mockResolvedValue({})
  m.files.action.mockResolvedValue({ action: 'mkdir', succeeded: [], failed: [] })
  m.files.remoteDownloadJobs.mockResolvedValue({ items: [] })
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.restoreAllMocks() })

describe('file window host context', () => {
  it('ignores a cancelled directory read even when its transport resolves late', async () => {
    const v = view(); const old = pending()
    m.files.list.mockImplementationOnce(() => old.promise)
    const oldRead = v.loadDirectory('/old-A')
    v.resetFileHostContext('B'); await v.loadDirectory('/B')
    old.resolve(directory('/old-A')); await oldRead
    expect(v.directory.value.path).toBe('/B')
    expect(m.files.list.mock.calls[0]![2].aborted).toBe(true)
  })

  it.each(['pending', 'failed'] as const)('blocks writes while the new host directory is %s', async state => {
    const v = view(); ready(v)
    const loadingB = pending()
    m.files.list.mockImplementation(() => loadingB.promise)
    v.handleFileHostSelection(host('B')); await nextTick()
    if (state === 'failed') { loadingB.reject(new Error('unavailable')); await flushPromises() }
    expect(v.fileHostId.value).toBe('B')
    expect(v.currentPath.value).toBe('/')
    expect(v.directoryReady.value).toBe(false)
    await v.uploadFiles([new File(['test'], 'file.txt')])
    v.openDialog('mkdir'); v.dialogValue.value = 'new'; await v.submitDialog()
    await v.pasteClipboard(); await v.onDrop({ dataTransfer: { files: [new File(['test'], 'drop.txt')] } })
    expect(m.files.upload).not.toHaveBeenCalled()
    expect(m.files.action).not.toHaveBeenCalled()
    if (state === 'pending') {
      loadingB.resolve(directory('/')); await flushPromises()
      await v.uploadFiles([new File(['test'], 'file.txt')])
      expect(m.files.upload).toHaveBeenCalledWith('/', expect.any(File), false, expect.any(Function), undefined, 'B')
    }
  })

  it.each([false, true])('ignores stale file lookups (host ABA: %s)', async aba => {
    const v = view(); const old = pending()
    m.files.entry.mockImplementationOnce(() => old.promise).mockResolvedValueOnce(entry('new.png'))
    const oldRead = v.openRequestedFile('/old.png')
    if (aba) { v.resetFileHostContext('B'); v.resetFileHostContext('A') }
    await v.openRequestedFile('/new.png')
    old.resolve(entry('old.png')); await oldRead
    expect(v.previewEntry.value.name).toBe('new.png')
  })

  it('does not replace a directly opened preview or its edits with a late route lookup', async () => {
    const v = view(); const old = pending()
    m.files.entry.mockImplementationOnce(() => old.promise)
    const oldRead = v.openRequestedFile('/old.png')
    await v.openPreview(entry('chosen.png'))
    v.previewDirty.value = true
    old.resolve(entry('old.png')); await oldRead
    expect(v.previewEntry.value.name).toBe('chosen.png')
    expect(v.previewDirty.value).toBe(true)
  })

  it('invalidates a pending route lookup when the user opens an archive browser', async () => {
    const v = view(); const old = pending(); const browse = vi.fn()
    m.files.entry.mockImplementationOnce(() => old.promise)
    const oldRead = v.openRequestedFile('/old.png')
    v.archiveTools.value = { available: true, checking: false, browse }
    await v.openPreview(entry('chosen.zip'))
    expect(browse).toHaveBeenCalledTimes(1)
    old.resolve(entry('old.png')); await oldRead
    expect(v.previewEntry.value).toBeUndefined()
  })

  it('suppresses an obsolete file lookup error after switching away and back', async () => {
    const v = view(); const old = pending()
    m.files.entry.mockImplementationOnce(() => old.promise)
    const oldRead = v.openRequestedFile('/old.png')
    v.resetFileHostContext('B'); v.resetFileHostContext('A')
    old.reject(new Error('old host failed')); await oldRead
    expect(m.danger).not.toHaveBeenCalled()
  })

  it.each([false, true])('keeps newer trash data and loading state (host ABA: %s)', async aba => {
    const v = view(); const old = pending(); const fresh = pending()
    m.files.trash.mockImplementationOnce(() => old.promise).mockImplementationOnce(() => fresh.promise)
    const oldRead = v.loadTrash()
    if (aba) { v.resetFileHostContext('B'); v.resetFileHostContext('A') }
    const newRead = v.loadTrash()
    old.resolve({ entries: [{ id: 'stale' }], total: 1, truncated: false }); await oldRead
    expect(v.trashLoading.value).toBe(true); expect(v.trashEntries.value).toEqual([])
    fresh.resolve({ entries: [{ id: 'fresh' }], total: 1, truncated: false }); await newRead
    expect(v.trashEntries.value[0].id).toBe('fresh'); expect(v.trashLoading.value).toBe(false)
  })

  it('keeps active uploads on A through both menu and route switch attempts', async () => {
    const v = view(); ready(v); const first = pending()
    m.files.upload.mockImplementationOnce(() => first.promise).mockResolvedValue({})
    const uploading = v.uploadFiles([new File(['1'], 'first.txt'), new File(['2'], 'second.txt')])
    v.handleFileHostSelection(host('B'))
    m.route.query = { hostId: 'B', path: '/' }; await flushPromises()
    expect(v.fileHostId.value).toBe('A')
    expect(m.route.query).toEqual({ hostId: 'A', path: '/only-on-A' })
    expect(v.uploadTasks.value).toHaveLength(1)
    first.resolve({}); await uploading
    expect(m.files.upload.mock.calls.map(call => [call[0], call[5]])).toEqual([['/only-on-A', 'A'], ['/only-on-A', 'A']])
    v.handleFileHostSelection(host('B')); await flushPromises()
    expect(v.fileHostId.value).toBe('B'); expect(v.uploadTasks.value).toEqual([])
  })

  it('restores host, path and file when browser history tries to leave a busy host', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/files', name: 'files', component: { render: () => null } }] })
    await router.push({ name: 'files', query: { hostId: 'B', path: '/' } })
    await router.push({ name: 'files', query: { hostId: 'A', path: '/only-on-A', file: '/new.png' } })
    m.router = router; m.route = reactive({ get query() { return router.currentRoute.value.query } })
    m.files.entry.mockResolvedValue(entry('new.png'))
    const v = view(); ready(v); const first = pending()
    m.files.upload.mockImplementationOnce(() => first.promise)
    const uploading = v.uploadFiles([new File(['1'], 'first.txt')])
    router.back(); await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ hostId: 'A', path: '/only-on-A', file: '/new.png' })
    expect(v.fileHostId.value).toBe('A')
    router.forward(); await flushPromises()
    expect(v.fileHostId.value).toBe('A')
    first.resolve({}); await uploading
    await router.push({ name: 'files', query: { hostId: 'B', path: '/' } }); await flushPromises()
    expect(v.fileHostId.value).toBe('B')
  })

  it('rediscovers local background downloads when a route returns from a remote host', async () => {
    const v = view(); ready(v)
    m.files.remoteDownloadJobs.mockResolvedValueOnce({ items: [{ id: 'local-download', state: 'complete' }] })
    m.route.query = { path: '/home' }; await flushPromises()
    expect(v.fileHostId.value).toBe('')
    expect(m.files.remoteDownloadJobs).toHaveBeenCalledTimes(1)
    expect(v.remoteDownloadJobs.value[0].id).toBe('local-download')
  })

  it('does not reopen the file or clear unsaved edits when a history switch is refused', async () => {
    m.route.query.file = '/edit.txt'
    const v = view(); ready(v)
    v.previewEntry.value = entry('edit.txt'); v.previewDirty.value = true
    v.previewContent.value = 'unsaved text'
    vi.mocked(window.confirm).mockReturnValue(false)
    m.route.query = { hostId: 'B', path: '/', file: '/other.txt' }; await flushPromises()
    expect(v.fileHostId.value).toBe('A')
    expect(v.previewDirty.value).toBe(true); expect(v.previewContent.value).toBe('unsaved text')
    expect(m.files.entry).not.toHaveBeenCalled()
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(m.route.query).toEqual({ hostId: 'A', path: '/only-on-A', file: '/edit.txt' })
  })

  it('keeps a directory form target fixed if the same host navigates before submit', async () => {
    const v = view(); ready(v)
    v.openDialog('mkdir'); v.dialogValue.value = 'new'
    await v.loadDirectory('/other')
    await v.submitDialog()
    expect(m.files.action).toHaveBeenCalledWith({ action: 'mkdir', target: '/only-on-A', name: 'new' }, undefined, 'A')
  })

  it('discards a native picker result after switching away and back', async () => {
    const v = view(); ready(v)
    v.uploadInput.value = { click: vi.fn(), value: '' }; v.selectUploadFiles()
    v.resetFileHostContext('B'); v.resetFileHostContext('A'); ready(v)
    await v.onUploadFilesSelected({ target: { files: [new File(['1'], 'stale.txt')], value: 'stale.txt' } })
    expect(m.files.upload).not.toHaveBeenCalled()
  })

  it('keeps overwrite retries bound to the original host and directory', async () => {
    const v = view(); ready(v)
    m.files.upload.mockRejectedValueOnce(new ApiError('exists', 409)).mockResolvedValueOnce({})
    await v.uploadFiles([new File(['1'], 'exists.txt')])
    expect(m.files.upload.mock.calls.map(call => [call[0], call[2], call[5]])).toEqual([['/only-on-A', false, 'A'], ['/only-on-A', true, 'A']])
  })

  it('does not lock another file window when this window uploads', async () => {
    const a = view(); ready(a); const b = view(); ready(b); const first = pending()
    m.files.upload.mockImplementationOnce(() => first.promise)
    const uploading = a.uploadFiles([new File(['1'], 'a.txt')])
    expect(b.resetFileHostContext('B')).toBe(true)
    expect(a.fileHostId.value).toBe('A'); expect(b.uploadTasks.value).toEqual([])
    first.resolve({}); await uploading
  })
})
