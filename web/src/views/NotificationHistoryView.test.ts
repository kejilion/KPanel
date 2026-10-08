// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import View from './NotificationHistoryView.vue'
import { ApiError } from '@/lib/api'

const mocks = vi.hoisted(() => ({ list: vi.fn(), hosts: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error { constructor(message: string, public status = 0) { super(message) } }, api: { cluster: { notificationHistory: mocks.list, hosts: mocks.hosts } } }))
vi.mock('@/i18n/phrase', () => ({ usePhraseCatalog: vi.fn(), phraseCatalogVersion: { value: 1 }, translatePhrase: (s: string) => s }))
vi.mock('vue-router', () => ({ useRoute: () => reactive({ query: {} }) }))
const wrappers: ReturnType<typeof mount>[] = []
const event = { id: '9', hostId: 'local', hostName: '本机', isLocal: true, rule: 'cpu', kind: 'alert', message: 'CPU 95% > 90%', delivery: 'local_only', attempts: 0, createdAt: '2026-09-28T01:00:00Z' }
const page = { items: [event], hosts: [{ id: 'local', name: '本机', isLocal: true }], nextCursor: '9', retentionDays: 30, maxEvents: 2000, maxBytes: 4194304 }
async function open() {
  const wrapper = mount(View, { global: { stubs: {
    LoadingState: { template: '<div>Loading</div>' },
    ErrorState: { props: ['message'], template: '<div role="alert">{{ message }}<button @click="$emit(\'retry\')">retry</button></div>' },
    ModalDialog: { props: ['open'], template: '<div v-if="open" role="dialog"><slot /></div>' },
  } } })
  wrappers.push(wrapper); await flushPromises(); return wrapper
}
beforeEach(() => { vi.clearAllMocks(); mocks.list.mockResolvedValue(page); mocks.hosts.mockResolvedValue({ items: [] }) })
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('notification history', () => {
  it('labels automatic backup events and applies the backup category filter', async () => {
    mocks.list.mockResolvedValue({ ...page, items: [{ ...event, rule: 'backup', message: 'Automatic backup failed' }] })
    const wrapper = await open()
    expect(wrapper.get('.notification-history__type').text()).toContain('自动备份')
    const category = wrapper.findAll('select').find(select => select.find('option[value="backup"]').exists())!
    await category.setValue('backup')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ rule: 'backup' }), expect.any(AbortSignal))
  })
  it('matches OS icons by host ID, normalizes the local host and keeps a fallback for removed hosts', async () => {
    mocks.hosts.mockResolvedValue({ items: [
      { id: 'panel-node-id', isLocal: true, lastSnapshot: { telemetry: { osId: 'debian' } } },
      { id: 'remote-a', isLocal: false, lastSnapshot: { telemetry: { osId: 'ubuntu' } } },
    ] })
    mocks.list.mockResolvedValue({ ...page, items: [event,
      { ...event, id: '8', hostId: 'remote-a', isLocal: false },
      { ...event, id: '7', hostId: 'removed-host', isLocal: false },
    ] })
    const wrapper = await open()
    const icons = wrapper.findAll('.notification-history__host .os-identity__mark')
    expect(icons.map(icon => icon.attributes('title'))).toEqual(['Debian', 'Ubuntu', 'Linux'])
    expect(mocks.hosts).toHaveBeenCalledTimes(1)
    const signal = mocks.hosts.mock.calls[0]![0] as AbortSignal
    wrappers.pop()!.unmount()
    expect(signal.aborted).toBe(true)
  })
  it('keeps history usable when system metadata cannot be loaded', async () => {
    mocks.hosts.mockRejectedValue(new Error('host inventory unavailable'))
    const wrapper = await open()
    expect(wrapper.get('.notification-history__event').text()).toContain('CPU 95% > 90%')
    expect(wrapper.get('.os-identity__mark').attributes('title')).toBe('Linux')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
  it('shows typed fields inline and preserves original messages and delivery details', async () => {
    const messages = [
      '⚠️ [KPanel 集群告警]\n\n主机：本机\nCPU 使用率达到 95.0%\n阈值：90.0%\n\n时间：2026-09-28 09:00:00 (UTC+08:00)',
      '✅ [KPanel 集群通知]\n\n主机：本机\n已恢复：CPU 使用率 当前 6.2%\n\n时间：2026-09-28 09:10:00 (UTC+08:00)',
    ]
    mocks.list.mockResolvedValue({ ...page, items: [
      { ...event, message: messages[0], delivery: 'failed', provider: 'telegram', attempts: 2 },
      { ...event, id: '10', message: messages[1], kind: 'recovery', relatedEventId: event.id },
    ] })
    const wrapper = await open()
    const records = wrapper.findAll('.notification-history__event')
    expect(records).toHaveLength(2)
    records.forEach((record) => {
      expect(record.find('details, summary').exists()).toBe(false)
      expect(record.get('.notification-history__fields').isVisible()).toBe(true)
    })
    expect(records[0]!.text()).toContain('阈值90.0%')
    expect(records[0]!.text()).toContain('到达值95.0%')
    expect(records[0]!.text()).toContain('发送失败')
    expect(records[1]!.text()).toContain('当前值6.2%')
    await records[0]!.get('button').trigger('click')
    expect(wrapper.get('.notification-history__original > p').element.textContent).toBe(messages[0])
    expect(wrapper.get('[role="dialog"]').text()).toContain('发送次数: 2')
    expect(wrapper.get('[role="dialog"]').text()).toContain('发送失败会自动重试')
    await records[1]!.get('button').trigger('click')
    expect(wrapper.get('.notification-history__original > p').element.textContent).toBe(messages[1])
    expect(wrapper.get('[role="dialog"]').text()).toContain('关联告警编号: 9')
  })
  it('identifies invalid filters without reporting a storage failure', async () => {
    mocks.list.mockRejectedValueOnce(new ApiError('invalid filters', 400))
    const wrapper = await open()
    expect(wrapper.text()).toContain('筛选条件无效')
    expect(wrapper.text()).not.toContain('数据目录')
  })
  it('filters on the server and paginates the applied query', async () => {
    const wrapper = await open()
    expect(wrapper.text()).toContain('仅本地')
    await wrapper.findAll('select')[1]!.setValue('local'); await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ host: 'local', since: expect.any(String) }), expect.any(AbortSignal))
    await wrapper.get('input[type="search"]').setValue('CPU')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    mocks.list.mockResolvedValue({ ...page, items: [{ ...event, id: '8' }], nextCursor: '' })
    await wrapper.findAll('button').find(button => button.text() === '加载更多')!.trigger('click'); await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ host: 'local', search: 'CPU', cursor: '9' }), expect.any(AbortSignal))
    expect(wrapper.findAll('.notification-history__event')).toHaveLength(2)
  })
  it('shows empty and failed requests and allows retry', async () => {
    mocks.list.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await open()
    expect(wrapper.text()).toContain('通知记录暂时不可用')
    mocks.list.mockResolvedValue({ ...page, items: [], nextCursor: '' })
    await wrapper.get('[role="alert"] button').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('暂无符合条件的通知')
  })
  it('ignores an obsolete response after a filter change', async () => {
    let resolveFirst!: (value: typeof page) => void
    mocks.list.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
    const wrapper = await open()
    mocks.list.mockResolvedValue({ ...page, items: [], nextCursor: '' })
    await wrapper.findAll('select')[1]!.setValue('local'); await flushPromises()
    resolveFirst(page); await flushPromises()
    expect(wrapper.findAll('.notification-history__event')).toHaveLength(0)
    expect(wrapper.text()).toContain('暂无符合条件的通知')
  })
})
