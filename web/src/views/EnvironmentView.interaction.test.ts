// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import EnvironmentView from './EnvironmentView.vue'
import { api } from '@/lib/api'
import type { WebEnvironmentJob, WebEnvironmentSummary } from '@/types/api'

vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {},
  api: { webEnvironment: { summary: vi.fn(), jobs: vi.fn(), job: vi.fn(), start: vi.fn() } },
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ success: vi.fn(), danger: vi.fn() }) }))
vi.mock('vue-router', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => ({ query: { section: 'protection' } }) }))
vi.mock('@/components/apps/AppInteractiveTerminal.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/sites/SitesSectionTabs.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/i18n/phrase', () => ({ phraseCatalogVersion: { value: 0 }, translatePhrase: (value: string) => value, usePhraseCatalog: vi.fn() }))

const state: WebEnvironmentSummary = {
  protocolVersion: '1', state: 'installed', profile: 'full', health: 'healthy',
  webRoot: '/home/web', diskBytes: 1, siteCount: 1, databaseCount: 1, certificateCount: 1,
  composeValid: true, nginxValid: true, resourceVersion: `sha256:${'a'.repeat(64)}`,
  scriptVersion: 'test', latestBackup: '', portConflicts: [], components: [],
  protection: { fail2ban: true, fail2banInstalled: true, waf: false, cloudflare: false, ddos: false },
  optimization: { mode: 'custom', gzip: false, brotli: false, zstd: false }, observedAt: '2026-10-31T00:00:00Z',
}
const job: WebEnvironmentJob = {
  id: 'a'.repeat(32), action: 'backup.create', target: 'web', status: 'running', stage: 'running',
  progress: 25, message: 'running', createdAt: '2026-10-31T00:00:00Z',
}
let wrapper: VueWrapper | undefined
beforeEach(() => {
  window.localStorage.clear()
  vi.clearAllMocks()
  vi.useFakeTimers()
  vi.mocked(api.webEnvironment.summary).mockResolvedValue(structuredClone(state))
  vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [], total: 0 })
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
async function open() {
  wrapper = mount(EnvironmentView, { global: { stubs: {
    SitesSectionTabs: true, AppInteractiveTerminal: true, ModalDialog: true, RouterLink: true,
  } } })
  await flushPromises()
  return wrapper
}

describe('environment protection actions and background banner', () => {
  it('keeps installed protection controls available without an install/repair button', async () => {
    const view = await open()
    expect(view.text()).not.toContain('检查迁移并修复防护')
    expect(view.findAll('button').some(button => button.text() === '卸载')).toBe(true)
    expect(view.findAll('button').some(button => button.text() === '开启')).toBe(true)
    expect(api.webEnvironment.start).not.toHaveBeenCalled()
  })
  it('offers installation when protection is absent', async () => {
    vi.mocked(api.webEnvironment.summary).mockResolvedValue({ ...state, protection: { ...state.protection, fail2ban: false, fail2banInstalled: false } })
    const view = await open()
    expect(view.findAll('button').some(button => button.text() === '安装防护')).toBe(true)
  })
  it('dismisses only the current banner while polling continues and shows a different job', async () => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [job], total: 1 })
    vi.mocked(api.webEnvironment.job).mockResolvedValue({ ...job, progress: 50 })
    const view = await open()
    expect(view.find('.environment-job-banner').exists()).toBe(true)
    await view.get('.environment-job-banner button[aria-label="隐藏进度，任务继续运行"]').trigger('click')
    expect(view.find('.environment-job-banner').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(api.webEnvironment.job).toHaveBeenCalledWith(job.id, expect.any(AbortSignal))
    expect(view.find('.environment-job-banner').exists()).toBe(false)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, id: 'b'.repeat(32) }], total: 1 })
    await view.findAll('button').find(button => button.text().includes('刷新状态'))!.trigger('click')
    await flushPromises()
    expect(view.find('.environment-job-banner').exists()).toBe(true)
    expect(api.webEnvironment.start).not.toHaveBeenCalled()
  })

  it.each(['succeeded', 'failed', 'needs_attention'] as const)('remembers a dismissed %s result after reopening without hiding a new task', async (status) => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, status, progress: 100 }], total: 1 })
    const view = await open()
    await view.get('.environment-job-banner .icon-button').trigger('click')
    view.unmount()
    const reopened = await open()
    expect(reopened.find('.environment-job-banner').exists()).toBe(false)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, id: 'b'.repeat(32), status }], total: 1 })
    await reopened.findAll('button').find(button => button.text().includes('刷新状态'))!.trigger('click')
    await flushPromises()
    expect(reopened.find('.environment-job-banner').exists()).toBe(true)
    expect(api.webEnvironment.start).not.toHaveBeenCalled()
  })

  it('restores hidden running progress on reopen and reveals its failure after polling', async () => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [job], total: 1 })
    const view = await open()
    await view.get('.environment-job-banner .icon-button').trigger('click')
    view.unmount()
    const reopened = await open()
    expect(reopened.find('.environment-job-banner').exists()).toBe(true)
    await reopened.get('.environment-job-banner .icon-button').trigger('click')
    const failed = { ...job, status: 'failed' as const, progress: 100, message: '任务执行失败' }
    vi.mocked(api.webEnvironment.job).mockResolvedValue(failed)
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [failed], total: 1 })
    await vi.advanceTimersByTimeAsync(1200)
    await flushPromises()
    expect(reopened.get('.environment-job-banner').text()).toContain('任务执行失败')
    expect(reopened.get('.environment-job-banner .icon-button').attributes('title')).toContain('活动记录')
  })

  it('reveals a request for input after running progress was hidden', async () => {
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [job], total: 1 })
    vi.mocked(api.webEnvironment.job).mockResolvedValue({ ...job, status: 'waiting_input' })
    const view = await open()
    await view.get('.environment-job-banner .icon-button').trigger('click')
    await vi.advanceTimersByTimeAsync(1200)
    await flushPromises()
    expect(view.get('.environment-job-banner').text()).toContain('等待输入')
  })

  it('still hides a result when local storage is blocked', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota') })
    vi.mocked(api.webEnvironment.jobs).mockResolvedValue({ items: [{ ...job, status: 'failed' }], total: 1 })
    const view = await open()
    await view.get('.environment-job-banner .icon-button').trigger('click')
    expect(view.find('.environment-job-banner').exists()).toBe(false)
    await view.findAll('button').find(button => button.text().includes('刷新状态'))!.trigger('click')
    await flushPromises()
    expect(view.find('.environment-job-banner').exists()).toBe(false)
  })
})
