// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CronManagerDialog from './CronManagerDialog.vue'
import FirewallManagerDialog from './FirewallManagerDialog.vue'
import HostsManagerDialog from './HostsManagerDialog.vue'
import NetworkInterfacesDialog from './NetworkInterfacesDialog.vue'

const mocks = vi.hoisted(() => ({
  hosts: vi.fn(),
  cron: vi.fn(),
  networkInterfaces: vi.fn(),
  firewall: vi.fn(),
  resourceAction: vi.fn(),
  success: vi.fn(),
  danger: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  ApiError: class MockApiError extends Error {},
  api: {
    system: {
      hosts: mocks.hosts,
      cron: mocks.cron,
      networkInterfaces: mocks.networkInterfaces,
      firewall: mocks.firewall,
      resourceAction: mocks.resourceAction,
    },
  },
}))

vi.mock('@/stores/toast', () => ({
  useToast: () => ({ success: mocks.success, danger: mocks.danger }),
}))

const emptySnapshot = { resourceVersion: 'rv-1', entries: [], total: 0, truncated: false }
const commonProps = { open: false, readable: true, writable: true }

beforeEach(() => {
  vi.clearAllMocks()
  mocks.hosts.mockResolvedValue(emptySnapshot)
  mocks.cron.mockResolvedValue(emptySnapshot)
  mocks.networkInterfaces.mockResolvedValue(emptySnapshot)
  mocks.firewall.mockResolvedValue({
    resourceVersion: 'rv-firewall',
    backend: 'iptables-nft',
    inputPolicy: 'DROP',
    rules: [
      { line: 1, chain: 'INPUT', target: 'ACCEPT', protocol: 'tcp', source: '0.0.0.0/0', destination: '0.0.0.0/0', options: ['--dport', '443'], raw: '-A INPUT -p tcp --dport 443 -j ACCEPT' },
      { line: 2, chain: 'OUTPUT', target: 'ACCEPT', protocol: 'tcp', source: '0.0.0.0/0', destination: '0.0.0.0/0', options: ['--dport', '443'], raw: '-A OUTPUT -p tcp --dport 443 -j ACCEPT' },
      { line: 3, chain: 'FORWARD', target: 'DROP', protocol: 'all', source: '192.0.2.0/24', destination: '0.0.0.0/0', options: [], raw: '-A FORWARD -s 192.0.2.0/24 -j DROP' },
    ],
    total: 3,
    truncated: false,
    pingAllowed: true,
    ddosEnabled: false,
  })
  mocks.resourceAction.mockResolvedValue({
    action: 'hosts-add',
    status: 'succeeded',
    changed: true,
    message: 'applied',
    resourceVersion: 'rv-2',
    appliedAt: '2026-08-10T08:00:00Z',
  })
  vi.stubGlobal('confirm', vi.fn(() => true))
})

describe('overview system resource dialogs', () => {
  it('defers every collection request until its dialog opens', async () => {
    const cases = [
      [HostsManagerDialog, mocks.hosts],
      [CronManagerDialog, mocks.cron],
      [NetworkInterfacesDialog, mocks.networkInterfaces],
      [FirewallManagerDialog, mocks.firewall],
    ] as const

    for (const [component, request] of cases) {
      const wrapper = mount(component, {
        props: commonProps,
        global: { stubs: { teleport: true } },
      })
      await flushPromises()
      expect(request).not.toHaveBeenCalled()

      await wrapper.setProps({ open: true })
      await flushPromises()
      expect(request).toHaveBeenCalledTimes(1)
      wrapper.unmount()
      request.mockClear()
    }
  })

  it('does not request an unavailable old-Agent adapter', async () => {
    const wrapper = mount(HostsManagerDialog, {
      props: { open: true, readable: false, writable: false, unavailableReason: '需要升级 Agent' },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    expect(mocks.hosts).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('需要升级 Agent')
  })

  it('adds a typed Hosts entry with the latest resource version and rereads state', async () => {
    const wrapper = mount(HostsManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    const inputs = wrapper.findAll('input')
    await inputs[0]!.setValue('192.0.2.10')
    await inputs[1]!.setValue('app.internal app')
    await inputs[2]!.setValue('fixture')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.resourceAction).toHaveBeenCalledWith({
      action: 'hosts-add',
      address: '192.0.2.10',
      hostnames: ['app.internal', 'app'],
      comment: 'fixture',
      expectedResourceVersion: 'rv-1',
    })
    expect(mocks.hosts).toHaveBeenCalledTimes(2)
  })

  it('updates a Cron entry by line and rereads state', async () => {
    mocks.cron.mockResolvedValue({
      resourceVersion: 'cron-rv-1',
      entries: [{ line: 4, kind: 'job', expression: '0 3 * * *', command: '/usr/local/bin/job', raw: '0 3 * * * /usr/local/bin/job' }],
      total: 1,
      truncated: false,
    })
    const wrapper = mount(CronManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    await wrapper.find('button[aria-label="编辑定时任务"]').trigger('click')
    await wrapper.findAll('input')[1]!.setValue('/usr/local/bin/job --quiet')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.resourceAction).toHaveBeenCalledWith({
      action: 'cron-update',
      line: 4,
      expression: '0 3 * * *',
      command: '/usr/local/bin/job --quiet',
      expectedResourceVersion: 'cron-rv-1',
    })
    expect(mocks.cron).toHaveBeenCalledTimes(2)
  })

  it('keeps a loopback interface actionable and confirms before disabling it', async () => {
    mocks.networkInterfaces.mockResolvedValue({
      resourceVersion: 'interfaces-rv-1',
      entries: [{
        name: 'lo',
        state: 'up',
        macAddress: '00:00:00:00:00:00',
        addresses: ['127.0.0.1/8', '::1/128'],
        loopback: true,
        resourceVersion: 'lo-rv-1',
      }],
      total: 1,
      truncated: false,
    })
    const wrapper = mount(NetworkInterfacesDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    const disableButton = wrapper.findAll('button').find((button) => button.text().trim() === '停用')
    expect(disableButton?.attributes('disabled')).toBeUndefined()
    await disableButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('可能中断面板和网络连接'))
    expect(mocks.resourceAction).toHaveBeenCalledWith({
      action: 'network-interface-state',
      interfaceName: 'lo',
      enabled: false,
      expectedResourceVersion: 'lo-rv-1',
    })
    expect(mocks.networkInterfaces).toHaveBeenCalledTimes(2)
  })

  it('shows firewall state, defaults to inbound, and filters readable rules', async () => {
    const wrapper = mount(FirewallManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    expect(wrapper.find('.firewall-manager__status').exists()).toBe(true)
    expect(wrapper.findAll('.firewall-manager__tab')).toHaveLength(4)
    expect(wrapper.find('button[data-firewall-zone="inbound"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.findAll('.firewall-manager__parsed-rule')).toHaveLength(1)
    expect(wrapper.find('.firewall-manager__rule-list-head').text()).toContain('动作')
    expect(wrapper.find('.firewall-manager__rule-list-head').text()).toContain('来源')
    expect(wrapper.find('.firewall-manager__rule-list-head').text()).toContain('目标')
    const actionBadges = wrapper.find('.firewall-manager__rule-badges').text()
    expect(actionBadges.indexOf('入站')).toBeLessThan(actionBadges.indexOf('允许'))
    expect(wrapper.find('.firewall-manager__parsed-rules').text()).toContain('TCP · 端口 443')
    expect(wrapper.find('.firewall-manager__parsed-rules').text()).toContain('所有 IP')
    expect(wrapper.find('.firewall-manager__parsed-rules').text()).not.toContain('INPUT · ACCEPT · tcp')
    expect(wrapper.find<HTMLDetailsElement>('details.firewall-manager__raw').element.open).toBe(false)

    await wrapper.find('button[data-firewall-zone="all"]').trigger('click')
    expect(wrapper.findAll('.firewall-manager__parsed-rule')).toHaveLength(3)

    await wrapper.find('button[data-firewall-decision="block"]').trigger('click')
    expect(wrapper.findAll('.firewall-manager__parsed-rule')).toHaveLength(1)
    expect(wrapper.find('.firewall-manager__parsed-rule').text()).toContain('阻止')

    await wrapper.find('button[data-firewall-zone="outbound"]').trigger('click')
    expect(wrapper.findAll('.firewall-manager__parsed-rule')).toHaveLength(0)
    expect(wrapper.find('.firewall-manager__other').exists()).toBe(true)
  })

  it('keeps the inbound quick action simple while opening a port for TCP and UDP', async () => {
    const wrapper = mount(FirewallManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    expect(wrapper.find('.firewall-manager__status').text()).toContain('默认禁止外部访问')
    expect(wrapper.find('.firewall-manager__other').text()).toContain('其他功能选项')
    expect(wrapper.find('.firewall-manager__other').text()).toContain('快速添加规则')
    expect(wrapper.find('.firewall-manager__other').text()).not.toContain('防护开关、快速添加规则和高风险操作。')
    expect(wrapper.find<HTMLDetailsElement>('details.firewall-manager__advanced').element.open).toBe(false)

    const portCard = wrapper.findAll('.firewall-manager__quick-card')[0]
    const openButton = portCard!.findAll('button').find((button) => button.text().trim() === '允许')
    await openButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('TCP 与 UDP'))
    expect(mocks.resourceAction).toHaveBeenCalledWith({
      action: 'firewall-open-port',
      port: 443,
      expectedResourceVersion: 'rv-firewall',
    })
    expect(mocks.firewall).toHaveBeenCalledTimes(2)
  })

  it('discloses that the script-wide firewall action clears custom and Docker chains', async () => {
    const wrapper = mount(FirewallManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    const allPortsButton = wrapper.find('.firewall-manager__advanced-section--danger button')
    await allPortsButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('包括 Docker 链'))
    expect(mocks.resourceAction).toHaveBeenCalledWith({
      action: 'firewall-open-all',
      expectedResourceVersion: 'rv-firewall',
    })
  })

  it('disables the all-ports action when the inbound policy is unknown', async () => {
    mocks.firewall.mockResolvedValue({
      resourceVersion: 'rv-firewall',
      backend: 'iptables-nft',
      inputPolicy: 'UNKNOWN',
      rules: [],
      total: 0,
      truncated: false,
      pingAllowed: true,
      ddosEnabled: false,
    })
    const wrapper = mount(FirewallManagerDialog, {
      props: { ...commonProps, open: true },
      global: { stubs: { teleport: true } },
    })
    await flushPromises()

    const allPortsButton = wrapper.find('.firewall-manager__advanced-section--danger button')
    expect(allPortsButton.text()).toBe('无法确认当前状态')
    expect(allPortsButton.attributes('disabled')).toBeDefined()
    expect(mocks.resourceAction).not.toHaveBeenCalled()
  })
})
