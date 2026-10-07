// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesktopMonitor from '@/components/desktop/DesktopMonitor.vue'
import { api } from '@/lib/api'
import type { SystemResourceSnapshot } from '@/lib/api'

vi.mock('@/lib/api', () => ({
  api: {
    system: {
      resources: vi.fn(),
    },
  },
}))

function makeOverview(): SystemResourceSnapshot {
  return {
    hostname: 'mock',
    os: 'Ubuntu 24.04 LTS',
    osId: 'ubuntu',
    osLike: ['debian'],
    kernel: '6.8',
    architecture: 'x86_64',
    timezone: 'Asia/Seoul',
    uptimeSeconds: 90061,
    observedAt: new Date().toISOString(),
    cpu: { value: 0.42, percent: 42, cores: 4, model: 'Intel' },
    memory: { value: 4294967296, total: 8589934592, percent: 50, unit: 'bytes' },
    disk: { value: 21474836480, total: 107374182400, percent: 20, unit: 'bytes' },
    load: { value: 1.5, one: 0.4, five: 0.7, fifteen: 0.9 },
    network: {
      receiveBytesPerSecond: 102400,
      transmitBytesPerSecond: 51200,
      rateAvailable: true,
      totalReceivedBytes: 18790481920,
      totalTransmittedBytes: 3221225472,
      tcpConnections: 10,
      udpConnections: 2,
    },
  }
}

describe('DesktopMonitor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.system.resources).mockResolvedValue(makeOverview())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads on a phone-width widget page', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query.includes('max-width'), media: query, addEventListener() {}, removeEventListener() {},
    }))
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    expect(api.system.resources).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.desktop-monitor__host').text()).toContain('Ubuntu 24.04 LTS')
    wrapper.unmount()
  })

  it('renders the monitor title', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    expect(wrapper.find('.desktop-monitor__header span').text()).toBe('服务器监控')
    wrapper.unmount()
  })

  it('shows the operating system and host identity without duplicating location', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()

    const host = wrapper.find('.desktop-monitor__host')
    expect(host.text()).toContain('Ubuntu 24.04 LTS')
    expect(host.text()).toContain('mock · x86_64')
    expect(host.find('.country-flag').exists()).toBe(false)
    expect(wrapper.emitted('snapshot')?.[0]?.[0]).toMatchObject({ timezone: 'Asia/Seoul' })
    wrapper.unmount()
  })

  it('shows CPU percentage and core count', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    const rows = wrapper.findAll('.desktop-monitor__row')
    const cpuRow = rows.find((row) => row.find('dt span').text() === 'CPU')
    expect(cpuRow?.find('dd').text()).toContain('42%')
    expect(cpuRow?.find('dd').text()).toContain('4')
    wrapper.unmount()
  })

  it('shows memory used/total', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    const rows = wrapper.findAll('.desktop-monitor__row')
    const memRow = rows.find((row) => row.find('dt span').text() === '内存')
    expect(memRow?.find('dd').text()).toContain('GB')
    wrapper.unmount()
  })

  it('renders progress tracks for cpu/memory/disk', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    const tracks = wrapper.findAll('.desktop-monitor__track')
    expect(tracks.length).toBeGreaterThanOrEqual(3)
    wrapper.unmount()
  })

  it('shows network receive/transmit rates', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    const rows = wrapper.findAll('.desktop-monitor__row')
    const netRow = rows.find((row) => row.find('dt span').text() === '网络')
    expect(netRow?.find('dd').text()).toContain('↓')
    expect(netRow?.find('dd').text()).toContain('↑')
    expect(netRow?.find('.desktop-monitor__network-total small').text()).toBe('累计')
    expect(netRow?.find('.desktop-monitor__network-total').text()).toContain('17.5 GB')
    expect(netRow?.find('.desktop-monitor__network-total').text()).toContain('3.0 GB')
    expect(api.system.resources).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('shows uptime formatted in days/hours', async () => {
    const wrapper = mount(DesktopMonitor)
    await new Promise((r) => setTimeout(r, 50))
    await nextTick()
    const rows = wrapper.findAll('.desktop-monitor__row')
    const upRow = rows.find((row) => row.find('dt span').text() === '运行时长')
    expect(upRow?.find('dd').text()).toContain('天')
    wrapper.unmount()
  })
})
