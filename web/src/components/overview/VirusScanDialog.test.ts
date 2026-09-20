// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.scan.mockResolvedValue(snapshot)
  mocks.action.mockResolvedValue({ status: 'accepted', taskId: 'scan-1', mode: 'important', message: 'queued', appliedAt: '2026-09-20T04:02:00Z' })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('VirusScanDialog', () => {
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
})
