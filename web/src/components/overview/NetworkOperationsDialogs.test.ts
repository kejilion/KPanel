// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PortUsageDialog from './PortUsageDialog.vue'
import TrafficShutdownDialog from './TrafficShutdownDialog.vue'

const mocks = vi.hoisted(() => ({
  portUsage: vi.fn(),
  trafficShutdown: vi.fn(),
  trafficShutdownAction: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: { system: {
    portUsage: mocks.portUsage,
    trafficShutdown: mocks.trafficShutdown,
    trafficShutdownAction: mocks.trafficShutdownAction,
  } },
}))

vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: mocks.success, danger: mocks.danger }) }))

const trafficSnapshot = {
  resourceVersion: 'a'.repeat(64), enabled: false, health: 'disabled',
  rxBytes: 1024, txBytes: 2048, rxThresholdGiB: 0, txThresholdGiB: 0,
  resetDay: 0, observedAt: '2026-08-10T08:00:00Z',
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.portUsage.mockResolvedValue({
    resourceVersion: 'b'.repeat(64), total: 3, truncated: false, observedAt: '2026-08-10T08:00:00Z',
    entries: [
      { protocol: 'tcp', state: 'LISTEN', localAddress: '127.0.0.1', localPort: '8080', peerAddress: '0.0.0.0', peerPort: '*', process: 'users:(("nginx",pid=798910,fd=16),("nginx",pid=798909,fd=16)) ino:2669375 cgroup:/system.slice/docker-example.scope', pid: 798910, raw: 'tcp LISTEN fixture one' },
      { protocol: 'tcp', state: 'LISTEN', localAddress: '127.0.0.1', localPort: '8080', peerAddress: '0.0.0.0', peerPort: '*', process: 'nginx', pid: 798911, raw: 'tcp LISTEN fixture duplicate' },
      { protocol: 'tcp', state: 'LISTEN', localAddress: '0.0.0.0', localPort: '8443', peerAddress: '0.0.0.0', peerPort: '*', process: 'nginx', pid: 798910, raw: 'tcp LISTEN fixture two' },
    ],
  })
  mocks.trafficShutdown.mockResolvedValue(trafficSnapshot)
  mocks.trafficShutdownAction.mockResolvedValue({
    action: 'enable', status: 'succeeded', changed: true, message: 'applied',
    resourceVersion: 'c'.repeat(64), appliedAt: '2026-08-10T08:01:00Z',
  })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('network operations dialogs', () => {
  it('defers port usage until open and renders typed process data', async () => {
    const wrapper = mount(PortUsageDialog, {
      props: { open: false, readable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()
    expect(mocks.portUsage).not.toHaveBeenCalled()
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(mocks.portUsage).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('127.0.0.1:8080')
    expect(wrapper.find('.port-usage-owner__name').text()).toBe('nginx')
    expect(wrapper.text()).toContain('PID 798910, 798911')
    expect(wrapper.text()).toContain('2 个监听端口')
    expect(wrapper.findAll('.port-usage-group')).toHaveLength(1)
    expect(wrapper.findAll('.port-usage-item')).toHaveLength(2)
    expect(wrapper.text()).toContain('仅本机')
    expect(wrapper.text()).toContain('TCP 监听')
    expect(wrapper.findAll('details').every((details) => details.attributes('open') === undefined)).toBe(true)
  })

  it('explains missing process data and hides raw socket output in technical details', async () => {
    mocks.portUsage.mockResolvedValueOnce({
      resourceVersion: 'c'.repeat(64), total: 1, truncated: false, observedAt: '2026-08-10T08:00:00Z',
      entries: [{ protocol: 'udp', state: 'UNCONN', localAddress: '0.0.0.0', localPort: '443', peerAddress: '0.0.0.0', peerPort: '*', raw: 'udp UNCONN 0 0 0.0.0.0:443 0.0.0.0:*' }],
    })
    const wrapper = mount(PortUsageDialog, {
      props: { open: true, readable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('系统未返回占用程序')
    expect(wrapper.text()).toContain('所有 IPv4 地址')
    expect(wrapper.text()).toContain('UDP 监听')
    expect(wrapper.find('details').attributes('open')).toBeUndefined()
    expect(wrapper.find('details code').text()).toContain('UNCONN')
  })

  it('confirms shutdown and monthly reboot semantics before enabling', async () => {
    const wrapper = mount(TrafficShutdownDialog, {
      props: { open: true, readable: true, writable: true }, global: { stubs: { teleport: true } },
    })
    await flushPromises()
    const inputs = wrapper.findAll('input')
    await inputs[0]!.setValue('100')
    await inputs[1]!.setValue('200')
    await inputs[2]!.setValue('5')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('会立即关闭服务器'))
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('5 日 01:00 会重启服务器'))
    expect(mocks.trafficShutdownAction).toHaveBeenCalledWith({
      action: 'enable', expectedResourceVersion: 'a'.repeat(64),
      rxThresholdGiB: 100, txThresholdGiB: 200, resetDay: 5,
    })
    expect(mocks.trafficShutdown).toHaveBeenCalledTimes(2)
  })

  it('does not call an unavailable adapter', async () => {
    const wrapper = mount(TrafficShutdownDialog, {
      props: { open: true, readable: false, writable: false, unavailableReason: '需要升级脚本' },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()
    expect(mocks.trafficShutdown).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('需要升级脚本')
  })
})
