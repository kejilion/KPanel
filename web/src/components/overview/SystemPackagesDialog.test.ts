// @vitest-environment jsdom

import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SystemPackagesDialog from './SystemPackagesDialog.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), action: vi.fn(), terminalOpen: vi.fn(), terminalInput: vi.fn(), terminalClose: vi.fn(),
  success: vi.fn(), danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    system: { packages: mocks.list, packagesAction: mocks.action },
    terminals: { open: mocks.terminalOpen, input: mocks.terminalInput, close: mocks.terminalClose },
  },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const items = [
  { id: 'curl', category: 'network', installed: true, launchable: false },
  { id: 'htop', category: 'monitor', installed: false, launchable: true },
  { id: 'ffmpeg', category: 'media', installed: false, launchable: false },
] as const
const snapshot = {
  manager: 'APT', items, resourceVersion: 'a'.repeat(64), observedAt: '2026-09-20T00:00:00Z',
  maintenance: { state: 'idle', progress: 0, rebootRequired: false },
}

enableAutoUnmount(afterEach)
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<unknown>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function mountDialog() {
  return mount(SystemPackagesDialog, {
    props: { open: true, readable: true, writable: true },
    global: { stubs: { teleport: true, HostTerminal: true } },
  })
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.list.mockResolvedValue(snapshot)
  mocks.action.mockResolvedValue({ status: 'accepted', message: 'queued' })
  mocks.terminalOpen.mockResolvedValue({ sessionId: 'terminal-1', hostId: 'local', offset: 0, createdAt: '2026-09-20T00:00:00Z' })
  mocks.terminalInput.mockResolvedValue({ accepted: true })
  mocks.terminalClose.mockResolvedValue({ closed: true })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('SystemPackagesDialog', () => {
  it('recovers a running task after a transient read error and stops at completion', async () => {
    vi.useFakeTimers()
    mocks.list
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 10 } })
      .mockRejectedValueOnce(new Error('temporary 503'))
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 65 } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'succeeded', action: 'packages', progress: 100 } })
    const wrapper = mount(SystemPackagesDialog, {
      props: { open: true, readable: true, writable: true },
      global: { stubs: { teleport: true, HostTerminal: true } },
    })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_800)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.packages-summary__progress').text()).toContain('10%')
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.list).toHaveBeenCalledTimes(3)
    expect(wrapper.find('.packages-summary__progress').text()).toContain('65%')
    await vi.advanceTimersByTimeAsync(1_800)
    expect(wrapper.find('.packages-summary__progress').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.list).toHaveBeenCalledTimes(4)
    expect(mocks.action).not.toHaveBeenCalled()
  })

  it('retries a failed initial read without requiring a manual refresh', async () => {
    vi.useFakeTimers()
    mocks.list.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(SystemPackagesDialog, {
      props: { open: true, readable: true, writable: true },
      global: { stubs: { teleport: true, HostTerminal: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('无法读取软件包状态。')
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.list).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper.text()).toContain('已安装 1/3 个常用工具')
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it.each(['close', 'unreadable', 'unmount'])('clears scheduled error recovery on %s', async (boundary) => {
    vi.useFakeTimers()
    mocks.list.mockRejectedValue(new Error('offline'))
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_800 * 8)
    expect(mocks.list).toHaveBeenCalledTimes(9)
    if (boundary === 'unmount') wrapper.unmount()
    else await wrapper.setProps(boundary === 'close' ? { open: false } : { readable: false })
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.list).toHaveBeenCalledTimes(9)
  })

  it.each(['close', 'unreadable', 'unmount'])('stops pending reads and recovery on %s', async (boundary) => {
    vi.useFakeTimers()
    const pending = deferred()
    mocks.list
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 10 } })
      .mockReturnValueOnce(pending.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_800)
    const signal = mocks.list.mock.calls[1]![0] as AbortSignal
    if (boundary === 'unmount') wrapper.unmount()
    else await wrapper.setProps(boundary === 'close' ? { open: false } : { readable: false })
    expect(signal.aborted).toBe(true)
    pending.reject(new Error('late 503'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it.each(['resolve', 'reject'])('ignores a stale %s after rapid reopen without clearing new loading', async (settlement) => {
    vi.useFakeTimers()
    const old = deferred()
    const fresh = deferred()
    mocks.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const wrapper = mountDialog()
    await flushPromises()
    const oldSignal = mocks.list.mock.calls[0]![0] as AbortSignal
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    expect(oldSignal.aborted).toBe(true)
    expect(mocks.list).toHaveBeenCalledTimes(1)
    if (settlement === 'resolve') old.resolve({ ...snapshot, manager: 'STALE' })
    else old.reject(new Error('stale failure'))
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(true)
    expect(wrapper.find('.packages-summary').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('无法读取软件包状态。')
    fresh.resolve({ ...snapshot, manager: 'FRESH' })
    await flushPromises()
    expect(wrapper.find('.packages-summary').text()).toContain('FRESH')
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('clears the scheduled poll while a manual refresh is pending', async () => {
    vi.useFakeTimers()
    const pending = deferred()
    const running = { ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 10 } }
    mocks.list.mockResolvedValueOnce(running).mockReturnValueOnce(pending.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(900)
    await wrapper.find('button[title="刷新软件包状态"]').trigger('click')
    await flushPromises()
    const signal = mocks.list.mock.calls[1]![0] as AbortSignal
    await vi.advanceTimersByTimeAsync(5_000)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(signal.aborted).toBe(false)
    pending.resolve(running)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.list).toHaveBeenCalledTimes(3)
  })

  it('reads fresh task state after a submission overtakes a manual refresh', async () => {
    vi.useFakeTimers()
    const old = deferred()
    mocks.list
      .mockResolvedValueOnce(snapshot)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 25 } })
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.findAll('.package-card')[1]!.find('.package-card__select').trigger('click')
    await wrapper.find('button[title="刷新软件包状态"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('.packages-footer .button')[1]!.trigger('click')
    await flushPromises()
    expect(mocks.action).toHaveBeenCalledTimes(1)
    expect((mocks.list.mock.calls[1]![0] as AbortSignal).aborted).toBe(true)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    old.resolve(snapshot)
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(3)
    expect(wrapper.find('.packages-summary__progress').text()).toContain('25%')
    expect(wrapper.findAll('.packages-footer .button')[1]!.attributes('disabled')).toBeDefined()
  })

  it.each(['idle', 'succeeded', 'failed'])('preserves normal cadence and stops polling at %s', async (state) => {
    vi.useFakeTimers()
    mocks.list
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'packages', progress: 10 } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state, action: 'packages', progress: 100, message: 'terminal result' } })
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.list).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.packages-summary__progress').exists()).toBe(false)
    if (state === 'failed') expect(wrapper.text()).toContain('terminal result')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('waits for both open and readable before reading', async () => {
    const wrapper = mount(SystemPackagesDialog, { props: { open: false, readable: true, writable: true }, global: { stubs: { teleport: true, HostTerminal: true } } })
    await flushPromises()
    expect(mocks.list).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true, readable: false })
    await flushPromises()
    expect(mocks.list).not.toHaveBeenCalled()
    await wrapper.setProps({ readable: true })
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(1)
  })

  it('keeps confirmation cancellation free of submission or extra reads', async () => {
    vi.useFakeTimers()
    vi.mocked(window.confirm).mockReturnValue(false)
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.findAll('.package-card')[1]!.find('.package-card__select').trigger('click')
    await wrapper.findAll('.packages-footer .button')[1]!.trigger('click')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(mocks.action).not.toHaveBeenCalled()
    expect(mocks.list).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已选择 1 项')
  })

  it('shows real install state and practical category icons', async () => {
    const wrapper = mount(SystemPackagesDialog, {
      props: { open: true, readable: true, writable: true },
      global: { stubs: { teleport: true, HostTerminal: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('已安装 1/3 个常用工具')
    expect(wrapper.findAll('[data-package-category]').map((item) => item.attributes('data-package-category'))).toEqual(['network', 'monitor', 'media'])
    await wrapper.find('.packages-filters').findAll('button')[2]!.trigger('click')
    expect(wrapper.findAll('.package-card')).toHaveLength(2)
    expect(wrapper.text()).toContain('htop')
    expect(wrapper.text()).toContain('ffmpeg')
  })

  it('submits only missing selected fixed IDs', async () => {
    const wrapper = mount(SystemPackagesDialog, {
      props: { open: true, readable: true, writable: true },
      global: { stubs: { teleport: true, HostTerminal: true } },
    })
    await flushPromises()
    await wrapper.findAll('.package-card')[1]!.find('.package-card__select').trigger('click')
    await wrapper.findAll('.packages-footer .button')[1]!.trigger('click')
    await flushPromises()
    expect(mocks.action).toHaveBeenCalledWith({ action: 'install', items: ['htop'], expectedResourceVersion: 'a'.repeat(64) })
  })

  it('starts launchable tools in an independent local terminal session', async () => {
    mocks.list.mockResolvedValueOnce({
      ...snapshot,
      items: items.map((item) => item.id === 'htop' ? { ...item, installed: true } : item),
    })
    const wrapper = mount(SystemPackagesDialog, {
      props: { open: true, readable: true, writable: true },
      global: { stubs: { teleport: true, HostTerminal: true } },
    })
    await flushPromises()
    await wrapper.find('.package-card__launch').trigger('click')
    await flushPromises()
    expect(mocks.terminalOpen).toHaveBeenCalledWith('local', 30, 120)
    expect(mocks.terminalInput).toHaveBeenCalledWith('terminal-1', 'aHRvcA0')
    expect(wrapper.text()).toContain('htop · 独立终端')
    expect(wrapper.findComponent({ name: 'HostTerminal' }).exists()).toBe(true)
  })
})
