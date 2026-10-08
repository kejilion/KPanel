// @vitest-environment jsdom

import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SystemTuningDialog from './SystemTuningDialog.vue'

const mocks = vi.hoisted(() => ({ status: vi.fn(), apply: vi.fn(), success: vi.fn(), danger: vi.fn() }))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: { system: { systemTuning: mocks.status, systemTuningAction: mocks.apply } },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const ids = ['system-update', 'system-cleanup', 'swap-1g', 'ssh-port-5522', 'ssh-defense', 'firewall-open-all', 'bbr', 'timezone-shanghai', 'dns-auto', 'ipv4-preferred', 'basic-tools', 'kernel-auto'] as const
const snapshot = {
  resourceVersion: 'a'.repeat(64), observedAt: '2026-08-11T08:00:00Z',
  items: ids.map((id) => ({ id, state: 'pending' as const })),
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
  return mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.status.mockResolvedValue(snapshot)
  mocks.apply.mockResolvedValue({ action: 'apply', items: ids, status: 'accepted', changed: true, message: 'queued', resourceVersion: 'a'.repeat(64), acceptedAt: '2026-08-11T08:01:00Z' })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('SystemTuningDialog', () => {
  it('recovers a running task after a transient read error and stops at completion', async () => {
    vi.useFakeTimers()
    const maintenance = { state: 'running', action: 'system-tuning', policy: `${'b'.repeat(64)}.bbr,kernel-auto`, stage: 'system_tuning_bbr', progress: 10 }
    mocks.status
      .mockResolvedValueOnce({ ...snapshot, maintenance })
      .mockRejectedValueOnce(new Error('temporary 503'))
      .mockResolvedValueOnce({ ...snapshot, maintenance: { ...maintenance, progress: 65 } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { ...maintenance, state: 'succeeded', progress: 100 } })
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_800)
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('正在执行 · 10%')
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.status).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.status).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('正在执行 · 65%')
    expect(wrapper.findAll('.tuning-item.is-selected')).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1_800)
    expect(wrapper.findAll('.tuning-item.is-complete')).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.status).toHaveBeenCalledTimes(4)
    expect(mocks.apply).not.toHaveBeenCalled()
  })

  it('retries a failed initial read without requiring a manual refresh', async () => {
    vi.useFakeTimers()
    mocks.status.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('无法读取系统综合调优状态。')
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper.text()).toContain('已选择 12/12 项')
    expect(mocks.status).toHaveBeenCalledTimes(2)
  })

  it.each(['close', 'unreadable', 'unmount'])('stops pending reads and recovery on %s', async (boundary) => {
    vi.useFakeTimers()
    const pending = deferred()
    mocks.status
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'system-tuning', progress: 10 } })
      .mockReturnValueOnce(pending.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_800)
    const signal = mocks.status.mock.calls[1]![0] as AbortSignal
    if (boundary === 'unmount') wrapper.unmount()
    else await wrapper.setProps(boundary === 'close' ? { open: false } : { readable: false })
    expect(signal.aborted).toBe(true)
    pending.reject(new Error('late 503'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.status).toHaveBeenCalledTimes(2)
  })

  it('ignores old policy and loading cleanup after rapid reopen', async () => {
    vi.useFakeTimers()
    const old = deferred()
    const fresh = deferred()
    mocks.status.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    expect((mocks.status.mock.calls[0]![0] as AbortSignal).aborted).toBe(true)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    old.resolve({ ...snapshot, maintenance: { state: 'succeeded', action: 'system-tuning', policy: `${'c'.repeat(64)}.bbr` } })
    await flushPromises()
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(true)
    expect(wrapper.find('.tuning-summary').exists()).toBe(false)
    fresh.resolve(snapshot)
    await flushPromises()
    expect(wrapper.text()).toContain('已选择 12/12 项')
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.status).toHaveBeenCalledTimes(2)
  })

  it.each(['succeeded', 'failed'])('preserves normal cadence and stops polling at %s', async (state) => {
    vi.useFakeTimers()
    const maintenance = { action: 'system-tuning', policy: `${'c'.repeat(64)}.bbr,kernel-auto`, stage: 'system_tuning_bbr', progress: 10, message: 'terminal result' }
    mocks.status
      .mockResolvedValueOnce({ ...snapshot, maintenance: { ...maintenance, state: 'running' } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { ...maintenance, state, progress: 100 } })
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_799)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.status).toHaveBeenCalledTimes(2)
    if (state === 'succeeded') expect(wrapper.text()).toContain('上次任务已完成')
    else expect(wrapper.find('.tuning-item.is-failed').text()).toContain('开启 BBR 加速')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.status).toHaveBeenCalledTimes(2)
  })

  it('waits for both open and readable before reading', async () => {
    const wrapper = mount(SystemTuningDialog, { props: { open: false, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(mocks.status).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true, readable: false })
    await flushPromises()
    expect(mocks.status).not.toHaveBeenCalled()
    await wrapper.setProps({ readable: true })
    await flushPromises()
    expect(mocks.status).toHaveBeenCalledTimes(1)
  })

  it('keeps confirmation cancellation free of submission or extra reads', async () => {
    vi.useFakeTimers()
    vi.mocked(window.confirm).mockReturnValue(false)
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.find('.tuning-footer .button').trigger('click')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(mocks.apply).not.toHaveBeenCalled()
    expect(mocks.status).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已选择 12/12 项')
  })

  it('stays open during the first load', async () => {
    mocks.status.mockReturnValueOnce(new Promise(() => undefined))
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('selects all 12 items by default and submits fixed IDs', async () => {
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('已选择 12/12 项')
    const apply = wrapper.findAll('button').find((button) => button.text().includes('一键调优'))!
    await apply.trigger('click')
    await flushPromises()
    expect(mocks.apply).toHaveBeenCalledWith({ action: 'apply', items: [...ids], expectedResourceVersion: 'a'.repeat(64) })
  })

  it('allows a single item to be unchecked and restores progress from a running task', async () => {
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.findAll('.tuning-item')[0]!.trigger('click')
    expect(wrapper.text()).toContain('已选择 11/12 项')
    mocks.status.mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'system-tuning', policy: `${'b'.repeat(64)}.bbr,kernel-auto`, stage: 'system_tuning_bbr', progress: 52, message: 'running', rebootRequired: false } })
    await wrapper.find('button[title="刷新调优状态"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('正在执行 · 52%')
    expect(wrapper.find('.tuning-item.is-running').text()).toContain('开启 BBR 加速')
  })

  it('marks the exact failed item without presenting later selected items as completed', async () => {
    mocks.status.mockResolvedValueOnce({
      ...snapshot,
      maintenance: {
        state: 'failed', action: 'system-tuning', policy: `${'c'.repeat(64)}.system-update,system-cleanup,swap-1g,dns-auto`,
        stage: 'system_tuning_swap-1g', progress: 100, message: '任务失败：swap activation failed', rebootRequired: false,
      },
    })
    const wrapper = mount(SystemTuningDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('.tuning-item.is-failed').text()).toContain('设置 1 GB 虚拟内存')
    expect(wrapper.findAll('.tuning-item.is-complete')).toHaveLength(2)
    expect(wrapper.findAll('.tuning-item')[3]!.classes()).not.toContain('is-complete')
    expect(wrapper.text()).toContain('任务失败：swap activation failed')
  })
})
