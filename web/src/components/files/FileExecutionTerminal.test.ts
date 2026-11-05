// @vitest-environment jsdom
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileExecutionTerminal from './FileExecutionTerminal.vue'
import type { FileEntry, TerminalSession } from '@/types/api'

const mocks = vi.hoisted(() => ({ entry: vi.fn(), open: vi.fn(), close: vi.fn(), detach: vi.fn(), markDetached: vi.fn(), closeOnUnload: vi.fn(), execute: vi.fn(), childClose: vi.fn(), owner: { authenticated: true, user: { id: 'owner-a' } } }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error { code = '' },
  api: { files: { entry: mocks.entry }, terminals: { open: mocks.open, close: mocks.close, detach: mocks.detach, closeOnUnload: mocks.closeOnUnload } },
}))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: mocks.owner }) }))
const HostTerminal = defineComponent({
  name: 'HostTerminal', props: ['sessionId', 'hostName', 'initialOffset'], emits: ['stateChange'],
  setup(_, { expose }) { expose({ executePresentedCommand: mocks.execute, closeSession: mocks.childClose, markDetached: mocks.markDetached }); return () => null },
})
const file: FileEntry = { path: '/home/script.sh', name: 'script.sh', kind: 'file', mode: '-rw-r--r--', resourceVersion: 'v1', sizeBytes: 5, owner: 'root', group: 'root', modifiedAt: '', editable: true, previewable: true }
const opened: TerminalSession = { sessionId: 'session-1', hostId: 'remote-1', offset: 0, createdAt: '' }
const wrappers: VueWrapper[] = []
function create() {
  const wrapper = mount(FileExecutionTerminal, {
    props: { entry: file, hostId: 'remote-1', hostName: 'Remote server' },
    global: { stubs: { HostTerminal, ModalDialog: { name: 'ModalDialog', props: ['closeDisabled'], emits: ['close'], template: '<div><slot /></div>' } } },
  })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.owner.user.id = 'owner-a'
  window.localStorage.clear()
  mocks.entry.mockResolvedValue(file)
  mocks.open.mockResolvedValue(opened)
  mocks.close.mockResolvedValue({ closed: true })
  mocks.detach.mockResolvedValue({ detached: true })
  mocks.execute.mockReturnValue(true)
  mocks.childClose.mockResolvedValue(undefined)
})
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount() })

describe('file execution terminal lifecycle', () => {
  it('dismisses immediately and continues cleanup after a slow close fails', async () => {
    let reject!: (reason: Error) => void
    mocks.childClose.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = create()
    await flushPromises()
    wrapper.findComponent({ name: 'ModalDialog' }).vm.$emit('close')
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    reject(new Error('offline'))
    await flushPromises()
    expect(mocks.detach).toHaveBeenCalledWith(opened.sessionId)
  })
  it('keeps late cleanup with the original account after switching users', async () => {
    let resolve!: (value: TerminalSession) => void
    mocks.open.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = create()
    await flushPromises()
    wrapper.unmount()
    mocks.owner.user.id = 'owner-b'
    const id = 'a'.repeat(64)
    resolve({ ...opened, sessionId: id })
    await flushPromises()
    expect(mocks.close).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner-a:${id}`)).toBe('1')
    expect(window.localStorage.getItem(`kpanel:pending-terminal-closes:owner-b:${id}`)).toBeNull()
  })
  it('closes silently after recording an unconfirmed terminal for cleanup', async () => {
    mocks.childClose.mockRejectedValueOnce(new Error('offline'))
    const wrapper = create()
    await flushPromises()
    wrapper.findComponent({ name: 'ModalDialog' }).vm.$emit('close')
    await flushPromises()
    expect(mocks.detach).toHaveBeenCalledWith('session-1')
    expect(mocks.markDetached).toHaveBeenCalledOnce()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('reads and opens the exact host, submits once after connection, and never replays on reconnect', async () => {
    const wrapper = create()
    await flushPromises()
    expect(mocks.entry).toHaveBeenCalledWith(file.path, expect.any(AbortSignal), 'remote-1')
    expect(mocks.open).toHaveBeenCalledWith('remote-1', 30, 120)
    expect(mocks.execute).not.toHaveBeenCalled()
    const terminal = wrapper.findComponent(HostTerminal)
    terminal.vm.$emit('stateChange', 'connected')
    terminal.vm.$emit('stateChange', 'connected')
    await flushPromises()
    terminal.vm.$emit('stateChange', 'reconnecting')
    terminal.vm.$emit('stateChange', 'connected')
    await flushPromises()
    expect(mocks.execute).toHaveBeenCalledTimes(1)
    expect(mocks.execute.mock.calls[0]![0]).toContain("command bash './script.sh'")
    expect(mocks.execute.mock.calls[0]![1]).toBe('cd -- /home && bash ./script.sh')
  })

  it('refuses a replaced file before opening a terminal', async () => {
    mocks.entry.mockResolvedValue({ ...file, resourceVersion: 'v2' })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.text()).toContain('文件已变化')
    expect(mocks.open).not.toHaveBeenCalled()
  })

  it('retains the target if the pane changes hosts while opening', async () => {
    let resolveEntry!: (value: FileEntry) => void
    mocks.entry.mockReturnValue(new Promise((resolve) => { resolveEntry = resolve }))
    const wrapper = create()
    await wrapper.setProps({ hostId: 'another-host' })
    resolveEntry(file)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledWith('remote-1', 30, 120)
  })

  it('reclaims a late session after the user closes during connection', async () => {
    let resolveOpen!: (value: TerminalSession) => void
    mocks.open.mockReturnValue(new Promise((resolve) => { resolveOpen = resolve }))
    const wrapper = create()
    await flushPromises()
    wrapper.findComponent({ name: 'ModalDialog' }).vm.$emit('close')
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    resolveOpen(opened)
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith(opened.sessionId)
    expect(mocks.execute).not.toHaveBeenCalled()
  })

  it('does not open a session when closed during metadata validation', async () => {
    let resolveEntry!: (value: FileEntry) => void
    mocks.entry.mockReturnValue(new Promise((resolve) => { resolveEntry = resolve }))
    const wrapper = create()
    wrapper.findComponent({ name: 'ModalDialog' }).vm.$emit('close')
    await flushPromises()
    resolveEntry(file)
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
  })

  it('reports open failure without submitting a command', async () => {
    mocks.open.mockRejectedValue(new Error('offline'))
    const wrapper = create()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('终端启动失败')
    expect(mocks.execute).not.toHaveBeenCalled()
  })

  it('closes locally when both close and cleanup registration fail', async () => {
    const wrapper = create()
    await flushPromises()
    mocks.childClose.mockRejectedValueOnce(new Error('offline'))
    mocks.detach.mockRejectedValueOnce(new Error('offline'))
    const modal = wrapper.findComponent({ name: 'ModalDialog' })
    modal.vm.$emit('close')
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('终端关闭失败')
  })
})
