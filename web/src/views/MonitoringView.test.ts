// @vitest-environment jsdom
import { shallowMount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MonitoringView from './MonitoringView.vue'
import type { ClusterHost, MonitoringHistory } from '@/types/api'

const mocks = vi.hoisted(() => ({ history: vi.fn(), hosts: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error {}, api: { monitoring: { history: mocks.history }, cluster: { hosts: mocks.hosts } } }))
vi.mock('@/i18n/phrase', () => ({ usePhraseCatalog: vi.fn(), phraseCatalogVersion: { value: 0 }, translatePhrase: (value: string) => value }))

const a = 'a'.repeat(32), b = 'b'.repeat(32)
function history(cpu = 10): MonitoringHistory {
  return { range: '6h', startedAt: '2026-09-11T00:00:00Z', endedAt: '2026-09-11T06:00:00Z', bucketSeconds: 300,
    host: [{ collectedAt: '2026-09-11T01:00:00Z', cpuPercent: cpu, cpuCores: 4, loadOne: 1, memoryTotalBytes: 100, memoryUsedBytes: 20, diskPercent: 10 }],
    containers: [], operatorLatency: [], storage: { retentionDays: 30, hostIntervalSeconds: 300, maxStorageBytes: 4194304 },
  } as unknown as MonitoringHistory
}
function deferred<T>() { let resolve!: (v: T) => void, reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
let wrapper: VueWrapper | undefined
async function mountAt(query = '') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/monitoring', component: MonitoringView }] })
  await router.push(`/monitoring${query}`)
  await router.isReady()
  wrapper = shallowMount(MonitoringView, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}
beforeEach(() => {
  mocks.history.mockReset().mockResolvedValue(history())
  mocks.hosts.mockReset().mockResolvedValue({ items: [
    { id: 'local', isLocal: true, name: '本机', state: 'online' },
    { id: a, isLocal: false, name: '远程 A', kind: 'panel', state: 'online' },
    { id: b, isLocal: false, name: '远程 B', kind: 'light_node', state: 'offline' },
  ] as ClusterHost[] })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; localStorage.clear(); vi.unstubAllGlobals(); delete (Element.prototype as Partial<Element>).scrollIntoView })

async function selectHost(view: VueWrapper, id: string) {
  await view.get('.monitoring-host-trigger').trigger('click')
  await view.get(`[data-monitoring-host-id="${id}"]`).trigger('click')
  await flushPromises()
}

describe('monitoring host selection', () => {
  it('renders Ping, TCP, and HTTP as three views on the shared timeline', async () => {
    const value = history()
    value.operatorLatency = [
      { id: 'ping-one', kind: 'ping', name: 'Ping 节点', address: '1.1.1.1', target: '1.1.1.1', points: [
        { collectedAt: '2026-09-11T00:30:00Z', latencyMilliseconds: 18, successCount: 1, failureCount: 0 },
        { collectedAt: '2026-09-11T01:00:00Z', latencyMilliseconds: null, successCount: 0, failureCount: 1 },
      ] },
      { id: 'tcp-one', kind: 'tcp', name: 'TCP 服务', address: 'example.com:443', target: 'example.com:443', points: [
        { collectedAt: '2026-09-11T00:30:00Z', latencyMilliseconds: 24, successCount: 1, failureCount: 0 },
      ] },
      { id: 'http-one', kind: 'http', name: 'HTTP 服务', address: 'https://example.com', target: 'https://example.com', points: [] },
    ]
    mocks.history.mockResolvedValue(value)
    const { wrapper } = await mountAt()
    expect(wrapper.text()).toContain('Ping 节点')
    expect(wrapper.text()).not.toContain('TCP 服务')
    expect(wrapper.find('.service-status-matrix').exists()).toBe(true)
    expect(wrapper.findAll('.service-status-row')).toHaveLength(1)
    expect(wrapper.find('.service-status-cell--success').exists()).toBe(true)
    expect(wrapper.find('.service-status-cell--failure').exists()).toBe(true)
    await wrapper.findAll('[role="tab"]').find((tab) => tab.text().includes('TCP'))!.trigger('click')
    expect(wrapper.text()).toContain('TCP 服务')
    expect(wrapper.text()).not.toContain('Ping 节点')
    expect(wrapper.findAll('.service-status-row')).toHaveLength(1)
  })

  it('switches between all, host, container and service check categories and remembers the choice', async () => {
    const { wrapper } = await mountAt()
    const sections = () => ({
      summary: wrapper.find('.summary-grid').exists(),
      host: wrapper.find('.chart-grid').exists(),
      containers: wrapper.find('.container-section').exists(),
      checks: wrapper.find('.service-check-card').exists(),
    })
    expect(wrapper.find('[data-monitoring-category="all"]').attributes('aria-selected')).toBe('true')
    expect(sections()).toEqual({ summary: true, host: true, containers: true, checks: true })
    await wrapper.get('[data-monitoring-category="containers"]').trigger('click')
    expect(sections()).toEqual({ summary: false, host: false, containers: true, checks: false })
    await wrapper.get('[data-monitoring-category="checks"]').trigger('click')
    expect(sections()).toEqual({ summary: false, host: false, containers: false, checks: true })
    await wrapper.get('[data-monitoring-category="host"]').trigger('click')
    expect(sections()).toEqual({ summary: true, host: true, containers: false, checks: false })
    expect(localStorage.getItem('kpanel:monitoring:category')).toBe('host')
    wrapper.unmount()
    const { wrapper: remounted } = await mountAt()
    expect(remounted.get('[data-monitoring-category="host"]').attributes('aria-selected')).toBe('true')
    expect(remounted.find('.container-section').exists()).toBe(false)
  })

  it('reveals host charts when a metric deep link arrives while another category is active', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    localStorage.setItem('kpanel:monitoring:category', 'checks')
    const { wrapper } = await mountAt('?metric=memory')
    await flushPromises()
    expect(wrapper.get('[data-monitoring-category="host"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('#host-memory-history').exists()).toBe(true)
  })

  it('keeps a manually chosen category when a metric deep link reloads history', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const { wrapper, router } = await mountAt('?metric=cpu')
    expect(wrapper.get('[data-monitoring-category="all"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-monitoring-category="containers"]').trigger('click')
    const reloaded = history()
    reloaded.host.push({ ...reloaded.host[0]!, collectedAt: '2026-09-11T01:05:00Z' })
    mocks.history.mockResolvedValue(reloaded)
    await router.push('/monitoring?metric=cpu&range=24h')
    await flushPromises()
    expect(mocks.history).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-monitoring-category="containers"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('.chart-grid').exists()).toBe(false)
  })

  it('orders the host picker from the panel preference', async () => {
    mocks.hosts.mockResolvedValueOnce({
      items: [
        { id: 'local', isLocal: true, name: '本机', state: 'online' },
        { id: a, isLocal: false, name: '远程 A', kind: 'panel', state: 'online' },
        { id: b, isLocal: false, name: '远程 B', kind: 'light_node', state: 'offline' },
      ] as ClusterHost[],
      hostOrder: {
        ids: [b, 'local', a],
        configured: true,
        resourceVersion: 'sha256:server-order',
      },
    })
    const { wrapper } = await mountAt(`?hostId=${a}`)

    await wrapper.get('.monitoring-host-trigger').trigger('click')

    expect(wrapper.findAll('[data-monitoring-host-id]').map((item) => item.attributes('data-monitoring-host-id'))).toEqual([
      b,
      'local',
      a,
    ])
  })

  it('cancels the previous host and ignores its late response', async () => {
    const first = deferred<MonitoringHistory>()
    mocks.history.mockReturnValueOnce(first.promise).mockResolvedValueOnce(history(75))
    const { wrapper } = await mountAt(`?hostId=${a}`)
    const oldSignal = mocks.history.mock.calls[0]?.[2] as AbortSignal
    await selectHost(wrapper, b)
    expect(oldSignal.aborted).toBe(true)
    expect(mocks.history.mock.calls.at(-1)?.[3]).toBe(b)
    first.resolve(history(11)); await flushPromises()
    expect(wrapper.text()).toContain('75%')
    expect(wrapper.text()).not.toContain('11%')
    expect(wrapper.text()).toContain('当前未在线')
    expect(wrapper.find('.container-section').exists()).toBe(true)
  })

  it('ignores a late error and clears old charts immediately when switching', async () => {
    const first = deferred<MonitoringHistory>(), second = deferred<MonitoringHistory>()
    mocks.history.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const { wrapper } = await mountAt(`?hostId=${a}`)
    await selectHost(wrapper, b)
    first.reject(new Error('old host failure')); await flushPromises()
    expect(wrapper.find('error-state-stub').exists()).toBe(false)
    second.resolve(history(66)); await flushPromises()
    expect(wrapper.text()).toContain('66%')
  })

  it('keeps all local capabilities and long ranges across hosts and browser back', async () => {
    const { wrapper, router } = await mountAt('?range=12m')
    expect(wrapper.findAll('.range-button')).toHaveLength(8)
    await selectHost(wrapper, a)
    expect(router.currentRoute.value.query).toEqual({ range: '12m', hostId: a })
    expect(wrapper.findAll('.range-button')).toHaveLength(8)
    expect(wrapper.text()).toContain('读写 I/O')
    router.back(); await flushPromises()
    expect(wrapper.get('.monitoring-host-trigger').text()).toContain('本机')
    expect(wrapper.findAll('.range-button')).toHaveLength(8)
    expect(wrapper.find('.container-section').exists()).toBe(true)
    expect(mocks.history.mock.calls.at(-1)?.[3]).toBeUndefined()
  })

  it('does not fall back to local history for an unknown host', async () => {
    mocks.history.mockRejectedValue(new Error('not found'))
    const { wrapper } = await mountAt(`?hostId=${'f'.repeat(32)}`)
    expect(wrapper.find('error-state-stub').exists()).toBe(true)
    expect(mocks.history).toHaveBeenCalledTimes(1)
    expect(mocks.history.mock.calls[0]?.[3]).toBe('f'.repeat(32))
    expect(wrapper.get('.monitoring-host-trigger').text()).toContain('所选主机（未在列表中）')
  })

  it('keeps history usable if the inventory fails and renders an honest empty state', async () => {
    mocks.hosts.mockRejectedValue(new Error('inventory unavailable'))
    mocks.history.mockResolvedValue({ ...history(), host: [] })
    const { wrapper } = await mountAt(`?hostId=${a}`)
    expect(wrapper.text()).toContain('主机列表读取失败')
    expect(wrapper.find('.summary-grid').exists()).toBe(false)
    expect(wrapper.find('empty-state-stub').attributes('title')).toBe('所选时间内暂无历史数据')
  })

  it('shows the shared icon, status and selection pattern, and searches locally without fetching history', async () => {
    const { wrapper } = await mountAt(`?hostId=${a}`)
    const trigger = wrapper.get('.monitoring-host-trigger')
    const triggerButton = trigger.element as HTMLButtonElement
    triggerButton.focus()
    await trigger.trigger('click')
    expect(document.activeElement).toBe(trigger.element)
    expect(wrapper.findAll('.monitoring-host-option')).toHaveLength(3)
    expect(wrapper.get(`[data-monitoring-host-id="${a}"]`).attributes('aria-pressed')).toBe('true')
    expect(wrapper.get(`[data-monitoring-host-id="${b}"]`).text()).toContain('离线')
    expect(wrapper.findAll('operating-system-icon-stub')).toHaveLength(3)
    const searchInput = wrapper.get('input[type="search"]').element as HTMLInputElement
    searchInput.focus()
    expect(document.activeElement).toBe(searchInput)
    await wrapper.get('input[type="search"]').setValue('远程 B')
    expect(wrapper.findAll('.monitoring-host-option')).toHaveLength(1)
    expect(mocks.history).toHaveBeenCalledTimes(1)
    await wrapper.get('input[type="search"]').setValue('missing-host')
    expect(wrapper.text()).toContain('没有匹配的主机')
    await wrapper.get('input[type="search"]').setValue('远程 B')
    await wrapper.get(`[data-monitoring-host-id="${b}"]`).trigger('click')
    await flushPromises()
    expect(mocks.history.mock.calls.at(-1)?.[3]).toBe(b)
    expect(wrapper.find('.monitoring-host-menu').exists()).toBe(false)
  })

  it('supports keyboard navigation, escape focus restoration and outside dismissal', async () => {
    const { wrapper } = await mountAt(`?hostId=${a}`)
    await wrapper.get('.monitoring-host-trigger').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get(`[data-monitoring-host-id="${a}"]`).element)
    await wrapper.get(`[data-monitoring-host-id="${a}"]`).trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(wrapper.get(`[data-monitoring-host-id="${b}"]`).element)
    await wrapper.get(`[data-monitoring-host-id="${b}"]`).trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(wrapper.find('.monitoring-host-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('.monitoring-host-trigger').element)
    await wrapper.get('.monitoring-host-trigger').trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(wrapper.find('.monitoring-host-menu').exists()).toBe(false)
  })
})
