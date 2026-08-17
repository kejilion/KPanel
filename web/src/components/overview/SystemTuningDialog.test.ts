// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.status.mockResolvedValue(snapshot)
  mocks.apply.mockResolvedValue({ action: 'apply', items: ids, status: 'accepted', changed: true, message: 'queued', resourceVersion: 'a'.repeat(64), acceptedAt: '2026-08-11T08:01:00Z' })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('SystemTuningDialog', () => {
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
