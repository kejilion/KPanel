// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalView from './TerminalView.vue'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ list: vi.fn(), open: vi.fn(), hosts: vi.fn(), close: vi.fn(), detach: vi.fn(), closeOnUnload: vi.fn() }))
vi.mock('vue-router', async (original) => ({ ...await original<typeof import('vue-router')>(), useRoute: () => ({ path: '/terminal', query: {} }) }))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: mocks.hosts }, terminals: { batches: () => Promise.resolve({ available: true, items: [] }), list: mocks.list, open: mocks.open, close: mocks.close, detach: mocks.detach, closeOnUnload: mocks.closeOnUnload } }, ApiError: class extends Error { code = '' } }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  name: 'HostTerminalStub', props: ['sessionId', 'retained', 'initialOffset'],
  setup(_, { expose }) { expose({ closeSession: async () => {}, focusTerminal() {}, executeCommand() { return true }, scheduleResize() {} }); return () => null },
}) }))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.hosts.mockResolvedValue({ items: [{ id: 'local', name: 'Local', isLocal: true, terminalAvailable: true }] })
  mocks.detach.mockResolvedValue({ detached: true })
  mocks.open.mockResolvedValue({ sessionId: 'new', hostId: 'local', retained: true, offset: 0 })
})

describe('retained terminal restoration', () => {
  it('reclaims a temporary connection that completes after unmount', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('old node', 409, 'terminal_retention_unavailable'), { code: 'terminal_retention_unavailable' }))
    let resolve!: (value: unknown) => void
    mocks.open.mockReturnValueOnce(new Promise(done => { resolve = done }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    mocks.close.mockResolvedValueOnce({ closed: false })
    resolve({ sessionId: 'late', retained: false })
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledWith('late')
    expect(mocks.detach).toHaveBeenCalledWith('late')
  })
  it.each(['terminal_retention_unavailable'])('automatically falls back once after %s', async (code) => {
    mocks.list.mockResolvedValue({ items: [] })
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('old node'), { code }))
    mocks.open.mockResolvedValue({ sessionId: 'temporary', hostId: 'local', retained: false, offset: 0 })
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(mocks.open).toHaveBeenNthCalledWith(1, 'local', 30, 120, { retained: true })
    expect(mocks.open).toHaveBeenLastCalledWith('local', 30, 120, { retained: false })
    expect(wrapper.findComponent({ name: 'HostTerminalStub' }).props('retained')).toBe(false)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it.each(['network_error', 'terminal_open_failed', 'forbidden', 'terminal_limit'])('does not retry %s', async (code) => {
    mocks.list.mockResolvedValue({ items: [] })
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('failed'), { code }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(wrapper.find('[role="alert"] button').exists()).toBe(false)
    expect(mocks.open).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('stops after a failed temporary fallback', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('unsupported'), { code: 'terminal_retention_unavailable' }))
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('failed'), { code: 'terminal_open_failed' }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    wrapper.unmount()
  })
  it('does not start fallback after unmount', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    let reject!: (reason: unknown) => void
    mocks.open.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    wrapper.unmount()
    reject(Object.assign(new ApiError('old node'), { code: 'terminal_retention_unavailable' }))
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
  })
  it('keeps supported hosts retained without fallback', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent({ name: 'HostTerminalStub' }).props('retained')).toBe(true)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('restores the same local and remote shortcut sessions without replay or new opens', async () => {
    mocks.list.mockResolvedValue({ items: [
      { sessionId: 'original', hostId: 'local', offset: 123, retained: true, detachable: true, workspaceVisible: true },
      { sessionId: 'remote', hostId: 'offline-node', offset: 456, retained: true, detachable: true, workspaceVisible: true, title: 'Saved command', shortcutId: 'a'.repeat(32), closeRequested: true },
    ] })
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(mocks.detach).toHaveBeenCalledWith('remote')
    expect(wrapper.findComponent({ name: 'HostTerminalStub' }).props()).toMatchObject({ sessionId: 'original', retained: true, initialOffset: 0 })
    expect(wrapper.text()).not.toContain('关闭未确认')
    wrapper.unmount()
  })
  it('dismisses a previously closed temporary session and retries background cleanup', async () => {
    mocks.list.mockResolvedValue({ items: [{ sessionId: 'pending-temporary', hostId: 'local', retained: false, closeRequested: true, detachable: true, workspaceVisible: true }] })
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(mocks.detach).toHaveBeenCalledWith('pending-temporary')
    expect(wrapper.findComponent({ name: 'HostTerminalStub' }).props('sessionId')).toBe('new')
    expect(wrapper.text()).not.toContain('关闭未确认')
    wrapper.unmount()
  })
  it('does not restore container or unclassified cleanup records into the host workspace', async () => {
    mocks.list.mockResolvedValue({ items: [
      { sessionId: 'container', hostId: 'local', retained: false, closeRequested: true },
      { sessionId: 'uncertain', hostId: 'local', retained: true, closeRequested: true },
    ] })
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(wrapper.findComponent({ name: 'HostTerminalStub' }).props('sessionId')).toBe('new')
    expect(wrapper.text()).not.toContain('移除本地页签')
    wrapper.unmount()
  })
  it('blocks automatic and manual opens after lookup failure until restoration succeeds', async () => {
    mocks.list.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('.terminal-host').trigger('click')
    expect(mocks.open).not.toHaveBeenCalled()
    mocks.list.mockResolvedValue({ items: [{ sessionId: 'original', hostId: 'local', offset: 0, retained: true, detachable: true, workspaceVisible: true }] })
    await wrapper.get('[aria-label="刷新连接"]').trigger('click')
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    wrapper.unmount()
  })
  it('does not open after unmount while the session lookup is pending', async () => {
    let resolve!: (value: unknown) => void
    mocks.list.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    wrapper.unmount()
    resolve({ items: [] })
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
  })
})
