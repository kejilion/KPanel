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
afterEach(() => { wrapper?.unmount(); wrapper = undefined; localStorage.clear() })

async function selectHost(view: VueWrapper, id: string) {
  await view.get('.monitoring-host-trigger').trigger('click')
  await view.get(`[data-monitoring-host-id="${id}"]`).trigger('click')
  await flushPromises()
}

describe('monitoring host selection', () => {
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
    await wrapper.get('.monitoring-host-trigger').trigger('click')
    expect(wrapper.findAll('.monitoring-host-option')).toHaveLength(3)
    expect(wrapper.get(`[data-monitoring-host-id="${a}"]`).attributes('aria-pressed')).toBe('true')
    expect(wrapper.get(`[data-monitoring-host-id="${b}"]`).text()).toContain('离线')
    expect(wrapper.findAll('operating-system-icon-stub')).toHaveLength(3)
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
