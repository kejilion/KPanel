// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HostTerminal from './HostTerminal.vue'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ close: vi.fn() }))
vi.mock('@/lib/api', async (original) => ({ ...await original<typeof import('@/lib/api')>(), api: { terminals: {
  close: mocks.close, output: () => new Promise(() => {}), resize: () => Promise.resolve({ accepted: true }),
} } }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {}
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
const props = { sessionId: 'retained-id', hostName: 'Local', initialOffset: 0 }

describe('HostTerminal close lifecycle', () => {
  beforeEach(() => {
    mocks.close.mockReset()
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('retains the ID across failed and unconfirmed attempts and skips duplicate unmount close', async () => {
    const wrapper = mount(HostTerminal, { props })
    const handle = wrapper.vm as unknown as { closeSession(): Promise<void> }
    mocks.close.mockRejectedValueOnce(new Error('timeout'))
    await expect(handle.closeSession()).rejects.toThrow('timeout')
    mocks.close.mockResolvedValueOnce({ closed: false })
    await expect(handle.closeSession()).rejects.toThrow('not confirmed')
    mocks.close.mockResolvedValueOnce({ closed: true })
    const first = handle.closeSession()
    expect(handle.closeSession()).toBe(first)
    await first
    wrapper.unmount()
    await flushPromises()
    expect(mocks.close.mock.calls).toEqual([['retained-id'], ['retained-id'], ['retained-id']])
  })

  it('accepts a confirmed missing session as idempotent close', async () => {
    mocks.close.mockRejectedValue(new ApiError('missing', 404, 'terminal_not_found'))
    const wrapper = mount(HostTerminal, { props })
    await (wrapper.vm as unknown as { closeSession(): Promise<void> }).closeSession()
    wrapper.unmount()
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(1)
  })

  it('handles best-effort unmount failure without an unhandled rejection', async () => {
    mocks.close.mockRejectedValue(new Error('offline'))
    const wrapper = mount(HostTerminal, { props })
    wrapper.unmount()
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(1)
  })
})
