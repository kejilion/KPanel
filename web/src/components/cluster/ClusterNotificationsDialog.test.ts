// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import dialogSource from './ClusterNotificationsDialog.vue?raw'
import Dialog from './ClusterNotificationsDialog.vue'
import english from '@/i18n/pages/ClusterNotifications/en-US'
import traditional from '@/i18n/pages/ClusterNotifications/zh-TW'
import { notificationPhrases } from '@/i18n/pages/ClusterNotifications/labels'

const mocks = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error {}, api: { cluster: { notifications: mocks.read, updateNotifications: mocks.save } } }))
vi.mock('@/i18n', () => ({ getLocale: () => 'zh-CN', useI18n: () => ({ locale: ref('zh-CN') }) }))
vi.mock('@/i18n/phrase', () => ({ usePhraseCatalog: vi.fn(), phraseCatalogVersion: { value: 1 }, translatePhrase: (s: string) => s }))

function snapshot() { return {
  enabled: false, locale: 'zh-CN', timezone: 'UTC', resourceVersion: 'v1', updatedAt: new Date().toISOString(),
  rules: { cpuEnabled: true, cpuThresholdPercent: 90, memoryEnabled: true, memoryThresholdPercent: 90, diskEnabled: true, diskThresholdPercent: 90, trafficEnabled: false, trafficThresholdMiBPerSecond: 100, trafficTotalReceivedEnabled: false, trafficTotalReceivedThresholdGiB: 100, trafficTotalSentEnabled: false, trafficTotalSentThresholdGiB: 100, sshLoginEnabled: true, hostOfflineEnabled: true },
  telegram: { configured: false, ready: false, status: 'not_configured' },
  resources: { certificateStatus: 'ready', containerStatus: 'ready', observedAt: new Date().toISOString(), stateCapacityReached: false,
    certificates: [{ id: 'a'.repeat(64), name: 'expired.test', expiresAt: '2020-01-01T00:00:00Z', known: true, maintenance: 'unknown' }],
    containers: [{ id: 'b'.repeat(64), name: 'important', state: 'running', health: 'healthy', resourceVersion: 'r1', known: true }, { id: 'c'.repeat(64), name: 'unknown', state: '', resourceVersion: '', known: false }],
  },
} }
const wrappers: ReturnType<typeof mount>[] = []
async function open() {
  const wrapper = mount(Dialog, { props: { open: true }, global: { stubs: { ModalDialog: { template: '<div><slot /><slot name="footer" /></div>' } } } })
  wrappers.push(wrapper); await flushPromises(); return wrapper
}
beforeEach(() => { vi.clearAllMocks(); mocks.read.mockResolvedValue(snapshot()); mocks.save.mockImplementation(async (input) => ({ ...snapshot(), ...input, resourceVersion: 'v2' })) })
afterEach(() => wrappers.splice(0).forEach((wrapper) => wrapper.unmount()))

describe('withdrawn local resource notifications', () => {
  it('keeps editable threshold values at the 14px control baseline', () => {
    const rule = dialogSource.match(/\.cluster-notifications__threshold input\s*\{([^}]+)\}/)?.[1]
    expect(rule).toMatch(/font-size:\s*14px;/)
  })
  it('hides resources even when a compatible server returns enabled rules and inventory', async () => {
    const value: any = snapshot()
    value.rules.resourceAlerts = { certificatesEnabled: true, containers: [{ id: 'b'.repeat(64), enabled: true }] }
    mocks.read.mockResolvedValue(value)
    const wrapper = await open()
    expect(wrapper.find('.resource-notifications').exists()).toBe(false)
    for (const text of ['本机资源提醒', 'expired.test', 'important', '启用证书到期提醒']) expect(wrapper.text()).not.toContain(text)
    for (const label of ['CPU 阈值百分比', '内存阈值百分比', '磁盘阈值百分比', '流量阈值', '累计接收阈值', '累计传送阈值', '启用主机掉线通知', '启用 SSH 登录通知']) expect(wrapper.find(`input[aria-label="${label}"]`).exists()).toBe(true)
    await wrapper.get('input[aria-label="CPU 阈值百分比"]').setValue(85)
    await wrapper.findAll('button').find((button) => button.text() === '保存设置')!.trigger('click'); await flushPromises()
    const input = mocks.save.mock.calls[0]![0]
    expect(input.rules.cpuThresholdPercent).toBe(85)
    expect(input.rules.resourceAlerts).toBeUndefined()
    expect(input.expectedResourceVersion).toBe('v1')
  })
  it('remains compatible with an old API without resource fields', async () => {
    const value: any = snapshot(); delete value.resources; mocks.read.mockResolvedValue(value)
    const wrapper = await open()
    expect(wrapper.text()).not.toContain('本机资源提醒')
    await wrapper.findAll('button').find((button) => button.text() === '保存设置')!.trigger('click'); await flushPromises()
    expect(mocks.save.mock.calls[0]![0].rules.resourceAlerts).toBeUndefined()
  })
  it('shows a read failure and recovers through retry', async () => {
    mocks.read.mockRejectedValueOnce(new Error('fixture unavailable')); const wrapper = await open()
    expect(wrapper.text()).toContain('暂时无法读取通知设置')
    await wrapper.findAll('button').find((button) => button.text() === '重试')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('事件通知')
    expect(wrapper.text()).not.toContain('本机资源提醒')
  })
  it('retains translated compatibility phrases', () => {
    for (const entries of [english, traditional]) { const catalog = new Map<string, string>(entries); for (const text of Object.values(notificationPhrases)) expect(catalog.get(text)?.trim()).toBeTruthy() }
    expect(new Set(english.map(([source]) => source))).toEqual(new Set(traditional.map(([source]) => source)))
  })
})
