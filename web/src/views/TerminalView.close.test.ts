// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { desktopWindowCloseGuardKey } from '@/lib/desktopRouteKeys'
import TerminalView from './TerminalView.vue'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ close: vi.fn(), detach: vi.fn(), markDetached: vi.fn(), hosts: vi.fn(), open: vi.fn(), list: vi.fn() }))
let closeGuard: () => boolean | Promise<boolean>
function create() {
  return mount(TerminalView, { global: { provide: {
    [desktopWindowCloseGuardKey as symbol]: { register: (guard: typeof closeGuard) => { closeGuard = guard; return () => undefined } },
  } } })
}
afterEach(() => vi.restoreAllMocks())
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ query: {} }),
}))

vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: mocks.hosts }, terminals: { batches: () => Promise.resolve({ available: true, items: [] }), list: mocks.list, open: mocks.open, detach: mocks.detach } }, ApiError: class extends Error { constructor(message: string, public status: number, public code: string) { super(message) } } }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(_, { expose }) {
    expose({ closeSession: mocks.close, markDetached: mocks.markDetached, focusTerminal() {}, executeCommand() { return true }, scheduleResize() {} })
    return () => null
  },
}) }))

describe('explicit terminal tab close', () => {
  beforeEach(() => {
    mocks.close.mockReset()
    mocks.detach.mockReset()
    mocks.markDetached.mockReset()
    mocks.list.mockResolvedValue({ items: [] })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mocks.hosts.mockResolvedValue({ items: [{ id: 'local', name: 'Local host', isLocal: true, terminalAvailable: true }] })
    mocks.open.mockResolvedValue({ sessionId: 'same-session', offset: 0 })
  })

  it('removes only the local tab after explicit confirmation and a successful detach', async () => {
    mocks.close.mockRejectedValue(new Error('offline'))
    mocks.detach.mockResolvedValue({ detached: true })
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    vi.mocked(window.confirm).mockReturnValueOnce(false)
    await wrapper.get('[role="alert"] button:last-child').trigger('click')
    expect(mocks.detach).not.toHaveBeenCalled()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    await wrapper.get('[role="alert"] button:last-child').trigger('click')
    await flushPromises()
    expect(mocks.detach).toHaveBeenCalledWith('same-session')
    expect(mocks.markDetached).toHaveBeenCalledOnce()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    wrapper.unmount()
  })

  it.each([
    ['terminal_not_found', 0],
    ['route_not_found', 1],
  ])('only dismisses a missing session for the specific error %s', async (code, remaining) => {
    mocks.close.mockRejectedValue(new Error('offline'))
    mocks.detach.mockRejectedValue(new ApiError('missing', 404, code))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    await wrapper.get('[role="alert"] button:last-child').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(remaining)
    expect(mocks.markDetached).toHaveBeenCalledTimes(remaining === 0 ? 1 : 0)
    wrapper.unmount()
  })

  it.each([['terminal_not_found', 0], ['route_not_found', 1]])('handles close error %s without mistaking generic 404 for a missing session', async (code, remaining) => {
    mocks.close.mockRejectedValue(new ApiError('missing', 404, code))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(remaining)
    wrapper.unmount()
  })

  it('keeps the tab on failure, deduplicates clicks and removes only after retry confirms close', async () => {
    let reject!: (reason: Error) => void
    mocks.close.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = create()
    await flushPromises()
    const close = wrapper.get('[aria-label="关闭终端"]')
    await close.trigger('click')
    await close.trigger('click')
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(wrapper.find('[aria-label="正在关闭终端"]').exists()).toBe(true)
    reject(new Error('Agent disconnected'))
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('会话已保留')
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    mocks.close.mockResolvedValueOnce(undefined)
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(2)
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

  it('closes every terminal on explicit window close and preserves a failed one for retry', async () => {
    mocks.open.mockResolvedValueOnce({ sessionId: 'first', offset: 0, retained: true })
      .mockResolvedValueOnce({ sessionId: 'second', offset: 0, retained: true })
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[aria-label="在 Local host 上新建终端"]').trigger('click')
    await flushPromises()
    mocks.close.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('offline'))
    expect(await closeGuard()).toBe(false)
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(wrapper.text()).toContain('关闭未确认')
    mocks.close.mockResolvedValueOnce(undefined)
    expect(await closeGuard()).toBe(true)
    expect(mocks.close).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('keeps the 1.26 window confirmation and does not stop a cancelled close', async () => {
    const wrapper = create()
    await flushPromises()
    vi.mocked(window.confirm).mockReturnValue(false)
    expect(await closeGuard()).toBe(false)
    expect(mocks.close).not.toHaveBeenCalled()
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    wrapper.unmount()
  })

  it('waits for an in-flight open before closing the window', async () => {
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
    expect(mocks.close).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('waits for refresh restoration and closes the original session without opening a replacement', async () => {
    let resolve!: (value: unknown) => void
    mocks.list.mockReturnValue(new Promise(done => { resolve = done }))
    mocks.close.mockResolvedValue(undefined)
    const wrapper = create()
    await flushPromises()
    const closing = closeGuard()
    resolve({ items: [{ sessionId: 'original', hostId: 'local', retained: true, offset: 0 }] })
    expect(await closing).toBe(true)
    expect(mocks.open).not.toHaveBeenCalled()
    expect(mocks.close).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
