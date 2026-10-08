// @vitest-environment jsdom

import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VirusScanDialog from './VirusScanDialog.vue'

const mocks = vi.hoisted(() => ({ scan: vi.fn(), action: vi.fn(), success: vi.fn(), danger: vi.fn() }))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: { system: { virusScan: mocks.scan, virusScanAction: mocks.action } },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const snapshot = {
  reportAvailable: true, reportPath: '/home/docker/clamav/log/scan.log', source: 'kpanel' as const,
  status: 'clean' as const, mode: 'important' as const, paths: ['/etc', '/var', '/usr', '/home', '/root'],
  scannedFiles: 128, infectedFiles: 0, errors: 0, findings: [], truncated: false,
  completedAt: '2026-09-20T04:00:00Z', observedAt: '2026-09-20T04:01:00Z',
  maintenance: { state: 'idle' as const, progress: 0, rebootRequired: false },
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
  return mount(VirusScanDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.scan.mockResolvedValue(snapshot)
  mocks.action.mockResolvedValue({ status: 'accepted', taskId: 'scan-1', mode: 'important', message: 'queued', appliedAt: '2026-09-20T04:02:00Z' })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('VirusScanDialog', () => {
  it('recovers a running scan after a transient read error and stops at completion', async () => {
    vi.useFakeTimers()
    mocks.scan
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'virus-scan', progress: 10 } })
      .mockRejectedValueOnce(new Error('temporary 503'))
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'virus-scan', progress: 65 } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'succeeded', action: 'virus-scan', progress: 100 } })
    const wrapper = mount(VirusScanDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2_000)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('扫描进行中 · 10%')
    await vi.advanceTimersByTimeAsync(1_999)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.scan).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('扫描进行中 · 65%')
    await vi.advanceTimersByTimeAsync(2_000)
    expect(wrapper.text()).toContain('未发现病毒')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.scan).toHaveBeenCalledTimes(4)
    expect(mocks.action).not.toHaveBeenCalled()
  })

  it('retries a failed initial read without requiring a manual refresh', async () => {
    vi.useFakeTimers()
    mocks.scan.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(VirusScanDialog, { props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('无法读取病毒扫描状态。')
    await vi.advanceTimersByTimeAsync(1_999)
    expect(mocks.scan).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper.text()).toContain('未发现病毒')
    expect(mocks.scan).toHaveBeenCalledTimes(2)
  })

  it.each(['close', 'unreadable', 'unmount'])('stops pending reads and recovery on %s', async (boundary) => {
    vi.useFakeTimers()
    const pending = deferred()
    mocks.scan
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'virus-scan', progress: 10 } })
      .mockReturnValueOnce(pending.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2_000)
    const signal = mocks.scan.mock.calls[1]![0] as AbortSignal
    if (boundary === 'unmount') wrapper.unmount()
    else await wrapper.setProps(boundary === 'close' ? { open: false } : { readable: false })
    expect(signal.aborted).toBe(true)
    pending.reject(new Error('late 503'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
  })

  it('ignores an old report and loading cleanup after rapid reopen', async () => {
    vi.useFakeTimers()
    const old = deferred()
    const fresh = deferred()
    mocks.scan.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    expect((mocks.scan.mock.calls[0]![0] as AbortSignal).aborted).toBe(true)
    expect(mocks.scan).toHaveBeenCalledTimes(1)
    old.resolve({ ...snapshot, status: 'infected', infectedFiles: 8 })
    await flushPromises()
    expect(mocks.scan).toHaveBeenCalledTimes(2)
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(true)
    expect(wrapper.text()).not.toContain('发现 8 个威胁')
    fresh.resolve(snapshot)
    await flushPromises()
    expect(wrapper.text()).toContain('未发现病毒')
    expect(wrapper.findComponent({ name: 'LoadingState' }).exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
  })

  it.each(['succeeded', 'failed'])('preserves normal cadence and stops polling at %s', async (state) => {
    vi.useFakeTimers()
    mocks.scan
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state: 'running', action: 'virus-scan', progress: 10 } })
      .mockResolvedValueOnce({ ...snapshot, maintenance: { state, action: 'virus-scan', progress: 100, message: 'terminal result' } })
    const wrapper = mountDialog()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1_999)
    expect(mocks.scan).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('扫描进行中')
    expect(wrapper.find('.virus-scan-footer .button').attributes('disabled')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(15_000)
    expect(mocks.scan).toHaveBeenCalledTimes(2)
  })

  it('waits for an open readable dialog before reading', async () => {
    const wrapper = mount(VirusScanDialog, { props: { open: false, readable: true, writable: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(mocks.scan).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(mocks.scan).toHaveBeenCalledTimes(1)
  })

  it('keeps confirmation cancellation free of submission or extra reads', async () => {
    vi.useFakeTimers()
    vi.mocked(window.confirm).mockReturnValue(false)
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.find('.virus-scan-footer .button').trigger('click')
    await vi.advanceTimersByTimeAsync(15_000)
    expect(window.confirm).toHaveBeenCalledTimes(1)
    expect(mocks.action).not.toHaveBeenCalled()
    expect(mocks.scan).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('未发现病毒')
  })

  it('shows the host report and explicit no-delete safety boundary', async () => {
    const wrapper = mount(VirusScanDialog, {
      props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('未发现病毒')
    expect(wrapper.text()).toContain('128')
    expect(wrapper.text()).toContain('不会自动删除或隔离文件')
  })

  it('submits only validated custom absolute paths after confirmation', async () => {
    const wrapper = mount(VirusScanDialog, {
      props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()
    await wrapper.findAll('.virus-scan-options button')[2]!.trigger('click')
    await wrapper.find('textarea').setValue('/srv/sites\n/home')
    await wrapper.find('.virus-scan-footer .button').trigger('click')
    await flushPromises()
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('2 个自定义目录'))
    expect(mocks.action).toHaveBeenCalledWith({ mode: 'custom', paths: ['/srv/sites', '/home'] })
  })

  it('does not query the host when the read capability is unavailable', async () => {
    const wrapper = mount(VirusScanDialog, {
      props: { open: true, readable: false, writable: false, unavailableReason: '需要更新 kejilion.sh' },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()
    expect(mocks.scan).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('需要更新 kejilion.sh')
  })

  it('keeps non-canonical custom paths disabled', async () => {
    const wrapper = mount(VirusScanDialog, {
      props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()
    await wrapper.findAll('.virus-scan-options button')[2]!.trigger('click')
    await wrapper.find('textarea').setValue('/srv/sites/../private')
    expect(wrapper.find('.virus-scan-footer .button').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.virus-scan-paths small').classes()).toContain('is-invalid')
  })
})
