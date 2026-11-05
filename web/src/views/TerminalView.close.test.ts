// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { desktopWindowCloseGuardKey, desktopWindowIdentityKey, desktopWindowLaunchKey } from '@/lib/desktopRouteKeys'
import { resetWindowIdentityForTest } from '@/lib/windowSessionIdentities'
import TerminalView from './TerminalView.vue'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ close: vi.fn(), closeApi: vi.fn(), detach: vi.fn(), markDetached: vi.fn(), hosts: vi.fn(), open: vi.fn(), list: vi.fn() }))
let closeGuard: () => boolean | Promise<boolean>
function create(windowId?: string, fresh = true) {
  return mount(TerminalView, { global: { provide: {
    [desktopWindowCloseGuardKey as symbol]: { register: (guard: typeof closeGuard) => { closeGuard = guard; return () => undefined } },
    ...(windowId ? { [desktopWindowIdentityKey as symbol]: windowId, [desktopWindowLaunchKey as symbol]: () => fresh } : {}),
  } } })
}
afterEach(() => { resetWindowIdentityForTest(); Reflect.deleteProperty(navigator, 'locks'); vi.restoreAllMocks() })
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ query: {} }),
}))

vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: mocks.hosts }, terminals: { batches: () => Promise.resolve({ available: true, items: [] }), list: mocks.list, open: mocks.open, close: mocks.closeApi, detach: mocks.detach } }, ApiError: class extends Error { constructor(message: string, public status: number, public code: string) { super(message) } } }))
vi.mock('@/stores/session', () => ({ useSession: () => ({ state: { authenticated: true, user: { id: 'workspace-owner' } } }) }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(_, { expose }) {
    expose({ closeSession: mocks.close, markDetached: mocks.markDetached, focusTerminal() {}, executeCommand() { return true }, scheduleResize() {} })
    return () => null
  },
}) }))

describe('explicit terminal tab close', () => {
  beforeEach(() => {
    resetWindowIdentityForTest()
    Object.defineProperty(navigator, 'locks', { configurable: true, value: { request: (_name: string, _options: unknown, callback: (lock: Lock) => unknown) => Promise.resolve(callback({ name: 'test', mode: 'exclusive' } as Lock)) } })
    window.localStorage.clear()
    window.sessionStorage.clear()
    mocks.close.mockReset()
    mocks.closeApi.mockReset().mockResolvedValue({ closed: true })
    mocks.detach.mockReset().mockResolvedValue({ detached: true })
    mocks.markDetached.mockReset()
    mocks.list.mockResolvedValue({ items: [] })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mocks.hosts.mockResolvedValue({ items: [{ id: 'local', name: 'Local host', isLocal: true, terminalAvailable: true }] })
    mocks.open.mockResolvedValue({ sessionId: 'same-session', offset: 0 })
  })

  it('automatically detaches and removes the local tab when close fails', async () => {
    mocks.close.mockRejectedValue(new Error('offline'))
    mocks.detach.mockResolvedValue({ detached: true })
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    expect(window.confirm).not.toHaveBeenCalled()
    expect(mocks.detach).toHaveBeenCalledWith('same-session')
    expect(mocks.markDetached).toHaveBeenCalledOnce()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    wrapper.unmount()
  })

  it.each([
    ['terminal_not_found', 0],
    ['route_not_found', 0],
  ])('only dismisses an already missing session for the specific error %s', async (code, remaining) => {
    mocks.close.mockRejectedValue(new Error('offline'))
    mocks.detach.mockRejectedValue(new ApiError('missing', 404, code))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(remaining)
    expect(mocks.markDetached).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it.each([['terminal_not_found', 0], ['route_not_found', 0]])('closes locally after close error %s without mistaking generic 404 for a missing session', async (code, remaining) => {
    mocks.close.mockRejectedValue(new ApiError('missing', 404, code))
    mocks.detach.mockRejectedValue(new Error('unavailable'))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(remaining)
    wrapper.unmount()
  })

  it('dismisses immediately and deduplicates cleanup when the network is slow', async () => {
    mocks.detach.mockRejectedValueOnce(new Error('unavailable'))
    let reject!: (reason: Error) => void
    mocks.close.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = create()
    await flushPromises()
    const close = wrapper.get('[aria-label="关闭终端"]')
    await close.trigger('click')
    await close.trigger('click')
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    expect(wrapper.find('[aria-label="正在关闭终端"]').exists()).toBe(false)
    reject(new Error('Agent disconnected'))
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('allows closing with the keyboard', async () => {
    mocks.close.mockResolvedValue(undefined)
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    wrapper.unmount()
  })

  it('closes every terminal window even when one cleanup request fails', async () => {
    mocks.detach.mockRejectedValueOnce(new Error('unavailable'))
    mocks.open.mockResolvedValueOnce({ sessionId: 'first', offset: 0, retained: true })
      .mockResolvedValueOnce({ sessionId: 'second', offset: 0, retained: true })
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="在 Local host 上新建终端"]').trigger('click')
    await flushPromises()
    mocks.close.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('offline'))
    expect(await closeGuard()).toBe(true)
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('关闭未确认')
    wrapper.unmount()
  })

  it('closes the window without a confirmation prompt', async () => {
    const wrapper = create()
    await flushPromises()
    vi.mocked(window.confirm).mockReturnValue(false)
    mocks.close.mockResolvedValue(undefined)
    expect(await closeGuard()).toBe(true)
    expect(window.confirm).not.toHaveBeenCalled()
    expect(mocks.close).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('closes immediately and cleans up a late terminal open', async () => {
    let resolve!: (value: unknown) => void
    mocks.open.mockReturnValue(new Promise(done => { resolve = done }))
    mocks.close.mockResolvedValue(undefined)
    const wrapper = create()
    await flushPromises()
    const closing = closeGuard()
    await flushPromises()
    expect(mocks.close).not.toHaveBeenCalled()
    resolve({ sessionId: 'late', offset: 0, retained: true })
    expect(await closing).toBe(true)
    await flushPromises()
    expect(mocks.closeApi).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('does not close sessions returned by a restoration request after the window closes', async () => {
    let resolve!: (value: unknown) => void
    mocks.list.mockReturnValue(new Promise(done => { resolve = done }))
    mocks.close.mockResolvedValue(undefined)
    const wrapper = create()
    await flushPromises()
    const closing = closeGuard()
    resolve({ items: [{ sessionId: 'original', hostId: 'local', retained: true, detachable: true, workspaceVisible: true, offset: 0 }] })
    expect(await closing).toBe(true)
    await flushPromises()
    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.closeApi).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not request a global session list after closing during host loading', async () => {
    let resolve!: (value: unknown) => void
    mocks.hosts.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = create()
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    resolve({ items: [{ id: 'local', name: 'Local host', isLocal: true, terminalAvailable: true }] })
    await flushPromises()
    expect(mocks.list).not.toHaveBeenCalled()
    expect(mocks.closeApi).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('restores only this window’s known session when closed before its list loads', async () => {
    const id = 'a'.repeat(64)
    mocks.list.mockResolvedValueOnce({ items: [{ sessionId: id, hostId: 'local', retained: true, workspaceVisible: true, offset: 0 }] })
    const original = create('window-a')
    await flushPromises()
    original.unmount()
    mocks.list.mockReturnValue(new Promise(() => undefined))
    const other = create('window-b')
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.closeApi).not.toHaveBeenCalled()
    other.unmount()
    const restored = create('window-a', false)
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.closeApi).toHaveBeenCalledWith(id)
    restored.unmount()
    mocks.closeApi.mockClear()
    const reused = create('window-a', true)
    await flushPromises()
    expect(await closeGuard()).toBe(true)
    expect(mocks.closeApi).not.toHaveBeenCalled()
    reused.unmount()
  })

})
