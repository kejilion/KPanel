// @vitest-environment jsdom
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { resetApiSecurityState } from '@/lib/api'
import { notifyFileDirectoriesChanged, resetFileWindowTransferForTest } from '@/lib/fileWindowTransfer'
import { hasDesktopFileDrag } from '@/lib/desktopFileShortcuts'
import { fileAPIForHost } from '@/lib/fileHostContext'

vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => ({ query: {} }), useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ show: vi.fn(), success: vi.fn(), danger: vi.fn() }) }))

const hosts = ['local', 'a', 'b'].map((id) => ({
  id, name: id, isLocal: id === 'local', kind: 'panel', state: 'online',
  fileManagementAvailable: true, remoteNodeId: id.repeat(32),
}))
const requests: { url: URL; method: string; body: unknown }[] = []
const wrappers: ReturnType<typeof shallowMount>[] = []
const entry = { name: 'test.txt', path: '/test.txt', kind: 'file', editable: true, resourceVersion: 'v1' }
let availableHosts = hosts
let respond: ((url: URL, init?: RequestInit) => Response | Promise<Response> | undefined) | undefined

function dragEvent(): DragEvent {
  const data = new Map<string, string>()
  return {
    preventDefault: vi.fn(), ctrlKey: false, altKey: false,
    dataTransfer: {
      get types() { return [...data.keys()] },
      setData: (type: string, value: string) => data.set(type, value),
      getData: (type: string) => data.get(type) || '',
    },
  } as unknown as DragEvent
}

beforeEach(() => {
  requests.length = 0
  availableHosts = hosts
  respond = undefined
  resetApiSecurityState()
  resetFileWindowTransferForTest()
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(input, 'http://localhost')
    requests.push({ url, method: init?.method || 'GET', body: init?.body ? JSON.parse(String(init.body)) : undefined })
    const response = respond?.(url, init)
    if (response) return response
    let body: unknown = {}
    if (url.pathname.endsWith('/cluster/hosts')) body = { nodeId: 'c'.repeat(32), items: availableHosts }
    if (url.pathname.endsWith('/files')) body = { path: url.searchParams.get('path'), entries: [entry] }
    if (url.pathname.endsWith('/remote-downloads')) body = { items: [] }
    if (url.pathname.endsWith('/actions')) body = { action: 'mkdir', succeeded: [], failed: [] }
    if (url.pathname.endsWith('/entry')) body = entry
    if (url.pathname.endsWith('/content') && init?.method === 'PUT') body = { entry: { ...entry, resourceVersion: 'v2' } }
    return new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } })
  }))
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.unstubAllGlobals()
})
async function windowFor(host: string) {
  const wrapper = shallowMount(FilesView)
  wrappers.push(wrapper)
  await flushPromises()
  // Invoke actual setup functions: API/fetch routing is deliberately not mocked.
  const vm = wrapper.vm as unknown as Record<string, any>
  vm.handleFileHostSelection(hosts.find((item) => item.id === host))
  await flushPromises()
  return { wrapper, vm }
}

describe('FilesView real API multi-window host context', () => {
  it('keeps A reads and writes on A after B switches and unmounts', async () => {
    const a = await windowFor('a')
    const b = await windowFor('b')
    requests.length = 0
    await a.vm.loadDirectory('/')
    a.vm.openDialog('mkdir')
    a.vm.dialogValue = 'new-folder'
    await a.vm.submitDialog()
    expect(requests.filter((request) => request.url.pathname.includes('/files')).map((request) => request.url.searchParams.get('hostId'))).toEqual(['a', 'a', 'a'])
    b.wrapper.unmount()
    wrappers.splice(wrappers.indexOf(b.wrapper), 1)
    requests.length = 0
    await a.vm.loadDirectory('/')
    expect(requests[0]?.url.searchParams.get('hostId')).toBe('a')
  })

  it('does not refresh or remap a same-path directory on another host', async () => {
    const a = await windowFor('a')
    await windowFor('b')
    requests.length = 0
    notifyFileDirectoriesChanged(['/'], undefined, [], 'b')
    await flushPromises()
    expect(requests.filter((request) => request.url.pathname.endsWith('/files')).map((request) => request.url.searchParams.get('hostId'))).toEqual(['b'])
    expect(a.vm.currentPath).toBe('/')
    notifyFileDirectoriesChanged(['/'], undefined, [{ source: '/', destination: '/moved' }], 'b')
    await flushPromises()
    expect(a.vm.currentPath).toBe('/')
  })

  it('keeps text, save, thumbnail, downloads, archive, trash and entry requests on their explicit host', async () => {
    const a = await windowFor('a')
    await windowFor('b')
    requests.length = 0
    await a.vm.openPreview(entry)
    a.vm.previewDirty = true
    await a.vm.savePreview('edited on a')
    const files = a.vm.fileAPI as ReturnType<typeof fileAPIForHost>
    await files.entry(entry.path)
    await files.entries([entry.path])
    await files.trash()
    for (const url of [files.contentUrl(entry.path, 'inline'), files.contentUrl(entry.path, 'attachment'), files.thumbnailUrl(entry.path, 'v1'), files.archiveUrl([entry], 'test.zip')]) {
      expect(new URL(url, 'http://localhost').searchParams.get('hostId')).toBe('a')
    }
    expect(requests.every((request) => request.url.searchParams.get('hostId') === 'a')).toBe(true)
    expect(requests.find((request) => request.method === 'PUT')?.body).toEqual({ content: 'edited on a', expectedResourceVersion: 'v1' })
  })

  it('captures the host for every file in an upload while another window changes hosts', async () => {
    const uploads: { url: URL; complete: () => void }[] = []
    vi.stubGlobal('XMLHttpRequest', class {
      upload = {}
      status = 200
      response = entry
      onload = () => {}
      open(_method: string, url: string) { uploads.push({ url: new URL(url, 'http://localhost'), complete: () => this.onload() }) }
      setRequestHeader() {}
      send() {}
    })
    const a = await windowFor('a')
    const b = await windowFor('b')
    const uploading = a.vm.uploadFiles([new File(['one'], 'one.txt'), new File(['two'], 'two.txt')])
    expect(uploads).toHaveLength(1)
    b.vm.handleFileHostSelection(hosts[0])
    await flushPromises()
    uploads[0]!.complete()
    await flushPromises()
    expect(uploads).toHaveLength(2)
    uploads[1]!.complete()
    await uploading
    expect(uploads.map((upload) => upload.url.searchParams.get('hostId'))).toEqual(['a', 'a'])
  })

  it('does not paste another host clipboard or erase a newer clipboard after an awaited move', async () => {
    const a = await windowFor('a')
    const b = await windowFor('b')
    a.vm.setClipboard('move', entry)
    requests.length = 0
    await b.vm.pasteClipboard('/target')
    expect(requests).toHaveLength(0)
    let finish!: (response: Response) => void
    respond = (url) => url.pathname.endsWith('/actions') ? new Promise((resolve) => { finish = resolve }) : undefined
    const pasting = a.vm.pasteClipboard('/target')
    await flushPromises()
    b.vm.setClipboard('copy', entry)
    finish(new Response(JSON.stringify({ action: 'move', succeeded: [], failed: [] }), { headers: { 'content-type': 'application/json' } }))
    await pasting
    expect(b.vm.clipboard.hostId).toBe('b')
    expect(b.vm.clipboard.mode).toBe('copy')
  })

  it('uses the existing cross-host transfer protocol for same-page drags between different hosts', async () => {
    const a = await windowFor('a')
    const b = await windowFor('b')
    respond = (url) => url.pathname.endsWith('/transfers')
      ? new Response(JSON.stringify({ state: 'complete', entry: { ...entry, path: '/target/test.txt' } }) + '\n', { headers: { 'content-type': 'application/x-ndjson' } }) : undefined
    const event = dragEvent()
    a.vm.startEntryDrag(event, entry)
    requests.length = 0
    await b.vm.transferInternalFileDrop(event, '/target')
    const transfers = requests.filter((request) => request.method === 'POST')
    expect(transfers).toHaveLength(1)
    expect(transfers[0]?.url.pathname).toBe('/api/v1/files/transfers')
    expect(transfers[0]?.url.searchParams.get('hostId')).toBe('b')
    expect(transfers[0]?.body).toMatchObject({ sourceNodeId: 'a'.repeat(32), path: entry.path, targetDirectory: '/target', resourceVersion: 'v1' })
  })

  it('closing B does not clear A active drag', async () => {
    const a = await windowFor('a')
    const b = await windowFor('b')
    const event = dragEvent()
    a.vm.startEntryDrag(event, entry)
    b.wrapper.unmount()
    wrappers.splice(wrappers.indexOf(b.wrapper), 1)
    expect(hasDesktopFileDrag(event)).toBe(true)
  })

  it.each([403, 503])('retains a missing/revoked host and fails closed without replay on HTTP %s', async (status) => {
    const a = await windowFor('a')
    availableHosts = [hosts[0]!]
    await a.vm.loadFileHosts()
    expect(a.vm.fileHostId).toBe('a')
    requests.length = 0
    respond = (url) => url.pathname.includes('/files')
      ? new Response(JSON.stringify({ code: 'host_unavailable', detail: 'unavailable' }), { status, headers: { 'content-type': 'application/json' } }) : undefined
    await a.vm.loadDirectory('/')
    a.vm.openDialog('mkdir')
    a.vm.dialogValue = 'must-not-be-local'
    await a.vm.submitDialog()
    expect(requests.filter((request) => request.method === 'POST')).toHaveLength(1)
    expect(requests.every((request) => request.url.searchParams.get('hostId') === 'a')).toBe(true)
  })

  it('ignores an old text response after switching the same window host', async () => {
    const a = await windowFor('a')
    let finish!: (response: Response) => void
    respond = (url) => url.pathname.endsWith('/content') ? new Promise((resolve) => { finish = resolve }) : undefined
    const opening = a.vm.openPreview(entry)
    await flushPromises()
    a.vm.handleFileHostSelection(hosts[2])
    await flushPromises()
    finish(new Response('old host content'))
    await opening
    expect(a.vm.fileHostId).toBe('b')
    expect(a.vm.previewEntry).toBeUndefined()
    expect(a.vm.previewContent).toBe('')
  })
})
