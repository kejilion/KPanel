// @vitest-environment jsdom
import { shallowMount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { computed } from 'vue'
import FilesView from './FilesView.vue'
import { createWindowRouter, reactiveRouteFor } from '@/lib/desktopWindowRoute'
import { desktopWindowActiveKey, windowRouterKey, windowRouteKey } from '@/lib/desktopRouteKeys'
import { resetFileWindowTransferForTest, useFileClipboard } from '@/lib/fileWindowTransfer'
import { api, ApiError } from '@/lib/api'
import type { FileEntry } from '@/types/api'

const mocks = vi.hoisted(() => ({ upload: vi.fn(), action: vi.fn(), list: vi.fn(), danger: vi.fn() }))
vi.mock('@/lib/api', async (original) => {
  const module = await original<typeof import('@/lib/api')>()
  return { ...module, api: { ...module.api,
    cluster: { hosts: vi.fn(async () => ({ items: [], nodeId: '' })) },
    files: { ...module.api.files, list: mocks.list, upload: mocks.upload, action: mocks.action,
      remoteDownloadJobs: vi.fn(async () => ({ items: [] })),
    },
  } }
})
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: vi.fn(), show: vi.fn(), danger: mocks.danger }) }))
const entry = { name: 'note.txt', path: '/home/note.txt', kind: 'file', resourceVersion: 'v1' } as FileEntry
const wrappers: VueWrapper[] = []
beforeEach(() => {
  vi.clearAllMocks()
  resetFileWindowTransferForTest()
  mocks.list.mockImplementation(async (path: string) => ({ path, entries: [entry], total: 1 }))
  mocks.action.mockResolvedValue({ action: 'copy', succeeded: [{ path: entry.path, destination: '/home/copy.txt' }], failed: [] })
  mocks.upload.mockImplementation(async (path: string, file: File) => ({ ...entry, name: file.name, path: `${path}/${file.name}` }))
})
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.classList.remove('has-modal')
  vi.restoreAllMocks()
})

async function page(path = '/files?path=/home', active = true) {
  const router = createWindowRouter(path)
  await router.isReady()
  const wrapper = shallowMount(FilesView, { attachTo: document.body, global: { provide: {
    [windowRouterKey as symbol]: router, [windowRouteKey as symbol]: reactiveRouteFor(router),
    [desktopWindowActiveKey as symbol]: computed(() => active),
  } } })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

function clipboardEvent(type: string, files: File[] = [], text = '') {
  const event = new Event(type, { bubbles: true, cancelable: true })
  const data = { files, items: [], getData: () => text, setData: vi.fn((_type: string, value: string) => { text = value }) }
  Object.defineProperty(event, 'clipboardData', { value: data })
  return { event, data }
}

it('pastes screenshot and named files into the current remote directory, ahead of stale internal files', async () => {
  const wrapper = await page('/files?path=/home&hostId=edge')
  useFileClipboard().set('copy', [entry], 'edge')
  const file = new File(['hi'], 'hello.txt')
  const files = [new File(['pixels'], 'image.png', { type: 'image/png' }), file]
  const { event } = clipboardEvent('paste', files)
  wrapper.element.dispatchEvent(event)
  files.length = 0 // browser data store is no longer readable after dispatch
  await flushPromises()
  expect(event.defaultPrevented).toBe(true)
  expect(mocks.upload).toHaveBeenCalledTimes(2)
  expect(mocks.upload.mock.calls[0]![1].name).toMatch(/^clipboard-\d+-1\.png$/)
  expect(mocks.upload).toHaveBeenCalledWith('/home', file, false, expect.any(Function), undefined, 'edge')
  expect(mocks.action).not.toHaveBeenCalled()
})

it.each(['copy', 'cut'])('keeps native %s / paste working for internal files and does not swallow the paste keystroke', async (type) => {
  const wrapper = await page()
  const vm = wrapper.vm as unknown as { selected: Set<string> }
  vm.selected = new Set([entry.path])
  const copied = clipboardEvent(type)
  wrapper.element.dispatchEvent(copied.event)
  expect(copied.data.setData).toHaveBeenCalledWith('text/plain', entry.path)
  expect(useFileClipboard().clipboard.value?.mode).toBe(type === 'cut' ? 'move' : 'copy')
  const keydown = new KeyboardEvent('keydown', { key: 'v', ctrlKey: true, bubbles: true, cancelable: true })
  wrapper.element.dispatchEvent(keydown)
  expect(keydown.defaultPrevented).toBe(false)
  const { event } = clipboardEvent('paste', [], entry.path)
  wrapper.element.dispatchEvent(event)
  await flushPromises()
  expect(mocks.action).toHaveBeenCalledWith(expect.objectContaining({ action: type === 'cut' ? 'move' : 'copy', target: '/home' }), undefined, '')
  expect(mocks.upload).not.toHaveBeenCalled()
})

it('keeps text, edited controls, modal dialogs, inactive windows and other panes out of the upload path', async () => {
  const wrapper = await page()
  const inactive = await page('/files?path=/other', false)
  useFileClipboard().set('copy', [entry])
  const file = new File(['hi'], 'test.png', { type: 'image/png' })
  const input = wrapper.get('input').element
  const editable = document.createElement('div')
  editable.setAttribute('contenteditable', '')
  const child = document.createElement('span')
  editable.append(child); wrapper.element.append(editable)
  for (const target of [input, child, inactive.element, document.body]) {
    const { event } = clipboardEvent('paste', [file])
    target.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
  }
  const textPaste = clipboardEvent('paste', [], 'https://example.com/image.png')
  wrapper.element.dispatchEvent(textPaste.event)
  const handled = clipboardEvent('paste', [file]); handled.event.preventDefault()
  wrapper.element.dispatchEvent(handled.event)
  document.body.classList.add('has-modal')
  const modal = clipboardEvent('paste', [file]); wrapper.element.dispatchEvent(modal.event)
  await flushPromises()
  expect(mocks.upload).not.toHaveBeenCalled()
  expect(mocks.action).not.toHaveBeenCalled()
})

it('uploads only in the focused pane and removes paste listeners on unmount', async () => {
  const first = await page('/files?path=/first')
  await page('/files?path=/second')
  first.element.dispatchEvent(clipboardEvent('paste', [new File(['x'], 'one.txt')]).event)
  await flushPromises()
  expect(mocks.upload).toHaveBeenCalledTimes(1)
  expect(mocks.upload.mock.calls[0]![0]).toBe('/first')
  const remove = vi.spyOn(window, 'removeEventListener')
  first.unmount(); wrappers.splice(wrappers.indexOf(first), 1)
  expect(remove).toHaveBeenCalledWith('paste', expect.any(Function))
})

it('shows failed uploads and retains the existing overwrite confirmation for a name conflict', async () => {
  const wrapper = await page()
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  mocks.upload.mockRejectedValueOnce(new ApiError('exists', 409, 'conflict'))
  wrapper.element.dispatchEvent(clipboardEvent('paste', [new File(['x'], 'one.txt')]).event)
  await flushPromises()
  expect(confirm).toHaveBeenCalledOnce()
  expect(wrapper.text()).toContain('上传失败')
  expect(mocks.upload).toHaveBeenCalledTimes(1)
  expect(api.files.upload).toBe(mocks.upload)
})
