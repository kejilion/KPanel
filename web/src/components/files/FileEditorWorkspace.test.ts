// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, onMounted, ref } from 'vue'
import { EditorState } from '@codemirror/state'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileEditorWorkspace from './FileEditorWorkspace.vue'
import type { FileEntry } from '@/types/api'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  text: vi.fn(),
  write: vi.fn(),
  getValue: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { files: mocks } }))
const Editor = defineComponent({
  props: ['modelValue', 'session'],
  emits: ['change', 'ready'],
  setup(props, { expose, emit }) {
    const value = ref(props.session?.state.doc.toString() ?? props.modelValue)
    expose({
      getValue: () => {
        mocks.getValue()
        return value.value
      },
      getSession: () => ({
        state: EditorState.create({ doc: value.value }),
        scrollTop: 100,
        scrollLeft: 0,
      }),
      markClean: vi.fn(),
      openSearch: vi.fn(),
    })
    onMounted(() => emit('ready', { label: 'JSON' }))
    return () =>
      h('textarea', {
        value: value.value,
        onInput: (event: Event) => {
          value.value = (event.target as HTMLTextAreaElement).value
          emit('change')
        },
      })
  },
})
function entry(name: string): FileEntry {
  return {
    name,
    path: `/demo/${name}`,
    kind: 'file',
    sizeBytes: 10,
    mode: '644',
    owner: 'root',
    group: 'root',
    modifiedAt: '',
    resourceVersion: `v-${name}`,
    editable: true,
    previewable: true,
  }
}
const a = entry('a.json'),
  b = entry('b.json')
const wrappers: ReturnType<typeof mount>[] = []
async function setup(navigationPath?: string) {
  const wrapper = mount(FileEditorWorkspace, {
    props: { entry: a, content: 'original a', hostId: 'remote-a', navigationPath },
    global: { stubs: { CodeEditor: Editor } },
    attachTo: document.body,
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue({ path: '/demo', entries: [a, b], offset: 0, truncated: false })
  mocks.text.mockImplementation(async (path: string) => `loaded ${path}`)
  mocks.write.mockResolvedValue({ entry: { ...a, resourceVersion: 'saved-a' } })
  vi.spyOn(window, 'confirm').mockReturnValue(false)
})
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
describe('file editor workspace', () => {
  it('records confirmed paths while history restoration keeps tabs and drafts without echoing navigation', async () => {
    mocks.list.mockImplementation(async (path: string) => ({
      path: path.replace(/\/$/, ''),
      entries: [a, b],
    }))
    const wrapper = await setup('/demo')
    await wrapper.get('textarea').setValue('draft survives history')
    await wrapper.vm.openFile({ ...a, kind: 'directory', path: '/demo/config/' })
    expect(wrapper.emitted('navigate')).toEqual([['/demo/config']])
    await wrapper.setProps({ navigationPath: '/demo/config' })
    expect(mocks.list).toHaveBeenCalledTimes(2)
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.setProps({ navigationPath: '/demo' })
    await flushPromises()
    expect(wrapper.get('.editor-sidebar__path').text()).toBe('/demo')
    await wrapper.setProps({ navigationPath: '/demo/config' })
    await flushPromises()
    await wrapper.vm.openFile(a)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('draft survives history')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(2)
    expect(wrapper.emitted('navigate')).toHaveLength(1)
    expect(window.confirm).not.toHaveBeenCalled()
  })
  it('cancels a pending navigation when browser history returns to the displayed path', async () => {
    const wrapper = await setup('/demo/config')
    const pending = deferred<unknown>()
    mocks.list.mockReturnValueOnce(pending.promise)
    const slow = wrapper.vm.openFile({ ...a, kind: 'directory', path: '/slow' })
    await wrapper.setProps({ navigationPath: '/demo' })
    await flushPromises()
    pending.resolve({ path: '/slow', entries: [] })
    await slow
    expect(wrapper.get('.editor-sidebar__path').text()).toBe('/demo')
    expect(wrapper.emitted('navigate')).toBeUndefined()
  })
  it('retries the failed destination and only records a successful navigation', async () => {
    const wrapper = await setup('/demo')
    mocks.list.mockRejectedValueOnce(new Error('directory offline'))
    await wrapper.vm.openFile({ ...a, kind: 'directory', path: '/missing' })
    expect(wrapper.emitted('navigate')).toBeUndefined()
    expect(wrapper.get('.editor-sidebar__path').text()).toBe('/demo')
    mocks.list.mockResolvedValueOnce({ path: '/missing', entries: [] })
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(mocks.list.mock.calls.at(-1)?.[0]).toBe('/missing')
    expect(wrapper.emitted('navigate')).toEqual([['/missing']])
  })
  it('keeps compact actions keyboard-accessible with Escape and wrap focus returning to the trigger', async () => {
    let resize!: (entries: unknown[]) => void
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(callback: typeof resize) {
          resize = callback
        }
        observe() {}
        disconnect() {}
      },
    )
    const wrapper = await setup()
    resize([{ contentRect: { width: 390 } }])
    await flushPromises()
    expect(wrapper.find('.editor-toolbar [role="tablist"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="查找或替换"]').exists()).toBe(false)
    const trigger = wrapper.get('button[aria-label="更多操作"]')
    await trigger.trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('[aria-label="查找或替换"]').element)
    await wrapper.get('[aria-label="查找或替换"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('.editor-more__panel').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('click')
    await wrapper.get('[aria-label="切换自动换行"]').trigger('click')
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('click')
    expect(wrapper.get('[aria-label="切换自动换行"]').attributes('aria-pressed')).toBe('true')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(wrapper.find('.editor-more__panel').exists()).toBe(false)
  })
  it('reopens a saved file with its new version from the sidebar', async () => {
    const wrapper = await setup()
    await wrapper.get('textarea').setValue('saved draft')
    await wrapper.get('.editor-save').trigger('click')
    await flushPromises()
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.get('[aria-label="关闭 a.json"]').trigger('click')
    await wrapper
      .findAll('.editor-file')
      .find((button) => button.text() === 'a.json')!
      .trigger('click')
    await flushPromises()
    await wrapper.get('textarea').setValue('next draft')
    await wrapper.get('.editor-save').trigger('click')
    expect(mocks.write).toHaveBeenLastCalledWith(a.path, 'next draft', 'saved-a', 'remote-a')
  })
  it('retains independent drafts, deduplicates paths and only reads whole text on save', async () => {
    const wrapper = await setup()
    await wrapper.get('textarea').setValue('edited a')
    expect(mocks.getValue).not.toHaveBeenCalled()
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.get('textarea').setValue('edited b')
    await wrapper.vm.openFile(a)
    await flushPromises()
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(2)
    expect(wrapper.get('textarea').element.value).toBe('edited a')
    await wrapper.get('.editor-save').trigger('click')
    await flushPromises()
    expect(mocks.write).toHaveBeenCalledWith(a.path, 'edited a', 'v-a.json', 'remote-a')
    expect(wrapper.emitted('dirty')?.at(-1)).toEqual([true])
    await wrapper.vm.openFile(b)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('edited b')
    expect(mocks.text).toHaveBeenCalledTimes(1)
  })
  it('keeps saves bound to the original tab and retains edits made during saving', async () => {
    const pending = deferred<{ entry: FileEntry }>()
    mocks.write.mockReturnValueOnce(pending.promise)
    const wrapper = await setup()
    await wrapper.get('textarea').setValue('first draft')
    await wrapper.get('.editor-save').trigger('click')
    await wrapper.get('textarea').setValue('newer draft')
    await wrapper.vm.openFile(b)
    await flushPromises()
    pending.resolve({ entry: { ...a, resourceVersion: 'v2' } })
    await flushPromises()
    expect(wrapper.get('[aria-selected="true"]').text()).toContain('b.json')
    await wrapper.vm.openFile(a)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('newer draft')
    expect(wrapper.get('.editor-save').attributes('disabled')).toBeUndefined()
    await wrapper.get('.editor-save').trigger('click')
    expect(mocks.write).toHaveBeenLastCalledWith(a.path, 'newer draft', 'v2', 'remote-a')
  })
  it('protects background drafts on close and refresh; confirms once for the last tab', async () => {
    const wrapper = await setup()
    await wrapper.get('textarea').setValue('unsaved')
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.get('[aria-label="关闭 a.json"]').trigger('click')
    expect(window.confirm).toHaveBeenCalledOnce()
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(2)
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    await wrapper.get('[aria-label="关闭 b.json"]').trigger('click')
    vi.mocked(window.confirm).mockReturnValue(true)
    await wrapper.get('[aria-label="关闭 a.json"]').trigger('click')
    expect(wrapper.emitted('dirty')?.at(-1)).toEqual([false])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('keeps a failed save and its draft recoverable without changing the resource version', async () => {
    mocks.write.mockRejectedValueOnce(new Error('version conflict'))
    const wrapper = await setup()
    await wrapper.get('textarea').setValue('keep me')
    await wrapper.get('.editor-save').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('version conflict')
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.vm.openFile(a)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('keep me')
    await wrapper.get('.editor-save').trigger('click')
    expect(mocks.write).toHaveBeenLastCalledWith(a.path, 'keep me', a.resourceVersion, 'remote-a')
  })
  it('ignores late reads after closing a loading tab and allows retry after a read failure', async () => {
    const pending = deferred<string>()
    mocks.text.mockReturnValueOnce(pending.promise)
    const wrapper = await setup()
    void wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.get('[aria-label="关闭 b.json"]').trigger('click')
    pending.resolve('late b')
    await flushPromises()
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    expect(wrapper.get('textarea').element.value).toBe('original a')
    mocks.text.mockRejectedValueOnce(new Error('offline'))
    await wrapper.vm.openFile(b)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('offline')
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('loaded /demo/b.json')
  })
  it('keeps same-tick switches associated with the mounted document', async () => {
    const wrapper = await setup()
    await wrapper.vm.openFile(b)
    await flushPromises()
    await wrapper.get('textarea').setValue('draft b')
    void wrapper.vm.openFile(a)
    void wrapper.vm.openFile(b)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('draft b')
    await wrapper.vm.openFile(a)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('original a')
  })
  it('bounds open tabs and supports keyboard tab navigation', async () => {
    const wrapper = await setup()
    for (let index = 0; index < 12; index++) {
      await wrapper.vm.openFile(entry(`${index}.txt`))
      await flushPromises()
    }
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(12)
    expect(wrapper.text()).toContain('最多打开 12 个文件')
    await wrapper.get('[aria-selected="true"]').trigger('keydown', { key: 'Home' })
    expect(wrapper.get('[aria-selected="true"]').text()).toContain('a.json')
    expect(mocks.text).toHaveBeenCalledTimes(11)
  })
  it('keeps the last directory on failure and ignores superseded directory responses', async () => {
    const wrapper = await setup()
    mocks.list.mockRejectedValueOnce(new Error('directory offline'))
    await wrapper.get('[aria-label="刷新文件列表"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.editor-file')).toHaveLength(2)
    expect(wrapper.get('[role="alert"]').text()).toContain('directory offline')
    const pending = deferred<unknown>()
    mocks.list.mockReturnValueOnce(pending.promise)
    void wrapper.vm.openFile({ ...a, kind: 'directory', path: '/slow' })
    mocks.list.mockResolvedValueOnce({ path: '/new', entries: [], offset: 0 })
    await wrapper.vm.openFile({ ...a, kind: 'directory', path: '/new' })
    pending.resolve({ path: '/slow', entries: [b], offset: 0 })
    await flushPromises()
    expect(wrapper.get('.editor-sidebar__path').text()).toBe('/new')
    expect(wrapper.text()).toContain('没有匹配的文件')
  })
})
