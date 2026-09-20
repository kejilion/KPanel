// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue(snapshot)
  mocks.action.mockResolvedValue({ status: 'accepted', message: 'queued' })
  mocks.terminalOpen.mockResolvedValue({ sessionId: 'terminal-1', hostId: 'local', offset: 0, createdAt: '2026-09-20T00:00:00Z' })
  mocks.terminalInput.mockResolvedValue({ accepted: true })
  mocks.terminalClose.mockResolvedValue({ closed: true })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('SystemPackagesDialog', () => {
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
