// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DesktopServiceStatus from './DesktopServiceStatus.vue'
import { api } from '@/lib/api'
import type { ClusterHostList, SystemOverview } from '@/types/api'

vi.mock('@/lib/api', () => ({
  api: {
    overview: { get: vi.fn() },
    cluster: { hosts: vi.fn() },
  },
}))

const overview = {
  agent: { connected: true, readOnly: false, compatible: true },
  sites: { total: 3, healthy: 3, drifted: 0 },
  containers: { total: 5, running: 4, stopped: 1 },
  apps: { total: 11, installed: 2, running: 2, updateAvailable: 0 },
  services: [{ id: 'docker', name: 'Docker Engine', state: 'running' }],
} as unknown as SystemOverview

const cluster = {
  items: [{ state: 'online' }, { state: 'degraded' }],
  total: 2,
} as unknown as ClusterHostList

describe('DesktopServiceStatus', () => {
  it.each([
    [{ total: 1, healthy: 0, drifted: 0 }, '整体运行正常', 'healthy', '100%'],
    [{ total: 1, healthy: 0, drifted: 1 }, '需要关注', 'attention', '0%'],
    [undefined, '状态不完整', 'unknown', '0%'],
  ])('uses site reconciliation for the desktop summary: %j', async (sites, title, state, progress) => {
    const value = { ...overview, sites, containers: { total: 2, running: 2, stopped: 0 } }
    vi.mocked(api.overview.get).mockImplementation(async (_signal, onUpdate) => {
      onUpdate?.(value)
      return value
    })
    vi.mocked(api.cluster.hosts).mockResolvedValue({ items: [], total: 0 } as unknown as ClusterHostList)
    const wrapper = mount(DesktopServiceStatus)
    await flushPromises()

    expect(wrapper.find('.desktop-service-status__hero-copy strong').text()).toBe(title)
    const siteMetric = wrapper.find('.desktop-service-status__metric')
    expect(siteMetric.classes()).toContain(`desktop-service-status__metric--${state}`)
    expect(siteMetric.find('.desktop-service-status__metric-track i').attributes('style')).toContain(`width: ${progress}`)
    wrapper.unmount()
  })

  it('summarizes service groups and exposes route shortcuts', async () => {
    vi.mocked(api.overview.get).mockImplementation(async (_signal, onUpdate) => {
      onUpdate?.(overview)
      return overview
    })
    vi.mocked(api.cluster.hosts).mockResolvedValue(cluster)
    const open = vi.fn()
    const wrapper = mount(DesktopServiceStatus, { props: { onOpen: open } })
    await flushPromises()

    expect(wrapper.find('.desktop-service-status__hero-copy strong').text()).toBe('需要关注')
    expect(wrapper.find('.desktop-service-status__hero-copy small').attributes('title')).toBeTruthy()
    const containerMetric = wrapper.findAll('.desktop-service-status__metric')
      .find((metric) => metric.text().includes('容器'))
    expect(containerMetric?.text()).toContain('5')
    expect(containerMetric?.text()).toContain('运行中 4')
    expect(wrapper.findAll('.desktop-service-status__metric')).toHaveLength(4)
    expect(wrapper.find('.desktop-service-status__footer').exists()).toBe(false)

    const metrics = wrapper.findAll('.desktop-service-status__metric')
    expect(metrics[0]?.text()).toContain('网站')
    expect(metrics[1]?.text()).toContain('容器')
    expect(metrics[2]?.text()).toContain('已安装应用')
    expect(metrics[2]?.text()).toContain('运行中 2')
    expect(metrics[2]?.text()).not.toContain('共 11 个')
    expect(metrics[3]?.text()).toContain('集群')
    expect(metrics[3]?.text()).toContain('1/2')
    expect(metrics[3]?.text()).toContain('在线')
    expect(metrics[3]?.text()).not.toContain('1 个在线')
    expect(metrics.every((metric) => metric.find('.desktop-service-status__metric-summary').exists())).toBe(true)
    expect(metrics[0]?.find('.desktop-service-status__metric-icon--brand').exists()).toBe(true)
    expect(metrics[1]?.find('.desktop-service-status__metric-icon--blue').exists()).toBe(true)
    expect(metrics[2]?.find('.desktop-service-status__metric-icon--amber').exists()).toBe(true)
    expect(metrics[3]?.find('.desktop-service-status__metric-icon--violet').exists()).toBe(true)
    expect(metrics.every((metric) => Boolean(metric.find('.desktop-service-status__metric-copy small').attributes('title')))).toBe(true)

    await containerMetric?.trigger('click')
    expect(open).toHaveBeenCalledWith('/docker')
    wrapper.unmount()
  })

  it('keeps a failed cluster request from hiding the rest of the summary', async () => {
    vi.mocked(api.overview.get).mockImplementation(async (_signal, onUpdate) => {
      onUpdate?.(overview)
      return overview
    })
    vi.mocked(api.cluster.hosts).mockRejectedValue(new Error('cluster unavailable'))
    const wrapper = mount(DesktopServiceStatus)
    await flushPromises()

    expect(wrapper.find('.desktop-service-status__metric').text()).toContain('3')
    expect(wrapper.find('.desktop-service-status__metric').text()).toContain('待核对 0')
    expect(wrapper.find('.desktop-service-status__metric--unknown').text()).toContain('—')
    wrapper.unmount()
  })
})
